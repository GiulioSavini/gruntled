package statusfile_test

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/GiulioSavini/gruntled/internal/infrastructure/statusfile"
)

func mkdirMode(t *testing.T, dir string, mode os.FileMode) {
	t.Helper()
	if err := os.Mkdir(dir, mode); err != nil {
		t.Fatalf("Mkdir: %v", err)
	}
	// Chmod: Mkdir is subject to umask.
	if err := os.Chmod(dir, mode); err != nil {
		t.Fatalf("Chmod: %v", err)
	}
}

func TestCheckDir(t *testing.T) {
	tests := []struct {
		name     string
		setup    func(t *testing.T, base string) string
		unixOnly bool
		wantErr  func(error) bool
	}{
		{
			name:    "absent",
			setup:   func(t *testing.T, base string) string { return filepath.Join(base, "nope") },
			wantErr: func(err error) bool { return errors.Is(err, fs.ErrNotExist) },
		},
		{
			name: "symlink to good dir",
			setup: func(t *testing.T, base string) string {
				good := filepath.Join(base, "good")
				mkdirMode(t, good, 0o700)
				link := filepath.Join(base, "link")
				if err := os.Symlink(good, link); err != nil {
					t.Skipf("symlink unsupported: %v", err)
				}
				return link
			},
			wantErr: func(err error) bool { return errors.Is(err, statusfile.ErrInsecureDir) },
		},
		{
			name: "regular file",
			setup: func(t *testing.T, base string) string {
				p := filepath.Join(base, "file")
				if err := os.WriteFile(p, nil, 0o600); err != nil {
					t.Fatalf("WriteFile: %v", err)
				}
				return p
			},
			wantErr: func(err error) bool { return err != nil },
		},
		{
			name: "0755",
			setup: func(t *testing.T, base string) string {
				d := filepath.Join(base, "open")
				mkdirMode(t, d, 0o755)
				return d
			},
			unixOnly: true,
			wantErr:  func(err error) bool { return errors.Is(err, statusfile.ErrInsecureDir) },
		},
		{
			name: "0700 good",
			setup: func(t *testing.T, base string) string {
				d := filepath.Join(base, "mine")
				mkdirMode(t, d, 0o700)
				return d
			},
			wantErr: func(err error) bool { return err == nil },
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if tc.unixOnly && runtime.GOOS == "windows" {
				t.Skip("permission bits not checked on windows")
			}
			base := t.TempDir()
			dir := tc.setup(t, base)
			err := statusfile.CheckDir(dir)
			if !tc.wantErr(err) {
				t.Fatalf("CheckDir(%s) = %v", tc.name, err)
			}
		})
	}
}

func TestCheckDirAbsentCreatesNothing(t *testing.T) {
	base := t.TempDir()
	dir := filepath.Join(base, "a", "b")
	if err := statusfile.CheckDir(dir); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("CheckDir = %v, want ErrNotExist", err)
	}
	if _, err := os.Lstat(filepath.Join(base, "a")); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("CheckDir created something: %v", err)
	}
}

func TestEnsureDirIdempotent(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "gruntled", "abcdef012345")
	for i := range 2 {
		if err := statusfile.EnsureDir(dir); err != nil {
			t.Fatalf("EnsureDir #%d = %v", i, err)
		}
	}
	fi, err := os.Lstat(dir)
	if err != nil || !fi.IsDir() {
		t.Fatalf("Lstat = %v, %v", fi, err)
	}
	if runtime.GOOS != "windows" && fi.Mode().Perm() != 0o700 {
		t.Fatalf("mode = %v, want 0700", fi.Mode().Perm())
	}
}

func TestEnsureDirRefusesSymlink(t *testing.T) {
	base := t.TempDir()
	good := filepath.Join(base, "good")
	mkdirMode(t, good, 0o700)
	link := filepath.Join(base, "link")
	if err := os.Symlink(good, link); err != nil {
		t.Skipf("symlink unsupported: %v", err)
	}
	if err := statusfile.EnsureDir(link); !errors.Is(err, statusfile.ErrInsecureDir) {
		t.Fatalf("EnsureDir = %v, want ErrInsecureDir", err)
	}
}

func TestEnsureRepoDir(t *testing.T) {
	root := realRoot(t)
	cache := t.TempDir()
	env := fakeEnv{goos: "darwin", cache: cache}.env(t)
	want, err := statusfile.Dir(root, env)
	if err != nil {
		t.Fatalf("Dir: %v", err)
	}
	got, err := statusfile.EnsureRepoDir(root, env)
	if err != nil {
		t.Fatalf("EnsureRepoDir = %v", err)
	}
	if got != want {
		t.Fatalf("EnsureRepoDir = %q, want %q", got, want)
	}
	for _, d := range []string{filepath.Dir(got), got} {
		if err := statusfile.CheckDir(d); err != nil {
			t.Fatalf("CheckDir(%s) = %v", d, err)
		}
	}
}

func TestEnsureRepoDirRefusesForeignParent(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("permission bits not checked on windows")
	}
	root := realRoot(t)
	cache := t.TempDir()
	mkdirMode(t, filepath.Join(cache, "gruntled"), 0o755)
	env := fakeEnv{goos: "darwin", cache: cache}.env(t)
	if _, err := statusfile.EnsureRepoDir(root, env); !errors.Is(err, statusfile.ErrInsecureDir) {
		t.Fatalf("EnsureRepoDir = %v, want ErrInsecureDir", err)
	}
	entries, err := os.ReadDir(filepath.Join(cache, "gruntled"))
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("repo dir created under foreign parent: %v", entries)
	}
}

func TestEnsureRepoDirRefusesSymlinkedParent(t *testing.T) {
	root := realRoot(t)
	cache := t.TempDir()
	elsewhere := filepath.Join(t.TempDir(), "elsewhere")
	mkdirMode(t, elsewhere, 0o700)
	if err := os.Symlink(elsewhere, filepath.Join(cache, "gruntled")); err != nil {
		t.Skipf("symlink unsupported: %v", err)
	}
	env := fakeEnv{goos: "darwin", cache: cache}.env(t)
	if _, err := statusfile.EnsureRepoDir(root, env); !errors.Is(err, statusfile.ErrInsecureDir) {
		t.Fatalf("EnsureRepoDir = %v, want ErrInsecureDir", err)
	}
}

func TestWriteRefusesSymlinkedDir(t *testing.T) {
	base := t.TempDir()
	good := filepath.Join(base, "good")
	mkdirMode(t, good, 0o700)
	link := filepath.Join(base, "link")
	if err := os.Symlink(good, link); err != nil {
		t.Skipf("symlink unsupported: %v", err)
	}
	err := statusfile.NewWriter(filepath.Join(link, "status")).Write("x\n")
	if !errors.Is(err, statusfile.ErrInsecureDir) {
		t.Fatalf("Write = %v, want ErrInsecureDir", err)
	}
	entries, err := os.ReadDir(good)
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("Write left files in symlink target: %v", entries)
	}
}
