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

## Cross-Milestone Trends

### Process Evolution

| Milestone | Sessions | Phases | Key Change |
|-----------|----------|--------|------------|
| v0.2 | n/a | 3 | Proofs moved into CI jobs; terragrunt binary used as oracle for new diagnostics |
| v0.3 | 2+ | 5 | Agent team on GSD Core; sec plan review + cross-phase audit; gap-closure phase |

### Cumulative Quality

| Milestone | Tests | Coverage | Zero-Dep Additions |
|-----------|-------|----------|-------------------|
| v0.2 | ~17.9k LOC of Go tests | not measured | 0 new runtime dependencies |
| v0.3 | ~25.9k LOC of Go tests | not measured | fsnotify (non-windows) only |

### Top Lessons (Verified Across Milestones)

1. Corpus validation against a real oracle finds what fixtures miss (v0.1 denis256 recall fix, v0.2 empty `config_path`).
2. Independent cross-phase review finds what per-phase verification misses (v0.3 dir-ignore BLOCKER, symlink alias MEDIUM).
