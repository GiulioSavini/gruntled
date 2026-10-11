package impact

import (
	"slices"
	"strings"

	"github.com/GiulioSavini/gruntled/internal/domain/repograph"
)

// SurfaceChange is the difference in a module's declared variable and
// output names between a base and a current tree, plus the type-level
// facts that changed (TypeChanges). A changed default value or description
// is never a change. All five slices are sorted and never nil.
//
// TypeChanges are Impacted-only facts: they make the module a changed
// module, so its instantiating units and their dependents are Impacted,
// but they never become a diagnostic, never make a unit Broken and never
// change the exit code (BLAST-09, Pitfall 4: inputs from include merges,
// tfvars and the environment make a "missing required variable" unknowable
// statically).
type SurfaceChange struct {
	Module           repograph.RepoPath
	AddedVariables   []string
	RemovedVariables []string
	AddedOutputs     []string
	RemovedOutputs   []string
	TypeChanges      []TypeChange
}

// Empty reports whether the change carries no added or removed name and no
// type-level change.
func (c SurfaceChange) Empty() bool {
	return len(c.AddedVariables) == 0 && len(c.RemovedVariables) == 0 &&
		len(c.AddedOutputs) == 0 && len(c.RemovedOutputs) == 0 &&
		len(c.TypeChanges) == 0
}

// TypeChangeKind is the kind of a type-level fact change. Its order is the
// order TypeChanges are sorted in.
type TypeChangeKind int

// The type-level change kinds, in sort order.
const (
	// VariableRequired: a variable new in the current tree is required
	// (Old "absent"), or an optional variable became required (Old
	// "optional"); New is "required".
	VariableRequired TypeChangeKind = iota + 1
	// VariableType: a variable's known type differs; Old and New are
	// canonical type strings.
	VariableType
	// OutputType: an output's declared, known type differs on both sides.
	OutputType
	// OutputSensitive: an output's literal sensitive fact differs; Old and
	// New are "false" or "true".
	OutputSensitive
)

// String returns the kind's stable snake_case name.
func (k TypeChangeKind) String() string {
	switch k {
	case VariableRequired:
		return "variable_required"
	case VariableType:
		return "variable_type"
	case OutputType:
		return "output_type"
	case OutputSensitive:
		return "output_sensitive"
	}
	return "unknown"
}

// TypeChange is one type-level fact that differs between the trees.
// Old/New are canonical type strings, "absent"/"optional"/"required", or
// "false"/"true".
type TypeChange struct {
	Kind TypeChangeKind
	Name string
	Old  string
	New  string
}

// SurfaceDiff returns, sorted by Module, the non-empty surface changes of
// every module present in both graphs with a known Surface in both. A module
// added or deleted between the trees, or unknown in either, yields nothing:
// an added module has no prior consumers to break, a deleted one is
// reported by GRT002 on its consumers, and an unknown surface cannot be
// compared honestly. A nil graph is treated as empty. Never nil.
func SurfaceDiff(base, cur *repograph.RepositoryGraph) []SurfaceChange {
	out := []SurfaceChange{}
	if base == nil || cur == nil {
		return out
	}
	for _, cm := range cur.Modules() {
		bm, ok := base.Module(cm.Path())
		if !ok {
			continue
		}
		bs, ok := bm.Surface()
		if !ok {
			continue
		}
		cs, ok := cm.Surface()
		if !ok {
			continue
		}
		c := SurfaceChange{
			Module:           cm.Path(),
			AddedVariables:   minus(cs.Variables(), bs.Variables()),
			RemovedVariables: minus(bs.Variables(), cs.Variables()),
			AddedOutputs:     minus(cs.Outputs(), bs.Outputs()),
			RemovedOutputs:   minus(bs.Outputs(), cs.Outputs()),
			TypeChanges:      typeChanges(bs, cs),
		}
		if !c.Empty() {
			out = append(out, c)
		}
	}
	// cur.Modules() is already sorted by path; sort again so the contract
	// does not depend on that.
	slices.SortFunc(out, func(a, b SurfaceChange) int { return a.Module.Compare(b.Module) })
	return out
}

// typeChanges compares the per-name facts of bs and cs. An unknown fact
// (TristateUnknown, or "" for a type) on either side reports nothing, so a
// surface built from names alone (repograph.NewSurface) never yields a type
// change. A variable new in cs reports only that it is required; a removed
// name reports nothing here (it is a name change already); an output type
// is compared only when declared on both sides, so absent -> declared is
// silent and absent is never read as "any". Sorted by Kind, then Name.
// Never nil.
func typeChanges(bs, cs repograph.Surface) []TypeChange {
	out := []TypeChange{}
	for _, n := range cs.Variables() {
		cv, _ := cs.Variable(n)
		bv, inBase := bs.Variable(n)
		if !inBase {
			if cv.Required == repograph.TristateTrue {
				out = append(out, TypeChange{Kind: VariableRequired, Name: n, Old: "absent", New: "required"})
			}
			continue
		}
		if bv.Required == repograph.TristateFalse && cv.Required == repograph.TristateTrue {
			out = append(out, TypeChange{Kind: VariableRequired, Name: n, Old: "optional", New: "required"})
		}
		if bv.Type != "" && cv.Type != "" && bv.Type != cv.Type {
			out = append(out, TypeChange{Kind: VariableType, Name: n, Old: bv.Type, New: cv.Type})
		}
	}
	for _, n := range cs.Outputs() {
		co, _ := cs.Output(n)
		bo, inBase := bs.Output(n)
		if !inBase {
			continue
		}
		if bo.Type != "" && co.Type != "" && bo.Type != co.Type {
			out = append(out, TypeChange{Kind: OutputType, Name: n, Old: bo.Type, New: co.Type})
		}
		if old, ok := knownBool(bo.Sensitive); ok {
			if cur, ok := knownBool(co.Sensitive); ok && old != cur {
				out = append(out, TypeChange{Kind: OutputSensitive, Name: n, Old: old, New: cur})
			}
		}
	}
	slices.SortFunc(out, func(a, b TypeChange) int {
		if a.Kind != b.Kind {
			return int(a.Kind) - int(b.Kind)
		}
		return strings.Compare(a.Name, b.Name)
	})
	return out
}

// knownBool renders a known Tristate as "true"/"false"; ok is false when
// it is unknown.
func knownBool(t repograph.Tristate) (string, bool) {
	switch t {
	case repograph.TristateTrue:
		return "true", true
	case repograph.TristateFalse:
		return "false", true
	}
	return "", false
}

// minus returns the names in a that are not in b. Both inputs are sorted
// (Surface guarantees it), so the result is sorted. Never nil.
func minus(a, b []string) []string {
	out := []string{}
	for _, n := range a {
		if _, found := slices.BinarySearch(b, n); !found {
			out = append(out, n)
		}
	}
	return out
}
