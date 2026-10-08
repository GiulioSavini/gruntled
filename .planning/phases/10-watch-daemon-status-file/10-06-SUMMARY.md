---
phase: 10-watch-daemon-status-file
plan: 06
subsystem: cmd/gruntled
tags: [watch, daemon, cli, status-file, docs]
requires:
  - "10-02: watching.Indexer, Loader mutex and batch Invalidate"
  - "10-03: presenter.Status*, statusfile.Path/Inside/Writer"
  - "10-04: watch.NewPoll, DefaultQuiet/DefaultPollInterval"
  - "10-05: watch.NewNative, ErrWatchLimit/ErrNativeUnavailable, WarnPollAdvised"
  - "10-07: watch.Run single indexer goroutine"
provides:
  - "gruntled watch [--poll] [--poll-interval d] [--debounce d] [--status-file p] [--print-status-path] [path]"
  - "runCtx(ctx, ...) with signal.NotifyContext in main; run() unchanged"
  - "docs/cli.md watch section: status path rule, line formats, background usage"
affects: [11 (report reads the same status file, single instance beside it)]
tech-stack:
  added: []
  patterns: [deps struct seam for clock/backends/env/loader hook, publisher owned by the Run goroutine, watcher created before initial index]
key-files:
  created:
    - cmd/gruntled/watch.go
    - cmd/gruntled/watch_test.go
    - cmd/gruntled/testdata/script/watch_usage.txtar
    - cmd/gruntled/testdata/script/watch_exitcodes.txtar
  modified:
    - cmd/gruntled/main.go
    - cmd/gruntled/e2e_test.go
    - docs/cli.md
    - .planning/STATE.md
    - .planning/phases/10-watch-daemon-status-file/10-VALIDATION.md
decisions:
  - "Status path (default or --status-file) inside the repository, also via symlink, is exit 2; checked before --print-status-path prints"
  - "Initial index failure: status file keeps 'failed (...)', stderr gets one 'gruntled: initial index: ...' line, exit 3; reindex failures go to stderr and the daemon keeps running"
  - "Run's debounce clock stays real time.Now; only the status stamp uses the injected clock"
  - "Status write failure is reported once on stderr and never stops the daemon"
requirements-completed: [DAEMON-01, DAEMON-03]
metrics:
  duration: 25min
  completed: 2026-10-08
  tasks: 2
  files: 9
---

# Phase 10 Plan 06: gruntled watch CLI Summary

`gruntled watch` wires the poll/native watcher, `watching.Indexer` and `watch.Run` into a foreground daemon that writes an atomic one-line status file at `<base>/gruntled/<hash12>/status` (outside the repo, `--print-status-path` to find it) and prints only changed diagnostics to stdout in `check` text format. DAEMON-01 and DAEMON-03 are complete.

## What was built

- **main.go**: `main` uses `signal.NotifyContext(SIGINT, SIGTERM)` and calls `runCtx`; `run` wraps `runCtx(context.Background(), ...)` so tests and testscript are untouched. `watch` added to the command switch and `topUsage`.
- **watch.go**: `watchUsage`, `defineWatchFlags` (`--debounce` defaults to `watch.DefaultQuiet`, `--poll-interval` to `watch.DefaultPollInterval`), `watchDeps`/`defaultWatchDeps` (clock, native/poll constructors, env, goos, maxWait, loaderHook), `runWatchWith`:
  parse and validate durations (exit 2) -> `openRepo` + Abs/EvalSymlinks (exit 3) -> status path, inside-repo rejection (exit 2) -> `--print-status-path` -> backend (poll on `--poll`/windows; native else, fallback notice on ErrWatchLimit/ErrNativeUnavailable, other error exit 3, `/mnt/` hint) -> stderr banner -> Loader + Indexer -> `watch.Run` with a publisher that writes status lines and changed-diagnostic stdout.
- **Tests** (`watch_test.go`, all waits through `eventually`, fixed clock 14:02:11, status file in a separate temp dir): StatusFile (error -> ok -> stopped, exit 0), ReindexesOnlyChanged (CacheStats misses == 1 after one save), Parity (7 ops x poll/native, each op must change the line, both equal a fresh `check`), BackendSelection (watch-limit fallback, other native error exit 3, `--poll` and windows never call native), StdoutOnChange (exactly two renderings across a no-op save, no absolute root), QuietRepoPrintsNothing, DebounceDefault (also pins the defaults spelled in the help text), StatusInsideRepo (absolute, relative via t.Chdir, symlink). Testscripts for usage and exit codes, including `--print-status-path` regex and spelling-independence.
- **docs/cli.md**: usage lines, watch help block, exit codes, `## gruntled watch` (flags table, behaviour, stdout, status file path rule/formats/atomicity/permissions, prompt/tmux example, background/systemd usage), guarantees note, three known-limitation bullets. `TestHelpMatchesDocs` now covers `watch`.

## Verification

- `go test -count=1 ./...`: green. `go test -count=30 -cpu 1,2,8 -run TestWatch ./cmd/gruntled/`: green (~28 s).
- `GOOS=windows GOARCH=arm64 go vet ./...`, `GOOS=darwin GOARCH=arm64 go vet ./...`: clean. `go mod tidy -diff`: clean. gofmt clean.
- `bash scripts/check-architecture.sh`: `architecture: OK (4 domain packages, 5 application packages, 1 interfaces packages)` with fsnotify linked into cmd/gruntled.
- Evidence: linux/arm64 binary `go tool nm | grep -cE ' (os\.StartProcess|syscall\.(forkExec|ForkExec|Exec))$'` = 0 (sanity: `main.runWatchWith` and `fsnotify.NewWatcher` symbols present); `go list -deps ./cmd/gruntled | grep -c fsnotify` = 2 on linux and darwin/arm64; `GOOS=windows GOARCH=amd64 go list -deps ./cmd/gruntled | grep -cE 'fsnotify|x/sys/windows'` = 0.
- Not run locally (CI only): `-race`, `scripts/test-check-architecture.sh`.

## Commits

- 85caa07 test(10-06): add failing tests for gruntled watch
- 7fab2d4 feat(10-06): gruntled watch command
- 0f7292f feat(10-06): gruntled watch with status file, docs and planning updates

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 2 - Correctness] Help text and docs kept in step**
- **Found during:** Task 2
- **Issue:** `TestHelpMatchesDocs` checked only check/graph/blast; the watch exit-code lines could drift from docs.
- **Fix:** Added `{"watch", 3}`; `TestWatchDebounceDefault` also asserts the help text spells `watch.DefaultQuiet` and `watch.DefaultPollInterval`.
- **Files:** cmd/gruntled/e2e_test.go, cmd/gruntled/watch_test.go
- **Commit:** 0f7292f, 7fab2d4

**2. [Rule 2 - Correctness] "No cache" guarantee clarified**
- docs/cli.md Guarantees said nothing is stored; added that watch caches in memory only and writes only its status file outside the repo.

Beyond the plan: `TestWatchQuietRepoPrintsNothing`, a parity guard that fails if an op leaves the status line unchanged (a step that changes nothing proves nothing), and a "other native error -> exit 3" case. The initial-index-failure exit 3 path has no CLI test (no fixture makes `checking.Check` return an error); it is covered by `TestRunInitialIndexError` in the watch package. ROADMAP plan list was already in place (7 plans, from 10-07 planning).

## Self-Check: PASSED
