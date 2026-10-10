package terragrunt

import (
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"testing"
	"testing/fstest"

	"github.com/GiulioSavini/gruntled/internal/application/ports"
)

// Symlink-alias regressions for the persistent parse cache (v0.3 sec
// MEDIUM, bus #7). An include reached through an in-repo symlink is stored
// under its lexical (alias) path, while the watchers report only the
// target path when the target is edited, or only the link path when a link
// is retargeted. Each test edits the tree, invalidates exactly what a
// watcher would report, and requires the incremental result to equal a
// fresh Loader's (DAEMON-02: incremental == full).

// aliasParentEdited is live/parent/terragrunt.hcl after the edit: the
// referenced output changes, so the edit is visible in the LoadResult.
const aliasParentEdited = `dependency "vpc" {
  config_path = "../vpc"
}
inputs = { vpc_id = dependency.vpc.outputs.CHANGED }
`

// assertIncrementalEqualsFresh fails unless got (an incremental load)
// equals a brand-new Loader's result over fsys, and the edit was
// observable at all (before differs from the fresh result), so the
// comparison is not vacuous.
func assertIncrementalEqualsFresh(t *testing.T, before, got ports.LoadResult, fsys fs.FS) {
	t.Helper()
	want := loadOK(t, NewLoader(fsys))
	if reflect.DeepEqual(before, want) {
		t.Fatalf("vacuous: the edit does not change the fresh LoadResult")
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("incremental result differs from fresh Loader (stale alias entry):\n got: %+v\nwant: %+v", got, want)
	}
}

// writeReal overwrites the repo-relative file p under dir.
func writeReal(t *testing.T, dir, p, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, filepath.FromSlash(p)), []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", p, err)
	}
}

// TestAliasCacheTargetEditRealFS is the sec PoC: the parent is included
// through a symlinked file (live/alias.hcl -> parent/terragrunt.hcl); the
// target is edited and only the target path is invalidated.
func TestAliasCacheTargetEditRealFS(t *testing.T) {
	if runtime.GOOS == "windows" {
		// Same reason as TestIncludeTargetSymlinkedFile: windows stores the
		// multi-segment link target with backslashes, which canonicalPath
		// refuses. TestAliasCacheMapFS covers the scenario on windows.
		t.Skip("windows rewrites the multi-segment link target with backslashes")
	}
	dir := realTree(t, symlinkParentTree("../../alias.hcl"))
	realSymlinkOrSkip(t, dir, "parent/terragrunt.hcl", "live/alias.hcl")
	fsys := openRealRoot(t, dir)
	l := NewLoader(fsys)
	before := loadOK(t, l)

	writeReal(t, dir, "live/parent/terragrunt.hcl", aliasParentEdited)
	l.Invalidate("live/parent/terragrunt.hcl")
	assertIncrementalEqualsFresh(t, before, loadOK(t, l), fsys)
}

// TestAliasCacheDirLinkRealFS: the parent is included through a symlinked
// directory (live/link -> parent); only the target file is invalidated.
func TestAliasCacheDirLinkRealFS(t *testing.T) {
	dir := realTree(t, symlinkParentTree("../../link/terragrunt.hcl"))
	realSymlinkOrSkip(t, dir, "parent", "live/link")
	fsys := openRealRoot(t, dir)
	l := NewLoader(fsys)
	before := loadOK(t, l)

	writeReal(t, dir, "live/parent/terragrunt.hcl", aliasParentEdited)
	l.Invalidate("live/parent/terragrunt.hcl")
	assertIncrementalEqualsFresh(t, before, loadOK(t, l), fsys)
}

// TestAliasCacheRetargetChain: the parent is included through a chain
// (live/link1 -> link2 -> parent) and the middle link is retargeted to
// parent2. The only reported path is live/link2, an ancestor of neither
// the cache key (live/link1/terragrunt.hcl) nor the old canonical path
// (live/parent/terragrunt.hcl): only the per-load canonical re-check can
// notice that the include now reads another file.
func TestAliasCacheRetargetChain(t *testing.T) {
	files := symlinkParentTree("../../link1/terragrunt.hcl")
	files["live/parent2/terragrunt.hcl"] = aliasParentEdited
	dir := realTree(t, files)
	realSymlinkOrSkip(t, dir, "link2", "live/link1")
	realSymlinkOrSkip(t, dir, "parent", "live/link2")
	fsys := openRealRoot(t, dir)
	l := NewLoader(fsys)
	before := loadOK(t, l)

	if err := os.Remove(filepath.Join(dir, "live", "link2")); err != nil {
		t.Fatalf("remove live/link2: %v", err)
	}
	realSymlinkOrSkip(t, dir, "parent2", "live/link2")
	l.Invalidate("live/link2")
	assertIncrementalEqualsFresh(t, before, loadOK(t, l), fsys)
}

// TestAliasCacheMapFS is the target-edit scenario over fstest.MapFS, whose
// MapFile with fs.ModeSymlink is a link (Data = target), so it runs on
// every OS, windows included.
func TestAliasCacheMapFS(t *testing.T) {
	m := filesFS(symlinkParentTree("../../alias.hcl"))
	m["live/alias.hcl"] = &fstest.MapFile{Mode: fs.ModeSymlink, Data: []byte("parent/terragrunt.hcl")}
	l := NewLoader(m)
	before := loadOK(t, l)
	if r := unitReason(t, before, "live/x/app"); r != "" {
		t.Fatalf("fixture: live/x/app reason = %q, want resolved (MapFS symlink not followed?)", r)
	}

	m["live/parent/terragrunt.hcl"] = &fstest.MapFile{Data: []byte(aliasParentEdited)}
	l.Invalidate("live/parent/terragrunt.hcl")
	assertIncrementalEqualsFresh(t, before, loadOK(t, l), m)
}

// TestAliasCacheNoopZeroMisses: entries reached through a symlink are
// still reused when nothing changed: a no-op reload reads no file.
func TestAliasCacheNoopZeroMisses(t *testing.T) {
	dir := realTree(t, symlinkParentTree("../../link/terragrunt.hcl"))
	realSymlinkOrSkip(t, dir, "parent", "live/link")
	l := NewLoader(openRealRoot(t, dir))
	loadOK(t, l)
	if _, misses := l.CacheStats(); misses == 0 {
		t.Fatalf("first load: misses = 0, want > 0")
	}
	loadOK(t, l)
	if hits, misses := l.CacheStats(); misses != 0 || hits == 0 {
		t.Fatalf("no-op reload: hits=%d misses=%d, want hits>0 misses=0", hits, misses)
	}
}
