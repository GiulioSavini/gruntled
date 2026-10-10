---
phase: 11-report-single-instance-release-proof
plan: 06
subsystem: docs, phase-gate
tags: [readme, docs, guard-test, release-proof, validation]
requires:
  - gruntled report + docs/cli.md report section (11-04)
  - Step 9 self-tests with ipc, CI test-os matrix (11-05)
provides:
  - README sections for watch, report, blast, status path, Windows limitation
  - cmd/gruntled/readme_doc_test.go (README keyword guard)
  - filled 11-VALIDATION.md (nyquist_compliant)
affects: [11-07]
tech-stack:
  added: []
  patterns: [doc guard test reading README.md like validation_doc_test.go]
key-files:
  created:
    - cmd/gruntled/readme_doc_test.go
  modified:
    - README.md
    - docs/cli.md
    - cmd/gruntled/watch.go
    - .planning/phases/11-report-single-instance-release-proof/11-VALIDATION.md
    - .planning/ROADMAP.md
    - .planning/STATE.md
key-decisions:
  - "README keeps GRT004-GRT006 under a truthful 'Later' line; watch/report/blast listed as new in v0.3"
  - "watch -h exit 0 now names 'already watching' (leftover from 11-03); docs/cli.md copies updated so TestHelpMatchesDocs holds"
  - "docs/cli.md Guarantees: runtime dir holds status, lock, socket (linux/darwin), report file (windows), all outside the repo; socket via syscall, not net"
  - "staticcheck@v0.8.1 cannot load the go1.27 module locally (tool built with go1.26): CI-only"
requirements-completed: [DAEMON-04, DAEMON-05, DAEMON-06]
metrics:
  duration: 14min
  completed: 2026-10-08
  tasks: 3
  files: 7
---

# Phase 11 Plan 06: README, guard test and phase gate Summary

README now documents `gruntled watch`, `gruntled report` and `gruntled blast`, the status file path (`<base>/gruntled/<hash>/status`, base `$XDG_RUNTIME_DIR` -> user cache dir -> temp dir, `--print-status-path`) and the Windows limitation (polling only, report file, no live query). A guard test pins this. The local phase gate is green and the six release targets build with ipc linked in.

## Tasks

| # | Task | Commit |
|---|------|--------|
| 1 | README/docs for watch, report, blast + README guard test (TDD) | 1eb08b8 (RED), f24a71c (GREEN) |
| 2 | Phase gate | no commit (no fixes needed) |
| 3 | VALIDATION + ROADMAP/REQUIREMENTS/STATE bookkeeping | final docs commit |

## Gate results (final tree, local linux/amd64)

| Command | Result |
|---------|--------|
| `gofmt -l .` | empty |
| `go vet ./...`, `GOOS=windows go vet ./...`, `GOOS=darwin go vet ./...` | ok |
| `go mod tidy -diff` | clean |
| `go test -count=1 ./...` | all ok (no `-race` locally, per orchestrator constraint; CI ubuntu runs `-race`) |
| `bash scripts/check-architecture.sh` | `architecture: OK` (incl. Step 9 per target) |
| `bash scripts/test-check-architecture.sh` | not run locally (orchestrator constraint); green in CI run 37780137349 |
| `scripts/build-release.sh v0.0.0-local <sha> <tmp>` | six archives OK, checksums, smoke `gruntled v0.0.0-local (f24a71cbab12)`; nothing tagged |
| `go list -deps ./cmd/gruntled` per linux/darwin/windows | ipc, watch, statusfile linked; no `net`, no `os/exec` |
| staticcheck v0.8.1 | CI-only: local tool built with go1.26 cannot load the go1.27 module |

CI run 37780137349: check, architecture and test-os macos-latest green; test-os windows-latest red on pre-existing tests never run on windows before (CRLF, `.exe`, bash, symlink, a watch test hang). Pending plan 11-07.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] watch -h omitted the "already watching" exit 0**
- **Found during:** Task 1 (noted as 11-03 leftover)
- **Fix:** exit 0 line in `watch` usage and both copies in docs/cli.md now say "already watching (another daemon runs for path)"
- **Files modified:** cmd/gruntled/watch.go, docs/cli.md
- **Commit:** f24a71c

**2. [Rule 1 - Bug] docs/cli.md claimed the status file is the only file watch writes**
- **Fix:** Guarantees bullet lists status, lock, socket (linux/darwin) and report file (windows), all in the runtime dir outside the repo
- **Commit:** f24a71c

### Scope adjustments (orchestrator constraints)

- `-race` and `scripts/test-check-architecture.sh` not run locally; both run in CI.
- REQUIREMENTS.md already had DAEMON-04/05/06 Complete from 11-03..11-05; left unchanged. DAEMON-06 windows runtime confirmation pending 11-07.
- `state advance-plan` could not parse STATE.md; position updated by hand. ROADMAP row restored by hand after `roadmap update-plan-progress`.

## Self-Check: PASSED
