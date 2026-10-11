---
phase: 15-type-level-surface-facts
plan: 01
subsystem: repograph, hclconv
tags: [domain-model, refactor, byte-identity, type-facts]
requires: []
provides:
  - "repograph.VariableDecl, OutputDecl, NewSurfaceFacts, Surface.Variable, Surface.Output"
  - "hclconv.LiteralBool, the single literal-bool reader"
affects: [15-02, 15-03]
tech-stack:
  added: []
  patterns: ["optional per-name facts beside the unchanged name slices; zero fact means unknown"]
key-files:
  created:
    - internal/infrastructure/hclconv/literal.go
    - internal/infrastructure/hclconv/literal_test.go
  modified:
    - internal/domain/repograph/surface.go
    - internal/domain/repograph/surface_test.go
    - internal/infrastructure/terragrunt/eval.go
    - internal/infrastructure/terragrunt/depfacts.go
    - internal/infrastructure/terragrunt/pathfuncs_test.go
key-decisions:
  - "Surface keeps its name slices as the contract for GRT001 and name-level Impacted; the decls are an extra, sorted by name, read by binary search"
  - "NewSurface builds zero-fact decls (TristateUnknown, empty type), so a name-only surface never claims a type-level fact"
  - "literalBool moved verbatim to hclconv.LiteralBool; only the doc comment gained the words 'such as \"true\"' and the sentence naming its two callers"
requirements-completed: []
duration: 20min
completed: 2026-10-11
---

# Phase 15 Plan 01: Surface Declaration Facts Summary

**repograph.Surface now carries per-name declaration facts (`VariableDecl{Name, Required, Type}`, `OutputDecl{Name, Type, Sensitive}`) next to the unchanged name slices, and the Terragrunt loader's literal-bool reader moved to `hclconv.LiteralBool` so the surface reader can share it in 15-02. Nothing observable changed: the full suite is green, `compare-ref.sh v0.3.0` reports 244/244 identical, and no file under cmd/gruntled/testdata differs from the pre-plan base.**

## Performance

- Duration: ~20 min (over two sessions; paused at aa6dc6f)
- Tasks: 2
- Files: 2 created, 5 modified

## Accomplishments

- **surface.go:**
  - `VariableDecl` (:30) and `OutputDecl` (:39) with doc comments matching the plan interface: `Type` "" means unknown, `Required`/`Sensitive` use Tristate.
  - `NewSurface` (:49) is unchanged in signature and validation; it now also builds zero-fact decls from the sorted names (:58-65).
  - `NewSurfaceFacts` (:72) takes the names from the decls, validates them through the same `sortedUniqueNames` (same error messages), and stores cloned decls sorted by name.
  - `Variable` (:133) and `Output` (:143) return a copy of the decl by binary search; ok is false for an undeclared name.
  - `Variables()`, `Outputs()`, `HasVariable`, `HasOutput` (:110-131) are untouched.
- **surface_test.go:** TestNewSurfaceFacts (:11), TestNewSurfaceFactsUnknownByDefault (:62), TestSurfaceViewsUnchanged (:89; table plus 2,000 cases from an in-test xorshift, :79, no rapid or math/rand), TestSurfaceFactsAreCopies (:134).
- **hclconv/literal.go:** `LiteralBool` (:15), the body of the old `terragrunt.literalBool` unchanged.
- **terragrunt:** `literalBool` deleted from eval.go along with its now-unused repograph import; the three depfacts.go calls use `hclconv.LiteralBool` (enabled :23, skip_outputs :26, mock merge :92). No `literalBool` remains in the tree.
- **hclconv/literal_test.go:** TestLiteralBool (:13), the table moved from pathfuncs_test.go and extended: native `true`/`false`, and Unknown for `"true"`, `var.x`, `local.y`, `local.x`, `null`, `1`, `upper("x")`; JSON `true`/`false` from a .tf.json body, and Unknown for JSON `"true"`.

### Red evidence

- 208cfc8: the surface tests referenced `NewSurfaceFacts`, `VariableDecl`, `OutputDecl`, `Surface.Variable` and `Surface.Output`, none of which existed, so the package did not compile.
- c6cb2d1: TestLiteralBool referenced `hclconv.LiteralBool` before aa6dc6f added it, so the hclconv tests did not compile.

## Task Commits

1. `208cfc8` test(15-01): per-name declaration facts on Surface with unchanged name views
2. `8a5611a` feat(15-01): per-name variable and output declaration facts on Surface
3. `c6cb2d1` test(15-01): LiteralBool table moves to hclconv, native and JSON
4. `aa6dc6f` refactor(15-01): literalBool moves to hclconv.LiteralBool

This summary is its own `docs(15-01): summary` commit.

## Verification

```
ok  	github.com/GiulioSavini/gruntled/cmd/gruntled	13.760s
ok  	github.com/GiulioSavini/gruntled/internal/application/blasting	0.536s
ok  	github.com/GiulioSavini/gruntled/internal/application/checking	0.003s
ok  	github.com/GiulioSavini/gruntled/internal/application/indexing	0.003s
?   	github.com/GiulioSavini/gruntled/internal/application/ports	[no test files]
ok  	github.com/GiulioSavini/gruntled/internal/application/watching	0.004s
ok  	github.com/GiulioSavini/gruntled/internal/domain/analysis	0.205s
ok  	github.com/GiulioSavini/gruntled/internal/domain/diagnostic	0.003s
ok  	github.com/GiulioSavini/gruntled/internal/domain/impact	1.171s
ok  	github.com/GiulioSavini/gruntled/internal/domain/repograph	0.025s
ok  	github.com/GiulioSavini/gruntled/internal/infrastructure/hclconv	2.945s
ok  	github.com/GiulioSavini/gruntled/internal/infrastructure/ipc	0.060s
ok  	github.com/GiulioSavini/gruntled/internal/infrastructure/sourceresolve	0.002s
ok  	github.com/GiulioSavini/gruntled/internal/infrastructure/statusfile	0.092s
ok  	github.com/GiulioSavini/gruntled/internal/infrastructure/terragrunt	3.792s
ok  	github.com/GiulioSavini/gruntled/internal/infrastructure/tfsurface	2.772s
ok  	github.com/GiulioSavini/gruntled/internal/infrastructure/watch	1.239s
ok  	github.com/GiulioSavini/gruntled/internal/interfaces/presenter	0.061s
ok  	github.com/GiulioSavini/gruntled/internal/testsupport/synthrepo	0.057s
ok  	github.com/GiulioSavini/gruntled/scripts/archscan	0.035s
architecture: OK (4 domain packages, 5 application packages, 1 interfaces packages)
```

`go vet ./...` is clean.

Byte-identity gate (Pitfall 7):

```
bash scripts/compare-ref.sh v0.3.0
...
same dir/sarif-fixture check --format sarif (exit 1)
same dir/sarif-fixture graph --json (exit 0)
244/244 identical
```

Golden check. The base is a9dcd4a, the pause commit; 208cfc8~1 (daa575f, the commit before this plan) gives the same result:

```
git diff a9dcd4a -- cmd/gruntled/testdata     # empty
git diff 208cfc8~1 -- cmd/gruntled/testdata   # empty
```

`-race` was not run locally: there is no gcc and no cgo. CI runs it.

## Threat Mitigations

| ID | Disposition | Where |
|----|-------------|-------|
| T-15-01-1 | mitigated | surface.go:110-131 name views untouched; TestSurfaceViewsUnchanged (surface_test.go:89); compare-ref.sh v0.3.0 244/244; empty testdata diff against a9dcd4a and daa575f |
| T-15-01-2 | mitigated | hclconv/literal.go:15 is the old body verbatim; depfacts.go:23/:26/:92; TestLiteralBool (literal_test.go:13) extended; terragrunt tests and DIAG-03 goldens unchanged and green |
| T-15-01-3 | mitigated | surface.go:58-65 zero-fact decls (TristateUnknown, ""); TestNewSurfaceFactsUnknownByDefault (surface_test.go:62) |

## Deviations from Plan

**1. [Commit type] Task 2 implementation is `refactor(15-01)`**
- The move adds no behaviour, so the commit is `refactor`, not `feat`.

**2. [Doc comment] LiteralBool's comment is slightly wider than the old one**
- It adds the words "such as \"true\"" and a sentence naming the two callers (the Terragrunt loader and the module surface reader). The code is the old body unchanged.

**3. [Resume] Task 2 was already complete at the pause**
- The handoff said the previous executor stopped "about to touch surface.go". On resume, aa6dc6f already held the full move and all three call sites, and Task 2 lists no surface.go change, so no code commit was needed after the pause; only the gate and this summary.

## Known Risks / Unverified

- `-race` not run locally (no gcc).

## Self-Check: PASSED
