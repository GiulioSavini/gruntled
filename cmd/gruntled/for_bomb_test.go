package main

import (
	"bytes"
	"context"
	"strconv"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/GiulioSavini/gruntled/internal/domain/repograph"
	"github.com/GiulioSavini/gruntled/internal/infrastructure/tfsurface"
)

// forBombBound is the wall-clock bound for one command on a for-bomb repo.
// The guarded run takes milliseconds; the unguarded 5 x 100 shape takes
// hours, so a loose bound that also holds under -race discriminates.
const forBombBound = 10 * time.Second

// nestedForList returns levels nested list for-expressions, each over the
// literal list [0, 1, ..., n-1]: n^levels inner evaluations for anything
// that calls Value on it (sec #317/#318).
func nestedForList(levels, n int) string {
	elems := make([]string, n)
	for i := range elems {
		elems[i] = strconv.Itoa(i)
	}
	list := "[" + strings.Join(elems, ",") + "]"
	var b strings.Builder
	for l := range levels {
		b.WriteString("[for v" + strconv.Itoa(l) + " in " + list + " : ")
	}
	b.WriteString("0" + strings.Repeat("]", levels))
	return b.String()
}

// forBombSizes runs check at 2 x 10 and then, only when guarded reports
// that the reader refuses a for-expression without evaluating it, at
// 5 x 100. Without the guard the CLI output of the small shape can match
// the plain repo, so the CLI result alone cannot protect the big run,
// which would not finish (and once took 6.7 GB).
func forBombSizes(t *testing.T, guarded func() bool, check func(t *testing.T, levels, n int)) {
	t.Helper()
	t.Run("2x10", func(t *testing.T) { check(t, 2, 10) })
	if !guarded() {
		t.Fatal("for-expressions are still evaluated; 5x100 skipped as it would not finish")
	}
	t.Run("5x100", func(t *testing.T) { check(t, 5, 100) })
}

// literalBoolGuarded reports whether hclconv.LiteralBool, shared by the
// dependency facts and the module reader, refuses a small nested
// for-expression (sec #318); read through an output's sensitive, as cmd
// may not import HCL.
func literalBoolGuarded() bool {
	src := "output \"o\" {\n  value = 1\n  sensitive = (" + nestedForList(2, 10) + ") == []\n}\n"
	o, ok := readGuardModule(src).Output("o")
	return ok && o.Sensitive == repograph.TristateUnknown
}

// typeFactGuarded reports whether the module reader refuses a small
// nested for-expression in an optional() default (sec #317).
func typeFactGuarded() bool {
	src := "variable \"x\" {\n  type = object({a=optional(any, " + nestedForList(2, 10) + ")})\n}\n"
	v, ok := readGuardModule(src).Variable("x")
	return ok && v.Type == ""
}

// readGuardModule reads a one-file module; the zero Surface on any error.
func readGuardModule(src string) repograph.Surface {
	fsys := fstest.MapFS{"m/main.tf": {Data: []byte(src)}}
	res, err := tfsurface.NewReader(fsys).ReadSurface(context.Background(), repograph.MustRepoPath("m"))
	if err != nil || res.UnknownReason != "" {
		return repograph.Surface{}
	}
	return res.Surface
}

// timedRun runs the CLI and fails the test when it exceeds forBombBound.
func timedRun(t *testing.T, args ...string) (stdout, stderr string, code int) {
	t.Helper()
	var out, errb bytes.Buffer
	start := time.Now()
	code = run(args, &out, &errb)
	if took := time.Since(start); took > forBombBound {
		t.Fatalf("%v took %v, want < %v", args, took, forBombBound)
	}
	return out.String(), errb.String(), code
}

// TestCheckDependencyForBomb (sec #318, pre-existing in v0.3.0): nested
// for-expressions in a dependency's enabled or skip_outputs make the fact
// unknown without evaluating it, so check behaves exactly as with a
// non-literal value (local.on): same exit code, same output, no false
// GRT001 for the valid reference and the same one for the misspelled one.
func TestCheckDependencyForBomb(t *testing.T) {
	for _, attr := range []string{"enabled", "skip_outputs"} {
		for _, ref := range []string{"vpc_id", "vpc_idd"} {
			t.Run(attr+" "+ref, func(t *testing.T) {
				forBombSizes(t, literalBoolGuarded, func(t *testing.T, levels, n int) {
					dep := func(v string) string {
						return "dependency \"vpc\" {\n  config_path = \"../vpc\"\n  " + attr + " = " + v + "\n}\ninputs = { id = dependency.vpc.outputs." + ref + " }\n"
					}
					plain, hostile := repo(t, ref), repo(t, ref)
					writeFiles(t, plain, map[string]string{"app/terragrunt.hcl": dep("local.on")})
					writeFiles(t, hostile, map[string]string{"app/terragrunt.hcl": dep("(" + nestedForList(levels, n) + ") == []")})

					want, wantErr, wantCode := timedRun(t, "check", plain)
					got, gotErr, code := timedRun(t, "check", hostile)
					norm := func(s, dir string) string { return strings.ReplaceAll(s, dir, "<repo>") }
					if code != wantCode || norm(got, hostile) != norm(want, plain) || gotErr != wantErr {
						t.Fatalf("exit %d, want %d\n--- hostile:\n%s%s\n--- plain:\n%s%s", code, wantCode, got, gotErr, want, wantErr)
					}
					if ref == "vpc_id" && strings.Contains(got, "GRT001") {
						t.Fatalf("false GRT001 for a valid reference:\n%s", got)
					}
				})
			})
		}
	}
}

// TestBlastForBomb (sec #317): nested for-expressions in an optional()
// default and in an output's sensitive, on both sides of blast --base,
// cost nothing and read as unknown facts: text and JSON output are
// exactly those of a module whose default and sensitive are references
// (local.d, local.s), so the type-level reasons stay silent and every
// name is kept.
func TestBlastForBomb(t *testing.T) {
	// extra is an attribute only the current side declares: a type change
	// that only a read type fact could report.
	module := func(def, extra, sens string) string {
		return "variable \"x\" {\n  type = object({a=optional(any, " + def + ")" + extra + "})\n}\noutput \"o\" {\n  value = 1\n  sensitive = " + sens + "\n}\n"
	}
	guarded := func() bool { return literalBoolGuarded() && typeFactGuarded() }
	for _, format := range []string{"text", "json"} {
		t.Run(format, func(t *testing.T) {
			forBombSizes(t, guarded, func(t *testing.T, levels, n int) {
				bomb := nestedForList(levels, n)
				hostileBase, hostileCur := shortBase(t), shortBase(t)
				plainBase, plainCur := shortBase(t), shortBase(t)
				writeFiles(t, hostileBase, chainTree(module(bomb, "", "("+bomb+") == []")))
				writeFiles(t, hostileCur, chainTree(module(bomb, ", b=optional(number)", "("+bomb+") != []")))
				writeFiles(t, plainBase, chainTree(module("local.d", "", "local.s")))
				writeFiles(t, plainCur, chainTree(module("local.d", ", b=optional(number)", "local.s")))

				want, wantErr, wantCode := timedRun(t, "blast", "--format", format, "--base", plainBase, plainCur)
				got, gotErr, code := timedRun(t, "blast", "--format", format, "--base", hostileBase, hostileCur)
				got = strings.ReplaceAll(got, hostileBase, "<base>")
				want = strings.ReplaceAll(want, plainBase, "<base>")
				if wantCode != exitOK || code != wantCode || got != want || gotErr != wantErr {
					t.Fatalf("exit %d, want %d (plain %d)\n--- hostile:\n%s%s\n--- plain:\n%s%s", code, exitOK, wantCode, got, gotErr, want, wantErr)
				}
				if strings.Contains(got, "variable x") || strings.Contains(got, "output o") {
					t.Fatalf("a name or a type-level reason changed:\n%s", got)
				}
			})
		})
	}
}

// nestedForString returns levels nested for-expressions over [0..n-1]
// whose value is the string val: an indexed list for, or a quoted
// template %{for} (val repeated n^levels times) (sec #327).
func nestedForString(kind string, levels, n int, val string) string {
	elems := make([]string, n)
	for i := range elems {
		elems[i] = strconv.Itoa(i)
	}
	list := "[" + strings.Join(elems, ",") + "]"
	var b strings.Builder
	if kind == "quoted" {
		b.WriteString(`"`)
		for l := range levels {
			b.WriteString("%{for v" + strconv.Itoa(l) + " in " + list + "}")
		}
		b.WriteString(val + strings.Repeat("%{endfor}", levels) + `"`)
		return b.String()
	}
	for l := range levels {
		b.WriteString("[for v" + strconv.Itoa(l) + " in " + list + " : ")
	}
	b.WriteString(strconv.Quote(val) + strings.Repeat("]", levels) + strings.Repeat("[0]", levels))
	return b.String()
}

// stringCallers are the attributes whose value terragrunt reads with
// literalString or evalPath, as an app/terragrunt.hcl template with X in
// place of the value and the string X would evaluate to.
var stringCallers = []struct{ name, unit, val string }{
	{"dependency config_path", "dependency \"vpc\" {\n  config_path = X\n}\n", "../vpc"},
	{"mock_outputs_merge_strategy_with_state", "dependency \"vpc\" {\n  config_path = \"../vpc\"\n  mock_outputs = { vpc_idd = \"m\" }\n  mock_outputs_merge_strategy_with_state = X\n}\n", "shallow"},
	{"mock_outputs key", "dependency \"vpc\" {\n  config_path = \"../vpc\"\n  mock_outputs = { (X) = \"m\" }\n}\n", "vpc_idd"},
	{"mock_outputs_allowed_terraform_commands", "dependency \"vpc\" {\n  config_path = \"../vpc\"\n  mock_outputs = { vpc_idd = \"m\" }\n  mock_outputs_allowed_terraform_commands = [X]\n}\n", "validate"},
	{"include path", "include \"root\" {\n  path = X\n}\n", "../root.hcl"},
	{"include merge_strategy", "include \"root\" {\n  path = \"../root.hcl\"\n  merge_strategy = X\n}\n", "deep"},
	{"find_in_parent_folders argument", "include \"root\" {\n  path = find_in_parent_folders(X)\n}\n", "root.hcl"},
	{"dynamic include file name", "include \"root\" {\n  path = \"${local.d}/${X}\"\n}\n", "root.hcl"},
	{"dependencies paths", "dependencies {\n  paths = [X]\n}\n", "../vpc"},
	{"terraform source", "terraform {\n  source = X\n}\n", "../vpc"},
}

// stringRepo writes a repo whose app/terragrunt.hcl is unit with X
// replaced by x, reading dependency.vpc.outputs.vpc_idd.
func stringRepo(t *testing.T, unit, x string) string {
	t.Helper()
	dir := t.TempDir()
	writeFiles(t, dir, map[string]string{
		"root.hcl":           "",
		"vpc/terragrunt.hcl": "",
		"vpc/main.tf":        "output \"vpc_id\" { value = \"x\" }\n",
		"app/terragrunt.hcl": strings.ReplaceAll(unit, "X", x) + "inputs = { id = dependency.vpc.outputs.vpc_idd }\n",
		"app/main.tf":        "variable \"id\" {}\n",
	})
	return dir
}

// checkSame runs check on a repo with X = shape and on the same repo with
// X = local.x and fails unless both are byte-identical (paths aside).
func checkSame(t *testing.T, unit, shape string) {
	t.Helper()
	plain, hostile := stringRepo(t, unit, "local.x"), stringRepo(t, unit, shape)
	want, wantErr, wantCode := timedRun(t, "check", plain)
	got, gotErr, code := timedRun(t, "check", hostile)
	norm := func(s, dir string) string { return strings.ReplaceAll(s, dir, "<repo>") }
	if code != wantCode || norm(got, hostile) != norm(want, plain) || norm(gotErr, hostile) != norm(wantErr, plain) {
		t.Fatalf("exit %d, want %d\n--- hostile:\n%s%s\n--- local.x:\n%s%s", code, wantCode, got, gotErr, want, wantErr)
	}
}

// stringGuarded reports whether both readers refuse a small for-built
// string: probed through config_path (evalPath) and include
// merge_strategy (literalString), whose check output differs from the
// local.x one whenever the value is evaluated.
func stringGuarded(t *testing.T) bool {
	for _, i := range []int{0, 5} {
		c := stringCallers[i]
		ok := t.Run("guard "+c.name, func(t *testing.T) { checkSame(t, c.unit, nestedForString("list", 2, 10, c.val)) })
		if !ok {
			return false
		}
	}
	return true
}

// TestCheckStringForBomb (sec #327, pre-existing in v0.3.0): a value read
// by literalString or evalPath that holds a for-expression is never
// evaluated; check is byte-identical to the same repo with local.x there,
// for every caller of the two readers.
func TestCheckStringForBomb(t *testing.T) {
	guarded := stringGuarded(t)
	for _, c := range stringCallers {
		for _, kind := range []string{"list", "quoted"} {
			t.Run(c.name+" "+kind, func(t *testing.T) {
				forBombSizes(t, func() bool { return guarded }, func(t *testing.T, levels, n int) {
					checkSame(t, c.unit, nestedForString(kind, levels, n, c.val))
				})
			})
		}
	}
}
