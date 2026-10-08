//go:build linux || darwin

package ipc

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"sync"
	"syscall"
	"time"
)

// serverReadDeadline bounds how long a connection may take to send its
// request line. Tests shorten it before calling Serve.
var serverReadDeadline = ioTimeout

// emfileBackoff is the pause after the process runs out of descriptors.
const emfileBackoff = 100 * time.Millisecond

// Listener is a bound, listening AF_UNIX socket. Its descriptor is
// non-blocking and registered with the runtime poller, so Close wakes a
// blocked accept and per-connection deadlines work. Fd() is never called on
// it or on accepted connections: that would switch them to blocking mode.
type Listener struct {
	f    *os.File
	path string
}

// socket returns a new close-on-exec AF_UNIX stream socket. ForkLock keeps
// a concurrent child process from inheriting it before the flag is set.
func socket() (int, error) {
	syscall.ForkLock.RLock()
	defer syscall.ForkLock.RUnlock()
	fd, err := syscall.Socket(syscall.AF_UNIX, syscall.SOCK_STREAM, 0)
	if err != nil {
		return -1, err
	}
	syscall.CloseOnExec(fd)
	return fd, nil
}

// Listen binds path and listens on it. The caller must hold the instance
// lock: a leftover socket file there is stale by definition and is removed.
// Anything else at path (regular file, symlink, directory) is an error and
// is left untouched.
func Listen(path string) (*Listener, error) {
	if err := CheckSockPath(path); err != nil {
		return nil, err
	}
	fi, err := os.Lstat(path)
	switch {
	case err == nil:
		if fi.Mode().Type() != fs.ModeSocket {
			return nil, fmt.Errorf("ipc: %s exists and is not a socket (%v); refusing to remove it", path, fi.Mode().Type())
		}
		if err := os.Remove(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return nil, err
		}
	case !errors.Is(err, fs.ErrNotExist):
		return nil, err
	}

	fd, err := socket()
	if err != nil {
		return nil, err
	}
	if err := syscall.Bind(fd, &syscall.SockaddrUnix{Name: path}); err != nil {
		_ = syscall.Close(fd)
		return nil, fmt.Errorf("ipc: bind %s: %w", path, err)
	}
	// The directory is 0700 already; this narrows the socket itself.
	err = os.Chmod(path, 0o600)
	if err == nil {
		err = syscall.Listen(fd, 16)
	}
	if err == nil {
		err = syscall.SetNonblock(fd, true)
	}
	if err != nil {
		_ = syscall.Close(fd)
		_ = os.Remove(path)
		return nil, fmt.Errorf("ipc: listen %s: %w", path, err)
	}
	return &Listener{f: os.NewFile(uintptr(fd), path), path: path}, nil
}

// Server answers one request per connection from get. It never touches
// the index: get returns an immutable, pre-rendered snapshot.
type Server struct {
	ln       *Listener
	info     Info
	get      func() *Snapshot
	deadline time.Duration

	wg   sync.WaitGroup // connection handlers
	quit chan struct{}  // closed first by Close
	done chan struct{}  // closed when the accept loop has returned

	mu     sync.Mutex
	closed bool
}

// Serve starts the accept loop on ln. get nil-snapshot means the daemon is
// still indexing.
func Serve(ln *Listener, info Info, get func() *Snapshot) *Server {
	s := &Server{
		ln:       ln,
		info:     info,
		get:      get,
		deadline: serverReadDeadline,
		quit:     make(chan struct{}),
		done:     make(chan struct{}),
	}
	go s.loop()
	return s
}

// Close stops accepting (waking a blocked accept), waits for running
// handlers, and removes the socket file. Later calls return nil.
func (s *Server) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil
	}
	s.closed = true
	close(s.quit)
	err := s.ln.f.Close()
	<-s.done
	s.wg.Wait()
	if rerr := os.Remove(s.ln.path); rerr != nil && !errors.Is(rerr, fs.ErrNotExist) && err == nil {
		err = rerr
	}
	return err
}

func (s *Server) stopping() bool {
	select {
	case <-s.quit:
		return true
	default:
		return false
	}
}

func (s *Server) loop() {
	defer close(s.done)
	rc, err := s.ln.f.SyscallConn()
	if err != nil {
		return
	}
	for {
		conn, err := accept(rc)
		if err == nil {
			s.wg.Add(1)
			go s.handle(conn)
			continue
		}
		if s.stopping() || errors.Is(err, os.ErrClosed) {
			return
		}
		if err == syscall.EINTR || err == syscall.ECONNABORTED {
			continue
		}
		// EMFILE, ENFILE or anything unexpected: pause instead of spinning,
		// but wake at once on Close.
		t := time.NewTimer(emfileBackoff)
		select {
		case <-s.quit:
			t.Stop()
			return
		case <-t.C:
		}
	}
}

// accept waits on the poller for one connection.
func accept(rc syscall.RawConn) (*os.File, error) {
	var nfd int
	var aerr error
	err := rc.Read(func(fd uintptr) bool {
		syscall.ForkLock.RLock()
		nfd, _, aerr = syscall.Accept(int(fd))
		if aerr == nil {
			syscall.CloseOnExec(nfd)
		}
		syscall.ForkLock.RUnlock()
		return aerr != syscall.EAGAIN // false: wait for readiness
	})
	if err != nil {
		return nil, err
	}
	if aerr != nil {
		return nil, aerr
	}
	if err := syscall.SetNonblock(nfd, true); err != nil {
		_ = syscall.Close(nfd)
		return nil, err
	}
	return os.NewFile(uintptr(nfd), "ipc-conn"), nil
}

func (s *Server) handle(conn *os.File) {
	defer s.wg.Done()
	defer conn.Close()
	if err := conn.SetReadDeadline(time.Now().Add(s.deadline)); err != nil {
		return
	}
	line, err := readLine(conn, maxRequest)
	if err != nil {
		return // silent, oversize or broken client: drop it
	}
	b, err := encodeLine(respond(line, s.info, s.get))
	if err != nil {
		b, _ = encodeLine(response{V: ProtocolVersion, Error: "internal error"})
	}
	if err := conn.SetWriteDeadline(time.Now().Add(ioTimeout)); err != nil {
		return
	}
	_, _ = conn.Write(b)
}

// dial connects to sock, bounding connect by timeout. A missing socket or
// one nobody listens on is ErrNoDaemon.
func dial(sock string, timeout time.Duration) (*os.File, error) {
	fd, err := socket()
	if err != nil {
		return nil, err
	}
	tv := syscall.NsecToTimeval(int64(timeout))
	_ = syscall.SetsockoptTimeval(fd, syscall.SOL_SOCKET, syscall.SO_SNDTIMEO, &tv)
	for {
		err = syscall.Connect(fd, &syscall.SockaddrUnix{Name: sock})
		if err != syscall.EINTR {
			break
		}
	}
	if err != nil {
		_ = syscall.Close(fd)
		if err == syscall.ENOENT || err == syscall.ECONNREFUSED {
			return nil, fmt.Errorf("%w at %s", ErrNoDaemon, sock)
		}
		return nil, fmt.Errorf("ipc: connect %s: %w", sock, err)
	}
	if err := syscall.SetNonblock(fd, true); err != nil {
		_ = syscall.Close(fd)
		return nil, err
	}
	return os.NewFile(uintptr(fd), sock), nil
}

// Query sends one op to the daemon at sock. timeout bounds the connect and
// the whole exchange (0 means 2s). The snapshot is nil while the daemon is
// indexing and for OpPing.
func Query(sock, op string, timeout time.Duration) (Info, *Snapshot, error) {
	return query(sock, op, ProtocolVersion, timeout)
}

func query(sock, op string, v int, timeout time.Duration) (Info, *Snapshot, error) {
	if timeout <= 0 {
		timeout = ioTimeout
	}
	if err := CheckSockPath(sock); err != nil {
		return Info{}, nil, err
	}
	f, err := dial(sock, timeout)
	if err != nil {
		return Info{}, nil, err
	}
	defer f.Close()
	if err := f.SetDeadline(time.Now().Add(timeout)); err != nil {
		return Info{}, nil, err
	}
	req, err := encodeLine(request{V: v, Op: op})
	if err != nil {
		return Info{}, nil, err
	}
	if _, err := f.Write(req); err != nil {
		return Info{}, nil, fmt.Errorf("ipc: send to %s: %w", sock, err)
	}
	line, err := readLine(f, maxResponse)
	if err != nil {
		return Info{}, nil, fmt.Errorf("ipc: read from %s: %w", sock, err)
	}
	return decodeResponse(line, v, op)
}
