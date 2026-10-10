package presenter

import (
	"bytes"
	"encoding/json"
	"io"
	"slices"

	"github.com/GiulioSavini/gruntled/internal/domain/diagnostic"
	"github.com/GiulioSavini/gruntled/internal/domain/repograph"
)

// graphSchemaVersion is the version of the graph document Graph writes.
// Bump it on any incompatible change to the shapes below.
const graphSchemaVersion = 1

// graphKind tells a graph document apart from a check report.
const graphKind = "graph"

// The graph document is built from structs only, never maps, so its key
// order is fixed by the field order here and the output is deterministic.
type graphDoc struct {
	Version    int                  `json:"version"`
	Kind       string               `json:"kind"`
	Units      []graphUnit          `json:"units"`
	Modules    []graphModule        `json:"modules"`
	Edges      []graphEdge          `json:"edges"`
	Unresolved []graphUnresolvedDep `json:"unresolved_dependencies"`
	Summary    graphSummary         `json:"summary"`
}

// graphPos is a source position. It is always written in full.
type graphPos struct {
	File   string `json:"file"`
	Line   int    `json:"line"`
	Column int    `json:"column"`
}

type graphUnit struct {
	Path       string           `json:"path"`
	Status     string           `json:"status"`
	Module     string           `json:"module,omitempty"`
	Reason     string           `json:"reason,omitempty"`
	References []graphReference `json:"references"`
}

type graphReference struct {
	Dependency string   `json:"dependency"`
	Output     string   `json:"output"`
	Position   graphPos `json:"position"`
}

type graphModule struct {
	Path      string   `json:"path"`
	Known     bool     `json:"known"`
	Variables []string `json:"variables"`
	Outputs   []string `json:"outputs"`
	Reason    string   `json:"reason,omitempty"`
}

type graphEdge struct {
	Kind        string   `json:"kind"`
	From        string   `json:"from"`
	To          string   `json:"to"`
	Position    graphPos `json:"position"`
	Name        string   `json:"name,omitempty"`
	TargetState string   `json:"target_state"`
	Enabled     string   `json:"enabled"`
	SkipOutputs string   `json:"skip_outputs"`
}

type graphUnresolvedDep struct {
	From     string   `json:"from"`
	Kind     string   `json:"kind"`
	Name     string   `json:"name,omitempty"`
	Position graphPos `json:"position"`
	Reason   string   `json:"reason"`
}

type graphSummary struct {
	Units          int `json:"units"`
	Resolved       int `json:"resolved"`
	ModuleUnknown  int `json:"module_unknown"`
	ConfigUnknown  int `json:"config_unknown"`
	UnknownModules int `json:"unknown_modules"`
}

func toGraphPos(p repograph.Position) graphPos {
	return graphPos{File: p.File().String(), Line: p.Line(), Column: p.Column()}
}

// Graph writes one indented JSON document (schema version 1, kind
// "graph") describing the repository graph: units, modules, resolved
// dependency edges (block and paths alike, in RepositoryGraph.Edges order),
// unresolved dependencies (block dependencies and paths entries whose
// target could not be determined, ordered by declaring unit then
// position) and a summary. Empty lists are written as [], never null, and
// HTML characters are not escaped. Columns are 1-based byte offsets.
// Reason strings are human-readable text, not a stable contract. The
// encoded bytes go through escapeJSON (DEL, C1, format and line/paragraph
// separator runes as \uXXXX).
func Graph(w io.Writer, g *repograph.RepositoryGraph) error {
	units := g.Units()
	modules := g.Modules()
	edges := g.Edges()
	doc := graphDoc{
		Version:    graphSchemaVersion,
		Kind:       graphKind,
		Units:      make([]graphUnit, 0, len(units)),
		Modules:    make([]graphModule, 0, len(modules)),
		Edges:      make([]graphEdge, 0, len(edges)),
		Unresolved: make([]graphUnresolvedDep, 0),
	}
	for _, u := range units {
		refs := u.References()
		gu := graphUnit{
			Path:       u.Path().String(),
			Status:     u.Status().String(),
			Reason:     u.UnknownReason(),
			References: make([]graphReference, 0, len(refs)),
		}
		if m, ok := u.Module(); ok {
			gu.Module = m.String()
		}
		for _, r := range refs {
			gu.References = append(gu.References, graphReference{
				Dependency: r.Dependency(),
				Output:     r.Output(),
				Position:   toGraphPos(r.Pos()),
			})
		}
		doc.Units = append(doc.Units, gu)
		doc.Unresolved = appendUnresolved(doc.Unresolved, u)
	}
	for _, m := range modules {
		gm := graphModule{
			Path:      m.Path().String(),
			Variables: make([]string, 0),
			Outputs:   make([]string, 0),
		}
		if s, ok := m.Surface(); ok {
			gm.Known = true
			gm.Variables = append(gm.Variables, s.Variables()...)
			gm.Outputs = append(gm.Outputs, s.Outputs()...)
		} else {
			gm.Reason = m.UnknownReason()
		}
		doc.Modules = append(doc.Modules, gm)
	}
	for _, e := range edges {
		doc.Edges = append(doc.Edges, graphEdge{
			Kind:        e.Kind().String(),
			From:        e.From().String(),
			To:          e.To().String(),
			Position:    toGraphPos(e.Pos()),
			Name:        e.Name(),
			TargetState: e.TargetState().String(),
			Enabled:     e.Enabled().String(),
			SkipOutputs: e.SkipOutputs().String(),
		})
	}
	c := tally(g, diagnostic.NewSet())
	doc.Summary = graphSummary{
		Units:          c.units,
		Resolved:       c.resolved,
		ModuleUnknown:  c.moduleUnknown,
		ConfigUnknown:  c.configUnknown,
		UnknownModules: c.unknownModules,
	}

	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(doc); err != nil {
		return err
	}
	_, err := w.Write(escapeJSON(b.Bytes()))
	return err
}

// appendUnresolved appends u's unresolved block dependencies (at their
// config_path position, as Edge.Pos) and unresolved paths entries, merged
// by position. Units arrive in path order, so the whole list ends up
// ordered by from, then position.
func appendUnresolved(dst []graphUnresolvedDep, u repograph.Unit) []graphUnresolvedDep {
	type item struct {
		pos repograph.Position
		dep graphUnresolvedDep
	}
	var items []item
	from := u.Path().String()
	for _, d := range u.Dependencies() {
		if _, ok := d.Target(); ok {
			continue
		}
		items = append(items, item{d.PathPos(), graphUnresolvedDep{
			From: from, Kind: repograph.EdgeBlock.String(), Name: d.Name(),
			Position: toGraphPos(d.PathPos()), Reason: d.UnresolvedReason(),
		}})
	}
	for _, pd := range u.PathDependencies() {
		if _, ok := pd.Target(); ok {
			continue
		}
		items = append(items, item{pd.Pos(), graphUnresolvedDep{
			From: from, Kind: repograph.EdgePaths.String(),
			Position: toGraphPos(pd.Pos()), Reason: pd.UnresolvedReason(),
		}})
	}
	slices.SortStableFunc(items, func(a, b item) int { return a.pos.Compare(b.pos) })
	for _, it := range items {
		dst = append(dst, it.dep)
	}
	return dst
}
