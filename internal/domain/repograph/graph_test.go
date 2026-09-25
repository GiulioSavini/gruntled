package repograph_test

import (
	"errors"
	"reflect"
	"testing"

	"github.com/GiulioSavini/gruntled/internal/domain/repograph"
)

// buildThreeUnitGraph constructs the reference fixture used across several
// tests: an "app" unit depending on "vpc" (a resolved unit/module pair) and
// on "legacy" (a config-unknown unit). unitOrder and moduleOrder let callers pass
// the same objects in different slice orders to prove order-independence.
func buildThreeUnitGraph(t *testing.T, unitOrder []int, moduleOrder []int) *repograph.RepositoryGraph {
	t.Helper()

	appPath := repograph.MustRepoPath("units/app")
	vpcPath := repograph.MustRepoPath("units/vpc")
	legacyPath := repograph.MustRepoPath("units/legacy")
	vpcModulePath := repograph.MustRepoPath("modules/vpc")
	appModulePath := repograph.MustRepoPath("modules/app")

	vpcSurface := mustSurface(t, nil, []string{"subnet_id"})
	appSurface := mustSurface(t, nil, nil)

	vpcModule := mustModule(t, vpcModulePath, vpcSurface)
	appModule := mustModule(t, appModulePath, appSurface)

	pos1 := mustPosition(t, appPath, 1, 1)
	pos2 := mustPosition(t, appPath, 5, 1)
	pos3 := mustPosition(t, appPath, 9, 1)

	depVPC := mustDependency(t, "vpc", vpcPath, pos1)
	depLegacy := mustDependency(t, "legacy", legacyPath, pos2)

	refGood := mustReference(t, "vpc", "subnet_id", pos1)
	refBad := mustReference(t, "vpc", "subnet_idz", pos2)
	refSkipped := mustReference(t, "legacy", "x", pos3)

	appUnit, err := repograph.NewResolvedUnit(appPath, appModulePath,
		[]repograph.Dependency{depVPC, depLegacy},
		[]repograph.Reference{refGood, refBad, refSkipped})
	if err != nil {
		t.Fatalf("NewResolvedUnit(app): unexpected error: %v", err)
	}

	vpcUnit, err := repograph.NewResolvedUnit(vpcPath, vpcModulePath, nil, nil)
	if err != nil {
		t.Fatalf("NewResolvedUnit(vpc): unexpected error: %v", err)
	}

	legacyUnit, err := repograph.NewConfigUnknownUnit(legacyPath, "remote source not fetched offline")
	if err != nil {
		t.Fatalf("NewConfigUnknownUnit(legacy): unexpected error: %v", err)
	}

	allUnits := []repograph.Unit{appUnit, vpcUnit, legacyUnit}
	allModules := []repograph.Module{appModule, vpcModule}

	units := make([]repograph.Unit, len(unitOrder))
	for i, idx := range unitOrder {
		units[i] = allUnits[idx]
	}
	modules := make([]repograph.Module, len(moduleOrder))
	for i, idx := range moduleOrder {
		modules[i] = allModules[idx]
	}

	g, err := repograph.NewRepositoryGraph(units, modules)
	if err != nil {
		t.Fatalf("NewRepositoryGraph: unexpected error: %v", err)
	}
	return g
}

func mustSurface(t *testing.T, variables, outputs []string) repograph.Surface {
	t.Helper()
	s, err := repograph.NewSurface(variables, outputs)
	if err != nil {
		t.Fatalf("NewSurface: unexpected error: %v", err)
	}
	return s
}

func mustModule(t *testing.T, path repograph.RepoPath, surface repograph.Surface) repograph.Module {
	t.Helper()
	m, err := repograph.NewModule(path, surface)
	if err != nil {
		t.Fatalf("NewModule: unexpected error: %v", err)
	}
	return m
}

func TestNewRepositoryGraph_OrderIndependent(t *testing.T) {
	g1 := buildThreeUnitGraph(t, []int{0, 1, 2}, []int{0, 1})
	g2 := buildThreeUnitGraph(t, []int{2, 0, 1}, []int{1, 0})

	if !reflect.DeepEqual(g1.Units(), g2.Units()) {
		t.Errorf("Units() differ by input order:\n g1=%+v\n g2=%+v", g1.Units(), g2.Units())
	}
	if !reflect.DeepEqual(g1.Modules(), g2.Modules()) {
		t.Errorf("Modules() differ by input order:\n g1=%+v\n g2=%+v", g1.Modules(), g2.Modules())
	}
	if !reflect.DeepEqual(g1.References(), g2.References()) {
		t.Errorf("References() differ by input order:\n g1=%+v\n g2=%+v", g1.References(), g2.References())
	}

	units := g1.Units()
	for i := 1; i < len(units); i++ {
		if units[i-1].Path().Compare(units[i].Path()) >= 0 {
			t.Errorf("Units() not sorted by path: %+v", units)
		}
	}
	modules := g1.Modules()
	for i := 1; i < len(modules); i++ {
		if modules[i-1].Path().Compare(modules[i].Path()) >= 0 {
			t.Errorf("Modules() not sorted by path: %+v", modules)
		}
	}
}

func TestNewRepositoryGraph_DuplicateUnit(t *testing.T) {
	p := repograph.MustRepoPath("units/a")
	u, err := repograph.NewConfigUnknownUnit(p, "reason")
	if err != nil {
		t.Fatalf("NewConfigUnknownUnit: unexpected error: %v", err)
	}

	_, err = repograph.NewRepositoryGraph([]repograph.Unit{u, u}, nil)
	if err == nil {
		t.Fatalf("NewRepositoryGraph with duplicate unit: expected error, got nil")
	}
	var dupErr *repograph.DuplicateUnitError
	if !errors.As(err, &dupErr) {
		t.Errorf("expected *DuplicateUnitError, got %T: %v", err, err)
	}
}

func TestNewRepositoryGraph_DuplicateModule(t *testing.T) {
	p := repograph.MustRepoPath("modules/a")
	surface := mustSurface(t, nil, nil)
	m := mustModule(t, p, surface)

	_, err := repograph.NewRepositoryGraph(nil, []repograph.Module{m, m})
	if err == nil {
		t.Fatalf("NewRepositoryGraph with duplicate module: expected error, got nil")
	}
	var dupErr *repograph.DuplicateModuleError
	if !errors.As(err, &dupErr) {
		t.Errorf("expected *DuplicateModuleError, got %T: %v", err, err)
	}
}

func TestNewRepositoryGraph_MissingModule(t *testing.T) {
	unitPath := repograph.MustRepoPath("units/app")
	modulePath := repograph.MustRepoPath("modules/app")
	u, err := repograph.NewResolvedUnit(unitPath, modulePath, nil, nil)
	if err != nil {
		t.Fatalf("NewResolvedUnit: unexpected error: %v", err)
	}

	_, err = repograph.NewRepositoryGraph([]repograph.Unit{u}, nil)
	if err == nil {
		t.Fatalf("NewRepositoryGraph with missing module: expected error, got nil")
	}
	var missingErr *repograph.MissingModuleError
	if !errors.As(err, &missingErr) {
		t.Errorf("expected *MissingModuleError, got %T: %v", err, err)
	}
}

func TestNewRepositoryGraph_ConfigUnknownUnitNeedsNoModule(t *testing.T) {
	p := repograph.MustRepoPath("units/legacy")
	u, err := repograph.NewConfigUnknownUnit(p, "reason")
	if err != nil {
		t.Fatalf("NewConfigUnknownUnit: unexpected error: %v", err)
	}

	g, err := repograph.NewRepositoryGraph([]repograph.Unit{u}, nil)
	if err != nil {
		t.Fatalf("NewRepositoryGraph: unexpected error: %v", err)
	}
	if len(g.Units()) != 1 {
		t.Errorf("Units() = %+v, want 1 unit", g.Units())
	}
}

func TestNewRepositoryGraph_ModuleUnknownUnitNeedsNoModule(t *testing.T) {
	p := repograph.MustRepoPath("units/app")
	u, err := repograph.NewModuleUnknownUnit(p, "remote-source", nil, nil)
	if err != nil {
		t.Fatalf("NewModuleUnknownUnit: unexpected error: %v", err)
	}

	g, err := repograph.NewRepositoryGraph([]repograph.Unit{u}, nil)
	if err != nil {
		t.Fatalf("NewRepositoryGraph: unexpected error: %v", err)
	}
	if len(g.Units()) != 1 {
		t.Errorf("Units() = %+v, want 1 unit", g.Units())
	}
}

func TestNewRepositoryGraph_ResolvedUnitWithUnknownSurfaceModule(t *testing.T) {
	unitPath := repograph.MustRepoPath("units/app")
	modulePath := repograph.MustRepoPath("modules/app")
	u, err := repograph.NewResolvedUnit(unitPath, modulePath, nil, nil)
	if err != nil {
		t.Fatalf("NewResolvedUnit: unexpected error: %v", err)
	}
	m, err := repograph.NewUnknownModule(modulePath, "no-terraform-files")
	if err != nil {
		t.Fatalf("NewUnknownModule: unexpected error: %v", err)
	}

	g, err := repograph.NewRepositoryGraph([]repograph.Unit{u}, []repograph.Module{m})
	if err != nil {
		t.Fatalf("NewRepositoryGraph: unexpected error: %v", err)
	}
	mod, ok := g.ModuleOf(unitPath)
	if !ok {
		t.Fatalf("ModuleOf(app) ok = false, want true")
	}
	if _, surfaceOK := mod.Surface(); surfaceOK {
		t.Errorf("ModuleOf(app).Surface() ok = true, want false (unknown surface)")
	}
}

func TestNewRepositoryGraph_CycleAndSelfDependencyBuildWithoutHang(t *testing.T) {
	aPath := repograph.MustRepoPath("units/a")
	bPath := repograph.MustRepoPath("units/b")
	modPath := repograph.MustRepoPath("modules/m")
	pos := mustPosition(t, aPath, 1, 1)

	depB := mustDependency(t, "b", bPath, pos)
	depA := mustDependency(t, "a", aPath, pos)
	depSelf := mustDependency(t, "self", aPath, pos)

	a, err := repograph.NewResolvedUnit(aPath, modPath, []repograph.Dependency{depB, depSelf}, nil)
	if err != nil {
		t.Fatalf("NewResolvedUnit(a): unexpected error: %v", err)
	}
	b, err := repograph.NewResolvedUnit(bPath, modPath, []repograph.Dependency{depA}, nil)
	if err != nil {
		t.Fatalf("NewResolvedUnit(b): unexpected error: %v", err)
	}
	m := mustModule(t, modPath, mustSurface(t, nil, nil))

	g, err := repograph.NewRepositoryGraph([]repograph.Unit{a, b}, []repograph.Module{m})
	if err != nil {
		t.Fatalf("NewRepositoryGraph: unexpected error: %v", err)
	}

	target, ok := g.DependencyTarget(aPath, "b")
	if !ok || target.Path() != bPath {
		t.Errorf("DependencyTarget(a, b) = (%+v, %v), want unit %v, true", target, ok, bPath)
	}
	target, ok = g.DependencyTarget(bPath, "a")
	if !ok || target.Path() != aPath {
		t.Errorf("DependencyTarget(b, a) = (%+v, %v), want unit %v, true", target, ok, aPath)
	}
	self, ok := g.DependencyTarget(aPath, "self")
	if !ok || self.Path() != aPath {
		t.Errorf("DependencyTarget(a, self) = (%+v, %v), want unit %v, true", self, ok, aPath)
	}
}

func TestNewRepositoryGraph_UnresolvableDependencyTargetIsNotAnError(t *testing.T) {
	unitPath := repograph.MustRepoPath("units/app")
	modulePath := repograph.MustRepoPath("modules/app")
	pos := mustPosition(t, unitPath, 1, 1)
	dep := mustDependency(t, "ghost", repograph.MustRepoPath("units/ghost"), pos)

	u, err := repograph.NewResolvedUnit(unitPath, modulePath, []repograph.Dependency{dep}, nil)
	if err != nil {
		t.Fatalf("NewResolvedUnit: unexpected error: %v", err)
	}
	m := mustModule(t, modulePath, mustSurface(t, nil, nil))

	g, err := repograph.NewRepositoryGraph([]repograph.Unit{u}, []repograph.Module{m})
	if err != nil {
		t.Fatalf("NewRepositoryGraph: unexpected error: %v", err)
	}

	if _, ok := g.DependencyTarget(unitPath, "ghost"); ok {
		t.Errorf("DependencyTarget for a target absent from the graph: got ok=true, want false")
	}
}

func TestRepositoryGraph_ModuleOf(t *testing.T) {
	g := buildThreeUnitGraph(t, []int{0, 1, 2}, []int{0, 1})

	vpcPath := repograph.MustRepoPath("units/vpc")
	vpcModulePath := repograph.MustRepoPath("modules/vpc")
	mod, ok := g.ModuleOf(vpcPath)
	if !ok || mod.Path() != vpcModulePath {
		t.Errorf("ModuleOf(vpc) = (%+v, %v), want module %v, true", mod, ok, vpcModulePath)
	}

	legacyPath := repograph.MustRepoPath("units/legacy")
	if _, ok := g.ModuleOf(legacyPath); ok {
		t.Errorf("ModuleOf(legacy) ok = true, want false (Unknown unit)")
	}

	absentPath := repograph.MustRepoPath("units/absent")
	if _, ok := g.ModuleOf(absentPath); ok {
		t.Errorf("ModuleOf(absent) ok = true, want false (unit not in graph)")
	}
}

func TestRepositoryGraph_DependencyTarget(t *testing.T) {
	g := buildThreeUnitGraph(t, []int{0, 1, 2}, []int{0, 1})

	appPath := repograph.MustRepoPath("units/app")
	vpcPath := repograph.MustRepoPath("units/vpc")

	target, ok := g.DependencyTarget(appPath, "vpc")
	if !ok || target.Path() != vpcPath {
		t.Errorf("DependencyTarget(app, vpc) = (%+v, %v), want unit %v, true", target, ok, vpcPath)
	}

	if _, ok := g.DependencyTarget(appPath, "nope"); ok {
		t.Errorf("DependencyTarget(app, nope) ok = true, want false")
	}
}

func TestRepositoryGraph_ReturnedSlicesAreDefensiveCopies(t *testing.T) {
	g := buildThreeUnitGraph(t, []int{0, 1, 2}, []int{0, 1})

	units := g.Units()
	originalFirstPath := units[0].Path()
	units[0] = units[1]
	if g.Units()[0].Path() != originalFirstPath {
		t.Errorf("mutating Units() result affected the graph")
	}

	refs := g.References()
	if len(refs) > 0 {
		original := refs[0]
		refs[0] = refs[len(refs)-1]
		if g.References()[0] != original {
			t.Errorf("mutating References() result affected the graph")
		}
	}
}

// unknownOutputRefs is the ARCH-02 proof: it uses only graph queries
// (References -> DependencyTarget -> ModuleOf -> Surface().HasOutput) to
// find references to outputs that don't exist on their target module. It
// intentionally stays test-local: the real GRT001 analyzer belongs to
// internal/domain/analysis in Phase 3.
func unknownOutputRefs(g *repograph.RepositoryGraph) []repograph.UnitReference {
	var bad []repograph.UnitReference
	for _, ur := range g.References() {
		unit, ok := g.Unit(ur.Unit)
		if !ok || unit.Status() != repograph.StatusResolved {
			continue
		}
		target, ok := g.DependencyTarget(ur.Unit, ur.Reference.Dependency())
		if !ok {
			continue
		}
		mod, ok := g.ModuleOf(target.Path())
		if !ok {
			continue
		}
		surface, ok := mod.Surface()
		if !ok {
			continue
		}
		if !surface.HasOutput(ur.Reference.Output()) {
			bad = append(bad, ur)
		}
	}
	return bad
}

func TestARCH02_UnknownOutputRefsOverHandBuiltGraph(t *testing.T) {
	g := buildThreeUnitGraph(t, []int{0, 1, 2}, []int{0, 1})

	bad := unknownOutputRefs(g)
	if len(bad) != 1 {
		t.Fatalf("unknownOutputRefs() = %+v, want exactly 1 bad reference", bad)
	}

	appPath := repograph.MustRepoPath("units/app")
	if bad[0].Unit != appPath {
		t.Errorf("bad reference unit = %v, want %v", bad[0].Unit, appPath)
	}
	if bad[0].Reference.Dependency() != "vpc" || bad[0].Reference.Output() != "subnet_idz" {
		t.Errorf("bad reference = %+v, want dependency=vpc output=subnet_idz", bad[0].Reference)
	}
}
