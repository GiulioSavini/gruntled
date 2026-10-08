package watch

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/GiulioSavini/gruntled/internal/application/checking"
	"github.com/GiulioSavini/gruntled/internal/domain/repograph"
)

// runTimeout bounds every wait in the run-loop tests.
const runTimeout = 10 * time.Second

// fixedNow is the injected clock: it never moves, so the loop must rely on
// its timer firing, not on the clock advancing, to flush a batch.
func fixedNow() time.Time { return time.Unix(1_700_000_000, 0) }

// fakeWatcher is fed by the test through the real pending accumulator, so
// it honours the take-drains-Ready invariant.
type fakeWatcher struct {
	p      *pending
	closes atomic.Int32
}

func newFakeWatcher() *fakeWatcher { return &fakeWatcher{p: newPending()} }

func (f *fakeWatcher) Ready() <-chan struct{} { return f.p.readyCh() }
func (f *fakeWatcher) Take() Changes          { return f.p.take() }
func (f *fakeWatcher) Close() error           { f.closes.Add(1); return nil }

func (f *fakeWatcher) feed(paths ...string) {
	for _, p := range paths {
		f.p.add(p)
	}
}

// spurious signals Ready without adding data: the following Take is empty.
func (f *fakeWatcher) spurious() {
	f.p.mu.Lock()
	f.p.signal()
	f.p.mu.Unlock()
}

type indexCall struct {
	dirty  []string
	resync bool
}

// reports gives every Index call a report with its own graph pointer, so a
// published report identifies the call that produced it.
var reports = func() []*repograph.RepositoryGraph {
	gs := make([]*repograph.RepositoryGraph, 4096)
	for i := range gs {
		gs[i] = new(repograph.RepositoryGraph)
	}
	return gs
}()

func reportFor(n int) checking.Report { return checking.Report{Graph: reports[n]} }

// callOf returns the 1-based Index call that produced rep, or 0.
func callOf(rep checking.Report) int {
	for i, g := range reports {
		if i > 0 && g == rep.Graph {
			return i
		}
	}
	return 0
}

// fakeIndexer records calls and fails the test if two ever overlap.
type fakeIndexer struct {
	mu       sync.Mutex
	calls    []indexCall
	hook     func(ctx context.Context, n int) error
	inflight atomic.Int32
	overlap  atomic.Bool
}

func (f *fakeIndexer) Index(ctx context.Context, dirty []string, resync bool) (checking.Report, error) {
	if f.inflight.Add(1) > 1 {
		f.overlap.Store(true)
	}
	defer f.inflight.Add(-1)
	f.mu.Lock()
	var d []string
	if dirty != nil {
		d = append([]string(nil), dirty...)
	}
	f.calls = append(f.calls, indexCall{dirty: d, resync: resync})
	n := len(f.calls)
	hook := f.hook
	f.mu.Unlock()
	if hook != nil {
		if err := hook(ctx, n); err != nil {
			return checking.Report{}, err
		}
	}
	return reportFor(n), nil
}

func (f *fakeIndexer) snapshot() []indexCall {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]indexCall(nil), f.calls...)
}

func (f *fakeIndexer) count() int { return len(f.snapshot()) }

type recorder struct {
	mu  sync.Mutex
	evs []Event
}

func (r *recorder) publish(e Event) {
	r.mu.Lock()
	r.evs = append(r.evs, e)
	r.mu.Unlock()
}

func (r *recorder) snapshot() []Event {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]Event(nil), r.evs...)
}

func (r *recorder) last() (Event, bool) {
	evs := r.snapshot()
	if len(evs) == 0 {
		return Event{}, false
	}
	return evs[len(evs)-1], true
}

func (r *recorder) String() string {
	parts := []string{}
	for _, e := range r.snapshot() {
		parts = append(parts, eventString(e))
	}
	return "[" + strings.Join(parts, " ") + "]"
}

func eventString(e Event) string {
	switch e.Kind {
	case EventIndexing:
		return "indexing"
	case EventReady:
		return fmt.Sprintf("ready#%d", callOf(e.Report))
	case EventFailed:
		return fmt.Sprintf("failed(%v)", e.Err)
	case EventStopped:
		return "stopped"
	}
	return fmt.Sprintf("kind%d", e.Kind)
}

// isReady reports whether e is Ready with the report of call n.
func isReady(e Event, n int) bool { return e.Kind == EventReady && callOf(e.Report) == n }

type harness struct {
	t      *testing.T
	w      *fakeWatcher
	idx    *fakeIndexer
	rec    *recorder
	cancel context.CancelFunc
	done   chan error
}

// start runs Run in its own goroutine. Cleanup cancels it, waits for it to
// return and checks the single-caller guarantee.
func start(t *testing.T, idx *fakeIndexer, quiet, maxWait time.Duration) *harness {
	t.Helper()
	h := &harness{t: t, w: newFakeWatcher(), idx: idx, rec: &recorder{}, done: make(chan error, 1)}
	ctx, cancel := context.WithCancel(context.Background())
	h.cancel = cancel
	cfg := Config{Watcher: h.w, Indexer: idx, Publish: h.rec.publish, Now: fixedNow, Quiet: quiet, MaxWait: maxWait}
	go func() { h.done <- Run(ctx, cfg) }()
	t.Cleanup(func() {
		cancel()
		select {
		case <-h.done:
		case <-time.After(runTimeout):
			t.Errorf("Run did not return after cancel")
		}
		if idx.overlap.Load() {
			t.Errorf("Indexer.Index was called concurrently")
		}
	})
	return h
}

// stop cancels Run and returns its result.
func (h *harness) stop() error {
	h.t.Helper()
	h.cancel()
	select {
	case err := <-h.done:
		h.done <- err // let Cleanup observe the return too
		return err
	case <-time.After(runTimeout):
		h.t.Fatalf("Run did not return after cancel; events %s", h.rec)
	}
	return nil
}

func (h *harness) waitFor(what string, cond func() bool) {
	h.t.Helper()
	eventually(h.t, runTimeout, cond, what, func() string {
		return fmt.Sprintf("events %s calls %v", h.rec, h.idx.snapshot())
	})
}

// waitLast waits until the last published event is Ready with call n.
func (h *harness) waitLast(n int) {
	h.t.Helper()
	h.waitFor(fmt.Sprintf("last event ready#%d", n), func() bool {
		e, ok := h.rec.last()
		return ok && isReady(e, n)
	})
}

// gate blocks a chosen Index call until released.
type gate struct {
	entered chan struct{}
	release chan struct{}
}

func newGate() *gate { return &gate{entered: make(chan struct{}), release: make(chan struct{})} }

func (g *gate) wait(ctx context.Context) error {
	close(g.entered)
	select {
	case <-g.release:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (g *gate) awaitEntered(t *testing.T) {
	t.Helper()
	select {
	case <-g.entered:
	case <-time.After(runTimeout):
		t.Fatalf("Index call never entered the gate")
	}
}

func TestRunInitialIndex(t *testing.T) {
	idx := &fakeIndexer{}
	h := start(t, idx, 5*time.Millisecond, 50*time.Millisecond)
	h.waitLast(1)
	if err := h.stop(); err != nil {
		t.Fatalf("Run returned %v", err)
	}
	evs := h.rec.snapshot()
	if len(evs) != 3 || evs[0].Kind != EventIndexing || !isReady(evs[1], 1) || evs[2].Kind != EventStopped {
		t.Fatalf("events = %s, want [indexing ready#1 stopped]", h.rec)
	}
	calls := idx.snapshot()
	if len(calls) != 1 || calls[0].dirty != nil || calls[0].resync {
		t.Fatalf("calls = %v, want one Index(nil,false)", calls)
	}
}

func TestRunOneSave(t *testing.T) {
	idx := &fakeIndexer{}
	h := start(t, idx, 5*time.Millisecond, 50*time.Millisecond)
	h.waitLast(1)
	h.w.feed("a/terragrunt.hcl")
	h.waitLast(2)
	if err := h.stop(); err != nil {
		t.Fatal(err)
	}
	calls := idx.snapshot()
	if len(calls) != 2 {
		t.Fatalf("calls = %v, want 2", calls)
	}
	if !reflect.DeepEqual(calls[1].dirty, []string{"a/terragrunt.hcl"}) || calls[1].resync {
		t.Fatalf("reindex call = %+v", calls[1])
	}
}

func TestRunBurstCoalesces(t *testing.T) {
	idx := &fakeIndexer{}
	// Quiet is generous so 20 back-to-back feeds always land in one window.
	h := start(t, idx, 300*time.Millisecond, 10*time.Second)
	h.waitLast(1)
	var want []string
	for i := 0; i < 20; i++ {
		p := fmt.Sprintf("u%02d/terragrunt.hcl", i)
		want = append(want, p)
		h.w.feed(p)
	}
	h.waitLast(2)
	if err := h.stop(); err != nil {
		t.Fatal(err)
	}
	calls := idx.snapshot()
	if len(calls) != 2 {
		t.Fatalf("got %d Index calls, want 2 (initial + one batch): %v", len(calls), calls)
	}
	if !reflect.DeepEqual(calls[1].dirty, want) {
		t.Fatalf("batch = %v, want %v", calls[1].dirty, want)
	}
}

func TestRunResync(t *testing.T) {
	idx := &fakeIndexer{}
	h := start(t, idx, 5*time.Millisecond, 50*time.Millisecond)
	h.waitLast(1)
	h.w.p.markResync()
	h.waitLast(2)
	calls := idx.snapshot()
	if !calls[1].resync || calls[1].dirty != nil {
		t.Fatalf("resync call = %+v, want resync true with no paths", calls[1])
	}
}

func TestRunReindexErrorKeepsRunning(t *testing.T) {
	boom := errors.New("boom")
	idx := &fakeIndexer{hook: func(_ context.Context, n int) error {
		if n == 2 {
			return boom
		}
		return nil
	}}
	h := start(t, idx, 5*time.Millisecond, 50*time.Millisecond)
	h.waitLast(1)
	h.w.feed("a/terragrunt.hcl")
	h.waitFor("failed event", func() bool {
		e, ok := h.rec.last()
		return ok && e.Kind == EventFailed && errors.Is(e.Err, boom)
	})
	select {
	case err := <-h.done:
		h.done <- err
		t.Fatalf("Run returned %v after a reindex error", err)
	default:
	}
	h.w.feed("b/terragrunt.hcl")
	h.waitLast(3)
	if err := h.stop(); err != nil {
		t.Fatal(err)
	}
	if got := idx.snapshot()[2].dirty; !reflect.DeepEqual(got, []string{"b/terragrunt.hcl"}) {
		t.Fatalf("third call dirty = %v", got)
	}
}

func TestRunInitialIndexError(t *testing.T) {
	boom := errors.New("boom")
	idx := &fakeIndexer{hook: func(context.Context, int) error { return boom }}
	w := newFakeWatcher()
	rec := &recorder{}
	err := Run(context.Background(), Config{Watcher: w, Indexer: idx, Publish: rec.publish, Now: fixedNow})
	if !errors.Is(err, boom) {
		t.Fatalf("Run = %v, want wrapping %v", err, boom)
	}
	evs := rec.snapshot()
	if len(evs) != 2 || evs[0].Kind != EventIndexing || evs[1].Kind != EventFailed || !errors.Is(evs[1].Err, boom) {
		t.Fatalf("events = %s, want [indexing failed(boom)]", rec)
	}
	if idx.count() != 1 {
		t.Fatalf("Index called %d times, want 1", idx.count())
	}
	if w.closes.Load() != 1 {
		t.Fatalf("Close called %d times, want 1", w.closes.Load())
	}
}

func TestRunSkipsIntermediateResult(t *testing.T) {
	g := newGate()
	idx := &fakeIndexer{hook: func(ctx context.Context, n int) error {
		if n == 2 {
			return g.wait(ctx)
		}
		return nil
	}}
	h := start(t, idx, 5*time.Millisecond, 50*time.Millisecond)
	h.waitLast(1)
	h.w.feed("a/terragrunt.hcl")
	g.awaitEntered(t)
	h.w.feed("b/terragrunt.hcl") // newer change pending while call 2 runs
	close(g.release)
	h.waitLast(3)
	if err := h.stop(); err != nil {
		t.Fatal(err)
	}
	for _, e := range h.rec.snapshot() {
		if isReady(e, 2) {
			t.Fatalf("intermediate result of call 2 was published: %s", h.rec)
		}
	}
	calls := idx.snapshot()
	if len(calls) != 3 || !reflect.DeepEqual(calls[2].dirty, []string{"b/terragrunt.hcl"}) {
		t.Fatalf("calls = %v", calls)
	}
}

func TestRunStaleSignalDoesNotSuppressPublish(t *testing.T) {
	g := newGate()
	idx := &fakeIndexer{hook: func(ctx context.Context, n int) error {
		if n == 2 {
			return g.wait(ctx)
		}
		return nil
	}}
	h := start(t, idx, 5*time.Millisecond, 50*time.Millisecond)
	h.waitLast(1)
	h.w.feed("a/terragrunt.hcl")
	g.awaitEntered(t)
	h.w.spurious() // Ready fires but Take will be empty
	close(g.release)
	h.waitLast(2)
	if n := idx.count(); n != 2 {
		t.Fatalf("Index called %d times, want 2", n)
	}
}

func TestRunSpuriousReadyAfterSkipEndsPublished(t *testing.T) {
	g2, g3 := newGate(), newGate()
	idx := &fakeIndexer{hook: func(ctx context.Context, n int) error {
		switch n {
		case 2:
			return g2.wait(ctx)
		case 3:
			return g3.wait(ctx)
		}
		return nil
	}}
	h := start(t, idx, 50*time.Millisecond, 500*time.Millisecond)
	h.waitLast(1)
	h.w.feed("a/terragrunt.hcl")
	g2.awaitEntered(t)
	h.w.feed("b/terragrunt.hcl")
	close(g2.release)
	h.w.spurious() // after (or racing) the skip: Take is empty or drains b
	g3.awaitEntered(t)
	h.w.spurious() // while call 3 runs: must not suppress its publish
	close(g3.release)
	h.waitLast(3)
	for _, e := range h.rec.snapshot() {
		if isReady(e, 2) {
			t.Fatalf("intermediate result of call 2 was published: %s", h.rec)
		}
	}
	if n := idx.count(); n != 3 {
		t.Fatalf("Index called %d times, want 3", n)
	}
}

func TestRunCancel(t *testing.T) {
	idx := &fakeIndexer{}
	h := start(t, idx, 5*time.Millisecond, 50*time.Millisecond)
	h.waitLast(1)
	if err := h.stop(); err != nil {
		t.Fatalf("Run = %v, want nil", err)
	}
	if e, _ := h.rec.last(); e.Kind != EventStopped {
		t.Fatalf("last event = %s, want stopped", eventString(e))
	}
	if c := h.w.closes.Load(); c != 1 {
		t.Fatalf("Close called %d times, want 1", c)
	}
}

func TestRunCancelDuringInitialIndex(t *testing.T) {
	g := newGate()
	idx := &fakeIndexer{hook: func(ctx context.Context, _ int) error { return g.wait(ctx) }}
	h := start(t, idx, 5*time.Millisecond, 50*time.Millisecond)
	g.awaitEntered(t)
	if err := h.stop(); err != nil {
		t.Fatalf("Run = %v, want nil on cancel", err)
	}
	evs := h.rec.snapshot()
	if len(evs) != 2 || evs[0].Kind != EventIndexing || evs[1].Kind != EventStopped {
		t.Fatalf("events = %s, want [indexing stopped]", h.rec)
	}
	if c := h.w.closes.Load(); c != 1 {
		t.Fatalf("Close called %d times, want 1", c)
	}
}

func TestRunCancelDuringReindex(t *testing.T) {
	g := newGate()
	idx := &fakeIndexer{hook: func(ctx context.Context, n int) error {
		if n == 2 {
			return g.wait(ctx)
		}
		return nil
	}}
	h := start(t, idx, 5*time.Millisecond, 50*time.Millisecond)
	h.waitLast(1)
	h.w.feed("a/terragrunt.hcl")
	g.awaitEntered(t)
	if err := h.stop(); err != nil {
		t.Fatalf("Run = %v, want nil on cancel", err)
	}
	for _, e := range h.rec.snapshot() {
		if e.Kind == EventFailed {
			t.Fatalf("cancellation reported as failure: %s", h.rec)
		}
	}
	if e, _ := h.rec.last(); e.Kind != EventStopped {
		t.Fatalf("last event = %s, want stopped", eventString(e))
	}
}

// TestRunStressSingleCallerNeverStale hammers the loop with a tiny debounce:
// Index calls must never overlap (checked in Cleanup), every fed path must
// reach the indexer, and once the queue is quiet the last published event is
// the result of the last Index call.
func TestRunStressSingleCallerNeverStale(t *testing.T) {
	idx := &fakeIndexer{}
	h := start(t, idx, time.Millisecond, 2*time.Millisecond)
	h.waitLast(1)
	want := map[string]bool{}
	for i := 0; i < 200; i++ {
		p := fmt.Sprintf("u%03d/terragrunt.hcl", i)
		want[p] = true
		h.w.feed(p)
		if i%50 == 0 {
			h.w.spurious()
		}
	}
	h.waitFor("all paths indexed and last result published", func() bool {
		calls := idx.snapshot()
		seen := map[string]bool{}
		for _, c := range calls {
			for _, p := range c.dirty {
				seen[p] = true
			}
		}
		for p := range want {
			if !seen[p] {
				return false
			}
		}
		e, ok := h.rec.last()
		return ok && isReady(e, len(calls))
	})
}

func TestRunConfigValidation(t *testing.T) {
	w := newFakeWatcher()
	idx := &fakeIndexer{}
	pub := func(Event) {}
	for name, cfg := range map[string]Config{
		"no watcher": {Indexer: idx, Publish: pub},
		"no indexer": {Watcher: w, Publish: pub},
		"no publish": {Watcher: w, Indexer: idx},
	} {
		if err := Run(context.Background(), cfg); err == nil {
			t.Errorf("%s: Run returned nil", name)
		}
	}
	if idx.count() != 0 {
		t.Fatalf("Index called on invalid config")
	}
}
