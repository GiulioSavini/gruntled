package repograph

import (
	"errors"
	"slices"
	"strconv"
)

// Tristate is a fact that is either a known boolean or "not a literal", i.e.
// not statically determinable without evaluating Terragrunt functions this
// domain does not model. The zero value is TristateUnknown, so a zero-value
// DependencyOptions is fail-safe: every fact defaults to "unknown" rather
// than silently defaulting to true or false.
type Tristate int

const (
	// TristateUnknown means the attribute is present but its value is not a
	// literal (for example it is a function call, a reference, or comes
	// from a deep merge this domain does not model). It is the zero value
	// on purpose.
	TristateUnknown Tristate = iota
	// TristateFalse means the attribute is a literal false.
	TristateFalse
	// TristateTrue means the attribute is a literal true.
	TristateTrue
)

// String renders the tristate as "unknown", "false" or "true".
func (t Tristate) String() string {
	switch t {
	case TristateUnknown:
		return "unknown"
	case TristateFalse:
		return "false"
	case TristateTrue:
		return "true"
	default:
		return "Tristate(" + strconv.Itoa(int(t)) + ")"
	}
}

// IsValid reports whether t is one of the three defined Tristate constants.
// A value outside them can only arise from an explicit conversion such as
// Tristate(99); the domain refuses to store or report a fact that was never
// actually observed.
func (t Tristate) IsValid() bool {
	switch t {
	case TristateUnknown, TristateFalse, TristateTrue:
		return true
	default:
		return false
	}
}

// TristateOf converts a plain bool into its Tristate equivalent.
func TristateOf(b bool) Tristate {
	if b {
		return TristateTrue
	}
	return TristateFalse
}

// nameListState is whether a NameList's names are unknown, absent, or a
// known set of names.
type nameListState int

const (
	// nameListUnknown means the underlying attribute is present but not a
	// literal list of names. It is the zero value so a zero-value NameList
	// is fail-safe.
	nameListUnknown nameListState = iota
	nameListAbsent
	nameListKnown
)

// NameList is a tri-state list of names: Unknown (the zero value, attribute
// present but not literal), Absent (attribute not present), or Known (a
// literal, sorted, deduplicated list of names, possibly empty).
type NameList struct {
	state nameListState
	names []string
}

// UnknownNames returns a NameList in the Unknown state. It is equivalent to
// the zero value; the function exists for readability at call sites.
func UnknownNames() NameList {
	return NameList{state: nameListUnknown}
}

// AbsentNames returns a NameList in the Absent state: the attribute this
// list was read from was not present at all.
func AbsentNames() NameList {
	return NameList{state: nameListAbsent}
}

// KnownNames validates names and returns a NameList in the Known state. No
// name may be empty. The returned list is sorted and deduplicated; a nil or
// empty input is a Known, empty list, distinct from AbsentNames().
func KnownNames(names []string) (NameList, error) {
	sorted := slices.Clone(names)
	slices.Sort(sorted)
	for _, n := range sorted {
		if n == "" {
			return NameList{}, errors.New("repograph: invalid name list: name must not be empty")
		}
	}
	deduped := sorted[:0:0]
	for i, n := range sorted {
		if i > 0 && sorted[i-1] == n {
			continue
		}
		deduped = append(deduped, n)
	}
	return NameList{state: nameListKnown, names: deduped}, nil
}

// IsUnknown reports whether l is in the Unknown state.
func (l NameList) IsUnknown() bool {
	return l.state == nameListUnknown
}

// IsAbsent reports whether l is in the Absent state.
func (l NameList) IsAbsent() bool {
	return l.state == nameListAbsent
}

// Names returns a sorted, deduplicated, defensive copy of the list's names
// and true, only when l is Known. It returns (nil, false) for Unknown and
// Absent.
func (l NameList) Names() ([]string, bool) {
	if l.state != nameListKnown {
		return nil, false
	}
	return slices.Clone(l.names), true
}

// Contains reports whether name is a member of a Known list (True/False),
// is TristateFalse for an Absent list (an absent attribute has no members),
// or is TristateUnknown when the list itself is Unknown.
func (l NameList) Contains(name string) Tristate {
	switch l.state {
	case nameListKnown:
		_, ok := slices.BinarySearch(l.names, name)
		return TristateOf(ok)
	case nameListAbsent:
		return TristateFalse
	default:
		return TristateUnknown
	}
}

// DependencyOptions captures the tri-state facts a `dependency` block can
// carry (DIAG-03), as they were written, without deciding how Phase 3's
// GRT001/mock_outputs rule should react to them: Phase 2 only records facts
// faithfully.
//
// The zero value DependencyOptions{} has every field in its "unknown" state
// (TristateUnknown / NameList's zero value, itself Unknown), which is
// fail-safe: a caller that forgets to set a field gets "not literal",
// never a false literal default. DefaultDependencyOptions returns the
// values Terragrunt itself uses when the corresponding attribute is
// entirely absent from the block.
type DependencyOptions struct {
	// Enabled is the dependency block's `enabled` attribute. Terragrunt's
	// default when absent is true (the dependency is active).
	Enabled Tristate
	// SkipOutputs is `skip_outputs`. Terragrunt's default when absent is
	// false (outputs are read normally).
	SkipOutputs Tristate
	// MockOutputs is the key set of `mock_outputs`. AbsentNames() when the
	// attribute itself is absent from the block.
	MockOutputs NameList
	// MockMergeWithState is whether mocks are merged with state.
	// `mock_outputs_merge_strategy_with_state`, when present, decides: a
	// literal "no_merge" is false, "shallow" and "deep_map_only" are true,
	// and anything else is Unknown. Only otherwise the deprecated
	// `mock_outputs_merge_with_state` bool decides. Terragrunt's default
	// when both are absent is false.
	MockMergeWithState Tristate
	// MockAllowedCommands is the list of
	// `mock_outputs_allowed_terraform_commands`. AbsentNames() when the
	// attribute is absent from the block.
	MockAllowedCommands NameList
}

// DefaultDependencyOptions returns the DependencyOptions Terragrunt applies
// when every one of the underlying attributes is entirely absent from the
// dependency block: Enabled defaults to true, SkipOutputs and
// MockMergeWithState default to false, and both name lists default to
// Absent.
func DefaultDependencyOptions() DependencyOptions {
	return DependencyOptions{
		Enabled:             TristateTrue,
		SkipOutputs:         TristateFalse,
		MockOutputs:         AbsentNames(),
		MockMergeWithState:  TristateFalse,
		MockAllowedCommands: AbsentNames(),
	}
}
