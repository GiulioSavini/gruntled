package main

// VALID-03 and VALID-04: the falsifiable experiment on the real primary
// corpus. Both tests are env-gated (GRUNTLED_CORPUS) and skip in CI.
//
//   - TestCorpusClean runs gruntled on the unmutated corpus at the pinned
//     commit and requires zero diagnostics of any code.
//   - TestCorpusMutation applies one rename and one deletion to a faithful
//     scratch copy and requires gruntled's diagnostic set to equal, exactly,
//     the set computed by corpusOracle, a textual oracle that shares no code
//     with gruntled's parser.
//
// The corpus checkout itself is never written: every mutation happens in a
// t.TempDir copy, and corpusDigest proves the checkout is unchanged.

import (
	"bytes"
	"cmp"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
)

// corpusPinnedCommit is the primary corpus commit the experiment is pinned
// to (PREP.md §2).
const corpusPinnedCommit = "e6c55d11fd1a01e75b78d7897be36c69fa26b8cc"

// corpusRequire returns the absolute path of the pinned corpus checkout, or
// skips when GRUNTLED_CORPUS is unset.
func corpusRequire(t testing.TB) string {
	t.Helper()
	dir := os.Getenv("GRUNTLED_CORPUS")
	if dir == "" {
		t.Skip("GRUNTLED_CORPUS not set")
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		t.Fatalf("corpus path: %v", err)
	}
	abs = filepath.Clean(abs)
	head, err := os.ReadFile(filepath.Join(abs, ".git", "HEAD"))
	if err != nil {
		t.Fatalf("GRUNTLED_CORPUS must be a detached checkout of the pinned commit; cannot read .git/HEAD: %v", err)
	}
	if got := strings.TrimSpace(string(head)); got != corpusPinnedCommit {
		t.Fatalf("GRUNTLED_CORPUS must be a detached checkout of the pinned commit; got %q, want %q", got, corpusPinnedCommit)
	}
	return abs
}

// corpusDigest hashes the tree under root (excluding .git): relative path,
// type, permission bits, and the sha256 of regular files or the target of
// symlinks. Timestamps are left out.
func corpusDigest(t testing.TB, root string) string {
	t.Helper()
	var lines []string
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() && d.Name() == ".git" {
			return filepath.SkipDir
		}
		rel, err := filepath.Rel(root, p)
		if err != nil {
			return err
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		mode := info.Mode()
		var extra string
		switch {
		case mode.Type()&fs.ModeSymlink != 0:
			target, err := os.Readlink(p)
			if err != nil {
				return err
			}
			extra = "link:" + target
		case mode.IsRegular():
			b, err := os.ReadFile(p)
			if err != nil {
				return err
			}
			sum := sha256.Sum256(b)
			extra = "sha256:" + hex.EncodeToString(sum[:])
		case mode.IsDir():
			extra = "dir"
		default:
			extra = "other:" + mode.Type().String()
		}
		lines = append(lines, fmt.Sprintf("%s\t%s\t%o\t%s", filepath.ToSlash(rel), mode.Type().String(), mode.Perm(), extra))
		return nil
	})
	if err != nil {
		t.Fatalf("digest %s: %v", root, err)
	}
	slices.Sort(lines)
	sum := sha256.Sum256([]byte(strings.Join(lines, "\n")))
	return hex.EncodeToString(sum[:])
}

// corpusCopy makes a faithful copy of src (excluding .git) in a fresh
// t.TempDir: same perms, same bytes, symlinks recreated with the same
// target and never followed. It proves faithfulness with corpusDigest.
func corpusCopy(t testing.TB, src string) string {
	t.Helper()
	dst := filepath.Join(t.TempDir(), "corpus")
	err := filepath.WalkDir(src, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() && d.Name() == ".git" {
			return filepath.SkipDir
		}
		rel, err := filepath.Rel(src, p)
		if err != nil {
			return err
		}
		out := filepath.Join(dst, rel)
		info, err := d.Info()
		if err != nil {
			return err
		}
		mode := info.Mode()
		switch {
		case mode.IsDir():
			if err := os.MkdirAll(out, mode.Perm()); err != nil {
				return err
			}
			return os.Chmod(out, mode.Perm())
		case mode.Type()&fs.ModeSymlink != 0:
			target, err := os.Readlink(p)
			if err != nil {
				return err
			}
			return os.Symlink(target, out)
		case mode.IsRegular():
			in, err := os.Open(p)
			if err != nil {
				return err
			}
			defer in.Close()
			f, err := os.OpenFile(out, os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode.Perm())
			if err != nil {
				return err
			}
			if _, err := io.Copy(f, in); err != nil {
				f.Close()
				return err
			}
			if err := f.Close(); err != nil {
				return err
			}
			return os.Chmod(out, mode.Perm())
		default:
			return fmt.Errorf("unsupported file type %s at %s", mode.Type(), rel)
		}
	})
	if err != nil {
		t.Fatalf("copy %s: %v", src, err)
	}
	if a, b := corpusDigest(t, src), corpusDigest(t, dst); a != b {
		t.Fatalf("corpus copy is not faithful: digest %s != %s", b, a)
	}
	return dst
}

// corpusReport mirrors JSON schema v1 (internal/interfaces/presenter/json.go).
type corpusReport struct {
	Version     int `json:"version"`
	Diagnostics []struct {
		Code     string `json:"code"`
		Severity string `json:"severity"`
		File     string `json:"file"`
		Line     int    `json:"line"`
		Column   int    `json:"column"`
		Unit     string `json:"unit"`
		Message  string `json:"message"`
	} `json:"diagnostics"`
	UnknownUnits []struct {
		Path   string `json:"path"`
		Status string `json:"status"`
		Reason string `json:"reason"`
	} `json:"unknown_units"`
	UnknownModules []struct {
		Path   string `json:"path"`
		Reason string `json:"reason"`
	} `json:"unknown_modules"`
	Summary struct {
		Units          int `json:"units"`
		Resolved       int `json:"resolved"`
		ModuleUnknown  int `json:"module_unknown"`
		ConfigUnknown  int `json:"config_unknown"`
		UnknownModules int `json:"unknown_modules"`
		Errors         int `json:"errors"`
		Warnings       int `json:"warnings"`
	} `json:"summary"`
}

// corpusDiag is the projection of a diagnostic the experiment compares.
type corpusDiag struct {
	Code, Severity, File string
	Line, Column         int
	Unit                 string
}

func (d corpusDiag) String() string {
	return fmt.Sprintf("%s %s %s:%d:%d %s", d.Code, d.Severity, d.File, d.Line, d.Column, d.Unit)
}

func corpusSortDiags(ds []corpusDiag) {
	slices.SortFunc(ds, func(a, b corpusDiag) int {
		return cmp.Or(
			cmp.Compare(a.File, b.File),
			cmp.Compare(a.Line, b.Line),
			cmp.Compare(a.Column, b.Column),
			cmp.Compare(a.Unit, b.Unit),
			cmp.Compare(a.Code, b.Code),
		)
	})
}

// corpusProject returns the sorted projection of rep's diagnostics.
func corpusProject(rep corpusReport) []corpusDiag {
	out := make([]corpusDiag, 0, len(rep.Diagnostics))
	for _, d := range rep.Diagnostics {
		out = append(out, corpusDiag{d.Code, d.Severity, d.File, d.Line, d.Column, d.Unit})
	}
	corpusSortDiags(out)
	return out
}

// corpusRunJSON runs gruntled in process with --format json on dir.
func corpusRunJSON(t testing.TB, dir string) (rep corpusReport, raw []byte, code int) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	code = run([]string{"check", "--format", "json", dir}, &stdout, &stderr)
	if stderr.Len() > 0 {
		t.Logf("gruntled stderr: %s", stderr.String())
	}
	raw = stdout.Bytes()
	if err := json.Unmarshal(raw, &rep); err != nil {
		t.Fatalf("gruntled JSON (exit %d) does not parse: %v\n%s", code, err, raw)
	}
	return rep, raw, code
}

var (
	corpusDepBlockRe   = regexp.MustCompile(`(?ms)^dependency "([^"]+)" \{\n(.*?)^\}`)
	corpusConfigPathRe = regexp.MustCompile(`config_path\s*=\s*"([^"]+)"`)
)

// corpusOracle computes, textually and independently of gruntled's parser,
// the GRT001 set that removing output from the module in targetDir must
// produce: one entry per `dependency.L.outputs.<output>` line in a
// terragrunt.hcl whose dependency L has config_path resolving to targetDir.
// Shapes it cannot model are a t.Fatalf, never a silent skip.
func corpusOracle(t testing.TB, root, targetDir, output string) []corpusDiag {
	t.Helper()
	var out []corpusDiag
	anyRef := regexp.MustCompile(`\bdependency\.([A-Za-z0-9_-]+)\.outputs\.` + regexp.QuoteMeta(output) + `\b`)
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			switch d.Name() {
			case ".git", ".terragrunt-cache", ".terraform":
				return filepath.SkipDir
			}
			return nil
		}
		if d.Name() != "terragrunt.hcl" {
			return nil
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, p)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		fileDir := path.Dir(rel)
		src := string(b)
		lines := strings.Split(src, "\n")

		blocks := map[string]string{}
		for _, m := range corpusDepBlockRe.FindAllStringSubmatch(src, -1) {
			blocks[m[1]] = m[2]
		}
		// A reference to a label with no block in this file is a shape the
		// oracle cannot model (it would need include merging).
		for _, m := range anyRef.FindAllStringSubmatch(src, -1) {
			if _, ok := blocks[m[1]]; !ok {
				t.Fatalf("oracle cannot model a reference to dependency %q with no block in %s", m[1], rel)
			}
		}
		for label, body := range blocks {
			cfg := corpusConfigPathRe.FindStringSubmatch(body)
			if cfg == nil {
				continue
			}
			if path.Clean(path.Join(fileDir, cfg[1])) != targetDir {
				continue
			}
			refRe := regexp.MustCompile(`\bdependency\.` + regexp.QuoteMeta(label) + `\.outputs\.` + regexp.QuoteMeta(output) + `\b`)
			plainRe := regexp.MustCompile(`^\s*[A-Za-z0-9_]+\s*=\s*dependency\.` + regexp.QuoteMeta(label) + `\.outputs\.` + regexp.QuoteMeta(output) + `\s*$`)
			for i, line := range lines {
				if !refRe.MatchString(line) {
					continue
				}
				if strings.Contains(body, "skip_outputs") || strings.Contains(body, "enabled") {
					t.Fatalf("oracle cannot model DIAG-03 silent rows; choose another mutation (%s dependency %q)", rel, label)
				}
				if !plainRe.MatchString(line) {
					t.Fatalf("oracle cannot model this reference shape: %s:%d: %s", rel, i+1, line)
				}
				out = append(out, corpusDiag{
					Code:     "GRT001",
					Severity: "error",
					File:     rel,
					Line:     i + 1,
					Column:   strings.Index(line, "dependency.") + 1,
					Unit:     fileDir,
				})
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("oracle walk: %v", err)
	}
	corpusSortDiags(out)
	return out
}

// corpusCrudeCount counts "dependency.<label>.outputs.<output>" followed by
// a non-word byte, across every terragrunt.hcl under root, for every label
// in labels. It is a second, cruder cross-check of the oracle.
func corpusCrudeCount(t testing.TB, root string, labels []string, output string) int {
	t.Helper()
	n := 0
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			switch d.Name() {
			case ".git", ".terragrunt-cache", ".terraform":
				return filepath.SkipDir
			}
			return nil
		}
		if d.Name() != "terragrunt.hcl" {
			return nil
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		for _, l := range labels {
			needle := []byte("dependency." + l + ".outputs." + output)
			rest := b
			for {
				i := bytes.Index(rest, needle)
				if i < 0 {
					break
				}
				after := rest[i+len(needle):]
				if len(after) == 0 || !corpusIsWordByte(after[0]) {
					n++
				}
				rest = after
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("crude count walk: %v", err)
	}
	return n
}

func corpusIsWordByte(c byte) bool {
	return c == '_' || c >= '0' && c <= '9' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z'
}

// corpusMutation is one breaking change applied to a scratch copy.
type corpusMutation struct {
	Name      string
	File      string // slash path relative to the corpus root
	Old, New  string
	TargetDir string // module dir whose output breaks
	Output    string
	Label     string // dependency label the references use (crude count only)
	WantRefs  int    // PREP.md §5 cross-check; the oracle is the authority
}

var corpusMutations = []corpusMutation{
	{
		Name:      "rename_role_name",
		File:      "iac.src/s3_runtime/state.tf",
		Old:       `output "role_name" {`,
		New:       `output "role_name_renamed" {`,
		TargetDir: "iac.src/s3_runtime",
		Output:    "role_name",
		Label:     "s3",
		WantRefs:  8,
	},
	{
		Name:      "delete_mq_region",
		File:      "iac.mq/mq_broker/state.tf",
		Old:       "output \"region\" {\n  value = data.aws_region.this.name\n}\n\n",
		New:       "",
		TargetDir: "iac.mq/mq_broker",
		Output:    "region",
		Label:     "mq",
		WantRefs:  3,
	},
}

// corpusApply applies m to the copy at dir and returns the original bytes
// for the revert.
func corpusApply(t testing.TB, dir string, m corpusMutation) []byte {
	t.Helper()
	p := filepath.Join(dir, filepath.FromSlash(m.File))
	info, err := os.Stat(p)
	if err != nil {
		t.Fatalf("mutation %s: %v", m.Name, err)
	}
	orig, err := os.ReadFile(p)
	if err != nil {
		t.Fatalf("mutation %s: %v", m.Name, err)
	}
	if n := strings.Count(string(orig), m.Old); n != 1 {
		t.Fatalf("mutation %s: the pinned text changed: %q occurs %d times in %s, want 1", m.Name, m.Old, n, m.File)
	}
	mutated := strings.Replace(string(orig), m.Old, m.New, 1)
	if err := os.WriteFile(p, []byte(mutated), info.Mode().Perm()); err != nil {
		t.Fatalf("mutation %s: %v", m.Name, err)
	}
	return orig
}

// corpusModuleText concatenates every .tf file in the module dir.
func corpusModuleText(t testing.TB, dir string) string {
	t.Helper()
	matches, err := filepath.Glob(filepath.Join(dir, "*.tf"))
	if err != nil {
		t.Fatal(err)
	}
	var sb strings.Builder
	for _, m := range matches {
		b, err := os.ReadFile(m)
		if err != nil {
			t.Fatal(err)
		}
		sb.Write(b)
		sb.WriteByte('\n')
	}
	return sb.String()
}

// TestCorpusClean is VALID-03: zero diagnostics on the unmutated corpus.
func TestCorpusClean(t *testing.T) {
	corpus := corpusRequire(t)
	d0 := corpusDigest(t, corpus)

	rep, raw, code := corpusRunJSON(t, corpus)
	if code != 0 || len(rep.Diagnostics) != 0 || rep.Summary.Errors != 0 {
		t.Errorf("VALID-03: unmutated corpus: exit %d, %d diagnostics, %d errors; want 0, 0, 0\n%s",
			code, len(rep.Diagnostics), rep.Summary.Errors, raw)
	}

	var stdout, stderr bytes.Buffer
	if tc := run([]string{"check", corpus}, &stdout, &stderr); tc != 0 || stdout.Len() != 0 {
		t.Errorf("VALID-03: text run: exit %d, stdout %q; want 0 and empty", tc, stdout.String())
	}

	s := rep.Summary
	t.Logf("summary: units=%d resolved=%d module_unknown=%d config_unknown=%d unknown_modules=%d errors=%d warnings=%d",
		s.Units, s.Resolved, s.ModuleUnknown, s.ConfigUnknown, s.UnknownModules, s.Errors, s.Warnings)
	for _, u := range rep.UnknownUnits {
		t.Logf("unknown unit: %s %s: %s", u.Path, u.Status, u.Reason)
	}
	for _, m := range rep.UnknownModules {
		t.Logf("unknown module: %s: %s", m.Path, m.Reason)
	}

	cp := corpusCopy(t, corpus)
	_, rawCp, _ := corpusRunJSON(t, cp)
	if !bytes.Equal(raw, rawCp) {
		t.Errorf("copy fidelity: JSON on the copy differs from the corpus\ncorpus:\n%s\ncopy:\n%s", raw, rawCp)
	}

	if d := corpusDigest(t, corpus); d != d0 {
		t.Errorf("corpus checkout changed: digest %s -> %s", d0, d)
	}
}

// TestCorpusMutation is VALID-04: after each mutation, gruntled's
// diagnostic set equals the oracle's exactly, and the revert is clean.
func TestCorpusMutation(t *testing.T) {
	corpus := corpusRequire(t)
	d0 := corpusDigest(t, corpus)

	for _, m := range corpusMutations {
		t.Run(m.Name, func(t *testing.T) {
			cp := corpusCopy(t, corpus)

			pre, preRaw, code := corpusRunJSON(t, cp)
			if code != 0 || len(pre.Diagnostics) != 0 {
				t.Fatalf("clean baseline on the copy: exit %d, %d diagnostics; want 0, 0\n%s", code, len(pre.Diagnostics), preRaw)
			}

			want := corpusOracle(t, cp, m.TargetDir, m.Output)
			if len(want) != m.WantRefs {
				t.Fatalf("oracle found %d references, PREP.md §5 and 04-CONTEXT measured %d: %v", len(want), m.WantRefs, want)
			}
			if crude := corpusCrudeCount(t, cp, []string{m.Label}, m.Output); crude != len(want) {
				t.Fatalf("crude byte count %d disagrees with oracle %d", crude, len(want))
			}
			for _, w := range want {
				t.Logf("oracle: %s", w)
			}

			orig := corpusApply(t, cp, m)
			modDir := filepath.Join(cp, filepath.FromSlash(m.TargetDir))
			if strings.Contains(corpusModuleText(t, modDir), `output "`+m.Output+`" {`) {
				t.Fatalf("mutation did not remove output %q from %s", m.Output, m.TargetDir)
			}

			rep, raw, code := corpusRunJSON(t, cp)
			if code != 1 {
				t.Errorf("VALID-04: mutated exit %d, want 1", code)
			}
			for _, d := range rep.Diagnostics {
				if d.Code != "GRT001" || d.Severity != "error" {
					t.Errorf("VALID-04: unexpected diagnostic %s %s %s:%d:%d", d.Code, d.Severity, d.File, d.Line, d.Column)
				}
				t.Logf("gruntled: %s:%d:%d: %s %s", d.File, d.Line, d.Column, d.Code, d.Message)
			}
			got := corpusProject(rep)
			if !slices.Equal(got, want) {
				var missing, extra []corpusDiag
				for _, w := range want {
					if !slices.Contains(got, w) {
						missing = append(missing, w)
					}
				}
				for _, g := range got {
					if !slices.Contains(want, g) {
						extra = append(extra, g)
					}
				}
				t.Errorf("VALID-04: diagnostic set != oracle\nwant %v\ngot  %v\nmissing %v\nextra   %v\n%s", want, got, missing, extra, raw)
			}

			p := filepath.Join(cp, filepath.FromSlash(m.File))
			info, err := os.Stat(p)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(p, orig, info.Mode().Perm()); err != nil {
				t.Fatal(err)
			}
			post, postRaw, code := corpusRunJSON(t, cp)
			if code != 0 || len(post.Diagnostics) != 0 || !bytes.Equal(postRaw, preRaw) {
				t.Errorf("revert: exit %d, %d diagnostics, JSON identical to pre-state=%v; want 0, 0, true\n%s",
					code, len(post.Diagnostics), bytes.Equal(postRaw, preRaw), postRaw)
			}
		})
	}

	if d := corpusDigest(t, corpus); d != d0 {
		t.Errorf("corpus checkout changed: digest %s -> %s", d0, d)
	}
}
