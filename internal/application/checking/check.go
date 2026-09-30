// Package checking holds the check use case: it builds the repository graph
// through the indexing use case and runs the GRT001, GRT002 and GRT003
// analyzers over it. It is
// pure orchestration; every decision lives in the domain.
package checking

import (
	"context"

	"github.com/GiulioSavini/gruntled/internal/application/indexing"
	"github.com/GiulioSavini/gruntled/internal/application/ports"
	"github.com/GiulioSavini/gruntled/internal/domain/analysis"
	"github.com/GiulioSavini/gruntled/internal/domain/diagnostic"
	"github.com/GiulioSavini/gruntled/internal/domain/repograph"
)

// Report is the result of one check run.
type Report struct {
	// Graph is the repository graph the diagnostics were computed over.
	Graph *repograph.RepositoryGraph
	// Diagnostics is the loader and surface diagnostics (GRT100) together
	// with GRT001 (unknown output), GRT002 (missing dependency target) and
	// GRT003 (dependency cycle), in canonical order and deduplicated by Key.
	Diagnostics diagnostic.Set
}

// Error reports that analysis failed after the graph was built.
type Error struct {
	// Stage is the step that failed; currently always "analyze" (every
	// analyzer shares it; the wrapped error names the analyzer).
	Stage string
	Err   error
}

// Error implements error.
func (e *Error) Error() string {
	return "checking: " + e.Stage + ": " + e.Err.Error()
}

// Unwrap returns the underlying error.
func (e *Error) Unwrap() error {
	return e.Err
}

// analyzers is every graph analyzer Check runs, in a fixed order. The order
// does not affect the Report: the Set is canonical.
var analyzers = []func(*repograph.RepositoryGraph) ([]diagnostic.Diagnostic, error){
	analysis.UnknownOutputs,
	analysis.MissingTargets,
	analysis.DependencyCycles,
}

// Check loads every unit, reads every module surface and reports GRT001,
// GRT002 and GRT003 alongside the diagnostics gathered while loading. An error from graph
// construction is returned unchanged (an *indexing.Error); an analyzer
// failure is returned as an *Error. On error the Report is zero.
func Check(ctx context.Context, units ports.UnitLoader, surfaces ports.SurfaceReader) (Report, error) {
	res, err := indexing.Build(ctx, units, surfaces)
	if err != nil {
		return Report{}, err
	}
	all := res.Diagnostics.All()
	for _, analyze := range analyzers {
		ds, err := analyze(res.Graph)
		if err != nil {
			return Report{}, &Error{Stage: "analyze", Err: err}
		}
		all = append(all, ds...)
	}
	return Report{Graph: res.Graph, Diagnostics: diagnostic.NewSet(all...)}, nil
}
