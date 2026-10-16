# Technology Stack — v0.4 Blast-aware Diagnostics

**Project:** gruntled
**Researched:** 2026-10-10
**Scope:** only what the four v0.4 features need (GRT004, `report --blast` + `rebase`, transitive Impacted with path, type-level surface changes). Existing validated stack (hcl/v2, go-cty, fsnotify, stdlib `flag`, raw-syscall AF_UNIX) is not re-researched.

## Verdict

**No new module dependency. `go.mod` and `go.sum` stay byte-identical.**

The single addition is one *import* from a module already required: `github.com/hashicorp/hcl/v2/ext/typeexpr` (hcl v2.25.0, the current release; `go list -m -u` reports no update for hcl v2.25.0, go-cty v1.19.0 or fsnotify v1.10.1). It is the same package Terraform itself uses to parse `type = ...`. Everything else in v0.4 (GRT004, transitive BFS with paths, baseline snapshot, `rebase`, `report --blast` wire format) is stdlib plus code already in the repo.

## Recommended Stack

### Core Framework (unchanged)
| Technology | Version | Purpose | Why |
|------------|---------|---------|-----|
| Go | 1.27 (toolchain 1.27.2 verified locally) | language | unchanged |
| `github.com/hashicorp/hcl/v2` | v2.25.0 (latest) | structural decode; **new:** `ext/typeexpr` for `variable`/`output` `type` | already required; `typeexpr` is a subpackage of the same module |
| `github.com/zclconf/go-cty` | v1.19.0 (latest) | already a direct dep; **new use:** inspect the `cty.Type` that typeexpr returns, inside infrastructure only | `cty.Type` is the return type of typeexpr; we walk it with `IsListType/IsObjectType/AttributeOptional/...` |

### Database / Infrastructure
None. The baseline stays in the daemon's memory (a pointer to a second `RepositoryGraph`); no on-disk cache (still Out of Scope per PROJECT.md).

### Supporting Libraries
| Library | Version | Purpose | When to Use |
|---------|---------|---------|-------------|
| `github.com/hashicorp/hcl/v2/ext/typeexpr` | v2.25.0 (in-module) | parse a type-constraint expression into `cty.Type` without evaluating anything beyond literals | in `internal/infrastructure/tfsurface` only |
| stdlib `slices`, `sort`, `strings`, `strconv` | Go 1.27 | canonical type string, BFS, sorted output | domain and infrastructure (domain allowlist already admits what existing domain code uses; verify any new stdlib import against the allowlist in `check-architecture.sh`) |
| stdlib `encoding/json` | Go 1.27 | `report --blast` response, `rebase` request | already in the interfaces allowlist and linked |

## Dependency-graph impact (verified with `go list -deps`)

Checked with Go 1.27.2 for all six release targets (linux/darwin/windows x amd64/arm64): `go list -deps github.com/hashicorp/hcl/v2/ext/typeexpr` (and a scratch main importing it, with the repo's `go.mod`/`go.sum`) contains **none** of `net`, `net/*`, `os/exec`, `plugin`, `crypto/tls`. Same result for `go-cty/cty`, `cty/convert`, `hclsyntax`.

Delta against `go list -deps ./cmd/...` (linux/amd64): **exactly one new package, `github.com/hashicorp/hcl/v2/ext/typeexpr` itself.** All of its imports (`hcl/v2`, `hclsyntax`, `ext/customdecode`, `cty`, `cty/convert`, `cty/function`, `bytes`, `fmt`, `reflect`, `sort`, `strconv`) are already linked into the shipped binary. `typeexpr` has no `os`/exec/net calls, so the "no linked non-std file calls `os.StartProcess`/`ForkExec`" half of `binary-no-net-no-exec` is unaffected. (Confidence HIGH: observed, not inferred.)

`scripts/check-architecture.sh` needs **no rule change**: its `hcl-only-in-infrastructure` layering rule already covers `hashicorp/hcl/...` and `zclconf/go-cty/...` by prefix, so a stray `typeexpr` or `cty` import in `internal/domain` or `internal/application` is still caught. Optionally add one regression test that asserts `ext/typeexpr` appears in `go list -deps ./cmd/...` only through `internal/infrastructure/tfsurface`; not required.

## How to compare `type` constraints without evaluating (the actual design)

### Where each piece lives (hexagonal)
- **Infrastructure (`tfsurface`)** parses `type` with `typeexpr`, walks the resulting `cty.Type`, and emits a **canonical string**.
- **Domain (`repograph.Surface` / `impact`)** stores that string as an opaque value (`TypeSig string`, `""` = unknown/not declared) and compares with `==`. The domain never sees `cty.Type`, `hcl.Expression` or `typeexpr`. This keeps ARCH-01 (domain imports no HCL) intact and makes the baseline snapshot a plain value that is trivially comparable, serialisable to JSON and cheap to hold in the daemon.
- The `ports.SurfaceResult` / `repograph.Surface` shape grows from "names only" to per-declaration records, e.g. `Variable{Name, TypeSig, HasDefault}` and `Output{Name, TypeSig, Sensitive Tristate}`; `Surface.Variables()/Outputs()` (names) stays as a derived view so GRT001/GRT002 callers and goldens do not move.

### Read the attributes structurally
Extend `moduleSchema` handling in `reader.go`: for each `variable`/`output` block call `b.Body.PartialContent(&hcl.BodySchema{Attributes: ...})` for `type`, `default`, `sensitive` (use `PartialContent`, not `JustAttributes`, because `validation`/`precondition`/`lifecycle` blocks live in the same body). Works identically for `.tf` and `.tf.json` bodies.
- **Required variable** = the `default` attribute is *absent*. Presence alone, never its value (`default = null` is still not required: confirmed in the Terraform variable-block docs). No evaluation.
- **Variable type** = `typeexpr.TypeConstraintWithDefaults(attr.Expr)`; an omitted `type` means `any` (Terraform docs: "the variable accepts a value of any type"), so canonical `"any"`.
- **Output type**: Terraform 1.15 added an optional `type` argument on `output` blocks (HashiCorp output-block reference, same constraint syntax as variables). If absent, the type is inferred from `value` and is **not knowable statically** — do not treat absence as `any`. Compare an output's type only when both baseline and current declare it and both parse; any other combination is silent.
- **Output sensitivity** = `sensitive` read with the existing literal-bool idiom (`expr.Value(nil)` -> Tristate, same as `literalBool` in `internal/infrastructure/terragrunt/eval.go`). A reference or non-bool is `TristateUnknown` -> silent. Absent = false.

### Canonicalisation: write our own ~40-line walker; do NOT use `typeexpr.TypeString`
`typeexpr.TypeString` is **lossy**: verified empirically on v2.25.0, `object({a=optional(string),b=number})` and `object({a=string,b=number})` both render `object({a=string,b=number})`, so an `optional()` change would be missed (a false negative today, a trap tomorrow). `cty.Type.Equals` *does* distinguish optional attributes (it compares `AttrOptional`), but returns a bool on types the domain cannot hold.

So: infrastructure walks the `cty.Type` and writes a deterministic string:
- `string`, `number`, `bool`, `any` (for `cty.DynamicPseudoType`);
- `list(T)`, `set(T)`, `map(T)`;
- `object({"a"=T,"b"=optional(T)})` with attribute names sorted and always quoted (`ty.AttributeTypes()` + `ty.AttributeOptional(name)`);
- `tuple([T1,T2])` in positional order.
Verified output for the cases above (including nested `list(object({a=optional(map(string))}))`). Whitespace, attribute order, and `any` spelled via `"any"` are normalised by construction, since the string is built from the type, not from source text.
- Anything the walker does not recognise (capsule type, future cty kind) returns `""` (unknown) — never panic (`TypeString` panics on capsule types).
- **Optional defaults are ignored**: `optional(string, "x")` and `optional(string)` canonicalise equal. A changed default is not a type change. Use `TypeConstraintWithDefaults` (not `TypeConstraint`, which rejects the 2-argument `optional(T, default)` form) and discard the returned `*Defaults`.
- **Unparseable -> unknown, never best-effort.** `typeexpr` still returns a `cty.Type` alongside error diagnostics. If `diags.HasErrors()` the type is `""`. Verified failure cases: bare `list`/`map`/`set`, quoted legacy `"string"`, JSON `"${list(string)}"` and array form, `optional(string, var.x)` ("Variables may not be used here"), `optional(string, upper("x"))` ("Functions may not be called here"), typos. All must be silent (zero-false-positive rule). Plain JSON string types like `"list(string)"` and `"object({a=optional(string)})"` parse correctly.
- Hold a property test: for random `cty.Type` pairs, `canon(a)==canon(b)` iff `a.Equals(b)` (test-only use of cty; this pins the canonicalizer to the library's own equality).

### Evaluation boundary (consistent with "structural decode only")
`typeexpr` evaluates nothing except `optional()` default *literals*, via `expr.Value(nil)` (nil `EvalContext`): variables and function calls produce a diagnostic, not a value, so there is no env, filesystem or process access. This is the same technique already used by `literalString`/`literalBool`. Recursion in `typeexpr.getType` is bounded by the existing `hclconv.CheckNativeDepth`/`CheckJSONDepth` pre-scan that already runs before every parse in `tfsurface`.

### Classification (keep it equality-only)
Report "type changed" on canonical-string inequality; do not try to classify widening vs narrowing (`cty/convert` is linked, but conversion rules + `any` + `optional` + marks make a sound subtype test a research project and a false-positive source). Every type-level change lands in **Impacted**, never **Broken**: whether a consumer unit's `inputs` actually violate the new constraint needs the include-merged `inputs` values, which is the deferred GRT005/GRT006 territory. Same for "new required variable": Impacted only (a consumer may supply it through an `include`-merged `inputs`, so Broken would be a false-positive risk). A variable that *loses* its default is the same event as a new required variable and should share the code path.

## Other v0.4 pieces — stack is "none"

| Feature | Stack | Notes |
|---------|-------|-------|
| GRT004 | stdlib only | needs the baseline `RepositoryGraph` (already built by `blast --base`) and current graph; pure domain function over `Surface` outputs + `dependency.X.outputs.Y` refs already extracted for GRT001. |
| Transitive Impacted + path | stdlib (`slices`, sort) | BFS over the unit graph's reverse edges from each directly-affected unit, recording the predecessor to rebuild the path; sort neighbours for determinism; visited set handles `GRT003` cycles. No graph library. |
| `report --blast`, `rebase` | existing `internal/infrastructure/ipc` + `encoding/json` | extend the request frame with a verb; baseline = a retained immutable `*RepositoryGraph` swapped under the daemon's existing lock on `rebase`. Windows report-file path needs an explicit decision (does it dump a blast section or stay unsupported). No `net`. |

## Alternatives Considered

| Category | Recommended | Alternative | Why Not |
|----------|-------------|-------------|---------|
| Type parsing | `hcl/v2/ext/typeexpr` | hand-written walker over `hclsyntax.FunctionCallExpr`/`ObjectConsExpr` | Re-implements the type grammar (`optional`, tuple, `any`, keyword handling) and drifts from Terraform. typeexpr costs 1 package, 0 modules. Only worth revisiting if typeexpr ever links something banned. |
| Canonical form | own `cty.Type` walker | `typeexpr.TypeString` | Verified lossy for `optional()`; panics on capsule types. |
| Canonical form | own `cty.Type` walker | `cty.Type.GoString()` / `FriendlyName()` | `GoString` format is a Go debug rendering not covered by compatibility; `FriendlyName` drops structure ("object"). |
| Equality in domain | opaque canonical string | domain imports `cty` and calls `Type.Equals` | Violates ARCH-01 and the `hcl-only-in-infrastructure` rule. |
| Equality | string `==` | textual comparison of the raw `type = ...` source | `list(string)` vs `list( string )`, attribute reordering, comment/format-only edits would be false "type changed" signals. |
| Graph traversal | stdlib BFS | `gonum/graph`, `dominikbraun/graph` | New module for ~30 lines; gonum is large; unnecessary supply-chain surface. |
| Output type for Terraform < 1.15 modules | omit (unknown) | infer from `value` | Requires evaluation: out of scope by project constraint. |

## What NOT to add

- **No new `require` line.** If a plan task proposes one, challenge it.
- **No `go-cty-yaml`, `gohcl`, `hclwrite`, `hclparse`**: `gohcl` pulls reflect-driven decoding we do not need; `hclwrite` is for generation; `hclparse` adds a parser cache that duplicates the existing parse cache.
- **No cobra/pflag** (link `net`), **no go-git**, **no diff libraries**, **no `x/tools`** beyond what is already indirect.
- **No `cty` or `typeexpr` import outside `internal/infrastructure`.** In particular do not leak `cty.Type` through `ports.SurfaceResult`.
- **No `convert.GetConversion*` subtype logic** in v0.4 (see Classification).
- **No evaluation of `default` values** (only the *presence* of `default`), and no inference of output types from `value`.

## Installation

```bash
# Nothing to install. go.mod / go.sum unchanged.
# Proof to keep in CI (already exists):
scripts/check-architecture.sh        # six-target no-net/no-exec + layering rules
```

## Integration points (files touched)

| File / package | Change |
|----------------|--------|
| `internal/infrastructure/tfsurface/reader.go` | read `type`/`default`/`sensitive` attrs per block; new `typesig.go` with the canonical walker; import `ext/typeexpr` + `go-cty/cty` here only |
| `internal/application/ports/ports.go` | `SurfaceResult` carries declarations (not just names) |
| `internal/domain/repograph/surface.go` | `Surface` stores `Variable`/`Output` records; keep `Variables()/Outputs()` name views and sorted/unique invariants |
| `internal/domain/impact/surface.go` | `SurfaceChange` gains `ChangedVariableTypes`, `NewRequiredVariables`, `ChangedOutputTypes`, `ChangedOutputSensitivity` (all sorted, never nil); `SurfaceDiff` keeps its "unknown in either tree -> silent" rule, now also per-declaration: unknown `TypeSig` on either side -> no type change reported |
| `internal/domain/analysis` | new `grt004.go` (baseline outputs minus current outputs, intersect with referenced outputs; same `enabled`/`skip_outputs` silencing as GRT001) |
| `internal/application/blasting`, `watching`, `internal/infrastructure/ipc` | transitive traversal, baseline retention, `rebase`, `report --blast` |

## Confidence

| Claim | Level | Basis |
|-------|-------|-------|
| typeexpr adds no banned package on any of the six targets; delta = one package | HIGH | `go list -deps` run per target, plus delta vs `./cmd/...` |
| No go.mod/go.sum change | HIGH | all imports already in the module graph; md5 of both files unchanged after the scratch run |
| `TypeString` loses `optional()`; custom walker fixes it | HIGH | reproduced on v2.25.0; walker output verified on 16 inputs incl. JSON |
| Failure cases (bare `list`, quoted types, non-literal optional defaults) return error diags | HIGH | reproduced |
| Output `type` exists only from Terraform 1.15 | MEDIUM | HashiCorp output-block docs and 1.15 release coverage; exact release note not read. OpenTofu parity not checked: treat an undeclared or unparseable output type as unknown either way |
| Variable required iff no `default`; omitted `type` = any | HIGH | HashiCorp variable-block docs |
| Impacted-not-Broken for type changes | MEDIUM | design judgement from the zero-false-positive rule and deferred GRT005/006 |

## Sources

- `github.com/hashicorp/hcl/v2@v2.25.0/ext/typeexpr` (`get_type.go`, `public.go`), read from the module cache — HIGH
- `github.com/zclconf/go-cty@v1.19.0/cty/object_type.go` (`Equals`, `AttributeOptional`) — HIGH
- Terraform output block reference: https://developer.hashicorp.com/terraform/language/block/output — MEDIUM
- Terraform variable block reference: https://developer.hashicorp.com/terraform/language/block/variable — HIGH
- Terraform 1.15 type constraints for outputs: https://github.com/hashicorp/terraform/pull/36411 and https://www.hashicorp.com/en/blog/new-in-terraform-115-dynamic-sources-variable-deprecation-and-more — MEDIUM (not opened in full)
- Repo: `internal/infrastructure/tfsurface/reader.go`, `internal/infrastructure/terragrunt/eval.go` (`literalBool`), `internal/domain/{impact,repograph}/surface.go`, `scripts/check-architecture.sh`
