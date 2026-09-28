package terragrunt

import (
	"sort"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/zclconf/go-cty/cty"

	"github.com/GiulioSavini/gruntled/internal/domain/repograph"
	"github.com/GiulioSavini/gruntled/internal/infrastructure/hclconv"
)

// extractRefs walks body's WHOLE subtree (every attribute and every nested
// block, in every top-level block: locals, inputs, terraform hooks,
// remote_state, dependency/dependencies, generate, ...) and returns every
// static dependency.<name>.outputs.<out> traversal it contains, sorted by
// byte offset (research Pattern 5). It never evaluates an expression: the
// AST shape alone decides, so a construct that is not exactly one of the
// verified shapes below is NOT a reference. Guessing would risk a false
// positive, which this project treats as unacceptable.
//
// Verified shapes (research Pattern 5's table):
//
//	dependency.vpc.outputs.id                    -> ref(vpc, id)
//	dependency.vpc.outputs.nested.deep            -> ref(vpc, nested)  (trailing steps ignored)
//	dependency.vpc.outputs["cidr"]                -> ref(vpc, cidr)
//	dependency.vpc.outputs.subnet_ids[*]          -> ref(vpc, subnet_ids) (SplatExpr wraps a plain traversal)
//	"${dependency.vpc.outputs.tmpl}-x", a heredoc -> ref (templates walk their parts)
//	try(dependency.vpc.outputs.maybe, null)       -> NO ref (try tolerates a missing output)
//	can(dependency.vpc.outputs.x)                 -> NO ref
//	dependency.vpc.outputs (whole object)         -> NO ref
//	lookup(dependency.vpc.outputs, "id", "")      -> NO ref (3-step traversal, no 4th step)
//	dependency.vpc.outputs[local.k]               -> NO ref (index key is not a known string)
//	dependency.vpc.outputs.*.id                   -> NO ref (splat over the whole outputs object)
//	dependency.aurora["web"].outputs.id           -> NO ref (expanded dependency: TraverseIndex at step 1)
//	dependency.x, dependency.x.inputs.y           -> NO ref (wrong shape entirely)
//	dependencies.x.outputs.y, local.dependency... -> NO ref (wrong root name)
//
// Lazy-evaluation shapes (research Pattern 2; same fail-silent treatment as
// try()/can(), see refWalker):
//
//	cond ? dependency.vpc.outputs.x : y  (condition)   -> ref (always evaluated)
//	cond ? dependency.vpc.outputs.x : y  (true branch)  -> NO ref (dropped when cond is false)
//	cond ? y : dependency.vpc.outputs.x  (false branch) -> NO ref (dropped when cond is true)
//	a && dependency.vpc.outputs.x, a || dependency.vpc.outputs.x (either operand) -> NO ref (short-circuit may drop it)
//	[for k in dependency.vpc.outputs.x : v]              (collection)  -> ref (always evaluated)
//	[for k in c : dependency.vpc.outputs.x]              (value)       -> NO ref (never runs over an empty collection)
//	{for k in c : dependency.vpc.outputs.x => v}         (key)         -> NO ref
//	[for k in c : v if dependency.vpc.outputs.x]         (if-condition) -> NO ref
//
// A template directive parses to the same node types (%{ if } to
// *ConditionalExpr, %{ for } to *TemplateJoinExpr wrapping *ForExpr), so
// these rows cover it too; no extra case is needed.
func extractRefs(file repograph.RepoPath, src []byte, body *hclsyntax.Body) ([]repograph.Reference, error) {
	w := &refWalker{}
	if diags := hclsyntax.Walk(body, w); diags.HasErrors() {
		// refWalker's Enter/Exit always return nil below, so this can never
		// actually trigger today; the guard exists so a future change to
		// the walker fails closed here instead of being silently dropped.
		return nil, diags
	}

	sort.SliceStable(w.hits, func(i, j int) bool {
		return w.hits[i].start.Byte < w.hits[j].start.Byte
	})

	refs := make([]repograph.Reference, 0, len(w.hits))
	for _, h := range w.hits {
		pos, err := hclconv.Position(file, src, h.start)
		if err != nil {
			return nil, err
		}
		ref, err := repograph.NewReference(h.dep, h.out, pos)
		if err != nil {
			return nil, err
		}
		refs = append(refs, ref)
	}
	return refs, nil
}

// rawRef is one dependency.<dep>.outputs.<out> traversal found by
// refWalker, before its start position is converted to a domain Position.
type rawRef struct {
	dep, out string
	start    hcl.Pos
}

// refWalker implements hclsyntax.Walker, collecting every
// *hclsyntax.ScopeTraversalExpr matching outputRef's exact shape, while
// suppressing two kinds of match: one nested inside a try(...) or can(...)
// call (research Pitfall 6), and one that sits in a sub-expression HCL may
// evaluate without surfacing its errors (research Pattern 2). Both are the
// same fail-silent shape: tolerating a missing output there is not a
// runtime error, so reporting a reference there would be a false positive.
//
// guard counts enclosing try/can calls, so nested and sibling guards
// compose correctly (a try inside a try, or a try followed by an unguarded
// reference, both behave per the plan's OUT-05 case).
//
// lazy is a stack of byte ranges pushed by lazyRanges on Enter and popped
// on the matching Exit (hclsyntax.Walk calls Enter(n), then n's children,
// then Exit(n), so push/pop always balance). A ScopeTraversalExpr whose
// start byte falls inside any pushed range is suppressed the same way a
// try/can guard suppresses it.
type refWalker struct {
	guard int
	lazy  []hcl.Range
	hits  []rawRef
}

// Enter records a matching traversal when no try/can guards it and it does
// not start inside a lazily evaluated sub-expression, enters a new guard
// level when n is a try(...) or can(...) call, and pushes n's lazy ranges
// (if any) onto the lazy stack.
func (w *refWalker) Enter(n hclsyntax.Node) hcl.Diagnostics {
	w.lazy = append(w.lazy, lazyRanges(n)...)
	switch e := n.(type) {
	case *hclsyntax.FunctionCallExpr:
		if e.Name == "try" || e.Name == "can" {
			w.guard++
		}
	case *hclsyntax.ScopeTraversalExpr:
		if w.guard == 0 && !w.inLazyRange(e.SrcRange.Start) {
			if dep, out, ok := outputRef(e.Traversal); ok {
				w.hits = append(w.hits, rawRef{dep: dep, out: out, start: e.SrcRange.Start})
			}
		}
	}
	return nil
}

// Exit restores the guard level on leaving a try(...) or can(...) call, so
// a reference written after the call (a sibling, not a nested argument) is
// reported again, and pops the lazy ranges n pushed on Enter.
func (w *refWalker) Exit(n hclsyntax.Node) hcl.Diagnostics {
	if e, ok := n.(*hclsyntax.FunctionCallExpr); ok && (e.Name == "try" || e.Name == "can") {
		w.guard--
	}
	w.lazy = w.lazy[:len(w.lazy)-len(lazyRanges(n))]
	return nil
}

// inLazyRange reports whether start falls inside any range currently on
// the lazy stack, using byte offsets only (never line/column).
func (w *refWalker) inLazyRange(start hcl.Pos) bool {
	for _, r := range w.lazy {
		if r.Start.Byte <= start.Byte && start.Byte < r.End.Byte {
			return true
		}
	}
	return false
}

// lazyRanges returns the byte ranges of n's sub-expressions that HCL may
// evaluate without surfacing their errors (research Pattern 2):
//
//   - ConditionalExpr: the unselected branch's diagnostics are dropped;
//     TrueResult and FalseResult are both lazy. Condition is NOT lazy: it
//     is always evaluated and its diagnostics always surface.
//   - ForExpr: KeyExpr, ValExpr and CondExpr run once per source element,
//     so zero times over an empty collection; all three are lazy.
//     CollExpr is NOT lazy: it is always evaluated.
//   - BinaryOpExpr with Op OpLogicalAnd or OpLogicalOr: ShortCircuit
//     returns only the controlling side's diagnostics, so both LHS and
//     RHS are lazy (either may be the side that gets skipped). Any other
//     BinaryOpExpr (arithmetic, comparison, ..) is NOT lazy.
//
// A template directive parses to the same node types (%{ if } to
// *ConditionalExpr, %{ for } to *TemplateJoinExpr wrapping *ForExpr), so
// this covers it with no extra case.
func lazyRanges(n hclsyntax.Node) []hcl.Range {
	switch e := n.(type) {
	case *hclsyntax.ConditionalExpr:
		return []hcl.Range{e.TrueResult.Range(), e.FalseResult.Range()}
	case *hclsyntax.ForExpr:
		rs := []hcl.Range{e.ValExpr.Range()}
		if e.KeyExpr != nil {
			rs = append(rs, e.KeyExpr.Range())
		}
		if e.CondExpr != nil {
			rs = append(rs, e.CondExpr.Range())
		}
		return rs
	case *hclsyntax.BinaryOpExpr:
		if e.Op == hclsyntax.OpLogicalAnd || e.Op == hclsyntax.OpLogicalOr {
			return []hcl.Range{e.LHS.Range(), e.RHS.Range()}
		}
	}
	return nil
}

// outputRef reports whether t is the exact static shape
// dependency.<name>.outputs.(<attr>|["known string literal"]), with any
// further trailing steps ignored. Anything else -- fewer than 4 steps, a
// root other than "dependency", an expanded dependency (TraverseIndex at
// step 1), a step-3 index whose key is not a known, non-null cty.String --
// is not a reference.
func outputRef(t hcl.Traversal) (dep, out string, ok bool) {
	if len(t) < 4 || t.RootName() != "dependency" {
		return "", "", false
	}
	name, ok1 := t[1].(hcl.TraverseAttr)
	outs, ok2 := t[2].(hcl.TraverseAttr)
	if !ok1 || !ok2 || outs.Name != "outputs" {
		return "", "", false
	}
	switch s := t[3].(type) {
	case hcl.TraverseAttr:
		return name.Name, s.Name, true
	case hcl.TraverseIndex:
		if s.Key.IsKnown() && !s.Key.IsNull() && s.Key.Type().Equals(cty.String) {
			return name.Name, s.Key.AsString(), true
		}
	}
	return "", "", false
}
