package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/GiulioSavini/gruntled/internal/application/blasting"
	"github.com/GiulioSavini/gruntled/internal/application/checking"
	"github.com/GiulioSavini/gruntled/internal/domain/diagnostic"
	"github.com/GiulioSavini/gruntled/internal/domain/impact"
	"github.com/GiulioSavini/gruntled/internal/infrastructure/ipc"
	"github.com/GiulioSavini/gruntled/internal/infrastructure/statusfile"
	"github.com/GiulioSavini/gruntled/internal/infrastructure/watch"
	"github.com/GiulioSavini/gruntled/internal/interfaces/presenter"
)

// abReports returns A, a clean tree where app reads vpc's output vpc_id,
// and B, the same tree with that output removed: Between(A, B) holds a
// GRT004 at app.
func abReports(t *testing.T) (checking.Report, checking.Report) {
	t.Helper()
	a := repo(t, "vpc_id")
	b := repo(t, "vpc_id")
	writeFiles(t, b, map[string]string{"vpc/main.tf": "\n"})
	return freshReport(t, a), freshReport(t, b)
}

// testPublisher is a publisher with captured stdout/stderr, a status file
// in a temp dir and a recorder for every handed-on snapshot.
type testPublisher struct {
	*watchPublisher
	out, errOut *bytes.Buffer
	statusPath  string
	served      atomic.Pointer[ipc.Snapshot]
	gens        sync.Map // every generation handed on
}

func newTestPublisher(t *testing.T) *testPublisher {
	t.Helper()
	dir := filepath.Join(shortBase(t), "rt")
	mustDo(t, statusfile.EnsureDir(dir))
	tp := &testPublisher{out: &bytes.Buffer{}, errOut: &bytes.Buffer{}, statusPath: filepath.Join(dir, "status")}
	tp.watchPublisher = &watchPublisher{
		stdout:    tp.out,
		stderr:    tp.errOut,
		now:       func() time.Time { return watchStamp },
		status:    statusfile.NewWriter(tp.statusPath),
		snapshots: func(s *ipc.Snapshot) { tp.gens.Store(s.Generation, true); tp.served.Store(s) },
	}
	return tp
}

func ready(rep checking.Report) watch.Event { return watch.Event{Kind: watch.EventReady, Report: rep} }

var failedEvent = watch.Event{Kind: watch.EventFailed, Err: errors.New("boom")}

func daemonLabel(src string, g0, g uint64) string {
	return presenter.DaemonBaselineLabel(presenter.DaemonBaseline{Source: src, Generation: g0, CurrentGeneration: g})
}

func brokenErrorCount(res impact.Result) int {
	n := 0
	for _, b := range res.Broken {
		for _, d := range b.Findings {
			if d.Severity() == diagnostic.SeverityError {
				n++
			}
		}
	}
	return n
}

func wantRefused(t *testing.T, err error, code string) *ipc.RefusedError {
	t.Helper()
	var re *ipc.RefusedError
	if !errors.As(err, &re) || re.Code != code {
		t.Fatalf("err = %v, want a refusal with code %s", err, code)
	}
	return re
}

func setSeam[T any](t *testing.T, v *T, x T) {
	t.Helper()
	old := *v
	*v = x
	t.Cleanup(func() { *v = old })
}

func TestPublisherBlastView(t *testing.T) {
	a, b := abReports(t)
	p := newTestPublisher(t)

	p.publish(ready(a))
	s1 := p.served.Load()
	want1, err := buildSnapshot(a, 1)
	mustDo(t, err)
	assertSnapshot(t, s1, want1)
	if s1.Blast == nil || s1.Blast.HasErrors || s1.Blast.BaselineGeneration != 1 || s1.Blast.BaselineSource != "watch start" {
		t.Fatalf("view 1 = %+v", s1.Blast)
	}
	text1 := string(s1.Blast.Text)
	for _, s := range []string{"baseline: " + daemonLabel("watch start", 1, 1) + "\n", "Broken (0):", "Impacted (0):"} {
		if !strings.Contains(text1, s) {
			t.Fatalf("view 1 text lacks %q:\n%s", s, text1)
		}
	}

	p.publish(ready(b))
	s2 := p.served.Load()
	want2, err := buildSnapshot(b, 2)
	mustDo(t, err)
	assertSnapshot(t, s2, want2)
	res, err := blasting.Between(a, b)
	mustDo(t, err)
	d := presenter.DaemonBaseline{Source: "watch start", Generation: 1, CurrentGeneration: 2}
	var text, js bytes.Buffer
	mustDo(t, presenter.BlastText(&text, res, presenter.DaemonBaselineLabel(d)))
	mustDo(t, presenter.BlastJSONDaemon(&js, res, d))
	v := s2.Blast
	if !bytes.Equal(v.Text, text.Bytes()) || !bytes.Equal(v.JSON, js.Bytes()) {
		t.Fatalf("view 2 differs from blast --base presenters:\n%s\n%s", v.Text, v.JSON)
	}
	if !v.HasErrors || v.BrokenErrors != brokenErrorCount(res) || v.BrokenErrors == 0 || !strings.Contains(string(v.Text), "GRT004") {
		t.Fatalf("view 2 = has %v broken %d (want %d):\n%s", v.HasErrors, v.BrokenErrors, brokenErrorCount(res), v.Text)
	}

	p.publish(failedEvent)
	s3 := p.served.Load()
	if s3.State != ipc.StateFailed || s3.Blast != s2.Blast || s3.Generation != 2 {
		t.Fatalf("failed snapshot = %+v", s3)
	}
}

func TestPublisherBetweenError(t *testing.T) {
	a, b := abReports(t)
	p := newTestPublisher(t)
	p.publish(ready(a))
	setSeam(t, &between, func(checking.Report, checking.Report) (impact.Result, error) {
		return impact.Result{}, errors.New("compare \x1b[2Jfailed\r\n")
	})
	p.publish(ready(b))
	s := p.served.Load()
	want, err := buildSnapshot(b, 2)
	mustDo(t, err)
	assertSnapshot(t, s, want)
	if s.Blast == nil || s.Blast.Error == "" || strings.ContainsAny(s.Blast.Error, "\x1b\r\n") ||
		s.Blast.BaselineGeneration != 1 || s.Blast.Text != nil || s.Blast.JSON != nil {
		t.Fatalf("view = %+v", s.Blast)
	}
	base := p.base
	_, err = p.rebase(nil)
	if re := wantRefused(t, err, ipc.CodeUnavailable); re.Msg != "the blast view could not be computed; nothing changed" {
		t.Fatalf("message %q", re.Msg)
	}
	if p.served.Load() != s || p.base.Graph != base.Graph || p.baseGen != 1 {
		t.Fatal("refused rebase changed the snapshot or the baseline")
	}
}

func TestPublisherBuildSnapshotError(t *testing.T) {
	a, b := abReports(t)
	p := newTestPublisher(t)
	p.publish(ready(a))
	s1 := p.served.Load()
	setSeam(t, &buildSnap, func(checking.Report, uint64) (*ipc.Snapshot, error) { return nil, errors.New("render") })
	p.publish(ready(b))
	if p.served.Load() != s1 || s1.Generation != 1 || p.gen != 1 || p.lastRep.Graph != a.Graph {
		t.Fatalf("failed build committed: gen %d, served gen %d", p.gen, p.served.Load().Generation)
	}
	if !strings.Contains(p.errOut.String(), "gruntled: rendering report: render") {
		t.Fatalf("stderr %q", p.errOut)
	}
	res, err := p.rebase(nil)
	if err != nil || res.Generation != 1 || p.base.Graph != a.Graph {
		t.Fatalf("rebase = (%+v, %v)", res, err)
	}
	one := uint64(1)
	if _, err := p.rebase(&one); err != nil {
		t.Fatalf("rebase(&1) = %v", err)
	}
	buildSnap = buildSnapshot
	p.publish(ready(b))
	if s := p.served.Load(); s.Generation != 2 {
		t.Fatalf("generation %d after the seam was restored", s.Generation)
	}
}

func TestRebaseRenderError(t *testing.T) {
	a, b := abReports(t)
	p := newTestPublisher(t)
	p.publish(ready(a))
	p.publish(ready(b))
	s := p.served.Load()
	base, baseGen, src := p.base, p.baseGen, p.baseSource
	setSeam(t, &between, func(checking.Report, checking.Report) (impact.Result, error) {
		return impact.Result{}, errors.New("boom")
	})
	_, err := p.rebase(nil)
	wantRefused(t, err, ipc.CodeInternal)
	if p.served.Load() != s || p.base.Graph != base.Graph || p.baseGen != baseGen || p.baseSource != src {
		t.Fatal("failed rebase changed state")
	}
}

func statusState(t *testing.T, path string) (string, time.Time) {
	t.Helper()
	b, err := os.ReadFile(path)
	mustDo(t, err)
	fi, err := os.Stat(path)
	mustDo(t, err)
	return string(b), fi.ModTime()
}

func TestPublisherRebase(t *testing.T) {
	a, b := abReports(t)
	p := newTestPublisher(t)
	if _, err := p.rebase(nil); err == nil {
		t.Fatal("rebase before any publish accepted")
	} else {
		wantRefused(t, err, ipc.CodeIndexing)
	}
	p.publish(ready(a))
	p.publish(ready(b))
	p.publish(failedEvent)
	s := p.served.Load()
	_, err := p.rebase(nil)
	wantRefused(t, err, ipc.CodeFailed)
	if p.served.Load() != s {
		t.Fatal("refused rebase replaced the snapshot")
	}

	p.publish(ready(b)) // C, generation 3
	s = p.served.Load()
	if s.Generation != 3 {
		t.Fatalf("generation %d", s.Generation)
	}
	content, mtime := statusState(t, p.statusPath)
	outLen, errLen := p.out.Len(), p.errOut.Len()

	two := uint64(2)
	_, err = p.rebase(&two)
	if re := wantRefused(t, err, ipc.CodeExpectMismatch); re.Msg != "the daemon is at generation 3, not 2" {
		t.Fatalf("message %q", re.Msg)
	}
	if p.served.Load() != s {
		t.Fatal("expect mismatch replaced the snapshot")
	}

	three := uint64(3)
	res, err := p.rebase(&three)
	if err != nil || res.Generation != 3 || res.BrokenErrors != s.Blast.BrokenErrors || res.BrokenErrors == 0 {
		t.Fatalf("rebase = (%+v, %v), view had %d", res, err, s.Blast.BrokenErrors)
	}
	n := p.served.Load()
	assertSnapshot(t, n, s)
	v := n.Blast
	text := string(v.Text)
	if !strings.HasPrefix(text, "baseline: "+daemonLabel("rebase", 3, 3)+"\n") || !strings.Contains(text, "Broken (0):") ||
		!strings.Contains(text, "Impacted (0):") || strings.Contains(text, "Type changes") || v.HasErrors || v.BrokenErrors != 0 {
		t.Fatalf("rebased view:\n%s", text)
	}
	var doc struct{ Broken, Impacted, Changes []json.RawMessage }
	if err := json.Unmarshal(v.JSON, &doc); err != nil || doc.Broken == nil || len(doc.Broken)+len(doc.Impacted)+len(doc.Changes) != 0 ||
		!bytes.Contains(v.JSON, []byte(`"source": "rebase"`)) {
		t.Fatalf("rebased JSON (%v):\n%s", err, v.JSON)
	}
	if p.baseSource != "rebase" || p.baseGen != 3 || p.gen != 3 || p.base.Graph != b.Graph {
		t.Fatalf("baseline %s/%d gen %d", p.baseSource, p.baseGen, p.gen)
	}

	res, err = p.rebase(nil)
	if err != nil || res != (ipc.RebaseResult{Generation: 3, BrokenErrors: 0}) {
		t.Fatalf("second rebase = (%+v, %v)", res, err)
	}
	if c, m := statusState(t, p.statusPath); c != content || !m.Equal(mtime) {
		t.Fatal("rebase wrote the status file")
	}
	if p.out.Len() != outLen || p.errOut.Len() != errLen {
		t.Fatal("rebase wrote to stdout or stderr")
	}
}

func TestRebaseBaselineNoGRT004(t *testing.T) {
	a, b := abReports(t)
	p := newTestPublisher(t)
	p.publish(ready(a))
	p.publish(ready(b))
	if !strings.Contains(string(p.served.Load().Blast.Text), "GRT004") {
		t.Fatal("view does not hold GRT004")
	}
	if _, err := p.rebase(nil); err != nil {
		t.Fatal(err)
	}
	grt001 := false
	for _, d := range p.base.Diagnostics.All() {
		if d.Code() == diagnostic.CodeRemovedOutput {
			t.Fatalf("baseline holds %v", d)
		}
		grt001 = grt001 || d.Code() == diagnostic.CodeUnknownOutput
	}
	if !p.base.Diagnostics.Equal(b.Diagnostics) || !grt001 {
		t.Fatal("baseline diagnostics are not B's (with GRT001)")
	}
	p.publish(ready(b))
	v := p.served.Load().Blast
	if v.BrokenErrors != 0 || !strings.Contains(string(v.Text), "Broken (0):") || !strings.Contains(string(v.Text), "Impacted (0):") {
		t.Fatalf("view after rebase and republish:\n%s", v.Text)
	}
}

// publisherAllow are the publisher fields that may hold reference-like
// kinds: I/O and clock dependencies, never index state.
var publisherAllow = map[string]bool{"stdout": true, "stderr": true, "now": true, "status": true, "snapshots": true, "printed": true}

// baselineWalk walks every field of a type reachable from root. A field of
// watchPublisher's type is walked with publisherAllow; last (*ipc.Snapshot,
// bytes only) is walked for kinds but exempt from the infrastructure
// package rule. It returns the violations and the visited type names.
func baselineWalk(root reflect.Type) (bad []string, seen map[string]bool) {
	pubT := reflect.TypeFor[watchPublisher]()
	snapT := reflect.TypeFor[ipc.Snapshot]()
	viewT := reflect.TypeFor[ipc.BlastView]()
	seen = map[string]bool{}
	visited := map[reflect.Type]bool{}
	var walk func(tp reflect.Type, path string)
	walk = func(tp reflect.Type, path string) {
		if visited[tp] {
			return
		}
		visited[tp] = true
		seen[tp.String()] = true
		if strings.Contains(tp.PkgPath(), "/internal/infrastructure/") && tp != snapT && tp != viewT {
			bad = append(bad, path+": infrastructure type "+tp.String())
		}
		switch tp.Kind() {
		case reflect.Interface, reflect.Func, reflect.Chan, reflect.UnsafePointer:
			bad = append(bad, path+": "+tp.Kind().String()+" "+tp.String())
		case reflect.Pointer, reflect.Slice, reflect.Array:
			walk(tp.Elem(), path+"[]")
		case reflect.Map:
			walk(tp.Key(), path+"{key}")
			walk(tp.Elem(), path+"{}")
		case reflect.Struct:
			if tp == reflect.TypeFor[sync.Mutex]() {
				return
			}
			for f := range tp.Fields() {
				name := path + "." + f.Name
				seen["field "+name] = true
				if tp == pubT && publisherAllow[f.Name] {
					continue
				}
				walk(f.Type, name)
			}
		}
	}
	walk(root, root.Name())
	return bad, seen
}

func TestBaselineHoldsOnlyReport(t *testing.T) {
	bad, seen := baselineWalk(reflect.TypeFor[watchPublisher]())
	if len(bad) != 0 {
		t.Fatalf("publisher holds reference-like state:\n%s", strings.Join(bad, "\n"))
	}
	for _, s := range []string{"diagnostic.Diagnostic", "repograph.Unit", "repograph.RepositoryGraph", "checking.Report", "ipc.BlastView"} {
		if !seen[s] {
			t.Errorf("walk did not visit %s", s)
		}
	}
	for f := range reflect.TypeFor[watchPublisher]().Fields() {
		if !seen["field watchPublisher."+f.Name] {
			t.Errorf("walk did not visit field %s", f.Name)
		}
	}
	type probe struct {
		P watchPublisher
		S blasting.Sources
	}
	bad, _ = baselineWalk(reflect.TypeFor[probe]())
	if len(bad) == 0 || !strings.Contains(strings.Join(bad, "\n"), "probe.S.") {
		t.Fatalf("walk accepted a publisher next to blasting.Sources: %v", bad)
	}
}

// TestRebaseConcurrentPublish runs with -race in CI: publishes (with an
// EventFailed every 7th event) on one goroutine, rebases on another, and
// readers of the served pointer on a third.
func TestRebaseConcurrentPublish(t *testing.T) {
	a, b := abReports(t)
	p := newTestPublisher(t)
	p.publish(ready(a))
	var bad atomic.Value
	check := func() {
		s := p.served.Load()
		if s == nil || s.Blast == nil {
			bad.Store("snapshot without a blast view")
			return
		}
		if _, ok := p.gens.Load(s.Blast.BaselineGeneration); !ok || s.Blast.BaselineGeneration > s.Generation {
			bad.Store(s.Blast.BaselineGeneration)
		}
	}
	var writers, readers sync.WaitGroup
	stop := make(chan struct{})
	writers.Go(func() {
		for i := range 200 {
			switch {
			case i%7 == 6:
				p.publish(failedEvent)
			case i%2 == 0:
				p.publish(ready(b))
			default:
				p.publish(ready(a))
			}
		}
	})
	writers.Go(func() {
		for range 200 {
			_, _ = p.rebase(nil)
			check()
		}
	})
	readers.Go(func() {
		for {
			select {
			case <-stop:
				return
			default:
				check()
			}
		}
	})
	writers.Wait()
	close(stop)
	readers.Wait()
	if v := bad.Load(); v != nil {
		t.Fatalf("bad snapshot: baseline generation %v not handed on, or ahead of its snapshot", v)
	}
	p.publish(ready(a))
	if _, err := p.rebase(nil); err != nil {
		t.Fatal(err)
	}
	v := p.served.Load().Blast
	if v.BrokenErrors != 0 || !strings.Contains(string(v.Text), "Broken (0):") || !strings.Contains(string(v.Text), "Impacted (0):") {
		t.Fatalf("final view:\n%s", v.Text)
	}
}
