---
phase: 04-real-repo-validation-experiment
plan: 01
subsystem: validation-testing
tags: [golden, txtar, synthrepo, oracle, valid-02]

requires:
  - phase: 03-grt001-diagnostic-cli
    provides: "run(args, stdout, stderr) composition root, JSON schema v1"
  - phase: 02-parsing-graph-construction
    provides: "gap closure 02-06 (lazy guards), 02-07 (stack / non-default config_path), 02-13 (include-target rules)"
provides:
  - "cmd/gruntled/golden_test.go: TestGoldenSynthrepoFullScale, TestGoldenFixtures, TestGoldenFixtureCount"
  - "cmd/gruntled/testdata/golden/: 10 hand-written txtar fixture repositories"
affects: [04-03]

tech-stack:
  added: []
  patterns:
    - "txtar golden fixture with reserved _golden/{want,exit,unknown,symlinks} sections"
    - "Want-position self-check against fixture bytes before gruntled runs (GRT001 -> 'dependency.', GRT100 -> '@')"

key-files:
  created:
    - cmd/gruntled/golden_test.go
    - cmd/gruntled/testdata/golden/live_clean.txtar
    - cmd/gruntled/testdata/golden/live_broken.txtar
    - cmd/gruntled/testdata/golden/shared_include.txtar
    - cmd/gruntled/testdata/golden/remote_local_mix.txtar
    - cmd/gruntled/testdata/golden/tf_json_surface.txtar
    - cmd/gruntled/testdata/golden/symlinked_module_files.txtar
    - cmd/gruntled/testdata/golden/mock_shapes.txtar
    - cmd/gruntled/testdata/golden/lazy_guards.txtar
    - cmd/gruntled/testdata/golden/syntax_errors_mixed.txtar
    - cmd/gruntled/testdata/golden/dependency_edges.txtar
  modified: []

key-decisions:
  - "Golden identifiers all prefixed golden*; the harness reuses nothing from e2e_test.go so 04-02 cannot collide"
  - "GRT100 unit is '-': hclconv.FirstSyntaxError returns a zero-Unit diagnostic and internal/application/indexing/build.go passes load diagnostics through unchanged"
  - "lazy_guards ternary/&& conditions use local.flag (get_env-derived) so the guarded branch is genuinely not provably evaluated"

requirements-completed: [VALID-02]

duration: 20min
completed: 2026-09-29
---

# Phase 4 Plan 01: Golden tests (VALID-02) Summary

**Always-on golden suite: 4 synthrepo trees at 400-600 units (12/25/8 BadOutputRef plus one clean 500-unit tree) match Manifest.Expected exactly, and 10 fresh hand-written txtar repositories match hand-counted, byte-self-checked diagnostic sets, exit codes and unknown-unit sets, each run twice with byte-identical JSON.**

## Performance

- Duration: ~20 min
- Tasks: 2/2
- Files: 11 created

## Accomplishments

- `TestGoldenSynthrepoFullScale`: seed1 (400u, depth 4, fanout 5, 12 errors), seed2 (400u, depth 2, fanout 8, 25 errors), seed3 (600u, depth 3, fanout 3, 8 errors), clean (500u, depth 4, fanout 6). Exact set equality, exit code, summary.units, zero unknown units, repeat-run byte identity. No spec needed reducing.
- `TestGoldenFixtures`: txtar harness; set equality with multiplicity, exact positions for every code (no wildcard), optional `_golden/unknown` exact set, optional `_golden/symlinks`, determinism check. No update flag.
- Self-check verified to bite: shifting a GRT001 column by one fails with "bytes at 14:15 do not start with dependency."; deleting a want line fails with a "reported but not wanted" diff.
- `TestGoldenFixtureCount`: at least 10 fixtures, added with the seventh-through-tenth fixtures so no pushed commit was red.

## Fixture results (all matched the hand-computed want on first run)

| Fixture | Want | Exit | Proves |
|---|---|---|---|
| live_clean | none | 0 | unit != module, include via find_in_parent_folders, two envs |
| live_broken | 2 GRT001 | 1 | renamed refs caught, sibling valid refs silent |
| shared_include | 3 GRT001 at `_envcommon/app.hcl:6:13` (apps/a,b,c) | 1 | per-including-unit reporting, get_terragrunt_dir in included scope, find_in_parent_folders with dir segment |
| remote_local_mix | 1 GRT001 + unknown {live/dns, live/vpc module-unknown remote-source} | 1 | remote surfaces silent |
| tf_json_surface | 1 GRT001 | 1 | .tf.json surface read |
| symlinked_module_files | 1 GRT001 | 1 | symlinked .tf in module dir read |
| mock_shapes | 2 GRT001 (c_corpus, c_validate_only) | 1 | DIAG-03: mocks never suppress; skip_outputs/enabled=false silent |
| lazy_guards | 1 GRT001 | 1 | try/can/ternary/&&/for silent (02-06 G1) |
| syntax_errors_mixed | 2 GRT100 at `@` (4:1, unit -) + 1 GRT001 | 1 | one GRT100 per broken file, broken-module surface silent |
| dependency_edges | 1 GRT001 + unknown {edge/dup config-unknown invalid-dependency} | 1 | no-dir, stack, non-default file, undeclared label silent |

No fixture uses a dynamic include file name or `terragrunt.hcl` as include target, so the 02-13 include-target widening does not apply to any of them.

## Task Commits

1. Task 1: harness, full-scale synthrepo goldens, first three fixtures - `216ea30`
2. Task 2: remaining seven fixtures and fixture-count guard - `5ed436b`

## Deviations from Plan

None - plan executed exactly as written. No gruntled disagreement with any hand-computed want (no experiment failure recorded).

## Verification

gofmt -l (empty), go mod tidy -diff, go vet ./..., staticcheck v0.8.1 ./..., go test -count=1 ./..., scripts/check-architecture.sh: all green. ci.yml untouched. Architecture scripts not touched, so test-check-architecture.sh not run.

## Self-Check: PASSED

All 11 created files present; commits 216ea30 and 5ed436b on origin/master.
