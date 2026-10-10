package repograph

import (
	"errors"
	"slices"
	"strconv"
)

// Surface is the public variable and output names of a Module (design doc
// §5.3): the contract a Unit binds against when it declares dependency
// blocks and inputs. Variables and Outputs are each stored sorted and
// duplicate-free.
//
// A Surface may also carry per-name declaration facts (VariableDecl,
// OutputDecl). They are optional extras: the names are the contract GRT001
// and name-level Impacted use, and the name views (Variables, Outputs,
// HasVariable, HasOutput) never depend on the facts.
type Surface struct {
	variables []string
	outputs   []string
	vars      []VariableDecl // sorted by Name, parallel to variables
	outs      []OutputDecl   // sorted by Name, parallel to outputs
}

// VariableDecl is a declared variable's type-level facts. Required is
// TristateTrue when the merged declaration has no default attribute,
// TristateFalse when it has one (default = null included), and
// TristateUnknown when the reader could not decide. Type is the canonical
// type string ("any" when no type attribute), "" when unknown.
type VariableDecl struct {
	Name     string
	Required Tristate
	Type     string
}

// OutputDecl is a declared output's type-level facts. Type is the canonical
// declared type, "" when not declared or unknown. Sensitive is the literal
// sensitive fact (absent = TristateFalse), TristateUnknown when non-literal.
type OutputDecl struct {
	Name      string
	Type      string
	Sensitive Tristate
}

// NewSurface validates variables and outputs and returns a Surface. Neither
// list may contain an empty name or a duplicate within itself; the stored
// copies are sorted. Every declaration fact is unknown (TristateUnknown and
// ""), so a Surface built from names alone never claims a type-level fact.
func NewSurface(variables, outputs []string) (Surface, error) {
	sortedVars, err := sortedUniqueNames("variable", variables)
	if err != nil {
		return Surface{}, err
	}
	sortedOuts, err := sortedUniqueNames("output", outputs)
	if err != nil {
		return Surface{}, err
	}
	vars := make([]VariableDecl, len(sortedVars))
	for i, n := range sortedVars {
		vars[i] = VariableDecl{Name: n}
	}
	outs := make([]OutputDecl, len(sortedOuts))
	for i, n := range sortedOuts {
		outs[i] = OutputDecl{Name: n}
	}
	return Surface{variables: sortedVars, outputs: sortedOuts, vars: vars, outs: outs}, nil
}

// NewSurfaceFacts is NewSurface with per-name facts. The names are taken
// from the decls and follow the same rules (non-empty, unique per kind, the
// same error messages); the decls are stored sorted by name.
func NewSurfaceFacts(vars []VariableDecl, outs []OutputDecl) (Surface, error) {
	var varNames, outNames []string // nil when empty, as NewSurface(nil, nil)
	for _, d := range vars {
		varNames = append(varNames, d.Name)
	}
	for _, d := range outs {
		outNames = append(outNames, d.Name)
	}
	sortedVars, err := sortedUniqueNames("variable", varNames)
	if err != nil {
		return Surface{}, err
	}
	sortedOuts, err := sortedUniqueNames("output", outNames)
	if err != nil {
		return Surface{}, err
	}
	sv := slices.Clone(vars)
	slices.SortFunc(sv, func(a, b VariableDecl) int { return compareStrings(a.Name, b.Name) })
	so := slices.Clone(outs)
	slices.SortFunc(so, func(a, b OutputDecl) int { return compareStrings(a.Name, b.Name) })
	return Surface{variables: sortedVars, outputs: sortedOuts, vars: sv, outs: so}, nil
}

func sortedUniqueNames(kind string, names []string) ([]string, error) {
	sorted := slices.Clone(names)
	slices.Sort(sorted)
	for i, name := range sorted {
		if name == "" {
			return nil, errors.New("repograph: invalid surface " + kind + " name: must not be empty")
		}
		if i > 0 && sorted[i-1] == name {
			return nil, errors.New("repograph: invalid surface " + kind + " name " + strconv.Quote(name) + ": duplicate")
		}
	}
	return sorted, nil
}

// Variables returns a sorted copy of the surface's variable names.
func (s Surface) Variables() []string {
	return slices.Clone(s.variables)
}

// Outputs returns a sorted copy of the surface's output names.
func (s Surface) Outputs() []string {
	return slices.Clone(s.outputs)
}

// HasVariable reports whether name is declared in the surface's variables.
func (s Surface) HasVariable(name string) bool {
	_, ok := slices.BinarySearch(s.variables, name)
	return ok
}

// HasOutput reports whether name is declared in the surface's outputs.
func (s Surface) HasOutput(name string) bool {
	_, ok := slices.BinarySearch(s.outputs, name)
	return ok
}

// Variable returns the facts of the named variable (all unknown for a
// Surface built with NewSurface); ok is false when it is not declared.
func (s Surface) Variable(name string) (VariableDecl, bool) {
	i, ok := slices.BinarySearchFunc(s.vars, name, func(d VariableDecl, n string) int { return compareStrings(d.Name, n) })
	if !ok {
		return VariableDecl{}, false
	}
	return s.vars[i], true
}

// Output returns the facts of the named output (all unknown for a Surface
// built with NewSurface); ok is false when it is not declared.
func (s Surface) Output(name string) (OutputDecl, bool) {
	i, ok := slices.BinarySearchFunc(s.outs, name, func(d OutputDecl, n string) int { return compareStrings(d.Name, n) })
	if !ok {
		return OutputDecl{}, false
	}
	return s.outs[i], true
}
