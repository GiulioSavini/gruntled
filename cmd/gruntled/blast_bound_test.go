//go:build linux || darwin

package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/GiulioSavini/gruntled/internal/infrastructure/hclconv"
)

// boundUnits is the length of the worst-case chain (BLAST-11).
const boundUnits = 5_000

// boundTree writes a linear chain live/u0000 <- live/u0001 <- ... into dir:
// u0000 instantiates modules/big, every other unit depends on the previous
// one by a default dependency block that reads no output. modules/big
// declares variable "big" with typ as its type.
func boundTree(t *testing.T, dir, typ string) {
	t.Helper()
	write := func(rel, data string) {
		p := filepath.Join(dir, filepath.FromSlash(rel))
		mustDo(t, os.MkdirAll(filepath.Dir(p), 0o755))
		mustDo(t, os.WriteFile(p, []byte(data), 0o644))
	}
	write("modules/big/main.tf", "variable \"big\" {\n  type = "+typ+"\n}\n")
	write("live/u0000/terragrunt.hcl", "terraform {\n  source = \"../../modules/big\"\n}\n")
	for i := 1; i < boundUnits; i++ {
		write(fmt.Sprintf("live/u%04d/terragrunt.hcl", i), fmt.Sprintf("dependency \"p\" {\n  config_path = \"../u%04d\"\n}\n", i-1))
	}
}

// bigObjectType is object({a000000=string,...}) of about size bytes, its
// last attribute of type last.
func bigObjectType(size int, last string) string {
	var b strings.Builder
	b.WriteString("object({")
	for i := 0; b.Len() < size-40; i++ {
		fmt.Fprintf(&b, "a%06d=string,", i)
	}
	b.WriteString("z=" + last + "})")
	return b.String()
}

// boundRun runs the built binary and returns stdout, the exit code and the
// peak RSS in bytes.
func boundRun(t *testing.T, bin, dir string, args ...string) (string, int, int64, time.Duration) {
	t.Helper()
	cmd := exec.Command(bin, args...)
	cmd.Dir = dir
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	start := time.Now()
	err := cmd.Run()
	took := time.Since(start)
	code := 0
	if err != nil {
		ee, ok := err.(*exec.ExitError)
		if !ok {
			t.Fatalf("%v: %v", args, err)
		}
		code = ee.ExitCode()
	}
	if stderr.Len() > 0 {
		t.Fatalf("%v: stderr:\n%s", args, stderr.String())
	}
	ru, ok := cmd.ProcessState.SysUsage().(*syscall.Rusage)
	if !ok {
		t.Fatal("no rusage")
	}
	rss := int64(ru.Maxrss)
	if runtime.GOOS == "linux" {
		rss *= 1024 // linux reports KiB, darwin bytes
	}
	return stdout.String(), code, rss, took
}

// TestBlastSizeBound (BLAST-11, sec #105, #249b): on a 5,000-unit chain
// whose seed module changes one attribute of a ~4 MiB variable type, blast
// output stays linear (type strings once per module) and peak RSS stays
// within control + 64 x type bytes, where the control is the same chain
// with a module file of the same size whose 4 MiB object is a default and
// whose type change is tiny, run on the same machine. Absolute values are
// only logged.
func TestBlastSizeBound(t *testing.T) {
	if testing.Short() {
		t.Skip("builds the binary and runs a 5,000-unit chain twice")
	}
	bin := benchBuildGruntled(t)

	run := func(name, oldType, newType string) (text, js string, rss int64) {
		root := t.TempDir()
		base, cur := filepath.Join(root, "base"), filepath.Join(root, "cur")
		boundTree(t, base, oldType)
		boundTree(t, cur, newType)
		text, code, rssText, tookText := boundRun(t, bin, root, "blast", "--base", "base", "cur")
		if code != exitOK {
			t.Fatalf("%s text: exit %d\n%.2000s", name, code, text)
		}
		js, code, rssJSON, tookJSON := boundRun(t, bin, root, "blast", "--base", "base", "--format", "json", "cur")
		if code != exitOK {
			t.Fatalf("%s json: exit %d", name, code)
		}
		t.Logf("%s: text %d bytes, %v, maxrss %d MiB; json %d bytes, %v, maxrss %d MiB",
			name, len(text), tookText, rssText>>20, len(js), tookJSON, rssJSON>>20)
		return text, js, max(rssText, rssJSON)
	}

	oldType := bigObjectType(hclconv.MaxFileBytes-200, "string")
	newType := bigObjectType(hclconv.MaxFileBytes-200, "number")
	typeBytes := len(newType)

	// Logged only: the chain with a tiny type and a tiny module file.
	run("tiny file (tiny type)", "string", "number")
	// The control: the same chain and a module file of the same size,
	// the 4 MiB object in default (parsed, never a type) and a tiny type
	// change. Lexing and parsing a MaxFileBytes file in both trees costs
	// hundreds of MiB before Phase 15 (see hclconv.MaxFileBytes); the
	// bound isolates what the type facts add on top of that.
	_, _, rssControl := run("control (same-size file, tiny type)", "string\n  default = "+oldType, "number\n  default = "+oldType)

	text, js, rssBig := run(fmt.Sprintf("big (%d-byte type)", typeBytes), oldType, newType)

	var doc struct {
		Impacted []struct {
			Unit     string `json:"unit"`
			Distance int    `json:"distance"`
		} `json:"impacted"`
		Changes []struct {
			Module      string `json:"module"`
			TypeChanges []struct {
				Kind, Name, Old, New string
			} `json:"type_changes"`
		} `json:"changes"`
	}
	if err := json.Unmarshal([]byte(js), &doc); err != nil {
		t.Fatal(err)
	}
	if len(doc.Impacted) != boundUnits {
		t.Fatalf("Impacted %d units, want %d", len(doc.Impacted), boundUnits)
	}
	seen := make([]bool, boundUnits+1)
	for _, e := range doc.Impacted {
		if e.Distance < 1 || e.Distance > boundUnits || seen[e.Distance] {
			t.Fatalf("distance %d of %s out of range or repeated", e.Distance, e.Unit)
		}
		seen[e.Distance] = true
	}
	if len(doc.Changes) != 1 || len(doc.Changes[0].TypeChanges) != 1 || doc.Changes[0].TypeChanges[0].Kind != "variable_type" {
		t.Fatalf("changes = %+v", doc.Changes)
	}
	tc := doc.Changes[0].TypeChanges[0]
	rendered := len(tc.Old) + len(tc.New)
	t.Logf("rendered old+new %d bytes (source type %d bytes)", rendered, typeBytes)

	if bound := rendered + boundUnits*200 + 1024; len(text) > bound {
		t.Errorf("text %d bytes, over %d", len(text), bound)
	}
	if bound := 2*rendered + boundUnits*400 + 1024; len(js) > bound {
		t.Errorf("json %d bytes, over %d", len(js), bound)
	}
	if strings.Count(text, tc.New) != 1 {
		t.Errorf("text prints the new type string %d times, want 1", strings.Count(text, tc.New))
	}

	bound := rssControl + 64*int64(typeBytes)
	t.Logf("maxrss control %d MiB, big %d MiB, bound %d MiB (control + 64 x %d type bytes); %s/%s, %d CPUs",
		rssControl>>20, rssBig>>20, bound>>20, typeBytes, runtime.GOOS, runtime.GOARCH, runtime.NumCPU())
	if rssBig > bound {
		t.Errorf("maxrss %d MiB over the bound %d MiB", rssBig>>20, bound>>20)
	}
}

// boundDaemon is a built-binary watch daemon on a private runtime dir.
type boundDaemon struct {
	t    *testing.T
	bin  string
	dir  string
	env  []string
	cmd  *exec.Cmd
	errb *syncBuf
	done bool
}

func startBoundDaemon(t *testing.T, bin, dir string) *boundDaemon {
	t.Helper()
	base := shortBase(t)
	rt := filepath.Join(base, "rt")
	mustDo(t, os.Mkdir(rt, 0o700))
	d := &boundDaemon{t: t, bin: bin, dir: dir, env: append(os.Environ(), "XDG_RUNTIME_DIR="+rt, "TMPDIR="+rt), errb: &syncBuf{}}
	d.cmd = exec.Command(bin, "watch", "--poll", "--poll-interval", "50ms", "--status-file", filepath.Join(base, "st", "status"), dir)
	d.cmd.Env = d.env
	d.cmd.Stdout = io.Discard
	d.cmd.Stderr = d.errb
	mustDo(t, d.cmd.Start())
	t.Cleanup(func() {
		if !d.done {
			_ = d.cmd.Process.Kill()
			_ = d.cmd.Wait()
		}
	})
	return d
}

// client runs the built binary against the daemon and returns stdout and
// the exit code; report --blast must print nothing on stderr unless it
// exits 3 (plain text report prints its summary there).
func (d *boundDaemon) client(args ...string) (string, int) {
	d.t.Helper()
	cmd := exec.Command(d.bin, append(args, d.dir)...)
	cmd.Env = d.env
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err := cmd.Run()
	code := 0
	if err != nil {
		ee, ok := err.(*exec.ExitError)
		if !ok {
			d.t.Fatalf("%v: %v", args, err)
		}
		code = ee.ExitCode()
	}
	if stderr.Len() > 0 && code != exitFailure && slices.Contains(args, "--blast") {
		d.t.Fatalf("%v: exit %d, stderr:\n%s", args, code, stderr.String())
	}
	return stdout.String(), code
}

// poll runs cond every 500 ms until it holds, failing after limit.
func (d *boundDaemon) poll(limit time.Duration, what string, cond func() bool) {
	d.t.Helper()
	deadline := time.Now().Add(limit)
	for !cond() {
		if time.Now().After(deadline) {
			d.t.Fatalf("timed out after %v waiting for %s; daemon stderr:\n%s", limit, what, d.errb.String())
		}
		time.Sleep(500 * time.Millisecond)
	}
}

// stop sends SIGTERM, waits, and returns the daemon's peak RSS in bytes.
func (d *boundDaemon) stop() int64 {
	d.t.Helper()
	mustDo(d.t, d.cmd.Process.Signal(syscall.SIGTERM))
	err := d.cmd.Wait()
	d.done = true
	if err != nil {
		d.t.Fatalf("daemon exit: %v; stderr:\n%s", err, d.errb.String())
	}
	ru, ok := d.cmd.ProcessState.SysUsage().(*syscall.Rusage)
	if !ok {
		d.t.Fatal("no rusage")
	}
	rss := int64(ru.Maxrss)
	if runtime.GOOS == "linux" {
		rss *= 1024 // linux reports KiB, darwin bytes
	}
	return rss
}

// TestReportBlastSizeBound (BLAST-11 report half, 16-04): a built-binary
// daemon on the 5,000-unit chain whose seed module changes one attribute of
// a ~4 MiB variable type serves report --blast within TestBlastSizeBound's
// byte bounds (type strings once), with 5,000 Impacted units, and its peak
// RSS stays within the in-run control daemon's (same-size file, tiny type
// change) + 64 x type bytes. Absolute values are only logged.
func TestReportBlastSizeBound(t *testing.T) {
	if testing.Short() {
		t.Skip("builds the binary and runs two daemons on a 5,000-unit chain")
	}
	bin := benchBuildGruntled(t)

	run := func(name, oldType, newType string) (text, js string, rss int64) {
		dir := filepath.Join(t.TempDir(), "repo")
		boundTree(t, dir, oldType)
		d := startBoundDaemon(t, bin, dir)
		start := time.Now()
		d.poll(60*time.Second, name+": first report", func() bool { _, code := d.client("report"); return code == exitOK })
		indexed := time.Since(start)

		p := filepath.Join(dir, "modules", "big", "main.tf")
		mustDo(t, os.WriteFile(p+".tmp", []byte("variable \"big\" {\n  type = "+newType+"\n}\n"), 0o644))
		mustDo(t, os.Rename(p+".tmp", p))
		start = time.Now()
		d.poll(180*time.Second, name+": 5,000 impacted", func() bool {
			out, code := d.client("report", "--blast", "--format", "json")
			if code != exitOK {
				return false
			}
			var doc struct {
				Impacted []json.RawMessage `json:"impacted"`
			}
			if json.Unmarshal([]byte(out), &doc) != nil || len(doc.Impacted) != boundUnits {
				return false
			}
			js = out
			return true
		})
		reindexed := time.Since(start)
		var code int
		if text, code = d.client("report", "--blast"); code != exitOK {
			t.Fatalf("%s: report --blast exit %d\n%.2000s", name, code, text)
		}
		rss = d.stop()
		t.Logf("%s: initial index %v, reindex to 5,000 impacted %v; text %d bytes, json %d bytes; daemon maxrss %d MiB",
			name, indexed.Round(time.Millisecond), reindexed.Round(time.Millisecond), len(text), len(js), rss>>20)
		return text, js, rss
	}

	oldType := bigObjectType(hclconv.MaxFileBytes-200, "string")
	newType := bigObjectType(hclconv.MaxFileBytes-200, "number")
	typeBytes := len(newType)

	// The control: same chain, a module file of the same size whose 4 MiB
	// object is a default (parsed, never a type), and a tiny type change.
	_, _, rssControl := run("control (same-size file, tiny type)", "string\n  default = "+oldType, "number\n  default = "+oldType)
	text, js, rssBig := run(fmt.Sprintf("big (%d-byte type)", typeBytes), oldType, newType)

	var doc struct {
		Impacted []struct {
			Unit     string `json:"unit"`
			Distance int    `json:"distance"`
		} `json:"impacted"`
		Changes []struct {
			TypeChanges []struct {
				Kind, Name, Old, New string
			} `json:"type_changes"`
		} `json:"changes"`
	}
	mustDo(t, json.Unmarshal([]byte(js), &doc))
	seen := make([]bool, boundUnits+1)
	for _, e := range doc.Impacted {
		if e.Distance < 1 || e.Distance > boundUnits || seen[e.Distance] {
			t.Fatalf("distance %d of %s out of range or repeated", e.Distance, e.Unit)
		}
		seen[e.Distance] = true
	}
	if len(doc.Changes) != 1 || len(doc.Changes[0].TypeChanges) != 1 || doc.Changes[0].TypeChanges[0].Kind != "variable_type" {
		t.Fatalf("changes = %+v", doc.Changes)
	}
	tc := doc.Changes[0].TypeChanges[0]
	rendered := len(tc.Old) + len(tc.New)
	t.Logf("rendered old+new %d bytes (source type %d bytes)", rendered, typeBytes)
	if bound := rendered + boundUnits*200 + 1024; len(text) > bound {
		t.Errorf("text %d bytes, over %d", len(text), bound)
	}
	if bound := 2*rendered + boundUnits*400 + 1024; len(js) > bound {
		t.Errorf("json %d bytes, over %d", len(js), bound)
	}
	if strings.Count(text, tc.New) != 1 {
		t.Errorf("text prints the new type string %d times, want 1", strings.Count(text, tc.New))
	}

	bound := rssControl + 64*int64(typeBytes)
	t.Logf("daemon maxrss control %d MiB, big %d MiB, bound %d MiB (control + 64 x %d type bytes); %s/%s, %d CPUs",
		rssControl>>20, rssBig>>20, bound>>20, typeBytes, runtime.GOOS, runtime.GOARCH, runtime.NumCPU())
	if rssBig > bound {
		t.Errorf("daemon maxrss %d MiB over the bound %d MiB", rssBig>>20, bound>>20)
	}
}
