package hclconv_test

import (
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/hashicorp/hcl/v2/json"

	"github.com/GiulioSavini/gruntled/internal/domain/repograph"
	"github.com/GiulioSavini/gruntled/internal/infrastructure/hclconv"
)

// forKinds are the four for-expression spellings hclsyntax builds a
// *hclsyntax.ForExpr for (sec #317/#318).
var forKinds = []string{"list", "object", "quoted", "heredoc"}

// nestedFor returns levels nested for-expressions of the given kind, each
// iterating the literal list [0, 1, ..., n-1]: n^levels inner evaluations
// for anything that calls Value on it.
func nestedFor(kind string, levels, n int) string {
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
			open.WriteString("%{for " + v + " in " + list + "}\n")
			close.WriteString("%{endfor}\n")
		}
	}
	switch kind {
	case "quoted":
		return `"` + open.String() + "x" + close.String() + `"`
	case "heredoc":
		return "<<EOT\n" + open.String() + "x\n" + close.String() + "EOT\n"
	}
	return open.String() + "0" + close.String()
}

// nestedForBool is nestedFor compared with the empty value of its kind: a
// literal bool for anything that evaluates it.
func nestedForBool(kind string, levels, n int) string {
	empty := map[string]string{"list": "[]", "object": "{}", "quoted": `""`, "heredoc": `""`}[kind]
	return "(" + nestedFor(kind, levels, n) + ") == " + empty
}

// TestLiteralBoolForExpr (sec #317 part 2, #318): LiteralBool never
// evaluates a native expression holding a for-expression; at 5 levels x
// 100 elements (1e10 inner evaluations) it answers unknown at once. The
// 2 x 10 shape runs first, so without the guard it fails fast instead of
// running the big one.
func TestLiteralBoolForExpr(t *testing.T) {
	for _, kind := range forKinds {
		t.Run(kind, func(t *testing.T) {
			check := func(t *testing.T, levels, n int) {
				src := nestedForBool(kind, levels, n)
				expr, diags := hclsyntax.ParseExpression([]byte(src), "t.hcl", hcl.InitialPos)
				if diags.HasErrors() {
					t.Fatalf("parse: %v", diags)
				}
				start := time.Now()
				got := hclconv.LiteralBool(expr)
				took := time.Since(start)
				if got != repograph.TristateUnknown {
					t.Fatalf("LiteralBool = %v, want unknown", got)
				}
				if took > time.Second {
					t.Fatalf("LiteralBool took %v, want < 1s", took)
				}
			}
			if !t.Run("2x10", func(t *testing.T) { check(t, 2, 10) }) {
				t.Skip("the small shape is evaluated; 5x100 would not finish")
			}
			t.Run("5x100", func(t *testing.T) { check(t, 5, 100) })
		})
	}
}

// TestHasForExpr: every for spelling is found at any depth, a plain
// literal (including the word for in a string or a traversal) is not, and
// a .tf.json expression is never one.
func TestHasForExpr(t *testing.T) {
	for _, kind := range forKinds {
		expr, diags := hclsyntax.ParseExpression([]byte("{a = [1, ("+nestedFor(kind, 1, 2)+")]}"), "t.hcl", hcl.InitialPos)
		if diags.HasErrors() {
			t.Fatalf("%s: parse: %v", kind, diags)
		}
		if !hclconv.HasForExpr(expr) {
			t.Fatalf("%s: HasForExpr = false, want true", kind)
		}
	}
	for _, src := range []string{"true", `"for"`, `[1, 2, {a = "x"}]`, "local.for", `"%{if a}x%{endif}"`} {
		expr, diags := hclsyntax.ParseExpression([]byte(src), "t.hcl", hcl.InitialPos)
		if diags.HasErrors() {
			t.Fatalf("%s: parse: %v", src, diags)
		}
		if hclconv.HasForExpr(expr) {
			t.Fatalf("%s: HasForExpr = true, want false", src)
		}
	}
	f, diags := json.Parse([]byte(`{"a": "%{for v in [1]}x%{endfor}"}`), "main.tf.json")
	if diags.HasErrors() {
		t.Fatalf("parse json: %v", diags)
	}
	attrs, _ := f.Body.JustAttributes()
	if hclconv.HasForExpr(attrs["a"].Expr) {
		t.Fatal("HasForExpr on a .tf.json expression = true")
	}
}
