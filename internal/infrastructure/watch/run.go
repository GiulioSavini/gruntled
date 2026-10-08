package watch

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/GiulioSavini/gruntled/internal/application/checking"
)

// Indexer is the seam between the run loop and the Loader. *watching.Indexer
// satisfies it; tests use fakes.
type Indexer interface {
	Index(ctx context.Context, dirty []string, resync bool) (checking.Report, error)
}

// EventKind is the state a published Event reports.
type EventKind int

// Event kinds, in the order a healthy daemon emits them.
const (
	EventIndexing EventKind = iota + 1
	EventReady
	EventFailed
	EventStopped
)

// Event is one status change of the daemon.
type Event struct {
	Kind EventKind
	// Report is set for EventReady.
	Report checking.Report
	// Err is set for EventFailed.
	Err error
}

// Config wires Run.
type Config struct {
	Watcher Watcher
	Indexer Indexer
	// Publish is required; it is called only from the Run goroutine.
	Publish func(Event)
	// Now defaults to time.Now.
	Now func() time.Time
	// Quiet and MaxWait default to DefaultQuiet and DefaultMaxWait.
	Quiet, MaxWait time.Duration
}

// Run is the daemon's single indexer goroutine: it is the only caller of
// Indexer.Index, so the Loader behind it never sees concurrent use from the
// daemon. It runs the initial full index, then one reindex per debounced
// batch with exactly the dirty paths.
//
// A failed initial index publishes EventFailed and returns an error wrapping
// it (the caller exits 3). A failed reindex publishes EventFailed and keeps
// running. Cancelling ctx publishes EventStopped and returns nil. Run closes
// the Watcher on every return once the config is valid.
//
// A reindex result is not published while newer changes are genuinely
// pending (it may come from a torn read mid-save); the next batch's result is
// published instead, so the status never stays stale once the queue is quiet.
func Run(ctx context.Context, cfg Config) error {
	if cfg.Watcher == nil || cfg.Indexer == nil || cfg.Publish == nil {
		return errors.New("watch: Run needs a Watcher, an Indexer and a Publish func")
	}
	w := cfg.Watcher
	defer func() { _ = w.Close() }()
	now := cfg.Now
	if now == nil {
		now = time.Now
	}
	quiet, maxWait := cfg.Quiet, cfg.MaxWait
	if quiet <= 0 {
		quiet = DefaultQuiet
	}
	if maxWait <= 0 {
		maxWait = DefaultMaxWait
	}
	publish := cfg.Publish

	publish(Event{Kind: EventIndexing})
	last, err := cfg.Indexer.Index(ctx, nil, false)
	if err != nil {
		if ctx.Err() != nil {
			publish(Event{Kind: EventStopped})
			return nil
		}
		publish(Event{Kind: EventFailed, Err: err})
		return fmt.Errorf("initial index: %w", err)
	}
	publish(Event{Kind: EventReady, Report: last})

	deb := NewDebouncer(quiet, maxWait)
	timer := time.NewTimer(time.Hour)
	stopTimer(timer)
	defer timer.Stop()
	var timerC <-chan time.Time // nil while nothing is pending
	arm := func() {
		stopTimer(timer)
		due, ok := deb.Due()
		if !ok {
			timerC = nil
			return
		}
		d := due.Sub(now())
		if d < 0 {
			d = 0
		}
		timer.Reset(d)
		timerC = timer.C
	}
	// unpublished: last holds a result that was skipped because more work
	// was pending.
	unpublished := false

	for {
		select {
		case <-ctx.Done():
			publish(Event{Kind: EventStopped})
			return nil

		case <-w.Ready():
			if ch := w.Take(); !ch.Empty() {
				deb.Add(now(), ch)
			}
			arm()
			// A spurious Ready after a skip must not leave the status stale.
			if _, pend := deb.Due(); unpublished && !pend {
				publish(Event{Kind: EventReady, Report: last})
				unpublished = false
			}

		case <-timerC:
			timerC = nil
			// The timer is re-armed on every Add, so its firing means the
			// current deadline has passed even if the injected clock lags.
			t := now()
			if due, ok := deb.Due(); ok && t.Before(due) {
				t = due
			}
			b, ok := deb.Flush(t)
			if !ok {
				arm()
				continue
			}
			rep, err := cfg.Indexer.Index(ctx, b.Paths, b.Resync)
			if err != nil {
				if ctx.Err() != nil {
					publish(Event{Kind: EventStopped})
					return nil
				}
				publish(Event{Kind: EventFailed, Err: err})
				unpublished = false
				arm()
				continue
			}
			last = rep
			// Take drains Ready under the accumulator's mutex, so a Ready
			// signal followed by an empty Take is stale and must not
			// suppress this publish.
			if len(w.Ready()) > 0 {
				if ch := w.Take(); !ch.Empty() {
					deb.Add(now(), ch)
				}
			}
			if _, pend := deb.Due(); pend {
				unpublished = true
				arm()
				continue
			}
			publish(Event{Kind: EventReady, Report: last})
			unpublished = false
		}
	}
}

// stopTimer stops t and drains a value that may already sit in its channel,
// so a later Reset never delivers a stale fire.
func stopTimer(t *time.Timer) {
	if !t.Stop() {
		select {
		case <-t.C:
		default:
		}
	}
}
