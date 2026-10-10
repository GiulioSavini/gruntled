package impact_test

// An independent brute-force oracle for transitive Impacted (BLAST-03/04/05).
// It shares no code with transitive.go: it builds its own reverse edge set
// from the generator's raw edge list with the BLAST-03 rule written from the
// requirement text, enumerates every simple path from a unit back to a seed
// by DFS, keeps the shortest, and among those the lexicographically smallest
// unit sequence read from the unit back to its seed (sec #111). It also
// computes the competing seed-first reading, to prove the two differ on the
// graphs it checks (sec #196).

import (
	"reflect"
	"slices"
	"strconv"
	"testing"

	"github.com/GiulioSavini/gruntled/internal/domain/diagnostic"
	"github.com/GiulioSavini/gruntled/internal/domain/impact"
	"github.com/GiulioSavini/gruntled/internal/domain/repograph"
)

// Edge variants, as written in a unit's configuration.
const (
	oBlock        = iota // dependency block, defaults (enabled true, skip_outputs false)
	oEnabledFalse        // enabled = false
	oEnabledUnk          // enabled = <non-literal>
	oSkipTrue            // skip_outputs = true
	oSkipUnk             // skip_outputs = <non-literal>
	oPaths               // dependencies { paths = [...] }
	oVariants
)

// oEdge is one raw dependency: from depends on to (to < 0: a directory that
// is not a unit).
type oEdge struct {
	from, to int
	variant  int
}

// oTree is the generator's raw description of a current tree.
type oTree struct {
	n          int
	module     []int  // module index per unit
	modUnknown []bool // unit's own module could not be resolved
	edges      []oEdge
	broken     []bool
	changed    []bool // per module: surface names differ between base and cur
}

func oName(i int) string { return "u" + strconv.Itoa(i) }

// oracleRule is BLAST-03 from the requirement text: only a dependency block
// whose enabled is literally true or absent and whose skip_outputs is
// literally false or absent, targeting a unit, carries impact.
func oracleRule(e oEdge) bool { return e.variant == oBlock && e.to >= 0 }

// oPath is a unit sequence read from a unit back to its seed.
type oPath []int

func lessNames(a, b []int) bool {
	for i := range a {
		if c := repograph.MustRepoPath(oName(a[i])).Compare(repograph.MustRepoPath(oName(b[i]))); c != 0 {
			return c < 0
		}
	}
	return false
}

func reversed(p oPath) oPath {
	r := slices.Clone(p)
	slices.Reverse(r)
	return r
}

type oResult struct {
	readBack  map[int]oPath // canonical (smallest read from the unit back)
	seedFirst map[int]oPath // competing reading (smallest read from the seed)
	ties      map[int]bool  // unit has several shortest paths with different first hops
}

// oracle enumerates every simple path over its own adjacency.
func oracle(tr oTree) oResult {
	seed := make([]bool, tr.n)
	for u := 0; u < tr.n; u++ {
		seed[u] = !tr.modUnknown[u] && tr.changed[tr.module[u]]
	}
	// uses[u] = units u depends on through a propagating edge.
	uses := make([][]int, tr.n)
	for _, e := range tr.edges {
		if oracleRule(e) && !slices.Contains(uses[e.from], e.to) {
			uses[e.from] = append(uses[e.from], e.to)
		}
	}
	res := oResult{readBack: map[int]oPath{}, seedFirst: map[int]oPath{}, ties: map[int]bool{}}
	for start := 0; start < tr.n; start++ {
		var best, bestSF oPath
		firstHops := map[int]bool{}
		onPath := make([]bool, tr.n)
		var path oPath
		var dfs func(u int)
		dfs = func(u int) {
			path = append(path, u)
			onPath[u] = true
			if seed[u] {
				cand := slices.Clone(path)
				switch {
				case best == nil || len(cand) < len(best):
					best, bestSF = cand, cand
					firstHops = map[int]bool{}
					if len(cand) > 1 {
						firstHops[cand[1]] = true
					}
				case len(cand) == len(best):
					if lessNames(cand, best) {
						best = cand
					}
					if lessNames(reversed(cand), reversed(bestSF)) {
						bestSF = cand
					}
					if len(cand) > 1 {
						firstHops[cand[1]] = true
					}
				}
			} else {
				for _, w := range uses[u] {
					if !onPath[w] {
						dfs(w)
					}
				}
			}
			onPath[u] = false
			path = path[:len(path)-1]
		}
		dfs(start)
		if best != nil {
			res.readBack[start] = best
			res.seedFirst[start] = bestSF
			res.ties[start] = len(firstHops) > 1
		}
	}
	return res
}

func depVariant(t *testing.T, from string, i int, target string, variant int) repograph.Dependency {
	t.Helper()
	o := repograph.DefaultDependencyOptions()
	switch variant {
	case oEnabledFalse:
		o.Enabled = repograph.TristateFalse
	case oEnabledUnk:
		o.Enabled = repograph.TristateUnknown
	case oSkipTrue:
		o.SkipOutputs = repograph.TristateTrue
	case oSkipUnk:
		o.SkipOutputs = repograph.TristateUnknown
	}
	ps := pos(t, from+"/terragrunt.hcl", 1+i)
	d, err := repograph.NewDependency("d"+strconv.Itoa(i), p(target), ps, ps, repograph.TargetHasConfig, o)
	if err != nil {
		t.Fatalf("NewDependency: %v", err)
	}
	return d
}

// oBuild returns base and cur graphs and cur diagnostics for tr. unitOrder
// and edgeOrder permute the input order (nil: identity).
func oBuild(t *testing.T, tr oTree, nMods int, unitOrder, edgeOrder []int) (*repograph.RepositoryGraph, *repograph.RepositoryGraph, diagnostic.Set) {
	t.Helper()
	if unitOrder == nil {
		unitOrder = identity(tr.n)
	}
	if edgeOrder == nil {
		edgeOrder = identity(len(tr.edges))
	}
	var units []repograph.Unit
	for _, u := range unitOrder {
		name := oName(u)
		var deps []repograph.Dependency
		var pds []repograph.PathDependency
		for i, ei := range edgeOrder {
			e := tr.edges[ei]
			if e.from != u {
				continue
			}
			target := "x/none"
			if e.to >= 0 {
				target = oName(e.to)
			}
			if e.variant == oPaths {
				pd, err := repograph.NewPathDependency(p(target), "../"+target, pos(t, name+"/terragrunt.hcl", 100+i), repograph.TargetHasConfig)
				if err != nil {
					t.Fatalf("NewPathDependency: %v", err)
				}
				pds = append(pds, pd)
				continue
			}
			deps = append(deps, depVariant(t, name, i, target, e.variant))
		}
		var unit repograph.Unit
		var err error
		if tr.modUnknown[u] {
			unit, err = repograph.NewModuleUnknownUnit(p(name), "source-remote", deps, nil)
		} else {
			unit, err = repograph.NewResolvedUnit(p(name), p("modules/m"+strconv.Itoa(tr.module[u])), deps, nil)
		}
		if err != nil {
			t.Fatalf("unit: %v", err)
		}
		if len(pds) > 0 {
			if unit, err = unit.WithPathDependencies(pds); err != nil {
				t.Fatalf("WithPathDependencies: %v", err)
			}
		}
		units = append(units, unit)
	}
	var baseMods, curMods []repograph.Module
	for m := 0; m < nMods; m++ {
		mp := "modules/m" + strconv.Itoa(m)
		baseMods = append(baseMods, knownMod(t, mp, nil, []string{"a"}))
		if tr.changed[m] {
			curMods = append(curMods, knownMod(t, mp, nil, nil))
		} else {
			curMods = append(curMods, knownMod(t, mp, nil, []string{"a"}))
		}
	}
	var diags []diagnostic.Diagnostic
	for u := 0; u < tr.n; u++ {
		if tr.broken[u] {
			diags = append(diags, unitDiag(t, diagnostic.SeverityError, oName(u), 1, "broken"))
		}
	}
	return graph(t, units, baseMods...), graph(t, units, curMods...), diagnostic.NewSet(diags...)
}

func identity(n int) []int {
	out := make([]int, n)
	for i := range out {
		out[i] = i
	}
	return out
}

// oCheck compares r with the oracle for tr, limited to depth (0 = no limit).
func oCheck(t *testing.T, label string, tr oTree, o oResult, r impact.Result, depth int) {
	t.Helper()
	reachOf := map[repograph.RepoPath]impact.Reach{}
	changeOf := map[repograph.RepoPath]repograph.RepoPath{}
	for _, iu := range r.Impacted {
		reachOf[iu.Unit] = iu.Reach
		changeOf[iu.Unit] = iu.Change.Module
	}
	for _, b := range r.Broken {
		if b.Reach.Distance > 0 {
			reachOf[b.Subject] = b.Reach
		}
	}
	impacted := map[repograph.RepoPath]bool{}
	for _, iu := range r.Impacted {
		impacted[iu.Unit] = true
	}
	for u := 0; u < tr.n; u++ {
		name := p(oName(u))
		path, ok := o.readBack[u]
		if ok && depth > 0 && len(path) > depth {
			ok = false
		}
		got, gotOK := reachOf[name]
		if ok != gotOK {
			t.Fatalf("%s: %s reached = %v, oracle %v (path %v)", label, name, gotOK, ok, path)
		}
		if !ok {
			continue
		}
		if impacted[name] == tr.broken[u] {
			t.Fatalf("%s: %s Impacted = %v but Broken = %v", label, name, impacted[name], tr.broken[u])
		}
		want := impact.Reach{Distance: len(path), Source: p(oName(path[len(path)-1]))}
		if len(path) > 1 {
			want.Via = p(oName(path[1]))
		}
		if got != want {
			t.Fatalf("%s: %s Reach = %+v, oracle path %v", label, name, got, path)
		}
		// The Via chain read back through the result is the oracle path.
		cur := name
		for i := 1; i < len(path); i++ {
			cur = reachOf[cur].Via
			if cur != p(oName(path[i])) {
				t.Fatalf("%s: %s Via chain diverges at hop %d: %s, oracle %v", label, name, i, cur, path)
			}
		}
		if impacted[name] {
			if wantMod := p("modules/m" + strconv.Itoa(tr.module[path[len(path)-1]])); changeOf[name] != wantMod {
				t.Fatalf("%s: %s Change module %s, want %s (its source's module)", label, name, changeOf[name], wantMod)
			}
		}
	}
	if len(impacted) != len(r.Impacted) {
		t.Fatalf("%s: duplicate Impacted entries: %v", label, r.Impacted)
	}
}

func disagrees(o oResult) bool {
	for u, rb := range o.readBack {
		if !slices.Equal(rb, o.seedFirst[u]) {
			return true
		}
	}
	return false
}

// TestTransitiveMatchesOracleExhaustive enumerates small graphs completely.
// Every 5-unit graph with up to 4 edges (473,556 graphs with all seed sets)
// took ~7 s, over the ~3 s budget, so per the plan's fallback this checks:
// every graph over 4 units with up to 3 dependency-block edges among the 16
// ordered pairs (self-loops included) and every non-empty seed set, plus
// every 5-unit 4-edge DAG with every two-unit seed set, which contains the
// sec #111 counterexample.
func TestTransitiveMatchesOracleExhaustive(t *testing.T) {
	graphs, disagree := 0, 0
	check := func(n int, edges []oEdge, mask int) {
		tr := oTree{n: n, module: make([]int, n), modUnknown: make([]bool, n), broken: make([]bool, n), changed: []bool{false, true}, edges: edges}
		for u := 0; u < n; u++ {
			if mask&(1<<u) != 0 {
				tr.module[u] = 1
			}
		}
		o := oracle(tr)
		baseG, curG, curD := oBuild(t, tr, 2, nil, nil)
		oCheck(t, "exhaustive", tr, o, impact.Compute(baseG, diagnostic.NewSet(), curG, curD), 0)
		graphs++
		if disagrees(o) {
			disagree++
		}
	}
	pairs := func(n int, self bool) [][2]int {
		var out [][2]int
		for a := 0; a < n; a++ {
			for b := 0; b < n; b++ {
				if self || a != b {
					out = append(out, [2]int{a, b})
				}
			}
		}
		return out
	}
	// subsets calls f with every subset of ps of size <= max (exactly max
	// when exact).
	var subsets func(ps [][2]int, start, max int, exact bool, cur []oEdge, f func([]oEdge))
	subsets = func(ps [][2]int, start, max int, exact bool, cur []oEdge, f func([]oEdge)) {
		if !exact || len(cur) == max {
			f(cur)
		}
		if len(cur) == max {
			return
		}
		for i := start; i < len(ps); i++ {
			subsets(ps, i+1, max, exact, append(slices.Clone(cur), oEdge{from: ps[i][0], to: ps[i][1], variant: oBlock}), f)
		}
	}
	subsets(pairs(4, true), 0, 3, false, nil, func(edges []oEdge) {
		for mask := 1; mask < 1<<4; mask++ {
			check(4, edges, mask)
		}
	})
	dags := 0
	subsets(pairs(5, false), 0, 4, true, nil, func(edges []oEdge) {
		if !acyclic(5, edges) {
			return
		}
		dags++
		for a := 0; a < 5; a++ {
			for b := a + 1; b < 5; b++ {
				check(5, edges, 1<<a|1<<b)
			}
		}
	})
	t.Logf("%d graphs (%d 5-unit 4-edge DAGs); read-back-smallest != seed-first-smallest in %d", graphs, dags, disagree)
	if disagree == 0 {
		t.Fatal("vacuous: no graph tells the two path readings apart")
	}
}

func acyclic(n int, edges []oEdge) bool {
	indeg := make([]int, n)
	for _, e := range edges {
		indeg[e.to]++
	}
	var q []int
	for u := 0; u < n; u++ {
		if indeg[u] == 0 {
			q = append(q, u)
		}
	}
	seen := 0
	for len(q) > 0 {
		u := q[0]
		q = q[1:]
		seen++
		for _, e := range edges {
			if e.from == u {
				indeg[e.to]--
				if indeg[e.to] == 0 {
					q = append(q, e.to)
				}
			}
		}
	}
	return seen == n
}

type xorshift uint64

func (x *xorshift) intn(n int) int {
	*x ^= *x << 13
	*x ^= *x >> 7
	*x ^= *x << 17
	return int(uint64(*x) % uint64(n))
}

func (x *xorshift) perm(n int) []int {
	out := identity(n)
	for i := n - 1; i > 0; i-- {
		j := x.intn(i + 1)
		out[i], out[j] = out[j], out[i]
	}
	return out
}

// TestTransitiveMatchesOracleRandom: 3000 generated trees with every edge
// variant, non-unit targets, module-unknown units, Broken units, cycles and
// self-loops; each built from two shuffles; every depth 1..4.
func TestTransitiveMatchesOracleRandom(t *testing.T) {
	rng := xorshift(0x14a)
	var cycle, tie, blocked, brokenOnPath, far, disagree int
	for c := range 3000 {
		n := 3 + rng.intn(7)
		nMods := 1 + rng.intn(3)
		// One case in two is layered (edges only from layer L+1 to layer L,
		// seeds in layer 0): it produces several equal-length paths from
		// different seeds, where the two path readings can differ.
		layered := rng.intn(2) == 0
		if layered {
			nMods = 2
			n = 6 + rng.intn(7)
		}
		tr := oTree{n: n, module: make([]int, n), modUnknown: make([]bool, n), broken: make([]bool, n), changed: make([]bool, nMods)}
		for m := range tr.changed {
			tr.changed[m] = rng.intn(2) == 0
		}
		layer := make([]int, n)
		for u := 0; u < n; u++ {
			tr.module[u] = rng.intn(nMods)
			tr.modUnknown[u] = rng.intn(8) == 0
			tr.broken[u] = rng.intn(6) == 0
			if layered {
				layer[u] = rng.intn(3)
				tr.module[u] = min(layer[u], 1)
				tr.modUnknown[u] = false
				tr.broken[u] = rng.intn(10) == 0
			}
		}
		if layered {
			tr.changed = []bool{true, false}
			for range 3 * n {
				a, b := rng.intn(n), rng.intn(n)
				if layer[a] == layer[b]+1 {
					tr.edges = append(tr.edges, oEdge{from: a, to: b, variant: oBlock})
				}
			}
		}
		pathsSeen := map[[2]int]bool{}
		for range rng.intn(15) {
			if layered {
				break
			}
			e := oEdge{from: rng.intn(n), to: rng.intn(n + 1), variant: oBlock}
			if e.to == n {
				e.to = -1
			}
			if rng.intn(3) == 0 {
				e.variant = rng.intn(oVariants)
			}
			if e.variant == oPaths {
				if pathsSeen[[2]int{e.from, e.to}] {
					continue
				}
				pathsSeen[[2]int{e.from, e.to}] = true
			}
			tr.edges = append(tr.edges, e)
		}
		o := oracle(tr)
		label := "case " + strconv.Itoa(c)
		var first impact.Result
		for k := range 2 {
			baseG, curG, curD := oBuild(t, tr, nMods, rng.perm(n), rng.perm(len(tr.edges)))
			r := impact.Compute(baseG, diagnostic.NewSet(), curG, curD)
			oCheck(t, label, tr, o, r, 0)
			for d := 1; d <= 4; d++ {
				oCheck(t, label+" depth "+strconv.Itoa(d), tr, o, r.WithMaxDistance(d), d)
			}
			if k == 0 {
				first = r
			} else if !reflect.DeepEqual(first, r) {
				t.Fatalf("%s: result depends on input order", label)
			}
		}

		// Non-vacuity facts, from the oracle and the raw edges only.
		hasCycle, hasTie, hasBlocked, hasBrokenOnPath, hasFar := false, false, false, false, false
		for u, path := range o.readBack {
			hasTie = hasTie || o.ties[u]
			hasFar = hasFar || len(path) >= 4
			for _, hop := range path[1:] {
				hasBrokenOnPath = hasBrokenOnPath || tr.broken[hop]
			}
		}
		for _, e := range tr.edges {
			_, toReached := o.readBack[e.to]
			_, fromReached := o.readBack[e.from]
			if oracleRule(e) && toReached && e.from == e.to {
				hasCycle = true
			}
			if !oracleRule(e) && e.to >= 0 && toReached && !fromReached {
				hasBlocked = true
			}
			if oracleRule(e) && toReached {
				// a two-unit or longer cycle through reached units
				for _, f := range tr.edges {
					if oracleRule(f) && f.from == e.to && f.to == e.from {
						hasCycle = true
					}
				}
			}
		}
		for b, v := range map[*int]bool{&cycle: hasCycle, &tie: hasTie, &blocked: hasBlocked, &brokenOnPath: hasBrokenOnPath, &far: hasFar, &disagree: disagrees(o)} {
			if v {
				*b++
			}
		}
	}
	t.Logf("3000 cases: cycle reached %d, tie-break mattered %d, blocked edge changed reachability %d, Broken unit on a shortest path %d, distance >= 4 %d, read-back != seed-first %d",
		cycle, tie, blocked, brokenOnPath, far, disagree)
	for name, v := range map[string]int{"cycle": cycle, "tie": tie, "blocked": blocked, "brokenOnPath": brokenOnPath, "far": far, "disagree": disagree} {
		if v == 0 {
			t.Errorf("vacuous: counter %s is 0", name)
		}
	}
}
