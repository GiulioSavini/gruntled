package presenter

import (
	"fmt"
	"io"

	"github.com/GiulioSavini/gruntled/internal/domain/diagnostic"
	"github.com/GiulioSavini/gruntled/internal/domain/repograph"
)

// counts is the tally shared by Summary and the JSON summary object.
type counts struct {
	units, resolved, moduleUnknown, configUnknown int
	unknownModules                                int
	errors, warnings                              int
}

func tally(g *repograph.RepositoryGraph, diags diagnostic.Set) counts {
	var c counts
	for _, u := range g.Units() {
		c.units++
		switch u.Status() {
		case repograph.StatusResolved:
			c.resolved++
		case repograph.StatusModuleUnknown:
			c.moduleUnknown++
		case repograph.StatusConfigUnknown:
			c.configUnknown++
		}
	}
	for _, m := range g.Modules() {
		if _, ok := m.Surface(); !ok {
			c.unknownModules++
		}
	}
	for _, d := range diags.All() {
		switch d.Severity() {
		case diagnostic.SeverityError:
			c.errors++
		case diagnostic.SeverityWarning:
			c.warnings++
		}
	}
	return c
}

// Summary writes the one-line run summary (meant for stderr):
//
//	gruntled: checked N units (K unknown): E errors, W warnings
//
// where K counts module-unknown and config-unknown units. When the graph
// has no units and diags is empty it writes
//
//	gruntled: no Terragrunt units found
//
// instead, so "nothing analysed" is never mistaken for "clean". The
// wording is fixed (no pluralisation) to keep the output deterministic.
func Summary(w io.Writer, g *repograph.RepositoryGraph, diags diagnostic.Set) error {
	c := tally(g, diags)
	if c.units == 0 && diags.Len() == 0 {
		_, err := io.WriteString(w, "gruntled: no Terragrunt units found\n")
		return err
	}
	_, err := fmt.Fprintf(w, "gruntled: checked %d units (%d unknown): %d errors, %d warnings\n",
		c.units, c.moduleUnknown+c.configUnknown, c.errors, c.warnings)
	return err
}
