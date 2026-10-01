package main

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/rogpeppe/go-internal/testscript"
)

func TestMain(m *testing.M) {
	testscript.Main(m, map[string]func(){
		"gruntled":      func() { os.Exit(run(os.Args[1:], os.Stdout, os.Stderr)) },
		"gruntled-exit": gruntledExit,
	})
}

// gruntledExit runs gruntled with os.Args[2:] and exits 0 only when the exit
// code equals os.Args[1], so a script can assert the exact number.
func gruntledExit() {
	if len(os.Args) < 2 {
		os.Stderr.WriteString("gruntled-exit: missing wanted exit code\n")
		os.Exit(1)
	}
	want, err := strconv.Atoi(os.Args[1])
	if err != nil {
		os.Stderr.WriteString("gruntled-exit: bad wanted exit code " + strconv.Quote(os.Args[1]) + "\n")
		os.Exit(1)
	}
	got := run(os.Args[2:], os.Stdout, os.Stderr)
	if got != want {
		os.Stderr.WriteString("gruntled-exit: got exit " + strconv.Itoa(got) + ", want " + strconv.Itoa(want) + "\n")
		os.Exit(1)
	}
	os.Exit(0)
}

func TestScripts(t *testing.T) {
	testscript.Run(t, testscript.Params{Dir: "testdata/script", RequireExplicitExec: true})
}

func writeFiles(t *testing.T, dir string, files map[string]string) {
	t.Helper()
	for name, content := range files {
		p := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func repo(t *testing.T, outputRef string) string {
	t.Helper()
	dir := t.TempDir()
	writeFiles(t, dir, map[string]string{
		"vpc/terragrunt.hcl": "",
		"vpc/main.tf":        "output \"vpc_id\" { value = \"x\" }\n",
		"app/terragrunt.hcl": "dependency \"vpc\" {\n  config_path = \"../vpc\"\n}\ninputs = { id = dependency.vpc.outputs." + outputRef + " }\n",
		"app/main.tf":        "variable \"id\" {}\n",
	})
	return dir
}

func TestRunExitCodes(t *testing.T) {
	clean := repo(t, "vpc_id")
	broken := repo(t, "vpc_idd")
	file := filepath.Join(t.TempDir(), "file.txt")
	if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	missing := filepath.Join(t.TempDir(), "missing")

	tests := []struct {
		name string
		args []string
		want int
	}{
		{"no args", nil, 2},
		{"top help", []string{"-h"}, 0},
		{"help command", []string{"help"}, 0},
		{"unknown command", []string{"frobnicate"}, 2},
		{"check help", []string{"check", "-h"}, 0},
		{"unknown flag", []string{"check", "--bogus"}, 2},
		{"bad format before path", []string{"check", "--format", "yaml", "."}, 2},
		{"bad format after path", []string{"check", ".", "--format", "yaml"}, 2},
		{"two paths", []string{"check", "a", "b"}, 2},
		{"path then extra", []string{"check", ".", "extra"}, 2},
		{"missing dir", []string{"check", missing}, 3},
		{"regular file", []string{"check", file}, 3},
		{"clean repo", []string{"check", clean}, 0},
		{"broken ref", []string{"check", broken}, 1},
		{"clean repo json", []string{"check", clean, "--format", "json"}, 0},
		{"broken ref json", []string{"check", "--format=json", broken}, 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			if got := run(tt.args, &stdout, &stderr); got != tt.want {
				t.Fatalf("run(%q) = %d, want %d\nstdout:\n%s\nstderr:\n%s", tt.args, got, tt.want, stdout.String(), stderr.String())
			}
		})
	}
}

type failWriter struct{ calls int }

func (w *failWriter) Write(p []byte) (int, error) {
	w.calls++
	return 0, errors.New("write failed")
}

func TestRunStdoutWriteFailure(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{"text with a finding", []string{"check", repo(t, "vpc_idd")}},
		{"json on a clean repo", []string{"check", "--format", "json", repo(t, "vpc_id")}},
		{"sarif with a finding", []string{"check", "--format", "sarif", repo(t, "vpc_idd")}},
		{"graph --json on a clean repo", []string{"graph", "--json", repo(t, "vpc_id")}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var w failWriter
			var stderr bytes.Buffer
			if got := run(tt.args, &w, &stderr); got != 3 {
				t.Fatalf("run = %d, want 3; stderr:\n%s", got, stderr.String())
			}
			if w.calls == 0 {
				t.Fatal("stdout writer was never called; the test proves nothing")
			}
		})
	}
}
