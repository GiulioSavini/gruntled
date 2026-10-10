package main

import (
	"bytes"
	"errors"
	"strings"
	"testing"
)

// TestWatchRunErrorSanitised covers the stderr line watch prints when
// watch.Run returns (today only an initial-index root walk error, sec #42).
// No existing seam makes watch.Run fail with attacker-shaped text (the
// walk error names "." relative to the repository, never a repo path), so
// the test drives the helper the line goes through: terminal controls and
// bidi/format runes must not reach the terminal.
func TestWatchRunErrorSanitised(t *testing.T) {
	var stderr bytes.Buffer
	err := errors.New("initial index: open evil\x1b]0;pwned\x07name\u202Etxt.hcl\u2028: permission denied")
	printRunError(&stderr, err)

	got := stderr.String()
	if !strings.HasPrefix(got, "gruntled: ") || !strings.HasSuffix(got, "\n") {
		t.Fatalf("stderr = %q, want one \"gruntled: ...\" line", got)
	}
	for _, bad := range []string{"\x1b", "\x07", "\u202E", "\u2028"} {
		if strings.Contains(got, bad) {
			t.Errorf("stderr %q contains %q", got, bad)
		}
	}
	if !strings.Contains(got, "permission denied") {
		t.Errorf("stderr %q lost the error text", got)
	}
}
