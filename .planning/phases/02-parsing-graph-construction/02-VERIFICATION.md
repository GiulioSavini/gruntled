---
phase: 02-parsing-graph-construction
verified: 2026-09-28T08:49:26Z
status: gaps_found
score: 5/5 roadmap success criteria verified; 32/33 gap-closure must-have truths verified (1 partial)
gaps:
  - truth: "A terragrunt.hcl that another unit includes is config-unknown include-target; its references are still checked once per including unit (02-07 must-have, 02-REVIEW G3)"
    status: partial
    reason: >
      The include-target post-pass keys `located` by the lexical include path
      (loader.go resolveIncludes: `located[p] = true`, then
      `located[path.Join(u.Path.String(), "terragrunt.hcl")]` in LoadUnits).
      When the include path reaches the parent config through an in-repo
      symlink (a symlinked directory such as live/link -> parent, or a
      symlinked file), the key is "live/link/terragrunt.hcl", never
      "live/parent/terragrunt.hcl", so the parent stays resolved and is
      analysed standalone. This is exactly the G3 false-GRT001 shape:
      reproduced on a real os.OpenRoot tree, live/parent (config_path
      "../vpc") resolves to live/vpc, whose module lacks output "id", while
      the only real consumer live/x/app resolves the same text to
      live/x/vpc, which declares it. With a plain path the same fixture
      gives live/parent config-unknown include-target and no missing
      output. fs.Stat and cache.get both follow the symlink, so the include
      itself parses fine; only the include-target bookkeeping misses it.
    artifacts:
      - path: "internal/infrastructure/terragrunt/loader.go"
        issue: "located map (resolveIncludes, LoadUnits step 13) is keyed by the lexical path, not the file's identity; a symlink alias bypasses the include-target guard"
    missing:
      - "Fail toward unknown when an include path traverses a symlink: either key located by the canonical in-repo path (resolve each path segment with fs.ReadLink/Lstat inside the repo), or make an include whose path crosses any symlink config-unknown with a new reason"
      - "A failing test first: a real-FS fixture (t.TempDir + os.Symlink + os.OpenRoot, like realfs_test.go) where live/x/app includes ../../link/terragrunt.hcl with link -> parent, asserting live/parent is config-unknown include-target"
      - "A catalogue row in 02-TERRAGRUNT-EDGECASES.md for the symlink-aliased include target"
---

# Phase 2: Parsing & Graph Construction Verification Report

**Phase Goal:** gruntled walks a real Terragrunt repository on disk and builds a complete, correctly-resolved `RepositoryGraph` (every unit, the module it resolves to, and that module's public surface) using only structural HCL decoding, never evaluating an expression to a value.
**Verified:** 2026-09-28T08:49:26Z
**Status:** gaps_found (one narrow, partial gap; all five roadmap success criteria hold)
**Re-verification:** No, initial verification (02-REVIEW.md is a review input, not a previous VERIFICATION.md)

## Commands run

| Command | Result |
|---|---|
| `GOTOOLCHAIN=go1.27.0 go test -count=1 ./...` | all packages ok (indexing, diagnostic, repograph, hclconv, sourceresolve, terragrunt, tfsurface, synthrepo) |
| `GOTOOLCHAIN=go1.27.0 go vet ./...` | clean |
| `bash scripts/check-architecture.sh` | `architecture: OK (2 domain packages, 2 application packages)`, exit 0 |
| `bash scripts/test-check-architecture.sh` | every case PASS, including the G13/G14 probes (layout-unknown-dir, infra-from-tagged-file, testsupport-in-tagged-prod, hcl-windows-file-in-cmd, hcl-tagged-in-testsupport, hcl-aliased-import-block) |
| `GRUNTLED_CORPUS=~/.cache/gruntled-phase4/corpus/primary go test -run TestCorpusSmoke -v` | PASS: 65 units, 3 config-unknown (duplicate `dependency "iam"` labels, documented), 0 module-unknown, 22 references, 0 missing outputs |
| `GRUNTLED_CORPUS_SECRET=~/.cache/gruntled-phase4/corpus/secret go test -run TestIncludeTargetSecretCorpus -v` | PASS: parent `terragrunt` is include-target; acm/ecr/lambda resolve to aws/*; 4 refs, 0 missing |
| `go test -run '^$' -fuzz FuzzLoadUnits -fuzztime 30s` | 121k execs, no crasher |
| `git status --short` after all runs | clean (probes ran in a scratch copy, since deleted) |

The corpus tests are env-gated: `TestCorpusSmoke` reads `GRUNTLED_CORPUS`, `TestIncludeTargetSecretCorpus` reads `GRUNTLED_CORPUS_SECRET`, and both skip when the variable is unset, so the plain `go test ./...` run does not exercise them. I ran both explicitly against the local checkouts.

## Goal Achievement

### Observable Truths (ROADMAP success criteria)

| # | Truth | Status | Evidence |
|---|---|---|---|
| 1 | Only `include`, `terraform.source`, `dependency` (plus `generate` for the output guard) are read structurally; the six path functions are evaluated correctly | VERIFIED | parse.go switches on block.Type only; eval.go `evalPath` uses `hcl.EvalContext{Functions: s.functions()}` with Variables nil and exactly the six functions (pathfuncs.go). find_in_parent_folders starts at the parent dir and probes terragrunt.hcl.json first, matching Terragrunt. S0/S1/S2 scopes are implemented per research Pattern 4. Tests: TestPathFuncs* (9), TestEvalPathFailsClosed, TestStructuralOnly. Probe: `get_env`, `local.x` in include/source fail closed |
| 2 | Each include file is parsed exactly once and shared | VERIFIED | parse.go `fileCache.get` memoizes by path; TestParseOnce (50 units, root.hcl ReadFile count == 1, every file <= 1), TestLoaderParseOnceIncludes, and a shared over-limit include read once (02-11) |
| 3 | Local source, absent source (own dir), and remote/dynamic source classified offline | VERIFIED | loader.go resolveSource + sourceresolve.Classify (pure string classification; no net/exec import in non-test code). Probe: `tfr:///...` gives module-unknown remote-source; `${local.x}/mod` and `get_env("X")` give source-dynamic-path; absent source gives module = unit dir |
| 4 | Dependency resolved through both hops; surface extracted when the unit dir differs from the module dir | VERIFIED | TestTwoHop (live/app -> aws/lambda, dep in include). Independent probe: live/vpc with `source = "${get_parent_terragrunt_dir()}/../modules//vpc"` resolves to modules/vpc; app's `vpc_id` ref is found, `nope` is reported missing, and a ternary-branch ref is dropped (G1). Primary corpus: 22/22 refs land on declared outputs |
| 5 | Unresolvable constructs give unknown; invalid HCL gives a diagnostic, never a crash; cache/.terraform/vendor/symlinks are not walked | VERIFIED | reasons.go catalogue (config-unknown, module-unknown, unresolved-dependency); probe: mid-edit `inputs = { a = [` gives config-unknown syntax-error + one GRT100 at e/terragrunt.hcl:3:1; hclconv limits (4 MiB, depth 1000) stop the G7 stack-overflow crash (TestDeepNestingNoCrash, TestDeepSharedIncludeNoCrash, TestDeepModuleFileEndToEnd); walk.go skips .git/.terraform/.terragrunt-cache/vendor and every symlink DirEntry (TestWalk* incl. symlink cycle and outside symlink) |

**Score:** 5/5 roadmap success criteria verified.

### Gap-closure must-haves (02-06..02-11, from 02-REVIEW.md G1..G14)

| Plan | Gap | Status | Evidence |
|---|---|---|---|
| 02-06 | G1 lazy evaluation | VERIFIED | refs.go `lazyRanges` (ternary branches, &&/\|\| operands, for key/value/cond; condition and for collection still count); TestExtractRefsLazyEvaluation, TestExtractRefsLazySiblingAttributesKeepPosition; probe confirmed |
| 02-07 | G2 stack target | VERIFIED | resolveOneDependency: stack file or dir holding one gives config-path-stack (wins over sibling terragrunt.hcl); loader_test rows |
| 02-07 | G3 include-target | PARTIAL | Works for lexical paths (TestIncludeTargetCorpusReproduction, TestIncludeTargetExplicitPath, secret corpus). Bypassed by a symlink alias, see Gaps |
| 02-07 | G4 non-default file | VERIFIED | config-path-nondefault-file for any regular file other than terragrunt.hcl |
| 02-07 | G5 JSON include | VERIFIED | `.json` include gives include-json-unsupported before parsing, so no GRT100 |
| 02-07 | G6 overlay ReadDir failure | VERIFIED | unitDirOverlaysModule returns module-file-unreadable |
| 02-07 | G8 invalid config_path | VERIFIED | config-path-invalid affects only that dependency |
| 02-07 | G9 malformed generate | VERIFIED | validateEffectiveFile gives invalid-generate |
| 02-07 | G11 comment | VERIFIED | loader.go resolveUnit comment now states the zero UnitConfig fails loudly in indexing.Build (NewRepositoryGraph/constructors reject a zero path) |
| 02-08 | G12 zero values | VERIFIED | position/unit/module/graph constructors reject zero Position, invalid Tristate, zero entries, zero-path units/modules, invalid UnitStatus, unknown module without reason |
| 02-09 | G13/G14 arch holes | VERIFIED | rules internal-layout, infrastructure-importers, testsupport-only-in-tests, source-level HCL import scan; self-tests PASS |
| 02-10 | G7a module files | VERIFIED | tfsurface refuses oversize/overdeep before parsing (module-file-too-large / -too-deep) |
| 02-11 | G7b unit/include files, G10 fuzz, catalogue | VERIFIED | config-too-large/-too-deep, no GRT100; FuzzLoadUnits fuzzes body + shared root.hcl/main.tf; STACK-09 and STACK-11 rows in 02-TERRAGRUNT-EDGECASES.md |

### Required Artifacts

| Artifact | Status | Details |
|---|---|---|
| internal/infrastructure/terragrunt/{walk,parse,refs,pathfuncs,eval,merge,depfacts,loader,reasons}.go | VERIFIED | substantive, implements ports.UnitLoader, used by indexing.Build in integration and corpus tests |
| internal/infrastructure/tfsurface/reader.go | VERIFIED | implements ports.SurfaceReader; .tf/.tf.json/.tofu precedence, override dedup, limits |
| internal/infrastructure/hclconv/{hclconv,limits}.go | VERIFIED | byte-column positions, FirstSyntaxError, ReadFileLimited, CheckNativeDepth/CheckJSONDepth; used by parse.go and reader.go |
| internal/infrastructure/sourceresolve/classify.go | VERIFIED | offline classifier used by loader.resolveSource |
| internal/application/indexing/build.go, ports/ports.go | VERIFIED | Build wires loader, then surfaces once per distinct module, then graph |
| internal/domain/repograph/* | VERIFIED | unknown states, unresolved deps, DependencyOptions, G12 invariants; architecture check proves no HCL/fs imports |

### Key Link Verification

| From | To | Via | Status |
|---|---|---|---|
| indexing.Build | terragrunt.Loader | ports.UnitLoader.LoadUnits | WIRED |
| indexing.Build | tfsurface.Reader | ports.SurfaceReader.ReadSurface per distinct resolved module | WIRED |
| loader.resolveUnit | fileCache | cache.get (parse once) | WIRED |
| loader | sourceresolve.Classify | resolveSource | WIRED |
| parse.go / tfsurface | hclconv limits | ReadFileLimited + depth check before hclsyntax | WIRED |
| RepositoryGraph | two-hop queries | DependencyTarget -> ModuleOf -> Surface | WIRED |
| cmd/gruntled/main.go | indexing.Build | composition root | NOT WIRED, by design: the CLI is Phase 3 (CLI-01..05); Phase 2 is proven through the corpus tests on os.OpenRoot |

### Requirements Coverage

| Requirement | Description | Status | Evidence |
|---|---|---|---|
| PARSE-01 | Structural decode of include/terraform.source/dependency | SATISFIED | parse.go, TestStructuralOnly |
| PARSE-02 | Six pure path functions | SATISFIED | pathfuncs.go, TestPathFuncs* |
| PARSE-03 | Includes, merge strategy, parse once | SATISFIED | merge.go precedence/no_merge/deep, TestIncludePrecedence, TestParseOnce |
| PARSE-04 | Unknown when offline-unresolvable; no diagnostic for unknown units | SATISFIED (see gap for one include-target alias) | reasons.go catalogue; only file-level GRT100 is emitted, no unit-scoped diagnostic |
| PARSE-05 | Invalid HCL gives a diagnostic, no crash | SATISFIED | GRT100 via FirstSyntaxError; limits prevent unrecoverable stack overflow; fuzz |
| PARSE-06 | Skip cache/.terraform/vendor/symlinks | SATISFIED | walk.go skipDirNames + ModeSymlink guard, TestWalk* |
| GRAPH-01 | Module via terraform.source | SATISFIED | resolveSource, TestSourceForms |
| GRAPH-02 | Module = own dir when source absent | SATISFIED | resolveSource fallback |
| GRAPH-03 | Remote classified without download, unit unknown | SATISFIED | Classify + ReasonRemoteSource, TestModuleUnknownBlastRadius |
| GRAPH-04 | Two-hop dependency resolution | SATISFIED | TestTwoHop, corpus 22/22 |
| GRAPH-05 | variable/output names from .tf and .tf.json | SATISFIED | tfsurface reader tests incl. JSON object form and mixed union |

No orphaned requirements: REQUIREMENTS.md maps exactly PARSE-01..06 and GRAPH-01..05 to Phase 2.

### Anti-Patterns Found

| File | Line | Pattern | Severity | Impact |
|---|---|---|---|---|
| (none in production code) | - | no TODO/FIXME/XXX/HACK in non-test Go files | - | - |
| cmd/gruntled/main.go | 13-14 | stub main ("no commands implemented yet") | Info | expected: Phase 3 wires the CLI |
| .planning/ROADMAP.md | 71-76 | gap-closure plans 02-06..02-11 still `[ ]` while the phase is `[x]` | Info | bookkeeping only |
| 02-VALIDATION.md | per-task map | GC rows still "pending" | Info | bookkeeping only |

### Human Verification Required

None needed for the phase goal. Everything above was checked programmatically, including the env-gated corpus runs.

### Gaps Summary

The phase goal is met. All five roadmap success criteria and all 11 requirements are backed by real code and passing tests, including both real-corpus checks and a short fuzz run. Every 02-REVIEW gap (G1..G14) is closed as specified, with one exception: the G3 include-target guard.

That guard identifies a parent config by the lexical path an including unit used. If a unit includes the parent through an in-repo symlink (`include { path = "../../link/terragrunt.hcl" }` with `link -> parent`), the parent is not marked include-target. It is then analysed standalone against the wrong directory. I reproduced this on a real filesystem: a reference in the parent lands on a module that lacks the output, which is the same false-GRT001 shape G3 was meant to remove. Without the symlink, the same fixture behaves correctly.

This is a narrow edge case, and nothing in the checked corpora hits it. The primary corpus only symlinks module `.tf` files, and denis256's symlinked `terragrunt.hcl` files are skipped by discovery. Still, it breaks the plan's own must-have, and the project's zero-false-positive rule means it should fail toward unknown. The fix is small and local to loader.go: canonicalize the include path, or mark a symlink-traversing include config-unknown. It should come with a real-FS test like the ones in realfs_test.go.

---

_Verified: 2026-09-28T08:49:26Z_
_Verifier: gsd-verifier_
