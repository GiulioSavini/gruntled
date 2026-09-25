package terragrunt

import (
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"testing"
	"testing/fstest"
	"time"

	"github.com/GiulioSavini/gruntled/internal/testsupport/synthrepo"
)

// dirs extracts the sorted list of directories from a discoverUnits result.
func dirs(t *testing.T, entries []unitEntry) []string {
	t.Helper()
	got := make([]string, len(entries))
	for i, e := range entries {
		got[i] = e.dir
	}
	sort.Strings(got)
	return got
}

func symlinkFile(target string) *fstest.MapFile {
	return &fstest.MapFile{Mode: fs.ModeSymlink, Data: []byte(target)}
}

func TestWalkSkipsDecoyDirectories(t *testing.T) {
	fsys := fstest.MapFS{
		".terragrunt-cache/x/terragrunt.hcl":     &fstest.MapFile{},
		"a/.terragrunt-cache/h/v/terragrunt.hcl": &fstest.MapFile{},
		"a/.terraform/modules/m/terragrunt.hcl":  &fstest.MapFile{},
		"vendor/m/terragrunt.hcl":                &fstest.MapFile{},
		".git/x/terragrunt.hcl":                  &fstest.MapFile{},
		"a/terragrunt.hcl":                       &fstest.MapFile{},
	}

	got, err := discoverUnits(fsys)
	if err != nil {
		t.Fatalf("discoverUnits: unexpected error: %v", err)
	}
	want := []string{"a"}
	if !slices.Equal(dirs(t, got), want) {
		t.Fatalf("dirs = %v, want %v", dirs(t, got), want)
	}
}

func TestWalkTerragruntStackIsWalked(t *testing.T) {
	fsys := fstest.MapFS{
		"stacks/prod/.terragrunt-stack/vpc/terragrunt.hcl": &fstest.MapFile{},
		"stacks/prod/terragrunt.stack.hcl":                 &fstest.MapFile{},
	}

	got, err := discoverUnits(fsys)
	if err != nil {
		t.Fatalf("discoverUnits: unexpected error: %v", err)
	}
	want := []string{"stacks/prod/.terragrunt-stack/vpc"}
	if !slices.Equal(dirs(t, got), want) {
		t.Fatalf("dirs = %v, want %v (terragrunt.stack.hcl alone must not be a unit)", dirs(t, got), want)
	}
}

func TestWalkSymlinkedDirectoryNotDescended(t *testing.T) {
	fsys := fstest.MapFS{
		"a/terragrunt.hcl": &fstest.MapFile{},
		"linked":           symlinkFile("a"),
	}

	got, err := discoverUnits(fsys)
	if err != nil {
		t.Fatalf("discoverUnits: unexpected error: %v", err)
	}
	want := []string{"a"}
	if !slices.Equal(dirs(t, got), want) {
		t.Fatalf("dirs = %v, want %v (nothing under the symlinked dir should be discovered)", dirs(t, got), want)
	}
}

func TestWalkSymlinkedFileNotCounted(t *testing.T) {
	fsys := fstest.MapFS{
		"a/terragrunt.hcl": &fstest.MapFile{},
		"b/terragrunt.hcl": symlinkFile("../a/terragrunt.hcl"),
	}

	got, err := discoverUnits(fsys)
	if err != nil {
		t.Fatalf("discoverUnits: unexpected error: %v", err)
	}
	want := []string{"a"}
	if !slices.Equal(dirs(t, got), want) {
		t.Fatalf("dirs = %v, want %v (b's symlinked terragrunt.hcl must not count)", dirs(t, got), want)
	}
}

func TestWalkSymlinkCycleReturnsPromptly(t *testing.T) {
	fsys := fstest.MapFS{
		"x": symlinkFile("y"),
		"y": symlinkFile("x"),
	}

	done := make(chan struct{})
	var got []unitEntry
	var err error
	go func() {
		got, err = discoverUnits(fsys)
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("discoverUnits did not return promptly on a symlink cycle")
	}
	if err != nil {
		t.Fatalf("discoverUnits: unexpected error: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("dirs = %v, want none", dirs(t, got))
	}
}

func TestWalkRootUnitAndBareRootHCL(t *testing.T) {
	fsys := fstest.MapFS{
		"terragrunt.hcl": &fstest.MapFile{},
	}
	got, err := discoverUnits(fsys)
	if err != nil {
		t.Fatalf("discoverUnits: unexpected error: %v", err)
	}
	want := []string{"."}
	if !slices.Equal(dirs(t, got), want) {
		t.Fatalf("dirs = %v, want %v", dirs(t, got), want)
	}

	fsys2 := fstest.MapFS{
		"root.hcl": &fstest.MapFile{},
	}
	got2, err := discoverUnits(fsys2)
	if err != nil {
		t.Fatalf("discoverUnits: unexpected error: %v", err)
	}
	if len(got2) != 0 {
		t.Fatalf("dirs = %v, want none (root.hcl alone is not a unit)", dirs(t, got2))
	}
}

func TestWalkJSONConfig(t *testing.T) {
	fsys := fstest.MapFS{
		"both/terragrunt.hcl":          &fstest.MapFile{},
		"both/terragrunt.hcl.json":     &fstest.MapFile{},
		"jsononly/terragrunt.hcl.json": &fstest.MapFile{},
	}
	got, err := discoverUnits(fsys)
	if err != nil {
		t.Fatalf("discoverUnits: unexpected error: %v", err)
	}
	byDir := map[string]unitEntry{}
	for _, e := range got {
		byDir[e.dir] = e
	}
	if len(byDir) != 2 {
		t.Fatalf("got %d units, want 2: %v", len(byDir), byDir)
	}
	if e := byDir["both"]; !e.jsonConfig {
		t.Fatalf(`"both".jsonConfig = false, want true`)
	}
	if e := byDir["jsononly"]; !e.jsonConfig {
		t.Fatalf(`"jsononly".jsonConfig = false, want true`)
	}
}

func TestWalkNestedUnitsAndVendorPrefix(t *testing.T) {
	fsys := fstest.MapFS{
		"a/terragrunt.hcl":        &fstest.MapFile{},
		"a/b/terragrunt.hcl":      &fstest.MapFile{},
		"vendor-x/terragrunt.hcl": &fstest.MapFile{},
	}
	got, err := discoverUnits(fsys)
	if err != nil {
		t.Fatalf("discoverUnits: unexpected error: %v", err)
	}
	want := []string{"a", "a/b", "vendor-x"}
	if !slices.Equal(dirs(t, got), want) {
		t.Fatalf("dirs = %v, want %v (only the exact name \"vendor\" is skipped)", dirs(t, got), want)
	}
}

func TestWalkSynthrepoScale(t *testing.T) {
	tree, manifest, err := synthrepo.Render(synthrepo.Spec{Units: 50, IncludeDepth: 3, DependencyFanout: 3, Seed: 7})
	if err != nil {
		t.Fatalf("synthrepo.Render: unexpected error: %v", err)
	}
	got, err := discoverUnits(mapFS(tree))
	if err != nil {
		t.Fatalf("discoverUnits: unexpected error: %v", err)
	}
	gotDirs := dirs(t, got)
	wantDirs := make([]string, len(manifest.Units))
	for i, u := range manifest.Units {
		wantDirs[i] = u.String()
	}
	sort.Strings(wantDirs)
	if !slices.Equal(gotDirs, wantDirs) {
		t.Fatalf("discovered dirs (sorted) != manifest.Units (sorted)\ngot:  %v\nwant: %v", gotDirs, wantDirs)
	}
}

// --- real FS: symlink pointing outside the temp repo -----------------------

func TestWalkRealFSOutsideSymlinkNotDiscovered(t *testing.T) {
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "terragrunt.hcl"), nil, 0o644); err != nil {
		t.Fatalf("write outside terragrunt.hcl: %v", err)
	}

	repo := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(repo, "escape")); err != nil {
		t.Skipf("os.Symlink unsupported on this platform: %v", err)
	}

	root, err := os.OpenRoot(repo)
	if err != nil {
		t.Fatalf("os.OpenRoot(%q): %v", repo, err)
	}
	t.Cleanup(func() { root.Close() })

	got, err := discoverUnits(root.FS())
	if err != nil {
		t.Fatalf("discoverUnits: unexpected error: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("dirs = %v, want none (the escaping symlink must not be discovered)", dirs(t, got))
	}
}
