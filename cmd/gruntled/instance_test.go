package main

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/GiulioSavini/gruntled/internal/infrastructure/ipc"
	"github.com/GiulioSavini/gruntled/internal/infrastructure/statusfile"
	"github.com/GiulioSavini/gruntled/internal/infrastructure/terragrunt"
	"github.com/GiulioSavini/gruntled/internal/infrastructure/watch"
)

// repoRuntime returns the canonical root of repoDir and its runtime dir
// under deps.env.
func repoRuntime(t *testing.T, repoDir string, deps watchDeps) (root, dir string) {
	t.Helper()
	abs, err := filepath.Abs(repoDir)
	mustDo(t, err)
	root, err = filepath.EvalSymlinks(abs)
	mustDo(t, err)
	dir, err = statusfile.Dir(root, deps.env)
	mustDo(t, err)
	return root, dir
}

// readDump returns the dump at path, or an error string for eventually.
func dumpState(path string) string {
	_, s, err := ipc.ReadDump(path)
	switch {
	case err != nil:
		return "<" + err.Error() + ">"
	case s == nil:
		return ipc.StateIndexing
	default:
		return s.State
	}
}

// assertLockFree fails unless the instance lock in dir can be taken now.
func assertLockFree(t *testing.T, dir string) {
	t.Helper()
	l, err := ipc.TryLock(filepath.Join(dir, ipc.LockName))
	if err != nil {
		t.Fatalf("lock not released: %v", err)
	}
	mustDo(t, l.Release())
}

// windowsDeps runs the daemon in report-file mode (no socket), whatever
// the host.
func windowsDeps(t *testing.T) watchDeps {
	d := testDeps(t)
	d.goos = "windows"
	return d
}

func TestWatchDumpMode(t *testing.T) {
	dir := repo(t, "vpc_idd")
	deps := windowsDeps(t)
	entered, release := make(chan struct{}), make(chan struct{})
	deps.loaderHook = func(*terragrunt.Loader) {
		close(entered)
		<-release
	}
	_, rt := repoRuntime(t, dir, deps)
	dump := filepath.Join(rt, ipc.DumpName)

	d := startWatch(t, dir, deps)
	<-entered
	// Before the first index the dump says indexing and carries no snapshot.
	info, s, err := ipc.ReadDump(dump)
	if err != nil || s != nil {
		t.Fatalf("dump before first index: snapshot %v, err %v", s, err)
	}
	if info.PID != os.Getpid() || info.StatusPath != d.status || info.Version != version {
		t.Fatalf("dump info %+v", info)
	}
	if _, err := os.Stat(filepath.Join(rt, ipc.SockName)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("windows mode created a socket: %v", err)
	}
	close(release)
	d.waitStatus(watchOneErr)
	eventually(t, func() bool { return dumpState(dump) == ipc.StateReady }, "ready dump", func() string { return dumpState(dump) })
	_, s, err = ipc.ReadDump(dump)
	mustDo(t, err)
	want, err := buildSnapshot(freshReport(t, dir), 1)
	mustDo(t, err)
	assertSnapshot(t, s, want)

	if code := d.stop(); code != exitOK {
		t.Fatalf("exit %d; stderr:\n%s", code, d.stderr.String())
	}
	if _, err := os.Stat(dump); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("dump not removed on clean exit: %v", err)
	}
	assertLockFree(t, rt)
}

// assertSnapshot compares the rendered bytes and flags of two snapshots.
func assertSnapshot(t *testing.T, got, want *ipc.Snapshot) {
	t.Helper()
	if got == nil {
		t.Fatal("snapshot is nil")
	}
	if got.State != want.State || got.HasErrors != want.HasErrors || got.Generation != want.Generation || got.LastError != want.LastError {
		t.Fatalf("snapshot state %q hasErrors %v gen %d lastError %q, want %q %v %d %q",
			got.State, got.HasErrors, got.Generation, got.LastError, want.State, want.HasErrors, want.Generation, want.LastError)
	}
	for _, f := range []struct {
		name      string
		got, want []byte
	}{{"text", got.Text, want.Text}, {"summary", got.Summary, want.Summary}, {"json", got.JSON, want.JSON}, {"sarif", got.SARIF, want.SARIF}} {
		if !bytes.Equal(f.got, f.want) {
			t.Fatalf("snapshot %s:\n%s\nwant:\n%s", f.name, f.got, f.want)
		}
	}
}

func TestWatchSecondInstanceDumpMode(t *testing.T) {
	dir := repo(t, "vpc_idd")
	deps := windowsDeps(t)
	root, rt := repoRuntime(t, dir, deps)
	dump := filepath.Join(rt, ipc.DumpName)
	a := startWatch(t, dir, deps)
	a.waitStatus(watchOneErr)
	eventually(t, func() bool { return dumpState(dump) == ipc.StateReady }, "ready dump", func() string { return dumpState(dump) })

	bDeps := deps
	var watchers atomic.Int32
	bDeps.newPoll = func(string, time.Duration) (watch.Watcher, error) {
		watchers.Add(1)
		return nil, errors.New("second instance must not start a watcher")
	}
	var sleeps []time.Duration
	bDeps.sleep = func(d time.Duration) { sleeps = append(sleeps, d) }
	var stdout, stderr bytes.Buffer
	other := filepath.Join(t.TempDir(), "other", "status")
	code := runWatchWith(context.Background(), []string{"--status-file", other, dir}, &stdout, &stderr, bDeps)
	if code != exitOK {
		t.Fatalf("exit %d, want %d; stderr:\n%s", code, exitOK, stderr.String())
	}
	want := "gruntled: already watching " + root + "; report: " + dump + "; status: " + a.status + "\n"
	if stdout.String() != want {
		t.Fatalf("stdout %q, want %q", stdout.String(), want)
	}
	if watchers.Load() != 0 {
		t.Fatal("second instance started a watcher")
	}
	if len(sleeps) != 2 {
		t.Fatalf("lock retried with %d sleeps, want 2 (3 attempts)", len(sleeps))
	}
	if _, err := os.Stat(other); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("second instance wrote its status file: %v", err)
	}

	writeFiles(t, dir, map[string]string{"app/terragrunt.hcl": goodApp})
	a.waitStatus(watchOK)
}

func TestSockPathTooLong(t *testing.T) {
	long := filepath.Join(shortBase(t), strings.Repeat("d", 60), strings.Repeat("e", 60))
	dir := repo(t, "vpc_id")

	t.Run("unix is exit 3", func(t *testing.T) {
		deps := testDeps(t)
		deps.goos = "linux"
		deps.env = envAt(long)
		_, rt := repoRuntime(t, dir, deps)
		sock := filepath.Join(rt, ipc.SockName)
		if len(sock) <= ipc.MaxSockPath {
			t.Fatalf("socket path %d bytes; the test proves nothing", len(sock))
		}
		var watchers atomic.Int32
		deps.newNative = func(string) (watch.Watcher, error) { watchers.Add(1); return nil, errors.New("no") }
		deps.newPoll = func(string, time.Duration) (watch.Watcher, error) { watchers.Add(1); return nil, errors.New("no") }
		var stdout, stderr bytes.Buffer
		code := runWatchWith(context.Background(), []string{"--status-file", filepath.Join(t.TempDir(), "s"), dir}, &stdout, &stderr, deps)
		if code != exitFailure {
			t.Fatalf("exit %d, want %d; stderr:\n%s", code, exitFailure, stderr.String())
		}
		for _, w := range []string{strconv.Itoa(len(sock)) + " bytes", "XDG_RUNTIME_DIR"} {
			if !strings.Contains(stderr.String(), w) {
				t.Fatalf("stderr lacks %q:\n%s", w, stderr.String())
			}
		}
		if watchers.Load() != 0 {
			t.Fatal("watcher started before the single-instance check")
		}
		ents, err := os.ReadDir(rt)
		mustDo(t, err)
		if len(ents) != 0 {
			t.Fatalf("runtime dir has %d entries, want none", len(ents))
		}
	})

	t.Run("windows accepts it", func(t *testing.T) {
		deps := windowsDeps(t)
		deps.env = envAt(long)
		_, rt := repoRuntime(t, dir, deps)
		d := startWatch(t, dir, deps)
		d.waitStatus(watchOK)
		dump := filepath.Join(rt, ipc.DumpName)
		eventually(t, func() bool { return dumpState(dump) == ipc.StateReady }, "ready dump", func() string { return dumpState(dump) })
		if code := d.stop(); code != exitOK {
			t.Fatalf("exit %d; stderr:\n%s", code, d.stderr.String())
		}
	})
}

// TestWatchRuntimeDirInsideRepo: a runtime base inside the repository would
// put the lock, socket and report file in the watched tree (the report
// rewrite after every index is itself a change: endless reindex). watch
// refuses with exit 2 before creating anything or starting a watcher.
func TestWatchRuntimeDirInsideRepo(t *testing.T) {
	for _, mode := range []struct {
		name string
		deps func(*testing.T) watchDeps
	}{{"socket", testDeps}, {"report-file", windowsDeps}} {
		t.Run(mode.name, func(t *testing.T) {
			dir := repo(t, "vpc_id")
			deps := mode.deps(t)
			deps.env = envAt(filepath.Join(dir, "cache"))
			root, _ := repoRuntime(t, dir, deps)
			var watchers atomic.Int32
			deps.newNative = func(string) (watch.Watcher, error) { watchers.Add(1); return nil, errors.New("no") }
			deps.newPoll = func(string, time.Duration) (watch.Watcher, error) { watchers.Add(1); return nil, errors.New("no") }
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			var stdout, stderr bytes.Buffer
			code := runWatchWith(ctx, []string{"--status-file", filepath.Join(t.TempDir(), "s"), dir}, &stdout, &stderr, deps)
			if code != exitUsage {
				t.Fatalf("exit %d, want %d; stderr:\n%s", code, exitUsage, stderr.String())
			}
			for _, w := range []string{"runtime directory", "inside the repository", root} {
				if !strings.Contains(stderr.String(), w) {
					t.Fatalf("stderr lacks %q:\n%s", w, stderr.String())
				}
			}
			if watchers.Load() != 0 {
				t.Fatal("watcher started despite the refusal")
			}
			if _, err := os.Stat(filepath.Join(dir, "cache")); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("runtime base created inside the repo: %v", err)
			}
			mustDo(t, filepath.WalkDir(dir, func(p string, _ os.DirEntry, err error) error {
				if err != nil {
					return err
				}
				switch filepath.Base(p) {
				case ipc.LockName, ipc.SockName, ipc.DumpName:
					t.Errorf("daemon state %s created inside the repo", p)
				}
				return nil
			}))
		})
	}
}

func TestPrintStatusPathTakesNoLock(t *testing.T) {
	dir := repo(t, "vpc_id")
	deps := testDeps(t)
	_, rt := repoRuntime(t, dir, deps)
	var stdout, stderr bytes.Buffer
	if code := runWatchWith(context.Background(), []string{"--print-status-path", dir}, &stdout, &stderr, deps); code != exitOK {
		t.Fatalf("exit %d; stderr:\n%s", code, stderr.String())
	}
	if _, err := os.Stat(rt); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("--print-status-path created the runtime dir: %v", err)
	}
}

// TestPublisherSnapshots drives the publisher directly: ready stores the
// pre-rendered snapshot; a later failure keeps those bytes, marks the
// snapshot failed and sanitises the reason; the dump follows every change.
func TestPublisherSnapshots(t *testing.T) {
	dir := repo(t, "vpc_idd")
	rt := filepath.Join(shortBase(t), "rt")
	mustDo(t, statusfile.EnsureDir(rt))
	inst := &instance{dump: filepath.Join(rt, ipc.DumpName), info: ipc.Info{PID: 1, Root: dir}}
	inst.dumpW = statusfile.NewWriter(inst.dump)
	var stdout, stderr bytes.Buffer
	p := &watchPublisher{
		stdout:    &stdout,
		stderr:    &stderr,
		now:       func() time.Time { return watchStamp },
		status:    statusfile.NewWriter(filepath.Join(rt, "status")),
		snapshots: inst.store,
	}
	if inst.snap.Load() != nil {
		t.Fatal("snapshot before any index")
	}
	rep := freshReport(t, dir)
	p.publish(watch.Event{Kind: watch.EventReady, Report: rep})
	want, err := buildSnapshot(rep, 1)
	mustDo(t, err)
	assertSnapshot(t, inst.snap.Load(), want)
	_, s, err := ipc.ReadDump(inst.dump)
	mustDo(t, err)
	assertSnapshot(t, s, want)

	p.publish(watch.Event{Kind: watch.EventFailed, Err: errors.New("parse \x1b[2Jboom\r\n")})
	failed := *want
	failed.State = ipc.StateFailed
	failed.LastError = "parse [2Jboom"
	assertSnapshot(t, inst.snap.Load(), &failed)
	_, s, err = ipc.ReadDump(inst.dump)
	mustDo(t, err)
	assertSnapshot(t, s, &failed)

	p.publish(watch.Event{Kind: watch.EventReady, Report: rep})
	want.Generation = 2
	assertSnapshot(t, inst.snap.Load(), want)
}

func skipNoSocket(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("no unix sockets")
	}
}

// newInstance acquires an instance for dir with deps and closes it at
// cleanup; it returns the instance and its socket path.
func newInstance(t *testing.T, dir string, deps watchDeps) (*instance, string) {
	t.Helper()
	root, rt := repoRuntime(t, dir, deps)
	var out, errb bytes.Buffer
	inst, code, done := acquireInstance(root, filepath.Join(t.TempDir(), "status"), deps, &out, &errb)
	if done {
		t.Fatalf("acquireInstance: exit %d, stderr %s", code, errb.String())
	}
	t.Cleanup(inst.close)
	return inst, filepath.Join(rt, ipc.SockName)
}

func TestInstanceRebaseHook(t *testing.T) {
	t.Run("socket", func(t *testing.T) {
		skipNoSocket(t)
		a, _ := abReports(t)
		inst, sock := newInstance(t, repo(t, "vpc_id"), testDeps(t))
		_, _, err := ipc.Rebase(sock, nil, queryTimeoutAll)
		wantRefused(t, err, ipc.CodeIndexing)

		p := newTestPublisher(t)
		p.snapshots = inst.store
		inst.setRebase(p.rebase)
		_, _, err = ipc.Rebase(sock, nil, queryTimeoutAll)
		wantRefused(t, err, ipc.CodeIndexing)
		p.publish(ready(a))
		_, res, err := ipc.Rebase(sock, nil, queryTimeoutAll)
		if err != nil || res != (ipc.RebaseResult{Generation: 1, BrokenErrors: 0}) {
			t.Fatalf("Rebase = (%+v, %v)", res, err)
		}
	})
	t.Run("report file", func(t *testing.T) {
		a, _ := abReports(t)
		deps := testDeps(t)
		deps.goos = "windows"
		inst, _ := newInstance(t, repo(t, "vpc_id"), deps)
		p := newTestPublisher(t)
		p.snapshots = inst.store
		inst.setRebase(p.rebase)
		p.publish(ready(a))
		before, err := os.ReadFile(inst.dump)
		mustDo(t, err)
		_, err = inst.Rebase(nil)
		wantRefused(t, err, ipc.CodeUnsupported)
		after, err := os.ReadFile(inst.dump)
		mustDo(t, err)
		if !bytes.Equal(before, after) || p.baseSource != sourceWatchStart {
			t.Fatal("refused rebase changed the dump or the baseline")
		}
	})
}

const queryTimeoutAll = 10 * time.Second

func TestInstanceStoreSeals(t *testing.T) {
	a, b := abReports(t)
	t.Run("socket", func(t *testing.T) {
		skipNoSocket(t)
		inst, sock := newInstance(t, repo(t, "vpc_id"), testDeps(t))
		p := newTestPublisher(t)
		p.snapshots = inst.store
		p.publish(ready(a))
		p.publish(ready(b))
		s := inst.snap.Load()
		if s == nil || !s.Sealed() {
			t.Fatal("served snapshot is not sealed")
		}
		_, got, err := ipc.Query(sock, ipc.OpBlast, queryTimeoutAll)
		if err != nil || !bytes.Equal(got.Blast.Text, s.Blast.Text) || !bytes.Equal(got.Blast.JSON, s.Blast.JSON) || got.Blast.BrokenErrors != s.Blast.BrokenErrors {
			t.Fatalf("Query(blast) = (%+v, %v)", got, err)
		}
	})
	t.Run("report file", func(t *testing.T) {
		deps := testDeps(t)
		deps.goos = "windows"
		inst, _ := newInstance(t, repo(t, "vpc_id"), deps)
		p := newTestPublisher(t)
		p.snapshots = inst.store
		p.publish(ready(a))
		p.publish(ready(b))
		s := inst.snap.Load()
		_, got, err := ipc.ReadDump(inst.dump)
		if err != nil || !s.Sealed() || got.Blast == nil || !bytes.Equal(got.Blast.Text, s.Blast.Text) || !bytes.Equal(got.Blast.JSON, s.Blast.JSON) {
			t.Fatalf("dump blast = (%+v, %v)", got, err)
		}
	})
}

// runtimeListing is the names, sizes and modes in dir.
func runtimeListing(t *testing.T, dir string) string {
	t.Helper()
	ents, err := os.ReadDir(dir)
	mustDo(t, err)
	var b strings.Builder
	for _, e := range ents {
		fi, err := e.Info()
		mustDo(t, err)
		b.WriteString(e.Name() + " " + strconv.FormatInt(fi.Size(), 10) + " " + fi.Mode().String() + "\n")
	}
	return b.String()
}

func TestDaemonRebaseSocket(t *testing.T) {
	skipNoSocket(t)
	dir := repo(t, "vpc_id")
	deps := testDeps(t)
	_, rt := repoRuntime(t, dir, deps)
	sock := filepath.Join(rt, ipc.SockName)
	d := startWatch(t, dir, deps)
	var gen uint64
	eventually(t, func() bool {
		_, s, err := ipc.Query(sock, ipc.OpReport, queryTimeoutAll)
		if err == nil && s != nil {
			gen = s.Generation
			return true
		}
		return false
	}, "a report", func() string { return d.stderr.String() })
	status, err := os.ReadFile(d.status)
	mustDo(t, err)
	listing := runtimeListing(t, rt)

	_, res, err := ipc.Rebase(sock, nil, queryTimeoutAll)
	if err != nil || res.Generation != gen {
		t.Fatalf("Rebase = (%+v, %v), want generation %d", res, err, gen)
	}
	_, s, err := ipc.Query(sock, ipc.OpBlast, queryTimeoutAll)
	if err != nil || !strings.HasPrefix(string(s.Blast.Text), "baseline: daemon generation "+strconv.FormatUint(gen, 10)+" (rebase), current generation ") {
		t.Fatalf("Query(blast) = (%v, %v)", s, err)
	}
	after, err := os.ReadFile(d.status)
	mustDo(t, err)
	if !bytes.Equal(status, after) || runtimeListing(t, rt) != listing {
		t.Fatal("rebase changed the status file or the runtime directory")
	}
}

func TestShutdownWithRebaseInFlight(t *testing.T) {
	skipNoSocket(t)
	dir := repo(t, "vpc_id")
	deps := testDeps(t)
	_, rt := repoRuntime(t, dir, deps)
	sock := filepath.Join(rt, ipc.SockName)
	d := startWatch(t, dir, deps)
	waitPingAny(t, sock)
	stop := make(chan struct{})
	var loops atomic.Int32
	go func() {
		for {
			select {
			case <-stop:
				return
			default:
				_, _, _ = ipc.Rebase(sock, nil, time.Second)
				loops.Add(1)
			}
		}
	}()
	defer close(stop)
	eventually(t, func() bool { return loops.Load() > 5 }, "rebase calls", nil)
	d.cancel()
	select {
	case code := <-d.done:
		d.stopped, d.code = true, code
		if code != exitOK {
			t.Fatalf("exit %d; stderr:\n%s", code, d.stderr.String())
		}
	case <-time.After(5 * time.Second):
		t.Fatal("daemon did not exit within 5 s with rebases in flight")
	}
}

// waitPingAny waits until something answers a ping on sock.
func waitPingAny(t *testing.T, sock string) {
	t.Helper()
	eventually(t, func() bool {
		_, _, err := ipc.Query(sock, ipc.OpPing, queryTimeoutAll)
		return err == nil
	}, "ping", nil)
}
