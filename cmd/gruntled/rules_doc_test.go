package main

// TestRuleRegistryDoc keeps the rule registry in step everywhere a user
// reads codes: the GRT004 section of docs/cli.md, Reserved codes, the SARIF
// table (no GRT004 rule), the Blast text example, the Blast JSON code-value
// note, the blast usage mirror and README's code table.

import (
	"os"
	"strings"
	"testing"
)

const (
	grt004Heading = "### GRT004: dependency output removed (error, blast only)"
	grt004Message = `dependency "<label>" output "<Y>" was removed from module "<module>" (target unit "<target>")`
	blastJSONNote = "New code values can appear without a version bump: since GRT004, a removed output is reported as GRT004 where earlier versions said GRT001. Gate on severity, not on a code list."
	blastExample  = `GRT004 dependency "db" output "id" was removed from module "modules/vpc" (target unit "live/db")`
	// Phase 14 (sec #195, #197a): what impacted[].module means, the version
	// check readers owe, and why JSON is the unambiguous path form.
	blastModuleMeaning  = "the changed module whose change reached this unit; for distance 1 it is also the unit's own module"
	blastCheckVersion   = "Readers must check `version`; a v1 reader must reject version 2."
	blastJSONPathNote   = "The JSON `source` and `via` keys are the unambiguous form of a path: text joins hops with ` -> `, which a directory name may itself contain."
	blastTextTransitive = "  live/edge (distance 2, from module modules/vpc, path live/edge -> live/db)"
	// Phase 14 (14-03): the edge rule in the blast intro and the lower-bound
	// limitation.
	blastEdgeRule   = "A dependent is reached only through a `dependency` block whose `enabled` is absent or literally `true` and whose `skip_outputs` is absent or literally `false`."
	blastLowerBound = "`blast` Impacted is a lower bound: a non-literal `enabled` or `skip_outputs` and a `dependencies { paths }` entry stop propagation, and propagation is not gated on which outputs a dependent reads."
)

func readDoc(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return strings.ReplaceAll(string(b), "\r\n", "\n")
}

// section returns the text after the exact heading line up to the next
// heading of the same or a higher level.
func section(t *testing.T, doc, heading string) string {
	t.Helper()
	_, rest, ok := strings.Cut(doc, "\n"+heading+"\n")
	if !ok {
		t.Fatalf("no %q heading", heading)
	}
	level := heading[:strings.Index(heading, " ")+1] // "## " or "### "
	end := len(rest)
	for _, h := range []string{"\n## ", "\n" + level} {
		if i := strings.Index(rest, h); i >= 0 && i < end {
			end = i
		}
	}
	return rest[:end]
}

func TestRuleRegistryDoc(t *testing.T) {
	doc := readDoc(t, "../../docs/cli.md")

	diags := section(t, doc, "## Diagnostics")
	if !strings.Contains(diags, "\n"+grt004Heading+"\n") {
		t.Errorf("## Diagnostics lacks the line %q", grt004Heading)
	}
	g4 := section(t, doc, grt004Heading)
	if !strings.Contains(g4, "\n"+grt004Message+"\n") {
		t.Errorf("GRT004 section lacks the message line %q", grt004Message)
	}

	reserved := section(t, doc, "### Reserved codes")
	if !strings.Contains(reserved, "GRT005") || !strings.Contains(reserved, "GRT006") || strings.Contains(reserved, "GRT004") {
		t.Errorf("Reserved codes must name GRT005 and GRT006 and not GRT004:\n%s", reserved)
	}

	if strings.Contains(section(t, doc, "### SARIF"), "GRT004") {
		t.Error("the SARIF section mentions GRT004: blast has no SARIF output and GRT004 has no rule")
	}

	bt := section(t, doc, "### Blast text")
	if !strings.Contains(bt, blastExample) {
		t.Errorf("Blast text example lacks %q", blastExample)
	}
	if strings.Contains(bt, `GRT001 dependency "db" output "id"`) {
		t.Error("Blast text example still shows GRT001 for the removed output")
	}

	bj := strings.Join(strings.Fields(section(t, doc, "### Blast JSON")), " ")
	for _, want := range []string{blastJSONNote, blastModuleMeaning, blastCheckVersion} {
		if !strings.Contains(bj, want) {
			t.Errorf("Blast JSON lacks %q", want)
		}
	}
	if !strings.Contains(bj, "this is version 2") {
		t.Error("Blast JSON does not say this is version 2")
	}
	btFolded := strings.Join(strings.Fields(bt), " ")
	if !strings.Contains(btFolded, blastJSONPathNote) {
		t.Errorf("Blast text lacks %q", blastJSONPathNote)
	}
	if !strings.Contains(bt, "\n"+blastTextTransitive+"\n") {
		t.Errorf("Blast text example lacks the transitive line %q", blastTextTransitive)
	}

	_, usage, code := runCLI(t, "blast", "-h")
	if code != exitOK {
		t.Fatalf("blast -h: exit %d", code)
	}
	if !strings.Contains(usage, "GRT004") {
		t.Errorf("blast usage does not mention GRT004:\n%s", usage)
	}
	if !strings.Contains(doc, "```\n"+usage+"```\n") {
		t.Errorf("docs/cli.md has no fenced block equal to blast -h:\n%s", usage)
	}

	folded := strings.Join(strings.Fields(doc), " ")
	intro := folded[strings.Index(folded, "`gruntled blast` answers"):]
	intro = intro[:strings.Index(intro, "gruntled never runs git")]
	if strings.Contains(intro, "Only direct consumers are listed") || !strings.Contains(intro, blastEdgeRule) {
		t.Errorf("blast intro must drop the one-hop wording and state %q:\n%s", blastEdgeRule, intro)
	}
	limits := strings.Join(strings.Fields(section(t, doc, "## Known limitations")), " ")
	if strings.Contains(limits, "`blast` Impacted is one hop") || !strings.Contains(limits, blastLowerBound) {
		t.Errorf("Known limitations must drop the one-hop bullet and state %q", blastLowerBound)
	}

	readme := readDoc(t, "../../README.md")
	for _, para := range strings.Split(readme, "\n\n") {
		if strings.HasPrefix(para, "`gruntled blast --base dir") {
			p := strings.Join(strings.Fields(para), " ")
			if !strings.Contains(p, "distance") || !strings.Contains(p, "path") || !strings.Contains(p, "--depth") {
				t.Errorf("README blast paragraph does not mention distance, path and --depth: %q", p)
			}
		}
	}
	// Paragraph-scoped (blank-line separated, whitespace folded), so a
	// reflow cannot split GRT004 and "no `Code` constant" across lines.
	for _, para := range strings.Split(readme, "\n\n") {
		p := strings.Join(strings.Fields(para), " ")
		if strings.Contains(p, "GRT004") && strings.Contains(p, "no `Code` constant") {
			t.Errorf("README paragraph still calls GRT004 unimplemented: %q", p)
		}
	}
	for _, l := range strings.Split(readme, "\n") {
		if strings.Contains(l, "`GRT004`-`GRT006`") {
			t.Errorf("README line still lists GRT004 as later: %q", l)
		}
		if strings.HasPrefix(l, "All four are emitted by `gruntled check`") {
			t.Errorf("README still says every defined code comes from check: %q", l)
		}
	}
	var row string
	for _, l := range strings.Split(readme, "\n") {
		if strings.HasPrefix(l, "| `GRT004` | `CodeRemovedOutput` |") {
			row = l
		}
	}
	if !strings.Contains(row, "blast") {
		t.Errorf("README code table lacks a blast-only GRT004 row (got %q)", row)
	}
	if !strings.Contains(readme, "`GRT005`-`GRT006`") {
		t.Error("README Later line does not say GRT005-GRT006")
	}
}
