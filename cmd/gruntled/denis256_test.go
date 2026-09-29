package main

// Env-gated secondary-corpus check on denis256/terragrunt-tests (04-04).
//
// The expectation table below is hand-derived from the corpus text and the
// locked DIAG-03 rules, never from gruntled output. Gap-closure plan 02-13
// (include-target rule) later made every include-free unit of a repository
// with a dynamic or terragrunt.hcl-named include an include target, reported
// unknown. On denis256 that covers the referring unit or the target unit of
// all 8 references, so DIAG-03 keeps them silent. The test therefore asserts
// that the GRT001 set is empty, and that each of the 8 known references is
// explained by an unknown unit or module, with the reason logged. A reference
// on a fully resolved path that gruntled does not report fails the test.

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"runtime/debug"
	"slices"
	"strconv"
	"strings"
	"testing"
)

const denisPinnedCommit = "726485e699a70c02dabbde629f66c0119e197357"

// denisMaskSuffix is the mock-masking suffix documented in docs/cli.md.
const denisMaskSuffix = "; mock_outputs supplies it, so apply would silently use the mock value"

type denisEntry struct {
	File   string
	Line   int
	Col    int
	Unit   string
	Dep    string
	Output string
	Target string
	Suffix bool
	Why    string
}

// denisExpected lists the 8 references that DIAG-03 would report if both the
// referring unit and the target unit resolved.
var denisExpected = []denisEntry{
	{
		File: "issue-2163/app/terragrunt.hcl", Line: 14, Col: 24, Unit: "issue-2163/app",
		Dep: "app_service_plan01", Output: "asp_id", Target: "issue-2163/module", Suffix: false,
		Why: `The module declares no outputs. asp_id is only mocked, and allowed commands are ["validate", "plan"], so apply fails loudly instead of masking (terragrunt#2163 reproduction).`,
	},
	{
		File: "issue-2405/app/terragrunt.hcl", Line: 16, Col: 21, Unit: "issue-2405/app",
		Dep: "vpc", Output: "vpc_id", Target: "issue-2405/vpc", Suffix: true,
		Why: "vpc/main.tf is empty. vpc_id exists only in mock_outputs, and no block-level allowed-commands list exists (it is misplaced inside the mock map as a key), so apply would use the mock.",
	},
	{
		File: "issue-2405/app/terragrunt.hcl", Line: 17, Col: 21, Unit: "issue-2405/app",
		Dep: "vpc", Output: "private_subnets", Target: "issue-2405/vpc", Suffix: true,
		Why: "Same fixture and facts as #2, for private_subnets.",
	},
	{
		File: "issue-2631/main/terragrunt.hcl", Line: 9, Col: 9, Unit: "issue-2631/main",
		Dep: "dep", Output: "a", Target: "issue-2631/dependency", Suffix: false,
		Why: "Genuine fixture bug: the module declares only `y`, and there is no mock_outputs at all.",
	},
	{
		File: "issue-2718/app/terragrunt.hcl", Line: 23, Col: 25, Unit: "issue-2718/app",
		Dep: "vpc_main", Output: "aws_subnet_public_output", Target: "issue-2718/vpc", Suffix: true,
		Why: "vpc/main.tf is empty. The key exists only in mock_outputs. merge and allowed-commands are misplaced inside the mock map, so block-level allowed is absent (every command) and the zero-output target gets the mocks at apply.",
	},
	{
		File: "mock-output/module1/terragrunt.hcl", Line: 22, Col: 16, Unit: "mock-output/module1",
		Dep: "module2", Output: "subnets", Target: "mock-output/module2", Suffix: true,
		Why: `module2 declares only hello and attribute. subnets is only mocked, strategy "shallow" merges mocks into state, and apply is allowed, so apply silently injects the mock.`,
	},
	{
		File: "mocks/module1/terragrunt.hcl", Line: 10, Col: 12, Unit: "mocks/module1",
		Dep: "module2", Output: "vpc_id2", Target: "mocks/module2", Suffix: false,
		Why: "Genuine fixture bug: a typo for vpc_id. module2/main.tf is empty, and mock_outputs is `yamldecode(file(...))` (not literal, so no suffix), whose YAML defines only vpc_id anyway.",
	},
	{
		File: "optional-dependency/reference-disabled-dependency/app/terragrunt.hcl", Line: 17, Col: 12,
		Unit: "optional-dependency/reference-disabled-dependency/app",
		Dep:  "vpc", Output: "vpc_id", Target: "optional-dependency/reference-disabled-dependency/vpc", Suffix: true,
		Why: "`enabled = true` literally. The module has one resource and zero outputs. vpc_id is only mocked, and allowed commands are absent, so apply returns the mock.",
	},
}

// denisSilent lists references that must never be reported.
var denisSilent = []denisEntry{
	{
		File: "optional-dependency/reference-disabled-dependency/app/terragrunt.hcl", Line: 18, Col: 12,
		Unit: "optional-dependency/reference-disabled-dependency/app",
		Dep:  "db", Output: "db", Target: "optional-dependency/reference-disabled-dependency/db", Suffix: false,
		Why: "`enabled = false`: DIAG-03 row 2 (enabled not literally true) is silent before mocks are consulted. The Phase 2 stress report listed it only because it ran without DIAG-03.",
	},
}

func (e denisEntry) pos() string {
	return e.File + ":" + strconv.Itoa(e.Line) + ":" + strconv.Itoa(e.Col)
}

// denisMessage is the GRT001 message DIAG-03 would give for e (docs/cli.md).
func denisMessage(e denisEntry) string {
	m := "dependency " + strconv.Quote(e.Dep) + " output " + strconv.Quote(e.Output) +
		" is not declared by module " + strconv.Quote(e.Target) + " (target unit " + strconv.Quote(e.Target) + ")"
	if e.Suffix {
		m += denisMaskSuffix
	}
	return m
}

type denisReport struct {
	Version     int `json:"version"`
	Diagnostics []struct {
		Code     string `json:"code"`
		Severity string `json:"severity"`
		File     string `json:"file"`
		Line     int    `json:"line"`
		Column   int    `json:"column"`
		Unit     string `json:"unit"`
		Message  string `json:"message"`
	} `json:"diagnostics"`
	UnknownUnits []struct {
		Path   string `json:"path"`
		Status string `json:"status"`
		Reason string `json:"reason"`
	} `json:"unknown_units"`
	UnknownModules []struct {
		Path   string `json:"path"`
		Reason string `json:"reason"`
	} `json:"unknown_modules"`
	Summary struct {
		Units          int `json:"units"`
		Resolved       int `json:"resolved"`
		ModuleUnknown  int `json:"module_unknown"`
		ConfigUnknown  int `json:"config_unknown"`
		UnknownModules int `json:"unknown_modules"`
		Errors         int `json:"errors"`
		Warnings       int `json:"warnings"`
	} `json:"summary"`
}

// denisHeadCommit resolves .git/HEAD through a loose ref or packed-refs.
func denisHeadCommit(t testing.TB, root string) string {
	t.Helper()
	head, err := os.ReadFile(filepath.Join(root, ".git", "HEAD"))
	if err != nil {
		t.Fatalf("read .git/HEAD: %v", err)
	}
	h := strings.TrimSpace(string(head))
	ref, ok := strings.CutPrefix(h, "ref: ")
	if !ok {
		return h
	}
	if b, err := os.ReadFile(filepath.Join(root, ".git", filepath.FromSlash(ref))); err == nil {
		return strings.TrimSpace(string(b))
	} else if !os.IsNotExist(err) {
		t.Fatalf("read ref %s: %v", ref, err)
	}
	packed, err := os.ReadFile(filepath.Join(root, ".git", "packed-refs"))
	if err != nil {
		t.Fatalf("ref %s not loose and packed-refs unreadable: %v", ref, err)
	}
	for line := range strings.SplitSeq(string(packed), "\n") {
		if sha, name, ok := strings.Cut(strings.TrimSpace(line), " "); ok && name == ref {
			return sha
		}
	}
	t.Fatalf("ref %s not found in .git or packed-refs", ref)
	return ""
}

func denisRequire(t testing.TB) string {
	t.Helper()
	dir := os.Getenv("GRUNTLED_CORPUS_DENIS256")
	if dir == "" {
		t.Skip("GRUNTLED_CORPUS_DENIS256 not set")
	}
	root, err := filepath.Abs(filepath.Clean(dir))
	if err != nil {
		t.Fatalf("abs %s: %v", dir, err)
	}
	if got := denisHeadCommit(t, root); got != denisPinnedCommit {
		t.Fatalf("GRUNTLED_CORPUS_DENIS256 must be a checkout of the pinned commit; got %s, want %s", got, denisPinnedCommit)
	}
	return root
}

// denisDigest hashes the tree under root (excluding .git) without following
// symlinks: path, type, permission, and file content or link target.
func denisDigest(t testing.TB, root string) string {
	t.Helper()
	var lines []string
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, p)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if d.IsDir() && d.Name() == ".git" {
			return filepath.SkipDir
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		entry := rel + "\x00" + info.Mode().Type().String() + "\x00" + info.Mode().Perm().String() + "\x00"
		switch {
		case info.Mode()&fs.ModeSymlink != 0:
			target, err := os.Readlink(p)
			if err != nil {
				return err
			}
			entry += "link:" + target
		case info.Mode().IsRegular():
			b, err := os.ReadFile(p)
			if err != nil {
				return err
			}
			sum := sha256.Sum256(b)
			entry += hex.EncodeToString(sum[:])
		}
		lines = append(lines, entry)
		return nil
	})
	if err != nil {
		t.Fatalf("digest %s: %v", root, err)
	}
	slices.Sort(lines)
	sum := sha256.Sum256([]byte(strings.Join(lines, "\n")))
	return hex.EncodeToString(sum[:])
}

func denisRun(t testing.TB, dir string) (raw []byte, code int) {
	t.Helper()
	var out, errb bytes.Buffer
	func() {
		defer func() {
			if r := recover(); r != nil {
				t.Fatalf("gruntled panicked on denis256: %v\n%s", r, debug.Stack())
			}
		}()
		code = run([]string{"check", "--format", "json", dir}, &out, &errb)
	}()
	if errb.Len() > 0 {
		t.Logf("gruntled stderr:\n%s", errb.String())
	}
	return out.Bytes(), code
}

func denisDecode(t testing.TB, raw []byte) denisReport {
	t.Helper()
	var rep denisReport
	if err := json.Unmarshal(raw, &rep); err != nil {
		t.Fatalf("decode gruntled JSON: %v", err)
	}
	return rep
}

func denisIsWordByte(c byte) bool {
	return c == '_' || (c >= '0' && c <= '9') || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}

func TestDenis256Corpus(t *testing.T) {
	root := denisRequire(t)
	d0 := denisDigest(t, root)

	t.Run("expectations_match_corpus_text", func(t *testing.T) {
		for _, e := range slices.Concat(denisExpected, denisSilent) {
			if e.Unit != path.Dir(e.File) {
				t.Fatalf("%s: unit %q is not the file's directory", e.pos(), e.Unit)
			}
			b, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(e.File)))
			if err != nil {
				t.Fatalf("read %s: %v", e.File, err)
			}
			lines := strings.Split(string(b), "\n")
			if e.Line < 1 || e.Line > len(lines) {
				t.Fatalf("%s: line out of range (file has %d lines)", e.pos(), len(lines))
			}
			line := lines[e.Line-1]
			ref := "dependency." + e.Dep + ".outputs." + e.Output
			if e.Col < 1 || e.Col-1 > len(line) || !strings.HasPrefix(line[e.Col-1:], ref) {
				t.Fatalf("%s: want %q at column %d, line is %q", e.pos(), ref, e.Col, line)
			}
			if end := e.Col - 1 + len(ref); end < len(line) && denisIsWordByte(line[end]) {
				t.Fatalf("%s: %q is followed by word byte %q in %q", e.pos(), ref, line[end], line)
			}
		}
	})

	raw1, c1 := denisRun(t, root)
	rep := denisDecode(t, raw1)

	t.Run("no_panic_deterministic", func(t *testing.T) {
		raw2, c2 := denisRun(t, root)
		if c1 != 1 || c2 != 1 {
			t.Errorf("exit codes %d, %d; want 1, 1", c1, c2)
		}
		if !bytes.Equal(raw1, raw2) {
			t.Errorf("JSON stdout differs between two runs (%d vs %d bytes)", len(raw1), len(raw2))
		}
		var grt001, grt100 int
		var grt100Pos []string
		for _, d := range rep.Diagnostics {
			switch d.Code {
			case "GRT001":
				grt001++
			case "GRT100":
				grt100++
				grt100Pos = append(grt100Pos, d.File+":"+strconv.Itoa(d.Line)+":"+strconv.Itoa(d.Column))
			default:
				t.Errorf("unexpected code %s at %s:%d:%d: %s", d.Code, d.File, d.Line, d.Column, d.Message)
			}
		}
		reasons := map[string]int{}
		for _, u := range rep.UnknownUnits {
			reasons[u.Reason]++
		}
		keys := make([]string, 0, len(reasons))
		for k := range reasons {
			keys = append(keys, k)
		}
		slices.SortFunc(keys, func(a, b string) int {
			if reasons[a] != reasons[b] {
				return reasons[b] - reasons[a]
			}
			return strings.Compare(a, b)
		})
		var sb strings.Builder
		s := rep.Summary
		for _, kv := range []struct {
			k string
			v int
		}{
			{"units", s.Units}, {"resolved", s.Resolved}, {"module_unknown", s.ModuleUnknown},
			{"config_unknown", s.ConfigUnknown}, {"unknown_modules", s.UnknownModules},
			{"errors", s.Errors}, {"warnings", s.Warnings}, {"grt100", grt100}, {"grt001", grt001},
		} {
			sb.WriteString("DENIS256 " + kv.k + "=" + strconv.Itoa(kv.v) + "\n")
		}
		for _, k := range keys {
			sb.WriteString("DENIS256 unknown_reason " + k + "=" + strconv.Itoa(reasons[k]) + "\n")
		}
		for _, p := range grt100Pos {
			sb.WriteString("DENIS256 grt100 " + p + "\n")
		}
		t.Logf("\n%s", sb.String())
	})

	t.Run("grt001_exact_set", func(t *testing.T) {
		// Since 02-13 the expected GRT001 set on denis256 is empty: every
		// hand-derived reference sits on an unknown referring or target unit.
		var got []string
		for _, d := range rep.Diagnostics {
			if d.Code != "GRT001" {
				continue
			}
			pos := d.File + ":" + strconv.Itoa(d.Line) + ":" + strconv.Itoa(d.Column)
			got = append(got, pos+" "+d.Unit+" | "+d.Message)
			if d.Severity != "error" {
				t.Errorf("GRT001 at %s has severity %q, want error", pos, d.Severity)
			}
			for _, s := range denisSilent {
				if s.pos() == pos {
					t.Errorf("GRT001 at silent position %s (%s): %s", pos, s.Why, d.Message)
				}
			}
		}
		slices.Sort(got)
		for _, g := range got {
			t.Logf("got GRT001 %s", g)
		}
		if len(got) != 0 {
			t.Errorf("GRT001 set on denis256 is not empty (%d extra):\n%s", len(got), strings.Join(got, "\n"))
		}
	})

	t.Run("expected_refs_explained_by_unknown", func(t *testing.T) {
		unit := map[string]string{}
		for _, u := range rep.UnknownUnits {
			unit[u.Path] = u.Status + "/" + u.Reason
		}
		module := map[string]string{}
		for _, m := range rep.UnknownModules {
			module[m.Path] = "module-unknown/" + m.Reason
		}
		reported := map[string]bool{}
		for _, d := range rep.Diagnostics {
			if d.Code == "GRT001" {
				reported[d.File+":"+strconv.Itoa(d.Line)+":"+strconv.Itoa(d.Column)] = true
			}
		}
		for _, e := range denisExpected {
			if reported[e.pos()] {
				continue
			}
			us, uOK := unit[e.Unit]
			ts, tOK := unit[e.Target]
			ms, mOK := module[e.Target]
			if !uOK && !tOK && !mOK {
				t.Errorf("MISS %s %s | %s: unit and target both resolved, but no GRT001", e.pos(), e.Unit, denisMessage(e))
				continue
			}
			t.Logf("not reported %s %s | %s\n    unit %s: %s; target %s: %s; target module: %s",
				e.pos(), e.Unit, denisMessage(e),
				e.Unit, denisOr(us, "resolved"), e.Target, denisOr(ts, "resolved"), denisOr(ms, "known"))
		}
	})

	if d1 := denisDigest(t, root); d1 != d0 {
		t.Fatalf("corpus digest changed: %s -> %s", d0, d1)
	}
}

func denisOr(s, def string) string {
	if s == "" {
		return def
	}
	return s
}
