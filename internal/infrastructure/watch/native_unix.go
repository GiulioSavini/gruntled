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

// lstatBeforeWatch is called on every directory below the walked root right
// before addWatch, so a directory swapped for a symlink after WalkDir listed
// it is not watched (10-sec#5). Test seam: production never reassigns it.
var lstatBeforeWatch = os.Lstat

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

	// patternDirs holds the repo-relative paths of the watched directories
	// whose base name matches an editor file pattern (2024, x.tmp, ...).
	// handleEvent uses it to keep classifying such a path as a directory
	// after it is gone. Touched only by the constructor before the loop
	// goroutine starts and by the loop goroutine afterwards: no mutex.
	patternDirs map[string]struct{}
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
	n := &native{
		pending: newPending(), root: root, fw: fw, safety: safety, done: make(chan struct{}),
		patternDirs: map[string]struct{}{},
	}
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
// following symlinks. Directories are judged by IgnoredEntry with their
// type, so only .git/.terraform/.terragrunt-cache subtrees are skipped; a
// directory named like an editor file is watched and recorded in
// patternDirs (the subtree root dir included, the repo root excluded). With emit, every path found is also marked dirty, so
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
		if p != dir && IgnoredEntry(rel, d.Type()) {
			if d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		if emit && p != dir {
			n.add(rel, d.Type())
		}
		if !d.IsDir() {
			return nil
		}
		if p != dir {
			// 10-sec#5: never watch a path that is a symlink (or no longer
			// a directory) at this point, even though WalkDir listed a dir.
			fi, err := lstatBeforeWatch(p)
			if err != nil || fi.Mode()&fs.ModeSymlink != 0 || !fi.IsDir() {
				return fs.SkipDir
			}
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
		if rel != "." && editorPattern(rel) {
			n.patternDirs[rel] = struct{}{}
		}
		return nil
	})
}

// prunePatternDirs forgets rel and every recorded directory below it. Called
// on every Remove/Rename; bounded by the number of recorded pattern dirs.
func (n *native) prunePatternDirs(rel string) {
	if len(n.patternDirs) == 0 {
		return
	}
	prefix := rel + "/"
	for k := range n.patternDirs {
		if k == rel || strings.HasPrefix(k, prefix) {
			delete(n.patternDirs, k)
		}
	}
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
			for _, c := range s.diff() {
				n.add(c.rel, c.typ)
			}
			if s.resync {
				n.markResync()
			}
		}
	}
}

// handleEvent marks the event path dirty and, for a created (or renamed-in)
// directory, starts watching its tree.
//
// Events carry no entry type. Paths under .git/.terraform/.terragrunt-cache
// are dropped outright; a path whose base name matches an editor pattern is
// classified (sec #37, over-approximation, never stale): a directory when it
// is a recorded pattern dir OR Lstat says dir, else Lstat's type, else 0
// (gone). Normal names pay no extra syscall.
func (n *native) handleEvent(ev fsnotify.Event) {
	rel := n.rel(ev.Name)
	if dropEvent != nil && dropEvent(rel) {
		return
	}
	if IgnoredEntry(rel, fs.ModeDir) { // component rule only
		return
	}
	_, known := n.patternDirs[rel]
	if ev.Has(fsnotify.Remove) || ev.Has(fsnotify.Rename) {
		n.prunePatternDirs(rel)
	}
	var (
		typ     fs.FileMode
		fi      fs.FileInfo
		lerr    error
		statted bool
	)
	if editorPattern(rel) {
		fi, lerr = os.Lstat(ev.Name)
		statted = true
		switch {
		case known || (lerr == nil && fi.IsDir()):
			typ = fs.ModeDir
		case lerr == nil:
			typ = fi.Mode().Type()
		}
	}
	if IgnoredEntry(rel, typ) {
		return
	}
	n.add(rel, typ) // out-of-contract rel (".", "..") becomes resync here
	if !ev.Has(fsnotify.Create) {
		return
	}
	if !statted {
		fi, lerr = os.Lstat(ev.Name)
	}
	if lerr != nil || !fi.IsDir() {
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
