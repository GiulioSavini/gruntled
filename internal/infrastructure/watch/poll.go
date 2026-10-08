package watch

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// DefaultPollInterval is the scan period when NewPoll is given interval <= 0.
const DefaultPollInterval = 500 * time.Millisecond

// entry is what a scan remembers about one path.
type entry struct {
	isDir bool
	size  int64
	mtime int64 // UnixNano
	mode  fs.FileMode
}

// scanner snapshots the tree under root and diffs successive snapshots.
// Not safe for concurrent use: one goroutine owns it.
//
// Limitation: an edit that keeps both size and mtime identical is invisible
// to a stat scan. Coarse-mtime filesystems make that possible; the native
// adapter does not have it, and tests always change size.
type scanner struct {
	root string
	prev map[string]entry
	// resync is set by diff when the root itself could not be read, meaning
	// changes may have been missed; cleared at the start of each diff.
	resync bool
}

// newScanner takes the baseline snapshot synchronously.
func newScanner(root string) *scanner {
	s := &scanner{root: root}
	snap, ok := s.scan()
	if !ok {
		snap = map[string]entry{}
	}
	s.prev = snap
	return s
}

// diff rescans, returns the repo-relative slash paths that were added,
// removed or changed since the previous snapshot (plus directory paths for
// added/removed directories) and replaces the snapshot. When the root scan
// fails it returns nothing, sets resync and keeps the previous snapshot.
func (s *scanner) diff() []string {
	s.resync = false
	cur, ok := s.scan()
	if !ok {
		s.resync = true
		return nil
	}
	var out []string
	for rel, c := range cur {
		p, existed := s.prev[rel]
		switch {
		case !existed:
			out = append(out, rel)
		case p.isDir != c.isDir || p.mode.Type() != c.mode.Type():
			out = append(out, rel)
		case c.isDir:
			// A directory's own mtime moves when children change; the
			// children are diffed individually, so nothing to report.
		case p.size != c.size || p.mtime != c.mtime || p.mode != c.mode:
			out = append(out, rel)
		}
	}
	for rel := range s.prev {
		if _, still := cur[rel]; !still {
			out = append(out, rel)
		}
	}
	s.prev = cur
	return out
}

// scan walks root without following symlinks, skipping ignored directories
// and ignored files. ok is false only when root itself cannot be read; an
// unreadable subdirectory keeps its previous entries (treated as unchanged
// for this scan).
func (s *scanner) scan() (map[string]entry, bool) {
	snap := map[string]entry{}
	if err := s.walk(s.root, "", snap); err != nil {
		return nil, false
	}
	return snap, true
}

func (s *scanner) walk(dir, rel string, snap map[string]entry) error {
	ents, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	for _, d := range ents {
		name := d.Name()
		childRel := name
		if rel != "" {
			childRel = rel + "/" + name
		}
		if d.IsDir() && IgnoredDir(name) {
			continue
		}
		if Ignored(childRel) {
			continue
		}
		info, err := d.Info() // lstat semantics: symlinks are not followed
		if err != nil {
			continue // vanished between ReadDir and Info; next scan sees it
		}
		e := entry{isDir: d.IsDir(), size: info.Size(), mtime: info.ModTime().UnixNano(), mode: info.Mode()}
		if e.isDir {
			e.size = 0
		}
		snap[childRel] = e
		if d.IsDir() {
			if err := s.walk(filepath.Join(dir, name), childRel, snap); err != nil {
				s.keepPrevious(childRel, snap)
			}
		}
	}
	return nil
}

// keepPrevious copies the previous snapshot's entries under dirRel into snap.
func (s *scanner) keepPrevious(dirRel string, snap map[string]entry) {
	prefix := dirRel + "/"
	for k, v := range s.prev {
		if strings.HasPrefix(k, prefix) {
			snap[k] = v
		}
	}
}

// poll is the stat-polling Watcher, available on every OS.
type poll struct {
	*pending
	done chan struct{}
	wg   sync.WaitGroup
	once sync.Once
}

// NewPoll returns a Watcher that stat-scans root every interval
// (DefaultPollInterval when interval <= 0). The baseline snapshot is taken
// before NewPoll returns, so constructing the watcher before the initial
// index loses no change made during that index.
func NewPoll(root string, interval time.Duration) (Watcher, error) {
	if interval <= 0 {
		interval = DefaultPollInterval
	}
	fi, err := os.Stat(root)
	if err != nil {
		return nil, err
	}
	if !fi.IsDir() {
		return nil, errors.New("watch: root is not a directory: " + root)
	}
	w := &poll{pending: newPending(), done: make(chan struct{})}
	s := newScanner(root)
	w.wg.Add(1)
	go w.run(s, interval)
	return w, nil
}

func (w *poll) run(s *scanner, interval time.Duration) {
	defer w.wg.Done()
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-w.done:
			return
		case <-t.C:
			for _, rel := range s.diff() {
				w.add(rel)
			}
			if s.resync {
				w.markResync()
			}
		}
	}
}

// Ready implements Watcher.
func (w *poll) Ready() <-chan struct{} { return w.readyCh() }

// Take implements Watcher.
func (w *poll) Take() Changes { return w.take() }

// Close implements Watcher; idempotent.
func (w *poll) Close() error {
	w.once.Do(func() { close(w.done) })
	w.wg.Wait()
	return nil
}
