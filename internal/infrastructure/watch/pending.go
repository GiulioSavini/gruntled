package watch

import (
	"path"
	"sort"
	"strings"
	"sync"
)

// pending is the accumulator adapters embed: a set of dirty repo-relative
// paths plus a sticky resync flag, with a buffered(1) ready channel.
//
// Invariant: add/markResync insert AND signal while holding mu; take clears
// the set and flag AND drains ready while holding mu. A Ready signal observed
// after Take therefore always means genuinely pending data, never a stale
// leftover. No goroutines live here.
type pending struct {
	mu     sync.Mutex
	set    map[string]struct{}
	resync bool
	ready  chan struct{}
}

func newPending() *pending {
	return &pending{set: map[string]struct{}{}, ready: make(chan struct{}, 1)}
}

// add records rel as dirty. It is the single entry for paths: ignored paths
// are dropped; anything outside the contract (empty, ".", absolute, volume,
// backslash, ".." escape) becomes resync instead of being stored.
func (p *pending) add(rel string) {
	clean, ok := normalise(rel)
	if ok && Ignored(clean) {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if ok {
		p.set[clean] = struct{}{}
	} else {
		p.resync = true
	}
	p.signal()
}

// markResync records that events may have been lost.
func (p *pending) markResync() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.resync = true
	p.signal()
}

// take drains everything pending and the ready signal.
func (p *pending) take() Changes {
	p.mu.Lock()
	defer p.mu.Unlock()
	select {
	case <-p.ready:
	default:
	}
	c := Changes{Resync: p.resync}
	if len(p.set) > 0 {
		c.Paths = make([]string, 0, len(p.set))
		for k := range p.set {
			c.Paths = append(c.Paths, k)
		}
		sort.Strings(c.Paths)
		p.set = map[string]struct{}{}
	}
	p.resync = false
	return c
}

func (p *pending) readyCh() <-chan struct{} { return p.ready }

// signal is a non-blocking send; callers hold mu.
func (p *pending) signal() {
	select {
	case p.ready <- struct{}{}:
	default:
	}
}

// normalise cleans rel and reports whether it honours the repo-relative,
// slash-separated contract.
func normalise(rel string) (string, bool) {
	if rel == "" || strings.ContainsRune(rel, '\\') {
		return "", false
	}
	if strings.HasPrefix(rel, "/") || (len(rel) >= 2 && rel[1] == ':') {
		return "", false
	}
	c := path.Clean(rel)
	if c == "." || c == ".." || strings.HasPrefix(c, "../") {
		return "", false
	}
	return c, true
}
