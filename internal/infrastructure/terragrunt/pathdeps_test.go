package terragrunt

import (
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/GiulioSavini/gruntled/internal/application/ports"
	"github.com/GiulioSavini/gruntled/internal/domain/repograph"
)

// pdStrings renders every path dependency of u as one comparable string,
// sorted: "target|state|pos|literal" when resolved, "!reason|pos" when not.
func pdStrings(u ports.UnitConfig) []string {
	out := make([]string, 0, len(u.PathDependencies))
	for _, pd := range u.PathDependencies {
		if target, ok := pd.Target(); ok {
			out = append(out, target.String()+"|"+pd.TargetState().String()+"|"+pd.Pos().String()+"|"+pd.Literal())
			continue
		}
		if pd.TargetState() != repograph.TargetUnknown {
			out = append(out, "BAD-STATE:"+pd.TargetState().String())
		}
		out = append(out, "!"+pd.UnresolvedReason()+"|"+pd.Pos().String())
	}
	slices.Sort(out)
	return out
}

// colOf returns the 1-based byte column of the first occurrence of tok in
// line.
func colOf(t *testing.T, line, tok string) string {
	t.Helper()
	i := strings.Index(line, tok)
	if i < 0 {
		t.Fatalf("%q not in %q", tok, line)
	}
	return strconv.Itoa(i + 1)
}

func assertResolvedUnit(t *testing.T, u ports.UnitConfig) {
	t.Helper()
	if u.ConfigUnknownReason != "" || u.ModuleUnknownReason != "" {
		t.Fatalf("unit %s not resolved: config=%q module=%q", u.Path, u.ConfigUnknownReason, u.ModuleUnknownReason)
	}
}

func TestPathDependencies(t *testing.T) {
	line := `  paths = ["../a", "../b", local.x, "${get_terragrunt_dir()}/../c", "", "../stk", "../../../esc", "../plain", "../nodir"]`
	fsys := filesFS(map[string]string{
		"u/terragrunt.hcl":         "dependencies {\n" + line + "\n}\n",
		"a/terragrunt.hcl":         "",
		"b/terragrunt.hcl":         "",
		"c/terragrunt.hcl":         "",
		"stk/terragrunt.stack.hcl": "",
		"plain/main.tf":            "",

		"dyn/terragrunt.hcl":    "dependencies {\n  paths = local.x\n}\n",
		"concat/terragrunt.hcl": "dependencies {\n  paths = concat([\"../a\"], [\"../b\"])\n}\n",
		"wrap/terragrunt.hcl":   "dependencies {\n  paths = \"${local.x}\"\n}\n",

		"dup/terragrunt.hcl":     "dependency \"a\" { config_path = \"../a\" }\ndependencies { paths = [\"../a\"] }\ndependencies { paths = [\"../b\"] }\n",
		"labeled/terragrunt.hcl": "dependency \"a\" { config_path = \"../a\" }\ndependencies \"x\" { paths = [\"../a\"] }\n",
		"nopaths/terragrunt.hcl": "dependency \"a\" { config_path = \"../a\" }\ndependencies {}\n",
		"string/terragrunt.hcl":  "dependency \"a\" { config_path = \"../a\" }\ndependencies { paths = \"../a\" }\n",
		"object/terragrunt.hcl":  "dependency \"a\" { config_path = \"../a\" }\ndependencies { paths = { a = 1 } }\n",
		"tmpl/terragrunt.hcl":    "dependency \"a\" { config_path = \"../a\" }\ndependencies { paths = \"x${local.y}\" }\n",
		"empty/terragrunt.hcl":   "dependencies { paths = [] }\n",
	})
	res := loadUnits(t, fsys)

	t.Run("elements", func(t *testing.T) {
		u := unitByPath(t, res, "u")
		assertResolvedUnit(t, u)
		at := func(tok string) string { return "u/terragrunt.hcl:2:" + colOf(t, line, tok) }
		want := []string{
			"a|has-config|" + at(`"../a"`) + "|../a",
			"b|has-config|" + at(`"../b"`) + "|../b",
			"c|has-config|" + at(`"${get`) + "|${get_terragrunt_dir()}/../c",
			"plain|no-config|" + at(`"../plain"`) + "|../plain",
			"nodir|dir-missing|" + at(`"../nodir"`) + "|../nodir",
			"!" + ReasonConfigPathDynamic + "|" + at(`local.x`),
			"u|has-config|" + at(`""`) + `|""`,
			"!" + ReasonConfigPathStack + "|" + at(`"../stk"`),
			"!" + ReasonConfigPathOutsideRepo + "|" + at(`"../../../esc"`),
		}
		slices.Sort(want)
		if got := pdStrings(u); !slices.Equal(got, want) {
			t.Errorf("path deps:\n got  %q\n want %q", got, want)
		}
	})

	for _, dir := range []string{"dyn", "concat", "wrap"} {
		t.Run("unknown/"+dir, func(t *testing.T) {
			u := unitByPath(t, res, dir)
			assertResolvedUnit(t, u)
			want := []string{"!" + ReasonDependenciesPathsDynamic + "|" + dir + "/terragrunt.hcl:2:11"}
			if got := pdStrings(u); !slices.Equal(got, want) {
				t.Errorf("path deps = %q, want %q", got, want)
			}
		})
	}

	for _, dir := range []string{"dup", "labeled", "nopaths", "string", "object", "tmpl", "empty"} {
		t.Run("invalid/"+dir, func(t *testing.T) {
			u := unitByPath(t, res, dir)
			assertResolvedUnit(t, u)
			if len(u.PathDependencies) != 0 {
				t.Errorf("path deps = %q, want none", pdStrings(u))
			}
			if dir != "empty" && len(u.Dependencies) != 1 {
				t.Errorf("block deps = %d, want 1", len(u.Dependencies))
			}
		})
	}
}

// wantPathDepUnresolved asserts a resolved unit whose only path dependency
// is unresolved with reason.
// TestPathDepsEmptyIsSelf pins that a `dependencies { paths }` element
// evaluating to "" resolves to the unit's own directory (a self-edge),
// also when inherited from an include: terragrunt v1.1.6 reports "cycle
// detected during queue construction" for it. A dependency block's empty
// config_path is different (unresolved, see loader_test).
func TestPathDepsEmptyIsSelf(t *testing.T) {
	fsys := filesFS(map[string]string{
		"a/terragrunt.hcl": "dependencies {\n  paths = [\"\"]\n}\n",
		"m/terragrunt.hcl": "dependencies {\n  paths = [\"../b\", \"\"]\n}\n",
		"t/terragrunt.hcl": "dependencies {\n  paths = [\"${\"\"}\"]\n}\n",
		"c/terragrunt.hcl": "include \"r\" {\n  path = \"../inc.hcl\"\n}\n",
		"inc.hcl":          "dependencies {\n  paths = [\"\"]\n}\n",
		"b/terragrunt.hcl": "",
	})
	res := loadUnits(t, fsys)
	cases := map[string][]string{
		"a": {`a|has-config|a/terragrunt.hcl:2:12|""`},
		"m": {`b|has-config|m/terragrunt.hcl:2:12|../b`, `m|has-config|m/terragrunt.hcl:2:20|""`},
		"t": {`t|has-config|t/terragrunt.hcl:2:12|"${""}"`},
		"c": {`c|has-config|inc.hcl:2:12|""`},
	}
	for dir, want := range cases {
		t.Run(dir, func(t *testing.T) {
			u := unitByPath(t, res, dir)
			assertResolvedUnit(t, u)
			if got := pdStrings(u); !slices.Equal(got, want) {
				t.Errorf("path deps:\n got  %q\n want %q", got, want)
			}
		})
	}
}

func wantPathDepUnresolved(reason string) func(*testing.T, ports.UnitConfig) {
	return func(t *testing.T, uc ports.UnitConfig) {
		t.Helper()
		assertResolvedUnit(t, uc)
		if len(uc.PathDependencies) != 1 {
			t.Fatalf("path deps = %d, want 1", len(uc.PathDependencies))
		}
		if got := uc.PathDependencies[0].UnresolvedReason(); got != reason {
			t.Fatalf("reason = %q, want %q", got, reason)
		}
	}
}

// TestPathDepsMerge covers the include merge of `dependencies`: a union
// de-duplicated by resolved target in shallow and deep mode, no_merge
// contributing nothing.
func TestPathDepsMerge(t *testing.T) {
	inc := func(name, strategy string) string {
		s := "include \"r\" {\n  path = \"../inc/" + name + ".hcl\"\n"
		if strategy != "" {
			s += "  merge_strategy = \"" + strategy + "\"\n"
		}
		return s + "}\n"
	}
	// The child's own dependencies block always comes first, on line 1.
	const childDeps = "dependencies { paths = [\"../a\"] }\n"
	fsys := filesFS(map[string]string{
		"a/terragrunt.hcl": "",
		"b/terragrunt.hcl": "",
		"inc/b.hcl":        "dependencies { paths = [\"../b\"] }\n",
		"inc/ab.hcl":       "dependencies { paths = [\"../a\", \"../b\"] }\n",
		"inc/dyn.hcl":      "dependencies { paths = local.x }\n",

		"shallow/terragrunt.hcl": childDeps + inc("b", ""),
		"deep/terragrunt.hcl":    childDeps + inc("b", "deep"),
		"dup/terragrunt.hcl":     childDeps + inc("ab", ""),
		"nomerge/terragrunt.hcl": childDeps + inc("b", "no_merge"),
		"inconly/terragrunt.hcl": inc("b", ""),
		"incdyn/terragrunt.hcl":  childDeps + inc("dyn", ""),
	})
	res := loadUnits(t, fsys)

	a := func(u string) string { return "a|has-config|" + u + "/terragrunt.hcl:1:25|../a" }
	cases := []struct {
		unit string
		want []string
	}{
		{"shallow", []string{a("shallow"), "b|has-config|inc/b.hcl:1:25|../b"}},
		{"deep", []string{a("deep"), "b|has-config|inc/b.hcl:1:25|../b"}},
		{"dup", []string{a("dup"), "b|has-config|inc/ab.hcl:1:33|../b"}},
		{"nomerge", []string{a("nomerge")}},
		{"inconly", []string{"b|has-config|inc/b.hcl:1:25|../b"}},
		{"incdyn", []string{"!" + ReasonDependenciesPathsDynamic + "|inc/dyn.hcl:1:24", a("incdyn")}},
	}
	for _, tc := range cases {
		t.Run(tc.unit, func(t *testing.T) {
			u := unitByPath(t, res, tc.unit)
			assertResolvedUnit(t, u)
			want := slices.Clone(tc.want)
			slices.Sort(want)
			if got := pdStrings(u); !slices.Equal(got, want) {
				t.Errorf("path deps:\n got  %q\n want %q", got, want)
			}
		})
	}
}
