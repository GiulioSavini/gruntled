package diagnostic_test

import (
	"testing"

	"github.com/GiulioSavini/gruntled/internal/domain/diagnostic"
	"github.com/GiulioSavini/gruntled/internal/domain/repograph"
)

func mustPosition(t *testing.T, file string, line, column int) repograph.Position {
	t.Helper()
	p, err := repograph.NewPosition(repograph.MustRepoPath(file), line, column)
	if err != nil {
		t.Fatalf("NewPosition(%q, %d, %d): unexpected error: %v", file, line, column, err)
	}
	return p
}

func TestParseCode(t *testing.T) {
	valid := []string{"GRT001", "GRT100"}
	for _, s := range valid {
		c, err := diagnostic.ParseCode(s)
		if err != nil {
			t.Errorf("ParseCode(%q): unexpected error: %v", s, err)
			continue
		}
		if !c.Valid() {
			t.Errorf("ParseCode(%q).Valid() = false, want true", s)
		}
	}

	invalid := []string{"GRT1", "grt001", "GRT0012", "XYZ001", "GRT00a"}
	for _, s := range invalid {
		if _, err := diagnostic.ParseCode(s); err == nil {
			t.Errorf("ParseCode(%q): expected error, got nil", s)
		}
	}
}

func TestCodeValid(t *testing.T) {
	if !diagnostic.CodeUnknownOutput.Valid() {
		t.Errorf("CodeUnknownOutput.Valid() = false, want true")
	}
	if diagnostic.Code("bogus").Valid() {
		t.Errorf("Code(\"bogus\").Valid() = true, want false")
	}
}

func TestSeverityString(t *testing.T) {
	if diagnostic.SeverityError.String() != "error" {
		t.Errorf("SeverityError.String() = %q, want %q", diagnostic.SeverityError.String(), "error")
	}
	if diagnostic.SeverityWarning.String() != "warning" {
		t.Errorf("SeverityWarning.String() = %q, want %q", diagnostic.SeverityWarning.String(), "warning")
	}
}

func TestNewRejectsInvalidInput(t *testing.T) {
	pos := mustPosition(t, "a.hcl", 1, 1)
	var zeroPos repograph.Position

	if _, err := diagnostic.New(diagnostic.Code("bogus"), diagnostic.SeverityError, pos, "msg"); err == nil {
		t.Errorf("New with invalid code: expected error, got nil")
	}
	if _, err := diagnostic.New(diagnostic.CodeUnknownOutput, diagnostic.Severity(0), pos, "msg"); err == nil {
		t.Errorf("New with Severity(0): expected error, got nil")
	}
	if _, err := diagnostic.New(diagnostic.CodeUnknownOutput, diagnostic.SeverityError, zeroPos, "msg"); err == nil {
		t.Errorf("New with zero Position: expected error, got nil")
	}
	if _, err := diagnostic.New(diagnostic.CodeUnknownOutput, diagnostic.SeverityError, pos, ""); err == nil {
		t.Errorf("New with empty message: expected error, got nil")
	}
}

func TestNewAccessors(t *testing.T) {
	pos := mustPosition(t, "a.hcl", 1, 1)
	d, err := diagnostic.New(diagnostic.CodeUnknownOutput, diagnostic.SeverityError, pos, "bad output reference")
	if err != nil {
		t.Fatalf("New: unexpected error: %v", err)
	}
	if d.Code() != diagnostic.CodeUnknownOutput {
		t.Errorf("Code() = %v, want %v", d.Code(), diagnostic.CodeUnknownOutput)
	}
	if d.Severity() != diagnostic.SeverityError {
		t.Errorf("Severity() = %v, want %v", d.Severity(), diagnostic.SeverityError)
	}
	if d.Pos() != pos {
		t.Errorf("Pos() = %v, want %v", d.Pos(), pos)
	}
	if d.Message() != "bad output reference" {
		t.Errorf("Message() = %q, want %q", d.Message(), "bad output reference")
	}
}

func TestKeyEqualityIgnoresSeverityButNotLine(t *testing.T) {
	pos := mustPosition(t, "a.hcl", 1, 1)
	dError, err := diagnostic.New(diagnostic.CodeUnknownOutput, diagnostic.SeverityError, pos, "msg")
	if err != nil {
		t.Fatalf("New: unexpected error: %v", err)
	}
	dWarning, err := diagnostic.New(diagnostic.CodeUnknownOutput, diagnostic.SeverityWarning, pos, "msg")
	if err != nil {
		t.Fatalf("New: unexpected error: %v", err)
	}
	if dError.Key() != dWarning.Key() {
		t.Errorf("Key() differs by severity alone: %v vs %v", dError.Key(), dWarning.Key())
	}

	pos2 := mustPosition(t, "a.hcl", 2, 1)
	dOtherLine, err := diagnostic.New(diagnostic.CodeUnknownOutput, diagnostic.SeverityError, pos2, "msg")
	if err != nil {
		t.Fatalf("New: unexpected error: %v", err)
	}
	if dError.Key() == dOtherLine.Key() {
		t.Errorf("Key() equal despite differing line: %v", dError.Key())
	}
}

func TestCompare(t *testing.T) {
	posA1 := mustPosition(t, "a.hcl", 1, 1)
	posA2 := mustPosition(t, "a.hcl", 2, 1)
	posB1 := mustPosition(t, "b.hcl", 1, 1)

	d1, err := diagnostic.New(diagnostic.CodeUnknownOutput, diagnostic.SeverityError, posA1, "msg")
	if err != nil {
		t.Fatalf("New: unexpected error: %v", err)
	}
	d2, err := diagnostic.New(diagnostic.CodeUnknownOutput, diagnostic.SeverityError, posA2, "msg")
	if err != nil {
		t.Fatalf("New: unexpected error: %v", err)
	}
	d3, err := diagnostic.New(diagnostic.CodeUnknownOutput, diagnostic.SeverityError, posB1, "msg")
	if err != nil {
		t.Fatalf("New: unexpected error: %v", err)
	}

	if diagnostic.Compare(d1, d2) >= 0 {
		t.Errorf("Compare(d1, d2) = %d, want < 0 (line order)", diagnostic.Compare(d1, d2))
	}
	if diagnostic.Compare(d2, d3) >= 0 {
		t.Errorf("Compare(d2, d3) = %d, want < 0 (file order)", diagnostic.Compare(d2, d3))
	}
	if diagnostic.Compare(d1, d1) != 0 {
		t.Errorf("Compare(d1, d1) = %d, want 0", diagnostic.Compare(d1, d1))
	}
}
