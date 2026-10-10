---
phase: 15-type-level-surface-facts
type: research
created: 2026-10-10
sources:
  - .planning/research/SUMMARY.md (Reconciled Conflicts #1 output type, #4 type-level changes; binding)
  - .planning/research/STACK.md (typeexpr, canonical walker), PITFALLS.md (4-7, 20, 21)
  - .planning/REQUIREMENTS.md (Terms, BLAST-06..11, SEC-01)
  - agent bus #104 (BLAST-10), #105 (BLAST-11), #106 (SEC-01), #114 (literal rules), #115 (renderer bound)
  - experiments in the session scratchpad (p15/main.go, p15/e2/main.go; not in the repo), hcl v2.25.0, go-cty v1.19.0, Go 1.27.2
  - Terraform output block reference and 1.15 release notes; OpenTofu output values docs
---

# Phase 15 — Research

Research-flagged phase. Everything marked **[exp]** was observed by running throwaway Go against the
repo's pinned modules (`GOFLAGS=-mod=mod GOPROXY=off`, module cache only), at HEAD `abcb39e`.

## 1. `typeexpr.TypeConstraintWithDefaults` on hcl v2.25.0 [exp]

Input read as `variable`/`output` block → `b.Body.PartialContent({type, default, sensitive})` →
`TypeConstraintWithDefaults(attrs["type"].Expr)`. Same path for `.tf` (hclsyntax) and `.tf.json`
(hcl/json string expression).

| Input (native `.tf` unless noted) | Result |
|---|---|
| `string`, `list(string)`, `tuple([string,number])`, `list(any)`, `any` | parses |
| `object({ y = number, x = optional(string, "d") })` | parses; optional default discarded by us |
| same with comments, line breaks, reordered attributes | same `cty.Type` |
| `set(map(list(object({é = string, ж = number}))))` (non-ASCII identifiers) | parses |
| bare `list`, bare `map` | error "constructor requires one argument" |
| legacy quoted `"string"` | error "A type specification is either a primitive type keyword..." |
| `optional(string)` at top level | error "valid only as a modifier for object type attributes" |
| `optional(string, var.x)` / `optional(string, upper("x"))` | error "Variables may not be used here" / "Functions may not be called here" |
| typo `strng` | error |
| object attribute key quoted (`{"q z" = bool}`, `{"\u202e" = string}`) | error "Object constructor map keys must be attribute names" |
| `.tf.json` `"list(string)"`, `" list( string ) "`, `"object({y=number, x=optional(string, \"d\")})"` | parses, same types as native |
| `.tf.json` `"list"`, `"${list(string)}"`, `"${string}"`, array form `["list","string"]` | error |
| `.tf.json` `"any"` | parses (`any`) |

Consequences:
- Errors are always diagnostics (never panics) on these inputs; any error → the fact is **unknown**.
- **Object attribute names are HCL identifiers only** (Unicode letters/digits, `_`, `-`); C0, C1,
  U+202E (Cf) and quoted keys are rejected by typeexpr itself, in both syntaxes. So SEC-01's
  "object attribute name containing C0, C1, U+202E" can never reach the renderer: such a type is
  unparseable → unknown → silent. The SEC-01 test asserts exactly that, and the renderer still
  writes every attribute name with `strconv.Quote` (non-ASCII letters stay readable).
- `typeexpr.TypeString` is lossy: `object({x=optional(string),y=number})` and
  `object({x=string,y=number})` both print `object({x=string,y=number})`; it **panics** on a
  capsule type. Do not use it.
- Cost [exp]: a 4 MiB `.tf` with one `object({...})` of 110,373 `optional(list(string))` attributes:
  `hclsyntax.ParseConfig` 0.88 s, typeexpr 0.10 s. A `list(` nested 900 deep (under the existing
  `MaxNestingDepth` 1000 pre-scan): typeexpr 0.16 ms, no stack problem.

## 2. Canonical renderer (infrastructure, `tfsurface/typesig.go`)

Draft verified [exp]:
- `any`, `string`, `number`, `bool`; `list(T)`, `set(T)`, `map(T)`; `tuple([T1,T2])` positional;
  `object({"a"=T,"b"=optional(T)})` attribute names sorted, each `strconv.Quote`d, optional marked,
  defaults ignored. Built from the `cty.Type`, so whitespace, comments, attribute order and
  `.tf`↔`.tf.json` spelling normalise by construction.
- Draft outputs: `object({"x"=optional(string),"y"=number})` for both the `optional(string,"d")` and
  `optional(string)` forms; `object({"x"=string,"y"=number})` for the non-optional one (distinct).
- Capsule or any unrecognised kind → `("", false)` (unknown), never a panic.
- Bound: write into one `strings.Builder` passed down (linear output, no per-level string copies),
  depth cap `maxTypeDepth = 100` (deeper → unknown; real types nest < 10, the pre-scan already caps
  source nesting at 1000). Iteration over object attributes uses a sorted name slice (no map order).
- Pin: golden table; permutation property (object attribute order); `canon(a)==canon(b)` iff
  `a.Equals(b)` over generated `cty.Type`s (test-only cty use in infrastructure tests); `FuzzTypeSig`
  over source text → parse → render: never panics, output valid UTF-8, deterministic. CI gets a
  `-fuzz FuzzTypeSig -fuzztime 30s` step (today CI runs only fuzz seed corpora).

## 3. Terraform output `type` and OpenTofu parity

- Terraform: the output block reference lists optional `type` ("constrains the type of value that
  you can assign to that output"); 1.15.0 (released 2026-04-29) changelog: "output blocks now can
  have an explicit type constraints" (PR #36411). 1.15 also adds output/variable `deprecated` and a
  variable `const` attribute.
- OpenTofu: the "Output Values" docs list `value`, `description`, `sensitive`, `ephemeral`,
  `depends_on`, `deprecated`, `precondition`; **no `type`**.
- Design unchanged either way (reconciliation #1): compare output types only when both sides declare
  a parseable `type`; absent→present or either side unparseable is silent; absent is never `any`.
  `const`, `deprecated`, `nullable`, `validation`, `ephemeral` never count (BLAST-06 list + `const`).

## 4. Override files and duplicates (BLAST-10)

- Terraform override files: base name (after stripping `.tf`, `.tf.json`, `.tofu`, `.tofu.json`) is
  `override` or ends in `_override`. Terraform merges their blocks into the primary declaration;
  gruntled does not model the merge.
- Rule (D-15-03): every type-level fact of a name that appears in **any** override file is unknown.
  A name declared in more than one kept non-override file (including the `.tf`/`.tofu` union) keeps
  a fact only if every declaration agrees on it; otherwise that fact is unknown. Names themselves
  are unchanged (still the union), so `Variables()/Outputs()`, GRT001 and name-level Impacted do not
  move.

## 5. Facts and where they live

| Fact | Source | Known values | Unknown when |
|------|--------|--------------|--------------|
| variable required | `default` attribute presence (never its value) | True (absent), False (present, incl. `default = null`, JSON `"default": null`) | conflict, override |
| variable type | `type` attr → canonical; absent → `any` | canonical string | unparseable, over depth cap, conflict, override |
| output type | `type` attr → canonical; absent → not declared | canonical string | not declared, unparseable, conflict, override |
| output sensitive | `sensitive` via `hclconv.LiteralBool`; absent → False | True/False | non-literal (`var.x`), non-bool (`"true"`) [exp], conflict, override |

- `literalBool` moves from `internal/infrastructure/terragrunt/eval.go:71-82` to
  `hclconv.LiteralBool` (exported, same body; terragrunt `depfacts.go:22,25,91` call it); the
  existing `pathfuncs_test.go:367` table moves with it.
- Domain (`internal/domain/repograph/surface.go`): `Surface` keeps `variables []string`,
  `outputs []string` and gains sorted decl slices `[]VariableDecl{Name, Required Tristate, Type string}`
  and `[]OutputDecl{Name, Type string, Sensitive Tristate}` (`""` = unknown). `NewSurface(names...)`
  stays and yields all-unknown facts; new `NewSurfaceFacts(vars, outs)`; accessors
  `Variable(name)`, `Output(name)`. `Variables()/Outputs()` unchanged, so `graph --json`
  (`presenter/graph.go:141` reads names only), SARIF, check and the status file are byte-identical.
  `Surface` is already non-comparable (slices), so no `==` use breaks.
- `ports.SurfaceResult` unchanged (it carries a `repograph.Surface`); `cty` never leaves
  `tfsurface` (STACK rule; `scripts/check-architecture.sh` Step 6 already blocks it outside
  infrastructure; add a step that only `internal/infrastructure/tfsurface` imports `ext/typeexpr`).

## 6. Impact and output shape

- `impact.SurfaceChange` (surface.go:9-25) gains `TypeChanges []TypeChange{Kind, Name, Old, New}`
  sorted by kind order (variable_required, variable_type, output_type, output_sensitive) then name.
  Rules: new variable with Required True → `variable_required` (old `absent`, new `required`);
  variable present both sides, base Required False, cur True → (`optional` → `required`); variable
  type both known and different; output type both declared, known and different; sensitive both
  known and different (`false`/`true`). Removed names produce no type change (name-level already).
  `Empty()` covers names and type changes; `SurfaceDiff` keeps only non-empty changes, so a
  type-only change makes the module a seed (Terms) and propagates like any change (Phase 14).
- No new `Code`, nothing reaches Broken or `HasErrors` (BLAST-09; Pitfall 4).
- JSON (`presenter/blast.go:71-77` `blastChange`): add `type_changes: [{kind, name, old, new}]`,
  always `[]` when none. Additive on version 2, which is unreleased (v0.4 not tagged): **no bump**.
  Impacted entries are unchanged (type strings only in `changes[]`, once per module — BLAST-11).
- Text: distance-1 lines get short tokens after the name tokens, without type strings:
  `required variable x`, `type variable x`, `type output y`, `sensitive output z`. The old → new
  strings print once per module in a new section after Impacted, printed only when non-empty:
  `Type changes (N):` then `  <module> variable <name>: type <old> -> <new>` /
  `: now required` / `output <name>: type <old> -> <new>` / `: sensitive <old> -> <new>`; every
  string through `escapeTerm`.
- **Golden impact:** `blast_impacted.txtar` already adds `variable "name" {}` (no default), so its
  distance-1 lines gain `required variable name` and a `Type changes (1):` section. Phase 14's
  `TestBlastDepth1MatchesV1` normaliser (`blast_depth_test.go:213`, strips `distance 1, `) must also
  strip these tokens and the section; Broken/Impacted sets and exit codes stay equal (D-15-05).

## 7. Size and security (BLAST-11, SEC-01)

- Output is O(units + edges + surface text): per-unit text tokens carry names only (as v0.3); type
  strings once per module in text and JSON. A test generates a 5,000-unit linear chain whose seed
  module has a ~4 MiB variable type that changes, runs the built binary (`benchBuildGruntled`
  pattern, `cmd/gruntled/bench_test.go:76`) and asserts stdout bytes < 2 × (input type bytes) +
  5,000 × per-line bound and peak RSS (`ProcessState.SysUsage()` `Maxrss`, linux/darwin) under a
  bound set at 2× the measured value recorded in the SUMMARY.
- Hostile names [exp]: native labels accept `\u` escapes (`variable "a\u001bb"` → label with ESC),
  JSON keys accept `\u001b\u202e`; invalid UTF-8 in a native file is a GRT100 parse error, in JSON it
  becomes U+FFFD. So hostile variable/output names do reach type-change tokens and the
  `Type changes` section: all through `escapeTerm` / `escapeJSON`. Extend
  `cmd/gruntled/escape_test.go` (phase 12/13/14) with a module whose variable and output names hold
  C0, C1, U+202E and whose object attribute name attempt is rejected (type unknown, no type change).

## 8. Six-target no-net/no-exec [exp]

`go list -deps` of a main importing `hcl/v2/ext/typeexpr`, for linux/darwin/windows × amd64/arm64:
0 of `net`, `net/*`, `os/exec`, `plugin`, `crypto/tls`. Delta against `go list -deps ./cmd/gruntled`:
exactly `github.com/hashicorp/hcl/v2/ext/typeexpr`. `go.mod`/`go.sum` unchanged. The plan that first
imports it runs `bash scripts/check-architecture.sh` (Step 9 checks all six targets, including the
linker symbol proof).

## 9. Decisions (to confirm)

| ID | Decision | Why |
|----|----------|-----|
| D-15-01 | Domain stores canonical type strings (`""` = unknown); `cty`/`typeexpr` only in `tfsurface`, enforced by a new architecture step. | STACK; keeps domain pure and comparable with `==`. |
| D-15-02 | Renderer depth cap 100 with a single passed-down builder; capsule/unknown kind → unknown. | BLAST-07 "iterative or depth-capped"; linear output. |
| D-15-03 | Any override file declaring a name → all that name's type facts unknown (not only "with differing facts"); duplicates in non-override files → per-fact agreement or unknown. | Terraform merges overrides; modelling the merge is out of scope; silence never creates a false report. Reads BLAST-10 conservatively. |
| D-15-04 | Variable `T -> any` is reported (equality only, no widening judgement). | SUMMARY gap "decided: equality-only". |
| D-15-05 | BLAST-09 text clause: with `--depth 1` text differs from v0.3 by the distance token AND by type-level reason tokens / `Type changes` section on goldens whose modules have type-level changes (blast_impacted adds a required variable). Sets and exit codes are identical. REQUIREMENTS BLAST-09 / ROADMAP criterion 3 wording should say so. | A requirement-level wording fix; otherwise BLAST-06 and BLAST-09 contradict on the existing golden. |
| D-15-06 | JSON stays `"version": 2`; `changes[].type_changes` additive; Impacted entries unchanged. | v2 unreleased; additive anyway. |
| D-15-07 | `Type changes` text section printed only when non-empty (unlike Broken/Impacted, which always print their header). | Keeps every existing golden without type changes byte-identical. |
| D-15-08 | SEC-01 object-attribute-name case is proven as "rejected by typeexpr → unknown → silent" rather than "escaped", since such names cannot be parsed. | Observed behaviour; nothing to escape. |
