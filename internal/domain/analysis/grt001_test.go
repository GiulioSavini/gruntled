package analysis_test

import (
	"reflect"
	"strings"
	"testing"

	"github.com/GiulioSavini/gruntled/internal/domain/analysis"
	"github.com/GiulioSavini/gruntled/internal/domain/diagnostic"
	"github.com/GiulioSavini/gruntled/internal/domain/repograph"
)

const (
	plainMsg   = `dependency "vpc" output "vpc_idd" is not declared by module "live/vpc" (target unit "live/vpc")`
	maskSuffix = `; mock_outputs supplies it, so apply would silently use the mock value`
)

var (
	appPath    = repograph.MustRepoPath("live/app")
	appModule  = repograph.MustRepoPath("modules/app")
	vpcPath    = repograph.MustRepoPath("live/vpc")
	vpcDepFile = repograph.MustRepoPath("live/app/terragrunt.hcl")
)

func mustPos(t *testing.T, file repograph.RepoPath, line, col int) repograph.Position {
	t.Helper()
	p, err := repograph.NewPosition(file, line, col)
	if err != nil {
		t.Fatalf("NewPosition: %v", err)
	}
	return p
}

func mustNames(t *testing.T, names ...string) repograph.NameList {
	t.Helper()
	l, err := repograph.KnownNames(names)
	if err != nil {
		t.Fatalf("KnownNames: %v", err)
	}
	return l
}

func mustSurface(t *testing.T, outputs ...string) repograph.Surface {
	t.Helper()
	s, err := repograph.NewSurface(nil, outputs)
	if err != nil {
		t.Fatalf("NewSurface: %v", err)
	}
	return s
}

func mustRef(t *testing.T, dep, output string, pos repograph.Position) repograph.Reference {
	t.Helper()
	r, err := repograph.NewReference(dep, output, pos)
	if err != nil {
		t.Fatalf("NewReference: %v", err)
	}
	return r
}

func mustModule(t *testing.T, path repograph.RepoPath, s repograph.Surface) repograph.Module {
	t.Helper()
	m, err := repograph.NewModule(path, s)
	if err != nil {
		t.Fatalf("NewModule: %v", err)
	}
	return m
}

func mustResolvedUnit(t *testing.T, path, module repograph.RepoPath, deps []repograph.Dependency, refs []repograph.Reference) repograph.Unit {
	t.Helper()
	u, err := repograph.NewResolvedUnit(path, module, deps, refs)
	if err != nil {
		t.Fatalf("NewResolvedUnit: %v", err)
	}
	return u
}

func mustGraph(t *testing.T, units []repograph.Unit, modules []repograph.Module) *repograph.RepositoryGraph {
	t.Helper()
	g, err := repograph.NewRepositoryGraph(units, modules)
	if err != nil {
		t.Fatalf("NewRepositoryGraph: %v", err)
	}
	return g
}

// depKind selects how the referencing unit's "vpc" dependency is built.
type depKind int

const (
	depResolved   depKind = iota // points at live/vpc, a unit in the graph
	depUnresolved                // NewUnresolvedDependency
	depNotAUnit                  // points at a path that is not a unit
	depUndeclared                // no dependency block at all
)

// targetKind selects what the dependency's target looks like.
type targetKind int

const (
	targetResolved       targetKind = iota // resolved unit, module live/vpc with a known surface
	targetConfigUnknown                    // config-unknown unit
	targetModuleUnknown                    // module-unknown unit
	targetSurfaceUnknown                   // resolved unit whose module surface is unknown
)

type scenario struct {
	opts    repograph.DependencyOptions
	dep     depKind
	target  targetKind
	outputs []string // target module surface outputs (targetResolved only)
}

// buildScenario builds live/app referencing dependency.vpc.outputs.vpc_idd
// at live/app/terragrunt.hcl:10:5, plus whatever the scenario says the
// target is.
func buildScenario(t *testing.T, s scenario) *repograph.RepositoryGraph {
	t.Helper()
	depPos := mustPos(t, vpcDepFile, 1, 1)
	refPos := mustPos(t, vpcDepFile, 10, 5)

	var deps []repograph.Dependency
	switch s.dep {
	case depResolved, depNotAUnit:
		target := vpcPath
		if s.dep == depNotAUnit {
			target = repograph.MustRepoPath("live/nowhere")
		}
		d, err := repograph.NewDependency("vpc", target, depPos, depPos, repograph.TargetUnknown, s.opts)
		if err != nil {
			t.Fatalf("NewDependency: %v", err)
		}
		deps = append(deps, d)
	case depUnresolved:
		d, err := repograph.NewUnresolvedDependency("vpc", "config-path-not-literal", depPos, depPos, s.opts)
		if err != nil {
			t.Fatalf("NewUnresolvedDependency: %v", err)
		}
		deps = append(deps, d)
	case depUndeclared:
	}

	refs := []repograph.Reference{mustRef(t, "vpc", "vpc_idd", refPos)}
	units := []repograph.Unit{mustResolvedUnit(t, appPath, appModule, deps, refs)}
	modules := []repograph.Module{mustModule(t, appModule, mustSurface(t))}

	switch s.target {
	case targetResolved:
		units = append(units, mustResolvedUnit(t, vpcPath, vpcPath, nil, nil))
		modules = append(modules, mustModule(t, vpcPath, mustSurface(t, s.outputs...)))
	case targetConfigUnknown:
		u, err := repograph.NewConfigUnknownUnit(vpcPath, "config-syntax-error")
		if err != nil {
			t.Fatalf("NewConfigUnknownUnit: %v", err)
		}
		units = append(units, u)
	case targetModuleUnknown:
		u, err := repograph.NewModuleUnknownUnit(vpcPath, "source-remote", nil, nil)
		if err != nil {
			t.Fatalf("NewModuleUnknownUnit: %v", err)
		}
		units = append(units, u)
	case targetSurfaceUnknown:
		units = append(units, mustResolvedUnit(t, vpcPath, vpcPath, nil, nil))
		m, err := repograph.NewUnknownModule(vpcPath, "module-file-syntax-error")
		if err != nil {
			t.Fatalf("NewUnknownModule: %v", err)
		}
		modules = append(modules, m)
	}
	return mustGraph(t, units, modules)
}

// withOpts returns the Terragrunt defaults with f applied.
func withOpts(f func(o *repograph.DependencyOptions)) repograph.DependencyOptions {
	o := repograph.DefaultDependencyOptions()
	if f != nil {
		f(&o)
	}
	return o
}

// diag03Row is one row of the DIAG-03 decision-table test: the scenario,
// how many GRT001 UnknownOutputs must return, and whether the message
// carries the mock suffix. grt004_test.go reuses the same rows for the
// GRT004 parity matrix.
type diag03Row struct {
	name   string
	build  func(t *testing.T) scenario
	want   int
	suffix bool
}

// diag03Rows returns the 22 DIAG-03 rows. Every row references
// dependency.vpc.outputs.vpc_idd from live/app; the target module is
// live/vpc.
func diag03Rows() []diag03Row {
	allCmds := func(t *testing.T) repograph.NameList {
		return mustNames(t, "init", "plan", "apply", "destroy", "validate")
	}
	return []diag03Row{
		{name: "row1 undeclared label", build: func(t *testing.T) scenario {
			return scenario{opts: withOpts(nil), dep: depUndeclared, outputs: []string{"vpc_id"}}
		}},
		{name: "row2a enabled false", build: func(t *testing.T) scenario {
			return scenario{opts: withOpts(func(o *repograph.DependencyOptions) { o.Enabled = repograph.TristateFalse }), outputs: []string{"vpc_id"}}
		}},
		{name: "row2b enabled unknown", build: func(t *testing.T) scenario {
			return scenario{opts: withOpts(func(o *repograph.DependencyOptions) { o.Enabled = repograph.TristateUnknown }), outputs: []string{"vpc_id"}}
		}},
		{name: "row3a skip_outputs true", build: func(t *testing.T) scenario {
			return scenario{opts: withOpts(func(o *repograph.DependencyOptions) { o.SkipOutputs = repograph.TristateTrue }), outputs: []string{"vpc_id"}}
		}},
		{name: "row3b skip_outputs unknown", build: func(t *testing.T) scenario {
			return scenario{opts: withOpts(func(o *repograph.DependencyOptions) { o.SkipOutputs = repograph.TristateUnknown }), outputs: []string{"vpc_id"}}
		}},
		{name: "row4a unresolved dependency", build: func(t *testing.T) scenario {
			return scenario{opts: withOpts(nil), dep: depUnresolved, outputs: []string{"vpc_id"}}
		}},
		{name: "row4b target is not a unit", build: func(t *testing.T) scenario {
			return scenario{opts: withOpts(nil), dep: depNotAUnit, outputs: []string{"vpc_id"}}
		}},
		{name: "row5a target config-unknown", build: func(t *testing.T) scenario {
			return scenario{opts: withOpts(nil), target: targetConfigUnknown}
		}},
		{name: "row5b target module-unknown", build: func(t *testing.T) scenario {
			return scenario{opts: withOpts(nil), target: targetModuleUnknown}
		}},
		{name: "row5c target module surface unknown", build: func(t *testing.T) scenario {
			return scenario{opts: withOpts(nil), target: targetSurfaceUnknown}
		}},
		{name: "row6a output declared, no mocks", build: func(t *testing.T) scenario {
			return scenario{opts: withOpts(nil), outputs: []string{"vpc_id", "vpc_idd"}}
		}},
		{name: "row6b output declared, mocks cover it", build: func(t *testing.T) scenario {
			return scenario{opts: withOpts(func(o *repograph.DependencyOptions) {
				o.MockOutputs = mustNames(t, "vpc_idd")
				o.MockMergeWithState = repograph.TristateTrue
			}), outputs: []string{"vpc_idd"}}
		}},
		{name: "row7a missing output, no mocks", want: 1, build: func(t *testing.T) scenario {
			return scenario{opts: withOpts(nil), outputs: []string{"vpc_id"}}
		}},
		{name: "row7b corpus shape: mock covers output, merge_with_state true, apply allowed", want: 1, suffix: true, build: func(t *testing.T) scenario {
			return scenario{opts: withOpts(func(o *repograph.DependencyOptions) {
				o.MockOutputs = mustNames(t, "vpc_idd", "region")
				o.MockMergeWithState = repograph.TristateTrue
				o.MockAllowedCommands = allCmds(t)
			}), outputs: []string{"vpc_id"}}
		}},
		{name: "row7c issue-2163 shape: apply not allowed", want: 1, build: func(t *testing.T) scenario {
			return scenario{opts: withOpts(func(o *repograph.DependencyOptions) {
				o.MockOutputs = mustNames(t, "vpc_idd")
				o.MockMergeWithState = repograph.TristateTrue
				o.MockAllowedCommands = mustNames(t, "validate", "plan")
			}), outputs: []string{"vpc_id"}}
		}},
		{name: "row7d mock keys unknown", want: 1, build: func(t *testing.T) scenario {
			return scenario{opts: withOpts(func(o *repograph.DependencyOptions) {
				o.MockOutputs = repograph.UnknownNames()
				o.MockMergeWithState = repograph.TristateTrue
			}), outputs: []string{"vpc_id"}}
		}},
		{name: "row7e allowed commands known empty list", want: 1, suffix: true, build: func(t *testing.T) scenario {
			return scenario{opts: withOpts(func(o *repograph.DependencyOptions) {
				o.MockOutputs = mustNames(t, "vpc_idd")
				o.MockMergeWithState = repograph.TristateTrue
				o.MockAllowedCommands = mustNames(t)
			}), outputs: []string{"vpc_id"}}
		}},
		{name: "row7f allowed commands absent", want: 1, suffix: true, build: func(t *testing.T) scenario {
			return scenario{opts: withOpts(func(o *repograph.DependencyOptions) {
				o.MockOutputs = mustNames(t, "vpc_idd")
				o.MockMergeWithState = repograph.TristateTrue
				o.MockAllowedCommands = repograph.AbsentNames()
			}), outputs: []string{"vpc_id"}}
		}},
		{name: "row7g zero-output target module, merge false", want: 1, suffix: true, build: func(t *testing.T) scenario {
			return scenario{opts: withOpts(func(o *repograph.DependencyOptions) {
				o.MockOutputs = mustNames(t, "vpc_idd")
				o.MockMergeWithState = repograph.TristateFalse
				o.MockAllowedCommands = allCmds(t)
			}), outputs: nil}
		}},
		{name: "row7h merge unknown", want: 1, build: func(t *testing.T) scenario {
			return scenario{opts: withOpts(func(o *repograph.DependencyOptions) {
				o.MockOutputs = mustNames(t, "vpc_idd")
				o.MockMergeWithState = repograph.TristateUnknown
				o.MockAllowedCommands = allCmds(t)
			}), outputs: []string{"vpc_id"}}
		}},
		{name: "row7i allowed commands unknown", want: 1, build: func(t *testing.T) scenario {
			return scenario{opts: withOpts(func(o *repograph.DependencyOptions) {
				o.MockOutputs = mustNames(t, "vpc_idd")
				o.MockMergeWithState = repograph.TristateTrue
				o.MockAllowedCommands = repograph.UnknownNames()
			}), outputs: []string{"vpc_id"}}
		}},
		{name: "row7j mocks do not contain the output", want: 1, build: func(t *testing.T) scenario {
			return scenario{opts: withOpts(func(o *repograph.DependencyOptions) {
				o.MockOutputs = mustNames(t, "other")
				o.MockMergeWithState = repograph.TristateTrue
			}), outputs: []string{"vpc_id"}}
		}},
	}
}

// TestDIAG03RowCount notices a row lost while the table is shared.
func TestDIAG03RowCount(t *testing.T) {
	if n := len(diag03Rows()); n != 22 {
		t.Fatalf("diag03Rows() has %d rows, want 22", n)
	}
}

func TestDIAG03(t *testing.T) {
	for _, r := range diag03Rows() {
		t.Run(r.name, func(t *testing.T) {
			g := buildScenario(t, r.build(t))
			got, err := analysis.UnknownOutputs(g)
			if err != nil {
				t.Fatalf("UnknownOutputs: %v", err)
			}
			if len(got) != r.want {
				t.Fatalf("got %d diagnostics, want %d: %v", len(got), r.want, got)
			}
			if r.want == 0 {
				return
			}
			d := got[0]
			if d.Code() != diagnostic.CodeUnknownOutput {
				t.Errorf("code = %v, want GRT001", d.Code())
			}
			if d.Severity() != diagnostic.SeverityError {
				t.Errorf("severity = %v, want error (mocks never downgrade)", d.Severity())
			}
			if u, ok := d.Unit(); !ok || u != appPath {
				t.Errorf("unit = %v (%v), want live/app", u, ok)
			}
			if d.Pos() != mustPos(t, vpcDepFile, 10, 5) {
				t.Errorf("pos = %v, want live/app/terragrunt.hcl:10:5", d.Pos())
			}
			want := plainMsg
			if r.suffix {
				want += maskSuffix
			}
			if d.Message() != want {
				t.Errorf("message =\n  %s\nwant\n  %s", d.Message(), want)
			}
		})
	}
}

func TestUnknownOutputsModuleUnknownReferencingUnit(t *testing.T) {
	refPos := mustPos(t, vpcDepFile, 10, 5)
	dep, err := repograph.NewDependency("vpc", vpcPath, mustPos(t, vpcDepFile, 1, 1), mustPos(t, vpcDepFile, 1, 1), repograph.TargetUnknown, repograph.DefaultDependencyOptions())
	if err != nil {
		t.Fatalf("NewDependency: %v", err)
	}
	app, err := repograph.NewModuleUnknownUnit(appPath, "source-remote", []repograph.Dependency{dep}, []repograph.Reference{mustRef(t, "vpc", "vpc_idd", refPos)})
	if err != nil {
		t.Fatalf("NewModuleUnknownUnit: %v", err)
	}
	g := mustGraph(t,
		[]repograph.Unit{app, mustResolvedUnit(t, vpcPath, vpcPath, nil, nil)},
		[]repograph.Module{mustModule(t, vpcPath, mustSurface(t, "vpc_id"))})
	got, err := analysis.UnknownOutputs(g)
	if err != nil {
		t.Fatalf("UnknownOutputs: %v", err)
	}
	if len(got) != 1 || got[0].Message() != plainMsg {
		t.Fatalf("got %v, want one GRT001 with %q", got, plainMsg)
	}
}

// sharedIncludeUnits builds two units that carry the same reference at the
// same position (a shared include) but resolve "vpc" to different targets.
func sharedIncludeUnits(t *testing.T) ([]repograph.Unit, []repograph.Module) {
	t.Helper()
	inc := repograph.MustRepoPath("_common/vpc.hcl")
	refPos := mustPos(t, inc, 3, 7)
	depPos := mustPos(t, inc, 1, 1)
	good := repograph.MustRepoPath("live/good-vpc")
	bad := repograph.MustRepoPath("live/bad-vpc")
	a := repograph.MustRepoPath("live/a")
	b := repograph.MustRepoPath("live/b")
	mk := func(unit, target repograph.RepoPath) repograph.Unit {
		d, err := repograph.NewDependency("vpc", target, depPos, depPos, repograph.TargetUnknown, repograph.DefaultDependencyOptions())
		if err != nil {
			t.Fatalf("NewDependency: %v", err)
		}
		return mustResolvedUnit(t, unit, appModule, []repograph.Dependency{d}, []repograph.Reference{mustRef(t, "vpc", "vpc_id", refPos)})
	}
	units := []repograph.Unit{
		mk(a, good),
		mk(b, bad),
		mustResolvedUnit(t, good, good, nil, nil),
		mustResolvedUnit(t, bad, bad, nil, nil),
	}
	modules := []repograph.Module{
		mustModule(t, appModule, mustSurface(t)),
		mustModule(t, good, mustSurface(t, "vpc_id")),
		mustModule(t, bad, mustSurface(t, "other")),
	}
	return units, modules
}

func TestUnknownOutputsSharedInclude(t *testing.T) {
	units, modules := sharedIncludeUnits(t)
	got, err := analysis.UnknownOutputs(mustGraph(t, units, modules))
	if err != nil {
		t.Fatalf("UnknownOutputs: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d diagnostics, want 1: %v", len(got), got)
	}
	if u, _ := got[0].Unit(); u != repograph.MustRepoPath("live/b") {
		t.Errorf("unit = %v, want live/b", u)
	}
	if !strings.Contains(got[0].Message(), `module "live/bad-vpc"`) {
		t.Errorf("message %q does not name live/bad-vpc", got[0].Message())
	}
}

func TestUnknownOutputsOrderAndDeterminism(t *testing.T) {
	units, modules := sharedIncludeUnits(t)
	// Make both units report: live/a's target loses vpc_id too.
	modules[1] = mustModule(t, repograph.MustRepoPath("live/good-vpc"), mustSurface(t))
	// A second, earlier reference in another file.
	early := repograph.MustRepoPath("_common/a.hcl")
	d, err := repograph.NewDependency("vpc", repograph.MustRepoPath("live/bad-vpc"), mustPos(t, early, 1, 1), mustPos(t, early, 1, 1), repograph.TargetUnknown, repograph.DefaultDependencyOptions())
	if err != nil {
		t.Fatalf("NewDependency: %v", err)
	}
	units = append(units, mustResolvedUnit(t, repograph.MustRepoPath("live/c"), appModule,
		[]repograph.Dependency{d}, []repograph.Reference{mustRef(t, "vpc", "zzz", mustPos(t, early, 2, 1))}))

	g := mustGraph(t, units, modules)
	first, err := analysis.UnknownOutputs(g)
	if err != nil {
		t.Fatalf("UnknownOutputs: %v", err)
	}
	if len(first) != 3 {
		t.Fatalf("got %d diagnostics, want 3: %v", len(first), first)
	}
	refs := g.References()
	for i, d := range first {
		if d.Pos() != refs[i].Reference.Pos() {
			t.Errorf("diag %d pos %v, want References()[%d] pos %v", i, d.Pos(), i, refs[i].Reference.Pos())
		}
		if u, _ := d.Unit(); u != refs[i].Unit {
			t.Errorf("diag %d unit %v, want %v", i, u, refs[i].Unit)
		}
	}

	again, err := analysis.UnknownOutputs(g)
	if err != nil {
		t.Fatalf("UnknownOutputs: %v", err)
	}
	if !reflect.DeepEqual(first, again) {
		t.Errorf("repeated call differs")
	}

	reversed := make([]repograph.Unit, len(units))
	for i, u := range units {
		reversed[len(units)-1-i] = u
	}
	other, err := analysis.UnknownOutputs(mustGraph(t, reversed, modules))
	if err != nil {
		t.Fatalf("UnknownOutputs: %v", err)
	}
	if !reflect.DeepEqual(first, other) {
		t.Errorf("unit input order changes the result")
	}
}

func TestUnknownOutputsEmptyGraph(t *testing.T) {
	got, err := analysis.UnknownOutputs(mustGraph(t, nil, nil))
	if err != nil {
		t.Fatalf("UnknownOutputs: %v", err)
	}
	if got != nil {
		t.Errorf("got %v, want nil", got)
	}
}
