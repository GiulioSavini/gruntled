//go:build !windows

package statusfile

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestCheckDirForeignOwner(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "d")
	if err := os.Mkdir(dir, 0o700); err != nil {
		t.Fatalf("Mkdir: %v", err)
	}
	if err := CheckDir(dir); err != nil {
		t.Fatalf("CheckDir own dir = %v", err)
	}
	orig := geteuid
	t.Cleanup(func() { geteuid = orig })
	geteuid = func() int { return os.Geteuid() + 1 }
	if err := CheckDir(dir); !errors.Is(err, ErrInsecureDir) {
		t.Fatalf("CheckDir foreign owner = %v, want ErrInsecureDir", err)
	}
}
