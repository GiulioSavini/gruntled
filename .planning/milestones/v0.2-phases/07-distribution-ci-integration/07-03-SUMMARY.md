---
phase: 07-distribution-ci-integration
plan: 03
subsystem: release
tags: [release, github-actions, REL-01, REL-02]
requires:
  - "07-01: --version prints 'gruntled <version> (<commit>)' from ldflags"
  - "07-02: scripts/build-release.sh <version> <commit> <outdir>, 6 targets"
provides:
  - ".github/workflows/release.yml: tag v* -> guard -> proofs -> build -> smoke -> single gh release create"
affects: [07-04 docs/ci.md can reference the release assets, 07-05 first real tag push]
tech-stack:
  added: []
  patterns: ["proof composition by re-running check-architecture.sh + go test inside release.yml (needs: cannot span workflows)"]
key-files:
  created:
    - .github/workflows/release.yml
  modified: []
decisions:
  - "release.yml re-runs check-architecture.sh and go test -count=1 on the tagged commit instead of depending on ci.yml (needs: cannot cross workflows)"
  - "Tag guard: ^v[0-9]+\\.[0-9]+\\.[0-9]+(-[0-9A-Za-z.-]+)?$ and merge-base --is-ancestor origin/master; any -suffix tag is published with --prerelease"
  - "Single publish step at the end; contents: write only on the release job; tag/sha read only via $GITHUB_* shell vars"
metrics:
  duration: 5min
  completed: 2026-10-01
  tasks: 1
  files: 1
---

# Phase 7 Plan 03: release workflow Summary

`.github/workflows/release.yml` is triggered by pushing a `v*` tag. It has one job, `release`, and that job is the only place with `contents: write`. The steps run in this order:

1. **guard:** the tag must match `vX.Y.Z[-suffix]` and its commit must be on `origin/master`.
2. **proofs:** runs `check-architecture.sh` and `go test -count=1 ./...` on the tagged commit.
3. **build:** `build-release.sh "$GITHUB_REF_NAME" <short=12 sha> $RUNNER_TEMP/dist`.
4. **smoke:** checks that `dist` holds exactly 7 files, extracts the linux_amd64 archive, and checks that `--version` starts with `gruntled <tag> (`.
5. **publish:** a single `gh release create --generate-notes --verify-tag`, adding `--prerelease` when the tag has a suffix.

checkout is pinned to v7.0.1 and setup-go to v7.0.0, each by full SHA with a version comment. Both SHAs were checked again today with `gh api`. No tag was pushed and the workflow has not been triggered; its first real run is planned for 07-05.

## Tasks

| # | Task | Commit |
|---|------|--------|
| 1 | release.yml written and statically validated | 9c66d7e |

## Verification

- actionlint v1.7.12 (pinned, run with `go run`, latest release per `gh api`) reports nothing on release.yml or ci.yml.
- Both `uses:` lines are pinned to a 40-hex SHA with a `# vX.Y.Z` comment. No `uses:` is pinned to a tag.
- `${{ }}` appears only in `env: GH_TOKEN` and the `concurrency` group, never inside a `run:` script.
- The guard regex accepts `v0.2.0` and `v0.2.0-rc.1`, and rejects `0.2.0`, `v1.2` and `vx`.
- `bash scripts/build-release.sh v0.0.0-dry abc1234 <tmp>` produced 7 files. I extracted the workflow's smoke script and ran it locally against that output. It passed and printed `gruntled v0.0.0-dry (abc1234)`.
- I echo-tested the publish argument logic: `v0.2.0` gets no flag and `v0.2.0-rc.1` gets `--prerelease`. An empty `pre` array is fine because the runner uses `bash -eo pipefail` without `-u`.
- I did not re-run check-architecture.sh or its self-test, because this plan does not touch them. They passed at 07-02.

## Deviations from Plan

**1. [Rule 2] Added a `concurrency: release-${{ github.ref }}` group with `cancel-in-progress: false`.** If the same tag is pushed twice, the two runs now queue instead of racing on `gh release create`. A run that is already going is never cancelled.

Otherwise the plan was executed as written.

## Requirements

I left REL-01 and REL-02 Pending. The workflow exists, but the requirement is a downloadable tagged release, and that only happens after the 07-05 tag push.

## Self-Check: PASSED
