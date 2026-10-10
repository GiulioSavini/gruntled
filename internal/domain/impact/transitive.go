package impact

import (
	"slices"

	"github.com/GiulioSavini/gruntled/internal/domain/repograph"
)

// Reach is how propagation reached a unit. Distance 1 is an instantiating
// unit (Source == the unit, Via zero); a dependent k reverse dependency
// edges from the nearest seed has Distance 1+k, Via its predecessor toward
// Source. The zero Reach (Distance 0) means "not reached".
type Reach struct {
	Distance int
	Source   repograph.RepoPath
	Via      repograph.RepoPath
}

// propagates reports whether e may carry impact from e.To() to e.From():
// only a dependency block (never a `dependencies { paths }` entry, which
// reports Enabled true and SkipOutputs false but reads no outputs), with
// enabled literally true or absent, skip_outputs literally false or absent,
// whose target is a unit of g. A non-literal (unknown) enabled or
// skip_outputs stops propagation.
func propagates(g *repograph.RepositoryGraph, e repograph.Edge) bool {
	if e.Kind() != repograph.EdgeBlock {
		return false
	}
	if e.Enabled() != repograph.TristateTrue || e.SkipOutputs() != repograph.TristateFalse {
		return false
	}
	_, ok := g.Unit(e.To())
	return ok
}

// dependents returns, for every unit, the sorted and deduplicated units that
// depend on it through a propagating edge.
func dependents(g *repograph.RepositoryGraph) map[repograph.RepoPath][]repograph.RepoPath {
	out := map[repograph.RepoPath][]repograph.RepoPath{}
	for _, e := range g.Edges() {
		if propagates(g, e) {
			out[e.To()] = append(out[e.To()], e.From())
		}
	}
	for k, v := range out {
		slices.SortFunc(v, repograph.RepoPath.Compare)
		out[k] = slices.Compact(v)
	}
	return out
}

// propagate runs the multi-source level-synchronous BFS over g's reverse
// propagating edges from seeds (any order; deduplicated) and returns the
// Reach of every visited unit, seeds included.
//
// Each level is expanded in RepoPath order and the first discovery wins, so
// a unit's Via is its smallest predecessor at the previous level. By
// induction, the path read from a unit back to its Source (unit, Via,
// Via's Via, ...) is the smallest shortest path under RepoPath.Compare at
// the first differing hop; the result does not depend on unit or edge
// input order. The loop is iterative with a visited set, so cycles,
// self-loops and diamonds terminate and every unit appears once, at its
// minimum distance. TestTransitiveMatchesOracle* pins this against a
// brute-force enumeration of simple paths.
func propagate(g *repograph.RepositoryGraph, seeds []repograph.RepoPath) map[repograph.RepoPath]Reach {
	reach := map[repograph.RepoPath]Reach{}
	frontier := slices.Clone(seeds)
	slices.SortFunc(frontier, repograph.RepoPath.Compare)
	frontier = slices.Compact(frontier)
	for _, s := range frontier {
		reach[s] = Reach{Distance: 1, Source: s}
	}
	deps := dependents(g)
	for len(frontier) > 0 {
		var next []repograph.RepoPath
		for _, u := range frontier {
			ru := reach[u]
			for _, d := range deps[u] {
				if _, seen := reach[d]; seen {
					continue
				}
				reach[d] = Reach{Distance: ru.Distance + 1, Source: ru.Source, Via: u}
				next = append(next, d)
			}
		}
		slices.SortFunc(next, repograph.RepoPath.Compare)
		frontier = next
	}
	return reach
}
