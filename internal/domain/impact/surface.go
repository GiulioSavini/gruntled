package impact

import (
	"slices"

	"github.com/GiulioSavini/gruntled/internal/domain/repograph"
)

// SurfaceChange is the difference in a module's declared variable and
// output NAMES between a base and a current tree. Only names count: a
// changed default, type or description is not a surface change. All four
// slices are sorted and never nil.
type SurfaceChange struct {
	Module           repograph.RepoPath
	AddedVariables   []string
	RemovedVariables []string
	AddedOutputs     []string
	RemovedOutputs   []string
}

// Empty reports whether the change carries no added or removed name.
func (c SurfaceChange) Empty() bool {
	return len(c.AddedVariables) == 0 && len(c.RemovedVariables) == 0 &&
		len(c.AddedOutputs) == 0 && len(c.RemovedOutputs) == 0
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
