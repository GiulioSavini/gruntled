package watch

import (
	"fmt"
	"reflect"
	"sync"
	"testing"
)

func readyLen(p *pending) int { return len(p.readyCh()) }

func TestPendingDedupSortAndClear(t *testing.T) {
	p := newPending()
	p.add("b/terragrunt.hcl")
	p.add("a/x.hcl")
	p.add("b/terragrunt.hcl")
	p.add("./a//x.hcl")
	got := p.take()
	want := Changes{Paths: []string{"a/x.hcl", "b/terragrunt.hcl"}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("take() = %#v, want %#v", got, want)
	}
	if again := p.take(); len(again.Paths) != 0 || again.Resync {
		t.Fatalf("second take() = %#v, want empty", again)
	}
}

func TestPendingReadySignalAndDrain(t *testing.T) {
	p := newPending()
	if readyLen(p) != 0 {
		t.Fatal("fresh pending has Ready signalled")
	}
	p.add("a.hcl")
	p.add("b.hcl") // second add must not block on the full buffer
	if readyLen(p) != 1 {
		t.Fatalf("Ready len = %d after adds, want 1", readyLen(p))
	}
	p.take()
	if readyLen(p) != 0 {
		t.Fatal("Ready still signalled after take drained everything")
	}
	p.add("c.hcl")
	if readyLen(p) != 1 {
		t.Fatal("add after take did not re-signal Ready")
	}
	if got := p.take(); !reflect.DeepEqual(got.Paths, []string{"c.hcl"}) {
		t.Fatalf("take() = %#v", got)
	}
	if readyLen(p) != 0 {
		t.Fatal("leftover Ready signal for items already returned")
	}
}

func TestPendingIgnoredPathsDropped(t *testing.T) {
	p := newPending()
	p.add(".git/HEAD")
	p.add("a/main.tf.swp")
	if readyLen(p) != 0 {
		t.Fatal("ignored path signalled Ready")
	}
	if got := p.take(); len(got.Paths) != 0 || got.Resync {
		t.Fatalf("take() = %#v, want empty", got)
	}
}

func TestPendingOutOfContractBecomesResync(t *testing.T) {
	for _, bad := range []string{"", ".", "/etc/passwd", "..", "../x", "a/../../x", `a\b.hcl`, "C:/x.hcl"} {
		t.Run(fmt.Sprintf("%q", bad), func(t *testing.T) {
			p := newPending()
			p.add(bad)
			if readyLen(p) != 1 {
				t.Fatal("resync did not signal Ready")
			}
			got := p.take()
			if !got.Resync || len(got.Paths) != 0 {
				t.Fatalf("take() = %#v, want Resync and no paths", got)
			}
		})
	}
}

func TestPendingResyncSticky(t *testing.T) {
	p := newPending()
	p.markResync()
	p.add("a.hcl")
	got := p.take()
	if !got.Resync || !reflect.DeepEqual(got.Paths, []string{"a.hcl"}) {
		t.Fatalf("take() = %#v", got)
	}
	if got := p.take(); got.Resync {
		t.Fatal("resync survived take")
	}
}

func TestPendingConcurrentAdders(t *testing.T) {
	p := newPending()
	const workers, per = 8, 200
	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			for i := 0; i < per; i++ {
				p.add(fmt.Sprintf("w%d/f%d.hcl", w, i))
			}
		}(w)
	}
	seen := map[string]bool{}
	done := make(chan struct{})
	go func() { wg.Wait(); close(done) }()
	for finished := false; !finished; {
		select {
		case <-p.readyCh():
			for _, q := range p.take().Paths {
				seen[q] = true
			}
		case <-done:
			finished = true
		}
	}
	for _, q := range p.take().Paths {
		seen[q] = true
	}
	if len(seen) != workers*per {
		t.Fatalf("collected %d paths, want %d", len(seen), workers*per)
	}
}
