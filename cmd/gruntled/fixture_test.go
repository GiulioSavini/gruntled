package main

import (
	"bytes"
	"testing"
)

// TestFixtureExitCodes pins the two hand-written fixtures the CI recipe and
// pre-commit proofs rely on: clean-fixture must stay clean (exit 0) and
// sarif-fixture must stay broken (exit 1).
func TestFixtureExitCodes(t *testing.T) {
	cases := []struct {
		dir      string
		want     int
		quietOut bool
	}{
		{"testdata/clean-fixture", exitOK, true},
		{"testdata/sarif-fixture", exitFindings, false},
	}
	for _, tc := range cases {
		t.Run(tc.dir, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			if got := run([]string{"check", tc.dir}, &stdout, &stderr); got != tc.want {
				t.Fatalf("exit %d, want %d\nstdout: %s\nstderr: %s", got, tc.want, stdout.String(), stderr.String())
			}
			if tc.quietOut && stdout.Len() != 0 {
				t.Errorf("stdout %q, want empty", stdout.String())
			}
		})
	}
}
