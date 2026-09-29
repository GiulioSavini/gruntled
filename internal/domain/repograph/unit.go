package repograph

import (
	"errors"
	"slices"
	"strconv"
)

// Dependency is a `dependency "<name>" { ... }` block. It is either
// resolved, in which case Target returns the repo-relative unit path it
// points at, or unresolved (its config_path was dynamic, unresolvable, or
// escaped the repository), in which case it is kept on the unit with a
// non-empty UnresolvedReason instead of being dropped: dropping it would
// turn a `dependency.X.outputs.Y` reference into a reference to an
// undeclared dependency, a false-positive risk. Options carries the DIAG-03
// facts (mock_outputs, enabled, skip_outputs, ...) Phase 3 needs regardless
// of whether the dependency resolved.
type Dependency struct {
	name             string
	target           RepoPath
	resolved         bool
	unresolvedReason string
	pos              Position
	pathPos          Position
	state            TargetState
	opts             DependencyOptions
}

// TargetState is what the loader observed at a resolved dependency
// target's directory. The zero value, TargetUnknown, means nothing was
// observed and analyzers must stay silent about the target.
type TargetState int

const (
	// TargetUnknown means the target's state was not (or could not be)
	// determined. It is the zero value on purpose.
	TargetUnknown TargetState = iota
	// TargetDirMissing means the target directory does not exist.
	TargetDirMissing
	// TargetNoConfig means the target directory exists but holds no unit
	// configuration file.
	TargetNoConfig
	// TargetHasConfig means the target directory holds a unit
	// configuration file.
	TargetHasConfig
)

// String renders the state as "unknown", "dir-missing", "no-config" or
// "has-config".
func (s TargetState) String() string {
	switch s {
	case TargetUnknown:
		return "unknown"
	case TargetDirMissing:
		return "dir-missing"
	case TargetNoConfig:
		return "no-config"
	case TargetHasConfig:
		return "has-config"
	default:
		return "TargetState(" + strconv.Itoa(int(s)) + ")"
	}
}

// IsValid reports whether s is one of the four defined TargetState
// constants.
func (s TargetState) IsValid() bool {
	switch s {
	case TargetUnknown, TargetDirMissing, TargetNoConfig, TargetHasConfig:
		return true
	default:
		return false
	}
}

// validateDependencyCommon checks the fields NewDependency and
// NewUnresolvedDependency both validate: a non-empty name, a non-zero
// position and config_path position, and that every Tristate option field is one of the defined
// states. A value outside those states can only come from a forged
// conversion like Tristate(99); storing it would let a later query report a
// fact that was never actually observed.
func validateDependencyCommon(name string, pos, pathPos Position, opts DependencyOptions) error {
	if name == "" {
		return errors.New("repograph: invalid dependency: name must not be empty")
	}
	if pos.IsZero() {
		return errors.New("repograph: invalid dependency " + strconv.Quote(name) + ": position must not be zero")
	}
	if pathPos.IsZero() {
		return errors.New("repograph: invalid dependency " + strconv.Quote(name) + ": config_path position must not be zero")
	}
	if !opts.Enabled.IsValid() {
		return errors.New("repograph: invalid dependency " + strconv.Quote(name) + ": invalid option Enabled: " + strconv.Itoa(int(opts.Enabled)))
	}
	if !opts.SkipOutputs.IsValid() {
		return errors.New("repograph: invalid dependency " + strconv.Quote(name) + ": invalid option SkipOutputs: " + strconv.Itoa(int(opts.SkipOutputs)))
	}
	if !opts.MockMergeWithState.IsValid() {
		return errors.New("repograph: invalid dependency " + strconv.Quote(name) + ": invalid option MockMergeWithState: " + strconv.Itoa(int(opts.MockMergeWithState)))
	}
	return nil
}

// NewDependency validates its arguments and returns a resolved Dependency.
// name must be non-empty; target, pos (the block) and pathPos (the
// config_path value, or the block when the attribute has no position of its
// own) must be non-zero; state must be a defined TargetState; and every
// Tristate field of opts must be a defined Tristate value.
func NewDependency(name string, target RepoPath, pos, pathPos Position, state TargetState, opts DependencyOptions) (Dependency, error) {
	if err := validateDependencyCommon(name, pos, pathPos, opts); err != nil {
		return Dependency{}, err
	}
	if target.IsZero() {
		return Dependency{}, errors.New("repograph: invalid dependency " + strconv.Quote(name) + ": target must not be zero")
	}
	if !state.IsValid() {
		return Dependency{}, errors.New("repograph: invalid dependency " + strconv.Quote(name) + ": invalid target state: " + strconv.Itoa(int(state)))
	}
	return Dependency{name: name, target: target, resolved: true, pos: pos, pathPos: pathPos, state: state, opts: opts}, nil
}

// NewUnresolvedDependency validates its arguments and returns a Dependency
// whose target could not be determined. name and reason must be non-empty,
// pos and pathPos must be non-zero, and every Tristate field of opts must be
// a defined Tristate value. Its TargetState is always TargetUnknown.
func NewUnresolvedDependency(name, reason string, pos, pathPos Position, opts DependencyOptions) (Dependency, error) {
	if err := validateDependencyCommon(name, pos, pathPos, opts); err != nil {
		return Dependency{}, err
	}
	if reason == "" {
		return Dependency{}, errors.New("repograph: invalid dependency " + strconv.Quote(name) + ": unresolved reason must not be empty")
	}
	return Dependency{name: name, resolved: false, unresolvedReason: reason, pos: pos, pathPos: pathPos, opts: opts}, nil
}

// Name returns the dependency block's label.
func (d Dependency) Name() string {
	return d.name
}

// Target returns the repo-relative path of the unit this dependency points
// at, and true, only when the dependency resolved. It returns the zero
// RepoPath and false for an unresolved dependency, so a caller must handle
// the unresolved case explicitly rather than silently traversing a zero
// path.
func (d Dependency) Target() (RepoPath, bool) {
	if !d.resolved {
		return RepoPath{}, false
	}
	return d.target, true
}

// UnresolvedReason returns why the dependency's target could not be
// determined. It is empty for a resolved dependency.
func (d Dependency) UnresolvedReason() string {
	return d.unresolvedReason
}

// Options returns the dependency's DIAG-03 facts.
func (d Dependency) Options() DependencyOptions {
	return d.opts
}

// Pos returns the position of the dependency block.
func (d Dependency) Pos() Position {
	return d.pos
}

// PathPos returns the position of the config_path value (the block position
// when the attribute had no position of its own). GRT002 and GRT003 report
// here.
func (d Dependency) PathPos() Position {
	return d.pathPos
}

// TargetState returns what the loader observed at the target directory. It
// is always TargetUnknown for an unresolved dependency.
func (d Dependency) TargetState() TargetState {
	return d.state
}

// PathDependency is one entry of a `dependencies { paths = [...] }` block.
// It is either resolved (Target returns the repo-relative directory it
// names) or unresolved (a non-literal or unresolvable entry, kept with a
// non-empty UnresolvedReason). Path dependencies are kept apart from
// Dependency: they have no name and a unit may list the same target in both.
type PathDependency struct {
	target           RepoPath
	resolved         bool
	literal          string
	unresolvedReason string
	pos              Position
	state            TargetState
}

// NewPathDependency validates its arguments and returns a resolved
// PathDependency. target and pos must be non-zero, literal (the path text as
// written) must be non-empty, and state must be a defined TargetState.
func NewPathDependency(target RepoPath, literal string, pos Position, state TargetState) (PathDependency, error) {
	if target.IsZero() {
		return PathDependency{}, errors.New("repograph: invalid path dependency: target must not be zero")
	}
	if literal == "" {
		return PathDependency{}, errors.New("repograph: invalid path dependency " + strconv.Quote(target.String()) + ": literal must not be empty")
	}
	if pos.IsZero() {
		return PathDependency{}, errors.New("repograph: invalid path dependency " + strconv.Quote(target.String()) + ": position must not be zero")
	}
	if !state.IsValid() {
		return PathDependency{}, errors.New("repograph: invalid path dependency " + strconv.Quote(target.String()) + ": invalid target state: " + strconv.Itoa(int(state)))
	}
	return PathDependency{target: target, resolved: true, literal: literal, pos: pos, state: state}, nil
}

// NewUnresolvedPathDependency validates its arguments and returns a
// PathDependency whose target could not be determined. reason must be
// non-empty and pos non-zero. Its TargetState is always TargetUnknown.
func NewUnresolvedPathDependency(reason string, pos Position) (PathDependency, error) {
	if reason == "" {
		return PathDependency{}, errors.New("repograph: invalid path dependency: unresolved reason must not be empty")
	}
	if pos.IsZero() {
		return PathDependency{}, errors.New("repograph: invalid path dependency: position must not be zero")
	}
	return PathDependency{unresolvedReason: reason, pos: pos}, nil
}

// Target returns the directory this entry names, and true, only when it
// resolved.
func (p PathDependency) Target() (RepoPath, bool) {
	if !p.resolved {
		return RepoPath{}, false
	}
	return p.target, true
}

// UnresolvedReason returns why the entry could not be resolved. It is empty
// for a resolved entry.
func (p PathDependency) UnresolvedReason() string {
	return p.unresolvedReason
}

// Pos returns the position of the entry's value.
func (p PathDependency) Pos() Position {
	return p.pos
}

// TargetState returns what the loader observed at the target directory.
func (p PathDependency) TargetState() TargetState {
	return p.state
}

// Literal returns the path text as written. It is empty for an unresolved
// entry.
func (p PathDependency) Literal() string {
	return p.literal
}

// Reference is one `dependency.<Dependency>.outputs.<Output>` traversal. Pos
// is the position of the first byte of `dependency`.
type Reference struct {
	dependency string
	output     string
	pos        Position
}

// NewReference validates its arguments and returns a Reference. dependency
// and output must be non-empty, and pos must be non-zero.
func NewReference(dependency, output string, pos Position) (Reference, error) {
	if dependency == "" {
		return Reference{}, errors.New("repograph: invalid reference: dependency must not be empty")
	}
	if output == "" {
		return Reference{}, errors.New("repograph: invalid reference: output must not be empty")
	}
	if pos.IsZero() {
		return Reference{}, errors.New("repograph: invalid reference: position must not be zero")
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

// UnitStatus is whether a Unit's own configuration and module could be
// resolved offline.
type UnitStatus int

const (
	// StatusResolved means the unit's own config is known and its module
	// path is known.
	StatusResolved UnitStatus = iota + 1
	// StatusModuleUnknown means the unit's own config is known, but its
	// module path or effective surface could not be determined offline
	// (a remote or dynamic source, a missing module directory, or an
	// unparsable module). Its dependencies and references are still kept:
	// GRT001 checks the TARGET unit's module, not the referencing unit's,
	// so references INTO other units' modules must stay checkable.
	StatusModuleUnknown
	// StatusConfigUnknown means the unit's own terragrunt.hcl, or a merged
	// include, is broken or dynamic in a way this domain does not model.
	// Such a unit carries no dependencies and no references and analyzers
	// must stay silent about it.
	StatusConfigUnknown
)

// String renders the status as "resolved", "module-unknown" or
// "config-unknown".
func (s UnitStatus) String() string {
	switch s {
	case StatusResolved:
		return "resolved"
	case StatusModuleUnknown:
		return "module-unknown"
	case StatusConfigUnknown:
		return "config-unknown"
	default:
		return "UnitStatus(" + strconv.Itoa(int(s)) + ")"
	}
}

// IsValid reports whether s is one of the three defined UnitStatus
// constants. UnitStatus(0) is the zero value and is never valid: only
// NewResolvedUnit, NewModuleUnknownUnit and NewConfigUnknownUnit produce a
// valid status.
func (s UnitStatus) IsValid() bool {
	switch s {
	case StatusResolved, StatusModuleUnknown, StatusConfigUnknown:
		return true
	default:
		return false
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
	pathDeps      []PathDependency
}

// sortAndValidateDepsRefs sorts deps by Name and refs by Pos (then
// Dependency, then Output), clones both defensively, rejects a zero-value
// Dependency or Reference entry, and rejects duplicate dependency names. It
// is shared by NewResolvedUnit and NewModuleUnknownUnit, the only two
// constructors that keep deps/refs.
func sortAndValidateDepsRefs(unitPath RepoPath, deps []Dependency, refs []Reference) ([]Dependency, []Reference, error) {
	// Every constructor rejects an empty name, so only the zero value can
	// have one: a caller that appended a zero-value Dependency{} or
	// Reference{} (for example a discarded error) must not have it
	// silently dropped, which would turn a real reference into one to an
	// undeclared dependency.
	for i, d := range deps {
		if d.name == "" {
			return nil, nil, errors.New("repograph: invalid unit " + strconv.Quote(unitPath.String()) + ": zero-value dependency at index " + strconv.Itoa(i))
		}
	}
	for i, r := range refs {
		if r.dependency == "" {
			return nil, nil, errors.New("repograph: invalid unit " + strconv.Quote(unitPath.String()) + ": zero-value reference at index " + strconv.Itoa(i))
		}
	}

	sortedDeps := slices.Clone(deps)
	slices.SortFunc(sortedDeps, func(a, b Dependency) int {
		return compareStrings(a.name, b.name)
	})
	for i := 1; i < len(sortedDeps); i++ {
		if sortedDeps[i].name == sortedDeps[i-1].name {
			return nil, nil, errors.New("repograph: invalid unit " + strconv.Quote(unitPath.String()) + ": duplicate dependency name " + strconv.Quote(sortedDeps[i].name))
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

	return sortedDeps, sortedRefs, nil
}

// NewResolvedUnit validates its arguments and returns a Unit whose own
// config and module are both known. path and module must be non-zero,
// dependency names must be unique within the unit, and deps/refs must not
// contain a zero-value Dependency or Reference entry. deps is sorted by
// Name and refs by Pos, then Dependency, then Output; both are cloned.
func NewResolvedUnit(path, module RepoPath, deps []Dependency, refs []Reference) (Unit, error) {
	if path.IsZero() {
		return Unit{}, errors.New("repograph: invalid unit: path must not be zero")
	}
	if module.IsZero() {
		return Unit{}, errors.New("repograph: invalid unit " + strconv.Quote(path.String()) + ": module must not be zero")
	}

	sortedDeps, sortedRefs, err := sortAndValidateDepsRefs(path, deps, refs)
	if err != nil {
		return Unit{}, err
	}

	return Unit{
		path:   path,
		status: StatusResolved,
		module: module,
		deps:   sortedDeps,
		refs:   sortedRefs,
	}, nil
}

// NewModuleUnknownUnit validates its arguments and returns a Unit whose own
// config is known but whose module path or effective surface could not be
// determined offline. reason must be non-empty. Unlike a config-unknown
// unit, its dependencies and references are validated, sorted and kept: a
// dependency name must still be unique within the unit, and deps/refs must
// not contain a zero-value Dependency or Reference entry.
func NewModuleUnknownUnit(path RepoPath, reason string, deps []Dependency, refs []Reference) (Unit, error) {
	if path.IsZero() {
		return Unit{}, errors.New("repograph: invalid unit: path must not be zero")
	}
	if reason == "" {
		return Unit{}, errors.New("repograph: invalid unit " + strconv.Quote(path.String()) + ": unknown reason must not be empty")
	}

	sortedDeps, sortedRefs, err := sortAndValidateDepsRefs(path, deps, refs)
	if err != nil {
		return Unit{}, err
	}

	return Unit{
		path:          path,
		status:        StatusModuleUnknown,
		unknownReason: reason,
		deps:          sortedDeps,
		refs:          sortedRefs,
	}, nil
}

// NewConfigUnknownUnit validates its arguments and returns a Unit whose own
// config could not be determined offline (its terragrunt.hcl, or a merged
// include, is broken or dynamic). reason must be non-empty. A config-unknown
// unit has no dependencies and no references.
func NewConfigUnknownUnit(path RepoPath, reason string) (Unit, error) {
	if path.IsZero() {
		return Unit{}, errors.New("repograph: invalid unit: path must not be zero")
	}
	if reason == "" {
		return Unit{}, errors.New("repograph: invalid unit " + strconv.Quote(path.String()) + ": unknown reason must not be empty")
	}
	return Unit{path: path, status: StatusConfigUnknown, unknownReason: reason}, nil
}

// Path returns the unit's repo-relative path.
func (u Unit) Path() RepoPath {
	return u.path
}

// Status reports whether the unit is resolved, module-unknown or
// config-unknown.
func (u Unit) Status() UnitStatus {
	return u.status
}

// Module returns the unit's module path and true when the unit is
// resolved; it returns the zero RepoPath and false for a module-unknown or
// config-unknown unit.
func (u Unit) Module() (RepoPath, bool) {
	if u.status != StatusResolved {
		return RepoPath{}, false
	}
	return u.module, true
}

// UnknownReason returns why a module-unknown or config-unknown unit could
// not be fully resolved offline. It is empty for a resolved unit.
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

// WithPathDependencies returns a copy of u carrying pds, sorted by Pos then
// target. u itself is unchanged. It rejects a config-unknown unit (which
// carries no dependencies of any kind) and a zero-value entry.
func (u Unit) WithPathDependencies(pds []PathDependency) (Unit, error) {
	if u.status == StatusConfigUnknown {
		return Unit{}, errors.New("repograph: invalid unit " + strconv.Quote(u.path.String()) + ": a config-unknown unit has no path dependencies")
	}
	for i, pd := range pds {
		if pd.pos.IsZero() {
			return Unit{}, errors.New("repograph: invalid unit " + strconv.Quote(u.path.String()) + ": zero-value path dependency at index " + strconv.Itoa(i))
		}
	}
	sorted := slices.Clone(pds)
	slices.SortFunc(sorted, comparePathDependencies)
	u.pathDeps = sorted
	return u, nil
}

// PathDependencies returns a sorted-by-position copy of the unit's
// `dependencies` block entries.
func (u Unit) PathDependencies() []PathDependency {
	return slices.Clone(u.pathDeps)
}

func comparePathDependencies(a, b PathDependency) int {
	if c := a.pos.Compare(b.pos); c != 0 {
		return c
	}
	if c := a.target.Compare(b.target); c != 0 {
		return c
	}
	return compareStrings(a.unresolvedReason, b.unresolvedReason)
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
