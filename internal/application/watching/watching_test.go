package watching_test

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/GiulioSavini/gruntled/internal/application/checking"
	"github.com/GiulioSavini/gruntled/internal/application/ports"
	"github.com/GiulioSavini/gruntled/internal/application/watching"
	"github.com/GiulioSavini/gruntled/internal/domain/repograph"
)

// recLoader is a hand-written ports.InvalidatingLoader recording every
// call in order: "invalidate:<p1>,<p2>" or "load".
type recLoader struct {
	calls []string
	res   ports.LoadResult
	err   error
}

func (r *recLoader) Invalidate(paths ...string) {
	r.calls = append(r.calls, "invalidate:"+strings.Join(paths, ","))
}

func (r *recLoader) LoadUnits(ctx context.Context) (ports.LoadResult, error) {
	r.calls = append(r.calls, "load")
	return r.res, r.err
}

// fakeSurfaces is a hand-written ports.SurfaceReader keyed by module path;
// an unknown module reads as a missing directory.
type fakeSurfaces map[string]ports.SurfaceResult

func (f fakeSurfaces) ReadSurface(ctx context.Context, module repograph.RepoPath) (ports.SurfaceResult, error) {
	if res, ok := f[module.String()]; ok {
		return res, nil
	}
	return ports.SurfaceResult{UnknownReason: "not found"}, nil
}

func mustPos(t *testing.T, file string, line, col int) repograph.Position {
	t.Helper()
	p, err := repograph.NewPosition(repograph.MustRepoPath(file), line, col)
	if err != nil {
		t.Fatalf("NewPosition: %v", err)
	}
	return p
}

// fixture is live/app -> live/vpc referencing an output modules/vpc does
// not declare, so the Report carries a GRT001.
func fixture(t *testing.T) (ports.LoadResult, fakeSurfaces) {
	t.Helper()
	pos := mustPos(t, "live/app/terragrunt.hcl", 1, 1)
	dep, err := repograph.NewDependency("vpc", repograph.MustRepoPath("live/vpc"), pos, pos, repograph.TargetUnknown, repograph.DefaultDependencyOptions())
	if err != nil {
		t.Fatalf("NewDependency: %v", err)
	}
	ref, err := repograph.NewReference("vpc", "missing", mustPos(t, "live/app/terragrunt.hcl", 6, 12))
	if err != nil {
		t.Fatalf("NewReference: %v", err)
	}
	vpc, err := repograph.NewSurface(nil, []string{"vpc_id"})
	if err != nil {
		t.Fatalf("NewSurface: %v", err)
	}
	app, err := repograph.NewSurface(nil, nil)
	if err != nil {
		t.Fatalf("NewSurface: %v", err)
	}
	res := ports.LoadResult{Units: []ports.UnitConfig{
		{
			Path:         repograph.MustRepoPath("live/app"),
			Module:       repograph.MustRepoPath("modules/app"),
			Dependencies: []repograph.Dependency{dep},
			References:   []repograph.Reference{ref},
		},
		{Path: repograph.MustRepoPath("live/vpc"), Module: repograph.MustRepoPath("modules/vpc")},
	}}
	return res, fakeSurfaces{"modules/app": {Surface: app}, "modules/vpc": {Surface: vpc}}
}

func TestIndexForwardsDirtyPaths(t *testing.T) {
	l := &recLoader{}
	if _, err := watching.NewIndexer(l, fakeSurfaces{}).Index(context.Background(), []string{"a/terragrunt.hcl", "b"}, false); err != nil {
		t.Fatalf("Index: %v", err)
	}
	want := []string{"invalidate:a/terragrunt.hcl,b", "load"}
	if !reflect.DeepEqual(l.calls, want) {
		t.Fatalf("calls = %q, want %q", l.calls, want)
	}
}

func TestIndexResyncInvalidatesEverything(t *testing.T) {
	l := &recLoader{}
	if _, err := watching.NewIndexer(l, fakeSurfaces{}).Index(context.Background(), []string{"a"}, true); err != nil {
		t.Fatalf("Index: %v", err)
	}
	want := []string{"invalidate:.", "load"}
	if !reflect.DeepEqual(l.calls, want) {
		t.Fatalf("calls = %q, want %q", l.calls, want)
	}
}

func TestIndexInitialInvalidatesNothing(t *testing.T) {
	l := &recLoader{}
	if _, err := watching.NewIndexer(l, fakeSurfaces{}).Index(context.Background(), nil, false); err != nil {
		t.Fatalf("Index: %v", err)
	}
	if want := []string{"load"}; !reflect.DeepEqual(l.calls, want) {
		t.Fatalf("calls = %q, want %q", l.calls, want)
	}
}

func TestIndexReportEqualsCheck(t *testing.T) {
	res, surfaces := fixture(t)
	ctx := context.Background()

	got, err := watching.NewIndexer(&recLoader{res: res}, surfaces).Index(ctx, []string{"live/app"}, false)
	if err != nil {
		t.Fatalf("Index: %v", err)
	}
	want, err := checking.Check(ctx, &recLoader{res: res}, surfaces)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if want.Diagnostics.Len() == 0 {
		t.Fatalf("fixture produced no diagnostics; test is vacuous")
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Index report differs from Check:\n got  %+v\n want %+v", got, want)
	}
}

func TestIndexLoaderErrorUnchanged(t *testing.T) {
	sentinel := errors.New("root unreadable")
	ctx := context.Background()

	got, err := watching.NewIndexer(&recLoader{err: sentinel}, fakeSurfaces{}).Index(ctx, nil, true)
	_, wantErr := checking.Check(ctx, &recLoader{err: sentinel}, fakeSurfaces{})
	if !errors.Is(err, sentinel) {
		t.Fatalf("err = %v, want it to wrap %v", err, sentinel)
	}
	if !reflect.DeepEqual(err, wantErr) {
		t.Fatalf("err = %#v, want Check's %#v", err, wantErr)
	}
	if !reflect.DeepEqual(got, checking.Report{}) {
		t.Fatalf("report = %+v, want zero", got)
	}
}
