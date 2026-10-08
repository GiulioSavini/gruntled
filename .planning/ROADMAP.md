# Roadmap: gruntled

## Milestones

- ✅ **v0.1 Validated Engine** - Phases 1-4 (shipped 2026-09-29) — archived in `milestones/v0.1-ROADMAP.md`
- ✅ **v0.2 CI-Ready** - Phases 5-7 (shipped 2026-10-08) — archived in `milestones/v0.2-ROADMAP.md`
- 🚧 **v0.3 Watch & Blast** - Phases 8-11 (in progress)

## Phases

<details>
<summary>✅ v0.1 Validated Engine (Phases 1-4) - SHIPPED 2026-09-29</summary>

See `milestones/v0.1-ROADMAP.md`.

</details>

<details>
<summary>✅ v0.2 CI-Ready (Phases 5-7) - SHIPPED 2026-10-08</summary>

- [x] Phase 5: Graph Diagnostics (7/7 plans) — completed 2026-09-30
- [x] Phase 6: Machine-Readable Output (5/5 plans) — completed 2026-10-01
- [x] Phase 7: Distribution & CI Integration (5/5 plans) — completed 2026-10-01

See `milestones/v0.2-ROADMAP.md`.

</details>

### 🚧 v0.3 Watch & Blast (In Progress)

**Milestone Goal:** A long-running `gruntled watch` daemon that keeps diagnostics fresh with incremental reindexing, plus a `blast` command that reports which units a change breaks or impacts.

- [x] **Phase 8: Incremental Index Foundation** - Persistent parse cache; incremental reindex provably equals full rescan (completed 2026-10-08)
- [x] **Phase 9: Blast Radius** - `gruntled blast --base` reports disjoint Broken and Impacted unit sets (completed 2026-10-08)
- [ ] **Phase 10: Watch Daemon & Status File** - `gruntled watch` reindexes on save and writes an atomic status line
- [ ] **Phase 11: Report, Single Instance & Release Proof** - Socket/lock/attach, `report`, six-target no-net/no-exec proof, docs

## Phase Details

### Phase 8: Incremental Index Foundation
**Goal**: The index can be updated incrementally from a set of dirty paths and always yields exactly what a full rescan yields.
**Depends on**: Phase 7
**Requirements**: DAEMON-02
**Success Criteria** (what must be TRUE):
  1. After any sequence of file creates, edits, renames and deletes on an in-memory filesystem, incremental reindex produces diagnostics and graph identical to a full rescan
  2. The rapid stateful property test runs in CI, shrinks failures to a minimal operation sequence, and uses no real watcher or sleeps
  3. `check` output on the existing fixtures is unchanged (no regression from the persistent cache)
  4. Unchanged files are not re-parsed on a reindex (observable via cache hit count)
**Plans**: 2 plans

Plans:
- [x] 08-01-PLAN.md — persistent parse store, Invalidate, CacheStats
- [x] 08-02-PLAN.md — rapid stateful incremental == full test

### Phase 9: Blast Radius
**Goal**: User can see which units a change breaks and which it puts at risk, against a baseline directory.
**Depends on**: Phase 8
**Requirements**: BLAST-01, BLAST-02
**Success Criteria** (what must be TRUE):
  1. `gruntled blast --base <dir> <path>` prints two disjoint, sorted sets, Broken and Impacted, in text and json
  2. A unit with a finding present in `<path>` and absent in `<dir>` is Broken; a finding present in both is never reported as Broken
  3. A unit directly consuming a module that gained or lost a `variable` or `output` name is Impacted; a module with an unchanged surface impacts nothing
  4. Without `--base`, only Broken is reported and labelled "no baseline"
**Plans**: 3 plans

Plans:
- [x] 09-01-PLAN.md — domain impact package (position-free finding identity, surface diff, Compute) + GRT100 stability test
- [x] 09-02-PLAN.md — blasting use case, BlastText/BlastJSON presenters
- [x] 09-03-PLAN.md — `blast` CLI, testscript scenarios, docs/cli.md, phase gate

### Phase 10: Watch Daemon & Status File
**Goal**: User can leave `gruntled watch` running and see current diagnostics from a prompt or editor bar after each save.
**Depends on**: Phase 8
**Requirements**: DAEMON-01, DAEMON-03
**Success Criteria** (what must be TRUE):
  1. `gruntled watch [path]` builds a full index at start, then after a save reindexes only the changed files within about 150 ms of trailing debounce
  2. Changes under `.git`, `.terraform`, `.terragrunt-cache` and editor swap/backup files trigger no reindex; newly created and deleted directories are picked up
  3. The daemon writes a one-line status (e.g. `gruntled: 2 errors (GRT001×1 GRT003×1) @ 14:02:11` or `gruntled: ok @ …`) atomically to a documented per-repository path outside the repository, readable with `cat`
  4. `--poll` (and windows) uses stat polling and yields the same results as the fsnotify watcher
**Plans**: 7 plans

Plans:
- [x] 10-01-PLAN.md — extend Step 9 binary proof (x/sys/unix scan exemption, per-target nm symbol check, windows-no-fsnotify) + self-test cases
- [x] 10-02-PLAN.md — Loader mutex, batch Invalidate with path contract, watching.Indexer
- [x] 10-03-PLAN.md — presenter status lines, statusfile path + atomic writer
- [x] 10-04-PLAN.md — watch core: ignore, pending, debouncer, poll adapter, contract suite
- [ ] 10-05-PLAN.md — fsnotify native adapter behind !windows, safety net, dependency
- [ ] 10-06-PLAN.md — `gruntled watch` CLI wiring, docs, parity/status tests, bookkeeping
- [ ] 10-07-PLAN.md — watch run loop (single indexer goroutine, skip-stale publish)

### Phase 11: Report, Single Instance & Release Proof
**Goal**: User can query a running daemon, never get two for one repository, and trust the release binaries still cannot touch net or exec.
**Depends on**: Phase 10
**Requirements**: DAEMON-04, DAEMON-05, DAEMON-06
**Success Criteria** (what must be TRUE):
  1. `gruntled report` returns the running daemon's diagnostics in text, json and sarif matching `check`, over a unix socket on linux/darwin and from the status file on windows
  2. `gruntled report` with no daemon exits non-zero with a clear message and starts nothing
  3. A second `gruntled watch` on the same repository prints where the daemon runs and exits 0; after a daemon crash, a restart succeeds (stale socket and lock recovered)
  4. The no-net/no-exec binary proof passes on all six release targets with watcher and socket code linked in
  5. README documents `watch`, `report`, `blast`, the status file path and the Windows limitation
**Plans**: TBD

## Progress

| Phase | Milestone | Plans Complete | Status | Completed |
|-------|-----------|----------------|--------|-----------|
| 5. Graph Diagnostics | v0.2 | 7/7 | Complete | 2026-09-30 |
| 6. Machine-Readable Output | v0.2 | 5/5 | Complete | 2026-10-01 |
| 7. Distribution & CI Integration | v0.2 | 5/5 | Complete | 2026-10-01 |
| 8. Incremental Index Foundation | v0.3 | 2/2 | Complete | 2026-10-08 |
| 9. Blast Radius | v0.3 | 3/3 | Complete | 2026-10-08 |
| 10. Watch Daemon & Status File | v0.3 | 4/7 | In Progress | - |
| 11. Report, Single Instance & Release Proof | v0.3 | 0/? | Not started | - |
