package tfsurface

import (
	"context"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/ext/typeexpr"
	"github.com/zclconf/go-cty/cty"

	"github.com/GiulioSavini/gruntled/internal/domain/repograph"
	"github.com/GiulioSavini/gruntled/internal/infrastructure/hclconv"
)

const (
	tTrue    = repograph.TristateTrue
	tFalse   = repograph.TristateFalse
	tUnknown = repograph.TristateUnknown
)

// readFacts reads module m from files and fails unless the surface is known.
func readFacts(t *testing.T, files map[string]string) repograph.Surface {
	t.Helper()
	fsys := fstest.MapFS{}
	for name, data := range files {
		fsys["m/"+name] = &fstest.MapFile{Data: []byte(data)}
	}
	res, err := NewReader(fsys).ReadSurface(context.Background(), repograph.MustRepoPath("m"))
	if err != nil {
		t.Fatal(err)
	}
	if res.UnknownReason != "" {
		t.Fatalf("UnknownReason = %q (diags %v)", res.UnknownReason, res.Diagnostics)
	}
	return res.Surface
}

func wantVar(t *testing.T, s repograph.Surface, want repograph.VariableDecl) {
	t.Helper()
	got, ok := s.Variable(want.Name)
	if !ok || got != want {
		t.Fatalf("Variable(%q) = %+v, %v; want %+v", want.Name, got, ok, want)
	}
}

func wantOut(t *testing.T, s repograph.Surface, want repograph.OutputDecl) {
	t.Helper()
	got, ok := s.Output(want.Name)
	if !ok || got != want {
		t.Fatalf("Output(%q) = %+v, %v; want %+v", want.Name, got, ok, want)
	}
}

func TestReadSurfaceFacts(t *testing.T) {
	s := readFacts(t, map[string]string{
		"main.tf": `
variable "a" {}
variable "n" { default = null }
variable "l" { type = list(string) }
variable "bad" { type = strng }
variable "d" {
  type    = string
  default = "x"
}
output "o" { value = 1 }
output "typed" {
  value = 1
  type  = string
}
output "s" {
  value     = 1
  sensitive = true
}
output "sv" {
  value     = 1
  sensitive = var.x
}
output "ss" {
  value     = 1
  sensitive = "true"
}
output "sf" {
  value     = 1
  sensitive = false
}
`,
		"j.tf.json": `{
  "variable": {"jn": {"default": null}, "jt": {"type": "map(number)"}},
  "output": {"js": {"value": 1, "sensitive": true}, "jdeep": {"value": 1, "sensitive": "${` + strings.Repeat("(", 5_000) + "1" + strings.Repeat(")", 5_000) + `}"}}
}`,
	})
	wantVar(t, s, repograph.VariableDecl{Name: "a", Required: tTrue, Type: "any"})
	wantVar(t, s, repograph.VariableDecl{Name: "n", Required: tFalse, Type: "any"})
	wantVar(t, s, repograph.VariableDecl{Name: "l", Required: tTrue, Type: "list(string)"})
	wantVar(t, s, repograph.VariableDecl{Name: "bad", Required: tTrue, Type: ""})
	wantVar(t, s, repograph.VariableDecl{Name: "d", Required: tFalse, Type: "string"})
	wantVar(t, s, repograph.VariableDecl{Name: "jn", Required: tFalse, Type: "any"})
	wantVar(t, s, repograph.VariableDecl{Name: "jt", Required: tTrue, Type: "map(number)"})
	wantOut(t, s, repograph.OutputDecl{Name: "o", Type: "", Sensitive: tFalse})
	wantOut(t, s, repograph.OutputDecl{Name: "typed", Type: "string", Sensitive: tFalse})
	wantOut(t, s, repograph.OutputDecl{Name: "s", Sensitive: tTrue})
	wantOut(t, s, repograph.OutputDecl{Name: "sv", Sensitive: tUnknown})
	wantOut(t, s, repograph.OutputDecl{Name: "ss", Sensitive: tUnknown})
	wantOut(t, s, repograph.OutputDecl{Name: "sf", Sensitive: tFalse})
	wantOut(t, s, repograph.OutputDecl{Name: "js", Sensitive: tTrue})
	// sec #280d: a JSON sensitive string holding a deep template is a
	// plain string to Value(nil), never parsed: unknown, no crash.
	wantOut(t, s, repograph.OutputDecl{Name: "jdeep", Sensitive: tUnknown})
}

func TestIsOverrideFile(t *testing.T) {
	for name, want := range map[string]bool{
		"override.tf": true, "x_override.tf": true, "override.tf.json": true,
		"x_override.tofu": true, "override.tofu.json": true, "_override.tf": true,
		"overrides.tf": false, "my_override_x.tf": false, "main.tf": false,
		"override.txt": false, "xoverride.tf": false,
	} {
		if got := isOverrideFile(name); got != want {
			t.Errorf("isOverrideFile(%q) = %v, want %v", name, got, want)
		}
	}
}

func TestReadSurfaceOverride(t *testing.T) {
	const main = "variable \"a\" { type = string }\nvariable \"b\" { type = string }\noutput \"o\" {\n  value = 1\n  sensitive = true\n}\n"
	for _, c := range []struct{ name, src string }{
		{"override.tf", "variable \"a\" { default = \"x\" }\noutput \"o\" { value = 2 }\n"},
		{"x_override.tf", "variable \"a\" {}\noutput \"o\" { value = 2 }\n"},
		{"override.tf.json", `{"variable": {"a": {}}, "output": {"o": {"value": 2}}}`},
		{"x_override.tofu", "variable \"a\" {}\noutput \"o\" { value = 2 }\n"},
		{"override.tofu.json", `{"variable": {"a": {"type": "string"}}, "output": {"o": {"value": 2, "sensitive": true}}}`},
	} {
		t.Run(c.name, func(t *testing.T) {
			s := readFacts(t, map[string]string{"main.tf": main, c.name: c.src})
			wantVar(t, s, repograph.VariableDecl{Name: "a", Required: tUnknown, Type: ""})
			wantOut(t, s, repograph.OutputDecl{Name: "o", Type: "", Sensitive: tUnknown})
			wantVar(t, s, repograph.VariableDecl{Name: "b", Required: tTrue, Type: "string"})
		})
	}
	for _, name := range []string{"overrides.tf", "my_override_x.tf"} {
		t.Run(name+" is not an override", func(t *testing.T) {
			s := readFacts(t, map[string]string{"main.tf": main, name: "variable \"c\" { type = number }\n"})
			wantVar(t, s, repograph.VariableDecl{Name: "a", Required: tTrue, Type: "string"})
			wantVar(t, s, repograph.VariableDecl{Name: "c", Required: tTrue, Type: "number"})
		})
	}
}

func TestReadSurfaceDuplicates(t *testing.T) {
	t.Run("identical .tf and .tofu", func(t *testing.T) {
		const v = "variable \"x\" {\n  type = list(string)\n  default = []\n}\noutput \"o\" {\n  value = 1\n  type = string\n}\n"
		s := readFacts(t, map[string]string{"a.tf": v, "a.tofu": v})
		wantVar(t, s, repograph.VariableDecl{Name: "x", Required: tFalse, Type: "list(string)"})
		wantOut(t, s, repograph.OutputDecl{Name: "o", Type: "string", Sensitive: tFalse})
	})
	t.Run("differing type", func(t *testing.T) {
		s := readFacts(t, map[string]string{
			"a.tf":   "variable \"x\" { type = string }\n",
			"a.tofu": "variable \"x\" { type = number }\n",
		})
		wantVar(t, s, repograph.VariableDecl{Name: "x", Required: tTrue, Type: ""})
	})
	t.Run("differing default presence", func(t *testing.T) {
		s := readFacts(t, map[string]string{
			"a.tf":   "variable \"x\" { type = string }\n",
			"a.tofu": "variable \"x\" {\n  type = string\n  default = \"d\"\n}\n",
		})
		wantVar(t, s, repograph.VariableDecl{Name: "x", Required: tUnknown, Type: "string"})
	})
	t.Run("differing sensitive and output type", func(t *testing.T) {
		s := readFacts(t, map[string]string{
			"a.tf":   "output \"o\" {\n  value = 1\n  type = string\n}\n",
			"b.tf":   "output \"o\" {\n  value = 1\n  sensitive = true\n}\n",
			"c.tofu": "variable \"v\" {}\n",
		})
		wantOut(t, s, repograph.OutputDecl{Name: "o", Type: "", Sensitive: tUnknown})
	})
}

// metamorphicBase is a module with nested object/optional types, defaults,
// and outputs with type and sensitive.
var metamorphicBase = map[string]string{"main.tf": `
variable "net" {
  type = object({
    cidr    = string
    subnets = optional(list(object({ az = string, size = optional(number, 24) })), [])
  })
}
variable "name" { type = string }
variable "tags" {
  type    = map(string)
  default = {}
}
variable "any_one" {}
output "id" {
  value = 1
  type  = string
}
output "secret" {
  value     = 2
  sensitive = true
}
output "plain" { value = 3 }
`}

func TestReadSurfaceMetamorphic(t *testing.T) {
	base := readFacts(t, metamorphicBase)
	wantVar(t, base, repograph.VariableDecl{Name: "net", Required: tTrue, Type: `object({"cidr"=string,"subnets"=optional(list(object({"az"=string,"size"=optional(number)})))})`})
	wantVar(t, base, repograph.VariableDecl{Name: "tags", Required: tFalse, Type: "map(string)"})
	wantVar(t, base, repograph.VariableDecl{Name: "any_one", Required: tTrue, Type: "any"})
	wantOut(t, base, repograph.OutputDecl{Name: "id", Type: "string", Sensitive: tFalse})
	wantOut(t, base, repograph.OutputDecl{Name: "secret", Type: "", Sensitive: tTrue})

	transforms := map[string]map[string]string{
		"reflow": {"main.tf": `variable "net" { type = object({ cidr = string, subnets = optional(
  list(
    object({
      az = string
      size = optional(number, 24)
    })
  ), []) }) }
variable "name" {
  type = string
}
variable "tags" {
type = map(string)
default = {}
}
variable "any_one" {
}
output "id" {
  value = 1
  type = string
}
output "secret" {
value = 2
sensitive = true
}
output "plain" {
  value = 3
}
`},
		"comments": {"main.tf": strings.ReplaceAll(strings.ReplaceAll(metamorphicBase["main.tf"], "{\n", "{ # c\n"), "\n}", "\n  // d\n  /* e */\n}")},
		"attribute reorder": {"main.tf": strings.Replace(metamorphicBase["main.tf"],
			"    cidr    = string\n    subnets = optional(list(object({ az = string, size = optional(number, 24) })), [])\n",
			"    subnets = optional(list(object({ size = optional(number, 24), az = string })), [])\n    cidr    = string\n", 1)},
		"optional default change": {"main.tf": strings.Replace(strings.Replace(metamorphicBase["main.tf"], "optional(number, 24)", "optional(number, 25)", 1), "})), [])", "})))", 1)},
		"variable moved to another file": {
			"main.tf": strings.Replace(metamorphicBase["main.tf"], "variable \"tags\" {\n  type    = map(string)\n  default = {}\n}\n", "", 1),
			"vars.tf": "variable \"tags\" {\n  type    = map(string)\n  default = {}\n}\n",
		},
		".tf.json": {"main.tf.json": `{
  "variable": {
    "net": {"type": "object({cidr=string, subnets=optional(list(object({az=string,size=optional(number,24)})),[])})"},
    "name": {"type": "string"},
    "tags": {"type": "map(string)", "default": {}},
    "any_one": {}
  },
  "output": {
    "id": {"value": 1, "type": "string"},
    "secret": {"value": 2, "sensitive": true},
    "plain": {"value": 3}
  }
}`},
		"extra attributes": {"main.tf": strings.NewReplacer(
			"variable \"name\" { type = string }",
			"variable \"name\" {\n  type        = string\n  description = \"n\"\n  nullable    = false\n  ephemeral   = true\n  const       = true\n  deprecated  = \"use x\"\n  validation {\n    condition     = length(var.name) > 0\n    error_message = \"empty\"\n  }\n}",
			"output \"plain\" { value = 3 }",
			"output \"plain\" {\n  value       = 3\n  description = \"p\"\n  ephemeral   = false\n  deprecated  = \"no\"\n}",
		).Replace(metamorphicBase["main.tf"])},
	}
	for name, files := range transforms {
		t.Run(name, func(t *testing.T) {
			got := readFacts(t, files)
			if !equalStrings(got.Variables(), base.Variables()) || !equalStrings(got.Outputs(), base.Outputs()) {
				t.Fatalf("names changed: %v %v, want %v %v", got.Variables(), got.Outputs(), base.Variables(), base.Outputs())
			}
			for _, n := range base.Variables() {
				b, _ := base.Variable(n)
				wantVar(t, got, b)
			}
			for _, n := range base.Outputs() {
				b, _ := base.Output(n)
				wantOut(t, got, b)
			}
		})
	}
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestReadSurfacePanickingParser(t *testing.T) {
	// The panic comes from inside typeexpr's seam, so it travels the
	// production path: reader -> parseTypeFact (safeTypeAttrSig) -> recover.
	orig := typeConstraint
	typeConstraint = func(hcl.Expression) (cty.Type, *typeexpr.Defaults, hcl.Diagnostics) { panic("boom") }
	t.Cleanup(func() { typeConstraint = orig })

	s := readFacts(t, metamorphicBase)
	if !equalStrings(s.Variables(), []string{"any_one", "name", "net", "tags"}) || !equalStrings(s.Outputs(), []string{"id", "plain", "secret"}) {
		t.Fatalf("names: %v %v", s.Variables(), s.Outputs())
	}
	wantVar(t, s, repograph.VariableDecl{Name: "net", Required: tTrue, Type: ""})
	wantVar(t, s, repograph.VariableDecl{Name: "tags", Required: tFalse, Type: ""})
	wantVar(t, s, repograph.VariableDecl{Name: "any_one", Required: tTrue, Type: "any"})
	wantOut(t, s, repograph.OutputDecl{Name: "id", Type: "", Sensitive: tFalse})
	wantOut(t, s, repograph.OutputDecl{Name: "secret", Type: "", Sensitive: tTrue})
}

func TestSafeTypeAttrSigRecovers(t *testing.T) {
	orig := typeConstraint
	typeConstraint = nil // calling it panics with a nil func dereference
	t.Cleanup(func() { typeConstraint = orig })
	expr, src := nativeType(t, "string")
	if got, ok := safeTypeAttrSig(expr, src, false); ok || got != "" {
		t.Fatalf("safeTypeAttrSig = (%q, %v), want (\"\", false)", got, ok)
	}
}

// TestReadSurfaceJSONDepthBomb reads every JSON type-string bomb through
// ReadSurface (sec #245, #260, #272): the name stays declared, the type is
// unknown, typeexpr is never called, no crash.
func TestReadSurfaceJSONDepthBomb(t *testing.T) {
	bombs := map[string]string{
		"list x 400,000":         deepList(400_000),
		"newline ternary x 690k": strings.Repeat("a?b:\n", 690_000) + "c",
	}
	for _, b := range lowBracketBombs {
		bombs[b.name+" big"] = b.big
		bombs[b.name+" under the cap"] = b.small
	}
	for name, typ := range bombs {
		t.Run(name, func(t *testing.T) {
			calls := countTypeexpr(t)
			src := `{"variable":{"v":{"type":` + jsonQuote(typ) + `},"ok":{"type":"string"}}}`
			if len(src) > hclconv.MaxFileBytes {
				t.Fatalf("%d bytes, over MaxFileBytes", len(src))
			}
			start := time.Now()
			s := readFacts(t, map[string]string{"vars.tf.json": src, "main.tf": "output \"o\" { value = 1 }\n"})
			took := time.Since(start)
			t.Logf("%d bytes in %v", len(src), took)
			wantVar(t, s, repograph.VariableDecl{Name: "v", Required: tTrue, Type: ""})
			wantVar(t, s, repograph.VariableDecl{Name: "ok", Required: tTrue, Type: "string"})
			if *calls != 1 { // only "ok"
				t.Fatalf("typeexpr called %d times, want 1 (the benign variable only)", *calls)
			}
			if took > 2*time.Second {
				t.Fatalf("took %v, over 2 s", took)
			}
		})
	}
}
