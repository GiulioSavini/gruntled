package terragrunt

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/GiulioSavini/gruntled/internal/domain/diagnostic"
	"github.com/GiulioSavini/gruntled/internal/domain/repograph"
	"github.com/GiulioSavini/gruntled/internal/infrastructure/tfsurface"
	"github.com/GiulioSavini/gruntled/internal/testsupport/synthrepo"
)

// TestTwoHop proves GRAPH-04's two-hop dependency resolution and GRAPH-01's
// unit-dir-!=-module-dir shape together, through the real adapters and
// indexing.Build: a dependency declared in a shared include resolves
// against the CHILD unit, and the reference it feeds keeps the include
// file's own position.
func TestTwoHop(t *testing.T) {
	fsys := filesFS(map[string]string{
		"_envcommon/app.hcl": `
dependency "vpc" {
  config_path = "../vpc"
}
inputs = {
  vpc_id = dependency.vpc.outputs.vpc_id
}
`,
		"live/app/terragrunt.hcl": `
include "common" { path = "../../_envcommon/app.hcl" }
terraform { source = "../../aws//lambda" }
`,
		"live/vpc/terragrunt.hcl": "",
		"live/vpc/main.tf": `
output "vpc_id" {
  value = "x"
}
`,
	})
	res := build(t, fsys)

	appPath := repograph.MustRepoPath("live/app")

	appModule, ok := res.Graph.ModuleOf(appPath)
	if !ok || appModule.Path().String() != "aws/lambda" {
		t.Fatalf("ModuleOf(live/app) = (%v, %v), want (aws/lambda, true)", appModule.Path(), ok)
	}

	vpcUnit, ok := res.Graph.DependencyTarget(appPath, "vpc")
	if !ok || vpcUnit.Path().String() != "live/vpc" {
		t.Fatalf("DependencyTarget(live/app, vpc) = (%v, %v), want (live/vpc, true)", vpcUnit.Path(), ok)
	}

	vpcModule, ok := res.Graph.ModuleOf(vpcUnit.Path())
	if !ok {
		t.Fatalf("ModuleOf(live/vpc) not found")
	}
	surf, ok := vpcModule.Surface()
	if !ok || !surf.HasOutput("vpc_id") {
		t.Fatalf("ModuleOf(live/vpc).Surface() = (%v, %v), want a surface declaring vpc_id", surf, ok)
	}

	appUnit, ok := res.Graph.Unit(appPath)
	if !ok {
		t.Fatalf("Unit(live/app) not found")
	}
	refs := appUnit.References()
	if len(refs) != 1 {
		t.Fatalf("len(References()) = %d, want 1: %+v", len(refs), refs)
	}
	if refs[0].Dependency() != "vpc" || refs[0].Output() != "vpc_id" {
		t.Fatalf("reference = %s.%s, want vpc.vpc_id", refs[0].Dependency(), refs[0].Output())
	}
	if refs[0].Pos().File().String() != "_envcommon/app.hcl" {
		t.Fatalf("reference file = %q, want %q (the include's own file, not the including unit's)", refs[0].Pos().File().String(), "_envcommon/app.hcl")
	}
}

// TestModuleUnknownBlastRadius proves GRAPH-03: a remote source makes only
// the owning unit's module unknown. Its own dependencies and references
// are kept, a unit depending ON it still resolves hop 1, and no other unit
// is affected.
func TestModuleUnknownBlastRadius(t *testing.T) {
	fsys := filesFS(map[string]string{
		"live/remote/terragrunt.hcl": `
terraform { source = "git::https://example.com/m.git//vpc?ref=v1" }
dependency "x" { config_path = "../x" }
inputs = {
  y = dependency.x.outputs.y
}
`,
		"live/x/terragrunt.hcl":        "",
		"live/consumer/terragrunt.hcl": `dependency "remote" { config_path = "../remote" }`,
	})
	res := build(t, fsys)

	remotePath := repograph.MustRepoPath("live/remote")
	remoteUnit, ok := res.Graph.Unit(remotePath)
	if !ok {
		t.Fatalf("Unit(live/remote) not found")
	}
	if remoteUnit.Status() != repograph.StatusModuleUnknown {
		t.Fatalf("live/remote Status() = %v, want module-unknown", remoteUnit.Status())
	}
	if remoteUnit.UnknownReason() != ReasonRemoteSource {
		t.Fatalf("live/remote UnknownReason() = %q, want %q", remoteUnit.UnknownReason(), ReasonRemoteSource)
	}
	if len(remoteUnit.Dependencies()) != 1 || len(remoteUnit.References()) != 1 {
		t.Fatalf("live/remote deps=%v refs=%v, want exactly one of each (kept, not dropped)", remoteUnit.Dependencies(), remoteUnit.References())
	}
	if _, ok := res.Graph.ModuleOf(remotePath); ok {
		t.Fatalf("ModuleOf(live/remote) = true, want false (module-unknown unit has no module)")
	}

	consumerPath := repograph.MustRepoPath("live/consumer")
	target, ok := res.Graph.DependencyTarget(consumerPath, "remote")
	if !ok || target.Path().String() != "live/remote" {
		t.Fatalf("DependencyTarget(live/consumer, remote) = (%v, %v), want (live/remote, true): hop 1 must succeed even though hop 2 (the target's own module) is unknown", target.Path(), ok)
	}
	consumerUnit, ok := res.Graph.Unit(consumerPath)
	if !ok || consumerUnit.Status() != repograph.StatusResolved {
		t.Fatalf("live/consumer Status() = %v, want resolved (unaffected by live/remote's module-unknown state)", consumerUnit.Status())
	}

	for _, d := range res.Diagnostics.All() {
		if u, ok := d.Unit(); ok && u.Compare(remotePath) == 0 {
			t.Errorf("unexpected diagnostic naming live/remote: %+v", d)
		}
	}
}

// TestGeneratedStackRemoteModuleUnknown proves STACK-07 and GRAPH-03
// together: a unit generated under .terragrunt-stack IS discovered by the
// walk, and a remote source on it produces the same module-unknown,
// deps/refs-kept behavior as an ordinary unit, with no diagnostic naming it
// or its module.
func TestGeneratedStackRemoteModuleUnknown(t *testing.T) {
	fsys := filesFS(map[string]string{
		"stacks/prod/.terragrunt-stack/vpc/terragrunt.hcl": `
terraform { source = "git::https://example.com/m.git//vpc" }
dependency "net" { config_path = "../net" }
inputs = {
  id = dependency.net.outputs.id
}
`,
		"stacks/prod/.terragrunt-stack/net/terragrunt.hcl": "",
	})
	res := build(t, fsys)

	unitPath := repograph.MustRepoPath("stacks/prod/.terragrunt-stack/vpc")
	unit, ok := res.Graph.Unit(unitPath)
	if !ok {
		t.Fatalf("unit %q not discovered: .terragrunt-stack must be walked, not skipped", unitPath.String())
	}
	if unit.Status() != repograph.StatusModuleUnknown {
		t.Fatalf("Status() = %v, want module-unknown", unit.Status())
	}
	if unit.UnknownReason() != ReasonRemoteSource {
		t.Fatalf("UnknownReason() = %q, want %q", unit.UnknownReason(), ReasonRemoteSource)
	}
	if len(unit.Dependencies()) != 1 {
		t.Fatalf("len(Dependencies()) = %d, want 1", len(unit.Dependencies()))
	}
	if len(unit.References()) != 1 {
		t.Fatalf("len(References()) = %d, want 1", len(unit.References()))
	}
	if _, ok := res.Graph.ModuleOf(unitPath); ok {
		t.Fatalf("ModuleOf(%q) = true, want false", unitPath.String())
	}

	for _, d := range res.Diagnostics.All() {
		if u, ok := d.Unit(); ok && u.Compare(unitPath) == 0 {
			t.Errorf("unexpected diagnostic naming the generated stack unit: %+v", d)
		}
	}
}

// TestSurfaceUnknownTarget proves SRC-13 and research Pitfall 10 together:
// a unit's own config is fully static even when its source points at a
// module directory that does not exist, and a root terragrunt.hcl with no
// .tf files (the cds-snc/secret shape) resolves as an ordinary unit whose
// module surface is unknown, with no diagnostic either way.
func TestSurfaceUnknownTarget(t *testing.T) {
	fsys := filesFS(map[string]string{
		"u/terragrunt.hcl":          `terraform { source = "../modules/typo-vpc" }`,
		"terragrunt/terragrunt.hcl": "",
	})
	res := build(t, fsys)

	uPath := repograph.MustRepoPath("u")
	uUnit, ok := res.Graph.Unit(uPath)
	if !ok || uUnit.Status() != repograph.StatusResolved {
		t.Fatalf("Unit(u) = (%v, %v), want (resolved, true): the source string itself is fully static", uUnit.Status(), ok)
	}
	uMod, ok := res.Graph.ModuleOf(uPath)
	if !ok {
		t.Fatalf("ModuleOf(u) not found in the graph")
	}
	if _, ok := uMod.Surface(); ok {
		t.Fatalf("ModuleOf(u).Surface() = ok, want unknown (the module directory does not exist)")
	}
	if uMod.UnknownReason() != tfsurface.ReasonModuleDirNotFound {
		t.Fatalf("ModuleOf(u).UnknownReason() = %q, want %q", uMod.UnknownReason(), tfsurface.ReasonModuleDirNotFound)
	}

	trPath := repograph.MustRepoPath("terragrunt")
	trUnit, ok := res.Graph.Unit(trPath)
	if !ok || trUnit.Status() != repograph.StatusResolved {
		t.Fatalf("Unit(terragrunt) = (%v, %v), want (resolved, true)", trUnit.Status(), ok)
	}
	trMod, ok := res.Graph.ModuleOf(trPath)
	if !ok {
		t.Fatalf("ModuleOf(terragrunt) not found in the graph")
	}
	if _, ok := trMod.Surface(); ok {
		t.Fatalf("ModuleOf(terragrunt).Surface() = ok, want unknown (no .tf files)")
	}
	if trMod.UnknownReason() != tfsurface.ReasonNoTerraformFiles {
		t.Fatalf("ModuleOf(terragrunt).UnknownReason() = %q, want %q", trMod.UnknownReason(), tfsurface.ReasonNoTerraformFiles)
	}

	for _, d := range res.Diagnostics.All() {
		if unit, ok := d.Unit(); ok && (unit.Compare(uPath) == 0 || unit.Compare(trPath) == 0) {
			t.Errorf("unexpected diagnostic: %+v", d)
		}
	}
}

// TestWholeBodyRefs proves references are extracted from the unit's ENTIRE
// effective body (locals, inputs, a nested terraform block, and a merged
// include file), with byte-accurate, not grapheme-accurate, positions.
func TestWholeBodyRefs(t *testing.T) {
	fsys := filesFS(map[string]string{
		// A single leading space shifts the "ééé" line's "d" in
		// "dependency" to byte column 23 (hclconv_test.go's own
		// TestPositionNonASCIIPrefixUsesBytes proves the unindented form of
		// this exact line gives column 22).
		"inc.hcl": " a = \"ééé\" == \"\" ? dependency.x.outputs.y : \"\"\n",
		"u/terragrunt.hcl": `
include "root" { path = "../inc.hcl" }
locals {
  l = dependency.a.outputs.l1
}
inputs = {
  i = dependency.a.outputs.i1
}
terraform {
  extra_arguments "vars" {
    commands  = ["plan"]
    arguments = [dependency.a.outputs.extra1]
  }
}
`,
	})
	res := build(t, fsys)

	unit, ok := res.Graph.Unit(repograph.MustRepoPath("u"))
	if !ok {
		t.Fatalf("Unit(u) not found")
	}
	refs := unit.References()
	if len(refs) != 4 {
		t.Fatalf("len(References()) = %d, want 4: %+v", len(refs), refs)
	}

	want := map[[2]string]bool{
		{"a", "l1"}:     false,
		{"a", "i1"}:     false,
		{"a", "extra1"}: false,
		{"x", "y"}:      false,
	}
	var xyColumn int
	for _, r := range refs {
		key := [2]string{r.Dependency(), r.Output()}
		if _, known := want[key]; !known {
			t.Errorf("unexpected reference %s.%s", r.Dependency(), r.Output())
			continue
		}
		want[key] = true
		if key == ([2]string{"x", "y"}) {
			xyColumn = r.Pos().Column()
			if r.Pos().File().String() != "inc.hcl" {
				t.Errorf("x.y reference file = %q, want %q", r.Pos().File().String(), "inc.hcl")
			}
		}
	}
	for k, found := range want {
		if !found {
			t.Errorf("missing reference %s.%s", k[0], k[1])
		}
	}
	if xyColumn != 23 {
		t.Errorf("x.y reference column = %d, want 23 (byte column, not grapheme column)", xyColumn)
	}
}

// TestSyntaxEndToEnd proves PARSE-05 end to end: a broken module file
// leaves the unit itself resolved but its module surface unknown with one
// GRT100 against the module file, and a broken unit file makes the unit
// config-unknown with one GRT100 against the unit file. Both diagnostics
// are file-level (zero Unit) and repo-relative.
func TestSyntaxEndToEnd(t *testing.T) {
	t.Run("broken-module-file", func(t *testing.T) {
		fsys := filesFS(map[string]string{
			"u/terragrunt.hcl": `terraform { source = "../mod" }`,
			"mod/main.tf":      "variable {",
		})
		res := build(t, fsys)

		uPath := repograph.MustRepoPath("u")
		unit, ok := res.Graph.Unit(uPath)
		if !ok || unit.Status() != repograph.StatusResolved {
			t.Fatalf("Unit(u) = (%v, %v), want (resolved, true): the unit's own config is fine", unit.Status(), ok)
		}
		mod, ok := res.Graph.ModuleOf(uPath)
		if !ok {
			t.Fatalf("ModuleOf(u) not found")
		}
		if _, ok := mod.Surface(); ok {
			t.Fatalf("ModuleOf(u).Surface() = ok, want unknown (syntax error)")
		}

		diags := res.Diagnostics.All()
		if len(diags) != 1 {
			t.Fatalf("len(Diagnostics) = %d, want 1: %+v", len(diags), diags)
		}
		if diags[0].Code() != diagnostic.CodeSyntaxError {
			t.Errorf("Code() = %v, want %v", diags[0].Code(), diagnostic.CodeSyntaxError)
		}
		if diags[0].Pos().File().String() != "mod/main.tf" {
			t.Errorf("Pos().File() = %q, want %q", diags[0].Pos().File().String(), "mod/main.tf")
		}
		if _, ok := diags[0].Unit(); ok {
			t.Errorf("Unit() = true, want false (file-level diagnostic)")
		}
	})

	t.Run("broken-unit-file", func(t *testing.T) {
		fsys := filesFS(map[string]string{"u/terragrunt.hcl": "locals {"})
		res := build(t, fsys)

		uPath := repograph.MustRepoPath("u")
		unit, ok := res.Graph.Unit(uPath)
		if !ok || unit.Status() != repograph.StatusConfigUnknown {
			t.Fatalf("Unit(u) = (%v, %v), want (config-unknown, true)", unit.Status(), ok)
		}
		if unit.UnknownReason() != ReasonSyntaxError {
			t.Errorf("UnknownReason() = %q, want %q", unit.UnknownReason(), ReasonSyntaxError)
		}

		diags := res.Diagnostics.All()
		if len(diags) != 1 {
			t.Fatalf("len(Diagnostics) = %d, want 1: %+v", len(diags), diags)
		}
		if diags[0].Pos().File().String() != "u/terragrunt.hcl" {
			t.Errorf("Pos().File() = %q, want %q", diags[0].Pos().File().String(), "u/terragrunt.hcl")
		}
	})
}

// TestParseOnce proves PARSE-03's speed claim end to end: building a
// 50-unit synthetic repository reads root.hcl, and every other file, at
// most once, regardless of how many units share it.
func TestParseOnce(t *testing.T) {
	tree, _, err := synthrepo.Render(synthrepo.Spec{Units: 50, IncludeDepth: 3, DependencyFanout: 3, Seed: 11})
	if err != nil {
		t.Fatalf("synthrepo.Render: %v", err)
	}
	cfs := newCountingFS(mapFS(tree))
	res := build(t, cfs)
	if len(res.Graph.Units()) != 50 {
		t.Fatalf("len(Units()) = %d, want 50", len(res.Graph.Units()))
	}

	if got := cfs.count("root.hcl"); got != 1 {
		t.Errorf("count(root.hcl) = %d, want 1", got)
	}
	for _, f := range tree {
		if got := cfs.count(f.Path); got > 1 {
			t.Errorf("count(%s) = %d, want <= 1", f.Path, got)
		}
	}
}

// TestSynthrepoOracle proves the graph's own unresolvable references match
// synthrepo's injected-mutation Manifest exactly: for every reference in
// the graph, following DependencyTarget, ModuleOf and Surface must find
// the output declared, except for exactly the references the generator
// corrupted.
func TestSynthrepoOracle(t *testing.T) {
	tree, manifest, err := synthrepo.Render(synthrepo.Spec{
		Units:            40,
		IncludeDepth:     2,
		DependencyFanout: 3,
		Seed:             5,
		Errors:           []synthrepo.ErrorKind{synthrepo.BadOutputRef, synthrepo.BadOutputRef, synthrepo.BadOutputRef},
	})
	if err != nil {
		t.Fatalf("synthrepo.Render: %v", err)
	}
	res := build(t, mapFS(tree))

	for _, u := range res.Graph.Units() {
		if u.Status() != repograph.StatusResolved {
			t.Errorf("unit %s Status() = %v, want resolved", u.Path(), u.Status())
		}
	}

	var actual []synthrepo.ExpectedDiagnostic
	for _, ur := range res.Graph.References() {
		target, ok := res.Graph.DependencyTarget(ur.Unit, ur.Reference.Dependency())
		if !ok {
			t.Fatalf("DependencyTarget(%s, %s) not found", ur.Unit, ur.Reference.Dependency())
		}
		mod, ok := res.Graph.ModuleOf(target.Path())
		if !ok {
			t.Fatalf("ModuleOf(%s) not found", target.Path())
		}
		surf, ok := mod.Surface()
		if !ok {
			t.Fatalf("Surface(%s) unknown, want known", mod.Path())
		}
		if surf.HasOutput(ur.Reference.Output()) {
			continue
		}
		actual = append(actual, synthrepo.ExpectedDiagnostic{
			Code:       diagnostic.CodeUnknownOutput,
			Pos:        ur.Reference.Pos(),
			Unit:       ur.Unit,
			Dependency: ur.Reference.Dependency(),
			Target:     target.Path(),
			Output:     ur.Reference.Output(),
		})
	}

	if !slices.Equal(actual, manifest.Expected) {
		t.Fatalf("mismatch between the graph's unresolvable references and the manifest:\n got=%+v\nwant=%+v", actual, manifest.Expected)
	}
}

// TestDeterministic proves building the same repository twice, and from
// two differently-named checkouts, gives an identical canonical dump, and
// that the dump never leaks a temp directory path or the virtual-root
// sentinel.
func TestDeterministic(t *testing.T) {
	spec := synthrepo.Spec{Units: 15, IncludeDepth: 2, DependencyFanout: 2, Seed: 7}

	base := t.TempDir()
	dirA := filepath.Join(base, "checkout-a")
	dirB := filepath.Join(base, "another-name")
	if err := os.Mkdir(dirA, 0o755); err != nil {
		t.Fatalf("Mkdir(%s): %v", dirA, err)
	}
	if err := os.Mkdir(dirB, 0o755); err != nil {
		t.Fatalf("Mkdir(%s): %v", dirB, err)
	}
	if _, err := synthrepo.Generate(spec, dirA); err != nil {
		t.Fatalf("Generate(%s): %v", dirA, err)
	}
	if _, err := synthrepo.Generate(spec, dirB); err != nil {
		t.Fatalf("Generate(%s): %v", dirB, err)
	}

	dumpOf := func(dir string) string {
		t.Helper()
		root, err := os.OpenRoot(dir)
		if err != nil {
			t.Fatalf("OpenRoot(%s): %v", dir, err)
		}
		defer root.Close()
		return dump(build(t, root.FS()))
	}

	a1, a2 := dumpOf(dirA), dumpOf(dirA)
	b1, b2 := dumpOf(dirB), dumpOf(dirB)

	if a1 != a2 {
		t.Fatalf("building %s twice gave different dumps", dirA)
	}
	if b1 != b2 {
		t.Fatalf("building %s twice gave different dumps", dirB)
	}
	if a1 != b1 {
		t.Fatalf("checkout-a and another-name gave different dumps:\nA=%s\nB=%s", a1, b1)
	}
	if strings.Contains(a1, base) {
		t.Errorf("dump contains the temp directory path")
	}
	if strings.Contains(a1, "__gruntled_repo_root__") {
		t.Errorf("dump contains the virtual-root sentinel")
	}
}

// TestCorpusSmoke is skipped unless GRUNTLED_CORPUS points at a local
// clone of the primary corpus
// (aws-solutions-library-samples/guidance-for-iso20022-messaging-workflows-on-aws).
// It asserts the corpus's known shape: 65 units, 22 references, every one
// resolving to a declared output, and 0 module-unknown units.
//
// Exactly 3 units are config-unknown, all for the identical real-world
// reason: iac.cicd/codebuild_project, iac.src/scheduler_recover and
// iac.src/scheduler_timeout each declare two `dependency "iam" { ... }`
// blocks with the SAME label in one file. Terragrunt itself only warns
// and lets the last one win (the duplicate-dependency-labels strict
// control); research Pattern 6 and this plan's own step 6
// (invalid-dependency on "a duplicate label within the file") deliberately
// refuse to guess which one Terragrunt would pick, since guessing wrong
// would silently drop a real dependency edge. This is not a defect: it is
// the zero-false-positives policy finding a real, previously undocumented
// shape in the primary corpus (the earlier prototype behind
// 02-RESEARCH.md's "0 unknown" finding predates this loader's stricter,
// plan-mandated duplicate-label handling).
func TestCorpusSmoke(t *testing.T) {
	corpus := os.Getenv("GRUNTLED_CORPUS")
	if corpus == "" {
		t.Skip("GRUNTLED_CORPUS not set")
	}

	root, err := os.OpenRoot(corpus)
	if err != nil {
		t.Fatalf("OpenRoot(%s): %v", corpus, err)
	}
	defer root.Close()

	res := build(t, root.FS())

	units := res.Graph.Units()
	if len(units) != 65 {
		t.Errorf("len(Units()) = %d, want 65", len(units))
	}
	wantConfigUnknown := map[string]bool{
		"iac.cicd/codebuild_project": true,
		"iac.src/scheduler_recover":  true,
		"iac.src/scheduler_timeout":  true,
	}
	var configUnknown, moduleUnknown int
	for _, u := range units {
		switch u.Status() {
		case repograph.StatusConfigUnknown:
			configUnknown++
			if !wantConfigUnknown[u.Path().String()] {
				t.Errorf("unexpected config-unknown unit %s (reason %q)", u.Path(), u.UnknownReason())
			} else if u.UnknownReason() != ReasonInvalidDependency {
				t.Errorf("%s UnknownReason() = %q, want %q", u.Path(), u.UnknownReason(), ReasonInvalidDependency)
			}
		case repograph.StatusModuleUnknown:
			moduleUnknown++
		}
	}
	if configUnknown != len(wantConfigUnknown) {
		t.Errorf("config-unknown units = %d, want %d: %v", configUnknown, len(wantConfigUnknown), wantConfigUnknown)
	}
	if moduleUnknown != 0 {
		t.Errorf("module-unknown units = %d, want 0", moduleUnknown)
	}

	refs := res.Graph.References()
	if len(refs) != 22 {
		t.Errorf("len(References()) = %d, want 22", len(refs))
	}
	missing := 0
	for _, ur := range refs {
		target, ok := res.Graph.DependencyTarget(ur.Unit, ur.Reference.Dependency())
		if !ok {
			t.Errorf("DependencyTarget(%s, %s) not found", ur.Unit, ur.Reference.Dependency())
			continue
		}
		mod, ok := res.Graph.ModuleOf(target.Path())
		if !ok {
			t.Errorf("ModuleOf(%s) not found", target.Path())
			continue
		}
		surf, ok := mod.Surface()
		if !ok {
			t.Errorf("Surface(%s) unknown, want known", mod.Path())
			continue
		}
		if !surf.HasOutput(ur.Reference.Output()) {
			missing++
		}
	}
	if missing != 0 {
		t.Errorf("references missing their declared output = %d, want 0", missing)
	}
}
