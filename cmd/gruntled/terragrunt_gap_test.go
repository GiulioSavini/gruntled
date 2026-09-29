package main

// VALID-05: plain `terragrunt hcl validate`, hash-pinned v1.1.6, does not
// report the mutations gruntled catches. Env-gated on GRUNTLED_CORPUS and
// GRUNTLED_TERRAGRUNT_BIN, so CI skips it. os/exec is used only here, in a
// _test.go file; the gruntled binary itself links no os/exec.

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"
)

const (
	tgPinnedVersion = "v1.1.6"
	tgPinnedSHA256  = "d75a80bb264758ba00dabcb17f4b507fcdab4ca90d9e41f96750df036bf69b04"
)

// tgVerifyPinned returns the terragrunt binary from GRUNTLED_TERRAGRUNT_BIN
// after checking its SHA256 and version, or skips when the variable is
// unset. It logs the measured values, not the constants.
func tgVerifyPinned(t testing.TB) string {
	t.Helper()
	bin := os.Getenv("GRUNTLED_TERRAGRUNT_BIN")
	if bin == "" {
		t.Skip("GRUNTLED_TERRAGRUNT_BIN not set")
	}
	resolved, err := filepath.EvalSymlinks(bin)
	if err != nil {
		t.Fatalf("terragrunt binary: %v", err)
	}
	b, err := os.ReadFile(resolved)
	if err != nil {
		t.Fatalf("terragrunt binary: %v", err)
	}
	sum := sha256.Sum256(b)
	got := hex.EncodeToString(sum[:])
	if got != tgPinnedSHA256 {
		t.Fatalf("terragrunt binary %s: sha256 %s, want pinned %s", resolved, got, tgPinnedSHA256)
	}
	cmd := exec.Command(bin, "--version")
	cmd.Env = tgEnv(t, bin)
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("terragrunt --version: %v", err)
	}
	version := strings.TrimSpace(string(out))
	if !strings.Contains(version, "terragrunt version "+tgPinnedVersion) {
		t.Fatalf("terragrunt --version = %q, want terragrunt version %s", version, tgPinnedVersion)
	}
	t.Logf("VALIDATION-TG sha256=%s path=%s", got, resolved)
	t.Logf("VALIDATION-TG version=%s", version)
	return bin
}

// tgEnv is an explicit minimal environment for terragrunt, never
// os.Environ(), so inherited TG_* variables (TG_INPUTS, TG_STRICT, ...)
// cannot silently change the comparator. PATH holds only the pinned bin
// dir (pinned terragrunt and tofu).
func tgEnv(t testing.TB, bin string) []string {
	t.Helper()
	return []string{
		"PATH=" + filepath.Dir(bin),
		"HOME=" + t.TempDir(),
		"TMPDIR=" + t.TempDir(),
		"TG_NON_INTERACTIVE=true",
	}
}

// tgRun runs plain `terragrunt hcl validate` on dir.
//
// Deliberately excluded flags: --inputs and --strict are forbidden
// (PREP.md §4.1-4.2: they already fail on the clean corpus and --inputs
// writes .terragrunt-cache into the repo). --tf-path is not needed for
// plain validate (PREP.md §1).
func tgRun(t testing.TB, bin, dir string) (code int, out string, dur time.Duration) {
	t.Helper()
	cmd := exec.Command(bin, "hcl", "validate", "--working-dir", dir, "--no-color")
	cmd.Env = tgEnv(t, bin)
	cmd.Dir = dir
	start := time.Now()
	b, err := cmd.CombinedOutput()
	dur = time.Since(start)
	if err != nil {
		var ee *exec.ExitError
		if !errors.As(err, &ee) {
			t.Fatalf("terragrunt hcl validate: %v", err)
		}
		return ee.ExitCode(), string(b), dur
	}
	return 0, string(b), dur
}

var (
	tgANSIRe      = regexp.MustCompile(`\x1b\[[0-9;]*[A-Za-z]`)
	tgTimestampRe = regexp.MustCompile(`^\S*\d{2}:\d{2}:\d{2}(\.\d+)?\S*\s+`)
)

// tgNormalize makes terragrunt output comparable across two copies: the
// root dir becomes <ROOT>, ANSI escapes and a leading timestamp field are
// stripped, and empty lines are dropped.
func tgNormalize(out, dir string) []string {
	if real, err := filepath.EvalSymlinks(dir); err == nil && real != dir {
		out = strings.ReplaceAll(out, real, "<ROOT>")
	}
	out = strings.ReplaceAll(out, dir, "<ROOT>")
	out = tgANSIRe.ReplaceAllString(out, "")
	var lines []string
	for _, l := range strings.Split(out, "\n") {
		l = tgTimestampRe.ReplaceAllString(l, "")
		l = strings.TrimSpace(l)
		if l != "" {
			lines = append(lines, l)
		}
	}
	return lines
}

// tgAdded is the multiset difference b - a.
func tgAdded(a, b []string) []string {
	count := map[string]int{}
	for _, l := range a {
		count[l]++
	}
	var added []string
	for _, l := range b {
		if count[l] > 0 {
			count[l]--
			continue
		}
		added = append(added, l)
	}
	return added
}

// TestTerragruntGap is VALID-05.
func TestTerragruntGap(t *testing.T) {
	corpus := corpusRequire(t)
	bin := tgVerifyPinned(t)

	for _, m := range corpusMutations {
		t.Run(m.Name, func(t *testing.T) {
			a := corpusCopy(t, corpus)
			b := corpusCopy(t, corpus)
			corpusApply(t, b, m)
			want := corpusOracle(t, a, m.TargetDir, m.Output)

			aBefore, bBefore := corpusDigest(t, a), corpusDigest(t, b)

			codeA, outA, durA := tgRun(t, bin, a)
			if codeA != 0 {
				t.Fatalf("comparator baseline not clean on the unmutated corpus: exit %d\n%s", codeA, strings.Join(tgNormalize(outA, a), "\n"))
			}
			codeB, outB, durB := tgRun(t, bin, b)
			normA, normB := tgNormalize(outA, a), tgNormalize(outB, b)
			added := tgAdded(normA, normB)

			t.Logf("terragrunt unmutated: exit %d in %s, %d output lines", codeA, durA, len(normA))
			t.Logf("terragrunt mutated:   exit %d in %s, %d output lines", codeB, durB, len(normB))
			t.Logf("terragrunt lines added by the mutation: %d", len(added))
			for _, l := range added {
				t.Logf("  + %s", l)
			}

			wordRe := regexp.MustCompile(`\b` + regexp.QuoteMeta(m.Output) + `\b`)
			var naming []string
			for _, l := range added {
				hit := wordRe.MatchString(l) || strings.Contains(l, m.TargetDir)
				for _, w := range want {
					if strings.Contains(l, w.File) {
						hit = true
					}
				}
				if hit {
					naming = append(naming, l)
				}
			}
			if codeB != 0 || len(naming) > 0 {
				t.Errorf("VALID-05: terragrunt reports the mutation (exit %d, %d added lines naming it: %q)\n%s",
					codeB, len(naming), naming, strings.Join(normB, "\n"))
			}

			start := time.Now()
			rep, _, gcode := corpusRunJSON(t, b)
			gdur := time.Since(start)
			got := corpusProject(rep)
			t.Logf("gruntled mutated:     exit %d in %s, %d GRT001", gcode, gdur, len(got))
			for _, g := range got {
				t.Logf("  gruntled %s", g)
			}
			if !slices.Equal(got, want) {
				t.Errorf("VALID-04 recheck on this copy: gruntled %v, oracle %v", got, want)
			}

			aAfter, bAfter := corpusDigest(t, a), corpusDigest(t, b)
			t.Logf("terragrunt side effects: unmutated copy changed=%v, mutated copy changed=%v", aAfter != aBefore, bAfter != bBefore)
		})
	}
}
