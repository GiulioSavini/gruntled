package watch

import (
	"os"
	"strings"
	"testing"
	"time"
)

// runContract is the behaviour every Watcher adapter must honour. The poll
// adapter runs it in poll_test.go; the native adapter reuses it.
//
// Each sub-test builds its fixture, THEN constructs a fresh watcher (the
// constructor installs watches / takes the poll baseline), THEN mutates the
// tree. File edits always change size so mtime granularity cannot hide them.
func runContract(t *testing.T, newWatcher func(root string) (Watcher, error)) {
	t.Helper()

	start := func(t *testing.T, root string) *collector {
		t.Helper()
		w, err := newWatcher(root)
		if err != nil {
			t.Fatalf("new watcher: %v", err)
		}
		t.Cleanup(func() { _ = w.Close() })
		return newCollector(t, w)
	}

	t.Run("create", func(t *testing.T) {
		root := t.TempDir()
		if err := os.Mkdir(abs(root, "a"), 0o755); err != nil {
			t.Fatal(err)
		}
		c := start(t, root)
		writeFile(t, root, "a/terragrunt.hcl", "x")
		collect(t, c, func(s map[string]bool) bool { return s["a/terragrunt.hcl"] }, "a/terragrunt.hcl created")
	})

	t.Run("modify", func(t *testing.T) {
		root := t.TempDir()
		writeFile(t, root, "a/f.hcl", "x")
		c := start(t, root)
		writeFile(t, root, "a/f.hcl", "xyz")
		collect(t, c, func(s map[string]bool) bool { return s["a/f.hcl"] }, "a/f.hcl modified")
	})

	t.Run("delete", func(t *testing.T) {
		root := t.TempDir()
		writeFile(t, root, "a/f.hcl", "x")
		c := start(t, root)
		if err := os.Remove(abs(root, "a/f.hcl")); err != nil {
			t.Fatal(err)
		}
		collect(t, c, func(s map[string]bool) bool { return s["a/f.hcl"] }, "a/f.hcl deleted")
	})

	t.Run("rename", func(t *testing.T) {
		root := t.TempDir()
		writeFile(t, root, "a/x.hcl", "x")
		c := start(t, root)
		if err := os.Rename(abs(root, "a/x.hcl"), abs(root, "a/y.hcl")); err != nil {
			t.Fatal(err)
		}
		collect(t, c, func(s map[string]bool) bool { return s["a/x.hcl"] && s["a/y.hcl"] }, "old and new rename paths")
	})

	t.Run("mkdir with files", func(t *testing.T) {
		root := t.TempDir()
		c := start(t, root)
		writeFile(t, root, "n/m/terragrunt.hcl", "x")
		writeFile(t, root, "n/m/other.tf", "y")
		collect(t, c, func(s map[string]bool) bool {
			return s["n/m/terragrunt.hcl"] || s["n"] || s["n/m"]
		}, "new directory tree reported (dir or file path)")
		// The new directory must now be watched: a later edit is reported
		// by its file path.
		c.reset()
		writeFile(t, root, "n/m/terragrunt.hcl", "xyz-longer")
		collect(t, c, func(s map[string]bool) bool { return s["n/m/terragrunt.hcl"] }, "edit inside new directory")
	})

	t.Run("rmdir", func(t *testing.T) {
		root := t.TempDir()
		writeFile(t, root, "d/e/f.hcl", "x")
		c := start(t, root)
		if err := os.RemoveAll(abs(root, "d")); err != nil {
			t.Fatal(err)
		}
		collect(t, c, func(s map[string]bool) bool {
			return s["d"] || s["d/e"] || s["d/e/f.hcl"]
		}, "removed directory tree reported")
	})

	t.Run("vim-style save", func(t *testing.T) {
		root := t.TempDir()
		writeFile(t, root, "f.hcl", "x")
		c := start(t, root)
		writeFile(t, root, "4913", "")
		if err := os.Remove(abs(root, "4913")); err != nil {
			t.Fatal(err)
		}
		if err := os.Rename(abs(root, "f.hcl"), abs(root, "f.hcl~")); err != nil {
			t.Fatal(err)
		}
		writeFile(t, root, "f.hcl", "xyz")
		if err := os.Remove(abs(root, "f.hcl~")); err != nil {
			t.Fatal(err)
		}
		writeFile(t, root, "sentinel.hcl", "s")
		collect(t, c, func(s map[string]bool) bool { return s["f.hcl"] && s["sentinel.hcl"] }, "f.hcl and sentinel")
		for _, p := range []string{"4913", "f.hcl~"} {
			if c.seen[p] {
				t.Errorf("ignored path %q reached the dirty set: %s", p, c)
			}
		}
	})

	t.Run("ignored dirs via sentinel", func(t *testing.T) {
		root := t.TempDir()
		c := start(t, root)
		writeFile(t, root, ".git/x", "x")
		writeFile(t, root, ".terraform/y", "y")
		writeFile(t, root, ".terragrunt-cache/z/w.hcl", "w")
		writeFile(t, root, "a.swp", "s")
		writeFile(t, root, "sentinel.hcl", "s")
		collect(t, c, func(s map[string]bool) bool { return s["sentinel.hcl"] }, "sentinel")
		for p := range c.seen {
			if p != "sentinel.hcl" {
				t.Errorf("unexpected path %q (ignored content leaked): %s", p, c)
			}
		}
	})

	// Directories named like editor files are ordinary directories: only
	// files are matched by the editor patterns (v0.3 audit BLOCKER).
	patternDirs := []string{"2024", "live/4913", "x.tmp", "bak~", "#d#", ".#d"}

	t.Run("edit inside pattern-named dirs", func(t *testing.T) {
		root := t.TempDir()
		for _, d := range patternDirs {
			writeFile(t, root, d+"/f.hcl", "x")
		}
		c := start(t, root)
		for _, d := range patternDirs {
			writeFile(t, root, d+"/f.hcl", "xyz-longer")
		}
		collect(t, c, func(s map[string]bool) bool {
			for _, d := range patternDirs {
				if !s[d+"/f.hcl"] {
					return false
				}
			}
			return true
		}, "every <pattern dir>/f.hcl edited")
	})

	t.Run("mkdir pattern-named dir", func(t *testing.T) {
		root := t.TempDir()
		c := start(t, root)
		writeFile(t, root, "live/2024/terragrunt.hcl", "x")
		collect(t, c, func(s map[string]bool) bool {
			return s["live/2024/terragrunt.hcl"] || s["live/2024"] || s["live"]
		}, "new pattern-named directory reported (dir or file path)")
		c.reset()
		writeFile(t, root, "live/2024/terragrunt.hcl", "xyz-longer")
		collect(t, c, func(s map[string]bool) bool { return s["live/2024/terragrunt.hcl"] }, "edit inside new pattern-named directory")
	})

	under := func(s map[string]bool, dir string) bool {
		for p := range s {
			if p == dir || strings.HasPrefix(p, dir+"/") {
				return true
			}
		}
		return false
	}

	t.Run("rename pattern-named dir away", func(t *testing.T) {
		root := t.TempDir()
		writeFile(t, root, "2024/f.hcl", "x")
		c := start(t, root)
		if err := os.Rename(abs(root, "2024"), abs(root, "y")); err != nil {
			t.Fatal(err)
		}
		collect(t, c, func(s map[string]bool) bool { return under(s, "2024") && under(s, "y") }, "old and new pattern-dir rename paths")
	})

	t.Run("mkdir pattern-named dir after start, then rename it away", func(t *testing.T) {
		root := t.TempDir()
		c := start(t, root)
		if err := os.Mkdir(abs(root, "2024"), 0o755); err != nil {
			t.Fatal(err)
		}
		writeFile(t, root, "2024/f.hcl", "x")
		collect(t, c, func(s map[string]bool) bool { return s["2024/f.hcl"] }, "2024/f.hcl created in a runtime pattern-named dir")
		c.reset()
		if err := os.Rename(abs(root, "2024"), abs(root, "y")); err != nil {
			t.Fatal(err)
		}
		collect(t, c, func(s map[string]bool) bool { return under(s, "2024") && under(s, "y") }, "old and new paths of the runtime pattern dir")
	})

	t.Run("emacs lock symlink", func(t *testing.T) {
		root := t.TempDir()
		writeFile(t, root, "terragrunt.hcl", "x")
		c := start(t, root)
		if err := os.Symlink("u@h.1:1", abs(root, ".#terragrunt.hcl")); err != nil {
			t.Skipf("symlinks unavailable: %v", err)
		}
		if err := os.Remove(abs(root, ".#terragrunt.hcl")); err != nil {
			t.Fatal(err)
		}
		writeFile(t, root, "s.hcl", "s")
		collect(t, c, func(s map[string]bool) bool { return s["s.hcl"] }, "sentinel s.hcl")
		if c.seen[".#terragrunt.hcl"] {
			t.Errorf("Emacs lock symlink reached the dirty set: %s", c)
		}
	})

	t.Run("no escapes", func(t *testing.T) {
		root := t.TempDir()
		writeFile(t, root, "a/b/c.hcl", "x")
		c := start(t, root)
		writeFile(t, root, "a/b/c.hcl", "xy")
		writeFile(t, root, "top.hcl", "t")
		if err := os.RemoveAll(abs(root, "a")); err != nil {
			t.Fatal(err)
		}
		writeFile(t, root, "sentinel.hcl", "s")
		collect(t, c, func(s map[string]bool) bool { return s["sentinel.hcl"] && s["top.hcl"] }, "sentinel and top.hcl")
		for p := range c.seen {
			if strings.HasPrefix(p, "/") || strings.Contains(p, "..") || strings.ContainsRune(p, '\\') {
				t.Errorf("escaping path %q", p)
			}
		}
	})

	t.Run("ready signals", func(t *testing.T) {
		root := t.TempDir()
		w, err := newWatcher(root)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = w.Close() })
		writeFile(t, root, "r.hcl", "r")
		select {
		case <-w.Ready():
		case <-time.After(contractTimeout):
			t.Fatal("Ready never signalled after a change")
		}
		c := newCollector(t, w)
		collect(t, c, func(s map[string]bool) bool { return s["r.hcl"] }, "r.hcl after Ready")
	})

	t.Run("close idempotent", func(t *testing.T) {
		root := t.TempDir()
		w, err := newWatcher(root)
		if err != nil {
			t.Fatal(err)
		}
		if err := w.Close(); err != nil {
			t.Fatalf("first Close: %v", err)
		}
		if err := w.Close(); err != nil {
			t.Fatalf("second Close: %v", err)
		}
		if w.Ready() == nil {
			t.Fatal("Ready() nil after Close")
		}
		select {
		case <-w.Ready():
		default:
		}
		_ = w.Take()
	})
}
