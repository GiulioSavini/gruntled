package main

// TestReadmeDocumentsV03 keeps README.md in step with the v0.3 commands:
// watch, report and blast, the status file location and the Windows
// limitation. It is always on.

import (
	"os"
	"strings"
	"testing"
)

func TestReadmeDocumentsV03(t *testing.T) {
	b, err := os.ReadFile("../../README.md")
	if err != nil {
		t.Fatalf("read README.md: %v", err)
	}
	doc := string(b)

	for _, s := range []string{
		"gruntled watch",
		"gruntled report",
		"gruntled blast",
		"--print-status-path",
		"XDG_RUNTIME_DIR",
		"Windows",
	} {
		if !strings.Contains(doc, s) {
			t.Errorf("README.md does not mention %q", s)
		}
	}
	if strings.Contains(doc, "Not built yet") {
		t.Errorf("README.md still says %q", "Not built yet")
	}
}
