# Project Retrospective

*A living document updated after each milestone. Lessons feed forward into future planning.*

## Milestone: v0.3 — Watch & Blast

**Shipped:** 2026-10-10 (phases 8-11 on 2026-10-08; audit, phase 12 gap closure and close on 2026-10-10)
**Phases:** 5 (8-12) | **Plans:** 24 | **Commits:** 145 since v0.2.0 | **Sessions:** 2+

### What Was Built
- Persistent parse store with `Invalidate`, proven equal to a full rescan by a rapid stateful property
- `gruntled blast --base`: Broken vs Impacted (one-hop surface change), multiset baseline
- `gruntled watch` (fsnotify / `--poll`, debounce, safety net, atomic status file) and `gruntled report`
  over raw-syscall AF_UNIX (report file on windows), single instance with crash recovery
- Phase 12 gap closure: editor patterns apply to files only, canonical-path cache validation, runtime dir
  and status path refused inside the repo by file identity, terminal/bidi escaping on every output

### What Worked
- A cross-phase milestone audit by independent agents: the integration checker reproduced a BLOCKER live
  (daemon stale forever for `live/2024/`) and sec found a MEDIUM (symlink alias cache), both invisible to
  four `passed` phase verifications
- Plan review by sec before execution: two rounds caught a property test that would have passed on the
  buggy code (#35), the Emacs lock symlink case and a JSON escaping blind spot
- Mutation proofs as a standing rule: every fix was shown to fail its test when reverted
- A shared bus with explicit message types made agent hand-offs auditable; deviations and questions were
  answered on the bus instead of guessed
- Pushing after each plan let CI run `-race`, macOS and Windows early; that surfaced the fsnotify kqueue
  bug (upstream #787) and the staticcheck/go1.27.2 incompatibility the same day

### What Was Inefficient
- The property test never generated directory names matching file ignore patterns or symlinked includes;
  "incremental == full" was proven over a generator that missed the real-world cases
- Ignore rules were written per path without the entry type, and three adapters inherited the mistake
- Agent definitions created mid-session are not loaded until restart, so the team ran as general-purpose
  agents adopting their definition files; lifecycle events had to be posted by hand
- GSD verification digests went stale after small post-verification edits (doc paragraph, YAML fix),
  requiring two verifier re-runs before the milestone could be archived
- `gsd-tools milestone complete` again wrote a MILESTONES entry that had to be rewritten (wrong task count)
- No local gcc again: `-race` only in CI; `test-check-architecture.sh` still slow locally

### Patterns Established
- Milestone close = integration check + independent security audit + re-audit after gap closure
- Every plan has a `<threat_model>`; sec reviews plans before execution and audits code after
- Ignore/filter predicates take the entry type; property generators include adversarial names
- Upstream bugs found in dependencies get a skip with an issue link, a documented limitation and an issue

### Key Lessons
1. A property test is only as good as its generator: add the names and file kinds real repositories use
   (dates as directories, symlinks) and prove non-vacuity with counters and a mutation.
2. Run the cross-phase audit before declaring a milestone done; per-phase verification is not enough for
   long-lived components that compose several phases.
3. Finish doc edits before running the verifier, or budget a re-verification.
4. Install a C toolchain (or use a container) so `-race` runs before push.

### Cost Observations
- Model mix: Opus for orchestrator and all agents
- Sessions: v0.3 execution (2026-10-08) plus one agent-team session (2026-10-10)
- Notable: phase 12 (5 plans, 2 plan-review rounds, code audit, 2 re-verifications) in one session

---

## Milestone: v0.2 — CI-Ready

**Shipped:** 2026-10-08 (execution 2026-09-29 to 2026-10-01; audit and close 2026-10-08)
**Phases:** 3 (5-7) | **Plans:** 17 | **Commits:** 107 | **Sessions:** not recorded

### What Was Built
- `GRT002` (dependency to no unit) and `GRT003` (dependency cycle) as pure analyzers over the existing graph,
  validated on the pinned corpus against terragrunt v1.1.6 and an independent oracle (13/13 mutations caught)
- `gruntled graph --json` and `gruntled check --format sarif` (SARIF 2.1.0, accepted by GitHub code scanning)
- `gruntled --version` via ldflags, `scripts/build-release.sh` for 6 static targets plus `checksums.txt`
- Tag-triggered `release.yml`; release `v0.2.0` published on the first real run
- `gruntled-check` pre-commit hook and `docs/ci.md` recipes, executed by the `recipe-check` CI job

### What Worked
- Ordering by dependency (final rule set, then output formats, then distribution) meant SARIF and the recipes
  never had to chase a moving rule list
- Using terragrunt v1.1.6 itself as the oracle settled ambiguous cases (empty `config_path`) with evidence
  instead of opinion; the 05-07 gap closure came straight out of that comparison
- Proofs as CI jobs: SARIF schema check, upload-sarif proof, packaging dry run on every push, `recipe-check`.
  The real release pipeline went green on its first run because every step had already run in CI
- Doc-sync tests (`TestSARIFDoc`, `TestCIDoc`, `TestReleaseTargetsInSync`, help-vs-docs) kept docs and code equal
- Phase verification caught everything requirement-level; the milestone audit found 9/9 satisfied

### What Was Inefficient
- `gsd-tools state advance-plan` repeatedly failed with "Cannot parse Current Plan" on STATE.md, and
  `roadmap update-plan-progress` was a silent no-op, so executors edited STATE.md and ROADMAP.md by hand
- SUMMARY frontmatter `requirements-completed` was mostly left empty or missing (only 05-06 filled it), so
  the audit had to re-derive coverage from VERIFICATION files
- `scripts/test-check-architecture.sh` takes ~18 minutes locally, which makes it a poor pre-push check
- No C compiler locally, so `go test -race` only ran in CI
- Nyquist validation left partial for phases 5 and 6
- `gsd-tools milestone complete` wrote a near-empty MILESTONES entry ("1 tasks", no accomplishments) that had
  to be rewritten by hand; README still described v0.1 until after the audit

### Patterns Established
- Validate every new diagnostic against the pinned terragrunt binary on the corpus, not only against fixtures
- Any doc that users copy (recipes, help text, SARIF rule titles) is executed or pinned by a test
- A release target list lives in exactly two places with a test keeping them equal
- Release workflow re-proves the tagged commit itself rather than trusting another workflow's result

### Key Lessons
1. Don't rely on gsd-tools state/roadmap updaters without checking their effect; verify the file changed, or
   edit by hand and say so in the SUMMARY.
2. Make filling `requirements-completed` part of each plan's done criteria, so the audit is a lookup.
3. Split or speed up `test-check-architecture.sh` before v0.3 adds the daemon packages.
4. Get a C toolchain locally (or a container) so `-race` runs before push, not only in CI.

### Cost Observations
- Model mix: not recorded
- Sessions: not recorded
- Notable: 17 plans in 3 days of execution; most plans 5-20 minutes, longest 05-07 (~65 min incl. checkpoint)

---

## Milestone: v0.4 — Blast-aware Diagnostics

**Shipped:** 2026-10-13 (phases 13-16 execution 2026-10-10 to 2026-10-12; phase 17 paused mid-execution
2026-10-12, resumed and closed 2026-10-13; audit and close same day)
**Phases:** 5 (13-17) | **Plans:** 23 | **Commits:** 217 since v0.3.0 | **Sessions:** 3+

### What Was Built
- `GRT004` replaces `GRT001` at every reference to an output a change removed, through one shared
  blast core (`Between`) reused by `check`/`report`/`blast`/the daemon
- Transitive Impacted follows dependents past one hop, with distance, one connecting path and `--depth`
- Type-level surface facts: new required variables, variable/output type or `sensitive` changes land
  in Impacted, never Broken; a 64 MiB blast-response size bound and escaping on every new surface
- Daemon answers `report --blast` against its in-memory baseline over IPC v2; `rebase [--expect N]`
  moves the baseline, which holds only `checking.Report`
- A real corpus mutation proof (REL-03): byte-identical `check`, empty noise on reformat-only
  mutations, 100% catch of the injected mutation set on three real corpora, network-isolated against
  a frozen, fingerprinted v0.3.0 reference binary — plus an independent cross-phase security and
  integration review with zero open BLOCKER/MEDIUM findings

### What Worked
- The corpus mutation proof's reference binary pinned by content (fingerprinted sha matching the
  v0.3.0 tag), not just a version string, and the gate re-exec'd into a network namespace
  (`unshare -rn`) to make "network-isolated" a verified property instead of a claim
- Explicit `.continue-here.md` pause notes (exact commit SHAs, exact CI/workflow run IDs, a numbered
  "Next" list) let a cold resume after a multi-day pause (phase 17, 2026-10-12 → 2026-10-13) pick up
  with zero re-derivation from git log alone
- Nyquist validation as a genuine post-execution audit: every task's automated command was
  cross-checked against its SUMMARY evidence rather than trusting pre-execution `nyquist_compliant:
  true` frontmatter — confirmed 0 gaps across all 17 tasks
- The milestone-level integration check and 3-source requirements cross-reference ran as two
  independent parallel agents and converged on the same verdict (17/17 satisfied, no wiring gaps) as
  phase 17's own internal review — redundant verification as confirmation, not rework
- Running the final gate one step at a time with `free -m` and `timeout` under 8 GB WSL caught a real
  failure (the flaky test below) without ever risking an OOM

### What Was Inefficient
- Local toolchain drift: `~/.local/go` is 1.24.2 and the scripts set `GOTOOLCHAIN=local`, so
  `compare-ref.sh` and friends refused to build the go1.27 module until the cached go1.27.0 toolchain
  was manually prepended to `PATH` — rediscovered mid-gate instead of being documented once
- `gsd-tools milestone complete` / the state writer again clobbered hand-maintained STATE.md
  frontmatter (dropped `current_phase`/`current_phase_name`/`state_head`, wrote a full sentence into
  the `status` enum field instead of a short value) — repaired by hand, continuing the v0.2/v0.3
  pattern of this tool needing a manual follow-up every time
- A pre-existing flaky test (`TestIncrementalEqualsFull`, from phase 12-05 in v0.3) only surfaced
  during the phase 17 final gate because `-short`'s reduced random sampling sometimes drew zero
  symlinked-include cases; CI never runs `-short`, so it had been invisible since v0.3 shipped
- The 17-05 plan's cached final-gate script (noted in the pause note as copied to
  `~/.cache/gruntled-phase4/resume/final-gate.sh`) had been lost by resume time, so the gate was
  re-run step-by-step from the plan's `<verify>` block instead of a saved script

### Patterns Established
- A mutation/corpus proof pins its reference binary by content fingerprint, not version string, and
  proves network isolation rather than asserting it
- Pause notes name exact commit SHAs and CI run IDs plus a numbered resume list, never just a prose summary
- Milestone-level audits re-verify specific wiring seams (actual call sites) instead of re-running
  whole phase suites, and run the integration check and the Nyquist/requirements cross-reference as
  independent parallel passes

### Key Lessons
1. Record the exact toolchain PATH fix in a script or dev doc, not an agent's scratch notes — it will
   be rediscovered the same way every session otherwise.
2. Budget a manual STATE.md frontmatter repair after every `gsd-tools milestone complete` / state-advance
   call; it has clobbered hand-maintained fields in v0.2, v0.3 and v0.4 alike.
3. A flaky test guarded only by CI's exact invocation (e.g. no `-short`) is a landmine for any
   local or `-short` gate; audit non-vacuity assertions against the real sample size, not just CI's path.
4. Don't rely on a cached resume script surviving between sessions — the plan's own `<verify>` block
   is the durable source of truth.

### Cost Observations
- Model mix: Opus for orchestrator/executor/verifier/sec agents, Sonnet for the integration-checker
  and the Nyquist/requirements cross-reference fork
- Sessions: at least 3 across 2026-10-10 to 2026-10-13, including one multi-day pause inside phase 17
- Notable: the milestone-level audit (integration-checker + 3-source cross-reference, run in parallel)
  completed in under two minutes wall-clock and found zero new gaps beyond phase 17's own review

---

## Cross-Milestone Trends

### Process Evolution

| Milestone | Sessions | Phases | Key Change |
|-----------|----------|--------|------------|
| v0.2 | n/a | 3 | Proofs moved into CI jobs; terragrunt binary used as oracle for new diagnostics |
| v0.3 | 2+ | 5 | Agent team on GSD Core; sec plan review + cross-phase audit; gap-closure phase |
| v0.4 | 3+ | 5 | Mutation proof network-isolated against a fingerprinted reference binary; parallel integration + Nyquist cross-reference at milestone audit |

### Cumulative Quality

| Milestone | Tests | Coverage | Zero-Dep Additions |
|-----------|-------|----------|-------------------|
| v0.2 | ~17.9k LOC of Go tests | not measured | 0 new runtime dependencies |
| v0.3 | ~25.9k LOC of Go tests | not measured | fsnotify (non-windows) only |
| v0.4 | ~42.4k LOC of Go tests | not measured | `ext/typeexpr` (already transitive via `hcl/v2`) only |

### Top Lessons (Verified Across Milestones)

1. Corpus validation against a real oracle finds what fixtures miss (v0.1 denis256 recall fix, v0.2 empty `config_path`).
2. Independent cross-phase review finds what per-phase verification misses (v0.3 dir-ignore BLOCKER, symlink alias MEDIUM).
3. `gsd-tools milestone complete` / state-advance tooling reliably clobbers hand-maintained STATE.md fields — budget a manual repair every milestone (v0.2, v0.3, v0.4).
4. A test's non-vacuity guard is only as strong as its actual sample size: `-short`'s reduced sampling can make a real-world case statistically invisible even though CI's full run always covers it (v0.4 `TestIncrementalEqualsFull`).
