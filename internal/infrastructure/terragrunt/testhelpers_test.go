package terragrunt

import (
	"context"
	"io/fs"
	"strconv"
	"strings"
	"sync"
	"testing"
	"testing/fstest"

	"github.com/GiulioSavini/gruntled/internal/application/indexing"
	"github.com/GiulioSavini/gruntled/internal/infrastructure/tfsurface"
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

// countingFS wraps an fs.FS and records how many times each name was
// opened or passed to ReadFile, so a parse-once test can assert every file
// was read at most once no matter how many distinct callers ask for it.
// hclconv.ReadFileLimited reads through Open (02-12), so Open is where
// reads are counted now; ReadFile is still counted in case anything reads
// that way. Stat, ReadDir, ReadLink and Lstat delegate to inner (through
// the fs package's generic helpers, or directly for the two symlink-aware
// methods, which io/fs has no generic helper for) and are not counted:
// fs.Stat, fs.ReadDir and fs.WalkDir use the StatFS/ReadDirFS methods and
// never call Open, so only file reads are counted.
type countingFS struct {
	inner fs.FS

	mu    sync.Mutex
	reads map[string]int
}

// newCountingFS returns a countingFS wrapping inner.
func newCountingFS(inner fs.FS) *countingFS {
	return &countingFS{inner: inner, reads: map[string]int{}}
}

// Open increments reads[name] and delegates to inner.
func (c *countingFS) Open(name string) (fs.File, error) {
	c.mu.Lock()
	c.reads[name]++
	c.mu.Unlock()
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

// count returns how many times name was opened or read with ReadFile.
func (c *countingFS) count(name string) int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.reads[name]
}

// build runs indexing.Build with the real Terragrunt loader and the real
// tfsurface reader, both over fsys, and fails the test on a Go-level
// error: every fixture used with this helper is expected to be a
// per-unit/per-file problem (surfaced as an unknown reason or a
// diagnostic), never a load-stage failure.
func build(t *testing.T, fsys fs.FS) indexing.Result {
	t.Helper()
	res, err := indexing.Build(context.Background(), NewLoader(fsys), tfsurface.NewReader(fsys))
	if err != nil {
		t.Fatalf("indexing.Build: %v", err)
	}
	return res
}

// dump renders a Result as a canonical, deterministic string for
// determinism assertions: two Results built from differently-ordered or
// differently-named input must render identically. Mirrors
// internal/application/indexing's own build_test.go dump, so a reader
// familiar with one recognizes the other.
func dump(r indexing.Result) string {
	var b strings.Builder

	b.WriteString("units:\n")
	for _, u := range r.Graph.Units() {
		b.WriteString("  ")
		b.WriteString(u.Path().String())
		b.WriteString(" status=")
		b.WriteString(u.Status().String())
		b.WriteString(" reason=")
		b.WriteString(strconv.Quote(u.UnknownReason()))
		if mod, ok := u.Module(); ok {
			b.WriteString(" module=")
			b.WriteString(mod.String())
		}
		b.WriteString(" deps=[")
		for i, d := range u.Dependencies() {
			if i > 0 {
				b.WriteString(",")
			}
			b.WriteString(d.Name())
			b.WriteString("=")
			if target, ok := d.Target(); ok {
				b.WriteString(target.String())
			} else {
				b.WriteString("unresolved:" + d.UnresolvedReason())
			}
		}
		b.WriteString("] refs=[")
		for i, ref := range u.References() {
			if i > 0 {
				b.WriteString(",")
			}
			b.WriteString(ref.Dependency())
			b.WriteString(".")
			b.WriteString(ref.Output())
			b.WriteString("@")
			b.WriteString(ref.Pos().String())
		}
		b.WriteString("]\n")
	}

	b.WriteString("modules:\n")
	for _, m := range r.Graph.Modules() {
		b.WriteString("  ")
		b.WriteString(m.Path().String())
		if surf, ok := m.Surface(); ok {
			b.WriteString(" known outputs=")
			b.WriteString(strings.Join(surf.Outputs(), ","))
		} else {
			b.WriteString(" unknown reason=")
			b.WriteString(strconv.Quote(m.UnknownReason()))
		}
		b.WriteString("\n")
	}

	b.WriteString("diagnostics:\n")
	for _, d := range r.Diagnostics.All() {
		k := d.Key()
		b.WriteString("  ")
		b.WriteString(string(k.Code))
		b.WriteString(" unit=")
		b.WriteString(k.Unit.String())
		b.WriteString(" ")
		b.WriteString(k.File)
		b.WriteString(":")
		b.WriteString(strconv.Itoa(k.Line))
		b.WriteString(":")
		b.WriteString(strconv.Itoa(k.Column))
		b.WriteString(" ")
		b.WriteString(k.Message)
		b.WriteString("\n")
	}

	return b.String()
}
