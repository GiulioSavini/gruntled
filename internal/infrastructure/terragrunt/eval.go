package terragrunt

import (
	"path"
	"strings"

	"github.com/hashicorp/hcl/v2"
	"github.com/zclconf/go-cty/cty"
)

// evalPath evaluates expr inside the closed EvalContext built from s:
// Variables nil, Functions exactly the six path functions. The raw string
// value is usable only when there are no error diagnostics, the result is
// wholly known, non-null and of type cty.String; any other construct
// (a variable, any other function, a non-string, an unknown or null
// result) makes it unusable and returns ok == false. Research Pattern 3.
func evalPath(expr hcl.Expression, s evalScope) (string, bool) {
	v, diags := expr.Value(&hcl.EvalContext{Functions: s.functions()})
	if diags.HasErrors() || !v.IsWhollyKnown() || v.IsNull() || !v.Type().Equals(cty.String) {
		return "", false
	}
	return v.AsString(), true
}

// resolvePath resolves an evaluated path p (whatever evalPath returned)
// against the CHILD unit dir unitDir into a cleaned repo-relative path.
// p may be:
//   - the virtual root itself, or a path under it (virtual(unitDir) from
//     one of the six functions): resolves to the repo-relative dir it
//     names, independent of unitDir;
//   - a real absolute path (never produced by the six functions, but
//     reachable via a template the user wrote): always unknown, since it
//     names something outside the analysed tree;
//   - a relative path: joined against unitDir.
//
// It never uses path.Clean directly on a "/"-rooted string to detect an
// escape, because path.Clean("/a/../../x") == "/x" silently clamps; the
// escape check instead runs on the already-relative result.
func resolvePath(unitDir, p string) (string, bool) {
	var rel string
	switch {
	case p == virtualRoot:
		rel = "."
	case strings.HasPrefix(p, virtualRoot+"/"):
		rel = path.Clean(strings.TrimPrefix(p, virtualRoot+"/"))
	case path.IsAbs(p):
		return "", false
	default:
		rel = path.Join(unitDir, p)
	}
	if rel == ".." || strings.HasPrefix(rel, "../") {
		return "", false
	}
	return rel, true
}

// literalString returns expr's value when it is a known, non-null
// cty.String literal evaluated with no variables and no functions
// (expr.Value(nil)): ok is false for anything else, including a reference
// like "${local.x}".
func literalString(expr hcl.Expression) (string, bool) {
	v, diags := expr.Value(nil)
	if diags.HasErrors() || !v.IsWhollyKnown() || v.IsNull() || !v.Type().Equals(cty.String) {
		return "", false
	}
	return v.AsString(), true
}
