---
phase: 17-corpus-mutation-proof-cross-phase-audit
plan: 02
subsystem: cmd/gruntled (tests)
tags: [corpus, rel-03, oracle, exclusions, harness, reformat]
requires: [17-01]
provides:
  - "buildCorpusModel: an independent textual model (units, seeds, live edges, reference sites, module surfaces, BFS)"
  - "TestCorpusBlastModel: model vs graph --json with a pinned per-item exclusion table (110 rows) checked by predicates"
  - "pinned included-module counts and model facts per corpus"
  - "module editor (findBlocks, ops, applyEdit confinement, digestGuard)"
  - "runMutation / expectBlast / compareBlast / assertSurfaceUnchanged, proven by the always-on TestCorpusHarnessSelfTest"
  - "TestCorpusReformat R1-R9 on the three corpora"
affects: [17-03, 17-04, 17-05]
tech-stack:
  added: []
  patterns: ["exclusion = (path, category predicate, direction, evidence), re-evaluated by the test", "touch set removes cones that meet an excluded unit", "fault hook on the real driver"]
key-files:
  created:
    - cmd/gruntled/corpus_model_test.go
    - cmd/gruntled/corpus_edit_test.go
    - cmd/gruntled/corpus_harness_test.go
    - cmd/gruntled/corpus_reformat_test.go
  modified: []
key-decisions:
  - "The model scans hidden directories (skipping only .git, .terragrunt-cache, .terraform). gruntled and Terragrunt discover units there, and a model that skipped them could not know their edges, so the touch rule could not remove the affected cones"
  - "Terragrunt path functions are evaluated by their documented meaning in source and config_path: get_terragrunt_dir, get_parent_terragrunt_dir (with and without an include label), path_relative_to_include, path_relative_from_include, find_in_parent_folders. A config_path that names the dependency's terragrunt.hcl maps to its directory"
  - "Eligibility drops only a generate that may change the compared surface: contents that may declare an output, non-literal contents or path, or a path that collides with a module file. A variables-only generate (secret's common_variables) writes the same file on both sides of every module edit"
  - "Differences are covered only by a row with the same subject and direction. The one derived rule is that a module-level difference whose every model seed is an excluded unit is covered (1 on denis256: issue-4209/base)"
  - "Touch set of an excluded unit: the unit, the targets of its unit-file dependency blocks that are not literally disabled, and its live model edges. Taking every target would have dropped iso20022 s3_runtime only because of skip_outputs=true edges"
  - "R3 has no site on iso20022: every variable and output block there has one attribute. R3 is exercised inside R8, after R7 adds a second attribute (R8-R3 = 471), and the test requires R3 > 0 or R8-R3 > 0"
requirements-completed: []
duration: 60min
completed: 2026-10-12
---

# Phase 17 Plan 02: Independent Model, Harness and Reformat Summary

**The corpus proof has an oracle that shares no code with gruntled. It also has an editor, a driver and a comparator that are each shown able to fail. REL-03 clause 2 holds on all three corpora.**

- **The model.** It is built from source text with the v0.2 textual scanner plus Terragrunt's documented path functions. On the pristine corpora it agrees with `graph --json` except for 110 pinned rows. Each row is re-checked by a source-text predicate and must reproduce its file:line evidence: 89 gruntled-less, 5 gruntled-more and 16 oracle-limit.
- **Included modules:** iso20022 62/62, secret 3/3, denis256 795/818 (0.972), all pinned.
- **The harness self-test.** It needs no corpus and runs in CI. The real driver catches each of 22 injected faults, and a parse-breaking reformat trips the guard. expectBlast equals gruntled on one mutation of each class.
- **R1-R9.** Every transform gives exit 0 with empty broken, impacted and changes on iso20022, secret and denis256.
- **sec acceptance of the table** was requested on the bus (#449, corrected in #450) and is still pending. 17-03 must not start before it.

## Performance

- Duration: about 60 min
- Tasks: 3
- Files: 4 created (4,128 lines of test code)

## Accomplishments

### Task 1: the model and the per-item exclusions (2c7be77 red, 007b24d green)

The model lives in `cmd/gruntled/corpus_model_test.go`. It imports no internal/ package and no HCL library, and the architecture check prints OK. It computes:

- **Units:** directories that hold a terragrunt.hcl.
- **Effective config:** the unit file plus the includes `oracleIncludes` resolves, with include labels.
- **terraform.source:** the unit file wins over includes. The `//` subdirectory split is dropped and the path functions are evaluated. Remote sources are recognised by scheme, forced getter or host.
- **Dependency blocks:** quoted or bare labels. Labels in the unit file override included ones.
- **Live edges (BLAST-03):** `enabled` absent or literally true, `skip_outputs` absent or literally false, and a target that is a unit.
- **Reference sites:** found with modelKinds. Traversals in code or `${}` templates count; plain string text and comments do not. Each site is classified plain, guarded or other, and DIAG-03 silence is applied.
- **Module surfaces:** .tf and .tf.json files, balanced under the scanner, with override files flagged.
- **BFS (:1043):** canonical tie-break. The via is the smallest predecessor one level closer, which is what `RepoPath.Compare`, a plain string compare, gives.

Red, with empty tables (2c7be77): 12, 1 and 136 failures on iso20022, secret and denis256. Each uncovered difference lists its direction and its predicate candidates; that output was the review input.

Green (007b24d), run with:

```sh
go test -p=1 -count=1 -run "TestCorpusBlastModel/$c" -v -timeout 15m ./cmd/gruntled/
```

```
CORPUS-MODEL-FACTS iso20022 {Seeds:62 LiveEdges:11 PlainSites:22 Pairs:4 Guarded:0 Reach2:2 Reach3:0 Reach4:0 Reach5:0}
CORPUS-MODEL iso20022 units=65 modules=65 eligible=65 included_modules=62 graph_known=62 share=1.000 touch=3 derived=0 excluded_duplicate_dependency_label_in_one_file=3
CORPUS-MODEL-FACTS secret {Seeds:3 LiveEdges:3 PlainSites:4 Pairs:3 Guarded:0 Reach2:2 Reach3:0 Reach4:0 Reach5:0}
CORPUS-MODEL secret units=4 modules=4 eligible=3 included_modules=3 graph_known=3 share=1.000 touch=1 derived=0 excluded_include_target=1
CORPUS-MODEL-FACTS denis256 {Seeds:799 LiveEdges:588 PlainSites:460 Pairs:93 Guarded:0 Reach2:151 Reach3:26 Reach4:12 Reach5:2}
CORPUS-MODEL denis256 units=1145 modules=923 eligible=811 included_modules=795 graph_known=818 share=0.972 touch=95 derived=1 excluded_config_path_absolute_or_unresolvable=1 excluded_deep-merged_dependency_(non-literal_facts)=2 excluded_generate_block_may_declare_outputs=21 excluded_include_path_not_modelled=1 excluded_include_target=52 excluded_module_dir_contains_the_unit_dir=1 excluded_module_syntax_error_the_oracle_does_not_detect=4 excluded_more_than_one_unlabeled_include=2 excluded_nesting_over_the_1000-level_cap=1 excluded_reference_shape_other=16 excluded_source_not_modelled=2 excluded_terragrunt.hcl.json_unit=1 excluded_unit_config_syntax_error_the_oracle_does_not_detect=2
```

The iso20022 and secret facts match 17-RESEARCH:
- iso20022: 62 seeds with module equal to unit dir, 11 live edges, and 22 plain sites over 4 (module, output) pairs. The 3 `invalid-dependency` units are excluded by path.
- secret: 3 seeds, 3 live block edges and 4 sites. The root include target is excluded.

On denis256, 26, 12 and 2 included modules have a cone reaching distance 3, 4 and 5, all above zero. The included share is 0.972, far above the 50% floor, so no question was needed.

**The per-item table** is `corpusExclusions` (corpus_model_test.go:1980). Each row is {Path, Category, Direction, Evidence}.

| Category | Rows | Direction | Predicate (re-evaluated per row) |
|----------|-----:|-----------|----------------------------------|
| include target | 53 (secret 1, denis256 52) | gruntled-less | another unit resolves this unit file as an include, or the documented ancestor rule: a strict ancestor of a unit with an unresolvable include, or of a terragrunt.hcl.json unit |
| generate block may declare outputs | 21 | gruntled-less | a generate block whose literal contents hold an output block, whose contents or path are non-literal, or whose path collides with a module file |
| reference shape other | 16 | oracle-limit | a firing site that is not plain or guarded (index, function, template, list) at file:line |
| module syntax error the oracle does not detect | 4 | gruntled-less | lint: multi-line body after a one-line opener, stray top-level tokens, an unlabelled variable, an argument and the closing brace on one line |
| duplicate dependency label in one file | 3 (iso20022) | gruntled-less | a second dependency block with the same label |
| more than one unlabeled include | 2 | gruntled-less | a second bare `include {` |
| unit config syntax error the oracle does not detect | 2 | gruntled-less | `{{` in code, or a labelled block header inside an object constructor |
| deep-merged dependency (non-literal facts) | 2 (1 unit, both directions) | less + more | `merge_strategy = "deep"` and the included file declares the same label |
| source not modelled | 2 | gruntled-more | a terraform.source the model cannot evaluate (scaffold `{{ .sourceUrl }}`) |
| include path not modelled | 1 | gruntled-more | `"${find_in_parent_folders()}"` |
| terragrunt.hcl.json unit | 1 | gruntled-more | the directory has terragrunt.hcl.json |
| config_path absolute or unresolvable | 1 | gruntled-less | a literal config_path that is absolute or cannot be placed |
| module dir contains the unit dir | 1 | gruntled-less | the source resolves to an ancestor of the unit dir |
| nesting over the 1000-level cap | 1 | gruntled-less | bracket nesting over 1,000 in the unit file |

Each category was cross-checked against gruntled's own reason for that unit or module (include-target ↔ include-target, generate ↔ generate-may-declare-outputs, and so on). Every one matched.

No module is aliased through a directory symlink. pr-3562/fixture2/t1 and t2 are reached neither by the model nor by gruntled, so the editor's skip rule is the only place they matter.

The test fails on any of these:
- an uncovered difference;
- a row whose predicate does not give its evidence;
- a stale row;
- a firing other-shaped site, or an aliased module, without a row;
- a change in the included count or the facts.

The table and the counts were posted to sec for acceptance: #449, corrected in #450. **Acceptance is pending at the time of writing; 17-03 waits for it.**

### Task 2: editor, comparator and harness self-test (d80d270)

**Editor** (`cmd/gruntled/corpus_edit_test.go`):
- findBlocks and scanAttrs over modelKinds. They handle multi-line values, heredocs, nested blocks and one-line blocks.
- Operations: delete block, new file, delete attribute, set attribute, insert attribute, move block, materialise symlink, and `.tf` → `.tf.json`. The JSON conversion handles simple blocks only and is emitted with encoding/json.
- `confined` (:191) refuses a symlinked component, a path outside the root, and a link target outside the root.
- `tryApplyEdit` (:236) and `applyEdit` (:320) record sha256 and mode; restore re-checks the sha256. Created files are removed, and a materialised link is put back as the same link.
- `digestGuard` (:358, :365) compares the working copy with the pristine digest.

**Harness** (`cmd/gruntled/corpus_harness_test.go`):
- `expectBlast` (:182) works from the model and the descriptor only and never reads graph --json. Broken findings come from M1's firing plain sites (GRT004, message prefix), and Broken units are traversed but not listed. Impacted comes from the model BFS, with the four lists at distance 1 only.
- `compareBlast` (:329) compares exit, version, kind, baseline, note, summary, exact broken and finding sets, impacted facts, and the changes lists and type_changes. `old` is compared exactly for absent and primitive types, otherwise only as non-empty and different from `new`.
- `runMutation(r reporter, h, m, hook)` (:487) is the real driver.
- `assertSurfaceUnchanged` (:589).

The M5 sentinel's canonical form, `object({"gruntled_m17"=string})`, was derived by hand from the D-15 rules and HEAD produces exactly that.

**`TestCorpusHarnessSelfTest` (:707)** runs always on, with no corpus env, and passes 26 subtests. The hand tree has 2 modules, 4 units, a 3-unit chain through a Broken unit, a skip_outputs edge, a guarded reference, a shared file linked into both modules, a heredoc, a dir-symlink alias and an escaping link.
- **expect matches blast:** 11 mutations covering every class, including both M9 shared-link variants, caught 11 of 11. The real JSON for M1 equals the hand derivation in the test comment: GRT004 at live/app/terragrunt.hcl:10:8 at distance 2 via live/net, and live/web at distance 3 through the Broken unit.
- **22 fault subtests through the real hook.** Each records a failure and leaves caught < sites, and the same mutation without the fault is caught. The faults:
  - impacted unit dropped; impacted unit added; distance off by one; wrong via; wrong source; wrong module;
  - wrong name in removed_outputs; wrong name in added_variables;
  - GRT001 left at a GRT004 site; one GRT004 site missing; GRT004 at a silent site;
  - extra changes entry; missing changes entry;
  - wrong type_changes kind; wrong type_changes new; type_changes old-rule violation;
  - wrong exit code; summary mismatch;
  - wrong version, kind or baseline (one combined subtest, plus one each).
- **The guard trips on a parse-breaking reformat**, and stays silent on an unchanged copy.
- **6 refusals:**
  - the escaping link;
  - a write through the dir-symlink alias;
  - a create through the alias;
  - materialising the escaping link;
  - `../outside.tf`;
  - a write to a symlinked shared file.
- **9 editor operations,** each checked for exactly its effect in graph --json and the file text, with a byte-exact restore.

Mutants:
- Removing the "impacted missing" check makes `fault/impacted_unit_dropped` fail ("fault not caught: 0 messages, caught 1 of 1").
- Counting `caught` before the comparison makes 23 subtests fail. That is the wiring fault from sec #416.

### Task 3: R1-R9 (94c580b fix, b2140a0)

`cmd/gruntled/corpus_reformat_test.go` holds the transforms. They are patches at code bytes, never inside a string, template, heredoc or comment. The R4 variants inject `gruntled_m17_opt` on both sides, as "added" (`optional(string)` → `optional(string, "x")`) and "changed" (`"y"` → `"x"`). Each R4 run first asserts that base' against the pristine tree adds the variable to every included module. R5 runs in both directions.

**Red first.** The always-on `TestCorpusReformat/selftest` applies a deliberately wrong R3 that reverses body lines across a heredoc, and the guard must trip. Developing against the corpora, the guard then caught three real transform bugs, and each was fixed:
- a `/* */` comment after the brace of a one-line block, which is invalid HCL;
- a heredoc opener's line end classified as code, so an attribute appeared to end at `<<EOT`. Fixed in modelKinds, 94c580b;
- CRLF files getting `// r2` between `\r` and `\n` (denis256 path-printing and stack-in-stack-error).

Self-test counts: R1 14, R2 19, R3 3, R4 2, R4-changed 2, R5 6 (1 skipped), R6 7, R7 7, R8 72, R9 2.

Corpus runs, one corpus at a time with `free -m` first:

```sh
go test -p=1 -count=1 -run "TestCorpusReformat/$c" -v -timeout 90m ./cmd/gruntled/
```

| Transform | iso20022 | secret | denis256 |
|-----------|---------:|-------:|---------:|
| R1 | 1443 | 54 | 1520 |
| R2 | 1413 | 44 | 2556 |
| R3 | 0 (R8-R3 471) | 5 | 43 |
| R4 | 62 | 3 | 795 |
| R4-changed | 62 | 3 | 795 |
| R5 | 469 (2 skipped) | 9 (3 skipped) | 1103 (88 skipped) |
| R6 | 471 | 12 | 1191 |
| R7 | 471 | 12 | 1191 |
| R8 | 5211 | 159 | 10875 |
| R9 | 62 | 0 (no symlinked module file) | 0 (no symlinked module file) |
| result | PASS 4.66 s | PASS 0.38 s | PASS 22.94 s, 144 MB RSS |

Every run had exit 0, broken, impacted and changes all `[]`, a passing guard, and corpusDigest of the checkout unchanged.

## Task Commits

| Task | Commit | Message |
|------|--------|---------|
| 1 (red) | 2c7be77 | test(17-02): independent corpus model and TestCorpusBlastModel, red with empty exclusion tables |
| 1 | 007b24d | test(17-02): pin the per-item exclusion table, included counts and model facts |
| 2 | d80d270 | test(17-02): module editor with confined applyEdit, mutation driver, comparator and always-on harness self-test |
| 3 | 94c580b | fix(17-02): a heredoc opener's line end belongs to the heredoc in modelKinds |
| 3 | b2140a0 | test(17-02): TestCorpusReformat R1-R9 with the non-vacuity guard, red on a reorder across a heredoc |

## Gate

```sh
go vet ./cmd/gruntled/                                        # clean
gofmt -l cmd/gruntled/                                        # empty
bash scripts/check-architecture.sh                            # architecture: OK (4 domain packages, 5 application packages, 1 interfaces packages)
go test -count=1 -short -timeout 1200s ./cmd/gruntled/        # ok 7.907s (no corpus env, as in CI)
go test -count=1 -run 'TestCorpusBlastModel|TestCorpusReformat|TestCorpusHarnessSelfTest' -v ./cmd/gruntled/   # without env: harness PASS, selftest PASS, 6 corpus subtests SKIP
```

The gated tests were run per corpus, serially, as in the tables above. Not run: -race, -fuzz, `go test ./...`, and scripts/test-check-architecture.sh, because no architecture script changed.

## Threat Mitigations

| ID | Status | Where |
|----|--------|-------|
| T-17-02-1 | mitigated | no internal/ or HCL import (architecture OK); per-item rows re-checked by predicate (corpus_model_test.go:1753 test, `modelPredicates`); direction rule (a row covers only its own direction); removal through the touch set (`universe`, :1710); expectBlast never reads graph --json (corpus_harness_test.go:182); sec review requested (#449) |
| T-17-02-2 | mitigated | included counts and facts pinned (`corpusIncluded`, `corpusModelFacts`); uncovered differences and stale rows fail; denis256 share 0.972 |
| T-17-02-3 | mitigated | assertSurfaceUnchanged (corpus_harness_test.go:589) on every transform; counters > 0; the red wrong-R3 case; three real breaks caught |
| T-17-02-4 | mitigated | 22 fault subtests through runMutation's hook (:487, :707); comparator and wiring mutants fail; strict decode |
| T-17-02-5 | mitigated | `confined` and `tryApplyEdit` (corpus_edit_test.go:191, :236); 6 refusal rows; sha256 re-check on restore; digestGuard; checkout corpusDigest before and after |
| T-17-02-6 | mitigated | `timeout` and `-timeout` on every run; one corpus at a time; peak 144 MB |
| T-17-02-7 | mitigated | every patch lands at a code byte (modelKinds); editor operations checked against graph --json in the self-test |

## Deviations from Plan

1. **[Model scope] Hidden directories are scanned, against the plan's "skipping dot-dirs".** gruntled lists units under .cloud, .ci, .scratch and .boilerplate. A model that skipped them could not see their edges, so cones touching them could not be removed. Only .git, .terragrunt-cache and .terraform are skipped.
2. **[Model scope] Terragrunt path functions are modelled.** These are get_terragrunt_dir, get_parent_terragrunt_dir with and without a label, path_relative_to_include, path_relative_from_include, and find_in_parent_folders in config_path. Each follows its documented meaning; nothing was copied from gruntled. Without them, denis256's perf-tests tree (34 units, the deep cones) would have become rows.
3. **[Eligibility] Only a surface-changing generate excludes a module.** The literal "no generate block in an instantiating unit" would leave secret with 0 eligible modules: its root generates variables only. A generate block counts when it may declare an output, is non-literal, or collides with a module file.
4. **[Coverage] One derived rule.** A module-level difference whose every model seed is an excluded unit counts as covered. This happened once, on issue-4209/base.
5. **[R3] Zero sites on iso20022.** Every block there has one attribute. R3 runs inside R8 after R7 adds a second one: 471 blocks reordered.
6. **[R9] Zero on secret and denis256.** Only iso20022 has symlinked module files, which the plan anticipated ("R5 and R9 > 0 on iso20022 at least").
7. **[Fault count]** The 20 listed faults are 22 subtests: wrong version, kind and baseline are tested together and also one at a time. The parse-breaking reformat is a separate subtest.

## Known Risks / Unverified

- sec's acceptance of the exclusion table is pending (#449, #450). 17-03 must not start without it.
- Guarded sites: no corpus has a firing guarded site (Guarded 0). The guarded rule is exercised only on the self-test tree, where `try()` is silent.
- The reference-shape classifier is line-based. A plain assignment that carries a trailing comment is classified "other" and becomes a row; no corpus has one today.
- Nothing is pushed, so the always-on self-test has no CI run yet.

## Self-Check: PASSED
