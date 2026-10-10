package presenter_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"
	"unicode"
	"unicode/utf8"

	"github.com/GiulioSavini/gruntled/internal/domain/diagnostic"
	"github.com/GiulioSavini/gruntled/internal/domain/repograph"
	"github.com/GiulioSavini/gruntled/internal/interfaces/presenter"
)

// Real reason strings, copied from internal/infrastructure/terragrunt
// (reasons.go) and internal/infrastructure/tfsurface (reader.go). The
// presenter package must not import infrastructure, so they are repeated
// here as literals.
const (
	reasonSyntaxError      = "syntax-error"
	reasonRemoteSource     = "remote-source"
	reasonNoTerraformFiles = "no-terraform-files"
)

func rp(t *testing.T, s string) repograph.RepoPath {
	t.Helper()
	p, err := repograph.NewRepoPath(s)
	if err != nil {
		t.Fatalf("NewRepoPath(%q): %v", s, err)
	}
	return p
}

func pos(t *testing.T, file string, line, col int) repograph.Position {
	t.Helper()
	p, err := repograph.NewPosition(rp(t, file), line, col)
	if err != nil {
		t.Fatalf("NewPosition(%q, %d, %d): %v", file, line, col, err)
	}
	return p
}

func unitDiag(t *testing.T, unit, file string, line, col int, msg string) diagnostic.Diagnostic {
	t.Helper()
	d, err := diagnostic.NewForUnit(diagnostic.CodeUnknownOutput, diagnostic.SeverityError, rp(t, unit), pos(t, file, line, col), msg)
	if err != nil {
		t.Fatalf("NewForUnit: %v", err)
	}
	return d
}

func fileDiag(t *testing.T, file string, line, col int, msg string) diagnostic.Diagnostic {
	t.Helper()
	d, err := diagnostic.New(diagnostic.CodeSyntaxError, diagnostic.SeverityError, pos(t, file, line, col), msg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return d
}

func emptyGraph(t *testing.T) *repograph.RepositoryGraph {
	t.Helper()
	g, err := repograph.NewRepositoryGraph(nil, nil)
	if err != nil {
		t.Fatalf("NewRepositoryGraph: %v", err)
	}
	return g
}

// mixedGraph holds one resolved unit, one config-unknown unit, one
// module-unknown unit, one known module and one unknown-surface module.
func mixedGraph(t *testing.T) *repograph.RepositoryGraph {
	t.Helper()
	surface, err := repograph.NewSurface([]string{"cidr"}, []string{"vpc_id"})
	if err != nil {
		t.Fatalf("NewSurface: %v", err)
	}
	vpcMod, err := repograph.NewModule(rp(t, "modules/vpc"), surface)
	if err != nil {
		t.Fatalf("NewModule: %v", err)
	}
	emptyMod, err := repograph.NewUnknownModule(rp(t, "modules/empty"), reasonNoTerraformFiles)
	if err != nil {
		t.Fatalf("NewUnknownModule: %v", err)
	}
	resolved, err := repograph.NewResolvedUnit(rp(t, "live/vpc"), rp(t, "modules/vpc"), nil, nil)
	if err != nil {
		t.Fatalf("NewResolvedUnit: %v", err)
	}
	broken, err := repograph.NewConfigUnknownUnit(rp(t, "live/broken"), reasonSyntaxError)
	if err != nil {
		t.Fatalf("NewConfigUnknownUnit: %v", err)
	}
	remote, err := repograph.NewModuleUnknownUnit(rp(t, "live/remote"), reasonRemoteSource, nil, nil)
	if err != nil {
		t.Fatalf("NewModuleUnknownUnit: %v", err)
	}
	g, err := repograph.NewRepositoryGraph(
		[]repograph.Unit{resolved, remote, broken},
		[]repograph.Module{vpcMod, emptyMod},
	)
	if err != nil {
		t.Fatalf("NewRepositoryGraph: %v", err)
	}
	return g
}

type failingWriter struct{ err error }

func (f failingWriter) Write([]byte) (int, error) { return 0, f.err }

func TestTextEmptySetWritesNothing(t *testing.T) {
	var b bytes.Buffer
	if err := presenter.Text(&b, diagnostic.NewSet()); err != nil {
		t.Fatalf("Text: %v", err)
	}
	if b.Len() != 0 {
		t.Fatalf("Text on empty Set wrote %q, want nothing", b.String())
	}
}

func TestTextUnitDiagnostic(t *testing.T) {
	msg := `dependency "vpc" output "vpc_idd" is not declared by module "modules/vpc"`
	set := diagnostic.NewSet(unitDiag(t, "live/app", "live/app/terragrunt.hcl", 7, 17, msg))
	var b strings.Builder
	if err := presenter.Text(&b, set); err != nil {
		t.Fatalf("Text: %v", err)
	}
	want := "live/app/terragrunt.hcl:7:17: GRT001 " + msg + " (unit live/app)\n"
	if b.String() != want {
		t.Fatalf("Text =\n%q\nwant\n%q", b.String(), want)
	}
}

func TestTextFileLevelDiagnosticHasNoUnitSuffix(t *testing.T) {
	msg := "Unclosed configuration block"
	set := diagnostic.NewSet(fileDiag(t, "root.hcl", 3, 1, msg))
	var b strings.Builder
	if err := presenter.Text(&b, set); err != nil {
		t.Fatalf("Text: %v", err)
	}
	want := "root.hcl:3:1: GRT100 " + msg + "\n"
	if b.String() != want {
		t.Fatalf("Text =\n%q\nwant\n%q", b.String(), want)
	}
}

func TestTextSharedIncludeTwoUnits(t *testing.T) {
	msg := `dependency "vpc" output "nope" is not declared by module "modules/vpc"`
	// Built in reverse order: the Set must reorder them canonically.
	set := diagnostic.NewSet(
		unitDiag(t, "live/b", "common/deps.hcl", 4, 9, msg),
		unitDiag(t, "live/a", "common/deps.hcl", 4, 9, msg),
	)
	var b strings.Builder
	if err := presenter.Text(&b, set); err != nil {
		t.Fatalf("Text: %v", err)
	}
	want := "common/deps.hcl:4:9: GRT001 " + msg + " (unit live/a)\n" +
		"common/deps.hcl:4:9: GRT001 " + msg + " (unit live/b)\n"
	if b.String() != want {
		t.Fatalf("Text =\n%q\nwant\n%q", b.String(), want)
	}
}

func TestJSONEmpty(t *testing.T) {
	var b bytes.Buffer
	if err := presenter.JSON(&b, emptyGraph(t), diagnostic.NewSet()); err != nil {
		t.Fatalf("JSON: %v", err)
	}
	want := `{
  "version": 1,
  "diagnostics": [],
  "unknown_units": [],
  "unknown_modules": [],
  "summary": {
    "units": 0,
    "resolved": 0,
    "module_unknown": 0,
    "config_unknown": 0,
    "unknown_modules": 0,
    "errors": 0,
    "warnings": 0
  }
}
`
	if b.String() != want {
		t.Fatalf("JSON =\n%s\nwant\n%s", b.String(), want)
	}
}

func mixedSet(t *testing.T) diagnostic.Set {
	t.Helper()
	return diagnostic.NewSet(
		unitDiag(t, "live/app", "live/app/terragrunt.hcl", 7, 17, `output "a<b>&c" is not declared`),
		fileDiag(t, "live/broken/terragrunt.hcl", 3, 1, "Unclosed configuration block"),
	)
}

func TestJSONMixed(t *testing.T) {
	var b bytes.Buffer
	if err := presenter.JSON(&b, mixedGraph(t), mixedSet(t)); err != nil {
		t.Fatalf("JSON: %v", err)
	}
	want := `{
  "version": 1,
  "diagnostics": [
    {
      "code": "GRT001",
      "severity": "error",
      "file": "live/app/terragrunt.hcl",
      "line": 7,
      "column": 17,
      "unit": "live/app",
      "message": "output \"a<b>&c\" is not declared"
    },
    {
      "code": "GRT100",
      "severity": "error",
      "file": "live/broken/terragrunt.hcl",
      "line": 3,
      "column": 1,
      "message": "Unclosed configuration block"
    }
  ],
  "unknown_units": [
    {
      "path": "live/broken",
      "status": "config-unknown",
      "reason": "syntax-error"
    },
    {
      "path": "live/remote",
      "status": "module-unknown",
      "reason": "remote-source"
    }
  ],
  "unknown_modules": [
    {
      "path": "modules/empty",
      "reason": "no-terraform-files"
    }
  ],
  "summary": {
    "units": 3,
    "resolved": 1,
    "module_unknown": 1,
    "config_unknown": 1,
    "unknown_modules": 1,
    "errors": 2,
    "warnings": 0
  }
}
`
	if b.String() != want {
		t.Fatalf("JSON =\n%s\nwant\n%s", b.String(), want)
	}
}

func TestJSONDeterministic(t *testing.T) {
	g := mixedGraph(t)
	set := mixedSet(t)
	var a, b bytes.Buffer
	if err := presenter.JSON(&a, g, set); err != nil {
		t.Fatalf("JSON: %v", err)
	}
	if err := presenter.JSON(&b, g, set); err != nil {
		t.Fatalf("JSON: %v", err)
	}
	if !bytes.Equal(a.Bytes(), b.Bytes()) {
		t.Fatalf("JSON not deterministic:\n%s\n---\n%s", a.String(), b.String())
	}
}

func TestSummaryCounts(t *testing.T) {
	var b strings.Builder
	set := diagnostic.NewSet(unitDiag(t, "live/app", "live/app/terragrunt.hcl", 7, 17, "x"))
	// mixedGraph has 3 units, 2 of them unknown; build a 3-unit graph with 1 unknown.
	surface, err := repograph.NewSurface(nil, []string{"id"})
	if err != nil {
		t.Fatalf("NewSurface: %v", err)
	}
	mod, err := repograph.NewModule(rp(t, "modules/m"), surface)
	if err != nil {
		t.Fatalf("NewModule: %v", err)
	}
	var units []repograph.Unit
	for _, p := range []string{"live/a", "live/b"} {
		u, err := repograph.NewResolvedUnit(rp(t, p), rp(t, "modules/m"), nil, nil)
		if err != nil {
			t.Fatalf("NewResolvedUnit: %v", err)
		}
		units = append(units, u)
	}
	bad, err := repograph.NewConfigUnknownUnit(rp(t, "live/c"), reasonSyntaxError)
	if err != nil {
		t.Fatalf("NewConfigUnknownUnit: %v", err)
	}
	units = append(units, bad)
	g, err := repograph.NewRepositoryGraph(units, []repograph.Module{mod})
	if err != nil {
		t.Fatalf("NewRepositoryGraph: %v", err)
	}
	if err := presenter.Summary(&b, g, set); err != nil {
		t.Fatalf("Summary: %v", err)
	}
	want := "gruntled: checked 3 units (1 unknown): 1 errors, 0 warnings\n"
	if b.String() != want {
		t.Fatalf("Summary = %q, want %q", b.String(), want)
	}
}

func TestSummaryNoUnits(t *testing.T) {
	var b strings.Builder
	if err := presenter.Summary(&b, emptyGraph(t), diagnostic.NewSet()); err != nil {
		t.Fatalf("Summary: %v", err)
	}
	want := "gruntled: no Terragrunt units found\n"
	if b.String() != want {
		t.Fatalf("Summary = %q, want %q", b.String(), want)
	}
}

func TestWriterErrorsAreReturned(t *testing.T) {
	sentinel := errors.New("disk full")
	w := failingWriter{err: sentinel}
	set := mixedSet(t)
	g := mixedGraph(t)
	cases := map[string]func(io.Writer) error{
		"Text":    func(w io.Writer) error { return presenter.Text(w, set) },
		"JSON":    func(w io.Writer) error { return presenter.JSON(w, g, set) },
		"Summary": func(w io.Writer) error { return presenter.Summary(w, g, set) },
		"SummaryNoUnits": func(w io.Writer) error {
			return presenter.Summary(w, emptyGraph(t), diagnostic.NewSet())
		},
	}
	for name, fn := range cases {
		t.Run(name, func(t *testing.T) {
			if err := fn(w); !errors.Is(err, sentinel) {
				t.Fatalf("%s returned %v, want %v", name, err, sentinel)
			}
		})
	}
}

// crafted holds an OSC title sequence (ESC ... BEL), a C1 CSI (U+009B),
// DEL and a bidi override (U+202E): a repo-controlled name that must never
// reach a terminal raw.
const crafted = "c\x1b]0;X\x07\u009b2Jd\x7fe\u202ef"

// craftedTerm is crafted as escapeTerm writes it.
const craftedTerm = `c\x1b]0;X\x07\u009b2Jd\x7fe\u202ef`

// assertTerminalSafe fails when out holds a byte or rune a terminal could
// act on: a C0 control other than '\n', DEL, invalid UTF-8, a C1 control,
// a format (Cf) rune or U+2028/U+2029.
func assertTerminalSafe(t *testing.T, out []byte) {
	t.Helper()
	if !utf8.Valid(out) {
		t.Fatalf("output is not valid UTF-8:\n%q", out)
	}
	for _, r := range string(out) {
		if r == '\n' {
			continue
		}
		if r < 0x20 || (r >= 0x7f && r <= 0x9f) || r == 0x2028 || r == 0x2029 || unicode.Is(unicode.Cf, r) {
			t.Fatalf("output holds raw %U:\n%q", r, out)
		}
	}
}

func TestTextEscapesControls(t *testing.T) {
	unit := "live/" + crafted
	file := unit + "/terragrunt.hcl"
	set := diagnostic.NewSet(
		unitDiag(t, unit, file, 3, 1, "bad "+crafted+"\nsecond line"),
		fileDiag(t, file, 1, 1, crafted),
	)
	var b bytes.Buffer
	if err := presenter.Text(&b, set); err != nil {
		t.Fatalf("Text: %v", err)
	}
	got := b.String()
	assertTerminalSafe(t, b.Bytes())
	if n := strings.Count(got, "\n"); n != 2 {
		t.Fatalf("Text wrote %d lines, want 2:\n%q", n, got)
	}
	wantFile := "live/" + craftedTerm + "/terragrunt.hcl"
	for _, want := range []string{
		wantFile + ":1:1: GRT100 " + craftedTerm + "\n",
		wantFile + ":3:1: GRT001 bad " + craftedTerm + `\x0asecond line (unit live/` + craftedTerm + ")\n",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("Text output lacks %q:\n%q", want, got)
		}
	}
}

// craftedGraph has one config-unknown unit and one unknown module whose
// paths and reasons carry crafted.
func craftedGraph(t *testing.T) *repograph.RepositoryGraph {
	t.Helper()
	u, err := repograph.NewConfigUnknownUnit(rp(t, "live/"+crafted), "reason "+crafted)
	if err != nil {
		t.Fatalf("NewConfigUnknownUnit: %v", err)
	}
	m, err := repograph.NewUnknownModule(rp(t, "modules/"+crafted), "reason "+crafted)
	if err != nil {
		t.Fatalf("NewUnknownModule: %v", err)
	}
	g, err := repograph.NewRepositoryGraph([]repograph.Unit{u}, []repograph.Module{m})
	if err != nil {
		t.Fatalf("NewRepositoryGraph: %v", err)
	}
	return g
}

// assertJSONEscaped checks a JSON document holding crafted: no raw byte a
// terminal acts on, the runes encoding/json leaves raw written as \uXXXX,
// and want decoded back exactly by get.
func assertJSONEscaped(t *testing.T, out []byte, get func(any) string, want string) {
	t.Helper()
	assertTerminalSafe(t, out)
	for _, esc := range []string{`\u009b`, `\u007f`, `\u202e`, `\u001b`, `\u0007`} {
		if !bytes.Contains(out, []byte(esc)) {
			t.Errorf("output lacks %s:\n%s", esc, out)
		}
	}
	var v any
	if err := json.Unmarshal(out, &v); err != nil {
		t.Fatalf("Unmarshal: %v\n%s", err, out)
	}
	if got := get(v); got != want {
		t.Fatalf("decoded %q, want %q", got, want)
	}
}

// jpath walks a decoded JSON value by object keys and array indexes.
func jpath(v any, keys ...any) any {
	for _, k := range keys {
		switch k := k.(type) {
		case string:
			m, _ := v.(map[string]any)
			v = m[k]
		case int:
			a, _ := v.([]any)
			if k >= len(a) {
				return nil
			}
			v = a[k]
		}
	}
	return v
}

func jstring(keys ...any) func(any) string {
	return func(v any) string {
		s, _ := jpath(v, keys...).(string)
		return s
	}
}

func craftedSet(t *testing.T) diagnostic.Set {
	t.Helper()
	unit := "live/" + crafted
	return diagnostic.NewSet(unitDiag(t, unit, unit+"/terragrunt.hcl", 3, 1, "bad "+crafted))
}

func TestJSONEscapesTerminalRunes(t *testing.T) {
	var b bytes.Buffer
	if err := presenter.JSON(&b, craftedGraph(t), craftedSet(t)); err != nil {
		t.Fatalf("JSON: %v", err)
	}
	assertJSONEscaped(t, b.Bytes(), jstring("diagnostics", 0, "file"), "live/"+crafted+"/terragrunt.hcl")
	assertJSONEscaped(t, b.Bytes(), jstring("unknown_units", 0, "path"), "live/"+crafted)
	assertJSONEscaped(t, b.Bytes(), jstring("diagnostics", 0, "message"), "bad "+crafted)
}

func TestSARIFEscapesTerminalRunes(t *testing.T) {
	var b bytes.Buffer
	if err := presenter.SARIF(&b, craftedGraph(t), craftedSet(t), presenter.ToolInfo{Version: "dev"}); err != nil {
		t.Fatalf("SARIF: %v", err)
	}
	assertJSONEscaped(t, b.Bytes(), jstring("runs", 0, "results", 0, "message", "text"),
		"bad "+crafted+" (unit live/"+crafted+")")
}

func TestGraphEscapesTerminalRunes(t *testing.T) {
	var b bytes.Buffer
	if err := presenter.Graph(&b, craftedGraph(t)); err != nil {
		t.Fatalf("Graph: %v", err)
	}
	assertJSONEscaped(t, b.Bytes(), jstring("units", 0, "path"), "live/"+crafted)
	assertJSONEscaped(t, b.Bytes(), jstring("modules", 0, "reason"), "reason "+crafted)
}
