---
phase: 03-grt001-diagnostic-cli
plan: 05
subsystem: cli-testing
tags: [e2e, oracle, determinism, no-writes, synthrepo, phase-gate]

requires:
  - phase: 03-grt001-diagnostic-cli
    provides: "gruntled check composition root (03-03), docs/cli.md (03-04)"
provides:
  - "cmd/gruntled/e2e_test.go: TestOracle, TestDeterministicAcrossCheckouts, TestNoWrites, TestMutationDiff, TestHelpMatchesDocs"
  - "README Status: Phases 1-3 complete, end-to-end guarantees listed"
  - "Phase gate result on the pinned primary corpus (e6c55d1)"
affects: [04]

tech-stack:
  added: []
  patterns:
    - "In-process e2e: run(args, &stdout, &stderr) with synthrepo trees in t.TempDir, no subprocess"
    - "Read-only snapshot order: chmod, snapshot, run, snapshot (Pitfall 11)"

key-files:
  created: [cmd/gruntled/e2e_test.go]
  modified: [README.md]

key-decisions:
  - "Mutation diff computed on decoded JSON diagnostics in the test (no application-internal import)"
  - "README Usage/intro/Diagnostic codes were already rewritten by 03-03; 03-05 only touched the phase sentence and the Phase 3 to-do paragraph"

requirements-completed: [DIAG-01, DIAG-04, CLI-03, CLI-04, CLI-05]

duration: 14min
completed: 2026-09-29
---

# Phase 3 Plan 05: End-to-end guarantees and phase gate Summary

**Five in-process e2e tests prove the synthrepo oracle is matched exactly (60 units, 4 BadOutputRef), output is byte-identical across checkout names and cwd, a read-only tree and empty HOME/XDG_CACHE_HOME/TMPDIR stay untouched, and `check -h` exit-code lines match docs/cli.md; the pinned corpus is clean and its role_name mutation yields exactly 8 GRT001s.**

## Performance

- Duration: ~14 min
- Tasks: 2/2
- Files: 1 created, 1 modified

## Task Commits

1. Task 1: e2e tests - `d93e63e`
2. Task 2: README status edit - `638c0ee`

## Accomplishments

- TestOracle: spec `{Units:60, IncludeDepth:3, DependencyFanout:3, Seed:7, 4x BadOutputRef}` accepted by Render unchanged; JSON GRT001 {file,line,column,unit} set equals Manifest.Expected, all severity error, no other codes, exit 1.
- TestDeterministicAcrossCheckouts: `checkout-a` and `another-name`, both formats, 2 runs by path + `check` + `check .` after t.Chdir: all stdout byte-identical, all exit 1, no temp path, no `__gruntled_repo_root__`, no absolute file/unit.
- TestNoWrites: runs (not skipped) as non-root on linux; (path, size, mode, mtime, sha256) snapshot unchanged; env dirs empty.
- TestMutationDiff: clean exit 0 / zero diagnostics; one mutation adds exactly the manifest entry, removes nothing; appending `output "zz_unrelated"` to the target module leaves JSON byte-identical.
- TestHelpMatchesDocs: exactly 4 `^  [0-3]  ` lines, each an exact line of docs/cli.md.

## Phase gate (pinned primary corpus)

- Corpus: `$HOME/.cache/gruntled-phase4/corpus/primary`, HEAD `e6c55d11fd1a01e75b78d7897be36c69fa26b8cc`; binary built to /tmp/gr-bin.
- Unmutated: exit 0, 0 stdout lines; stderr `gruntled: checked 65 units (3 unknown): 0 errors, 0 warnings`.
- Mutated (/tmp/iso-mut3, `output "role_name"` renamed only in `iac.src/s3_runtime/state.tf`): exit 1, 8 stdout lines, 8 GRT001, 8 with the `; mock_outputs supplies it, so apply would silently use the mock value (unit ...)` suffix; stderr `gruntled: checked 65 units (3 unknown): 8 errors, 0 warnings`.

## README diff

`git diff --stat README.md`: `1 file changed, 5 insertions(+), 5 deletions(-)`. The intro, `### Usage` (console example, exit-code table, docs/cli.md link) and Diagnostic codes sentence had already been rewritten in 03-03 (commit 1f568ac), so the plan's replacements were no-ops. Remaining edits: phase sentence now "Phases 1-3 are complete, Phase 4 hasn't started"; removed the "Still to do in Phase 3" paragraph; added a "Works today" bullet for the CLI end-to-end guarantees.

## Deviations from Plan

- README intro/Usage/Diagnostic-codes edits were already done by 03-03; not redone. Intro keeps the accurate "mid-development ... not validated against a real corpus yet (Phase 4)" wording, which satisfies the must-have (no "does not exist yet").
- TestNoWrites chmod helper sets every dir to 0o755 before descending, then applies the final dir mode deepest-first, so both lock and restore walk a read-only tree safely.

Otherwise none - plan executed as written.

## Verification

Full local suite with GOTOOLCHAIN=go1.27.0: gofmt clean, `go mod tidy -diff`, `go vet`, staticcheck v0.8.1, `go test -count=1 ./...`, check-architecture.sh, test-check-architecture.sh, env-gated run (GRUNTLED_CORPUS, GRUNTLED_CORPUS_SECRET, GRUNTLED_HEAVY_TESTS=1) with no skips, cross-build for 5 targets.

## Self-Check: PASSED
