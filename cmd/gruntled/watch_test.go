package main

import (
	"bytes"
	"context"
	"errors"
	"flag"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
	"unicode"

	"github.com/GiulioSavini/gruntled/internal/application/checking"
	"github.com/GiulioSavini/gruntled/internal/domain/diagnostic"
	"github.com/GiulioSavini/gruntled/internal/infrastructure/ipc"
	"github.com/GiulioSavini/gruntled/internal/infrastructure/statusfile"
	"github.com/GiulioSavini/gruntled/internal/infrastructure/terragrunt"
	"github.com/GiulioSavini/gruntled/internal/infrastructure/tfsurface"
	"github.com/GiulioSavini/gruntled/internal/infrastructure/watch"
	"github.com/GiulioSavini/gruntled/internal/interfaces/presenter"
)

// watchTimeout bounds every eventually in this file. Generous on purpose:
// a loaded CI runner must not turn a slow scan into a failure.
const watchTimeout = 10 * time.Second

// watchStamp is the fixed clock every watch test injects, so status lines
// are exact.
var watchStamp = time.Date(2026, 10, 8, 14, 2, 11, 0, time.UTC)

// eventually polls cond every 5 ms until it holds or watchTimeout elapses,
// then fails with what and the last observed state. Never sleep-then-assert.
func eventually(t *testing.T, cond func() bool, what string, state func() string) {
	t.Helper()
	deadline := time.Now().Add(watchTimeout)
	for {
		if cond() {
			return
		}
		if time.Now().After(deadline) {
			last := ""
			if state != nil {
				last = state()
			}
			t.Fatalf("timed out after %v waiting for %s; last state: %s", watchTimeout, what, last)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// syncBuf is a mutex-guarded buffer: the daemon writes from its goroutine,
// the test reads from its own.
type syncBuf struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (s *syncBuf) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.Write(p)
}

func (s *syncBuf) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.String()
}

// daemon is one runWatchWith call running in a goroutine.
type daemon struct {
	t       *testing.T
	stdout  *syncBuf
	stderr  *syncBuf
	status  string
	cancel  context.CancelFunc
	done    chan int
	stopped bool
	code    int
}

// testDeps returns production-like deps with a fixed clock, real adapters
// and a private runtime base, so lock, socket and report dump never touch
// the real runtime directory.
func testDeps(t *testing.T) watchDeps {
	t.Helper()
	d := defaultWatchDeps()
	d.now = func() time.Time { return watchStamp }
	d.maxWait = 50 * time.Millisecond
	d.env = envAt(shortBase(t))
	return d
}

// shortBase is a fresh temp dir with a short name: sun_path is ~104 bytes.
func shortBase(t *testing.T) string {
	t.Helper()
	d, err := os.MkdirTemp("", "g")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(d) })
	return d
}

// envAt resolves every runtime base candidate to base.
func envAt(base string) statusfile.Env {
	return statusfile.Env{
		GOOS: runtime.GOOS,
		Getenv: func(k string) string {
			if k == "XDG_RUNTIME_DIR" {
				return base
			}
			return ""
		},
		UserCacheDir: func() (string, error) { return base, nil },
		TempDir:      func() string { return base },
	}
}

// startWatch runs `watch` on repoDir with a status file in a separate temp
// dir, tiny debounce and poll interval, plus extra args. The daemon is
// stopped on cleanup.
func startWatch(t *testing.T, repoDir string, deps watchDeps, extra ...string) *daemon {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	d := &daemon{
		t:      t,
		stdout: &syncBuf{},
		stderr: &syncBuf{},
		status: filepath.Join(t.TempDir(), "st", "status"),
		cancel: cancel,
		done:   make(chan int, 1),
	}
	args := append([]string{"--status-file", d.status, "--debounce", "5ms", "--poll-interval", "10ms"}, extra...)
	args = append(args, repoDir)
	go func() { d.done <- runWatchWith(ctx, args, d.stdout, d.stderr, deps) }()
	t.Cleanup(func() { d.stop() })
	return d
}

// stop cancels the daemon and returns its exit code.
func (d *daemon) stop() int {
	d.t.Helper()
	if d.stopped {
		return d.code
	}
	d.stopped = true
	d.cancel()
	select {
	case d.code = <-d.done:
	case <-time.After(watchTimeout):
		d.t.Fatalf("daemon did not stop within %v; stderr:\n%s", watchTimeout, d.stderr.String())
	}
	return d.code
}

func (d *daemon) readStatus() string {
	b, err := os.ReadFile(d.status)
	if err != nil {
		return "<" + err.Error() + ">"
	}
	return string(b)
}

// waitStatus waits until the status file holds exactly want.
func (d *daemon) waitStatus(want string) {
	d.t.Helper()
	eventually(d.t, func() bool { return d.readStatus() == want }, "status "+strings.TrimSpace(want), func() string {
		return d.readStatus() + "\nstderr:\n" + d.stderr.String()
	})
}

// freshReport runs `check` over a fresh loader on dir.
func freshReport(t *testing.T, dir string) checking.Report {
	t.Helper()
	fsys := os.DirFS(dir)
	rep, err := checking.Check(context.Background(), terragrunt.NewLoader(fsys), tfsurface.NewReader(fsys))
	if err != nil {
		t.Fatal(err)
	}
	return rep
}

// freshStatus is the status line a fresh `check` of dir produces.
func freshStatus(t *testing.T, dir string) string {
	t.Helper()
	var b bytes.Buffer
	if err := presenter.StatusLine(&b, freshReport(t, dir).Diagnostics, watchStamp.Format("15:04:05")); err != nil {
		t.Fatal(err)
	}
	return b.String()
}

func renderText(t *testing.T, diags diagnostic.Set) string {
	t.Helper()
	var b bytes.Buffer
	if err := presenter.Text(&b, diags); err != nil {
		t.Fatal(err)
	}
	return b.String()
}

const (
	watchOK     = "gruntled: ok @ 14:02:11\n"
	watchOneErr = "gruntled: 1 error (GRT001×1) @ 14:02:11\n"
	watchStop   = "gruntled: stopped @ 14:02:11\n"
	badApp      = "dependency \"vpc\" {\n  config_path = \"../vpc\"\n}\ninputs = { id = dependency.vpc.outputs.vpc_idd }\n"
	goodApp     = "dependency \"vpc\" {\n  config_path = \"../vpc\"\n}\ninputs = { id = dependency.vpc.outputs.vpc_id }\n"
)

func TestWatchStatusFile(t *testing.T) {
	dir := repo(t, "vpc_idd")
	d := startWatch(t, dir, testDeps(t), "--poll")
	d.waitStatus(watchOneErr)

	writeFiles(t, dir, map[string]string{"app/terragrunt.hcl": goodApp})
	d.waitStatus(watchOK)

	if code := d.stop(); code != exitOK {
		t.Fatalf("exit %d, want %d; stderr:\n%s", code, exitOK, d.stderr.String())
	}
	if got := d.readStatus(); got != watchStop {
		t.Fatalf("final status %q, want %q", got, watchStop)
	}
}

func TestWatchReindexesOnlyChanged(t *testing.T) {
	dir := repo(t, "vpc_idd")
	deps := testDeps(t)
	var loader atomic.Pointer[terragrunt.Loader]
	deps.loaderHook = func(l *terragrunt.Loader) { loader.Store(l) }
	d := startWatch(t, dir, deps, "--poll")
	d.waitStatus(watchOneErr)

	l := loader.Load()
	if l == nil {
		t.Fatal("loaderHook was never called")
	}
	_, initialMisses := l.CacheStats()
	if initialMisses < 2 {
		t.Fatalf("initial index parsed %d files, want at least 2", initialMisses)
	}

	writeFiles(t, dir, map[string]string{"app/terragrunt.hcl": goodApp})
	d.waitStatus(watchOK)
	hits, misses := l.CacheStats()
	if misses != 1 || hits != initialMisses-1 {
		t.Fatalf("after one save: hits=%d misses=%d, want hits=%d misses=1", hits, misses, initialMisses-1)
	}
}

// parityOp mutates the repository; every op changes the status line, so
// reaching the expected line proves the daemon reindexed.
type parityOp struct {
	name string
	do   func(t *testing.T, dir string)
}

// retryFS runs a remove or rename on a tree a --poll daemon is reading,
// retrying 20 times 25 ms apart: on windows the daemon's open handle makes
// it fail with a sharing violation for a moment (same as removeAll in
// internal/infrastructure/watch/helpers_test.go).
func retryFS(t *testing.T, op func() error) {
	t.Helper()
	var err error
	for range 20 {
		if err = op(); err == nil {
			return
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatal(err)
}

func mustDo(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

var parityOps = []parityOp{
	{"create unit", func(t *testing.T, dir string) {
		writeFiles(t, dir, map[string]string{
			"db/terragrunt.hcl": "dependency \"vpc\" {\n  config_path = \"../vpc\"\n}\ninputs = { id = dependency.vpc.outputs.subnet }\n",
		})
	}},
	{"edit", func(t *testing.T, dir string) {
		writeFiles(t, dir, map[string]string{"app/terragrunt.hcl": goodApp})
	}},
	{"delete", func(t *testing.T, dir string) {
		retryFS(t, func() error { return os.Remove(filepath.Join(dir, "db", "terragrunt.hcl")) })
	}},
	{"mkdir with new unit", func(t *testing.T, dir string) {
		writeFiles(t, dir, map[string]string{
			"net/edge/terragrunt.hcl": "dependency \"vpc\" {\n  config_path = \"../../vpc\"\n}\ninputs = { id = dependency.vpc.outputs.nope }\n",
		})
	}},
	{"change dependency output", func(t *testing.T, dir string) {
		writeFiles(t, dir, map[string]string{
			"vpc/main.tf": "output \"vpc_id\" { value = \"x\" }\noutput \"nope\" { value = \"y\" }\n",
		})
	}},
	{"rename", func(t *testing.T, dir string) {
		retryFS(t, func() error { return os.Rename(filepath.Join(dir, "vpc"), filepath.Join(dir, "vpc2")) })
	}},
	{"delete dir", func(t *testing.T, dir string) {
		retryFS(t, func() error { return os.RemoveAll(filepath.Join(dir, "net")) })
	}},
}

// TestWatchParity scripts the same filesystem changes against the poll and
// the native backend: after every change both must reach the status line a
// fresh `check` produces.
func TestWatchParity(t *testing.T) {
	type step struct{ op, line string }
	backends := []struct {
		name string
		args []string
	}{
		{"poll", []string{"--poll"}},
		{"native", nil},
	}
	results := map[string][]step{}
	for _, be := range backends {
		t.Run(be.name, func(t *testing.T) {
			if be.name == "native" && runtime.GOOS == "windows" {
				t.Skip("no native watcher on windows")
			}
			dir := repo(t, "vpc_idd")
			d := startWatch(t, dir, testDeps(t), be.args...)
			want := freshStatus(t, dir)
			d.waitStatus(want)
			got := []step{{"initial", want}}
			for _, op := range parityOps {
				prev := want
				op.do(t, dir)
				want = freshStatus(t, dir)
				if want == prev {
					t.Fatalf("op %q leaves the status at %q; the step proves nothing", op.name, want)
				}
				d.waitStatus(want)
				got = append(got, step{op.name, want})
			}
			if code := d.stop(); code != exitOK {
				t.Fatalf("exit %d; stderr:\n%s", code, d.stderr.String())
			}
			results[be.name] = got
		})
	}
	p, n := results["poll"], results["native"]
	if p == nil || n == nil {
		return
	}
	if len(p) != len(n) {
		t.Fatalf("poll took %d steps, native %d", len(p), len(n))
	}
	for i := range p {
		if p[i] != n[i] {
			t.Errorf("step %d: poll %+v, native %+v", i, p[i], n[i])
		}
	}
}

// countingPoll wraps the real poll constructor and records calls.
func countingPoll(calls *atomic.Int32) func(string, time.Duration) (watch.Watcher, error) {
	return func(root string, d time.Duration) (watch.Watcher, error) {
		calls.Add(1)
		return watch.NewPoll(root, d)
	}
}

func TestWatchBackendSelection(t *testing.T) {
	// Windows always polls (deps.goos forces it), so the injected native
	// constructor is never called there: the two native-fallback subtests
	// only apply where a native backend exists.
	nativeOnly := func(t *testing.T) {
		t.Helper()
		if runtime.GOOS == "windows" {
			t.Skip("no native watcher on windows")
		}
	}
	t.Run("watch limit falls back to polling", func(t *testing.T) {
		nativeOnly(t)
		var nativeCalls, pollCalls atomic.Int32
		deps := testDeps(t)
		deps.newNative = func(string) (watch.Watcher, error) {
			nativeCalls.Add(1)
			return nil, watch.ErrWatchLimit
		}
		deps.newPoll = countingPoll(&pollCalls)
		dir := repo(t, "vpc_idd")
		d := startWatch(t, dir, deps)
		d.waitStatus(watchOneErr)
		writeFiles(t, dir, map[string]string{"app/terragrunt.hcl": goodApp})
		d.waitStatus(watchOK)
		if nativeCalls.Load() != 1 || pollCalls.Load() != 1 {
			t.Fatalf("native calls %d, poll calls %d; want 1 and 1", nativeCalls.Load(), pollCalls.Load())
		}
		if !strings.Contains(d.stderr.String(), "falling back to polling") {
			t.Fatalf("stderr has no fallback notice:\n%s", d.stderr.String())
		}
	})

	t.Run("other native error is exit 3", func(t *testing.T) {
		nativeOnly(t)
		deps := testDeps(t)
		deps.newNative = func(string) (watch.Watcher, error) { return nil, errors.New("boom") }
		var stdout, stderr bytes.Buffer
		status := filepath.Join(t.TempDir(), "status")
		code := runWatchWith(context.Background(), []string{"--status-file", status, repo(t, "vpc_id")}, &stdout, &stderr, deps)
		if code != exitFailure {
			t.Fatalf("exit %d, want %d; stderr:\n%s", code, exitFailure, stderr.String())
		}
	})

	forced := []struct {
		name string
		goos string
		args []string
	}{
		{"--poll never calls native", runtime.GOOS, []string{"--poll"}},
		{"windows forces polling", "windows", nil},
	}
	for _, tc := range forced {
		t.Run(tc.name, func(t *testing.T) {
			var nativeCalls, pollCalls atomic.Int32
			deps := testDeps(t)
			deps.goos = tc.goos
			deps.newNative = func(string) (watch.Watcher, error) {
				nativeCalls.Add(1)
				return nil, errors.New("native must not be called")
			}
			deps.newPoll = countingPoll(&pollCalls)
			d := startWatch(t, repo(t, "vpc_idd"), deps, tc.args...)
			d.waitStatus(watchOneErr)
			if nativeCalls.Load() != 0 || pollCalls.Load() != 1 {
				t.Fatalf("native calls %d, poll calls %d; want 0 and 1", nativeCalls.Load(), pollCalls.Load())
			}
			if strings.Contains(d.stderr.String(), "falling back") {
				t.Fatalf("forced polling printed a fallback notice:\n%s", d.stderr.String())
			}
		})
	}
}

func TestWatchStdoutOnChange(t *testing.T) {
	dir := repo(t, "vpc_idd")
	deps := testDeps(t)
	var loader atomic.Pointer[terragrunt.Loader]
	deps.loaderHook = func(l *terragrunt.Loader) { loader.Store(l) }
	d := startWatch(t, dir, deps, "--poll")
	d.waitStatus(watchOneErr)
	first := renderText(t, freshReport(t, dir).Diagnostics)
	if first == "" {
		t.Fatal("broken fixture renders no diagnostics; the test proves nothing")
	}
	eventually(t, func() bool { return d.stdout.String() == first }, "first rendering on stdout", d.stdout.String)

	// No-op save: identical content, mtime bumped so polling sees it.
	app := filepath.Join(dir, "app", "terragrunt.hcl")
	writeFiles(t, dir, map[string]string{"app/terragrunt.hcl": badApp})
	future := time.Now().Add(time.Hour)
	mustDo(t, os.Chtimes(app, future, future))
	eventually(t, func() bool {
		_, misses := loader.Load().CacheStats()
		return misses == 1
	}, "reindex of the no-op save", func() string { return d.stderr.String() })

	// Sentinel: a change that alters the set.
	writeFiles(t, dir, map[string]string{
		"db/terragrunt.hcl": "dependency \"vpc\" {\n  config_path = \"../vpc\"\n}\ninputs = { id = dependency.vpc.outputs.subnet }\n",
	})
	second := renderText(t, freshReport(t, dir).Diagnostics)
	d.waitStatus("gruntled: 2 errors (GRT001×2) @ 14:02:11\n")
	if code := d.stop(); code != exitOK {
		t.Fatalf("exit %d; stderr:\n%s", code, d.stderr.String())
	}
	if got, want := d.stdout.String(), first+second; got != want {
		t.Fatalf("stdout:\n%s\nwant exactly two renderings:\n%s", got, want)
	}
	abs, err := filepath.Abs(dir)
	mustDo(t, err)
	real, err := filepath.EvalSymlinks(abs)
	mustDo(t, err)
	for _, p := range []string{abs, real} {
		if strings.Contains(d.stdout.String(), p) {
			t.Fatalf("stdout contains the absolute root %q:\n%s", p, d.stdout.String())
		}
	}
}

func TestWatchQuietRepoPrintsNothing(t *testing.T) {
	d := startWatch(t, repo(t, "vpc_id"), testDeps(t), "--poll")
	d.waitStatus(watchOK)
	if code := d.stop(); code != exitOK {
		t.Fatalf("exit %d; stderr:\n%s", code, d.stderr.String())
	}
	if got := d.stdout.String(); got != "" {
		t.Fatalf("stdout %q, want empty", got)
	}
}

func TestWatchDebounceDefault(t *testing.T) {
	if watch.DefaultQuiet != 150*time.Millisecond {
		t.Fatalf("watch.DefaultQuiet = %v, want 150ms", watch.DefaultQuiet)
	}
	fs := flag.NewFlagSet("watch", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	o := defineWatchFlags(fs)
	if err := fs.Parse(nil); err != nil {
		t.Fatal(err)
	}
	if *o.debounce != watch.DefaultQuiet {
		t.Fatalf("default --debounce %v, want %v", *o.debounce, watch.DefaultQuiet)
	}
	if *o.pollInterval != watch.DefaultPollInterval {
		t.Fatalf("default --poll-interval %v, want %v", *o.pollInterval, watch.DefaultPollInterval)
	}
	// The help text spells the defaults out; keep it in step.
	for _, want := range []string{"(default " + watch.DefaultQuiet.String() + ")", "(default " + watch.DefaultPollInterval.String() + ")"} {
		if !strings.Contains(watchUsage, want) {
			t.Errorf("watchUsage does not mention %q", want)
		}
	}
}

func TestWatchStatusInsideRepo(t *testing.T) {
	dir := repo(t, "vpc_id")
	check := func(t *testing.T, statusArg, statusAbs string) {
		t.Helper()
		var stdout, stderr bytes.Buffer
		code := runWatchWith(context.Background(), []string{"--status-file", statusArg, dir}, &stdout, &stderr, testDeps(t))
		if code != exitUsage {
			t.Fatalf("exit %d, want %d; stderr:\n%s", code, exitUsage, stderr.String())
		}
		if !strings.Contains(stderr.String(), "inside the repository") {
			t.Fatalf("stderr does not explain the rejection:\n%s", stderr.String())
		}
		if _, err := os.Stat(statusAbs); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("status file %s exists or stat failed: %v", statusAbs, err)
		}
	}

	t.Run("absolute", func(t *testing.T) {
		p := filepath.Join(dir, "s")
		check(t, p, p)
	})
	t.Run("relative", func(t *testing.T) {
		t.Chdir(dir)
		check(t, filepath.Join("sub", "s"), filepath.Join(dir, "sub", "s"))
	})
	t.Run("symlink into the repo", func(t *testing.T) {
		link := filepath.Join(t.TempDir(), "link")
		if err := os.Symlink(dir, link); err != nil {
			t.Skipf("cannot create symlink: %v", err)
		}
		check(t, filepath.Join(link, "s"), filepath.Join(dir, "s"))
	})
}

// TestRenderSnapshotParity: the daemon's snapshot bytes equal what check
// writes for every format, and Summary equals check's text-mode stderr.
func TestRenderSnapshotParity(t *testing.T) {
	for _, fixture := range []string{"testdata/sarif-fixture", "testdata/clean-fixture"} {
		rep := freshReport(t, fixture)
		snap, err := buildSnapshot(rep, 7)
		if err != nil {
			t.Fatal(err)
		}
		if snap.State != ipc.StateReady || snap.Generation != 7 || snap.HasErrors != rep.Diagnostics.HasErrors() {
			t.Fatalf("%s: snapshot state %q gen %d hasErrors %v", fixture, snap.State, snap.Generation, snap.HasErrors)
		}
		for _, f := range []struct {
			format string
			got    []byte
		}{{"text", snap.Text}, {"json", snap.JSON}, {"sarif", snap.SARIF}} {
			t.Run(fixture+"/"+f.format, func(t *testing.T) {
				var stdout, stderr bytes.Buffer
				code := run([]string{"check", "--format", f.format, fixture}, &stdout, &stderr)
				if code != exitOK && code != exitFindings {
					t.Fatalf("check exit %d; stderr:\n%s", code, stderr.String())
				}
				if !bytes.Equal(f.got, stdout.Bytes()) {
					t.Fatalf("snapshot %s differs from check stdout:\n%s\nwant:\n%s", f.format, f.got, stdout.Bytes())
				}
				if f.format == "text" && !bytes.Equal(snap.Summary, stderr.Bytes()) {
					t.Fatalf("snapshot summary %q, check stderr %q", snap.Summary, stderr.Bytes())
				}
			})
		}
	}
}

func TestRenderReportFormats(t *testing.T) {
	rep := freshReport(t, "testdata/sarif-fixture")
	for _, f := range []string{"json", "sarif"} {
		out, sum, err := renderReport(f, rep)
		if err != nil || len(out) == 0 || sum != nil {
			t.Fatalf("%s: out %d bytes, summary %q, err %v", f, len(out), sum, err)
		}
	}
	if _, sum, err := renderReport("text", rep); err != nil || len(sum) == 0 {
		t.Fatalf("text: summary %q, err %v", sum, err)
	}
	if _, _, err := renderReport("yaml", rep); err == nil {
		t.Fatal("unknown format rendered without error")
	}
}

// TestWatchFailedReasonSanitised: a reindex failure after a ready index
// reaches stderr without control runes.
func TestWatchFailedReasonSanitised(t *testing.T) {
	var stdout, stderr bytes.Buffer
	p := &watchPublisher{
		stdout: &stdout,
		stderr: &stderr,
		now:    func() time.Time { return watchStamp },
		status: statusfile.NewWriter(filepath.Join(t.TempDir(), "st", "status")),
	}
	p.publish(watch.Event{Kind: watch.EventReady, Report: freshReport(t, repo(t, "vpc_id"))})
	p.publish(watch.Event{Kind: watch.EventFailed, Err: errors.New("bad \x1b[31mred\x1b[0m\r\nline\x07")})
	got := stderr.String()
	if !strings.Contains(got, "reindex failed: bad") {
		t.Fatalf("stderr has no reindex failure line:\n%q", got)
	}
	for _, r := range strings.TrimSuffix(got, "\n") {
		if unicode.IsControl(r) {
			t.Fatalf("stderr has control rune %U:\n%q", r, got)
		}
	}
}
