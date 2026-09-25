package terragrunt

import (
	"io/fs"
	"sync"
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

// filesFS builds an fstest.MapFS from a map of repo-relative path to file
// content, for tests that write out a small hand-written fixture tree
// inline rather than through the synthrepo generator.
func filesFS(files map[string]string) fstest.MapFS {
	m := fstest.MapFS{}
	for p, content := range files {
		m[p] = &fstest.MapFile{Data: []byte(content)}
	}
	return m
}

// countingFS wraps an fs.FS and records how many times ReadFile was called
// for each name, so a parse-once test can assert every file was read at
// most once no matter how many distinct callers ask for it. Stat, ReadDir,
// ReadLink and Lstat delegate to inner (through the fs package's generic
// helpers, or directly for the two symlink-aware methods, which io/fs has
// no generic helper for); Open delegates directly.
type countingFS struct {
	inner fs.FS

	mu    sync.Mutex
	reads map[string]int
}

// newCountingFS returns a countingFS wrapping inner.
func newCountingFS(inner fs.FS) *countingFS {
	return &countingFS{inner: inner, reads: map[string]int{}}
}

// Open delegates to inner.
func (c *countingFS) Open(name string) (fs.File, error) {
	return c.inner.Open(name)
}

// ReadFile increments reads[name] and delegates to inner.
func (c *countingFS) ReadFile(name string) ([]byte, error) {
	c.mu.Lock()
	c.reads[name]++
	c.mu.Unlock()
	return fs.ReadFile(c.inner, name)
}

// Stat delegates to inner.
func (c *countingFS) Stat(name string) (fs.FileInfo, error) {
	return fs.Stat(c.inner, name)
}

// ReadDir delegates to inner.
func (c *countingFS) ReadDir(name string) ([]fs.DirEntry, error) {
	return fs.ReadDir(c.inner, name)
}

// ReadLink delegates to inner when inner implements fs.ReadLinkFS.
func (c *countingFS) ReadLink(name string) (string, error) {
	rl, ok := c.inner.(fs.ReadLinkFS)
	if !ok {
		return "", &fs.PathError{Op: "readlink", Path: name, Err: fs.ErrInvalid}
	}
	return rl.ReadLink(name)
}

// Lstat delegates to inner when inner implements fs.ReadLinkFS.
func (c *countingFS) Lstat(name string) (fs.FileInfo, error) {
	rl, ok := c.inner.(fs.ReadLinkFS)
	if !ok {
		return nil, &fs.PathError{Op: "lstat", Path: name, Err: fs.ErrInvalid}
	}
	return rl.Lstat(name)
}

// count returns how many times ReadFile was called for name.
func (c *countingFS) count(name string) int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.reads[name]
}
