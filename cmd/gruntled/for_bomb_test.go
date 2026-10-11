package main

import (
	"bytes"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"

	"github.com/GiulioSavini/gruntled/internal/domain/repograph"
	"github.com/GiulioSavini/gruntled/internal/infrastructure/hclconv"
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

// literalBoolGuarded reports whether hclconv.LiteralBool refuses a small
// nested for-expression (sec #318).
func literalBoolGuarded() bool {
	expr, diags := hclsyntax.ParseExpression([]byte("("+nestedForList(2, 10)+") == []"), "g.hcl", hcl.InitialPos)
	return !diags.HasErrors() && hclconv.LiteralBool(expr) == repograph.TristateUnknown
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
