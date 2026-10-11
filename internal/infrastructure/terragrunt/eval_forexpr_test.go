package terragrunt

import (
	"strconv"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
)

// stringForKinds are for-expression spellings that evaluate to a string
// (sec #327): an indexed list or object for, and a quoted or heredoc
// template %{for}, whose joined result alone is n^levels bytes.
var stringForKinds = []string{"list", "object", "quoted", "heredoc"}

// nestedForString returns levels nested for-expressions of kind over the
// literal list [0, 1, ..., n-1] whose value is the string val (list and
// object: every leaf is val and the result is indexed back down to one
// leaf) or val repeated n^levels times (templates).
func nestedForString(kind string, levels, n int, val string) string {
	elems := make([]string, n)
	for i := range elems {
		elems[i] = strconv.Itoa(i)
	}
	list := "[" + strings.Join(elems, ",") + "]"
	var open, close strings.Builder
	for l := range levels {
		v := "v" + strconv.Itoa(l)
		switch kind {
		case "list":
			open.WriteString("[for " + v + " in " + list + " : ")
			close.WriteString("]")
		case "object":
			open.WriteString("{for " + v + " in " + list + " : " + v + " => ")
			close.WriteString("}")
		case "quoted":
			open.WriteString("%{for " + v + " in " + list + "}")
			close.WriteString("%{endfor}")
		case "heredoc":
			open.WriteString("%{for " + v + " in " + list + "~}\n")
			close.WriteString("%{endfor~}\n")
		}
	}
	switch kind {
	case "list":
		return open.String() + strconv.Quote(val) + close.String() + strings.Repeat("[0]", levels)
	case "object":
		return open.String() + strconv.Quote(val) + close.String() + strings.Repeat(`["0"]`, levels)
	case "quoted":
		return `"` + open.String() + val + close.String() + `"`
	}
	return "<<EOT\n" + open.String() + val + "\n" + close.String() + "EOT\n"
}

// forStringSizes runs check at 2 x 10 and, only when that passes, at
// 5 x 100: without the guard the small shape fails fast (the value is
// read) and the big one, which would not finish, is skipped.
func forStringSizes(t *testing.T, check func(t *testing.T, levels, n int)) {
	t.Helper()
	if !t.Run("2x10", func(t *testing.T) { check(t, 2, 10) }) {
		t.Skip("the small shape is evaluated; 5x100 would not finish")
	}
	t.Run("5x100", func(t *testing.T) { check(t, 5, 100) })
}

func parseForExpr(t *testing.T, src string) hclsyntax.Expression {
	t.Helper()
	expr, diags := hclsyntax.ParseExpression([]byte(src), "t.hcl", hcl.InitialPos)
	if diags.HasErrors() {
		t.Fatalf("parse: %v", diags)
	}
	return expr
}

// TestLiteralStringForExpr (sec #327, pre-existing in v0.3.0): a string
// built by a for-expression is not a literal and is never evaluated.
func TestLiteralStringForExpr(t *testing.T) {
	for _, kind := range stringForKinds {
		t.Run(kind, func(t *testing.T) {
			forStringSizes(t, func(t *testing.T, levels, n int) {
				expr := parseForExpr(t, nestedForString(kind, levels, n, "shallow"))
				start := time.Now()
				s, ok := literalString(expr)
				if took := time.Since(start); took > time.Second {
					t.Fatalf("literalString took %v, want < 1s", took)
				}
				if ok {
					t.Fatalf("literalString = (%.40q, true), want not literal", s)
				}
			})
		})
	}
}

// TestEvalPathForExpr (sec #327, pre-existing in v0.3.0): a path built by
// a for-expression, also around a path function, is dynamic and is never
// evaluated.
func TestEvalPathForExpr(t *testing.T) {
	s := evalScope{fsys: fstest.MapFS{"app/terragrunt.hcl": {}}, unitDir: "app", kind: scopeUnit}
	for _, kind := range stringForKinds {
		for _, wrap := range []string{"plain", "function"} {
			t.Run(kind+" "+wrap, func(t *testing.T) {
				forStringSizes(t, func(t *testing.T, levels, n int) {
					src := nestedForString(kind, levels, n, "../vpc")
					if wrap == "function" {
						src = `"${get_terragrunt_dir()}/${` + strings.TrimSuffix(src, "\n") + "\n}\""
					}
					expr := parseForExpr(t, src)
					start := time.Now()
					p, ok := evalPath(expr, s)
					if took := time.Since(start); took > time.Second {
						t.Fatalf("evalPath took %v, want < 1s", took)
					}
					if ok {
						t.Fatalf("evalPath = (%.40q, true), want dynamic", p)
					}
				})
			})
		}
	}
}
