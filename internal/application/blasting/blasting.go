// Package blasting holds the blast use case: it checks the current tree
// and, when given, a baseline tree, and hands both Reports to Between.
// Between is the shared pure core that turns two Reports into an
// impact.Result; it is the only place GRT004 (a removed output that is
// still referenced) is produced, so check never emits it. The package is
// pure orchestration; every decision lives in the domain.
package blasting

import (
	"context"

	"github.com/GiulioSavini/gruntled/internal/application/checking"
	"github.com/GiulioSavini/gruntled/internal/application/ports"
	"github.com/GiulioSavini/gruntled/internal/domain/analysis"
	"github.com/GiulioSavini/gruntled/internal/domain/impact"
)

// Sources are the ports reading one tree. Each tree needs its own pair:
// a loader must never be shared between the baseline and the current tree.
type Sources struct {
	Units    ports.UnitLoader
	Surfaces ports.SurfaceReader
}

// Error reports which stage of a blast failed.
type Error struct {
	// Stage is "current" or "baseline" (that tree failed to check), or
	// "compare" (Between failed to build a diagnostic).
	Stage string
	Err   error
}

// Error implements error.
func (e *Error) Error() string {
	return "blasting: " + e.Stage + ": " + e.Err.Error()
}

// Unwrap returns the underlying error.
func (e *Error) Unwrap() error {
	return e.Err
}

// Blast checks cur, then base, and returns the blast radius from base to
// cur. A nil base yields impact.NoBaseline: every current finding is
// Broken and nothing is Impacted. The current tree is checked first, so a
// failure there is reported even when the baseline is also broken. On
// error the Result is zero.
func Blast(ctx context.Context, cur Sources, base *Sources) (impact.Result, error) {
	curRep, err := checking.Check(ctx, cur.Units, cur.Surfaces)
	if err != nil {
		return impact.Result{}, &Error{Stage: "current", Err: err}
	}
	if base == nil {
		return impact.NoBaseline(curRep.Diagnostics), nil
	}
	baseRep, err := checking.Check(ctx, base.Units, base.Surfaces)
	if err != nil {
		return impact.Result{}, &Error{Stage: "baseline", Err: err}
	}
	res, err := Between(baseRep, curRep)
	if err != nil {
		return impact.Result{}, &Error{Stage: "compare", Err: err}
	}
	return res, nil
}

// Between returns the blast radius from base to cur. It reclassifies, in
// cur's diagnostics only, every GRT001 that analysis.RemovedOutputs proves is
// a removed output as GRT004 (analysis.SupersedeUnknownOutputs), then calls
// impact.Compute. It is pure: it reads only the two Reports, so blast --base
// and the watch daemon agree by construction. The error comes only from
// building a diagnostic; on error the Result is zero.
//
// base must come from checking.Check: baseline diagnostics never contain
// GRT004, which is what makes every GRT004 new and keeps Broken equal to
// v0.3. A base holding GRT004 is outside the contract.
func Between(base, cur checking.Report) (impact.Result, error) {
	curD := cur.Diagnostics
	if base.Graph != nil && cur.Graph != nil {
		removed, err := analysis.RemovedOutputs(base.Graph, cur.Graph)
		if err != nil {
			return impact.Result{}, err
		}
		curD = analysis.SupersedeUnknownOutputs(cur.Diagnostics, removed)
	}
	return impact.Compute(base.Graph, base.Diagnostics, cur.Graph, curD), nil
}
