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
			"!" + ReasonConfigPathEmpty + "|" + at(`""`),
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
