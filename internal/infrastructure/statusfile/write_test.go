package statusfile_test

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/GiulioSavini/gruntled/internal/infrastructure/statusfile"
)

func noLeftovers(t *testing.T, dir string) {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".status-") {
			t.Fatalf("leftover temp file %q", e.Name())
		}
	}
}

func TestWriteCreatesDirAndFile(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "gruntled", "abcdef012345")
	path := filepath.Join(dir, "status")
	w := statusfile.NewWriter(path)
	line := "gruntled: ok @ 14:02:11\n"
	if err := w.Write(line); err != nil {
		t.Fatalf("Write: %v", err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(got) != line {
		t.Fatalf("content %q, want %q", got, line)
	}
	if err := w.Write("gruntled: stopped @ 14:02:12\n"); err != nil {
		t.Fatalf("second Write: %v", err)
	}
	got, _ = os.ReadFile(path)
	if string(got) != "gruntled: stopped @ 14:02:12\n" {
		t.Fatalf("overwrite content %q", got)
	}
	noLeftovers(t, dir)
	if runtime.GOOS == "windows" {
		return
	}
	for _, d := range []string{dir, filepath.Dir(dir)} {
		fi, err := os.Stat(d)
		if err != nil {
			t.Fatal(err)
		}
		if fi.Mode().Perm() != 0o700 {
			t.Fatalf("dir %q mode %o, want 0700", d, fi.Mode().Perm())
		}
	}
	fi, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode().Perm() != 0o600 {
		t.Fatalf("file mode %o, want 0600", fi.Mode().Perm())
	}
}

func TestWriteInsecureDir(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unix permission bits only")
	}
	dir := filepath.Join(t.TempDir(), "shared")
	if err := os.Mkdir(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	err := statusfile.NewWriter(filepath.Join(dir, "status")).Write("x\n")
	if !errors.Is(err, statusfile.ErrInsecureDir) {
		t.Fatalf("Write = %v, want ErrInsecureDir", err)
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 0 {
		t.Fatalf("insecure dir got %d entries, want none", len(entries))
	}
}

func TestWriteConcurrentReader(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "s")
	path := filepath.Join(dir, "status")
	lines := [2]string{"gruntled: ok @ 14:02:11\n", "gruntled: 12 errors (GRT001×7 GRT003×5) @ 14:02:12\n"}
	w := statusfile.NewWriter(path)
	windows := runtime.GOOS == "windows"

	done := make(chan struct{})
	bad := make(chan string, 1)
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-done:
				return
			default:
			}
			b, err := os.ReadFile(path)
			if err != nil {
				// Absent before the first write; windows may also deny a read
				// that races the replace. Neither is a torn line.
				if errors.Is(err, os.ErrNotExist) || windows {
					continue
				}
				select {
				case bad <- "read: " + err.Error():
				default:
				}
				return
			}
			if s := string(b); s != lines[0] && s != lines[1] {
				select {
				case bad <- "torn read: " + s:
				default:
				}
				return
			}
		}
	}()

	var writeErr error
	for i := range 200 {
		if err := w.Write(lines[i%2]); err != nil && !windows {
			writeErr = err
			break
		}
	}
	close(done)
	wg.Wait()
	if writeErr != nil {
		t.Fatalf("Write: %v", writeErr)
	}
	select {
	case msg := <-bad:
		t.Fatal(msg)
	default:
	}
	noLeftovers(t, dir)
}

// renameSeam fails the first fails calls, then delegates to os.Rename.
type renameSeam struct {
	fails, calls int
	err          error
}

func (r *renameSeam) rename(o, n string) error {
	r.calls++
	if r.calls <= r.fails {
		return r.err
	}
	return os.Rename(o, n)
}

func TestWriteRenameRetry(t *testing.T) {
	ms := time.Millisecond
	errBusy := errors.New("sharing violation")
	cases := []struct {
		name      string
		fails     int
		wantErr   bool
		wantCalls int
		wantSleep []time.Duration
	}{
		{"first try", 0, false, 1, nil},
		{"two failures", 2, false, 3, []time.Duration{10 * ms, 20 * ms}},
		{"four failures", 4, false, 5, []time.Duration{10 * ms, 20 * ms, 40 * ms, 80 * ms}},
		{"always fails", 1000, true, 5, []time.Duration{10 * ms, 20 * ms, 40 * ms, 80 * ms}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := filepath.Join(t.TempDir(), "s")
			path := filepath.Join(dir, "status")
			seam := &renameSeam{fails: tc.fails, err: errBusy}
			var slept []time.Duration
			w := statusfile.NewWriter(path)
			w.Rename = seam.rename
			w.Sleep = func(d time.Duration) { slept = append(slept, d) }

			err := w.Write("line\n")
			if tc.wantErr {
				if !errors.Is(err, errBusy) {
					t.Fatalf("Write = %v, want %v", err, errBusy)
				}
				if _, statErr := os.Stat(path); !errors.Is(statErr, os.ErrNotExist) {
					t.Fatalf("status file exists after failed write: %v", statErr)
				}
			} else {
				if err != nil {
					t.Fatalf("Write: %v", err)
				}
				if b, _ := os.ReadFile(path); string(b) != "line\n" {
					t.Fatalf("content %q", b)
				}
			}
			if seam.calls != tc.wantCalls {
				t.Fatalf("rename calls = %d, want %d", seam.calls, tc.wantCalls)
			}
			if !reflect.DeepEqual(slept, tc.wantSleep) {
				t.Fatalf("sleeps = %v, want %v", slept, tc.wantSleep)
			}
			noLeftovers(t, dir)
		})
	}
}
