// Package ipc is the daemon's local transport: a crash-safe instance lock,
// an AF_UNIX request/response server built on raw stdlib syscalls (linux and
// darwin; the net package is never linked), and a file dump used as the
// report transport on windows.
//
// The lock is the only source of truth for "a daemon runs". The socket and
// dump files are never trusted for liveness.
package ipc

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"time"
)

const (
	// ProtocolVersion is the wire and dump format version.
	ProtocolVersion = 1

	// File names inside statusfile.Dir(root, env).
	LockName = "lock"
	SockName = "sock"
	DumpName = "report"

	OpPing   = "ping"
	OpReport = "report"

	StateIndexing = "indexing"
	StateReady    = "ready"
	StateFailed   = "failed"

	// MaxSockPath is the longest socket path accepted: darwin's sun_path is
	// 104 bytes including the terminating NUL (linux allows 108).
	MaxSockPath = 103
)

const (
	maxRequest  = 4 << 10  // request line cap, server side
	maxResponse = 64 << 20 // response line cap, client side
	ioTimeout   = 2 * time.Second
	retrySleep  = 20 * time.Millisecond
)

var (
	// ErrHeld means another process owns the instance lock.
	ErrHeld = errors.New("ipc: lock held")
	// ErrNoDaemon means nothing listens on the socket (absent or refused).
	ErrNoDaemon = errors.New("ipc: no daemon")
	// ErrUnsupported is returned by the socket API on platforms without it.
	ErrUnsupported = errors.New("ipc: sockets unsupported on this platform")
)

// VersionError reports a protocol mismatch between daemon and client.
type VersionError struct {
	Daemon, Client int
}

func (e *VersionError) Error() string {
	return fmt.Sprintf("ipc: protocol mismatch: daemon speaks v%d, this client speaks v%d; restart the daemon with the same gruntled", e.Daemon, e.Client)
}

// Info is static per daemon process.
type Info struct {
	PID        int    `json:"pid"`
	Version    string `json:"version"`
	Root       string `json:"root"`
	StatusPath string `json:"status_path"`
}

// Snapshot is the pre-rendered result of one successful index. The []byte
// fields are base64 in JSON, so they round trip byte for byte, invalid
// UTF-8 included.
type Snapshot struct {
	State      string `json:"state"`                // StateReady or StateFailed
	LastError  string `json:"last_error,omitempty"` // sanitised; set with StateFailed
	HasErrors  bool   `json:"has_errors"`
	Generation uint64 `json:"generation"`
	Text       []byte `json:"text"`
	Summary    []byte `json:"summary"`
	JSON       []byte `json:"json"`
	SARIF      []byte `json:"sarif"`
}

// CheckSockPath returns nil when path fits in sun_path on every supported
// platform, otherwise an error naming the length and how to shorten it.
func CheckSockPath(path string) error {
	if len(path) <= MaxSockPath {
		return nil
	}
	return fmt.Errorf("ipc: socket path is %d bytes, over the %d-byte limit: %s; set XDG_RUNTIME_DIR (linux) or TMPDIR to a shorter directory", len(path), MaxSockPath, path)
}

// Lock is an exclusive, crash-safe instance lock. The OS drops it when the
// process exits, however it exits.
type Lock struct {
	f *os.File
}

// Release closes the lock handle. The lock file stays: deleting it would
// race a starting daemon that already opened it.
func (l *Lock) Release() error {
	if l == nil || l.f == nil {
		return nil
	}
	err := l.f.Close()
	l.f = nil
	return err
}

// LockRetry calls TryLock up to attempts times (at least once), sleeping
// 20ms between tries that fail with ErrHeld. sleep nil means time.Sleep.
func LockRetry(path string, attempts int, sleep func(time.Duration)) (*Lock, error) {
	if attempts < 1 {
		attempts = 1
	}
	if sleep == nil {
		sleep = time.Sleep
	}
	for i := range attempts {
		l, err := TryLock(path)
		if !errors.Is(err, ErrHeld) {
			return l, err
		}
		if i < attempts-1 {
			sleep(retrySleep)
		}
	}
	return nil, ErrHeld
}

// request is the single line a client sends.
type request struct {
	V  int    `json:"v"`
	Op string `json:"op"`
}

// response is the single line the server answers with.
type response struct {
	V        int       `json:"v"`
	OK       bool      `json:"ok"`
	Error    string    `json:"error,omitempty"`
	Info     *Info     `json:"info,omitempty"`
	State    string    `json:"state,omitempty"`
	Snapshot *Snapshot `json:"snapshot,omitempty"`
}

func encodeLine(v any) ([]byte, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	return append(b, '\n'), nil
}

var errLineTooLong = errors.New("ipc: line too long")

// readLine reads one newline-terminated line of at most max bytes
// (newline excluded). A missing newline or a longer line is an error.
func readLine(r io.Reader, max int) ([]byte, error) {
	br := bufio.NewReader(io.LimitReader(r, int64(max)+1))
	line, err := br.ReadBytes('\n')
	if err != nil {
		if len(line) > max {
			return nil, errLineTooLong
		}
		if errors.Is(err, io.EOF) {
			return nil, io.ErrUnexpectedEOF
		}
		return nil, err
	}
	return line, nil
}

// respond builds the answer to one request line. It only calls get; it
// never touches the index.
func respond(line []byte, info Info, get func() *Snapshot) response {
	fail := func(format string, args ...any) response {
		return response{V: ProtocolVersion, Error: fmt.Sprintf(format, args...)}
	}
	var req request
	if err := json.Unmarshal(line, &req); err != nil {
		return fail("malformed request")
	}
	if req.V != ProtocolVersion {
		return fail("unsupported protocol version %d (daemon speaks %d)", req.V, ProtocolVersion)
	}
	if req.Op != OpPing && req.Op != OpReport {
		return fail("unknown op %q", req.Op)
	}
	in := info
	r := response{V: ProtocolVersion, OK: true, Info: &in, State: StateIndexing}
	s := get()
	if s == nil {
		return r
	}
	r.State = s.State
	if r.State == "" {
		r.State = StateReady
	}
	if req.Op == OpReport {
		r.Snapshot = s
	}
	return r
}

// decodeResponse turns a response line into Query's results for a client
// speaking clientV. A ready report must carry a snapshot.
func decodeResponse(line []byte, clientV int, op string) (Info, *Snapshot, error) {
	var r response
	if err := json.Unmarshal(line, &r); err != nil {
		return Info{}, nil, fmt.Errorf("ipc: malformed response: %w", err)
	}
	if r.V != clientV {
		return Info{}, nil, &VersionError{Daemon: r.V, Client: clientV}
	}
	if !r.OK {
		return Info{}, nil, fmt.Errorf("ipc: daemon refused the request: %s", r.Error)
	}
	if r.Info == nil {
		return Info{}, nil, errors.New("ipc: malformed response: no info")
	}
	if r.State == StateIndexing {
		return *r.Info, nil, nil
	}
	if op == OpReport && r.Snapshot == nil {
		return Info{}, nil, errors.New("ipc: malformed response: no snapshot")
	}
	return *r.Info, r.Snapshot, nil
}
