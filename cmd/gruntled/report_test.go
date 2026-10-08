package main

import (
	"strings"
	"testing"
)

// TestReportUsage: -h exits 0 with the usage text; bad flags, an invalid
// --format and two paths exit 2 with it; help lists report.
func TestReportUsage(t *testing.T) {
	_, stderr, code := runCLI(t, "report", "-h")
	if code != exitOK || !strings.HasPrefix(stderr, "usage: gruntled report ") {
		t.Fatalf("report -h: exit %d, stderr:\n%s", code, stderr)
	}
	for _, args := range [][]string{
		{"report", "--bogus"},
		{"report", "--format", "xml", "."},
		{"report", "a", "b"},
	} {
		stdout, stderr, code := runCLI(t, args...)
		if code != exitUsage || stdout != "" || !strings.Contains(stderr, "usage: gruntled report ") {
			t.Errorf("%v: exit %d, stdout %q, stderr:\n%s", args, code, stdout, stderr)
		}
	}
	_, stderr, _ = runCLI(t, "help")
	if !strings.Contains(stderr, "\n  report  ") || !strings.Contains(stderr, `"gruntled report -h"`) {
		t.Errorf("top usage does not list report:\n%s", stderr)
	}
}
