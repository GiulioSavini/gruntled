package hclconv

import (
	"github.com/hashicorp/hcl/v2"
	"github.com/zclconf/go-cty/cty"

	"github.com/GiulioSavini/gruntled/internal/domain/repograph"
)

// LiteralBool reads expr as a literal boolean (expr.Value(nil)):
// TristateTrue or TristateFalse for a known, non-null cty.Bool, and
// TristateUnknown for anything else (a reference, a string such as
// "true", null, or an evaluation error). It is the one literal-bool reader
// shared by the Terragrunt loader and the module surface reader.
func LiteralBool(expr hcl.Expression) repograph.Tristate {
	v, diags := expr.Value(nil)
	if diags.HasErrors() || !v.IsWhollyKnown() || v.IsNull() || !v.Type().Equals(cty.Bool) {
		return repograph.TristateUnknown
	}
	return repograph.TristateOf(v.True())
}
