package terragrunt

import (
	"github.com/hashicorp/hcl/v2/hclsyntax"

	"github.com/GiulioSavini/gruntled/internal/domain/repograph"
)

// dependencyOptions reads the DIAG-03 facts a `dependency` block's own body
// carries -- enabled, skip_outputs, mock_outputs, the two
// mock_outputs_merge*_with_state attributes, and
// mock_outputs_allowed_terraform_commands -- as literals or "not literal"
// (unknown), never interpreted: Phase 3 decides what to do with these
// facts, Phase 2 only captures them faithfully. body is the dependency
// block's own *hclsyntax.Body (block.Body), not the file it appears in. An
// attribute absent from body takes Terragrunt's own default, per
// DefaultDependencyOptions.
func dependencyOptions(body *hclsyntax.Body) repograph.DependencyOptions {
	opts := repograph.DefaultDependencyOptions()

	if attr, ok := body.Attributes["enabled"]; ok {
		opts.Enabled = literalBool(attr.Expr)
	}
	if attr, ok := body.Attributes["skip_outputs"]; ok {
		opts.SkipOutputs = literalBool(attr.Expr)
	}
	opts.MockOutputs = mockOutputKeys(body)
	opts.MockMergeWithState = mockMergeWithState(body)
	opts.MockAllowedCommands = allowedCommands(body)

	return opts
}

// mockOutputKeys reads mock_outputs' key set: AbsentNames() when the
// attribute is missing, UnknownNames() when it is present but not a
// literal object expression, has any non-literal or computed key, or has a
// duplicate key (deep-merge and any other shape this domain does not
// model, per research Open Question 6), or KnownNames(keys) for a literal
// object, including an empty one ({} is Known [], distinct from absent).
func mockOutputKeys(body *hclsyntax.Body) repograph.NameList {
	attr, ok := body.Attributes["mock_outputs"]
	if !ok {
		return repograph.AbsentNames()
	}
	obj, ok := attr.Expr.(*hclsyntax.ObjectConsExpr)
	if !ok {
		return repograph.UnknownNames()
	}

	keys := make([]string, 0, len(obj.Items))
	seen := make(map[string]bool, len(obj.Items))
	for _, item := range obj.Items {
		k, ok := literalString(item.KeyExpr)
		if !ok {
			return repograph.UnknownNames()
		}
		if seen[k] {
			return repograph.UnknownNames()
		}
		seen[k] = true
		keys = append(keys, k)
	}

	names, err := repograph.KnownNames(keys)
	if err != nil {
		// KnownNames only rejects an empty-string name; an HCL object
		// constructor can syntactically have one ({ "" = 1 }), so this
		// stays fail-safe (unknown) rather than propagating an error a
		// caller has nowhere sensible to send.
		return repograph.UnknownNames()
	}
	return names
}

// mockMergeWithState combines mock_outputs_merge_with_state and
// mock_outputs_merge_strategy_with_state exactly as Terragrunt reads them
// together: the fact is True when EITHER attribute literally says so, a
// literal True from either one is never overridden by the other being
// non-literal, and the fact is Unknown only when neither is literally True
// and at least one of them is present but not literal (or not one of the
// three strategy values this domain recognizes).
func mockMergeWithState(body *hclsyntax.Body) repograph.Tristate {
	merge := repograph.TristateFalse
	if attr, ok := body.Attributes["mock_outputs_merge_with_state"]; ok {
		merge = literalBool(attr.Expr)
	}

	strategy := repograph.TristateFalse
	if attr, ok := body.Attributes["mock_outputs_merge_strategy_with_state"]; ok {
		s, litOK := literalString(attr.Expr)
		if !litOK {
			strategy = repograph.TristateUnknown
		} else {
			strategy = mergeStrategyState(s)
		}
	}

	switch {
	case merge == repograph.TristateTrue || strategy == repograph.TristateTrue:
		return repograph.TristateTrue
	case merge == repograph.TristateUnknown || strategy == repograph.TristateUnknown:
		return repograph.TristateUnknown
	default:
		return repograph.TristateFalse
	}
}

// mergeStrategyState maps mock_outputs_merge_strategy_with_state's three
// Terragrunt-recognized literal values: "no_merge" means the state is not
// merged, "shallow" and "deep_map_only" both mean it is. Any other literal
// string is not a value this domain recognizes, so it is Unknown rather
// than guessed as either true or false.
func mergeStrategyState(v string) repograph.Tristate {
	switch v {
	case "no_merge":
		return repograph.TristateFalse
	case "shallow", "deep_map_only":
		return repograph.TristateTrue
	default:
		return repograph.TristateUnknown
	}
}

// allowedCommands reads mock_outputs_allowed_terraform_commands:
// AbsentNames() when missing, UnknownNames() when present but not a
// literal list of string literals, or KnownNames(names) for a literal
// list, including an empty one.
func allowedCommands(body *hclsyntax.Body) repograph.NameList {
	attr, ok := body.Attributes["mock_outputs_allowed_terraform_commands"]
	if !ok {
		return repograph.AbsentNames()
	}
	tuple, ok := attr.Expr.(*hclsyntax.TupleConsExpr)
	if !ok {
		return repograph.UnknownNames()
	}

	names := make([]string, 0, len(tuple.Exprs))
	for _, e := range tuple.Exprs {
		s, ok := literalString(e)
		if !ok {
			return repograph.UnknownNames()
		}
		names = append(names, s)
	}

	got, err := repograph.KnownNames(names)
	if err != nil {
		return repograph.UnknownNames()
	}
	return got
}
