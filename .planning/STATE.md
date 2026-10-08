---
gsd_state_version: 1.0
milestone: v0.3
milestone_name: Watch & Blast
status: ready_to_plan
stopped_at: Phase 10 verified + security audited (a12aaf1)
last_updated: "2026-10-08T09:57:28.236Z"
last_activity: 2026-10-08 — completed 10-06 (gruntled watch CLI)
progress:
  total_phases: 4
  completed_phases: 3
  total_plans: 12
  completed_plans: 12
---

# Project State

## Project Reference

See: .planning/PROJECT.md (updated 2026-10-08)

**Core value:** Tell the user, before they run anything slow, that `dependency.X.outputs.Y` does not exist in the module it points to.
**Current focus:** v0.3 Watch & Blast — Phase 11 (Report, Single Instance & Release Proof)

## Current Position

Phase: 11 of 11 (Report, Single Instance & Release Proof) — not planned
Plan: 7 of 7 (10-01 .. 10-07 complete)
Status: Phase 10 complete (verified passed, sec fix-small applied); Phase 11 ready to plan
Last activity: 2026-10-08 — completed 10-06 (gruntled watch CLI)

Progress: [██████████] 7/7 Phase 10 plans (v0.3: 2/4 phases verified)

## Performance Metrics

**Velocity:**
- Total plans completed: 10
- Average duration: ~29 min
- Total execution time: 3.99 hours

**By Phase:**

| Phase | Plans | Total | Avg/Plan |
|-------|-------|-------|----------|
| Phase 1 P1 | 35min | 3 tasks | 14 files |
| Phase 1 P2 | 15min | 2 tasks | 3 files |
| Phase 1 P3 | 20min | 2 tasks | 6 files |
| Phase 2 P1 | 25min | 2 tasks | 9 files |
| Phase 2 P2 | 24min | 3 tasks | 5 files |
| Phase 2 P3 | 53min | 3 tasks | 13 files |
| Phase 2 P4 | 27min | 3 tasks | 8 files |
| Phase 2 P5 | 40min | 3 tasks | 7 files |

**Recent Trend:**
- Last 5 plans: 25min, 24min, 53min, 27min, 40min
- Trend: Plan 02-05 (loader + integration + fuzz, the phase's largest wiring surface) ran longer than the P4 baseline but in line with P3's similar-scope leaf-infrastructure plan; Phase 2 is now complete

*Updated after each plan completion*
| Phase 02 P05 | 40 | 3 tasks | 7 files |
| Phase 03 P01 | 20 | 3 tasks | 7 files |
| Phase 03 P03 | 40 | 3 tasks | 16 files |
| Phase 03 P05 | 14 | 2 tasks | 2 files |
| Phase 04 P03 | 35 | 2 tasks | 3 files |
| Phase 05 P01 | interrupted | 3 tasks | 14 files |
| Phase 05 P03 | 2 | 2 tasks | 4 files |
| Phase 05 P02 | 5 | 3 tasks | 7 files |
| Phase 05 P04 | 6min | 3 tasks | 10 files |
| Phase 05 P05 | 15min | 2 tasks | 9 files |
| Phase 05 P06 | 35min | 3 tasks | 5 files |
| Phase 06 P01 | 6min | 2 tasks | 4 files |
| Phase 06 P02 | 10min | 2 tasks | 3 files |
| Phase 06 P03 | 9min | 2 tasks | 8 files |
| Phase 06 P04 | 10min | 2 tasks | 4 files |
| Phase 06 P05 | 15min | 3 tasks | 7 files |
| Phase 07 P01 | 12min | 2 tasks | 8 files |
| Phase 07 P02 | 19min | 3 tasks | 9 files |
| Phase 07 P03 | 5min | 1 tasks | 1 files |
| Phase 07 P04 | 8min | 3 tasks | 5 files |
| Phase 07 P05 | 15min | 3 tasks | 0 files |
| Phase 08 P01 | 2min | 2 tasks | 3 files |
| Phase 08 P02 | 8min | 2 tasks | 5 files |
| Phase 09 P01 | 2min | 2 tasks | 5 files |
| Phase 09 P02 | 4min | 2 tasks | 4 files |
| Phase 09 P03 | 12min | 2 tasks | 8 files |
| Phase 10 P01 | 9min | 2 tasks | 2 files |
| Phase 10 P02 | 6min | 2 tasks | 5 files |
| Phase 10 P03 | 4min | 2 tasks | 6 files |
| Phase 10 P04 | 5min | 2 tasks | 11 files |
| Phase 10 P05 | 8min | 1 tasks | 5 files |
| Phase 10 P07 | 10min | 1 tasks | 2 files |
| Phase 10 P06 | 25min | 2 tasks | 9 files |

## Accumulated Context

### Decisions

Decisions are logged in PROJECT.md Key Decisions table (v0.1 and v0.2 decisions recorded there).
Per-plan decision history lives in the phase SUMMARY files and git history.
- [Phase 08]: Only parse results persist across LoadUnits; discovery/resolution/assembly recomputed so incremental == full by construction
- [Phase 08]: GRT100 driven by per-load touched set, store pruned to touched on success
- [Phase 08]: Dirty-set contract: Invalidate on create/write/remove/rename (old+new); negative entries cached
- [Phase 08]: rapid incremental==full test compares serialised JSON+graph, seeded baseline per sequence; rapid test-only
- [Phase 09]: KeyOf = diagnostic Key minus Line/Column; GRT100 message verbatim (pinned position-free by hclconv stability test)
- [Phase 09]: Domain tests stay on pure allowlist (no rapid/fmt): property tests use fixed-seed LCG
- [Phase 09]: Blast checks current tree first; staged *blasting.Error current|baseline
- [Phase 09]: Blast change tokens ordered -variable,+variable,-output,+output; no-baseline output drops Impacted
- [Phase 09]: blast: unopenable --base exits 3, never silent no-baseline fallback; exit 1 only on error findings in Broken (HasErrors)
- [Phase 10]: Step 9 proof: per-target go tool nm linker check; textual scan exempts exactly golang.org/x/sys/unix; windows rejects fsnotify and x/sys/windows
- [Phase 10]: Loader mutex-guarded (LoadUnits/Invalidate/CacheStats) but single indexer goroutine still the model; out-of-contract Invalidate path (empty/./abs/../backslash) clears whole store
- [Phase 10]: watching.Indexer is the single seam daemon->Loader; resync = Invalidate("."), Report/error = checking.Check unchanged
- [Phase 10]: Status line: errors only, codes sorted, single write; StatusFailed collapses whitespace, 120-rune cap
- [Phase 10]: Status path <base>/gruntled/<sha256(EvalSymlinks root)[:12]>/status, base linux abs XDG_RUNTIME_DIR -> UserCacheDir -> TempDir; no case folding
- [Phase 10]: statusfile.Writer: 0700 dir (refuse perm&077 on non-windows), 0600 tmp+rename, 5 attempts 10/20/40/80ms
- [Phase 10]: pending.add: '.', volume paths, empty/abs/backslash/'..' -> resync; ignored paths dropped without signalling; take drains Ready under the same mutex
- [Phase 10]: Debouncer pure (Add/Due/Flush, caller time), DefaultQuiet 150ms / DefaultMaxWait 1s, empty Changes do not arm
- [Phase 10]: Poll: baseline in constructor, dirs reported only on add/remove/type change, unreadable subdir keeps previous entries, root failure -> resync; scanner reusable as native safety net
- [Phase 10]: 10-05: native watcher baseline taken after watches; ENFILE counts as watch limit; safety net 30s feeds same dirty set
- [Phase 10]: 10-07: Run single indexer goroutine; timer fire flushes at max(Now,Due); skip publish only if post-Index Take non-empty; Stopped only on cancel, initial failure leaves Failed
- [Phase 10]: 10-06: status path inside repo (also via symlink) is exit 2, checked before --print-status-path
- [Phase 10]: 10-06: initial index failure leaves 'failed' status, one stderr line, exit 3; reindex failures keep running
- [Phase 10]: 10-06: Run debounce uses real clock; injected clock only stamps status lines; status write failure logged once, never fatal

### Pending Todos

None yet.

### Blockers/Concerns

- RESOLVED in Phase 10: the Phase 8 security findings (`phases/08-incremental-index-foundation/08-VERIFICATION.md` → Security) are closed. 1 Loader shared with the daemon: Loader mutex (10-02) plus a single indexer goroutine in `watch.Run`, the only caller of `Indexer.Index` (10-07, wired in 10-06). 2 Lost events after watcher overflow: `Changes.Resync` → `Indexer.Index(resync)` → `Invalidate(".")` (10-02, 10-04), native overflow/error marks resync and a 30 s stat safety net feeds the same dirty set (10-05). 3 Per-path Invalidate cost: single-pass batch Invalidate (10-02). 4 Path contract: repo-relative slash paths only; empty/./absolute/../backslash paths clear the whole store (10-02, enforced again in the watcher pending set by 10-04).

## Session Continuity

Last session: 2026-10-08T09:48:39.872Z
Stopped at: Completed 10-06-PLAN.md
Resume file: None
