package terragrunt

import (
	"context"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"slices"
	"strconv"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/GiulioSavini/gruntled/internal/application/ports"
	"github.com/GiulioSavini/gruntled/internal/domain/diagnostic"
	"github.com/GiulioSavini/gruntled/internal/domain/repograph"
)

// --- shared test helpers ----------------------------------------------

// loadUnits runs NewLoader(fsys).LoadUnits against a background context and
// fails the test on a Go-level error (which should never happen for these
// fixtures: every one of them is a per-unit/per-file problem, not a load
// failure).
func loadUnits(t *testing.T, fsys fs.FS) ports.LoadResult {
	t.Helper()
	res, err := NewLoader(fsys).LoadUnits(context.Background())
	if err != nil {
		t.Fatalf("LoadUnits: %v", err)
	}
	return res
}

// unitByPath returns the UnitConfig at dir, failing the test if it is not
// present in res.Units.
func unitByPath(t *testing.T, res ports.LoadResult, dir string) ports.UnitConfig {
	t.Helper()
	for _, u := range res.Units {
		if u.Path.String() == dir {
			return u
		}
	}
	t.Fatalf("unit %q not found in LoadResult.Units (have: %v)", dir, unitDirs(res))
	return ports.UnitConfig{}
}

func unitDirs(res ports.LoadResult) []string {
	dirs := make([]string, len(res.Units))
	for i, u := range res.Units {
		dirs[i] = u.Path.String()
	}
	return dirs
}

func findDep(deps []repograph.Dependency, name string) (repograph.Dependency, bool) {
	for _, d := range deps {
		if d.Name() == name {
			return d, true
		}
	}
	return repograph.Dependency{}, false
}

// readFailFS wraps an fstest.MapFS so that ReadFile fails for exactly one
// named file, while Stat (promoted, unmodified, from the embedded MapFS)
// still succeeds: this exercises ReasonUnreadableConfig, which requires a
// file that fs.Stat can see but fs.ReadFile cannot read (a dangling symlink
// would instead hit ReasonIncludeNotFound at the Stat step).
type readFailFS struct {
	fstest.MapFS
	failFile string
}

func (f readFailFS) ReadFile(name string) ([]byte, error) {
	if name == f.failFile {
		return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrPermission}
	}
	return f.MapFS.ReadFile(name)
}

// allReasonConstants parses reasons.go with go/parser and returns every
// exported Reason* constant's name and string value, so TestUnknownReasons
// can assert every one of them has a fixture, with no dead or untested
// reason going unnoticed.
func allReasonConstants(t *testing.T) map[string]string {
	t.Helper()
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "reasons.go", nil, 0)
	if err != nil {
		t.Fatalf("parsing reasons.go: %v", err)
	}
	out := map[string]string{}
	for _, decl := range f.Decls {
		gd, ok := decl.(*ast.GenDecl)
		if !ok || gd.Tok != token.CONST {
			continue
		}
		for _, spec := range gd.Specs {
			vs, ok := spec.(*ast.ValueSpec)
			if !ok {
				continue
			}
			for i, name := range vs.Names {
				if !strings.HasPrefix(name.Name, "Reason") {
					continue
				}
				if i >= len(vs.Values) {
					continue
				}
				lit, ok := vs.Values[i].(*ast.BasicLit)
				if !ok || lit.Kind != token.STRING {
					continue
				}
				val, err := strconv.Unquote(lit.Value)
				if err != nil {
					t.Fatalf("unquoting %s: %v", lit.Value, err)
				}
				out[name.Name] = val
			}
		}
	}
	if len(out) == 0 {
		t.Fatalf("no Reason* constants found in reasons.go: parser or path is wrong")
	}
	return out
}

// --- TestUnknownReasons -------------------------------------------------

type unknownReasonCase struct {
	name   string
	reason string // the Reason* value this row proves
	fsys   fs.FS
	unit   string
	check  func(t *testing.T, uc ports.UnitConfig)
}

func wantConfigUnknown(reason string) func(*testing.T, ports.UnitConfig) {
	return func(t *testing.T, uc ports.UnitConfig) {
		t.Helper()
		if uc.ConfigUnknownReason != reason {
			t.Fatalf("ConfigUnknownReason = %q, want %q (ModuleUnknownReason=%q)", uc.ConfigUnknownReason, reason, uc.ModuleUnknownReason)
		}
		if len(uc.Dependencies) != 0 {
			t.Fatalf("config-unknown unit has %d dependencies, want 0", len(uc.Dependencies))
		}
		if len(uc.References) != 0 {
			t.Fatalf("config-unknown unit has %d references, want 0", len(uc.References))
		}
	}
}

func wantModuleUnknown(reason string) func(*testing.T, ports.UnitConfig) {
	return func(t *testing.T, uc ports.UnitConfig) {
		t.Helper()
		if uc.ConfigUnknownReason != "" {
			t.Fatalf("ConfigUnknownReason = %q, want empty", uc.ConfigUnknownReason)
		}
		if uc.ModuleUnknownReason != reason {
			t.Fatalf("ModuleUnknownReason = %q, want %q", uc.ModuleUnknownReason, reason)
		}
	}
}

// wantDepUnresolved asserts the unit is fully resolved (neither
// config-unknown nor module-unknown), that its dependency depName is
// unresolved with reason, and that every dependency named in
// otherResolved is present and resolved -- proving an unresolved
// dependency's reason has a contained blast radius (research: "a remote
// source on one unit should make only that unit's module surface unknown,"
// and symmetrically here, one bad dependency must not poison its siblings).
func wantDepUnresolved(depName, reason string, otherResolved ...string) func(*testing.T, ports.UnitConfig) {
	return func(t *testing.T, uc ports.UnitConfig) {
		t.Helper()
		if uc.ConfigUnknownReason != "" || uc.ModuleUnknownReason != "" {
			t.Fatalf("unit unknown: config=%q module=%q, want fully resolved", uc.ConfigUnknownReason, uc.ModuleUnknownReason)
		}
		d, ok := findDep(uc.Dependencies, depName)
		if !ok {
			t.Fatalf("dependency %q not found on unit (have: %v)", depName, uc.Dependencies)
		}
		if _, ok := d.Target(); ok {
			t.Fatalf("dependency %q resolved, want unresolved (reason %q)", depName, reason)
		}
		if d.UnresolvedReason() != reason {
			t.Fatalf("dependency %q UnresolvedReason = %q, want %q", depName, d.UnresolvedReason(), reason)
		}
		for _, other := range otherResolved {
			od, ok := findDep(uc.Dependencies, other)
			if !ok {
				t.Fatalf("dependency %q not found on unit", other)
			}
			if _, ok := od.Target(); !ok {
				t.Fatalf("dependency %q unresolved (reason %q), want resolved", other, od.UnresolvedReason())
			}
		}
	}
}

func TestUnknownReasons(t *testing.T) {
	cases := []unknownReasonCase{
		{
			name:   "syntax-error/unit-file",
			reason: ReasonSyntaxError,
			fsys:   filesFS(map[string]string{"u/terragrunt.hcl": "locals {"}),
			unit:   "u",
			check:  wantConfigUnknown(ReasonSyntaxError),
		},
		{
			name:   "syntax-error/include-file",
			reason: ReasonSyntaxError,
			fsys: filesFS(map[string]string{
				"root.hcl":         "locals {",
				"u/terragrunt.hcl": `include "root" { path = "../root.hcl" }`,
			}),
			unit:  "u",
			check: wantConfigUnknown(ReasonSyntaxError),
		},
		{
			name:   "unreadable-config/include-file",
			reason: ReasonUnreadableConfig,
			fsys: readFailFS{
				MapFS: filesFS(map[string]string{
					"root.hcl":         "locals { a = 1 }",
					"u/terragrunt.hcl": `include "root" { path = "../root.hcl" }`,
				}),
				failFile: "root.hcl",
			},
			unit:  "u",
			check: wantConfigUnknown(ReasonUnreadableConfig),
		},
		{
			name:   "json-config-unsupported",
			reason: ReasonJSONConfigUnsupported,
			fsys:   filesFS(map[string]string{"u/terragrunt.hcl.json": `{}`}),
			unit:   "u",
			check:  wantConfigUnknown(ReasonJSONConfigUnsupported),
		},
		{
			name:   "autoinclude-unsupported",
			reason: ReasonAutoincludeUnsupported,
			fsys: filesFS(map[string]string{
				"u/terragrunt.hcl":             "",
				"u/terragrunt.autoinclude.hcl": "",
			}),
			unit:  "u",
			check: wantConfigUnknown(ReasonAutoincludeUnsupported),
		},
		{
			name:   "invalid-include/duplicate-labels",
			reason: ReasonInvalidInclude,
			fsys: filesFS(map[string]string{
				"root.hcl": "",
				"u/terragrunt.hcl": `
include "a" { path = "../root.hcl" }
include "a" { path = "../root.hcl" }
`,
			}),
			unit:  "u",
			check: wantConfigUnknown(ReasonInvalidInclude),
		},
		{
			name:   "invalid-include/two-bare",
			reason: ReasonInvalidInclude,
			fsys: filesFS(map[string]string{
				"root.hcl": "",
				"u/terragrunt.hcl": `
include { path = "../root.hcl" }
include { path = "../root.hcl" }
`,
			}),
			unit:  "u",
			check: wantConfigUnknown(ReasonInvalidInclude),
		},
		{
			name:   "invalid-include/merge-strategy-not-recognized",
			reason: ReasonInvalidInclude,
			fsys: filesFS(map[string]string{
				"root.hcl": "",
				"u/terragrunt.hcl": `
include "root" {
  path            = "../root.hcl"
  merge_strategy  = "deep_map_only"
}
`,
			}),
			unit:  "u",
			check: wantConfigUnknown(ReasonInvalidInclude),
		},
		{
			name:   "invalid-include/merge-strategy-not-literal",
			reason: ReasonInvalidInclude,
			fsys: filesFS(map[string]string{
				"root.hcl": "",
				"u/terragrunt.hcl": `
locals { s = "shallow" }
include "root" {
  path            = "../root.hcl"
  merge_strategy  = local.s
}
`,
			}),
			unit:  "u",
			check: wantConfigUnknown(ReasonInvalidInclude),
		},
		{
			name:   "invalid-include/missing-path",
			reason: ReasonInvalidInclude,
			fsys: filesFS(map[string]string{
				"u/terragrunt.hcl": `include "root" {}`,
			}),
			unit:  "u",
			check: wantConfigUnknown(ReasonInvalidInclude),
		},
		{
			name:   "invalid-include/self-inclusion",
			reason: ReasonInvalidInclude,
			fsys: filesFS(map[string]string{
				"u/terragrunt.hcl": `include "self" { path = "terragrunt.hcl" }`,
			}),
			unit:  "u",
			check: wantConfigUnknown(ReasonInvalidInclude),
		},
		{
			name:   "include-dynamic-path/get_env",
			reason: ReasonIncludeDynamicPath,
			fsys: filesFS(map[string]string{
				"u/terragrunt.hcl": `include "root" { path = get_env("ROOT") }`,
			}),
			unit:  "u",
			check: wantConfigUnknown(ReasonIncludeDynamicPath),
		},
		{
			name:   "include-dynamic-path/find-in-parent-folders-exhausted",
			reason: ReasonIncludeDynamicPath,
			fsys: filesFS(map[string]string{
				"u/terragrunt.hcl": `include "root" { path = find_in_parent_folders("nope.hcl") }`,
			}),
			unit:  "u",
			check: wantConfigUnknown(ReasonIncludeDynamicPath),
		},
		{
			name:   "include-outside-repo",
			reason: ReasonIncludeOutsideRepo,
			fsys: filesFS(map[string]string{
				"u/terragrunt.hcl": `include "root" { path = "../../../x.hcl" }`,
			}),
			unit:  "u",
			check: wantConfigUnknown(ReasonIncludeOutsideRepo),
		},
		{
			name:   "include-not-found/missing-file",
			reason: ReasonIncludeNotFound,
			fsys: filesFS(map[string]string{
				"u/terragrunt.hcl": `include "root" { path = "../nope.hcl" }`,
			}),
			unit:  "u",
			check: wantConfigUnknown(ReasonIncludeNotFound),
		},
		{
			name:   "include-not-found/path-is-directory",
			reason: ReasonIncludeNotFound,
			fsys: filesFS(map[string]string{
				"adir/x.txt":       "x",
				"u/terragrunt.hcl": `include "root" { path = "../adir" }`,
			}),
			unit:  "u",
			check: wantConfigUnknown(ReasonIncludeNotFound),
		},
		{
			name:   "nested-include",
			reason: ReasonNestedInclude,
			fsys: filesFS(map[string]string{
				"root.hcl":         `include "gp" { path = "grandparent.hcl" }`,
				"u/terragrunt.hcl": `include "root" { path = "../root.hcl" }`,
			}),
			unit:  "u",
			check: wantConfigUnknown(ReasonNestedInclude),
		},
		{
			name:   "invalid-terraform-block",
			reason: ReasonInvalidTerraformBlock,
			fsys: filesFS(map[string]string{
				"u/terragrunt.hcl": `
terraform { source = "../a" }
terraform { source = "../b" }
`,
			}),
			unit:  "u",
			check: wantConfigUnknown(ReasonInvalidTerraformBlock),
		},
		{
			name:   "invalid-dependency/duplicate-label-in-file",
			reason: ReasonInvalidDependency,
			fsys: filesFS(map[string]string{
				"u/terragrunt.hcl": `
dependency "vpc" { config_path = "../a" }
dependency "vpc" { config_path = "../b" }
`,
			}),
			unit:  "u",
			check: wantConfigUnknown(ReasonInvalidDependency),
		},
		{
			name:   "invalid-dependency/expansion-block",
			reason: ReasonInvalidDependency,
			fsys: filesFS(map[string]string{
				"u/terragrunt.hcl": `
dependency "vpc" {
  config_path = "../a"
  expansion {
    for_each = []
  }
}
`,
			}),
			unit:  "u",
			check: wantConfigUnknown(ReasonInvalidDependency),
		},
		{
			name:   "invalid-dependency/unlabeled",
			reason: ReasonInvalidDependency,
			fsys: filesFS(map[string]string{
				"u/terragrunt.hcl": `dependency { config_path = "../a" }`,
			}),
			unit:  "u",
			check: wantConfigUnknown(ReasonInvalidDependency),
		},
		{
			name:   "invalid-dependency/missing-config-path-after-merge",
			reason: ReasonInvalidDependency,
			fsys: filesFS(map[string]string{
				"u/terragrunt.hcl": `dependency "vpc" {}`,
			}),
			unit:  "u",
			check: wantConfigUnknown(ReasonInvalidDependency),
		},
		{
			name:   "source-dynamic-path",
			reason: ReasonSourceDynamicPath,
			fsys: filesFS(map[string]string{
				"u/terragrunt.hcl": `
locals { base = "../../modules" }
terraform { source = "${local.base}/vpc" }
`,
			}),
			unit:  "u",
			check: wantModuleUnknown(ReasonSourceDynamicPath),
		},
		{
			name:   "source-outside-repo",
			reason: ReasonSourceOutsideRepo,
			fsys: filesFS(map[string]string{
				"u/terragrunt.hcl": `terraform { source = "/opt/shared-modules/vpc" }`,
			}),
			unit:  "u",
			check: wantModuleUnknown(ReasonSourceOutsideRepo),
		},
		{
			name:   "remote-source",
			reason: ReasonRemoteSource,
			fsys: filesFS(map[string]string{
				"u/terragrunt.hcl": `terraform { source = "git::https://example.com/modules.git//vpc?ref=v1" }`,
			}),
			unit:  "u",
			check: wantModuleUnknown(ReasonRemoteSource),
		},
		{
			name:   "invalid-source",
			reason: ReasonInvalidSource,
			fsys: filesFS(map[string]string{
				"u/terragrunt.hcl": `terraform { source = "../modules/vpc?ref=1" }`,
			}),
			unit:  "u",
			check: wantModuleUnknown(ReasonInvalidSource),
		},
		{
			name:   "generate-may-declare-outputs",
			reason: ReasonGenerateMayDeclareOutputs,
			fsys: filesFS(map[string]string{
				"u/terragrunt.hcl": `
generate "provider" {
  path      = "provider.tf"
  if_exists = "overwrite"
  contents  = <<-EOF
    output "x" {
      value = 1
    }
  EOF
}
`,
			}),
			unit:  "u",
			check: wantModuleUnknown(ReasonGenerateMayDeclareOutputs),
		},
		{
			name:   "unit-dir-overlays-module",
			reason: ReasonUnitDirOverlaysModule,
			fsys: filesFS(map[string]string{
				"u/terragrunt.hcl":    `terraform { source = "../modules/vpc" }`,
				"u/extra.tf":          `variable "x" {}`,
				"modules/vpc/main.tf": `variable "y" {}`,
			}),
			unit:  "u",
			check: wantModuleUnknown(ReasonUnitDirOverlaysModule),
		},
		{
			name:   "config-path-dynamic",
			reason: ReasonConfigPathDynamic,
			fsys: filesFS(map[string]string{
				"u/terragrunt.hcl": `
locals { p = "../vpc" }
dependency "bad" { config_path = local.p }
dependency "good" { config_path = "../vpc" }
`,
				"vpc/terragrunt.hcl": "",
			}),
			unit:  "u",
			check: wantDepUnresolved("bad", ReasonConfigPathDynamic, "good"),
		},
		{
			name:   "config-path-outside-repo",
			reason: ReasonConfigPathOutsideRepo,
			fsys: filesFS(map[string]string{
				"u/terragrunt.hcl": `
dependency "bad" { config_path = "../../../../vpc" }
dependency "good" { config_path = "../vpc" }
`,
				"vpc/terragrunt.hcl": "",
			}),
			unit:  "u",
			check: wantDepUnresolved("bad", ReasonConfigPathOutsideRepo, "good"),
		},
		{
			name:   "config-path-stack/dir-only-stack",
			reason: ReasonConfigPathStack,
			fsys: filesFS(map[string]string{
				"u/terragrunt.hcl": `
dependency "bad" { config_path = "../stk" }
dependency "good" { config_path = "../vpc" }
`,
				"stk/terragrunt.stack.hcl": "",
				"vpc/terragrunt.hcl":       "",
			}),
			unit:  "u",
			check: wantDepUnresolved("bad", ReasonConfigPathStack, "good"),
		},
		{
			name:   "config-path-stack/dir-with-both",
			reason: ReasonConfigPathStack,
			fsys: filesFS(map[string]string{
				"u/terragrunt.hcl": `
dependency "bad" { config_path = "../stk2" }
dependency "good" { config_path = "../vpc" }
`,
				"stk2/terragrunt.stack.hcl": "",
				"stk2/terragrunt.hcl":       "",
				"vpc/terragrunt.hcl":        "",
			}),
			unit:  "u",
			check: wantDepUnresolved("bad", ReasonConfigPathStack, "good"),
		},
		{
			name:   "config-path-stack/file",
			reason: ReasonConfigPathStack,
			fsys: filesFS(map[string]string{
				"u/terragrunt.hcl": `
dependency "bad" { config_path = "../stk/terragrunt.stack.hcl" }
dependency "good" { config_path = "../vpc" }
`,
				"stk/terragrunt.stack.hcl": "",
				"vpc/terragrunt.hcl":       "",
			}),
			unit:  "u",
			check: wantDepUnresolved("bad", ReasonConfigPathStack, "good"),
		},
		{
			name:   "config-path-nondefault-file/named-file",
			reason: ReasonConfigPathNondefaultFile,
			fsys: filesFS(map[string]string{
				"u/terragrunt.hcl": `
dependency "bad" { config_path = "../vpc/alt.hcl" }
dependency "good" { config_path = "../vpc" }
`,
				"vpc/terragrunt.hcl": "",
				"vpc/alt.hcl":        "",
			}),
			unit:  "u",
			check: wantDepUnresolved("bad", ReasonConfigPathNondefaultFile, "good"),
		},
		{
			name:   "config-path-nondefault-file/json-config",
			reason: ReasonConfigPathNondefaultFile,
			fsys: filesFS(map[string]string{
				"u/terragrunt.hcl": `
dependency "bad" { config_path = "../vpc/terragrunt.hcl.json" }
dependency "good" { config_path = "../vpc" }
`,
				"vpc/terragrunt.hcl":      "",
				"vpc/terragrunt.hcl.json": `{}`,
			}),
			unit:  "u",
			check: wantDepUnresolved("bad", ReasonConfigPathNondefaultFile, "good"),
		},
		{
			name:   "config-path-invalid",
			reason: ReasonConfigPathInvalid,
			fsys: filesFS(map[string]string{
				"u/terragrunt.hcl": `
dependency "bad" { config_path = "..\\vpc" }
dependency "good" { config_path = "../vpc" }
`,
				"vpc/terragrunt.hcl": "",
			}),
			unit:  "u",
			check: wantDepUnresolved("bad", ReasonConfigPathInvalid, "good"),
		},
		{
			name:   "config-path-default-file-regression",
			reason: ReasonConfigPathOutsideRepo, // regression only; reuses an already-covered reason
			fsys: filesFS(map[string]string{
				"u/terragrunt.hcl":   `dependency "vpc" { config_path = "../vpc/terragrunt.hcl" }`,
				"vpc/terragrunt.hcl": "",
			}),
			unit: "u",
			check: func(t *testing.T, uc ports.UnitConfig) {
				t.Helper()
				d, ok := findDep(uc.Dependencies, "vpc")
				if !ok {
					t.Fatalf("dependency %q not found", "vpc")
				}
				target, ok := d.Target()
				if !ok || target.String() != "vpc" {
					t.Fatalf("Target() = (%q, %v), want (%q, true) (config_path naming the default terragrunt.hcl still maps to its directory)", target.String(), ok, "vpc")
				}
			},
		},
	}

	covered := map[string]bool{}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res := loadUnits(t, tc.fsys)
			uc := unitByPath(t, res, tc.unit)
			tc.check(t, uc)
		})
		covered[tc.reason] = true
	}

	for name, val := range allReasonConstants(t) {
		if !covered[val] {
			t.Errorf("reason constant %s = %q has no fixture row in TestUnknownReasons", name, val)
		}
	}
}

// --- TestStructuralOnly ---------------------------------------------------

// TestStructuralOnly proves PARSE-01/INC-11: a construct outside the three
// path-bearing attributes never makes a unit unknown, no matter which
// function it calls or how deeply it is nested, because gruntled only ever
// decodes include/terraform.source/dependency blocks and never evaluates
// locals, inputs or remote_state.
func TestStructuralOnly(t *testing.T) {
	fsys := filesFS(map[string]string{
		"u/terragrunt.hcl": `
locals {
  a = run_cmd("echo", "x")
  b = get_env("Y")
  c = read_terragrunt_config("x.hcl")
}
inputs = {
  d = sops_decrypt_file("s")
}
remote_state {
  backend = "s3"
  config = {
    bucket = get_env("BUCKET")
  }
}
`,
	})
	uc := unitByPath(t, loadUnits(t, fsys), "u")
	if uc.ConfigUnknownReason != "" {
		t.Fatalf("ConfigUnknownReason = %q, want empty", uc.ConfigUnknownReason)
	}
	if uc.ModuleUnknownReason != "" {
		t.Fatalf("ModuleUnknownReason = %q, want empty", uc.ModuleUnknownReason)
	}
	if uc.Module.String() != "u" {
		t.Fatalf("Module = %q, want %q (GRAPH-02: no source => unit dir)", uc.Module.String(), "u")
	}
}

// --- TestSource: GRAPH-01/02/03 forms -------------------------------------

func TestSourceForms(t *testing.T) {
	t.Run("empty-config-is-graph-02", func(t *testing.T) {
		uc := unitByPath(t, loadUnits(t, filesFS(map[string]string{"u/terragrunt.hcl": ""})), "u")
		if uc.ConfigUnknownReason != "" || uc.ModuleUnknownReason != "" {
			t.Fatalf("unit unknown: config=%q module=%q", uc.ConfigUnknownReason, uc.ModuleUnknownReason)
		}
		if uc.Module.String() != "u" {
			t.Fatalf("Module = %q, want %q", uc.Module.String(), "u")
		}
	})

	t.Run("local-relative-path", func(t *testing.T) {
		fsys := filesFS(map[string]string{
			"u/terragrunt.hcl": `terraform { source = "../modules/vpc" }`,
		})
		uc := unitByPath(t, loadUnits(t, fsys), "u")
		if uc.ModuleUnknownReason != "" || uc.Module.String() != "modules/vpc" {
			t.Fatalf("Module = %q (unknown=%q), want %q", uc.Module.String(), uc.ModuleUnknownReason, "modules/vpc")
		}
	})

	t.Run("double-slash-subdir", func(t *testing.T) {
		fsys := filesFS(map[string]string{
			"live/app/terragrunt.hcl": `terraform { source = "../../aws//lambda" }`,
		})
		uc := unitByPath(t, loadUnits(t, fsys), "live/app")
		if uc.ModuleUnknownReason != "" || uc.Module.String() != "aws/lambda" {
			t.Fatalf("Module = %q (unknown=%q), want %q", uc.Module.String(), uc.ModuleUnknownReason, "aws/lambda")
		}
	})

	t.Run("get-terragrunt-dir-composition", func(t *testing.T) {
		fsys := filesFS(map[string]string{
			"live/app/terragrunt.hcl": `terraform { source = "${get_terragrunt_dir()}/../../modules/vpc" }`,
		})
		uc := unitByPath(t, loadUnits(t, fsys), "live/app")
		if uc.ModuleUnknownReason != "" || uc.Module.String() != "modules/vpc" {
			t.Fatalf("Module = %q (unknown=%q), want %q", uc.Module.String(), uc.ModuleUnknownReason, "modules/vpc")
		}
	})

	t.Run("get-parent-terragrunt-dir-in-include", func(t *testing.T) {
		fsys := filesFS(map[string]string{
			"root.hcl":         `terraform { source = "${get_parent_terragrunt_dir()}/modules/vpc" }`,
			"u/terragrunt.hcl": `include "root" { path = "../root.hcl" }`,
		})
		uc := unitByPath(t, loadUnits(t, fsys), "u")
		if uc.ModuleUnknownReason != "" || uc.Module.String() != "modules/vpc" {
			t.Fatalf("Module = %q (unknown=%q), want %q", uc.Module.String(), uc.ModuleUnknownReason, "modules/vpc")
		}
	})

	t.Run("dot-source-is-unit-dir", func(t *testing.T) {
		fsys := filesFS(map[string]string{
			"u/terragrunt.hcl": `terraform { source = "." }`,
		})
		uc := unitByPath(t, loadUnits(t, fsys), "u")
		if uc.ModuleUnknownReason != "" || uc.Module.String() != "u" {
			t.Fatalf("Module = %q (unknown=%q), want %q", uc.Module.String(), uc.ModuleUnknownReason, "u")
		}
	})

	t.Run("registry-looking-source-resolves-to-local-path", func(t *testing.T) {
		fsys := filesFS(map[string]string{
			"u/terragrunt.hcl": `terraform { source = "terraform-aws-modules/vpc/aws" }`,
		})
		uc := unitByPath(t, loadUnits(t, fsys), "u")
		want := "u/terraform-aws-modules/vpc/aws"
		if uc.ModuleUnknownReason != "" || uc.Module.String() != want {
			t.Fatalf("Module = %q (unknown=%q), want %q (nonexistent, the surface reader makes it unknown later)", uc.Module.String(), uc.ModuleUnknownReason, want)
		}
	})
}

// --- TestInclude: precedence, no_merge, include-declared dependency ------

func TestIncludePrecedence(t *testing.T) {
	t.Run("child-beats-include", func(t *testing.T) {
		fsys := filesFS(map[string]string{
			"root.hcl": `terraform { source = "../root-module" }`,
			"u/terragrunt.hcl": `
include "root" { path = "../root.hcl" }
terraform { source = "../child-module" }
`,
		})
		uc := unitByPath(t, loadUnits(t, fsys), "u")
		if uc.Module.String() != "child-module" {
			t.Fatalf("Module = %q, want %q (child must beat include)", uc.Module.String(), "child-module")
		}
	})

	t.Run("last-include-beats-first", func(t *testing.T) {
		fsys := filesFS(map[string]string{
			"a.hcl": `terraform { source = "../module-a" }`,
			"b.hcl": `terraform { source = "../module-b" }`,
			"u/terragrunt.hcl": `
include "a" { path = "../a.hcl" }
include "b" { path = "../b.hcl" }
`,
		})
		uc := unitByPath(t, loadUnits(t, fsys), "u")
		if uc.Module.String() != "module-b" {
			t.Fatalf("Module = %q, want %q (last-declared include must win)", uc.Module.String(), "module-b")
		}
	})

	t.Run("empty-terraform-block-does-not-override", func(t *testing.T) {
		fsys := filesFS(map[string]string{
			"root.hcl": `terraform { source = "../root-module" }`,
			"u/terragrunt.hcl": `
include "root" { path = "../root.hcl" }
terraform {}
`,
		})
		uc := unitByPath(t, loadUnits(t, fsys), "u")
		if uc.Module.String() != "root-module" {
			t.Fatalf("Module = %q, want %q (a sourceless terraform{} must not override the include's source)", uc.Module.String(), "root-module")
		}
	})

	t.Run("shared-include-at-different-depths", func(t *testing.T) {
		fsys := filesFS(map[string]string{
			"env/region.hcl":        `terraform { source = "../modules/vpc" }`,
			"u1/terragrunt.hcl":     `include "region" { path = "../env/region.hcl" }`,
			"grp/u2/terragrunt.hcl": `include "region" { path = "../../env/region.hcl" }`,
		})
		res := loadUnits(t, fsys)
		u1 := unitByPath(t, res, "u1")
		u2 := unitByPath(t, res, "grp/u2")
		if u1.Module.String() != "modules/vpc" {
			t.Fatalf("u1 Module = %q, want %q", u1.Module.String(), "modules/vpc")
		}
		if u2.Module.String() != "grp/modules/vpc" {
			t.Fatalf("u2 Module = %q, want %q (config_path/source resolve against the CHILD dir, not the shared include's own dir)", u2.Module.String(), "grp/modules/vpc")
		}
	})

	t.Run("no-merge-contributes-nothing-but-is-still-a-visible-include", func(t *testing.T) {
		fsys := filesFS(map[string]string{
			"root.hcl": `
terraform { source = "../root-module" }
dependency "shared" { config_path = "../shared" }
generate "prov" {
  path      = "p.tf"
  if_exists = "overwrite"
  contents  = "provider \"aws\" {}"
}
locals { z = dependency.shared.outputs.zzz }
`,
			"u/terragrunt.hcl": `
include "root" {
  path           = "../root.hcl"
  merge_strategy = "no_merge"
}
`,
			"root-module/main.tf":   "",
			"shared/terragrunt.hcl": "",
		})
		uc := unitByPath(t, loadUnits(t, fsys), "u")
		if uc.ConfigUnknownReason != "" || uc.ModuleUnknownReason != "" {
			t.Fatalf("unit unknown: config=%q module=%q", uc.ConfigUnknownReason, uc.ModuleUnknownReason)
		}
		if uc.Module.String() != "u" {
			t.Fatalf("Module = %q, want %q (no_merge include's source must not merge in)", uc.Module.String(), "u")
		}
		if len(uc.Dependencies) != 0 {
			t.Fatalf("Dependencies = %v, want none (no_merge include's dependency must not merge in)", uc.Dependencies)
		}
		if len(uc.References) != 0 {
			t.Fatalf("References = %v, want none (no_merge include's references must not merge in)", uc.References)
		}

		// path_relative_to_include must still see the no_merge include:
		// selecting it by label must succeed (not fail closed to
		// source-dynamic-path), proving the six functions' includes list is
		// built from every resolved include, no_merge or not.
		fsys2 := filesFS(map[string]string{
			"g1/root.hcl":   "",
			"g1/region.hcl": "",
			"g1/u/terragrunt.hcl": `
include "root" {
  path           = "../root.hcl"
  merge_strategy = "no_merge"
}
include "region" { path = "../region.hcl" }
terraform { source = "${path_relative_to_include("root")}/modvia" }
`,
		})
		uc2 := unitByPath(t, loadUnits(t, fsys2), "g1/u")
		if uc2.ModuleUnknownReason != "" {
			t.Fatalf("ModuleUnknownReason = %q, want empty (path_relative_to_include(\"root\") must resolve even though root is no_merge)", uc2.ModuleUnknownReason)
		}
		want := "g1/u/u/modvia"
		if uc2.Module.String() != want {
			t.Fatalf("Module = %q, want %q", uc2.Module.String(), want)
		}
	})

	t.Run("include-declared-dependency-resolves-against-child", func(t *testing.T) {
		// research Pitfall 4 / INC-12: a dependency block written in a
		// shared include (_envcommon-style) must resolve its config_path
		// against the CHILD unit's directory, never the include file's own
		// directory.
		fsys := filesFS(map[string]string{
			"envcommon.hcl":           `dependency "vpc" { config_path = "../vpc" }`,
			"live/app/terragrunt.hcl": `include "common" { path = "../../envcommon.hcl" }`,
			"live/vpc/terragrunt.hcl": "",
		})
		uc := unitByPath(t, loadUnits(t, fsys), "live/app")
		d, ok := findDep(uc.Dependencies, "vpc")
		if !ok {
			t.Fatalf("dependency %q not found", "vpc")
		}
		target, ok := d.Target()
		if !ok || target.String() != "live/vpc" {
			t.Fatalf("Target() = (%q, %v), want (%q, true)", target.String(), ok, "live/vpc")
		}
	})
}

// --- TestDependency: merge, target forms, references ----------------------

func TestDependencyMerge(t *testing.T) {
	t.Run("shallow-child-replaces-whole-block", func(t *testing.T) {
		fsys := filesFS(map[string]string{
			"root.hcl": "dependency \"vpc\" {\n  config_path  = \"../root-vpc\"\n  mock_outputs = { a = 1 }\n}\n",
			"u/terragrunt.hcl": `
include "root" { path = "../root.hcl" }
dependency "vpc" { config_path = "../child-vpc" }
`,
			"child-vpc/terragrunt.hcl": "",
			"root-vpc/terragrunt.hcl":  "",
		})
		uc := unitByPath(t, loadUnits(t, fsys), "u")
		d, ok := findDep(uc.Dependencies, "vpc")
		if !ok {
			t.Fatalf("dependency %q not found", "vpc")
		}
		target, ok := d.Target()
		if !ok || target.String() != "child-vpc" {
			t.Fatalf("Target() = (%q, %v), want (%q, true) (child must win wholly)", target.String(), ok, "child-vpc")
		}
		if !d.Options().MockOutputs.IsAbsent() {
			t.Fatalf("Options().MockOutputs = %v, want Absent (child's own block has no mock_outputs; the include's must not leak in under shallow merge)", d.Options().MockOutputs)
		}
	})

	t.Run("deep-merge-single-occurrence-keeps-literal-facts", func(t *testing.T) {
		fsys := filesFS(map[string]string{
			"deep-root.hcl":      "dependency \"shared_kms\" {\n  config_path  = \"../kms\"\n  mock_outputs = { arn = \"x\" }\n}\n",
			"u/terragrunt.hcl":   "include \"root\" {\n  path           = \"../deep-root.hcl\"\n  merge_strategy = \"deep\"\n}\n",
			"kms/terragrunt.hcl": "",
		})
		uc := unitByPath(t, loadUnits(t, fsys), "u")
		d, ok := findDep(uc.Dependencies, "shared_kms")
		if !ok {
			t.Fatalf("dependency %q not found", "shared_kms")
		}
		target, ok := d.Target()
		if !ok || target.String() != "kms" {
			t.Fatalf("Target() = (%q, %v), want (%q, true)", target.String(), ok, "kms")
		}
		names, ok := d.Options().MockOutputs.Names()
		if !ok || !slices.Equal(names, []string{"arn"}) {
			t.Fatalf("Options().MockOutputs.Names() = (%v, %v), want ([arn], true) (a label in only one file keeps its literal facts)", names, ok)
		}
	})

	t.Run("deep-merge-multi-occurrence-config-path-precedence-and-unknown-opts", func(t *testing.T) {
		fsys := filesFS(map[string]string{
			"deep-root2.hcl": "dependency \"vpc\" {\n  config_path  = \"../root-vpc2\"\n  mock_outputs = { a = 1 }\n}\n",
			"u/terragrunt.hcl": `
include "root" {
  path           = "../deep-root2.hcl"
  merge_strategy = "deep"
}
dependency "vpc" { enabled = false }
`,
			"root-vpc2/terragrunt.hcl": "",
		})
		uc := unitByPath(t, loadUnits(t, fsys), "u")
		d, ok := findDep(uc.Dependencies, "vpc")
		if !ok {
			t.Fatalf("dependency %q not found", "vpc")
		}
		target, ok := d.Target()
		if !ok || target.String() != "root-vpc2" {
			t.Fatalf("Target() = (%q, %v), want (%q, true) (config_path from the highest-precedence block that SETS it)", target.String(), ok, "root-vpc2")
		}
		opts := d.Options()
		if opts.Enabled != repograph.TristateUnknown || opts.SkipOutputs != repograph.TristateUnknown ||
			!opts.MockOutputs.IsUnknown() || opts.MockMergeWithState != repograph.TristateUnknown ||
			!opts.MockAllowedCommands.IsUnknown() {
			t.Fatalf("Options() = %+v, want the all-unknown zero value (label appears in more than one file under deep merge)", opts)
		}
	})
}

func TestDependencyTarget(t *testing.T) {
	fsys := filesFS(map[string]string{
		"live/app1/terragrunt.hcl": `dependency "vpc" { config_path = "../vpc" }`,
		"live/app2/terragrunt.hcl": `dependency "vpc" { config_path = "../vpc/terragrunt.hcl" }`,
		"live/app3/terragrunt.hcl": `dependency "ghost" { config_path = "../ghost" }`,
		"live/app4/terragrunt.hcl": `dependency "self" { config_path = "." }`,
		"live/vpc/terragrunt.hcl":  "",
	})
	res := loadUnits(t, fsys)

	cases := []struct {
		unit, dep, wantTarget string
	}{
		{"live/app1", "vpc", "live/vpc"},
		{"live/app2", "vpc", "live/vpc"},
		{"live/app3", "ghost", "live/ghost"}, // DEP-04: kept even though nonexistent
		{"live/app4", "self", "live/app4"},   // DEP-11: self-reference, no hang
	}
	for _, tc := range cases {
		t.Run(tc.unit+"/"+tc.dep, func(t *testing.T) {
			uc := unitByPath(t, res, tc.unit)
			d, ok := findDep(uc.Dependencies, tc.dep)
			if !ok {
				t.Fatalf("dependency %q not found", tc.dep)
			}
			target, ok := d.Target()
			if !ok || target.String() != tc.wantTarget {
				t.Fatalf("Target() = (%q, %v), want (%q, true)", target.String(), ok, tc.wantTarget)
			}
		})
	}
}

func TestDependencyReferences(t *testing.T) {
	fsys := filesFS(map[string]string{
		"live/refroot.hcl": `
locals {
  x = dependency.vpc.outputs.vpc_id
}
`,
		"live/refapp/terragrunt.hcl": `
include "root" { path = "../refroot.hcl" }
dependency "vpc" { config_path = "../refvpc" }
inputs = {
  subnet     = dependency.vpc.outputs.subnet_id
  undeclared = dependency.ghost.outputs.y
}
dependencies {
  paths = ["../refvpc"]
}
`,
		"live/refvpc/terragrunt.hcl": "",
	})
	uc := unitByPath(t, loadUnits(t, fsys), "live/refapp")

	if len(uc.References) != 3 {
		t.Fatalf("len(References) = %d, want 3: %+v", len(uc.References), uc.References)
	}

	type gotRef struct {
		dep, out, file string
	}
	got := make([]gotRef, len(uc.References))
	for i, r := range uc.References {
		got[i] = gotRef{dep: r.Dependency(), out: r.Output(), file: r.Pos().File().String()}
	}

	wantAppFile := "live/refapp/terragrunt.hcl"
	wantRootFile := "live/refroot.hcl"
	foundSubnet, foundGhost, foundVPCID := false, false, false
	for _, r := range got {
		switch {
		case r.dep == "vpc" && r.out == "subnet_id":
			foundSubnet = true
			if r.file != wantAppFile {
				t.Errorf("subnet_id ref file = %q, want %q", r.file, wantAppFile)
			}
		case r.dep == "ghost" && r.out == "y":
			foundGhost = true
			if r.file != wantAppFile {
				t.Errorf("ghost ref file = %q, want %q", r.file, wantAppFile)
			}
		case r.dep == "vpc" && r.out == "vpc_id":
			foundVPCID = true
			if r.file != wantRootFile {
				t.Errorf("vpc_id ref file = %q, want %q", r.file, wantRootFile)
			}
		}
	}
	if !foundSubnet {
		t.Errorf("missing reference (vpc, subnet_id) from the unit's own inputs")
	}
	if !foundGhost {
		t.Errorf("missing reference (ghost, y): a reference to an undeclared dependency label must still be kept (DEP-12)")
	}
	if !foundVPCID {
		t.Errorf("missing reference (vpc, vpc_id) from the merged include's locals block")
	}

	// dependencies{ paths = [...] } must add no dependency at all.
	if _, ok := findDep(uc.Dependencies, "refvpc"); ok {
		t.Errorf("dependencies{paths=[...]} must not create a dependency block")
	}
	if len(uc.Dependencies) != 1 {
		t.Errorf("len(Dependencies) = %d, want 1 (only the explicit dependency \"vpc\" block)", len(uc.Dependencies))
	}
}

// --- TestLoader: parse-once, shared diagnostics, sort order, cancellation -

func TestLoaderParseOnceIncludes(t *testing.T) {
	files := map[string]string{
		"root.hcl":   `inputs = {}`,
		"region.hcl": `inputs = {}`,
	}
	for i := 0; i < 10; i++ {
		dir := fmt.Sprintf("u%02d", i)
		files[dir+"/terragrunt.hcl"] = `
include "root" { path = "../root.hcl" }
include "region" { path = "../region.hcl" }
`
	}
	cfs := newCountingFS(filesFS(files))
	res, err := NewLoader(cfs).LoadUnits(context.Background())
	if err != nil {
		t.Fatalf("LoadUnits: %v", err)
	}
	if len(res.Units) != 10 {
		t.Fatalf("len(Units) = %d, want 10", len(res.Units))
	}
	if got := cfs.count("root.hcl"); got != 1 {
		t.Errorf("count(root.hcl) = %d, want 1", got)
	}
	if got := cfs.count("region.hcl"); got != 1 {
		t.Errorf("count(region.hcl) = %d, want 1", got)
	}
	for i := 0; i < 10; i++ {
		dir := fmt.Sprintf("u%02d", i)
		name := dir + "/terragrunt.hcl"
		if got := cfs.count(name); got != 1 {
			t.Errorf("count(%s) = %d, want 1", name, got)
		}
	}
}

func TestLoaderSharedBrokenIncludeDiagnostics(t *testing.T) {
	fsys := filesFS(map[string]string{
		"broken.hcl":        "locals {",
		"u1/terragrunt.hcl": `include "root" { path = "../broken.hcl" }`,
		"u2/terragrunt.hcl": `include "root" { path = "../broken.hcl" }`,
		"u3/terragrunt.hcl": `include "root" { path = "../broken.hcl" }`,
	})
	res := loadUnits(t, fsys)

	for _, dir := range []string{"u1", "u2", "u3"} {
		uc := unitByPath(t, res, dir)
		if uc.ConfigUnknownReason != ReasonSyntaxError {
			t.Errorf("%s ConfigUnknownReason = %q, want %q", dir, uc.ConfigUnknownReason, ReasonSyntaxError)
		}
	}
	if len(res.Diagnostics) != 1 {
		t.Fatalf("len(Diagnostics) = %d, want exactly 1 (one GRT100 for the shared broken include): %+v", len(res.Diagnostics), res.Diagnostics)
	}
	if res.Diagnostics[0].Code() != diagnostic.CodeSyntaxError {
		t.Errorf("Diagnostics[0].Code() = %v, want %v", res.Diagnostics[0].Code(), diagnostic.CodeSyntaxError)
	}
}

func TestLoaderUnitsSortedByRepoPath(t *testing.T) {
	fsys := filesFS(map[string]string{
		"a/terragrunt.hcl":   "",
		"a-b/terragrunt.hcl": "",
		"a/b/terragrunt.hcl": "",
	})
	res := loadUnits(t, fsys)
	got := unitDirs(res)
	want := []string{"a", "a-b", "a/b"}
	if !slices.Equal(got, want) {
		t.Fatalf("unit order = %v, want %v (RepoPath.Compare order, not fs.WalkDir's depth-first order)", got, want)
	}
}

func TestLoaderCancelledContext(t *testing.T) {
	fsys := filesFS(map[string]string{"u/terragrunt.hcl": ""})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := NewLoader(fsys).LoadUnits(ctx); err == nil {
		t.Fatalf("LoadUnits with a cancelled context: want an error, got nil")
	}
}
