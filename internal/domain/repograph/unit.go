package repograph

import (
	"errors"
	"slices"
	"strconv"
)

// Dependency is a `dependency "<name>" { config_path = ... }` block, with
// its target already resolved to a unit path.
type Dependency struct {
	name   string
	target RepoPath
	pos    Position
}

// NewDependency validates its arguments and returns a Dependency. name must
// be non-empty and target must be non-zero.
func NewDependency(name string, target RepoPath, pos Position) (Dependency, error) {
	if name == "" {
		return Dependency{}, errors.New("repograph: invalid dependency: name must not be empty")
	}
	if target.IsZero() {
		return Dependency{}, errors.New("repograph: invalid dependency " + strconv.Quote(name) + ": target must not be zero")
	}
	return Dependency{name: name, target: target, pos: pos}, nil
}

// Name returns the dependency block's label.
func (d Dependency) Name() string {
	return d.name
}

// Target returns the repo-relative path of the unit this dependency points
// at.
func (d Dependency) Target() RepoPath {
	return d.target
}

// Pos returns the position of the dependency block.
func (d Dependency) Pos() Position {
	return d.pos
}

// Reference is one `dependency.<Dependency>.outputs.<Output>` traversal. Pos
// is the position of the first byte of `dependency`.
type Reference struct {
	dependency string
	output     string
	pos        Position
}

// NewReference validates its arguments and returns a Reference. Both
// dependency and output must be non-empty.
func NewReference(dependency, output string, pos Position) (Reference, error) {
	if dependency == "" {
		return Reference{}, errors.New("repograph: invalid reference: dependency must not be empty")
	}
	if output == "" {
		return Reference{}, errors.New("repograph: invalid reference: output must not be empty")
	}
	return Reference{dependency: dependency, output: output, pos: pos}, nil
}

// Dependency returns the referenced dependency block's label.
func (r Reference) Dependency() string {
	return r.dependency
}

// Output returns the referenced output name.
func (r Reference) Output() string {
	return r.output
}

// Pos returns the position of the reference.
func (r Reference) Pos() Position {
	return r.pos
}

// UnitStatus is whether a Unit's module could be resolved offline.
type UnitStatus int

const (
	// StatusResolved means the unit's module is known and local.
	StatusResolved UnitStatus = iota + 1
	// StatusUnknown means the unit's module could not be resolved
	// offline; analyzers must stay silent about such units (design doc
	// §7).
	StatusUnknown
)

// String renders the status as "resolved" or "unknown".
func (s UnitStatus) String() string {
	switch s {
	case StatusResolved:
		return "resolved"
	case StatusUnknown:
		return "unknown"
	default:
		return "UnitStatus(" + strconv.Itoa(int(s)) + ")"
	}
}

// Unit is a Terragrunt unit: a repo-relative path, the module it resolves
// to (when known), and the dependency blocks and output references it
// declares.
type Unit struct {
	path          RepoPath
	status        UnitStatus
	module        RepoPath
	unknownReason string
	deps          []Dependency
	refs          []Reference
}

// NewResolvedUnit validates its arguments and returns a Unit whose module is
// known. path and module must be non-zero, and dependency names must be
// unique within the unit. deps is sorted by Name and refs by Pos, then
// Dependency, then Output; both are cloned.
func NewResolvedUnit(path, module RepoPath, deps []Dependency, refs []Reference) (Unit, error) {
	if path.IsZero() {
		return Unit{}, errors.New("repograph: invalid unit: path must not be zero")
	}
	if module.IsZero() {
		return Unit{}, errors.New("repograph: invalid unit " + strconv.Quote(path.String()) + ": module must not be zero")
	}

	sortedDeps := slices.Clone(deps)
	slices.SortFunc(sortedDeps, func(a, b Dependency) int {
		return compareStrings(a.name, b.name)
	})
	for i := 1; i < len(sortedDeps); i++ {
		if sortedDeps[i].name == sortedDeps[i-1].name {
			return Unit{}, errors.New("repograph: invalid unit " + strconv.Quote(path.String()) + ": duplicate dependency name " + strconv.Quote(sortedDeps[i].name))
		}
	}

	sortedRefs := slices.Clone(refs)
	slices.SortFunc(sortedRefs, func(a, b Reference) int {
		if c := a.pos.Compare(b.pos); c != 0 {
			return c
		}
		if c := compareStrings(a.dependency, b.dependency); c != 0 {
			return c
		}
		return compareStrings(a.output, b.output)
	})

	return Unit{
		path:   path,
		status: StatusResolved,
		module: module,
		deps:   sortedDeps,
		refs:   sortedRefs,
	}, nil
}

// NewUnknownUnit validates its arguments and returns a Unit whose module
// could not be resolved offline. reason must be non-empty. An Unknown unit
// has no dependencies and no references.
func NewUnknownUnit(path RepoPath, reason string) (Unit, error) {
	if path.IsZero() {
		return Unit{}, errors.New("repograph: invalid unit: path must not be zero")
	}
	if reason == "" {
		return Unit{}, errors.New("repograph: invalid unit " + strconv.Quote(path.String()) + ": unknown reason must not be empty")
	}
	return Unit{path: path, status: StatusUnknown, unknownReason: reason}, nil
}

// Path returns the unit's repo-relative path.
func (u Unit) Path() RepoPath {
	return u.path
}

// Status reports whether the unit's module is resolved or unknown.
func (u Unit) Status() UnitStatus {
	return u.status
}

// Module returns the unit's module path and true when the unit is
// resolved; it returns the zero RepoPath and false when the unit is
// Unknown.
func (u Unit) Module() (RepoPath, bool) {
	if u.status != StatusResolved {
		return RepoPath{}, false
	}
	return u.module, true
}

// UnknownReason returns why an Unknown unit could not be resolved offline.
// It is empty for a resolved unit.
func (u Unit) UnknownReason() string {
	return u.unknownReason
}

// Dependencies returns a sorted-by-name copy of the unit's dependency
// blocks.
func (u Unit) Dependencies() []Dependency {
	return slices.Clone(u.deps)
}

// Dependency returns the named dependency block and true if it exists on
// this unit.
func (u Unit) Dependency(name string) (Dependency, bool) {
	for _, d := range u.deps {
		if d.name == name {
			return d, true
		}
	}
	return Dependency{}, false
}

// References returns a sorted-by-position copy of the unit's output
// references.
func (u Unit) References() []Reference {
	return slices.Clone(u.refs)
}

func compareStrings(a, b string) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	default:
		return 0
	}
}
