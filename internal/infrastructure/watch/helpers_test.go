package watch

import (
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"
)

// contractTimeout bounds every eventually in the contract suite. Generous on
// purpose: a loaded CI runner must not turn a slow scan into a failure.
const contractTimeout = 10 * time.Second

// eventually polls cond every 5 ms until it holds or timeout elapses, then
// fails with what and the last observed state. It is the only way tests in
// this package wait: never sleep-then-assert.
func eventually(t *testing.T, timeout time.Duration, cond func() bool, what string, state func() string) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for {
		if cond() {
			return
		}
		if time.Now().After(deadline) {
			last := ""
			if state != nil {
				last = state()
			}
			t.Fatalf("timed out after %v waiting for %s; last state: %s", timeout, what, last)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// collector accumulates Take() results. It is used only from the test
// goroutine, so it needs no locking; the watcher's own goroutine touches only
// its pending accumulator.
type collector struct {
	t      *testing.T
	w      Watcher
	seen   map[string]bool
	resync bool
}

func newCollector(t *testing.T, w Watcher) *collector {
	return &collector{t: t, w: w, seen: map[string]bool{}}
}

// drain merges one Take() into the set, checking the path contract on every
// path ever reported.
func (c *collector) drain() {
	c.t.Helper()
	got := c.w.Take()
	c.resync = c.resync || got.Resync
	for _, p := range got.Paths {
		if p == "" || path.IsAbs(p) || filepath.IsAbs(p) || strings.ContainsRune(p, '\\') ||
			p == ".." || strings.HasPrefix(p, "../") || strings.Contains(p, "/../") || path.Clean(p) != p {
			c.t.Errorf("watcher reported out-of-contract path %q", p)
		}
		c.seen[p] = true
	}
}

func (c *collector) reset() { c.drain(); c.seen = map[string]bool{}; c.resync = false }

func (c *collector) String() string {
	ks := make([]string, 0, len(c.seen))
	for k := range c.seen {
		ks = append(ks, k)
	}
	sort.Strings(ks)
	return "paths=[" + strings.Join(ks, " ") + "] resync=" + boolStr(c.resync)
}

func boolStr(b bool) string {
	if b {
		return "true"
	}
	return "false"
}

// collect drains w until pred holds over the accumulated set.
func collect(t *testing.T, c *collector, pred func(seen map[string]bool) bool, what string) {
	t.Helper()
	eventually(t, contractTimeout, func() bool { c.drain(); return pred(c.seen) }, what, c.String)
}

// writeFile writes rel under root, creating parents.
func writeFile(t *testing.T, root, rel, content string) {
	t.Helper()
	full := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func abs(root, rel string) string { return filepath.Join(root, filepath.FromSlash(rel)) }
