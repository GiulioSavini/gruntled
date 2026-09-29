package repograph

import (
	"slices"
	"strconv"
)

// UnitReference pairs a Reference with the Unit it was declared on, so a
// caller iterating RepositoryGraph.References can tell which unit each
// reference belongs to.
type UnitReference struct {
	Unit      RepoPath
	Reference Reference
}

// DuplicateUnitError is returned by NewRepositoryGraph when two units share
// the same Path.
type DuplicateUnitError struct {
	Path RepoPath
}

func (e *DuplicateUnitError) Error() string {
	return "repograph: duplicate unit path " + strconv.Quote(e.Path.String())
}

// DuplicateModuleError is returned by NewRepositoryGraph when two modules
// share the same Path.
type DuplicateModuleError struct {
	Path RepoPath
}

func (e *DuplicateModuleError) Error() string {
	return "repograph: duplicate module path " + strconv.Quote(e.Path.String())
}

// MissingModuleError is returned by NewRepositoryGraph when a resolved
// unit's module is absent from the graph's modules.
type MissingModuleError struct {
	Unit   RepoPath
	Module RepoPath
}

func (e *MissingModuleError) Error() string {
	return "repograph: unit " + strconv.Quote(e.Unit.String()) + " resolves to module " + strconv.Quote(e.Module.String()) + ", which is not in the graph"
}

// InvalidUnitError is returned by NewRepositoryGraph when units contains a
// zero-value Unit: a zero Path, or a Status outside the three defined
// UnitStatus constants. Index is the unit's position in the original,
// unsorted units slice, so the caller can trace it back to its input.
type InvalidUnitError struct {
	Index int
}

func (e *InvalidUnitError) Error() string {
	return "repograph: invalid unit at input index " + strconv.Itoa(e.Index) + ": zero value"
}

// InvalidModuleError is returned by NewRepositoryGraph when modules contains
// a zero-value Module: a zero Path, or an unknown module (Surface not
// known) with an empty UnknownReason. Index is the module's position in the
// original, unsorted modules slice.
type InvalidModuleError struct {
	Index int
}

func (e *InvalidModuleError) Error() string {
	return "repograph: invalid module at input index " + strconv.Itoa(e.Index) + ": zero value"
}

// RepositoryGraph is the aggregate root over a repository's units and
// modules. It normalizes its input to a sorted, deduplicated, defensively
// copied internal representation at construction time, so that the same
// units and modules passed in any order produce an identical graph and
// every query is a pure function over that graph.
type RepositoryGraph struct {
	units       []Unit
	modules     []Module
	unitIndex   map[RepoPath]int
	moduleIndex map[RepoPath]int
}

// NewRepositoryGraph validates and normalizes units and modules into a
// RepositoryGraph. Before sorting, it rejects any zero-value Unit or Module
// in the input (input-order index reported via InvalidUnitError /
// InvalidModuleError). It then sorts both by Path, rejects duplicate unit or
// module paths, and rejects a resolved unit whose module is absent from
// modules. A dependency whose Target is not a unit in the graph is not an
// error: a future analyzer (GRT002) diagnoses that case, not the aggregate.
func NewRepositoryGraph(units []Unit, modules []Module) (*RepositoryGraph, error) {
	for i, u := range units {
		if u.Path().IsZero() || !u.Status().IsValid() {
			return nil, &InvalidUnitError{Index: i}
		}
	}
	for i, m := range modules {
		if m.path.IsZero() || (!m.known && m.unknownReason == "") {
			return nil, &InvalidModuleError{Index: i}
		}
	}

	sortedUnits := slices.Clone(units)
	slices.SortFunc(sortedUnits, func(a, b Unit) int {
		return a.Path().Compare(b.Path())
	})
	for i := 1; i < len(sortedUnits); i++ {
		if sortedUnits[i].Path() == sortedUnits[i-1].Path() {
			return nil, &DuplicateUnitError{Path: sortedUnits[i].Path()}
		}
	}

	sortedModules := slices.Clone(modules)
	slices.SortFunc(sortedModules, func(a, b Module) int {
		return a.Path().Compare(b.Path())
	})
	for i := 1; i < len(sortedModules); i++ {
		if sortedModules[i].Path() == sortedModules[i-1].Path() {
			return nil, &DuplicateModuleError{Path: sortedModules[i].Path()}
		}
	}

	moduleIndex := make(map[RepoPath]int, len(sortedModules))
	for i, m := range sortedModules {
		moduleIndex[m.Path()] = i
	}

	unitIndex := make(map[RepoPath]int, len(sortedUnits))
	for i, u := range sortedUnits {
		unitIndex[u.Path()] = i
	}

	for _, u := range sortedUnits {
		modPath, ok := u.Module()
		if !ok {
			continue
		}
		if _, ok := moduleIndex[modPath]; !ok {
			return nil, &MissingModuleError{Unit: u.Path(), Module: modPath}
		}
	}

	return &RepositoryGraph{
		units:       sortedUnits,
		modules:     sortedModules,
		unitIndex:   unitIndex,
		moduleIndex: moduleIndex,
	}, nil
}

// Units returns a sorted-by-path defensive copy of the graph's units.
func (g *RepositoryGraph) Units() []Unit {
	return slices.Clone(g.units)
}

// Unit returns the unit at p and true if it exists in the graph.
func (g *RepositoryGraph) Unit(p RepoPath) (Unit, bool) {
	i, ok := g.unitIndex[p]
	if !ok {
		return Unit{}, false
	}
	return g.units[i], true
}

// Modules returns a sorted-by-path defensive copy of the graph's modules.
func (g *RepositoryGraph) Modules() []Module {
	return slices.Clone(g.modules)
}

// Module returns the module at p and true if it exists in the graph.
func (g *RepositoryGraph) Module(p RepoPath) (Module, bool) {
	i, ok := g.moduleIndex[p]
	if !ok {
		return Module{}, false
	}
	return g.modules[i], true
}

// ModuleOf returns the module a unit resolves to. It returns false if the
// unit is absent from the graph, or is module-unknown or config-unknown.
// The returned Module's own Surface() may itself be unknown (a resolved
// unit whose module directory could not be read): callers must check
// Surface()'s ok result too.
func (g *RepositoryGraph) ModuleOf(unit RepoPath) (Module, bool) {
	u, ok := g.Unit(unit)
	if !ok {
		return Module{}, false
	}
	modPath, ok := u.Module()
	if !ok {
		return Module{}, false
	}
	return g.Module(modPath)
}

// DependencyTarget resolves hop 1 of the GRAPH-04 traversal: it returns the
// unit that unit's dependency named dep points at. It returns false if the
// unit or the dependency is missing from the graph, if the dependency is
// unresolved, or if its target is not a unit in the graph.
func (g *RepositoryGraph) DependencyTarget(unit RepoPath, dep string) (Unit, bool) {
	u, ok := g.Unit(unit)
	if !ok {
		return Unit{}, false
	}
	d, ok := u.Dependency(dep)
	if !ok {
		return Unit{}, false
	}
	target, ok := d.Target()
	if !ok {
		return Unit{}, false
	}
	return g.Unit(target)
}

// References returns every reference of every unit in the graph, sorted by
// Reference.Pos, then Unit, then Dependency, then Output, so the order is
// total and deterministic regardless of unit input order.
func (g *RepositoryGraph) References() []UnitReference {
	var all []UnitReference
	for _, u := range g.units {
		for _, r := range u.References() {
			all = append(all, UnitReference{Unit: u.Path(), Reference: r})
		}
	}
	slices.SortFunc(all, func(a, b UnitReference) int {
		if c := a.Reference.Pos().Compare(b.Reference.Pos()); c != 0 {
			return c
		}
		if c := a.Unit.Compare(b.Unit); c != 0 {
			return c
		}
		if c := compareStrings(a.Reference.Dependency(), b.Reference.Dependency()); c != 0 {
			return c
		}
		return compareStrings(a.Reference.Output(), b.Reference.Output())
	})
	return all
}

// EdgeKind is where a dependency edge was declared.
type EdgeKind int

const (
	// EdgeBlock is a `dependency "<name>"` block's config_path.
	EdgeBlock EdgeKind = iota
	// EdgePaths is an entry of a `dependencies { paths = [...] }` block.
	EdgePaths
)

// String renders the kind as "block" or "paths".
func (k EdgeKind) String() string {
	switch k {
	case EdgeBlock:
		return "block"
	case EdgePaths:
		return "paths"
	default:
		return "EdgeKind(" + strconv.Itoa(int(k)) + ")"
	}
}

// Edge is one resolved dependency edge from a unit to the directory it
// depends on. The target need not be a unit in the graph. Pos is the
// config_path value position for a block edge and the entry position for a
// paths edge. Enabled is the block's enabled fact; a paths edge is always
// TristateTrue (the `dependencies` block has no enabled attribute).
type Edge struct {
	kind    EdgeKind
	from    RepoPath
	to      RepoPath
	pos     Position
	enabled Tristate
}

// Kind returns where the edge was declared.
func (e Edge) Kind() EdgeKind { return e.kind }

// From returns the declaring unit's path.
func (e Edge) From() RepoPath { return e.from }

// To returns the target directory's repo-relative path.
func (e Edge) To() RepoPath { return e.to }

// Pos returns where the edge's target is written.
func (e Edge) Pos() Position { return e.pos }

// Enabled returns the edge's enabled fact.
func (e Edge) Enabled() Tristate { return e.enabled }

// Edges returns every resolved dependency edge in the graph, block and
// paths alike, skipping unresolved ones. The order is deterministic: from
// ascending, then Pos, then kind, then to.
func (g *RepositoryGraph) Edges() []Edge {
	var edges []Edge
	for _, u := range g.units {
		for _, d := range u.deps {
			to, ok := d.Target()
			if !ok {
				continue
			}
			edges = append(edges, Edge{kind: EdgeBlock, from: u.path, to: to, pos: d.pathPos, enabled: d.opts.Enabled})
		}
		for _, pd := range u.pathDeps {
			to, ok := pd.Target()
			if !ok {
				continue
			}
			edges = append(edges, Edge{kind: EdgePaths, from: u.path, to: to, pos: pd.pos, enabled: TristateTrue})
		}
	}
	slices.SortFunc(edges, func(a, b Edge) int {
		if c := a.from.Compare(b.from); c != 0 {
			return c
		}
		if c := a.pos.Compare(b.pos); c != 0 {
			return c
		}
		if a.kind != b.kind {
			if a.kind < b.kind {
				return -1
			}
			return 1
		}
		return a.to.Compare(b.to)
	})
	return edges
}
