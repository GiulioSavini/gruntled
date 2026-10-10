// Package analysis holds gruntled's pure analyzers over a RepositoryGraph.
//
// UnknownOutputs implements GRT001 with the DIAG-03 decision table. For
// every reference dependency.X.outputs.Y, in RepositoryGraph.References
// order, the first matching row wins:
//
//  1. X names no dependency block of the referencing unit: silent.
//  2. enabled is not literally true: silent.
//  3. skip_outputs is not literally false: silent.
//  4. the dependency is unresolved, or its target is not a unit: silent.
//  5. the target unit's module, or that module's surface, is unknown: silent.
//  6. the target module declares Y: silent.
//  7. otherwise: GRT001 at SeverityError, attributed to the referencing unit.
//
// Rows 1-5 are resolveReference, shared with GRT004 (RemovedOutputs).
//
// mock_outputs never suppresses and never downgrades GRT001. When a mock
// covers Y, apply silently uses the mock value instead of failing, which is
// the worse variant of the same bug. Mock facts only select a message
// suffix, and only when every fact involved is a certain literal; an
// unknown mock fact drops the suffix and never adds or removes a
// diagnostic.
package analysis

import (
	"slices"
	"strconv"

	"github.com/GiulioSavini/gruntled/internal/domain/diagnostic"
	"github.com/GiulioSavini/gruntled/internal/domain/repograph"
)

// mockMaskSuffix is appended to a GRT001 message when mock_outputs would
// certainly supply the missing output at apply.
const mockMaskSuffix = "; mock_outputs supplies it, so apply would silently use the mock value"

// UnknownOutputs returns one GRT001 per reference whose resolved target
// module has a known surface that does not declare the referenced output,
// in g.References() order. It returns nil when there is nothing to report.
// The message carries no volatile content (no suggestions, no list of
// available outputs), since it is part of diagnostic.Key.
func UnknownOutputs(g *repograph.RepositoryGraph) ([]diagnostic.Diagnostic, error) {
	var out []diagnostic.Diagnostic
	for _, ur := range g.References() {
		r, ok := resolveReference(g, ur) // rows 1-5
		if !ok {
			continue
		}
		output := ur.Reference.Output()
		if r.surface.HasOutput(output) { // row 6
			continue
		}
		msg := "dependency " + strconv.Quote(r.dep.Name()) +
			" output " + strconv.Quote(output) +
			" is not declared by module " + strconv.Quote(r.module.Path().String()) +
			" (target unit " + strconv.Quote(r.target.Path().String()) + ")"
		if mockMasksAtApply(r.dep.Options(), output, r.surface) {
			msg += mockMaskSuffix
		}
		d, err := diagnostic.NewForUnit(diagnostic.CodeUnknownOutput, diagnostic.SeverityError, ur.Unit, ur.Reference.Pos(), msg)
		if err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, nil
}

// resolution is a reference's dependency resolved through rows 1, 4 and 5.
type resolution struct {
	dep     repograph.Dependency
	target  repograph.Unit
	module  repograph.Module
	surface repograph.Surface
}

// resolveDependency applies rows 1, 4 and 5 of the DIAG-03 table to unit's
// dependency named dep: the block exists, resolves to a unit of g, whose
// module and that module's surface are known. ok is false otherwise.
func resolveDependency(g *repograph.RepositoryGraph, unit repograph.RepoPath, dep string) (resolution, bool) {
	u, ok := g.Unit(unit)
	if !ok {
		return resolution{}, false
	}
	d, ok := u.Dependency(dep) // row 1
	if !ok {
		return resolution{}, false
	}
	target, ok := g.DependencyTarget(unit, d.Name()) // row 4
	if !ok {
		return resolution{}, false
	}
	mod, ok := g.ModuleOf(target.Path()) // row 5
	if !ok {
		return resolution{}, false
	}
	surf, ok := mod.Surface()
	if !ok {
		return resolution{}, false
	}
	return resolution{dep: d, target: target, module: mod, surface: surf}, true
}

// resolveReference applies rows 1-5: resolveDependency plus row 2 (enabled
// literally true) and row 3 (skip_outputs literally false). Rows 1-5 are all
// silent outcomes, so evaluating them in this order changes nothing.
func resolveReference(g *repograph.RepositoryGraph, ur repograph.UnitReference) (resolution, bool) {
	r, ok := resolveDependency(g, ur.Unit, ur.Reference.Dependency())
	if !ok {
		return resolution{}, false
	}
	opts := r.dep.Options()
	if opts.Enabled != repograph.TristateTrue { // row 2
		return resolution{}, false
	}
	if opts.SkipOutputs != repograph.TristateFalse { // row 3
		return resolution{}, false
	}
	return r, true
}

// mockMasksAtApply reports whether apply would certainly return a mock value
// for output instead of failing. Terragrunt returns mocks at apply when the
// mock covers the output, apply is an allowed command (an absent or
// literally empty allowed-commands list allows every command), and either
// state is merged with mocks or the target's state outputs are empty, which
// is always the case for a module declaring zero outputs. Any unknown fact
// makes the answer false.
func mockMasksAtApply(o repograph.DependencyOptions, output string, s repograph.Surface) bool {
	if o.MockOutputs.Contains(output) != repograph.TristateTrue {
		return false
	}
	applyAllowed := o.MockAllowedCommands.IsAbsent()
	if names, ok := o.MockAllowedCommands.Names(); ok {
		applyAllowed = len(names) == 0 || slices.Contains(names, "apply")
	}
	if !applyAllowed {
		return false
	}
	return o.MockMergeWithState == repograph.TristateTrue || len(s.Outputs()) == 0
}
