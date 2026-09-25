package tfsurface_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/GiulioSavini/gruntled/internal/application/ports"
	"github.com/GiulioSavini/gruntled/internal/domain/repograph"
	"github.com/GiulioSavini/gruntled/internal/infrastructure/tfsurface"
)

// rootFS builds an os.Root-backed fs.FS over t.TempDir(), which is what the
// walker (Plans 04-05) will hand this reader in production: repo-relative
// paths, in-repo file symlinks followed, escapes and dangling links refused
// by os.Root.
func rootFS(t *testing.T, dir string) ports.SurfaceReader {
	t.Helper()
	root, err := os.OpenRoot(dir)
	if err != nil {
		t.Fatalf("os.OpenRoot(%q): %v", dir, err)
	}
	t.Cleanup(func() { root.Close() })
	return tfsurface.NewReader(root.FS())
}

func symlinkOrSkip(t *testing.T, oldname, newname string) {
	t.Helper()
	if err := os.Symlink(oldname, newname); err != nil {
		t.Skipf("os.Symlink unsupported on this platform: %v", err)
	}
}

// TestRealFSInRepoFileSymlinkFollowed is the primary corpus shape (research
// Pitfall 1): a module directory symlinks a file from outside itself but
// still inside the repository, and its declarations must be included.
func TestRealFSInRepoFileSymlinkFollowed(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "global.tf"), []byte(`output "g" { value = 1 }`), 0o644); err != nil {
		t.Fatalf("write global.tf: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "mod"), 0o755); err != nil {
		t.Fatalf("mkdir mod: %v", err)
	}
	symlinkOrSkip(t, "../global.tf", filepath.Join(dir, "mod", "global.tf"))

	r := rootFS(t, dir)
	res, err := r.ReadSurface(context.Background(), repograph.MustRepoPath("mod"))
	if err != nil {
		t.Fatalf("ReadSurface: unexpected error: %v", err)
	}
	if res.UnknownReason != "" {
		t.Fatalf("UnknownReason = %q, want \"\"", res.UnknownReason)
	}
	if outs := res.Surface.Outputs(); len(outs) != 1 || outs[0] != "g" {
		t.Fatalf("Outputs() = %v, want [g]", outs)
	}
}

// TestRealFSEscapingSymlinkUnreadable is a module file symlinked to an
// absolute path in a different temp dir: os.Root must refuse the escape,
// which this reader turns into module-file-unreadable.
func TestRealFSEscapingSymlinkUnreadable(t *testing.T) {
	other := t.TempDir()
	evil := filepath.Join(other, "evil.tf")
	if err := os.WriteFile(evil, []byte(`output "e" { value = 1 }`), 0o644); err != nil {
		t.Fatalf("write evil.tf: %v", err)
	}

	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "mod"), 0o755); err != nil {
		t.Fatalf("mkdir mod: %v", err)
	}
	symlinkOrSkip(t, evil, filepath.Join(dir, "mod", "evil.tf"))

	r := rootFS(t, dir)
	res, err := r.ReadSurface(context.Background(), repograph.MustRepoPath("mod"))
	if err != nil {
		t.Fatalf("ReadSurface: unexpected error: %v", err)
	}
	if res.UnknownReason != tfsurface.ReasonModuleFileUnreadable {
		t.Fatalf("UnknownReason = %q, want %q", res.UnknownReason, tfsurface.ReasonModuleFileUnreadable)
	}
}

// TestRealFSDanglingSymlinkUnreadable is a module file symlinked to a
// target that does not exist.
func TestRealFSDanglingSymlinkUnreadable(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "mod"), 0o755); err != nil {
		t.Fatalf("mkdir mod: %v", err)
	}
	symlinkOrSkip(t, "missing.tf", filepath.Join(dir, "mod", "gone.tf"))

	r := rootFS(t, dir)
	res, err := r.ReadSurface(context.Background(), repograph.MustRepoPath("mod"))
	if err != nil {
		t.Fatalf("ReadSurface: unexpected error: %v", err)
	}
	if res.UnknownReason != tfsurface.ReasonModuleFileUnreadable {
		t.Fatalf("UnknownReason = %q, want %q", res.UnknownReason, tfsurface.ReasonModuleFileUnreadable)
	}
}
