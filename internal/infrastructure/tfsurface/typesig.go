package tfsurface

import (
	"encoding/json"
	"slices"
	"strconv"
	"strings"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/ext/typeexpr"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/zclconf/go-cty/cty"

	"github.com/GiulioSavini/gruntled/internal/infrastructure/hclconv"
)

// maxTypeDepth bounds the canonical renderer; deeper types are unknown.
// Real type constraints nest well under 10 levels.
const maxTypeDepth = 100

// maxJSONTypeBytes caps a decoded .tf.json type string; longer is unknown
// and never reaches typeexpr (sec #272). It bounds every recursion source
// in the string at once, before any scan runs over it.
const maxJSONTypeBytes = 64 << 10

// maxJSONTypeNesting bounds the ( [ { nesting of a .tf.json type string
// before it reaches typeexpr, which parses it with the recursive native
// parser and so bypasses CheckJSONDepth (sec #245).
const maxJSONTypeNesting = 2*maxTypeDepth + 2

// typeConstraint is typeexpr.TypeConstraintWithDefaults; a variable only so
// tests can count calls and prove a refused string never reaches it.
var typeConstraint = typeexpr.TypeConstraintWithDefaults

// typeSig returns ty's canonical string, or ("", false) for a capsule or
// unrecognised kind or a type nested deeper than maxTypeDepth. It never
// panics. Object attribute names are sorted and strconv.Quote'd, optional
// attributes are marked optional(T), and defaults are not part of the type,
// so whitespace, comments, attribute order, optional() defaults and the
// .tf / .tf.json spelling all render the same. Everything is written into
// one builder, so the output is linear in the type's size.
func typeSig(ty cty.Type) (string, bool) {
	var b strings.Builder
	if !writeType(&b, ty, 0) {
		return "", false
	}
	return b.String(), true
}

func writeType(b *strings.Builder, ty cty.Type, depth int) bool {
	if depth > maxTypeDepth {
		return false
	}
	switch {
	case ty.Equals(cty.DynamicPseudoType):
		b.WriteString("any")
	case ty.Equals(cty.String):
		b.WriteString("string")
	case ty.Equals(cty.Number):
		b.WriteString("number")
	case ty.Equals(cty.Bool):
		b.WriteString("bool")
	case ty.IsListType():
		return writeCollection(b, "list(", ty.ElementType(), depth)
	case ty.IsSetType():
		return writeCollection(b, "set(", ty.ElementType(), depth)
	case ty.IsMapType():
		return writeCollection(b, "map(", ty.ElementType(), depth)
	case ty.IsTupleType():
		b.WriteString("tuple([")
		for i, e := range ty.TupleElementTypes() {
			if i > 0 {
				b.WriteByte(',')
			}
			if !writeType(b, e, depth+1) {
				return false
			}
		}
		b.WriteString("])")
	case ty.IsObjectType():
		attrs := ty.AttributeTypes()
		names := make([]string, 0, len(attrs))
		for n := range attrs {
			names = append(names, n)
		}
		slices.Sort(names)
		b.WriteString("object({")
		for i, n := range names {
			if i > 0 {
				b.WriteByte(',')
			}
			b.WriteString(strconv.Quote(n))
			b.WriteByte('=')
			opt := ty.AttributeOptional(n)
			if opt {
				b.WriteString("optional(")
			}
			if !writeType(b, attrs[n], depth+1) {
				return false
			}
			if opt {
				b.WriteByte(')')
			}
		}
		b.WriteString("})")
	default:
		return false // capsule or a kind this renderer does not know
	}
	return true
}

func writeCollection(b *strings.Builder, open string, elem cty.Type, depth int) bool {
	b.WriteString(open)
	if !writeType(b, elem, depth+1) {
		return false
	}
	b.WriteByte(')')
	return true
}

// typeAttrSig parses a type constraint expression with
// typeexpr.TypeConstraintWithDefaults (defaults discarded) and renders it;
// ("", false) on any error diagnostic. For a .tf.json file (fromJSON) it
// first reads the raw string from src[expr.Range()] with encoding/json (a
// non-string JSON value is unknown; expr.Value is never used, as it would
// parse a `${...}` template) and returns ("", false), without calling
// typeexpr, when the decoded string exceeds maxJSONTypeBytes,
// jsonTypeNesting exceeds maxJSONTypeNesting, or hclconv.CheckNativeExprDepth
// refuses it, or the string holds a `for` keyword (hasForKeyword), in that
// order. A native .tf expression was already bounded by the whole-file
// CheckNativeDepth pre-scan, and typeexpr walks the AST that ParseConfig
// built without re-parsing it; it is unknown, without calling typeexpr,
// when it holds a for-expression anywhere (hclconv.HasForExpr).
//
// The for refusal is sec #317: typeexpr evaluates every optional() default
// with Value(nil), which stops variables and functions but not a for over
// a literal list, and nested fors multiply (1.3 KB took 45 s, the list form
// 6.7 GB). A type constraint never needs a for and defaults are discarded,
// so no real type is lost.
//
// It has no recover: FuzzTypeSig calls it directly so a library panic
// surfaces in CI. The reader calls it through safeTypeAttrSig.
func typeAttrSig(expr hcl.Expression, src []byte, fromJSON bool) (string, bool) {
	if fromJSON {
		s, ok := jsonTypeString(expr, src)
		if !ok || len(s) > maxJSONTypeBytes {
			return "", false
		}
		if jsonTypeNesting(s) > maxJSONTypeNesting {
			return "", false
		}
		if hclconv.CheckNativeExprDepth([]byte(s)) != nil {
			return "", false
		}
		if hasForKeyword(s) {
			return "", false
		}
	} else if hclconv.HasForExpr(expr) {
		return "", false
	}
	ty, _, diags := typeConstraint(expr)
	if diags.HasErrors() {
		return "", false
	}
	return typeSig(ty)
}

// jsonTypeString returns the JSON string at expr's source range in src,
// decoded with encoding/json: the same text hcl/json hands
// hclsyntax.ParseExpression when typeexpr asks for a call. ok is false when
// the range is not a JSON string.
func jsonTypeString(expr hcl.Expression, src []byte) (string, bool) {
	r := expr.Range()
	if r.Start.Byte < 0 || r.End.Byte > len(src) || r.Start.Byte >= r.End.Byte {
		return "", false
	}
	raw := src[r.Start.Byte:r.End.Byte]
	if raw[0] != '"' {
		return "", false
	}
	var s string
	if err := json.Unmarshal(raw, &s); err != nil {
		return "", false
	}
	return s, true
}

// hasForKeyword reports whether s, lexed as hcl/json hands it to
// hclsyntax.ParseExpression, holds an identifier token `for`: the keyword
// of every for-expression spelling ([for, {for and the template %{for}).
// A `for` inside a quoted literal is not an identifier and does not count;
// an attribute named for counts, which only makes the type unknown.
func hasForKeyword(s string) bool {
	toks, _ := hclsyntax.LexExpression([]byte(s), "", hcl.InitialPos)
	for _, t := range toks {
		if t.Type == hclsyntax.TokenIdent && string(t.Bytes) == "for" {
			return true
		}
	}
	return false
}

// jsonTypeNesting is a linear byte scan returning the maximum ( [ {
// nesting depth of s. Brackets inside quoted defaults count too, which only
// makes the answer more conservative; a stray closer never goes below 0.
func jsonTypeNesting(s string) int {
	depth, deepest := 0, 0
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '(', '[', '{':
			depth++
			deepest = max(deepest, depth)
		case ')', ']', '}':
			depth = max(depth-1, 0)
		}
	}
	return deepest
}

// parseTypeFact is the seam the fact reader calls; tests swap it for a
// panicking parser. The production value is safeTypeAttrSig.
var parseTypeFact = safeTypeAttrSig

// safeTypeAttrSig is typeAttrSig behind a narrow recover: a typeexpr or cty
// panic on unforeseen input makes the one fact unknown instead of crashing
// check, report, watch or the daemon (sec #246).
//
// The recover cannot catch a stack overflow: Go turns one into a fatal
// error that ends the process, never a panic. Deep input is stopped only
// by the pre-scans (CheckNativeDepth on the whole file and, for a .tf.json
// type string, the 64 KiB cap, the bracket scan and CheckNativeExprDepth);
// nothing here may count on this recover for that (sec #262a).
func safeTypeAttrSig(expr hcl.Expression, src []byte, fromJSON bool) (sig string, ok bool) {
	defer func() {
		if recover() != nil {
			sig, ok = "", false
		}
	}()
	return typeAttrSig(expr, src, fromJSON)
}
