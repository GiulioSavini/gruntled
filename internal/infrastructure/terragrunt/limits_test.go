package terragrunt

import (
	"os"
	"strings"
	"testing"

	"github.com/GiulioSavini/gruntled/internal/domain/repograph"
	"github.com/GiulioSavini/gruntled/internal/infrastructure/hclconv"
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

// --- 02-REVIEW G17: ternary nesting and long operator chains ---------------

// ternaryAlwaysOnN is the ternary count of the always-on G17 regressions:
// the largest n for which `inputs = { x = <chain> }` stays within 1 MiB
// (both chain shapes are 4 bytes per `?`). The 02-12 probe found that
// hclsyntax.ParseConfig first dies with a fatal stack overflow between
// 500k and 600k levels for both shapes, so no file this size crashes the
// parser: these tests assert refusal, and the crash shapes themselves are
// covered by the env-gated TestHeavyG17Reproductions.
const ternaryAlwaysOnN = (1<<20 - 64) / 4

// trueChain is `1?1?...1:1:1` with n `?`: nested in the true branch.
func trueChain(n int) string {
	return strings.Repeat("1?", n) + "1" + strings.Repeat(":1", n)
}

// elseChain is `a?b:a?b:...1` with n `?`: nested in the false branch.
func elseChain(n int) string {
	return strings.Repeat("a?b:", n) + "1"
}

// assertTooDeep fails the test unless dir is a config-unknown
// config-too-deep unit.
func assertTooDeep(t *testing.T, g *repograph.RepositoryGraph, dir string) {
	t.Helper()
	u, ok := g.Unit(repograph.MustRepoPath(dir))
	if !ok {
		t.Fatalf("%s not in graph", dir)
	}
	if u.Status() != repograph.StatusConfigUnknown || u.UnknownReason() != ReasonConfigTooDeep {
		t.Fatalf("%s = (%v, %q), want (config-unknown, %q)", dir, u.Status(), u.UnknownReason(), ReasonConfigTooDeep)
	}
}

// TestDeepTernaryNoCrash is the always-on G17 regression for a unit's own
// terragrunt.hcl, in both ternary shapes: both are refused before parsing,
// and their sibling is untouched.
func TestDeepTernaryNoCrash(t *testing.T) {
	app := "inputs = { x = " + trueChain(ternaryAlwaysOnN) + " }\n"
	db := "inputs = { x = " + elseChain(ternaryAlwaysOnN) + " }\n"
	if len(app) > 1<<20 || len(db) > 1<<20 {
		t.Fatalf("fixture sizes %d/%d exceed 1 MiB", len(app), len(db))
	}
	res := build(t, filesFS(map[string]string{
		"live/app/terragrunt.hcl": app,
		"live/db/terragrunt.hcl":  db,
		"live/vpc/terragrunt.hcl": "",
		"live/vpc/main.tf":        "output \"id\" {\n  value = 1\n}\n",
	}))

	assertTooDeep(t, res.Graph, "live/app")
	assertTooDeep(t, res.Graph, "live/db")
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

// TestDeepTernaryIncludeNoCrash is the include half: a shared root.hcl
// holding the else-chain is read once and makes both includers
// config-too-deep, with no GRT100.
func TestDeepTernaryIncludeNoCrash(t *testing.T) {
	inc := `include "root" {
  path = find_in_parent_folders("root.hcl")
}
`
	cfs := newCountingFS(filesFS(map[string]string{
		"root.hcl":              "inputs = { x = " + elseChain(ternaryAlwaysOnN) + " }\n",
		"live/a/terragrunt.hcl": inc,
		"live/b/terragrunt.hcl": inc,
	}))
	res := build(t, cfs)

	assertTooDeep(t, res.Graph, "live/a")
	assertTooDeep(t, res.Graph, "live/b")
	if got := cfs.count("root.hcl"); got != 1 {
		t.Fatalf(`reads["root.hcl"] = %d, want 1`, got)
	}
	if all := res.Diagnostics.All(); len(all) != 0 {
		t.Fatalf("Diagnostics = %v, want none", all)
	}
}

// TestLongOperatorChainRefused: one expression chaining more than
// hclconv.MaxExpressionChain operators is refused like a deep nesting.
func TestLongOperatorChainRefused(t *testing.T) {
	res := build(t, filesFS(map[string]string{
		"live/app/terragrunt.hcl": "inputs = { x = " + strings.Repeat("1+", hclconv.MaxExpressionChain+1) + "1 }\n",
	}))
	assertTooDeep(t, res.Graph, "live/app")
	if all := res.Diagnostics.All(); len(all) != 0 {
		t.Fatalf("Diagnostics = %v, want none", all)
	}
}

// TestHeavyG17Reproductions runs the exact 02-REVIEW G17 shapes end to
// end. Before 02-12 the two ternary rows killed the process with a fatal
// stack overflow and the plus row peaked at about 2.2 GB. Each file is
// about 4 MB, and lexing alone still costs over a gigabyte, so this only
// runs when GRUNTLED_HEAVY_TESTS=1.
func TestHeavyG17Reproductions(t *testing.T) {
	if os.Getenv("GRUNTLED_HEAVY_TESTS") != "1" {
		t.Skip("set GRUNTLED_HEAVY_TESTS=1 to run the multi-megabyte G17 reproductions")
	}
	const prefix, suffix = "inputs = { x = ", " }\n"
	trueN := (hclconv.MaxFileBytes - len(prefix) - len(suffix) - 1) / 4
	cases := []struct {
		name, src string
		wantLen   int
	}{
		{"else", prefix + strings.Repeat("a?b:", 1_000_000) + "1" + suffix, 4_000_019},
		{"true", prefix + trueChain(trueN) + suffix, 0},
		{"plus", prefix + strings.Repeat("1+", 2_000_000) + "1" + suffix, 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if c.wantLen != 0 && len(c.src) != c.wantLen {
				t.Fatalf("len = %d, want %d", len(c.src), c.wantLen)
			}
			if len(c.src) > hclconv.MaxFileBytes {
				t.Fatalf("len = %d exceeds MaxFileBytes", len(c.src))
			}
			res := build(t, filesFS(map[string]string{"live/app/terragrunt.hcl": c.src}))
			assertTooDeep(t, res.Graph, "live/app")
		})
	}
}
