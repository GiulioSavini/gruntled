package terragrunt

import (
	"io/fs"
	"path"
)

// unitEntry is one unit directory discoverUnits found: a directory
// containing a regular file named terragrunt.hcl and/or terragrunt.hcl.json.
type unitEntry struct {
	// dir is the repo-relative unit directory, "." for a unit at the repo
	// root.
	dir string
	// jsonConfig is true when the directory holds terragrunt.hcl.json.
	// Terragrunt's own DefaultTerragruntConfigPaths lists the JSON variant
	// first, so a directory holding both files is one unit that prefers the
	// JSON config; the loader (Plan 05) marks such a unit
	// json-config-unsupported.
	jsonConfig bool
}

// skipDirNames are directory names discoverUnits never descends into, per
// PARSE-06 and Terragrunt's own util.SkipDirIfIgnorable set, plus "vendor"
// for vendored module directories (research Open Question 3). A module
// directory inside one of these is still readable when a unit's source
// attribute points there: skipping a directory here can only drop candidate
// units, it can never fabricate or suppress a diagnostic.
var skipDirNames = map[string]bool{
	".git":              true,
	".terraform":        true,
	".terragrunt-cache": true,
	"vendor":            true,
}

// discoverUnits walks fsys and returns one unitEntry per directory that
// directly contains a regular file named terragrunt.hcl or
// terragrunt.hcl.json. Order matches fs.WalkDir's lexical order; the caller
// sorts.
//
// discoverUnits never descends into .git, .terraform, .terragrunt-cache or
// vendor. It never follows, and never counts, a symlinked directory or a
// symlinked terragrunt.hcl/terragrunt.hcl.json: fs.WalkDir reports any
// symlink as a non-directory DirEntry and never recurses into it on its
// own, so the fs.ModeSymlink guard below is what keeps a symlinked
// terragrunt.hcl from being counted as a unit file.
//
// Nor does it count a terragrunt.hcl or terragrunt.hcl.json that is not a
// regular file (a FIFO, socket or device, 02-REVIEW G18): opening a FIFO
// for reading blocks until a writer appears. Skipping one is safe: a
// dependency on that directory then targets a path that is not a unit, so
// RepositoryGraph.DependencyTarget finds no target and no reference into
// it is ever checked (03-01 row 4b gives it no diagnostic either).
//
// .terragrunt-stack IS walked on purpose: PROJECT.md is authoritative over
// research/PITFALLS.md §6 here, Terragrunt's own unit discovery includes
// it, and once rendered its generated units are ordinary terragrunt.hcl
// files that gruntled's walk finds like any other.
//
// A WalkDir error reading the root "." is returned as-is. An error reading
// any other directory skips that directory (fs.SkipDir) instead of
// aborting the whole walk: skipping a directory can only drop units it
// would have found, it can never manufacture a diagnostic gruntled has no
// position to attach.
func discoverUnits(fsys fs.FS) ([]unitEntry, error) {
	entries := map[string]*unitEntry{}
	var order []string

	err := fs.WalkDir(fsys, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			if p == "." {
				return err
			}
			return fs.SkipDir
		}
		if d.Type()&fs.ModeSymlink != 0 {
			// Never descend into, or count, a symlinked directory or file
			// (PARSE-06). WalkDir would not recurse into it anyway, since
			// d.IsDir() is false for a symlink DirEntry, but a symlinked
			// terragrunt.hcl must not match the name switch below either.
			return nil
		}
		if d.IsDir() {
			if p != "." && skipDirNames[d.Name()] {
				return fs.SkipDir
			}
			return nil
		}
		if !d.Type().IsRegular() {
			// A FIFO, socket or device named terragrunt.hcl is never a
			// unit (02-REVIEW G18): reading a FIFO blocks forever.
			return nil
		}

		var isJSON bool
		switch d.Name() {
		case "terragrunt.hcl":
			isJSON = false
		case "terragrunt.hcl.json":
			isJSON = true
		default:
			return nil
		}

		dir := path.Dir(p)
		e, ok := entries[dir]
		if !ok {
			e = &unitEntry{dir: dir}
			entries[dir] = e
			order = append(order, dir)
		}
		if isJSON {
			e.jsonConfig = true
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	units := make([]unitEntry, len(order))
	for i, dir := range order {
		units[i] = *entries[dir]
	}
	return units, nil
}
