---
phase: 11-report-single-instance-release-proof
plan: 01
subsystem: statusfile, presenter
tags: [security, runtime-dir, symlink, owner-check, sanitize]
requires: []
provides:
  - statusfile.EnsureDir / CheckDir / EnsureRepoDir (trust boundary for lock, socket, status file, report dump)
  - presenter.SanitizeReason (control-rune free failure reasons)
affects: [11-02, 11-03, 11-04, cmd/gruntled watch]
tech-stack:
  added: []
  patterns: [Lstat-based dir check, build-tagged platform hook, package-var seam for geteuid]
key-files:
  created:
    - internal/infrastructure/statusfile/ensure.go
    - internal/infrastructure/statusfile/ensure_unix.go
    - internal/infrastructure/statusfile/ensure_other.go
    - internal/infrastructure/statusfile/ensure_test.go
    - internal/infrastructure/statusfile/ensure_unix_test.go
  modified:
    - internal/infrastructure/statusfile/write.go
    - internal/interfaces/presenter/status.go
    - internal/interfaces/presenter/status_test.go
decisions:
  - "CheckDir: Lstat; symlink, non-directory, perm&077 (non-windows), foreign uid (unix) all wrap ErrInsecureDir with path and reason; absent -> fs.ErrNotExist, nothing created"
  - "EnsureRepoDir checks <base>/gruntled and <base>/gruntled/<hash12>; Writer checks only its own directory so a custom --status-file under /tmp still works"
  - "SanitizeReason maps unicode.IsControl runes to spaces then collapses whitespace; StatusFailed caps at 120 runes after sanitising"
requirements-completed: []
metrics:
  duration: 4min
  completed: 2026-10-08
  tasks: 2
  files: 8
---

# Phase 11 Plan 01: Runtime dir hardening + reason sanitising Summary

Closes Phase 10 security findings 2 and 3: per-repo runtime directory refused when symlinked, non-directory, group/other accessible or (unix) foreign-owned, checked at both `<base>/gruntled` and `<hash12>` levels; control runes stripped from failure reasons before the status line.

## Tasks

| Task | Name | Commits |
| ---- | ---- | ------- |
| 1 | statusfile EnsureDir / CheckDir / EnsureRepoDir | 7c6e260 (test), b9e524a (feat) |
| 2 | presenter.SanitizeReason in StatusFailed | db686f9 (test), 24b9418 (feat) |

## What was built

- `CheckDir(dir)`: `os.Lstat`, refuses symlink / non-dir / perm&077 (skipped on windows) / uid != geteuid (unix via `checkOwner` hook in `ensure_unix.go`; no-op in `ensure_other.go`).
- `EnsureDir(dir)`: `MkdirAll(0700)` + `CheckDir`; idempotent.
- `EnsureRepoDir(root, env)`: `Dir` + `EnsureDir(parent)` + `EnsureDir(d)`.
- `Writer.Write` now calls `EnsureDir(dir)` instead of the inline MkdirAll/Stat/perm block; `ErrInsecureDir` doc widened.
- `SanitizeReason(s)` exported; `StatusFailed` uses it before the 120-rune cut. Existing golden lines byte-identical.

## Verification

- `go test -count=1 ./...` green; `go vet` clean for linux, windows/arm64, darwin/arm64.
- `bash scripts/check-architecture.sh` exit 0 (presenter adds only `unicode`, already allowlisted).
- Owner check tested without root via `geteuid` override (restored with `t.Cleanup`).
- New tests fail without the fix: symlink/owner cases (old Writer used `os.Stat`, followed links, no uid check); ESC case (old `strings.Fields` kept ESC).
- `-race` and `scripts/test-check-architecture.sh` left to CI per orchestrator constraint.

## Deviations from Plan

None - plan executed exactly as written. Non-directory paths wrap `ErrInsecureDir` ("not a directory") rather than a bare error; the plan only required "error".

## Notes

- Stderr sanitising of reasons (truth 4, stderr half) is enabled by the exported `SanitizeReason`; wiring into cmd stderr lines belongs to later plans that touch `cmd/gruntled/watch.go`.
- `requirements-completed: []`: DAEMON-05 is only advanced (safe directory for lock + socket), not completed.

## Self-Check: PASSED
