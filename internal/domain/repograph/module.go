package repograph

import (
	"errors"
	"strconv"
)

// Module is a Terragrunt/Terraform module: a repo-relative path plus the
// Surface it exposes to units that depend on it, when that surface could be
// read offline.
type Module struct {
	path          RepoPath
	known         bool
	surface       Surface
	unknownReason string
}

// NewModule validates path and returns a Module with a known Surface. path
// must be non-zero.
func NewModule(path RepoPath, surface Surface) (Module, error) {
	if path.IsZero() {
		return Module{}, errors.New("repograph: invalid module: path must not be zero")
	}
	return Module{path: path, known: true, surface: surface}, nil
}

// NewUnknownModule validates its arguments and returns a Module whose
// surface could not be read offline (a local module directory that is
// missing, empty or unparsable). reason must be non-empty. The graph can
// still say "this target exists, its surface is unknown" instead of
// dropping the target entirely.
func NewUnknownModule(path RepoPath, reason string) (Module, error) {
	if path.IsZero() {
		return Module{}, errors.New("repograph: invalid module: path must not be zero")
	}
	if reason == "" {
		return Module{}, errors.New("repograph: invalid module " + strconv.Quote(path.String()) + ": unknown reason must not be empty")
	}
	return Module{path: path, known: false, unknownReason: reason}, nil
}

// Path returns the module's repo-relative path.
func (m Module) Path() RepoPath {
	return m.path
}

// Surface returns the module's public variable and output names, and true,
// only when the surface could be read. It returns the zero Surface and
// false for a module created with NewUnknownModule.
func (m Module) Surface() (Surface, bool) {
	if !m.known {
		return Surface{}, false
	}
	return m.surface, true
}

// UnknownReason returns why the module's surface could not be read
// offline. It is empty for a known module.
func (m Module) UnknownReason() string {
	return m.unknownReason
}
