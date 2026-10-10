package statusfile

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// TestInsideBySameFile simulates a case-insensitive filesystem on any host:
// the statFn seam reports the root's own FileInfo for a second spelling of
// the root, so only an identity comparison can see that a path under that
// spelling is inside.
func TestInsideBySameFile(t *testing.T) {
	parent, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(parent, "repo")
	variant := filepath.Join(parent, "Repo")
	if err := os.Mkdir(root, 0o755); err != nil {
		t.Fatal(err)
	}
	rootFI, err := os.Stat(root)
	if err != nil {
		t.Fatal(err)
	}
	orig := statFn
	t.Cleanup(func() { statFn = orig })
	statFn = func(p string) (os.FileInfo, error) {
		if p == variant {
			return rootFI, nil
		}
		return os.Stat(p)
	}

	cases := []struct {
		name string
		p    string
		want bool
	}{
		{"case variant of root", filepath.Join(variant, "st", "status"), true},
		{"case variant itself", variant, true},
		{"sibling of variant", filepath.Join(parent, "Repo2", "status"), false},
		{"parent", parent, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := Inside(root, tc.p)
			if err != nil {
				t.Fatalf("Inside: %v", err)
			}
			if got != tc.want {
				t.Fatalf("Inside(%q, %q) = %v, want %v", root, tc.p, got, tc.want)
			}
		})
	}
}

// TestInsideSkipsUnreadableAncestor: a stat error other than not-exist on
// an ancestor is skipped, never returned, and a deeper match still wins.
func TestInsideSkipsUnreadableAncestor(t *testing.T) {
	parent, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(parent, "repo")
	variant := filepath.Join(parent, "Repo")
	if err := os.Mkdir(root, 0o755); err != nil {
		t.Fatal(err)
	}
	rootFI, err := os.Stat(root)
	if err != nil {
		t.Fatal(err)
	}
	orig := statFn
	t.Cleanup(func() { statFn = orig })
	statFn = func(p string) (os.FileInfo, error) {
		switch p {
		case variant:
			return rootFI, nil
		case filepath.Join(variant, "st"):
			return nil, errors.New("permission denied")
		}
		return os.Stat(p)
	}
	got, err := Inside(root, filepath.Join(variant, "st", "status"))
	if err != nil {
		t.Fatalf("Inside: %v", err)
	}
	if !got {
		t.Fatal("variant path under an unreadable ancestor must still count as inside")
	}
}
