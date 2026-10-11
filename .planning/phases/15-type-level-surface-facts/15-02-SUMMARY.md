---
phase: 15-type-level-surface-facts
plan: 02
subsystem: tfsurface, hclconv, architecture, ci
tags: [type-facts, typeexpr, dos, stack-overflow, fuzz, architecture-rule]
requires: [15-00, 15-01]
provides:
  - "tfsurface.typeSig, the canonical, depth-capped, panic-free type renderer"
  - "per-name variable/output facts read by ReadSurface (NewSurfaceFacts), with override and duplicate rules"
  - "hclconv.CheckNativeExprDepth, the depth scan in expression mode"
  - "typeexpr-only-in-tfsurface architecture rule with self-test probes"
  - "CI step: 30 s of FuzzTypeSig"
affects: [15-03, 15-04, 15-05]
tech-stack:
  added: ["github.com/hashicorp/hcl/v2/ext/typeexpr (already in go.sum; go.mod/go.sum unchanged)"]
  patterns: ["JSON type string: 64 KiB cap, then bracket scan, then expression-mode depth scan, all before typeexpr", "narrow recover only around typeexpr + renderer, unwrapped for fuzz"]
key-files:
  created:
    - internal/infrastructure/tfsurface/typesig.go
    - internal/infrastructure/tfsurface/typesig_test.go
    - internal/infrastructure/tfsurface/facts.go
    - internal/infrastructure/tfsurface/facts_test.go
    - cmd/gruntled/type_bomb_test.go
  modified:
    - internal/infrastructure/hclconv/limits.go
    - internal/infrastructure/hclconv/limits_test.go
    - internal/infrastructure/tfsurface/reader.go
    - scripts/check-architecture.sh
    - scripts/test-check-architecture.sh
    - .github/workflows/ci.yml
key-decisions:
  - "The cmd-level JSON bomb smoke is a Go test (cmd/gruntled/type_bomb_test.go) instead of check_json_type_bomb.txtar: a 2.4 MB hostile file cannot be committed, testscript cannot generate it, and the orchestrator asked for an empty cmd/gruntled/testdata diff"
  - "The typeexpr rule exempts only the tfsurface package itself, not packages below it; the source-scan half filters by file path instead of -exclude, which is a directory prefix"
  - "typeexpr is reached through a package variable (typeConstraint) so tests count calls and prove a refused string never reaches it"
requirements-completed: []
duration: 75min
completed: 2026-10-11
---

# Phase 15 Plan 02: Type Facts in the Surface Reader Summary

**ReadSurface now reads per-name type facts: whether a variable is required, a variable's canonical type (`any` when absent), an output's declared type and an output's literal `sensitive`. They go through a bounded renderer of its own (sorted, `strconv.Quote`d attribute names, depth cap 100, one builder, never panics). Overrides and conflicting duplicates make a fact unknown. A `.tf.json` type string reaches typeexpr only after it passes the 64 KiB cap, the bracket scan and the new expression-mode `CheckNativeExprDepth`. Every sec repro (#245, #260, #272, #280) is refused with typeexpr never called, in milliseconds. typeexpr is confined to tfsurface by a new architecture rule. Nothing observable changed: `compare-ref.sh v0.3.0` is 244/244 and cmd/gruntled/testdata is unchanged against 7d5cf0e.**

## Performance

- Duration: ~75 min
- Tasks: 3
- Files: 5 created, 6 modified

## Accomplishments

### Task 1: renderer, guards, fuzz
- **hclconv/limits.go:** `CheckNativeDepth` (:171) is now `checkNativeDepth(src, true)`. The new `CheckNativeExprDepth` (:185) is the same scanner with the root frame's newlines off (:192), including the 15-00 `ctl` counter. Its doc comment states the config/expression difference and the sec #272 repro.
- **typesig.go:**
  - The constants are `maxTypeDepth` (:18), `maxJSONTypeBytes` = 64 KiB (:23) and `maxJSONTypeNesting` = 202 (:28).
  - `typeSig` (:41) and `writeType` (:49) render `any`/`string`/`number`/`bool`, `list`/`set`/`map(T)`, `tuple([..])` and `object({"a"=T,"b"=optional(T)})`, with names sorted and `strconv.Quote`d. A capsule, an unknown kind, or depth over 100 gives `("", false)`.
  - `typeAttrSig` (:134) runs the JSON guards in order: cap (:137), bracket scan (:140), CheckNativeExprDepth (:143). It calls `typeConstraint` (:32, typeexpr) only after all three pass, and it has no recover.
  - `jsonTypeString` (:158) decodes `src[expr.Range()]` with encoding/json and never calls `expr.Value`. `jsonTypeNesting` (:177) is the linear bracket scan.
  - `safeTypeAttrSig` (:204) holds the narrow recover (:206). Its doc comment says it cannot catch a stack overflow (sec #262a). `parseTypeFact` (:193) is the seam the reader uses.
- **ci.yml:**
  - The new step `fuzz FuzzTypeSig` (:50) sits in the `check` job and has `timeout-minutes: 5` (:54).
  - It runs `go test -run '^$' -fuzz '^FuzzTypeSig$' -fuzztime 30s ./internal/infrastructure/tfsurface/` and runs `ls -R` on testdata/fuzz on failure.
  - It adds no secrets and no permissions, and never commits.

### Task 2: facts
- **facts.go:**
  - `factSchema` (:15) reads `type`/`default`/`sensitive` only.
  - `factSet.add` (:49): a PartialContent error makes that block's facts unknown, while the name and the surface stay known.
  - `blockFacts` (:73) reads required from the presence of `default`, the type through `parseTypeFact` (`any` when absent for a variable, `""` for an output), and sensitive through `hclconv.LiteralBool` (false when absent).
  - `merge` (:104): any override makes every fact unknown (:107); otherwise each fact needs agreement.
  - `isOverrideFile` is at :142.
- **reader.go:** blocks feed `facts.add` (:193) only after the size cap, the depth pre-scan and an error-free parse. `NewSurfaceFacts` (:211) replaces `NewSurface(setKeys...)`. The names are the same union of labels.

### Task 3: architecture
- **check-architecture.sh:**
  - Step 6b `typeexpr-only-in-tfsurface` (:395) uses go list over prod and test imports, plus the archscan source scan filtered to files directly in tfsurface.
  - Its non-vacuous guard (:432) requires tfsurface's production imports to include typeexpr.
- **test-check-architecture.sh:** five new probes:
  - typeexpr-in-terragrunt (:529)
  - typeexpr-in-terragrunt-test (:538)
  - typeexpr-windows-file (:547)
  - typeexpr-in-tfsurface-subpackage (:557)
  - typeexpr-guard-vacuous (:572): a shim package takes over the import, and the message asserts the guard fired.

### Tests
- **typesig_test.go:**
  - TestTypeSigGolden (:107) covers the 15-RESEARCH s1 table, native and .tf.json, including the quoted-key and `\u202e`-key rejections.
  - TestTypeSigCapsuleAndDepth (:181).
  - TestTypeSigEqualsProperty (:304): 2,000 xorshift-generated pairs. Equal signatures hold exactly when `Equals` holds, rebuilt and permuted objects render the same, and the pairs are non-vacuous (at least 100 equal and at least 100 distinct).
  - TestTypeSigLinear (:334): a 100,000-attribute object renders in 38 ms.
  - TestTypeSigJSONDeepString (:396): 2,000 and 400,000 levels, the `${}` wrapper, `(`-escaped parens, 500 levels (only the bracket scan stops this one), and 50 levels, which parse.
  - TestTypeSigJSONNewlineTernary (:438): the 690,000 repro, 2,000 newline ternaries (passes the config-mode scan and is stopped by the expression-mode one), a 64 KiB + 1 object (refused) and a 60 KiB object (parses).
  - TestTypeSigJSONLowBracketBombs (:489): `!`, `-`, `a?b:` and nested-if-default, each at a big size (cap) and just under 64 KiB (CheckNativeExprDepth), plus the sec #280b heredoc + nested `%{if}` case.
  - TestTypeSigNativeNestedIfDefault (:508), TestJSONTypeStringExtraction (:526; `(`, `\"`, `\\`, `\/`, `[`, compared with hcl/json's own value), TestTypeSigNativeDeepBounded (:556), and FuzzTypeSig (:595; unwrapped, native and JSON).
- **facts_test.go:**
  - TestReadSurfaceFacts (:57) includes JSON `"sensitive": "${` + 5,000 parens + `}"`, which is Unknown with no crash (sec #280d).
  - TestIsOverrideFile (:114), TestReadSurfaceOverride (:127), TestReadSurfaceDuplicates (:152).
  - TestReadSurfaceMetamorphic (:209): reflow, comments, attribute reorder, an optional() default change, a variable moved to another file, .tf.json, and deprecated/nullable/validation/ephemeral/const/description.
  - TestReadSurfacePanickingParser (:305): a panic inside the typeexpr seam travels the production recover; names are kept and types are unknown.
  - TestSafeTypeAttrSigRecovers (:323).
  - TestReadSurfaceJSONDepthBomb (:336): every bomb through ReadSurface. The name is kept, the type is unknown, and typeexpr is called once, only for the benign sibling variable. Each runs in 8-130 ms.
- **cmd/gruntled/type_bomb_test.go:** TestCheckJSONTypeBomb (:16): `check` (text and json) on a repo whose module has the 400,000-level .tf.json gives the same exit code 1, the same GRT001 and byte-identical output as the plain repo, in 67 ms.
- **hclconv/limits_test.go:** `depthCheckers` (:650) runs every existing reject table (Rejects, TemplateControl rejects, deep Ternary, deep Chain cases) against both scanners. TestCheckNativeExprDepth (:663) pins the mode difference, the realistic multi-line type and a heredoc with nested `%{if}`.

### Mutation spot-checks (each reverted)
- Typesig mutants, all caught:
  - the 64 KiB cap ×1000
  - the bracket cap ×100000 (caught once the 500-level case was added; it survived before)
  - CheckNativeExprDepth → CheckNativeDepth
  - the renderer depth ×10
  - object names unsorted
- Fact-rule mutants, all caught:
  - override ignored
  - type conflict keeps the last value
  - required conflict keeps the last value
  - an absent variable type becomes ""
  - `_override` suffix ignored

## Task Commits

1. `dd931e7` test(15-02): CheckNativeExprDepth refuses the newline ternary chain config mode releases
2. `5d40d5d` feat(15-02): CheckNativeExprDepth, the depth scan in expression mode
3. `80a055e` test(15-02): canonical type renderer, JSON type-string guards, fuzz target
4. `cd0958c` feat(15-02): canonical type renderer with bounded JSON type-string guards, CI fuzz step (first commit linking typeexpr)
5. `efc0896` test(15-02): pin the JSON bracket scan with a 500-level type string
6. `9e5884f` test(15-02): surface facts, overrides, duplicates, metamorphic, panic and JSON bomb cases
7. `b21c777` feat(15-02): read variable and output type-level facts in the surface reader
8. `bd5c492` test(15-02): architecture probes for typeexpr outside tfsurface and the vacuous guard
9. `730b58d` feat(15-02): typeexpr-only-in-tfsurface architecture rule with a non-vacuous guard
10. `29eb95d` docs(15-02): reflow the tfsurface package comment

This summary is its own `docs(15-02): summary` commit.

## Verification

```
ok  	github.com/GiulioSavini/gruntled/cmd/gruntled	21.388s
ok  	github.com/GiulioSavini/gruntled/internal/application/blasting	0.481s
ok  	github.com/GiulioSavini/gruntled/internal/application/checking	0.002s
ok  	github.com/GiulioSavini/gruntled/internal/application/indexing	0.006s
?   	github.com/GiulioSavini/gruntled/internal/application/ports	[no test files]
ok  	github.com/GiulioSavini/gruntled/internal/application/watching	0.005s
ok  	github.com/GiulioSavini/gruntled/internal/domain/analysis	0.179s
ok  	github.com/GiulioSavini/gruntled/internal/domain/diagnostic	0.005s
ok  	github.com/GiulioSavini/gruntled/internal/domain/impact	1.446s
ok  	github.com/GiulioSavini/gruntled/internal/domain/repograph	0.042s
ok  	github.com/GiulioSavini/gruntled/internal/infrastructure/hclconv	9.386s
ok  	github.com/GiulioSavini/gruntled/internal/infrastructure/ipc	0.058s
ok  	github.com/GiulioSavini/gruntled/internal/infrastructure/sourceresolve	0.033s
ok  	github.com/GiulioSavini/gruntled/internal/infrastructure/statusfile	0.086s
ok  	github.com/GiulioSavini/gruntled/internal/infrastructure/terragrunt	6.711s
ok  	github.com/GiulioSavini/gruntled/internal/infrastructure/tfsurface	10.729s
ok  	github.com/GiulioSavini/gruntled/internal/infrastructure/watch	1.221s
ok  	github.com/GiulioSavini/gruntled/internal/interfaces/presenter	0.049s
ok  	github.com/GiulioSavini/gruntled/internal/testsupport/synthrepo	0.052s
ok  	github.com/GiulioSavini/gruntled/scripts/archscan	0.057s
```

`go vet ./...` is clean, `gofmt -l` prints nothing, and `go mod tidy -diff` is clean.

Fuzz, 20 s locally (the CI step runs 30 s):

```
go test -run '^$' -fuzz '^FuzzTypeSig$' -fuzztime 20s ./internal/infrastructure/tfsurface/
fuzz: elapsed: 21s, execs: 66468 (0/sec), new interesting: 152 (total: 171)
PASS
```

Byte identity:

```
bash scripts/compare-ref.sh v0.3.0
244/244 identical
git diff 7d5cf0e -- cmd/gruntled/testdata     # empty (0 lines)
```

### Six-target proof (first typeexpr import, cd0958c)

`bash scripts/check-architecture.sh` (Step 9 covers all six targets, including the linker symbol proof) prints:

```
architecture: OK (4 domain packages, 5 application packages, 1 interfaces packages)
```

Per-target `go list -deps ./cmd/gruntled`, counting `net`, `net/*`, `os/exec`, `plugin` and `crypto/tls`, and whether typeexpr is linked:

```
linux/amd64 forbidden=0 typeexpr=1
linux/arm64 forbidden=0 typeexpr=1
darwin/amd64 forbidden=0 typeexpr=1
darwin/arm64 forbidden=0 typeexpr=1
windows/amd64 forbidden=0 typeexpr=1
windows/arm64 forbidden=0 typeexpr=1
```

Delta of `go list -deps ./cmd/gruntled` against 7d5cf0e, taken from a temporary worktree that was then removed:

```
> github.com/hashicorp/hcl/v2/ext/typeexpr
```

`git diff --exit-code go.mod go.sum` is clean.

### Architecture self-tests

The full `bash scripts/test-check-architecture.sh` ends with `all architecture self-tests passed`, including the five new probes:

```
PASS typeexpr-in-terragrunt
PASS typeexpr-in-terragrunt-test
PASS typeexpr-windows-file
PASS typeexpr-in-tfsurface-subpackage
PASS typeexpr-guard-vacuous
```

`-race` was not run locally (no gcc). staticcheck was not run locally either, because it is not in the module cache and there is no network. CI runs both.

## Threat Mitigations

| ID | Disposition | Where |
|----|-------------|-------|
| T-15-02-1 | mitigated | native .tf: whole-file CheckNativeDepth (with the 15-00 ctl counter) before parse, reader.go; renderer depth cap typesig.go:18/:49 and one builder; TestTypeSigCapsuleAndDepth :181, TestTypeSigNativeDeepBounded :556 (999 levels in 0.2 ms, 1,000 refused), TestTypeSigNativeNestedIfDefault :508 (4.05 MB refused, typeexpr 0 calls), TestTypeSigLinear :334, FuzzTypeSig :595 plus the CI step ci.yml:50 |
| T-15-02-10 | mitigated | typesig.go:137 (64 KiB cap), :143 CheckNativeExprDepth (limits.go:185/:192, root newlines off); TestCheckNativeExprDepth limits_test.go:663, TestTypeSigJSONNewlineTernary :438 |
| T-15-02-8 | mitigated | typesig.go:158 (raw string via encoding/json, never expr.Value), :137/:140/:143 guards before :32 typeexpr; TestTypeSigJSONDeepString :396, TestTypeSigJSONLowBracketBombs :489, TestJSONTypeStringExtraction :526, TestReadSurfaceJSONDepthBomb facts_test.go:336, TestCheckJSONTypeBomb type_bomb_test.go:16, fuzz seeds; the recover is not counted on (doc comment typesig.go:195-203) |
| T-15-02-9 | mitigated | safeTypeAttrSig typesig.go:204/:206; TestReadSurfacePanickingParser facts_test.go:305, TestSafeTypeAttrSigRecovers :323; FuzzTypeSig calls typeAttrSig unwrapped |
| T-15-02-2 | mitigated | typeexpr.TypeString never used; capsule or unknown kind gives ("", false) at typesig.go:107; fuzz |
| T-15-02-3 | mitigated | canonical from cty.Type (typesig.go:41); TestTypeSigEqualsProperty :304; TestReadSurfaceMetamorphic facts_test.go:209 |
| T-15-02-4 | mitigated | facts.go:104/:107 merge; TestReadSurfaceOverride :127, TestReadSurfaceDuplicates :152 |
| T-15-02-5 | accepted | typeexpr gets no EvalContext: `optional(string, var.x)` and `upper("x")` defaults are errors, so the fact is unknown (TestTypeSigGolden), and defaults are discarded |
| T-15-02-6 | mitigated | Step 9 six targets on cd0958c (output above); Step 6b check-architecture.sh:395 and guard :432; five probes test-check-architecture.sh:529-572 |
| T-15-02-7 | mitigated | names come from the same labelled blocks (reader.go:193, facts.go:125); compare-ref 244/244; existing reader tests unchanged and green |

## Deviations from Plan

**1. [Test placement] The cmd-level smoke is a Go test, not check_json_type_bomb.txtar**
- The plan lists `cmd/gruntled/testdata/script/check_json_type_bomb.txtar`, and its verify runs `TestScripts/check_json_type_bomb`.
- The hostile file is 2.4 MB, testscript has no generator command, and the orchestrator required an empty `cmd/gruntled/testdata` diff.
- `cmd/gruntled/type_bomb_test.go` builds the repo in a temp dir and asserts the same exit code and byte-identical text and JSON output against the plain repo.

**2. [Sizes] Nested-if-default bomb at 250,000 (JSON) and 270,000 (native), not 300,000**
- 300,000 levels of `%{if a}…%{endif}` is 4.5 MB, over MaxFileBytes, so the file would be refused as too large and the type-string guards would never be reached. The sizes used stay under 4 MiB.

**3. [Extra test] 500-level bracket case (efc0896)**
- The bracket-scan mutant survived the planned cases, because CheckNativeExprDepth also refuses at 1,000 levels. A 500-level string is refused only by the bracket scan, which pins it.

**4. [Panicking-parser seam] The panic is injected at typeexpr, not by replacing parseTypeFact**
- Replacing parseTypeFact itself would bypass the recover it carries. The test makes the inner typeexpr seam panic, so the production path reader → safeTypeAttrSig → recover is the one exercised.

**5. [Rule scope] Only the tfsurface package itself is exempt**
- The source-scan half filters by file path, not by archscan's -exclude, which is a directory prefix and would also have exempted subpackages. A probe covers the subpackage case.

## Known Risks / Unverified

- `-race` and staticcheck were not run locally (no gcc; staticcheck is not cached and there is no network). CI runs both, including the new fuzz step.
- The recover cannot stop a stack overflow. Only the pre-scans do, as stated in the doc comment.

## Self-Check: PASSED
