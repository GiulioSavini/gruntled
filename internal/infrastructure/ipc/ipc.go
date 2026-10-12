// Package ipc is the daemon's local transport: a crash-safe instance lock,
// an AF_UNIX request/response server built on raw stdlib syscalls (linux and
// darwin; the net package is never linked), and a file dump used as the
// report transport on windows.
//
// The lock is the only source of truth for "a daemon runs". The socket and
// dump files are never trusted for liveness.
//
// Protocol v2: one request line {"v","op","expect"?} in canonical form, one
// response line. Ops are ping, report, blast and rebase. The version is
// checked first and must match exactly on both sides and in the dump. A
// refusal is ok:false with a code; a missing blast view or rebase result
// is an error, never a zero value.
package ipc

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"time"
)

const (
	// ProtocolVersion is the wire and dump format version.
	ProtocolVersion = 2

	// File names inside statusfile.Dir(root, env).
	LockName = "lock"
	SockName = "sock"
	DumpName = "report"

	OpPing   = "ping"
	OpReport = "report"
	OpBlast  = "blast"
	OpRebase = "rebase"

	StateIndexing = "indexing"
	StateReady    = "ready"
	StateFailed   = "failed"

	// MaxSockPath is the longest socket path accepted: darwin's sun_path is
	// 104 bytes including the terminating NUL (linux allows 108).
	MaxSockPath = 103
)

const (
	maxRequest  = 4 << 10  // request line cap, server side
	maxResponse = 64 << 20 // default response cap (responseCap)
	ioTimeout   = 2 * time.Second
	retrySleep  = 20 * time.Millisecond
)

// Refusal codes carried by a response with ok:false (RefusedError.Code).
const (
	CodeIndexing       = "indexing"
	CodeFailed         = "failed"
	CodeExpectMismatch = "expect_mismatch"
	CodeUnavailable    = "unavailable" // the blast view could not be computed
	CodeTooLarge       = "too_large"
	CodeUnsupported    = "unsupported"
	CodeMalformed      = "malformed"
	CodeUnknownOp      = "unknown_op"
	CodeInternal       = "internal"
)

// responseCap bounds a response line (client), a dump (ReadDump and
// WriteDump) and a blast response (server). Tests lower it.
var responseCap = maxResponse

// capString names a cap for messages: "64 MiB", "8 KiB" or "N bytes".
func capString(n int) string {
	switch {
	case n >= 1<<20 && n%(1<<20) == 0:
		return fmt.Sprintf("%d MiB", n>>20)
	case n >= 1<<10 && n%(1<<10) == 0:
		return fmt.Sprintf("%d KiB", n>>10)
	}
	return fmt.Sprintf("%d bytes", n)
}

var (
	// ErrHeld means another process owns the instance lock.
	ErrHeld = errors.New("ipc: lock held")
	// ErrNoDaemon means nothing listens on the socket (absent or refused).
	ErrNoDaemon = errors.New("ipc: no daemon")
	// ErrUnsupported is returned by the socket API on platforms without it.
	ErrUnsupported = errors.New("ipc: sockets unsupported on this platform")
	// ErrNoReply means the request was sent but no reply was read (deadline,
	// EOF, reset): the daemon may have acted on it.
	ErrNoReply = errors.New("ipc: no reply")
	// ErrOutcomeUnknown wraps every Rebase failure after the request was
	// sent that is not a decoded refusal or version error: the baseline may
	// or may not have moved.
	ErrOutcomeUnknown = errors.New("ipc: rebase outcome unknown")
)

// RefusedError is a refusal from a daemon speaking the client's version.
type RefusedError struct {
	Code string // one of the Code* constants
	Msg  string
}

func (e *RefusedError) Error() string {
	return "ipc: daemon refused the request: " + e.Msg
}

// Refuse returns a *RefusedError; a Handler returns it to refuse a request
// with code and a fixed message.
func Refuse(code, msg string) error {
	return &RefusedError{Code: code, Msg: msg}
}

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

	// Blast is the daemon-baseline blast view; only the blast response and
	// the dump carry it, never the report response.
	Blast *BlastView `json:"blast,omitempty"`

	// blastLine is the pre-encoded blast response line set by Seal.
	blastLine []byte
}

// Sealed reports whether Seal ran on s. A struct copy of a sealed snapshot
// is Sealed too while its line may be stale: use it only in tests.
func (s *Snapshot) Sealed() bool { return s.blastLine != nil }

// BlastView is the pre-rendered blast of the current tree against the
// daemon's baseline. Text and JSON are base64 in JSON, byte for byte.
type BlastView struct {
	BaselineGeneration uint64 `json:"baseline_generation"`
	BaselineSource     string `json:"baseline_source"` // "watch start" or "rebase"
	HasErrors          bool   `json:"has_errors"`      // Broken holds an error
	BrokenErrors       int    `json:"broken_errors"`   // error findings in Broken
	Text               []byte `json:"text,omitempty"`
	JSON               []byte `json:"json,omitempty"`
	TooLarge           int64  `json:"too_large,omitempty"` // encoded size over the cap; bytes then nil
	Error              string `json:"error,omitempty"`     // sanitised failure to compute the view
}

// ValidBlastView refuses a missing view and a view with neither an error,
// a too-large marker nor both renderings: an empty view must never read
// as "nothing impacted".
func ValidBlastView(v *BlastView) error {
	if v == nil {
		return errors.New("no blast view")
	}
	if v.Error == "" && v.TooLarge == 0 && (len(v.Text) == 0 || len(v.JSON) == 0) {
		return errors.New("empty blast view")
	}
	return nil
}

// RebaseResult is the outcome of an accepted rebase.
type RebaseResult struct {
	Generation   uint64 `json:"generation"`    // the new baseline generation
	BrokenErrors int    `json:"broken_errors"` // error findings accepted into the baseline
}

// Handler is what the server asks. Snapshot returns the published,
// immutable snapshot (nil while indexing); Rebase moves the baseline and
// refuses with a *RefusedError (Refuse). Any other error is answered as
// code internal with a generic message.
type Handler interface {
	Snapshot() *Snapshot
	Rebase(expect *uint64) (RebaseResult, error)
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
	V      int     `json:"v"`
	Op     string  `json:"op"`
	Expect *uint64 `json:"expect,omitempty"` // OpRebase only
}

// response is the single line the server answers with.
type response struct {
	V        int       `json:"v"`
	OK       bool      `json:"ok"`
	Error    string    `json:"error,omitempty"`
	Info     *Info     `json:"info,omitempty"`
	State    string    `json:"state,omitempty"`
	Snapshot *Snapshot `json:"snapshot,omitempty"`
	// v2 additions, after the v1 fields so the key order is kept.
	Code   string        `json:"code,omitempty"`
	Rebase *RebaseResult `json:"rebase,omitempty"`
}

func encodeLine(v any) ([]byte, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	return append(b, '\n'), nil
}

var errLineTooLong = errors.New("ipc: line too long")

// respond builds the answer to one request line and returns the op it
// served ("" when the request was refused before the op was known). It
// only calls h; it never touches the index.
func respond(line []byte, info Info, h Handler) (response, string) {
	fail := func(code, format string, args ...any) response {
		return response{V: ProtocolVersion, Error: fmt.Sprintf(format, args...), Code: code}
	}
	// Version first, leniently, so any other version gets the version
	// error whatever else the line holds.
	var ver struct {
		V int `json:"v"`
	}
	if err := json.Unmarshal(line, &ver); err != nil {
		return fail(CodeMalformed, "malformed request"), ""
	}
	if ver.V != ProtocolVersion {
		return fail("", "unsupported protocol version %d (daemon speaks %d)", ver.V, ProtocolVersion), ""
	}
	req, ok := strictRequest(line)
	if !ok {
		return fail(CodeMalformed, "malformed request"), ""
	}
	switch req.Op {
	case OpPing, OpReport, OpBlast:
	case OpRebase:
		res, err := h.Rebase(req.Expect)
		if err != nil {
			var re *RefusedError
			if errors.As(err, &re) {
				return fail(re.Code, "%s", re.Msg), req.Op
			}
			return fail(CodeInternal, "internal error"), req.Op
		}
		in := info
		return response{V: ProtocolVersion, OK: true, Info: &in, Rebase: &res}, req.Op
	default:
		return fail(CodeUnknownOp, "unknown op %q", req.Op), ""
	}
	return snapshotResponse(info, h.Snapshot(), req.Op), req.Op
}

// strictRequest decodes a fixed-shape request: no unknown field, exactly
// one JSON value, expect only on rebase, and the line (newline stripped)
// byte-equal to the request's own encoding, which rejects duplicate and
// case-variant keys, null and extra whitespace.
func strictRequest(line []byte) (request, bool) {
	var req request
	dec := json.NewDecoder(bytes.NewReader(line))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		return request{}, false
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		return request{}, false
	}
	if req.Expect != nil && req.Op != OpRebase {
		return request{}, false
	}
	canon, err := json.Marshal(req)
	if err != nil || !bytes.Equal(bytes.TrimSuffix(line, []byte("\n")), canon) {
		return request{}, false
	}
	return req, true
}

// snapshotResponse answers ping, report or blast for s (nil: indexing).
// report gets the snapshot without its blast view; blast gets it without
// the check bytes. Both are struct copies: s is never modified.
func snapshotResponse(info Info, s *Snapshot, op string) response {
	in := info
	r := response{V: ProtocolVersion, OK: true, Info: &in, State: StateIndexing}
	if s == nil {
		return r
	}
	r.State = s.State
	if r.State == "" {
		r.State = StateReady
	}
	switch op {
	case OpReport:
		c := *s
		c.Blast = nil
		r.Snapshot = &c
	case OpBlast:
		c := *s
		c.Text, c.Summary, c.JSON, c.SARIF = nil, nil, nil, nil
		r.Snapshot = &c
	}
	return r
}

// decodeResponse turns a response line into the client's results for op,
// speaking clientV. A ready or failed report or blast must carry a
// snapshot, a blast snapshot a valid blast view, and an ok rebase its
// result.
func decodeResponse(line []byte, clientV int, op string) (Info, *Snapshot, *RebaseResult, error) {
	var r response
	if err := json.Unmarshal(line, &r); err != nil {
		return Info{}, nil, nil, fmt.Errorf("ipc: malformed response: %w", err)
	}
	if r.V != clientV {
		return Info{}, nil, nil, &VersionError{Daemon: r.V, Client: clientV}
	}
	if !r.OK {
		return Info{}, nil, nil, &RefusedError{Code: r.Code, Msg: r.Error}
	}
	if r.Info == nil {
		return Info{}, nil, nil, errors.New("ipc: malformed response: no info")
	}
	if op == OpRebase {
		if r.Rebase == nil {
			return Info{}, nil, nil, errors.New("ipc: malformed response: rebase ok without a result")
		}
		return *r.Info, nil, r.Rebase, nil
	}
	if r.State == StateIndexing {
		return *r.Info, nil, nil, nil
	}
	if (op == OpReport || op == OpBlast) && r.Snapshot == nil {
		return Info{}, nil, nil, errors.New("ipc: malformed response: no snapshot")
	}
	if op == OpBlast {
		if err := ValidBlastView(r.Snapshot.Blast); err != nil {
			return Info{}, nil, nil, fmt.Errorf("ipc: malformed response: %w", err)
		}
	}
	return *r.Info, r.Snapshot, nil, nil
}

// Seal pre-encodes the blast response line for s, once per published
// snapshot, so a blast request costs O(1) whatever the view size. If the
// line is over responseCap, s.Blast is replaced by a copy without the
// rendered bytes and with TooLarge set to the encoded size, keeping the
// metadata, and that small line is stored instead. If encoding fails, s
// gets an error view. Seal always re-encodes (a struct copy of a sealed
// snapshot carries a stale line) and never writes through s.Blast: the
// view may be shared with a snapshot handlers are reading. It must run
// before s itself is shared.
func Seal(info Info, s *Snapshot) {
	if s == nil {
		return
	}
	s.blastLine = nil
	b, err := encodeLine(snapshotResponse(info, s, OpBlast))
	if err == nil && len(b) > responseCap && s.Blast != nil && (s.Blast.Text != nil || s.Blast.JSON != nil) {
		v := *s.Blast
		v.Text, v.JSON, v.TooLarge = nil, nil, int64(len(b))
		s.Blast = &v
		b, err = encodeLine(snapshotResponse(info, s, OpBlast))
	}
	if err != nil {
		v := BlastView{Error: "the blast view could not be encoded"}
		if s.Blast != nil {
			v.BaselineGeneration, v.BaselineSource = s.Blast.BaselineGeneration, s.Blast.BaselineSource
		}
		s.Blast = &v
		if b, err = encodeLine(snapshotResponse(info, s, OpBlast)); err != nil {
			b = internalLine()
		}
	}
	s.blastLine = b
}

func internalLine() []byte {
	b, _ := encodeLine(response{V: ProtocolVersion, Error: "internal error", Code: CodeInternal})
	return b
}

// answer is the server's per-request path: respond, then the sealed blast
// line as is when there is one. Without a sealed line (the Serve wrapper,
// tests) the response is encoded here, O(view) for blast, and a blast
// line over responseCap becomes a too_large refusal. Plain report keeps
// the v1 behaviour: no server-side cap.
func answer(line []byte, info Info, h Handler) []byte {
	r, op := respond(line, info, h)
	if op == OpBlast && r.OK && r.Snapshot != nil && r.Snapshot.blastLine != nil && len(r.Snapshot.blastLine) <= responseCap {
		return r.Snapshot.blastLine
	}
	if r.Snapshot != nil {
		r.Snapshot.blastLine = nil
	}
	b, err := encodeLine(r)
	if err != nil {
		return internalLine()
	}
	if op == OpBlast && len(b) > responseCap {
		b, err = encodeLine(response{V: ProtocolVersion, Code: CodeTooLarge,
			Error: fmt.Sprintf("blast view is %d bytes, over the %s response cap; run gruntled blast --base", len(b), capString(responseCap))})
		if err != nil {
			return internalLine()
		}
	}
	return b
}
