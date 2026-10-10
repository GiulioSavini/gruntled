---
phase: 09-blast-radius
verified: 2026-10-10T11:09:08Z
status: passed
score: 4/4 must-haves verified
covered_files:
  - ".planning/phases/09-blast-radius/09-01-PLAN.md"
  - ".planning/phases/09-blast-radius/09-01-SUMMARY.md"
  - ".planning/phases/09-blast-radius/09-02-PLAN.md"
  - ".planning/phases/09-blast-radius/09-02-SUMMARY.md"
  - ".planning/phases/09-blast-radius/09-03-PLAN.md"
  - ".planning/phases/09-blast-radius/09-03-SUMMARY.md"
  - "cmd/gruntled/main.go"
  - "cmd/gruntled/testdata/script/blast_exitcodes.txtar"
  - "cmd/gruntled/testdata/script/blast_impacted.txtar"
  - "cmd/gruntled/testdata/script/blast_nobase.txtar"
  - "docs/cli.md"
  - "internal/application/blasting/blasting.go"
  - "internal/application/blasting/blasting_test.go"
  - "internal/domain/impact/impact.go"
  - "internal/domain/impact/impact_test.go"
  - "internal/domain/impact/surface.go"
  - "internal/domain/impact/surface_test.go"
  - "internal/infrastructure/hclconv/blast_stability_test.go"
  - "internal/interfaces/presenter/blast.go"
  - "internal/interfaces/presenter/blast_test.go"
  - "internal/interfaces/presenter/escape.go"
covered_digest: "v3:sha256:87343ec3e235515c9d93fb5b07f96775590ecb1ccb1eddd6f48bd3d920b186ac"
behavior_unverified: 0
overrides_applied: 0
re_verification:
  previous_status: passed
  previous_score: 4/4
  reason: "Report went stale: 09-03-SUMMARY.md frontmatter quoting fix (bf9143e) and Phase 12 plan 12-04 changed internal/interfaces/presenter/blast.go (escapeTerm in BlastText incl. the --base label, escapeJSON in BlastJSON)"
  gaps_closed: []
  gaps_remaining: []
  regressions: []
human_verification: []
---

# Phase 9: Blast Radius Verification Report

**Phase Goal:** User can see which units a change breaks and which it puts at risk, against a baseline directory.
**Verified:** 2026-10-10T11:09:08Z (HEAD `8a94bb8`)
**Status:** passed
**Re-verification:** Yes. The 2026-10-08 report (passed, 4/4, legacy format without a digest) went stale. No gaps were open, so every must-have was re-verified in full against current HEAD, not as a regression spot-check.

## Re-verification Note

What changed since the 2026-10-08 report (`b96c4c8`):

- `bf9143e` (2026-10-10): `09-03-SUMMARY.md` frontmatter only. The `decisions` item `--base "" is treated as no --base` was requoted so the YAML parses (it had hidden BLAST-01/02 from the milestone audit). No content change.
- `2f1815c` `fix(09-sec)`: the baseline in `impact.NewFindings` became a multiset (09-sec#1). This landed two minutes after the original report, so the original report did not cover it. It is covered now.
- `08f0c41`, `1508fbf` (Phase 12, plan 12-04): `escape.go` added; `BlastText` writes the `--base` label, subjects, file paths, messages, unit and module paths and change names through `escapeTerm`; `BlastJSON` passes the encoder buffer through `escapeJSON`. `blast_test.go` gained only additions (`TestBlastTextEscapesControls`, `TestBlastJSONEscapesTerminalRunes`). No blast golden or testscript line was removed or changed.
- `7fab2d4`, `c2d73b0`, `3151eac` (Phases 10, 11): `cmd/gruntled/main.go` gained `watch` and `report`. `runBlast` is unchanged in behavior (re-read below).

12-04 does not change output for normal names. A pre-12-04 binary built from `git archive 1508fbf^` and the HEAD binary gave byte-identical stdout, stderr and exit code on 9 blast invocations: base vs renamed output (text and json), no base (text and json), identical trees (text and json), moved pre-existing finding, and duplicated finding (text and json). For a `--base` directory named `evil<ESC>[31mX`, HEAD prints `baseline: evil\x1b[31mX` and the pre-12-04 binary printed the raw ESC byte.

## Goal Achievement

### Observable Truths

| # | Truth (ROADMAP SC) | Status | Evidence (HEAD `8a94bb8`) |
|---|--------------------|--------|---------------------------|
| 1 | `gruntled blast --base <dir> <path>` prints two disjoint, sorted sets, Broken and Impacted, in text and json | VERIFIED | Live: `blast --base base cur` (vpc_id renamed to network_id in cur) printed `baseline: base`, `Broken (1)` live/app GRT001, `Impacted (1)` `live/network (module modules/network: -output vpc_id, +output network_id)`, exit 1. `--format json` gave valid JSON (`kind: blast`, `baseline: true`, `added_outputs: ["network_id"]`, `removed_outputs: ["vpc_id"]`, summary 1/1), exit 1. This exit was not masked, unlike the original report. Sorting and disjointness are covered by the `TestDisjoint` property test (generated trees: strictly sorted, so unique, and disjoint) and by `blast_impacted.txtar` (live/app appears once, under Broken only). |
| 2 | A unit with a finding present in `<path>` and absent in `<dir>` is Broken; a finding present in both is never reported as Broken | VERIFIED | Live: the GRT001 new in cur makes live/app Broken. Base `pre` (GRT001 at line 6) vs `moved` (3 comment lines inserted, so the finding is at 9:12, confirmed by a no-base run) gave `Broken (0)`, exit 0. `KeyOf` drops Line/Column. Unit tests `TestNewFindings`, `TestNewFindingsCountsDuplicates` and `TestBlastShiftedFindingNotBroken`, the GRT100 stability test in hclconv, and `blast_impacted.txtar` (GRT100 present in both trees, not reported) pass. |
| 3 | A unit directly consuming a module that gained or lost a `variable` or `output` name is Impacted; a module with an unchanged surface impacts nothing | VERIFIED | Live: live/network (`source = ../../modules/network`) was Impacted after the output rename. A comment-only edit to `modules/network/outputs.tf` gave `Impacted (0)`. Identical trees gave `Broken (0)` and `Impacted (0)`, exit 0. `Compute` takes one hop over `curG.Units()` → `u.Module()`, and `SurfaceDiff` compares names only and skips added, deleted and unknown modules. `blast_impacted.txtar` covers +variable/-output, consumers of consumers not Impacted, comment/reorder edits, and new/deleted modules. `TestSurface` and `TestSurfaceIdenticalAndNil` pass. |
| 4 | Without `--base`, only Broken is reported and labelled "no baseline" | VERIFIED | Live: `blast cur` printed `baseline: none (no baseline)` and `Broken (1)` with no Impacted section, exit 1. JSON gave `baseline: false`, `note: "no baseline"`, `impacted: []`, exit 1. `--base ""` is also treated as no base (first line `baseline: none (no baseline)`). A bad `--base` exits 3 and never falls back: a nonexistent path printed `cannot open repository ... no such file or directory`, and a file path printed `not a directory`. `TestBlastNoBaseline`, `TestBlastNilBaseSkipsBaseline`, `TestNoBaseline` and `blast_nobase.txtar` pass. |

**Score:** 4/4 truths verified (0 present, behavior-unverified)

PLAN-level must-haves (09-01..09-03) were checked too and all hold. They cover a stage-tagged `*Error` (`TestBlastStagedErrors`), JSON lists never null (`TestBlastJSONNilListsNeverNull`), determinism (`TestBlastDeterministic`), exit codes 0/1/2/3 (live: bad `--format sarif` gave 2, more than one path gave 2, bad `--base` gave 3; `blast_exitcodes.txtar`), and docs/cli.md matching `blast -h` (all 5 exit-code lines of `blast -h` appear verbatim in docs/cli.md).

### Required Artifacts

| Artifact | Expected | Status | Details |
|----------|----------|--------|---------|
| `internal/domain/impact/impact.go` | FindingKey, KeyOf, NewFindings, Result, BrokenUnit, ImpactedUnit, Compute, NoBaseline | VERIFIED | `verify.artifacts` 09-01: 3/3. Substantive multiset NewFindings; used by blasting. |
| `internal/domain/impact/surface.go` | SurfaceChange, SurfaceDiff | VERIFIED | Called from Compute. |
| `internal/infrastructure/hclconv/blast_stability_test.go` | GRT100 position-free message test | VERIFIED | `go test -run 'GRT100\|Stability' ./internal/infrastructure/hclconv/` ok. |
| `internal/application/blasting/blasting.go` | Sources, Blast, Error | VERIFIED | `verify.artifacts` 09-02: 2/2. Imported and called by `runBlast`. |
| `internal/interfaces/presenter/blast.go` | BlastText, BlastJSON | VERIFIED | Now uses escapeTerm/escapeJSON (12-04); called by `runBlast`. |
| `internal/interfaces/presenter/escape.go` | escapeTerm, escapeJSON (12-04, on the blast output path) | VERIFIED | `TestEscapeTerm*`, `TestEscapeJSON*`, fuzz seeds pass; no-op for printable input (`TestEscapeTermNoAllocForNormalNames`). |
| `cmd/gruntled/main.go` | runBlast, blastUsage, case blast | VERIFIED | `verify.artifacts` 09-03: 2/2. `case "blast": return runBlast(...)`. |
| `docs/cli.md` | blast section, Known limitations entry | VERIFIED | Blast text/JSON sections, exit codes match `blast -h`. |

### Key Link Verification

| From | To | Via | Status | Details |
|------|----|-----|--------|---------|
| `impact.go` | `diagnostic.Diagnostic.Key()` | `KeyOf` | WIRED | `verify.key-links` 09-01 1/1 |
| `blasting.go` | `checking.Check`, `impact.Compute` | `Blast` | WIRED | `verify.key-links` 09-02 1/1 |
| `main.go` | `blasting.Blast`, `presenter.BlastText/BlastJSON` | `runBlast` | WIRED | `verify.key-links` 09-03 1/1 |
| `blast.go` | `escape.go` | `escapeTerm` / `escapeJSON` | WIRED | Read in source; `TestBlastTextEscapesControls`, `TestBlastJSONEscapesTerminalRunes`, live ESC label check |

### Data-Flow Trace (Level 4)

| Artifact | Data Variable | Source | Produces Real Data | Status |
|----------|---------------|--------|--------------------|--------|
| `BlastText`/`BlastJSON` | `impact.Result` | `blasting.Blast` → `checking.Check` on both `os.OpenRoot` trees → `impact.Compute` | Yes: live runs show real GRT001 positions and real surface names from the fixture | FLOWING |

### Behavioral Spot-Checks

Binary: `go build -o /tmp/claude-1000/verify9/gruntled ./cmd/gruntled`, fixture copies of `cmd/gruntled/testdata/clean-fixture`.

| Behavior | Command | Result | Status |
|----------|---------|--------|--------|
| Renamed output → Broken + Impacted (text) | `blast --base base cur` | Broken (1) live/app GRT001; Impacted (1) live/network `-output vpc_id, +output network_id`; exit 1 | PASS |
| Same, json | `blast --base base cur --format json` | valid JSON, summary 1/1, exit 1 | PASS |
| No base | `blast cur` / `--format json` | `baseline: none (no baseline)`, no Impacted; json `baseline:false`, `note`, `impacted: []`; exit 1 | PASS |
| Identical trees | `blast --base base base` (text, json) | Broken (0), Impacted (0); exit 0 | PASS |
| Bad `--base` | `--base /nonexistent/zz`, `--base <file>` | `cannot open repository`, exit 3 | PASS |
| Usage errors | `--format sarif`; two paths | exit 2 | PASS |
| Pre-existing finding moved + comment-only module edit | `blast --base pre moved` | Broken (0), Impacted (0), exit 0 | PASS |
| Duplicate identical finding added | `blast --base pre dup` | Broken (1), exit 1 (09-sec#1 fix) | PASS |
| 12-04 does not change normal output | HEAD vs `1508fbf^` binary, 9 invocations | byte-identical stdout/stderr/exit | PASS |
| Hostile `--base` label | `--base $'evil\e[31mX'` | `baseline: evil\x1b[31mX` (old binary: raw ESC) | PASS |

Test runs (each run once):

- `go test -count=1 ./internal/domain/impact/ ./internal/application/blasting/ ./internal/interfaces/presenter/`: ok (3 packages).
- `go test -count=1 -run 'Blast|TestScripts|Golden' ./cmd/gruntled/`: ok. `TestScripts/blast_nobase`, `blast_impacted` and `blast_exitcodes` pass.
- `go test -count=1 -run 'GRT100|Stability' ./internal/infrastructure/hclconv/`: ok.

### Probe Execution

None. The phase declares no `scripts/*/tests/probe-*.sh`, and this is not a migration or tooling phase.

### Requirements Coverage

| Requirement | Source Plan | Description | Status | Evidence |
|-------------|-------------|-------------|--------|----------|
| BLAST-01 | 09-01, 09-02, 09-03 | `blast --base <dir> <path>`: disjoint, sorted Broken/Impacted in text and json; without baseline only Broken, "no baseline" | SATISFIED | Truths 1, 4; live runs; 09-03-SUMMARY frontmatter now parses and lists it |
| BLAST-02 | 09-01, 09-03 | Impacted only for direct (one-hop) consumers of a module whose variable/output names changed; unchanged surface impacts nothing; pre-existing findings never Broken | SATISFIED | Truths 2, 3; live moved-finding and comment-only runs; `blast_impacted.txtar` |

No orphaned requirements. REQUIREMENTS.md maps only BLAST-01 and BLAST-02 to Phase 9.

### Decision Coverage

Not evaluated. The phase has no CONTEXT.md, so the gate skips cleanly.

### Test Quality Audit

| Test File | Linked Req | Active | Skipped | Circular | Assertion Level | Verdict |
|-----------|-----------|--------|---------|----------|-----------------|---------|
| `internal/domain/impact/impact_test.go` | BLAST-01, BLAST-02 | yes | 0 | no | Value + property (`TestDisjoint`) | OK |
| `internal/domain/impact/surface_test.go` | BLAST-02 | yes | 0 | no | Value | OK |
| `internal/application/blasting/blasting_test.go` | BLAST-01, BLAST-02 | yes | 0 | no | Value/behavioral | OK |
| `internal/interfaces/presenter/blast_test.go` | BLAST-01 | yes | 0 | no (hand-written inline goldens, no `-update`) | Value | OK |
| `cmd/gruntled/testdata/script/blast_*.txtar` | BLAST-01, BLAST-02 | yes | 0 | no | Behavioral end-to-end | OK |

Disabled tests on requirements: 0. Circular patterns: 0. Insufficient assertions: 0.

### Anti-Patterns Found

| File | Line | Pattern | Severity | Impact |
|------|------|---------|----------|--------|
| `internal/interfaces/presenter/escape.go` | 102-103, 153 | `XXX` in `\uXXXX` | Info | Hex-escape notation in doc comments, not a debt marker |
| `internal/interfaces/presenter/blast.go` | 155 | `XXX` in `\uXXXX` | Info | Same |
| `docs/cli.md` | 365 | `XXX` in `\uXXXX` | Info | Same |
| `internal/domain/impact/impact.go` | `NewFindings` | duplicate-key attribution | Info | When an identical finding is added above an existing one, the multiset correctly counts 1 new finding. The position printed is whichever occurrence comes later in canonical order. Live: the new finding was at 6:12, and `blast` printed 7:12, the pre-existing finding after its shift. Unit, count and exit code are correct, and the two occurrences cannot be told apart by design (position-free key). Not a must-have failure. |

No TBD/FIXME/TODO/HACK/placeholder markers in the phase files. No stubs.

### Advisory (New Scope, Unevidenced)

None.

### Human Verification Required

None. Every success criterion is CLI output that was exercised directly, with exact assertions in the testscripts.

### Gaps Summary

No gaps. All four roadmap success criteria and both requirements still hold at HEAD `8a94bb8`. The Phase 12 (12-04) escaping is wired into both blast presenters. It leaves output for normal names byte-identical, which was checked against a pre-12-04 build, and it neutralises control sequences in the `--base` label and repository-controlled names. 09-sec#2 (raw terminal controls in blast text) is closed by 12-04. 09-sec#3 is recorded as closed by 12-04 (T-12-04-3: the label is escaped). The label is still printed as given, so an absolute `--base` path still appears on stdout. That is the user's own argument echoed back, which is informational only. The Security section below is the original 2026-10-08 audit, kept verbatim. Its "Open" statuses describe the state at that date.

---

_Re-verified: 2026-10-10T11:09:08Z_
_Verifier: Claude (gsd-verifier)_

---

## Original Verification (2026-10-08, kept verbatim)

**Phase Goal:** User can see which units a change breaks and which it puts at risk, against a baseline directory.
**Status:** passed. Initial verification, not a re-verification.

## Truths

| # | Truth | Status | Evidence |
|---|-------|--------|----------|
| 1 | `blast --base <dir> <path>` prints disjoint, sorted Broken and Impacted in text and json | VERIFIED | Manual run printed both formats. The json output has `broken`, `impacted` and `summary`. Script tests blast_impacted, blast_nobase and blast_exitcodes pass. |
| 2 | A finding new in path is Broken; a finding in both is never Broken | VERIFIED | Removing `output vpc_id` made live/app Broken (GRT001). Base vs base gave Broken (0), exit 0. The unit tests in domain/impact and the pre-existing-finding script test pass. |
| 3 | A direct consumer of a module whose variable/output names changed is Impacted; an unchanged surface impacts nothing | VERIFIED | live/network was Impacted (`modules/network: -output vpc_id`). Base vs base gave Impacted (0). |
| 4 | Without `--base`, only Broken is reported, labelled "no baseline" | VERIFIED | Output was `baseline: none (no baseline)` and Broken (1), with no Impacted section. Exit code 1. |

**Score:** 4/4.

## Evidence

- `go test -count=1 ./...` passed in every package, including cmd/gruntled, application/blasting, domain/impact and interfaces/presenter. I did not run -race or scripts/test-check-architecture.sh.
- `go test ./cmd/gruntled -run TestScripts/blast -v` passed for blast_nobase, blast_impacted and blast_exitcodes.
- Manual run on two copies of clean-fixture, with `modules/network/outputs.tf` emptied in cur:
  - `blast --base base cur` (text) printed `baseline: base`, then Broken (1) with live/app GRT001, then Impacted (1) with live/network. Exit 1.
  - `blast --base base cur --format json` returned valid JSON with `baseline: true`, one broken entry, one impacted entry (`removed_outputs: ["vpc_id"]`) and a summary of 1 broken and 1 impacted. Exit 0 in my shell, but the exit code was masked by a `||` fallback in my command. The text run and the no-base run show exit 1 for the same condition, and the script test blast_exitcodes passes.
  - `blast cur` (no base) printed `baseline: none (no baseline)` and Broken (1). Exit 1.
  - `blast --base base base` printed Broken (0) and Impacted (0). Exit 0.
  - `--json` is not a flag. It gave a usage error with exit 2, and the real flag is `--format json`.

## Requirements

| ID | Plans | Status |
|----|-------|--------|
| BLAST-01 | 09-01, 09-02, 09-03 | SATISFIED. Marked complete in REQUIREMENTS.md. 09-03-SUMMARY lists it as completed. |
| BLAST-02 | 09-01, 09-03 | SATISFIED. Marked complete in REQUIREMENTS.md. 09-01 and 09-03 SUMMARYs list it as completed. |

No orphaned requirements.

## Anti-patterns

None blocking. I did not run a stub scan beyond the behavioral checks above, which exercised real output.

## Human verification

None required.

_Verifier: Claude (gsd-verifier)_

## Security

Audit by `proj-sec:auditor` on 2026-10-08, diff `48a41d4..b96c4c8`. Verdict: fix-small.

| # | Severity | Location | Problem | Status |
|---|----------|----------|---------|--------|
| 1 | medium | `internal/domain/impact/impact.go` `NewFindings` | The baseline was treated as a set. A second identical GRT001 added on another line in the same unit and file was hidden, and `blast` exited 0. | Fixed in `fix(09-sec)`: the baseline is now a multiset. Regression test `TestNewFindingsCountsDuplicates`. |
| 2 | low | `internal/interfaces/presenter/blast.go:95-105,135-147` | Text output prints variable and output names, paths and GRT100 messages raw. `tfsurface` does not validate labels, so a hostile repo can inject terminal escape sequences. JSON output is safe. The `check` text presenter has the same exposure. | Open. Quote with `strconv.Quote`, or reject labels that `hclsyntax.ValidIdentifier` refuses. Fix across presenters in one go. |
| 3 | low | `cmd/gruntled/main.go` runBlast → `BlastText(..., *base)` | The text header echoes the `--base` argument verbatim, so an absolute path appears on stdout. | Open. Print a fixed label or `filepath.Base`. |
| info | — | `impact.FindingKey` | A finding that goes from warning to error is not counted as new. This is the documented design. | Revisit if exit codes should count it. |

Checked and OK:
- Both trees are opened with `os.OpenRoot`, so neither symlinks nor `..` escape the root.
- Every error path exits 3, with no silent fallback to "no baseline".
- No new imports or dependencies.
- Memory grows linearly.
- JSON output is deterministic.

Not run:
- `govulncheck`: go1.26 toolchain against go1.27 packages.
- `-race`: no C compiler.
