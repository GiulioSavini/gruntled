// Package repograph holds the domain's ubiquitous-language value objects
// (RepoPath, Position, Surface, Module, Dependency, Reference, Unit) and the
// RepositoryGraph aggregate root that ties them together. Every type in this
// package is immutable and every query is a pure function: no filesystem, no
// HCL parsing, nothing but the Go standard library.
package repograph

import (
	"errors"
	"path"
	"strconv"
	"strings"
)

// RepoPath is a repository-relative, slash-separated, cleaned path. It
// carries an unexported field so a value can only be produced through
// NewRepoPath or MustRepoPath, never by an untyped string conversion. It is
// comparable and therefore usable as a map key.
type RepoPath struct {
	p string
}

// NewRepoPath validates s and returns a RepoPath. s must be non-empty,
// already clean per path.Clean, not absolute, contain no backslashes, and
// not be (or start with) "..". The repository root is spelled ".".
func NewRepoPath(s string) (RepoPath, error) {
	if s == "" {
		return RepoPath{}, errors.New("repograph: invalid repo path " + strconv.Quote(s) + ": must not be empty")
	}
	if strings.Contains(s, `\`) {
		return RepoPath{}, errors.New("repograph: invalid repo path " + strconv.Quote(s) + ": must not contain a backslash")
	}
	if strings.HasPrefix(s, "/") {
		return RepoPath{}, errors.New("repograph: invalid repo path " + strconv.Quote(s) + ": must not be absolute")
	}
	if s == ".." || strings.HasPrefix(s, "../") {
		return RepoPath{}, errors.New("repograph: invalid repo path " + strconv.Quote(s) + ": must not escape the repository root")
	}
	if path.Clean(s) != s {
		return RepoPath{}, errors.New("repograph: invalid repo path " + strconv.Quote(s) + ": must already be clean (got " + strconv.Quote(path.Clean(s)) + ")")
	}
	return RepoPath{p: s}, nil
}

// MustRepoPath is like NewRepoPath but panics on invalid input. It exists
// for tests and generator constants, where the input is a compile-time
// literal known to be valid.
func MustRepoPath(s string) RepoPath {
	p, err := NewRepoPath(s)
	if err != nil {
		panic(err)
	}
	return p
}

// String returns the repo-relative path as a plain string.
func (p RepoPath) String() string {
	return p.p
}

// IsZero reports whether p is the zero value, i.e. it was never constructed
// through NewRepoPath or MustRepoPath.
func (p RepoPath) IsZero() bool {
	return p.p == ""
}

// Compare orders RepoPath values by their string form, so it agrees with
// strings.Compare and is total, transitive and reflexive.
func (p RepoPath) Compare(q RepoPath) int {
	return strings.Compare(p.p, q.p)
}
