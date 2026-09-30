package indexing_test

import (
	"context"
	"errors"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/GiulioSavini/gruntled/internal/application/indexing"
	"github.com/GiulioSavini/gruntled/internal/application/ports"
	"github.com/GiulioSavini/gruntled/internal/domain/diagnostic"
	"github.com/GiulioSavini/gruntled/internal/domain/repograph"
)

// fakeLoader is a hand-written fake ports.UnitLoader: no filesystem.
type fakeLoader struct {
	res ports.LoadResult
	err error
}

func (f fakeLoader) LoadUnits(ctx context.Context) (ports.LoadResult, error) {
	return f.res, f.err
}

// fakeSurfaces is a hand-written fake ports.SurfaceReader: no filesystem.
// It records every call, in order, and fails the test if Build asks for a
// module path it was not told about.
type fakeSurfaces struct {
	t         *testing.T
	bySurface map[string]ports.SurfaceResult
	calls     []string
	err       error
}

func (f *fakeSurfaces) ReadSurface(ctx context.Context, module repograph.RepoPath) (ports.SurfaceResult, error) {
	f.calls = append(f.calls, module.String())
	if f.err != nil {
		return ports.SurfaceResult{}, f.err
	}
	res, ok := f.bySurface[module.String()]
	if !ok {
		f.t.Fatalf("fakeSurfaces.ReadSurface: no entry for module %q", module.String())
	}
	return res, nil
}

func mustPath(t *testing.T, s string) repograph.RepoPath {
	t.Helper()
	p, err := repograph.NewRepoPath(s)
	if err != nil {
		t.Fatalf("NewRepoPath(%q): %v", s, err)
	}
	return p
}

func mustPos(t *testing.T, file string, line, col int) repograph.Position {
	t.Helper()
	pos, err := repograph.NewPosition(mustPath(t, file), line, col)
	if err != nil {
		t.Fatalf("NewPosition: %v", err)
	}
	return pos
}

func mustSurface(t *testing.T, variables, outputs []string) repograph.Surface {
	t.Helper()
	s, err := repograph.NewSurface(variables, outputs)
	if err != nil {
		t.Fatalf("NewSurface: %v", err)
	}
	return s
}

// dump renders a Result as a canonical, deterministic string for
// determinism assertions: two Results built from differently ordered
// input must render identically.
func dump(r indexing.Result) string {
	var b strings.Builder

	b.WriteString("units:\n")
	for _, u := range r.Graph.Units() {
		b.WriteString("  ")
		b.WriteString(u.Path().String())
		b.WriteString(" status=")
		b.WriteString(u.Status().String())
		b.WriteString(" reason=")
		b.WriteString(strconv.Quote(u.UnknownReason()))
		if mod, ok := u.Module(); ok {
			b.WriteString(" module=")
			b.WriteString(mod.String())
		}
		b.WriteString(" deps=[")
		for i, d := range u.Dependencies() {
			if i > 0 {
				b.WriteString(",")
			}
			b.WriteString(d.Name())
			b.WriteString("=")
			if target, ok := d.Target(); ok {
				b.WriteString(target.String())
			} else {
				b.WriteString("unresolved:" + d.UnresolvedReason())
			}
		}
		b.WriteString("] refs=[")
		for i, ref := range u.References() {
			if i > 0 {
				b.WriteString(",")
			}
			b.WriteString(ref.Dependency())
			b.WriteString(".")
			b.WriteString(ref.Output())
			b.WriteString("@")
			b.WriteString(ref.Pos().String())
		}
		b.WriteString("]\n")
	}

	b.WriteString("modules:\n")
	for _, m := range r.Graph.Modules() {
		b.WriteString("  ")
		b.WriteString(m.Path().String())
		if surf, ok := m.Surface(); ok {
			b.WriteString(" known outputs=")
			b.WriteString(strings.Join(surf.Outputs(), ","))
		} else {
			b.WriteString(" unknown reason=")
			b.WriteString(strconv.Quote(m.UnknownReason()))
		}
		b.WriteString("\n")
	}

	b.WriteString("diagnostics:\n")
	for _, d := range r.Diagnostics.All() {
		k := d.Key()
		b.WriteString("  ")
		b.WriteString(string(k.Code))
		b.WriteString(" unit=")
		b.WriteString(k.Unit.String())
		b.WriteString(" ")
		b.WriteString(k.File)
		b.WriteString(":")
		b.WriteString(strconv.Itoa(k.Line))
		b.WriteString(":")
		b.WriteString(strconv.Itoa(k.Column))
		b.WriteString(" ")
		b.WriteString(k.Message)
		b.WriteString("\n")
	}

	return b.String()
}

// --- state mapping (config-unknown / module-unknown / precedence) ---------

func TestBuild_UnitStateMapping(t *testing.T) {
	unitPath := mustPath(t, "live/app")
	depPos := mustPos(t, "live/app/terragrunt.hcl", 1, 1)
	dep, err := repograph.NewDependency("vpc", mustPath(t, "live/vpc"), depPos, depPos, repograph.TargetUnknown, repograph.DependencyOptions{})
	if err != nil {
		t.Fatalf("NewDependency: %v", err)
	}
	refPos := mustPos(t, "live/app/terragrunt.hcl", 2, 1)
	ref, err := repograph.NewReference("vpc", "id", refPos)
	if err != nil {
		t.Fatalf("NewReference: %v", err)
	}

	tests := []struct {
		name       string
		uc         ports.UnitConfig
		wantStatus repograph.UnitStatus
		wantReason string
		wantDeps   int
		wantRefs   int
	}{
		{
			name: "config-unknown carries no deps or refs even if the DTO has them",
			uc: ports.UnitConfig{
				Path:                unitPath,
				ConfigUnknownReason: "broken-hcl",
				Dependencies:        []repograph.Dependency{dep},
				References:          []repograph.Reference{ref},
			},
			wantStatus: repograph.StatusConfigUnknown,
			wantReason: "broken-hcl",
		},
		{
			name: "both reasons set: config-unknown wins",
			uc: ports.UnitConfig{
				Path:                unitPath,
				ConfigUnknownReason: "broken-hcl",
				ModuleUnknownReason: "remote-source",
				Dependencies:        []repograph.Dependency{dep},
			},
			wantStatus: repograph.StatusConfigUnknown,
			wantReason: "broken-hcl",
		},
		{
			name: "module-unknown keeps deps and refs",
			uc: ports.UnitConfig{
				Path:                unitPath,
				ModuleUnknownReason: "remote-source",
				Dependencies:        []repograph.Dependency{dep},
				References:          []repograph.Reference{ref},
			},
			wantStatus: repograph.StatusModuleUnknown,
			wantReason: "remote-source",
			wantDeps:   1,
			wantRefs:   1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			loader := fakeLoader{res: ports.LoadResult{Units: []ports.UnitConfig{tt.uc}}}
			surfaces := &fakeSurfaces{t: t, bySurface: map[string]ports.SurfaceResult{}}

			result, err := indexing.Build(context.Background(), loader, surfaces)
			if err != nil {
				t.Fatalf("Build: %v", err)
			}

			u, ok := result.Graph.Unit(unitPath)
			if !ok {
				t.Fatalf("unit %q not in graph", unitPath)
			}
			if u.Status() != tt.wantStatus {
				t.Errorf("Status = %v, want %v", u.Status(), tt.wantStatus)
			}
			if u.UnknownReason() != tt.wantReason {
				t.Errorf("UnknownReason = %q, want %q", u.UnknownReason(), tt.wantReason)
			}
			if len(u.Dependencies()) != tt.wantDeps {
				t.Errorf("len(Dependencies) = %d, want %d", len(u.Dependencies()), tt.wantDeps)
			}
			if len(u.References()) != tt.wantRefs {
				t.Errorf("len(References) = %d, want %d", len(u.References()), tt.wantRefs)
			}
			if len(surfaces.calls) != 0 {
				t.Errorf("ReadSurface called %v, want none", surfaces.calls)
			}
		})
	}
}

// --- module surfaces: read once, in sorted order ---------------------------

func TestBuild_DistinctModulesReadOnceInSortedOrder(t *testing.T) {
	unitA := mustPath(t, "live/a")
	unitB := mustPath(t, "live/b")
	unitC := mustPath(t, "live/c")
	modZeta := mustPath(t, "modules/zeta")
	modAlpha := mustPath(t, "modules/alpha")

	units := []ports.UnitConfig{
		{Path: unitA, Module: modZeta},
		{Path: unitB, Module: modAlpha},
		{Path: unitC, Module: modZeta}, // shares modZeta with unitA
	}
	loader := fakeLoader{res: ports.LoadResult{Units: units}}
	surfaces := &fakeSurfaces{t: t, bySurface: map[string]ports.SurfaceResult{
		modZeta.String():  {Surface: mustSurface(t, nil, nil)},
		modAlpha.String(): {Surface: mustSurface(t, nil, nil)},
	}}

	if _, err := indexing.Build(context.Background(), loader, surfaces); err != nil {
		t.Fatalf("Build: %v", err)
	}

	want := []string{modAlpha.String(), modZeta.String()}
	if !reflect.DeepEqual(surfaces.calls, want) {
		t.Errorf("calls = %v, want %v", surfaces.calls, want)
	}
}

// --- unknown surface keeps the module, keeps the unit resolved -------------

func TestBuild_UnknownSurfaceKeepsModuleAndUnitResolved(t *testing.T) {
	unitPath := mustPath(t, "live/app")
	modPath := mustPath(t, "modules/app")
	units := []ports.UnitConfig{{Path: unitPath, Module: modPath}}
	loader := fakeLoader{res: ports.LoadResult{Units: units}}
	surfaces := &fakeSurfaces{t: t, bySurface: map[string]ports.SurfaceResult{
		modPath.String(): {UnknownReason: "module-dir-missing"},
	}}

	result, err := indexing.Build(context.Background(), loader, surfaces)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}

	u, ok := result.Graph.Unit(unitPath)
	if !ok {
		t.Fatalf("unit %q not in graph", unitPath)
	}
	if u.Status() != repograph.StatusResolved {
		t.Errorf("Status = %v, want StatusResolved", u.Status())
	}

	mod, ok := result.Graph.ModuleOf(unitPath)
	if !ok {
		t.Fatalf("ModuleOf(%q) ok = false", unitPath)
	}
	if _, ok := mod.Surface(); ok {
		t.Errorf("Surface() ok = true, want false")
	}
	if mod.UnknownReason() != "module-dir-missing" {
		t.Errorf("UnknownReason = %q, want %q", mod.UnknownReason(), "module-dir-missing")
	}
}

// --- two-hop traversal over fakes -------------------------------------------

func TestBuild_TwoHopTraversal(t *testing.T) {
	appPath := mustPath(t, "live/app")
	vpcUnitPath := mustPath(t, "live/vpc")
	vpcModPath := mustPath(t, "modules/vpc")

	depPos := mustPos(t, "live/app/terragrunt.hcl", 3, 1)
	dep, err := repograph.NewDependency("vpc", vpcUnitPath, depPos, depPos, repograph.TargetUnknown, repograph.DependencyOptions{})
	if err != nil {
		t.Fatalf("NewDependency: %v", err)
	}

	units := []ports.UnitConfig{
		{Path: appPath, Module: appPath, Dependencies: []repograph.Dependency{dep}},
		{Path: vpcUnitPath, Module: vpcModPath},
	}
	loader := fakeLoader{res: ports.LoadResult{Units: units}}
	surfaces := &fakeSurfaces{t: t, bySurface: map[string]ports.SurfaceResult{
		appPath.String():    {Surface: mustSurface(t, nil, nil)},
		vpcModPath.String(): {Surface: mustSurface(t, nil, []string{"id"})},
	}}

	result, err := indexing.Build(context.Background(), loader, surfaces)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}

	target, ok := result.Graph.DependencyTarget(appPath, "vpc")
	if !ok {
		t.Fatalf("DependencyTarget(%q, vpc) ok = false", appPath)
	}
	if target.Path() != vpcUnitPath {
		t.Errorf("target path = %v, want %v", target.Path(), vpcUnitPath)
	}

	mod, ok := result.Graph.ModuleOf(vpcUnitPath)
	if !ok {
		t.Fatalf("ModuleOf(%q) ok = false", vpcUnitPath)
	}
	surf, ok := mod.Surface()
	if !ok {
		t.Fatalf("Surface() ok = false")
	}
	if !surf.HasOutput("id") {
		t.Errorf("HasOutput(id) = false")
	}
}

// --- unresolved dependency is kept, not dropped -----------------------------

func TestBuild_UnresolvedDependencyKept(t *testing.T) {
	unitPath := mustPath(t, "live/app")
	pos := mustPos(t, "live/app/terragrunt.hcl", 4, 1)
	dep, err := repograph.NewUnresolvedDependency("vpc", "config-path-dynamic", pos, pos, repograph.DependencyOptions{})
	if err != nil {
		t.Fatalf("NewUnresolvedDependency: %v", err)
	}

	units := []ports.UnitConfig{
		{Path: unitPath, Module: unitPath, Dependencies: []repograph.Dependency{dep}},
	}
	loader := fakeLoader{res: ports.LoadResult{Units: units}}
	surfaces := &fakeSurfaces{t: t, bySurface: map[string]ports.SurfaceResult{
		unitPath.String(): {Surface: mustSurface(t, nil, nil)},
	}}

	result, err := indexing.Build(context.Background(), loader, surfaces)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}

	if _, ok := result.Graph.DependencyTarget(unitPath, "vpc"); ok {
		t.Errorf("DependencyTarget ok = true, want false")
	}

	u, ok := result.Graph.Unit(unitPath)
	if !ok {
		t.Fatalf("unit %q not in graph", unitPath)
	}
	d, ok := u.Dependency("vpc")
	if !ok {
		t.Fatalf("Dependency(vpc) missing")
	}
	if _, ok := d.Target(); ok {
		t.Errorf("Target ok = true, want false")
	}
	if d.UnresolvedReason() != "config-path-dynamic" {
		t.Errorf("UnresolvedReason = %q, want %q", d.UnresolvedReason(), "config-path-dynamic")
	}
}

// --- resolved dependency whose target is not a discovered unit -------------

func TestBuild_DependencyTargetNotAUnit(t *testing.T) {
	unitPath := mustPath(t, "live/app")
	missingTarget := mustPath(t, "live/missing")
	pos := mustPos(t, "live/app/terragrunt.hcl", 5, 1)
	dep, err := repograph.NewDependency("missing", missingTarget, pos, pos, repograph.TargetUnknown, repograph.DependencyOptions{})
	if err != nil {
		t.Fatalf("NewDependency: %v", err)
	}

	units := []ports.UnitConfig{
		{Path: unitPath, Module: unitPath, Dependencies: []repograph.Dependency{dep}},
	}
	loader := fakeLoader{res: ports.LoadResult{Units: units}}
	surfaces := &fakeSurfaces{t: t, bySurface: map[string]ports.SurfaceResult{
		unitPath.String(): {Surface: mustSurface(t, nil, nil)},
	}}

	result, err := indexing.Build(context.Background(), loader, surfaces)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}

	if _, ok := result.Graph.DependencyTarget(unitPath, "missing"); ok {
		t.Errorf("DependencyTarget ok = true, want false")
	}
	u, ok := result.Graph.Unit(unitPath)
	if !ok {
		t.Fatalf("unit %q not in graph", unitPath)
	}
	d, ok := u.Dependency("missing")
	if !ok {
		t.Fatalf("Dependency(missing) missing")
	}
	target, ok := d.Target()
	if !ok {
		t.Fatalf("Target ok = false, want true (dependency itself resolved)")
	}
	if target != missingTarget {
		t.Errorf("Target = %v, want %v", target, missingTarget)
	}
}

// --- diagnostics from both ports collapse into one canonical Set -----------

func TestBuild_DiagnosticsCanonicalized(t *testing.T) {
	unitPath := mustPath(t, "live/app")
	units := []ports.UnitConfig{{Path: unitPath, Module: unitPath}}

	posA := mustPos(t, "live/app/terragrunt.hcl", 1, 1)
	dA, err := diagnostic.New(diagnostic.CodeSyntaxError, diagnostic.SeverityError, posA, "broken include")
	if err != nil {
		t.Fatalf("diagnostic.New: %v", err)
	}
	posB := mustPos(t, "modules/app/main.tf", 2, 1)
	dB, err := diagnostic.New(diagnostic.CodeSyntaxError, diagnostic.SeverityError, posB, "broken module file")
	if err != nil {
		t.Fatalf("diagnostic.New: %v", err)
	}

	loader := fakeLoader{res: ports.LoadResult{Units: units, Diagnostics: []diagnostic.Diagnostic{dA}}}
	surfaces := &fakeSurfaces{t: t, bySurface: map[string]ports.SurfaceResult{
		unitPath.String(): {Surface: mustSurface(t, nil, nil), Diagnostics: []diagnostic.Diagnostic{dB}},
	}}

	result, err := indexing.Build(context.Background(), loader, surfaces)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if result.Diagnostics.Len() != 2 {
		t.Fatalf("Len = %d, want 2", result.Diagnostics.Len())
	}
	want := diagnostic.NewSet(dA, dB)
	if !result.Diagnostics.Equal(want) {
		t.Errorf("Diagnostics = %v, want %v", result.Diagnostics.All(), want.All())
	}
}

func TestBuild_DiagnosticsOrderIndependentOfInputOrder(t *testing.T) {
	unitPath := mustPath(t, "live/app")
	units := []ports.UnitConfig{{Path: unitPath, Module: unitPath}}

	pos1 := mustPos(t, "live/app/terragrunt.hcl", 1, 1)
	d1, err := diagnostic.New(diagnostic.CodeSyntaxError, diagnostic.SeverityError, pos1, "first")
	if err != nil {
		t.Fatalf("diagnostic.New: %v", err)
	}
	pos2 := mustPos(t, "live/app/terragrunt.hcl", 2, 1)
	d2, err := diagnostic.New(diagnostic.CodeSyntaxError, diagnostic.SeverityError, pos2, "second")
	if err != nil {
		t.Fatalf("diagnostic.New: %v", err)
	}

	build := func(diags []diagnostic.Diagnostic) []diagnostic.Diagnostic {
		loader := fakeLoader{res: ports.LoadResult{Units: units, Diagnostics: diags}}
		surfaces := &fakeSurfaces{t: t, bySurface: map[string]ports.SurfaceResult{
			unitPath.String(): {Surface: mustSurface(t, nil, nil)},
		}}
		result, err := indexing.Build(context.Background(), loader, surfaces)
		if err != nil {
			t.Fatalf("Build: %v", err)
		}
		return result.Diagnostics.All()
	}

	order1 := build([]diagnostic.Diagnostic{d1, d2})
	order2 := build([]diagnostic.Diagnostic{d2, d1})
	if !reflect.DeepEqual(order1, order2) {
		t.Errorf("order1 = %v, order2 = %v", order1, order2)
	}
}

// --- determinism regardless of unit input order -----------------------------

func TestBuild_DeterministicRegardlessOfInputOrder(t *testing.T) {
	unitA := mustPath(t, "live/a")
	unitB := mustPath(t, "live/b")
	modPath := mustPath(t, "modules/shared")

	posA := mustPos(t, "live/a/terragrunt.hcl", 1, 1)
	depA, err := repograph.NewDependency("b", unitB, posA, posA, repograph.TargetUnknown, repograph.DependencyOptions{})
	if err != nil {
		t.Fatalf("NewDependency: %v", err)
	}

	unitsOrder1 := []ports.UnitConfig{
		{Path: unitA, Module: modPath, Dependencies: []repograph.Dependency{depA}},
		{Path: unitB, Module: modPath},
	}
	unitsOrder2 := []ports.UnitConfig{
		{Path: unitB, Module: modPath},
		{Path: unitA, Module: modPath, Dependencies: []repograph.Dependency{depA}},
	}

	buildAndDump := func(units []ports.UnitConfig) string {
		loader := fakeLoader{res: ports.LoadResult{Units: units}}
		surfaces := &fakeSurfaces{t: t, bySurface: map[string]ports.SurfaceResult{
			modPath.String(): {Surface: mustSurface(t, nil, []string{"id"})},
		}}
		result, err := indexing.Build(context.Background(), loader, surfaces)
		if err != nil {
			t.Fatalf("Build: %v", err)
		}
		return dump(result)
	}

	d1 := buildAndDump(unitsOrder1)
	d2 := buildAndDump(unitsOrder2)
	if d1 != d2 {
		t.Errorf("dump mismatch:\n%s\n---\n%s", d1, d2)
	}
}

// --- error stages ------------------------------------------------------------

func TestBuild_LoadUnitsError(t *testing.T) {
	cause := errors.New("boom")
	loader := fakeLoader{err: cause}
	surfaces := &fakeSurfaces{t: t, bySurface: map[string]ports.SurfaceResult{}}

	_, err := indexing.Build(context.Background(), loader, surfaces)
	if err == nil {
		t.Fatalf("err = nil, want error")
	}
	var ie *indexing.Error
	if !errors.As(err, &ie) {
		t.Fatalf("errors.As(err, *indexing.Error) failed: %v", err)
	}
	if ie.Stage != "load-units" {
		t.Errorf("Stage = %q, want %q", ie.Stage, "load-units")
	}
	if !errors.Is(err, cause) {
		t.Errorf("errors.Is(err, cause) = false")
	}
}

func TestBuild_ReadSurfaceError(t *testing.T) {
	unitPath := mustPath(t, "live/app")
	modPath := mustPath(t, "modules/app")
	units := []ports.UnitConfig{{Path: unitPath, Module: modPath}}
	cause := errors.New("boom")

	loader := fakeLoader{res: ports.LoadResult{Units: units}}
	surfaces := &fakeSurfaces{t: t, bySurface: map[string]ports.SurfaceResult{}, err: cause}

	_, err := indexing.Build(context.Background(), loader, surfaces)
	if err == nil {
		t.Fatalf("err = nil, want error")
	}
	var ie *indexing.Error
	if !errors.As(err, &ie) {
		t.Fatalf("errors.As(err, *indexing.Error) failed: %v", err)
	}
	if ie.Stage != "read-surface" {
		t.Errorf("Stage = %q, want %q", ie.Stage, "read-surface")
	}
	if ie.Path != modPath.String() {
		t.Errorf("Path = %q, want %q", ie.Path, modPath.String())
	}
	if !errors.Is(err, cause) {
		t.Errorf("errors.Is(err, cause) = false")
	}
}

func TestBuild_AssembleErrorOnZeroModule(t *testing.T) {
	unitPath := mustPath(t, "live/app")
	// A "resolved" UnitConfig (no ConfigUnknownReason, no ModuleUnknownReason)
	// with a zero Module: the domain constructor rejects it.
	units := []ports.UnitConfig{{Path: unitPath}}
	loader := fakeLoader{res: ports.LoadResult{Units: units}}
	surfaces := &fakeSurfaces{t: t, bySurface: map[string]ports.SurfaceResult{}}

	_, err := indexing.Build(context.Background(), loader, surfaces)
	if err == nil {
		t.Fatalf("err = nil, want error")
	}
	var ie *indexing.Error
	if !errors.As(err, &ie) {
		t.Fatalf("errors.As(err, *indexing.Error) failed: %v", err)
	}
	if ie.Stage != "assemble" {
		t.Errorf("Stage = %q, want %q", ie.Stage, "assemble")
	}
	if ie.Path != unitPath.String() {
		t.Errorf("Path = %q, want %q", ie.Path, unitPath.String())
	}
	if len(surfaces.calls) != 0 {
		t.Errorf("ReadSurface called %v, want none", surfaces.calls)
	}
}

func TestBuild_CancelledContextBeforeLoadUnits(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	loader := fakeLoader{res: ports.LoadResult{}}
	surfaces := &fakeSurfaces{t: t, bySurface: map[string]ports.SurfaceResult{}}

	_, err := indexing.Build(ctx, loader, surfaces)
	if err == nil {
		t.Fatalf("err = nil, want error")
	}
	var ie *indexing.Error
	if !errors.As(err, &ie) {
		t.Fatalf("errors.As(err, *indexing.Error) failed: %v", err)
	}
	if ie.Stage != "load-units" {
		t.Errorf("Stage = %q, want %q", ie.Stage, "load-units")
	}
	if len(surfaces.calls) != 0 {
		t.Errorf("ReadSurface called %v, want none", surfaces.calls)
	}
}
