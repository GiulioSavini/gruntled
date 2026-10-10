# Research Summary — v0.3 Watch & Blast

**Date:** 2026-10-08
**Confidence:** MEDIUM-HIGH (dependency graphs compiled with `go list -deps` on Go 1.27 for linux/darwin/windows; editor and watcher behaviour from documentation and training knowledge)

Detail: `STACK.md`, `FEATURES.md`, `ARCHITECTURE.md`, `PITFALLS.md`.

## Stack additions

- `fsnotify` v1.10.1, build-tagged `!windows` (on windows it links `net` via `x/sys/windows`, breaking the no-net proof).
- Stdlib stat-polling watcher: the windows watcher, `--poll`, and fallback where inotify is blind (WSL2 `/mnt/c`, ENOSPC).
- `pgregory.net/rapid` v1.3.0, test-only, for the incremental == full property test (stateful, shrinking).
- Raw `syscall` AF_UNIX + `syscall.Flock` on unix; kernel releases flock on crash.
- Never: `x/sys/windows`, gofrs/flock, gRPC, go-winio, go-git.

## Table stakes

Initial full index; ~150 ms trailing debounce; ignore `.git`, `.terraform`, `.terragrunt-cache`, swap/backup files; new/deleted directories; clean shutdown (socket removed, lock released); atomic status file (tmp + rename); `report` fails cleanly with no daemon; output identical to `check`; blast sets disjoint and sorted.

**Differentiators:** equivalence property test carries the zero-false-positive promise to the daemon; Impacted from semantic surface, not git diff.

**Anti-features:** daemonizing/forking; `report` auto-starting the daemon; status/lock/socket files inside the analysed repo; git-based blast.

## Architecture

- Persistent parse cache (the loader's `fileCache`) invalidated by dirty path; discovery and graph rebuilt on every reindex, so incremental == full by construction.
- Watcher events are hints: each marks a path dirty, a stat decides. Neutralises editor save patterns, buffer overflows, checkout storms.
- Debounce in adapters / `cmd` (application layer's stdlib allowlist excludes `time`).
- Status file and socket are adapters.

## Watch out for

Dependency bumps breaking the six-target proof; trusting events for correctness (add periodic revalidate); torn reads mid-save (GRT100 flashes); stale sockets (connect → flock → unlink → bind); macOS `sun_path` ~104 bytes; real-watcher tests in CI (deadline conditions, never sleeps); cache memory growth; existing debt reported as Broken.

## Decisions taken (unattended run, recommended options — revisit if wrong)

1. **DAEMON-04 on Windows:** option A — raw syscall sockets on linux/darwin; on windows `report` reads the status file only. Proof stays strict ("cannot", not "does not").
2. **Blast baseline:** `gruntled blast --base <dir> <path>` (one-shot, CI-friendly) plus the daemon's in-memory baseline. Without a baseline: Broken only, labelled "no baseline". Broken = finding present after and absent before.
3. **Impacted:** one hop (direct consumers), surface = variable/output names added or removed.
4. **DAEMON-03:** status file path and line format fixed in the requirement.
5. **DAEMON-05:** detect running instance, exit 0 pointing at it; no multiplexed client.

## Suggested build order

1. Persistent parse cache + rapid equivalence test (in-memory `fs.FS`, no watcher).
2. Blast with `--base` (no daemon dependency).
3. Watcher + `watch` (poll first, fsnotify second, same contract tests).
4. Status file.
5. Socket, lock, attach, `report`.
6. Proof extension for new deps, docs, release.

Deeper research flagged for steps 3 and 5 (darwin kqueue, raw AF_UNIX behaviour).
