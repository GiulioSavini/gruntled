---
phase: 16-daemon-baseline-report-blast-rebase
plan: 00
subsystem: ci, tfsurface (tests)
tags: [ci, fuzz, test-strength, evidence]
requires: []
provides:
  - "FuzzTypeSig seed corpus of seeds of at most 2,048 bytes (fuzzTypeSigSeeds), with a depth-boundary seed"
  - "TestTypeSigFormerFuzzSeeds: the five former large seeds run the shared fuzz body on every go test"
  - "CI fuzz step with count-bounded minimization and a tee'd log that keeps crash evidence"
  - "TestCIDoc pin of the fuzz step"
affects: []
tech-stack:
  added: []
  patterns: ["one shared fuzz body (fuzzTypeSigOnce) for the fuzz target and its deterministic test"]
key-files:
  created: []
  modified:
    - internal/infrastructure/tfsurface/typesig_test.go
    - .github/workflows/ci.yml
    - cmd/gruntled/ci_doc_test.go
key-decisions:
  - "The boundary seed is written as strings.Repeat(\"[\", hclconv.MaxNestingDepth+1) + strings.Repeat(\"]\", hclconv.MaxNestingDepth+1): the same 2,002 bytes as the plan's literal 1_001, but it follows the constant"
  - "TestCIDoc extracts the fuzz step itself (new helper workflowStep), so the pins cannot be satisfied by text in another step of the check job; it also asserts the step has no `secrets.` and no `permissions:` (T-16-00-3)"
  - "The 2 s per-seed bound in TestTypeSigFormerFuzzSeeds is multiplied by raceSlowdown, as the orchestrator asked for new time bounds (bus #304)"
requirements-completed: []
duration: 20min
completed: 2026-10-12
---

# Phase 16 Plan 00: CI Fuzz Fix Summary

**FuzzTypeSig's seed corpus now holds only seeds of at most 2,048 bytes. The five multi-KB seeds that kept the 30 s CI window busy minimizing mutants are gone from the corpus, but they still run through the same fuzz body on every `go test` (TestTypeSigFormerFuzzSeeds, 3-22 ms each). A 2,002-byte seed one level past `hclconv.MaxNestingDepth` keeps the mutator on the refusal edge. The CI step bounds minimization with `-fuzzminimizetime 200x`, tees the full output to `$RUNNER_TEMP/fuzz.log`, prints the last progress lines on success, and on failure prints 200 lines of panic and stack plus every failing input. TestCIDoc pins all of it.**

## Performance

- Duration: ~20 min
- Tasks: 2
- Files: 0 created, 3 modified

## Accomplishments

### Task 1: Small seed corpus and the former large seeds (internal/infrastructure/tfsurface/typesig_test.go)
- **fuzzTypeSigSeeds** (:597): the 14 small seeds as before, plus `deepList(300)` (:605, 1,806 bytes, accepted by the pre-scans and parsed) and the boundary seed (:609, 2,002 bytes).
- **fuzzTypeSigOnce** (:614): the former inline fuzz body: the native ParseConfig path when the file fits and CheckNativeDepth accepts it, and the JSON path when CheckJSONDepth accepts it, each through fuzzSig. `FuzzTypeSig` (:629) ranges over the slice and calls `f.Fuzz(fuzzTypeSigOnce)`, so the target and the deterministic test cannot drift.
- **TestFuzzTypeSigSeedsSmall** (:641): every seed is at most `maxFuzzSeedBytes` = 2,048 (:639). The corpus is non-empty and still has `string`, an `object(...)` with `optional(`, a `${...}` form and the boundary seed. CheckNativeDepth refuses the boundary seed inside a `variable` block.
- **TestTypeSigFormerFuzzSeeds** (:671): runs fuzzTypeSigOnce on the five removed seeds, each under 2 s × raceSlowdown. Measured locally:

| Former seed | bytes | time |
|-------------|------:|-----:|
| deepList(5_000) | 30,006 | 4.2 ms |
| deepList(999) | 6,000 | 22.2 ms |
| `"!"`×5,000 + `"x"` | 5,001 | 2.8 ms |
| nestedIfDefault(1_100) | 16,532 | 6.2 ms |
| `"a?b:\n"`×1,100 + `"c"` | 5,501 | 3.0 ms |

- Red first (28d387b): with the old 19 seeds in the slice, TestFuzzTypeSigSeedsSmall failed on seeds 14-18 (30006, 6000, 5001, 16532 and 5501 bytes) and on the missing boundary seed.

### Task 2: Bounded minimization in CI (.github/workflows/ci.yml, cmd/gruntled/ci_doc_test.go)
- **ci.yml** (:50-71): the step runs under `set -o pipefail`:

```sh
go test -run '^$' -fuzz '^FuzzTypeSig$' -fuzztime 30s -fuzzminimizetime 200x ./internal/infrastructure/tfsurface/ 2>&1 | tee "$log" >/dev/null
```

  - On success it prints `tail -n 5 "$log"`.
  - On failure it prints `tail -n 200 "$log"`, then `cat`s each file under `internal/infrastructure/tfsurface/testdata/fuzz/FuzzTypeSig/`, and exits 1.
  - The step comment says why the bound is a count: 200x is the same budget on every runner, while the 60 s per-input default ate the window. No rate threshold, since it would flake. `timeout-minutes: 5` is unchanged and no other step was touched.
- **TestCIDoc** "fuzz step" subtest (ci_doc_test.go:116): it finds the step in the `check` job with the new `workflowStep` helper (:180) and requires `timeout-minutes: 5`, `set -o pipefail`, the anchored target, `-fuzztime 30s`, `-fuzzminimizetime 200x`, `log="$RUNNER_TEMP/fuzz.log"`, `tee "$log"`, `tail -n 5 "$log"`, `tail -n 200 "$log"`, the FuzzTypeSig input loop and `cat "$f"`. It rejects `secrets.` and `permissions:` in the step. Red first (5debdd0): 8 missing pins.
- The step's run block, extracted from the YAML, passes `bash -n`, and the workflow loads with `yaml.safe_load`.

## Task Commits

1. `28d387b` test(16-00): FuzzTypeSig seeds must be small; former large seeds run deterministically
2. `c10c309` fix(16-00): keep the FuzzTypeSig seed corpus under 2 KiB
3. `5debdd0` test(16-00): pin the bounded, logged CI fuzz step in TestCIDoc
4. `9cc01c5` ci(16-00): bound fuzz minimization and keep the crash evidence

This summary is its own `docs(16-00): summary` commit.

## Gate (at 9cc01c5)

Run with `GOMAXPROCS=2 GOFLAGS=-p=2` (8 GB WSL):

```
timeout 300 go test -count=1 ./internal/infrastructure/tfsurface/ -timeout 300s
ok  	github.com/GiulioSavini/gruntled/internal/infrastructure/tfsurface	5.912s
timeout 300 go test -count=1 -run TestCIDoc ./cmd/gruntled/
ok  	github.com/GiulioSavini/gruntled/cmd/gruntled	0.004s
timeout 300 go vet ./...
(no output, exit 0)
bash scripts/check-architecture.sh
architecture: OK (4 domain packages, 5 application packages, 1 interfaces packages)
gofmt -l cmd internal
(no output)
```

Not run locally: `-fuzz`, `-race` and the full `go test ./...`. CI covers them. The plan does not ask for compare-ref, and no golden file changed.

CI evidence still to collect: the plan asks for the final `fuzz:` progress line, with a non-zero rate, from the CI log of the next push. The orchestrator pushes, so that line is not in this summary yet.

## Threat Mitigations

| ID | Disposition | Where |
|----|-------------|-------|
| T-16-00-1 | mitigated | typesig_test.go:671 TestTypeSigFormerFuzzSeeds runs the five former seeds through the shared fuzzTypeSigOnce (:614); boundary seed in the corpus (:609), its refusal asserted at :650 onward |
| T-16-00-2 | mitigated | ci.yml:62 `-fuzzminimizetime 200x`; :63 tail of the log on success shows execs and rate; pinned at ci_doc_test.go:116 |
| T-16-00-5 | mitigated | ci.yml:60-62 tee to `$RUNNER_TEMP/fuzz.log` under pipefail; :65 `tail -n 200`; the loop that follows cats every FuzzTypeSig input; pinned at ci_doc_test.go:116 |
| T-16-00-3 | mitigated | only the fuzz step's comment and run block changed; ci_doc_test.go:138 rejects `secrets.` and `permissions:` in the step |
| T-16-00-4 | accepted | `timeout-minutes: 5` kept (ci.yml:58) and pinned |

## Deviations from Plan

**1. [Seed count] The plan says "the current 14 seeds"; the corpus had 19**
- 14 small seeds plus the 5 large ones. The 14 small seeds are kept unchanged, and the 5 large ones are removed as the plan says.

**2. [Gate] The full tfsurface package ran locally**
- The plan's verification runs `go test ./internal/infrastructure/tfsurface/`, which includes that package's hostile-input bound tests. It finished in 5.9 s with about 7 GB available. No -race and no -fuzz were run.

## Known Risks / Unverified

- Whether the CI fuzz step now keeps a non-zero exec rate for its whole window is proven only by the next CI run.

## Self-Check: PASSED
