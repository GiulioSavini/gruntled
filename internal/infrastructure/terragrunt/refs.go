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
//	inside a for expression, a ternary branch     -> ref (Walk covers every sub-expression)
//	try(dependency.vpc.outputs.maybe, null)       -> NO ref (try tolerates a missing output)
//	can(dependency.vpc.outputs.x)                 -> NO ref
//	dependency.vpc.outputs (whole object)         -> NO ref
//	lookup(dependency.vpc.outputs, "id", "")      -> NO ref (3-step traversal, no 4th step)
//	dependency.vpc.outputs[local.k]               -> NO ref (index key is not a known string)
//	dependency.vpc.outputs.*.id                   -> NO ref (splat over the whole outputs object)
//	dependency.aurora["web"].outputs.id           -> NO ref (expanded dependency: TraverseIndex at step 1)
//	dependency.x, dependency.x.inputs.y           -> NO ref (wrong shape entirely)
//	dependencies.x.outputs.y, local.dependency... -> NO ref (wrong root name)
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
// suppressing matches nested inside a try(...) or can(...) call (research
// Pitfall 6): both tolerate a missing output, so reporting a reference
// inside one would be a false positive. guard counts enclosing try/can
// calls, so nested and sibling guards compose correctly (a try inside a
// try, or a try followed by an unguarded reference, both behave per the
// plan's OUT-05 case).
type refWalker struct {
	guard int
	hits  []rawRef
}

// Enter records a matching traversal when no try/can guards it, and enters
// a new guard level when n is a try(...) or can(...) call.
func (w *refWalker) Enter(n hclsyntax.Node) hcl.Diagnostics {
	switch e := n.(type) {
	case *hclsyntax.FunctionCallExpr:
		if e.Name == "try" || e.Name == "can" {
			w.guard++
		}
	case *hclsyntax.ScopeTraversalExpr:
		if w.guard == 0 {
			if dep, out, ok := outputRef(e.Traversal); ok {
				w.hits = append(w.hits, rawRef{dep: dep, out: out, start: e.SrcRange.Start})
			}
		}
	}
	return nil
}

// Exit restores the guard level on leaving a try(...) or can(...) call, so
// a reference written after the call (a sibling, not a nested argument) is
// reported again.
func (w *refWalker) Exit(n hclsyntax.Node) hcl.Diagnostics {
	if e, ok := n.(*hclsyntax.FunctionCallExpr); ok && (e.Name == "try" || e.Name == "can") {
		w.guard--
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
