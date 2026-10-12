//go:build linux || darwin

package ipc

import (
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"
	"time"
)

// v1Of is the v0.3.0 view of a v2 snapshot's check fields.
func v1Of(s *Snapshot) *v1Snapshot {
	return &v1Snapshot{State: s.State, LastError: s.LastError, HasErrors: s.HasErrors, Generation: s.Generation,
		Text: s.Text, Summary: s.Summary, JSON: s.JSON, SARIF: s.SARIF}
}

// TestCompatV1DaemonV2Client: a v0.3.0 daemon (frozen respond) answers
// every v2 op with its version error, and the v2 client returns
// *VersionError{1,2} with no snapshot and no rebase result.
func TestCompatV1DaemonV2Client(t *testing.T) {
	snap := v1Of(sampleSnapshot())
	sock := rawServer(t, func(conn *os.File, line []byte) {
		b, err := encodeLine(v1Respond(line, sampleInfo(), func() *v1Snapshot { return snap }))
		if err == nil {
			_, _ = conn.Write(b)
		}
	})
	isV1 := func(t *testing.T, what string, err error) {
		t.Helper()
		var ve *VersionError
		if err == nil || !errors.As(err, &ve) || ve.Daemon != 1 || ve.Client != 2 {
			t.Fatalf("%s: err = %v, want *VersionError{1,2}", what, err)
		}
	}
	for _, op := range []string{OpReport, OpBlast, OpPing} {
		_, got, err := Query(sock, op, time.Second)
		isV1(t, op, err)
		if got != nil {
			t.Fatalf("%s: snapshot %+v", op, got)
		}
	}
	three := uint64(3)
	for name, expect := range map[string]*uint64{"rebase expect 3": &three, "rebase": nil} {
		_, res, err := Rebase(sock, expect, time.Second)
		isV1(t, name, err)
		if res != (RebaseResult{}) || errors.Is(err, ErrOutcomeUnknown) {
			t.Fatalf("%s: result %+v, err %v", name, res, err)
		}
	}
}

// TestCompatV2DaemonV1Request: a v2 daemon answers raw v1 requests with a
// v2 version error, which the v0.3.0 client logic reads as a mismatch.
func TestCompatV2DaemonV1Request(t *testing.T) {
	sock := serveHandler(t, &fakeHandler{snap: blastSnapshot()})
	for _, line := range []string{`{"v":1,"op":"report"}`, `{"v":1,"op":"ping"}`} {
		f := rawDial(t, sock)
		if _, err := f.Write([]byte(line + "\n")); err != nil {
			t.Fatal(err)
		}
		reply, err := readLine(f, maxResponse)
		if err != nil {
			t.Fatal(err)
		}
		var r response
		if err := json.Unmarshal(reply, &r); err != nil {
			t.Fatal(err)
		}
		if r.V != 2 || r.OK || !strings.Contains(r.Error, "unsupported protocol version 1") || r.Snapshot != nil {
			t.Fatalf("%s: response %s", line, reply)
		}
		// v0.3.0 decodeResponse: r.V != clientV is a mismatch.
		var v1r v1Response
		if err := json.Unmarshal(reply, &v1r); err != nil || v1r.V == v1ProtocolVersion {
			t.Fatalf("%s: the v1 client would accept %s (%v)", line, reply, err)
		}
	}
	for _, op := range []string{OpReport, OpPing} {
		_, got, _, err := query(sock, request{V: 1, Op: op}, time.Second)
		var ve *VersionError
		if !errors.As(err, &ve) || ve.Daemon != 2 || ve.Client != 1 || got != nil {
			t.Fatalf("%s: (%v, %v), want *VersionError{2,1}", op, got, err)
		}
	}
}
