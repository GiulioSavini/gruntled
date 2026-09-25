package synthrepo

import (
	"github.com/GiulioSavini/gruntled/internal/domain/diagnostic"
	"github.com/GiulioSavini/gruntled/internal/domain/repograph"
)

// ExpectedDiagnostic is one diagnostic a correct analyzer must report
// against a generated tree, expressed with domain types so Phase 4's
// golden tests can compare it directly against a diagnostic.Set.
type ExpectedDiagnostic struct {
	// Code is the diagnostic code the reference should produce.
	Code diagnostic.Code
	// Pos is the position of the "d" in "dependency" for this specific
	// reference, inside <Unit>/terragrunt.hcl.
	Pos repograph.Position
	// Unit is the unit directory containing the reference.
	Unit repograph.RepoPath
	// Dependency is the dependency block's label (e.g. "dep_003").
	Dependency string
	// Target is the unit directory the dependency block points at.
	Target repograph.RepoPath
	// Output is the undeclared output name the reference was mutated to
	// name.
	Output string
}

// Manifest is the exact oracle for a generated tree: every unit it
// contains, and every diagnostic a correct analyzer must report against
// it. Any reference not listed in Expected is valid.
type Manifest struct {
	// Units lists every unit directory in the tree, sorted.
	Units []repograph.RepoPath
	// Expected lists every injected diagnostic, sorted by Pos.
	Expected []ExpectedDiagnostic
}
