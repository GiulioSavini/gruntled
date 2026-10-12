package ipc

// Frozen v0.3.0 protocol reference (DAEMON-09). The declarations below are
// verbatim copies of v0.3.0, only renamed (ProtocolVersion -> the literal
// v1ProtocolVersion, Snapshot, request, response, respond, dumpFile and
// ReadDump -> v1*). Regenerate, never edit by hand:
//
//	git show v0.3.0:internal/infrastructure/ipc/ipc.go
//	git show v0.3.0:internal/infrastructure/ipc/dump.go
//
// keeping only the six declarations, renamed with
//
//	sed -E 's/\bProtocolVersion\b/v1ProtocolVersion/g; s/\*Snapshot\b/*v1Snapshot/g; s/^type Snapshot\b/type v1Snapshot/; s|^// Snapshot is|// v1Snapshot is|; s/^type request\b/type v1Request/; s/var req request\b/var req v1Request/; s|^// request is|// v1Request is|; s/\bresponse\b/v1Response/g; s/\brespond\b/v1Respond/g; s/\bdumpFile\b/v1DumpFile/g; s/\bReadDump\b/v1ReadDump/g'
//
// then gofmt.
//
// git rev-parse v0.3.0:internal/infrastructure/ipc/ipc.go  = 8ba5757d7a141f7849dc3eb1d6613500d0575077
// git rev-parse v0.3.0:internal/infrastructure/ipc/dump.go = 9cf0dbffce458c42beb734173f8535ee983f2976

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
)

// v1ProtocolVersion is v0.3.0's ProtocolVersion.
const v1ProtocolVersion = 1

// v1Snapshot is the pre-rendered result of one successful index. The []byte
// fields are base64 in JSON, so they round trip byte for byte, invalid
// UTF-8 included.
type v1Snapshot struct {
	State      string `json:"state"`                // StateReady or StateFailed
	LastError  string `json:"last_error,omitempty"` // sanitised; set with StateFailed
	HasErrors  bool   `json:"has_errors"`
	Generation uint64 `json:"generation"`
	Text       []byte `json:"text"`
	Summary    []byte `json:"summary"`
	JSON       []byte `json:"json"`
	SARIF      []byte `json:"sarif"`
}

// v1Request is the single line a client sends.
type v1Request struct {
	V  int    `json:"v"`
	Op string `json:"op"`
}

// v1Response is the single line the server answers with.
type v1Response struct {
	V        int         `json:"v"`
	OK       bool        `json:"ok"`
	Error    string      `json:"error,omitempty"`
	Info     *Info       `json:"info,omitempty"`
	State    string      `json:"state,omitempty"`
	Snapshot *v1Snapshot `json:"snapshot,omitempty"`
}

// v1Respond builds the answer to one request line. It only calls get; it
// never touches the index.
func v1Respond(line []byte, info Info, get func() *v1Snapshot) v1Response {
	fail := func(format string, args ...any) v1Response {
		return v1Response{V: v1ProtocolVersion, Error: fmt.Sprintf(format, args...)}
	}
	var req v1Request
	if err := json.Unmarshal(line, &req); err != nil {
		return fail("malformed request")
	}
	if req.V != v1ProtocolVersion {
		return fail("unsupported protocol version %d (daemon speaks %d)", req.V, v1ProtocolVersion)
	}
	if req.Op != OpPing && req.Op != OpReport {
		return fail("unknown op %q", req.Op)
	}
	in := info
	r := v1Response{V: v1ProtocolVersion, OK: true, Info: &in, State: StateIndexing}
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

// v1DumpFile is the on-disk report used where sockets are unavailable.
type v1DumpFile struct {
	V        int         `json:"v"`
	Info     Info        `json:"info"`
	State    string      `json:"state"`
	Snapshot *v1Snapshot `json:"snapshot,omitempty"`
}

// v1ReadDump reads a dump written by WriteDump. The snapshot is nil when the
// daemon was still indexing.
func v1ReadDump(path string) (Info, *v1Snapshot, error) {
	f, err := os.Open(path)
	if err != nil {
		return Info{}, nil, err
	}
	defer f.Close()
	b, err := io.ReadAll(io.LimitReader(f, maxResponse+1))
	if err != nil {
		return Info{}, nil, err
	}
	if len(b) > maxResponse {
		return Info{}, nil, fmt.Errorf("ipc: %s: %w", path, errLineTooLong)
	}
	var d v1DumpFile
	if err := json.Unmarshal(b, &d); err != nil {
		return Info{}, nil, fmt.Errorf("ipc: malformed dump %s: %w", path, err)
	}
	if d.V != v1ProtocolVersion {
		return Info{}, nil, &VersionError{Daemon: d.V, Client: v1ProtocolVersion}
	}
	if d.State == StateIndexing {
		return d.Info, nil, nil
	}
	if d.Snapshot == nil {
		return Info{}, nil, errors.New("ipc: malformed dump: no snapshot")
	}
	return d.Info, d.Snapshot, nil
}
