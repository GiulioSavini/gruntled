package ipc

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"sync"
	"testing"

	"github.com/GiulioSavini/gruntled/internal/infrastructure/statusfile"
)

// shortDir returns a short private directory (os.MkdirTemp("", "g")), so
// socket paths stay well under the sun_path limit on every platform.
func shortDir(t *testing.T) string {
	t.Helper()
	d, err := os.MkdirTemp("", "g")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(d) })
	return d
}

// invalidUTF8 holds bytes encoding/json would rewrite to U+FFFD in a string.
var invalidUTF8 = []byte{'a', 0xff, 0xfe, '\n', 0x1b, '[', 'z'}

func sampleSnapshot() *Snapshot {
	return &Snapshot{
		State:      StateReady,
		HasErrors:  true,
		Generation: 7,
		Text:       append([]byte("text "), invalidUTF8...),
		Summary:    []byte("1 error\n"),
		JSON:       []byte("{\"x\":1}\n"),
		SARIF:      []byte{0x00, 0xff, 0x80},
	}
}

func sampleInfo() Info {
	return Info{PID: 4242, Version: "v0.3.0", Root: "/repo", StatusPath: "/run/x/status"}
}

func equalSnapshot(t *testing.T, got, want *Snapshot) {
	t.Helper()
	if got == nil {
		t.Fatal("snapshot is nil")
	}
	if got.State != want.State || got.LastError != want.LastError ||
		got.HasErrors != want.HasErrors || got.Generation != want.Generation {
		t.Fatalf("snapshot header = %+v, want %+v", got, want)
	}
	for _, f := range []struct {
		name      string
		got, want []byte
	}{
		{"Text", got.Text, want.Text},
		{"Summary", got.Summary, want.Summary},
		{"JSON", got.JSON, want.JSON},
		{"SARIF", got.SARIF, want.SARIF},
	} {
		if !bytes.Equal(f.got, f.want) {
			t.Fatalf("%s = %q, want %q", f.name, f.got, f.want)
		}
	}
}

func TestSnapshotJSONByteExact(t *testing.T) {
	want := sampleSnapshot()
	want.LastError = "boom"
	b, err := json.Marshal(want)
	if err != nil {
		t.Fatal(err)
	}
	var got Snapshot
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatal(err)
	}
	equalSnapshot(t, &got, want)
}

func TestCheckSockPath(t *testing.T) {
	ok := "/" + strings.Repeat("a", MaxSockPath-1)
	if len(ok) != 103 {
		t.Fatalf("len = %d", len(ok))
	}
	if err := CheckSockPath(ok); err != nil {
		t.Fatalf("103-byte path rejected: %v", err)
	}
	long := ok + "b"
	err := CheckSockPath(long)
	if err == nil {
		t.Fatal("104-byte path accepted")
	}
	for _, want := range []string{"104", "XDG_RUNTIME_DIR", "TMPDIR"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not mention %q", err, want)
		}
	}
}

func TestVersionErrorMessage(t *testing.T) {
	msg := (&VersionError{Daemon: 1, Client: 2}).Error()
	if !strings.Contains(msg, "1") || !strings.Contains(msg, "2") {
		t.Fatalf("message %q does not name both versions", msg)
	}
}

// fakeHandler is a Handler over a fixed snapshot that records Rebase calls.
type fakeHandler struct {
	snap *Snapshot
	res  RebaseResult
	err  error

	mu     sync.Mutex
	calls  int
	expect *uint64 // copy of the last expect, nil when none was sent
}

func (h *fakeHandler) Snapshot() *Snapshot { return h.snap }

func (h *fakeHandler) Rebase(expect *uint64) (RebaseResult, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.calls++
	h.expect = nil
	if expect != nil {
		e := *expect
		h.expect = &e
	}
	return h.res, h.err
}

func (h *fakeHandler) rebaseCalls() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.calls
}

func sampleBlast() *BlastView {
	return &BlastView{
		BaselineGeneration: 3,
		BaselineSource:     "watch start",
		HasErrors:          true,
		BrokenErrors:       2,
		Text:               append([]byte("baseline: daemon generation 3\n"), invalidUTF8...),
		JSON:               []byte("{\"version\":2}\n"),
	}
}

// blastSnapshot is sampleSnapshot with a blast view.
func blastSnapshot() *Snapshot {
	s := sampleSnapshot()
	s.Blast = sampleBlast()
	return s
}

func TestSnapshotJSONByteExactBlast(t *testing.T) {
	want := blastSnapshot()
	want.Blast.JSON = append([]byte{0x01, 0x7f}, invalidUTF8...)
	b, err := json.Marshal(want)
	if err != nil {
		t.Fatal(err)
	}
	var got Snapshot
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatal(err)
	}
	equalSnapshot(t, &got, want)
	if !reflect.DeepEqual(got.Blast, want.Blast) {
		t.Fatalf("blast = %+v, want %+v", got.Blast, want.Blast)
	}
	plain, err := json.Marshal(sampleSnapshot())
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(plain, []byte(`"blast"`)) {
		t.Fatalf("nil Blast encoded a blast key: %s", plain)
	}
}

func TestRequestFitsMaxRequest(t *testing.T) {
	max := uint64(18446744073709551615)
	b, err := encodeLine(request{V: ProtocolVersion, Op: OpRebase, Expect: &max})
	if err != nil {
		t.Fatal(err)
	}
	if len(b) >= 64 {
		t.Fatalf("largest valid request is %d bytes: %s", len(b), b)
	}
	if maxRequest != 4<<10 {
		t.Fatalf("maxRequest = %d, want 4 KiB", maxRequest)
	}
}

func TestRespond(t *testing.T) {
	info := sampleInfo()
	snap := blastSnapshot()
	ready := &fakeHandler{snap: snap, res: RebaseResult{Generation: 9, BrokenErrors: 1}}
	indexing := &fakeHandler{}

	cases := []struct {
		name  string
		line  string
		h     *fakeHandler
		ok    bool
		state string
		code  string
		msg   string
		check func(t *testing.T, r response)
	}{
		{name: "report ready", line: `{"v":2,"op":"report"}`, h: ready, ok: true, state: StateReady, check: func(t *testing.T, r response) {
			if r.Snapshot == nil || r.Snapshot.Blast != nil {
				t.Fatalf("report snapshot = %+v, want check bytes and no blast", r.Snapshot)
			}
			equalSnapshot(t, r.Snapshot, snap)
			if snap.Blast == nil {
				t.Fatal("respond cleared the handler's Blast")
			}
		}},
		{name: "report indexing", line: `{"v":2,"op":"report"}`, h: indexing, ok: true, state: StateIndexing, check: noSnapshot},
		{name: "blast ready", line: `{"v":2,"op":"blast"}`, h: ready, ok: true, state: StateReady, check: func(t *testing.T, r response) {
			s := r.Snapshot
			if s == nil || s.Text != nil || s.Summary != nil || s.JSON != nil || s.SARIF != nil {
				t.Fatalf("blast snapshot = %+v, want no check bytes", s)
			}
			if s.Blast != snap.Blast || s.State != snap.State || s.LastError != snap.LastError ||
				s.HasErrors != snap.HasErrors || s.Generation != snap.Generation {
				t.Fatalf("blast snapshot header/view = %+v, want %+v", s, snap)
			}
			if snap.Text == nil {
				t.Fatal("respond cleared the handler's Text")
			}
		}},
		{name: "blast indexing", line: `{"v":2,"op":"blast"}`, h: indexing, ok: true, state: StateIndexing, check: noSnapshot},
		{name: "ping", line: `{"v":2,"op":"ping"}`, h: ready, ok: true, state: StateReady, check: noSnapshot},
		{name: "rebase ok", line: `{"v":2,"op":"rebase","expect":4}`, h: &fakeHandler{snap: snap, res: RebaseResult{Generation: 9, BrokenErrors: 1}}, ok: true, check: func(t *testing.T, r response) {
			if r.Rebase == nil || *r.Rebase != (RebaseResult{Generation: 9, BrokenErrors: 1}) || r.Snapshot != nil {
				t.Fatalf("rebase = %+v, snapshot %+v", r.Rebase, r.Snapshot)
			}
		}},
		{name: "rebase refused", line: `{"v":2,"op":"rebase","expect":4}`, h: &fakeHandler{snap: snap, err: Refuse(CodeExpectMismatch, "generation moved")}, code: CodeExpectMismatch, msg: "generation moved"},
		{name: "rebase plain error", line: `{"v":2,"op":"rebase"}`, h: &fakeHandler{snap: snap, err: errors.New("open /secret/path: denied")}, code: CodeInternal, msg: "internal error"},
		{name: "v1", line: `{"v":1,"op":"report"}`, h: ready, msg: "unsupported protocol version 1"},
		{name: "v3", line: `{"v":3,"op":"report"}`, h: ready, msg: "unsupported protocol version 3"},
		{name: "v1 unknown field", line: `{"v":1,"op":"report","x":1}`, h: ready, msg: "unsupported protocol version 1"},
		{name: "v3 unknown op", line: `{"v":3,"op":"shutdown","x":1}`, h: ready, msg: "unsupported protocol version 3"},
		{name: "unknown field", line: `{"v":2,"op":"report","x":1}`, h: ready, code: CodeMalformed, msg: "malformed request"},
		{name: "expect on report", line: `{"v":2,"op":"report","expect":1}`, h: ready, code: CodeMalformed, msg: "malformed request"},
		{name: "expect on blast", line: `{"v":2,"op":"blast","expect":1}`, h: ready, code: CodeMalformed, msg: "malformed request"},
		{name: "negative expect", line: `{"v":2,"op":"rebase","expect":-1}`, h: ready, code: CodeMalformed, msg: "malformed request"},
		{name: "fractional expect", line: `{"v":2,"op":"rebase","expect":1.5}`, h: ready, code: CodeMalformed, msg: "malformed request"},
		{name: "expect over 2^64-1", line: `{"v":2,"op":"rebase","expect":18446744073709551616}`, h: ready, code: CodeMalformed, msg: "malformed request"},
		{name: "trailing value", line: `{"v":2,"op":"report"}{}{}`, h: ready, code: CodeMalformed, msg: "malformed request"},
		{name: "trailing values only", line: `{}{}`, h: ready, code: CodeMalformed, msg: "malformed request"},
		{name: "garbage", line: `not json`, h: ready, code: CodeMalformed, msg: "malformed request"},
		{name: "expect null", line: `{"v":2,"op":"report","expect":null}`, h: ready, code: CodeMalformed, msg: "malformed request"},
		{name: "case variants", line: `{"V":2,"OP":"rebase","EXPECT":5}`, h: ready, code: CodeMalformed, msg: "malformed request"},
		{name: "duplicate op", line: `{"v":2,"op":"report","op":"rebase","expect":3}`, h: ready, code: CodeMalformed, msg: "malformed request"},
		{name: "duplicate v", line: `{"v":1,"v":2,"op":"report"}`, h: ready, code: CodeMalformed, msg: "malformed request"},
		{name: "extra space", line: `{"v":2, "op":"report"}`, h: ready, code: CodeMalformed, msg: "malformed request"},
		{name: "rebase extra space", line: `{"v":2,"op":"rebase", "expect":3}`, h: ready, code: CodeMalformed, msg: "malformed request"},
		{name: "unknown op", line: `{"v":2,"op":"shutdown"}`, h: ready, code: CodeUnknownOp, msg: `unknown op "shutdown"`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			before := c.h.rebaseCalls()
			r, _ := respond([]byte(c.line+"\n"), info, c.h)
			if r.V != ProtocolVersion {
				t.Fatalf("v = %d", r.V)
			}
			if r.OK != c.ok {
				t.Fatalf("ok = %v (%+v)", r.OK, r)
			}
			if !c.ok {
				if !strings.Contains(r.Error, c.msg) || r.Code != c.code {
					t.Fatalf("refusal (%q, %q), want code %q containing %q", r.Code, r.Error, c.code, c.msg)
				}
				if strings.Contains(r.Error, "/secret/path") {
					t.Fatalf("handler error text leaked: %q", r.Error)
				}
				if r.Snapshot != nil || r.Rebase != nil || r.Info != nil {
					t.Fatalf("refusal carries data: %+v", r)
				}
				if c.code != CodeExpectMismatch && c.code != CodeInternal && c.h.rebaseCalls() != before {
					t.Fatalf("Handler.Rebase called for a refused request")
				}
				return
			}
			if r.Info == nil || *r.Info != info {
				t.Fatalf("info = %+v", r.Info)
			}
			if r.State != c.state {
				t.Fatalf("state = %q, want %q", r.State, c.state)
			}
			if c.check != nil {
				c.check(t, r)
			}
		})
	}
	if n := ready.rebaseCalls(); n != 0 {
		t.Fatalf("Handler.Rebase reached %d times through refused requests", n)
	}
}

func noSnapshot(t *testing.T, r response) {
	t.Helper()
	if r.Snapshot != nil || r.Rebase != nil {
		t.Fatalf("snapshot %+v, rebase %+v; want neither", r.Snapshot, r.Rebase)
	}
}

func TestRespondPassesExpect(t *testing.T) {
	h := &fakeHandler{snap: blastSnapshot()}
	if r, op := respond([]byte(`{"v":2,"op":"rebase","expect":18446744073709551615}`+"\n"), sampleInfo(), h); !r.OK || op != OpRebase {
		t.Fatalf("respond = %+v, %q", r, op)
	}
	if h.expect == nil || *h.expect != 18446744073709551615 {
		t.Fatalf("expect = %v", h.expect)
	}
	if r, _ := respond([]byte(`{"v":2,"op":"rebase"}`+"\n"), sampleInfo(), h); !r.OK || h.expect != nil {
		t.Fatalf("respond = %+v, expect %v; want ok and nil", r, h.expect)
	}
}

func TestDecodeResponse(t *testing.T) {
	t.Run("version mismatch", func(t *testing.T) {
		_, _, _, err := decodeResponse([]byte(`{"v":1,"ok":false,"error":"unsupported protocol version 2"}`), 2, OpReport)
		var ve *VersionError
		if !errors.As(err, &ve) || ve.Daemon != 1 || ve.Client != 2 {
			t.Fatalf("err = %v, want *VersionError{1,2}", err)
		}
	})
	t.Run("refused", func(t *testing.T) {
		_, _, _, err := decodeResponse([]byte(`{"v":2,"ok":false,"error":"unknown op \"x\"","code":"unknown_op"}`), 2, OpReport)
		var re *RefusedError
		if !errors.As(err, &re) || re.Code != CodeUnknownOp || re.Msg != `unknown op "x"` {
			t.Fatalf("err = %#v, want *RefusedError{unknown_op}", err)
		}
		if err.Error() != `ipc: daemon refused the request: unknown op "x"` {
			t.Fatalf("text %q", err)
		}
	})
	malformed := []struct {
		name, line, op, want string
	}{
		{"ready report without snapshot", `{"v":2,"ok":true,"info":{"pid":1},"state":"ready"}`, OpReport, "no snapshot"},
		{"garbage", `xx`, OpReport, "malformed response"},
		{"blast ready without view", `{"v":2,"ok":true,"info":{"pid":1},"state":"ready","snapshot":{"state":"ready"}}`, OpBlast, "no blast view"},
		{"blast failed without view", `{"v":2,"ok":true,"info":{"pid":1},"state":"failed","snapshot":{"state":"failed","last_error":"x"}}`, OpBlast, "no blast view"},
		{"blast empty view", `{"v":2,"ok":true,"info":{"pid":1},"state":"ready","snapshot":{"state":"ready","blast":{}}}`, OpBlast, "empty blast view"},
		{"blast text only", `{"v":2,"ok":true,"info":{"pid":1},"state":"ready","snapshot":{"state":"ready","blast":{"text":"eA=="}}}`, OpBlast, "empty blast view"},
		{"blast json only", `{"v":2,"ok":true,"info":{"pid":1},"state":"ready","snapshot":{"state":"ready","blast":{"json":"eA=="}}}`, OpBlast, "empty blast view"},
		{"rebase ok without result", `{"v":2,"ok":true,"info":{"pid":1}}`, OpRebase, "malformed"},
	}
	for _, c := range malformed {
		t.Run(c.name, func(t *testing.T) {
			_, s, res, err := decodeResponse([]byte(c.line), 2, c.op)
			if err == nil || !strings.Contains(err.Error(), c.want) || s != nil || res != nil {
				t.Fatalf("(%v, %v, %v), want an error containing %q", s, res, err, c.want)
			}
		})
	}
	for _, line := range []string{
		`{"v":2,"ok":true,"info":{"pid":1},"state":"ready","snapshot":{"state":"ready","blast":{"too_large":5}}}`,
		`{"v":2,"ok":true,"info":{"pid":1},"state":"ready","snapshot":{"state":"ready","blast":{"error":"x"}}}`,
		`{"v":2,"ok":true,"info":{"pid":1},"state":"ready","snapshot":{"state":"ready","blast":{"text":"eA==","json":"eA=="}}}`,
	} {
		if _, s, _, err := decodeResponse([]byte(line), 2, OpBlast); err != nil || s == nil || s.Blast == nil {
			t.Fatalf("%s: (%v, %v)", line, s, err)
		}
	}
	t.Run("blast indexing", func(t *testing.T) {
		info, s, res, err := decodeResponse([]byte(`{"v":2,"ok":true,"info":{"pid":7},"state":"indexing"}`), 2, OpBlast)
		if err != nil || s != nil || res != nil || info.PID != 7 {
			t.Fatalf("(%+v, %v, %v, %v)", info, s, res, err)
		}
	})
	t.Run("rebase ok", func(t *testing.T) {
		info, _, res, err := decodeResponse([]byte(`{"v":2,"ok":true,"info":{"pid":7},"rebase":{"generation":4,"broken_errors":2}}`), 2, OpRebase)
		if err != nil || res == nil || *res != (RebaseResult{Generation: 4, BrokenErrors: 2}) || info.PID != 7 {
			t.Fatalf("(%+v, %v, %v)", info, res, err)
		}
	})
}

func TestValidBlastView(t *testing.T) {
	for _, c := range []struct {
		v    *BlastView
		want string
	}{
		{nil, "no blast view"},
		{&BlastView{}, "empty blast view"},
		{&BlastView{Text: []byte("x")}, "empty blast view"},
		{&BlastView{JSON: []byte("x")}, "empty blast view"},
		{&BlastView{TooLarge: 1}, ""},
		{&BlastView{Error: "x"}, ""},
		{&BlastView{Text: []byte("x"), JSON: []byte("y")}, ""},
	} {
		err := ValidBlastView(c.v)
		if (c.want == "") != (err == nil) || (err != nil && err.Error() != c.want) {
			t.Fatalf("ValidBlastView(%+v) = %v, want %q", c.v, err, c.want)
		}
	}
}

func dumpWriter(t *testing.T) (*statusfile.Writer, string) {
	t.Helper()
	dir := shortDir(t)
	return statusfile.NewWriter(filepath.Join(dir, DumpName)), dir
}

func TestDumpRoundTrip(t *testing.T) {
	w, dir := dumpWriter(t)
	info := sampleInfo()
	want := sampleSnapshot()
	if err := WriteDump(w, info, want); err != nil {
		t.Fatal(err)
	}
	gotInfo, got, err := ReadDump(w.Path)
	if err != nil {
		t.Fatal(err)
	}
	if gotInfo != info {
		t.Fatalf("info = %+v", gotInfo)
	}
	equalSnapshot(t, got, want)

	if runtime.GOOS != "windows" {
		fi, err := os.Stat(w.Path)
		if err != nil {
			t.Fatal(err)
		}
		if fi.Mode().Perm() != 0o600 {
			t.Fatalf("mode = %v", fi.Mode().Perm())
		}
	}

	// Replace with an indexing dump: atomic, no temp leftovers.
	if err := WriteDump(w, info, nil); err != nil {
		t.Fatal(err)
	}
	_, got, err = ReadDump(w.Path)
	if err != nil {
		t.Fatal(err)
	}
	if got != nil {
		t.Fatalf("indexing dump returned snapshot %+v", got)
	}
	ents, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(ents) != 1 || ents[0].Name() != DumpName {
		names := make([]string, 0, len(ents))
		for _, e := range ents {
			names = append(names, e.Name())
		}
		t.Fatalf("dir holds %v, want only %s", names, DumpName)
	}
}

func TestReadDumpErrors(t *testing.T) {
	dir := shortDir(t)
	if _, _, err := ReadDump(filepath.Join(dir, "missing")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("missing: err = %v", err)
	}
	p := filepath.Join(dir, DumpName)
	for _, body := range []string{"garbage\n", "", `{"v":1,"info":{"pid":1},"state":"ready"}`} {
		if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, _, err := ReadDump(p); err == nil {
			t.Fatalf("body %q accepted", body)
		}
	}
	if err := os.WriteFile(p, []byte(`{"v":9,"info":{"pid":1},"state":"indexing"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	var ve *VersionError
	if _, _, err := ReadDump(p); !errors.As(err, &ve) || ve.Daemon != 9 || ve.Client != ProtocolVersion {
		t.Fatalf("version: err = %v", err)
	}
}

// setCap lowers responseCap for one test. No test allocates 64 MiB.
func setCap(t *testing.T, n int) {
	t.Helper()
	old := responseCap
	responseCap = n
	t.Cleanup(func() { responseCap = old })
}

// bigBlastSnapshot has a blast view of about n bytes and small check bytes.
func bigBlastSnapshot(n int) *Snapshot {
	s := blastSnapshot()
	s.Blast.Text = bytes.Repeat([]byte("t"), n/2)
	s.Blast.JSON = bytes.Repeat([]byte("j"), n/2)
	return s
}

func TestCapString(t *testing.T) {
	for n, want := range map[int]string{64 << 20: "64 MiB", 8 << 10: "8 KiB", 1000: "1000 bytes"} {
		if got := capString(n); got != want {
			t.Fatalf("capString(%d) = %q, want %q", n, got, want)
		}
	}
}

var blastRequest = []byte(`{"v":2,"op":"blast"}` + "\n")

func TestSealBlast(t *testing.T) {
	info := sampleInfo()
	t.Run("in cap keeps bytes, line equals the per-request encoding", func(t *testing.T) {
		s := blastSnapshot()
		view := s.Blast
		Seal(info, s)
		if !s.Sealed() || s.Blast != view {
			t.Fatalf("sealed %v, view replaced %v", s.Sealed(), s.Blast != view)
		}
		r, _ := respond(blastRequest, info, &fakeHandler{snap: s})
		want, err := encodeLine(r)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(s.blastLine, want) {
			t.Fatalf("sealed line\n%s\nwant\n%s", s.blastLine, want)
		}
		if got := answer(blastRequest, info, &fakeHandler{snap: s}); !bytes.Equal(got, want) {
			t.Fatalf("answer = %s, want %s", got, want)
		}
		first := append([]byte(nil), s.blastLine...)
		Seal(info, s)
		if !bytes.Equal(s.blastLine, first) {
			t.Fatal("second Seal changed the line")
		}
	})
	t.Run("over cap swaps in a too-large copy", func(t *testing.T) {
		setCap(t, 8<<10)
		s := bigBlastSnapshot(16 << 10)
		orig := s.Blast
		origText := append([]byte(nil), orig.Text...)
		Seal(info, s)
		v := s.Blast
		if v == orig || v.Text != nil || v.JSON != nil || v.TooLarge <= int64(8<<10) {
			t.Fatalf("view after Seal = %+v", v)
		}
		if v.BaselineGeneration != orig.BaselineGeneration || v.BaselineSource != orig.BaselineSource ||
			v.HasErrors != orig.HasErrors || v.BrokenErrors != orig.BrokenErrors || v.Error != "" {
			t.Fatalf("metadata changed: %+v vs %+v", v, orig)
		}
		if !bytes.Equal(orig.Text, origText) || orig.TooLarge != 0 {
			t.Fatal("Seal mutated the shared view in place")
		}
		if len(s.blastLine) > responseCap || !bytes.Contains(s.blastLine, []byte(`"too_large":`)) {
			t.Fatalf("sealed line %d bytes: %.200s", len(s.blastLine), s.blastLine)
		}
		first := append([]byte(nil), s.blastLine...)
		Seal(info, s)
		if !bytes.Equal(s.blastLine, first) {
			t.Fatal("second Seal changed the line")
		}
		if _, got, _, err := decodeResponse(answer(blastRequest, info, &fakeHandler{snap: s}), ProtocolVersion, OpBlast); err != nil || got.Blast.TooLarge != v.TooLarge {
			t.Fatalf("decoded (%+v, %v)", got, err)
		}
	})
	t.Run("struct copy re-sealed as failed", func(t *testing.T) {
		s := blastSnapshot()
		Seal(info, s)
		ready := append([]byte(nil), s.blastLine...)
		c := *s
		c.State, c.LastError = StateFailed, "boom"
		Seal(info, &c)
		if !bytes.Contains(c.blastLine, []byte(`"state":"failed"`)) || !bytes.Contains(c.blastLine, []byte(`"last_error":"boom"`)) {
			t.Fatalf("copy line %s", c.blastLine)
		}
		if !bytes.Equal(s.blastLine, ready) || c.Blast != s.Blast {
			t.Fatal("re-sealing the copy changed the original or its view")
		}
	})
}

func TestBlastRequestAllocsConstant(t *testing.T) {
	info := sampleInfo()
	measure := func(t *testing.T, s *Snapshot) {
		t.Helper()
		h := &fakeHandler{snap: s}
		_ = answer(blastRequest, info, h) // warm up
		var before, after runtime.MemStats
		runtime.GC()
		runtime.ReadMemStats(&before)
		for range 20 {
			if b := answer(blastRequest, info, h); len(b) == 0 {
				t.Fatal("empty answer")
			}
		}
		runtime.ReadMemStats(&after)
		d := after.TotalAlloc - before.TotalAlloc
		t.Logf("20 requests allocated %d bytes (sealed line %d bytes)", d, len(s.blastLine))
		if d >= 64<<10 {
			t.Fatalf("20 requests allocated %d bytes, want < 64 KiB", d)
		}
	}
	t.Run("in cap, 2 MiB view", func(t *testing.T) {
		s := bigBlastSnapshot(2 << 20)
		Seal(info, s)
		if s.Blast.TooLarge != 0 {
			t.Fatal("2 MiB view marked too large")
		}
		measure(t, s)
	})
	t.Run("over cap", func(t *testing.T) {
		setCap(t, 8<<10)
		s := bigBlastSnapshot(64 << 10)
		Seal(info, s)
		if s.Blast.TooLarge == 0 {
			t.Fatal("over-cap view kept")
		}
		measure(t, s)
	})
}

func TestWriteDumpDropsOversizeView(t *testing.T) {
	setCap(t, 8<<10)
	info := sampleInfo()
	t.Run("over cap", func(t *testing.T) {
		w, _ := dumpWriter(t)
		s := bigBlastSnapshot(16 << 10)
		orig := s.Blast
		if err := WriteDump(w, info, s); err != nil {
			t.Fatal(err)
		}
		if s.Blast != orig || len(orig.Text) == 0 {
			t.Fatal("WriteDump changed the caller's snapshot")
		}
		raw, err := os.ReadFile(w.Path)
		if err != nil {
			t.Fatal(err)
		}
		if len(raw) > responseCap {
			t.Fatalf("dump is %d bytes, over the cap", len(raw))
		}
		_, got, err := ReadDump(w.Path)
		if err != nil {
			t.Fatal(err)
		}
		equalSnapshot(t, got, s)
		if got.Blast == nil || got.Blast.Text != nil || got.Blast.JSON != nil || got.Blast.TooLarge <= int64(8<<10) ||
			got.Blast.BaselineGeneration != orig.BaselineGeneration || got.Blast.BrokenErrors != orig.BrokenErrors {
			t.Fatalf("dumped view = %+v", got.Blast)
		}
	})
	t.Run("in cap", func(t *testing.T) {
		w, _ := dumpWriter(t)
		s := blastSnapshot()
		if err := WriteDump(w, info, s); err != nil {
			t.Fatal(err)
		}
		_, got, err := ReadDump(w.Path)
		if err != nil {
			t.Fatal(err)
		}
		equalSnapshot(t, got, s)
		if !reflect.DeepEqual(got.Blast, s.Blast) {
			t.Fatalf("view = %+v, want %+v", got.Blast, s.Blast)
		}
	})
	t.Run("ReadDump over cap names the cap", func(t *testing.T) {
		p := filepath.Join(shortDir(t), DumpName)
		if err := os.WriteFile(p, bytes.Repeat([]byte("x"), 9<<10), 0o600); err != nil {
			t.Fatal(err)
		}
		_, _, err := ReadDump(p)
		if !errors.Is(err, errLineTooLong) || !strings.Contains(err.Error(), "8 KiB") {
			t.Fatalf("err = %v", err)
		}
	})
}
