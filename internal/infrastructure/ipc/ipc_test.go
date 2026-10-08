package ipc

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
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

func TestRespond(t *testing.T) {
	info := sampleInfo()
	snap := sampleSnapshot()
	ready := func() *Snapshot { return snap }
	indexing := func() *Snapshot { return nil }

	cases := []struct {
		name      string
		line      string
		get       func() *Snapshot
		ok        bool
		state     string
		snapshot  bool
		errSubstr string
	}{
		{"report ready", `{"v":1,"op":"report"}`, ready, true, StateReady, true, ""},
		{"report indexing", `{"v":1,"op":"report"}`, indexing, true, StateIndexing, false, ""},
		{"ping", `{"v":1,"op":"ping"}`, ready, true, StateReady, false, ""},
		{"bad version", `{"v":2,"op":"report"}`, ready, false, "", false, "unsupported protocol version"},
		{"unknown op", `{"v":1,"op":"shutdown"}`, ready, false, "", false, "unknown op"},
		{"garbage", `not json`, ready, false, "", false, "malformed"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := respond([]byte(c.line+"\n"), info, c.get)
			if r.V != ProtocolVersion {
				t.Fatalf("v = %d", r.V)
			}
			if r.OK != c.ok {
				t.Fatalf("ok = %v (%+v)", r.OK, r)
			}
			if !c.ok {
				if !strings.Contains(r.Error, c.errSubstr) {
					t.Fatalf("error %q, want %q", r.Error, c.errSubstr)
				}
				return
			}
			if r.Info == nil || *r.Info != info {
				t.Fatalf("info = %+v", r.Info)
			}
			if r.State != c.state {
				t.Fatalf("state = %q, want %q", r.State, c.state)
			}
			if (r.Snapshot != nil) != c.snapshot {
				t.Fatalf("snapshot present = %v", r.Snapshot != nil)
			}
		})
	}
}

func TestDecodeResponse(t *testing.T) {
	if _, _, err := decodeResponse([]byte(`{"v":1,"ok":false,"error":"unsupported protocol version 2"}`), 2, OpReport); err == nil {
		t.Fatal("no error on version mismatch")
	} else {
		var ve *VersionError
		if !errors.As(err, &ve) || ve.Daemon != 1 || ve.Client != 2 {
			t.Fatalf("err = %v, want *VersionError{1,2}", err)
		}
	}
	if _, _, err := decodeResponse([]byte(`{"v":1,"ok":false,"error":"unknown op"}`), 1, OpReport); err == nil ||
		!strings.Contains(err.Error(), "unknown op") {
		t.Fatalf("err = %v", err)
	}
	if _, _, err := decodeResponse([]byte(`{"v":1,"ok":true,"info":{"pid":1},"state":"ready"}`), 1, OpReport); err == nil {
		t.Fatal("ready report without snapshot accepted")
	}
	if _, _, err := decodeResponse([]byte(`xx`), 1, OpReport); err == nil {
		t.Fatal("garbage accepted")
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
