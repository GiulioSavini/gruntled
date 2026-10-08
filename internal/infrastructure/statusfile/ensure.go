package statusfile

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
)

// CheckDir verifies that dir is safe to hold the lock, socket and status
// file: a real directory (not a symlink), not accessible by group or others
// (not checked on windows) and, on unix, owned by the effective uid.
// An absent dir returns an error satisfying errors.Is(err, fs.ErrNotExist)
// and nothing is created.
func CheckDir(dir string) error {
	fi, err := os.Lstat(dir)
	if err != nil {
		return err
	}
	if fi.Mode()&fs.ModeSymlink != 0 {
		return fmt.Errorf("%w: %s: is a symlink", ErrInsecureDir, dir)
	}
	if !fi.IsDir() {
		return fmt.Errorf("%w: %s: not a directory", ErrInsecureDir, dir)
	}
	if runtime.GOOS != "windows" && fi.Mode().Perm()&0o077 != 0 {
		return fmt.Errorf("%w: %s: mode %v", ErrInsecureDir, dir, fi.Mode().Perm())
	}
	return checkOwner(dir, fi)
}

// EnsureDir creates dir with mode 0700 when missing, then applies CheckDir.
// Calling it on an already good directory is a no-op.
func EnsureDir(dir string) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	return CheckDir(dir)
}

// EnsureRepoDir returns Dir(root, env) after ensuring both <base>/gruntled
// and <base>/gruntled/<hash12> pass EnsureDir. Checking the parent stops a
// pre-created foreign <base>/gruntled from swapping the repo directory.
// Only the default layout goes through here; a custom --status-file
// directory is checked by Writer alone.
func EnsureRepoDir(root string, env Env) (string, error) {
	d, err := Dir(root, env)
	if err != nil {
		return "", err
	}
	if err := EnsureDir(filepath.Dir(d)); err != nil {
		return "", err
	}
	if err := EnsureDir(d); err != nil {
		return "", err
	}
	return d, nil
}
