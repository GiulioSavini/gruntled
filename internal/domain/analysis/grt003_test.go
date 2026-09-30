package analysis_test

import (
	"reflect"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/GiulioSavini/gruntled/internal/domain/analysis"
	"github.com/GiulioSavini/gruntled/internal/domain/diagnostic"
	"github.com/GiulioSavini/gruntled/internal/domain/repograph"
)

// cycEdge is one declared edge in a GRT003 test graph. The position is
// <from>/terragrunt.hcl:<line>:17.
type cycEdge struct {
	from, to string
	line     int
	paths    bool                                 // a dependencies paths entry instead of a block
	opts     func(o *repograph.DependencyOptions) // block only
}

// cycGraph describes a test graph: units in the graph, config-unknown units,
// and edges.
type cycGraph struct {
	units         []string
	configUnknown []string
	edges         []cycEdge
}

func cycPos(t *testing.T, from string, line int) repograph.Position {
	t.Helper()
	return mustPos(t, repograph.MustRepoPath(from+"/terragrunt.hcl"), line, 17)
}

// buildCycGraph builds the graph, feeding units to NewRepositoryGraph in
// unitOrder (a permutation of indexes into spec.units) and each unit's
// edges in reverse declaration order when reverseEdges is set.
func buildCycGraph(t *testing.T, spec cycGraph, unitOrder []int, reverseEdges bool) *repograph.RepositoryGraph {
	t.Helper()
	var units []repograph.Unit
	for _, i := range unitOrder {
		name := spec.units[i]
		var deps []repograph.Dependency
		var pds []repograph.PathDependency
		edges := slices.Clone(spec.edges)
		if reverseEdges {
			slices.Reverse(edges)
		}
		for _, e := range edges {
			if e.from != name {
				continue
			}
			pos := cycPos(t, e.from, e.line)
			if e.paths {
				pds = append(pds, mustPathDep(t, e.to, "../"+e.to, pos, repograph.TargetHasConfig))
				continue
			}
			depName := "d" + strconv.Itoa(e.line)
			deps = append(deps, mustDep(t, depName, e.to, pos, repograph.TargetHasConfig, withOpts(e.opts)))
		}
		units = append(units, mustUnit(t, name, deps, pds))
	}
	for _, name := range spec.configUnknown {
		u, err := repograph.NewConfigUnknownUnit(repograph.MustRepoPath(name), "config-syntax-error")
		if err != nil {
			t.Fatalf("NewConfigUnknownUnit: %v", err)
		}
		units = append(units, u)
	}
	return mustGraph(t, units, nil)
}

func identity(n int) []int {
	o := make([]int, n)
	for i := range o {
		o[i] = i
	}
	return o
}

func cycDiag(unit, pos, msg string) gotDiag {
	return gotDiag{Code: diagnostic.CodeDependencyCycle, Unit: unit, Pos: pos, Msg: msg}
}

var cycleCases = []struct {
	name string
	spec cycGraph
	want []gotDiag
}{
	{"self-loop is a one-member ring", cycGraph{
		units: []string{"a"},
		edges: []cycEdge{{from: "a", to: "a", line: 3}},
	}, []gotDiag{cycDiag("a", "a/terragrunt.hcl:3:17", `dependency cycle: "a" -> "a"`)}},
	{"2-ring", cycGraph{
		units: []string{"a", "b"},
		edges: []cycEdge{{from: "a", to: "b", line: 3}, {from: "b", to: "a", line: 4}},
	}, []gotDiag{cycDiag("a", "a/terragrunt.hcl:3:17", `dependency cycle: "a" -> "b" -> "a"`)}},
	{"3-ring walks successors from smallest member", cycGraph{
		units: []string{"a", "b", "c"},
		edges: []cycEdge{{from: "a", to: "c", line: 5}, {from: "c", to: "b", line: 2}, {from: "b", to: "a", line: 7}},
	}, []gotDiag{cycDiag("a", "a/terragrunt.hcl:5:17", `dependency cycle: "a" -> "c" -> "b" -> "a"`)}},
	{"non-ring SCC (diamond back-edge) renders among", cycGraph{
		units: []string{"a", "b", "c", "d"},
		edges: []cycEdge{
			{from: "a", to: "b", line: 6}, {from: "a", to: "c", line: 4},
			{from: "b", to: "d", line: 1}, {from: "c", to: "d", line: 1},
			{from: "d", to: "a", line: 1},
		},
	}, []gotDiag{cycDiag("a", "a/terragrunt.hcl:4:17", `dependency cycle among: "a", "b", "c", "d"`)}},
	{"self-loop inside larger SCC gives one among diagnostic", cycGraph{
		units: []string{"a", "b"},
		edges: []cycEdge{{from: "a", to: "a", line: 2}, {from: "a", to: "b", line: 5}, {from: "b", to: "a", line: 3}},
	}, []gotDiag{cycDiag("a", "a/terragrunt.hcl:5:17", `dependency cycle among: "a", "b"`)}},
	{"block and paths edges to same target dedupe, ring stays ring", cycGraph{
		units: []string{"a", "b"},
		edges: []cycEdge{
			{from: "a", to: "b", line: 4}, {from: "a", to: "b", line: 2, paths: true},
			{from: "b", to: "a", line: 3},
		},
	}, []gotDiag{cycDiag("a", "a/terragrunt.hcl:2:17", `dependency cycle: "a" -> "b" -> "a"`)}},
	{"disabled and non-literal enabled edges dropped", cycGraph{
		units: []string{"a", "b", "c", "d"},
		edges: []cycEdge{
			{from: "a", to: "b", line: 3},
			{from: "b", to: "a", line: 4, opts: func(o *repograph.DependencyOptions) { o.Enabled = repograph.TristateFalse }},
			{from: "c", to: "d", line: 3},
			{from: "d", to: "c", line: 4, opts: func(o *repograph.DependencyOptions) { o.Enabled = repograph.TristateUnknown }},
		},
	}, nil},
	{"skip_outputs still an edge", cycGraph{
		units: []string{"a", "b"},
		edges: []cycEdge{
			{from: "a", to: "b", line: 3, opts: func(o *repograph.DependencyOptions) { o.SkipOutputs = repograph.TristateTrue }},
			{from: "b", to: "a", line: 4},
		},
	}, []gotDiag{cycDiag("a", "a/terragrunt.hcl:3:17", `dependency cycle: "a" -> "b" -> "a"`)}},
	{"edge to non-graph target dropped", cycGraph{
		units: []string{"a", "b"},
		edges: []cycEdge{{from: "a", to: "x", line: 3}, {from: "b", to: "x", line: 3}, {from: "a", to: "b", line: 4}},
	}, nil},
	{"config-unknown unit contributes no edges", cycGraph{
		units:         []string{"a"},
		configUnknown: []string{"c"},
		edges:         []cycEdge{{from: "a", to: "c", line: 3}},
	}, nil},
	{"acyclic", cycGraph{
		units: []string{"a", "b", "c"},
		edges: []cycEdge{{from: "a", to: "b", line: 3}, {from: "b", to: "c", line: 3}, {from: "a", to: "c", line: 4}},
	}, nil},
	{"two disjoint cycles", cycGraph{
		units: []string{"a", "b", "c", "d"},
		edges: []cycEdge{
			{from: "d", to: "c", line: 1}, {from: "c", to: "d", line: 2},
			{from: "a", to: "b", line: 1}, {from: "b", to: "a", line: 1},
		},
	}, []gotDiag{
		cycDiag("a", "a/terragrunt.hcl:1:17", `dependency cycle: "a" -> "b" -> "a"`),
		cycDiag("c", "c/terragrunt.hcl:2:17", `dependency cycle: "c" -> "d" -> "c"`),
	}},
	{"two cycles linked by a one-way edge stay separate", cycGraph{
		units: []string{"a", "b", "c", "d"},
		edges: []cycEdge{
			{from: "a", to: "b", line: 1}, {from: "b", to: "a", line: 1},
			{from: "b", to: "c", line: 2},
			{from: "c", to: "d", line: 1}, {from: "d", to: "c", line: 1},
		},
	}, []gotDiag{
		cycDiag("a", "a/terragrunt.hcl:1:17", `dependency cycle: "a" -> "b" -> "a"`),
		cycDiag("c", "c/terragrunt.hcl:1:17", `dependency cycle: "c" -> "d" -> "c"`),
	}},
}

func TestDependencyCycles(t *testing.T) {
	for _, tc := range cycleCases {
		t.Run(tc.name, func(t *testing.T) {
			g := buildCycGraph(t, tc.spec, identity(len(tc.spec.units)), false)
			ds, err := analysis.DependencyCycles(g)
			if err != nil {
				t.Fatalf("DependencyCycles: %v", err)
			}
			if tc.want == nil && ds != nil {
				t.Errorf("want nil slice, got %#v", ds)
			}
			if got := project(t, ds); !reflect.DeepEqual(got, tc.want) {
				t.Errorf("got  %#v\nwant %#v", got, tc.want)
			}
		})
	}
}

func TestCyclesDeterministic(t *testing.T) {
	for _, tc := range cycleCases {
		t.Run(tc.name, func(t *testing.T) {
			n := len(tc.spec.units)
			base := project(t, mustCycles(t, buildCycGraph(t, tc.spec, identity(n), false)))
			for rot := 0; rot < n; rot++ {
				for _, rev := range []bool{false, true} {
					order := identity(n)
					slices.Reverse(order)
					order = append(order[rot:], order[:rot]...)
					got := project(t, mustCycles(t, buildCycGraph(t, tc.spec, order, rev)))
					if !reflect.DeepEqual(got, base) {
						t.Errorf("order %v reverseEdges=%v: got %#v, want %#v", order, rev, got, base)
					}
				}
			}
		})
	}
}

func mustCycles(t *testing.T, g *repograph.RepositoryGraph) []diagnostic.Diagnostic {
	t.Helper()
	ds, err := analysis.DependencyCycles(g)
	if err != nil {
		t.Fatalf("DependencyCycles: %v", err)
	}
	return ds
}

func TestCyclesDeepChain(t *testing.T) {
	const n = 10000
	name := func(i int) string {
		s := strconv.Itoa(i)
		return "c/" + strings.Repeat("0", 5-len(s)) + s
	}
	build := func(closed bool) *repograph.RepositoryGraph {
		units := make([]repograph.Unit, 0, n)
		for i := 0; i < n; i++ {
			var deps []repograph.Dependency
			if i+1 < n || closed {
				pos := cycPos(t, name(i), 1)
				deps = append(deps, mustDep(t, "next", name((i+1)%n), pos, repograph.TargetHasConfig, withOpts(nil)))
			}
			units = append(units, mustUnit(t, name(i), deps, nil))
		}
		return mustGraph(t, units, nil)
	}

	if ds := mustCycles(t, build(false)); ds != nil {
		t.Fatalf("open chain: got %d diagnostics, want none", len(ds))
	}
	ds := mustCycles(t, build(true))
	if len(ds) != 1 {
		t.Fatalf("closed chain: got %d diagnostics, want 1", len(ds))
	}
	got := project(t, ds)[0]
	if got.Unit != "c/00000" || got.Pos != "c/00000/terragrunt.hcl:1:17" {
		t.Errorf("anchor = %s at %s", got.Unit, got.Pos)
	}
	wantPrefix := `dependency cycle: "c/00000" -> "c/00001" -> "c/00002"`
	wantSuffix := `"c/09999" -> "c/00000"`
	if !strings.HasPrefix(got.Msg, wantPrefix) || !strings.HasSuffix(got.Msg, wantSuffix) {
		t.Errorf("message = %.80s ... %s", got.Msg, got.Msg[len(got.Msg)-40:])
	}
	if c := strings.Count(got.Msg, " -> "); c != n {
		t.Errorf("message has %d arrows, want %d", c, n)
	}
}
