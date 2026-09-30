---
phase: 05-graph-diagnostics
plan: 05
subsystem: cli-goldens-docs
tags: [goldens, docs, grt002, grt003]
requires:
  - phase: 05-graph-diagnostics
    provides: "05-02 GRT002/GRT003 analyzers; 05-03 target state; 05-04 dependencies paths"
provides:
  - "Hand-reviewed dependency_edges golden with GRT002"
  - "Goldens grt002_missing_target, grt002_shared_include, grt003_cycles (positions and exact messages)"
  - "Golden harness: optional _golden/messages section; GRT002/GRT003 self-check (position is an opening quote)"
  - "docs/cli.md GRT002/GRT003 sections, decision tables, rejected alternatives, limitations; README codes table"
affects: [05-06]
tech-stack:
  added: []
  patterns:
    - "_golden/messages: `CODE file:line:col message`, exact multiset over all diagnostics when present"
key-files:
  created:
    - cmd/gruntled/testdata/golden/grt002_missing_target.txtar
    - cmd/gruntled/testdata/golden/grt002_shared_include.txtar
    - cmd/gruntled/testdata/golden/grt003_cycles.txtar
  modified:
    - cmd/gruntled/golden_test.go
    - cmd/gruntled/testdata/golden/dependency_edges.txtar
    - cmd/gruntled/main.go
    - docs/cli.md
    - README.md
    - internal/infrastructure/terragrunt/parse.go
key-decisions:
  - "dependency_edges GRT002 on ../nodir is correct: the dir exists with only README.md, so the message is 'directory has no terragrunt.hcl' (the plan guessed 'does not exist'); golden updated by hand, code untouched"
  - "config_path = \"\" on a block resolves to the unit's own dir: no GRT002, GRT003 self-loop (Terragrunt's filepath.Join gives the same); pinned in grt002_missing_target"
  - "check -h exit-code line now names GRT001-GRT003; TestHelpMatchesDocs pins it to docs/cli.md"
requirements-completed: []
duration: 15min
completed: 2026-09-30
---

# Phase 5 Plan 05: Goldens and Docs Summary

GRT002/GRT003 end-to-end output is locked by three new txtar goldens (positions and exact messages) plus the hand-reviewed dependency_edges update, and docs/cli.md documents both codes with decision tables mirroring the analyzer doc comments.

## Tasks

| Task | Name | Commit |
| ---- | ---- | ------ |
| 1 | Goldens + harness messages section | 96f0a27 |
| 2 | cli.md / README / check -h | 6346c65 |
| fix | staticcheck U1000 on pathsInvalid | ceebb76 |

## Golden review

- **dependency_edges** (only changed golden): new line `GRT002 error edge/consumer/terragrunt.hcl:2:17`. Correct, not a false positive: `edge/nodir` exists but holds only README.md, classifyTarget gives TargetNoConfig and Terragrunt would refuse the dependency. `../stack` is unresolved (config-path-stack) and `../target/alt.hcl` unresolved (config-path-nondefault-file), both silent; `edge/dup` is config-unknown. Exit stays 1 (GRT001 already present). Edited by hand; the harness has no update flag. Messages pinned.
- **grt002_missing_target**: DirMissing, module-only NoConfig, skip_outputs=true still fires, paths entry missing fires; `get_env(...)` config_path, enabled=false, paths `""` silent. Block `config_path = ""` gives a GRT003 self-loop (see decisions).
- **grt002_shared_include**: `_envcommon/db.hcl` with `${get_terragrunt_dir()}/../vpc`, two including units, two GRT002 at `_envcommon/db.hcl:2:17` attributed to live/app1 and live/app2.
- **grt003_cycles**: `"."` and `"../named"` self-loops, 2-ring, 3-unit "among" SCC, block+paths pair gives one ring, disabled back edge gives nothing.
- All other GRT001/GRT100 goldens unchanged and green.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 3 - Blocking] Golden harness could not express GRT002/GRT003**
- **Found during:** Task 1
- **Issue:** goldenSelfCheck fataled on any code other than GRT001/GRT100, and the harness compared no messages, so "both GRT002 messages" and ring vs among could not be locked. No `-update` flag exists (by design).
- **Fix:** GRT002/GRT003 self-check rule (position must be `"`); optional `_golden/messages` section compared as an exact sorted set.
- **Commit:** 96f0a27

**2. [Rule 1 - Bug] check -h exit-code line would contradict docs**
- **Found during:** Task 2
- **Issue:** TestHelpMatchesDocs requires the doc's exit-code lines to equal `check -h`; updating the docs alone breaks it, and help listed only GRT001, GRT100.
- **Fix:** main.go help line `(GRT001-GRT003, GRT100)`, same in both cli.md blocks.
- **Commit:** 6346c65

**3. [Rule 3 - Blocking] staticcheck U1000 from 05-04**
- **Found during:** full suite
- **Issue:** `pathsInvalid` const unused (zero value used implicitly), staticcheck failed.
- **Fix:** parsePathsDecl initialises `shape: pathsInvalid` explicitly.
- **Commit:** ceebb76

## Verification

- `go test -count=1 ./cmd/gruntled -run 'TestGolden|TestE2E'` OK
- Task 2 grep checks OK; `TestValidationDocPins` OK (validation.md untouched)
- Full suite: gofmt OK, `go mod tidy -diff` OK, `go vet` OK, staticcheck OK, `go test -count=1 ./...` OK, `check-architecture.sh` OK, `test-check-architecture.sh` OK (all self-tests passed)

## Requirements

MORE-01/MORE-02 not marked complete: corpus validation (05-06) pending.

## Self-Check: PASSED
