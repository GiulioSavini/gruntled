package main

import (
	"strings"
	"testing"
)

// TestDaemonDocs pins the claims about the daemon's baseline, rebase and
// the blast view in docs/cli.md and README.md: the old "never changes the
// daemon's state" sentence is gone, and the trust boundary, the windows
// gap, the cap, the label and the member are written down.
func TestDaemonDocs(t *testing.T) {
	doc := readRepoFile(t, "docs/cli.md")
	readme := readRepoFile(t, "README.md")
	if strings.Contains(doc, "The query never changes the daemon's state") {
		t.Error("docs/cli.md still says the query never changes the daemon's state")
	}
	for _, s := range []string{
		"rebase is the only operation that changes the daemon, and only its in-memory baseline",
		"only the daemon owner's user can connect",
		"rebase is not available on windows",
		"report never carries the blast view",
		"baseline: daemon generation 1 (watch start), current generation 1",
		`"daemon_baseline"`,
		"after a rebase, a reference that still names a removed output leaves Broken, while report keeps its GRT001",
		"gruntled: baseline moved to generation N; M error findings in Broken accepted",
		"gruntled: no usable answer from the daemon; the baseline may or may not have moved; run gruntled report --blast to check",
		"## gruntled rebase",
	} {
		if !strings.Contains(doc, s) {
			t.Errorf("docs/cli.md lacks %q", s)
		}
	}
	for _, s := range []string{"report --blast", "gruntled rebase"} {
		if !strings.Contains(readme, s) {
			t.Errorf("README.md lacks %q", s)
		}
	}
}
