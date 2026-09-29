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
		unit, ok := g.Unit(ur.Unit)
		if !ok {
			continue
		}
		dep, ok := unit.Dependency(ur.Reference.Dependency())
		if !ok { // row 1
			continue
		}
		opts := dep.Options()
		if opts.Enabled != repograph.TristateTrue { // row 2
			continue
		}
		if opts.SkipOutputs != repograph.TristateFalse { // row 3
			continue
		}
		target, ok := g.DependencyTarget(ur.Unit, dep.Name()) // row 4
		if !ok {
			continue
		}
		mod, ok := g.ModuleOf(target.Path()) // row 5
		if !ok {
			continue
		}
		surf, ok := mod.Surface()
		if !ok {
			continue
		}
		output := ur.Reference.Output()
		if surf.HasOutput(output) { // row 6
			continue
		}
		msg := "dependency " + strconv.Quote(dep.Name()) +
			" output " + strconv.Quote(output) +
			" is not declared by module " + strconv.Quote(mod.Path().String()) +
			" (target unit " + strconv.Quote(target.Path().String()) + ")"
		if mockMasksAtApply(opts, output, surf) {
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
