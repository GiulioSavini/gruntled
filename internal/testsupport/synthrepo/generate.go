package synthrepo

import (
	"fmt"
	"os"
	"path/filepath"
)

// Generate renders spec with Render and writes the resulting Tree into
// destDir. destDir must already exist, be a directory, and be empty;
// Generate performs every validation check before writing anything, and
// never removes or overwrites a file.
func Generate(spec Spec, destDir string) (Manifest, error) {
	tree, manifest, err := Render(spec)
	if err != nil {
		return Manifest{}, err
	}

	info, err := os.Stat(destDir)
	if err != nil {
		return Manifest{}, fmt.Errorf("synthrepo: destination %q: %w", destDir, err)
	}
	if !info.IsDir() {
		return Manifest{}, fmt.Errorf("synthrepo: destination %q is not a directory", destDir)
	}
	entries, err := os.ReadDir(destDir)
	if err != nil {
		return Manifest{}, fmt.Errorf("synthrepo: destination %q: %w", destDir, err)
	}
	if len(entries) > 0 {
		return Manifest{}, fmt.Errorf("synthrepo: destination %q is not empty", destDir)
	}

	for _, f := range tree {
		full := filepath.Join(destDir, filepath.FromSlash(f.Path))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			return Manifest{}, fmt.Errorf("synthrepo: writing %q: %w", f.Path, err)
		}
		if err := os.WriteFile(full, f.Content, 0o644); err != nil {
			return Manifest{}, fmt.Errorf("synthrepo: writing %q: %w", f.Path, err)
		}
	}

	return manifest, nil
}
