package impact_test

import (
	"slices"
	"testing"

	"github.com/GiulioSavini/gruntled/internal/domain/impact"
	"github.com/GiulioSavini/gruntled/internal/domain/repograph"
)

func p(s string) repograph.RepoPath { return repograph.MustRepoPath(s) }

func knownMod(t tb, path string, vars, outs []string) repograph.Module {
	t.Helper()
	s, err := repograph.NewSurface(vars, outs)
	if err != nil {
		t.Fatalf("NewSurface: %v", err)
	}
	m, err := repograph.NewModule(p(path), s)
	if err != nil {
		t.Fatalf("NewModule: %v", err)
	}
	return m
}

func unknownMod(t tb, path string) repograph.Module {
	t.Helper()
	m, err := repograph.NewUnknownModule(p(path), "remote source")
	if err != nil {
		t.Fatalf("NewUnknownModule: %v", err)
	}
	return m
}

func graph(t tb, units []repograph.Unit, mods ...repograph.Module) *repograph.RepositoryGraph {
	t.Helper()
	g, err := repograph.NewRepositoryGraph(units, mods)
	if err != nil {
		t.Fatalf("NewRepositoryGraph: %v", err)
	}
	return g
}

func TestSurface(t *testing.T) {
	base := graph(t, nil,
		knownMod(t, "modules/a", []string{"x", "y"}, []string{"o1"}),
		knownMod(t, "modules/same", []string{"v"}, []string{"o"}),
		knownMod(t, "modules/gone", []string{"v"}, nil),
		knownMod(t, "modules/turns-unknown", []string{"v"}, nil),
		unknownMod(t, "modules/was-unknown"),
	)
	cur := graph(t, nil,
		knownMod(t, "modules/a", []string{"y", "z", "w"}, []string{"o2", "o1"}),
		knownMod(t, "modules/same", []string{"v"}, []string{"o"}),
		knownMod(t, "modules/new", []string{"v"}, nil),
		unknownMod(t, "modules/turns-unknown"),
		knownMod(t, "modules/was-unknown", []string{"v"}, nil),
	)

	got := impact.SurfaceDiff(base, cur)
	if len(got) != 1 {
		t.Fatalf("want 1 change, got %+v", got)
	}
	c := got[0]
	if c.Module != p("modules/a") {
		t.Fatalf("module = %s", c.Module)
	}
	check := func(name string, got, want []string) {
		if !slices.Equal(got, want) {
			t.Errorf("%s = %v, want %v", name, got, want)
		}
	}
	check("AddedVariables", c.AddedVariables, []string{"w", "z"})
	check("RemovedVariables", c.RemovedVariables, []string{"x"})
	check("AddedOutputs", c.AddedOutputs, []string{"o2"})
	check("RemovedOutputs", c.RemovedOutputs, []string{})
	if c.RemovedOutputs == nil {
		t.Error("RemovedOutputs must be non-nil")
	}
	if c.Empty() {
		t.Error("change must not be Empty")
	}
}

func TestSurfaceIdenticalAndNil(t *testing.T) {
	g := graph(t, nil, knownMod(t, "m", []string{"a"}, []string{"b"}))
	if got := impact.SurfaceDiff(g, g); got == nil || len(got) != 0 {
		t.Fatalf("identical: want empty non-nil, got %#v", got)
	}
	if got := impact.SurfaceDiff(nil, g); got == nil || len(got) != 0 {
		t.Fatalf("nil base: want empty non-nil, got %#v", got)
	}
	if !(impact.SurfaceChange{}).Empty() {
		t.Fatal("zero SurfaceChange must be Empty")
	}
}

// tb is the subset of testing.TB the helpers need, shared by the impact test helpers.
type tb interface {
	Helper()
	Fatalf(format string, args ...any)
}
