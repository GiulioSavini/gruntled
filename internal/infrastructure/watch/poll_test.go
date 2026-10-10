package watch

import (
	"io/fs"
	"os"
	"reflect"
	"sort"
	"testing"
	"time"
)

func TestWatcherContractPoll(t *testing.T) {
	runContract(t, func(root string) (Watcher, error) {
		return NewPoll(root, 10*time.Millisecond)
	})
}

func TestPollRejectsMissingRoot(t *testing.T) {
	if _, err := NewPoll(abs(t.TempDir(), "nope"), 10*time.Millisecond); err == nil {
		t.Fatal("NewPoll on a missing root returned no error")
	}
}

// TestPollScannerDiff drives the scanner directly: no goroutine, no timing.
func TestPollScannerDiff(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "keep.hcl", "k")
	writeFile(t, root, "mod.hcl", "m")
	writeFile(t, root, "gone/x.hcl", "x")
	writeFile(t, root, ".terraform/p.tf", "p")
	writeFile(t, root, "2024/f.hcl", "f")
	s := newScanner(root)
	if got := s.diff(); len(got) != 0 || s.resync {
		t.Fatalf("diff on unchanged tree = %v resync=%v", got, s.resync)
	}

	writeFile(t, root, "mod.hcl", "mm")
	if err := os.RemoveAll(abs(root, "gone")); err != nil {
		t.Fatal(err)
	}
	writeFile(t, root, "new/y.hcl", "y")
	writeFile(t, root, ".terraform/q.tf", "q")
	writeFile(t, root, "x.swp", "s")
	writeFile(t, root, "2024/f.hcl", "ff") // file inside a pattern-named dir
	writeFile(t, root, "4913", "")         // vim probe: a regular file, ignored
	got := rels(s.diff())
	want := []string{"2024/f.hcl", "gone", "gone/x.hcl", "mod.hcl", "new", "new/y.hcl"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("diff = %v, want %v", got, want)
	}
	if got := s.diff(); len(got) != 0 {
		t.Fatalf("diff after snapshot replaced = %v, want none", got)
	}
}

// rels returns the sorted paths of a scanner diff.
func rels(cs []change) []string {
	out := make([]string, 0, len(cs))
	for _, c := range cs {
		out = append(out, c.rel)
	}
	sort.Strings(out)
	return out
}

// TestPollScannerDiffTypes: every change carries the entry type, taken from
// the current scan for present paths and from the previous one for removed
// paths, so pending.add applies the right ignore rule.
func TestPollScannerDiffTypes(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "2024/f.hcl", "f")
	s := newScanner(root)
	if err := os.RemoveAll(abs(root, "2024")); err != nil {
		t.Fatal(err)
	}
	writeFile(t, root, "x.tmp/g.hcl", "g")
	types := map[string]fs.FileMode{}
	for _, c := range s.diff() {
		types[c.rel] = c.typ
	}
	want := map[string]fs.FileMode{"2024": fs.ModeDir, "2024/f.hcl": 0, "x.tmp": fs.ModeDir, "x.tmp/g.hcl": 0}
	if !reflect.DeepEqual(types, want) {
		t.Fatalf("diff types = %v, want %v", types, want)
	}
}

func TestPollScannerRootGoneIsResync(t *testing.T) {
	parent := t.TempDir()
	root := abs(parent, "r")
	writeFile(t, root, "a.hcl", "a")
	s := newScanner(root)
	if err := os.RemoveAll(root); err != nil {
		t.Fatal(err)
	}
	got := s.diff()
	if !s.resync || len(got) != 0 {
		t.Fatalf("diff = %v resync=%v, want resync and no paths", got, s.resync)
	}
}

func TestPollScannerSymlinkNotFollowed(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	writeFile(t, outside, "o.hcl", "o")
	if err := os.Symlink(outside, abs(root, "link")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	s := newScanner(root)
	writeFile(t, outside, "o.hcl", "changed")
	if got := s.diff(); len(got) != 0 {
		t.Fatalf("change behind a symlinked dir reported: %v", got)
	}
}
