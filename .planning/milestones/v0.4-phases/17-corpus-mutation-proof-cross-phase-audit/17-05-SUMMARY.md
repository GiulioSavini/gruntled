---
phase: 17-corpus-mutation-proof-cross-phase-audit
plan: 05
subsystem: review, cmd/gruntled (tests), ci, docs
tags: [audit, integration, security, gate, release]
requires: [17-04]
provides:
  - "17-INTEGRATION.md: cross-phase review, findings F1, F2, F4 fixed"
  - "17-SECURITY.md: SECURED after re-audit; its open condition closed by corpus run 38264937436"
  - "the final gate on the v0.4 tree, recorded below"
  - "the v0.3.1 release follow-up note and the carried tech debt"
affects: [milestone v0.4 audit]
tech-stack:
  added: []
  patterns: ["final gate run one step at a time, free -m and timeout per step"]
key-files:
  created:
    - .planning/phases/17-corpus-mutation-proof-cross-phase-audit/17-INTEGRATION.md
    - .planning/phases/17-corpus-mutation-proof-cross-phase-audit/17-SECURITY.md
  modified:
    - cmd/gruntled/ci_doc_test.go
    - .github/workflows/corpus.yml
    - docs/validation.md
    - docs/cli.md
    - cmd/gruntled/main.go
    - internal/infrastructure/terragrunt/incremental_test.go
key-decisions:
  - "The 50-minute local corpus gate was not re-run. The 17-03 recorded run (31/31) and corpus.yml run 38264937436 on a11dc37 (31/31, network isolated, real v0.3.0 ref binary) are cited instead; no product logic changed after a11dc37"
  - "TestIncrementalEqualsFull is made deterministic by seeding a linked include, not by relaxing linkedIncludes > 0"
  - "No tag is created. Tagging v0.4.0 is Giulio's decision"
requirements-completed: []
duration: 1 day (review, fixes, re-audit, gate)
completed: 2026-10-13
---

# Phase 17 Plan 05: Cross-phase Audit and Final Gate Summary

**The v0.4 tree passes the final gate. Integration review and security re-audit have no open BLOCKER or MEDIUM. compare-ref is 256/256 against v0.3.0, compat-v1 is 7/7 with no daemon left, the doc pins and the exact eval-site set hold, go.mod and go.sum are unchanged since v0.3.0, CI is green on three OSes and the corpus workflow passed 31/31 on a11dc37. One pre-existing -short flake found during the gate was fixed in 78aa8b5. No tag was created.**

## Task 1: independent review (done before this session)

- 17-INTEGRATION.md (7708983), by gsd-integration-checker: section 6 cross-phase invariant table checked on the final tree.
- 17-SECURITY.md, by gruntled-sec: first audit 88a62aa, re-audit 4f966d7, verdict SECURED with one open condition (corpus run 38264937436), now closed (see Gate).

## Task 2: fixes

Integration findings F1, F2, F4 (0682227..7bf149c):

| Commit | Message |
|--------|---------|
| 0682227 | fix(17-05): staticcheck S1011, S1016 and U1000 in the corpus tests |
| 1a6467b | test(17-05): TestCIDoc requires the AppArmor userns sysctl before the corpus gate |
| 78fa801 | ci(17-05): corpus.yml lifts the AppArmor userns restriction before the gate |
| ed1ce2f | test(17-05): the top-level usage equals docs/cli.md and its report line names --blast |
| 5ee63ff | fix(17-05): the top-level usage names report --blast, in main.go and docs/cli.md |
| 7bf149c | ci(17-05): validation.md records the AppArmor userns step of corpus.yml |

Security findings F1, F4, F5 (4c42aab..a11dc37):

| Commit | Message |
|--------|---------|
| 4c42aab | test(17-05): TestCIDoc requires go mod download then go mod verify before the corpus gate |
| acf2759 | ci(17-05): corpus.yml downloads and verifies the Go modules before the offline gate |
| 8bdb620 | docs(17-05): scope the no-execution claim to the fetch script, the gate and corpus.yml |
| a11dc37 | fix(17-05): move the unix-only ipc reply reader, getHandler and editorPattern into the unix files |

## Task 3: final gate

Run on 2026-10-13, at baae464 for every step except the terragrunt package, which was re-run after 78aa8b5. One step at a time, each preceded by `free -m` and wrapped in `timeout`, with `GOMAXPROCS=2 GOFLAGS=-p=2`. No -race, -fuzz or bare `go test ./...`. Toolchain: go1.27.0 (the module-cache toolchain first on PATH, because the scripts set `GOTOOLCHAIN=local` and `~/.local/go` is go1.24.2).

| Step | How | Result |
|------|-----|--------|
| `check-architecture.sh` | fresh | `architecture: OK (4 domain packages, 5 application packages, 1 interfaces packages)` |
| `test-check-architecture.sh` | cited: passed at 4f966d7 (about 21 min), only docs changed before baae464 | all self-tests passed |
| `go vet ./...` | fresh | clean, exit 0 |
| `-short` per package (`go list ./...`, one at a time, `-p=1`) | fresh | 18 ok, 1 no test files; terragrunt failed once (flake, fixed below), then 10/10 ok |
| doc pins | fresh | 7 PASS, `ok cmd/gruntled 0.016s` |
| eval-site (file, function) set | fresh | exact match |
| `compare-ref.sh v0.3.0` with the three corpora | fresh | `256/256 identical` |
| `compat-v1.sh v0.3.0` | fresh | `compat-v1: 7/7`; no process left |
| `test-txtar-extract.sh` | fresh | `all txtar-extract cases passed` |
| `test-fetch-corpora.sh` | fresh | `all fetch-corpora cases passed` |
| `go mod verify` | fresh | `all modules verified` |
| `git diff --exit-code v0.3.0 -- go.mod go.sum` | fresh | exit 0 |
| corpus gate | cited: 17-03 record and corpus run 38264937436 | 31/31 |
| CI (ci.yml) | cited: run 38264926783 on a11dc37 | success, all jobs |
| no tag at HEAD | fresh | 0 tags |

Tails:

```
$ bash scripts/check-architecture.sh
architecture: OK (4 domain packages, 5 application packages, 1 interfaces packages)

$ go vet ./...
(no output, exit 0)

$ go test -count=1 -short -v -run 'TestRuleRegistryDoc|TestHelpMatchesDocs|TestTopUsageMatchesDocs|TestDaemonDocs|TestCIDoc|TestValidationDocPins|TestReadme' ./cmd/gruntled/
--- PASS: TestCIDoc (0.00s)
--- PASS: TestDaemonDocs (0.00s)
--- PASS: TestHelpMatchesDocs (0.00s)
--- PASS: TestTopUsageMatchesDocs (0.00s)
--- PASS: TestReadmeDocumentsV03 (0.00s)
--- PASS: TestRuleRegistryDoc (0.00s)
--- PASS: TestValidationDocPins (0.00s)
ok  	github.com/GiulioSavini/gruntled/cmd/gruntled	0.016s

$ (eval-site set, the plan's grep | awk pipeline)
internal/infrastructure/hclconv/literal.go LiteralBool;internal/infrastructure/terragrunt/eval.go evalPath;internal/infrastructure/terragrunt/eval.go literalString;internal/infrastructure/tfsurface/typesig.go typeAttrSig;
EXACT-SET-OK

$ free -m | sed -n 2p
Mem:            5927        1961        1810           5        2349        3965
$ COMPARE_REF_CORPORA="$C/primary:$C/secret:$C/denis256" timeout 1200 bash scripts/compare-ref.sh v0.3.0
same corpus/denis256 graph --json (exit 0)
same corpus/denis256 digest 53d7733cad9e65c50d0542947a6fd78941b515ef9462e6cdc5e1bed14a700c39
256/256 identical

$ free -m | sed -n 2p
Mem:            5927        1927         268           5        3921        3999
$ timeout 900 bash scripts/compat-v1.sh v0.3.0
ok HEAD watch, v0.3.0 report
ok no daemon left running
compat-v1: 7/7
$ pgrep -af 'gruntled|gref|ghead'
(nothing)

$ bash scripts/test-txtar-extract.sh
ok blast-snapshots.sh ref no-such-ref-txtar-test
all txtar-extract cases passed

$ bash scripts/test-fetch-corpora.sh
ok 8 escaping symlink (relative, absolute): exit 1, no dest
all fetch-corpora cases passed

$ go mod verify
all modules verified
$ git diff --exit-code v0.3.0 -- go.mod go.sum; echo $?
0

$ for p in $(go list ./...); do go test -count=1 -short -p=1 $p; done   # at baae464
ok  	.../cmd/gruntled	6.373s
ok  	.../internal/application/blasting	0.309s
ok  	.../internal/infrastructure/hclconv	4.945s
ok  	.../internal/infrastructure/ipc	0.740s
--- FAIL: TestIncrementalEqualsFull (0.01s)
    incremental_test.go:479: vacuous run: resolvedIncludes=37 grt100=12 noop=8 patternDirRenames=11 linkedIncludes=0, all must be > 0
FAIL	.../internal/infrastructure/terragrunt	4.233s
(18 ok, 1 no test files, 1 FAIL)

$ go test -count=1 -short ./internal/infrastructure/terragrunt/   # x10 after 78aa8b5
ok  	github.com/GiulioSavini/gruntled/internal/infrastructure/terragrunt	10.287s
...
ok  	github.com/GiulioSavini/gruntled/internal/infrastructure/terragrunt	3.052s
(10/10 ok; and once without -short: ok)

$ gh run view 38264926783 --json conclusion,status,headSha,jobs
conclusion success, headSha a11dc3785e4779aeaa928d5fc670193a64932b4c, jobs: check, architecture,
test-os (windows-latest), test-os (macos-latest), recipe-check, sarif-upload: all success

$ gh run view 38264937436 --json conclusion,status,headSha; gh run view 38264937436 --log | grep 'corpus-gate:'
conclusion success, headSha a11dc3785e4779aeaa928d5fc670193a64932b4c
2026-10-12T15:51:19.0267772Z corpus-gate: shard TestCorpusMutation17/denis256/M8 ok 2s
2026-10-12T15:51:19.0268427Z corpus-gate: 31/31

$ git tag --points-at HEAD | grep -c .
0
```

### Corpus gate

Not re-run locally (50 min). Two runs cover it:
- **17-03 record** (17-03-SUMMARY "The recorded run"): `scripts/corpus-gate.sh`, offline, `corpus-gate: network=isolated`, 15:18:50 to 16:09:10 on 2026-10-12, wall 3,020 s, `corpus-gate: 31/31`.
- **corpus.yml run 38264937436** on a11dc37: network isolated, real v0.3.0 ref binary, conclusion success, `corpus-gate: 31/31`. This closes the one open condition in 17-SECURITY.md (F1/F2 closes on corpus run 38264937436).

After a11dc37 the only changes are docs and the test-only 78aa8b5, so neither run is stale.

### Flake fixed during the gate (78aa8b5)

`TestIncrementalEqualsFull` (from 497d48f, 12-05; already in v0.3.0) asserts that the random run reached a linked include (`linkedIncludes > 0`). Under `-short` rapid draws 20 cases, and in 3 of 8 runs none of them built one. ci.yml does not pass `-short`, so CI never saw it. Giulio approved a minimal test-only fix.

The fix is in `internal/infrastructure/terragrunt/incremental_test.go`, in `TestIncrementalEqualsFull` after `m.seed()`. Every case starts from unit c including `common.hcl` through the symlink `common.hcl -> b/common.hcl`, loaded once, before the random operations. The `> 0` assertion is unchanged. The random operations then act on a linked include from the first step. After the fix, 12/12 runs of the test report linkedIncludes from 26 to 43 (earlier failing runs reported 0), and the package passes 10/10 under -short and once without it.

## Task Commits

| Task | Commit | Message |
|------|--------|---------|
| 1 | 7708983 | docs(17): integration check |
| 1 | 88a62aa, 4f966d7 | docs(17): security audit; docs(17): security re-audit |
| 2 | 0682227..7bf149c | integration F1, F2, F4 (table above) |
| 2 | 4c42aab..a11dc37 | security F1, F4, F5 (table above) |
| 3 | 78aa8b5 | test(17-05): make TestIncrementalEqualsFull deterministic under -short |
| 3 | (this commit) | docs(17-05): summary |

## Threat Mitigations

| ID | Status | Where |
|----|--------|-------|
| T-17-05-1 | mitigated | review by gsd-integration-checker (17-INTEGRATION.md) and gruntled-sec (17-SECURITY.md), not by the executor |
| T-17-05-2 | mitigated | 17-INTEGRATION.md section 6 invariant table; corpus runs 31/31 as end-to-end evidence |
| T-17-05-3 | mitigated | compare-ref 256/256 on the final tree; corpus run 38264937436 on a11dc37, after the last output-relevant fix |
| T-17-05-4 | mitigated | `git.create_tag=false` (d1f79ed); `git tag --points-at HEAD` empty at baae464 and checked again at handoff; no tag command run |
| T-17-05-5 | mitigated | one step at a time, `free -m` and `timeout` each; no -race, -fuzz or bare `go test ./...` locally |
| T-17-05-6 | mitigated | exact (file, function) set assertion above: LiteralBool, evalPath, literalString, typeAttrSig |
| T-17-05-7 | mitigated | Task 1 review scope in 17-SECURITY.md (blast --base input, instance store/lock, windows dump, exclusion table; phase 17 scripts and workflow) |

## Release follow-up (for Giulio)

**v0.4.0.** The tree is ready for `/gsd-audit-milestone v0.4`. Tagging v0.4.0 is Giulio's decision. This plan creates no tag, and none points at HEAD.

**v0.3.1.** v0.3.0 lacks the for-expression guard that v0.4 has. To ship a v0.3.1:
1. Branch from v0.3.0.
2. Cherry-pick the 15-00 commits c1440d7, dfe565b and 3279bec.
3. Backport the `HasForExpr` guard by hand into v0.3.0 `internal/infrastructure/terragrunt/eval.go`, in `literalBool`, `evalPath` and `literalString`. The v0.4 code was refactored (`literalBool` moved to `hclconv.LiteralBool`), so a straight cherry-pick does not apply.
4. Run the v0.3 gate on that branch, then tag v0.3.1 by hand.

## Carried tech debt

- **16-SECURITY M18.** The EventFailed copy of `*p.last` outside `p.mu` survives mutation without -race. Only `TestRebaseConcurrentPublish` under -race catches it, in CI.
- **TestBlastSizeBound** (`cmd/gruntled/blast_bound_test.go`). Its RSS bound is loose. It catches a blow-up, not a moderate regression.
- **`cmd/gruntled/rebase.go:123`.** The pre-send failure path prints the raw error with `%v` (`cannot reach the watch daemon for %s: %v`).
- **`internal/infrastructure/watch/run.go:129-135`.** The branch was kept, with a comment explaining why, instead of being removed (comment-only diff since 57d74d8, per 17-SECURITY T-17-00-3).

## Deviations from Plan

1. **[Gate scope] Cited, not re-run:** `test-check-architecture.sh` (passed at 4f966d7, only docs changed before baae464), the local corpus gate (17-03 record plus corpus run 38264937436), and CI (run 38264926783). Instructed by the orchestrator.
2. **[Rule 1, test bug] 78aa8b5:** fixed the -short flake in TestIncrementalEqualsFull, approved by Giulio. Test-only.
3. **[Toolchain]** The gate ran on the module-cache go1.27.0 because `~/.local/go` is go1.24.2 and the scripts set `GOTOOLCHAIN=local`. Environment only; no script changed.
4. **[Push]** Pushed to master on the orchestrator's instruction. CI on 78aa8b5 and on this commit runs on push; not re-triggered by hand.

## Self-Check: PASSED
