---
phase: 16-daemon-baseline-report-blast-rebase
plan: 04
subsystem: proofs, gate
tags: [daemon, blast, equivalence, property, memory, compatibility, gate]
requires: [16-00, 16-03]
provides:
  - "DAEMON-07 equivalence property with a value-checking normaliser, plus e2e on both transports"
  - "SEC-01 and BLAST-11 report --blast halves; T-13-02-7 end to end"
  - "DAEMON-10 memory record in docs/validation.md"
  - "scripts/compat-v1.sh: the DAEMON-09 matrix against the real v0.3.0 binary"
affects: []
tech-stack:
  added: []
  patterns: ["normaliser that asserts what it removes", "built-binary daemon with rusage after SIGTERM", "trap cleanup that kills background daemons before rm"]
key-files:
  created:
    - cmd/gruntled/daemon_blast_test.go
    - cmd/gruntled/daemon_mem_test.go
    - scripts/compat-v1.sh
    - internal/infrastructure/ipc/race_on_test.go
    - internal/infrastructure/ipc/race_off_test.go
  modified:
    - cmd/gruntled/escape_test.go
    - cmd/gruntled/blast_bound_test.go
    - cmd/gruntled/validation_doc_test.go
    - docs/validation.md
    - README.md
    - internal/infrastructure/ipc/ipc_test.go
    - cmd/gruntled/rebase_test.go
    - cmd/gruntled/testdata/script/rebase_usage.txtar
key-decisions:
  - "The normaliser spells the expected label out with fmt, not through presenter.DaemonBaselineLabel, so a wrong presenter cannot agree with itself"
  - "The memory test measures one Report with its indexer dropped, and the publisher sealed as instance.store seals"
  - "compat-v1.sh keeps the literal `trap cleanup EXIT INT TERM HUP`; cleanup exits non-zero when the script did not reach its end"
requirements-completed: [DAEMON-07, DAEMON-09, DAEMON-10]
duration: 75min
completed: 2026-10-12
---

# Phase 16 Plan 04: Proofs and Phase Gate Summary

**`report --blast` equals `blast --base <copy of the baseline>` except for the label line and the `daemon_baseline` member. A seeded property shows it over 200 cases (1,000 without `-short`), through fresh loaders and through the daemon's shared invalidating loader. The normaliser asserts the exact label and the member's values and position. Three scenario pairs show it through a real daemon on both transports. Three recorded mutants fail it, including the off-by-one label. The SEC-01 and BLAST-11 halves pass through the daemon, and so does Pitfall 15 end to end. The daemon baseline costs about 2.3 KB per unit (publisher state 2.37 x one Report at 5,000 units), now recorded in docs/validation.md. `scripts/compat-v1.sh` builds v0.3.0 from its tag and gets 7/7 against real daemons in both directions, with no daemon left behind, also when interrupted. The local gate is green. Three CI failures left by 16-01/16-03 (race alloc bound, two windows rebase rows) are fixed test-side.**

## Performance

- Duration: ~75 min
- Tasks: 3
- Files: 5 created, 8 modified

## Accomplishments

### Task 1: equivalence, e2e, Pitfall 15, SEC-01 (cmd/gruntled/daemon_blast_test.go, escape_test.go)
- **Normaliser** (sec F7):
  - `normaliseDaemonBlast` (:46). For text, line 1 must equal `baseline: daemon generation <G0> (<source>), current generation <G>`, spelled out with fmt; it is then replaced by `baseline: X`.
  - For JSON, exactly one `"daemon_baseline"` line in the document. It must be `  "daemon_baseline": {`, directly after `  "baseline": true,`, and be exactly three members then `  },`. The block is decoded with DisallowUnknownFields and its values compared with the expected ones; only then is it deleted.
  - `normaliseBlastBase` (:96) needs a `baseline: ` first line (text) and refuses any member (JSON).
- **TestNormaliseDaemonBlast** (:110):
  - normalised daemon text and JSON equal blast --base, and the normalised daemon JSON is byte-equal to BlastJSON (exactly one block removed);
  - 14 failing shapes are refused: wrong generation, current generation or source (text and JSON), missing label line, a blast --base label, a label with a suffix, zero blocks, two blocks, a misplaced block, an extra member, an unknown member. Two blast --base shapes are refused too.
- **TestDaemonBlastEqualsBlastBase** (:566):
  - in-test xorshift generator: 1-3 modules (typed variables, sensitive outputs), 4-12 units under `live/`, dependency chains biased to the previous unit, about half reading an output;
  - 1-3 of 11 mutations per case: remove or rename an output (a referenced one 3 times in 4), add a variable, change a type, toggle sensitive, GRT001, GRT002, GRT100, move lines, delete a unit, extend the chain;
  - variant a uses fresh loaders; variant b uses one invalidating loader over an `os.Root` with the dirty paths from `applyTree` (:453), the daemon cache path;
  - text, JSON and exit code are compared with `run(blast --base ...)`.

  | Run | Broken | GRT004 | distance ≥ 2 | type changes | Time |
  |---|---:|---:|---:|---:|---:|
  | 200 cases (`-short`) | 120 | 42 | 66 | 102 | 2.2 s |
  | 1,000 cases | 687 | 256 | 295 | 434 | 14.6 s |

  No variant-b-only mismatch appeared, so no Loader cache bug was found.
- **TestReportBlastE2E** (:810), on both transports:
  - three pairs: GRT004 plus a distance-4 chain; a type-only change; GRT100 plus GRT002;
  - `waitBlastFinal` (:696) waits until the label's current generation is ≥ 2 AND plain report equals check;
  - `assertBlastEquivalent` (:717) checks the label is `(watch start)` at generation 1, then compares text and JSON with blast --base over the copy, and the exit codes;
  - each scenario's key lines must be present (non-vacuity);
  - on the socket transport: `rebase --expect G`, then the text is exactly `baseline: daemon generation G (rebase), current generation G\nBroken (0):\nImpacted (0):\n` with exit 0, and the JSON has `[]` lists and source `rebase`.
- **TestReportBlastPitfall15** (:871): before rebase, `app/terragrunt.hcl:4:17: GRT004` is in Broken and there is no GRT001. rebase prints `; 1 error findings in Broken accepted`. After it, `Broken (0):`, no GRT004, exit 0, while plain report still has `app/terragrunt.hcl:4:17: GRT001`.
- **TestTextOutputsEscapeControls:**
  - the blast want lines are hoisted into `blastWant` and the blast JSON predicate into `blastJSONHas`, both shared;
  - a daemon starts on a copy of `craftedRepo(·, true)`, which is then rewritten to `craftedRepo(·, false)` (escape_test.go:331);
  - subtests `report --blast` (:343) and `report --blast json` (:358): no raw control, every `blastWant` line after the label, the same JSON predicate, and normalised equality with `blast --base <crafted base>`.

### Task 2: memory and the report bound (daemon_mem_test.go, blast_bound_test.go, docs/validation.md)
- **TestDaemonMemorySynthrepo** (daemon_mem_test.go:55):
  - synthrepo `{Units, IncludeDepth 3, DependencyFanout 3, Seed 16}`;
  - `report_heap` is one Report from the daemon's Indexer path, with the indexer dropped;
  - `publisher_heap` is a publisher whose snapshots are sealed as `instance.store` seals them. It holds rep1 and then rep2, where rep2 is unit-000's first output renamed and reindexed with that dirty path on a warm indexer;
  - it asserts `publisher ≤ 2.5 x report + view + 1 MiB` and that the edit is in the view;
  - the 5,000-unit size is skipped under `-short`.
- **TestReportBlastSizeBound** (blast_bound_test.go:278):
  - `startBoundDaemon` (:196) runs the built binary's `watch --poll --poll-interval 50ms` with a private XDG_RUNTIME_DIR and TMPDIR;
  - it polls `report` (60 s), rewrites the fixture path modules/big/main.tf through a rename, then polls `report --blast --format json` until 5,000 impacted (180 s);
  - after SIGTERM it reads Maxrss;
  - it applies TestBlastSizeBound's byte bounds and the single type-string check, plus RSS(big) ≤ RSS(control) + 64 x typeBytes (:365).
- **docs/validation.md** has a new `## v0.4 daemon memory (DAEMON-10)` section: method, table, VALIDATION-BENCH lines, the comparison with the ARCHITECTURE estimate, the BLAST-11 daemon figures and the command. TestValidationDocPins was red first in b2d1472; it pins the heading, the three `VALIDATION-BENCH daemon_mem units=N ` lines and the test name.

Local measurements, each run once on its own after `free -m` showed 6.9 GB available, under `timeout 900`:

```
VALIDATION-BENCH daemon_mem units=65 report_heap=352256 publisher_heap=876544 view_bytes=20424 ratio=2.49
VALIDATION-BENCH daemon_mem units=500 report_heap=1171456 publisher_heap=2826240 view_bytes=151221 ratio=2.41
VALIDATION-BENCH daemon_mem units=5000 report_heap=11460608 publisher_heap=27172864 view_bytes=1609736 ratio=2.37
--- PASS: TestDaemonMemorySynthrepo (7.68s)

control (same-size file, tiny type): initial index 2.644s, reindex to 5,000 impacted 2.638s; text 727709 bytes, json 759584 bytes; daemon maxrss 627 MiB
big (4194078-byte type): initial index 3.124s, reindex to 5,000 impacted 3.282s; text 10234273 bytes, json 11384568 bytes; daemon maxrss 635 MiB
rendered old+new 9506576 bytes (source type 4194078 bytes)
daemon maxrss control 627 MiB, big 635 MiB, bound 883 MiB (control + 64 x 4194078 type bytes); linux/amd64, 8 CPUs
--- PASS: TestReportBlastSizeBound (14.62s)
```

Machine: linux/amd64, 8 CPUs, Intel Core Ultra 7 155H, 8,132,820 kB, WSL2 kernel 6.6.87.2, go1.27.2. No daemon was left running after the bound test.

### Task 3: compat-v1.sh and the gate
- **scripts/compat-v1.sh `[ref]`** (default v0.3.0):
  - verifies the ref with `rev-parse --verify --quiet --end-of-options ^{commit}` (:34) and exports `GOFLAGS=-mod=readonly GOPROXY=off GOTOOLCHAIN=local` (:32);
  - builds `$T/gref` in a detached worktree and `$T/ghead` from the working tree, and runs the sarif-fixture copy;
  - each daemon is a plain `"$bin" watch ... &` with `pids+=($!)`, its own `XDG_RUNTIME_DIR`/`TMPDIR` (mkdir -m 700) and `--status-file $T/st/status`;
  - every client runs under `timeout 20` (:97);
  - `trap cleanup EXIT INT TERM HUP` (:71). cleanup does, in order and each `|| true`: TERM, a wait of up to 5 s, KILL plus wait, worktree remove --force, worktree prune, rm -rf. It exits non-zero if the script did not reach its end;
  - the final matrix line runs `pgrep -f -- "$T"` (:170).
- **Self-checks:**
  - `compat-v1.sh HEAD` (both v2) gives 3/7 with DIFF lines and exit 1, so the matrix can fail;
  - INT, TERM and HUP sent while the v0.3.0 daemon ran each exited non-zero, left no gref/ghead process, no extra worktree and no `/tmp/tmp.*` dir.
- **README** documents the script next to the other maintenance scripts.

## Gate (at 40dd3b2)

Run with `GOMAXPROCS=2 GOFLAGS=-p=2`, sequentially; compat-v1 ran alone:

```
bash scripts/compat-v1.sh v0.3.0
ok v0.3.0 watch ready
ok v0.3.0 watch, HEAD report
ok v0.3.0 watch, HEAD report --blast
ok v0.3.0 watch, HEAD rebase
ok HEAD watch ready
ok HEAD watch, v0.3.0 report
ok no daemon left running
compat-v1: 7/7
pgrep -fa gruntled                       (no output)
bash scripts/check-architecture.sh
architecture: OK (4 domain packages, 5 application packages, 1 interfaces packages)
bash scripts/compare-ref.sh v0.3.0 | tail -1
244/244 identical                        (then: branch master, clean tree, one worktree)
bash scripts/test-txtar-extract.sh | tail -1
all txtar-extract cases passed
go mod verify
all modules verified
git diff --exit-code 56c0e39 -- go.mod go.sum
go.mod/go.sum unchanged
bash scripts/build-release.sh v0.0.0-gate <sha> <scratch>/dist
rc=0 (6 archives + checksums.txt; output removed)
go vet ./...                             vet-ok
GOOS=windows GOARCH=amd64 go vet ./cmd/gruntled/ ./internal/infrastructure/ipc/   win-vet-ok
GOOS=darwin go vet ./cmd/gruntled/       darwin-vet-ok
gofmt -l .                               (no output)
go test -count=1 -short -timeout 900s ./internal/...
ok  blasting 0.373s, checking, indexing, watching, analysis 0.131s, diagnostic, impact 0.930s,
    repograph, hclconv 5.434s, ipc 0.697s, sourceresolve, statusfile, terragrunt 5.616s,
    tfsurface 5.578s, watch 1.181s, presenter 0.073s, synthrepo 0.030s (ports: no test files)
go test -count=1 -short -timeout 900s ./cmd/gruntled/
ok  github.com/GiulioSavini/gruntled/cmd/gruntled	9.857s
grep -rnE '\.Value\(|typeConstraint\(' internal --include=*.go | grep -v _test.go | grep -v ':\s*//'
internal/infrastructure/hclconv/literal.go:26
internal/infrastructure/tfsurface/typesig.go:161
internal/infrastructure/terragrunt/eval.go:28
internal/infrastructure/terragrunt/eval.go:76
```

Also run: the Task 1 verify line (`TestDaemonBlastEqualsBlastBase|TestNormaliseDaemonBlast|TestReportBlastE2E|TestReportBlastPitfall15|TestTextOutputsEscapeControls`, `-short`) is ok, and the 1,000-case property without `-short` is ok in 14.6 s.

Not run locally: `-race`, `-fuzz`, the windows and macOS jobs, and `scripts/test-check-architecture.sh` (no architecture script changed). CI is the proof for these after the push. The 16-04 commits are not pushed.

## Mutation proofs (throwaway `git archive` copy, one at a time, `-short`)

| Mutant | Result |
|---|---|
| Baseline from every publish (`base := rep` in commitReady, keeping baseGen) | TestDaemonBlastEqualsBlastBase FAIL (`daemon exit 0, blast exit 1`); TestReportBlastE2E FAIL |
| Label written into Broken lines (BlastText) | TestDaemonBlastEqualsBlastBase FAIL (`view differs ... line 3`); TestReportBlastE2E FAIL |
| `current_generation` off by one in DaemonBaselineLabel | TestDaemonBlastEqualsBlastBase and TestNormaliseDaemonBlast FAIL (`first line ... current generation 3, want ... 2`); TestReportBlastE2E FAIL |
| Unmutated copy | ok |

The first version of the normaliser built the expected line with `presenter.DaemonBaselineLabel`, so the off-by-one mutant agreed with itself and passed the property (only the e2e caught it). The normaliser now spells the label out with fmt. The fix is in the Task 1 commit, and the mutant now fails the property, as sec F7 requires.

## Task Commits

1. `46576c1` test(16-04): report --blast equals blast --base: property, e2e on both transports, Pitfall 15, SEC-01 escape
2. `b2d1472` test(16-04): daemon memory at 65/500/5,000 units, report --blast size bound, pin the DAEMON-10 record
3. `c988d36` test(16-04): the bound test allows plain report's stderr summary
4. `5877d24` docs(16-04): record the daemon baseline memory at 65, 500 and 5,000 units
5. `668a946` feat(16-04): compat-v1.sh checks the protocol mismatch against the real v0.3.0 binary
6. `190b82d` test(16-04): the blast request alloc bound allows the race detector's overhead
7. `40dd3b2` test(16-04): rebase windows rows: the windows message, a held lock is unreadable

This summary, with 16-VALIDATION.md set to green, is its own `docs(16-04): summary` commit.

## Threat Mitigations

| ID | Disposition | Where |
|----|-------------|-------|
| T-16-04-1 | mitigated | normaliseDaemonBlast daemon_blast_test.go:46 (exact label via fmt, one block, position, three members, values); TestNormaliseDaemonBlast :110 (14 refused shapes, removal count exactly 1); counters asserted :671; three mutants recorded above |
| T-16-04-2 | mitigated | variant b via cachedReports :487 (one invalidating loader, dirty paths from applyTree :453); TestReportBlastE2E :810 through the real daemon |
| T-16-04-3 | mitigated | escape_test.go:343 and :358: assertNoRawControls, blastWant lines, blastJSONHas round-trip, normalised equality |
| T-16-04-4 | mitigated | TestReportBlastSizeBound blast_bound_test.go:278 (byte bounds, single type string, RSS vs in-run control :365); TestDaemonMemorySynthrepo daemon_mem_test.go:55 (relative bound) |
| T-16-04-5 | mitigated | `-short` skips both heavy tests; each ran once locally on its own after `free -m` (6.9 GB available), under `timeout 900` |
| T-16-04-6 | mitigated | compat-v1.sh, both directions, exact stderr, exit 3 and empty stdout: 7/7 |
| T-16-04-7 | mitigated | compat-v1.sh:32 (GOFLAGS=-mod=readonly GOPROXY=off GOTOOLCHAIN=local), :34 (rev-parse --verify --end-of-options ^{commit}), worktree removed by the trap |
| T-16-04-8 | accepted | RSS compared only with the in-run control; absolute values logged |
| T-16-04-9 | mitigated | compat-v1.sh: pid from a plain `&`, `trap cleanup EXIT INT TERM HUP` :71, TERM then a 5 s wait then KILL before rm, `timeout 20` per client :97, `pgrep -f -- "$T"` line :170; checked with INT, TERM and HUP while a daemon ran |

## Deviations from Plan

**1. [TDD] Task 1 has no feat commit**
- The implementation landed in 16-01..16-03, so the Task 1 proofs passed on first run. Non-vacuity comes from the counters and the three recorded mutants, not from a red commit.
- Task 2's red commit is the doc pin (b2d1472); the docs commit turns it green.

**2. [Normaliser] The label is spelled out, not taken from the presenter**
- See Mutation proofs. With `DaemonBaselineLabel`, the off-by-one label mutant passed the property.

**3. [Memory method] One Report is measured with its indexer dropped**
- `report_heap` must be one Report, not a Report plus a parse cache, or the 2.5 x bound could not catch a pinned cache.
- rep2 comes from a second, warm indexer (Index(nil), then the edit and the dirty path), which is dropped before h2.
- The publisher seals like `instance.store`, so the sealed blast line is counted too.

**4. [Bound test] Plain `report` may print its summary on stderr**
- The first local run failed on that summary. Only `report --blast` must keep stderr empty (c988d36).

**5. [CI fixes outside the plan's files] Three failures left by 16-01/16-03 on the bf354a3 run**
- **Race alloc bound:** `check` (-race) failed TestBlastRequestAllocsConstant, with 93,000 and 79,112 bytes per 20 requests against a 64 KiB bound. That overhead is the race detector's: the 352-byte line also failed. ipc gets `raceAllocSlack` (256 KiB under `-race`, 0 otherwise; race_on/off_test.go); one per-request encode would still be over 50 MB. (190b82d)
- **Windows usage row:** windows failed TestScripts/rebase_usage, because the no-daemon row prints the windows message there. The row now has a `[windows]` variant. (40dd3b2)
- **Windows held lock:** windows failed TestRebaseWindowsUnsupported/report-file_daemon, because treeState cannot open the held lock file. On windows it now records `unreadable` and still compares mode, size and mtime. (40dd3b2)
- None of the three can be checked locally (no -race, no windows). The next CI run confirms them.

**6. [README] compat-v1.sh documented next to the other maintenance scripts**
- One paragraph in README.md.

**7. [Matrix rows] Two readiness rows**
- The two `ready` checks are counted, so the matrix is 7 rows: 2 readiness, 4 mismatch and 1 leak line. A daemon that never becomes ready also fails the gate.

## Known Risks / Unverified

- **CI time:** the 1,000-case property takes 14.6 s locally and will run under `-race` in CI. The `cmd/gruntled` package took 200 s under -race before this plan, against the 10-minute default. Expected headroom: about 200 s.
- **CI-only items still to confirm:** -race on TestRebaseConcurrentPublish; the fuzz step showing a non-zero final rate; macOS and windows; TestReportBlastSizeBound and TestDaemonMemorySynthrepo at 5,000 units on CI runners. Copy their logged values from the CI run after the push.

## Self-Check: PASSED
