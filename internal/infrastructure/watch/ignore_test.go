package watch

import (
	"io/fs"
	"testing"
)

func TestIgnoredEntry(t *testing.T) {
	ignored := []string{
		".git",
		".git/HEAD",
		"a/.terraform/b",
		".terragrunt-cache/x/y.hcl",
		"a/.terragrunt-cache",
		"main.tf.swp",
		".main.tf.swp",
		"x.swo",
		"a/x.swn",
		"a/x.swx",
		"terragrunt.hcl~",
		".#terragrunt.hcl",
		"#terragrunt.hcl#",
		"___jb_tmp___",
		"a/___jb_old___",
		"foo.tmp",
		"a/.DS_Store",
		"4913",
		"a/49134",
	}
	for _, p := range ignored {
		if !IgnoredEntry(p, 0) {
			t.Errorf("IgnoredEntry(%q, 0) = false, want true", p)
		}
	}
	kept := []string{
		"terragrunt.hcl",
		".terraform.lock.hcl",
		"a.hcl.json",
		"vendor/x.tf",
		"a/b/main.tf",
		".terraformrc",
		"491",
		"4913.hcl",
		"123456",
		"#notclosed",
		"a/.gitignore",
		"git/x",
	}
	for _, p := range kept {
		for _, typ := range []fs.FileMode{0, fs.ModeDir} {
			if IgnoredEntry(p, typ) {
				t.Errorf("IgnoredEntry(%q, %v) = true, want false", p, typ)
			}
		}
	}

	// Directories named like editor files are walked and watched.
	patternNamed := []string{"4913", "a/49134", "x.tmp", "bak~", "#d#", ".#d", ".DS_Store", "a.swp"}
	for _, p := range patternNamed {
		if IgnoredEntry(p, fs.ModeDir) {
			t.Errorf("IgnoredEntry(%q, ModeDir) = true, want false", p)
		}
	}
	// Symlinks are exempt from every pattern except the Emacs lock rule.
	for _, p := range patternNamed {
		if p == ".#d" {
			continue
		}
		if IgnoredEntry(p, fs.ModeSymlink) {
			t.Errorf("IgnoredEntry(%q, ModeSymlink) = true, want false", p)
		}
	}
	for _, p := range []string{".#d", ".#terragrunt.hcl", "a/.#x.hcl"} {
		if !IgnoredEntry(p, fs.ModeSymlink) {
			t.Errorf("IgnoredEntry(%q, ModeSymlink) = false, want true (Emacs lock)", p)
		}
		if IgnoredEntry(p, fs.ModeDir) {
			t.Errorf("IgnoredEntry(%q, ModeDir) = true, want false", p)
		}
	}
	// Ignored-directory components apply to every entry type.
	for _, p := range []string{".git", "a/.terraform", "a/.terragrunt-cache/x", ".git/HEAD"} {
		for _, typ := range []fs.FileMode{0, fs.ModeDir, fs.ModeSymlink} {
			if !IgnoredEntry(p, typ) {
				t.Errorf("IgnoredEntry(%q, %v) = false, want true", p, typ)
			}
		}
	}
	// A file inside a pattern-named directory is judged by its own name.
	for _, p := range []string{"live/2024/terragrunt.hcl", "x.tmp/f.hcl", "bak~/f.hcl", "#d#/f.hcl", ".#d/f.hcl"} {
		if IgnoredEntry(p, 0) {
			t.Errorf("IgnoredEntry(%q, 0) = true, want false", p)
		}
	}
}

func TestIgnoredDir(t *testing.T) {
	for _, n := range []string{".git", ".terraform", ".terragrunt-cache"} {
		if !IgnoredDir(n) {
			t.Errorf("IgnoredDir(%q) = false, want true", n)
		}
	}
	for _, n := range []string{"vendor", "modules", ".terraformrc", "git", ""} {
		if IgnoredDir(n) {
			t.Errorf("IgnoredDir(%q) = true, want false", n)
		}
	}
}
