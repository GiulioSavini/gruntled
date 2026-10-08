// Package watch turns filesystem activity under a repository root into a
// pull-style dirty set for the watch daemon.
//
// Events are hints (a dirty path), never meaning: an adapter never branches
// on event type for correctness, it only reports which repo-relative paths
// may have changed, or that events may have been lost (Resync). The consumer
// feeds them to Loader.Invalidate and reloads; the result is the same as a
// full index by construction.
package watch

import "errors"

// Changes is one drained batch of dirty paths.
type Changes struct {
	// Paths are repo-relative, slash-separated, cleaned and sorted; never
	// absolute, never ".."-prefixed, never containing a backslash.
	Paths []string
	// Resync means events may have been lost: the caller must treat the batch
	// as Invalidate(".").
	Resync bool
}

// Empty reports whether c carries nothing to do.
func (c Changes) Empty() bool { return len(c.Paths) == 0 && !c.Resync }

// Watcher is the port every adapter (stat-poll, native) implements.
type Watcher interface {
	// Ready is buffered(1) and signalled when Take would return something.
	Ready() <-chan struct{}
	// Take drains the pending set; it returns empty Changes if nothing is
	// pending.
	Take() Changes
	// Close is idempotent and stops the adapter's goroutines.
	Close() error
}

// ErrWatchLimit is returned by a native adapter constructor when the OS
// watch limit (inotify watches, kqueue descriptors) is exhausted. The caller
// falls back to polling.
var ErrWatchLimit = errors.New("watch: OS watch limit reached")

// ErrNativeUnavailable is returned where no native watcher is built for the
// platform.
var ErrNativeUnavailable = errors.New("watch: native watcher not available on this platform")
