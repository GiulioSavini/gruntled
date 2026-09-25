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
// RepositoryGraph. It sorts both by Path, rejects duplicate unit or module
// paths, and rejects a resolved unit whose module is absent from modules.
// A dependency whose Target is not a unit in the graph is not an error: a
// future analyzer (GRT002) diagnoses that case, not the aggregate.
func NewRepositoryGraph(units []Unit, modules []Module) (*RepositoryGraph, error) {
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
// unit is absent from the graph or is Unknown.
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
// unit, the dependency, or the target unit is missing from the graph.
func (g *RepositoryGraph) DependencyTarget(unit RepoPath, dep string) (Unit, bool) {
	u, ok := g.Unit(unit)
	if !ok {
		return Unit{}, false
	}
	d, ok := u.Dependency(dep)
	if !ok {
		return Unit{}, false
	}
	return g.Unit(d.Target())
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
