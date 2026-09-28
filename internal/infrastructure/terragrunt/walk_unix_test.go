//go:build unix

package terragrunt

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"syscall"
	"testing"
	"testing/fstest"
	"time"

	"github.com/GiulioSavini/gruntled/internal/application/indexing"
	"github.com/GiulioSavini/gruntled/internal/domain/repograph"
	"github.com/GiulioSavini/gruntled/internal/infrastructure/tfsurface"
)

// mkfifoOrSkip creates a FIFO at dir/rel (making its parent directories)
// and skips the test when the platform refuses. A cleanup opens the FIFO
// read-write and closes it, which completes a reader open still blocked
// on it (Linux), so a failing test never leaks a stuck goroutine.
func mkfifoOrSkip(t *testing.T, dir, rel string) {
	t.Helper()
	p := filepath.Join(dir, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := syscall.Mkfifo(p, 0o644); err != nil {
		t.Skipf("syscall.Mkfifo unsupported: %v", err)
	}
	t.Cleanup(func() {
		if f, err := os.OpenFile(p, os.O_RDWR, 0); err == nil {
			f.Close()
		}
	})
}

// writeTree writes files (repo-relative path to content) under dir.
func writeTree(t *testing.T, dir string, files map[string]string) {
	t.Helper()
	for rel, content := range files {
		p := filepath.Join(dir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatalf("MkdirAll: %v", err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatalf("WriteFile: %v", err)
		}
	}
}

// openRootFS opens dir as an os.Root-backed fs.FS, like production.
func openRootFS(t *testing.T, dir string) fs.FS {
	t.Helper()
	root, err := os.OpenRoot(dir)
	if err != nil {
		t.Fatalf("os.OpenRoot: %v", err)
	}
	t.Cleanup(func() { root.Close() })
	return root.FS()
}

// withinHangGuard runs f in a goroutine and fails the test if it has not
// returned within 10 seconds (02-REVIEW G18: a FIFO blocks a reader open
// forever).
func withinHangGuard[T any](t *testing.T, f func() T) T {
	t.Helper()
	done := make(chan T, 1)
	go func() { done <- f() }()
	select {
	case v := <-done:
		return v
	case <-time.After(10 * time.Second):
		t.Fatal("blocked on FIFO")
		panic("unreachable")
	}
}

// TestDiscoverUnitsSkipsFIFO: a FIFO named terragrunt.hcl is not a unit.
func TestDiscoverUnitsSkipsFIFO(t *testing.T) {
	dir := t.TempDir()
	writeTree(t, dir, map[string]string{"live/vpc/terragrunt.hcl": ""})
	mkfifoOrSkip(t, dir, "live/pipe/terragrunt.hcl")
	fsys := openRootFS(t, dir)

	got := withinHangGuard(t, func() []unitEntry {
		units, err := discoverUnits(fsys)
		if err != nil {
			t.Errorf("discoverUnits: %v", err)
		}
		return units
	})
	if want := []string{"live/vpc"}; !slices.Equal(dirs(t, got), want) {
		t.Fatalf("dirs = %v, want %v", dirs(t, got), want)
	}
}

// TestDiscoverUnitsSkipsNonRegularMapFS is the platform-neutral half: any
// non-regular terragrunt.hcl (pipe, socket, device) is never a unit.
func TestDiscoverUnitsSkipsNonRegularMapFS(t *testing.T) {
	fsys := fstest.MapFS{
		"a/terragrunt.hcl":         &fstest.MapFile{},
		"pipe/terragrunt.hcl":      &fstest.MapFile{Mode: fs.ModeNamedPipe},
		"sock/terragrunt.hcl":      &fstest.MapFile{Mode: fs.ModeSocket},
		"dev/terragrunt.hcl":       &fstest.MapFile{Mode: fs.ModeDevice},
		"json/terragrunt.hcl.json": &fstest.MapFile{Mode: fs.ModeNamedPipe},
	}
	got, err := discoverUnits(fsys)
	if err != nil {
		t.Fatalf("discoverUnits: %v", err)
	}
	if want := []string{"a"}; !slices.Equal(dirs(t, got), want) {
		t.Fatalf("dirs = %v, want %v", dirs(t, got), want)
	}
}

// TestBuildWithFIFOFiles drives the whole pipeline over a real tree
// holding a FIFO in every place gruntled reads: a unit config, an include
// target and a module file. It completes, and each FIFO fails toward
// unknown.
func TestBuildWithFIFOFiles(t *testing.T) {
	dir := t.TempDir()
	writeTree(t, dir, map[string]string{
		"live/app/terragrunt.hcl": `include "root" {
  path = find_in_parent_folders("root.hcl")
}
`,
		"live/vpc/terragrunt.hcl": "",
		"live/vpc/main.tf":        "output \"id\" {\n  value = 1\n}\n",
	})
	mkfifoOrSkip(t, dir, "live/pipe/terragrunt.hcl")
	mkfifoOrSkip(t, dir, "live/root.hcl")
	mkfifoOrSkip(t, dir, "live/vpc/extra.tf")
	fsys := openRootFS(t, dir)

	type built struct {
		res indexing.Result
		err error
	}
	b := withinHangGuard(t, func() built {
		res, err := indexing.Build(context.Background(), NewLoader(fsys), tfsurface.NewReader(fsys))
		return built{res, err}
	})
	if b.err != nil {
		t.Fatalf("indexing.Build: %v", b.err)
	}
	res := b.res

	if _, ok := res.Graph.Unit(repograph.MustRepoPath("live/pipe")); ok {
		t.Fatal("live/pipe (a FIFO terragrunt.hcl) is a unit")
	}
	app, ok := res.Graph.Unit(repograph.MustRepoPath("live/app"))
	if !ok {
		t.Fatal("live/app not in graph")
	}
	if app.Status() != repograph.StatusConfigUnknown || app.UnknownReason() != ReasonIncludeNotFound {
		t.Fatalf("live/app = (%v, %q), want (config-unknown, %q)", app.Status(), app.UnknownReason(), ReasonIncludeNotFound)
	}
	mod, ok := res.Graph.ModuleOf(repograph.MustRepoPath("live/vpc"))
	if !ok {
		t.Fatal("ModuleOf(live/vpc) not found")
	}
	if _, known := mod.Surface(); known {
		t.Fatal("live/vpc surface known, want unknown")
	}
	if mod.UnknownReason() != tfsurface.ReasonModuleFileUnreadable {
		t.Fatalf("live/vpc module UnknownReason = %q, want %q", mod.UnknownReason(), tfsurface.ReasonModuleFileUnreadable)
	}
}
