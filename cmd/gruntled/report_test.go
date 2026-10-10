package main

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"unicode"

	"github.com/GiulioSavini/gruntled/internal/infrastructure/ipc"
	"github.com/GiulioSavini/gruntled/internal/infrastructure/statusfile"
	"github.com/GiulioSavini/gruntled/internal/infrastructure/terragrunt"
)

// TestReportUsage: -h exits 0 with the usage text; bad flags, an invalid
// --format and two paths exit 2 with it; help lists report.
func TestReportUsage(t *testing.T) {
	_, stderr, code := runCLI(t, "report", "-h")
	if code != exitOK || !strings.HasPrefix(stderr, "usage: gruntled report ") {
		t.Fatalf("report -h: exit %d, stderr:\n%s", code, stderr)
	}
	for _, args := range [][]string{
		{"report", "--bogus"},
		{"report", "--format", "xml", "."},
		{"report", "a", "b"},
	} {
		stdout, stderr, code := runCLI(t, args...)
		if code != exitUsage || stdout != "" || !strings.Contains(stderr, "usage: gruntled report ") {
			t.Errorf("%v: exit %d, stdout %q, stderr:\n%s", args, code, stdout, stderr)
		}
	}
	_, stderr, _ = runCLI(t, "help")
	if !strings.Contains(stderr, "\n  report  ") || !strings.Contains(stderr, `"gruntled report -h"`) {
		t.Errorf("top usage does not list report:\n%s", stderr)
	}
}

var reportFormats = []string{"text", "json", "sarif"}

// cliResult is one command run: stdout, stderr and exit code.
type cliResult struct {
	stdout, stderr string
	code           int
}

func (r cliResult) String() string {
	return "exit " + strconv.Itoa(r.code) + "\nstdout:\n" + r.stdout + "\nstderr:\n" + r.stderr
}

// runReportAt runs report on dir against the runtime base of env with goos.
func runReportAt(env statusfile.Env, goos, format, dir string) cliResult {
	var out, errb bytes.Buffer
	code := runReportWith([]string{"--format", format, dir}, &out, &errb, reportDeps{env: env, goos: goos})
	return cliResult{out.String(), errb.String(), code}
}

func runCheckAt(format, dir string) cliResult {
	var out, errb bytes.Buffer
	code := run([]string{"check", "--format", format, dir}, &out, &errb)
	return cliResult{out.String(), errb.String(), code}
}

// copyFixture copies testdata/<name> to a fresh temp dir.
func copyFixture(t *testing.T, name string) string {
	t.Helper()
	dir := t.TempDir()
	mustDo(t, os.CopyFS(dir, os.DirFS(filepath.Join("testdata", name))))
	return dir
}

// reportGOOS are the two transports: the socket (any non-windows goos) and
// the lock-probed report file.
var reportGOOS = []string{"linux", "windows"}

// TestReportParity: report prints byte-for-byte what check prints, in every
// format and over both transports, before and after an edit.
func TestReportParity(t *testing.T) {
	for _, goos := range reportGOOS {
		t.Run(goos, func(t *testing.T) {
			dir := copyFixture(t, "sarif-fixture")
			deps := testDeps(t)
			deps.goos = goos
			startWatch(t, dir, deps, "--poll")

			eventually(t, func() bool { return runReportAt(deps.env, goos, "text", dir).code == exitFindings },
				"first report", func() string { return runReportAt(deps.env, goos, "text", dir).String() })
			for _, f := range reportFormats {
				got, want := runReportAt(deps.env, goos, f, dir), runCheckAt(f, dir)
				if want.code != exitFindings || got != want {
					t.Fatalf("%s: report differs from check\nreport %s\ncheck %s", f, got, want)
				}
			}

			// Fix every finding; the reindex must reach report.
			writeFiles(t, dir, map[string]string{
				"live/app/terragrunt.hcl": "dependency \"network\" {\n  config_path = \"../network\"\n}\n\ninputs = {\n  vpc_id = dependency.network.outputs.vpc_id\n}\n",
			})
			retryFS(t, func() error { return os.RemoveAll(filepath.Join(dir, "live", "ring")) })
			if c := runCheckAt("text", dir); c.code != exitOK {
				t.Fatalf("edited fixture is not clean: %s", c)
			}
			var last string
			eventually(t, func() bool {
				for _, f := range reportFormats {
					got, want := runReportAt(deps.env, goos, f, dir), runCheckAt(f, dir)
					if got != want {
						last = f + ": report " + got.String() + "\ncheck " + want.String()
						return false
					}
				}
				return true
			}, "report after the edit", func() string { return last })
		})
	}
}

// TestReportParityClean: a clean repository reports exit 0 in every format.
func TestReportParityClean(t *testing.T) {
	for _, goos := range reportGOOS {
		t.Run(goos, func(t *testing.T) {
			dir := copyFixture(t, "clean-fixture")
			deps := testDeps(t)
			deps.goos = goos
			startWatch(t, dir, deps, "--poll")
			eventually(t, func() bool { return runReportAt(deps.env, goos, "json", dir).code == exitOK },
				"first report", func() string { return runReportAt(deps.env, goos, "json", dir).String() })
			for _, f := range reportFormats {
				if got, want := runReportAt(deps.env, goos, f, dir), runCheckAt(f, dir); got != want || got.code != exitOK {
					t.Fatalf("%s: report %s\ncheck %s", f, got, want)
				}
			}
		})
	}
}

// failedSnapshot is the snapshot of fixture after a failed reindex whose
// raw reason carries control runes.
func failedSnapshot(t *testing.T, fixture string) *ipc.Snapshot {
	t.Helper()
	s, err := buildSnapshot(freshReport(t, fixture), 3)
	mustDo(t, err)
	s.State = ipc.StateFailed
	s.LastError = "parse \x1b[2Jboom\r\nline\x07"
	return s
}

const failedWarning = "gruntled: last reindex failed: parse [2Jboom line; showing the previous result\n"

func assertNoControl(t *testing.T, s string) {
	t.Helper()
	for _, r := range s {
		if r != '\n' && unicode.IsControl(r) {
			t.Fatalf("control rune %q in %q", r, s)
		}
	}
}

// TestReportStates: still indexing is exit 3; after a failed reindex the
// last good bytes are printed with a sanitised warning.
func TestReportStates(t *testing.T) {
	for _, goos := range reportGOOS {
		t.Run(goos+"/indexing", func(t *testing.T) {
			dir := repo(t, "vpc_idd")
			deps := testDeps(t)
			deps.goos = goos
			entered, release := make(chan struct{}), make(chan struct{})
			deps.loaderHook = func(*terragrunt.Loader) {
				close(entered)
				<-release
			}
			startWatch(t, dir, deps, "--poll")
			<-entered
			got := runReportAt(deps.env, goos, "text", dir)
			if got.code != exitFailure || got.stdout != "" || got.stderr != "gruntled: the daemon is still indexing; try again\n" {
				t.Fatalf("report while indexing: %s", got)
			}
			close(release)
			eventually(t, func() bool { return runReportAt(deps.env, goos, "text", dir).code == exitFindings },
				"report after the first index", func() string { return runReportAt(deps.env, goos, "text", dir).String() })
		})
	}

	fixture := "testdata/sarif-fixture"
	snap := failedSnapshot(t, fixture)
	check := func(t *testing.T, env statusfile.Env, goos string) {
		t.Helper()
		for _, f := range reportFormats {
			got, want := runReportAt(env, goos, f, fixture), runCheckAt(f, fixture)
			want.stderr += failedWarning
			if got != want {
				t.Fatalf("%s: report %s\nwant %s", f, got, want)
			}
			if strings.ContainsRune(got.stderr, '\x1b') {
				t.Fatalf("%s: ESC from LastError reached stderr: %q", f, got.stderr)
			}
			assertNoControl(t, got.stderr)
		}
	}
	t.Run("windows/failed", func(t *testing.T) {
		env := envAt(shortBase(t))
		holdDump(t, env, fixture, snap)
		check(t, env, "windows")
	})
	t.Run("socket/failed", func(t *testing.T) {
		if runtime.GOOS != "linux" && runtime.GOOS != "darwin" {
			t.Skip("unix sockets on linux and darwin only")
		}
		env := envAt(shortBase(t))
		rt := ensureRuntime(t, env, fixture)
		ln, err := ipc.Listen(filepath.Join(rt, ipc.SockName))
		mustDo(t, err)
		srv := ipc.Serve(ln, ipc.Info{PID: os.Getpid()}, func() *ipc.Snapshot { return snap })
		t.Cleanup(func() { _ = srv.Close() })
		check(t, env, "linux")
	})
}

// ensureRuntime creates the runtime dir of repoDir under env.
func ensureRuntime(t *testing.T, env statusfile.Env, repoDir string) string {
	t.Helper()
	root, _ := repoRuntime(t, repoDir, watchDeps{env: env})
	rt, err := statusfile.EnsureRepoDir(root, env)
	mustDo(t, err)
	return rt
}

// holdDump acts as a report-file daemon: it holds the lock and writes s.
func holdDump(t *testing.T, env statusfile.Env, repoDir string, s *ipc.Snapshot) string {
	t.Helper()
	rt := ensureRuntime(t, env, repoDir)
	l, err := ipc.TryLock(filepath.Join(rt, ipc.LockName))
	mustDo(t, err)
	t.Cleanup(func() { _ = l.Release() })
	mustDo(t, ipc.WriteDump(statusfile.NewWriter(filepath.Join(rt, ipc.DumpName)), ipc.Info{PID: os.Getpid()}, s))
	return rt
}

// TestReportWindowsFile: the report file is read only while its lock is
// held; a stale file is no daemon.
func TestReportWindowsFile(t *testing.T) {
	fixture := "testdata/sarif-fixture"
	snap, err := buildSnapshot(freshReport(t, fixture), 1)
	mustDo(t, err)
	noDaemon := func(t *testing.T, env statusfile.Env) {
		t.Helper()
		got := runReportAt(env, "windows", "text", fixture)
		if got.code != exitFailure || got.stdout != "" || !strings.Contains(got.stderr, "no watch daemon is running for ") {
			t.Fatalf("stale report file: %s", got)
		}
	}

	t.Run("stale", func(t *testing.T) {
		env := envAt(shortBase(t))
		rt := ensureRuntime(t, env, fixture)
		mustDo(t, ipc.WriteDump(statusfile.NewWriter(filepath.Join(rt, ipc.DumpName)), ipc.Info{PID: 1}, snap))
		noDaemon(t, env)
		// A lock file left behind but not held is no daemon either.
		l, err := ipc.TryLock(filepath.Join(rt, ipc.LockName))
		mustDo(t, err)
		mustDo(t, l.Release())
		noDaemon(t, env)
	})
	t.Run("held", func(t *testing.T) {
		env := envAt(shortBase(t))
		holdDump(t, env, fixture, snap)
		for _, f := range reportFormats {
			if got, want := runReportAt(env, "windows", f, fixture), runCheckAt(f, fixture); got != want {
				t.Fatalf("%s: report %s\ncheck %s", f, got, want)
			}
		}
	})
	t.Run("held/no file yet", func(t *testing.T) {
		env := envAt(shortBase(t))
		rt := ensureRuntime(t, env, fixture)
		l, err := ipc.TryLock(filepath.Join(rt, ipc.LockName))
		mustDo(t, err)
		t.Cleanup(func() { _ = l.Release() })
		if got := runReportAt(env, "windows", "text", fixture); got.code != exitFailure || !strings.Contains(got.stderr, "still indexing") {
			t.Fatalf("held lock without a file: %s", got)
		}
	})
	t.Run("protocol mismatch", func(t *testing.T) {
		env := envAt(shortBase(t))
		rt := holdDump(t, env, fixture, snap)
		mustDo(t, statusfile.NewWriter(filepath.Join(rt, ipc.DumpName)).Write(`{"v":99,"info":{},"state":"ready"}`+"\n"))
		got := runReportAt(env, "windows", "text", fixture)
		want := "gruntled: daemon runs protocol v99, this gruntled speaks v" + strconv.Itoa(ipc.ProtocolVersion) + "; restart the daemon\n"
		if got.code != exitFailure || got.stdout != "" || got.stderr != want {
			t.Fatalf("protocol mismatch: %s", got)
		}
	})
}

// TestReportNoDaemonCreatesNothing: with no daemon report exits 3 and
// leaves the runtime base exactly as it found it.
func TestReportNoDaemonCreatesNothing(t *testing.T) {
	dir := repo(t, "vpc_idd")
	for _, goos := range reportGOOS {
		t.Run(goos+"/empty base", func(t *testing.T) {
			base := shortBase(t)
			got := runReportAt(envAt(base), goos, "text", dir)
			if got.code != exitFailure || got.stdout != "" || !strings.Contains(got.stderr, "gruntled: no watch daemon is running for ") {
				t.Fatalf("no daemon: %s", got)
			}
			ents, err := os.ReadDir(base)
			mustDo(t, err)
			if len(ents) != 0 {
				t.Fatalf("report created %v in the runtime base", ents)
			}
		})
		t.Run(goos+"/daemon gone", func(t *testing.T) {
			env := envAt(shortBase(t))
			rt := ensureRuntime(t, env, dir)
			got := runReportAt(env, goos, "text", dir)
			if got.code != exitFailure || !strings.Contains(got.stderr, "no watch daemon is running for ") {
				t.Fatalf("no daemon: %s", got)
			}
			ents, err := os.ReadDir(rt)
			mustDo(t, err)
			if len(ents) != 0 {
				t.Fatalf("report created %v in the runtime dir", ents)
			}
		})
	}
	t.Run("missing path", func(t *testing.T) {
		got := runReportAt(envAt(shortBase(t)), runtime.GOOS, "text", filepath.Join(dir, "missing"))
		if got.code != exitFailure || !strings.Contains(got.stderr, "cannot open repository") {
			t.Fatalf("missing path: %s", got)
		}
	})
}
