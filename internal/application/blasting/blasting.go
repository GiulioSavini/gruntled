// Package blasting holds the blast use case: it checks the current tree
// and, when given, a baseline tree, and hands both snapshots to
// impact.Compute. It is pure orchestration; every decision lives in the
// domain.
package blasting

import (
	"context"

	"github.com/GiulioSavini/gruntled/internal/application/checking"
	"github.com/GiulioSavini/gruntled/internal/application/ports"
	"github.com/GiulioSavini/gruntled/internal/domain/impact"
)

// Sources are the ports reading one tree. Each tree needs its own pair:
// a loader must never be shared between the baseline and the current tree.
type Sources struct {
	Units    ports.UnitLoader
	Surfaces ports.SurfaceReader
}

// Error reports which tree failed to check.
type Error struct {
	// Stage is "current" or "baseline".
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
	return impact.Compute(baseRep.Graph, baseRep.Diagnostics, curRep.Graph, curRep.Diagnostics), nil
}
