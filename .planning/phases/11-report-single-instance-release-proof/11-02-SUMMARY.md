---
phase: 11-report-single-instance-release-proof
plan: 02
subsystem: ipc
tags: [ipc, af_unix, flock, single-instance, crash-safe, no-net]
requires:
  - statusfile.Writer / EnsureDir (11-01)
provides:
  - ipc.TryLock / LockRetry / Probe / Lock.Release (flock linux+darwin, CreateFile share 0 windows, no-op elsewhere)
  - ipc.Listen / Serve / Server.Close / Query (linux || darwin; ErrUnsupported stubs elsewhere)
  - ipc.WriteDump / ReadDump (windows report transport, all platforms)
  - ipc.Info / Snapshot / VersionError / CheckSockPath / ErrHeld / ErrNoDaemon / ErrUnsupported
affects: [11-03, 11-04, 11-05, 11-06]
tech-stack:
  added: []
  patterns: [os.NewFile on non-blocking fd + SyscallConn().Read accept, ForkLock around socket/accept + CloseOnExec, one-line versioned JSON wire, package-var deadline seam, self re-exec crash helper in tests]
key-files:
  created:
    - internal/infrastructure/ipc/ipc.go
    - internal/infrastructure/ipc/dump.go
    - internal/infrastructure/ipc/lock_unix.go
    - internal/infrastructure/ipc/lock_windows.go
    - internal/infrastructure/ipc/lock_other.go
    - internal/infrastructure/ipc/sock_unix.go
    - internal/infrastructure/ipc/sock_other.go
    - internal/infrastructure/ipc/ipc_test.go
    - internal/infrastructure/ipc/lock_test.go
    - internal/infrastructure/ipc/sock_unix_test.go
    - internal/infrastructure/ipc/crash_unix_test.go
  modified: []
decisions:
  - "Lock open uses O_NOFOLLOW on unix (TryLock and Probe): a symlink at the lock path errors instead of locking the target"
  - "Server answers with v=ProtocolVersion always; client compares response v to its own and returns *VersionError{Daemon, Client}; unexported query(sock, op, v, timeout) lets tests speak v2"
  - "Query timeout bounds connect (SO_SNDTIMEO) and the whole exchange; 0 means 2s"
  - "Accept loop exits on Close via a quit channel checked on any accept error (the raw poller error is not os.ErrClosed); EINTR/ECONNABORTED continue; other errors pause 100ms on a timer that Close also wakes"
  - "Server read deadline is serverReadDeadline (package var, 2s), write deadline 2s after the request is read"
  - "Snapshot.State empty is served as ready; ping carries info + state but never a snapshot"
requirements-completed: []
metrics:
  duration: 12min
  completed: 2026-10-08
  tasks: 2
  files: 11
---

# Phase 11 Plan 02: ipc package (lock, AF_UNIX server, dump) Summary

Stdlib-only `internal/infrastructure/ipc`: kernel/OS-released instance lock (flock / CreateFile share 0), AF_UNIX request/response server on raw `syscall` with poller-integrated accept, stale-socket recovery under the lock, and a JSON dump transport for windows. No `net`, `os/exec` or `x/sys` in non-test files on any release target.

## Tasks

| Task | Name | Commits |
| ---- | ---- | ------- |
| 1 | Contract, locks (3 platforms), Probe, dump transport | b2f647b (test), 4e61535 (feat) |
| 2 | AF_UNIX Listen/Serve/Query, stale recovery, crash test | 0f63809 (test), 3abf1f4 (feat) |

## What was built

- `ipc.go`: constants, errors, `Info`, `Snapshot` (`[]byte` fields, base64 in JSON), `VersionError`, `CheckSockPath` (103 bytes, message names length, XDG_RUNTIME_DIR, TMPDIR), `Lock.Release` (keeps the file), `LockRetry` (20ms injected sleep), wire codec (`request`/`response`, `readLine` with cap, `respond`, `decodeResponse`).
- `lock_unix.go` (`linux || darwin`): `OpenFile(O_RDWR|O_CREATE|O_NOFOLLOW, 0600)` + `Flock(LOCK_EX|LOCK_NB)` via `SyscallConn().Control`, EINTR loop; `Probe` opens without O_CREATE.
- `lock_windows.go`: `syscall.CreateFile` share 0, `OPEN_ALWAYS` / `OPEN_EXISTING`, `Errno(32)` => held, file/path not found => free.
- `lock_other.go`: no-op lock, Probe false.
- `sock_unix.go`: `Listen` (CheckSockPath, Lstat: socket removed, anything else refused untouched, bind, chmod 0600, listen 16, non-blocking, `os.NewFile`), `Serve` (accept loop + WaitGroup handlers, 4 KiB request, read deadline, one response line), `Close` (idempotent: quit, close listener, wait loop + handlers, remove socket), `dial`/`Query` (SO_SNDTIMEO, deadline, 64 MiB response cap, ENOENT/ECONNREFUSED => ErrNoDaemon).
- `sock_other.go`: ErrUnsupported stubs, types declared.
- `dump.go`: `WriteDump` through `statusfile.Writer` (atomic, 0600, checked dir), `ReadDump` (64 MiB cap, version check, nil snapshot when indexing).

## Verification

- `go test -count=1 ./internal/infrastructure/ipc/` green (26 tests); `-count=20` green (0.94s, no flakes).
- Crash test: test binary re-runs itself as helper (`GRUNTLED_IPC_HELPER`), holds lock + serves; parent sees ErrHeld, Probe true, ping pid; SIGKILL; stale socket gives ErrNoDaemon; TryLock succeeds; Listen replaces socket; new server answers. Synchronised on the helper's "ready" line, no sleeps.
- `go vet` clean for linux/amd64, linux/arm64, darwin/amd64, darwin/arm64, windows/amd64, windows/arm64 (plus freebsd for the `_other` files); `GOOS=windows GOARCH=arm64 go vet ./...` and `GOOS=darwin GOARCH=arm64 go vet ./...` clean.
- `go list -deps` of ipc on all six targets: no `net`, `net/*`, `os/exec`, `golang.org/x/sys/*`.
- No spawner names in non-test ipc files (grep empty).
- `bash scripts/check-architecture.sh` exit 0. `go test ./...` green.
- `-race` and `scripts/test-check-architecture.sh` left to CI per orchestrator constraint.

## Deviations from Plan

None - plan executed as written. Additions within the contract:
- O_NOFOLLOW on the unix lock open (hardening, Rule 2).
- Accept also holds `ForkLock.RLock` around `Accept`+`CloseOnExec` (darwin has no accept4), so accepted fds cannot leak into a child process.
- Bind/listen failure after bind removes the half-created socket file.

## Notes

- `requirements-completed: []`: DAEMON-04/05/06 are advanced (transport, single-instance primitive) but completed only when cmd wires them (11-03..11-06).
- darwin socket/lock and windows lock code compile and vet but run only on CI matrices (Pitfall 6).

## Self-Check: PASSED
