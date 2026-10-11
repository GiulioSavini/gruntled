package hclconv

import (
	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/zclconf/go-cty/cty"

	"github.com/GiulioSavini/gruntled/internal/domain/repograph"
)

// LiteralBool reads expr as a literal boolean (expr.Value(nil)):
// TristateTrue or TristateFalse for a known, non-null cty.Bool, and
// TristateUnknown for anything else (a reference, a string such as
// "true", null, or an evaluation error). It is the one literal-bool reader
// shared by the Terragrunt loader and the module surface reader.
//
// A native expression holding a for-expression anywhere is unknown and is
// never evaluated: a nil EvalContext stops variables and functions but not
// a for over a literal list, and nested fors multiply, so 5 levels of 100
// elements in 2 KB would run 1e10 iterations (sec #317, #318). A literal
// bool never needs a for, so every other result is unchanged.
func LiteralBool(expr hcl.Expression) repograph.Tristate {
	if HasForExpr(expr) {
		return repograph.TristateUnknown
	}
	v, diags := expr.Value(nil)
	if diags.HasErrors() || !v.IsWhollyKnown() || v.IsNull() || !v.Type().Equals(cty.Bool) {
		return repograph.TristateUnknown
	}
	return repograph.TristateOf(v.True())
}

// HasForExpr reports whether expr is a native (hclsyntax) expression with
// a *hclsyntax.ForExpr anywhere in it: a list [for ...], an object
// {for ...}, or a template %{for} (which hclsyntax builds as a ForExpr
// under a TemplateJoinExpr). A non-native expression, such as a .tf.json
// value, is never one: hcl/json returns a string verbatim under a nil
// EvalContext. The walk is linear in the AST, whose depth the
// CheckNativeDepth pre-scan already bounds.
func HasForExpr(expr hcl.Expression) bool {
	node, ok := expr.(hclsyntax.Expression)
	if !ok {
		return false
	}
	found := false
	hclsyntax.VisitAll(node, func(n hclsyntax.Node) hcl.Diagnostics {
		if _, ok := n.(*hclsyntax.ForExpr); ok {
			found = true
		}
		return nil
	})
	return found
}
