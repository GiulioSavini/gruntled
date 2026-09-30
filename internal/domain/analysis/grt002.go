package analysis

import (
	"strconv"

	"github.com/GiulioSavini/gruntled/internal/domain/diagnostic"
	"github.com/GiulioSavini/gruntled/internal/domain/repograph"
)

// MissingTargets implements GRT002. For every unit in g.Units() order, its
// `dependency` blocks in Dependencies() order, then its `dependencies`
// paths entries in PathDependencies() order, the first matching row wins:
//
//  1. the dependency is unresolved: silent.
//  2. the target state is TargetUnknown or TargetHasConfig: silent.
//  3. a block whose enabled is not literally true (absent counts as true;
//     false, non-literal and deep-merged labels are not): silent. A paths
//     entry has no enabled attribute and skips this row.
//  4. TargetDirMissing: GRT002 "directory does not exist".
//  5. TargetNoConfig: GRT002 "directory has no terragrunt.hcl".
//
// skip_outputs and mock_outputs never gate: Terragrunt refuses to run a
// dependency on a directory holding no unit regardless of either. A block
// diagnostic sits at its config_path value position, a paths diagnostic at
// its entry position; both are attributed to the declaring unit, so a
// shared include yields one diagnostic per including unit. A block and a
// paths entry naming the same missing target both fire.
func MissingTargets(g *repograph.RepositoryGraph) ([]diagnostic.Diagnostic, error) {
	var out []diagnostic.Diagnostic
	emit := func(unit repograph.RepoPath, pos repograph.Position, msg string) error {
		d, err := diagnostic.NewForUnit(diagnostic.CodeMissingDependencyTarget, diagnostic.SeverityError, unit, pos, msg)
		if err != nil {
			return err
		}
		out = append(out, d)
		return nil
	}
	for _, unit := range g.Units() {
		for _, dep := range unit.Dependencies() {
			target, ok := dep.Target()
			if !ok { // row 1
				continue
			}
			problem, ok := missingProblem(dep.TargetState()) // row 2
			if !ok {
				continue
			}
			if dep.Options().Enabled != repograph.TristateTrue { // row 3
				continue
			}
			msg := "dependency " + strconv.Quote(dep.Name()) +
				" config_path resolves to " + strconv.Quote(target.String()) + ": " + problem
			if err := emit(unit.Path(), dep.PathPos(), msg); err != nil {
				return nil, err
			}
		}
		for _, pd := range unit.PathDependencies() {
			target, ok := pd.Target()
			if !ok { // row 1
				continue
			}
			problem, ok := missingProblem(pd.TargetState()) // row 2
			if !ok {
				continue
			}
			msg := "dependencies path " + strconv.Quote(pd.Literal()) +
				" resolves to " + strconv.Quote(target.String()) + ": " + problem
			if err := emit(unit.Path(), pd.Pos(), msg); err != nil {
				return nil, err
			}
		}
	}
	return out, nil
}

// missingProblem returns the message tail for a reportable target state
// (rows 4 and 5), and false for every other state.
func missingProblem(s repograph.TargetState) (string, bool) {
	switch s {
	case repograph.TargetDirMissing:
		return "directory does not exist", true
	case repograph.TargetNoConfig:
		return "directory has no terragrunt.hcl", true
	default:
		return "", false
	}
}
