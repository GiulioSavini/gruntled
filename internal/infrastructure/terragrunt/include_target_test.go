package terragrunt

import (
	"os"
	"strings"
	"testing"

	"github.com/GiulioSavini/gruntled/internal/application/indexing"
	"github.com/GiulioSavini/gruntled/internal/domain/repograph"
	"github.com/GiulioSavini/gruntled/internal/infrastructure/hclconv"
)

// TestIncludeTargetCorpusReproduction is the EXACT 03-RESEARCH.md Pattern 4
// reproduction: a legacy root live/terragrunt.hcl that live/prod/app
// includes via find_in_parent_folders(). Before G3's fix, the parent was
// also analysed standalone, resolving its own dependency.vpc.config_path
// ("../vpc") against ITS OWN directory ("live"), reaching the repo-root
// "vpc" unit -- which declares a different output -- and printing a false
// GRT001 at live/terragrunt.hcl:4:21. After the fix, "live" is
// config-unknown include-target and drops out of the standalone
// interpretation entirely; the reference is still checked, correctly, once
// through the including unit live/prod/app.
func TestIncludeTargetCorpusReproduction(t *testing.T) {
	fsys := filesFS(map[string]string{
		// Column 21 on line 4: "inputs = { vpc_id = " is 20 bytes, so the
		// "d" of "dependency" starts at byte column 21 -- the exact
		// position the prototype printed.
		"live/terragrunt.hcl": `dependency "vpc" {
  config_path = "../vpc"
}
inputs = { vpc_id = dependency.vpc.outputs.id }
`,
		"live/prod/app/terragrunt.hcl": `include { path = find_in_parent_folders() }`,
		"live/prod/app/main.tf":        "",
		"live/prod/vpc/terragrunt.hcl": "",
		"live/prod/vpc/main.tf":        `output "id" { value = 1 }`,
		"vpc/terragrunt.hcl":           "",
		"vpc/main.tf":                  `output "other" { value = 1 }`,
	})
	res := build(t, fsys)

	live, ok := res.Graph.Unit(repograph.MustRepoPath("live"))
	if !ok {
		t.Fatalf("unit %q not found", "live")
	}
	if live.Status() != repograph.StatusConfigUnknown {
		t.Fatalf("live Status() = %v, want %v", live.Status(), repograph.StatusConfigUnknown)
	}
	if live.UnknownReason() != ReasonIncludeTarget {
		t.Fatalf("live UnknownReason() = %q, want %q", live.UnknownReason(), ReasonIncludeTarget)
	}
	if len(live.Dependencies()) != 0 {
		t.Fatalf("live Dependencies() = %v, want none", live.Dependencies())
	}
	if len(live.References()) != 0 {
		t.Fatalf("live References() = %v, want none", live.References())
	}

	app, ok := res.Graph.Unit(repograph.MustRepoPath("live/prod/app"))
	if !ok {
		t.Fatalf("unit %q not found", "live/prod/app")
	}
	if app.Status() != repograph.StatusResolved {
		t.Fatalf("live/prod/app Status() = %v, want %v (reason %q)", app.Status(), repograph.StatusResolved, app.UnknownReason())
	}
	dep, ok := app.Dependency("vpc")
	if !ok {
		t.Fatalf("live/prod/app has no dependency %q", "vpc")
	}
	target, ok := dep.Target()
	if !ok || target.String() != "live/prod/vpc" {
		t.Fatalf("Target() = (%q, %v), want (%q, true)", target.String(), ok, "live/prod/vpc")
	}

	foundRef := false
	for _, r := range app.References() {
		if r.Dependency() == "vpc" && r.Output() == "id" {
			foundRef = true
			if got, want := r.Pos().String(), "live/terragrunt.hcl:4:21"; got != want {
				t.Errorf("reference position = %q, want %q", got, want)
			}
		}
	}
	if !foundRef {
		t.Fatalf("live/prod/app has no (vpc, id) reference: %+v", app.References())
	}

	missing := 0
	for _, ur := range res.Graph.References() {
		target, ok := res.Graph.DependencyTarget(ur.Unit, ur.Reference.Dependency())
		if !ok {
			continue
		}
		mod, ok := res.Graph.ModuleOf(target.Path())
		if !ok {
			continue
		}
		surf, ok := mod.Surface()
		if !ok {
			continue
		}
		if !surf.HasOutput(ur.Reference.Output()) {
			missing++
		}
	}
	if missing != 0 {
		t.Fatalf("references missing their declared output = %d, want 0 (the false GRT001 at live/terragrunt.hcl:4:21 must be gone)", missing)
	}
}

// TestIncludeTargetExplicitPath proves G3 for an explicit (non-
// find_in_parent_folders) include path: b is itself a discovered unit that
// a includes by an explicit relative path. b becomes include-target, and a
// is analysed normally.
func TestIncludeTargetExplicitPath(t *testing.T) {
	fsys := filesFS(map[string]string{
		"a/terragrunt.hcl": `include "x" { path = "../b/terragrunt.hcl" }`,
		"b/terragrunt.hcl": "",
	})
	res := loadUnits(t, fsys)

	b := unitByPath(t, res, "b")
	if b.ConfigUnknownReason != ReasonIncludeTarget {
		t.Fatalf("b ConfigUnknownReason = %q, want %q", b.ConfigUnknownReason, ReasonIncludeTarget)
	}

	a := unitByPath(t, res, "a")
	if a.ConfigUnknownReason != "" || a.ModuleUnknownReason != "" {
		t.Fatalf("a unknown: config=%q module=%q, want fully resolved", a.ConfigUnknownReason, a.ModuleUnknownReason)
	}
}

// TestIncludeTargetKeepsEarlierConfigUnknownReason proves that an included
// unit's own, earlier config-unknown reason is never overwritten by the
// include-target post-pass (resolveUnit's fixed order: the first check that
// applies always wins), and that GRT100 still fires exactly once for the
// broken file even though it is read through both the discovery of "bad"
// itself and the include resolution of "child".
func TestIncludeTargetKeepsEarlierConfigUnknownReason(t *testing.T) {
	fsys := filesFS(map[string]string{
		"bad/terragrunt.hcl":   "locals {",
		"child/terragrunt.hcl": `include { path = "../bad/terragrunt.hcl" }`,
	})
	res := loadUnits(t, fsys)

	bad := unitByPath(t, res, "bad")
	if bad.ConfigUnknownReason != ReasonSyntaxError {
		t.Fatalf("bad ConfigUnknownReason = %q, want %q (first check wins over the include-target post-pass)", bad.ConfigUnknownReason, ReasonSyntaxError)
	}

	if len(res.Diagnostics) != 1 {
		t.Fatalf("len(Diagnostics) = %d, want exactly 1: %+v", len(res.Diagnostics), res.Diagnostics)
	}
}

// TestIncludeTargetSecretCorpus checks the documented facts of the
// cds-snc/secret secondary corpus (checkout at commit 341e8a95, root =
// repo root) against the real fix: verified directly against the checkout
// at ~/.cache/gruntled-phase4/corpus/secret with
//
//	GRUNTLED_CORPUS_SECRET=$HOME/.cache/gruntled-phase4/corpus/secret \
//	  go test -count=1 ./internal/infrastructure/terragrunt -run TestIncludeTargetSecretCorpus -v
func TestIncludeTargetSecretCorpus(t *testing.T) {
	corpus := os.Getenv("GRUNTLED_CORPUS_SECRET")
	if corpus == "" {
		t.Skip("GRUNTLED_CORPUS_SECRET not set")
	}

	root, err := os.OpenRoot(corpus)
	if err != nil {
		t.Fatalf("OpenRoot(%s): %v", corpus, err)
	}
	defer root.Close()

	res := build(t, root.FS())

	units := res.Graph.Units()
	if len(units) != 4 {
		t.Fatalf("len(Units()) = %d, want 4: %v", len(units), units)
	}

	parent, ok := res.Graph.Unit(repograph.MustRepoPath("terragrunt"))
	if !ok {
		t.Fatalf("unit %q not found", "terragrunt")
	}
	if parent.Status() != repograph.StatusConfigUnknown || parent.UnknownReason() != ReasonIncludeTarget {
		t.Fatalf("terragrunt Status()=%v UnknownReason()=%q, want %v/%q", parent.Status(), parent.UnknownReason(), repograph.StatusConfigUnknown, ReasonIncludeTarget)
	}

	wantModules := map[string]string{
		"terragrunt/acm":    "aws/acm",
		"terragrunt/ecr":    "aws/ecr",
		"terragrunt/lambda": "aws/lambda",
	}
	for unitPath, wantModule := range wantModules {
		u, ok := res.Graph.Unit(repograph.MustRepoPath(unitPath))
		if !ok {
			t.Fatalf("unit %q not found", unitPath)
		}
		if u.Status() != repograph.StatusResolved {
			t.Fatalf("%s Status() = %v (reason %q), want %v", unitPath, u.Status(), u.UnknownReason(), repograph.StatusResolved)
		}
		mod, ok := u.Module()
		if !ok || mod.String() != wantModule {
			t.Fatalf("%s Module() = (%q, %v), want (%q, true)", unitPath, mod.String(), ok, wantModule)
		}
	}

	refs := res.Graph.References()
	if len(refs) != 4 {
		t.Fatalf("len(References()) = %d, want 4: %+v", len(refs), refs)
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
		t.Fatalf("references missing their declared output = %d, want 0", missing)
	}
}

// g16Tree is the TestIncludeTargetCorpusReproduction tree with app's own
// terragrunt.hcl replaced by appHCL: the G16 repros change ONLY that file.
func g16Tree(appHCL string) map[string]string {
	return map[string]string{
		"live/terragrunt.hcl": `dependency "vpc" {
  config_path = "../vpc"
}
inputs = { vpc_id = dependency.vpc.outputs.id }
`,
		"live/prod/app/terragrunt.hcl": appHCL,
		"live/prod/app/main.tf":        "",
		"live/prod/vpc/terragrunt.hcl": "",
		"live/prod/vpc/main.tf":        `output "id" { value = 1 }`,
		"vpc/terragrunt.hcl":           "",
		"vpc/main.tf":                  `output "other" { value = 1 }`,
	}
}

// assertG16 checks the shared G16 outcome: live is config-unknown
// include-target with no references, app has wantAppReason, and no
// reference misses its output (the false GRT001 at live/terragrunt.hcl:4:21
// is gone).
func assertG16(t *testing.T, appHCL, wantAppReason string) indexing.Result {
	t.Helper()
	res := build(t, filesFS(g16Tree(appHCL)))
	live, ok := res.Graph.Unit(repograph.MustRepoPath("live"))
	if !ok {
		t.Fatalf("unit %q not found", "live")
	}
	if live.Status() != repograph.StatusConfigUnknown || live.UnknownReason() != ReasonIncludeTarget {
		t.Fatalf("live = (%v, %q), want (%v, %q)", live.Status(), live.UnknownReason(), repograph.StatusConfigUnknown, ReasonIncludeTarget)
	}
	if len(live.References()) != 0 {
		t.Fatalf("live References() = %v, want none", live.References())
	}
	app, ok := res.Graph.Unit(repograph.MustRepoPath("live/prod/app"))
	if !ok {
		t.Fatalf("unit %q not found", "live/prod/app")
	}
	if app.UnknownReason() != wantAppReason {
		t.Fatalf("live/prod/app UnknownReason() = %q, want %q", app.UnknownReason(), wantAppReason)
	}
	if n := missingOutputs(res); n != 0 {
		t.Fatalf("references missing their declared output = %d, want 0", n)
	}
	return res
}

func TestIncludeTargetG16RepoRoot(t *testing.T) {
	assertG16(t, `include { path = "${get_repo_root()}/live/terragrunt.hcl" }`, ReasonIncludeDynamicPath)
}

func TestIncludeTargetG16PathToRepoRoot(t *testing.T) {
	assertG16(t, `include { path = "${get_path_to_repo_root()}/live/terragrunt.hcl" }`, ReasonIncludeDynamicPath)
}

func TestIncludeTargetG16SyntaxError(t *testing.T) {
	res := assertG16(t, "include { path = find_in_parent_folders() }\ninputs = {\n", ReasonSyntaxError)
	if res.Diagnostics.Len() != 1 {
		t.Fatalf("Diagnostics.Len() = %d, want exactly 1 GRT100 for app's file", res.Diagnostics.Len())
	}
}

func TestIncludeTargetG16InvalidDecls(t *testing.T) {
	assertG16(t, "include { path = find_in_parent_folders() }\ninclude { path = find_in_parent_folders() }\n", ReasonInvalidInclude)
}

func TestIncludeTargetMarkedPastFirstFailure(t *testing.T) {
	res := loadUnits(t, filesFS(map[string]string{
		"x.hcl.json":            "{}",
		"shared/terragrunt.hcl": "",
		"u/terragrunt.hcl": `include "a" { path = "../x.hcl.json" }
include "b" { path = "../shared/terragrunt.hcl" }
`,
	}))
	if u := unitByPath(t, res, "u"); u.ConfigUnknownReason != ReasonIncludeJSONUnsupported {
		t.Fatalf("u ConfigUnknownReason = %q, want %q", u.ConfigUnknownReason, ReasonIncludeJSONUnsupported)
	}
	if s := unitByPath(t, res, "shared"); s.ConfigUnknownReason != ReasonIncludeTarget {
		t.Fatalf("shared ConfigUnknownReason = %q, want %q", s.ConfigUnknownReason, ReasonIncludeTarget)
	}
}

func TestIncludeTargetDynamicSibling(t *testing.T) {
	res := loadUnits(t, filesFS(map[string]string{
		"root.hcl":                     "",
		"shared/terragrunt.hcl":        "",
		"other/terragrunt.hcl":         "",
		"live/prod/app/terragrunt.hcl": `include { path = "${get_repo_root()}/shared/terragrunt.hcl" }`,
		"live/prod/vpc/terragrunt.hcl": `include { path = find_in_parent_folders("root.hcl") }`,
	}))
	for _, dir := range []string{"shared", "other"} {
		if u := unitByPath(t, res, dir); u.ConfigUnknownReason != ReasonIncludeTarget {
			t.Fatalf("%s ConfigUnknownReason = %q, want %q", dir, u.ConfigUnknownReason, ReasonIncludeTarget)
		}
	}
	if v := unitByPath(t, res, "live/prod/vpc"); v.ConfigUnknownReason != "" {
		t.Fatalf("live/prod/vpc ConfigUnknownReason = %q, want resolved (it has its own include)", v.ConfigUnknownReason)
	}
}

// assertAncestorsOnly checks that app's ancestor units are include-target
// while the non-ancestor, include-free sibling "shared" stays resolved.
func assertAncestorsOnly(t *testing.T, appHCL string) {
	t.Helper()
	res := loadUnits(t, filesFS(map[string]string{
		"terragrunt.hcl":               "",
		"live/terragrunt.hcl":          "",
		"shared/terragrunt.hcl":        "",
		"live/prod/app/terragrunt.hcl": appHCL,
	}))
	for _, dir := range []string{".", "live"} {
		if u := unitByPath(t, res, dir); u.ConfigUnknownReason != ReasonIncludeTarget {
			t.Fatalf("%s ConfigUnknownReason = %q, want %q", dir, u.ConfigUnknownReason, ReasonIncludeTarget)
		}
	}
	if s := unitByPath(t, res, "shared"); s.ConfigUnknownReason != "" {
		t.Fatalf("shared ConfigUnknownReason = %q, want resolved", s.ConfigUnknownReason)
	}
}

func TestIncludeTargetDynamicFixedName(t *testing.T) {
	assertAncestorsOnly(t, `include { path = "${get_repo_root()}/_envcommon/vpc.hcl" }`)
}

func TestIncludeTargetDynamicConditional(t *testing.T) {
	assertAncestorsOnly(t, `include { path = get_env("CI", "") == "true" ? "ci.hcl" : "local.hcl" }`)
}

// assertSharedTarget loads app next to the include-free, non-ancestor unit
// "shared" and checks whether shared is an include target.
func assertSharedTarget(t *testing.T, appHCL string, want bool) {
	t.Helper()
	res := loadUnits(t, filesFS(map[string]string{
		"shared/terragrunt.hcl":   "",
		"live/app/terragrunt.hcl": appHCL,
	}))
	if got := unitByPath(t, res, "shared").ConfigUnknownReason == ReasonIncludeTarget; got != want {
		t.Fatalf("shared is include target = %v, want %v", got, want)
	}
}

// TestIncludeTargetRejectedByTerragrunt pins that an include path naming a
// variable Terragrunt 1.1.6 does not provide at include time marks
// nothing: Terragrunt fails to parse the unit, so it includes no file.
func TestIncludeTargetRejectedByTerragrunt(t *testing.T) {
	for name, hcl := range map[string]string{
		"local": `locals { root = "../../shared/terragrunt.hcl" }
include { path = local.root }
`,
		"local_template": `locals { f = "terragrunt" }
include { path = "${local.f}.hcl" }
`,
		"local_find":  `include { path = find_in_parent_folders("${local.file}") }`,
		"include_ref": `include { path = find_in_parent_folders(include.other.locals.f) }`,
	} {
		t.Run(name, func(t *testing.T) { assertSharedTarget(t, hcl, false) })
	}
}

// TestIncludeTargetDynamicValues pins that values, which Terragrunt does
// provide at include time, and a try that can catch an unknown variable,
// both keep the conservative marking.
func TestIncludeTargetDynamicValues(t *testing.T) {
	for name, hcl := range map[string]string{
		"values": `include { path = values.parent }`,
		"try":    `include { path = try(local.root, "../../shared/terragrunt.hcl") }`,
	} {
		t.Run(name, func(t *testing.T) { assertSharedTarget(t, hcl, true) })
	}
}

// TestIncludeTargetFindInParentFoldersMiss pins that a
// find_in_parent_folders call that finds nothing in the repository marks
// nothing, unless its argument could climb out of the ancestor chain.
func TestIncludeTargetFindInParentFoldersMiss(t *testing.T) {
	for name, tc := range map[string]struct {
		hcl  string
		want bool
	}{
		"no_arg":    {`include { path = find_in_parent_folders() }`, false},
		"base_name": {`include { path = find_in_parent_folders("root.hcl") }`, false},
		"slash":     {`include { path = find_in_parent_folders("x/terragrunt.hcl") }`, true},
		"fallback":  {`include { path = find_in_parent_folders(get_env("N"), "x") }`, true},
	} {
		t.Run(name, func(t *testing.T) { assertSharedTarget(t, tc.hcl, tc.want) })
	}
}

func TestIncludeTargetUnknowableIncludes(t *testing.T) {
	cases := map[string]map[string]string{
		"json": {"live/app/terragrunt.hcl.json": "{}"},
		"autoinclude": {
			"live/app/terragrunt.hcl":             "",
			"live/app/terragrunt.autoinclude.hcl": "",
		},
		"oversize": {"live/app/terragrunt.hcl": "#" + strings.Repeat("x", hclconv.MaxFileBytes)},
	}
	for name, extra := range cases {
		t.Run(name, func(t *testing.T) {
			files := map[string]string{
				"live/terragrunt.hcl":   "",
				"shared/terragrunt.hcl": "",
			}
			for k, v := range extra {
				files[k] = v
			}
			res := loadUnits(t, filesFS(files))
			if u := unitByPath(t, res, "live"); u.ConfigUnknownReason != ReasonIncludeTarget {
				t.Fatalf("live ConfigUnknownReason = %q, want %q", u.ConfigUnknownReason, ReasonIncludeTarget)
			}
			if s := unitByPath(t, res, "shared"); s.ConfigUnknownReason != "" {
				t.Fatalf("shared ConfigUnknownReason = %q, want resolved", s.ConfigUnknownReason)
			}
		})
	}
}

func TestIncludeTargetNotFoundMarksAncestors(t *testing.T) {
	res := loadUnits(t, filesFS(map[string]string{
		"live/terragrunt.hcl":     "",
		"live/app/terragrunt.hcl": `include { path = "../missing.hcl" }`,
	}))
	if a := unitByPath(t, res, "live/app"); a.ConfigUnknownReason != ReasonIncludeNotFound {
		t.Fatalf("live/app ConfigUnknownReason = %q, want %q", a.ConfigUnknownReason, ReasonIncludeNotFound)
	}
	if u := unitByPath(t, res, "live"); u.ConfigUnknownReason != ReasonIncludeTarget {
		t.Fatalf("live ConfigUnknownReason = %q, want %q", u.ConfigUnknownReason, ReasonIncludeTarget)
	}
}

func TestIncludeTargetPrecedence(t *testing.T) {
	res := loadUnits(t, filesFS(map[string]string{
		"live/terragrunt.hcl":     "locals {",
		"live/app/terragrunt.hcl": `include { path = "${get_repo_root()}/x.hcl" }`,
	}))
	if u := unitByPath(t, res, "live"); u.ConfigUnknownReason != ReasonSyntaxError {
		t.Fatalf("live ConfigUnknownReason = %q, want %q (its own earlier reason wins)", u.ConfigUnknownReason, ReasonSyntaxError)
	}
}
