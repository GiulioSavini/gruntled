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
