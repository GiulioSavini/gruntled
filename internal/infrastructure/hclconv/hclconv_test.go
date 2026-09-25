package hclconv_test

import (
	"testing"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"

	"github.com/GiulioSavini/gruntled/internal/domain/repograph"
	"github.com/GiulioSavini/gruntled/internal/infrastructure/hclconv"
)

func mustFile(t *testing.T) repograph.RepoPath {
	t.Helper()
	return repograph.MustRepoPath("test.hcl")
}

// findScopeTraversal returns the SrcRange.Start of the first
// *hclsyntax.ScopeTraversalExpr found by walking expr.
func findScopeTraversal(t *testing.T, expr hclsyntax.Expression) hcl.Pos {
	t.Helper()
	var found *hcl.Pos
	diags := hclsyntax.VisitAll(expr, func(n hclsyntax.Node) hcl.Diagnostics {
		if st, ok := n.(*hclsyntax.ScopeTraversalExpr); ok && found == nil {
			p := st.SrcRange.Start
			found = &p
		}
		return nil
	})
	if diags.HasErrors() {
		t.Fatalf("VisitAll: %v", diags)
	}
	if found == nil {
		t.Fatalf("no ScopeTraversalExpr found")
	}
	return *found
}

// TestPositionNonASCIIPrefixUsesBytes proves Position recomputes from
// hcl.Pos.Byte, not hcl.Pos.Column: a non-ASCII prefix ("ééé", 3 characters
// each 2 bytes in UTF-8) on the line makes hcl's own grapheme-counted Column
// diverge from the byte column Position returns. Both numbers below were
// verified against hcl v2.25.0 for this exact source string; hcl's Column
// is documented here precisely so a future hcl upgrade that changes it is
// caught by this test rather than silently trusted.
func TestPositionNonASCIIPrefixUsesBytes(t *testing.T) {
	src := []byte(`a = "ééé" == "" ? dependency.x.outputs.y : ""`)
	f, diags := hclsyntax.ParseConfig(src, "test.hcl", hcl.InitialPos)
	if diags.HasErrors() {
		t.Fatalf("ParseConfig: %v", diags)
	}
	body := f.Body.(*hclsyntax.Body)
	attr, ok := body.Attributes["a"]
	if !ok {
		t.Fatalf("attribute \"a\" not found")
	}
	p := findScopeTraversal(t, attr.Expr)

	// Document why byte-based recomputation is necessary: hcl's own
	// grapheme-counted Column at this position is 19, not the byte column.
	if p.Column != 19 {
		t.Fatalf("hcl.Pos.Column = %d, want 19 (documents grapheme-cluster counting)", p.Column)
	}

	got, err := hclconv.Position(mustFile(t), src, p)
	if err != nil {
		t.Fatalf("Position: %v", err)
	}
	if got.Line() != 1 || got.Column() != 22 {
		t.Fatalf("Position = %s, want test.hcl:1:22", got)
	}
}

// TestPositionCRLFLineThree proves line and byte-column tracking survives
// CRLF line endings: a reference on line 3, after two CRLF-terminated
// lines, still resolves to the correct line and byte column.
func TestPositionCRLFLineThree(t *testing.T) {
	src := []byte("a = 1\r\nb = 2\r\nc = dependency.vpc.outputs.id\r\n")
	f, diags := hclsyntax.ParseConfig(src, "crlf.hcl", hcl.InitialPos)
	if diags.HasErrors() {
		t.Fatalf("ParseConfig: %v", diags)
	}
	body := f.Body.(*hclsyntax.Body)
	attr, ok := body.Attributes["c"]
	if !ok {
		t.Fatalf("attribute \"c\" not found")
	}
	p := findScopeTraversal(t, attr.Expr)

	got, err := hclconv.Position(repograph.MustRepoPath("crlf.hcl"), src, p)
	if err != nil {
		t.Fatalf("Position: %v", err)
	}
	if got.Line() != 3 || got.Column() != 5 {
		t.Fatalf("Position = %s, want crlf.hcl:3:5", got)
	}
}

// TestPositionOffsetClamped proves an out-of-range byte offset is clamped
// into [0, len(src)] instead of panicking or indexing out of bounds.
func TestPositionOffsetClamped(t *testing.T) {
	src := []byte("a = 1\n")
	file := repograph.MustRepoPath("clamp.hcl")

	got, err := hclconv.Position(file, src, hcl.Pos{Line: 1, Column: 1, Byte: -5})
	if err != nil {
		t.Fatalf("Position (negative): %v", err)
	}
	if got.Line() != 1 || got.Column() != 1 {
		t.Fatalf("Position (negative) = %s, want clamp.hcl:1:1", got)
	}

	got, err = hclconv.Position(file, src, hcl.Pos{Line: 99, Column: 99, Byte: 999})
	if err != nil {
		t.Fatalf("Position (overflow): %v", err)
	}
	// src is "a = 1\n" (6 bytes); clamped to len(src)=6, which is the byte
	// right after the trailing newline: line 2, column 1.
	if got.Line() != 2 || got.Column() != 1 {
		t.Fatalf("Position (overflow) = %s, want clamp.hcl:2:1", got)
	}
}

// TestFirstSyntaxErrorUnterminatedBlock proves a mid-edit, unterminated
// block yields exactly one GRT100 diagnostic, deterministically across many
// runs, and never panics.
func TestFirstSyntaxErrorUnterminatedBlock(t *testing.T) {
	src := []byte("dependency \"vpc\" {\n  config_path = \"../vpc\"\n")
	file := repograph.MustRepoPath("unterminated.hcl")

	var want string
	for i := 0; i < 20; i++ {
		_, diags := hclsyntax.ParseConfig(src, "unterminated.hcl", hcl.InitialPos)
		d, ok, err := hclconv.FirstSyntaxError(file, src, diags)
		if err != nil {
			t.Fatalf("run %d: FirstSyntaxError: %v", i, err)
		}
		if !ok {
			t.Fatalf("run %d: ok = false, want true", i)
		}
		if d.Code() != "GRT100" {
			t.Fatalf("run %d: Code = %s, want GRT100", i, d.Code())
		}
		if unit, hasUnit := d.Unit(); hasUnit {
			t.Fatalf("run %d: Unit = %s, want file-level (zero)", i, unit)
		}
		got := d.Message()
		if i == 0 {
			want = got
		} else if got != want {
			t.Fatalf("run %d: Message = %q, want %q (non-deterministic)", i, got, want)
		}
	}
}

// TestFirstSyntaxErrorTrailingDot proves a mid-edit expression
// (`dependency.vpc.outputs.` with a trailing dot and no attribute name)
// also yields exactly one GRT100.
func TestFirstSyntaxErrorTrailingDot(t *testing.T) {
	src := []byte("inputs = { a = dependency.vpc.outputs. }")
	file := repograph.MustRepoPath("trailingdot.hcl")

	_, diags := hclsyntax.ParseConfig(src, "trailingdot.hcl", hcl.InitialPos)
	d, ok, err := hclconv.FirstSyntaxError(file, src, diags)
	if err != nil {
		t.Fatalf("FirstSyntaxError: %v", err)
	}
	if !ok {
		t.Fatalf("ok = false, want true")
	}
	if d.Code() != "GRT100" {
		t.Fatalf("Code = %s, want GRT100", d.Code())
	}
}

// TestFirstSyntaxErrorNonUTF8NoPanic proves non-UTF-8 bytes inside a string
// produce exactly one GRT100 and never panic, even though hclsyntax reports
// two error diagnostics at the same Subject start (deterministic tie-break
// by Summary).
func TestFirstSyntaxErrorNonUTF8NoPanic(t *testing.T) {
	src := []byte("a = \"\xff\xfe\"\n")
	file := repograph.MustRepoPath("badutf8.hcl")

	_, diags := hclsyntax.ParseConfig(src, "badutf8.hcl", hcl.InitialPos)
	if len(diags.Errs()) < 2 {
		t.Fatalf("expected at least 2 error diagnostics for non-UTF-8 input, got %d", len(diags.Errs()))
	}

	d, ok, err := hclconv.FirstSyntaxError(file, src, diags)
	if err != nil {
		t.Fatalf("FirstSyntaxError: %v", err)
	}
	if !ok {
		t.Fatalf("ok = false, want true")
	}
	if d.Code() != "GRT100" {
		t.Fatalf("Code = %s, want GRT100", d.Code())
	}
	if got, want := d.Message(), "Invalid character encoding: All input files must be UTF-8 encoded. Ensure that UTF-8 encoding is selected in your editor."; got != want {
		t.Fatalf("Message = %q, want %q", got, want)
	}
}

// TestFirstSyntaxErrorOnlyWarnings proves diagnostics containing only
// warnings (no error-severity entry) give ok = false.
func TestFirstSyntaxErrorOnlyWarnings(t *testing.T) {
	file := repograph.MustRepoPath("warn.hcl")
	src := []byte("a = 1\n")
	diags := hcl.Diagnostics{
		{
			Severity: hcl.DiagWarning,
			Summary:  "a warning",
			Subject:  &hcl.Range{Filename: "warn.hcl", Start: hcl.Pos{Line: 1, Column: 1, Byte: 0}},
		},
	}

	_, ok, err := hclconv.FirstSyntaxError(file, src, diags)
	if err != nil {
		t.Fatalf("FirstSyntaxError: %v", err)
	}
	if ok {
		t.Fatalf("ok = true, want false (warnings only)")
	}
}

// TestFirstSyntaxErrorNoDiagnostics proves an empty Diagnostics gives
// ok = false.
func TestFirstSyntaxErrorNoDiagnostics(t *testing.T) {
	file := repograph.MustRepoPath("clean.hcl")
	src := []byte("a = 1\n")

	_, ok, err := hclconv.FirstSyntaxError(file, src, nil)
	if err != nil {
		t.Fatalf("FirstSyntaxError: %v", err)
	}
	if ok {
		t.Fatalf("ok = true, want false (no diagnostics)")
	}
}

// TestFirstSyntaxErrorNilSubject proves a diagnostic with a nil Subject
// becomes position 1:1.
func TestFirstSyntaxErrorNilSubject(t *testing.T) {
	file := repograph.MustRepoPath("nilsubject.hcl")
	src := []byte("a = 1\n")
	diags := hcl.Diagnostics{
		{
			Severity: hcl.DiagError,
			Summary:  "something broke",
		},
	}

	d, ok, err := hclconv.FirstSyntaxError(file, src, diags)
	if err != nil {
		t.Fatalf("FirstSyntaxError: %v", err)
	}
	if !ok {
		t.Fatalf("ok = false, want true")
	}
	if d.Pos().Line() != 1 || d.Pos().Column() != 1 {
		t.Fatalf("Pos = %s, want nilsubject.hcl:1:1", d.Pos())
	}
	if d.Message() != "something broke" {
		t.Fatalf("Message = %q, want %q", d.Message(), "something broke")
	}
}

// TestFirstSyntaxErrorPicksSmallestOffset proves the error with the
// smallest Subject.Start.Byte wins when multiple error diagnostics are
// present at different positions.
func TestFirstSyntaxErrorPicksSmallestOffset(t *testing.T) {
	file := repograph.MustRepoPath("multi.hcl")
	src := []byte("aaaaaaaaaa\n")
	diags := hcl.Diagnostics{
		{
			Severity: hcl.DiagError,
			Summary:  "second",
			Subject:  &hcl.Range{Filename: "multi.hcl", Start: hcl.Pos{Line: 1, Column: 6, Byte: 5}},
		},
		{
			Severity: hcl.DiagError,
			Summary:  "first",
			Subject:  &hcl.Range{Filename: "multi.hcl", Start: hcl.Pos{Line: 1, Column: 1, Byte: 0}},
		},
	}

	d, ok, err := hclconv.FirstSyntaxError(file, src, diags)
	if err != nil {
		t.Fatalf("FirstSyntaxError: %v", err)
	}
	if !ok {
		t.Fatalf("ok = false, want true")
	}
	if d.Message() != "first" {
		t.Fatalf("Message = %q, want %q", d.Message(), "first")
	}
}
