package impact

import (
	"reflect"
	"slices"
	"strconv"
	"testing"

	"github.com/GiulioSavini/gruntled/internal/domain/repograph"
)

// iEdge is one dependency of a test unit. paths selects a
// `dependencies { paths }` entry instead of a dependency block; unresolved
// builds an unresolved dependency block (no target).
type iEdge struct {
	to         string
	paths      bool
	unresolved bool
	enabled    repograph.Tristate
	skip       repograph.Tristate
}

func blk(to string) iEdge {
	return iEdge{to: to, enabled: repograph.TristateTrue, skip: repograph.TristateFalse}
}

type iUnit struct {
	path string
	deps []iEdge
}

func rp(s string) repograph.RepoPath { return repograph.MustRepoPath(s) }

// iGraph builds the units in the given order, each its own resolved unit of
// module modules/m, with dependency blocks named d0, d1, ... in edge order.
func iGraph(t testing.TB, units []iUnit) *repograph.RepositoryGraph {
	t.Helper()
	var us []repograph.Unit
	for _, iu := range units {
		var deps []repograph.Dependency
		var pds []repograph.PathDependency
		for i, e := range iu.deps {
			ps, err := repograph.NewPosition(rp(iu.path+"/terragrunt.hcl"), 1+i, 1)
			if err != nil {
				t.Fatal(err)
			}
			if e.paths {
				pd, err := repograph.NewPathDependency(rp(e.to), "../x", ps, repograph.TargetHasConfig)
				if err != nil {
					t.Fatal(err)
				}
				pds = append(pds, pd)
				continue
			}
			opts := repograph.DefaultDependencyOptions()
			opts.Enabled, opts.SkipOutputs = e.enabled, e.skip
			var d repograph.Dependency
			if e.unresolved {
				d, err = repograph.NewUnresolvedDependency("d"+strconv.Itoa(i), "config-path-not-literal", ps, ps, opts)
			} else {
				d, err = repograph.NewDependency("d"+strconv.Itoa(i), rp(e.to), ps, ps, repograph.TargetHasConfig, opts)
			}
			if err != nil {
				t.Fatal(err)
			}
			deps = append(deps, d)
		}
		u, err := repograph.NewResolvedUnit(rp(iu.path), rp("modules/m"), deps, nil)
		if err != nil {
			t.Fatal(err)
		}
		if len(pds) > 0 {
			if u, err = u.WithPathDependencies(pds); err != nil {
				t.Fatal(err)
			}
		}
		us = append(us, u)
	}
	s, err := repograph.NewSurface(nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	m, err := repograph.NewModule(rp("modules/m"), s)
	if err != nil {
		t.Fatal(err)
	}
	g, err := repograph.NewRepositoryGraph(us, []repograph.Module{m})
	if err != nil {
		t.Fatal(err)
	}
	return g
}

func seeds(paths ...string) []repograph.RepoPath {
	var out []repograph.RepoPath
	for _, s := range paths {
		out = append(out, rp(s))
	}
	return out
}

func r(d int, source, via string) Reach {
	re := Reach{Distance: d, Source: rp(source)}
	if via != "" {
		re.Via = rp(via)
	}
	return re
}

func TestPropagatesEdgeRule(t *testing.T) {
	cases := []struct {
		name string
		edge iEdge
		want bool
	}{
		{"default options", blk("live/a"), true},
		{"enabled false", iEdge{to: "live/a", enabled: repograph.TristateFalse, skip: repograph.TristateFalse}, false},
		{"enabled unknown", iEdge{to: "live/a", enabled: repograph.TristateUnknown, skip: repograph.TristateFalse}, false},
		{"skip_outputs true", iEdge{to: "live/a", enabled: repograph.TristateTrue, skip: repograph.TristateTrue}, false},
		{"skip_outputs unknown", iEdge{to: "live/a", enabled: repograph.TristateTrue, skip: repograph.TristateUnknown}, false},
		{"paths edge", iEdge{to: "live/a", paths: true}, false},
		{"target is not a unit", blk("live/nowhere"), false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			g := iGraph(t, []iUnit{{path: "live/a"}, {path: "live/b", deps: []iEdge{c.edge}}})
			var got []bool
			for _, e := range g.Edges() {
				if e.From() == rp("live/b") {
					got = append(got, propagates(g, e))
				}
			}
			if len(got) != 1 || got[0] != c.want {
				t.Fatalf("propagates = %v, want [%v]", got, c.want)
			}
			if c.edge.paths && g.Edges()[0].Enabled() != repograph.TristateTrue {
				t.Fatal("a paths edge reports Enabled True: the rule must test the kind")
			}
			reach := propagate(g, seeds("live/a"))
			if _, ok := reach[rp("live/b")]; ok != c.want {
				t.Fatalf("live/b reached = %v, want %v", ok, c.want)
			}
		})
	}
	t.Run("unresolved dependency", func(t *testing.T) {
		g := iGraph(t, []iUnit{{path: "live/a"}, {path: "live/b", deps: []iEdge{{unresolved: true, enabled: repograph.TristateTrue, skip: repograph.TristateFalse}}}})
		if n := len(g.Edges()); n != 0 {
			t.Fatalf("Edges() = %d, want 0 for an unresolved dependency", n)
		}
		if _, ok := propagate(g, seeds("live/a"))[rp("live/b")]; ok {
			t.Fatal("unresolved dependency propagated")
		}
	})
}

func TestPropagateChain(t *testing.T) {
	g := iGraph(t, []iUnit{
		{path: "a"},
		{path: "b", deps: []iEdge{blk("a")}},
		{path: "c", deps: []iEdge{blk("b")}},
	})
	got := propagate(g, seeds("a"))
	want := map[repograph.RepoPath]Reach{rp("a"): r(1, "a", ""), rp("b"): r(2, "a", "a"), rp("c"): r(3, "a", "b")}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("propagate = %v, want %v", got, want)
	}
}

// TestPropagateDirection: impact flows from a dependency to its dependents,
// never forward.
func TestPropagateDirection(t *testing.T) {
	g := iGraph(t, []iUnit{
		{path: "a", deps: []iEdge{blk("b")}},
		{path: "b", deps: []iEdge{blk("c")}},
		{path: "c"},
	})
	got := propagate(g, seeds("c"))
	want := map[repograph.RepoPath]Reach{rp("c"): r(1, "c", ""), rp("b"): r(2, "c", "c"), rp("a"): r(3, "c", "b")}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("seed c: %v, want %v", got, want)
	}
	if got := propagate(g, seeds("a")); !reflect.DeepEqual(got, map[repograph.RepoPath]Reach{rp("a"): r(1, "a", "")}) {
		t.Fatalf("seed a walked forward: %v", got)
	}
}

func TestPropagateCycleSelfLoopDiamond(t *testing.T) {
	t.Run("two-cycle", func(t *testing.T) {
		g := iGraph(t, []iUnit{{path: "a", deps: []iEdge{blk("b")}}, {path: "b", deps: []iEdge{blk("a")}}})
		got := propagate(g, seeds("a"))
		want := map[repograph.RepoPath]Reach{rp("a"): r(1, "a", ""), rp("b"): r(2, "a", "a")}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("%v, want %v", got, want)
		}
	})
	t.Run("self-loops", func(t *testing.T) {
		g := iGraph(t, []iUnit{{path: "a", deps: []iEdge{blk("a")}}, {path: "b", deps: []iEdge{blk("a"), blk("b")}}})
		got := propagate(g, seeds("a"))
		want := map[repograph.RepoPath]Reach{rp("a"): r(1, "a", ""), rp("b"): r(2, "a", "a")}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("%v, want %v", got, want)
		}
	})
	t.Run("diamond", func(t *testing.T) {
		g := iGraph(t, []iUnit{
			{path: "a"},
			{path: "c", deps: []iEdge{blk("a")}},
			{path: "b", deps: []iEdge{blk("a")}},
			{path: "d", deps: []iEdge{blk("c"), blk("b")}},
		})
		if got := propagate(g, seeds("a"))[rp("d")]; got != r(3, "a", "b") {
			t.Fatalf("d = %v, want distance 3 via b", got)
		}
	})
}

// TestPropagateTieBreak pins sec #111: the path read from the unit back to
// its source is the smallest at the first differing hop (smallest
// predecessor), not the seed-first lexicographic path.
func TestPropagateTieBreak(t *testing.T) {
	g := iGraph(t, []iUnit{
		{path: "a"},
		{path: "b"},
		{path: "x", deps: []iEdge{blk("b")}},
		{path: "y", deps: []iEdge{blk("a")}},
		{path: "z", deps: []iEdge{blk("x"), blk("y")}},
	})
	got := propagate(g, seeds("b", "a"))
	if got[rp("z")] != r(3, "b", "x") {
		t.Fatalf("z = %v, want distance 3, source b, via x (z <- x <- b beats seed-first a -> y -> z)", got[rp("z")])
	}
	g2 := iGraph(t, []iUnit{
		{path: "s2"},
		{path: "s1"},
		{path: "u", deps: []iEdge{blk("s2"), blk("s1")}},
	})
	if got := propagate(g2, seeds("s2", "s1"))[rp("u")]; got != r(2, "s1", "s1") {
		t.Fatalf("u = %v, want via the smaller seed s1", got)
	}
}

// xorshift drives the shuffles: domain tests may not import math/rand.
type xorshift uint64

func (x *xorshift) intn(n int) int {
	*x ^= *x << 13
	*x ^= *x >> 7
	*x ^= *x << 17
	return int(uint64(*x) % uint64(n))
}

func (x *xorshift) shuffle(n int, swap func(i, j int)) {
	for i := n - 1; i > 0; i-- {
		swap(i, x.intn(i+1))
	}
}

func TestPropagateOrderIndependent(t *testing.T) {
	units := []iUnit{
		{path: "a"},
		{path: "b", deps: []iEdge{blk("a"), blk("c")}},
		{path: "c", deps: []iEdge{blk("a")}},
		{path: "d", deps: []iEdge{blk("b"), blk("c"), blk("e")}},
		{path: "e", deps: []iEdge{blk("d"), blk("f")}},
		{path: "f"},
		{path: "g", deps: []iEdge{blk("e"), blk("g")}},
	}
	want := propagate(iGraph(t, units), seeds("a", "f"))
	if len(want) != len(units) {
		t.Fatalf("not every unit reached: %v", want)
	}
	rng := xorshift(0x5eed)
	for range 50 {
		perm := make([]iUnit, len(units))
		for i, u := range units {
			perm[i] = iUnit{path: u.path, deps: slices.Clone(u.deps)}
			rng.shuffle(len(perm[i].deps), func(a, b int) { perm[i].deps[a], perm[i].deps[b] = perm[i].deps[b], perm[i].deps[a] })
		}
		rng.shuffle(len(perm), func(a, b int) { perm[a], perm[b] = perm[b], perm[a] })
		sd := seeds("f", "a")
		rng.shuffle(len(sd), func(a, b int) { sd[a], sd[b] = sd[b], sd[a] })
		if got := propagate(iGraph(t, perm), append(sd, sd...)); !reflect.DeepEqual(got, want) {
			t.Fatalf("order-dependent result:\n got %v\nwant %v", got, want)
		}
	}
}

// TestPropagateNoRecursion: a 20,000-unit chain proves the BFS is iterative.
func TestPropagateNoRecursion(t *testing.T) {
	const n = 20000
	units := make([]iUnit, n)
	name := func(i int) string { return "u" + strconv.Itoa(i) }
	units[0] = iUnit{path: name(0)}
	for i := 1; i < n; i++ {
		units[i] = iUnit{path: name(i), deps: []iEdge{blk(name(i - 1))}}
	}
	got := propagate(iGraph(t, units), seeds(name(0)))
	if last := got[rp(name(n-1))]; last.Distance != n || last.Via != rp(name(n-2)) {
		t.Fatalf("last = %v, want distance %d", last, n)
	}
}
