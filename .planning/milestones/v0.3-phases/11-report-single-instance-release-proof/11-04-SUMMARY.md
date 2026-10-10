---
phase: 11-report-single-instance-release-proof
plan: 04
subsystem: cmd/gruntled (report command)
tags: [report, ipc, parity, read-only, windows-dump]
requires:
  - ipc.Query / Probe / ReadDump / VersionError / ErrNoDaemon (11-02)
  - statusfile.Dir / CheckDir (11-01)
  - buildSnapshot, acquireInstance, testDeps(t), startWatch, eventually (11-03)
provides:
  - gruntled report [--format text|json|sarif] [path] (runReport / runReportWith / reportUsage)
  - openRepoRoot shared root canonicalisation for watch and report
  - docs/cli.md report usage, exit codes and "## gruntled report" section
affects: [11-06 README/docs, 11-07 CI confirmation]
tech-stack:
  added: []
  patterns: [reportDeps env/goos seam, lock-probed dump read, check-vs-report byte parity tests on both transports]
key-files:
  created:
    - cmd/gruntled/report.go
    - cmd/gruntled/report_test.go
    - cmd/gruntled/testdata/script/report_nodaemon.txtar
    - cmd/gruntled/testdata/script/report_usage.txtar
  modified:
    - cmd/gruntled/main.go
    - cmd/gruntled/watch.go
    - cmd/gruntled/instance.go
    - cmd/gruntled/e2e_test.go
    - docs/cli.md
decisions:
  - "report checks both <base>/gruntled and <base>/gruntled/<hash12> with CheckDir (absent => no daemon, insecure => exit 3), mirroring EnsureRepoDir without creating anything"
  - "Socket Query returning ErrUnsupported (non linux/darwin unix) falls back to the lock-probed report file, matching the daemon's fallback"
  - "Lock held but report file not yet written => 'still indexing' (exit 3), not an error"
  - "Failed-reindex warning goes to stderr after the output/summary; LastError re-sanitised, empty => 'unknown error'"
requirements-completed: [DAEMON-04]
metrics:
  duration: 15min
  completed: 2026-10-08
  tasks: 2
  files: 9
---

# Phase 11 Plan 04: gruntled report Summary

`gruntled report` asks the running watch daemon for its snapshot (unix socket on linux/darwin, lock-probed report file on windows) and prints exactly what `check` prints, with check's exit codes and exit 3 when there is no result. It creates nothing.

## Tasks

| Task | Name | Commits |
| ---- | ---- | ------- |
| 1 | runReport, command wiring, usage text, docs/cli.md | 57187f4 (test), 3151eac (feat) |
| 2 | Report tests (parity, states, windows file, no-daemon scripts) | ae1febf (test) |

## What was built

- `report.go`: `reportUsage`, `reportDeps{env, goos}`, `runReportWith`. Flow: parseArgs, format check (exit 2), `openRepoRoot` (exit 3 "cannot open repository"), `statusfile.Dir`, CheckDir on parent and repo dir (absent => "gruntled: no watch daemon is running for <root>" exit 3), `queryDaemon` (Query OpReport 2s; windows or ErrUnsupported => Probe lock, free => no daemon, held => ReadDump). VersionError => "daemon runs protocol vX, this gruntled speaks vY; restart the daemon". nil snapshot => "the daemon is still indexing; try again". Success writes Text/JSON/SARIF via writeOut, Summary to stderr for text, sanitised failed-reindex warning, exit 1 if HasErrors.
- `instance.go`: `openRepoRoot` (open + Abs + EvalSymlinks) now used by watch and report.
- `main.go`: topUsage lists `report`, "Run ..." line names it, runCtx dispatches it.
- `docs/cli.md`: usage list, examples, top usage block, report usage block, exit-code paragraph, `## gruntled report` section (read-only, last published result, linux/darwin socket, windows report file under lock, no live query).

## Verification

- `go test -count=1 ./...` green; report tests `-count=10` green (1.8s).
- Mutation checks: skipping the lock probe, dropping SanitizeReason, swapping SARIF for JSON each fail the new tests.
- `go vet ./...` clean on linux/{amd64,arm64}, darwin/{amd64,arm64}, windows/{amd64,arm64}; freebsd build ok.
- `bash scripts/check-architecture.sh` exit 0.
- `-race` and `scripts/test-check-architecture.sh` left to CI per orchestrator constraint.

## Deviations from Plan

- TestReportUsage (unit, in report_test.go) written in Task 1 as its RED test alongside the TestHelpMatchesDocs row; Task 2 tests passed on first run because Task 1 had implemented the behaviour (verified non-vacuous by mutation).
- Failed state is tested against a fake daemon (ipc.Listen+Serve or TryLock+WriteDump serving a failed snapshot with ESC/CR/LF/BEL in LastError), as no fixture makes the real indexer fail deterministically (same approach as 11-03).
- Added cases beyond the plan: lock held without report file (still indexing), protocol mismatch via a v99 dump, runtime dir present but no daemon creates nothing, explicit ESC-never-reaches-stderr assertion.

## Self-Check: PASSED
