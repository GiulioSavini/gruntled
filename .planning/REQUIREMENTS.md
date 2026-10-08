# Requirements: gruntled

**Defined:** 2026-10-08
**Core Value:** see `.planning/PROJECT.md` → Core Value (tell the user, before they run anything slow, that a dependency output reference is broken)

## v0.3 Requirements

### Daemon

- [x] **DAEMON-01**: User runs `gruntled watch [path]` and the daemon builds a full index at start, then reindexes only changed files after a save (~150 ms trailing debounce), ignoring `.git`, `.terraform`, `.terragrunt-cache` and editor swap/backup files, and picking up new and deleted directories
- [x] **DAEMON-02**: Incremental reindexing produces exactly the diagnostics and graph a full rescan produces, proven by a rapid stateful property test over an in-memory filesystem
- [x] **DAEMON-03**: The daemon writes one status line (e.g. `gruntled: 2 errors (GRT001×1 GRT003×1) @ 14:02:11`, or `gruntled: ok @ …`) atomically to a documented per-repository path outside the repository, readable from a shell prompt, tmux or an editor status bar on every target OS
- [ ] **DAEMON-04**: User runs `gruntled report` and gets the running daemon's current diagnostics in the same formats as `check` (text/json/sarif) over a unix socket on linux and darwin; on windows `report` reads the status file; with no daemon running it exits non-zero with a clear message and never starts one
- [x] **DAEMON-05**: Starting `gruntled watch` while a daemon already serves that repository detects it (lock + socket, stale socket recovered), prints where it runs and exits 0 without starting a second; a crashed daemon leaves no lock that blocks a restart
- [ ] **DAEMON-06**: The no-net/no-exec binary proof still passes on all six release targets with the watcher and socket code linked in (fsnotify only on `!windows`, stat polling on windows and via `--poll`)

### Blast radius

- [x] **BLAST-01**: User runs `gruntled blast --base <dir> <path>` and gets two disjoint, sorted sets: Broken (units with a finding present in `<path>` and absent in `<dir>`) and Impacted (units consuming a module whose surface changed), in text and json; without a baseline only Broken is reported, labelled "no baseline"
- [x] **BLAST-02**: A unit is Impacted only when a module it directly consumes (one hop) gained or lost a `variable` or `output` name; unchanged surfaces impact nothing, and pre-existing findings are never reported as Broken

## Future Requirements

Deferred. Tracked, not in the current roadmap.

### More diagnostics

- **MORE-03**: `GRT004` — output removed from a module but still referenced downstream (pairs naturally with blast's baseline)
- **MORE-04**: `GRT005` — `inputs` key matching no `variable` in the module
- **MORE-05**: `GRT006` — `variable` without default that no unit sets

### Daemon follow-ups

- Blast from the daemon's in-memory baseline (`report --blast`, `rebase` command)
- `report` over AF_UNIX on windows (needs a scoped `net` exception to the proof)
- Transitive Impacted, type-level surface changes (added required variable, type change)

## Out of Scope

| Feature | Reason |
|---------|--------|
| Daemonizing / forking into background | fork/exec breaks the no-exec guarantee; run under tmux, systemd or the shell's `&` |
| `report` auto-starting a daemon | Hidden process start; violates explicitness |
| State files inside the analysed repository | Pollutes the user's tree and git status |
| Git-based blast (shelling out or go-git) | External process / huge dependency; `--base <dir>` covers CI via `git worktree` |
| `x/sys/windows`, gofrs/flock, go-winio, gRPC | Link `net` on windows and break the six-target no-net proof |
| On-disk index cache | Unchanged from v0.1: invalidation and corruption cost vs sub-second start |
| Desktop notifications, LSP | Unchanged from v0.1: status file covers editors and prompts |

## Traceability

Which phases cover which requirements. Updated during roadmap creation.

| Requirement | Phase | Status |
|-------------|-------|--------|
| DAEMON-01 | Phase 10 | Complete |
| DAEMON-02 | Phase 8 | Complete |
| DAEMON-03 | Phase 10 | Complete |
| DAEMON-04 | Phase 11 | Pending |
| DAEMON-05 | Phase 11 | Complete |
| DAEMON-06 | Phase 11 | Pending |
| BLAST-01 | Phase 9 | Complete |
| BLAST-02 | Phase 9 | Complete |

---
*Requirements defined: 2026-10-08*
*Last updated: 2026-10-08 after starting milestone v0.3*

**Coverage:** 8/8 v0.3 requirements mapped, 0 orphans.
