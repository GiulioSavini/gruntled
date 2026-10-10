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

// statFn is os.Stat; tests replace it to simulate two spellings of one
// directory on a case-sensitive host.
var statFn = os.Stat

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

// Inside reports whether p is root or lies under it. Identity, not
// spelling: case variants and aliases (bind mounts, a case-insensitive
// filesystem's other spellings) count as inside.
//
// The fast path compares both after resolving symlinks (p through its
// deepest existing ancestor, so p itself need not exist). When that says
// outside, every existing ancestor of p, p included, is compared with root
// by os.SameFile. Sibling directories sharing a prefix (/r/repo vs
// /r/repo2) are not inside. An ancestor that cannot be stat'ed is skipped.
func Inside(root, p string) (bool, error) {
	r, err := canonical(root)
	if err != nil {
		return false, err
	}
	q, err := canonicalPartial(p)
	if err != nil {
		return false, err
	}
	if insideByName(r, q) {
		return true, nil
	}
	rootFI, err := statFn(r)
	if err != nil {
		return false, err
	}
	abs, err := filepath.Abs(p)
	if err != nil {
		return false, err
	}
	for cur := abs; ; {
		if fi, err := statFn(cur); err == nil && os.SameFile(fi, rootFI) {
			return true, nil
		}
		parent := filepath.Dir(cur)
		if parent == cur {
			return false, nil
		}
		cur = parent
	}
}

// insideByName reports whether the canonical path q is r or under it,
// comparing spellings only.
func insideByName(r, q string) bool {
	rel, err := filepath.Rel(r, q)
	if err != nil {
		// Different volumes on windows: not inside.
		return false
	}
	if rel == "." {
		return true
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel)
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
