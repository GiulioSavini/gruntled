package main

import (
	"regexp"
	"runtime/debug"
	"strings"
)

// This file is the only source of the version string printed by --version,
// the SARIF tool.driver.version and the daemon's ipc.Info (INST-01).
//
// Precedence: the ldflags pair wins whenever version != "dev" (release
// archives). Otherwise the main module version from the embedded build info
// is accepted only when every one of these holds:
//
//   - Main.Path is this module (a fork at another path stays dev);
//   - it is a plain release tag, vX.Y.Z with an optional ASCII pre-release
//     suffix, at most maxVersionLen bytes (no "(devel)", no "+dirty",
//     no "+incompatible", nothing a shell or SARIF viewer could misread);
//   - it is not a pseudo-version (…-YYYYMMDDHHMMSS-abcdefabcdef);
//   - no build setting key is "vcs" or starts with "vcs.".
//
// The vcs.* gate is what separates the two ways a tag can appear in the
// build info: a local `go build` in a checkout at the tag is stamped with
// vcs.revision/vcs.time/vcs.modified, while the module-cache build made by
// `go install …/cmd/gruntled@vX.Y.Z` has no vcs settings at all. Only the
// second case is a release the user asked for by name, so only it prints
// the tag. In every other case the result is "dev", "none", so no
// pseudo-version, commit or dirty marker ever reaches the SARIF bytes of a
// dev build and check's output stays reproducible.
//
// On the build-info path the commit is the documented placeholder "none":
// the tag pins the commit through the module proxy, and only release
// archives carry the commit through -ldflags.

// modulePath is pinned to go.mod's module line by TestModulePathMatchesGoMod.
const modulePath = "github.com/GiulioSavini/gruntled"

// maxVersionLen bounds the build-info string that reaches SARIF and IPC;
// "v0.4.0" is 6 bytes, a long pre-release tag a few dozen.
const maxVersionLen = 64

// readBuildInfo is a var so tests can swap the reader (D-18-01).
var readBuildInfo = debug.ReadBuildInfo

var (
	// releaseVersionRE: vX.Y.Z with an optional pre-release suffix of
	// ASCII letters, digits, dots and hyphens. "(devel)" and anything
	// with "+" already fail here.
	releaseVersionRE = regexp.MustCompile(`^v[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.-]+)?$`)
	// pseudoVersionRE matches the three pseudo-version shapes Go emits:
	// vX.Y.Z-0.<ts>-<hash>, vX.Y.Z-pre.0.<ts>-<hash>, v0.0.0-<ts>-<hash>.
	pseudoVersionRE = regexp.MustCompile(`[.-][0-9]{14}-[0-9a-f]{12}$`)
)

// versionInfo returns the version and commit every output site prints.
func versionInfo() (version, commit string) {
	bi, ok := readBuildInfo()
	return resolveVersion(version, commit, bi, ok)
}

// resolveVersion is the pure decision behind versionInfo; see the file
// comment for the predicate. It reads bi and never modifies it.
func resolveVersion(ldVersion, ldCommit string, bi *debug.BuildInfo, ok bool) (string, string) {
	if ldVersion != "dev" {
		return ldVersion, ldCommit
	}
	if !ok || bi == nil || bi.Main.Path != modulePath {
		return "dev", "none"
	}
	v := bi.Main.Version
	if len(v) > maxVersionLen || !releaseVersionRE.MatchString(v) {
		return "dev", "none"
	}
	// The regex already rejects "+"; keep an explicit check as a second
	// line of defence against a future loosening of the pattern.
	if strings.Contains(v, "+") || pseudoVersionRE.MatchString(v) {
		return "dev", "none"
	}
	for _, s := range bi.Settings {
		// Go stamps "vcs=git" next to vcs.revision; treat the bare key as
		// a stamp too, so the gate can only get stricter, never looser.
		if s.Key == "vcs" || strings.HasPrefix(s.Key, "vcs.") {
			return "dev", "none"
		}
	}
	return v, "none"
}
