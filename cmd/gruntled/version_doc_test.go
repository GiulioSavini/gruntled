package main

// The VERSION file at the repository root is the one place the shipped
// version is written. Every install snippet that names a version (docs/ci.md,
// README.md, the pre-commit rev, the docs/cli.md --version example) must equal
// it; checkVersionPins proves that, and a designated pattern that never
// matches is itself a failure, so a renamed snippet cannot make the check
// vacuous. Plain regexp checks; no YAML or markdown dependency.

import (
	"regexp"
	"sort"
	"strings"
	"testing"
)

// versionToken is the shape of a version pin: vX.Y.Z plus an optional
// pre-release/build suffix. It is the capture group of every pin pattern.
const versionToken = `(?P<v>v[0-9]+\.[0-9]+\.[0-9]+[-0-9A-Za-z.]*)`

// versionFileRe is the exact shape of VERSION: one tag, one trailing newline.
var versionFileRe = regexp.MustCompile(`^v[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.-]+)?\n$`)

// versionPins is the table of (file, pattern) the checker enforces. A new
// install channel adds its row here (Phase 23: install.sh, formula, image).
var versionPins = []struct {
	file    string
	pattern *regexp.Regexp
}{
	{"docs/ci.md", regexp.MustCompile(`go install github\.com/GiulioSavini/gruntled/cmd/gruntled@` + versionToken)},
	{"docs/ci.md", regexp.MustCompile(`rev: ` + versionToken)},
	{"docs/ci.md", regexp.MustCompile(`tag=` + versionToken)},
	{"docs/ci.md", regexp.MustCompile(`gruntled_` + versionToken + `_linux_amd64\.tar\.gz`)},
	{"README.md", regexp.MustCompile(`go install github\.com/GiulioSavini/gruntled/cmd/gruntled@` + versionToken)},
	{"docs/cli.md", regexp.MustCompile(`gruntled ` + versionToken + ` \(none\)`)},
}

// anyVersionToken finds every version-shaped token on a line, for the
// all-tokens rule over docs/ci.md (sec #484): prose such as "In the examples
// `v0.2.0` stands for" is a pin too, even though no snippet pattern covers it.
var anyVersionToken = regexp.MustCompile(`v[0-9]+\.[0-9]+\.[0-9]+[-0-9A-Za-z.]*`)

// allowedForeignToken reports whether a version token on a docs/ci.md line is
// not a gruntled version: only the "# vX.Y.Z" comment after an action pin
// from ciPins (actions/checkout, actions/setup-go, codeql upload-sarif).
func allowedForeignToken(line, tok string) bool {
	for _, p := range ciPins {
		if tok == p.version && strings.Contains(line, "uses: "+p.action+"@"+p.sha+" # "+p.version) {
			return true
		}
	}
	return false
}

// checkVersionPins returns one failure per pin that differs from version, per
// designated pattern with no occurrence, per file not provided, and (for
// docs/ci.md) per version-shaped token outside the allowlist that differs
// from version. The result is sorted; empty means every pin equals version.
func checkVersionPins(version string, files map[string]string) []string {
	var fails []string
	missing := map[string]bool{}
	for _, p := range versionPins {
		text, ok := files[p.file]
		if !ok {
			if !missing[p.file] {
				fails = append(fails, p.file+": not provided")
				missing[p.file] = true
			}
			continue
		}
		ms := p.pattern.FindAllStringSubmatch(text, -1)
		if len(ms) == 0 {
			fails = append(fails, p.file+": no occurrence of "+p.pattern.String())
			continue
		}
		vi := p.pattern.SubexpIndex("v")
		for _, m := range ms {
			if m[vi] != version {
				fails = append(fails, p.file+": "+m[0]+" pins "+m[vi]+", VERSION is "+version)
			}
		}
	}
	if text, ok := files["docs/ci.md"]; ok {
		for i, line := range strings.Split(text, "\n") {
			for _, tok := range anyVersionToken.FindAllString(line, -1) {
				if tok == version || allowedForeignToken(line, tok) {
					continue
				}
				fails = append(fails, "docs/ci.md:"+itoa(i+1)+": token "+tok+" differs from VERSION "+version+" in: "+strings.TrimSpace(line))
			}
		}
	}
	sort.Strings(fails)
	return fails
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}

func repoVersion(t *testing.T) string {
	t.Helper()
	raw := readRepoFile(t, "VERSION")
	if !versionFileRe.MatchString(raw) {
		t.Fatalf("VERSION must be vX.Y.Z[-suffix] plus one trailing newline, got %q", raw)
	}
	return strings.TrimSuffix(raw, "\n")
}

func pinFiles(t *testing.T) map[string]string {
	t.Helper()
	files := map[string]string{}
	for _, f := range []string{"docs/ci.md", "docs/cli.md", "README.md"} {
		files[f] = readRepoFile(t, f)
	}
	return files
}

func TestVersionFileShape(t *testing.T) {
	v := repoVersion(t)
	if v != "v0.4.0" {
		t.Errorf("VERSION is %q; this milestone ships v0.4.0 (bump it only in the commit that gets tagged)", v)
	}
	for _, bad := range []string{"v0.4.0", "v0.4.0\r\n", "v0.4.0\n\n", " v0.4.0\n", "0.4.0\n", "v0.4\n"} {
		if versionFileRe.MatchString(bad) {
			t.Errorf("shape regexp accepts %q", bad)
		}
	}
	for _, good := range []string{"v0.4.0\n", "v1.2.3-rc.1\n"} {
		if !versionFileRe.MatchString(good) {
			t.Errorf("shape regexp rejects %q", good)
		}
	}
}

func TestVersionPins(t *testing.T) {
	v := repoVersion(t)
	for _, f := range checkVersionPins(v, pinFiles(t)) {
		t.Error(f)
	}
}

func TestVersionPinsNegative(t *testing.T) {
	v := repoVersion(t)
	real := pinFiles(t)
	if fails := checkVersionPins(v, real); len(fails) != 0 {
		t.Fatalf("real files must be clean before the negatives mean anything: %v", fails)
	}
	clone := func() map[string]string {
		m := map[string]string{}
		for k, s := range real {
			m[k] = s
		}
		return m
	}
	hasAll := func(t *testing.T, fails []string, want int, subs ...string) {
		t.Helper()
		if len(fails) != want {
			t.Fatalf("want %d failure(s), got %d: %v", want, len(fails), fails)
		}
		joined := strings.Join(fails, "\n")
		for _, s := range subs {
			if !strings.Contains(joined, s) {
				t.Errorf("failures %v do not mention %q", fails, s)
			}
		}
	}

	t.Run("one drifted pin", func(t *testing.T) {
		m := clone()
		m["docs/ci.md"] = strings.Replace(m["docs/ci.md"], "rev: "+v, "rev: v0.0.1", 1)
		// The rev line drifts for the snippet pattern and the all-tokens rule.
		hasAll(t, checkVersionPins(v, m), 2, "docs/ci.md", "rev: v0.0.1", "v0.0.1")
	})
	t.Run("drift in README", func(t *testing.T) {
		m := clone()
		m["README.md"] = strings.Replace(m["README.md"], "cmd/gruntled@"+v, "cmd/gruntled@v9.9.9", 1)
		hasAll(t, checkVersionPins(v, m), 1, "README.md", "v9.9.9")
	})
	t.Run("pattern deleted", func(t *testing.T) {
		m := clone()
		var keep []string
		for _, l := range strings.Split(m["docs/ci.md"], "\n") {
			if !strings.Contains(l, "tag="+v) {
				keep = append(keep, l)
			}
		}
		m["docs/ci.md"] = strings.Join(keep, "\n")
		hasAll(t, checkVersionPins(v, m), 1, "docs/ci.md: no occurrence of")
	})
	t.Run("missing file", func(t *testing.T) {
		m := clone()
		delete(m, "docs/cli.md")
		hasAll(t, checkVersionPins(v, m), 1, "docs/cli.md: not provided")
	})
	t.Run("checker is not fooled by the VERSION value", func(t *testing.T) {
		fails := checkVersionPins("v0.0.1", real)
		if len(fails) < 6 {
			t.Fatalf("want at least 6 failures with version v0.0.1, got %d: %v", len(fails), fails)
		}
	})
	t.Run("prose token drifts (sec #484)", func(t *testing.T) {
		// Line 7 of docs/ci.md names the version in prose only; no snippet
		// pattern covers it, the all-tokens rule must.
		m := clone()
		lines := strings.Split(m["docs/ci.md"], "\n")
		if !strings.Contains(lines[6], "`"+v+"`") {
			t.Fatalf("docs/ci.md line 7 does not name %s in prose: %q", v, lines[6])
		}
		lines[6] = strings.Replace(lines[6], "`"+v+"`", "`v0.0.1`", 1)
		m["docs/ci.md"] = strings.Join(lines, "\n")
		hasAll(t, checkVersionPins(v, m), 1, "docs/ci.md:7:", "v0.0.1")
	})
	t.Run("action pin comments are the only foreign tokens", func(t *testing.T) {
		// The allowlist is exactly the ciPins uses: lines; a foreign token
		// anywhere else, or a changed action version, fails.
		m := clone()
		m["docs/ci.md"] += "\nnothing to do with gruntled v7.0.1\n"
		hasAll(t, checkVersionPins(v, m), 1, "token v7.0.1")
		m = clone()
		m["docs/ci.md"] = strings.Replace(m["docs/ci.md"], "# v7.0.1", "# v7.0.2", 1)
		hasAll(t, checkVersionPins(v, m), 1, "token v7.0.2")
	})
}
