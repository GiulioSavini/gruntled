//go:build !windows

package watch

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/fsnotify/fsnotify"
)

func TestWatcherContractNative(t *testing.T) {
	runContract(t, NewNative)
}

func newTestNative(t *testing.T, root string, safety time.Duration) *native {
	t.Helper()
	w, err := newNative(root, safety)
	if err != nil {
		t.Fatalf("newNative: %v", err)
	}
	t.Cleanup(func() { _ = w.Close() })
	return w
}

func TestNativeOverflowResync(t *testing.T) {
	n := newTestNative(t, t.TempDir(), DefaultSafetyNet)
	n.handleErr(fsnotify.ErrEventOverflow)
	if got := n.Take(); !got.Resync {
		t.Fatalf("overflow did not set Resync: %+v", got)
	}
	n.handleErr(errors.New("some other watcher error"))
	if got := n.Take(); !got.Resync {
		t.Fatalf("generic error did not set Resync: %+v", got)
	}
}

func TestNativeWatchLimit(t *testing.T) {
	orig := addWatch
	t.Cleanup(func() { addWatch = orig })
	for _, errno := range []syscall.Errno{syscall.ENOSPC, syscall.EMFILE, syscall.ENFILE} {
		addWatch = func(*fsnotify.Watcher, string) error {
			return fmt.Errorf("inotify_add_watch: %w", errno)
		}
		w, err := NewNative(t.TempDir())
		if !errors.Is(err, ErrWatchLimit) {
			if w != nil {
				_ = w.Close()
			}
			t.Fatalf("%v: want ErrWatchLimit, got %v", errno, err)
		}
		if w != nil {
			t.Fatalf("%v: watcher returned alongside ErrWatchLimit", errno)
		}
	}
}

func TestNativeSafetyNet(t *testing.T) {
	orig := dropEvent
	t.Cleanup(func() { dropEvent = orig }) // runs after Close (LIFO)
	root := t.TempDir()
	writeFile(t, root, "f.hcl", "x")
	dropEvent = func(rel string) bool { return rel == "f.hcl" }
	n := newTestNative(t, root, 20*time.Millisecond)
	c := newCollector(t, n)
	writeFile(t, root, "f.hcl", "xyz-longer")
	collect(t, c, func(s map[string]bool) bool { return s["f.hcl"] }, "f.hcl via safety scan")
}

func TestNativeIgnoredDirNotWatched(t *testing.T) {
	orig := addWatch
	t.Cleanup(func() { addWatch = orig })
	var mu sync.Mutex
	var watched []string
	addWatch = func(fw *fsnotify.Watcher, dir string) error {
		mu.Lock()
		watched = append(watched, dir)
		mu.Unlock()
		return fw.Add(dir)
	}
	root := t.TempDir()
	writeFile(t, root, ".git/objects/x", "x")
	writeFile(t, root, "a/.terraform/p", "p")
	writeFile(t, root, "a/.terragrunt-cache/z/w.hcl", "w")
	n := newTestNative(t, root, DefaultSafetyNet)
	c := newCollector(t, n)
	writeFile(t, root, ".git/objects/y", "y")
	writeFile(t, root, ".git/HEAD", "h")
	writeFile(t, root, "a/.terraform/q", "q")
	writeFile(t, root, "sentinel.hcl", "s")
	collect(t, c, func(s map[string]bool) bool { return s["sentinel.hcl"] }, "sentinel")
	for p := range c.seen {
		if p != "sentinel.hcl" {
			t.Errorf("unexpected path %q (ignored content leaked): %s", p, c)
		}
	}
	mu.Lock()
	defer mu.Unlock()
	for _, d := range watched {
		rel, _ := filepath.Rel(root, d)
		if rel != "." && IgnoredEntry(filepath.ToSlash(rel), fs.ModeDir) {
			t.Errorf("ignored directory %q was watched", d)
		}
	}
	if len(watched) != 2 { // root and a
		t.Errorf("watched dirs = %v, want root and a", strings.Join(watched, ", "))
	}
}

// recordWatches replaces the addWatch seam with one that records every
// directory passed to it (then installs the real watch).
func recordWatches(t *testing.T) func() []string {
	t.Helper()
	orig := addWatch
	t.Cleanup(func() { addWatch = orig })
	var mu sync.Mutex
	var watched []string
	addWatch = func(fw *fsnotify.Watcher, dir string) error {
		mu.Lock()
		watched = append(watched, dir)
		mu.Unlock()
		return fw.Add(dir)
	}
	return func() []string {
		mu.Lock()
		defer mu.Unlock()
		return append([]string(nil), watched...)
	}
}

func TestNativePatternDirWatched(t *testing.T) {
	watched := recordWatches(t)
	root := t.TempDir()
	writeFile(t, root, "live/2024/x.hcl", "x")
	writeFile(t, root, "x.tmp/y.hcl", "y")
	newTestNative(t, root, DefaultSafetyNet)
	got := map[string]bool{}
	for _, d := range watched() {
		rel, _ := filepath.Rel(root, d)
		got[filepath.ToSlash(rel)] = true
	}
	for _, want := range []string{".", "live", "live/2024", "x.tmp"} {
		if !got[want] {
			t.Errorf("%q not watched; watched = %v", want, got)
		}
	}
}

func TestNativePatternDirSafetyNet(t *testing.T) {
	orig := dropEvent
	t.Cleanup(func() { dropEvent = orig }) // runs after Close (LIFO)
	root := t.TempDir()
	writeFile(t, root, "live/2024/x.hcl", "x")
	dropEvent = func(rel string) bool { return rel == "live/2024" || strings.HasPrefix(rel, "live/2024/") }
	n := newTestNative(t, root, 20*time.Millisecond)
	c := newCollector(t, n)
	writeFile(t, root, "live/2024/x.hcl", "xyz-longer")
	collect(t, c, func(s map[string]bool) bool { return s["live/2024/x.hcl"] }, "live/2024/x.hcl via safety scan")
}

// TestNativePatternDirReplacedByFile drives handleEvent directly, with no
// loop goroutine: a watched pattern-named dir moved out of the repository
// and replaced by a same-named regular file before its event is handled is
// still reported as a directory (sec #37), and its keys are pruned.
func TestNativePatternDirReplacedByFile(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "2024/4913/f.hcl", "x")
	fw, err := fsnotify.NewWatcher()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = fw.Close() })
	n := &native{pending: newPending(), root: root, fw: fw, patternDirs: map[string]struct{}{}}
	if err := n.addTree(root, false); err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{"2024", "2024/4913"} {
		if _, ok := n.patternDirs[k]; !ok {
			t.Fatalf("patternDirs = %v, want %q recorded", n.patternDirs, k)
		}
	}
	if err := os.Rename(abs(root, "2024"), abs(t.TempDir(), "moved")); err != nil {
		t.Fatal(err)
	}
	writeFile(t, root, "2024", "now a regular file")
	n.handleEvent(fsnotify.Event{Name: abs(root, "2024"), Op: fsnotify.Rename})
	if got := n.Take(); len(got.Paths) != 1 || got.Paths[0] != "2024" || got.Resync {
		t.Fatalf("after Rename: Take() = %+v, want [2024]", got)
	}
	for k := range n.patternDirs {
		if k == "2024" || strings.HasPrefix(k, "2024/") {
			t.Errorf("patternDirs still holds %q after Rename", k)
		}
	}
	n.handleEvent(fsnotify.Event{Name: abs(root, "2024"), Op: fsnotify.Create})
	if got := n.Take(); len(got.Paths) != 0 || got.Resync {
		t.Fatalf("Create of regular file 2024 (vim probe): Take() = %+v, want empty", got)
	}
}

// TestNativeAddTreeSkipsSwappedDir: a directory swapped for a symlink
// between WalkDir listing it and addWatch is never watched (10-sec#5).
func TestNativeAddTreeSkipsSwappedDir(t *testing.T) {
	watched := recordWatches(t)
	root := t.TempDir()
	writeFile(t, root, "real/x.hcl", "x")
	swapped := abs(root, "real")
	origLstat := lstatBeforeWatch
	t.Cleanup(func() { lstatBeforeWatch = origLstat }) // runs after Close (LIFO)
	lstatBeforeWatch = func(p string) (fs.FileInfo, error) {
		fi, err := os.Lstat(p)
		if err != nil || p != swapped {
			return fi, err
		}
		return symlinkInfo{fi}, nil
	}
	newTestNative(t, root, DefaultSafetyNet)
	for _, d := range watched() {
		if d == swapped {
			t.Fatalf("swapped directory %q was watched; watched = %v", d, watched())
		}
	}
}

// symlinkInfo reports a symlink mode for an otherwise real FileInfo.
type symlinkInfo struct{ fs.FileInfo }

func (symlinkInfo) Mode() fs.FileMode { return fs.ModeSymlink | 0o777 }
func (symlinkInfo) IsDir() bool       { return false }
