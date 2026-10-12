---
phase: 15
slug: type-level-surface-facts
status: validated
nyquist_compliant: true
wave_0_complete: true
created: 2026-10-10
validated: 2026-10-12 (17-00)
---

# Phase 15 — Validation Strategy

> Per-phase validation contract for feedback sampling during execution.

---

## Test Infrastructure

| Property | Value |
|----------|-------|
| **Framework** | Go testing (domain/application: stdlib + in-test xorshift, no rapid/math/rand) + Go fuzzing (FuzzTypeSig, CI 30 s) + testscript + built-binary RSS test |
| **Config file** | none (CI fuzz step added to `.github/workflows/ci.yml` in 15-02) |
| **Quick run command** | `go test -count=1 ./internal/domain/repograph/ ./internal/domain/impact/ ./internal/infrastructure/tfsurface/ ./internal/infrastructure/hclconv/ ./internal/interfaces/presenter/ && go test -count=1 -run 'TestScripts/blast\|TestBlastDepth1MatchesV1\|TestRuleRegistryDoc\|TestTextOutputsEscapeControls' ./cmd/gruntled/` |
| **Full suite command** | `go test -count=1 ./... && go vet ./... && bash scripts/check-architecture.sh && bash scripts/test-check-architecture.sh && bash scripts/compare-ref.sh v0.3.0` (CI adds `-race` on three OSes and the fuzz step) |
| **Estimated runtime** | ~15 seconds (quick), ~2-3 minutes (full incl. compare-ref and TestBlastSizeBound) |

PATH for every command: `export PATH=$HOME/.local/go/bin:$HOME/go/bin:$PATH`.

---

## Sampling Rate

- **After every task commit:** quick run command
- **After every plan wave:** full suite command
- **Before `/gsd:verify-work`:** 15-05 gate evidence in 15-05-SUMMARY.md, CI green on three OSes
- **Max feedback latency:** 20 seconds

---

## Success Criteria → Tests

| # | Phase success criterion (ROADMAP) | Proven by |
|---|-----------------------------------|-----------|
| 1 | New/lost-default variable -> "now required"; `default = null` is a default; nullable/validation/ephemeral/deprecated alone change nothing | `TestReadSurfaceFacts`, `TestTypeChangeMatrix`, `blast_types.txtar` cases 1-3, 15; `blast_impacted.txtar` (required name) |
| 2 | Variable type / declared output type (both sides) / sensitive flip -> reason; formatting, comments, order, optional defaults, .tf->.tf.json -> empty | `TestTypeSigGolden`, `TestTypeSigEqualsProperty`, `TestReadSurfaceMetamorphic`, `TestTypeChangeMatrix`, `blast_types.txtar` cases 4-11, 16 |
| 3 | Never a diagnostic, never Broken, never exit code; --depth 1 equals v0.3 sets/exit | `TestTypeFactsNeverBroken`, `TestTypeFactsNeverChangeBrokenProperty`, `blast_types.txtar` case 14 (every case exit 0), `TestBlastDepth1MatchesV1` (extended normaliser, D-15-05) |
| 4 | Ambiguous facts silent: duplicates with differing facts, override files, non-literal/unparseable, unknown either side; names unchanged | `TestReadSurfaceOverride`, `TestReadSurfaceDuplicates`, `TestTypeChangeMatrix` unknown rows, `TestSurfaceViewsUnchanged`, `TestSurfaceDiffNamesOnlyUnchanged`, `blast_types.txtar` cases 7, 9, 12, 13; compare-ref v0.3.0 N/N |
| 5 | Hostile names escaped; 4 MiB type + 5,000-unit chain bounded; renderer never panics (fuzz in CI); six-target proof on first typeexpr import | `TestCheckNativeDepthTemplateControl`, `TestNestedTemplateDirectiveUnit/Module` (15-00, sec #261), `TestTypeSigJSONDeepString`, `TestTypeSigJSONLowBracketBombs`, `TestCheckNativeExprDepth`, `TestTypeSigJSONNewlineTernary` (sec #272), the four sec #271 directive forms and the 4.08 MB repro (15-00), `TestTypeSigNativeNestedIfDefault`, `TestJSONTypeStringExtraction`, `TestTypeSigNativeDeepBounded`, `TestReadSurfaceJSONDepthBomb`, `check_json_type_bomb.txtar` (sec #245), `TestReadSurfacePanickingParser` (sec #246), `TestTextOutputsEscapeControls` (15-05, incl. invalid UTF-8), `TestBlastTextEscapesTypeChanges`, `TestBlastSizeBound`, `TestBlastTypeStringOncePerModule`, `TestTypeSigCapsuleAndDepth`, `FuzzTypeSig` (CI step), 15-02 Task 1 verify (check-architecture Step 9), `typeexpr-only-in-tfsurface` rule |

---

## Per-Task Verification Map

| Task ID | Plan | Wave | Requirement | Test Type | Automated Command | File Exists | Status |
|---------|------|------|-------------|-----------|-------------------|-------------|--------|
| 15-00-01 | 00 | 0 | BLAST-07 (sec #261) | unit (pre-scan) | `go test -count=1 ./internal/infrastructure/hclconv/` | ✅ limits_test.go (extended) | ✅ green (local 17-00: hclconv ok) |
| 15-00-02 | 00 | 0 | BLAST-07 (sec #261) | loader regression + byte gate | `go test -count=1 ./internal/infrastructure/... && bash scripts/compare-ref.sh v0.3.0` | ❌ W0 (terragrunt + tfsurface limits_regression_test.go) | ✅ green (local 17-00: 7 infrastructure packages ok one at a time; compare-ref 244/244) |
| 15-01-01 | 01 | 1 | BLAST-06..08, 10 | unit | `go test -count=1 ./internal/domain/repograph/` | ✅ surface_test.go (extended) | ✅ green (local 17-00: repograph ok) |
| 15-01-02 | 01 | 1 | BLAST-08 | unit + byte gate | `go test -count=1 ./internal/infrastructure/... && bash scripts/compare-ref.sh v0.3.0` | ❌ W0 (hclconv/literal.go, literal_test.go) | ✅ green (local 17-00: infrastructure packages ok; compare-ref 244/244) |
| 15-02-01 | 02 | 2 | BLAST-07, SEC-01 | unit + property + fuzz + six-target | `go test -count=1 -run TestTypeSig ./internal/infrastructure/tfsurface/ && go test -run '^$' -fuzz FuzzTypeSig -fuzztime 20s ./internal/infrastructure/tfsurface/ && bash scripts/check-architecture.sh` | ❌ W0 (typesig.go, typesig_test.go) | ✅ green (local 17-00: TestTypeSig ok, check-architecture OK; fuzz: CI 38157144902, CI 38229787854 FuzzTypeSig step 161,135 execs/30 s) |
| 15-02-02 | 02 | 2 | BLAST-06..08, 10 | unit + metamorphic | `go test -count=1 ./internal/infrastructure/tfsurface/ && bash scripts/compare-ref.sh v0.3.0` | ❌ W0 (facts.go, facts_test.go) | ✅ green (local 17-00: tfsurface ok; compare-ref 244/244) |
| 15-02-03 | 02 | 2 | BLAST-07 (proof) | architecture rule + self-test | `bash scripts/check-architecture.sh && bash scripts/test-check-architecture.sh` | ✅ scripts (extended) | ✅ green (local 17-00: check-architecture OK; test-check-architecture: CI architecture job 38157144902, CI 38229787854, no architecture script changed since) |
| 15-03-01 | 03 | 2 | BLAST-06..08, 10 | unit matrix + xorshift | `go test -count=1 -run 'TestTypeChange\|TestSurfaceDiff' ./internal/domain/impact/` | ❌ W0 (typefacts_test.go) | ✅ green (local 17-00: impact -run TestTypeChange|TestSurfaceDiff ok) |
| 15-03-02 | 03 | 2 | BLAST-09 | unit + property | `go test -count=1 ./internal/domain/... ./internal/application/...` | ✅ | ✅ green (local 17-00: 4 domain + 4 application packages ok) |
| 15-04-01 | 04 | 3 | BLAST-11, SEC-01 | presenter unit | `go test -count=1 ./internal/interfaces/presenter/` | ✅ blast_test.go (extended) | ✅ green (local 17-00: presenter ok) |
| 15-04-02 | 04 | 3 | BLAST-06..10 | testscript + depth-1 contract | `go test -count=1 -run 'TestScripts/blast\|TestBlastDepth1MatchesV1\|TestBlastTransitive' ./cmd/gruntled/` | ❌ W0 (blast_types.txtar) | ✅ green (local 17-00: 31 PASS, 0 FAIL/SKIP) |
| 15-04-03 | 04 | 3 | all (docs) | doc guard | `go test -count=1 -run 'TestRuleRegistryDoc\|TestHelpMatchesDocs\|TestReadme\|TestSARIFDoc' ./cmd/gruntled/` | ✅ | ✅ green (local 17-00: 4 PASS) |
| 15-05-01 | 05 | 4 | SEC-01 | e2e escape | `go test -count=1 -run TestTextOutputsEscapeControls ./cmd/gruntled/` | ✅ escape_test.go (extended) | ✅ green (local 17-00: 10 PASS) |
| 15-05-02 | 05 | 4 | BLAST-11 | built-binary size/RSS | `go test -count=1 -run TestBlastSizeBound -timeout 600s ./cmd/gruntled/` | ❌ W0 (blast_bound_test.go) | ✅ green (CI-only bound test: non-short `go test ./...` in 38157144902 and CI 38229787854: check job linux -race, test-os macos and windows, cmd/gruntled ok) |
| 15-05-03 | 05 | 4 | all | gate | full suite + compare-ref (+ perturb) + test-txtar-extract + snapshot provenance + 60 s fuzz + build-release + go.mod/go.sum | ✅ | ✅ green (local 17-00: -short suites, vet, compare-ref 244/244, test-txtar-extract, go mod verify; CI-only: -race, fuzz, build-release, non-short suite in 38157144902 and CI 38229787854) |

*Status: ⬜ pending · ✅ green · ❌ red · ⚠️ flaky*

---

## Wave 0 Requirements

Existing infrastructure covers the phase; every ❌ W0 file is created by the first (TDD, red-first)
step of its own task.

---

## Mutation Proofs (non-vacuity)

| Mutation | Must fail | Where recorded |
|----------|-----------|----------------|
| Renderer drops `optional(...)` marker (TypeString-like) | `TestTypeSigGolden`, `TestTypeSigEqualsProperty` | 15-02-SUMMARY |
| Object attributes not sorted | `TestTypeSigEqualsProperty` (permutations), metamorphic reorder | 15-02-SUMMARY |
| Required read from default VALUE (null = required) | `TestReadSurfaceFacts` default = null rows | 15-02-SUMMARY |
| Override files not detected / last-file-wins on duplicates | `TestReadSurfaceOverride`, `TestReadSurfaceDuplicates` | 15-02-SUMMARY |
| Unknown side compared as known (output absent type = any) | `TestTypeChangeMatrix` | 15-03-SUMMARY |
| Type change feeds Broken/HasErrors | `TestTypeFactsNeverBroken`, property | 15-03-SUMMARY |
| Type string rendered per Impacted unit | `TestBlastTypeStringOncePerModule`, `TestBlastSizeBound` | 15-04/05-SUMMARY |
| escapeTerm missing on a new field | `TestBlastTextEscapesTypeChanges`, e2e escape | 15-04/05-SUMMARY |
| JSON type-string nesting cap removed | `TestTypeSigJSONDeepString` at 2,000 levels (the 400,000 case would be fatal, so mutate and run only the 2,000 case) and `TestReadSurfaceJSONDepthBomb` | 15-02-SUMMARY |
| Control-block counter removed from CheckNativeDepth | `TestCheckNativeDepthTemplateControl` (1,001 levels; the 270k loader tests would be fatal under the mutation, so run only the small case) | 15-00-SUMMARY |
| 15-00 keyword check made adjacent-only (`%{` immediately followed by if/for) | `TestCheckNativeDepthTemplateControl` four trivia forms at 1,001 levels | 15-00-SUMMARY |
| CheckNativeExprDepth replaced by config-mode CheckNativeDepth on the JSON string | `TestCheckNativeExprDepth`, `TestTypeSigJSONNewlineTernary` at 2,000 repetitions (avoid the fatal 690k case under mutation) | 15-02-SUMMARY |
| CheckNativeDepth on the decoded JSON type string skipped | `TestTypeSigJSONLowBracketBombs` (run the mutated build only on 10,000-sized inputs to avoid a fatal overflow) | 15-02-SUMMARY |
| Recover widened to the whole reader or removed | `TestReadSurfacePanickingParser` | 15-02-SUMMARY |
| Depth-1 normaliser over-deletes | per-case deletion counts in `TestBlastDepth1MatchesV1` | 15-04-SUMMARY |

---

## Manual-Only Verifications

| Behavior | Requirement | Why Manual | Test Instructions |
|----------|-------------|------------|-------------------|
| `-race` and the 30 s CI fuzz step | all, BLAST-07 | No local gcc; fuzz time budget | Confirm CI `check` job green including the FuzzTypeSig step |
| RSS bound | BLAST-11 | Absolute numbers are machine-dependent | The bound is relative to an in-run control (no calibration); 15-05-SUMMARY logs absolute Maxrss and machine info; confirm CI linux/macos pass |

---

## Validation Sign-Off

- [x] All tasks have `<automated>` verify or Wave 0 dependencies
- [x] Sampling continuity: no 3 consecutive tasks without automated verify
- [x] Wave 0 covers all MISSING references
- [x] No watch-mode flags
- [x] Feedback latency < 20s
- [x] `nyquist_compliant: true` set in frontmatter

**Approval:** validated 2026-10-12 (17-00): every row re-run locally (`-short`, one package at a time) or cited to a green CI run.

## Validation Audit (17-00, 2026-10-12, master 8f7f441)

| Item | Result |
|------|--------|
| Gaps found | 0 (every requirement BLAST-06..11, SEC-01 has an automated command) |
| Wave 0 files | all present: terragrunt and tfsurface limits_regression_test.go, hclconv/literal.go + literal_test.go, tfsurface typesig.go, typesig_test.go, facts.go, facts_test.go, impact/typefacts_test.go, blast_types.txtar, blast_bound_test.go |
| Local evidence | per-row in the map above |
| CI evidence | 38157144902 (3ed44ae, phase 15 close) and 38229787854 (d99714e): check (linux, -race, fuzz), test-os macos and windows, architecture all success |
| Manual-only rows | `-race` and fuzz: CI runs above; RSS bound: CI linux/macos/windows non-short runs above |
| Note | check_json_type_bomb.txtar from the Success Criteria table was replaced by cmd/gruntled/type_bomb_test.go (15-02 deviation 1) |

