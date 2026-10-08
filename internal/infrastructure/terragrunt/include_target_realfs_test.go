package terragrunt

import (
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/GiulioSavini/gruntled/internal/application/indexing"
	"github.com/GiulioSavini/gruntled/internal/domain/repograph"
)

// realTree writes files (repo-relative slash path -> content) under a
// fresh t.TempDir() and returns the directory.
func realTree(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for p, content := range files {
		full := filepath.Join(dir, filepath.FromSlash(p))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", filepath.Dir(full), err)
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			t.Fatalf("write %s: %v", full, err)
		}
	}
	return dir
}

// realSymlinkOrSkip creates newname (repo-relative, under dir) as a
// symbolic link to oldname, skipping the test where os.Symlink is not
// supported, like tfsurface/realfs_test.go.
func realSymlinkOrSkip(t *testing.T, dir, oldname, newname string) {
	t.Helper()
	if err := os.Symlink(oldname, filepath.Join(dir, filepath.FromSlash(newname))); err != nil {
		t.Skipf("os.Symlink unsupported on this platform: %v", err)
	}
}

// openRealRoot opens dir with os.OpenRoot, the production fs.FS, and closes
// it when the test ends.
func openRealRoot(t *testing.T, dir string) fs.FS {
	t.Helper()
	root, err := os.OpenRoot(dir)
	if err != nil {
		t.Fatalf("os.OpenRoot(%q): %v", dir, err)
	}
	t.Cleanup(func() { root.Close() })
	return root.FS()
}

// symlinkParentTree is the 02-VERIFICATION repro tree for G15: live/parent
// is a parent config whose dependency "../vpc" resolves, standalone, to
// live/vpc (which lacks output "id"), and, through live/x/app, to live/x/vpc
// (which declares it). appInclude is live/x/app's include path.
func symlinkParentTree(appInclude string) map[string]string {
	return map[string]string{
		"live/parent/terragrunt.hcl": `dependency "vpc" {
  config_path = "../vpc"
}
inputs = { vpc_id = dependency.vpc.outputs.id }
`,
		"live/vpc/terragrunt.hcl":   "",
		"live/vpc/main.tf":          `output "other" { value = 1 }`,
		"live/x/vpc/terragrunt.hcl": "",
		"live/x/vpc/main.tf":        `output "id" { value = 1 }`,
		"live/x/app/terragrunt.hcl": `include { path = "` + appInclude + `" }`,
		"live/x/app/main.tf":        "",
	}
}

// missingOutputs counts the graph references whose target module surface
// is known and lacks the referenced output: every one would be a GRT001.
func missingOutputs(res indexing.Result) int {
	missing := 0
	for _, ur := range res.Graph.References() {
		target, ok := res.Graph.DependencyTarget(ur.Unit, ur.Reference.Dependency())
		if !ok {
			continue
		}
		mod, ok := res.Graph.ModuleOf(target.Path())
		if !ok {
			continue
		}
		surf, ok := mod.Surface()
		if !ok {
			continue
		}
		if !surf.HasOutput(ur.Reference.Output()) {
			missing++
		}
	}
	return missing
}

// assertSymlinkParentMarked checks the shared expectations of the G15
// symlink rows: live/parent is include-target, live/x/app is resolved with
// its vpc dependency on live/x/vpc, and no reference misses its output.
func assertSymlinkParentMarked(t *testing.T, res indexing.Result) {
	t.Helper()
	parent, ok := res.Graph.Unit(repograph.MustRepoPath("live/parent"))
	if !ok {
		t.Fatalf("unit live/parent not found")
	}
	if parent.Status() != repograph.StatusConfigUnknown || parent.UnknownReason() != ReasonIncludeTarget {
		t.Fatalf("live/parent Status()=%v UnknownReason()=%q, want %v/%q", parent.Status(), parent.UnknownReason(), repograph.StatusConfigUnknown, ReasonIncludeTarget)
	}
	app, ok := res.Graph.Unit(repograph.MustRepoPath("live/x/app"))
	if !ok {
		t.Fatalf("unit live/x/app not found")
	}
	if app.Status() != repograph.StatusResolved {
		t.Fatalf("live/x/app Status() = %v (reason %q), want %v", app.Status(), app.UnknownReason(), repograph.StatusResolved)
	}
	dep, ok := app.Dependency("vpc")
	if !ok {
		t.Fatalf("live/x/app has no dependency vpc")
	}
	if target, ok := dep.Target(); !ok || target.String() != "live/x/vpc" {
		t.Fatalf("vpc Target() = (%q, %v), want (live/x/vpc, true)", target.String(), ok)
	}
	if n := missingOutputs(res); n != 0 {
		t.Fatalf("references missing their declared output = %d, want 0", n)
	}
}

// TestIncludeTargetSymlinkedDir is the exact 02-VERIFICATION repro: the
// parent is included through a symlinked directory (live/link -> parent).
func TestIncludeTargetSymlinkedDir(t *testing.T) {
	dir := realTree(t, symlinkParentTree("../../link/terragrunt.hcl"))
	realSymlinkOrSkip(t, dir, "parent", "live/link")
	assertSymlinkParentMarked(t, build(t, openRealRoot(t, dir)))
}

// TestIncludeTargetSymlinkedFile: the parent is included through a
// symlinked file (live/alias.hcl -> parent/terragrunt.hcl).
func TestIncludeTargetSymlinkedFile(t *testing.T) {
	if runtime.GOOS == "windows" {
		// os.Symlink stores "parent/terragrunt.hcl" as "parent\terragrunt.hcl"
		// on windows, and canonicalPath deliberately fails closed on a
		// backslash link target.
		t.Skip("windows rewrites the multi-segment link target with backslashes")
	}
	dir := realTree(t, symlinkParentTree("../../alias.hcl"))
	realSymlinkOrSkip(t, dir, "parent/terragrunt.hcl", "live/alias.hcl")
	assertSymlinkParentMarked(t, build(t, openRealRoot(t, dir)))
}

// TestIncludeTargetSymlinkChain: the parent is included through a chain of
// directory links (live/link1 -> link2 -> parent).
func TestIncludeTargetSymlinkChain(t *testing.T) {
	dir := realTree(t, symlinkParentTree("../../link1/terragrunt.hcl"))
	realSymlinkOrSkip(t, dir, "link2", "live/link1")
	realSymlinkOrSkip(t, dir, "parent", "live/link2")
	assertSymlinkParentMarked(t, build(t, openRealRoot(t, dir)))
}

// TestIncludeSelfViaSymlink: a unit including its own terragrunt.hcl
// through a symlinked directory is self-inclusion, detected canonically.
func TestIncludeSelfViaSymlink(t *testing.T) {
	dir := realTree(t, map[string]string{
		"live/a/terragrunt.hcl": `include { path = "../self/terragrunt.hcl" }`,
	})
	realSymlinkOrSkip(t, dir, "a", "live/self")
	res := loadUnits(t, openRealRoot(t, dir))
	a := unitByPath(t, res, "live/a")
	if a.ConfigUnknownReason != ReasonInvalidInclude {
		t.Fatalf("live/a ConfigUnknownReason = %q, want %q", a.ConfigUnknownReason, ReasonInvalidInclude)
	}
}

// TestIncludeSymlinkEscapesRepo: an include reached through a link that
// points outside the root never resolves against the outside file. os.Root
// already refuses to follow the escaping link at the include's Stat, so the
// includer is include-not-found (canonicalPath's own canonOutside would give
// include-outside-repo for an fs.FS that followed it).
func TestIncludeSymlinkEscapesRepo(t *testing.T) {
	outside := realTree(t, map[string]string{
		"terragrunt.hcl": `dependency "vpc" { config_path = "../vpc" }`,
	})
	dir := realTree(t, map[string]string{
		"live/app/terragrunt.hcl": `include { path = "../link/terragrunt.hcl" }`,
	})
	realSymlinkOrSkip(t, dir, outside, "live/link")
	res := loadUnits(t, openRealRoot(t, dir))
	app := unitByPath(t, res, "live/app")
	if app.ConfigUnknownReason != ReasonIncludeNotFound {
		t.Fatalf("live/app ConfigUnknownReason = %q, want %q", app.ConfigUnknownReason, ReasonIncludeNotFound)
	}
}
