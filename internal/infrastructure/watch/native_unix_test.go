//go:build !windows

package watch

import (
	"errors"
	"fmt"
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
		if rel != "." && Ignored(filepath.ToSlash(rel)) {
			t.Errorf("ignored directory %q was watched", d)
		}
	}
	if len(watched) != 2 { // root and a
		t.Errorf("watched dirs = %v, want root and a", strings.Join(watched, ", "))
	}
}
