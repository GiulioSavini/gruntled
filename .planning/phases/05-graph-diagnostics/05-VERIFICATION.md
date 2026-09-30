---
phase: 05-graph-diagnostics
verified: 2026-09-30T00:00:00Z
status: gaps_found
score: 4/4 roadmap criteria verified mechanically; 1 zero-false-positive gap (GRT003 on config_path = "")
gaps:
  - truth: "GRT003 carries the same zero-false-positive guarantee as GRT001 (goal clause)"
    status: partial
    reason: "dependency { config_path = \"\" } is reported as a GRT003 self-loop, but terragrunt v1.1.6 reports 'config_path could not be resolved' and no cycle; a single-unit run in that unit exits 0."
    artifacts:
      - path: "internal/infrastructure/terragrunt (config_path resolution)"
        issue: "empty literal resolves to the unit's own dir, producing a self-edge"
      - path: "cmd/gruntled/testdata golden grt002_missing_target"
        issue: "pins the self-loop behaviour"
      - path: "docs/validation.md (Open issue: config_path = \"\" on a block, ~line 682)"
        issue: "records it as open, not fixed"
    missing:
      - "Treat empty-string literal config_path as unresolvable (silent), or report it under a separate code"
      - "Update the golden and the docs section, and add a unit test"
---

# Phase 5: Graph Diagnostics Verification Report

**Phase Goal:** Users see wiring mistakes that the existing graph already answers, a dependency pointing at no unit and a dependency cycle, with the same zero-false-positive guarantee as `GRT001`.
**Status:** gaps_found
**Re-verification:** No, initial verification

## Automated checks (run by verifier)

- `go vet ./... && go test -count=1 ./... && bash scripts/check-architecture.sh`: all packages ok, `architecture: OK`.
- Env-gated corpus command from 05-VALIDATION.md (primary, secret, denis256, terragrunt v1.1.6 binary): `TestCorpusGraphClean`, `TestCorpusGraphMutation` (41.8s), `TestCorpusClean`, `TestCorpusMutation`, `TestDenis256Corpus` all PASS.

## Success criteria

| # | Criterion | Status | Evidence |
|---|-----------|--------|----------|
| 1 | GRT002 on literal missing-target config_path, silent on non-literal/unresolvable | VERIFIED | `analysis/grt002.go` + tests, golden suite green, loader TargetState (only `fs.ErrNotExist` gives DirMissing, else Unknown) |
| 2 | GRT003 once per cycle, lexically smallest start, deterministic | VERIFIED | `analysis/grt003.go` + tests, ring/non-ring/self-loop goldens |
| 3 | Unmutated iso20022 and secret clean; denis256 == oracle set | VERIFIED | TestCorpusGraphClean, TestDenis256Corpus pass |
| 4 | Every mutation caught; results recorded in docs/validation.md | VERIFIED | TestCorpusGraphMutation pass; TestValidationDocPins pass in full suite |

## Requirements coverage

| ID | Plans | Status |
|----|-------|--------|
| MORE-01 | 05-01..05-05 | SATISFIED |
| MORE-02 | 05-01, 05-02, 05-04, 05-05 | SATISFIED |
| MORE-06 | 05-01, 05-06 | SATISFIED on the corpus; see gap on the guarantee |

No orphaned requirements: REQUIREMENTS.md maps only MORE-01, MORE-02, MORE-06 to Phase 5, and all appear in plan frontmatter.

## Judgement: `config_path = ""` reported as GRT003 self-loop

**Classification: gap (low severity, narrow input), not an acceptable documented limitation.**

Evidence:
- The goal states the same zero-false-positive guarantee as GRT001, meaning gruntled must not assert something terragrunt contradicts. GRT003 asserts "a dependency cycle exists". For this input terragrunt v1.1.6 says the opposite: no cycle, the `config_path` "could not be resolved" (docs/validation.md ~682-698, verified by 05-06 on a scratch tree).
- Terragrunt does not uniformly reject the input: a single-unit `run` in `a` exits 0, while gruntled `check` exits 1 with a cycle message. That is a false positive against a working invocation. Under `run --all` both tools fail, but for different reasons, so the diagnostic text is factually wrong.
- Not a documented limitation: docs/validation.md and STATE.md label it an "open issue" with a candidate fix, and the golden `grt002_missing_target` pins the wrong behaviour. The project's own conventions (fail-silent on non-literal/unresolvable) point to the fix: treat `""` as unresolvable and stay silent.
- Mitigating: no corpus contains an active empty `config_path` (the only two, in denis256 `deep-merge-fix/common.hcl`, are commented out), so all corpus criteria pass. Impact is limited to hand-written edge input, so this is not a blocker for the four success criteria, but it should be closed before claiming the guarantee.

Recommended: a small gap-closure plan (`/gsd:plan-phase 5 --gaps`) to make empty literal silent (or a distinct code), update the golden, the docs section, and STATE.md.

## Anti-patterns

None blocking found in the reviewed paths; no stubs, all analyzers wired through `checking`.

## Human verification

None required.

_Verified: 2026-09-30_
_Verifier: Claude (gsd-verifier)_
