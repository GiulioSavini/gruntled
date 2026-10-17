---
phase: 18-truth-foundations
plan: 01
subsystem: cli
tags: [version, build-info, sarif, ipc]
requires: []
provides:
  - "resolveVersion(ldVersion, ldCommit, bi, ok): pure decision between ldflags and clean go-install build info"
  - "versionInfo() used by --version, SARIF tool.driver.version and ipc.Info"
  - "setVersion/setBuildInfo test seams (readBuildInfo var)"
affects: [18-02, 18-03, 23-install-channels]
tech-stack:
  added: []
  patterns: ["pure resolver with a table test, build info read behind a swappable var"]
key-files:
  created:
    - cmd/gruntled/version.go
    - cmd/gruntled/version_test.go
  modified:
    - cmd/gruntled/main.go
    - cmd/gruntled/main_test.go
    - cmd/gruntled/render.go
    - cmd/gruntled/instance.go
key-decisions:
  - "ldflags always win; only when version is still \"dev\" is build info consulted"
  - "A build-info version is accepted only if Main.Path is the module path, it matches the release regexp (vX.Y.Z[-suffix], ASCII, at most 64 bytes), is not a pseudo-version, has no `+`, and no build setting is `vcs` or starts with `vcs.`; the commit is then always the literal `none`"
  - "Bare `vcs` settings key also rejects (deviation, bus #497): the gate can only get stricter"
requirements-completed: [INST-01]
duration: n/a
completed: 2026-10-16
---

# Phase 18 Plan 01: One resolved version string Summary

**`gruntled --version`, SARIF `tool.driver.version` and the daemon's `ipc.Info` now all come from `versionInfo()`. ldflags win (release archives: `gruntled v0.4.0 (<12-hex>)`); a clean `go install ...@vX.Y.Z` prints `gruntled vX.Y.Z (none)`; a plain `go build`, a checkout build (pseudo-version, `+dirty`, any VCS stamp) and anything odd prints `gruntled dev (none)`. Nothing from the build info other than a vetted `Main.Version` is ever formatted, so SARIF bytes stay deterministic.**

## Accomplishments

- **cmd/gruntled/version.go:** `versionInfo()` (:59) reads `readBuildInfo()` and calls the pure `resolveVersion` (:66). Predicate: ldflags version != `dev` returns ldflags (:67); no build info or foreign module path -> `dev`/`none` (:70); length cap and release regexp (:74); explicit `+` and pseudo-version rejection (:79); `vcs` / `vcs.*` setting -> `dev` (:85); otherwise `(Main.Version, "none")`.
- **Call sites (36ad782):** main.go:154 (`--version`), render.go:24 (SARIF), instance.go:227 (`ipc.Info`) call `versionInfo()`; main.go:45 comments that output sites never read `version`/`commit` directly.
- **Tests:** `TestResolveVersion` (version_test.go:25, 28 rows incl. ldflags, go-install tag and prerelease, devel, dirty, three pseudo shapes, checkout, `vcs.revision`, `vcs.modified`, bare `vcs`, non-vcs settings, incompatible, other module, empty, nil, no leading v, trailing space/newline, two components, 64-byte suffix rejected, 57-byte suffix accepted, control char, non-ASCII), `TestModulePathMatchesGoMod` (:75), `TestResolveVersionIsPure` (:86), `TestVersionInfoFeedsAllSites` (:130), `TestVersionBuildInfoNoLeak` (:169), `TestVersionPlainBuild` (:198, real toolchain).

## Task Commits

1. `e466ed9` test(18-01): add failing table for resolveVersion
2. `d5e4ae6` feat(18-01): resolve version from ldflags or clean go-install build info (version.go)
3. `a0aaa64` test(18-01): pin versionInfo at all three sites
4. `36ad782` feat(18-01): print the resolved version in --version, SARIF and ipc.Info

## Test evidence (master at 636d601)

```
go vet ./...                                   ok
go test -count=1 -short ./cmd/gruntled/        ok  github.com/GiulioSavini/gruntled/cmd/gruntled  20.492s
go list -deps ./cmd/gruntled | grep -E '^net$|os/exec|text/template'    (empty)
bash scripts/check-architecture.sh             architecture: OK (4 domain packages, 5 application packages, 1 interfaces packages)
```

`go test -race` could not run locally (no gcc/cgo); CI's race job covers it. The `readBuildInfo`/`version` swaps are safe because cmd/gruntled has no `t.Parallel` (sec #511).

## Deviations

- **Bare `vcs` key rejected too (bus #497).** The plan's gate was `vcs.*`; Go also emits a `vcs=git` setting, so the bare key is rejected as well (row "clean tag with vcs only"). Strictly stricter.
- **Sec plan-review #486 applied:** length cap (`maxVersionLen` 64, rows "overlong suffix"/"longest accepted") and control-char / non-ASCII rows.

## Threat mitigations

| ID | Disposition | Where |
|----|-------------|-------|
| T-18-01 foreign module path | mitigate | version.go:70 `bi.Main.Path != modulePath`; test row "other module"; TestModulePathMatchesGoMod version_test.go:75 |
| T-18-02 pseudo/dirty/VCS leak into SARIF | mitigate | version.go:74-90 (regexp, `+`, pseudo, `vcs`); TestVersionBuildInfoNoLeak version_test.go:169; goldens untouched |
| T-18-03 revision/time/path disclosure | mitigate | version.go:90 returns only `Main.Version` and literal `none`; TestResolveVersion rows with settings |
| T-18-06 net/exec in the binary | mitigate | imports only `runtime/debug`, `regexp`, `strings`; `go list -deps` grep empty; check-architecture.sh |
| T-18-07 vacuity of devel rows | mitigate | setVersion pins `readBuildInfo` to `(nil,false)`; NoLeak feeds explicit info; TestVersionPlainBuild uses the real toolchain (version_test.go:198) |
| T-18-04, -05, -08 | accept | as planned (T-18-08 transfers to Phase 23 `verify-published`) |

## Open items

- The go-install path against the real module proxy is only verifiable after the `v0.4.0` tag exists (Phase 23 INST-07).
