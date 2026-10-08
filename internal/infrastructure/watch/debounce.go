package watch

import (
	"sort"
	"time"
)

// Debounce defaults: a 150 ms trailing quiet period, capped at 1 s since the
// first change of a burst so continuous output cannot starve the index.
const (
	DefaultQuiet   = 150 * time.Millisecond
	DefaultMaxWait = time.Second
)

// Debouncer coalesces Changes into batches. It is a pure state machine: the
// caller passes the current time, owns the timer and resets it to Due().
// Not safe for concurrent use; the run loop owns it.
type Debouncer struct {
	quiet, maxWait time.Duration
	first, last    time.Time
	armed          bool
	set            map[string]struct{}
	resync         bool
}

// NewDebouncer returns a Debouncer with the given trailing quiet period and
// maximum wait.
func NewDebouncer(quiet, maxWait time.Duration) *Debouncer {
	return &Debouncer{quiet: quiet, maxWait: maxWait, set: map[string]struct{}{}}
}

// Add merges c into the current batch at time now. Empty Changes are ignored.
func (d *Debouncer) Add(now time.Time, c Changes) {
	if c.Empty() {
		return
	}
	if !d.armed {
		d.first = now
		d.armed = true
	}
	d.last = now
	for _, p := range c.Paths {
		d.set[p] = struct{}{}
	}
	d.resync = d.resync || c.Resync
}

// Due returns the flush deadline min(last+quiet, first+maxWait); ok is false
// when nothing is pending.
func (d *Debouncer) Due() (time.Time, bool) {
	if !d.armed {
		return time.Time{}, false
	}
	t := d.last.Add(d.quiet)
	if limit := d.first.Add(d.maxWait); limit.Before(t) {
		t = limit
	}
	return t, true
}

// Flush returns the batch and resets when now has reached Due().
func (d *Debouncer) Flush(now time.Time) (Changes, bool) {
	due, ok := d.Due()
	if !ok || now.Before(due) {
		return Changes{}, false
	}
	c := Changes{Resync: d.resync}
	if len(d.set) > 0 {
		c.Paths = make([]string, 0, len(d.set))
		for p := range d.set {
			c.Paths = append(c.Paths, p)
		}
		sort.Strings(c.Paths)
	}
	d.set = map[string]struct{}{}
	d.resync = false
	d.armed = false
	return c, true
}
