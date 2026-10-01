---
phase: 07-distribution-ci-integration
plan: 01
subsystem: cli
tags: [version, ldflags, sarif, fixture, REL-02]
requires: []
provides:
  - "gruntled --version / -version: `gruntled <version> (<commit>)` on stdout, exit 0"
  - "main.version / main.commit package vars injectable via -ldflags -X (defaults dev/none)"
  - "SARIF tool.driver.version = version var"
  - "cmd/gruntled/testdata/clean-fixture: 2-unit repo, check exits 0"
affects: [07-02 build-release smoke, 07-03 release.yml, 07-04 recipe-check and pre-commit proof]
tech-stack:
  added: []
  patterns: ["build-time -X injection into package-level string vars"]
key-files:
  created:
    - cmd/gruntled/fixture_test.go
    - cmd/gruntled/testdata/clean-fixture/live/network/terragrunt.hcl
    - cmd/gruntled/testdata/clean-fixture/live/app/terragrunt.hcl
    - cmd/gruntled/testdata/clean-fixture/modules/network/outputs.tf
  modified:
    - cmd/gruntled/main.go
    - cmd/gruntled/main_test.go
    - cmd/gruntled/testdata/script/usage.txtar
    - docs/cli.md
decisions:
  - "ReadBuildInfo fallback dropped: local `go build` stamps a VCS pseudo-version (v0.0.0-...+dirty), which would break `built without ldflags prints gruntled dev (none)`; go install @tag therefore prints dev (none)"
  - "--version listed under a Flags: block in topUsage; pinned in usage.txtar"
metrics:
  duration: 12min
  completed: 2026-10-01
  tasks: 2
  files: 8
---

# Phase 7 Plan 01: --version and clean fixture Summary

`gruntled --version` prints `gruntled <version> (<commit>)` from `-X main.version`/`-X main.commit` (defaults `dev`/`none`). SARIF `tool.driver.version` now uses the same var. Also adds a clean 2-unit fixture that exits 0, which later plans need.

## Tasks

| # | Task | Commit |
|---|------|--------|
| 1 (RED) | failing tests TestVersion, TestVersionSARIF, TestVersionLdflags | 0b7f48b |
| 1 (GREEN) | version/commit vars, `--version`/`-version` arm, `ToolInfo{Version: version}`, help line | 6f9d26a |
| 2 | clean-fixture, TestFixtureExitCodes (clean=0, sarif-fixture=1), docs/cli.md `--version` | 78b68c8 |

## Verification

- `go test -count=1 ./...` all green; `go vet ./cmd/gruntled` clean; `scripts/check-architecture.sh` OK.
- `go build ./cmd/gruntled && ./gruntled --version` prints `gruntled dev (none)`.
- TestVersionLdflags builds with `-X main.version=v9.9.9 -X main.commit=abc1234` and gets `gruntled v9.9.9 (abc1234)`.
- `go run ./cmd/gruntled check cmd/gruntled/testdata/clean-fixture` exits 0.
- The golden tests only glob `testdata/golden/*.txtar`, so clean-fixture is not picked up by them.

## Deviations from Plan

**1. [Plan-sanctioned] ReadBuildInfo fallback dropped**
- **Found during:** Task 1
- **Issue:** With Go 1.27, a plain `go build` inside the checkout stamps `info.Main.Version` with a VCS pseudo-version (`v0.0.0-20261001120257-0b7f48b50bd7+dirty`), not `(devel)`. The fallback would have printed that instead of `gruntled dev (none)`.
- **Fix:** Removed the fallback, as the plan says to do in this case. As a result, `go install ...@vX.Y.Z` prints `gruntled dev (none)`, and docs/cli.md says so. 07-04 (docs/ci.md recipe) should not promise a version string for `go install` builds.

**2. [Rule 3 - Blocking] `-race` not runnable locally**
- No C compiler in this environment (`gcc` missing, cgo needed by `-race`), so the suite ran without `-race`. CI still runs `-race`.

**3. [Rule 2] usage.txtar pins the new `--version` help line**, so topUsage and the test stay in sync.

## Self-Check: PASSED
