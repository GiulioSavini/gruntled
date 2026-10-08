//go:build linux || darwin

package main

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/GiulioSavini/gruntled/internal/infrastructure/ipc"
	"github.com/GiulioSavini/gruntled/internal/infrastructure/statusfile"
	"github.com/GiulioSavini/gruntled/internal/infrastructure/terragrunt"
	"github.com/GiulioSavini/gruntled/internal/infrastructure/watch"
)

const queryTimeout = 2 * time.Second

// waitPing waits until the daemon answers a ping on sock.
func waitPing(t *testing.T, sock string) ipc.Info {
	t.Helper()
	var info ipc.Info
	var last error
	eventually(t, func() bool {
		info, _, last = ipc.Query(sock, ipc.OpPing, queryTimeout)
		return last == nil
	}, "ping on "+sock, func() string { return strconv.Quote(errString(last)) })
	return info
}

func errString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func TestWatchSecondInstance(t *testing.T) {
	dir := repo(t, "vpc_idd")
	deps := testDeps(t)
	root, rt := repoRuntime(t, dir, deps)
	sock := filepath.Join(rt, ipc.SockName)
	a := startWatch(t, dir, deps, "--poll")
	info := waitPing(t, sock)
	if info.PID != os.Getpid() || info.Root != root || info.StatusPath != a.status {
		t.Fatalf("ping info %+v", info)
	}

	bDeps := deps
	var watchers atomic.Int32
	count := func() { watchers.Add(1) }
	bDeps.newNative = func(string) (watch.Watcher, error) { count(); return nil, errors.New("no") }
	bDeps.newPoll = func(string, time.Duration) (watch.Watcher, error) { count(); return nil, errors.New("no") }
	var stdout, stderr bytes.Buffer
	other := filepath.Join(t.TempDir(), "other", "status")
	code := runWatchWith(context.Background(), []string{"--status-file", other, dir}, &stdout, &stderr, bDeps)
	if code != exitOK {
		t.Fatalf("exit %d, want %d; stderr:\n%s", code, exitOK, stderr.String())
	}
	want := "gruntled: already watching " + root + " (pid " + strconv.Itoa(os.Getpid()) + "); socket: " + sock + "; status: " + a.status + "\n"
	if stdout.String() != want {
		t.Fatalf("stdout %q, want %q", stdout.String(), want)
	}
	if watchers.Load() != 0 {
		t.Fatal("second instance started a watcher")
	}

	// A is unaffected: it still answers and still reindexes.
	a.waitStatus(watchOneErr)
	writeFiles(t, dir, map[string]string{"app/terragrunt.hcl": goodApp})
	a.waitStatus(watchOK)
	waitPing(t, sock)

	if code := a.stop(); code != exitOK {
		t.Fatalf("exit %d; stderr:\n%s", code, a.stderr.String())
	}
	if _, err := os.Lstat(sock); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("socket not removed on clean exit: %v", err)
	}
	assertLockFree(t, rt)
}

// TestWatchSecondInstanceNoPing: when the holder never answers, the line
// names the socket without a pid and the retries go through deps.sleep.
func TestWatchSecondInstanceNoPing(t *testing.T) {
	dir := repo(t, "vpc_id")
	deps := testDeps(t)
	root, rt := repoRuntime(t, dir, deps)
	mustDo(t, os.MkdirAll(rt, 0o700))
	l, err := ipc.TryLock(filepath.Join(rt, ipc.LockName))
	mustDo(t, err)
	defer l.Release()

	var sleeps atomic.Int32
	deps.sleep = func(time.Duration) { sleeps.Add(1) }
	var stdout, stderr bytes.Buffer
	if code := runWatchWith(context.Background(), []string{dir}, &stdout, &stderr, deps); code != exitOK {
		t.Fatalf("exit %d; stderr:\n%s", code, stderr.String())
	}
	want := "gruntled: already watching " + root + "; socket: " + filepath.Join(rt, ipc.SockName) + "\n"
	if stdout.String() != want {
		t.Fatalf("stdout %q, want %q", stdout.String(), want)
	}
	if sleeps.Load() != 2 {
		t.Fatalf("ping retried with %d sleeps, want 2", sleeps.Load())
	}
}

// staleSocket leaves a bound socket file at path with no listener.
func staleSocket(t *testing.T, path string) {
	t.Helper()
	fd, err := syscall.Socket(syscall.AF_UNIX, syscall.SOCK_STREAM, 0)
	mustDo(t, err)
	mustDo(t, syscall.Bind(fd, &syscall.SockaddrUnix{Name: path}))
	mustDo(t, syscall.Close(fd))
	fi, err := os.Lstat(path)
	mustDo(t, err)
	if fi.Mode().Type() != os.ModeSocket {
		t.Fatalf("%s is %v, want a socket", path, fi.Mode().Type())
	}
}

func TestStaleRecovery(t *testing.T) {
	t.Run("stale socket and unheld lock", func(t *testing.T) {
		dir := repo(t, "vpc_idd")
		deps := testDeps(t)
		_, rt := repoRuntime(t, dir, deps)
		_, err := statusfile.EnsureRepoDir(dir, deps.env)
		mustDo(t, err)
		sock := filepath.Join(rt, ipc.SockName)
		staleSocket(t, sock)
		mustDo(t, os.WriteFile(filepath.Join(rt, ipc.LockName), nil, 0o600))
		if _, _, err := ipc.Query(sock, ipc.OpPing, queryTimeout); !errors.Is(err, ipc.ErrNoDaemon) {
			t.Fatalf("stale socket answered: %v", err)
		}

		d := startWatch(t, dir, deps, "--poll")
		d.waitStatus(watchOneErr)
		eventually(t, func() bool {
			_, s, err := ipc.Query(sock, ipc.OpReport, queryTimeout)
			return err == nil && s != nil && s.State == ipc.StateReady
		}, "ready report", func() string { return d.stderr.String() })
	})

	t.Run("regular file at sock is exit 3", func(t *testing.T) {
		dir := repo(t, "vpc_id")
		deps := testDeps(t)
		_, rt := repoRuntime(t, dir, deps)
		_, err := statusfile.EnsureRepoDir(dir, deps.env)
		mustDo(t, err)
		sock := filepath.Join(rt, ipc.SockName)
		mustDo(t, os.WriteFile(sock, []byte("keep"), 0o600))
		var stdout, stderr bytes.Buffer
		code := runWatchWith(context.Background(), []string{"--poll", dir}, &stdout, &stderr, deps)
		if code != exitFailure {
			t.Fatalf("exit %d, want %d; stderr:\n%s", code, exitFailure, stderr.String())
		}
		if !strings.Contains(stderr.String(), "not a socket") {
			t.Fatalf("stderr does not explain:\n%s", stderr.String())
		}
		b, err := os.ReadFile(sock)
		if err != nil || string(b) != "keep" {
			t.Fatalf("regular file touched: %q, %v", b, err)
		}
		assertLockFree(t, rt)
	})
}

// TestWatchServesSnapshots: indexing before the first index, then the
// exact bytes buildSnapshot renders.
func TestWatchServesSnapshots(t *testing.T) {
	dir := repo(t, "vpc_idd")
	deps := testDeps(t)
	entered, release := make(chan struct{}), make(chan struct{})
	deps.loaderHook = func(*terragrunt.Loader) {
		close(entered)
		<-release
	}
	_, rt := repoRuntime(t, dir, deps)
	sock := filepath.Join(rt, ipc.SockName)
	d := startWatch(t, dir, deps, "--poll")
	<-entered
	info, s, err := ipc.Query(sock, ipc.OpReport, queryTimeout)
	if err != nil || s != nil || info.PID != os.Getpid() {
		t.Fatalf("report while indexing: info %+v snapshot %v err %v", info, s, err)
	}
	if _, err := os.Stat(filepath.Join(rt, ipc.DumpName)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("socket mode wrote a report file: %v", err)
	}
	close(release)
	d.waitStatus(watchOneErr)
	_, s, err = ipc.Query(sock, ipc.OpReport, queryTimeout)
	mustDo(t, err)
	want, err := buildSnapshot(freshReport(t, dir), 1)
	mustDo(t, err)
	assertSnapshot(t, s, want)
}
