package diagnostic_test

import (
	"reflect"
	"testing"

	"github.com/GiulioSavini/gruntled/internal/domain/diagnostic"
	"github.com/GiulioSavini/gruntled/internal/domain/repograph"
)

func mustDiagnostic(t *testing.T, code diagnostic.Code, severity diagnostic.Severity, file string, line, column int, message string) diagnostic.Diagnostic {
	t.Helper()
	pos, err := repograph.NewPosition(repograph.MustRepoPath(file), line, column)
	if err != nil {
		t.Fatalf("NewPosition: unexpected error: %v", err)
	}
	d, err := diagnostic.New(code, severity, pos, message)
	if err != nil {
		t.Fatalf("New: unexpected error: %v", err)
	}
	return d
}

func mustDiagnosticForUnit(t *testing.T, code diagnostic.Code, severity diagnostic.Severity, unit, file string, line, column int, message string) diagnostic.Diagnostic {
	t.Helper()
	pos, err := repograph.NewPosition(repograph.MustRepoPath(file), line, column)
	if err != nil {
		t.Fatalf("NewPosition: unexpected error: %v", err)
	}
	d, err := diagnostic.NewForUnit(code, severity, repograph.MustRepoPath(unit), pos, message)
	if err != nil {
		t.Fatalf("NewForUnit: unexpected error: %v", err)
	}
	return d
}

func TestNewSetOrderIndependentAndCanonicalOrder(t *testing.T) {
	d1 := mustDiagnostic(t, diagnostic.CodeUnknownOutput, diagnostic.SeverityError, "a.hcl", 1, 1, "m1")
	d2 := mustDiagnostic(t, diagnostic.CodeUnknownOutput, diagnostic.SeverityError, "a.hcl", 2, 1, "m2")
	d3 := mustDiagnostic(t, diagnostic.CodeSyntaxError, diagnostic.SeverityError, "b.hcl", 1, 1, "m3")

	s1 := diagnostic.NewSet(d3, d1, d2)
	s2 := diagnostic.NewSet(d2, d3, d1)

	if !s1.Equal(s2) {
		t.Errorf("NewSet with different input order: sets not Equal")
	}
	if !reflect.DeepEqual(s1.All(), s2.All()) {
		t.Errorf("All() differs by input order:\n s1=%+v\n s2=%+v", s1.All(), s2.All())
	}

	all := s1.All()
	if len(all) != 3 || all[0] != d1 || all[1] != d2 || all[2] != d3 {
		t.Errorf("All() = %+v, want [d1 d2 d3] (file, line, column, code order)", all)
	}
}

func TestNewSetDropsDuplicatesByKey(t *testing.T) {
	d1 := mustDiagnostic(t, diagnostic.CodeUnknownOutput, diagnostic.SeverityError, "a.hcl", 1, 1, "m1")

	s := diagnostic.NewSet(d1, d1)
	if s.Len() != 1 {
		t.Errorf("NewSet(d1, d1).Len() = %d, want 1", s.Len())
	}
}

func TestNewSetDropsNonAdjacentDuplicateKeys(t *testing.T) {
	// Same position and code. Compare sorts these as
	// {error,m2}, {warning,m1}, {warning,m2}, so the two "m2" diagnostics,
	// which share a Key, are not neighbours after sorting.
	errM2 := mustDiagnostic(t, diagnostic.CodeUnknownOutput, diagnostic.SeverityError, "a.hcl", 1, 1, "m2")
	warnM1 := mustDiagnostic(t, diagnostic.CodeUnknownOutput, diagnostic.SeverityWarning, "a.hcl", 1, 1, "m1")
	warnM2 := mustDiagnostic(t, diagnostic.CodeUnknownOutput, diagnostic.SeverityWarning, "a.hcl", 1, 1, "m2")

	for _, in := range [][]diagnostic.Diagnostic{
		{errM2, warnM1, warnM2},
		{warnM2, warnM1, errM2},
		{warnM1, warnM2, errM2},
	} {
		s := diagnostic.NewSet(in...)
		if s.Len() != 2 {
			t.Fatalf("NewSet(%v).Len() = %d, want 2", in, s.Len())
		}
		keys := make(map[diagnostic.Key]int)
		for _, d := range s.All() {
			keys[d.Key()]++
		}
		for k, n := range keys {
			if n != 1 {
				t.Errorf("Key %+v appears %d times, want 1", k, n)
			}
		}
		// The error wins the collision on the shared "m2" Key.
		all := s.All()
		if all[0] != errM2 || all[1] != warnM1 {
			t.Errorf("All() = %v, want [errM2 warnM1]", all)
		}
	}
}

func TestNewSetKeepsBothUnitsWhenOnlyUnitDiffers(t *testing.T) {
	dA := mustDiagnosticForUnit(t, diagnostic.CodeUnknownOutput, diagnostic.SeverityError, "live/a", "a.hcl", 1, 1, "m")
	dB := mustDiagnosticForUnit(t, diagnostic.CodeUnknownOutput, diagnostic.SeverityError, "live/b", "a.hcl", 1, 1, "m")

	for _, in := range [][]diagnostic.Diagnostic{
		{dA, dB},
		{dB, dA},
	} {
		s := diagnostic.NewSet(in...)
		if s.Len() != 2 {
			t.Fatalf("NewSet(%v).Len() = %d, want 2", in, s.Len())
		}
		all := s.All()
		if all[0] != dA || all[1] != dB {
			t.Errorf("All() = %v, want [dA dB] regardless of input order", all)
		}
	}
}

func TestDiffOnlyUnitChangeIsOneAddedOneRemoved(t *testing.T) {
	dA := mustDiagnosticForUnit(t, diagnostic.CodeUnknownOutput, diagnostic.SeverityError, "live/a", "a.hcl", 1, 1, "m")
	dB := mustDiagnosticForUnit(t, diagnostic.CodeUnknownOutput, diagnostic.SeverityError, "live/b", "a.hcl", 1, 1, "m")

	prev := diagnostic.NewSet(dA)
	next := diagnostic.NewSet(dB)

	added, removed := diagnostic.Diff(prev, next)
	if added.Len() != 1 || added.All()[0] != dB {
		t.Errorf("Diff added = %+v, want [dB]", added.All())
	}
	if removed.Len() != 1 || removed.All()[0] != dA {
		t.Errorf("Diff removed = %+v, want [dA]", removed.All())
	}
}

func TestDiffIgnoresSeverityOnlyChange(t *testing.T) {
	errD := mustDiagnostic(t, diagnostic.CodeUnknownOutput, diagnostic.SeverityError, "a.hcl", 1, 1, "m")
	warnD := mustDiagnostic(t, diagnostic.CodeUnknownOutput, diagnostic.SeverityWarning, "a.hcl", 1, 1, "m")

	prev := diagnostic.NewSet(errD)
	next := diagnostic.NewSet(warnD)

	added, removed := diagnostic.Diff(prev, next)
	if added.Len() != 0 {
		t.Errorf("Diff added = %+v, want empty (severity-only change is not a new finding)", added.All())
	}
	if removed.Len() != 0 {
		t.Errorf("Diff removed = %+v, want empty (severity-only change is not a new finding)", removed.All())
	}
}

func TestDiff(t *testing.T) {
	a := mustDiagnostic(t, diagnostic.CodeUnknownOutput, diagnostic.SeverityError, "a.hcl", 1, 1, "a")
	b := mustDiagnostic(t, diagnostic.CodeUnknownOutput, diagnostic.SeverityError, "a.hcl", 2, 1, "b")
	c := mustDiagnostic(t, diagnostic.CodeUnknownOutput, diagnostic.SeverityError, "a.hcl", 3, 1, "c")

	prev := diagnostic.NewSet(a, b)
	next := diagnostic.NewSet(b, c)

	added, removed := diagnostic.Diff(prev, next)
	if added.Len() != 1 || added.All()[0] != c {
		t.Errorf("Diff added = %+v, want [c]", added.All())
	}
	if removed.Len() != 1 || removed.All()[0] != a {
		t.Errorf("Diff removed = %+v, want [a]", removed.All())
	}

	sameAdded, sameRemoved := diagnostic.Diff(prev, prev)
	if sameAdded.Len() != 0 || sameRemoved.Len() != 0 {
		t.Errorf("Diff(s, s) = (%+v, %+v), want two empty sets", sameAdded.All(), sameRemoved.All())
	}
}

func TestHasErrors(t *testing.T) {
	warn := mustDiagnostic(t, diagnostic.CodeUnknownOutput, diagnostic.SeverityWarning, "a.hcl", 1, 1, "w")
	errD := mustDiagnostic(t, diagnostic.CodeUnknownOutput, diagnostic.SeverityError, "b.hcl", 1, 1, "e")

	onlyWarn := diagnostic.NewSet(warn)
	if onlyWarn.HasErrors() {
		t.Errorf("HasErrors() = true for a set with only warnings, want false")
	}

	withError := diagnostic.NewSet(warn, errD)
	if !withError.HasErrors() {
		t.Errorf("HasErrors() = false for a set containing an error, want true")
	}
}

func TestAllReturnsCopy(t *testing.T) {
	d1 := mustDiagnostic(t, diagnostic.CodeUnknownOutput, diagnostic.SeverityError, "a.hcl", 1, 1, "m1")
	d2 := mustDiagnostic(t, diagnostic.CodeUnknownOutput, diagnostic.SeverityError, "a.hcl", 2, 1, "m2")

	s := diagnostic.NewSet(d1, d2)
	all := s.All()
	all[0] = d2
	if s.All()[0] != d1 {
		t.Errorf("mutating All() result affected the Set")
	}
}

func TestSetEqual(t *testing.T) {
	d1 := mustDiagnostic(t, diagnostic.CodeUnknownOutput, diagnostic.SeverityError, "a.hcl", 1, 1, "m1")
	d2 := mustDiagnostic(t, diagnostic.CodeUnknownOutput, diagnostic.SeverityError, "a.hcl", 2, 1, "m2")

	s1 := diagnostic.NewSet(d1)
	s2 := diagnostic.NewSet(d1, d2)
	if s1.Equal(s2) {
		t.Errorf("Equal() = true for sets of different size, want false")
	}
}
