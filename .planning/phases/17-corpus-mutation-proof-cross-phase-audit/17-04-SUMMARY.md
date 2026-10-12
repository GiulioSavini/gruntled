---
phase: 17-corpus-mutation-proof-cross-phase-audit
plan: 04
subsystem: cmd/gruntled (tests), docs, ci
tags: [corpus, rel-03, daemon, docs, ci]
requires: [17-03]
provides:
  - "TestCorpusDaemonSample: one site per class, R8, the shared global.tf edit and a rebase cycle through a real built daemon on each corpus"
  - "docs/validation.md `## v0.4: blast on the real corpora (REL-03)`, pinned by TestValidationDocPins from the gated tests' package-level values"
  - ".github/workflows/corpus.yml (workflow_dispatch + weekly), pinned by TestCIDoc corpus_workflow"
affects: [17-05]
tech-stack:
  added: []
  patterns: ["doc pins generated from the same package-level values the gated tests assert", "daemon wait: generation advanced + plain report == check + normalised view == blast --base"]
key-files:
  created:
    - cmd/gruntled/corpus_daemon_test.go
    - .github/workflows/corpus.yml
  modified:
    - docs/validation.md
    - cmd/gruntled/validation_doc_test.go
    - cmd/gruntled/ci_doc_test.go
    - cmd/gruntled/corpus_mutation_test.go
    - cmd/gruntled/corpus_model_test.go
key-decisions:
  - "The daemon wait cannot pass on an intermediate state, the 16-fix lesson. It requires three things together: a served generation after the one before the edit, the plain report equal to `check` on the final tree, and the normalised view equal to blast --base. Only then does assertBlastEquivalent run"
  - "M6 runs one-sided through the daemon. The daemon's baseline is the pristine copy, so only cur' (type = number) is applied, and the expected view is blast --base pristine cur, which is empty because a one-sided type is silent. The two-sided M6 is proven in 17-03"
  - "The four exception sentences (rel03Exceptions) and the graph-known counts (corpusGraphKnown, now asserted by TestCorpusBlastModel) became package-level values. TestValidationDocPins builds its strings from these and from corpusMutationPins, corpusDepthPins, corpusIncluded, corpusExclusions and m17SentinelCanon"
  - "corpus.yml pins checkout and setup-go by commit sha, the same v7.0.1 and v7.0.0 releases and the same form ci.yml's recipe-check job uses. TestCIDoc reuses ciPins for it"
requirements-completed: [REL-03]
duration: 50min
completed: 2026-10-12
---

# Phase 17 Plan 04: Daemon Sample, REL-03 Record and Corpus Workflow Summary

**The daemon path agrees with `blast --base` on real corpus edits. On each corpus this was checked with one site per class, R8, the shared iso20022 `global.tf` edit, and a rebase cycle, with no daemon left afterwards. The REL-03 evidence is in docs/validation.md. TestValidationDocPins pins it from the same package-level values the gated tests assert, so the doc and the tests cannot drift. The corpus suite has its own workflow: manual plus weekly, read-only, no persisted token, no shared Go cache. ci.yml stays free of it.**

## Performance

- Duration: about 50 min
- Tasks: 3
- Files: 2 created, 5 modified

## Accomplishments

### Task 1: TestCorpusDaemonSample (9840b4e)

`cmd/gruntled/corpus_daemon_test.go` (`//go:build linux || darwin`):
- `startCorpusDaemon` runs the built binary (`benchBuildGruntled`) as `watch --poll --poll-interval 200ms`, with a private runtime dir. Every client call has a 20 s timeout.
- `stop` sends TERM, waits up to 5 s, then sends KILL. A final `assertNoProcessMentions` scans /proc for any command line naming the temp dirs. This is the compat-v1 pattern.
- **Red row:** before the first equivalence check, the normaliser is fed the served label with its generation off by one and must reject it.
- **Per corpus:** the first eligible site of M1-M7, then R8, then M9 on iso20022. Each edit is waited on, then checked with `assertBlastEquivalent` (text and JSON, same exit code). It is then reverted, and the view must be exactly `Broken (0):\nImpacted (0):\n` at exit 0.
- **The rebase cycle:**
  - apply M1 and wait;
  - `rebase --expect <gen>` must print exactly `gruntled: baseline moved to generation <gen>; <n> error findings in Broken accepted`, where n comes from expectBlast;
  - the view is then empty;
  - revert;
  - JSON must show one change, `added_outputs [y]`, source `rebase`, Broken [], exit 0, and the text must contain `+output y`.

```sh
go test -p=1 -count=1 -run "TestCorpusDaemonSample/$c" -v -timeout 90m ./cmd/gruntled/
```

```
CORPUS-DAEMON secret M1 … M7 ok; M9 no eligible site; R8 ok; rebase cycle ok (2 errors accepted; revert shows +output domain_cert_arn)   PASS 8.34s
CORPUS-DAEMON iso20022 M1 M2 M3 M5 M6 M7 ok; M4 no eligible site; M9 iac.src/global.tf account M4 ok; R8 ok; rebase cycle ok (3 errors accepted; revert shows +output region)   PASS 17.90s
CORPUS-DAEMON denis256 M1 … M7 ok; M9 no eligible site; R8 ok; rebase cycle ok (1 errors accepted; revert shows +output outputY)   PASS 38.02s (147,072 KB RSS)
```

After each corpus, `pgrep -fa '[g]runtled watch'` printed nothing ("no daemon"). Without the env vars, the 3 subtests SKIP.

### Task 2: the REL-03 record in docs/validation.md (5228eb7 red, 0547e8e)

**Red.** TestValidationDocPins gained `rel03DocPins()`. Against the old doc it reported 56 missing strings. The pins are:
- the heading, the three commits, both scripts, `corpus-gate: network=isolated`, `corpus-gate: 31/31` and `256/256 identical`;
- the exact `CORPUS-MUTATION <corpus> <class> sites=… caught=… excluded_by_touch=…` line for every corpus and class, built from `corpusMutationPins`;
- the M8 lines from `corpusDepthPins`;
- the included-share rows from `corpusIncluded` and `corpusGraphKnown`;
- the exclusion totals per category and corpus, counted from `corpusExclusions`;
- the four `rel03Exceptions` sentences;
- the SENTINEL (`m17SentinelCanon`);
- `TestCorpusDaemonSample`.

**Green.** The new section has an outcome table, the four exceptions next to it, pinned commits, the command, the model and exclusion totals with the 818/811/795 explanation, the mutation table, the recorded counts, the dual oracle and SENTINEL hand check, the R1-R9 counters, the daemon sample, the recorded gate run (date, commit, machine, gate excerpt, `free -m` lines) and the limitations. The intro links to it, and Reproduce now starts with the fetch-corpora.sh one-step fetch; the manual steps stay as the fallback.

Every number traces to a 17-01..17-03 SUMMARY line or the gate log. The machine line comes from `lscpu`, `/proc/meminfo`, `uname -r` and `go version`, and is presented as prose, not as gate output.

```sh
go test -count=1 -short -run 'TestValidationDocPins|TestReadme|TestHelpMatchesDocs' ./cmd/gruntled/   # ok
```

### Task 3: corpus.yml and its pin (e53fac8 red, aad8e5e)

**Red.** The TestCIDoc subtest `corpus_workflow` failed while the file was missing. It asserts:
- `workflow_dispatch:`, `schedule:` with a cron, and `permissions:\n  contents: read`;
- `timeout-minutes: 150`, `persist-credentials: false`, `fetch-depth: 0`, `cache: false` and `go-version-file: go.mod`;
- the fetch step into `$RUNNER_TEMP/corpus`, with the export lines appended to `"$GITHUB_ENV"`;
- the gate step with `CORPUS_GATE_NONET: "1"`;
- the two sha-pinned actions from `ciPins`, and exactly 2 `uses:`;
- the absence of `secrets.`, `write`, `upload-artifact`, `pull_request` and `push:`;
- that ci.yml contains no `GRUNTLED_CORPUS`, `corpus-gate` or `fetch-corpora`.

**Green.** `.github/workflows/corpus.yml` runs Mondays at 03:17 UTC and on demand, on ubuntu-latest, with `GOTOOLCHAIN: local`. It uploads nothing. The YAML parses (`yaml.safe_load`).

```sh
go test -count=1 -run TestCIDoc ./cmd/gruntled/          # 7 subtests PASS
! grep -nE 'GRUNTLED_CORPUS|corpus-gate' .github/workflows/ci.yml   # clean
```

**First dispatch:** pending. Nothing is pushed yet (no push by instruction). After the push, run `gh workflow run corpus.yml` and record the run id. If the runner forbids unprivileged user namespaces, the gate exits 1 by design: report it, do not drop CORPUS_GATE_NONET.

## Task Commits

| Task | Commit | Message |
|------|--------|---------|
| 1 | 9840b4e | test(17-04): TestCorpusDaemonSample runs one site per class, R8, the shared global.tf edit and a rebase cycle through a real daemon |
| 2 (red) | 5228eb7 | test(17-04): TestValidationDocPins pins the REL-03 record to the gated tests' package-level values |
| 2 | 0547e8e | docs(17-04): REL-03 record in validation.md, with the exactness exceptions and fetch-corpora.sh in Reproduce |
| 3 (red) | e53fac8 | test(17-04): TestCIDoc pins corpus.yml (dispatch + weekly, read-only, no persisted token, no Go cache) and keeps the corpus suite out of ci.yml |
| 3 | aad8e5e | ci(17-04): corpus.yml runs fetch-corpora.sh and corpus-gate.sh on demand and weekly |

## Gate

```sh
go vet ./cmd/gruntled/                       # clean
GOOS=windows GOARCH=amd64 go vet ./cmd/gruntled/   # clean (the daemon test is linux || darwin)
gofmt -l cmd/gruntled/                       # empty
bash scripts/check-architecture.sh           # architecture: OK (4 domain packages, 5 application packages, 1 interfaces packages)
go test -count=1 -short ./cmd/gruntled/      # ok 8.120s (no corpus env)
go test -count=1 -run "TestCorpusBlastModel/$c" ./cmd/gruntled/   # ok x3 with the new corpusGraphKnown assertion
```

There was no gruntled watch process and no extra worktree at the end. Runs were serial, one daemon at a time, under `ulimit -v 4000000`. Not run: -race, -fuzz, `go test ./...`, test-check-architecture.sh, and the full corpus gate (no product or gated-test logic changed after the 17-03 recorded run).

## Threat Mitigations

| ID | Status | Where |
|----|--------|-------|
| T-17-04-1 | mitigated | corpus_daemon_test.go `startCorpusDaemon`/`stop` (TERM, 5 s, KILL), 20 s client timeout, `assertNoProcessMentions`; "no daemon" after each run |
| T-17-04-2 | mitigated | exact-label normaliser from 16-04; the off-by-one generation row in TestCorpusDaemonSample |
| T-17-04-8 | mitigated | corpus.yml `persist-credentials: false`, `cache: false`; pinned in TestCIDoc corpus_workflow |
| T-17-04-3 | mitigated | `permissions: contents: read`, no `secrets.` and no `write`, the same sha-pinned actions as ci.yml; asserted |
| T-17-04-4 | mitigated | fetch-corpora.sh verifies each sha; the require helpers fail on another commit |
| T-17-04-5 | mitigated | fetch isolation and ELF strip (17-01); nothing from a corpus is executed; no secrets in the job |
| T-17-04-6 | mitigated | `rel03DocPins` built from the package-level values the gated tests assert |
| T-17-04-7 | mitigated | TestCIDoc asserts ci.yml has no corpus env, gate or fetch call |

## Deviations from Plan

1. **[M6 through the daemon] One-sided.** The daemon's baseline is its watch-start index of the pristine copy, so a base' edit is not possible without a rebase. The sample applies cur' only and compares with blast --base pristine cur (an empty view). The two-sided M6 is covered by 17-03.
2. **[Action refs] Pinned by sha, not `@v7`.** corpus.yml uses the full-sha form ci.yml's recipe-check job uses, for the same releases (v7.0.1 and v7.0.0). This is stricter than the plan's `@v7`.
3. **[New shared values]** `rel03Exceptions` and `corpusGraphKnown` were added so the doc pins and the tests share them. TestCorpusBlastModel now asserts the graph-known counts (62, 3, 818).
4. **[corpus.yml first run] Pending until after the push.**

## Known Risks / Unverified

- corpus.yml has not run yet. GitHub's ubuntu runners have historically allowed `unshare -rn`. If they do not, the job fails at the gate's first step by design.
- The daemon sample uses the poll watcher (200 ms) on Linux only. The darwin native path is not run locally.

## Self-Check: PASSED
