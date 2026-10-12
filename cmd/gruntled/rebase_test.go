package main

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"

	"github.com/GiulioSavini/gruntled/internal/infrastructure/ipc"
	"github.com/GiulioSavini/gruntled/internal/infrastructure/statusfile"
)

// runRebaseAt runs rebase with args against the runtime base of env.
func runRebaseAt(env statusfile.Env, goos string, args ...string) cliResult {
	var out, errb bytes.Buffer
	code := runRebaseWith(args, &out, &errb, reportDeps{env: env, goos: goos})
	return cliResult{out.String(), errb.String(), code}
}

func skipNoUnixSocket(t *testing.T) {
	t.Helper()
	if runtime.GOOS != "linux" && runtime.GOOS != "darwin" {
		t.Skip("unix sockets on linux and darwin only")
	}
}

// servedGeneration is the generation of the daemon's served report.
func servedGeneration(t *testing.T, env statusfile.Env, dir string) uint64 {
	t.Helper()
	_, rt := repoRuntime(t, dir, watchDeps{env: env})
	_, s, err := ipc.Query(filepath.Join(rt, ipc.SockName), ipc.OpReport, queryTimeoutAll)
	if err != nil || s == nil {
		return 0
	}
	return s.Generation
}

func TestRebaseCLI(t *testing.T) {
	skipNoUnixSocket(t)
	dir := repo(t, "vpc_idd") // a GRT001 from the first index
	deps := testDeps(t)
	startWatch(t, dir, deps, "--poll")
	var gen uint64
	eventually(t, func() bool { gen = servedGeneration(t, deps.env, dir); return gen > 0 }, "first report", nil)
	g := strconv.FormatUint(gen, 10)

	got := runRebaseAt(deps.env, "linux", "--expect", g, dir)
	if got.code != exitOK || got.stderr != "" || got.stdout != "gruntled: baseline moved to generation "+g+"; 0 error findings in Broken accepted\n" {
		t.Fatalf("rebase --expect %s: %s", g, got)
	}

	writeFiles(t, dir, map[string]string{
		"db/terragrunt.hcl": "dependency \"vpc\" {\n  config_path = \"../vpc\"\n}\ninputs = { id = dependency.vpc.outputs.nope }\n",
	})
	eventually(t, func() bool { return runReportArgs(deps.env, "linux", "--blast", dir).code == exitFindings },
		"a new GRT001 in Broken", func() string { return runReportArgs(deps.env, "linux", "--blast", dir).String() })
	got = runRebaseAt(deps.env, "linux", dir)
	if got.code != exitOK || !strings.HasSuffix(got.stdout, "; 1 error findings in Broken accepted\n") {
		t.Fatalf("rebase over the new finding: %s", got)
	}
	gen = servedGeneration(t, deps.env, dir)
	g = strconv.FormatUint(gen, 10)
	if gen < 2 {
		t.Fatalf("generation %d after the edit", gen)
	}
	got = runReportArgs(deps.env, "linux", "--blast", dir)
	if got.code != exitOK || !strings.Contains(got.stdout, "Broken (0):") ||
		!strings.HasPrefix(got.stdout, "baseline: daemon generation "+g+" (rebase), current generation "+g+"\n") {
		t.Fatalf("report --blast after rebase: %s", got)
	}

	got = runRebaseAt(deps.env, "linux", "--expect", "1", dir)
	if got.code != exitFailure || got.stdout != "" || got.stderr != "gruntled: the daemon is at generation "+g+", not 1; nothing changed\n" {
		t.Fatalf("rebase --expect 1: %s", got)
	}
	if after := runReportArgs(deps.env, "linux", "--blast", dir); !strings.HasPrefix(after.stdout, "baseline: daemon generation "+g+" (rebase)") {
		t.Fatalf("refused rebase moved the baseline: %s", after)
	}
}

func TestRebaseRefusalsCLI(t *testing.T) {
	fixture := "testdata/sarif-fixture"
	for _, c := range []struct {
		code, msg, want string
	}{
		{ipc.CodeIndexing, "x", "gruntled: the daemon is still indexing; try again\n"},
		{ipc.CodeFailed, "parse \x1b[2Jboom\r\n", "gruntled: the last reindex failed: parse [2Jboom; fix the tree and save, then rebase\n"},
		{ipc.CodeExpectMismatch, "the daemon is at generation 3, not 2", "gruntled: the daemon is at generation 3, not 2; nothing changed\n"},
		{ipc.CodeUnavailable, "x", "gruntled: the daemon could not compute the blast view; nothing changed\n"},
		{ipc.CodeInternal, "x", "gruntled: the daemon could not rebase; nothing changed\n"},
		{ipc.CodeUnsupported, "x", "gruntled: the daemon does not support rebase; restart gruntled watch to reset the baseline\n"},
	} {
		t.Run(c.code, func(t *testing.T) {
			env := envAt(shortBase(t))
			serveAt(t, env, fixture, snapHandler{rebase: func(*uint64) (ipc.RebaseResult, error) {
				return ipc.RebaseResult{}, ipc.Refuse(c.code, c.msg)
			}})
			got := runRebaseAt(env, "linux", fixture)
			if got.code != exitFailure || got.stdout != "" || got.stderr != c.want {
				t.Fatalf("%s", got)
			}
		})
	}
}

// treeState is path, mode, size, mtime and sha256 of every entry under
// root, or "" when root does not exist. Metadata comes from os.Lstat, never
// from the walk's DirEntry: on windows that is the directory listing's
// copy (FindFirstFile data), which NTFS updates lazily, so a file held
// open or a directory just written to can show a stale size or mtime.
func treeState(t *testing.T, root string) string {
	t.Helper()
	var b strings.Builder
	err := filepath.WalkDir(root, func(p string, _ fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		fi, err := os.Lstat(p)
		if err != nil {
			return err
		}
		b.WriteString(p + " " + fi.Mode().String() + " " + strconv.FormatInt(fi.Size(), 10) + " " + fi.ModTime().String())
		if fi.Mode().IsRegular() {
			data, err := os.ReadFile(p)
			switch {
			case err == nil:
				b.WriteString(fmt.Sprintf(" %x", sha256.Sum256(data)))
			case runtime.GOOS == "windows":
				// A held lock file cannot be opened on windows; its
				// mode, size and mtime are still compared.
				b.WriteString(" unreadable")
			default:
				return err
			}
		}
		b.WriteByte('\n')
		return nil
	})
	if os.IsNotExist(err) {
		return ""
	}
	mustDo(t, err)
	return b.String()
}

func TestRebaseWindowsUnsupported(t *testing.T) {
	const want = "gruntled: rebase needs the daemon socket, which this platform does not have; restart gruntled watch to reset the baseline\n"
	dir := repo(t, "vpc_idd")
	t.Run("no runtime dir", func(t *testing.T) {
		base := shortBase(t)
		got := runRebaseAt(envAt(base), "windows", dir)
		if got.code != exitFailure || got.stdout != "" || got.stderr != want {
			t.Fatalf("%s", got)
		}
		if s := treeState(t, base); strings.Count(s, "\n") != 1 {
			t.Fatalf("rebase created entries:\n%s", s)
		}
	})
	t.Run("report-file daemon", func(t *testing.T) {
		base := shortBase(t)
		env := envAt(base)
		_, broken := blastSnapshots(t)
		holdDump(t, env, dir, broken)
		before := treeState(t, base)
		got := runRebaseAt(env, "windows", dir)
		if got.code != exitFailure || got.stdout != "" || got.stderr != want {
			t.Fatalf("%s", got)
		}
		if after := treeState(t, base); after != before {
			t.Fatalf("rebase changed the runtime base:\n%s", lineDiff(before, after))
		}
	})
}

// lineDiff lists the lines only in before ("- ") and only in after
// ("+ "), so a failed no-write check says what changed.
func lineDiff(before, after string) string {
	in := func(s string) map[string]bool {
		m := map[string]bool{}
		for _, l := range strings.Split(s, "\n") {
			m[l] = true
		}
		return m
	}
	b, a := in(before), in(after)
	var out strings.Builder
	for _, l := range strings.Split(before, "\n") {
		if !a[l] {
			out.WriteString("- " + l + "\n")
		}
	}
	for _, l := range strings.Split(after, "\n") {
		if !b[l] {
			out.WriteString("+ " + l + "\n")
		}
	}
	return out.String()
}

func TestRebaseWritesNothing(t *testing.T) {
	skipNoUnixSocket(t)
	dir := repo(t, "vpc_idd")
	deps := testDeps(t)
	base := deps.env.TempDir()
	d := startWatch(t, dir, deps, "--poll")
	eventually(t, func() bool { return servedGeneration(t, deps.env, dir) > 0 }, "first report", nil)
	state := func() string {
		st, err := os.ReadFile(d.status)
		mustDo(t, err)
		return treeState(t, dir) + "--\n" + treeState(t, base) + "--\n" + string(st)
	}
	before := state()
	for _, args := range [][]string{{dir}, {"--expect", "999", dir}, {dir}} {
		runRebaseAt(deps.env, "linux", args...)
		if after := state(); after != before {
			t.Fatalf("rebase %v changed the repo, runtime base or status file:\n%s", args, lineDiff(before, after))
		}
	}
	t.Run("no daemon", func(t *testing.T) {
		empty := shortBase(t)
		got := runRebaseAt(envAt(empty), "linux", dir)
		if got.code != exitFailure || got.stdout != "" || !strings.Contains(got.stderr, "gruntled: no watch daemon is running for ") {
			t.Fatalf("%s", got)
		}
		if ents, err := os.ReadDir(empty); err != nil || len(ents) != 0 {
			t.Fatalf("rebase created %v (%v)", ents, err)
		}
	})
}
