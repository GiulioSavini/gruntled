//go:build unix

package terragrunt

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/GiulioSavini/gruntled/internal/domain/repograph"
)

func symlinkOrSkip(t *testing.T, target, link string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(link), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("os.Symlink unsupported: %v", err)
	}
}

func TestTargetStateRealFS(t *testing.T) {
	outside := t.TempDir()
	writeTree(t, outside, map[string]string{"terragrunt.hcl": ""})
	dir := t.TempDir()
	writeTree(t, dir, map[string]string{
		"real/terragrunt.hcl": "",
		"linked/keep.tf":      "",
		"dangling/keep.tf":    "",
		"escape/keep.tf":      "",
	})
	symlinkOrSkip(t, "missing.hcl", filepath.Join(dir, "dangling", "terragrunt.hcl"))
	symlinkOrSkip(t, "../real/terragrunt.hcl", filepath.Join(dir, "linked", "terragrunt.hcl"))
	symlinkOrSkip(t, filepath.Join(outside, "terragrunt.hcl"), filepath.Join(dir, "escape", "terragrunt.hcl"))
	symlinkOrSkip(t, outside, filepath.Join(dir, "escapedir"))

	l := NewLoader(openRootFS(t, dir))
	tests := []struct {
		dir  string
		want repograph.TargetState
	}{
		{"real", repograph.TargetHasConfig},
		{"linked", repograph.TargetHasConfig},
		{"dangling", repograph.TargetNoConfig},
		{"escape", repograph.TargetUnknown},
		{"escapedir", repograph.TargetUnknown},
		{"nope", repograph.TargetDirMissing},
	}
	for _, tt := range tests {
		t.Run(tt.dir, func(t *testing.T) {
			if got := l.classifyTarget(tt.dir); got != tt.want {
				t.Errorf("classifyTarget(%q) = %v, want %v", tt.dir, got, tt.want)
			}
		})
	}
}

func TestTargetStateUnreadableDir(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root ignores directory permissions")
	}
	dir := t.TempDir()
	writeTree(t, dir, map[string]string{"locked/terragrunt.hcl": ""})
	locked := filepath.Join(dir, "locked")
	if err := os.Chmod(locked, 0o000); err != nil {
		t.Fatalf("Chmod: %v", err)
	}
	t.Cleanup(func() { os.Chmod(locked, 0o755) })

	l := NewLoader(openRootFS(t, dir))
	if got := l.classifyTarget("locked"); got != repograph.TargetUnknown {
		t.Errorf("classifyTarget(locked) = %v, want %v", got, repograph.TargetUnknown)
	}
}
