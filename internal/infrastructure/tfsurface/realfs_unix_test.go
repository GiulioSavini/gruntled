//go:build unix

package tfsurface_test

import (
	"context"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/GiulioSavini/gruntled/internal/application/ports"
	"github.com/GiulioSavini/gruntled/internal/domain/repograph"
	"github.com/GiulioSavini/gruntled/internal/infrastructure/tfsurface"
)

// TestRealFSFIFOModuleFile is the 02-REVIEW G18 regression for module
// files: before 02-12 the reader blocked forever opening mod/pipe.tf. A
// non-regular kept file must now make the surface unknown
// module-file-unreadable, promptly.
func TestRealFSFIFOModuleFile(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "mod"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "mod", "main.tf"), []byte(`output "a" { value = 1 }`), 0o644); err != nil {
		t.Fatal(err)
	}
	fifo := filepath.Join(dir, "mod", "pipe.tf")
	if err := syscall.Mkfifo(fifo, 0o644); err != nil {
		t.Skipf("syscall.Mkfifo unsupported: %v", err)
	}
	// Complete a reader open still blocked on the FIFO (Linux).
	t.Cleanup(func() {
		if f, err := os.OpenFile(fifo, os.O_RDWR, 0); err == nil {
			f.Close()
		}
	})
	r := rootFS(t, dir)

	type result struct {
		res ports.SurfaceResult
		err error
	}
	done := make(chan result, 1)
	go func() {
		res, err := r.ReadSurface(context.Background(), repograph.MustRepoPath("mod"))
		done <- result{res, err}
	}()
	select {
	case got := <-done:
		if got.err != nil {
			t.Fatalf("ReadSurface: %v", got.err)
		}
		if got.res.UnknownReason != tfsurface.ReasonModuleFileUnreadable {
			t.Fatalf("UnknownReason = %q, want %q", got.res.UnknownReason, tfsurface.ReasonModuleFileUnreadable)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("blocked on FIFO")
	}
}
