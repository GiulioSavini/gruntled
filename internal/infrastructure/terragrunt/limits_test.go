package terragrunt

import (
	"strings"
	"testing"

	"github.com/GiulioSavini/gruntled/internal/domain/repograph"
	"github.com/GiulioSavini/gruntled/internal/infrastructure/tfsurface"
)

// crashDepth is the 02-REVIEW G7 crash shape's nesting depth. It is far
// above the ~60k levels at which hclsyntax's recursive descent overflows
// Go's stack (02-10 planning probe), yet small enough (a few hundred KB of
// source) to keep the test binary's memory sane.
const crashDepth = 200_000

// deepParens returns inner wrapped in n levels of parentheses.
func deepParens(n int, inner string) string {
	return strings.Repeat("(", n) + inner + strings.Repeat(")", n)
}

// TestDeepNestingNoCrash is the end-to-end G7 regression for a unit's own
// terragrunt.hcl: before the fix, hclsyntax.ParseConfig recursed once per
// paren and killed the whole process with a fatal stack overflow, which no
// recover() can catch. The file must now be refused before parsing,
// leaving the unit config-unknown config-too-deep with no GRT100 (the file
// is not known to be invalid), and its sibling unit untouched.
func TestDeepNestingNoCrash(t *testing.T) {
	fsys := filesFS(map[string]string{
		"live/app/terragrunt.hcl": "inputs = { x = " + deepParens(crashDepth, "1") + " }\n",
		"live/vpc/terragrunt.hcl": "",
		"live/vpc/main.tf":        "output \"id\" {\n  value = 1\n}\n",
	})
	res := build(t, fsys)

	app, ok := res.Graph.Unit(repograph.MustRepoPath("live/app"))
	if !ok {
		t.Fatal("live/app not in graph")
	}
	if app.Status() != repograph.StatusConfigUnknown || app.UnknownReason() != ReasonConfigTooDeep {
		t.Fatalf("live/app = (%v, %q), want (config-unknown, %q)", app.Status(), app.UnknownReason(), ReasonConfigTooDeep)
	}
	vpc, ok := res.Graph.Unit(repograph.MustRepoPath("live/vpc"))
	if !ok {
		t.Fatal("live/vpc not in graph")
	}
	if vpc.Status() != repograph.StatusResolved {
		t.Fatalf("live/vpc status = %v (reason %q), want resolved", vpc.Status(), vpc.UnknownReason())
	}
	if all := res.Diagnostics.All(); len(all) != 0 {
		t.Fatalf("Diagnostics = %v, want none", all)
	}
}

// TestDeepSharedIncludeNoCrash covers the include half of G7b: a shared
// root.hcl holding the crash shape is read exactly once and makes every
// unit including it config-unknown config-too-deep, with no GRT100.
func TestDeepSharedIncludeNoCrash(t *testing.T) {
	inc := `include "root" {
  path = find_in_parent_folders("root.hcl")
}
`
	cfs := newCountingFS(filesFS(map[string]string{
		"root.hcl":              "inputs = { x = " + deepParens(crashDepth, "1") + " }\n",
		"live/a/terragrunt.hcl": inc,
		"live/b/terragrunt.hcl": inc,
	}))
	res := build(t, cfs)

	for _, dir := range []string{"live/a", "live/b"} {
		u, ok := res.Graph.Unit(repograph.MustRepoPath(dir))
		if !ok {
			t.Fatalf("%s not in graph", dir)
		}
		if u.Status() != repograph.StatusConfigUnknown || u.UnknownReason() != ReasonConfigTooDeep {
			t.Fatalf("%s = (%v, %q), want (config-unknown, %q)", dir, u.Status(), u.UnknownReason(), ReasonConfigTooDeep)
		}
	}
	if got := cfs.count("root.hcl"); got != 1 {
		t.Fatalf(`reads["root.hcl"] = %d, want 1`, got)
	}
	if all := res.Diagnostics.All(); len(all) != 0 {
		t.Fatalf("Diagnostics = %v, want none", all)
	}
}

// TestDeepModuleFileEndToEnd drives 02-10's module-file guard (G7a)
// through the whole pipeline: the dependency target's module file holds
// the crash shape, so the target module's surface is unknown and the
// reference into it cannot be checked. Nothing crashes, and no reference
// is counted as missing its output.
func TestDeepModuleFileEndToEnd(t *testing.T) {
	fsys := filesFS(map[string]string{
		"live/vpc/terragrunt.hcl": "",
		"live/vpc/main.tf":        "output \"id\" {\n  value = " + deepParens(crashDepth, "1") + "\n}\n",
		"live/app/terragrunt.hcl": `dependency "vpc" {
  config_path = "../vpc"
}

inputs = {
  id = dependency.vpc.outputs.id
}
`,
		"live/app/main.tf": "output \"o\" {\n  value = 1\n}\n",
	})
	res := build(t, fsys)

	mod, ok := res.Graph.ModuleOf(repograph.MustRepoPath("live/vpc"))
	if !ok {
		t.Fatal("ModuleOf(live/vpc) not found")
	}
	if _, known := mod.Surface(); known {
		t.Fatal("live/vpc surface known, want unknown")
	}
	if mod.UnknownReason() != tfsurface.ReasonModuleFileTooDeep {
		t.Fatalf("live/vpc module UnknownReason = %q, want %q", mod.UnknownReason(), tfsurface.ReasonModuleFileTooDeep)
	}

	refs := res.Graph.References()
	if len(refs) != 1 {
		t.Fatalf("len(References()) = %d, want 1", len(refs))
	}
	missing := 0
	for _, ur := range refs {
		target, ok := res.Graph.DependencyTarget(ur.Unit, ur.Reference.Dependency())
		if !ok {
			t.Fatalf("DependencyTarget(%s, %s) not found", ur.Unit, ur.Reference.Dependency())
		}
		m, ok := res.Graph.ModuleOf(target.Path())
		if !ok {
			t.Fatalf("ModuleOf(%s) not found", target.Path())
		}
		surf, ok := m.Surface()
		if !ok {
			continue // unknown surface: the reference cannot be checked
		}
		if !surf.HasOutput(ur.Reference.Output()) {
			missing++
		}
	}
	if missing != 0 {
		t.Fatalf("references missing their declared output = %d, want 0", missing)
	}
	if all := res.Diagnostics.All(); len(all) != 0 {
		t.Fatalf("Diagnostics = %v, want none", all)
	}
}
