// Package indexing implements the indexing use case: turning a
// repository's units and module surfaces into a repograph.RepositoryGraph.
package indexing

import (
	"context"
	"slices"
	"strconv"

	"github.com/GiulioSavini/gruntled/internal/application/ports"
	"github.com/GiulioSavini/gruntled/internal/domain/diagnostic"
	"github.com/GiulioSavini/gruntled/internal/domain/repograph"
)

// Result is the outcome of Build: the assembled graph and every
// diagnostic collected while loading units and reading module surfaces.
type Result struct {
	Graph       *repograph.RepositoryGraph
	Diagnostics diagnostic.Set
}

// Error wraps a failure with the Build stage it happened in: "load-units",
// "read-surface", or "assemble". Path is the unit or module path involved,
// when there is one. Unwrap returns the cause so errors.Is/As work.
type Error struct {
	Stage string
	Path  string
	Err   error
}

func (e *Error) Error() string {
	s := "indexing: " + e.Stage
	if e.Path != "" {
		s += " " + strconv.Quote(e.Path)
	}
	s += ": " + e.Err.Error()
	return s
}

// Unwrap returns the cause, so errors.Is/As see through the stage
// wrapper.
func (e *Error) Unwrap() error {
	return e.Err
}

// Build loads every unit through units, reads each distinct resolved
// module's surface exactly once through surfaces, and assembles the
// result into a RepositoryGraph.
//
// Units are assembled before any module surface is read: a UnitConfig
// whose fields the domain constructors reject (for example a "resolved"
// unit with a zero Module) fails at the "assemble" stage without ever
// calling ReadSurface, since the module such a unit "resolves" to is not
// well-formed to begin with.
func Build(ctx context.Context, units ports.UnitLoader, surfaces ports.SurfaceReader) (Result, error) {
	if err := ctx.Err(); err != nil {
		return Result{}, &Error{Stage: "load-units", Err: err}
	}

	loadResult, err := units.LoadUnits(ctx)
	if err != nil {
		return Result{}, &Error{Stage: "load-units", Err: err}
	}

	graphUnits := make([]repograph.Unit, 0, len(loadResult.Units))
	for _, uc := range loadResult.Units {
		u, err := assembleUnit(uc)
		if err != nil {
			return Result{}, &Error{Stage: "assemble", Path: uc.Path.String(), Err: err}
		}
		graphUnits = append(graphUnits, u)
	}

	modulePaths := distinctResolvedModules(loadResult.Units)

	graphModules := make([]repograph.Module, 0, len(modulePaths))
	var surfaceDiags []diagnostic.Diagnostic
	for _, modPath := range modulePaths {
		if err := ctx.Err(); err != nil {
			return Result{}, &Error{Stage: "read-surface", Path: modPath.String(), Err: err}
		}

		res, err := surfaces.ReadSurface(ctx, modPath)
		if err != nil {
			return Result{}, &Error{Stage: "read-surface", Path: modPath.String(), Err: err}
		}
		surfaceDiags = append(surfaceDiags, res.Diagnostics...)

		var mod repograph.Module
		if res.UnknownReason == "" {
			mod, err = repograph.NewModule(modPath, res.Surface)
		} else {
			mod, err = repograph.NewUnknownModule(modPath, res.UnknownReason)
		}
		if err != nil {
			return Result{}, &Error{Stage: "assemble", Path: modPath.String(), Err: err}
		}
		graphModules = append(graphModules, mod)
	}

	graph, err := repograph.NewRepositoryGraph(graphUnits, graphModules)
	if err != nil {
		return Result{}, &Error{Stage: "assemble", Err: err}
	}

	allDiags := make([]diagnostic.Diagnostic, 0, len(loadResult.Diagnostics)+len(surfaceDiags))
	allDiags = append(allDiags, loadResult.Diagnostics...)
	allDiags = append(allDiags, surfaceDiags...)

	return Result{
		Graph:       graph,
		Diagnostics: diagnostic.NewSet(allDiags...),
	}, nil
}

// assembleUnit maps one UnitConfig to a domain Unit, applying the
// precedence documented on ports.UnitConfig: config-unknown, then
// module-unknown, then resolved. Path dependencies are attached to
// module-unknown and resolved units only; a config-unknown unit stays
// dep-free whatever the UnitConfig carries.
func assembleUnit(uc ports.UnitConfig) (repograph.Unit, error) {
	var u repograph.Unit
	var err error
	switch {
	case uc.ConfigUnknownReason != "":
		return repograph.NewConfigUnknownUnit(uc.Path, uc.ConfigUnknownReason)
	case uc.ModuleUnknownReason != "":
		u, err = repograph.NewModuleUnknownUnit(uc.Path, uc.ModuleUnknownReason, uc.Dependencies, uc.References)
	default:
		u, err = repograph.NewResolvedUnit(uc.Path, uc.Module, uc.Dependencies, uc.References)
	}
	if err != nil || len(uc.PathDependencies) == 0 {
		return u, err
	}
	return u.WithPathDependencies(uc.PathDependencies)
}

// distinctResolvedModules returns the distinct Module paths of every
// resolved UnitConfig (both reasons empty), sorted by RepoPath.Compare so
// ReadSurface is called in a deterministic order regardless of the
// loader's own unit order.
func distinctResolvedModules(units []ports.UnitConfig) []repograph.RepoPath {
	seen := make(map[repograph.RepoPath]struct{})
	var paths []repograph.RepoPath
	for _, uc := range units {
		if uc.ConfigUnknownReason != "" || uc.ModuleUnknownReason != "" {
			continue
		}
		if _, ok := seen[uc.Module]; ok {
			continue
		}
		seen[uc.Module] = struct{}{}
		paths = append(paths, uc.Module)
	}
	slices.SortFunc(paths, func(a, b repograph.RepoPath) int {
		return a.Compare(b)
	})
	return paths
}
