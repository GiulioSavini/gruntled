package watch

import "testing"

func TestIgnored(t *testing.T) {
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
		if !Ignored(p) {
			t.Errorf("Ignored(%q) = false, want true", p)
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
		if Ignored(p) {
			t.Errorf("Ignored(%q) = true, want false", p)
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
