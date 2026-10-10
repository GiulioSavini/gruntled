---
phase: 07-distribution-ci-integration
plan: 05
subsystem: release
tags: [release, github-actions, ci, REL-01, REL-02, INT-03, INT-04]
requires:
  - "07-03: release.yml (tag-triggered, 6 archives + checksums.txt)"
  - "07-04: recipe-check job, pre-commit hook, docs/ci.md"
provides:
  - "master pushed (f12885d..27cc603), ci.yml green on 27cc603: check, architecture, recipe-check, sarif-upload"
  - "Annotated tag v0.2.0 on 27cc603 (tagger Giulio Savini) and public GitHub release v0.2.0"
affects: [v0.2 milestone close]
tech-stack:
  added: []
  patterns: ["push gated on explicit user approval; tag only after master CI green on the same SHA"]
key-files:
  created: []
  modified: []
decisions:
  - "Tag v0.2.0 (user choice push-and-tag-v0.2.0, 2026-10-01), placed on the CI-green SHA 27cc603"
metrics:
  duration: 15min
  completed: 2026-10-01
  tasks: 3
  files: 0
---

# Phase 7 Plan 05: First push and v0.2.0 release Summary

The pipeline ran for real. master was pushed and ci.yml went green. Tag v0.2.0 then published a GitHub release with 6 archives and a checksums file. All checksums verify, and the downloaded binary reports the tag and commit.

## Evidence

- **Master push:** `f12885d..27cc603 master -> master`, with user approval.
- **CI run:** https://github.com/GiulioSavini/gruntled/actions/runs/36864852598 finished with conclusion success. All jobs passed: check, architecture, recipe-check, sarif-upload.
- **Tag:** annotated `v0.2.0` on object `27cc60399795f088cb21dfe04b9b9f18cb7935e7`. Tagger is Giulio Savini <giuliosavini@proton.me>. Pushed with user approval.
- **Release run:** https://github.com/GiulioSavini/gruntled/actions/runs/36865113237 finished with conclusion success.
- **Release:** https://github.com/GiulioSavini/gruntled/releases/tag/v0.2.0 is not a draft and not a prerelease. It has 7 assets:
  - checksums.txt
  - gruntled_v0.2.0_{darwin,linux}_{amd64,arm64}.tar.gz (4 files)
  - gruntled_v0.2.0_windows_{amd64,arm64}.zip (2 files)
- **Checksums:** `sha256sum -c checksums.txt` reports OK for all 6 archives.
- **Binary:** the linux_amd64 archive contains `gruntled` and `LICENSE`. `gruntled --version` prints `gruntled v0.2.0 (27cc60399795)`, which matches `git rev-parse --short=12 v0.2.0^{commit}`.
- **Windows archive:** `unzip -l` on the windows_amd64 zip lists `gruntled.exe` and `LICENSE`.

## Tasks

| Task | Name | Commit |
|------|------|--------|
| 1 | Pre-flight (local gates) | none (checks only) |
| 2 | Checkpoint: user approval | none (approved push-and-tag-v0.2.0) |
| 3 | Push, CI, tag, release verification | none (push/tag only) |

## Deviations from Plan

None. The plan was executed as written. Nothing was pushed or tagged without explicit approval, and no tag was deleted.

## Self-Check: PASSED
- Release v0.2.0 exists with 7 assets, and its checksums verify.
- Tag v0.2.0 points at 27cc603. The CI run on that SHA succeeded.
