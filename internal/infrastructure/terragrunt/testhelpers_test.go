package terragrunt

import (
	"testing/fstest"

	"github.com/GiulioSavini/gruntled/internal/testsupport/synthrepo"
)

// mapFS converts a synthrepo.Tree into an fstest.MapFS, so tests can
// exercise the generator's synthetic repositories without touching a real
// filesystem.
func mapFS(tree synthrepo.Tree) fstest.MapFS {
	m := fstest.MapFS{}
	for _, f := range tree {
		m[f.Path] = &fstest.MapFile{Data: f.Content}
	}
	return m
}
