package terragrunt

import (
	"testing"
	"testing/fstest"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
)

// parseExpr parses src as a standalone hcl expression, failing the test on
// a syntax error. It is a test-only helper; the six path functions are
// never evaluated on anything but a successfully parsed expression.
func parseExpr(t *testing.T, src string) hcl.Expression {
	t.Helper()
	expr, diags := hclsyntax.ParseExpression([]byte(src), "test.hcl", hcl.InitialPos)
	if diags.HasErrors() {
		t.Fatalf("parse expression %q: %v", src, diags)
	}
	return expr
}

// repoFS is the fstest.MapFS repo described by the plan's <behavior>
// section: root.hcl, env/region.hcl, env/terragrunt.hcl,
// env/app/terragrunt.hcl, env/app/sub/terragrunt.hcl, modules/vpc/main.tf,
// terragrunt/terragrunt.hcl, terragrunt/secrets/mysql/terragrunt.hcl.
func repoFS() fstest.MapFS {
	return fstest.MapFS{
		"root.hcl":                                &fstest.MapFile{},
		"env/region.hcl":                          &fstest.MapFile{},
		"env/terragrunt.hcl":                      &fstest.MapFile{},
		"env/app/terragrunt.hcl":                  &fstest.MapFile{},
		"env/app/sub/terragrunt.hcl":              &fstest.MapFile{},
		"modules/vpc/main.tf":                     &fstest.MapFile{},
		"terragrunt/terragrunt.hcl":               &fstest.MapFile{},
		"terragrunt/secrets/mysql/terragrunt.hcl": &fstest.MapFile{},
	}
}

// --- get_terragrunt_dir / get_original_terragrunt_dir --------------------

func TestPathFuncsGetTerragruntDir(t *testing.T) {
	const unitDir = "env/app/sub"
	const want = "/__gruntled_repo_root__/env/app/sub"
	fsys := repoFS()

	for _, fn := range []string{"get_terragrunt_dir", "get_original_terragrunt_dir"} {
		for _, tc := range []struct {
			name string
			kind scopeKind
		}{
			{"S0", scopeInclude},
			{"S1", scopeUnit},
			{"S2", scopeIncluded},
		} {
			t.Run(fn+"/"+tc.name, func(t *testing.T) {
				s := evalScope{fsys: fsys, unitDir: unitDir, kind: tc.kind, included: "env"}
				got, ok := evalPath(parseExpr(t, fn+"()"), s)
				if !ok {
					t.Fatalf("%s(): ok = false, want true", fn)
				}
				if got != want {
					t.Fatalf("%s() = %q, want %q", fn, got, want)
				}
			})
		}
	}

	t.Run("extra argument fails closed", func(t *testing.T) {
		s := evalScope{fsys: fsys, unitDir: unitDir, kind: scopeUnit}
		if _, ok := evalPath(parseExpr(t, `get_terragrunt_dir("extra-arg")`), s); ok {
			t.Fatalf("get_terragrunt_dir(\"extra-arg\"): ok = true, want false")
		}
	})
}

// --- find_in_parent_folders -----------------------------------------------

func TestPathFuncsFindInParentFolders(t *testing.T) {
	fsys := repoFS()
	const unitDir = "env/app/sub"

	cases := []struct {
		name string
		expr string
		want string
		ok   bool
	}{
		{"no-arg probes json then hcl, finds env/app", `find_in_parent_folders()`, "/__gruntled_repo_root__/env/app/terragrunt.hcl", true},
		{"named file found at repo root", `find_in_parent_folders("root.hcl")`, "/__gruntled_repo_root__/root.hcl", true},
		{"named file found at env", `find_in_parent_folders("region.hcl")`, "/__gruntled_repo_root__/env/region.hcl", true},
		{"named directory found at repo root", `find_in_parent_folders("modules")`, "/__gruntled_repo_root__/modules", true},
		{"not found, no fallback fails", `find_in_parent_folders("nope.hcl")`, "", false},
		{"not found, fallback returned literally", `find_in_parent_folders("nope.hcl", "fallback.hcl")`, "fallback.hcl", true},
		{"three args fails", `find_in_parent_folders("a", "b", "c")`, "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := evalScope{fsys: fsys, unitDir: unitDir, kind: scopeUnit}
			got, ok := evalPath(parseExpr(t, tc.expr), s)
			if ok != tc.ok {
				t.Fatalf("%s: ok = %v, want %v", tc.expr, ok, tc.ok)
			}
			if ok && got != tc.want {
				t.Fatalf("%s = %q, want %q", tc.expr, got, tc.want)
			}
		})
	}
}

func TestPathFuncsFindInParentFoldersScopeInvariant(t *testing.T) {
	fsys := repoFS()
	const unitDir = "env/app/sub"
	const want = "/__gruntled_repo_root__/env/app/terragrunt.hcl"

	for _, tc := range []struct {
		name string
		kind scopeKind
	}{
		{"S0", scopeInclude},
		{"S1", scopeUnit},
		{"S2", scopeIncluded},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := evalScope{fsys: fsys, unitDir: unitDir, kind: tc.kind, included: "env"}
			got, ok := evalPath(parseExpr(t, `find_in_parent_folders()`), s)
			if !ok || got != want {
				t.Fatalf("find_in_parent_folders() = (%q, %v), want (%q, true)", got, ok, want)
			}
		})
	}
}

func TestPathFuncsFindInParentFoldersJSONWinsOverHCL(t *testing.T) {
	fsys := fstest.MapFS{
		"env/app/terragrunt.hcl.json": &fstest.MapFile{},
		"env/app/terragrunt.hcl":      &fstest.MapFile{},
	}
	s := evalScope{fsys: fsys, unitDir: "env/app/sub", kind: scopeUnit}
	got, ok := evalPath(parseExpr(t, `find_in_parent_folders()`), s)
	if !ok {
		t.Fatalf("find_in_parent_folders(): ok = false, want true")
	}
	const want = "/__gruntled_repo_root__/env/app/terragrunt.hcl.json"
	if got != want {
		t.Fatalf("find_in_parent_folders() = %q, want %q (the .json file must win)", got, want)
	}
}

// --- path_relative_to_include ---------------------------------------------

func TestPathFuncsPathRelativeToInclude(t *testing.T) {
	fsys := repoFS()
	const unitDir = "env/app/sub"

	t.Run("S0", func(t *testing.T) {
		s := evalScope{fsys: fsys, unitDir: unitDir, kind: scopeInclude}
		assertEvalPath(t, s, `path_relative_to_include()`, ".", true)
	})

	t.Run("S1 zero includes", func(t *testing.T) {
		s := evalScope{fsys: fsys, unitDir: unitDir, kind: scopeUnit}
		assertEvalPath(t, s, `path_relative_to_include()`, ".", true)
	})

	t.Run("S1 one include at repo root, arg ignored", func(t *testing.T) {
		s := evalScope{fsys: fsys, unitDir: unitDir, kind: scopeUnit, includes: []includeRef{{label: "root", dir: "."}}}
		assertEvalPath(t, s, `path_relative_to_include()`, "env/app/sub", true)
		assertEvalPath(t, s, `path_relative_to_include("ignored")`, "env/app/sub", true)
	})

	t.Run("S1 two includes, no arg fails", func(t *testing.T) {
		s := evalScope{fsys: fsys, unitDir: unitDir, kind: scopeUnit, includes: []includeRef{
			{label: "root", dir: "."},
			{label: "region", dir: "env"},
		}}
		assertEvalPath(t, s, `path_relative_to_include()`, "", false)
	})

	t.Run("S1 two includes, arg selects by label", func(t *testing.T) {
		s := evalScope{fsys: fsys, unitDir: unitDir, kind: scopeUnit, includes: []includeRef{
			{label: "root", dir: "."},
			{label: "region", dir: "env"},
		}}
		assertEvalPath(t, s, `path_relative_to_include("region")`, "app/sub", true)
	})

	t.Run("S1 two includes, missing label fails", func(t *testing.T) {
		s := evalScope{fsys: fsys, unitDir: unitDir, kind: scopeUnit, includes: []includeRef{
			{label: "root", dir: "."},
			{label: "region", dir: "env"},
		}}
		assertEvalPath(t, s, `path_relative_to_include("nope")`, "", false)
	})

	t.Run("S2 inside env", func(t *testing.T) {
		s := evalScope{fsys: fsys, unitDir: unitDir, kind: scopeIncluded, included: "env"}
		assertEvalPath(t, s, `path_relative_to_include()`, "app/sub", true)
	})
}

// --- path_relative_from_include --------------------------------------------

func TestPathFuncsPathRelativeFromInclude(t *testing.T) {
	fsys := repoFS()
	const unitDir = "env/app/sub"

	t.Run("S0", func(t *testing.T) {
		s := evalScope{fsys: fsys, unitDir: unitDir, kind: scopeInclude}
		assertEvalPath(t, s, `path_relative_from_include()`, ".", true)
	})

	t.Run("S1 one include at repo root", func(t *testing.T) {
		s := evalScope{fsys: fsys, unitDir: unitDir, kind: scopeUnit, includes: []includeRef{{label: "root", dir: "."}}}
		assertEvalPath(t, s, `path_relative_from_include()`, "../../..", true)
	})

	t.Run("S2 inside env", func(t *testing.T) {
		s := evalScope{fsys: fsys, unitDir: unitDir, kind: scopeIncluded, included: "env"}
		assertEvalPath(t, s, `path_relative_from_include()`, "../..", true)
	})
}

// --- get_parent_terragrunt_dir ---------------------------------------------

func TestPathFuncsGetParentTerragruntDir(t *testing.T) {
	fsys := repoFS()
	const unitDir = "env/app/sub"

	t.Run("S0", func(t *testing.T) {
		s := evalScope{fsys: fsys, unitDir: unitDir, kind: scopeInclude}
		assertEvalPath(t, s, `get_parent_terragrunt_dir()`, "/__gruntled_repo_root__/env/app/sub", true)
	})

	t.Run("S1 one include at env", func(t *testing.T) {
		s := evalScope{fsys: fsys, unitDir: unitDir, kind: scopeUnit, includes: []includeRef{{label: "root", dir: "env"}}}
		assertEvalPath(t, s, `get_parent_terragrunt_dir()`, "/__gruntled_repo_root__/env", true)
	})

	t.Run("S2 inside env", func(t *testing.T) {
		s := evalScope{fsys: fsys, unitDir: unitDir, kind: scopeIncluded, included: "env"}
		assertEvalPath(t, s, `get_parent_terragrunt_dir()`, "/__gruntled_repo_root__/env", true)
	})

	t.Run("S1 two includes, no arg fails", func(t *testing.T) {
		s := evalScope{fsys: fsys, unitDir: unitDir, kind: scopeUnit, includes: []includeRef{
			{label: "root", dir: "."},
			{label: "region", dir: "env"},
		}}
		assertEvalPath(t, s, `get_parent_terragrunt_dir()`, "", false)
	})
}

// --- the Terragrunt docs example -------------------------------------------

func TestPathFuncsDocsExample(t *testing.T) {
	fsys := repoFS()
	s := evalScope{
		fsys:     fsys,
		unitDir:  "terragrunt/secrets/mysql",
		kind:     scopeUnit,
		includes: []includeRef{{label: "root", dir: "terragrunt"}},
	}
	const expr = `"${path_relative_from_include()}/../sources//${path_relative_to_include()}"`
	assertEvalPath(t, s, expr, "../../../sources//secrets/mysql", true)
}

// --- conditional and template expressions -----------------------------------

func TestEvalPathConditionalAndTemplate(t *testing.T) {
	fsys := repoFS()
	s := evalScope{fsys: fsys, unitDir: "env/app/sub", kind: scopeUnit}

	assertEvalPath(t, s, `"a" == "a" ? "../x" : "../y"`, "../x", true)
	assertEvalPath(t, s, `"${get_terragrunt_dir()}/../../vpc"`, "/__gruntled_repo_root__/env/app/sub/../../vpc", true)
}

// --- everything outside the six functions fails closed ----------------------

func TestEvalPathFailsClosed(t *testing.T) {
	fsys := repoFS()
	s := evalScope{fsys: fsys, unitDir: "env/app/sub", kind: scopeUnit}

	exprs := []string{
		`local.x`,
		`"${local.base}/vpc"`,
		`get_env("X", "d")`,
		`run_cmd("echo")`,
		`dirname("a/b")`,
		`get_repo_root()`,
		`format("%s", "a")`,
		`include.root.locals.x`,
		`1`,
		`null`,
		`get_terragrunt_dir("extra-arg")`,
	}
	for _, expr := range exprs {
		t.Run(expr, func(t *testing.T) {
			if got, ok := evalPath(parseExpr(t, expr), s); ok {
				t.Fatalf("evalPath(%q) = (%q, true), want ok = false", expr, got)
			}
		})
	}
}

func assertEvalPath(t *testing.T, s evalScope, expr, want string, wantOK bool) {
	t.Helper()
	got, ok := evalPath(parseExpr(t, expr), s)
	if ok != wantOK {
		t.Fatalf("evalPath(%q): ok = %v, want %v (got %q)", expr, ok, wantOK, got)
	}
	if ok && got != want {
		t.Fatalf("evalPath(%q) = %q, want %q", expr, got, want)
	}
}

// --- resolvePath -------------------------------------------------------------

func TestResolvePath(t *testing.T) {
	cases := []struct {
		name    string
		unitDir string
		p       string
		want    string
		ok      bool
	}{
		{"virtual root itself", "a", virtualRoot, ".", true},
		{"virtual path with dot-dot resolved inside", "a", virtualRoot + "/x/../y", "y", true},
		{"virtual path escaping the repo", "a", virtualRoot + "/../x", "", false},
		{"real absolute path", "a", "/etc/passwd", "", false},
		{"relative path", "a/b", "../c", "a/c", true},
		{"relative path escaping the repo", "a", "../../c", "", false},
		{"relative path to self", "a", "./", "a", true},
		{"root dir escaping via dot-dot", ".", "..", "", false},
		{"virtual-root lookalike is not the virtual root", "a", virtualRoot + "x/y", "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := resolvePath(tc.unitDir, tc.p)
			if ok != tc.ok {
				t.Fatalf("resolvePath(%q, %q): ok = %v, want %v", tc.unitDir, tc.p, ok, tc.ok)
			}
			if ok && got != tc.want {
				t.Fatalf("resolvePath(%q, %q) = %q, want %q", tc.unitDir, tc.p, got, tc.want)
			}
		})
	}
}

// --- literalString (literalBool moved to hclconv.LiteralBool) ----------------

func TestLiteralString(t *testing.T) {
	if got, ok := literalString(parseExpr(t, `"no_merge"`)); !ok || got != "no_merge" {
		t.Fatalf(`literalString("no_merge") = (%q, %v), want ("no_merge", true)`, got, ok)
	}
	if got, ok := literalString(parseExpr(t, `"${local.x}"`)); ok {
		t.Fatalf(`literalString("${local.x}") = (%q, true), want ok = false`, got)
	}
}
