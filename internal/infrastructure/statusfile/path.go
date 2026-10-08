// Package statusfile decides where the watch daemon's one-line status file
// lives and writes it atomically.
package statusfile

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// Env is the slice of the process environment path resolution reads.
// Tests inject it; production uses OSEnv.
type Env struct {
	GOOS         string
	Getenv       func(string) string
	UserCacheDir func() (string, error)
	TempDir      func() string
}

// OSEnv returns the real environment.
func OSEnv() Env {
	return Env{
		GOOS:         runtime.GOOS,
		Getenv:       os.Getenv,
		UserCacheDir: os.UserCacheDir,
		TempDir:      os.TempDir,
	}
}

// hashLen is the number of hex characters of the root hash used as the
// per-repository directory name. Kept short: Phase 11 puts a unix socket
// beside the status file and sun_path is ~104 bytes on darwin.
const hashLen = 12

// Dir returns <base>/gruntled/<hash12> for the repository at root, where
// hash12 is the first 12 hex characters of sha256 over the absolute,
// symlink-resolved root. base is, in order: $XDG_RUNTIME_DIR on linux when
// it is absolute; os.UserCacheDir; os.TempDir.
//
// The root is not case-folded on any OS. On a case-insensitive filesystem
// two spellings of the same directory that EvalSymlinks does not
// canonicalise hash differently and get separate status directories.
//
// root must exist.
func Dir(root string, env Env) (string, error) {
	canon, err := canonical(root)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256([]byte(canon))
	return filepath.Join(base(env), "gruntled", hex.EncodeToString(sum[:])[:hashLen]), nil
}

// Path returns the status file path for root: Dir(root) + "/status".
func Path(root string, env Env) (string, error) {
	d, err := Dir(root, env)
	if err != nil {
		return "", err
	}
	return filepath.Join(d, "status"), nil
}

// Inside reports whether p is root or lies under it, comparing both after
// resolving symlinks (p through its deepest existing ancestor, so p itself
// need not exist). Sibling directories sharing a prefix (/r/repo vs
// /r/repo2) are not inside.
func Inside(root, p string) (bool, error) {
	r, err := canonical(root)
	if err != nil {
		return false, err
	}
	q, err := canonicalPartial(p)
	if err != nil {
		return false, err
	}
	rel, err := filepath.Rel(r, q)
	if err != nil {
		// Different volumes on windows: not inside.
		return false, nil
	}
	if rel == "." {
		return true, nil
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return false, nil
	}
	return true, nil
}

func base(env Env) string {
	if env.GOOS == "linux" {
		if x := env.Getenv("XDG_RUNTIME_DIR"); x != "" && filepath.IsAbs(x) {
			return x
		}
	}
	if c, err := env.UserCacheDir(); err == nil && c != "" {
		return c
	}
	return env.TempDir()
}

func canonical(p string) (string, error) {
	abs, err := filepath.Abs(p)
	if err != nil {
		return "", err
	}
	return filepath.EvalSymlinks(abs)
}

// canonicalPartial resolves symlinks in the longest existing prefix of p
// and re-appends the components that do not exist yet.
func canonicalPartial(p string) (string, error) {
	abs, err := filepath.Abs(p)
	if err != nil {
		return "", err
	}
	var rest []string
	cur := abs
	for {
		resolved, err := filepath.EvalSymlinks(cur)
		if err == nil {
			for i := len(rest) - 1; i >= 0; i-- {
				resolved = filepath.Join(resolved, rest[i])
			}
			return resolved, nil
		}
		if !errors.Is(err, os.ErrNotExist) {
			return "", err
		}
		parent := filepath.Dir(cur)
		if parent == cur {
			return abs, nil
		}
		rest = append(rest, filepath.Base(cur))
		cur = parent
	}
}
