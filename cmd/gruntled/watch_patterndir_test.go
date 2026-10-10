package main

import (
	"runtime"
	"testing"
)

// patternDirUnit is a consumer of ../vpc's output ref, depth levels below
// the repository root.
func patternDirUnit(up, outputRef string) string {
	return "dependency \"vpc\" {\n  config_path = \"" + up + "vpc\"\n}\ninputs = { id = dependency.vpc.outputs." + outputRef + " }\n"
}

// TestWatchPatternDirParity is the v0.3 audit BLOCKER end to end: units
// living in directories named like editor files (live/2024, x.tmp) must be
// reindexed by the daemon on both backends, so the status line converges to
// what a fresh `check` produces after every edit inside them.
func TestWatchPatternDirParity(t *testing.T) {
	consumers := []struct{ file, up string }{
		{"live/2024/app/terragrunt.hcl", "../../../"},
		{"x.tmp/app/terragrunt.hcl", "../../"},
	}
	for _, be := range []struct {
		name string
		args []string
	}{
		{"poll", []string{"--poll"}},
		{"native", nil},
	} {
		t.Run(be.name, func(t *testing.T) {
			if be.name == "native" && runtime.GOOS == "windows" {
				t.Skip("no native watcher on windows")
			}
			dir := t.TempDir()
			files := map[string]string{
				"vpc/terragrunt.hcl": "",
				"vpc/main.tf":        "output \"vpc_id\" { value = \"x\" }\n",
			}
			for _, c := range consumers {
				files[c.file] = patternDirUnit(c.up, "vpc_id")
			}
			writeFiles(t, dir, files)
			d := startWatch(t, dir, testDeps(t), be.args...)
			want := freshStatus(t, dir)
			d.waitStatus(want)
			for _, c := range consumers {
				for _, ref := range []string{"vpc_idd", "vpc_id"} {
					prev := want
					writeFiles(t, dir, map[string]string{c.file: patternDirUnit(c.up, ref)})
					want = freshStatus(t, dir)
					if want == prev {
						t.Fatalf("%s -> %s leaves the status at %q; the step proves nothing", c.file, ref, want)
					}
					d.waitStatus(want)
				}
			}
			if code := d.stop(); code != exitOK {
				t.Fatalf("exit %d; stderr:\n%s", code, d.stderr.String())
			}
		})
	}
}
