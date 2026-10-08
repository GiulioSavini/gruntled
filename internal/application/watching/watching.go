// Package watching holds the use case the watch daemon drives on every
// debounced batch of file changes: report the dirty paths to the Loader's
// parse cache, then run the same check `gruntled check` runs.
//
// An Indexer is NOT safe for concurrent use: the daemon run loop is its
// only caller (a single indexer goroutine). The Loader underneath is
// mutex-guarded, but interleaving two Index calls would still make their
// reports race each other.
package watching

import (
	"context"

	"github.com/GiulioSavini/gruntled/internal/application/checking"
	"github.com/GiulioSavini/gruntled/internal/application/ports"
)

// Indexer re-indexes the repository incrementally over a cached loader.
type Indexer struct {
	loader   ports.InvalidatingLoader
	surfaces ports.SurfaceReader
}

// NewIndexer returns an Indexer over l and s.
func NewIndexer(l ports.InvalidatingLoader, s ports.SurfaceReader) *Indexer {
	return &Indexer{loader: l, surfaces: s}
}

// Index reports dirty (repo-relative, slash-separated paths) to the
// loader's cache, or the whole tree when resync is true (watcher overflow,
// periodic revalidation: dirty is then ignored), and returns the Report
// checking.Check produces over the refreshed loader. A nil dirty batch
// without resync (the initial index) invalidates nothing. Errors are
// checking.Check's, unchanged.
func (i *Indexer) Index(ctx context.Context, dirty []string, resync bool) (checking.Report, error) {
	switch {
	case resync:
		i.loader.Invalidate(".")
	case len(dirty) > 0:
		i.loader.Invalidate(dirty...)
	}
	return checking.Check(ctx, i.loader, i.surfaces)
}
