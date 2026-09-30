package checking_test

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/GiulioSavini/gruntled/internal/application/checking"
	"github.com/GiulioSavini/gruntled/internal/application/indexing"
	"github.com/GiulioSavini/gruntled/internal/application/ports"
	"github.com/GiulioSavini/gruntled/internal/domain/diagnostic"
	"github.com/GiulioSavini/gruntled/internal/domain/repograph"
)

// fakeLoader is a hand-written ports.UnitLoader: no filesystem.
type fakeLoader struct {
	res ports.LoadResult
	err error
}

func (f fakeLoader) LoadUnits(ctx context.Context) (ports.LoadResult, error) {
	return f.res, f.err
}

// fakeSurfaces is a hand-written ports.SurfaceReader keyed by module path.
type fakeSurfaces struct {
	t        *testing.T
	byModule map[string]ports.SurfaceResult
}

func (f fakeSurfaces) ReadSurface(ctx context.Context, module repograph.RepoPath) (ports.SurfaceResult, error) {
	res, ok := f.byModule[module.String()]
	if !ok {
		f.t.Fatalf("fakeSurfaces: no entry for module %q", module.String())
	}
	return res, nil
}

func mustPos(t *testing.T, file string, line, col int) repograph.Position {
	t.Helper()
	p, err := repograph.NewPosition(repograph.MustRepoPath(file), line, col)
	if err != nil {
		t.Fatalf("NewPosition: %v", err)
	}
	return p
}

func mustSurface(t *testing.T, outputs ...string) repograph.Surface {
	t.Helper()
	s, err := repograph.NewSurface(nil, outputs)
	if err != nil {
		t.Fatalf("NewSurface: %v", err)
	}
	return s
}

// repo returns a loader and surface reader for live/app -> live/vpc, where
// live/app references dependency.vpc.outputs.<output> and modules/vpc
// declares vpc_id.
func repo(t *testing.T, output string) (fakeLoader, fakeSurfaces) {
	t.Helper()
	dep, err := repograph.NewDependency("vpc", repograph.MustRepoPath("live/vpc"), mustPos(t, "live/app/terragrunt.hcl", 1, 1), mustPos(t, "live/app/terragrunt.hcl", 1, 1), repograph.TargetUnknown, repograph.DefaultDependencyOptions())
	if err != nil {
		t.Fatalf("NewDependency: %v", err)
	}
	ref, err := repograph.NewReference("vpc", output, mustPos(t, "live/app/terragrunt.hcl", 6, 12))
	if err != nil {
		t.Fatalf("NewReference: %v", err)
	}
	loader := fakeLoader{res: ports.LoadResult{Units: []ports.UnitConfig{
		{
			Path:         repograph.MustRepoPath("live/app"),
			Module:       repograph.MustRepoPath("modules/app"),
			Dependencies: []repograph.Dependency{dep},
			References:   []repograph.Reference{ref},
		},
		{
			Path:   repograph.MustRepoPath("live/vpc"),
			Module: repograph.MustRepoPath("modules/vpc"),
		},
	}}}
	surfaces := fakeSurfaces{t: t, byModule: map[string]ports.SurfaceResult{
		"modules/app": {Surface: mustSurface(t)},
		"modules/vpc": {Surface: mustSurface(t, "vpc_id")},
	}}
	return loader, surfaces
}

func grt100(t *testing.T, file string, line int) diagnostic.Diagnostic {
	t.Helper()
	d, err := diagnostic.New(diagnostic.CodeSyntaxError, diagnostic.SeverityError, mustPos(t, file, line, 1), "syntax error")
	if err != nil {
		t.Fatalf("diagnostic.New: %v", err)
	}
	return d
}

func TestCheckClean(t *testing.T) {
	loader, surfaces := repo(t, "vpc_id")
	rep, err := checking.Check(context.Background(), loader, surfaces)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if rep.Diagnostics.Len() != 0 {
		t.Errorf("got %d diagnostics, want 0: %v", rep.Diagnostics.Len(), rep.Diagnostics.All())
	}
	if rep.Graph == nil || len(rep.Graph.Units()) != 2 {
		t.Fatalf("graph = %v, want 2 units", rep.Graph)
	}
}

func TestCheckBrokenReference(t *testing.T) {
	loader, surfaces := repo(t, "vpc_idd")
	rep, err := checking.Check(context.Background(), loader, surfaces)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	all := rep.Diagnostics.All()
	if len(all) != 1 || all[0].Code() != diagnostic.CodeUnknownOutput {
		t.Fatalf("got %v, want exactly one GRT001", all)
	}
	if !rep.Diagnostics.HasErrors() {
		t.Errorf("HasErrors = false, want true")
	}
}

func TestCheckMergesLoaderAndSurfaceDiagnostics(t *testing.T) {
	loader, surfaces := repo(t, "vpc_idd")
	loaderDiag := grt100(t, "live/zzz/terragrunt.hcl", 3)
	surfaceDiag := grt100(t, "modules/app/main.tf", 9)
	loader.res.Diagnostics = []diagnostic.Diagnostic{loaderDiag}
	app := surfaces.byModule["modules/app"]
	app.Diagnostics = []diagnostic.Diagnostic{surfaceDiag}
	surfaces.byModule["modules/app"] = app

	rep, err := checking.Check(context.Background(), loader, surfaces)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	all := rep.Diagnostics.All()
	if len(all) != 3 {
		t.Fatalf("got %d diagnostics, want 3: %v", len(all), all)
	}
	var grt001 diagnostic.Diagnostic
	for _, d := range all {
		if d.Code() == diagnostic.CodeUnknownOutput {
			grt001 = d
		}
	}
	if grt001.Code() != diagnostic.CodeUnknownOutput {
		t.Fatalf("no GRT001 in %v", all)
	}
	want := diagnostic.NewSet(surfaceDiag, grt001, loaderDiag).All()
	if !reflect.DeepEqual(all, want) {
		t.Errorf("order =\n  %v\nwant canonical\n  %v", all, want)
	}
}

func TestCheckLoaderErrorPassesThrough(t *testing.T) {
	boom := errors.New("boom")
	rep, err := checking.Check(context.Background(), fakeLoader{err: boom}, fakeSurfaces{t: t})
	var ie *indexing.Error
	if !errors.As(err, &ie) {
		t.Fatalf("err = %v, want *indexing.Error", err)
	}
	if !errors.Is(err, boom) {
		t.Errorf("err = %v, want it to wrap boom", err)
	}
	if !reflect.DeepEqual(rep, checking.Report{}) {
		t.Errorf("report = %v, want zero", rep)
	}
}

func TestCheckCancelledContext(t *testing.T) {
	loader, surfaces := repo(t, "vpc_id")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := checking.Check(ctx, loader, surfaces)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
}

func TestErrorMessageAndUnwrap(t *testing.T) {
	inner := errors.New("inner")
	e := &checking.Error{Stage: "analyze", Err: inner}
	if e.Error() != "checking: analyze: inner" {
		t.Errorf("Error() = %q", e.Error())
	}
	if !errors.Is(e, inner) {
		t.Errorf("Unwrap does not reach inner")
	}
}

// TestCheckAllAnalyzers builds a graph with one broken reference (GRT001),
// one dependency on a missing directory (GRT002) and one cycle (GRT003),
// and expects all three in one canonical Set.
func TestCheckAllAnalyzers(t *testing.T) {
	loader, surfaces := repo(t, "vpc_idd")
	mustDep := func(name, target, file string, line int, state repograph.TargetState) repograph.Dependency {
		t.Helper()
		pos := mustPos(t, file, line, 17)
		d, err := repograph.NewDependency(name, repograph.MustRepoPath(target), pos, pos, state, repograph.DefaultDependencyOptions())
		if err != nil {
			t.Fatalf("NewDependency: %v", err)
		}
		return d
	}
	units := loader.res.Units
	units[0].Dependencies = append(units[0].Dependencies,
		mustDep("gone", "live/gone", "live/app/terragrunt.hcl", 2, repograph.TargetDirMissing))
	units = append(units,
		ports.UnitConfig{
			Path:                repograph.MustRepoPath("live/x"),
			ModuleUnknownReason: "source-remote",
			Dependencies:        []repograph.Dependency{mustDep("y", "live/y", "live/x/terragrunt.hcl", 1, repograph.TargetHasConfig)},
		},
		ports.UnitConfig{
			Path:                repograph.MustRepoPath("live/y"),
			ModuleUnknownReason: "source-remote",
			Dependencies:        []repograph.Dependency{mustDep("x", "live/x", "live/y/terragrunt.hcl", 1, repograph.TargetHasConfig)},
		},
	)
	loader.res.Units = units

	rep, err := checking.Check(context.Background(), loader, surfaces)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	type row struct {
		Code diagnostic.Code
		Pos  string
		Msg  string
	}
	var got []row
	for _, d := range rep.Diagnostics.All() {
		got = append(got, row{d.Code(), d.Pos().String(), d.Message()})
	}
	want := []row{
		{diagnostic.CodeMissingDependencyTarget, "live/app/terragrunt.hcl:2:17", `dependency "gone" config_path resolves to "live/gone": directory does not exist`},
		{diagnostic.CodeUnknownOutput, "live/app/terragrunt.hcl:6:12", `dependency "vpc" output "vpc_idd" is not declared by module "modules/vpc" (target unit "live/vpc")`},
		{diagnostic.CodeDependencyCycle, "live/x/terragrunt.hcl:1:17", `dependency cycle: "live/x" -> "live/y" -> "live/x"`},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got  %#v\nwant %#v", got, want)
	}
	if !rep.Diagnostics.Equal(diagnostic.NewSet(rep.Diagnostics.All()...)) {
		t.Errorf("Set is not canonical")
	}
}
