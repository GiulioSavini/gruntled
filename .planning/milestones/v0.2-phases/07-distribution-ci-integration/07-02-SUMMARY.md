---
phase: 07-distribution-ci-integration
plan: 02
subsystem: release
tags: [release, cross-build, ci, REL-01, windows-arm64]
requires:
  - "07-01: main.version / main.commit ldflags vars, --version output"
provides:
  - "scripts/build-release.sh <version> <commit> <outdir>: 6 archives + checksums.txt, host --version smoke"
  - "6-target release list (adds windows/arm64), proven by binary-no-net-no-exec"
  - "TestReleaseTargetsInSync: check-architecture.sh and build-release.sh lists must match"
  - "ci.yml release-build step: full packaging on every push/PR"
affects: [07-03 release.yml should call build-release.sh, 07-04 docs/ci.md archive naming]
tech-stack:
  added: []
  patterns: ["single packaging script shared by CI dry run and release workflow"]
key-files:
  created:
    - scripts/build-release.sh
    - cmd/gruntled/release_test.go
  modified:
    - scripts/check-architecture.sh
    - scripts/test-check-architecture.sh
    - docs/cli.md
    - .planning/phases/07-distribution-ci-integration/07-CONTEXT.md
    - .github/workflows/ci.yml
    - README.md
    - .gitignore
decisions:
  - "USER DECISION 2026-10-01: windows/arm64 added as 6th release target, overriding the CONTEXT 5-target lock"
  - "The release target list lives in exactly two places (check-architecture.sh Step 9, build-release.sh); ci.yml has no copy; a Go test keeps them equal"
  - "Archive names keep the leading v: gruntled_<version>_<os>_<arch>.tar.gz|zip; members are the binary and LICENSE with no path prefix"
  - "build-release.sh fails (no silent fallback) if zip is missing or the host is not a release target"
metrics:
  duration: 19min
  completed: 2026-10-01
  tasks: 3
  files: 9
---

# Phase 7 Plan 02: 6-target release packaging Summary

`scripts/build-release.sh` builds static (`CGO_ENABLED=0`, `-trimpath`) binaries for six targets. windows/arm64 is new. Each binary goes into a `.tar.gz` (linux, darwin) or `.zip` (windows) together with LICENSE. The script then writes `checksums.txt`, checks it with `sha256sum -c`, and runs the host binary's `--version` to confirm the ldflags injection worked. ci.yml now runs this script on every push and PR. The no-net/no-exec proof covers all six targets, and a self-test case checks that windows/arm64 is included.

## Tasks

| # | Task | Commit |
|---|------|--------|
| 1 | windows/arm64 in check-architecture.sh, self-test `binary-exec-windows-arm64-file`, docs/cli.md, CONTEXT decision note | 382b29a |
| 2 (RED) | TestReleaseTargetsInSync, TestBuildRelease | dfb39b8 |
| 2 (GREEN) | scripts/build-release.sh | c0b31c8 |
| 3 | ci.yml `release-build` step replaces `cross-build` loop; README targets + local command; /dist/ ignored | 98cc662 |

## Verification

- `go test -count=1 ./...` passes. TestBuildRelease takes about 60s locally because it does the full 6-target build.
- `bash scripts/build-release.sh v0.0.0-ci abc1234 $T` produced 7 files and all six checksums came back OK. The smoke run printed `gruntled v0.0.0-ci (abc1234)`.
- `bash scripts/test-check-architecture.sh` exits 0: all 57 cases pass, including the new `binary-exec-windows-arm64-file` (about 12 min locally).
- `bash scripts/check-architecture.sh` passes. As a manual check, I put a `zz_probe_windows_arm64.go` importing os/exec into the real tree and ran it: it failed with exactly one finding, `windows/arm64: package os/exec`.
- actionlint (v1.7.12, fetched via go run) is clean on ci.yml.
- `zip` is installed locally (/usr/bin/zip), so the windows archives were built here as well.
- I grepped for `windows/amd64` to look for old 5-target lists. The only hits outside phases 01-06 are the 07-RESEARCH.md / 07-DISCUSSION-LOG.md / 07-02-PLAN.md planning records, which I left unchanged.

## Deviations from Plan

**1. [Rule 2] `/dist/` added to .gitignore** (commit 98cc662). The README's local command writes to `dist/`, which should not be committed by mistake.

**2. [Rule 3] `-race` not runnable locally.** There is no C compiler in this environment, so the suite ran without `-race`. CI still runs it.

**3. README CI bullet mentions the SARIF schema step.** I rewrote that sentence anyway to add release-build, so I also named the step it had been leaving out.

## Self-Check: PASSED

## Requirements

I left REL-01 **Pending** in REQUIREMENTS.md on purpose. The packaging half is done here, but the requirement is about downloading from a tagged GitHub release, and that only exists after 07-03 (release.yml) and 07-05 (the tag push). Both of those plans also list REL-01.
