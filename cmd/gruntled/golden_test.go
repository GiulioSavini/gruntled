package main

// VALID-02 golden tests. Two kinds, both always on (no environment
// variable, no external binary), so they run in CI's existing go test
// step:
//
//   - TestGoldenSynthrepoFullScale generates synthrepo trees of 400-600
//     units and asserts that the GRT001 set equals Manifest.Expected
//     exactly.
//   - TestGoldenFixtures runs every hand-written txtar repository under
//     testdata/golden and asserts the exact diagnostic set, exit code and
//     (when given) unknown-unit set written by hand in its _golden/
//     section. Every want position is checked against the fixture bytes
//     before gruntled runs, so a hand-count mistake fails the harness
//     instead of becoming a silent golden. There is no update flag: want
//     files are never generated from gruntled's output.

import (
	"bytes"
	"cmp"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/GiulioSavini/gruntled/internal/testsupport/synthrepo"
	"github.com/rogpeppe/go-internal/txtar"
)

// goldenReport mirrors the fields of JSON schema v1
// (internal/interfaces/presenter/json.go) that the goldens assert.
type goldenReport struct {
	Version     int `json:"version"`
	Diagnostics []struct {
		Code     string `json:"code"`
		Severity string `json:"severity"`
		File     string `json:"file"`
		Line     int    `json:"line"`
		Column   int    `json:"column"`
		Unit     string `json:"unit"`
	} `json:"diagnostics"`
	UnknownUnits []goldenUnknown `json:"unknown_units"`
	Summary      struct {
		Units          int `json:"units"`
		Resolved       int `json:"resolved"`
		ModuleUnknown  int `json:"module_unknown"`
		ConfigUnknown  int `json:"config_unknown"`
		UnknownModules int `json:"unknown_modules"`
		Errors         int `json:"errors"`
		Warnings       int `json:"warnings"`
	} `json:"summary"`
}

// goldenUnknown is one unknown_units entry.
type goldenUnknown struct {
	Path   string `json:"path"`
	Status string `json:"status"`
	Reason string `json:"reason"`
}

func (u goldenUnknown) String() string { return u.Path + " " + u.Status + " " + u.Reason }

// goldenDiag is the projection of a diagnostic the goldens compare.
type goldenDiag struct {
	Code, Severity, File string
	Line, Column         int
	Unit                 string
}

func (d goldenDiag) String() string {
	unit := d.Unit
	if unit == "" {
		unit = "-"
	}
	return fmt.Sprintf("%s %s %s:%d:%d %s", d.Code, d.Severity, d.File, d.Line, d.Column, unit)
}

// goldenCompare is a total order: file, line, column, unit, code, severity.
func goldenCompare(a, b goldenDiag) int {
	return cmp.Or(
		cmp.Compare(a.File, b.File),
		cmp.Compare(a.Line, b.Line),
		cmp.Compare(a.Column, b.Column),
		cmp.Compare(a.Unit, b.Unit),
		cmp.Compare(a.Code, b.Code),
		cmp.Compare(a.Severity, b.Severity),
	)
}

// goldenRun runs `gruntled check --format json dir` in process and
// decodes its stdout.
func goldenRun(t *testing.T, dir string) (rep goldenReport, stdout []byte, code int) {
	t.Helper()
	var out, errb bytes.Buffer
	code = run([]string{"check", "--format", "json", dir}, &out, &errb)
	if err := json.Unmarshal(out.Bytes(), &rep); err != nil {
		t.Fatalf("decoding JSON output (exit %d): %v\nstdout: %s\nstderr: %s", code, err, out.String(), errb.String())
	}
	if rep.Version != 1 {
		t.Fatalf("JSON schema version %d, want 1", rep.Version)
	}
	return rep, out.Bytes(), code
}

// goldenGot projects and sorts a report's diagnostics.
func goldenGot(rep goldenReport) []goldenDiag {
	got := make([]goldenDiag, 0, len(rep.Diagnostics))
	for _, d := range rep.Diagnostics {
		got = append(got, goldenDiag{Code: d.Code, Severity: d.Severity, File: d.File, Line: d.Line, Column: d.Column, Unit: d.Unit})
	}
	slices.SortFunc(got, goldenCompare)
	return got
}

// goldenDiff returns the entries of a missing from b and of b missing from
// a, counting multiplicity. Both inputs must be sorted by goldenCompare.
func goldenDiff(a, b []goldenDiag) (onlyA, onlyB []goldenDiag) {
	i, j := 0, 0
	for i < len(a) && j < len(b) {
		switch c := goldenCompare(a[i], b[j]); {
		case c < 0:
			onlyA = append(onlyA, a[i])
			i++
		case c > 0:
			onlyB = append(onlyB, b[j])
			j++
		default:
			i++
			j++
		}
	}
	onlyA = append(onlyA, a[i:]...)
	onlyB = append(onlyB, b[j:]...)
	return onlyA, onlyB
}

// goldenAssertSet fails t unless got equals want exactly, with
// multiplicity. Both must be sorted by goldenCompare.
func goldenAssertSet(t *testing.T, got, want []goldenDiag) {
	t.Helper()
	if slices.Equal(got, want) {
		return
	}
	extra, missing := goldenDiff(got, want)
	var b strings.Builder
	fmt.Fprintf(&b, "diagnostic set differs\n got (%d):\n", len(got))
	for _, d := range got {
		fmt.Fprintf(&b, "   %s\n", d)
	}
	fmt.Fprintf(&b, " want (%d):\n", len(want))
	for _, d := range want {
		fmt.Fprintf(&b, "   %s\n", d)
	}
	b.WriteString(" reported but not wanted:\n")
	for _, d := range extra {
		fmt.Fprintf(&b, "   %s\n", d)
	}
	b.WriteString(" wanted but not reported:\n")
	for _, d := range missing {
		fmt.Fprintf(&b, "   %s\n", d)
	}
	t.Fatal(b.String())
}

func TestGoldenSynthrepoFullScale(t *testing.T) {
	bad := func(n int) []synthrepo.ErrorKind {
		return slices.Repeat([]synthrepo.ErrorKind{synthrepo.BadOutputRef}, n)
	}
	cases := []struct {
		name string
		spec synthrepo.Spec
	}{
		{"seed1_depth4_fanout5", synthrepo.Spec{Units: 400, IncludeDepth: 4, DependencyFanout: 5, Seed: 1, Errors: bad(12)}},
		{"seed2_depth2_fanout8", synthrepo.Spec{Units: 400, IncludeDepth: 2, DependencyFanout: 8, Seed: 2, Errors: bad(25)}},
		{"seed3_depth3_fanout3", synthrepo.Spec{Units: 600, IncludeDepth: 3, DependencyFanout: 3, Seed: 3, Errors: bad(8)}},
		{"clean_full_scale", synthrepo.Spec{Units: 500, IncludeDepth: 4, DependencyFanout: 6, Seed: 4}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			m, err := synthrepo.Generate(tc.spec, dir)
			if err != nil {
				t.Fatalf("synthrepo.Generate: %v", err)
			}
			if len(m.Expected) != len(tc.spec.Errors) {
				t.Fatalf("manifest has %d expected diagnostics, want %d", len(m.Expected), len(tc.spec.Errors))
			}
			if len(m.Units) != tc.spec.Units {
				t.Fatalf("manifest has %d units, want %d", len(m.Units), tc.spec.Units)
			}

			want := make([]goldenDiag, 0, len(m.Expected))
			for _, e := range m.Expected {
				want = append(want, goldenDiag{
					Code:     string(e.Code),
					Severity: "error",
					File:     e.Pos.File().String(),
					Line:     e.Pos.Line(),
					Column:   e.Pos.Column(),
					Unit:     e.Unit.String(),
				})
			}
			slices.SortFunc(want, goldenCompare)

			rep, stdout, code := goldenRun(t, dir)
			wantCode := exitFindings
			if len(want) == 0 {
				wantCode = exitOK
			}
			if code != wantCode {
				t.Fatalf("exit code %d, want %d", code, wantCode)
			}
			goldenAssertSet(t, goldenGot(rep), want)
			if rep.Summary.Units != len(m.Units) {
				t.Errorf("summary.units = %d, want %d", rep.Summary.Units, len(m.Units))
			}
			if rep.Summary.ModuleUnknown != 0 || rep.Summary.ConfigUnknown != 0 || len(rep.UnknownUnits) != 0 {
				t.Errorf("unknown units: module_unknown=%d config_unknown=%d list=%v, want none",
					rep.Summary.ModuleUnknown, rep.Summary.ConfigUnknown, rep.UnknownUnits)
			}

			_, stdout2, code2 := goldenRun(t, dir)
			if code2 != code || !bytes.Equal(stdout, stdout2) {
				t.Fatalf("second run differs: exit %d vs %d, stdout identical=%v", code, code2, bytes.Equal(stdout, stdout2))
			}
		})
	}
}

// goldenFixture is a parsed txtar golden repository.
type goldenFixture struct {
	files      map[string][]byte // repository files, by slash path
	symlinks   [][2]string       // link path, relative target
	exit       int
	want       []goldenDiag
	unknown    []goldenUnknown
	hasUnknown bool
}

// goldenParseFixture parses a txtar archive into a goldenFixture. Only the
// _golden/ section is interpreted; every other file is repository content.
func goldenParseFixture(t *testing.T, path string) goldenFixture {
	t.Helper()
	ar, err := txtar.ParseFile(path)
	if err != nil {
		t.Fatalf("parsing %s: %v", path, err)
	}
	fx := goldenFixture{files: map[string][]byte{}}
	hasExit, hasWant := false, false
	for _, f := range ar.Files {
		switch f.Name {
		case "_golden/exit":
			n, err := strconv.Atoi(strings.TrimSpace(string(f.Data)))
			if err != nil {
				t.Fatalf("_golden/exit: %v", err)
			}
			fx.exit, hasExit = n, true
		case "_golden/want":
			fx.want, hasWant = goldenParseWant(t, f.Data), true
		case "_golden/unknown":
			fx.hasUnknown = true
			for _, line := range goldenLines(f.Data) {
				fields := strings.Fields(line)
				if len(fields) != 3 {
					t.Fatalf("_golden/unknown: want `path status reason`, got %q", line)
				}
				fx.unknown = append(fx.unknown, goldenUnknown{Path: fields[0], Status: fields[1], Reason: fields[2]})
			}
		case "_golden/symlinks":
			for _, line := range goldenLines(f.Data) {
				link, target, ok := strings.Cut(line, " -> ")
				if !ok {
					t.Fatalf("_golden/symlinks: want `link -> target`, got %q", line)
				}
				fx.symlinks = append(fx.symlinks, [2]string{strings.TrimSpace(link), strings.TrimSpace(target)})
			}
		default:
			if strings.HasPrefix(f.Name, "_golden/") {
				t.Fatalf("unknown golden section %q", f.Name)
			}
			if _, dup := fx.files[f.Name]; dup {
				t.Fatalf("duplicate fixture file %q", f.Name)
			}
			fx.files[f.Name] = f.Data
		}
	}
	if !hasExit || !hasWant {
		t.Fatalf("%s: _golden/exit and _golden/want are both required", path)
	}
	return fx
}

// goldenLines returns data's lines without blank lines and # comments.
func goldenLines(data []byte) []string {
	var out []string
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		out = append(out, line)
	}
	return out
}

// goldenParseWant parses `CODE SEVERITY file:line:col unit` lines. Every
// line carries an exact position; there is no wildcard.
func goldenParseWant(t *testing.T, data []byte) []goldenDiag {
	t.Helper()
	var want []goldenDiag
	for _, line := range goldenLines(data) {
		fields := strings.Fields(line)
		if len(fields) != 4 {
			t.Fatalf("_golden/want: want `CODE SEVERITY file:line:col unit`, got %q", line)
		}
		pos := fields[2]
		i := strings.LastIndexByte(pos, ':')
		j := -1
		if i > 0 {
			j = strings.LastIndexByte(pos[:i], ':')
		}
		if j <= 0 {
			t.Fatalf("_golden/want: bad position in %q", line)
		}
		ln, err1 := strconv.Atoi(pos[j+1 : i])
		col, err2 := strconv.Atoi(pos[i+1:])
		if err1 != nil || err2 != nil || ln < 1 || col < 1 {
			t.Fatalf("_golden/want: bad line:col in %q", line)
		}
		unit := fields[3]
		if unit == "-" {
			unit = ""
		}
		want = append(want, goldenDiag{Code: fields[0], Severity: fields[1], File: pos[:j], Line: ln, Column: col, Unit: unit})
	}
	slices.SortFunc(want, goldenCompare)
	return want
}

// goldenSelfCheck verifies every want position against the fixture text:
// a GRT001 position must start with "dependency.", a GRT100 position must
// be an '@' byte (the only syntax error the goldens construct).
func goldenSelfCheck(t *testing.T, fx goldenFixture) {
	t.Helper()
	for _, w := range fx.want {
		var prefix string
		switch w.Code {
		case "GRT001":
			prefix = "dependency."
		case "GRT100":
			prefix = "@"
		default:
			t.Fatalf("self-check: no position rule for code %s in want line %s", w.Code, w)
		}
		src, ok := fx.files[w.File]
		if !ok {
			t.Fatalf("self-check: want line %s names a file not in the fixture", w)
		}
		lines := strings.Split(string(src), "\n")
		if w.Line > len(lines) {
			t.Fatalf("self-check: want line %s: file has only %d lines", w, len(lines))
		}
		text := lines[w.Line-1]
		if w.Column-1 > len(text) || !strings.HasPrefix(text[w.Column-1:], prefix) {
			t.Fatalf("self-check: want line %s: bytes at %d:%d do not start with %q (line is %q)", w, w.Line, w.Column, prefix, text)
		}
	}
}

// goldenMaterialize writes fx's files and symlinks under dir.
func goldenMaterialize(t *testing.T, fx goldenFixture, dir string) {
	t.Helper()
	for name, data := range fx.files {
		p := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, data, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	for _, s := range fx.symlinks {
		p := filepath.Join(dir, filepath.FromSlash(s[0]))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(filepath.FromSlash(s[1]), p); err != nil {
			t.Fatal(err)
		}
	}
}

func TestGoldenFixtures(t *testing.T) {
	paths, err := filepath.Glob(filepath.Join("testdata", "golden", "*.txtar"))
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range paths {
		name := strings.TrimSuffix(filepath.Base(path), ".txtar")
		t.Run(name, func(t *testing.T) {
			fx := goldenParseFixture(t, path)
			if len(fx.symlinks) > 0 && runtime.GOOS == "windows" {
				t.Skip("fixture needs symlinks")
			}
			goldenSelfCheck(t, fx)

			dir := t.TempDir()
			goldenMaterialize(t, fx, dir)

			rep, stdout, code := goldenRun(t, dir)
			if code != fx.exit {
				t.Errorf("exit code %d, want %d", code, fx.exit)
			}
			goldenAssertSet(t, goldenGot(rep), fx.want)
			if fx.hasUnknown {
				got := slices.Clone(rep.UnknownUnits)
				want := slices.Clone(fx.unknown)
				byPath := func(a, b goldenUnknown) int { return cmp.Compare(a.String(), b.String()) }
				slices.SortFunc(got, byPath)
				slices.SortFunc(want, byPath)
				if !slices.Equal(got, want) {
					t.Errorf("unknown_units differ\n got: %v\nwant: %v", got, want)
				}
			}

			_, stdout2, code2 := goldenRun(t, dir)
			if code2 != code || !bytes.Equal(stdout, stdout2) {
				t.Fatalf("second run differs: exit %d vs %d, stdout identical=%v", code, code2, bytes.Equal(stdout, stdout2))
			}
		})
	}
}
