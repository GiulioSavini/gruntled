package presenter

import (
	"bytes"
	"encoding/json"
	"io"

	"github.com/GiulioSavini/gruntled/internal/domain/diagnostic"
	"github.com/GiulioSavini/gruntled/internal/domain/repograph"
)

// jsonSchemaVersion is the version of the JSON document JSON writes. Bump
// it on any incompatible change to the shapes below.
const jsonSchemaVersion = 1

// The JSON document is built from structs only, never maps, so its key
// order is fixed by the field order here and the output is deterministic.
type jsonReport struct {
	Version        int                 `json:"version"`
	Diagnostics    []jsonDiag          `json:"diagnostics"`
	UnknownUnits   []jsonUnknownUnit   `json:"unknown_units"`
	UnknownModules []jsonUnknownModule `json:"unknown_modules"`
	Summary        jsonSummary         `json:"summary"`
}

type jsonDiag struct {
	Code     string `json:"code"`
	Severity string `json:"severity"`
	File     string `json:"file"`
	Line     int    `json:"line"`
	Column   int    `json:"column"`
	Unit     string `json:"unit,omitempty"`
	Message  string `json:"message"`
}

type jsonUnknownUnit struct {
	Path   string `json:"path"`
	Status string `json:"status"`
	Reason string `json:"reason"`
}

type jsonUnknownModule struct {
	Path   string `json:"path"`
	Reason string `json:"reason"`
}

type jsonSummary struct {
	Units          int `json:"units"`
	Resolved       int `json:"resolved"`
	ModuleUnknown  int `json:"module_unknown"`
	ConfigUnknown  int `json:"config_unknown"`
	UnknownModules int `json:"unknown_modules"`
	Errors         int `json:"errors"`
	Warnings       int `json:"warnings"`
}

// JSON writes one indented JSON document (schema version 1) holding the
// diagnostics in diags' canonical order, the units that are not resolved
// and the modules whose surface is unknown (both in the graph's path
// order), and the run summary. Empty lists are written as [], never null,
// and HTML characters are not escaped. The encoded bytes go through
// escapeJSON, so DEL, C1, format and line/paragraph separator runes are
// written as \uXXXX escapes (same decoded value).
func JSON(w io.Writer, g *repograph.RepositoryGraph, diags diagnostic.Set) error {
	all := diags.All()
	report := jsonReport{
		Version:        jsonSchemaVersion,
		Diagnostics:    make([]jsonDiag, 0, len(all)),
		UnknownUnits:   make([]jsonUnknownUnit, 0),
		UnknownModules: make([]jsonUnknownModule, 0),
	}
	for _, d := range all {
		p := d.Pos()
		jd := jsonDiag{
			Code:     string(d.Code()),
			Severity: d.Severity().String(),
			File:     p.File().String(),
			Line:     p.Line(),
			Column:   p.Column(),
			Message:  d.Message(),
		}
		if u, ok := d.Unit(); ok {
			jd.Unit = u.String()
		}
		report.Diagnostics = append(report.Diagnostics, jd)
	}
	for _, u := range g.Units() {
		if u.Status() == repograph.StatusResolved {
			continue
		}
		report.UnknownUnits = append(report.UnknownUnits, jsonUnknownUnit{
			Path:   u.Path().String(),
			Status: u.Status().String(),
			Reason: u.UnknownReason(),
		})
	}
	for _, m := range g.Modules() {
		if _, ok := m.Surface(); ok {
			continue
		}
		report.UnknownModules = append(report.UnknownModules, jsonUnknownModule{
			Path:   m.Path().String(),
			Reason: m.UnknownReason(),
		})
	}
	c := tally(g, diags)
	report.Summary = jsonSummary{
		Units:          c.units,
		Resolved:       c.resolved,
		ModuleUnknown:  c.moduleUnknown,
		ConfigUnknown:  c.configUnknown,
		UnknownModules: c.unknownModules,
		Errors:         c.errors,
		Warnings:       c.warnings,
	}

	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(report); err != nil {
		return err
	}
	_, err := w.Write(escapeJSON(b.Bytes()))
	return err
}
