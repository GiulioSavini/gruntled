package repograph_test

import (
	"reflect"
	"testing"

	"github.com/GiulioSavini/gruntled/internal/domain/repograph"
)

type edgeRow struct {
	kind    repograph.EdgeKind
	from    string
	to      string
	pos     repograph.Position
	enabled repograph.Tristate
}

func edgeRows(es []repograph.Edge) []edgeRow {
	rows := make([]edgeRow, 0, len(es))
	for _, e := range es {
		rows = append(rows, edgeRow{e.Kind(), e.From().String(), e.To().String(), e.Pos(), e.Enabled()})
	}
	return rows
}

// edgeMeta is the per-edge metadata the graph presenter reads straight from
// the edge: block label, observed target state and skip_outputs.
type edgeMeta struct {
	name        string
	state       repograph.TargetState
	skipOutputs repograph.Tristate
}

func edgeMetas(es []repograph.Edge) []edgeMeta {
	rows := make([]edgeMeta, 0, len(es))
	for _, e := range es {
		rows = append(rows, edgeMeta{e.Name(), e.TargetState(), e.SkipOutputs()})
	}
	return rows
}

func TestEdges(t *testing.T) {
	aFile := repograph.MustRepoPath("units/a/terragrunt.hcl")
	bFile := repograph.MustRepoPath("units/b/terragrunt.hcl")
	mod := repograph.MustRepoPath("modules/m")
	opts := repograph.DefaultDependencyOptions()
	disabled := opts
	disabled.Enabled = repograph.TristateFalse
	disabled.SkipOutputs = repograph.TristateUnknown

	// units/a: block dep "c" (enabled) at pathPos 3:17, block dep "x"
	// (disabled, target outside the graph) at pathPos 8:17, unresolved
	// block dep "dyn", and a paths entry to units/b at 12:14 plus an
	// unresolved paths entry.
	depC, err := repograph.NewDependency("c", repograph.MustRepoPath("units/c"), mustPosition(t, aFile, 2, 1), mustPosition(t, aFile, 3, 17), repograph.TargetHasConfig, opts)
	if err != nil {
		t.Fatal(err)
	}
	depX, err := repograph.NewDependency("x", repograph.MustRepoPath("units/missing"), mustPosition(t, aFile, 7, 1), mustPosition(t, aFile, 8, 17), repograph.TargetDirMissing, disabled)
	if err != nil {
		t.Fatal(err)
	}
	depDyn, err := repograph.NewUnresolvedDependency("dyn", "config-path-dynamic", mustPosition(t, aFile, 9, 1), mustPosition(t, aFile, 10, 17), opts)
	if err != nil {
		t.Fatal(err)
	}
	unitA, err := repograph.NewResolvedUnit(repograph.MustRepoPath("units/a"), mod, []repograph.Dependency{depX, depDyn, depC}, nil)
	if err != nil {
		t.Fatal(err)
	}
	pdB, err := repograph.NewPathDependency(repograph.MustRepoPath("units/b"), "../b", mustPosition(t, aFile, 12, 14), repograph.TargetNoConfig)
	if err != nil {
		t.Fatal(err)
	}
	pdDyn, err := repograph.NewUnresolvedPathDependency("config-path-dynamic", mustPosition(t, aFile, 12, 30))
	if err != nil {
		t.Fatal(err)
	}
	if unitA, err = unitA.WithPathDependencies([]repograph.PathDependency{pdDyn, pdB}); err != nil {
		t.Fatal(err)
	}

	// units/b: a block dep and a paths entry at the same file, block first.
	depA, err := repograph.NewDependency("a", repograph.MustRepoPath("units/a"), mustPosition(t, bFile, 1, 1), mustPosition(t, bFile, 2, 17), repograph.TargetHasConfig, opts)
	if err != nil {
		t.Fatal(err)
	}
	unitB, err := repograph.NewResolvedUnit(repograph.MustRepoPath("units/b"), mod, []repograph.Dependency{depA}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if unitB, err = unitB.WithPathDependencies([]repograph.PathDependency{mustPathDependency(t, "units/c", mustPosition(t, bFile, 5, 14))}); err != nil {
		t.Fatal(err)
	}

	unitC, err := repograph.NewResolvedUnit(repograph.MustRepoPath("units/c"), mod, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	m := mustModule(t, mod, mustSurface(t, nil, nil))

	want := []edgeRow{
		{repograph.EdgeBlock, "units/a", "units/c", mustPosition(t, aFile, 3, 17), repograph.TristateTrue},
		{repograph.EdgeBlock, "units/a", "units/missing", mustPosition(t, aFile, 8, 17), repograph.TristateFalse},
		{repograph.EdgePaths, "units/a", "units/b", mustPosition(t, aFile, 12, 14), repograph.TristateTrue},
		{repograph.EdgeBlock, "units/b", "units/a", mustPosition(t, bFile, 2, 17), repograph.TristateTrue},
		{repograph.EdgePaths, "units/b", "units/c", mustPosition(t, bFile, 5, 14), repograph.TristateTrue},
	}

	wantMeta := []edgeMeta{
		{"c", repograph.TargetHasConfig, repograph.TristateFalse},
		{"x", repograph.TargetDirMissing, repograph.TristateUnknown},
		{"", repograph.TargetNoConfig, repograph.TristateFalse},
		{"a", repograph.TargetHasConfig, repograph.TristateFalse},
		{"", repograph.TargetHasConfig, repograph.TristateFalse},
	}

	orders := [][]repograph.Unit{
		{unitA, unitB, unitC},
		{unitC, unitB, unitA},
		{unitB, unitC, unitA},
	}
	for i, units := range orders {
		g, err := repograph.NewRepositoryGraph(units, []repograph.Module{m})
		if err != nil {
			t.Fatalf("order %d: NewRepositoryGraph: %v", i, err)
		}
		if got := edgeRows(g.Edges()); !reflect.DeepEqual(got, want) {
			t.Errorf("order %d: Edges() =\n%v\nwant\n%v", i, got, want)
		}
		if got := edgeMetas(g.Edges()); !reflect.DeepEqual(got, wantMeta) {
			t.Errorf("order %d: Edges() metadata =\n%v\nwant\n%v", i, got, wantMeta)
		}
	}
}

func TestEdgesTieBreakKindThenTarget(t *testing.T) {
	file := repograph.MustRepoPath("units/a/terragrunt.hcl")
	mod := repograph.MustRepoPath("modules/m")
	same := mustPosition(t, file, 3, 1)
	dep, err := repograph.NewDependency("z", repograph.MustRepoPath("units/z"), mustPosition(t, file, 1, 1), same, repograph.TargetHasConfig, repograph.DefaultDependencyOptions())
	if err != nil {
		t.Fatal(err)
	}
	u, err := repograph.NewResolvedUnit(repograph.MustRepoPath("units/a"), mod, []repograph.Dependency{dep}, nil)
	if err != nil {
		t.Fatal(err)
	}
	u, err = u.WithPathDependencies([]repograph.PathDependency{mustPathDependency(t, "units/y", same), mustPathDependency(t, "units/b", same)})
	if err != nil {
		t.Fatal(err)
	}
	g, err := repograph.NewRepositoryGraph([]repograph.Unit{u}, []repograph.Module{mustModule(t, mod, mustSurface(t, nil, nil))})
	if err != nil {
		t.Fatal(err)
	}
	want := []edgeRow{
		{repograph.EdgeBlock, "units/a", "units/z", same, repograph.TristateTrue},
		{repograph.EdgePaths, "units/a", "units/b", same, repograph.TristateTrue},
		{repograph.EdgePaths, "units/a", "units/y", same, repograph.TristateTrue},
	}
	if got := edgeRows(g.Edges()); !reflect.DeepEqual(got, want) {
		t.Errorf("Edges() =\n%v\nwant\n%v", got, want)
	}
}

func TestEdgesEmpty(t *testing.T) {
	mod := repograph.MustRepoPath("modules/m")
	u, err := repograph.NewResolvedUnit(repograph.MustRepoPath("units/a"), mod, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	cu, err := repograph.NewConfigUnknownUnit(repograph.MustRepoPath("units/b"), "broken")
	if err != nil {
		t.Fatal(err)
	}
	g, err := repograph.NewRepositoryGraph([]repograph.Unit{u, cu}, []repograph.Module{mustModule(t, mod, mustSurface(t, nil, nil))})
	if err != nil {
		t.Fatal(err)
	}
	if got := g.Edges(); len(got) != 0 {
		t.Errorf("Edges() = %v, want empty", got)
	}
	empty, err := repograph.NewRepositoryGraph(nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got := empty.Edges(); len(got) != 0 {
		t.Errorf("empty graph Edges() = %v, want empty", got)
	}
}
