//go:build linux || darwin

package ipc

import (
	"bytes"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
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
	_, _, _, err := query(sock, request{V: 1, Op: OpReport}, time.Second)
	var ve *VersionError
	if !errors.As(err, &ve) || ve.Daemon != 2 || ve.Client != 1 {
		t.Fatalf("err = %v, want *VersionError{2,1}", err)
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

// serveHandler starts a ServeHandler on a fresh short dir.
func serveHandler(t *testing.T, h Handler) string {
	t.Helper()
	sock := filepath.Join(shortDir(t), SockName)
	ln, err := Listen(sock)
	if err != nil {
		t.Fatal(err)
	}
	s := ServeHandler(ln, sampleInfo(), h)
	t.Cleanup(func() { _ = s.Close() })
	return sock
}

// rawServer accepts connections on a fresh socket, reads one request line
// from each and hands it to reply, then closes the connection.
func rawServer(t *testing.T, reply func(conn *os.File, req []byte)) string {
	t.Helper()
	sock := filepath.Join(shortDir(t), SockName)
	ln, err := Listen(sock)
	if err != nil {
		t.Fatal(err)
	}
	rc, err := ln.f.SyscallConn()
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			conn, err := accept(rc)
			if err != nil {
				if errors.Is(err, os.ErrClosed) {
					return
				}
				continue
			}
			wg.Go(func() {
				defer conn.Close()
				_ = conn.SetDeadline(time.Now().Add(testTimeout))
				line, err := readLine(conn, maxRequest)
				if err != nil {
					return
				}
				reply(conn, line)
			})
		}
	}()
	t.Cleanup(func() {
		_ = ln.f.Close()
		<-done
		wg.Wait()
		_ = os.Remove(sock)
	})
	return sock
}

func writeLine(s string) func(*os.File, []byte) {
	return func(conn *os.File, _ []byte) { _, _ = conn.Write([]byte(s + "\n")) }
}

// neverAnswer reads until the client gives up.
func neverAnswer(conn *os.File, _ []byte) { _, _ = io.Copy(io.Discard, conn) }

func TestServeHandlerRoundTrip(t *testing.T) {
	snap := blastSnapshot()
	h := &fakeHandler{snap: snap, res: RebaseResult{Generation: 11, BrokenErrors: 3}}
	sock := serveHandler(t, h)

	_, got, err := Query(sock, OpReport, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	equalSnapshot(t, got, snap)
	if got.Blast != nil {
		t.Fatalf("report carried a blast view: %+v", got.Blast)
	}

	_, got, err = Query(sock, OpBlast, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got.Blast, snap.Blast) || got.Text != nil || got.Generation != snap.Generation {
		t.Fatalf("blast = %+v (view %+v)", got, got.Blast)
	}

	five := uint64(5)
	info, res, err := Rebase(sock, &five, time.Second)
	if err != nil || res != h.res || info != sampleInfo() {
		t.Fatalf("Rebase = (%+v, %+v, %v)", info, res, err)
	}
	if h.expect == nil || *h.expect != 5 {
		t.Fatalf("handler expect = %v, want 5", h.expect)
	}
	if _, _, err := Rebase(sock, nil, time.Second); err != nil || h.expect != nil {
		t.Fatalf("Rebase(nil) = %v, handler expect %v", err, h.expect)
	}
}

func TestServeWrapperRebaseUnsupported(t *testing.T) {
	_, sock := serve(t, func() *Snapshot { return sampleSnapshot() })
	_, _, err := Rebase(sock, nil, time.Second)
	var re *RefusedError
	if !errors.As(err, &re) || re.Code != CodeUnsupported {
		t.Fatalf("err = %v, want code unsupported", err)
	}
}

func TestBlastResponseCap(t *testing.T) {
	setCap(t, 8<<10)
	snap := bigBlastSnapshot(16 << 10)
	sock := serveHandler(t, &fakeHandler{snap: snap})

	full, err := encodeLine(snapshotResponse(sampleInfo(), snap, OpBlast))
	if err != nil {
		t.Fatal(err)
	}
	_, got, err := Query(sock, OpBlast, time.Second)
	var re *RefusedError
	want := "blast view is " + itoa(len(full)) + " bytes, over the 8 KiB response cap; run gruntled blast --base"
	if !errors.As(err, &re) || re.Code != CodeTooLarge || re.Msg != want || got != nil {
		t.Fatalf("err = %v (snapshot %v), want too_large %q", err, got, want)
	}

	if _, got, err := Query(sock, OpReport, time.Second); err != nil || got == nil {
		t.Fatalf("report with small check bytes: (%v, %v)", got, err)
	}

	big := sampleSnapshot()
	big.Text = bytes.Repeat([]byte("x"), 16<<10)
	sock = serveHandler(t, &fakeHandler{snap: big})
	_, _, err = Query(sock, OpReport, time.Second)
	if !errors.Is(err, errLineTooLong) || !strings.Contains(err.Error(), "8 KiB") || errors.As(err, &re) || errors.Is(err, ErrNoReply) {
		t.Fatalf("over-cap report: err = %v, want the client-side line-too-long naming 8 KiB", err)
	}
}

func itoa(n int) string { return strconv.Itoa(n) }

func TestSealedBlastOverSocket(t *testing.T) {
	setCap(t, 8<<10)
	s := bigBlastSnapshot(16 << 10)
	Seal(sampleInfo(), s)
	sock := serveHandler(t, &fakeHandler{snap: s})
	_, got, err := Query(sock, OpBlast, time.Second)
	if err != nil || got == nil || got.Blast.TooLarge != s.Blast.TooLarge || got.Blast.Text != nil {
		t.Fatalf("Query(blast) = (%+v, %v), want the too-large view", got, err)
	}
}

func TestNoReplyAfterSend(t *testing.T) {
	const short = 200 * time.Millisecond
	for name, reply := range map[string]func(*os.File, []byte){
		"never answers":        neverAnswer,
		"closes after reading": func(*os.File, []byte) {},
	} {
		t.Run(name, func(t *testing.T) {
			sock := rawServer(t, reply)
			_, _, err := Query(sock, OpReport, short)
			if !errors.Is(err, ErrNoReply) || !strings.HasPrefix(err.Error(), "ipc: read from ") {
				t.Fatalf("Query: err = %v, want ErrNoReply", err)
			}
			_, _, err = Rebase(sock, nil, short)
			if !errors.Is(err, ErrNoReply) {
				t.Fatalf("Rebase: err = %v, want ErrNoReply", err)
			}
		})
	}
	t.Run("dial failure", func(t *testing.T) {
		_, _, err := Query(filepath.Join(shortDir(t), SockName), OpReport, short)
		if !errors.Is(err, ErrNoDaemon) || errors.Is(err, ErrNoReply) {
			t.Fatalf("err = %v", err)
		}
	})
	t.Run("oversize reply", func(t *testing.T) {
		setCap(t, 8<<10)
		sock := rawServer(t, writeLine(strings.Repeat("x", 9<<10)))
		_, _, err := Query(sock, OpReport, time.Second)
		if err == nil || errors.Is(err, ErrNoReply) || !errors.Is(err, errLineTooLong) {
			t.Fatalf("err = %v", err)
		}
	})
}

func TestRebaseOutcomeUnknown(t *testing.T) {
	unknown := map[string]func(*os.File, []byte){
		"never answers":     neverAnswer,
		"ok without rebase": writeLine(`{"v":2,"ok":true,"info":{"pid":1}}`),
		"garbage":           writeLine(`garbage`),
		"over the cap":      writeLine(strings.Repeat("x", 9<<10)),
	}
	for name, reply := range unknown {
		t.Run(name, func(t *testing.T) {
			setCap(t, 8<<10)
			sock := rawServer(t, reply)
			_, res, err := Rebase(sock, nil, 200*time.Millisecond)
			if !errors.Is(err, ErrOutcomeUnknown) || res != (RebaseResult{}) {
				t.Fatalf("(%+v, %v), want ErrOutcomeUnknown", res, err)
			}
		})
	}
	t.Run("refused", func(t *testing.T) {
		sock := rawServer(t, writeLine(`{"v":2,"ok":false,"error":"generation moved","code":"expect_mismatch"}`))
		_, _, err := Rebase(sock, nil, time.Second)
		var re *RefusedError
		if !errors.As(err, &re) || re.Code != CodeExpectMismatch || errors.Is(err, ErrOutcomeUnknown) {
			t.Fatalf("err = %v", err)
		}
	})
	t.Run("version", func(t *testing.T) {
		sock := rawServer(t, writeLine(`{"v":1,"ok":false,"error":"unsupported protocol version 2 (daemon speaks 1)"}`))
		_, _, err := Rebase(sock, nil, time.Second)
		var ve *VersionError
		if !errors.As(err, &ve) || ve.Daemon != 1 || errors.Is(err, ErrOutcomeUnknown) {
			t.Fatalf("err = %v", err)
		}
	})
	t.Run("dial failure", func(t *testing.T) {
		_, _, err := Rebase(filepath.Join(shortDir(t), SockName), nil, time.Second)
		if !errors.Is(err, ErrNoDaemon) || errors.Is(err, ErrOutcomeUnknown) {
			t.Fatalf("err = %v", err)
		}
	})
}
