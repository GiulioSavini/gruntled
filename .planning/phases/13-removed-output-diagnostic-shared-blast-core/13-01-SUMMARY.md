---
phase: 13-removed-output-diagnostic-shared-blast-core
plan: 01
subsystem: domain-analysis
tags: [grt004, grt001, blast, decision-table, mutation-test, refactor]
requires:
  - phase: 03
    provides: UnknownOutputs (GRT001) with the DIAG-03 decision table
  - phase: 09
    provides: impact.Compute and the blast comparison GRT004 will plug into (13-02)
provides:
  - diagnostic.CodeRemovedOutput ("GRT004")
  - resolveReference / resolveDependency, the shared rows 1-5 of DIAG-03
  - analysis.RemovedOutputs(base, cur) with four named, individually necessary preconditions
  - analysis.SupersedeUnknownOutputs(cur, removed) keyed on (GRT001, Unit, Pos)
  - diag03Rows() shared test table
affects: [13-02, 13-03]
tech-stack:
  added: []
  patterns: [named precondition table over independently computed facts, with a check-removal necessity test]
key-files:
  created:
    - internal/domain/analysis/grt004.go
    - internal/domain/analysis/grt004_test.go
    - internal/domain/analysis/grt004_internal_test.go
  modified:
    - internal/domain/diagnostic/diagnostic.go
    - internal/domain/diagnostic/diagnostic_test.go
    - internal/domain/analysis/grt001.go
    - internal/domain/analysis/grt001_test.go
key-decisions:
  - "The GRT004 message is built from resolveDependency(cur), not resolveReference(cur). They hold the same values whenever fires holds. This keeps the necessity mutants able to emit."
  - "A site whose cur dependency does not resolve is skipped before the message is built. fires and sameModule each imply that resolveDependency(cur) is ok, so the skip never masks a check that a one-check-removed mutant drops."
  - "The never-adds property uses a fixed in-test xorshift over 3000 cases instead of rapid. The domain test allowlist (check-architecture.sh:278-297) permits only stdlib plus testing/reflect. Asked the planner on bus #158."
  - "TestRemovedOutputsDeterministic shuffles with fixed rotations, forwards and reversed, for the same allowlist reason (math/rand/v2 failed domain-stdlib-allowlist)"
requirements-completed: []
duration: 12min
completed: 2026-10-10
---

# Phase 13 Plan 01: GRT004 Domain Core Summary

**GRT001's rows 1-5 of DIAG-03 now live in `resolveReference` (rows 1, 4 and 5 in `resolveDependency`), with no change to output. On top of them sit the GRT004 code, the two-graph `RemovedOutputs` analyzer and `SupersedeUnknownOutputs`, which uses the structured key. Each GRT004 precondition has a matrix row that kills it, and the automated necessity test names that row.**

## Performance

- Duration: ~12 min
- Tasks: 3
- Files: 3 created, 4 modified

## Accomplishments

- `diagnostic.CodeRemovedOutput = "GRT004"` (diagnostic.go:31). It is not added to `checking.analyzers`, because that list takes one graph.
- grt001.go:80 `resolveDependency` covers rows 1, 4 and 5. grt001.go:107 `resolveReference` adds row 2 (:113) and row 3 (:116). `UnknownOutputs` now calls `resolveReference(g, ur)` (grt001.go:45). The message, suffix, order and error path are unchanged. The package doc gains one line: rows 1-5 are shared with GRT004.
- grt004.go: `removedOutputChecks` (:42) is the precondition list fires, baseHadRef, sameModule, baseDeclared. The facts are computed independently at :87-90. The base side is `resolveDependency(base, ur.Unit, dep)`, computed for every site and never gated on baseHadRef (sec #132). The base reference map (:72) answers baseHadRef and nothing else.
- GRT004 message (:101-104): `dependency "<label>" output "<Y>" was removed from module "<module>" (target unit "<target>")`. All four names go through `strconv.Quote`. The GRT001 mock suffix is added iff `mockMasksAtApply(cur options, Y, cur surface)` (:105).
- `SupersedeUnknownOutputs` (grt004.go:141) builds a multiset of cur GRT001s keyed on `siteKey{Unit, Pos}`. Pos includes the file. Each GRT004 consumes one GRT001, and a GRT004 with nothing to consume is dropped (:168). Messages are never compared. The doc comment states the References invariant: at most one GRT001 per (Unit, Pos) (sec #136).
- The TestDIAG03 rows moved verbatim to `diag03Rows()` (grt001_test.go). `TestDIAG03RowCount` pins 22 rows (:310).

### Necessity test (automated, TestRemovedOutputsChecksAreNecessary)

```
check "fires" killed by: still declared in cur; cur enabled false; cur enabled unknown; cur skip_outputs true; cur skip_outputs unknown; unrelated never-declared output in cur only
check "baseHadRef" killed by: reference added in this change
check "sameModule" killed by: dependency re-pointed to another module; target unit's source changed
check "baseDeclared" killed by: base module never declared id; same line:col in two files of one unit
```

Each check also has to be killed by its named row: fires by "still declared in cur", baseHadRef by "reference added in this change", sameModule by "dependency re-pointed to another module", baseDeclared by "base module never declared id". Only one row kills baseHadRef. That is the situation sec #132 warned about, and ungated base facts are what keep the check from being vacuous.

### Manual mutations (run, then reverted)

| Mutation | Failing tests / rows |
|----------|----------------------|
| sameModule `==` flipped to `!=` (grt004.go:89) | Matrix: removed, base references id at another position, re-pointed to another module, target unit's source changed, re-pointed within the same module, base enabled false, both mock rows, two references, same line:col in two files, shared include; Necessity: fires, baseHadRef, baseDeclared; Parity: row7a-row7j; Deterministic |
| suffix unconditional (`if true`) | Matrix: removed, re-pointed within the same module, base enabled false, mock apply not allowed, two references, same line:col in two files, shared include; Parity: row7a, 7c, 7d, 7h, 7i, 7j |
| suffix never (`if false`) | Matrix: cur mock covers id, merge true, apply allowed; Parity: row7b, 7e, 7f, 7g |
| supersede key on Unit only | Supersede: only the GRT001 at the same Pos is replaced; same line:col in another file; NeverAddsProperty |
| supersede key without Position.File | Supersede: same line:col in another file of the unit; NeverAddsProperty |

## Task Commits

1. Task 1 red: `ba3cb3c` test(13-01): pin GRT004 code and share the DIAG-03 rows
2. Task 1 green: `cb90e5b` refactor(13-01): extract resolveReference from GRT001 and add GRT004 code
3. Task 2 red: `c009525` test(13-01): GRT004 precondition matrix, DIAG-03 parity and necessity test
4. Task 2 green: `ba67424` feat(13-01): RemovedOutputs (GRT004) over two graphs with named preconditions
5. Task 3 red: `1cb395c` test(13-01): SupersedeUnknownOutputs table and never-adds property
6. Task 3 green: `7e3ff16` feat(13-01): SupersedeUnknownOutputs by (Unit, Pos) structured key

## Verification

- Task 1 gate: `go test -count=1 ./internal/domain/diagnostic/ ./internal/domain/analysis/` ok. `go test -count=1 -run 'Golden|TestScripts|Corpus|Denis|Sarif|Graph' ./cmd/gruntled/` ok. `git diff --exit-code v0.3.0 -- cmd/gruntled/testdata` is empty.
- `TestRemovedOutputsMatrix`: 27/27 rows PASS. `TestRemovedOutputsDIAG03Parity`: 22/22 PASS. `TestRemovedOutputsDeterministic` and `TestRemovedOutputsChecksAreNecessary` (4/4) PASS.
- `TestSupersedeUnknownOutputs`: 7/7 PASS. `TestSupersedeNeverAddsProperty`: `3000 cases: 2830 with >=1 replacement, 2830 with >=1 dropped GRT004`. Another seed gave 2822/2826, so the equal counts above are a coincidence.
- `go test -count=1 ./...`: every package ok (cmd/gruntled 13.5s, analysis 0.22s, ...).
- `go vet ./...`: clean.
- `bash scripts/check-architecture.sh`: `architecture: OK (4 domain packages, 5 application packages, 1 interfaces packages)`.
- `bash scripts/test-check-architecture.sh`: `all architecture self-tests passed`.
- `git diff --exit-code v0.3.0 -- cmd/gruntled/testdata` after all three tasks: empty. `git diff v0.3.0 --stat -- cmd internal docs` touches only the 7 files listed above.
- `-race` was not run locally: there is no gcc and no cgo. CI runs it.

## Threat Mitigations

| ID | Disposition | Where |
|----|-------------|-------|
| T-13-01-1 | mitigated | grt001.go:45 (single resolveReference call), :80-102 (rows 1, 4, 5 in the same order as before), :113/:116 (the same tristate comparisons); TestDIAG03 unchanged, 22 rows pinned (grt001_test.go:310); cmd goldens green; `git diff v0.3.0 -- cmd/gruntled/testdata` empty |
| T-13-01-2 | mitigated | grt004.go:87 (fires = resolveReference(cur) ok and output undeclared); TestRemovedOutputsDIAG03Parity (grt004_test.go:17); the necessity test kills fires on "still declared in cur" (grt004_internal_test.go:463) |
| T-13-01-3 | mitigated | grt004.go:83 (base resolveDependency computed unconditionally), :88-90 (baseHadRef / sameModule / baseDeclared); matrix rows: reference added, re-pointed to another module, target source changed, base module/target unknown or unresolved (grt004_internal_test.go:421) |
| T-13-01-4 | mitigated | grt004.go:154/:167/:184 (key is siteKey{Unit, Pos} on GRT001/GRT004 codes only; no message read); TestSupersedeUnknownOutputs "message never matched", "same line:col in another file"; unit-only and file-less key mutants killed |
| T-13-01-5 | accepted | one map of base references; O(base refs + cur refs) |
| T-13-01-6 | mitigated | grt004.go:101-104 (RepoPath strings and labels, each strconv.Quote'd); the matrix "removed" row asserts the exact message |

## Deviations from Plan

**1. [Rule 3 - blocking] No rapid or math/rand in domain tests**
- The plan names rapid for `TestSupersedeNeverAddsProperty`. `domain-stdlib-allowlist` (scripts/check-architecture.sh:278-297) allows only stdlib plus testing/reflect in `internal/domain` tests, and the first try with `math/rand/v2` in `TestRemovedOutputsDeterministic` failed the gate. Both tests now use deterministic generators: a fixed in-test xorshift (3000 cases) and fixed rotations, forwards and reversed. The plan's generator constraints (at most one GRT001 and at most one GRT004 per (Unit, Pos)) and the non-vacuity counters are kept. The allowlist is unchanged. Question to the planner: bus #158. No answer arrived before handoff.
- The rand fix went into the Task 2 green commit (`ba67424`), not into its red commit.

**2. [Clarification] GRT004 message source**
- The plan builds the message from the cur `resolveReference` result. The code uses `resolveDependency(cur)` (grt004.go:81), which gives the same dep, module, target and surface whenever fires holds. This lets the fires-removed mutant emit, so the fires check stays provably necessary.

## Known Risks / Unverified

- `-race` not run locally (no gcc); CI runs it on linux/macos/windows.
- `requirements-completed` is `[]`: MORE-03 needs 13-02 (wiring into `blasting.Between`) before anything observable changes.

## Self-Check: PASSED
