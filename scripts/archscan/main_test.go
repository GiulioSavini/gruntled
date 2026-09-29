package main

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

var hclRE = regexp.MustCompile(`^github\.com/(hashicorp|zclconf)/`)

func writeTree(t *testing.T, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	for name, body := range files {
		p := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func runScan(t *testing.T, files map[string]string, re *regexp.Regexp, excludes []string, skipTests bool) ([]string, string, error) {
	t.Helper()
	root := writeTree(t, files)
	var out, errOut bytes.Buffer
	err := scan(root, re, excludes, skipTests, &out, &errOut)
	var lines []string
	for l := range strings.SplitSeq(strings.TrimSpace(out.String()), "\n") {
		if l != "" {
			lines = append(lines, l)
		}
	}
	return lines, errOut.String(), err
}

func assertLines(t *testing.T, got, want []string) {
	t.Helper()
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("matches:\ngot:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

func TestScanMatches(t *testing.T) {
	cases := []struct {
		name string
		file string
		src  string
		want []string
	}{
		{
			name: "G21 comment before spec in block",
			file: "cmd/zz_windows.go",
			src:  "package main\n\nimport ( /* x */ _ \"github.com/hashicorp/hcl/v2\" )\n",
			want: []string{`./cmd/zz_windows.go:3: import "github.com/hashicorp/hcl/v2"`},
		},
		{
			name: "G21 semicolon-separated specs in block",
			file: "cmd/zz_windows.go",
			src:  "package main\n\nimport ( _ \"fmt\"; _ \"github.com/hashicorp/hcl/v2\" )\n",
			want: []string{`./cmd/zz_windows.go:3: import "github.com/hashicorp/hcl/v2"`},
		},
		{
			name: "single-line import",
			file: "a/a.go",
			src:  "package a\n\nimport \"github.com/zclconf/go-cty/cty\"\n\nvar _ = cty.String\n",
			want: []string{`./a/a.go:3: import "github.com/zclconf/go-cty/cty"`},
		},
		{
			name: "aliased, dot and blank imports in a block",
			file: "a/a.go",
			src:  "package a\n\nimport (\n\t\"fmt\"\n\th \"github.com/hashicorp/hcl/v2\"\n\t. \"github.com/zclconf/go-cty/cty\"\n\t_ \"github.com/hashicorp/x\"\n)\n",
			want: []string{
				`./a/a.go:5: import "github.com/hashicorp/hcl/v2"`,
				`./a/a.go:6: import "github.com/zclconf/go-cty/cty"`,
				`./a/a.go:7: import "github.com/hashicorp/x"`,
			},
		},
		{
			name: "import keyword and spec on different lines",
			file: "a/a.go",
			src:  "package a\n\nimport\n\th \"github.com/hashicorp/hcl/v2\"\n\nimport\n(\n\t_ \"github.com/zclconf/go-cty\"\n)\n",
			want: []string{
				`./a/a.go:4: import "github.com/hashicorp/hcl/v2"`,
				`./a/a.go:8: import "github.com/zclconf/go-cty"`,
			},
		},
		{
			name: "raw-string import path",
			file: "a/a.go",
			src:  "package a\n\nimport _ `github.com/hashicorp/hcl/v2`\n",
			want: []string{`./a/a.go:3: import "github.com/hashicorp/hcl/v2"`},
		},
		{
			name: "go:build ignore file is scanned",
			file: "a/a.go",
			src:  "//go:build ignore\n\npackage a\n\nimport _ \"github.com/hashicorp/hcl/v2\"\n",
			want: []string{`./a/a.go:5: import "github.com/hashicorp/hcl/v2"`},
		},
		{
			name: "windows-only file is scanned",
			file: "a/a_windows.go",
			src:  "package a\n\nimport _ \"github.com/hashicorp/hcl/v2\"\n",
			want: []string{`./a/a_windows.go:3: import "github.com/hashicorp/hcl/v2"`},
		},
		{
			name: "path only in a line comment",
			file: "a/a.go",
			src:  "package a\n\n// import _ \"github.com/hashicorp/hcl/v2\"\nimport _ \"fmt\"\n",
		},
		{
			name: "path only in a block comment outside the import decl",
			file: "a/a.go",
			src:  "package a\n\n/*\nimport _ \"github.com/hashicorp/hcl/v2\"\n*/\n\nimport _ \"fmt\"\n",
		},
		{
			name: "path only in a raw string inside a func body",
			file: "a/a.go",
			src:  "package a\n\nfunc f() string {\n\treturn `\nimport _ \"github.com/hashicorp/hcl/v2\"\n`\n}\n",
		},
		{
			name: "pattern is matched against the unquoted path, anchored",
			file: "a/a.go",
			src:  "package a\n\nimport _ \"example.com/github.com/hashicorp/hcl\"\n",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, errOut, err := runScan(t, map[string]string{tc.file: tc.src}, hclRE, nil, false)
			if err != nil {
				t.Fatalf("scan: %v (stderr %q)", err, errOut)
			}
			assertLines(t, got, tc.want)
		})
	}
}

func TestScanPrunesIgnoredDirectories(t *testing.T) {
	imp := "package a\n\nimport _ \"github.com/hashicorp/hcl/v2\"\n"
	got, errOut, err := runScan(t, map[string]string{
		"a/testdata/a.go":     imp,
		"vendor/x/a.go":       imp,
		"_x/a.go":             imp,
		".x/a.go":             imp,
		"a/b/.hidden/a.go":    imp,
		"a/b/_under/a.go":     imp,
		"a/nested/go.mod":     "module example.com/nested\n",
		"a/nested/a.go":       imp,
		"a/testdata/bad.go":   "not go at all",
		"a/.x/bad_import.go":  "package a\nimport (\n",
		"a/ordinary/plain.go": "package ordinary\n",
	}, hclRE, nil, false)
	if err != nil {
		t.Fatalf("scan: %v (stderr %q)", err, errOut)
	}
	// A nested go.mod does not hide its directory: single-module rejects it.
	assertLines(t, got, []string{`./a/nested/a.go:3: import "github.com/hashicorp/hcl/v2"`})
}

func TestScanExcludeAndSkipTests(t *testing.T) {
	imp := "package a\n\nimport _ \"github.com/hashicorp/hcl/v2\"\n"
	files := map[string]string{
		"internal/infrastructure/x/a.go":  imp,
		"internal/infrastructurex/a.go":   imp,
		"internal/domain/a.go":            imp,
		"internal/domain/a_test.go":       imp,
		"cmd/gruntled/main.go":            imp,
		"internal/infrastructure/root.go": imp,
	}
	got, errOut, err := runScan(t, files, hclRE, []string{"internal/infrastructure/", "cmd/"}, true)
	if err != nil {
		t.Fatalf("scan: %v (stderr %q)", err, errOut)
	}
	assertLines(t, got, []string{
		`./internal/domain/a.go:3: import "github.com/hashicorp/hcl/v2"`,
		`./internal/infrastructurex/a.go:3: import "github.com/hashicorp/hcl/v2"`,
	})

	got, errOut, err = runScan(t, files, hclRE, nil, false)
	if err != nil {
		t.Fatalf("scan: %v (stderr %q)", err, errOut)
	}
	if len(got) != len(files) {
		t.Fatalf("without -exclude/-skip-tests want %d matches, got %d:\n%s", len(files), len(got), strings.Join(got, "\n"))
	}
}

func TestScanParseErrorIsReported(t *testing.T) {
	got, errOut, err := runScan(t, map[string]string{
		"a/bad.go":  "package a\n\nimport (\n\t_ \"github.com/hashicorp/hcl/v2\"\n",
		"a/good.go": "package a\n\nimport _ \"github.com/zclconf/go-cty\"\n",
	}, hclRE, nil, false)
	if !errors.Is(err, errParse) {
		t.Fatalf("want errParse, got %v", err)
	}
	if !strings.Contains(errOut, "./a/bad.go") || !strings.Contains(errOut, "parse error") {
		t.Fatalf("stderr does not name the broken file: %q", errOut)
	}
	// The rest of the tree is still scanned.
	found := false
	for _, l := range got {
		if strings.HasPrefix(l, "./a/good.go:3:") {
			found = true
		}
	}
	if !found {
		t.Fatalf("good.go match missing after a parse error elsewhere: %v", got)
	}
}

func TestScanBodyErrorsAreIgnored(t *testing.T) {
	// ImportsOnly stops after the imports: a broken body is compile-gate's
	// business, not the scanner's.
	got, errOut, err := runScan(t, map[string]string{
		"a/a.go": "package a\n\nimport _ \"github.com/hashicorp/hcl/v2\"\n\nfunc {\n",
	}, hclRE, nil, false)
	if err != nil {
		t.Fatalf("scan: %v (stderr %q)", err, errOut)
	}
	assertLines(t, got, []string{`./a/a.go:3: import "github.com/hashicorp/hcl/v2"`})
}

func TestRunExitCodes(t *testing.T) {
	var out, errOut bytes.Buffer
	if rc := run(nil, &out, &errOut); rc != 2 {
		t.Fatalf("missing -match: rc %d, want 2", rc)
	}
	if rc := run([]string{"-match", "("}, &out, &errOut); rc != 2 {
		t.Fatalf("bad regexp: rc %d, want 2", rc)
	}
	if rc := run([]string{"-nope"}, &out, &errOut); rc != 2 {
		t.Fatalf("unknown flag: rc %d, want 2", rc)
	}
	if rc := run([]string{"-match", "x", "extra"}, &out, &errOut); rc != 2 {
		t.Fatalf("extra argument: rc %d, want 2", rc)
	}
}
