// Package ports declares the driven ports of the indexing use case. It
// depends only on the domain; adapters in internal/infrastructure implement
// it.
package ports

import (
	"context"

	"github.com/GiulioSavini/gruntled/internal/domain/diagnostic"
	"github.com/GiulioSavini/gruntled/internal/domain/repograph"
)

// UnitLoader discovers every unit in the repository and returns its
// structural configuration.
//
// Per-file problems (syntax errors, dynamic paths, remote sources) are NOT
// errors: they are reported through UnknownReason fields on UnitConfig and
// through Diagnostics. error is reserved for context cancellation and
// failures that make the whole load meaningless, such as the repository
// root being unreadable.
type UnitLoader interface {
	LoadUnits(ctx context.Context) (LoadResult, error)
}

// InvalidatingLoader is a UnitLoader that keeps a parse cache which the caller
// must keep honest. Paths are repo-relative, slash-separated; a rename reports
// old and new path; "." (or an empty/absolute/escaping path) means everything.
type InvalidatingLoader interface {
	UnitLoader
	Invalidate(paths ...string)
}

// LoadResult is the outcome of loading every unit in the repository.
type LoadResult struct {
	// Units is every discovered unit, sorted by Path.
	Units []UnitConfig
	// Diagnostics holds file-level findings that are not attributable to a
	// single unit, such as GRT100 for a broken terragrunt.hcl or include
	// file (zero Unit).
	Diagnostics []diagnostic.Diagnostic
}

// UnitConfig is one discovered unit's structural configuration, as read
// from its terragrunt.hcl and any merged include files, before the graph is
// assembled.
//
// All paths are repo-relative: RepoPath already guarantees that. Exactly
// one of the following three states applies to a UnitConfig, checked in
// this order:
//
//   - ConfigUnknownReason != "": the unit is config-unknown. Its own
//     terragrunt.hcl, or a merged include, is broken or uses a dynamic path
//     this domain does not model. Every other field is ignored.
//   - ModuleUnknownReason != "": the unit's config is known but its module
//     could not be determined (a remote or dynamic source, for example).
//     Module is ignored; Dependencies and References are kept, because
//     GRT001 checks the TARGET unit's module, not the referencing unit's.
//   - otherwise: the unit is resolved. Module is the unit's local module
//     directory, which equals Path when the terraform.source attribute is
//     absent.
type UnitConfig struct {
	Path                repograph.RepoPath
	ConfigUnknownReason string
	ModuleUnknownReason string
	Module              repograph.RepoPath
	// Dependencies is already include-merged with unique names: the domain
	// constructors reject duplicate dependency names, so the adapter must
	// merge dependency blocks from includes into the unit before returning
	// them here. May contain unresolved dependencies.
	Dependencies []repograph.Dependency
	// References is drawn from the unit's entire effective body, including
	// merged include files.
	References []repograph.Reference
	// PathDependencies is every entry of the unit's `dependencies { paths }`
	// blocks, already include-merged (union, de-duplicated by resolved
	// target). May contain unresolved entries. Empty for a config-unknown
	// unit.
	PathDependencies []repograph.PathDependency
}

// SurfaceReader reads a local module directory's variable and output
// names.
type SurfaceReader interface {
	ReadSurface(ctx context.Context, module repograph.RepoPath) (SurfaceResult, error)
}

// SurfaceResult is the outcome of reading one module directory's surface.
type SurfaceResult struct {
	// Surface is valid only when UnknownReason == "".
	Surface repograph.Surface
	// UnknownReason is non-empty when the surface could not be read: the
	// directory is missing, contains no .tf files, or fails to parse.
	UnknownReason string
	// Diagnostics holds file-level findings from broken module files, such
	// as GRT100.
	Diagnostics []diagnostic.Diagnostic
}
