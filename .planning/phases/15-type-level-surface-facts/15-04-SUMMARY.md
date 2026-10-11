---
phase: 15-type-level-surface-facts
plan: 04
subsystem: presenter, cli, docs
tags: [blast, type-facts, json-schema, golden, docs]
requires: [15-02, 15-03]
provides:
  - "blast JSON changes[].type_changes [{kind, name, old, new}] (version 2, additive)"
  - "blast text reason tokens on distance-1 lines and a Type changes section printed only when non-empty"
  - "blast_types.txtar: 17 end-to-end cases on real HCL"
  - "TestBlastDepth1MatchesV1 normaliser with per-case deletion counts (D-15-05)"
  - "docs/cli.md blast sections and Known limitations, pinned"
affects: [15-05]
tech-stack:
  added: []
  patterns: ["type strings once per module (changes[] / Type changes), names-only tokens per unit", "normaliser deletions counted per case"]
key-files:
  created:
    - internal/interfaces/presenter/blast_types_test.go
    - cmd/gruntled/testdata/script/blast_types.txtar
  modified:
    - internal/interfaces/presenter/blast.go
    - internal/interfaces/presenter/blast_test.go
    - cmd/gruntled/testdata/script/blast_impacted.txtar
    - cmd/gruntled/blast_depth_test.go
    - cmd/gruntled/rules_doc_test.go
    - docs/cli.md
    - README.md
key-decisions:
  - "Type changes section order: Changes order (by module), then the domain's kind/name order, so variable_required precedes variable_type"
  - "variable_required prints 'now required' for both absent -> required and optional -> required; JSON keeps old absent/optional"
  - "The depth-1 normaliser touches text only: JSON already drops the whole changes key, where type_changes lives"
requirements-completed: []
duration: 45min
completed: 2026-10-11
---

# Phase 15 Plan 04: Type-Level Reasons in Blast Output Summary

**`blast` now prints why a unit is Impacted at the type level. Distance-1 lines gain the short tokens `required variable n`, `type variable n`, `type output n` and `sensitive output n`, with no type strings. A `Type changes (N):` section, printed only when N > 0, lists every change once per module with its old and new value. JSON adds `changes[].type_changes`, still version 2, and `impacted[]` is unchanged. 17 testscript cases prove the behaviour on real HCL: every case exits 0 with nothing Broken, and the silent cases stay silent. Among existing goldens only blast_impacted changed, as planned. `compare-ref.sh v0.3.0` is 244/244.**

## Performance

- Duration: ~45 min
- Tasks: 3
- Files: 2 created, 7 modified

## Accomplishments

### Task 1: presenter
- **blast.go:**
  - The JSON types are `blastChange.TypeChanges` (:79, `json:"type_changes"`) and `blastTypeChange` (:85). `blastTypeChanges` (:402) converts them in domain order and gives [] when there are none (:384). The output still goes through `escapeJSON` (:397).
  - **Text tokens:** `writeChangeTokens` appends the type tokens after the name groups (:269, `escapeTerm(tc.Name)`). The token strings are in `typeToken` (:275).
  - **Text section:** `writeTypeChanges` (:285) is called after Impacted, only with a baseline (:197), and writes nothing when there are no type changes. The module, name, old and new strings all go through `escapeTerm` (:299/:306/:316/:318).
  - The BlastText and BlastJSON doc comments describe the tokens, the section, the Impacted-only rule and `type_changes`.
- **blast_test.go:** the one intended presenter-golden change: TestBlastJSONGolden's changes entry gains `"type_changes": []`.
- **blast_types_test.go:**
  - TestBlastTextTypeChanges (:50) covers the full text with four kinds. With no type changes there is no section; a type-only change shows only the reason token; with no baseline there is no section.
  - TestBlastJSONTypeChanges (:97) checks the kinds, names and old/new values in domain order, the literal `"type_changes": []`, and that `impacted[]` is identical to the same Result without TypeChanges.
  - TestBlastTypeStringOncePerModule (:147) uses a 1 MiB object type and 1,000 Impacted units. The old and new strings each appear exactly once in text and once in JSON, and the size bound holds.
  - TestBlastTextEscapesTypeChanges (:181) puts crafted ESC/C1/U+202E text in the module, names and type strings. The text is terminal-safe and matches the escaped golden exactly, and the JSON fields round-trip through `escapeJSON`.

### Task 2: end to end and the depth-1 contract
- **blast_types.txtar** (generated once by a scratch script, committed as text): 17 cases, each `exec gruntled-exit 0 blast --base base<k> cur<k>` with `Broken (0)` and `! stderr .`.
  - **Reported cases**, each with the exact Impacted line and Type changes lines:
    - (1) a new `variable "x" {}`;
    - (2) a removed default;
    - (4) `string` -> `list(string)`;
    - (5) an absent type -> `string`, shown as `any -> string`;
    - (6) an output type changed on both sides;
    - (8) `sensitive` absent -> `true`;
    - (14) a unit that does not set the new required variable: Impacted, not Broken, exit 0;
    - (17) a type-only change reaching dependent `live/b` at `distance 2, from module modules/m, path live/b -> live/a`.
  - **Silent cases**, each `Impacted (0)` with no section:
    - (3) `default = null`;
    - (7) an output type declared on one side only;
    - (9) `sensitive = var.s`;
    - (10) reformat, comments, attribute reorder and an optional() default change;
    - (11) a variable moved to an equivalent vars.tf.json;
    - (12) an override.tf;
    - (13) a.tf and a.tofu that conflict;
    - (15) description, nullable, ephemeral, const, deprecated and validation added.
  - **JSON:** (16) is `--format json` of case 4 and asserts the `type_changes` entry and version 2.
  - No `$WORK` in any regexp; the path-leak check is `! stdout '^/'`.
  - A deliberately wrong assertion made the script fail, confirmed and reverted.
- **blast_impacted.txtar:** the intended golden change. The live/cache and live/db lines gain `, required variable name`, and the text now asserts `Type changes (1):` and `  modules/vpc variable name: now required`. The JSON asserts the `variable_required` entry (`absent` -> `required`). Nothing else changed.
- **blast_depth_test.go** (sec #248):
  - `typeTokenRe` (:131) matches the four exact token forms after `, `. `typeSectionRe` (:135) matches the section header and its indented lines.
  - `stripTypeLevel` (:148) runs on HEAD text only and returns its deletion counts. They must equal `typeLevelDeletions` (:142): {2 tokens, 1 section, 1 section line} for `blast_impacted__base__cur`, 0 for every other case (:256).
  - The snapshot text must contain none of the four forms or `Type changes` (:230), and every name in `typeLevelDeletions` must have a snapshot.
  - Changing the expectation to {3,1,1} fails with `removed [2 1 1] ... want [3 1 1]`, confirmed and reverted.
  - PROVENANCE and the snapshots are unchanged.

### Task 3: docs
- **docs/cli.md:**
  - **Blast intro:** a type-only module change puts its units at distance 1, and the pinned may-be-affected sentence follows for distance > 1.
  - **Blast text:** the token list, the updated example (the real blast_impacted output), the `Type changes` section with its four line forms and "printed only when at least one changed module has a type-level change", and the Impacted-only sentence.
  - **Blast JSON:** `changes[]` lists `type_changes`, and a new `changes[].type_changes[]` row gives the four kinds and the old/new value forms. The Impacted-only sentence appears here too.
  - **Known limitations:** the "invisible" bullet is replaced by the precise rules.
- **rules_doc_test.go:** the new pins are `blastTypeOnly`, `blastMayBeAffected`, `blastTypeTokenLine`, `blastTypeSection`, `blastTypeSectionWhen`, `blastTypeLimits`, the four token forms and the JSON kind and value names. A check also asserts that `blastOldInvisible` is gone. 18 assertions were red first (be290c4). All existing pins are unchanged.
- **README.md:** the blast paragraph names the type-level reasons and says they are never Broken (deviation 2).

## Task Commits

1. `d08445e` test(15-04): blast type_changes in JSON, reason tokens and Type changes section in text
2. `0f663b8` feat(15-04): type-level reasons in blast text and type_changes in blast JSON
3. `dc66b8d` test(15-04): blast_impacted gains the required-variable reason; depth-1 normaliser counts type-level removals
4. `803edf4` test(15-04): blast_types.txtar, type-level reasons end to end on real HCL
5. `be290c4` test(15-04): pin the type-level reasons in docs/cli.md
6. `669a260` docs(15-04): blast type-level reasons, Type changes section and type_changes
7. `7856604` docs(15-04): README blast paragraph names the type-level reasons

This summary is its own `docs(15-04): summary` commit.

## Verification

```
ok  	github.com/GiulioSavini/gruntled/cmd/gruntled	8.838s
ok  	github.com/GiulioSavini/gruntled/internal/application/blasting	0.523s
ok  	github.com/GiulioSavini/gruntled/internal/application/checking	0.035s
ok  	github.com/GiulioSavini/gruntled/internal/application/indexing	0.003s
?   	github.com/GiulioSavini/gruntled/internal/application/ports	[no test files]
ok  	github.com/GiulioSavini/gruntled/internal/application/watching	0.032s
ok  	github.com/GiulioSavini/gruntled/internal/domain/analysis	0.186s
ok  	github.com/GiulioSavini/gruntled/internal/domain/diagnostic	0.032s
ok  	github.com/GiulioSavini/gruntled/internal/domain/impact	1.118s
ok  	github.com/GiulioSavini/gruntled/internal/domain/repograph	0.032s
ok  	github.com/GiulioSavini/gruntled/internal/infrastructure/hclconv	4.080s
ok  	github.com/GiulioSavini/gruntled/internal/infrastructure/ipc	0.048s
ok  	github.com/GiulioSavini/gruntled/internal/infrastructure/sourceresolve	0.032s
ok  	github.com/GiulioSavini/gruntled/internal/infrastructure/statusfile	0.085s
ok  	github.com/GiulioSavini/gruntled/internal/infrastructure/terragrunt	3.379s
ok  	github.com/GiulioSavini/gruntled/internal/infrastructure/tfsurface	4.548s
ok  	github.com/GiulioSavini/gruntled/internal/infrastructure/watch	1.169s
ok  	github.com/GiulioSavini/gruntled/internal/interfaces/presenter	0.081s
ok  	github.com/GiulioSavini/gruntled/internal/testsupport/synthrepo	0.044s
ok  	github.com/GiulioSavini/gruntled/scripts/archscan	0.022s
VET-OK
architecture: OK (4 domain packages, 5 application packages, 1 interfaces packages)
```

```
bash scripts/compare-ref.sh v0.3.0
244/244 identical
```

Golden diff against fc51b01 (the whole of cmd/gruntled/testdata; blast_v1 snapshots and PROVENANCE untouched):

```
git diff --stat fc51b01 -- cmd/gruntled/testdata
 cmd/gruntled/testdata/script/blast_impacted.txtar |   9 +-
 cmd/gruntled/testdata/script/blast_types.txtar    | 453 ++++++++++++++++++++++
```

`check`, `graph` and the status file are untouched: compare-ref covers check and graph against v0.3.0. `-race` was not run locally (no gcc); CI runs it. `test-check-architecture.sh` was not re-run because no architecture script changed.

## Threat Mitigations

| ID | Disposition | Where |
|----|-------------|-------|
| T-15-04-1 | mitigated | escapeTerm on every new text field: blast.go:269 (token name), :299 (module), :306 (name), :316/:318 (old/new); JSON through escapeJSON :397; TestBlastTextEscapesTypeChanges blast_types_test.go:181 (exact escaped golden plus assertTerminalSafe plus JSON round-trip). The e2e escape case is in 15-05 |
| T-15-04-2 | mitigated | type strings only in changes[].type_changes (blast.go:384) and the Type changes section (:285); impacted[] and the per-unit tokens carry names only; TestBlastTypeStringOncePerModule :147 (1 MiB type, 1,000 units, exactly one occurrence each). The 5,000-unit bound is in 15-05 |
| T-15-04-3 | mitigated | the normaliser deletes only the four exact token forms and the section (blast_depth_test.go:131/:135), from HEAD text only, with per-case counts asserted (:142/:256) and snapshot absence asserted (:230); JSON comparison unchanged; sets and exit codes compared exactly |
| T-15-04-4 | mitigated | blast_types.txtar silent cases 3, 7, 9, 10, 11, 12, 13, 15 assert `Impacted (0)` and no section |
| T-15-04-5 | mitigated | every blast_types case runs `gruntled-exit 0` with `Broken (0)`; case 14 is the unset required variable |

## Deviations from Plan

**1. [Section order] Type changes lines follow the domain kind order**
- The plan's example section lists `variable cidr: type ...` before `variable name: now required`, but its action says to write the section "in Changes order", with each module's changes in the domain order from 15-03 (variable_required first).
- The domain order was kept for both the tokens and the section, so they agree; TestBlastTextTypeChanges pins it.

**2. [Extra file] README.md blast paragraph updated (7856604)**
- The plan lists docs/cli.md only. README said Impacted is about "variable or output names" alone, which became false with this plan. One paragraph now names the type-level reasons; the README pins (distance, path, --depth) still pass.

**3. [Generated fixture] blast_types.txtar was produced by a scratch generator**
- It was generated once, from the session scratchpad, and committed as plain text, which is the reviewable artifact. The generator is not in the repo.

**4. [Case numbering] Case (16), the JSON case, sits right after case 4 in the script**
- It reuses case 4's trees. The numbering follows the plan's list.

## Known Risks / Unverified

- `-race` was not run locally (no gcc).
- Hostile names through the real reader end to end, and the 5,000-unit size/RSS bound, are 15-05's (SEC-01, BLAST-11).

## Self-Check: PASSED
