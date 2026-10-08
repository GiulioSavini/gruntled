package statusfile_test

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/GiulioSavini/gruntled/internal/infrastructure/statusfile"
)

// realRoot returns the canonical absolute form of a fresh temp dir.
func realRoot(t *testing.T) string {
	t.Helper()
	r, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatalf("EvalSymlinks: %v", err)
	}
	abs, err := filepath.Abs(r)
	if err != nil {
		t.Fatalf("Abs: %v", err)
	}
	return abs
}

func hash12(root string) string {
	sum := sha256.Sum256([]byte(root))
	return hex.EncodeToString(sum[:])[:12]
}

type fakeEnv struct {
	goos     string
	xdg      string
	cache    string
	cacheErr error
	temp     string
}

func (f fakeEnv) env(t *testing.T) statusfile.Env {
	return statusfile.Env{
		GOOS: f.goos,
		Getenv: func(k string) string {
			if k == "XDG_RUNTIME_DIR" {
				return f.xdg
			}
			t.Errorf("unexpected Getenv(%q)", k)
			return ""
		},
		UserCacheDir: func() (string, error) { return f.cache, f.cacheErr },
		TempDir:      func() string { return f.temp },
	}
}

func TestDirBaseSelection(t *testing.T) {
	root := realRoot(t)
	xdg := t.TempDir()
	cache := t.TempDir()
	temp := t.TempDir()
	h := hash12(root)
	cases := []struct {
		name string
		env  fakeEnv
		base string
	}{
		{"linux absolute XDG", fakeEnv{goos: "linux", xdg: xdg, cache: cache, temp: temp}, xdg},
		{"linux relative XDG", fakeEnv{goos: "linux", xdg: "run/user/1000", cache: cache, temp: temp}, cache},
		{"linux empty XDG", fakeEnv{goos: "linux", cache: cache, temp: temp}, cache},
		{"darwin ignores XDG", fakeEnv{goos: "darwin", xdg: xdg, cache: cache, temp: temp}, cache},
		{"windows ignores XDG", fakeEnv{goos: "windows", xdg: xdg, cache: cache, temp: temp}, cache},
		{"cache error falls to temp", fakeEnv{goos: "darwin", cacheErr: errors.New("no home"), temp: temp}, temp},
		{"linux no XDG cache error", fakeEnv{goos: "linux", cacheErr: errors.New("no home"), temp: temp}, temp},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := statusfile.Dir(root, tc.env.env(t))
			if err != nil {
				t.Fatalf("Dir: %v", err)
			}
			want := filepath.Join(tc.base, "gruntled", h)
			if got != want {
				t.Fatalf("Dir = %q, want %q", got, want)
			}
			p, err := statusfile.Path(root, tc.env.env(t))
			if err != nil {
				t.Fatalf("Path: %v", err)
			}
			if p != filepath.Join(want, "status") {
				t.Fatalf("Path = %q, want %q", p, filepath.Join(want, "status"))
			}
		})
	}
}

func TestDirHashStable(t *testing.T) {
	cache := t.TempDir()
	env := fakeEnv{goos: "darwin", cache: cache}.env(t)
	a := realRoot(t)
	b := realRoot(t)
	da1, err := statusfile.Dir(a, env)
	if err != nil {
		t.Fatalf("Dir: %v", err)
	}
	da2, err := statusfile.Dir(a, env)
	if err != nil {
		t.Fatalf("Dir: %v", err)
	}
	db, err := statusfile.Dir(b, env)
	if err != nil {
		t.Fatalf("Dir: %v", err)
	}
	if da1 != da2 {
		t.Fatalf("not deterministic: %q vs %q", da1, da2)
	}
	if da1 == db {
		t.Fatalf("different roots share %q", da1)
	}
	if len(filepath.Base(da1)) != 12 {
		t.Fatalf("hash dir %q is not 12 chars", filepath.Base(da1))
	}
}

func TestDirSymlinkAlias(t *testing.T) {
	cache := t.TempDir()
	env := fakeEnv{goos: "darwin", cache: cache}.env(t)
	root := realRoot(t)
	link := filepath.Join(t.TempDir(), "alias")
	if err := os.Symlink(root, link); err != nil {
		t.Skipf("symlink unsupported: %v", err)
	}
	d1, err := statusfile.Dir(root, env)
	if err != nil {
		t.Fatalf("Dir(root): %v", err)
	}
	d2, err := statusfile.Dir(link, env)
	if err != nil {
		t.Fatalf("Dir(link): %v", err)
	}
	if d1 != d2 {
		t.Fatalf("alias hashes differ: %q vs %q", d1, d2)
	}
}

func TestDirMissingRoot(t *testing.T) {
	env := fakeEnv{goos: "darwin", cache: t.TempDir()}.env(t)
	if _, err := statusfile.Dir(filepath.Join(t.TempDir(), "nope"), env); err == nil {
		t.Fatal("Dir on a missing root: want error")
	}
}

func TestDirInside(t *testing.T) {
	parent := realRoot(t)
	root := filepath.Join(parent, "repo")
	sibling := filepath.Join(parent, "repo2")
	for _, d := range []string{root, sibling, filepath.Join(root, "sub")} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	cases := []struct {
		name string
		p    string
		want bool
	}{
		{"root itself", root, true},
		{"file under root", filepath.Join(root, "status"), true},
		{"not-yet-existing nested", filepath.Join(root, "sub", "x", "y", "status"), true},
		{"sibling prefix", filepath.Join(sibling, "status"), false},
		{"parent", parent, false},
		{"elsewhere", filepath.Join(t.TempDir(), "status"), false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := statusfile.Inside(root, tc.p)
			if err != nil {
				t.Fatalf("Inside: %v", err)
			}
			if got != tc.want {
				t.Fatalf("Inside(%q, %q) = %v, want %v", root, tc.p, got, tc.want)
			}
		})
	}
}

func TestDirInsideViaSymlink(t *testing.T) {
	root := realRoot(t)
	link := filepath.Join(t.TempDir(), "alias")
	if err := os.Symlink(root, link); err != nil {
		t.Skipf("symlink unsupported: %v", err)
	}
	got, err := statusfile.Inside(root, filepath.Join(link, "status"))
	if err != nil {
		t.Fatalf("Inside: %v", err)
	}
	if !got {
		t.Fatal("path reached through a symlink to root must count as inside")
	}
}
