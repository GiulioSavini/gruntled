---
phase: 02-parsing-graph-construction
verified: 2026-09-29T00:00:00Z
status: passed
score: 5/5 roadmap success criteria verified; 14/14 plans' must-haves verified; 11/11 requirements satisfied; G15..G22 all closed
re_verification:
  previous_status: gaps_found
  previous_score: "5/5 roadmap criteria; 32/33 gap-closure truths (1 partial)"
  gaps_closed:
    - "G3/G15: include-target guard bypassed by a symlinked include path (02-13: targets matched by canonical in-repo path)"
    - "G16: parent only marked when its includer resolves (02-13: unknowable/failed includers mark ancestors; dynamic paths mark include-free units)"
    - "G17: ternary nesting and long operator chains crash or exhaust memory (02-12)"
    - "G18: FIFO named terragrunt.hcl / module file hangs the process (02-12)"
    - "G19: generate output detector was a line regex (02-13: any 'output' substring or \\u escape may declare)"
    - "G20: x.tofu shadowed x.tf (02-12: union surface)"
    - "G21: awk import scan bypassable by a comment or ';' (02-14: go/parser scanner)"
    - "G22: nested go.mod escaped the domain rules (02-14: single-module rule)"
  gaps_remaining: []
  regressions: []
---

# Phase 2: Parsing & Graph Construction Verification Report

**Phase Goal:** gruntled walks a real Terragrunt repository on disk and builds a complete, correctly-resolved `RepositoryGraph` (every unit, the module it resolves to, and that module's public surface) using only structural HCL decoding, never evaluating an expression to a value. Zero false positives is paramount.
**Verified:** 2026-09-29 (master 8e8ec52)
**Status:** passed
**Re-verification:** Yes, after gap cycle 1 (plans 02-12, 02-13, 02-14). The previous report was `gaps_found` for one partial truth: the G3 include-target guard was bypassed through a symlinked include.

## Commands run (GOTOOLCHAIN=go1.27.0)

| Command | Result |
|---|---|
| `go vet ./...` | clean |
| `go test -count=1 ./...` | all packages ok (incl. new `scripts/archscan`) |
| `bash scripts/check-architecture.sh` | `architecture: OK (2 domain packages, 2 application packages, 1 interfaces packages)` |
| `bash scripts/test-check-architecture.sh` | 56 PASS, "all architecture self-tests passed", exit 0. Includes hcl-comment-in-import-block, hcl-semicolon-import-block, infra-comment-in-import-block, testsupport-semicolon-in-tagged-prod, nested-go-mod, nested-go-mod-dot-dir, go-work, and the no-trip cases hcl-mention-in-comment-allowed and unrelated-go-mod-in-dot-dir-allowed |
| `TestCorpusSmoke` (`GRUNTLED_CORPUS=.../corpus/primary`) and `TestIncludeTargetSecretCorpus` (`GRUNTLED_CORPUS_SECRET=.../corpus/secret`) | both PASS |
| `go test -run '^$' -fuzz FuzzLoadUnits -fuzztime 30s` | PASS, about 247k execs, no crasher |
| `git status --short` after all runs | clean; every probe ran in a mktemp copy, since deleted |

## Goal Achievement

### Observable Truths (ROADMAP success criteria)

| # | Truth | Status | Evidence |
|---|---|---|---|
| 1 | Only include / terraform.source / dependency (plus generate for the output guard) read structurally; six path functions evaluated | VERIFIED | Unchanged from the previous report; tests green, no regression in parse.go, pathfuncs.go, eval.go |
| 2 | Each include file parsed exactly once and shared | VERIFIED | TestParseOnce and include parse-once tests pass. 02-12 moved ReadFileLimited to a single Open + `io.LimitReader`; parse-once tests still prove one read per file |
| 3 | Local, absent and remote/dynamic source classified offline | VERIFIED | sourceresolve tests green; no net/exec imports |
| 4 | Dependency resolved through both hops; surface extracted | VERIFIED | TestTwoHop; primary corpus 65 units, 22 refs, 0 missing |
| 5 | Unresolvable gives unknown; invalid HCL gives a diagnostic, never a crash; cache/.terraform/vendor/symlinks not walked | VERIFIED | G17/G18 closed (below); fuzz clean; walk tests green |

**Score:** 5/5.

### Gap closure G15..G22 (reproduced in a scratch copy on a real os.OpenRoot tree)

| Gap | Status | Independent evidence |
|---|---|---|
| G15 symlinked include | CLOSED | Fixture: live/terragrunt.hcl has dependency `./vpc` (lacks `id`); live/x/app includes it. symlinked dir (`live/link -> .`), symlinked file, alias of an ancestor dir: `live` = config-unknown/include-target, missing outputs = 0. Escaping, looping, backslash and unresolvable links: the includer goes to include-not-found, never resolved against a guessed path |
| G16 failing/dynamic includers | CLOSED | `${get_repo_root()}/...`, `${get_path_to_repo_root()}/...`, `local.*`, `get_env()`: `live` include-target, missing=0. `find_in_parent_folders()` plus a syntax error: `live` include-target. First include failing, second explicit: `live` still include-target. Dynamic path with a fixed non-terragrunt.hcl name marks ancestors only (residual catalogued) |
| G17 ternary/chain depth | CLOSED | 1M-link else-chain (4 MB), 100k true-chain, 2M `+` chain, 5000 parens, ternary inside call args, newline-in-parens and template interpolation: all config-unknown/config-too-deep, no crash (worst case 3.8 s). Module file with a 1M else-chain: surface unknown, module-file-too-deep |
| G18 FIFO | CLOSED | FIFO `terragrunt.hcl` is not a unit; FIFO `extra.tf`: surface unknown, module-file-unreadable; build finishes, no hang |
| G19 generate detector | CLOSED | contents with a `/* */` comment prefix, `;`-prefixed, indented, JSON, `o` escape, `file()`, `local.*`, quoted and templated forms: all module-unknown/generate-may-declare-outputs, missing=0. Contents with no "output" still resolve (real finding preserved) |
| G20 tf/tofu | CLOSED | outputs.tf `id` plus outputs.tofu `other` (and reverse, and .tf.json/.tofu.json): both names present, missing=0 |
| G21 awk scan | CLOSED | go/parser ImportsOnly helper in scripts/archscan (own unit tests); comment and `;` bypass probes fail by rule name in the self-test |
| G22 nested go.mod | CLOSED | single-module rule; nested-go-mod, dot-dir variant and go.work each fail by name; an unrelated go.mod under a dot dir is allowed |

### Plan must-haves 02-01..02-14

02-01..02-05 (parsing, path functions, includes, sources, graph, surface) and 02-06..02-11 (G1..G14) were verified in the previous report and remain green: the full test suite, the corpus smokes and the fuzz run show no regression. 02-12, 02-13 and 02-14 truths are covered by the G15..G22 table plus the green self-test. The 02-13 catalogue truth holds: 02-TERRAGRUNT-EDGECASES.md has INC-14/15/16, SRC-16/17, STACK-13/14, the STACK-09 revision and a "Gap closure (02-12..02-14)" list.

### Requirements Coverage

| Requirement | Status | Evidence |
|---|---|---|
| PARSE-01..PARSE-06 | SATISFIED | parse.go, pathfuncs.go, merge.go, reasons.go, walk.go; PARSE-05 is now also crash- and hang-proof (G17/G18). PARSE-04's include-target caveat from the previous report is closed |
| GRAPH-01..GRAPH-05 | SATISFIED | resolveSource, Classify, TestTwoHop, tfsurface (now union of tf/tofu) |

No orphaned requirements: REQUIREMENTS.md maps exactly PARSE-01..06 and GRAPH-01..05 to Phase 2.

### Accepted false-negative trade-off (not a gap)

02-13-SUMMARY measured the include-free rule on denis256: include-target units rise from 53 to 719 of 1146, and the two genuine findings (`issue-2631/main`, `mocks/module1`) become hidden. This is documented in 02-13-SUMMARY (key decisions and the measurement table) and in 02-TERRAGRUNT-EDGECASES.md (STACK-09 revision, "Measured on denis256: 53 to 719 of 1146"), under the stated policy "ten false negatives beat one false positive". It produces unknowns only; I found no false positive from it. Phase 4 denis256 expectations must account for it.

### Key Link Verification

All links from the previous report remain WIRED. New: LoadUnits step 13 uses `includeTargets.isTarget` (canonical path, ancestor, include-free flag); resolveUnit calls `markIncludeDecls` before validation; `check-architecture.sh` calls `go run ./scripts/archscan`. `cmd/gruntled/main.go` still does not call indexing.Build, by design (CLI is Phase 3).

### Anti-Patterns Found

| File | Pattern | Severity | Impact |
|---|---|---|---|
| cmd/gruntled/main.go | stub main | Info | expected, Phase 3 |
| .planning/ROADMAP.md lines 29, 79-81 | Phase 2 still `[ ]`; plans 02-12..02-14 still `[ ]` (line 29 says 11/14 executed) | Info | bookkeeping only; the orchestrator should tick them |
| 02-VALIDATION.md | GC rows may still read "pending" | Info | bookkeeping only |

No TODO/FIXME in production code.

### Human Verification Required

None.

### Gaps Summary

No gaps. All five roadmap criteria, all 11 requirements and gaps G15..G22 are verified against the code and by independent reproduction. The accepted include-free coverage cost is documented. Every new failure mode fails toward unknown, and I found no false positive.

---

_Verified: 2026-09-29_
_Verifier: gsd-verifier_
