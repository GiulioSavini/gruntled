package main

// TestChangelog pins the shape of CHANGELOG.md (Keep a Changelog 1.1.0) and
// ties its newest section to VERSION; TestReleaseNotesScript proves
// scripts/release-notes.sh extracts exactly that section, fail-closed, without
// ever interpreting the tag. checkChangelog is a pure checker so the negatives
// can show each rule fires on a synthetic drift (sec #488).

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"
)

var (
	changelogHeadingRe = regexp.MustCompile(`^## \[(\d+\.\d+\.\d+(?:-[0-9A-Za-z.-]+)?)\] - (\d{4}-\d{2}-\d{2})$`)
	changelogRefRe     = regexp.MustCompile(`^\[([^\]]+)\]: https://github\.com/GiulioSavini/gruntled/(compare/[^ ]+|releases/tag/v[^ ]+)$`)
)

// changelogDates are the shipped dates from .planning/MILESTONES.md.
var changelogDates = map[string]string{
	"0.4.0": "2026-10-13",
	"0.3.0": "2026-10-10",
	"0.2.0": "2026-10-08",
	"0.1.0": "2026-09-29",
}

var changelogGroups = map[string]bool{
	"Added": true, "Changed": true, "Deprecated": true, "Removed": true, "Fixed": true, "Security": true,
}

// semverLess reports a < b for X.Y.Z[-pre]; a prerelease sorts below its release.
func semverLess(a, b string) bool {
	core := func(v string) ([3]int, string) {
		base, pre, _ := strings.Cut(v, "-")
		var n [3]int
		for i, p := range strings.SplitN(base, ".", 3) {
			n[i], _ = strconv.Atoi(p)
		}
		return n, pre
	}
	na, pa := core(a)
	nb, pb := core(b)
	for i := range na {
		if na[i] != nb[i] {
			return na[i] < nb[i]
		}
	}
	if (pa == "") != (pb == "") {
		return pa != "" // the prerelease is lower
	}
	return pa < pb
}

// checkChangelog returns every shape failure of a CHANGELOG text against the
// VERSION content (with or without its v). An empty result means the file is
// well-formed.
func checkChangelog(text, version string) []string {
	var fails []string
	failf := func(format string, a ...any) { fails = append(fails, fmt.Sprintf(format, a...)) }
	want := strings.TrimPrefix(strings.TrimSpace(version), "v")

	if strings.Contains(text, "\r") {
		failf("file contains a CR")
	}
	if !strings.HasSuffix(text, "\n") || strings.HasSuffix(text, "\n\n") {
		failf("file must end with exactly one newline")
	}
	// Parse without CRs so a CRLF file reports the CR once, not on every rule.
	lines := strings.Split(strings.ReplaceAll(text, "\r", ""), "\n")
	first := ""
	for _, l := range lines {
		if strings.TrimSpace(l) != "" {
			first = l
			break
		}
	}
	if first != "# Changelog" {
		failf("first non-empty line is %q, want \"# Changelog\"", first)
	}
	for _, u := range []string{"https://keepachangelog.com/en/1.1.0/", "https://semver.org/spec/v2.0.0.html"} {
		if !strings.Contains(text, u) {
			failf("missing %s", u)
		}
	}

	type section struct {
		name, date string
		body       []string
	}
	var sections []*section
	refs := map[string]int{}
	for i, l := range lines {
		switch {
		case strings.HasPrefix(l, "## ["):
			s := &section{}
			if l == "## [Unreleased]" {
				s.name = "Unreleased"
			} else if m := changelogHeadingRe.FindStringSubmatch(l); m != nil {
				s.name, s.date = m[1], m[2]
				if _, err := time.Parse("2006-01-02", m[2]); err != nil {
					failf("line %d: date does not parse: %q", i+1, l)
					s.date = "" // reported once; the MILESTONES rule skips it
				}
			} else {
				failf("line %d: malformed version heading %q (want \"## [X.Y.Z] - YYYY-MM-DD\")", i+1, l)
				// Best-effort name so the other rules do not cascade.
				name, _, _ := strings.Cut(strings.TrimPrefix(l, "## ["), "]")
				s.name = strings.TrimPrefix(name, "v")
			}
			sections = append(sections, s)
		case strings.HasPrefix(l, "### "):
			if !changelogGroups[strings.TrimPrefix(l, "### ")] {
				failf("line %d: unknown group %q", i+1, l)
			}
			if len(sections) > 0 {
				sections[len(sections)-1].body = append(sections[len(sections)-1].body, l)
			}
		case strings.HasPrefix(l, "[") && strings.Contains(l, "]: "):
			m := changelogRefRe.FindStringSubmatch(l)
			if m == nil {
				failf("line %d: malformed reference %q", i+1, l)
				continue
			}
			refs[m[1]]++
			if m[1] == "Unreleased" && !strings.HasSuffix(l, "...HEAD") {
				failf("line %d: [Unreleased] reference must end in ...HEAD", i+1)
			}
		default:
			if len(sections) > 0 {
				sections[len(sections)-1].body = append(sections[len(sections)-1].body, l)
			}
		}
	}

	if len(sections) == 0 || sections[0].name != "Unreleased" {
		failf("the first \"## [\" heading must be [Unreleased]")
	}
	unreleased := 0
	var versions []*section
	for _, s := range sections {
		if s.name == "Unreleased" {
			unreleased++
			bullets := 0
			for _, l := range s.body {
				if strings.HasPrefix(l, "- ") {
					bullets++
				}
			}
			if bullets == 0 {
				failf("[Unreleased] has no \"- \" bullet")
			}
			continue
		}
		versions = append(versions, s)
	}
	if unreleased != 1 {
		failf("[Unreleased] appears %d times, want 1", unreleased)
	}
	for i := 1; i < len(versions); i++ {
		a, b := versions[i-1], versions[i]
		if !semverLess(b.name, a.name) {
			failf("versions not strictly descending: [%s] is listed above [%s]", a.name, b.name)
			continue // the date rule would only repeat it
		}
		if a.date != "" && b.date != "" && b.date > a.date {
			failf("dates not non-increasing: [%s] %s above [%s] %s", a.name, a.date, b.name, b.date)
		}
	}
	// The newest release is the semver maximum (the order rule above reports
	// a misplaced section on its own); it must be the VERSION file.
	newest := ""
	for _, s := range versions {
		if newest == "" || semverLess(newest, s.name) {
			newest = s.name
		}
	}
	if newest != "" && newest != want {
		failf("newest section is [%s], VERSION is %s", newest, want)
	}
	if len(versions) == 0 {
		failf("no released section")
	}
	seen := map[string]string{}
	for _, s := range versions {
		seen[s.name] = s.date
	}
	for v, d := range changelogDates {
		switch got, ok := seen[v]; {
		case !ok:
			failf("missing section [%s] (backfill)", v)
		case got == "":
			// malformed heading, already reported
		case got != d:
			failf("[%s] dated %s, MILESTONES.md says %s", v, got, d)
		}
	}
	for _, name := range append([]string{"Unreleased"}, func() []string {
		var vs []string
		for _, s := range versions {
			vs = append(vs, s.name)
		}
		return vs
	}()...) {
		if n := refs[name]; n != 1 {
			failf("reference line [%s]: appears %d times, want 1", name, n)
		}
	}
	sort.Strings(fails)
	return fails
}

func TestChangelog(t *testing.T) {
	text := readRepoFile(t, "CHANGELOG.md")
	version := readRepoFile(t, "VERSION")
	for _, f := range checkChangelog(text, version) {
		t.Error(f)
	}
	if strings.Count(text, "\n## [") < 5 {
		t.Errorf("want Unreleased plus at least four released sections")
	}
}

// TestChangelogNegative: each synthetic drift of the real file yields exactly
// the one named failure, so the checker reads the text and not its argument
// (sec #488).
func TestChangelogNegative(t *testing.T) {
	text := readRepoFile(t, "CHANGELOG.md")
	version := readRepoFile(t, "VERSION")
	if fails := checkChangelog(text, version); len(fails) != 0 {
		t.Fatalf("real CHANGELOG must be clean first: %q", fails)
	}
	mustReplace := func(old, new string) string {
		t.Helper()
		if !strings.Contains(text, old) {
			t.Fatalf("fixture text %q not in CHANGELOG.md", old)
		}
		return strings.Replace(text, old, new, 1)
	}
	swap := func() string {
		// Move the whole [0.3.0] section above [0.4.0].
		i4 := strings.Index(text, "\n## [0.4.0]")
		i3 := strings.Index(text, "\n## [0.3.0]")
		i2 := strings.Index(text, "\n## [0.2.0]")
		if !(0 < i4 && i4 < i3 && i3 < i2) {
			t.Fatal("sections 0.4.0, 0.3.0, 0.2.0 not in the expected order")
		}
		return text[:i4] + text[i3:i2] + text[i4:i3] + text[i2:]
	}
	for _, c := range []struct {
		name, want string
		text       string
		version    string
	}{
		{"v prefix in heading", "malformed version heading", mustReplace("## [0.4.0] - 2026-10-13", "## [v0.4.0] - 2026-10-13"), version},
		{"heading without date", "malformed version heading", mustReplace("## [0.4.0] - 2026-10-13", "## [0.4.0]"), version},
		{"0.3.0 above 0.4.0", "not strictly descending", swap(), version},
		{"missing reference", "reference line [0.2.0]: appears 0 times", mustReplace("[0.2.0]: https://github.com/GiulioSavini/gruntled/compare/v0.1.0...v0.2.0\n", ""), version},
		{"newest section != VERSION", "newest section is [0.4.0], VERSION is 0.5.0", text, "v0.5.0\n"},
		{"CR in the file", "contains a CR", mustReplace("# Changelog\n", "# Changelog\r\n"), version},
		{"unknown group", "unknown group", mustReplace("### Added", "### New"), version},
		{"bad date", "date does not parse", mustReplace("## [0.4.0] - 2026-10-13", "## [0.4.0] - 2026-13-13"), version},
	} {
		t.Run(c.name, func(t *testing.T) {
			fails := checkChangelog(c.text, c.version)
			if len(fails) != 1 || !strings.Contains(fails[0], c.want) {
				t.Errorf("want exactly one failure containing %q, got %q", c.want, fails)
			}
		})
	}
}

// runReleaseNotes runs scripts/release-notes.sh with the given argv in dir and
// returns stdout, stderr and the exit code. Every argument reaches the script
// literally (argv via os/exec, no shell in between).
func runReleaseNotes(t *testing.T, dir string, args ...string) (stdout, stderr string, code int) {
	t.Helper()
	script, err := filepath.Abs(filepath.Join("..", "..", "scripts", "release-notes.sh"))
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("bash", append([]string{script}, args...)...)
	cmd.Dir = dir
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	err = cmd.Run()
	code = 0
	if err != nil {
		var ee *exec.ExitError
		if !errors.As(err, &ee) {
			t.Fatalf("release-notes.sh: %v", err)
		}
		code = ee.ExitCode()
	}
	return out.String(), errb.String(), code
}

func TestReleaseNotesScript(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("release-notes.sh runs where release.yml runs (bash)")
	}
	repo, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	version := strings.TrimSpace(readRepoFile(t, "VERSION"))

	t.Run("real changelog, VERSION tag", func(t *testing.T) {
		out, errs, code := runReleaseNotes(t, repo, version)
		if code != 0 || out == "" {
			t.Fatalf("exit %d, stdout %q, stderr %q", code, out, errs)
		}
		if !strings.Contains(out, "### ") {
			t.Errorf("notes have no group heading:\n%s", out)
		}
		for _, bad := range []string{"## [", "[0.3.0]:", "## [0.3.0] - "} {
			if strings.Contains(out, bad) {
				t.Errorf("notes leak %q:\n%s", bad, out)
			}
		}
	})

	synthetic := "# Changelog\n\n## [Unreleased]\n\n- soon\n\n## [1.2.3] - 2026-01-02\n\n### Added\n- x\n\n\n## [1.2.3-rc.1] - 2026-01-01\n- rc\n## [1.2.2] - 2025-12-31\n\n- old\n\n[Unreleased]: https://github.com/GiulioSavini/gruntled/compare/v1.2.3...HEAD\n[1.2.3]: https://github.com/GiulioSavini/gruntled/compare/v1.2.2...v1.2.3\n"
	dir := t.TempDir()
	cl := filepath.Join(dir, "cl.md")
	if err := os.WriteFile(cl, []byte(synthetic), 0o644); err != nil {
		t.Fatal(err)
	}
	empty := filepath.Join(dir, "empty.md")
	if err := os.WriteFile(empty, []byte("# Changelog\n\n## [Unreleased]\n\n## [1.2.3] - 2026-01-02\n## [1.2.2] - 2025-12-31\n\n- old\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Only the last section: the body ends at the first reference line.
	last := filepath.Join(dir, "last.md")
	if err := os.WriteFile(last, []byte("# Changelog\n\n## [0.1.0] - 2026-01-02\n\n- first\n\n[0.1.0]: https://github.com/GiulioSavini/gruntled/releases/tag/v0.1.0\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	for _, c := range []struct {
		name          string
		args          []string
		code          int
		stdout, inErr string
	}{
		{"explicit changelog path", []string{"v1.2.3", cl}, 0, "### Added\n- x\n", ""},
		{"prerelease is its own section", []string{"v1.2.3-rc.1", cl}, 0, "- rc\n", ""},
		{"last section stops at the references", []string{"v0.1.0", last}, 0, "- first\n", ""},
		{"absent section", []string{"v9.9.9", cl}, 1, "", "release-notes: no section [9.9.9] in " + cl},
		{"unreleased is not a release", []string{"vUnreleased", cl}, 2, "", "release-notes: tag 'vUnreleased' does not match vX.Y.Z[-suffix]"},
		{"empty body", []string{"v1.2.3", empty}, 1, "", "release-notes: no section [1.2.3] in " + empty},
		{"no args", nil, 2, "", "usage:"},
		{"three args", []string{"v1.2.3", cl, "x"}, 2, "", "usage:"},
		{"missing changelog file", []string{"v1.2.3", filepath.Join(dir, "nope.md")}, 1, "", "release-notes: cannot read"},
		{"real changelog, no section", []string{"v0.0.0-nosuch", filepath.Join(repo, "CHANGELOG.md")}, 1, "", "release-notes: no section [0.0.0-nosuch]"},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, errs, code := runReleaseNotes(t, dir, c.args...)
			if code != c.code {
				t.Errorf("exit %d, want %d (stdout %q, stderr %q)", code, c.code, out, errs)
			}
			if out != c.stdout {
				t.Errorf("stdout %q, want %q", out, c.stdout)
			}
			if !strings.Contains(errs, c.inErr) {
				t.Errorf("stderr %q, want it to contain %q", errs, c.inErr)
			}
		})
	}

	// Malformed tags: exit 2 before the changelog is even opened, and the
	// tag text is echoed back verbatim.
	t.Run("malformed tag", func(t *testing.T) {
		for _, tag := range []string{
			"1.2.3", // no v
			"v1.2",
			"v1.2.3 OR 1=1",
			"v1.2.3*",
			"v1.2.3$(touch x)", // proves the script never evals its argument (argv is literal via os/exec)
			"v1.2.3\n",
			"",
		} {
			out, errs, code := runReleaseNotes(t, dir, tag, cl)
			want := "release-notes: tag '" + tag + "' does not match vX.Y.Z[-suffix]"
			if code != 2 || out != "" || !strings.Contains(errs, want) {
				t.Errorf("tag %q: exit %d, stdout %q, stderr %q (want exit 2 and %q)", tag, code, out, errs, want)
			}
		}
		if _, err := os.Stat(filepath.Join(dir, "x")); !os.IsNotExist(err) {
			t.Errorf("a file x appeared in the cwd: %v", err)
		}
	})
}
