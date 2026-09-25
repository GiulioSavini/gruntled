package repograph_test

import (
	"reflect"
	"testing"

	"github.com/GiulioSavini/gruntled/internal/domain/repograph"
)

func TestTristateString(t *testing.T) {
	cases := []struct {
		t    repograph.Tristate
		want string
	}{
		{repograph.TristateUnknown, "unknown"},
		{repograph.TristateFalse, "false"},
		{repograph.TristateTrue, "true"},
		{repograph.Tristate(99), "Tristate(99)"},
	}
	for _, c := range cases {
		if got := c.t.String(); got != c.want {
			t.Errorf("Tristate(%d).String() = %q, want %q", c.t, got, c.want)
		}
	}
}

func TestTristateOf(t *testing.T) {
	if repograph.TristateOf(true) != repograph.TristateTrue {
		t.Errorf("TristateOf(true) = %v, want TristateTrue", repograph.TristateOf(true))
	}
	if repograph.TristateOf(false) != repograph.TristateFalse {
		t.Errorf("TristateOf(false) = %v, want TristateFalse", repograph.TristateOf(false))
	}
}

func TestZeroDependencyOptionsIsFailSafeUnknown(t *testing.T) {
	var opts repograph.DependencyOptions
	if opts.Enabled != repograph.TristateUnknown {
		t.Errorf("zero DependencyOptions.Enabled = %v, want TristateUnknown", opts.Enabled)
	}
	if opts.SkipOutputs != repograph.TristateUnknown {
		t.Errorf("zero DependencyOptions.SkipOutputs = %v, want TristateUnknown", opts.SkipOutputs)
	}
	if opts.MockMergeWithState != repograph.TristateUnknown {
		t.Errorf("zero DependencyOptions.MockMergeWithState = %v, want TristateUnknown", opts.MockMergeWithState)
	}
	if !opts.MockOutputs.IsUnknown() {
		t.Errorf("zero DependencyOptions.MockOutputs.IsUnknown() = false, want true")
	}
	if !opts.MockAllowedCommands.IsUnknown() {
		t.Errorf("zero DependencyOptions.MockAllowedCommands.IsUnknown() = false, want true")
	}
}

func TestDefaultDependencyOptions(t *testing.T) {
	opts := repograph.DefaultDependencyOptions()
	if opts.Enabled != repograph.TristateTrue {
		t.Errorf("DefaultDependencyOptions().Enabled = %v, want TristateTrue", opts.Enabled)
	}
	if opts.SkipOutputs != repograph.TristateFalse {
		t.Errorf("DefaultDependencyOptions().SkipOutputs = %v, want TristateFalse", opts.SkipOutputs)
	}
	if opts.MockMergeWithState != repograph.TristateFalse {
		t.Errorf("DefaultDependencyOptions().MockMergeWithState = %v, want TristateFalse", opts.MockMergeWithState)
	}
	if !opts.MockOutputs.IsAbsent() {
		t.Errorf("DefaultDependencyOptions().MockOutputs.IsAbsent() = false, want true")
	}
	if !opts.MockAllowedCommands.IsAbsent() {
		t.Errorf("DefaultDependencyOptions().MockAllowedCommands.IsAbsent() = false, want true")
	}
}

func TestKnownNamesSortsDedupsAndRejectsEmpty(t *testing.T) {
	l, err := repograph.KnownNames([]string{"b", "a", "b"})
	if err != nil {
		t.Fatalf("KnownNames: unexpected error: %v", err)
	}
	names, ok := l.Names()
	if !ok || !reflect.DeepEqual(names, []string{"a", "b"}) {
		t.Errorf("Names() = (%v, %v), want ([a b], true)", names, ok)
	}

	if _, err := repograph.KnownNames([]string{""}); err == nil {
		t.Errorf("KnownNames([\"\"]): expected error, got nil")
	}

	empty, err := repograph.KnownNames(nil)
	if err != nil {
		t.Fatalf("KnownNames(nil): unexpected error: %v", err)
	}
	if empty.IsUnknown() || empty.IsAbsent() {
		t.Errorf("KnownNames(nil): IsUnknown()=%v IsAbsent()=%v, want both false (Known, empty)", empty.IsUnknown(), empty.IsAbsent())
	}
	emptyNames, ok := empty.Names()
	if !ok || len(emptyNames) != 0 {
		t.Errorf("KnownNames(nil).Names() = (%v, %v), want ([], true)", emptyNames, ok)
	}
}

func TestNameListContains(t *testing.T) {
	known, err := repograph.KnownNames([]string{"a"})
	if err != nil {
		t.Fatalf("KnownNames: unexpected error: %v", err)
	}
	if known.Contains("a") != repograph.TristateTrue {
		t.Errorf("Known{a}.Contains(a) = %v, want TristateTrue", known.Contains("a"))
	}
	if known.Contains("z") != repograph.TristateFalse {
		t.Errorf("Known{a}.Contains(z) = %v, want TristateFalse", known.Contains("z"))
	}

	absent := repograph.AbsentNames()
	if absent.Contains("a") != repograph.TristateFalse {
		t.Errorf("Absent.Contains(a) = %v, want TristateFalse", absent.Contains("a"))
	}

	unknown := repograph.UnknownNames()
	if unknown.Contains("a") != repograph.TristateUnknown {
		t.Errorf("Unknown.Contains(a) = %v, want TristateUnknown", unknown.Contains("a"))
	}
}

func TestNameListNamesIsDefensiveCopy(t *testing.T) {
	known, err := repograph.KnownNames([]string{"a", "b"})
	if err != nil {
		t.Fatalf("KnownNames: unexpected error: %v", err)
	}
	names, ok := known.Names()
	if !ok {
		t.Fatalf("Names() ok = false, want true")
	}
	names[0] = "mutated"
	again, _ := known.Names()
	if again[0] != "a" {
		t.Errorf("mutating Names() result affected the NameList: got %q, want %q", again[0], "a")
	}
}

func TestNameListZeroValueIsUnknown(t *testing.T) {
	var l repograph.NameList
	if !l.IsUnknown() {
		t.Errorf("zero-value NameList.IsUnknown() = false, want true")
	}
	if l.IsAbsent() {
		t.Errorf("zero-value NameList.IsAbsent() = true, want false")
	}
	if _, ok := l.Names(); ok {
		t.Errorf("zero-value NameList.Names() ok = true, want false")
	}
}
