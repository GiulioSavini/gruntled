package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
)

// blastDoc mirrors the Blast JSON table in docs/cli.md (version 2) for
// `gruntled blast`. decodeBlast rejects unknown keys, so a key added to the
// output is noticed here. daemon_baseline is left out on purpose: it is
// documented as never present in `gruntled blast` output.
type blastDoc struct {
	Version  int             `json:"version"`
	Kind     string          `json:"kind"`
	Baseline bool            `json:"baseline"`
	Note     *string         `json:"note,omitempty"`
	Broken   []blastBroken   `json:"broken"`
	Impacted []blastImpacted `json:"impacted"`
	Changes  []blastChange   `json:"changes"`
	Summary  blastSummary    `json:"summary"`
}

type blastBroken struct {
	Unit     string         `json:"unit"`
	Findings []blastFinding `json:"findings"`
	Distance *int           `json:"distance,omitempty"`
	Source   *string        `json:"source,omitempty"`
	Via      *string        `json:"via,omitempty"`
}

type blastFinding struct {
	Code     string `json:"code"`
	Severity string `json:"severity"`
	File     string `json:"file"`
	Line     int    `json:"line"`
	Column   int    `json:"column"`
	Message  string `json:"message"`
}

type blastImpacted struct {
	Unit             string   `json:"unit"`
	Module           string   `json:"module"`
	Distance         int      `json:"distance"`
	Source           string   `json:"source"`
	Via              *string  `json:"via,omitempty"`
	AddedVariables   []string `json:"added_variables,omitempty"`
	RemovedVariables []string `json:"removed_variables,omitempty"`
	AddedOutputs     []string `json:"added_outputs,omitempty"`
	RemovedOutputs   []string `json:"removed_outputs,omitempty"`
}

type blastChange struct {
	Module           string            `json:"module"`
	AddedVariables   []string          `json:"added_variables"`
	RemovedVariables []string          `json:"removed_variables"`
	AddedOutputs     []string          `json:"added_outputs"`
	RemovedOutputs   []string          `json:"removed_outputs"`
	TypeChanges      []blastTypeChange `json:"type_changes"`
}

type blastTypeChange struct {
	Kind string `json:"kind"`
	Name string `json:"name"`
	Old  string `json:"old"`
	New  string `json:"new"`
}

type blastSummary struct {
	Broken   int `json:"broken"`
	Impacted int `json:"impacted"`
}

// blastRun runs `gruntled blast --base base cur` in process, in text or
// JSON. Anything on stderr is a test failure: blast writes to stdout only.
func blastRun(t testing.TB, base, cur string, asJSON bool) (stdout string, code int) {
	t.Helper()
	f := "text"
	if asJSON {
		f = "json"
	}
	var out, errb bytes.Buffer
	code = run([]string{"blast", "--base", base, "--format", f, cur}, &out, &errb)
	if errb.Len() > 0 {
		t.Errorf("blast --format %s wrote to stderr (exit %d):\n%s", f, code, errb.String())
	}
	return out.String(), code
}

// decodeBlast decodes one Blast JSON document strictly: unknown keys and
// trailing data fail the test.
func decodeBlast(t testing.TB, raw string) blastDoc {
	t.Helper()
	dec := json.NewDecoder(strings.NewReader(raw))
	dec.DisallowUnknownFields()
	var doc blastDoc
	if err := dec.Decode(&doc); err != nil {
		t.Fatalf("blast JSON does not decode: %v\n%s", err, raw)
	}
	if dec.More() {
		t.Fatalf("blast JSON has trailing data:\n%s", raw)
	}
	return doc
}

// assertBlastEmpty requires the empty blast of a tree against itself: exit
// 0, text from line 2 on exactly "Broken (0):\nImpacted (0):\n" (no Type
// changes section), JSON version 2, baseline true, every list [] (not
// null) and a zero summary.
func assertBlastEmpty(t *testing.T, label, base, cur string) {
	t.Helper()
	text, code := blastRun(t, base, cur, false)
	if code != 0 {
		t.Errorf("%s: text exit %d, want 0\n%s", label, code, text)
	}
	first, rest, ok := strings.Cut(text, "\n")
	if !ok || !strings.HasPrefix(first, "baseline: ") {
		t.Errorf("%s: text line 1 is not a baseline label:\n%s", label, text)
	}
	if rest != "Broken (0):\nImpacted (0):\n" {
		t.Errorf("%s: text from line 2:\n%q\nwant %q", label, rest, "Broken (0):\nImpacted (0):\n")
	}

	raw, code := blastRun(t, base, cur, true)
	if code != 0 {
		t.Errorf("%s: json exit %d, want 0\n%s", label, code, raw)
	}
	doc := decodeBlast(t, raw)
	if doc.Version != 2 || doc.Kind != "blast" || !doc.Baseline || doc.Note != nil {
		t.Errorf("%s: header version=%d kind=%q baseline=%v note=%v, want 2 blast true none", label, doc.Version, doc.Kind, doc.Baseline, doc.Note)
	}
	if doc.Broken == nil || doc.Impacted == nil || doc.Changes == nil {
		t.Errorf("%s: a list is null, want []:\n%s", label, raw)
	}
	if len(doc.Broken) != 0 || len(doc.Impacted) != 0 || len(doc.Changes) != 0 {
		t.Errorf("%s: broken %d, impacted %d, changes %d, want all empty:\n%s", label, len(doc.Broken), len(doc.Impacted), len(doc.Changes), raw)
	}
	if doc.Summary != (blastSummary{}) {
		t.Errorf("%s: summary %+v, want {0 0}", label, doc.Summary)
	}
}

// selfBlastOutput names the output the non-vacuity row deletes. File is
// relative to the corpus root; empty File means "pick from graph --json".
var selfBlastOutput = map[string]struct{ Module, File, Name string }{
	"iso20022": {"", "iac.src/s3_runtime/state.tf", "arn"},
	"secret":   {},
	"denis256": {},
}

// graphModulesDoc is the part of Graph JSON the non-vacuity pick reads.
type graphModulesDoc struct {
	Modules []struct {
		Path    string   `json:"path"`
		Known   bool     `json:"known"`
		Outputs []string `json:"outputs"`
	} `json:"modules"`
}

// pickModuleOutput returns, from graph --json on root, the first module (in
// path order) with a known surface whose first output (in name order) is
// declared as a plain `output "name" {` block in one of its .tf files, and
// that file. Every module in Graph JSON is pointed at by a unit.
func pickModuleOutput(t *testing.T, root string) (module, file, name string) {
	t.Helper()
	var out, errb bytes.Buffer
	if code := run([]string{"graph", "--json", root}, &out, &errb); code != 0 && code != 1 {
		t.Fatalf("graph --json exit %d: %s", code, errb.String())
	}
	var g graphModulesDoc
	if err := json.Unmarshal(out.Bytes(), &g); err != nil {
		t.Fatalf("graph --json: %v", err)
	}
	for _, m := range g.Modules {
		if !m.Known || len(m.Outputs) == 0 {
			continue
		}
		names := slices.Sorted(slices.Values(m.Outputs))
		files, _ := filepath.Glob(filepath.Join(root, filepath.FromSlash(m.Path), "*.tf"))
		slices.Sort(files)
		re := outputHeaderRe(names[0])
		for _, f := range files {
			b, err := os.ReadFile(f)
			if err == nil && re.Match(b) {
				rel, _ := filepath.Rel(root, f)
				return m.Path, filepath.ToSlash(rel), names[0]
			}
		}
	}
	t.Fatalf("no known module with a plain output block in graph --json of %s", root)
	return "", "", ""
}

func outputHeaderRe(name string) *regexp.Regexp {
	return regexp.MustCompile(`(?m)^output\s+"` + regexp.QuoteMeta(name) + `"\s*\{`)
}

// deleteOutputBlock removes the `output "name" { ... }` block from src. The
// end is found by brace depth, skipping double-quoted strings; the caller
// proves the result still parses by requiring blast to report exactly this
// output as removed.
func deleteOutputBlock(t *testing.T, src []byte, name string) []byte {
	t.Helper()
	loc := outputHeaderRe(name).FindIndex(src)
	if loc == nil {
		t.Fatalf("output %q not found", name)
	}
	depth, inStr := 0, false
	for i := loc[1] - 1; i < len(src); i++ {
		c := src[i]
		switch {
		case inStr && c == '\\':
			i++
		case c == '"':
			inStr = !inStr
		case !inStr && c == '{':
			depth++
		case !inStr && c == '}':
			depth--
			if depth == 0 {
				return append(append([]byte{}, src[:loc[0]]...), src[i+1:]...)
			}
		}
	}
	t.Fatalf("output %q block has no closing brace", name)
	return nil
}

// TestCorpusBlastSelf (REL-03 clause 1b): on each pinned corpus, blast of
// the checkout against itself, and against a faithful copy, is empty in
// text and JSON; the checkout is never written. Non-vacuity: the same
// harness on a copy with one output deleted reports that removal.
func TestCorpusBlastSelf(t *testing.T) {
	for _, r := range graphRepos {
		t.Run(r.Name, func(t *testing.T) {
			root := r.Require(t)
			before := corpusDigest(t, root)

			assertBlastEmpty(t, "same dir", root, root)
			cp := r.Copy(t, root)
			assertBlastEmpty(t, "copy as base", cp, root)

			pick := selfBlastOutput[r.Name]
			module, file, name := pick.Module, pick.File, pick.Name
			if file == "" {
				module, file, name = pickModuleOutput(t, root)
			}
			mut := r.Copy(t, root)
			p := filepath.Join(mut, filepath.FromSlash(file))
			src, err := os.ReadFile(p)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(p, deleteOutputBlock(t, src, name), 0o644); err != nil {
				t.Fatal(err)
			}
			raw, code := blastRun(t, root, mut, true)
			doc := decodeBlast(t, raw)
			found := false
			for _, c := range doc.Changes {
				if (module == "" || c.Module == module) && slices.Equal(c.RemovedOutputs, []string{name}) &&
					len(c.AddedOutputs) == 0 && len(c.AddedVariables) == 0 && len(c.RemovedVariables) == 0 {
					found = true
				}
			}
			if !found || len(doc.Impacted) == 0 {
				t.Errorf("non-vacuity: deleting output %q from %s (module %q) must give a changes entry removing exactly it and a non-empty impacted (exit %d):\n%s", name, file, module, code, raw)
			}
			t.Logf("non-vacuity: output %q of %s deleted: %d changes, %d impacted, %d broken, exit %d", name, file, len(doc.Changes), len(doc.Impacted), len(doc.Broken), code)

			if after := corpusDigest(t, root); after != before {
				t.Errorf("checkout changed: digest %s -> %s", before, after)
			}
		})
	}
}
