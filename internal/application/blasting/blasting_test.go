package blasting_test

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/GiulioSavini/gruntled/internal/application/blasting"
	"github.com/GiulioSavini/gruntled/internal/application/ports"
	"github.com/GiulioSavini/gruntled/internal/domain/diagnostic"
	"github.com/GiulioSavini/gruntled/internal/domain/impact"
	"github.com/GiulioSavini/gruntled/internal/domain/repograph"
)

// fakeLoader is a hand-written ports.UnitLoader that counts its calls.
type fakeLoader struct {
	res   ports.LoadResult
	err   error
	calls *int
}

func (f fakeLoader) LoadUnits(ctx context.Context) (ports.LoadResult, error) {
	if f.calls != nil {
		*f.calls++
	}
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

func grt100(t *testing.T, file string, line int) diagnostic.Diagnostic {
	t.Helper()
	d, err := diagnostic.New(diagnostic.CodeSyntaxError, diagnostic.SeverityError, mustPos(t, file, line, 1), "syntax error")
	if err != nil {
		t.Fatalf("diagnostic.New: %v", err)
	}
	return d
}

// tree returns Sources for live/app -> live/vpc where live/app references
// dependency.vpc.outputs.<output> at refLine and modules/vpc declares
// vpcOutputs. extra diagnostics are reported by the loader.
func tree(t *testing.T, output string, refLine int, vpcOutputs []string, calls *int, extra ...diagnostic.Diagnostic) blasting.Sources {
	t.Helper()
	dep, err := repograph.NewDependency("vpc", repograph.MustRepoPath("live/vpc"), mustPos(t, "live/app/terragrunt.hcl", 1, 1), mustPos(t, "live/app/terragrunt.hcl", 1, 1), repograph.TargetUnknown, repograph.DefaultDependencyOptions())
	if err != nil {
		t.Fatalf("NewDependency: %v", err)
	}
	ref, err := repograph.NewReference("vpc", output, mustPos(t, "live/app/terragrunt.hcl", refLine, 12))
	if err != nil {
		t.Fatalf("NewReference: %v", err)
	}
	loader := fakeLoader{calls: calls, res: ports.LoadResult{
		Units: []ports.UnitConfig{
			{
				Path:         repograph.MustRepoPath("live/app"),
				Module:       repograph.MustRepoPath("modules/app"),
				Dependencies: []repograph.Dependency{dep},
				References:   []repograph.Reference{ref},
			},
			{Path: repograph.MustRepoPath("live/vpc"), Module: repograph.MustRepoPath("modules/vpc")},
			{Path: repograph.MustRepoPath("live/db"), Module: repograph.MustRepoPath("modules/vpc")},
		},
		Diagnostics: extra,
	}}
	surfaces := fakeSurfaces{t: t, byModule: map[string]ports.SurfaceResult{
		"modules/app": {Surface: mustSurface(t)},
		"modules/vpc": {Surface: mustSurface(t, vpcOutputs...)},
	}}
	return blasting.Sources{Units: loader, Surfaces: surfaces}
}

func subjects(res impact.Result) (broken, impacted []string) {
	broken, impacted = []string{}, []string{}
	for _, b := range res.Broken {
		broken = append(broken, b.Subject.String())
	}
	for _, i := range res.Impacted {
		impacted = append(impacted, i.Unit.String())
	}
	return broken, impacted
}

func TestBlastNoBaseline(t *testing.T) {
	cur := tree(t, "vpc_idd", 6, []string{"vpc_id"}, nil, grt100(t, "live/zzz/terragrunt.hcl", 3))
	res, err := blasting.Blast(context.Background(), cur, nil)
	if err != nil {
		t.Fatalf("Blast: %v", err)
	}
	if res.Baseline {
		t.Errorf("Baseline = true, want false")
	}
	broken, impacted := subjects(res)
	if want := []string{"live/app", "live/zzz/terragrunt.hcl"}; !reflect.DeepEqual(broken, want) {
		t.Errorf("Broken = %v, want %v", broken, want)
	}
	if len(impacted) != 0 || res.Impacted == nil {
		t.Errorf("Impacted = %#v, want empty non-nil", res.Impacted)
	}
}

func TestBlastShiftedFindingNotBroken(t *testing.T) {
	var baseCalls, curCalls int
	base := tree(t, "vpc_idd", 6, []string{"vpc_id"}, &baseCalls, grt100(t, "live/zzz/terragrunt.hcl", 3))
	cur := tree(t, "vpc_idd", 9, []string{"vpc_id"}, &curCalls, grt100(t, "live/zzz/terragrunt.hcl", 7))
	res, err := blasting.Blast(context.Background(), cur, &base)
	if err != nil {
		t.Fatalf("Blast: %v", err)
	}
	if !res.Baseline {
		t.Errorf("Baseline = false, want true")
	}
	if len(res.Broken) != 0 || len(res.Impacted) != 0 {
		t.Errorf("got Broken %v Impacted %v, want both empty", res.Broken, res.Impacted)
	}
	if baseCalls != 1 || curCalls != 1 {
		t.Errorf("loader calls base=%d cur=%d, want 1 each", baseCalls, curCalls)
	}
}

func TestBlastBrokenAndImpacted(t *testing.T) {
	// modules/vpc renames vpc_id to id: live/app now references a missing
	// output (Broken), live/db and live/vpc consume modules/vpc (Impacted).
	base := tree(t, "vpc_id", 6, []string{"vpc_id"}, nil)
	cur := tree(t, "vpc_id", 6, []string{"id"}, nil)
	res, err := blasting.Blast(context.Background(), cur, &base)
	if err != nil {
		t.Fatalf("Blast: %v", err)
	}
	broken, impacted := subjects(res)
	if want := []string{"live/app"}; !reflect.DeepEqual(broken, want) {
		t.Errorf("Broken = %v, want %v", broken, want)
	}
	if want := []string{"live/db", "live/vpc"}; !reflect.DeepEqual(impacted, want) {
		t.Errorf("Impacted = %v, want %v", impacted, want)
	}
	if !res.HasErrors() {
		t.Errorf("HasErrors = false, want true")
	}
}

func TestBlastNilBaseSkipsBaseline(t *testing.T) {
	var calls int
	cur := tree(t, "vpc_id", 6, []string{"vpc_id"}, &calls)
	if _, err := blasting.Blast(context.Background(), cur, nil); err != nil {
		t.Fatalf("Blast: %v", err)
	}
	if calls != 1 {
		t.Errorf("loader calls = %d, want 1", calls)
	}
}

func TestBlastStagedErrors(t *testing.T) {
	boom := errors.New("boom")
	good := tree(t, "vpc_id", 6, []string{"vpc_id"}, nil)
	bad := blasting.Sources{Units: fakeLoader{err: boom}, Surfaces: fakeSurfaces{t: t}}

	cases := []struct {
		name  string
		cur   blasting.Sources
		base  *blasting.Sources
		stage string
	}{
		{"current", bad, &good, "current"},
		{"current without base", bad, nil, "current"},
		{"baseline", good, &bad, "baseline"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res, err := blasting.Blast(context.Background(), tc.cur, tc.base)
			var be *blasting.Error
			if !errors.As(err, &be) {
				t.Fatalf("err = %v, want *blasting.Error", err)
			}
			if be.Stage != tc.stage {
				t.Errorf("Stage = %q, want %q", be.Stage, tc.stage)
			}
			if !errors.Is(err, boom) {
				t.Errorf("err = %v, want it to wrap boom", err)
			}
			if !reflect.DeepEqual(res, impact.Result{}) {
				t.Errorf("result = %v, want zero", res)
			}
		})
	}
}

func TestErrorMessageAndUnwrap(t *testing.T) {
	inner := errors.New("inner")
	e := &blasting.Error{Stage: "baseline", Err: inner}
	if e.Error() != "blasting: baseline: inner" {
		t.Errorf("Error() = %q", e.Error())
	}
	if !errors.Is(e, inner) {
		t.Errorf("Unwrap does not reach inner")
	}
}
