package presenter

import (
	"bytes"
	"encoding/json"
	"io"
	"strconv"

	"github.com/GiulioSavini/gruntled/internal/domain/diagnostic"
	"github.com/GiulioSavini/gruntled/internal/domain/impact"
)

// blastSchemaVersion is the version of the blast document BlastJSON
// writes. Bump it on any incompatible change to the shapes below.
const blastSchemaVersion = 1

// blastKind tells a blast document apart from a check report and a graph.
const blastKind = "blast"

// blastNoBaselineNote is the label of a run without a baseline, shared by
// the text and JSON formats.
const blastNoBaselineNote = "no baseline"

// The blast document is built from structs only, never maps, so its key
// order is fixed by the field order here and the output is deterministic.
type blastDoc struct {
	Version  int             `json:"version"`
	Kind     string          `json:"kind"`
	Baseline bool            `json:"baseline"`
	Note     string          `json:"note,omitempty"`
	Broken   []blastBroken   `json:"broken"`
	Impacted []blastImpacted `json:"impacted"`
	Summary  blastSummary    `json:"summary"`
}

type blastBroken struct {
	Unit     string         `json:"unit"`
	Findings []blastFinding `json:"findings"`
}

type blastFinding struct {
	Code     string `json:"code"`
	Severity string `json:"severity"`
	File     string `json:"file"`
	Line     int    `json:"line"`
	Column   int    `json:"column"`
	Message  string `json:"message"`
}

type blastImpacted struct {
	Unit             string   `json:"unit"`
	Module           string   `json:"module"`
	AddedVariables   []string `json:"added_variables"`
	RemovedVariables []string `json:"removed_variables"`
	AddedOutputs     []string `json:"added_outputs"`
	RemovedOutputs   []string `json:"removed_outputs"`
}

type blastSummary struct {
	Broken   int `json:"broken"`
	Impacted int `json:"impacted"`
}

// BlastText writes a blast radius as plain text:
//
//	baseline: <label>
//	Broken (N):
//	  <subject>
//	    file:line:col: CODE message
//	Impacted (M):
//	  <unit> (module <m>: -variable a, +variable b, -output c, +output d)
//
// baselineLabel is printed as given. Without a baseline the first line is
// "baseline: none (no baseline)" and the Impacted section is omitted, since
// nothing can be compared. Empty sections still print their header.
func BlastText(w io.Writer, res impact.Result, baselineLabel string) error {
	var b bytes.Buffer
	b.WriteString("baseline: ")
	if res.Baseline {
		b.WriteString(baselineLabel)
	} else {
		b.WriteString("none (" + blastNoBaselineNote + ")")
	}
	b.WriteByte('\n')

	b.WriteString("Broken (")
	b.WriteString(strconv.Itoa(len(res.Broken)))
	b.WriteString("):\n")
	for _, u := range res.Broken {
		b.WriteString("  ")
		b.WriteString(u.Subject.String())
		b.WriteByte('\n')
		for _, d := range u.Findings {
			p := d.Pos()
			b.WriteString("    ")
			b.WriteString(p.File().String())
			b.WriteByte(':')
			b.WriteString(strconv.Itoa(p.Line()))
			b.WriteByte(':')
			b.WriteString(strconv.Itoa(p.Column()))
			b.WriteString(": ")
			b.WriteString(string(d.Code()))
			b.WriteByte(' ')
			b.WriteString(d.Message())
			b.WriteByte('\n')
		}
	}

	if res.Baseline {
		b.WriteString("Impacted (")
		b.WriteString(strconv.Itoa(len(res.Impacted)))
		b.WriteString("):\n")
		for _, u := range res.Impacted {
			b.WriteString("  ")
			b.WriteString(u.Unit.String())
			b.WriteString(" (module ")
			b.WriteString(u.Change.Module.String())
			b.WriteString(": ")
			writeChangeTokens(&b, u.Change)
			b.WriteString(")\n")
		}
	}

	_, err := w.Write(b.Bytes())
	return err
}

// writeChangeTokens writes c's names as "-variable x, +variable y,
// -output z, +output w", in that group order; names within a group keep
// the domain's sorted order.
func writeChangeTokens(b *bytes.Buffer, c impact.SurfaceChange) {
	first := true
	group := func(prefix string, names []string) {
		for _, n := range names {
			if !first {
				b.WriteString(", ")
			}
			first = false
			b.WriteString(prefix)
			b.WriteString(n)
		}
	}
	group("-variable ", c.RemovedVariables)
	group("+variable ", c.AddedVariables)
	group("-output ", c.RemovedOutputs)
	group("+output ", c.AddedOutputs)
}

// BlastJSON writes a blast radius as an indented JSON document (version 1,
// kind "blast"). Every list is written as [] when empty, never null. A run
// without a baseline carries baseline:false, a "note" and an empty
// impacted list.
func BlastJSON(w io.Writer, res impact.Result) error {
	doc := blastDoc{
		Version:  blastSchemaVersion,
		Kind:     blastKind,
		Baseline: res.Baseline,
		Broken:   []blastBroken{},
		Impacted: []blastImpacted{},
	}
	if !res.Baseline {
		doc.Note = blastNoBaselineNote
	}
	for _, u := range res.Broken {
		bu := blastBroken{Unit: u.Subject.String(), Findings: []blastFinding{}}
		for _, d := range u.Findings {
			bu.Findings = append(bu.Findings, blastFindingOf(d))
		}
		doc.Broken = append(doc.Broken, bu)
	}
	if res.Baseline {
		for _, u := range res.Impacted {
			doc.Impacted = append(doc.Impacted, blastImpacted{
				Unit:             u.Unit.String(),
				Module:           u.Change.Module.String(),
				AddedVariables:   nonNil(u.Change.AddedVariables),
				RemovedVariables: nonNil(u.Change.RemovedVariables),
				AddedOutputs:     nonNil(u.Change.AddedOutputs),
				RemovedOutputs:   nonNil(u.Change.RemovedOutputs),
			})
		}
	}
	doc.Summary = blastSummary{Broken: len(doc.Broken), Impacted: len(doc.Impacted)}

	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(doc); err != nil {
		return err
	}
	_, err := w.Write(b.Bytes())
	return err
}

func blastFindingOf(d diagnostic.Diagnostic) blastFinding {
	p := d.Pos()
	return blastFinding{
		Code:     string(d.Code()),
		Severity: d.Severity().String(),
		File:     p.File().String(),
		Line:     p.Line(),
		Column:   p.Column(),
		Message:  d.Message(),
	}
}

// nonNil returns s, or an empty slice when s is nil, so JSON never shows
// null for a list.
func nonNil(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}
