package main

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"strings"
	"testing"
)

// info builds the build info a go-install or checkout build of this module
// reports: Main at modulePath with mainVersion, plus key/value settings.
func info(mainVersion string, settings ...string) *debug.BuildInfo {
	bi := &debug.BuildInfo{Main: debug.Module{Path: modulePath, Version: mainVersion}}
	for i := 0; i+1 < len(settings); i += 2 {
		bi.Settings = append(bi.Settings, debug.BuildSetting{Key: settings[i], Value: settings[i+1]})
	}
	return bi
}

func TestResolveVersion(t *testing.T) {
	checkout := info("v0.3.1-0.20261016194242-18a1ba5e3a35",
		"vcs", "git", "vcs.revision", "18a1ba5e3a35a4fbac7061ecf77163f599de686d",
		"vcs.time", "2026-10-16T19:42:42Z", "vcs.modified", "false")
	cases := []struct {
		name         string
		ldV, ldC     string
		bi           *debug.BuildInfo
		ok           bool
		wantV, wantC string
	}{
		{"ldflags win", "v9.9.9", "abc1234", info("v0.4.0"), true, "v9.9.9", "abc1234"},
		{"ldflags win over devel", "v9.9.9", "abc1234", info("(devel)"), true, "v9.9.9", "abc1234"},
		{"ldflags win without build info", "v9.9.9", "abc1234", nil, false, "v9.9.9", "abc1234"},
		{"go install tag", "dev", "none", info("v0.4.0"), true, "v0.4.0", "none"},
		{"go install prerelease tag", "dev", "none", info("v0.5.0-rc.1"), true, "v0.5.0-rc.1", "none"},
		{"devel", "dev", "none", info("(devel)"), true, "dev", "none"},
		{"dirty", "dev", "none", info("v0.4.0+dirty"), true, "dev", "none"},
		{"pseudo after tag", "dev", "none", info("v0.3.1-0.20261016194242-18a1ba5e3a35"), true, "dev", "none"},
		{"pseudo prerelease", "dev", "none", info("v0.5.0-rc.1.0.20261016194242-18a1ba5e3a35"), true, "dev", "none"},
		{"pseudo no base", "dev", "none", info("v0.0.0-20261016194242-18a1ba5e3a35"), true, "dev", "none"},
		{"checkout build", "dev", "none", checkout, true, "dev", "none"},
		{"clean tag with vcs.revision", "dev", "none", info("v0.4.0", "vcs.revision", "18a1ba5e3a35a4fbac7061ecf77163f599de686d", "vcs.modified", "false"), true, "dev", "none"},
		{"clean tag with vcs.modified only", "dev", "none", info("v0.4.0", "vcs.modified", "true"), true, "dev", "none"},
		{"clean tag with vcs only", "dev", "none", info("v0.4.0", "vcs", "git"), true, "dev", "none"},
		{"clean tag with non-vcs settings", "dev", "none", info("v0.4.0", "GOOS", "linux", "-trimpath", "true"), true, "v0.4.0", "none"},
		{"incompatible", "dev", "none", info("v2.0.0+incompatible"), true, "dev", "none"},
		{"other module", "dev", "none", &debug.BuildInfo{Main: debug.Module{Path: "github.com/other/gruntled", Version: "v0.4.0"}}, true, "dev", "none"},
		{"empty main version", "dev", "none", info(""), true, "dev", "none"},
		{"no build info", "dev", "none", nil, false, "dev", "none"},
		{"nil with ok true", "dev", "none", nil, true, "dev", "none"},
		{"no leading v", "dev", "none", info("0.4.0"), true, "dev", "none"},
		{"trailing space", "dev", "none", info("v0.4.0 "), true, "dev", "none"},
		{"trailing newline", "dev", "none", info("v0.4.0\n"), true, "dev", "none"},
		{"two components", "dev", "none", info("v0.4"), true, "dev", "none"},
		{"overlong suffix", "dev", "none", info("v0.4.0-" + strings.Repeat("a", 64)), true, "dev", "none"},
		{"longest accepted", "dev", "none", info("v0.4.0-" + strings.Repeat("a", 57)), true, "v0.4.0-" + strings.Repeat("a", 57), "none"},
		{"control char in suffix", "dev", "none", info("v0.4.0-a\x01b"), true, "dev", "none"},
		{"non-ascii in suffix", "dev", "none", info("v0.4.0-é"), true, "dev", "none"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			v, c := resolveVersion(tc.ldV, tc.ldC, tc.bi, tc.ok)
			if v != tc.wantV || c != tc.wantC {
				t.Errorf("resolveVersion = %q, %q; want %q, %q", v, c, tc.wantV, tc.wantC)
			}
		})
	}
}

func TestModulePathMatchesGoMod(t *testing.T) {
	data, err := os.ReadFile("../../go.mod")
	if err != nil {
		t.Fatal(err)
	}
	first, _, _ := strings.Cut(string(data), "\n")
	if got := strings.TrimPrefix(first, "module "); got != modulePath {
		t.Errorf("go.mod module %q, modulePath %q", got, modulePath)
	}
}

func TestResolveVersionIsPure(t *testing.T) {
	bi := info("v0.4.0", "GOOS", "linux")
	v1, c1 := resolveVersion("dev", "none", bi, true)
	v2, c2 := resolveVersion("dev", "none", bi, true)
	if v1 != v2 || c1 != c2 {
		t.Errorf("second call = %q, %q; first %q, %q", v2, c2, v1, c1)
	}
	if bi.Main.Version != "v0.4.0" || len(bi.Settings) != 1 {
		t.Errorf("build info mutated: %+v", bi)
	}
}

// setBuildInfo swaps the build-info reader for one test.
func setBuildInfo(t *testing.T, bi *debug.BuildInfo, ok bool) {
	t.Helper()
	old := readBuildInfo
	readBuildInfo = func() (*debug.BuildInfo, bool) { return bi, ok }
	t.Cleanup(func() { readBuildInfo = old })
}

// sarifToolVersion decodes runs[0].tool.driver.version from a SARIF log.
func sarifToolVersion(t *testing.T, data []byte) string {
	t.Helper()
	var log struct {
		Runs []struct {
			Tool struct {
				Driver struct {
					Version string `json:"version"`
				} `json:"driver"`
			} `json:"tool"`
		} `json:"runs"`
	}
	if err := json.Unmarshal(data, &log); err != nil {
		t.Fatal(err)
	}
	if len(log.Runs) != 1 {
		t.Fatalf("%d runs, want 1", len(log.Runs))
	}
	return log.Runs[0].Tool.Driver.Version
}

// TestVersionInfoFeedsAllSites: with ldflags at dev and a clean go-install
// build info, --version, the SARIF tool version (through check and through
// renderReport) and the daemon's ipc.Info all carry the tag.
func TestVersionInfoFeedsAllSites(t *testing.T) {
	setVersion(t, "dev", "none")
	setBuildInfo(t, info("v7.7.7"), true)

	var stdout, stderr bytes.Buffer
	if code := run([]string{"--version"}, &stdout, &stderr); code != exitOK {
		t.Fatalf("exit %d, want %d", code, exitOK)
	}
	if got := stdout.String(); got != "gruntled v7.7.7 (none)\n" {
		t.Errorf("--version %q, want %q", got, "gruntled v7.7.7 (none)\n")
	}
	if stderr.Len() != 0 {
		t.Errorf("stderr %q, want empty", stderr.String())
	}

	stdout.Reset()
	stderr.Reset()
	if code := run([]string{"check", "--format", "sarif", "testdata/sarif-fixture"}, &stdout, &stderr); code != exitFindings {
		t.Fatalf("check exit %d, want %d; stderr %s", code, exitFindings, stderr.String())
	}
	if got := sarifToolVersion(t, stdout.Bytes()); got != "v7.7.7" {
		t.Errorf("check sarif tool.driver.version %q, want v7.7.7", got)
	}

	out, _, err := renderReport("sarif", freshReport(t, "testdata/clean-fixture"))
	if err != nil {
		t.Fatal(err)
	}
	if got := sarifToolVersion(t, out); got != "v7.7.7" {
		t.Errorf("renderReport sarif tool.driver.version %q, want v7.7.7", got)
	}

	if got := daemonInfo("r", "s"); got.Version != "v7.7.7" || got.Root != "r" || got.StatusPath != "s" || got.PID != os.Getpid() {
		t.Errorf("daemonInfo = %+v, want Version v7.7.7, Root r, StatusPath s, PID %d", got, os.Getpid())
	}
}

// TestVersionBuildInfoNoLeak is the golden-safety row: a checkout-style
// build info (pseudo-version plus vcs.revision) never reaches the output.
func TestVersionBuildInfoNoLeak(t *testing.T) {
	setVersion(t, "dev", "none")
	setBuildInfo(t, info("v0.3.1-0.20261016194242-18a1ba5e3a35",
		"vcs", "git", "vcs.revision", "18a1ba5e3a35a4fbac7061ecf77163f599de686d",
		"vcs.time", "2026-10-16T19:42:42Z", "vcs.modified", "true"), true)

	var stdout, stderr bytes.Buffer
	if code := run([]string{"--version"}, &stdout, &stderr); code != exitOK {
		t.Fatalf("exit %d, want %d", code, exitOK)
	}
	if got := stdout.String(); got != "gruntled dev (none)\n" {
		t.Errorf("--version %q, want %q", got, "gruntled dev (none)\n")
	}

	stdout.Reset()
	if code := run([]string{"check", "--format", "sarif", "testdata/sarif-fixture"}, &stdout, &stderr); code != exitFindings {
		t.Fatalf("check exit %d, want %d; stderr %s", code, exitFindings, stderr.String())
	}
	if got := sarifToolVersion(t, stdout.Bytes()); got != "dev" {
		t.Errorf("sarif tool.driver.version %q, want dev", got)
	}
	if got := daemonInfo("r", "s").Version; got != "dev" {
		t.Errorf("daemonInfo version %q, want dev", got)
	}
}

// TestVersionPlainBuild builds the real binary with no ldflags: the
// toolchain stamps a pseudo-version plus vcs.* (or "(devel)" without git),
// and both must print dev (none).
func TestVersionPlainBuild(t *testing.T) {
	if testing.Short() {
		t.Skip("builds the binary")
	}
	bin := filepath.Join(t.TempDir(), "g")
	if runtime.GOOS == "windows" {
		bin += ".exe"
	}
	build := exec.Command("go", "build", "-o", bin, ".")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("go build: %v\n%s", err, out)
	}
	out, err := exec.Command(bin, "--version").Output()
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if got, want := string(out), "gruntled dev (none)\n"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}
