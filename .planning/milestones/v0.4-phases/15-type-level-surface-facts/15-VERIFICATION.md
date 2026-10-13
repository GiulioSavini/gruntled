---
phase: 15-type-level-surface-facts
verified: 2026-10-11T17:20:00Z
status: passed
score: 5/5 success criteria verified (7/7 requirements satisfied; report --blast halves of BLAST-11 and SEC-01 deferred to Phase 16)
covered_files: [".github/workflows/ci.yml", ".planning/phases/15-type-level-surface-facts/15-00-PLAN.md", ".planning/phases/15-type-level-surface-facts/15-00-SUMMARY.md", ".planning/phases/15-type-level-surface-facts/15-01-PLAN.md", ".planning/phases/15-type-level-surface-facts/15-01-SUMMARY.md", ".planning/phases/15-type-level-surface-facts/15-02-PLAN.md", ".planning/phases/15-type-level-surface-facts/15-02-SUMMARY.md", ".planning/phases/15-type-level-surface-facts/15-03-PLAN.md", ".planning/phases/15-type-level-surface-facts/15-03-SUMMARY.md", ".planning/phases/15-type-level-surface-facts/15-04-PLAN.md", ".planning/phases/15-type-level-surface-facts/15-04-SUMMARY.md", ".planning/phases/15-type-level-surface-facts/15-05-PLAN.md", ".planning/phases/15-type-level-surface-facts/15-05-SUMMARY.md", "README.md", "cmd/gruntled/blast_bound_test.go", "cmd/gruntled/blast_depth_test.go", "cmd/gruntled/escape_test.go", "cmd/gruntled/for_bomb_test.go", "cmd/gruntled/rules_doc_test.go", "cmd/gruntled/testdata/script/blast_impacted.txtar", "cmd/gruntled/testdata/script/blast_types.txtar", "cmd/gruntled/type_bomb_test.go", "docs/cli.md", "internal/domain/impact/surface.go", "internal/domain/impact/typefacts_test.go", "internal/domain/repograph/surface.go", "internal/domain/repograph/surface_test.go", "internal/infrastructure/hclconv/forexpr_test.go", "internal/infrastructure/hclconv/limits.go", "internal/infrastructure/hclconv/limits_test.go", "internal/infrastructure/hclconv/literal.go", "internal/infrastructure/hclconv/literal_test.go", "internal/infrastructure/terragrunt/depfacts.go", "internal/infrastructure/terragrunt/depfacts_forexpr_test.go", "internal/infrastructure/terragrunt/eval.go", "internal/infrastructure/terragrunt/eval_forexpr_test.go", "internal/infrastructure/terragrunt/fuzz_test.go", "internal/infrastructure/terragrunt/limits_regression_test.go", "internal/infrastructure/terragrunt/pathfuncs_test.go", "internal/infrastructure/terragrunt/race_off_test.go", "internal/infrastructure/terragrunt/race_on_test.go", "internal/infrastructure/tfsurface/facts.go", "internal/infrastructure/tfsurface/facts_test.go", "internal/infrastructure/tfsurface/forexpr_test.go", "internal/infrastructure/tfsurface/limits_regression_test.go", "internal/infrastructure/tfsurface/race_export_test.go", "internal/infrastructure/tfsurface/race_off_test.go", "internal/infrastructure/tfsurface/race_on_test.go", "internal/infrastructure/tfsurface/reader.go", "internal/infrastructure/tfsurface/typesig.go", "internal/infrastructure/tfsurface/typesig_test.go", "internal/interfaces/presenter/blast.go", "internal/interfaces/presenter/blast_test.go", "internal/interfaces/presenter/blast_types_test.go", "scripts/check-architecture.sh", "scripts/test-check-architecture.sh"]
covered_digest: "v3:sha256:80710f417340d9236c189748a999da09536d19093133ab6a7c09c815ce13b83f"
behavior_unverified: 0
overrides_applied: 0
deferred:
  - truth: "BLAST-11: `report --blast` memory and output size are O(units + edges + surface text)"
    addressed_in: "Phase 16"
    evidence: "Phase 16 SC1: report --blast bytes equal `blast --base <copy of the tree at baseline time>` (property test); SC5: a blast response over 64 MiB is an explicit exit-3 error, and daemon memory on synthrepo at 65, 500 and 5,000 units is measured and recorded. `report --blast` does not exist yet (Phase 16 DAEMON-07)."
  - truth: "SEC-01: the phase-12 e2e escape test is extended to `report --blast`"
    addressed_in: "Phase 16"
    evidence: "Phase 16 SC1: '... including for a module with hostile names (the daemon-path half of the SEC-01 escape test)'"
human_verification: []
---

# Phase 15: Type-Level Surface Facts Verification Report

**Phase Goal:** User sees a new required variable, a changed variable or output type, or a flipped `sensitive` put the units that instantiate the module in Impacted with a named reason, without it ever becoming a diagnostic or a Broken unit.
**Verified:** 2026-10-11T17:20:00Z, code at master 71620f3 (last code commit 6dc6537; 71620f3 and 3ed44ae are docs-only)
**Status:** passed
**Re-verification:** No, initial verification

Evidence classes in this report:
- **[local]**: run by the verifier in this session (WSL, GOMAXPROCS=2, per package, hostile tests skipped).
- **[CI]**: GitHub Actions run 38157144902 on 3ed44ae (same code as 71620f3). All six jobs are green: check (`go test -race -count=1 ./...`, staticcheck, govulncheck, `fuzz FuzzTypeSig`, release-build), architecture, test-os on macOS and Windows, sarif-upload and recipe-check.
- **[code]**: read in the source.

SUMMARY claims were not used as evidence unless re-checked.

## Goal Achievement

### Observable Truths (ROADMAP success criteria)

| # | Truth | Status | Evidence |
|---|-------|--------|----------|
| 1 | A variable that is new without a `default`, or that loses its `default`, puts its module's instantiating units in Impacted with a "now required" reason. `default = null` counts as a default. A change to `nullable`, `validation`, `ephemeral` or `deprecated` alone changes nothing. | ✓ VERIFIED | **[code]** `facts.go` `factSchema` reads only `type`/`default`/`sensitive`, and Required is the presence of `default` (`blockFacts`). `impact/surface.go` `typeChanges` emits `absent->required` for a new variable whose Required is True, and `optional->required` for a variable that lost its default. **[local]** `blast_types.txtar` cases 1 (`+variable x, required variable x` / `modules/m variable x: now required`), 2 (default removed), 3 (`default = null`: `Impacted (0)`) and 15 (nullable/validation/ephemeral/deprecated/const/description: `Impacted (0)`) pass. TestReadSurfaceFacts (`default = null` and JSON `"default": null` give Required False) and TestTypeChangeMatrix pass. **[local] ad-hoc binary run**: `nullable = true -> false` alone gave no reason. |
| 2 | A changed variable `type`, a changed declared output `type` (only when both sides declare a parseable type) or a flipped output `sensitive` puts the instantiating units in Impacted with a "type changed" reason. Formatting, comments, attribute order, `optional()` defaults and `.tf`→`.tf.json` conversion give an empty Impacted set. | ✓ VERIFIED | **[code]** `typesig.go` renders from the `cty.Type`: object attribute names are sorted and `strconv.Quote`d, and defaults are dropped (`TypeConstraintWithDefaults`, defaults discarded). `typeChanges` compares an output type only when it is non-empty on both sides; sensitive goes through `knownBool`. **[local]** `blast_types.txtar` cases 4, 5 (`any -> string`), 6, 8 and 16 (JSON) report the reason. Cases 7 (output type on one side only), 9 (non-literal sensitive), 10 (reformat, comments, reorder, optional default) and 11 (`.tf`→`.tf.json`) give `Impacted (0)`. TestTypeSigEqualsProperty (2,000 generated pairs: same signature exactly when the types are `Equals`) and TestReadSurfaceMetamorphic (7 transforms) pass. **[local] ad-hoc binary run**: `list(string)->set(string)` and a `.tf.json` output `sensitive false->true` both reported, exit 0. |
| 3 | Type-level reasons never create a diagnostic, never make a unit Broken and never change the exit code. With `--depth 1`, every v0.3 blast golden gives the same Broken/Impacted sets and exit code (text differs only by distance/path tokens and the D-15-05 type tokens/section; JSON only by version 2 and additive keys). | ✓ VERIFIED (behavioral) | **[code]** TypeChanges exist only in `SurfaceChange`, and Broken comes from diagnostics alone (`impact.go` Compute is unchanged and seeds from `SurfaceDiff`). **[local]** TestTypeFactsNeverChangeBrokenProperty passes: 2,000 cases compare facts-on vs names-only Compute. Broken and HasErrors are identical, Impacted is a superset, and distances are ≤. Counters: facts added Impacted units in 1,156 cases and shortened a distance in 183. TestTypeFactsNeverBroken passes. `blast_types.txtar` case 14 (the unit does not set the new required variable) and every other case run `exec gruntled-exit 0` with `Broken (0)`. TestBlastDepth1MatchesV1 passes all 22 snapshot cases: provenance sha256 is checked, sets and exit codes compared exactly, and the normaliser deletes exactly {2 tokens, 1 section, 1 line} for `blast_impacted__base__cur` and 0 elsewhere. **[local]** `compare-ref.sh v0.3.0` prints `244/244 identical` (check/graph/SARIF unchanged). |
| 4 | An ambiguous fact reports nothing: a name declared in more than one kept file with differing facts, any Terraform override file, a non-literal or unparseable value, or an unknown fact on either side stays silent, while names stay known, so `GRT001` and name-level Impacted are unchanged. | ✓ VERIFIED | **[code]** `facts.go` `merge` makes all facts unknown for a declaration in any override file, and keeps a fact only when every declaration agrees. `isOverrideFile` covers `override`/`*_override` × `.tf/.tf.json/.tofu/.tofu.json`. `typeChanges` skips `""`/TristateUnknown on either side. The names come from the same labelled blocks (`reader.go` → `NewSurfaceFacts`). **[local]** TestReadSurfaceOverride (5 override spellings, plus 2 names that are not override files), TestReadSurfaceDuplicates, TestIsOverrideFile, TestSurfaceViewsUnchanged, TestSurfaceDiffNamesOnlyUnchanged (2,000 pairs) and `blast_types.txtar` cases 12 (override.tf) and 13 (a.tf vs a.tofu) pass. compare-ref 244/244. |
| 5 | Hostile or huge input is safe and bounded: names with C0, C1, U+202E and invalid UTF-8 print escaped in blast text and JSON. A 4 MiB type and a 5,000-unit chain stay within the asserted RSS and output bounds. The renderer never panics (fuzz in CI). The six-target no-net/no-exec proof passes on the first commit that imports `ext/typeexpr`. | ✓ VERIFIED | **Escaping [local]:** TestTextOutputsEscapeControls/blast and /blast_json pass. They cover crafted variable/output names (ESC, U+009B, U+202E) in the type tokens, the Type changes section and `type_changes`, a raw 0xff `.tf.json` key (U+FFFD), and a raw 0xff `.tf` (an escaped GRT100, no type change). An object attribute name with U+202E cannot be a type: typeexpr refuses quoted keys (TestTypeSigGolden row `object({"\u202e" = string})` → unknown), and the e2e case asserts nothing is printed for it. **[code]** Every dynamic string the blast presenter writes goes through `escapeTerm`; the only other dynamic writes are integers and constant tokens. JSON goes through `escapeJSON`. **Bounds [CI]:** TestBlastSizeBound is not `-short`-gated in CI and runs in the linux `-race` job (`ok cmd/gruntled 164.128s`) and on macOS (`ok cmd/gruntled 55.120s`). **[code]** It asserts 5,000 distinct distances, text < rendered + 5,000×200 + 1 KiB, JSON < 2×rendered + 5,000×400 + 1 KiB, the type string printed once, and RSS ≤ same-size control + 64×typeBytes. **Never panics [code+CI]:** `writeType` is a kind switch with a depth cap of 100 and a default branch that returns false. TestTypeSigCapsuleAndDepth passes [local], and `fuzz FuzzTypeSig` passes in CI (see Warning W1). **Six targets [local]:** on cd0958c (the first commit that links typeexpr; 80a055e only imported it in a red test), `go list -deps` for linux/darwin/windows × amd64/arm64 shows forbidden=0 and typeexpr=1, and `check-architecture.sh` prints OK. The same holds at HEAD. go.mod/go.sum are unchanged since v0.3.0. typeexpr is imported only in `internal/infrastructure/tfsurface`. |

**Score:** 5/5 truths verified (0 present, behavior-unverified)

### Deferred Items

| # | Item | Addressed In | Evidence |
|---|------|-------------|----------|
| 1 | BLAST-11 for `report --blast` (memory/output O(units+edges+surface text)) | Phase 16 | SC1 byte equality with `blast --base`, SC5 64 MiB cap and daemon memory measurement; `report --blast` itself is DAEMON-07 |
| 2 | SEC-01 escape test on `report --blast` | Phase 16 | SC1 "the daemon-path half of the SEC-01 escape test" |

### Required Artifacts

| Artifact | Expected | Status | Details |
|----------|----------|--------|---------|
| `internal/infrastructure/hclconv/limits.go` | CheckNativeDepth counts `%{if}/%{for}`; CheckNativeExprDepth | ✓ VERIFIED | `ctl` counter + `controlDepth`; expression-mode wrapper; reached from `tfsurface/reader.go` and `typesig.go` |
| `internal/domain/repograph/surface.go` | VariableDecl, OutputDecl, NewSurfaceFacts, Variable/Output | ✓ VERIFIED | used by `tfsurface/reader.go` (`NewSurfaceFacts`) and `impact/surface.go` (`.Variable(`/`.Output(`) |
| `internal/infrastructure/hclconv/literal.go` | LiteralBool (+ HasForExpr guard) | ✓ VERIFIED | called from `terragrunt/depfacts.go` (enabled, skip_outputs, mock merge) and `tfsurface/facts.go` (sensitive) |
| `internal/infrastructure/tfsurface/typesig.go` | bounded canonical renderer + JSON guards | ✓ VERIFIED | 64 KiB cap, bracket scan, CheckNativeExprDepth and `for` refusal before typeexpr; narrow recover in `safeTypeAttrSig` |
| `internal/infrastructure/tfsurface/facts.go` | per-block facts, override/duplicate merge | ✓ VERIFIED | `facts.add` is called in the ReadSurface loop only after the size cap, depth pre-scan and clean parse |
| `internal/domain/impact/surface.go` | TypeChange, TypeChangeKind, SurfaceDiff extension | ✓ VERIFIED | `Empty()` counts TypeChanges, so type-only modules seed Compute |
| `internal/interfaces/presenter/blast.go` | tokens, Type changes section, `type_changes` JSON | ✓ VERIFIED | `writeChangeTokens`, `writeTypeChanges`, `blastTypeChanges` |
| `cmd/gruntled/testdata/script/blast_types.txtar` | BLAST-06..10 end to end | ✓ VERIFIED | 17 cases, run locally |
| `cmd/gruntled/blast_bound_test.go` | TestBlastSizeBound | ✓ VERIFIED | runs in CI linux (-race job, binary built without -race) and macOS |
| `cmd/gruntled/testdata/script/check_json_type_bomb.txtar` (15-02 plan) | cmd-level JSON bomb smoke | ✓ VERIFIED (substituted) | Replaced by `cmd/gruntled/type_bomb_test.go` TestCheckJSONTypeBomb: a 2.4 MB fixture cannot be committed or generated in testscript. It runs in CI. |

### Key Link Verification

| From | To | Via | Status | Details |
|------|----|-----|--------|---------|
| `tfsurface/reader.go` ReadSurface | `repograph.NewSurfaceFacts` | merged decls | WIRED | `vars, outs := facts.decls(); repograph.NewSurfaceFacts(vars, outs)` |
| `impact.SurfaceDiff` | `Surface.Variable/Output` | per-name facts, unknown = silent | WIRED | `typeChanges(bs, cs)` |
| `impact.Compute` | type-only modules | `SurfaceChange.Empty()` includes TypeChanges | WIRED | proven by TestComputeTypeOnlySeeds and txtar case 17 (`live/b (distance 2, from module modules/m ...)`) |
| `BlastText` Type changes section | `escapeTerm` | module, name, old, new | WIRED | `escapeTerm(c.Module.String())`, `escapeTerm(tc.Name/Old/New)` |
| `terragrunt/depfacts.go` | `hclconv.LiteralBool` | enabled/skip_outputs/mock merge | WIRED | three call sites; no `literalBool` remains |
| `ci.yml` | `FuzzTypeSig` | `fuzz FuzzTypeSig` step, timeout-minutes 5 | WIRED | step ran green in run 38157144902 |

### Data-Flow Trace (Level 4)

| Artifact | Data Variable | Source | Produces Real Data | Status |
|----------|---------------|--------|--------------------|--------|
| blast text/JSON `type_changes` | `res.Changes[].TypeChanges` | `SurfaceDiff` ← `Surface.Variable/Output` ← `tfsurface` facts parsed from real HCL | Yes (ad-hoc binary run printed `list(string) -> set(string)` from real files) | ✓ FLOWING |

### Behavioral Spot-Checks

| Behavior | Command | Result | Status |
|----------|---------|--------|--------|
| Type-level reasons end to end, exit 0, Broken 0 | `go test -short -run 'TestScripts/blast' ./cmd/gruntled/` | blast_types, blast_impacted, blast_grt004, blast_transitive, blast_exitcodes and blast_nobase PASS | ✓ PASS |
| `--depth 1` contract vs v0.3 snapshots | `go test -short -run TestBlastDepth1MatchesV1 ./cmd/gruntled/` | 22/22 cases PASS | ✓ PASS |
| SEC-01 e2e | `go test -short -run TestTextOutputsEscapeControls ./cmd/gruntled/` | blast and blast_json subtests PASS | ✓ PASS |
| Never Broken (property) | `go test -run TestTypeFactsNeverChangeBrokenProperty -v ./internal/domain/impact/` | PASS, counters non-zero | ✓ PASS |
| Byte identity vs v0.3.0 | `bash scripts/compare-ref.sh v0.3.0` | `244/244 identical`; the repo is back on master, worktree list clean | ✓ PASS |
| Ad-hoc tree on the built binary | `gruntled blast --base base cur` (list→set, `.tf.json` sensitive flip, nullable-only change) | the 2 reasons printed, nullable silent, exit 0; JSON `type_changes` correct | ✓ PASS |
| Remaining suites | per package: repograph, impact, presenter, blasting, hclconv, tfsurface, terragrunt, application, analysis, diagnostic, watch, ipc, statusfile, sourceresolve; `cmd/gruntled -short -skip 'Bomb\|TestBlastSizeBound'`; `go vet ./...`; `gofmt -l` | all ok, VET-OK, GOFMT-OK | ✓ PASS |
| Hostile 4 MB / 5,000-unit / fuzz / -race | not run locally (OOM rule) | CI run 38157144902 (3ed44ae): `go test -race -count=1 ./...` all packages ok (tfsurface 73.7 s, hclconv 62.7 s, terragrunt 45.0 s, cmd 164.1 s); macOS `go test ./...` ok; `fuzz FuzzTypeSig` PASS | ✓ PASS [CI] |

### Probe Execution

Not applicable: no `scripts/*/tests/probe-*.sh` exists, and no plan declares a probe.

### Requirements Coverage

| Requirement | Source Plan | Description (short) | Status | Evidence |
|-------------|-------------|---------------------|--------|----------|
| BLAST-06 | 15-01..04 | new/lost-default variable → "now required"; `default = null` is a default; nullable/validation/ephemeral/deprecated never count | ✓ SATISFIED | SC1 evidence |
| BLAST-07 | 15-00..05 | variable type change → reason; canonical structural compare; absent = any; facts only from pre-scanned files; renderer depth-capped, never panics, fuzz in CI | ✓ SATISFIED | SC2 and SC5 evidence; facts are read only after the size/depth/parse gates (`reader.go`); depth cap 100; FuzzTypeSig CI step |
| BLAST-08 | 15-01..04 | output type change (both declared, never inferred) or sensitive flip (absent = false, non-literal unknown) → reason | ✓ SATISFIED | SC2 evidence; txtar cases 6–9; TestReadSurfaceFacts sensitive rows |
| BLAST-09 | 15-03, 15-04 | never diagnostic/Broken/exit; depth-1 v0.3 equality with D-15-05 | ✓ SATISFIED | SC3 evidence |
| BLAST-10 | 15-01..03 | per-name facts unknown on duplicates with differing facts, override files, non-literal/unparseable; names unchanged | ✓ SATISFIED | SC4 evidence |
| BLAST-11 | 15-04, 15-05 | blast memory/output linear; type change rendered once per module; linear path representation; 5,000-unit + 4 MiB test | ✓ SATISFIED (blast); `report --blast` half deferred to Phase 16 | TestBlastTypeStringOncePerModule [local]; TestBlastSizeBound [CI] |
| SEC-01 | 15-02, 15-04, 15-05 | new text through escapeTerm, JSON escapeJSON, renderer strconv.Quote, e2e escape for blast | ✓ SATISFIED (blast); `report --blast` half deferred to Phase 16 | SC5 escaping evidence; `strconv.Quote(n)` in `writeType` |

No orphaned requirements: REQUIREMENTS.md maps exactly BLAST-06..11 and SEC-01 to Phase 15, and the plans claim all seven. REQUIREMENTS.md still shows them as `[ ]`/Pending, and the ROADMAP Phase 15 plan list is still unchecked. Updating both is bookkeeping for the orchestrator.

### Test Quality Audit

| Test File | Linked Req | Skipped | Circular | Assertion Level | Verdict |
|-----------|-----------|---------|----------|-----------------|---------|
| `impact/typefacts_test.go` | BLAST-06..10 | 0 | no (property against an independent names-only Compute) | Behavioral | OK |
| `tfsurface/facts_test.go`, `typesig_test.go` | BLAST-06..08, 10 | 0 | no (hand-written expectations; `Equals` oracle from cty) | Value | OK |
| `blast_types.txtar` | BLAST-06..10 | 0 | generated once by a scratch script, but each line is an exact regexp assertion reviewed against the HCL in the archive (cases 10–13 checked by hand) | Value | OK |
| `blast_depth_test.go` | BLAST-09 | 0 | no: snapshots from pre-phase-14 commit 67c9ace with sha256 PROVENANCE | Value | OK |
| `escape_test.go` | SEC-01 | Windows only (NTFS) | no | Value | OK |
| `blast_bound_test.go` | BLAST-11 | `-short` and Windows only; runs in CI linux and macOS | no | Value (bounds) | OK |
| `*forexpr_test.go` | sec F1/F2/F5 | the 5×100 shape is skipped only when the 2×10 subtest already failed | no | Behavioral | OK |

Disabled tests on requirements: 0. Circular patterns: 0. Insufficient assertions: 0. The JSON check `!tc("variable_type", "w", "", "")` in `escape_test.go` is weak, but the text assertion `!strings.Contains(out, "variable w")` covers that case.

### Anti-Patterns Found

| File | Line | Pattern | Severity | Impact |
|------|------|---------|----------|--------|
| (44 phase-modified files) | — | TBD/FIXME/XXX/TODO/HACK/placeholder | none found | — |
| `.github/workflows/ci.yml` fuzz step (W1) | 56 | The fuzzer makes no progress after 3 s in both inspected CI runs: run 38157144902 stops at `execs: 4523 (0/sec)` from 6 s to 31 s, and run 38150510889 (2e6c6f8) at `execs: 8529 (0/sec)`. | ⚠️ Warning | The step is green, but it fuzzes for about 3 s, not 30 s. This fits Go's coordinator giving the 4 CI workers minimization of new-coverage inputs derived from the large seeds (`deepList(5_000)`, `nestedIfDefault(1_100)`, `"!"×5,000`); minimization execs are reported only when it finishes. A slow input cannot be excluded from the log, because Go discards an input still running when fuzztime ends. Not run locally (no `-fuzz` under the OOM rule). Suggested follow-up: `-fuzzminimizetime=5s` or smaller seeds, and a log line that shows execs keep growing. Non-blocking: the renderer's panic freedom also rests on code structure, TestTypeSigCapsuleAndDepth and the 2,000-case equality property. |
| `cmd/gruntled/blast_bound_test.go` | 175 | The RSS bound is control + 64×typeBytes (about +256 MiB). | ℹ️ Info | It catches per-unit copies of the type string (about 20 GB) but nothing smaller; already recorded as sec F4 and accepted. The plan's "2× measured" wording was replaced by the relative control (bus #308/#311). |

### Human Verification Required

N/A. This is a CLI/infrastructure phase whose behaviour is fully covered by automated e2e, property and CI tests. No ⚠️ PRESENT_BEHAVIOR_UNVERIFIED or insufficient-spec truths.

### Decision Coverage

Not evaluated: there is no CONTEXT.md in the phase directory.

### Gaps Summary

No gaps. Every ROADMAP success criterion holds in the code at 71620f3, backed by tests the verifier ran and by CI run 38157144902 for the hostile-input, `-race`, 5,000-unit and fuzz paths it may not run locally. The `report --blast` halves of BLAST-11 and SEC-01 belong to Phase 16, whose success criteria name them explicitly. One non-blocking warning (W1): the CI fuzz step does almost no work after its first 3 s.

---

_Verified: 2026-10-11T17:20:00Z_
_Verifier: Claude (gsd-verifier)_
