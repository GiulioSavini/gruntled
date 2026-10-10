# Feature Landscape (v0.3 Watch & Blast)

**Domain:** watch daemon + change impact for a static Terragrunt analyser
**Researched:** 2026-10-08
**Confidence:** MEDIUM (behaviour of gopls/tsc/watchexec from training knowledge and project design doc; not re-verified online; the design choices below are mostly derived from this project's constraints)

## How comparable tools behave
- **tsc --watch**: initial full build, then incremental by file with a ~250 ms debounce, prints "Found N errors. Watching for file changes." Re-prints the full current error set every cycle (state, not delta).
- **gopls**: long-lived, in-memory snapshots, immutable per change; unsaved buffers matter (LSP, out of scope). Lesson: immutable snapshot swapped atomically so readers never see half-updated state.
- **watchexec / entr**: coalesce bursts (debounce 50-100 ms), ignore VCS dirs and editor temp files, restart/queue policy. Lesson: ignore `.git`, `.terragrunt-cache`, `.terraform`, swap/backup files.
- **bazel-watcher (ibazel)**: change -> affected targets only via the build graph. Lesson: this is the blast-radius idea; Bazel computes it from the graph, not from content diff.
- **terramate / terragrunt `--filter 'git...'`**: change detection by git diff, reports changed stacks and dependents, no semantic surface comparison. gruntled's differentiator is the semantic filter (surface changed vs comment changed).

## Table Stakes

| Feature | Why expected | Complexity | Notes |
|---------|--------------|------------|-------|
| Initial full index at start, then print current findings | every watch tool does | Low | reuse `check` pipeline |
| Debounce (100-200 ms) and coalesce bursts | editors write 2-4 events per save | Low | timer reset on each dirty path |
| Treat rename/atomic-save as a change | vim/VSCode/JetBrains save via temp+rename | Med | by design: events only mark paths dirty; stat decides |
| Ignore noise dirs and temp files | `.git`, `.terraform`, `.terragrunt-cache`, `*.swp`, `4913`, `*~`, `.#*` | Low | filter before debounce |
| Only `terragrunt.hcl`, included files, `*.tf`, `*.hcl` matter | avoid pointless reindex | Low | but includes can have any name: dirty set must be intersected with the files the index actually read, plus new files by pattern |
| Handle new/deleted directories and units | repos change shape | Med | add watches for new dirs, rescan them |
| Clean shutdown on SIGINT/SIGTERM, remove socket, release lock | users Ctrl-C | Low | |
| Attach, not duplicate (DAEMON-05) | stated | Med | try-connect first, flock second |
| One-line status file, atomic write (tmp+rename in same dir) | prompts read it mid-write | Low | content like `ok 0E 2W 65u 12ms` or `err 3E ...`; no ANSI; trailing newline |
| `report` works when daemon is absent | scripts must not hang | Low | clear message, distinct exit code |
| Deterministic output identical to `check` | project value | Low | `report` reuses presenters |
| `blast <path>`: Broken and Impacted as disjoint sets, deterministic order | BLAST-01 | Med | |

## Differentiators

| Feature | Value | Complexity | Notes |
|---------|-------|------------|-------|
| Incremental result provably equal to full rescan (rapid model test) | trust: zero-false-positive ethos extends to the daemon | Med | strongest selling point; tests are the feature |
| Surface-only Impacted (comment edits impact nothing) | signal not noise | Med | BLAST-02 |
| `report --format json/sarif/text` same as check | prompt, CI and editor consume one thing | Low | |
| `watch` prints only deltas after the first run (new/fixed findings) plus a count | quiet terminal | Low | optional |
| Status file path discoverable (`gruntled status-path` or printed at start; default under repo `.gruntled/`?) | prompt integration | Low | see pitfall: read-only rule |
| Poll fallback automatically on watcher failure | works on WSL2 `/mnt/c`, NFS | Med | |

## Anti-Features

| Anti-feature | Why avoid | Instead |
|--------------|-----------|---------|
| Desktop notifications | Out of Scope | status file |
| LSP / diagnostics push | Out of Scope | status file + `report` |
| On-disk index cache / persistent daemon state | Out of Scope | rebuild at start (<1 s) |
| Auto-restarting daemon from `report` | surprising background processes, windows | `report` fails clearly; user runs `watch` |
| Daemonizing/forking into background (`watch -d`) | fork/exec forbidden, cross-platform pain | foreground process; users use `&`, tmux, systemd |
| Writing inside the analysed repo | Read-only rule | status/socket/lock in `$XDG_RUNTIME_DIR` or `os.TempDir()/gruntled-<hash(abs repo)>/` |
| Git integration for blast | exec forbidden, go-git is huge | baseline from daemon memory or `--base <dir>` |
| Content-hash "has file changed" semantic diffs of expressions | false positives | name-level surface only |
| Remote/multi-client protocol, auth | no network | local socket mode 0600 |

## Feature Dependencies

```
file-level parse cache (memoised loader)  -> incremental reindex (DAEMON-01)
incremental reindex                       -> property test (DAEMON-02)
Watcher port (fsnotify | poll)            -> DAEMON-01
snapshot holder (atomic swap)             -> status file writer (DAEMON-03), socket server (DAEMON-04)
instance lock + try-connect               -> DAEMON-05 ; socket server -> DAEMON-04
surface diff (domain)                     -> BLAST-02 -> BLAST-01
baseline snapshot (daemon start or --base)-> surface diff
```

## MVP Recommendation

1. Memoised incremental index + rapid equivalence test (DAEMON-01/02) -- everything else rides on it.
2. Blast core as a pure domain function `Blast(before, after *RepositoryGraph, changedModules)`; ship `gruntled blast --base <dir> <path>` one-shot first (works in CI, needs no daemon).
3. Watcher adapters, status file, then socket + attach.

Defer: delta printing, `status-path` helper, Windows IPC.

## Requirements that need rewording
- **DAEMON-04**: "over a unix socket" -> "on linux/darwin; on windows `report` reads the status file" (see STACK.md decision).
- **BLAST-01/02**: `blast <path>` cannot be answered from a single index: Impacted needs a before state. Reword: "`gruntled blast [--base <dir>] <path>`; with a running daemon the baseline is the surface at daemon start (or last `report --rebase`), without one `--base` is required to report Impacted; without a baseline only Broken is reported and the output says so."
- **BLAST-02**: current `Surface` holds names only (no required/default/type). "Surface changed" = a variable or output name added or removed. Added variable with no default is invisible; say so in docs.
- **DAEMON-03**: define the path and format in the requirement, otherwise "readable from a prompt" is untestable.

## Sources
- Project design doc and PROJECT.md (HIGH for constraints)
- Training knowledge of tsc/gopls/watchexec/ibazel/terramate (LOW-MEDIUM, not re-verified this session)
