//go:build !windows

package watch

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/fsnotify/fsnotify"
)

// DefaultSafetyNet is the period of the native adapter's stat re-scan. A
// lost kernel event is bounded to this much staleness.
const DefaultSafetyNet = 30 * time.Second

// addWatch installs one directory watch. Test seam: production never
// reassigns it.
var addWatch = func(fw *fsnotify.Watcher, dir string) error { return fw.Add(dir) }

// dropEvent, when non-nil, makes handleEvent discard events for matching
// repo-relative paths. Test seam for the safety net; nil in production.
var dropEvent func(rel string) bool

// native is the fsnotify-backed Watcher (inotify on linux, kqueue on
// darwin/BSD). Events are hints only: every event on a non-ignored path
// marks it dirty, regardless of Op.
type native struct {
	*pending
	root   string
	fw     *fsnotify.Watcher
	safety time.Duration
	done   chan struct{}
	wg     sync.WaitGroup
	once   sync.Once
}

// NewNative returns a Watcher backed by the OS notification API. root must
// be absolute and symlink-resolved (the CLI guarantees it): reported paths
// are made relative to it, and symlinked directories below it are not
// followed. When the OS watch limit is exhausted it returns ErrWatchLimit
// and the caller falls back to NewPoll.
func NewNative(root string) (Watcher, error) {
	w, err := newNative(root, DefaultSafetyNet)
	if err != nil {
		return nil, err
	}
	return w, nil
}

func newNative(root string, safety time.Duration) (*native, error) {
	if safety <= 0 {
		safety = DefaultSafetyNet
	}
	fi, err := os.Stat(root)
	if err != nil {
		return nil, err
	}
	if !fi.IsDir() {
		return nil, errors.New("watch: root is not a directory: " + root)
	}
	fw, err := fsnotify.NewWatcher()
	if err != nil {
		if isLimit(err) {
			return nil, fmt.Errorf("%w: %v", ErrWatchLimit, err)
		}
		return nil, err
	}
	n := &native{pending: newPending(), root: root, fw: fw, safety: safety, done: make(chan struct{})}
	if err := n.addTree(root, false); err != nil {
		_ = fw.Close()
		return nil, err
	}
	// Baseline AFTER the watches exist: a change in between is seen by
	// fsnotify, by the scanner, or by both; never by neither.
	s := newScanner(root)
	n.wg.Add(1)
	go n.loop(s)
	return n, nil
}

// isLimit reports whether err means the OS ran out of watches or
// descriptors.
func isLimit(err error) bool {
	return errors.Is(err, syscall.ENOSPC) || errors.Is(err, syscall.EMFILE) || errors.Is(err, syscall.ENFILE)
}

// addTree watches dir and every non-ignored directory below it without
// following symlinks. With emit, every path found is also marked dirty, so
// files created before the new directory's watch existed are not lost.
// It returns an ErrWatchLimit-wrapped error on watch exhaustion, the error
// when dir itself cannot be walked or watched, and ignores subdirectories
// that vanish or cannot be read mid-walk.
func (n *native) addTree(dir string, emit bool) error {
	return filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			if p == dir {
				return err
			}
			if d != nil && d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		rel := n.rel(p)
		if p != dir && (Ignored(rel) || (d.IsDir() && IgnoredDir(d.Name()))) {
			if d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		if emit && p != dir {
			n.add(rel)
		}
		if !d.IsDir() {
			return nil
		}
		if err := addWatch(n.fw, p); err != nil {
			if isLimit(err) {
				return fmt.Errorf("%w: %v", ErrWatchLimit, err)
			}
			if p == dir {
				return err
			}
			return fs.SkipDir // vanished or unreadable; the safety net covers it
		}
		return nil
	})
}

// rel maps an absolute event path to the repo-relative slash form; paths
// outside root come back as ".." so pending.add turns them into resync.
func (n *native) rel(p string) string {
	r, err := filepath.Rel(n.root, p)
	if err != nil {
		return ".."
	}
	return filepath.ToSlash(r)
}

func (n *native) loop(s *scanner) {
	defer n.wg.Done()
	t := time.NewTicker(n.safety)
	defer t.Stop()
	for {
		select {
		case <-n.done:
			return
		case ev, ok := <-n.fw.Events:
			if !ok {
				return
			}
			n.handleEvent(ev)
		case err, ok := <-n.fw.Errors:
			if !ok {
				return
			}
			n.handleErr(err)
		case <-t.C:
			for _, rel := range s.diff() {
				n.add(rel)
			}
			if s.resync {
				n.markResync()
			}
		}
	}
}

// handleEvent marks the event path dirty and, for a created (or renamed-in)
// directory, starts watching its tree.
func (n *native) handleEvent(ev fsnotify.Event) {
	rel := n.rel(ev.Name)
	if dropEvent != nil && dropEvent(rel) {
		return
	}
	if Ignored(rel) {
		return
	}
	n.add(rel) // out-of-contract rel (".", "..") becomes resync here
	if !ev.Has(fsnotify.Create) {
		return
	}
	fi, err := os.Lstat(ev.Name)
	if err != nil || !fi.IsDir() {
		return
	}
	if err := n.addTree(ev.Name, true); err != nil && !errors.Is(err, fs.ErrNotExist) {
		n.markResync()
	}
}

// handleErr: any watcher error, kernel queue overflow included, means
// events may have been lost.
func (n *native) handleErr(error) { n.markResync() }

// Ready implements Watcher.
func (n *native) Ready() <-chan struct{} { return n.readyCh() }

// Take implements Watcher.
func (n *native) Take() Changes { return n.take() }

// Close implements Watcher; idempotent.
func (n *native) Close() error {
	n.once.Do(func() {
		close(n.done)
		_ = n.fw.Close()
	})
	n.wg.Wait()
	return nil
}

// WarnPollAdvised reports whether native events are known to be unreliable
// for root: inotify does not see Windows-side edits under WSL's /mnt/ drives.
// The CLI prints a hint only; it does not switch adapters.
func WarnPollAdvised(root string) bool {
	return runtime.GOOS == "linux" && strings.HasPrefix(root, "/mnt/")
}
