package main

import (
	"bytes"
	"cmp"
	"crypto/sha256"
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/GiulioSavini/gruntled/internal/testsupport/synthrepo"
)

// e2eSpec is the synthetic repository shared by the oracle and
// determinism tests: 60 units, four injected bad output references.
var e2eSpec = synthrepo.Spec{
	Units:            60,
	IncludeDepth:     3,
	DependencyFanout: 3,
	Seed:             7,
	Errors: []synthrepo.ErrorKind{
		synthrepo.BadOutputRef, synthrepo.BadOutputRef,
		synthrepo.BadOutputRef, synthrepo.BadOutputRef,
	},
}

type e2eDiag struct {
	Code     string `json:"code"`
	Severity string `json:"severity"`
	File     string `json:"file"`
	Line     int    `json:"line"`
	Column   int    `json:"column"`
	Unit     string `json:"unit"`
	Message  string `json:"message"`
}

type e2eReport struct {
	Version     int       `json:"version"`
	Diagnostics []e2eDiag `json:"diagnostics"`
}

// e2eKey is the oracle projection of a diagnostic.
type e2eKey struct {
	File   string
	Line   int
	Column int
	Unit   string
}

func compareKey(a, b e2eKey) int {
	return cmp.Or(
		cmp.Compare(a.File, b.File),
		cmp.Compare(a.Line, b.Line),
		cmp.Compare(a.Column, b.Column),
		cmp.Compare(a.Unit, b.Unit),
	)
}

func runCLI(t *testing.T, args ...string) (stdout, stderr string, code int) {
	t.Helper()
	var out, errb bytes.Buffer
	code = run(args, &out, &errb)
	return out.String(), errb.String(), code
}

func decodeReport(t *testing.T, stdout string) e2eReport {
	t.Helper()
	var r e2eReport
	if err := json.Unmarshal([]byte(stdout), &r); err != nil {
		t.Fatalf("decoding JSON output: %v\n%s", err, stdout)
	}
	return r
}

func expectedKeys(m synthrepo.Manifest) []e2eKey {
	keys := make([]e2eKey, 0, len(m.Expected))
	for _, e := range m.Expected {
		keys = append(keys, e2eKey{
			File:   e.Pos.File().String(),
			Line:   e.Pos.Line(),
			Column: e.Pos.Column(),
			Unit:   e.Unit.String(),
		})
	}
	slices.SortFunc(keys, compareKey)
	return keys
}

func generate(t *testing.T, spec synthrepo.Spec, dir string) synthrepo.Manifest {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	m, err := synthrepo.Generate(spec, dir)
	if err != nil {
		t.Fatalf("synthrepo.Generate: %v", err)
	}
	return m
}

func TestOracle(t *testing.T) {
	dir := t.TempDir()
	m := generate(t, e2eSpec, dir)
	if len(m.Expected) != len(e2eSpec.Errors) {
		t.Fatalf("manifest has %d expected diagnostics, want %d", len(m.Expected), len(e2eSpec.Errors))
	}

	stdout, stderr, code := runCLI(t, "check", "--format", "json", dir)
	if code != exitFindings {
		t.Fatalf("exit code %d, want %d\nstderr: %s", code, exitFindings, stderr)
	}
	r := decodeReport(t, stdout)

	var got []e2eKey
	for _, d := range r.Diagnostics {
		if d.Code != "GRT001" {
			t.Errorf("unexpected non-GRT001 diagnostic: %+v", d)
			continue
		}
		if d.Severity != "error" {
			t.Errorf("GRT001 severity %q, want error: %+v", d.Severity, d)
		}
		got = append(got, e2eKey{File: d.File, Line: d.Line, Column: d.Column, Unit: d.Unit})
	}
	slices.SortFunc(got, compareKey)
	if want := expectedKeys(m); !reflect.DeepEqual(got, want) {
		t.Fatalf("GRT001 set differs from the manifest\n got: %v\nwant: %v", got, want)
	}
}

func TestDeterministicAcrossCheckouts(t *testing.T) {
	base := t.TempDir()
	dirs := []string{filepath.Join(base, "checkout-a"), filepath.Join(base, "another-name")}
	for _, d := range dirs {
		generate(t, e2eSpec, d)
	}

	cases := []struct {
		format string
		args   []string
		want   int
	}{
		{"text", []string{"check", "--format", "text"}, exitFindings},
		{"json", []string{"check", "--format", "json"}, exitFindings},
		{"sarif", []string{"check", "--format", "sarif"}, exitFindings},
		{"graph", []string{"graph", "--json"}, exitOK},
	}
	for _, c := range cases {
		format := c.format
		var outs []string
		var codes []int
		record := func(extra ...string) {
			args := append(slices.Clone(c.args), extra...)
			out, stderr, code := runCLI(t, args...)
			if code != c.want {
				t.Fatalf("%s %v: exit %d, want %d\nstderr: %s", format, args, code, c.want, stderr)
			}
			outs = append(outs, out)
			codes = append(codes, code)
		}
		for _, d := range dirs {
			record(d)
			record(d)
			t.Chdir(d)
			record()
			record(".")
		}

		for i, out := range outs {
			if out != outs[0] {
				t.Fatalf("%s: run %d stdout differs from run 0\n--- run 0:\n%s\n--- run %d:\n%s", format, i, outs[0], i, out)
			}
			if codes[i] != codes[0] {
				t.Fatalf("%s: run %d exit %d, run 0 exit %d", format, i, codes[i], codes[0])
			}
		}
		out := outs[0]
		if out == "" {
			t.Fatalf("%s: empty stdout on a repository with injected errors", format)
		}
		for _, leak := range []string{base, dirs[0], dirs[1], "__gruntled_repo_root__"} {
			if strings.Contains(out, leak) {
				t.Fatalf("%s: stdout contains %q:\n%s", format, leak, out)
			}
		}
		if format == "json" {
			r := decodeReport(t, out)
			for _, d := range r.Diagnostics {
				for _, p := range []string{d.File, d.Unit} {
					if strings.HasPrefix(p, "/") || strings.Contains(p, `:\`) {
						t.Fatalf("absolute path in JSON diagnostic: %+v", d)
					}
				}
			}
			for i := 1; i < len(outs); i++ {
				if !reflect.DeepEqual(decodeReport(t, outs[i]).Diagnostics, r.Diagnostics) {
					t.Fatalf("decoded diagnostics of run %d differ from run 0", i)
				}
			}
		}
	}
}

type entrySnap struct {
	Size   int64
	Mode   fs.FileMode
	MTime  time.Time
	SHA256 [32]byte
}

func snapshot(t *testing.T, root string) map[string]entrySnap {
	t.Helper()
	snap := map[string]entrySnap{}
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		e := entrySnap{Size: info.Size(), Mode: info.Mode(), MTime: info.ModTime()}
		if info.Mode().IsRegular() {
			b, err := os.ReadFile(p)
			if err != nil {
				return err
			}
			e.SHA256 = sha256.Sum256(b)
		}
		rel, err := filepath.Rel(root, p)
		if err != nil {
			return err
		}
		snap[rel] = e
		return nil
	})
	if err != nil {
		t.Fatalf("snapshot %s: %v", root, err)
	}
	return snap
}

func chmodTree(t *testing.T, root string, dirMode, fileMode fs.FileMode) {
	t.Helper()
	// Files first, directories deepest-last, so that a read-only directory
	// never blocks reaching its children and a restore always succeeds.
	var dirs []string
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if err := os.Chmod(p, 0o755); err != nil {
				return err
			}
			dirs = append(dirs, p)
			return nil
		}
		return os.Chmod(p, fileMode)
	})
	if err != nil {
		t.Fatalf("chmod %s: %v", root, err)
	}
	for i := len(dirs) - 1; i >= 0; i-- {
		if err := os.Chmod(dirs[i], dirMode); err != nil {
			t.Fatalf("chmod %s: %v", dirs[i], err)
		}
	}
}

func TestNoWrites(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX permissions required")
	}
	if os.Geteuid() == 0 {
		t.Skip("root ignores read-only permissions")
	}
	dir := filepath.Join(t.TempDir(), "repo")
	generate(t, e2eSpec, dir)
	chmodTree(t, dir, 0o555, 0o444)
	t.Cleanup(func() { chmodTree(t, dir, 0o755, 0o644) })

	before := snapshot(t, dir)
	envDirs := map[string]string{}
	for _, k := range []string{"HOME", "XDG_CACHE_HOME", "TMPDIR"} {
		envDirs[k] = t.TempDir()
		t.Setenv(k, envDirs[k])
	}

	for _, format := range []string{"text", "json", "sarif"} {
		if _, stderr, code := runCLI(t, "check", "--format", format, dir); code != exitFindings {
			t.Fatalf("%s: exit %d, want %d\nstderr: %s", format, code, exitFindings, stderr)
		}
	}
	if _, stderr, code := runCLI(t, "graph", "--json", dir); code != exitOK {
		t.Fatalf("graph: exit %d, want %d\nstderr: %s", code, exitOK, stderr)
	}

	if after := snapshot(t, dir); !reflect.DeepEqual(before, after) {
		t.Fatalf("repository changed during check")
	}
	for k, d := range envDirs {
		entries, err := os.ReadDir(d)
		if err != nil {
			t.Fatal(err)
		}
		if len(entries) != 0 {
			t.Fatalf("%s directory %s has %d entries after check", k, d, len(entries))
		}
	}
}

func writeTree(t *testing.T, tree synthrepo.Tree, dir string) {
	t.Helper()
	for _, f := range tree {
		full := filepath.Join(dir, filepath.FromSlash(f.Path))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, f.Content, 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func TestMutationDiff(t *testing.T) {
	spec := e2eSpec
	spec.Errors = nil

	clean := t.TempDir()
	generate(t, spec, clean)
	cleanOut, stderr, code := runCLI(t, "check", "--format", "json", clean)
	if code != exitOK {
		t.Fatalf("clean repo: exit %d, want %d\nstderr: %s", code, exitOK, stderr)
	}
	cleanDiags := decodeReport(t, cleanOut).Diagnostics
	if len(cleanDiags) != 0 {
		t.Fatalf("clean repo has diagnostics: %+v", cleanDiags)
	}

	spec.Errors = []synthrepo.ErrorKind{synthrepo.BadOutputRef}
	tree, m, err := synthrepo.Render(spec)
	if err != nil {
		t.Fatalf("synthrepo.Render: %v", err)
	}
	if len(m.Expected) != 1 {
		t.Fatalf("manifest has %d expected diagnostics, want 1", len(m.Expected))
	}
	mutated := t.TempDir()
	writeTree(t, tree, mutated)
	mutOut, stderr, code := runCLI(t, "check", "--format", "json", mutated)
	if code != exitFindings {
		t.Fatalf("mutated repo: exit %d, want %d\nstderr: %s", code, exitFindings, stderr)
	}
	mutDiags := decodeReport(t, mutOut).Diagnostics

	// added = mutated minus clean; removed = clean minus mutated.
	var added []e2eKey
	for _, d := range mutDiags {
		if !slices.Contains(cleanDiags, d) {
			if d.Code != "GRT001" {
				t.Errorf("added non-GRT001 diagnostic: %+v", d)
			}
			added = append(added, e2eKey{File: d.File, Line: d.Line, Column: d.Column, Unit: d.Unit})
		}
	}
	for _, d := range cleanDiags {
		if !slices.Contains(mutDiags, d) {
			t.Errorf("diagnostic removed by the mutation: %+v", d)
		}
	}
	if want := expectedKeys(m); !reflect.DeepEqual(added, want) {
		t.Fatalf("added diagnostics differ from the manifest\n got: %v\nwant: %v", added, want)
	}

	// An unrelated new output in the target module must not change a byte.
	target := filepath.Join(mutated, filepath.FromSlash(m.Expected[0].Target.String()))
	tfs, err := filepath.Glob(filepath.Join(target, "*.tf"))
	if err != nil || len(tfs) == 0 {
		t.Fatalf("no .tf file in target module %s (err %v)", target, err)
	}
	f, err := os.OpenFile(tfs[0], os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString("\noutput \"zz_unrelated\" {\n  value = 1\n}\n"); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	againOut, stderr, code := runCLI(t, "check", "--format", "json", mutated)
	if code != exitFindings {
		t.Fatalf("mutated repo after unrelated edit: exit %d\nstderr: %s", code, stderr)
	}
	if againOut != mutOut {
		t.Fatalf("unrelated module change altered the output\n--- before:\n%s\n--- after:\n%s", mutOut, againOut)
	}
}

var exitCodeLine = regexp.MustCompile(`^  [0-3]  `)

func TestHelpMatchesDocs(t *testing.T) {
	docs, err := os.ReadFile("../../docs/cli.md")
	if err != nil {
		t.Fatal(err)
	}
	docLines := strings.Split(strings.ReplaceAll(string(docs), "\r\n", "\n"), "\n")
	for _, c := range []struct {
		cmd  string
		want int
	}{
		{"check", 4},
		{"graph", 3},
		{"blast", 4},
		{"watch", 3},
		{"report", 4},
	} {
		_, stderr, code := runCLI(t, c.cmd, "-h")
		if code != exitOK {
			t.Fatalf("%s -h: exit %d, want %d", c.cmd, code, exitOK)
		}
		var lines []string
		for l := range strings.SplitSeq(stderr, "\n") {
			if exitCodeLine.MatchString(l) {
				lines = append(lines, l)
			}
		}
		if len(lines) != c.want {
			t.Fatalf("%s -h prints %d exit-code lines, want %d:\n%s", c.cmd, len(lines), c.want, stderr)
		}
		for _, l := range lines {
			if !slices.Contains(docLines, l) {
				t.Errorf("docs/cli.md lacks the exact line %q (from %s -h)", l, c.cmd)
			}
		}
	}
}
