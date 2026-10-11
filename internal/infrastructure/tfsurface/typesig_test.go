package tfsurface

import (
	"context"
	"encoding/json"
	"reflect"
	"slices"
	"strings"
	"testing"
	"testing/fstest"
	"time"
	"unicode/utf8"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/ext/typeexpr"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	hcljson "github.com/hashicorp/hcl/v2/json"
	"github.com/zclconf/go-cty/cty"

	"github.com/GiulioSavini/gruntled/internal/domain/repograph"
	"github.com/GiulioSavini/gruntled/internal/infrastructure/hclconv"
)

// typeSchema selects a variable block's type attribute.
var typeSchema = &hcl.BodySchema{Attributes: []hcl.AttributeSchema{{Name: "type"}}}

// typeExprs returns every `type` expression of every variable block in
// body, in source order.
func typeExprs(t testing.TB, body hcl.Body) []hcl.Expression {
	t.Helper()
	content, _, diags := body.PartialContent(moduleSchema)
	if diags.HasErrors() {
		return nil
	}
	var out []hcl.Expression
	for _, b := range content.Blocks {
		attrs, _, adiags := b.Body.PartialContent(typeSchema)
		if adiags.HasErrors() {
			continue
		}
		if a, ok := attrs.Attributes["type"]; ok {
			out = append(out, a.Expr)
		}
	}
	return out
}

// nativeType parses `variable "v" { type = <typ> }` and returns the type
// expression and the source.
func nativeType(t testing.TB, typ string) (hcl.Expression, []byte) {
	t.Helper()
	src := []byte("variable \"v\" {\n  type = " + typ + "\n}\n")
	f, diags := hclsyntax.ParseConfig(src, "main.tf", hcl.InitialPos)
	if diags.HasErrors() {
		t.Fatalf("parse %q: %v", typ, diags)
	}
	exprs := typeExprs(t, f.Body)
	if len(exprs) != 1 {
		t.Fatalf("parse %q: %d type expressions", typ, len(exprs))
	}
	return exprs[0], src
}

// jsonSrc is a .tf.json declaring variable v with raw as its JSON type value.
func jsonSrc(raw string) []byte {
	return []byte(`{"variable":{"v":{"type":` + raw + `}}}`)
}

// jsonType parses jsonSrc(raw) and returns the type expression and the source.
func jsonType(t testing.TB, raw string) (hcl.Expression, []byte) {
	t.Helper()
	src := jsonSrc(raw)
	f, diags := hcljson.Parse(src, "main.tf.json")
	if diags.HasErrors() {
		t.Fatalf("parse json: %v", diags)
	}
	exprs := typeExprs(t, f.Body)
	if len(exprs) != 1 {
		t.Fatalf("parse json: %d type expressions", len(exprs))
	}
	return exprs[0], src
}

// jsonQuote returns s as a JSON string literal.
func jsonQuote(s string) string {
	b, err := json.Marshal(s)
	if err != nil {
		panic(err)
	}
	return string(b)
}

// countTypeexpr replaces the typeexpr seam with a counting wrapper for the
// test's duration and returns the counter.
func countTypeexpr(t *testing.T) *int {
	t.Helper()
	n := new(int)
	orig := typeConstraint
	typeConstraint = func(expr hcl.Expression) (cty.Type, *typeexpr.Defaults, hcl.Diagnostics) {
		*n++
		return orig(expr)
	}
	t.Cleanup(func() { typeConstraint = orig })
	return n
}

func TestTypeSigGolden(t *testing.T) {
	const objXY = `object({"x"=optional(string),"y"=number})`
	native := []struct {
		src, want string
		ok        bool
	}{
		{"string", "string", true},
		{"list(string)", "list(string)", true},
		{"tuple([string,number])", "tuple([string,number])", true},
		{"list(any)", "list(any)", true},
		{"any", "any", true},
		{"map(bool)", "map(bool)", true},
		{"set(number)", "set(number)", true},
		{`object({ y = number, x = optional(string, "d") })`, objXY, true},
		{"object({x = optional(string), y = number})", objXY, true},
		{"object({\n  # c\n  y = number, // d\n  x = optional(string)\n})", objXY, true},
		{"object({x=string, y=number})", `object({"x"=string,"y"=number})`, true},
		{"set(map(list(object({é = string, ж = number}))))", `set(map(list(object({"é"=string,"ж"=number}))))`, true},
		{"list", "", false},
		{"map", "", false},
		{`"string"`, "", false},
		{"optional(string)", "", false},
		{"object({x = optional(string, var.x)})", "", false},
		{`object({x = optional(string, upper("x"))})`, "", false},
		{"strng", "", false},
		{`object({"q z" = bool})`, "", false},
		{`object({"\u202e" = string})`, "", false},
	}
	for _, c := range native {
		t.Run("native "+c.src, func(t *testing.T) {
			expr, src := nativeType(t, c.src)
			got, ok := typeAttrSig(expr, src, false)
			if got != c.want || ok != c.ok {
				t.Fatalf("typeAttrSig(%s) = (%q, %v), want (%q, %v)", c.src, got, ok, c.want, c.ok)
			}
		})
	}

	jsonCases := []struct {
		raw, want string
		ok        bool
	}{
		{`"list(string)"`, "list(string)", true},
		{`" list( string ) "`, "list(string)", true},
		{`"object({y=number, x=optional(string, \"d\")})"`, objXY, true},
		{`"any"`, "any", true},
		{`"string"`, "string", true},
		{`"list"`, "", false},
		{`"${list(string)}"`, "", false},
		{`"${string}"`, "", false},
		{`["list","string"]`, "", false},
		{`{"a":"string"}`, "", false},
		{`1`, "", false},
	}
	for _, c := range jsonCases {
		t.Run("json "+c.raw, func(t *testing.T) {
			expr, src := jsonType(t, c.raw)
			got, ok := typeAttrSig(expr, src, true)
			if got != c.want || ok != c.ok {
				t.Fatalf("typeAttrSig(%s) = (%q, %v), want (%q, %v)", c.raw, got, ok, c.want, c.ok)
			}
		})
	}
}

// nestList returns list(...list(leaf)...) n levels deep.
func nestList(n int, leaf cty.Type) cty.Type {
	ty := leaf
	for range n {
		ty = cty.List(ty)
	}
	return ty
}

func TestTypeSigCapsuleAndDepth(t *testing.T) {
	capsule := cty.Capsule("x", reflect.TypeOf(0))
	for _, ty := range []cty.Type{capsule, cty.List(capsule), cty.Object(map[string]cty.Type{"a": capsule}), cty.Tuple([]cty.Type{cty.String, capsule})} {
		if got, ok := typeSig(ty); ok || got != "" {
			t.Fatalf("typeSig(%#v) = (%q, %v), want (\"\", false)", ty, got, ok)
		}
	}
	if got, ok := typeSig(nestList(maxTypeDepth, cty.String)); !ok || got != strings.Repeat("list(", maxTypeDepth)+"string"+strings.Repeat(")", maxTypeDepth) {
		t.Fatalf("typeSig(list x %d) = (%q, %v), want the rendered type", maxTypeDepth, got, ok)
	}
	if got, ok := typeSig(nestList(maxTypeDepth+1, cty.String)); ok || got != "" {
		t.Fatalf("typeSig(list x %d) = (%q, %v), want (\"\", false)", maxTypeDepth+1, got, ok)
	}

	deep := strings.Repeat("list(", 900) + "string" + strings.Repeat(")", 900)
	expr, src := nativeType(t, deep)
	if err := hclconv.CheckNativeDepth(src); err != nil {
		t.Fatalf("900-deep source refused by the pre-scan: %v", err)
	}
	if got, ok := typeAttrSig(expr, src, false); ok || got != "" {
		t.Fatalf("typeAttrSig(900-deep) = (%q, %v), want (\"\", false)", got, ok)
	}
}

// xorshift drives the generated-type property; no math/rand (house rule).
type xorshift uint64

func (x *xorshift) intn(n int) int {
	*x ^= *x << 13
	*x ^= *x >> 7
	*x ^= *x << 17
	return int(uint64(*x) % uint64(n))
}

var genNames = []string{"a", "b", "c", "é"}

// genType builds a random type at most maxDepth constructors deep from a
// small alphabet, so equal pairs occur often.
func genType(r *xorshift, depth int) cty.Type {
	k := r.intn(9)
	if depth >= 5 {
		k = r.intn(4)
	}
	switch k {
	case 0:
		return cty.DynamicPseudoType
	case 1:
		return cty.String
	case 2:
		return cty.Number
	case 3:
		return cty.Bool
	case 4:
		return cty.List(genType(r, depth+1))
	case 5:
		return cty.Set(genType(r, depth+1))
	case 6:
		return cty.Map(genType(r, depth+1))
	case 7:
		elems := make([]cty.Type, r.intn(3))
		for i := range elems {
			elems[i] = genType(r, depth+1)
		}
		return cty.Tuple(elems)
	default:
		attrs := map[string]cty.Type{}
		var optional []string
		for range r.intn(4) {
			n := genNames[r.intn(len(genNames))]
			attrs[n] = genType(r, depth+1)
			if r.intn(2) == 0 && !slices.Contains(optional, n) {
				optional = append(optional, n)
			}
		}
		optional = slices.DeleteFunc(optional, func(n string) bool { _, ok := attrs[n]; return !ok })
		if len(optional) == 0 {
			return cty.Object(attrs)
		}
		return cty.ObjectWithOptionalAttrs(attrs, optional)
	}
}

// rebuild returns an equal type built again, object attributes inserted in
// reverse sorted order.
func rebuild(ty cty.Type) cty.Type {
	switch {
	case ty.IsListType():
		return cty.List(rebuild(ty.ElementType()))
	case ty.IsSetType():
		return cty.Set(rebuild(ty.ElementType()))
	case ty.IsMapType():
		return cty.Map(rebuild(ty.ElementType()))
	case ty.IsTupleType():
		var elems []cty.Type
		for _, e := range ty.TupleElementTypes() {
			elems = append(elems, rebuild(e))
		}
		return cty.Tuple(elems)
	case ty.IsObjectType():
		names := slices.Sorted(func(yield func(string) bool) {
			for n := range ty.AttributeTypes() {
				if !yield(n) {
					return
				}
			}
		})
		slices.Reverse(names)
		attrs := map[string]cty.Type{}
		var optional []string
		for _, n := range names {
			attrs[n] = rebuild(ty.AttributeType(n))
			if ty.AttributeOptional(n) {
				optional = append(optional, n)
			}
		}
		if len(optional) == 0 {
			return cty.Object(attrs)
		}
		return cty.ObjectWithOptionalAttrs(attrs, optional)
	}
	return ty
}

func TestTypeSigEqualsProperty(t *testing.T) {
	r := xorshift(0x15_02)
	equalPairs, distinctPairs := 0, 0
	for i := range 2000 {
		a, b := genType(&r, 0), genType(&r, 0)
		if i%4 == 0 {
			b = rebuild(a)
		}
		sa, oka := typeSig(a)
		sb, okb := typeSig(b)
		if !oka || !okb {
			t.Fatalf("case %d: typeSig refused a generated type: %#v / %#v", i, a, b)
		}
		if (sa == sb) != a.Equals(b) {
			t.Fatalf("case %d: typeSig equality %v (%q vs %q) but Equals %v", i, sa == sb, sa, sb, a.Equals(b))
		}
		if again, _ := typeSig(a); again != sa {
			t.Fatalf("case %d: typeSig not deterministic: %q then %q", i, sa, again)
		}
		if sa == sb {
			equalPairs++
		} else {
			distinctPairs++
		}
	}
	if equalPairs < 100 || distinctPairs < 100 {
		t.Fatalf("vacuous property: %d equal pairs, %d distinct pairs", equalPairs, distinctPairs)
	}
}

func TestTypeSigLinear(t *testing.T) {
	attrs := make(map[string]cty.Type, 100_000)
	sum := 0
	for i := range 100_000 {
		n := "a" + strings.Repeat("x", i%7) + string(rune('a'+i%26)) + string(rune('0'+i%10)) + itoa(i)
		attrs[n] = cty.String
		sum += len(n) + len("string")
	}
	start := time.Now()
	got, ok := typeSig(cty.Object(attrs))
	t.Logf("100,000-attribute object rendered in %v, %d bytes", time.Since(start), len(got))
	if !ok {
		t.Fatal("typeSig refused a 100,000-attribute object")
	}
	if len(got) > 2*sum {
		t.Fatalf("rendered %d bytes, over 2x the %d bytes of names and types", len(got), sum)
	}
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	var b []byte
	for ; i > 0; i /= 10 {
		b = append([]byte{byte('0' + i%10)}, b...)
	}
	return string(b)
}

// assertJSONRefused runs typeAttrSig over a .tf.json type string decoded
// to s and asserts an unknown fact, typeexpr never called, under 1 s.
func assertJSONRefused(t *testing.T, s string) {
	t.Helper()
	calls := countTypeexpr(t)
	src := jsonSrc(jsonQuote(s))
	if len(src) > hclconv.MaxFileBytes {
		t.Fatalf("test input %d bytes, over MaxFileBytes", len(src))
	}
	if err := hclconv.CheckJSONDepth(src); err != nil {
		t.Fatalf("CheckJSONDepth refused the file: %v", err)
	}
	expr, _ := jsonType(t, jsonQuote(s))
	start := time.Now()
	got, ok := typeAttrSig(expr, src, true)
	took := time.Since(start)
	t.Logf("%d-byte type string refused in %v", len(s), took)
	if ok || got != "" {
		t.Fatalf("typeAttrSig = (%q, %v), want (\"\", false)", got, ok)
	}
	if *calls != 0 {
		t.Fatalf("typeexpr called %d times, want 0", *calls)
	}
	if took > time.Second {
		t.Fatalf("took %v, over 1 s", took)
	}
}

func deepList(n int) string {
	return strings.Repeat("list(", n) + "string" + strings.Repeat(")", n)
}

func TestTypeSigJSONDeepString(t *testing.T) {
	for _, n := range []int{2_000, 400_000} {
		t.Run("list x "+itoa(n), func(t *testing.T) { assertJSONRefused(t, deepList(n)) })
		t.Run("${} list x "+itoa(n), func(t *testing.T) { assertJSONRefused(t, "${"+deepList(n)+"}") })
	}
	t.Run("escaped paren list x 2000", func(t *testing.T) {
		calls := countTypeexpr(t)
		raw := `"` + strings.Repeat(`list\u0028`, 2_000) + "string" + strings.Repeat(")", 2_000) + `"`
		expr, src := jsonType(t, raw)
		if got, ok := typeAttrSig(expr, src, true); ok || got != "" || *calls != 0 {
			t.Fatalf("typeAttrSig = (%q, %v), typeexpr calls %d; want (\"\", false), 0", got, ok, *calls)
		}
	})
	t.Run("list x 50 parses", func(t *testing.T) {
		expr, src := jsonType(t, jsonQuote(deepList(50)))
		if got, ok := typeAttrSig(expr, src, true); !ok || got != deepList(50) {
			t.Fatalf("typeAttrSig = (%q, %v), want (%q, true)", got, ok, deepList(50))
		}
	})
}

// wideObject returns object({...}) of string attributes padded with spaces
// to exactly size bytes.
func wideObject(size int) string {
	var b strings.Builder
	b.WriteString("object({")
	for i := 0; b.Len() < size-40; i++ {
		b.WriteString("a" + itoa(i) + "=string,")
	}
	pad := size - b.Len() - len("})")
	b.WriteString(strings.Repeat(" ", pad))
	b.WriteString("})")
	return b.String()
}

func TestTypeSigJSONNewlineTernary(t *testing.T) {
	t.Run("690,000 newline ternaries (sec #272): the 64 KiB cap", func(t *testing.T) {
		assertJSONRefused(t, strings.Repeat("a?b:\n", 690_000)+"c")
	})
	t.Run("2,000 newline ternaries: CheckNativeExprDepth", func(t *testing.T) {
		s := strings.Repeat("a?b:\n", 2_000) + "c"
		if len(s) > maxJSONTypeBytes || jsonTypeNesting(s) > maxJSONTypeNesting {
			t.Fatal("test input does not reach CheckNativeExprDepth")
		}
		if err := hclconv.CheckNativeDepth([]byte(s)); err != nil {
			t.Fatalf("config-mode scan refused it (%v): the case would not show the mode difference", err)
		}
		assertJSONRefused(t, s)
	})
	t.Run("benign object of 64 KiB + 1", func(t *testing.T) {
		s := wideObject(maxJSONTypeBytes + 1)
		if len(s) != maxJSONTypeBytes+1 {
			t.Fatalf("built %d bytes", len(s))
		}
		assertJSONRefused(t, s)
	})
	t.Run("realistic 60 KiB object parses", func(t *testing.T) {
		calls := countTypeexpr(t)
		s := wideObject(60 << 10)
		expr, src := jsonType(t, jsonQuote(s))
		got, ok := typeAttrSig(expr, src, true)
		if !ok || !strings.HasPrefix(got, `object({"a0"=string,`) || *calls != 1 {
			t.Fatalf("typeAttrSig = (%.40q..., %v), typeexpr calls %d; want a rendered object and 1 call", got, ok, *calls)
		}
	})
}

// nestedIfDefault is an object type whose optional() default nests n
// %{if} blocks.
func nestedIfDefault(n int) string {
	return `object({a=optional(string,"` + strings.Repeat("%{if a}", n) + "x" + strings.Repeat("%{endif}", n) + `")})`
}

// lowBracketBombs are type strings with ( [ { nesting <= 4 that recurse
// in typeexpr's native parser (sec #260); big ones stop at the 64 KiB cap,
// small ones (under the cap) at CheckNativeExprDepth.
var lowBracketBombs = []struct {
	name       string
	big, small string
}{
	{"bang", strings.Repeat("!", 2_000_000) + "x", strings.Repeat("!", 65_000) + "x"},
	{"minus", strings.Repeat("-", 2_000_000) + "1", strings.Repeat("-", 65_000) + "1"},
	{"ternary", strings.Repeat("a?b:", 1_000_000) + "c", strings.Repeat("a?b:", 16_000) + "c"},
	{"nested-if default", nestedIfDefault(250_000), nestedIfDefault(4_000)},
}

func TestTypeSigJSONLowBracketBombs(t *testing.T) {
	for _, c := range lowBracketBombs {
		t.Run(c.name+" big", func(t *testing.T) { assertJSONRefused(t, c.big) })
		t.Run(c.name+" under the cap", func(t *testing.T) {
			if len(c.small) > maxJSONTypeBytes || jsonTypeNesting(c.small) > maxJSONTypeNesting {
				t.Fatalf("%d bytes, nesting %d: does not reach CheckNativeExprDepth", len(c.small), jsonTypeNesting(c.small))
			}
			assertJSONRefused(t, c.small)
		})
	}
	t.Run("heredoc with nested if (sec #280b)", func(t *testing.T) {
		s := "object({a=optional(string,<<EOT\n" + strings.Repeat("%{if a}\n", 2_000) + "x\n" + strings.Repeat("%{endif}\n", 2_000) + "EOT\n)})"
		if len(s) > maxJSONTypeBytes || jsonTypeNesting(s) > maxJSONTypeNesting {
			t.Fatal("does not reach CheckNativeExprDepth")
		}
		assertJSONRefused(t, s)
	})
}

func TestTypeSigNativeNestedIfDefault(t *testing.T) {
	calls := countTypeexpr(t)
	tf := "variable \"v\" {\n  type = " + nestedIfDefault(270_000) + "\n}\n"
	if len(tf) > hclconv.MaxFileBytes {
		t.Fatalf("%d bytes, over MaxFileBytes", len(tf))
	}
	fsys := fstest.MapFS{"m/main.tf": {Data: []byte(tf)}}
	start := time.Now()
	res, err := NewReader(fsys).ReadSurface(context.Background(), repograph.MustRepoPath("m"))
	t.Logf("%d bytes read in %v", len(tf), time.Since(start))
	if err != nil {
		t.Fatal(err)
	}
	if res.UnknownReason != ReasonModuleFileTooDeep || *calls != 0 {
		t.Fatalf("UnknownReason %q, typeexpr calls %d; want %q, 0", res.UnknownReason, *calls, ReasonModuleFileTooDeep)
	}
}

func TestJSONTypeStringExtraction(t *testing.T) {
	cases := []struct {
		raw, want string
		nesting   int
	}{
		{`"list\u0028string)"`, "list(string)", 1},
		{`"\u006cist(\u0028string))"`, "list((string))", 2},
		{`"object({a=optional(string,\"x\\\\y\")})"`, `object({a=optional(string,"x\\y")})`, 3},
		{`"object({a=optional(string,\"a\/b\")})"`, `object({a=optional(string,"a/b")})`, 3},
		{`"object({a=optional(string,\"\u005b\")})"`, `object({a=optional(string,"[")})`, 4},
	}
	for _, c := range cases {
		t.Run(c.raw, func(t *testing.T) {
			expr, src := jsonType(t, c.raw)
			got, ok := jsonTypeString(expr, src)
			if !ok || got != c.want {
				t.Fatalf("jsonTypeString = (%q, %v), want (%q, true)", got, ok, c.want)
			}
			// What hcl/json itself hands ParseExpression; benign inputs only.
			v, diags := expr.Value(nil)
			if diags.HasErrors() || !v.Type().Equals(cty.String) || v.AsString() != got {
				t.Fatalf("hcl/json value %#v (%v) differs from the extracted %q", v, diags, got)
			}
			if n := jsonTypeNesting(got); n != c.nesting {
				t.Fatalf("jsonTypeNesting(%q) = %d, want %d", got, n, c.nesting)
			}
		})
	}
}

func TestTypeSigNativeDeepBounded(t *testing.T) {
	expr, src := nativeType(t, deepList(999))
	if err := hclconv.CheckNativeDepth(src); err != nil {
		t.Fatalf("999 nested list( refused by the pre-scan: %v", err)
	}
	start := time.Now()
	got, ok := typeAttrSig(expr, src, false)
	took := time.Since(start)
	t.Logf("999 nested list( in %v", took)
	if ok || got != "" {
		t.Fatalf("typeAttrSig = (%q, %v), want (\"\", false)", got, ok)
	}
	if took > 50*time.Millisecond {
		t.Fatalf("took %v, over 50 ms", took)
	}
	if err := hclconv.CheckNativeDepth([]byte("variable \"v\" {\n  type = " + deepList(1000) + "\n}\n")); err == nil {
		t.Fatal("1,000 nested list( passed the pre-scan")
	}
}

// fuzzSig parses src with parse and runs typeAttrSig twice over every
// type expression, checking it never panics (no recover here), returns
// valid UTF-8 and is stable.
func fuzzSig(t *testing.T, body hcl.Body, src []byte, fromJSON bool) {
	for _, expr := range typeExprs(t, body) {
		a, oka := typeAttrSig(expr, src, fromJSON)
		b, okb := typeAttrSig(expr, src, fromJSON)
		if a != b || oka != okb {
			t.Fatalf("unstable: (%q, %v) then (%q, %v)", a, oka, b, okb)
		}
		if !utf8.ValidString(a) {
			t.Fatalf("invalid UTF-8 result %q", a)
		}
		if !oka && a != "" {
			t.Fatalf("refused with a non-empty result %q", a)
		}
	}
}

func FuzzTypeSig(f *testing.F) {
	for _, s := range []string{
		"string", "list(string)", "tuple([string,number])", "any", "map(bool)",
		`object({ y = number, x = optional(string, "d") })`,
		"object({x = optional(string), y = number})",
		"set(map(list(object({é = string, ж = number}))))",
		"list", `"string"`, "optional(string)", "strng", `object({"q z" = bool})`,
		"${list(string)}",
		deepList(5_000),
		deepList(999),
		strings.Repeat("!", 5_000) + "x",
		nestedIfDefault(1_100),
		strings.Repeat("a?b:\n", 1_100) + "c",
	} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		native := []byte("variable \"v\" {\n  type = " + s + "\n}\n")
		if len(native) <= hclconv.MaxFileBytes && hclconv.CheckNativeDepth(native) == nil {
			if file, diags := hclsyntax.ParseConfig(native, "main.tf", hcl.InitialPos); !diags.HasErrors() {
				fuzzSig(t, file.Body, native, false)
			}
		}
		js := jsonSrc(jsonQuote(s))
		if len(js) <= hclconv.MaxFileBytes && hclconv.CheckJSONDepth(js) == nil {
			if file, diags := hcljson.Parse(js, "main.tf.json"); !diags.HasErrors() {
				fuzzSig(t, file.Body, js, true)
			}
		}
	})
}
