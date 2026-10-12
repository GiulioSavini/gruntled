//go:build linux || darwin

package main

import (
	"bufio"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/GiulioSavini/gruntled/internal/infrastructure/statusfile"
)

// v1Reply is what a v0.3.0 daemon answers to any v2 request.
const v1Reply = `{"v":1,"ok":false,"error":"unsupported protocol version 2 (daemon speaks 1)"}` + "\n"

const v1Mismatch = "gruntled: daemon runs protocol v1, this gruntled speaks v2; restart the daemon\n"

// rawDaemon listens on the runtime socket of repoDir under env with plain
// syscalls and calls reply with each request line (connection still open;
// reply writes to it or not). It stops at cleanup.
func rawDaemon(t *testing.T, env statusfile.Env, repoDir string, reply func(conn *os.File, line []byte)) {
	t.Helper()
	rt := ensureRuntime(t, env, repoDir)
	path := filepath.Join(rt, "sock")
	fd, err := syscall.Socket(syscall.AF_UNIX, syscall.SOCK_STREAM, 0)
	mustDo(t, err)
	mustDo(t, syscall.Bind(fd, &syscall.SockaddrUnix{Name: path}))
	mustDo(t, os.Chmod(path, 0o600))
	mustDo(t, syscall.Listen(fd, 8))
	mustDo(t, syscall.SetNonblock(fd, true))
	var stop atomic.Bool
	var wg sync.WaitGroup
	wg.Go(func() {
		for !stop.Load() {
			nfd, _, err := syscall.Accept(fd)
			if err != nil {
				if errors.Is(err, syscall.EAGAIN) || errors.Is(err, syscall.EINTR) {
					time.Sleep(2 * time.Millisecond)
					continue
				}
				return
			}
			_ = syscall.SetNonblock(nfd, false)
			conn := os.NewFile(uintptr(nfd), "conn")
			wg.Go(func() {
				defer conn.Close()
				line, err := bufio.NewReader(conn).ReadBytes('\n')
				if err != nil {
					return
				}
				reply(conn, line)
			})
		}
	})
	t.Cleanup(func() {
		stop.Store(true)
		wg.Wait()
		_ = syscall.Close(fd)
		_ = os.Remove(path)
	})
}

func answer(s string) func(*os.File, []byte) {
	return func(conn *os.File, _ []byte) { _, _ = conn.WriteString(s) }
}

// TestReportBlastV1: report and report --blast against a v0.3.0 daemon
// exit 3 with the mismatch message, and so does report --blast against a
// v1 report file.
func TestReportBlastV1(t *testing.T) {
	fixture := "testdata/sarif-fixture"
	env := envAt(shortBase(t))
	rawDaemon(t, env, fixture, answer(v1Reply))
	for _, args := range [][]string{
		{"--format", "text"}, {"--format", "json"}, {"--format", "sarif"},
		{"--blast"}, {"--blast", "--format", "json"},
	} {
		got := runReportArgs(env, "linux", append(args, fixture)...)
		if got.code != exitFailure || got.stdout != "" || got.stderr != v1Mismatch {
			t.Fatalf("%v: %s", args, got)
		}
	}
	t.Run("v1 report file", func(t *testing.T) {
		env := envAt(shortBase(t))
		rt := holdDump(t, env, fixture, nil)
		mustDo(t, statusfile.NewWriter(filepath.Join(rt, "report")).Write(`{"v":1,"info":{"pid":1},"state":"ready","snapshot":{"state":"ready","has_errors":false,"generation":1,"text":"","summary":"","json":"","sarif":""}}`+"\n"))
		got := runReportArgs(env, "windows", "--blast", fixture)
		if got.code != exitFailure || got.stdout != "" || got.stderr != v1Mismatch {
			t.Fatalf("%s", got)
		}
	})
}

// TestReportBlastTooLargeRefusal: the server-side too_large backstop
// refusal prints the cap message.
func TestReportBlastTooLargeRefusal(t *testing.T) {
	fixture := "testdata/sarif-fixture"
	env := envAt(shortBase(t))
	rawDaemon(t, env, fixture, answer(`{"v":2,"ok":false,"error":"blast view is 70000123 bytes, over the 64 MiB response cap; run gruntled blast --base","code":"too_large"}`+"\n"))
	got := runReportArgs(env, "linux", "--blast", fixture)
	if got.code != exitFailure || got.stdout != "" ||
		got.stderr != "gruntled: the blast view is 70000123 bytes, over the 64 MiB response cap; run gruntled blast --base <dir>\n" {
		t.Fatalf("%s", got)
	}
}

const outcomeUnknown = "gruntled: no usable answer from the daemon; the baseline may or may not have moved; run gruntled report --blast to check\n"

func TestRebaseOutcomeUnknownCLI(t *testing.T) {
	fixture := "testdata/sarif-fixture"
	for name, reply := range map[string]func(*os.File, []byte){
		"ok without rebase": answer(`{"v":2,"ok":true,"info":{"pid":1}}` + "\n"),
		"garbage":           answer("garbage\n"),
		"truncated":         answer(`{"v":2,"ok":true,"info":`),
	} {
		t.Run(name, func(t *testing.T) {
			env := envAt(shortBase(t))
			rawDaemon(t, env, fixture, reply)
			got := runRebaseAt(env, "linux", fixture)
			if got.code != exitFailure || got.stdout != "" || got.stderr != outcomeUnknown {
				t.Fatalf("%s", got)
			}
		})
	}
}

func TestRebaseTimeoutUnknown(t *testing.T) {
	setSeam(t, &rebaseTimeout, 200*time.Millisecond)
	fixture := "testdata/sarif-fixture"
	env := envAt(shortBase(t))
	rawDaemon(t, env, fixture, func(conn *os.File, _ []byte) { _, _ = io.Copy(io.Discard, conn) })
	got := runRebaseAt(env, "linux", fixture)
	want := "gruntled: no usable answer from the daemon (no answer within 200ms); the baseline may or may not have moved; run gruntled report --blast to check\n"
	if got.code != exitFailure || got.stdout != "" || got.stderr != want {
		t.Fatalf("%s", got)
	}
}

func TestRebaseV1(t *testing.T) {
	fixture := "testdata/sarif-fixture"
	env := envAt(shortBase(t))
	rawDaemon(t, env, fixture, answer(v1Reply))
	got := runRebaseAt(env, "linux", fixture)
	if got.code != exitFailure || got.stdout != "" || got.stderr != v1Mismatch {
		t.Fatalf("%s", got)
	}
}
