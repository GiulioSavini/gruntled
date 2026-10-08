package main

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"strings"
	"testing"
)

var wantReleaseTargets = []string{
	"linux/amd64", "linux/arm64",
	"darwin/amd64", "darwin/arm64",
	"windows/amd64", "windows/arm64",
}

var releaseTargetsLine = regexp.MustCompile(`(?m)^release_targets="([^"]*)"$`)

// readReleaseTargets returns the single release_targets="..." list in a
// script under scripts/.
func readReleaseTargets(t *testing.T, script string) []string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "..", "scripts", script))
	if err != nil {
		t.Fatal(err)
	}
	m := releaseTargetsLine.FindAllStringSubmatch(string(data), -1)
	if len(m) != 1 {
		t.Fatalf("%s: want exactly one release_targets=\"...\" line, found %d", script, len(m))
	}
	return strings.Fields(m[0][1])
}

// The no-net/no-exec proof and the release build must cover the same
// targets: a target built but not proven would ship unchecked.
func TestReleaseTargetsInSync(t *testing.T) {
	arch := readReleaseTargets(t, "check-architecture.sh")
	rel := readReleaseTargets(t, "build-release.sh")
	if !slices.Equal(arch, rel) {
		t.Errorf("release_targets differ:\ncheck-architecture.sh: %v\nbuild-release.sh:      %v", arch, rel)
	}
	if !slices.Equal(arch, wantReleaseTargets) {
		t.Errorf("release_targets = %v, want %v", arch, wantReleaseTargets)
	}
}

func TestBuildRelease(t *testing.T) {
	if testing.Short() {
		t.Skip("cross-builds 6 targets")
	}
	if runtime.GOOS == "windows" {
		// Releases are built on linux CI; the script needs bash and zip,
		// which windows runners do not provide.
		t.Skip("build-release.sh runs on linux only")
	}
	const version, commit = "v0.0.0-test", "abc1234"
	out := t.TempDir()
	cmd := exec.Command("bash", filepath.Join("..", "..", "scripts", "build-release.sh"), version, commit, out)
	if b, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build-release.sh: %v\n%s", err, b)
	}

	want := []string{"checksums.txt"}
	for _, target := range wantReleaseTargets {
		goos, goarch, _ := strings.Cut(target, "/")
		ext := ".tar.gz"
		if goos == "windows" {
			ext = ".zip"
		}
		want = append(want, "gruntled_"+version+"_"+goos+"_"+goarch+ext)
	}
	slices.Sort(want)

	entries, err := os.ReadDir(out)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, e := range entries {
		got = append(got, e.Name())
	}
	if !slices.Equal(got, want) {
		t.Fatalf("outdir files:\n got %v\nwant %v", got, want)
	}

	check := exec.Command("sha256sum", "-c", "checksums.txt")
	check.Dir = out
	if b, err := check.CombinedOutput(); err != nil {
		t.Fatalf("sha256sum -c: %v\n%s", err, b)
	}

	for _, name := range want {
		path := filepath.Join(out, name)
		switch {
		case strings.HasSuffix(name, ".zip"):
			if m := zipMembers(t, path); !slices.Equal(m, []string{"LICENSE", "gruntled.exe"}) {
				t.Errorf("%s members = %v, want [LICENSE gruntled.exe]", name, m)
			}
		case strings.HasSuffix(name, ".tar.gz"):
			if m := tarGzMembers(t, path); !slices.Equal(m, []string{"LICENSE", "gruntled"}) {
				t.Errorf("%s members = %v, want [LICENSE gruntled]", name, m)
			}
		}
	}
}

func zipMembers(t *testing.T, path string) []string {
	t.Helper()
	r, err := zip.OpenReader(path)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	var names []string
	for _, f := range r.File {
		names = append(names, f.Name)
	}
	slices.Sort(names)
	return names
}

func tarGzMembers(t *testing.T, path string) []string {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		t.Fatal(err)
	}
	tr := tar.NewReader(gz)
	var names []string
	for {
		h, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		names = append(names, h.Name)
	}
	slices.Sort(names)
	return names
}
