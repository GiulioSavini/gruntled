package analysis

import (
	"slices"
	"strconv"
	"strings"

	"github.com/GiulioSavini/gruntled/internal/domain/diagnostic"
	"github.com/GiulioSavini/gruntled/internal/domain/repograph"
)

// DependencyCycles implements GRT003: one diagnostic per strongly connected
// component (SCC) of the unit dependency graph that is a cycle, meaning it
// has more than one member or is a single unit with an edge to itself.
//
// Nodes are all units of g. Edges are g.Edges() (block and paths alike)
// whose enabled is literally true (absent counts as true; skip_outputs does
// not matter, Terragrunt still orders the units) and whose target is a unit
// of g. Out-edges are deduplicated by target, so a `dependency` block plus a
// `dependencies` path to the same unit are one edge.
//
// The diagnostic is attributed to the lexically smallest member. Its
// position is that member's first edge (by Position.Compare) to another
// member, or its self-edge for a one-member SCC. An SCC where every member
// has exactly one distinct in-SCC successor is a ring and renders as
// `dependency cycle: "a" -> "b" -> "a"`, walking successors from the
// smallest member. Any other SCC, including one where a member also loops on
// itself, renders as `dependency cycle among: "a", "b", "c"` (sorted, no
// size cap). The message is part of diagnostic.Key, so the Key changes
// whenever the membership changes.
//
// Limits (false negatives only): config-unknown units carry no edges, so a
// cycle through one is invisible; an edge to a directory that is not a unit
// of g (a symlink alias, vendored or cached copy) is dropped, as targets are
// not canonicalised.
//
// Tarjan's algorithm runs iteratively with an explicit frame stack, so a
// deep chain cannot overflow the goroutine stack, and every step follows
// RepoPath order, so the output does not depend on input order.
func DependencyCycles(g *repograph.RepositoryGraph) ([]diagnostic.Diagnostic, error) {
	units := g.Units()
	n := len(units)
	idx := make(map[repograph.RepoPath]int, n)
	for i, u := range units {
		idx[u.Path()] = i
	}

	// adj[v]: distinct in-graph targets in RepoPath order. edges[v]: every
	// kept edge (with duplicates) for anchor-position selection.
	type outEdge struct {
		to  int
		pos repograph.Position
	}
	adj := make([][]int, n)
	edges := make([][]outEdge, n)
	for _, e := range g.Edges() {
		if e.Enabled() != repograph.TristateTrue {
			continue
		}
		from, ok := idx[e.From()]
		if !ok {
			continue
		}
		to, ok := idx[e.To()]
		if !ok {
			continue
		}
		adj[from] = append(adj[from], to)
		edges[from] = append(edges[from], outEdge{to: to, pos: e.Pos()})
	}
	for v := range adj {
		slices.Sort(adj[v])
		adj[v] = slices.Compact(adj[v])
	}

	var out []diagnostic.Diagnostic
	for _, members := range tarjan(adj) {
		if len(members) == 1 && !slices.Contains(adj[members[0]], members[0]) {
			continue
		}
		in := make(map[int]bool, len(members))
		for _, m := range members {
			in[m] = true
		}
		anchor := members[0]

		var pos repograph.Position
		found := false
		for _, e := range edges[anchor] {
			if !in[e.to] || (len(members) > 1 && e.to == anchor) {
				continue
			}
			if !found || e.pos.Compare(pos) < 0 {
				pos, found = e.pos, true
			}
		}

		name := func(v int) string { return strconv.Quote(units[v].Path().String()) }
		var b strings.Builder
		if next, ok := ringSuccessors(adj, members, in); ok {
			b.WriteString("dependency cycle: ")
			b.WriteString(name(anchor))
			for v := next[anchor]; ; v = next[v] {
				b.WriteString(" -> ")
				b.WriteString(name(v))
				if v == anchor {
					break
				}
			}
		} else {
			b.WriteString("dependency cycle among: ")
			for i, m := range members {
				if i > 0 {
					b.WriteString(", ")
				}
				b.WriteString(name(m))
			}
		}
		d, err := diagnostic.NewForUnit(diagnostic.CodeDependencyCycle, diagnostic.SeverityError, units[anchor].Path(), pos, b.String())
		if err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, nil
}

// ringSuccessors returns each member's single in-SCC successor, and true,
// when every member has exactly one distinct in-SCC successor.
func ringSuccessors(adj [][]int, members []int, in map[int]bool) (map[int]int, bool) {
	next := make(map[int]int, len(members))
	for _, m := range members {
		count := 0
		for _, w := range adj[m] {
			if in[w] {
				count++
				next[m] = w
			}
		}
		if count != 1 {
			return nil, false
		}
	}
	return next, true
}

// tarjan returns the strongly connected components of the graph given by
// adj, iteratively. Roots are visited in index order and adjacency in slice
// order; each component is sorted and the result is ordered by smallest
// member.
func tarjan(adj [][]int) [][]int {
	n := len(adj)
	index := make([]int, n) // 0 = unvisited, else visit order + 1
	low := make([]int, n)
	onStack := make([]bool, n)
	var stack []int
	var sccs [][]int
	type frame struct{ v, next int }
	counter := 0
	visit := func(v int) {
		counter++
		index[v], low[v] = counter, counter
		stack = append(stack, v)
		onStack[v] = true
	}
	for root := 0; root < n; root++ {
		if index[root] != 0 {
			continue
		}
		visit(root)
		call := []frame{{root, 0}}
		for len(call) > 0 {
			f := &call[len(call)-1]
			if f.next < len(adj[f.v]) {
				w := adj[f.v][f.next]
				f.next++
				if index[w] == 0 {
					visit(w)
					call = append(call, frame{w, 0})
				} else if onStack[w] {
					low[f.v] = min(low[f.v], index[w])
				}
				continue
			}
			v := f.v
			if low[v] == index[v] {
				var scc []int
				for {
					w := stack[len(stack)-1]
					stack = stack[:len(stack)-1]
					onStack[w] = false
					scc = append(scc, w)
					if w == v {
						break
					}
				}
				slices.Sort(scc)
				sccs = append(sccs, scc)
			}
			call = call[:len(call)-1]
			if len(call) > 0 {
				p := call[len(call)-1].v
				low[p] = min(low[p], low[v])
			}
		}
	}
	slices.SortFunc(sccs, func(a, b []int) int { return a[0] - b[0] })
	return sccs
}
