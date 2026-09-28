//go:build unix

package hclconv_test

import (
	"errors"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/GiulioSavini/gruntled/internal/infrastructure/hclconv"
)

// TestReadFileLimitedFIFO is the 02-REVIEW G18 regression: opening a FIFO
// for reading blocks until a writer appears, so before 02-12 this call
// never returned. It must now refuse the FIFO promptly.
func TestReadFileLimitedFIFO(t *testing.T) {
	dir := t.TempDir()
	fifo := filepath.Join(dir, "f.tf")
	if err := syscall.Mkfifo(fifo, 0o644); err != nil {
		t.Skipf("syscall.Mkfifo unsupported: %v", err)
	}
	// Release a goroutine still blocked in a reader open (Linux): opening
	// the FIFO read-write never blocks and completes the pending open.
	t.Cleanup(func() {
		if f, err := os.OpenFile(fifo, os.O_RDWR, 0); err == nil {
			f.Close()
		}
	})
	root, err := os.OpenRoot(dir)
	if err != nil {
		t.Fatalf("os.OpenRoot: %v", err)
	}
	t.Cleanup(func() { root.Close() })

	done := make(chan error, 1)
	go func() {
		_, err := hclconv.ReadFileLimited(root.FS(), "f.tf")
		done <- err
	}()
	select {
	case err := <-done:
		if !errors.Is(err, hclconv.ErrNotRegularFile) {
			t.Fatalf("err = %v, want ErrNotRegularFile", err)
		}
		if errors.Is(err, hclconv.ErrFileTooLarge) {
			t.Fatalf("err = %v, want not ErrFileTooLarge", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("blocked on FIFO")
	}
}
