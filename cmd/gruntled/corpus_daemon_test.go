//go:build linux || darwin

package main

// REL-03 through the daemon (17-04): a sample of the corpus mutations runs
// through a real built-binary `gruntled watch` on a working copy of each
// corpus. After each edit, report --blast (text and JSON) must equal
// `blast --base <pristine copy> <working copy>` after the exact-label
// normaliser, with the same exit code; after the revert the view is empty
// again. One rebase cycle per corpus. Daemon cleanup follows compat-v1.sh
// (16 sec F8): pid recorded, 20 s client timeouts, TERM then KILL, and a
// final check that no process mentions the temp dirs.

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

// corpusDaemon is one built-binary watch daemon with a private runtime dir.
type corpusDaemon struct {
	t    *testing.T
	bin  string
	env  []string
	cmd  *exec.Cmd
	errb *syncBuf
	done bool
}

func startCorpusDaemon(t *testing.T, bin, dir string) *corpusDaemon {
	t.Helper()
	base := shortBase(t)
	rt := filepath.Join(base, "rt")
	mustDo(t, os.Mkdir(rt, 0o700))
	d := &corpusDaemon{t: t, bin: bin, env: append(os.Environ(), "XDG_RUNTIME_DIR="+rt, "TMPDIR="+rt), errb: &syncBuf{}}
	d.cmd = exec.Command(bin, "watch", "--poll", "--poll-interval", "200ms", "--status-file", filepath.Join(base, "st", "status"), dir)
	d.cmd.Env = d.env
	d.cmd.Stderr = d.errb
	mustDo(t, d.cmd.Start())
	t.Cleanup(d.stop)
	return d
}

// stop sends SIGTERM, waits up to 5 s, then SIGKILL.
func (d *corpusDaemon) stop() {
	if d.done {
		return
	}
	d.done = true
	_ = d.cmd.Process.Signal(syscall.SIGTERM)
	exited := make(chan struct{})
	go func() { _ = d.cmd.Wait(); close(exited) }()
	select {
	case <-exited:
	case <-time.After(5 * time.Second):
		_ = d.cmd.Process.Kill()
		<-exited
	}
}

// client runs the built binary with a 20 s timeout.
func (d *corpusDaemon) client(args ...string) cliResult {
	d.t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, d.bin, args...)
	cmd.Env = d.env
	var o, e bytes.Buffer
	cmd.Stdout, cmd.Stderr = &o, &e
	err := cmd.Run()
	code := 0
	var ee *exec.ExitError
	switch {
	case errors.As(err, &ee):
		code = ee.ExitCode()
	case err != nil:
		d.t.Fatalf("%v: %v", args, err)
	}
	if ctx.Err() != nil {
		d.t.Fatalf("%v: client timed out after 20 s; daemon stderr:\n%s", args, d.errb.String())
	}
	return cliResult{stdout: o.String(), stderr: e.String(), code: code}
}

func (d *corpusDaemon) report(args ...string) cliResult {
	return d.client(append([]string{"report"}, args...)...)
}

// waitFinal waits until the daemon serves a generation after `after`, its
// plain report equals check on cur (it indexed the final tree, never a
// half-written file), and its normalised blast view equals blast --base
// pristine cur in text.
func (d *corpusDaemon) waitFinal(pristine, cur string, after uint64, what string) uint64 {
	d.t.Helper()
	deadline := time.Now().Add(3 * time.Minute)
	var last string
	for {
		text := d.report("--blast", cur)
		lb, ok := servedBaseline(text.stdout)
		if ok && lb.CurrentGeneration > after {
			plain, want := d.report("--format", "text", cur), runCheckAt("text", cur)
			if plain.stdout == want.stdout && plain.code == want.code {
				out, _, _ := runCLI(d.t, "blast", "--base", pristine, cur)
				g, err1 := normaliseDaemonBlast(text.stdout, lb)
				b, err2 := normaliseBlastBase(out)
				if err1 == nil && err2 == nil && g == b {
					return lb.CurrentGeneration
				}
				last = fmt.Sprintf("view differs from blast --base: %v %v %s", err1, err2, firstDiff(g, b))
			} else {
				last = "plain report differs from check"
			}
		} else {
			last = "generation not past " + strconv.FormatUint(after, 10) + ": " + text.String()
		}
		if time.Now().After(deadline) {
			d.t.Fatalf("%s: timed out: %s; daemon stderr:\n%s", what, last, d.errb.String())
		}
		time.Sleep(300 * time.Millisecond)
	}
}

// assertNoProcessMentions fails if any process command line names one of
// the dirs (a leaked daemon).
func assertNoProcessMentions(t *testing.T, dirs ...string) {
	t.Helper()
	ents, err := os.ReadDir("/proc")
	if err != nil {
		cmd := exec.Command("pgrep", "-fl", dirs[0])
		if out, err := cmd.Output(); err == nil && len(bytes.TrimSpace(out)) > 0 {
			t.Errorf("leaked process: %s", out)
		}
		return
	}
	for _, e := range ents {
		if _, err := strconv.Atoi(e.Name()); err != nil {
			continue
		}
		b, err := os.ReadFile(filepath.Join("/proc", e.Name(), "cmdline"))
		if err != nil {
			continue
		}
		for _, dir := range dirs {
			if bytes.Contains(b, []byte(dir)) {
				t.Errorf("leaked process %s: %q", e.Name(), bytes.ReplaceAll(b, []byte{0}, []byte{' '}))
			}
		}
	}
}

// TestCorpusDaemonSample runs, per corpus, the first eligible site of each
// class M1-M7, R8 and (iso20022) M9 through a real daemon, then one rebase
// cycle.
func TestCorpusDaemonSample(t *testing.T) {
	var bin string
	for _, repo := range graphRepos {
		t.Run(repo.Name, func(t *testing.T) {
			root := repo.Require(t)
			before := corpusDigest(t, root)
			if bin == "" {
				bin = benchBuildGruntled(t)
			}
			model := buildCorpusModel(t, root)
			ex := corpusExclusions[repo.Name]
			included, _, _ := model.universe(ex)
			pristine, cur := repo.Copy(t, root), repo.Copy(t, root)
			h := newHarness(t, model, repo.Copy(t, root), cur)
			d := startCorpusDaemon(t, bin, cur)
			gen := d.waitFinal(pristine, cur, 0, "first index")

			// Red row: the normaliser rejects a label whose generation is off
			// by one (a recorded mutant of its input).
			text := d.report("--blast", cur)
			lb, _ := servedBaseline(text.stdout)
			lb.Generation++
			if _, err := normaliseDaemonBlast(text.stdout, lb); err == nil {
				t.Fatalf("normaliser accepted an off-by-one baseline generation:\n%s", text.stdout)
			}

			step := func(name string, apply func() func()) {
				t.Helper()
				restore := apply()
				gen = d.waitFinal(pristine, cur, gen, name)
				assertBlastEquivalent(t, d.report, pristine, cur)
				restore()
				gen = d.waitFinal(pristine, cur, gen, name+" reverted")
				empty := d.report("--blast", cur)
				if _, rest, _ := strings.Cut(empty.stdout, "\n"); rest != "Broken (0):\nImpacted (0):\n" || empty.code != 0 {
					t.Fatalf("%s reverted: view not empty: %s", name, empty)
				}
				t.Logf("CORPUS-DAEMON %s %s ok (generation %d)", repo.Name, name, gen)
			}
			var m1 *mutation
			for _, class := range []string{"M1", "M2", "M3", "M4", "M5", "M6", "M7", "M9"} {
				set := enumerate(model, ex, included, class)
				if len(set.Muts) == 0 {
					t.Logf("CORPUS-DAEMON %s %s no eligible site", repo.Name, class)
					continue
				}
				m := set.Muts[0]
				if class == "M1" {
					m1 = &m
				}
				step(class+" "+m.String(), func() func() {
					_, es, err := h.plan(m)
					mustDo(t, err)
					// M6 runs one-sided here: the daemon's baseline is the
					// pristine tree, so only cur' (type = number) is applied
					// and the expected view is blast --base pristine cur.
					return applyEdits(t, cur, es)
				})
			}
			step("R8", func() func() {
				var rs []func()
				r, _ := applyFileTransform(t, cur, included, r7Deprecated)
				rs = append(rs, r)
				r, _ = r6Move(t, cur, included)
				rs = append(rs, r)
				for _, f := range []fileTransform{r3Reorder, r2Comments, r1Whitespace} {
					r, _ = applyFileTransform(t, cur, included, f)
					rs = append(rs, r)
				}
				return func() {
					for i := len(rs) - 1; i >= 0; i-- {
						rs[i]()
					}
				}
			})

			// One rebase cycle: mutate (M1), accept it, then revert.
			if m1 == nil {
				t.Fatalf("no M1 site for the rebase cycle")
			}
			_, es, err := h.plan(*m1)
			mustDo(t, err)
			restore := applyEdits(t, cur, es)
			gen = d.waitFinal(pristine, cur, gen, "rebase cycle M1")
			want := expectBlast(model, *m1)
			nErr := 0
			for _, b := range want.Doc.Broken {
				nErr += len(b.Findings)
			}
			got := d.client("rebase", "--expect", strconv.FormatUint(gen, 10), cur)
			wantOut := fmt.Sprintf("gruntled: baseline moved to generation %d; %d error findings in Broken accepted\n", gen, nErr)
			if got.code != 0 || got.stdout != wantOut || got.stderr != "" {
				t.Fatalf("rebase: %s\nwant stdout %q", got, wantOut)
			}
			after := d.report("--blast", cur)
			if _, rest, _ := strings.Cut(after.stdout, "\n"); rest != "Broken (0):\nImpacted (0):\n" || after.code != 0 {
				t.Fatalf("report --blast after rebase: %s", after)
			}
			restore()
			deadline := time.Now().Add(3 * time.Minute)
			for {
				js := d.report("--blast", "--format", "json", cur)
				var doc struct {
					Broken  []any `json:"broken"`
					Changes []struct {
						Module       string   `json:"module"`
						AddedOutputs []string `json:"added_outputs"`
					} `json:"changes"`
					Daemon struct {
						Source            string `json:"source"`
						CurrentGeneration uint64 `json:"current_generation"`
					} `json:"daemon_baseline"`
				}
				if json.Unmarshal([]byte(js.stdout), &doc) == nil && doc.Daemon.CurrentGeneration > gen &&
					len(doc.Changes) == 1 && doc.Changes[0].Module == m1.Module && slices.Equal(doc.Changes[0].AddedOutputs, []string{m1.Name}) {
					text := d.report("--blast", cur)
					if js.code != 0 || len(doc.Broken) != 0 || doc.Daemon.Source != sourceRebase ||
						!strings.Contains(text.stdout, "+output "+m1.Name) || !strings.Contains(text.stdout, "Broken (0):") {
						t.Fatalf("revert after rebase: %s\n%s", js, text)
					}
					t.Logf("CORPUS-DAEMON %s rebase cycle ok (%d errors accepted; revert shows +output %s)", repo.Name, nErr, m1.Name)
					break
				}
				if time.Now().After(deadline) {
					t.Fatalf("revert after rebase never showed +output %s: %s", m1.Name, js)
				}
				time.Sleep(300 * time.Millisecond)
			}

			d.stop()
			assertNoProcessMentions(t, cur, pristine)
			if after := corpusDigest(t, root); after != before {
				t.Errorf("checkout changed")
			}
		})
	}
}
