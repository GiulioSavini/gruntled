package tfsurface

import (
	"strings"

	"github.com/hashicorp/hcl/v2"

	"github.com/GiulioSavini/gruntled/internal/domain/repograph"
	"github.com/GiulioSavini/gruntled/internal/infrastructure/hclconv"
)

// factSchema selects the attributes the type-level facts come from; every
// other attribute or nested block (description, nullable, validation,
// ephemeral, const, deprecated, value, precondition) is ignored.
var factSchema = &hcl.BodySchema{
	Attributes: []hcl.AttributeSchema{
		{Name: "type"},
		{Name: "default"},
		{Name: "sensitive"},
	},
}

// declFacts is one declaration's facts as read from one block. For a
// variable, sensitive is unused; for an output, required is unused.
type declFacts struct {
	override  bool
	required  repograph.Tristate
	typ       string
	sensitive repograph.Tristate
}

// unknownFacts is a declaration whose facts are all unknown.
var unknownFacts = declFacts{required: repograph.TristateUnknown, sensitive: repograph.TristateUnknown}

// factSet collects every declaration of every name across the kept files
// of one module. Names are the union of the declared labels, exactly the
// names the reader reported before facts existed.
type factSet struct {
	vars, outs map[string][]declFacts
}

func newFactSet() *factSet {
	return &factSet{vars: map[string][]declFacts{}, outs: map[string][]declFacts{}}
}

// add records block b (a labelled variable or output block) declared in
// the kept file named file with source src. src already passed the size
// cap and depth pre-scan, and parsed without errors.
func (f *factSet) add(b *hcl.Block, file string, src []byte) {
	name := b.Labels[0]
	fromJSON := strings.HasSuffix(file, ".json")
	attrs, _, diags := b.Body.PartialContent(factSchema)
	d := unknownFacts
	if !diags.HasErrors() {
		d = blockFacts(b.Type, attrs.Attributes, src, fromJSON)
	}
	d.override = isOverrideFile(file)
	switch b.Type {
	case "variable":
		f.vars[name] = append(f.vars[name], d)
	case "output":
		f.outs[name] = append(f.outs[name], d)
	}
}

// blockFacts reads one block's facts:
//   - variable: required is the absence of a default attribute (presence
//     only, so default = null is not required); type is the canonical
//     type, "any" when absent, "" when it does not parse;
//   - output: type is the canonical declared type, "" when absent or
//     unparseable (absent is never "any"); sensitive is the literal bool,
//     false when absent, unknown when not a literal bool.
func blockFacts(kind string, attrs hcl.Attributes, src []byte, fromJSON bool) declFacts {
	d := unknownFacts
	typeAttr, hasType := attrs["type"]
	if hasType {
		if sig, ok := parseTypeFact(typeAttr.Expr, src, fromJSON); ok {
			d.typ = sig
		}
	}
	switch kind {
	case "variable":
		if !hasType {
			d.typ = "any"
		}
		if _, ok := attrs["default"]; ok {
			d.required = repograph.TristateFalse
		} else {
			d.required = repograph.TristateTrue
		}
	case "output":
		d.sensitive = repograph.TristateFalse
		if a, ok := attrs["sensitive"]; ok {
			d.sensitive = hclconv.LiteralBool(a.Expr)
		}
	}
	return d
}

// merge folds every declaration of one name: any declaration in an
// override file makes every fact unknown (Terraform merges overrides into
// the primary block, which gruntled does not model; D-15-03); otherwise a
// fact is kept only when every declaration agrees on it.
func merge(ds []declFacts) declFacts {
	m := ds[0]
	for _, d := range ds {
		if d.override {
			return unknownFacts
		}
		if d.required != m.required {
			m.required = repograph.TristateUnknown
		}
		if d.typ != m.typ {
			m.typ = ""
		}
		if d.sensitive != m.sensitive {
			m.sensitive = repograph.TristateUnknown
		}
	}
	return m
}

// decls returns the merged declarations, in map order;
// repograph.NewSurfaceFacts sorts and validates them.
func (f *factSet) decls() ([]repograph.VariableDecl, []repograph.OutputDecl) {
	vars := make([]repograph.VariableDecl, 0, len(f.vars))
	for name, ds := range f.vars {
		m := merge(ds)
		vars = append(vars, repograph.VariableDecl{Name: name, Required: m.required, Type: m.typ})
	}
	outs := make([]repograph.OutputDecl, 0, len(f.outs))
	for name, ds := range f.outs {
		m := merge(ds)
		outs = append(outs, repograph.OutputDecl{Name: name, Type: m.typ, Sensitive: m.sensitive})
	}
	return vars, outs
}

// isOverrideFile reports whether a kept module file name is a Terraform
// override file: base name "override" or ending in "_override" after
// stripping .tf, .tf.json, .tofu or .tofu.json.
func isOverrideFile(name string) bool {
	for _, ext := range []string{".tf.json", ".tofu.json", ".tf", ".tofu"} {
		if base, ok := strings.CutSuffix(name, ext); ok {
			return base == "override" || strings.HasSuffix(base, "_override")
		}
	}
	return false
}
