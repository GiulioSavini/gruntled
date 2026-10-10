---
phase: 15
slug: type-level-surface-facts
status: draft
nyquist_compliant: true
wave_0_complete: false
created: 2026-10-10
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
| 5 | Hostile names escaped; 4 MiB type + 5,000-unit chain bounded; renderer never panics (fuzz in CI); six-target proof on first typeexpr import | `TestTextOutputsEscapeControls` (15-05), `TestBlastTextEscapesTypeChanges`, `TestBlastSizeBound`, `TestBlastTypeStringOncePerModule`, `TestTypeSigCapsuleAndDepth`, `FuzzTypeSig` (CI step), 15-02 Task 1 verify (check-architecture Step 9), `typeexpr-only-in-tfsurface` rule |

---

## Per-Task Verification Map

| Task ID | Plan | Wave | Requirement | Test Type | Automated Command | File Exists | Status |
|---------|------|------|-------------|-----------|-------------------|-------------|--------|
| 15-01-01 | 01 | 1 | BLAST-06..08, 10 | unit | `go test -count=1 ./internal/domain/repograph/` | ✅ surface_test.go (extended) | ⬜ pending |
| 15-01-02 | 01 | 1 | BLAST-08 | unit + byte gate | `go test -count=1 ./internal/infrastructure/... && bash scripts/compare-ref.sh v0.3.0` | ❌ W0 (hclconv/literal.go, literal_test.go) | ⬜ pending |
| 15-02-01 | 02 | 2 | BLAST-07, SEC-01 | unit + property + fuzz + six-target | `go test -count=1 -run TestTypeSig ./internal/infrastructure/tfsurface/ && go test -run '^$' -fuzz FuzzTypeSig -fuzztime 20s ./internal/infrastructure/tfsurface/ && bash scripts/check-architecture.sh` | ❌ W0 (typesig.go, typesig_test.go) | ⬜ pending |
| 15-02-02 | 02 | 2 | BLAST-06..08, 10 | unit + metamorphic | `go test -count=1 ./internal/infrastructure/tfsurface/ && bash scripts/compare-ref.sh v0.3.0` | ❌ W0 (facts.go, facts_test.go) | ⬜ pending |
| 15-02-03 | 02 | 2 | BLAST-07 (proof) | architecture rule + self-test | `bash scripts/check-architecture.sh && bash scripts/test-check-architecture.sh` | ✅ scripts (extended) | ⬜ pending |
| 15-03-01 | 03 | 2 | BLAST-06..08, 10 | unit matrix + xorshift | `go test -count=1 -run 'TestTypeChange\|TestSurfaceDiff' ./internal/domain/impact/` | ❌ W0 (typefacts_test.go) | ⬜ pending |
| 15-03-02 | 03 | 2 | BLAST-09 | unit + property | `go test -count=1 ./internal/domain/... ./internal/application/...` | ✅ | ⬜ pending |
| 15-04-01 | 04 | 3 | BLAST-11, SEC-01 | presenter unit | `go test -count=1 ./internal/interfaces/presenter/` | ✅ blast_test.go (extended) | ⬜ pending |
| 15-04-02 | 04 | 3 | BLAST-06..10 | testscript + depth-1 contract | `go test -count=1 -run 'TestScripts/blast\|TestBlastDepth1MatchesV1\|TestBlastTransitive' ./cmd/gruntled/` | ❌ W0 (blast_types.txtar) | ⬜ pending |
| 15-04-03 | 04 | 3 | all (docs) | doc guard | `go test -count=1 -run 'TestRuleRegistryDoc\|TestHelpMatchesDocs\|TestReadme\|TestSARIFDoc' ./cmd/gruntled/` | ✅ | ⬜ pending |
| 15-05-01 | 05 | 4 | SEC-01 | e2e escape | `go test -count=1 -run TestTextOutputsEscapeControls ./cmd/gruntled/` | ✅ escape_test.go (extended) | ⬜ pending |
| 15-05-02 | 05 | 4 | BLAST-11 | built-binary size/RSS | `go test -count=1 -run TestBlastSizeBound -timeout 600s ./cmd/gruntled/` | ❌ W0 (blast_bound_test.go) | ⬜ pending |
| 15-05-03 | 05 | 4 | all | gate | full suite + compare-ref (+ perturb) + test-txtar-extract + snapshot provenance + 60 s fuzz + build-release + go.mod/go.sum | ✅ | ⬜ pending |

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

---

## Manual-Only Verifications

| Behavior | Requirement | Why Manual | Test Instructions |
|----------|-------------|------------|-------------------|
| `-race` and the 30 s CI fuzz step | all, BLAST-07 | No local gcc; fuzz time budget | Confirm CI `check` job green including the FuzzTypeSig step |
| RSS bound calibration | BLAST-11 | Machine-dependent | 15-05-SUMMARY records the measured Maxrss, machine info and the chosen bound (2×); confirm CI linux/macos runs pass |

---

## Validation Sign-Off

- [x] All tasks have `<automated>` verify or Wave 0 dependencies
- [x] Sampling continuity: no 3 consecutive tasks without automated verify
- [x] Wave 0 covers all MISSING references
- [x] No watch-mode flags
- [x] Feedback latency < 20s
- [x] `nyquist_compliant: true` set in frontmatter

**Approval:** pending (draft at plan time; set to validated after execution)
