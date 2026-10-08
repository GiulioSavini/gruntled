# Project Retrospective

*A living document updated after each milestone. Lessons feed forward into future planning.*

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

### Cumulative Quality

| Milestone | Tests | Coverage | Zero-Dep Additions |
|-----------|-------|----------|-------------------|
| v0.2 | ~17.9k LOC of Go tests | not measured | 0 new runtime dependencies |

### Top Lessons (Verified Across Milestones)

1. Corpus validation against a real oracle finds what fixtures miss (v0.1 denis256 recall fix, v0.2 empty `config_path`).
