package repograph

import "errors"

// Module is a Terragrunt/Terraform module: a repo-relative path plus the
// Surface it exposes to units that depend on it.
type Module struct {
	path    RepoPath
	surface Surface
}

// NewModule validates path and returns a Module. path must be non-zero.
func NewModule(path RepoPath, surface Surface) (Module, error) {
	if path.IsZero() {
		return Module{}, errors.New("repograph: invalid module: path must not be zero")
	}
	return Module{path: path, surface: surface}, nil
}

// Path returns the module's repo-relative path.
func (m Module) Path() RepoPath {
	return m.path
}

// Surface returns the module's public variable and output names.
func (m Module) Surface() Surface {
	return m.surface
}
