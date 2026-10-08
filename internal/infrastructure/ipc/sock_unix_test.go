//go:build linux || darwin

package ipc

import (
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
)

const testTimeout = 10 * time.Second

// serve starts a server on a fresh short dir and closes it at cleanup.
func serve(t *testing.T, get func() *Snapshot) (*Server, string) {
	t.Helper()
	sock := filepath.Join(shortDir(t), SockName)
	ln, err := Listen(sock)
	if err != nil {
		t.Fatal(err)
	}
	s := Serve(ln, sampleInfo(), get)
	t.Cleanup(func() { _ = s.Close() })
	return s, sock
}

// rawDial connects with the package dialer and sets a generous deadline.
func rawDial(t *testing.T, sock string) *os.File {
	t.Helper()
	f, err := dial(sock, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = f.Close() })
	if err := f.SetDeadline(time.Now().Add(testTimeout)); err != nil {
		t.Fatal(err)
	}
	return f
}

func waitClosed(t *testing.T, ch <-chan struct{}, what string) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(testTimeout):
		t.Fatalf("%s did not happen within %v", what, testTimeout)
	}
}

func TestQueryRoundTrip(t *testing.T) {
	want := sampleSnapshot()
	_, sock := serve(t, func() *Snapshot { return want })

	info, got, err := Query(sock, OpReport, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if info != sampleInfo() {
		t.Fatalf("info = %+v", info)
	}
	equalSnapshot(t, got, want)

	info, got, err = Query(sock, OpPing, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if info != sampleInfo() || got != nil {
		t.Fatalf("ping: info=%+v snapshot=%+v", info, got)
	}
}

func TestQueryIndexing(t *testing.T) {
	_, sock := serve(t, func() *Snapshot { return nil })
	info, got, err := Query(sock, OpReport, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if got != nil || info.PID != sampleInfo().PID {
		t.Fatalf("indexing: info=%+v snapshot=%+v", info, got)
	}
}

func TestQueryVersionMismatch(t *testing.T) {
	_, sock := serve(t, func() *Snapshot { return sampleSnapshot() })
	_, _, err := query(sock, OpReport, 2, time.Second)
	var ve *VersionError
	if !errors.As(err, &ve) || ve.Daemon != 1 || ve.Client != 2 {
		t.Fatalf("err = %v, want *VersionError{1,2}", err)
	}
}

func TestQueryUnknownOp(t *testing.T) {
	_, sock := serve(t, func() *Snapshot { return sampleSnapshot() })
	_, _, err := Query(sock, "shutdown", time.Second)
	if err == nil || !strings.Contains(err.Error(), "unknown op") {
		t.Fatalf("err = %v", err)
	}
}

func TestOversizeRequestDropped(t *testing.T) {
	_, sock := serve(t, func() *Snapshot { return sampleSnapshot() })
	f := rawDial(t, sock)
	big := strings.Repeat("x", 5<<10) + "\n"
	_, _ = f.Write([]byte(big)) // the server may close before reading it all
	b, _ := io.ReadAll(f)       // EOF or ECONNRESET, never a response
	if len(b) != 0 {
		t.Fatalf("server answered an oversize request: %q", b)
	}
	if _, _, err := Query(sock, OpPing, time.Second); err != nil {
		t.Fatalf("server wedged after oversize request: %v", err)
	}
}

func TestSilentClientDropped(t *testing.T) {
	old := serverReadDeadline
	serverReadDeadline = 20 * time.Millisecond
	t.Cleanup(func() { serverReadDeadline = old })

	_, sock := serve(t, func() *Snapshot { return sampleSnapshot() })
	f := rawDial(t, sock)
	b, err := io.ReadAll(f) // returns once the server drops us
	if err != nil && !errors.Is(err, syscall.ECONNRESET) {
		t.Fatalf("read: %v", err)
	}
	if len(b) != 0 {
		t.Fatalf("unexpected bytes %q", b)
	}
	if _, _, err := Query(sock, OpPing, time.Second); err != nil {
		t.Fatalf("server wedged after silent client: %v", err)
	}
}

func TestSocketMode(t *testing.T) {
	_, sock := serve(t, func() *Snapshot { return nil })
	fi, err := os.Lstat(sock)
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode().Type() != fs.ModeSocket || fi.Mode().Perm() != 0o600 {
		t.Fatalf("mode = %v", fi.Mode())
	}
}

func TestCloseWakesAcceptAndCleansUp(t *testing.T) {
	s, sock := serve(t, func() *Snapshot { return nil })
	// The accept loop is running and blocked again once this returns.
	if _, _, err := Query(sock, OpPing, time.Second); err != nil {
		t.Fatal(err)
	}
	closed := make(chan struct{})
	go func() {
		defer close(closed)
		if err := s.Close(); err != nil {
			t.Errorf("Close: %v", err)
		}
	}()
	waitClosed(t, closed, "Close")
	waitClosed(t, s.done, "accept loop exit")
	if _, err := os.Lstat(sock); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("socket still present: %v", err)
	}
	if err := s.Close(); err != nil {
		t.Fatalf("second Close: %v", err)
	}
	if _, _, err := Query(sock, OpPing, time.Second); !errors.Is(err, ErrNoDaemon) {
		t.Fatalf("Query after Close: err = %v, want ErrNoDaemon", err)
	}
}

func TestCloseWaitsForHandlers(t *testing.T) {
	release := make(chan struct{})
	entered := make(chan struct{})
	var once sync.Once
	s, sock := serve(t, func() *Snapshot {
		once.Do(func() { close(entered) })
		<-release
		return nil
	})
	queried := make(chan error, 1)
	go func() {
		_, _, err := Query(sock, OpPing, testTimeout)
		queried <- err
	}()
	waitClosed(t, entered, "handler start")

	closed := make(chan struct{})
	go func() { _ = s.Close(); close(closed) }()
	select {
	case <-closed:
		t.Fatal("Close returned while a handler was running")
	case <-s.done: // listener closed, handler still blocked: Close must wait
	case <-time.After(testTimeout):
		t.Fatal("accept loop did not exit")
	}
	close(release)
	waitClosed(t, closed, "Close")
	if err := <-queried; err != nil {
		t.Fatalf("in-flight query: %v", err)
	}
}

func TestDialMissing(t *testing.T) {
	sock := filepath.Join(shortDir(t), SockName)
	if _, _, err := Query(sock, OpPing, time.Second); !errors.Is(err, ErrNoDaemon) {
		t.Fatalf("err = %v, want ErrNoDaemon", err)
	}
}

func TestListenReplacesStaleSocket(t *testing.T) {
	sock := filepath.Join(shortDir(t), SockName)
	ln, err := Listen(sock)
	if err != nil {
		t.Fatal(err)
	}
	_ = ln.f.Close() // crash-like: socket file left, nobody listening
	if _, _, err := Query(sock, OpPing, time.Second); !errors.Is(err, ErrNoDaemon) {
		t.Fatalf("stale socket: err = %v, want ErrNoDaemon", err)
	}
	ln, err = Listen(sock)
	if err != nil {
		t.Fatalf("Listen over stale socket: %v", err)
	}
	s := Serve(ln, sampleInfo(), func() *Snapshot { return nil })
	defer s.Close()
	if _, _, err := Query(sock, OpPing, time.Second); err != nil {
		t.Fatal(err)
	}
}

func TestListenRefusesNonSocket(t *testing.T) {
	dir := shortDir(t)
	sock := filepath.Join(dir, SockName)

	if err := os.WriteFile(sock, []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Listen(sock); err == nil {
		t.Fatal("Listen replaced a regular file")
	}
	if b, err := os.ReadFile(sock); err != nil || string(b) != "keep" {
		t.Fatalf("regular file touched: %q %v", b, err)
	}
	if err := os.Remove(sock); err != nil {
		t.Fatal(err)
	}

	target := filepath.Join(dir, "target")
	if err := os.WriteFile(target, []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, sock); err != nil {
		t.Fatal(err)
	}
	if _, err := Listen(sock); err == nil {
		t.Fatal("Listen replaced a symlink")
	}
	if fi, err := os.Lstat(sock); err != nil || fi.Mode()&fs.ModeSymlink == 0 {
		t.Fatalf("symlink touched: %v %v", fi, err)
	}
	if b, err := os.ReadFile(target); err != nil || string(b) != "keep" {
		t.Fatalf("symlink target touched: %q %v", b, err)
	}
}

func TestListenTooLong(t *testing.T) {
	sock := "/" + strings.Repeat("a", MaxSockPath)
	if _, err := Listen(sock); err == nil || !strings.Contains(err.Error(), "XDG_RUNTIME_DIR") {
		t.Fatalf("err = %v", err)
	}
}

func TestConcurrentQueries(t *testing.T) {
	want := sampleSnapshot()
	_, sock := serve(t, func() *Snapshot { return want })
	const workers, each = 16, 8
	var wg sync.WaitGroup
	errs := make(chan error, workers*each)
	for range workers {
		wg.Go(func() {
			for range each {
				_, got, err := Query(sock, OpReport, testTimeout)
				if err == nil && (got == nil || string(got.Text) != string(want.Text)) {
					err = errors.New("wrong snapshot")
				}
				if err != nil {
					errs <- err
				}
			}
		})
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatal(err)
	}
}
