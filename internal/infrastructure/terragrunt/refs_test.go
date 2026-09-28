package terragrunt

import (
	"fmt"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"

	"github.com/GiulioSavini/gruntled/internal/domain/repograph"
)

// parseBody parses src as a whole HCL config file, failing the test on a
// syntax error, and returns its *hclsyntax.Body alongside the exact bytes
// parsed (extractRefs needs both).
func parseBody(t *testing.T, filename, src string) (*hclsyntax.Body, []byte) {
	t.Helper()
	b := []byte(src)
	f, diags := hclsyntax.ParseConfig(b, filename, hcl.InitialPos)
	if diags.HasErrors() {
		t.Fatalf("parse %s: %v", filename, diags)
	}
	body, ok := f.Body.(*hclsyntax.Body)
	if !ok {
		t.Fatalf("parse %s: body is not *hclsyntax.Body", filename)
	}
	return body, b
}

// mustExtractRefs parses src and returns extractRefs' result, failing the
// test on any error.
func mustExtractRefs(t *testing.T, src string) []repograph.Reference {
	t.Helper()
	body, b := parseBody(t, "test.hcl", src)
	refs, err := extractRefs(repograph.MustRepoPath("test.hcl"), b, body)
	if err != nil {
		t.Fatalf("extractRefs: unexpected error: %v", err)
	}
	return refs
}

// wantRef is one expected (dependency, output) pair, ignoring position.
type wantRef struct {
	dep, out string
}

func assertRefPairs(t *testing.T, refs []repograph.Reference, want []wantRef) {
	t.Helper()
	if len(refs) != len(want) {
		t.Fatalf("got %d refs, want %d\ngot:  %s\nwant: %v", len(refs), len(want), dumpRefs(refs), want)
	}
	for i, r := range refs {
		w := want[i]
		if r.Dependency() != w.dep || r.Output() != w.out {
			t.Fatalf("refs[%d] = (%s, %s), want (%s, %s)\ngot:  %s\nwant: %v", i, r.Dependency(), r.Output(), w.dep, w.out, dumpRefs(refs), want)
		}
	}
}

func dumpRefs(refs []repograph.Reference) string {
	var b strings.Builder
	for _, r := range refs {
		fmt.Fprintf(&b, "(%s,%s)@%s ", r.Dependency(), r.Output(), r.Pos())
	}
	return b.String()
}

// TestExtractRefsShapes is the research Pattern 5 verified-shapes table,
// one case per row, each as a single-attribute HCL body.
func TestExtractRefsShapes(t *testing.T) {
	tests := []struct {
		name string
		src  string
		want []wantRef
	}{
		{"OUT-01 simple traversal", `inputs = { a = dependency.vpc.outputs.id }` + "\n", []wantRef{{"vpc", "id"}}},
		{"OUT-02 string index trailing step", `a = dependency.vpc.outputs.subnet_ids["private"]` + "\n", []wantRef{{"vpc", "subnet_ids"}}},
		{"OUT-02 number index trailing step", `a = dependency.vpc.outputs.subnet_ids[0]` + "\n", []wantRef{{"vpc", "subnet_ids"}}},
		{"OUT-02 attribute trailing step", `a = dependency.vpc.outputs.nested.deep` + "\n", []wantRef{{"vpc", "nested"}}},
		{"OUT-03 index splat after attribute", `a = dependency.vpc.outputs.subnet_ids[*]` + "\n", []wantRef{{"vpc", "subnet_ids"}}},
		{"OUT-03 attribute splat over whole outputs", `a = dependency.vpc.outputs.*.id` + "\n", nil},
		{"OUT-04 literal string index", `a = dependency.vpc.outputs["cidr"]` + "\n", []wantRef{{"vpc", "cidr"}}},
		{"OUT-05 try tolerates missing", `a = try(dependency.vpc.outputs.maybe, null)` + "\n", nil},
		{"OUT-05 can tolerates missing", `a = can(dependency.vpc.outputs.x)` + "\n", nil},
		{"OUT-05 try wraps nested call", `a = try(merge(dependency.vpc.outputs.a), {})` + "\n", nil},
		{"OUT-05 guard restored after try", `a = [try(dependency.a.outputs.x, 1), dependency.b.outputs.y]` + "\n", []wantRef{{"b", "y"}}},
		{"OUT-06 lookup on whole outputs", `a = lookup(dependency.vpc.outputs, "id", "")` + "\n", nil},
		{"OUT-07 whole object attribute", `inputs = dependency.vpc.outputs` + "\n", nil},
		{"OUT-08 dynamic index key", `a = dependency.vpc.outputs[local.k]` + "\n", nil},
		{"OUT-09 for expression", `a = [for s in dependency.vpc.outputs.subnet_ids : s]` + "\n", []wantRef{{"vpc", "subnet_ids"}}},
		{"OUT-10 ternary branches are lazy", `a = local.x ? dependency.nat.outputs.gw : dependency.igw.outputs.gw` + "\n", nil},
		{"OUT-14 merge on whole outputs", `a = merge(dependency.vpc.outputs, {a = 1})` + "\n", nil},
		{"expansion dependency (dynamic key)", `a = dependency.aurora["web"].outputs.id` + "\n", nil},
		{"bare dependency", `a = dependency.x` + "\n", nil},
		{"inputs, not outputs", `a = dependency.x.inputs.y` + "\n", nil},
		{"wrong root: dependencies", `a = dependencies.x.outputs.y` + "\n", nil},
		{"wrong root: local.dependency", `a = local.dependency.x.outputs.y` + "\n", nil},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			refs := mustExtractRefs(t, tc.src)
			assertRefPairs(t, refs, tc.want)
		})
	}
}

// TestExtractRefsWholeBody proves references are pulled from every block
// and attribute in a file's body, not just a top-level "inputs" attribute:
// locals, nested terraform hook blocks, remote_state, inside a dependency
// block's own mock_outputs, and inside the ordering-only dependencies
// block. Order must match source order.
func TestExtractRefsWholeBody(t *testing.T) {
	src := `
locals {
  v = dependency.vpc.outputs.id
}

terraform {
  extra_arguments "a" {
    arguments = [dependency.x.outputs.args]
  }
}

remote_state {
  config = {
    bucket = dependency.x.outputs.bucket
  }
}

dependency "y" {
  mock_outputs = {
    k = dependency.x.outputs.k
  }
}

dependencies {
  paths = [dependency.x.outputs.p]
}
`
	want := []wantRef{
		{"vpc", "id"},
		{"x", "args"},
		{"x", "bucket"},
		{"x", "k"},
		{"x", "p"},
	}
	refs := mustExtractRefs(t, src)
	assertRefPairs(t, refs, want)
}

// TestExtractRefsTemplatesAndHeredocs proves a reference embedded inside a
// template interpolation, including one written in a heredoc, is extracted.
func TestExtractRefsTemplatesAndHeredocs(t *testing.T) {
	src := "a = \"${dependency.x.outputs.y}-z\"\n" +
		"b = <<EOT\n" +
		"${dependency.x.outputs.h}\n" +
		"EOT\n"
	want := []wantRef{
		{"x", "y"},
		{"x", "h"},
	}
	refs := mustExtractRefs(t, src)
	assertRefPairs(t, refs, want)
}

// TestExtractRefsNonASCIIColumn is research Pitfall 2: the column must be
// counted in bytes, never in grapheme clusters, so a non-ASCII prefix on
// the same line shifts the reported column past what a rune count would
// give. The reference sits in a list literal, not a ternary branch, so the
// lazy-evaluation guard (TestExtractRefsLazyEvaluation) does not suppress
// it.
func TestExtractRefsNonASCIIColumn(t *testing.T) {
	src := `a = ["ééé", dependency.x.outputs.y]` + "\n"
	idx := strings.Index(src, "dependency")
	if idx < 0 {
		t.Fatalf("test source does not contain \"dependency\": %q", src)
	}
	wantCol := idx + 1 // 1-based byte column

	naiveRuneCol := utf8.RuneCountInString(src[:idx]) + 1
	if naiveRuneCol == wantCol {
		t.Fatalf("test source is not actually exercising byte vs. grapheme columns: both give %d", wantCol)
	}

	refs := mustExtractRefs(t, src)
	if len(refs) != 1 {
		t.Fatalf("got %d refs, want 1: %s", len(refs), dumpRefs(refs))
	}
	if got := refs[0].Pos().Column(); got != wantCol {
		t.Fatalf("Pos().Column() = %d, want %d (byte column, not grapheme column %d)", got, wantCol, naiveRuneCol)
	}
	if got := refs[0].Pos().Line(); got != 1 {
		t.Fatalf("Pos().Line() = %d, want 1", got)
	}
}

// TestExtractRefsOrderIsSourceOrderAcrossRuns proves the output order is
// always source order, even though hclsyntax.Body.Attributes is a map:
// extractRefs must sort by byte offset regardless of the walk's own
// iteration order. Ten attributes, re-parsed and re-extracted 20 times.
func TestExtractRefsOrderIsSourceOrderAcrossRuns(t *testing.T) {
	var b strings.Builder
	var want []wantRef
	for i := 0; i < 10; i++ {
		dep := fmt.Sprintf("d%d", i)
		out := fmt.Sprintf("o%d", i)
		fmt.Fprintf(&b, "a%d = dependency.%s.outputs.%s\n", i, dep, out)
		want = append(want, wantRef{dep, out})
	}
	src := b.String()

	for run := 0; run < 20; run++ {
		refs := mustExtractRefs(t, src)
		assertRefPairs(t, refs, want)
	}
}

// TestExtractRefsLazyEvaluation is 03-RESEARCH.md's Pattern 2: HCL evaluates
// a ternary's unselected branch, a short-circuited &&/|| operand, and a for
// expression's key/value/if once per element (so zero times over an empty
// collection) without surfacing their diagnostics. A reference in one of
// those positions must NOT be reported, exactly like the try()/can() guard.
// The ternary condition and the for collection are always evaluated and
// stay checked. Template directives (%{ if }, %{ for }) parse to the same
// node types, so the same rows prove the guard covers them too.
func TestExtractRefsLazyEvaluation(t *testing.T) {
	tests := []struct {
		name string
		src  string
		want []wantRef
	}{
		{"ternary: condition and both branches are local -> nothing", `a = local.c ? dependency.x.outputs.t : dependency.y.outputs.f` + "\n", nil},
		{"ternary: dependency condition with literal branches -> condition ref only", `a = dependency.c.outputs.flag ? 1 : 2` + "\n", []wantRef{{"c", "flag"}}},
		{"ternary: dependency condition, ref in true branch -> condition ref only", `a = dependency.c.outputs.flag ? dependency.x.outputs.t : null` + "\n", []wantRef{{"c", "flag"}}},
		{"&&: ref on RHS is short-circuited away", `a = local.b && dependency.x.outputs.v` + "\n", nil},
		{"&&: ref on LHS is short-circuited away", `a = dependency.x.outputs.v && local.b` + "\n", nil},
		{"||: ref on RHS is short-circuited away", `a = local.b || dependency.x.outputs.v` + "\n", nil},
		{"||: ref on LHS is short-circuited away", `a = dependency.x.outputs.v || local.b` + "\n", nil},
		{"unary negation is not lazy", `a = !dependency.x.outputs.v` + "\n", []wantRef{{"x", "v"}}},
		{"arithmetic is not lazy", `a = dependency.x.outputs.n + 1` + "\n", []wantRef{{"x", "n"}}},
		{"for: collection stays checked", `a = [for s in dependency.x.outputs.list : s]` + "\n", []wantRef{{"x", "list"}}},
		{"for: value expression is lazy", `a = [for s in local.l : dependency.x.outputs.v]` + "\n", nil},
		{"for: key expression is lazy", `a = {for k, v in local.m : dependency.x.outputs.k => v}` + "\n", nil},
		{"for: if-condition is lazy", `a = [for s in local.l : s if dependency.x.outputs.ok]` + "\n", nil},
		{"for: collection checked, key and value lazy", `a = {for k, v in dependency.x.outputs.m : k => dependency.y.outputs.z}` + "\n", []wantRef{{"x", "m"}}},
		{"template %{ if }: both branches lazy", `a = "%{ if local.c }${dependency.x.outputs.t}%{ else }${dependency.y.outputs.f}%{ endif }"` + "\n", nil},
		{"template %{ if }: condition ref stays checked", `a = "%{ if dependency.c.outputs.flag }x%{ endif }"` + "\n", []wantRef{{"c", "flag"}}},
		{"template %{ for }: body is lazy", `a = "%{ for s in local.l }${dependency.x.outputs.v}%{ endfor }"` + "\n", nil},
		{"template %{ for }: collection stays checked", `a = "%{ for s in dependency.x.outputs.l }${s}%{ endfor }"` + "\n", []wantRef{{"x", "l"}}},
		{"try() nested inside a lazy branch stays suppressed", `a = local.c ? try(dependency.x.outputs.t, null) : null` + "\n", nil},
		{"sibling ref after a lazy ternary is still checked", `a = [local.c ? dependency.x.outputs.t : null, dependency.y.outputs.after]` + "\n", []wantRef{{"y", "after"}}},
		{"nested ternary inside a lazy branch stays lazy", `a = local.c ? (local.d ? dependency.x.outputs.t : null) : null` + "\n", nil},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			refs := mustExtractRefs(t, tc.src)
			assertRefPairs(t, refs, tc.want)
		})
	}
}

// TestExtractRefsLazySiblingAttributesKeepPosition proves the lazy-range
// stack is popped correctly across sibling attributes: a ref suppressed
// inside one attribute's ternary must not leak suppression into, or miss,
// an unguarded ref with the identical shape on the next line.
func TestExtractRefsLazySiblingAttributesKeepPosition(t *testing.T) {
	src := "a = local.c ? dependency.x.outputs.t : null\n" +
		"b = dependency.x.outputs.t\n"
	refs := mustExtractRefs(t, src)
	if len(refs) != 1 {
		t.Fatalf("got %d refs, want 1: %s", len(refs), dumpRefs(refs))
	}
	if got := refs[0].Dependency(); got != "x" {
		t.Fatalf("refs[0].Dependency() = %q, want %q", got, "x")
	}
	if got := refs[0].Output(); got != "t" {
		t.Fatalf("refs[0].Output() = %q, want %q", got, "t")
	}
	if got := refs[0].Pos().Line(); got != 2 {
		t.Fatalf("refs[0].Pos().Line() = %d, want 2 (the unguarded sibling, not the lazy branch on line 1)", got)
	}
}
