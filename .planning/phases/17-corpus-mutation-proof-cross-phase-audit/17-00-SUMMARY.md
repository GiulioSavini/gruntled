---
phase: 17-corpus-mutation-proof-cross-phase-audit
plan: 00
subsystem: planning bookkeeping, watch (comment)
tags: [bookkeeping, validation, tech-debt, ci]
requires: []
provides:
  - "requirements-completed claims for BLAST-06/07/08/10 (15-03) and BLAST-09 (15-04)"
  - "verify-summary passes on all 17 SUMMARY files of phases 13-16"
  - "15-VALIDATION.md and 16-VALIDATION.md at status: validated, with per-row evidence"
  - "run.go comment explaining the unreachable republish branch"
  - "windows test-os confirmed green on 4 consecutive master runs from c083bef"
affects: [17-05]
tech-stack:
  added: []
  patterns: []
key-files:
  created:
    - .planning/phases/17-corpus-mutation-proof-cross-phase-audit/17-00-SUMMARY.md
  modified:
    - .planning/phases/13-removed-output-diagnostic-shared-blast-core/13-01-SUMMARY.md
    - .planning/phases/13-removed-output-diagnostic-shared-blast-core/13-02-SUMMARY.md
    - .planning/phases/13-removed-output-diagnostic-shared-blast-core/13-03-SUMMARY.md
    - .planning/phases/15-type-level-surface-facts/15-00-SUMMARY.md
    - .planning/phases/15-type-level-surface-facts/15-02-SUMMARY.md
    - .planning/phases/15-type-level-surface-facts/15-03-SUMMARY.md
    - .planning/phases/15-type-level-surface-facts/15-04-SUMMARY.md
    - .planning/phases/16-daemon-baseline-report-blast-rebase/16-00-SUMMARY.md
    - .planning/phases/16-daemon-baseline-report-blast-rebase/16-04-SUMMARY.md
    - .planning/phases/15-type-level-surface-facts/15-VALIDATION.md
    - .planning/phases/16-daemon-baseline-report-blast-rebase/16-VALIDATION.md
    - internal/infrastructure/watch/run.go
key-decisions:
  - "The 'Self-check section indicates failure' trigger in 13-01, 13-02 and 15-00 was the earlier Verification heading: verify.cjs slices from the first Self-Check|Verification|Quality Check heading, and the Self-Check section was already last. Renamed those headings to Gate (the 13-03/16-00/16-04 style) instead of moving Self-Check"
  - "The claim loop is scoped to the `## v0.4 Requirements` section: the plan's grep also matched MORE-04/MORE-05, which are Future Requirements"
  - "16-VALIDATION row 16-04-01 is recorded as flaky (CI 38231782360), not green, and the gap is on the bus (#440)"
  - "Windows: 4 consecutive green test-os runs from c083bef, so rebase_test.go is unchanged and no extra run was triggered"
requirements-completed: []
duration: 15min
completed: 2026-10-12
---

# Phase 17 Plan 00: Bookkeeping and Tech Debt Summary

**The milestone audit's false gaps are cleared. BLAST-06/07/08/10 are claimed in 15-03 and BLAST-09 in 15-04, so every v0.4 requirement of phases 13-16 (except REL-03) is in some SUMMARY's `requirements-completed`. verify-summary passes on 17/17 SUMMARYs (it failed on 7). 15-VALIDATION and 16-VALIDATION are `validated`, and each row cites a command re-run now or a green CI run id. The run.go republish branch is documented as defence in depth, with a comment-only diff. Windows `test-os` is green on 4 consecutive master runs from c083bef, so no test change was needed. One new gap is listed rather than fixed: a macOS CI flake in TestReportBlastE2E/linux/type-only_change.**

## Performance

- Duration: ~15 min
- Tasks: 3
- Files: 1 created, 12 modified

## Accomplishments

### Task 1: requirements claims and verify-summary false positives (8f7f441)

The state before the change, from verify-summary over the 17 SUMMARYs of phases 13-16:

| SUMMARY | Errors before |
|---------|---------------|
| 13-01 | Missing files: bash scripts/check-architecture.sh, bash scripts/test-check-architecture.sh; Self-check section indicates failure |
| 13-02 | same two errors as 13-01 |
| 13-03 | Missing files: svc/api/common.tf -> ../../shared/common.tf |
| 15-00 | Self-check section indicates failure |
| 15-02 | Missing files: bash scripts/check-architecture.sh, bash scripts/test-check-architecture.sh |
| 16-00 | Missing files: $RUNNER_TEMP/fuzz.log |
| 16-04 | Missing files: modules/big/main.tf |

What changed (presentation only, no fact, number, hash or verdict):
- 15-03 frontmatter `requirements-completed: [BLAST-06, BLAST-07, BLAST-08, BLAST-10]`; 15-04 `requirements-completed: [BLAST-09]`. The source plans follow the 15-VERIFICATION Requirements Coverage table.
- 13-01 and 13-02: the two architecture list items became one item with a fenced block that holds both commands and their output lines.
- 15-02: both commands moved into the fenced blocks that already held their output (`$ bash …` as the first line). Once those were gone, the heuristic reached a third backticked path, the never-created check_json_type_bomb.txtar in deviation 1. It is now written unbackticked as "the never-created path".
- 13-03: the symlink notation is unbackticked as "fixture link svc/api/common.tf -> ../../shared/common.tf".
- 16-00: both mentions are written as `"$RUNNER_TEMP/fuzz.log"`, quoted as in ci.yml.
- 16-04: "the fixture path modules/big/main.tf", unbackticked.
- 13-01, 13-02, 15-00: the Verification heading renamed to Gate (see Deviations).

After the change:

```sh
for f in .planning/phases/1[3-6]-*/*-SUMMARY.md; do node ~/.claude/gsd-core/bin/gsd-tools.cjs query verify-summary "$f" | grep -q '"passed": true' || echo "FAIL $f"; done
for r in $(sed -n '/^## v0.4 Requirements/,/^## Future/p' .planning/REQUIREMENTS.md | grep -oE '\*\*(MORE|BLAST|DAEMON|SEC)-[0-9]+' | tr -d '*' | sort -u); do grep -l "requirements-completed:.*$r" .planning/phases/1[3-6]-*/*-SUMMARY.md >/dev/null || echo "UNCLAIMED $r"; done
```

Neither loop prints anything: there is no FAIL and no UNCLAIMED line. `git diff --word-diff` on the commit shows only the two frontmatter claims, the fences, the three heading renames and the unbackticked paths.

### Task 2: validate phases 15 and 16 (6ccee45)

The state-A workflow from validate-phase.md was applied to both files. Every row's command was re-run on master 8f7f441, one package at a time, with `-short`, `free -m` first (5.0-5.5 GB free) and `timeout 900`:

- Infrastructure packages (hclconv, ipc, sourceresolve, statusfile, terragrunt, tfsurface, watch), each domain and application package, and presenter: all ok.
- cmd/gruntled rows, counted with `-v`. 15: `TestScripts/blast|TestBlastDepth1MatchesV1|TestBlastTransitive` 31 PASS; the doc guards 4 PASS; `TestTextOutputsEscapeControls` 10 PASS. 16: TestCIDoc 7, 16-01-03 66, 16-02-02 29, 16-02-03 126, 16-03-01 68, 16-03-02 45, 16-03-03 37, 16-04-01 20, 16-04-02 4 PASS. Nothing failed. The only skips were `TestReportBlastSizeBound` and `TestDaemonMemorySynthrepo/units=5000` (`-short`, CI-only).
- Gate pieces, run as:

  ```sh
  bash scripts/check-architecture.sh   # architecture: OK (4 domain packages, 5 application packages, 1 interfaces packages)
  bash scripts/compare-ref.sh v0.3.0   # 244/244 identical
  bash scripts/compat-v1.sh v0.3.0     # compat-v1: 7/7 (ok no daemon left running)
  bash scripts/test-txtar-extract.sh   # all txtar-extract cases passed
  go mod verify                        # all modules verified
  go vet ./...                         # clean
  GOOS=windows GOARCH=arm64 go build -o /dev/null ./cmd/gruntled   # ok
  ```

- CI-only rows (-race, fuzz, bound tests, 5,000 units, build-release, windows, test-check-architecture) cite these runs. Phase 15: 38157144902 (3ed44ae) and 38229787854 (d99714e). Phase 16: 38225491068 (f05cecc), 38227195245 (a50d203), 38228443297 (c083bef) and 38229787854 (d99714e). All of their jobs succeeded. ci.yml runs the non-short `go test ./...` on linux (-race), macOS and windows. In 38229787854 the FuzzTypeSig step logged 161,135 execs in 30 s.
- Frontmatter: both files are `status: validated` and `nyquist_compliant: true`, with `validated: 2026-10-12 (17-00)`. For 15, `wave_0_complete: true`: all 11 ❌ W0 files exist.
- Each file has a `## Validation Audit` table. 15: 0 gaps. 16: 1 gap (below).

### Task 3: run.go comment and the windows flake (91f0641)

- run.go: 7 comment lines on the `case <-w.Ready():` republish branch. They say the branch is defence in depth and unreachable today: a skip leaves the debouncer armed; only a successful Flush disarms it, and that Flush is always followed by Index and then a publish or a new skip. They cite TestRunSpuriousReadyAfterSkipEndsPublished and 16-SECURITY F3. The diff grep for non-comment lines prints nothing.
- Windows `test-os (windows-latest)` from c083bef:

  | Run | Commit | windows test-os |
  |-----|--------|-----------------|
  | 38228443297 | c083bef | success |
  | 38229787854 | d99714e | success |
  | 38231782360 | d1f79ed | success |
  | 38233897723 | 5ba0605 | success |

  That is 4 consecutive green runs (3 required). TestRebaseWindowsUnsupported has not failed again, so rebase_test.go is unchanged. No run was triggered for this count.

## Task Commits

| Task | Commit | Message |
|------|--------|---------|
| 1 | 8f7f441 | docs(17-00): requirements claims and summary presentation |
| 2 | 6ccee45 | docs(17-00): validate phases 15 and 16 |
| 3 | 91f0641 | docs(17-00): explain the unreachable republish branch |

## Gate

```sh
go test -count=1 -short ./internal/infrastructure/watch/            # ok 1.053s
timeout 900 go test -count=1 -short -run 'TestRebase' ./cmd/gruntled/   # ok
GOOS=windows GOARCH=amd64 go vet ./cmd/gruntled/                     # clean
bash scripts/check-architecture.sh                                   # architecture: OK (4 domain packages, 5 application packages, 1 interfaces packages)
```

Not run locally, per the RAM rules: `-race`, `-fuzz`, `go test ./...` without `-short`, and the bound tests. scripts/test-check-architecture.sh was also not run, because no architecture script changed. Nothing was pushed, so there is no 17-00 CI run id yet.

## Threat Mitigations

| ID | Status | Where |
|----|--------|-------|
| T-17-00-1 | mitigated | 8f7f441 is presentation only: frontmatter claims follow the 15-VERIFICATION source plans; everything else is fences, heading renames and unbackticked paths |
| T-17-00-2 | mitigated | each VALIDATION row names its local result or a CI run id; the one flaky row is recorded as ⚠️ flaky and posted on the bus (#440) |
| T-17-00-3 | mitigated | run.go diff is comment-only (internal/infrastructure/watch/run.go:129-135); watch tests ok |
| T-17-00-4 | mitigated | no windows hardening was needed; rebase_test.go is untouched |
| T-17-00-5 | mitigated | no tag was created |

## Deviations from Plan

**1. [Heuristic] The self-check fix is a heading rename, not a section move.** The plan says to move the Self-Check: PASSED section to the end in 13-01, 13-02 and 15-00, but it was already the last line in all three. verify.cjs searches for the first heading named Self-Check, Verification or Quality Check, so the slice started at the earlier Verification heading and took in deviation text containing "failed". Those headings are now `## Gate`.

**2. [Verify scope] The claim loop is limited to the v0.4 section.** The plan's grep over the whole of REQUIREMENTS.md reported MORE-04 and MORE-05 as UNCLAIMED. Both are under `## Future Requirements`. The must-have says "v0.4 IDs minus REL-03", so the loop now reads only the `## v0.4 Requirements` section.

**3. [Uncovered presentation case] 15-02 had a third false positive.** Once the two architecture commands were gone, the heuristic's sample reached a third path: check_json_type_bomb.txtar, a path the plan named but that was never created (15-02 deviation 1). It is unbackticked, with the words "the never-created path".

## Known Risks / Unverified

- **CI flake (gap, not fixed here):** run 38231782360 (d1f79ed), test-os macos, failed TestReportBlastE2E/linux/type-only_change at daemon_blast_test.go:827. report --blast showed `Impacted (0)` at current generation 2, while blast --base showed `Impacted (2)`. The likely cause is in waitBlastFinal (daemon_blast_test.go:696). It compares report text with check text, and a type-only change leaves those identical. So `CurrentGeneration >= 2` can be met by a poll that ran before the rename landed. This is a test race in phase-16 code and outside 17-00's files. It was posted to the orchestrator as #440. 16-VALIDATION row 16-04-01 is marked ⚠️ flaky.
- 38233897723 (5ba0605) still had the architecture job running when this was written. Its windows, macos and check jobs were green.

## Self-Check: PASSED
