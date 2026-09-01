> **CORRECTION — verified by compiling, 2026-09-01.**
> This document's *conclusion* is correct: the Terragrunt library cannot be used from an
> external module. Its stated *mechanism* is not.
>
> `pkg/config` imports `internal/*` packages, but that is legal — internal packages are
> importable within their own module, and `import "github.com/gruntwork-io/terragrunt/pkg/config"`
> from a foreign module compiles fine. `ParseConfigFile` and `PartialParseConfigFile` are
> genuinely reachable.
>
> The real blocker is narrower: the only `*ParsingContext` constructor is
> `NewParsingContext(ctx, log.Logger, v *venv.Venv, ...)`, where `venv` is
> `internal/venv`. An external module cannot import that package
> (`use of internal package ... not allowed`), no exported function anywhere in `pkg/`
> returns a `*venv.Venv`, and passing `nil` hits an explicit `panic(ErrParsingContextVenvNil)`.
>
> Also measured: importing pulls **672** modules including full AWS/Azure/GCP SDKs, and
> `go.mod` declares `go 1.27`.

# Stack Research: Terragrunt-as-a-library feasibility

**Domain:** Go CLI, static analysis of Terragrunt repositories
**Researched:** 2026-09-01
**Confidence:** HIGH (verified against primary source: actual GitHub source of `gruntwork-io/terragrunt@v1.1.4`, its `go.mod`, and its official 1.0 compatibility-guarantees doc — not training data)

## VERDICT — this overturns a Key Decision in PROJECT.md

**`github.com/gruntwork-io/terragrunt` cannot be used as a Go library to parse `terragrunt.hcl`, full stop — not "discouraged," not "unstable," but *will not compile* from an external module.**

The PROJECT.md decision *"Import `gruntwork-io/terragrunt` as a library in M1 — Correct HCL function semantics for free"* rests on an assumption that does not survive contact with the current source. Every public entry point that parses a `terragrunt.hcl` requires a value of a type declared under `internal/`, and Go's compiler enforces `internal/` package boundaries at the import-path level — there is no workaround short of vendoring/forking the whole module into `github.com/gruntwork-io/terragrunt/...`'s own path, which is not a real option.

This was cross-verified three independent ways (source inspection, dependency-graph inspection, and the project's own official documentation), detailed in section 1 below. Recommended replacement path is section "Recommended Stack" / "What NOT to Use."

## 1. Can `pkg/config` parse a `terragrunt.hcl` into a struct? (exact paths, current release)

Module: `github.com/gruntwork-io/terragrunt`, **v1.1.4** (released 2026-08-27, latest as of research date; releases run roughly biweekly — v1.1.0 2026-07-01, v1.1.1 2026-07-14, v1.1.2 2026-07-29, v1.1.3 2026-08-13, v1.1.4 2026-08-27).

The parse entry points genuinely exist, with exactly the signatures a roadmap would hope for:

```go
// github.com/gruntwork-io/terragrunt/pkg/config
func ParseConfigFile(
    ctx context.Context,
    pctx *ParsingContext,
    l log.Logger,
    configPath string,
    includeFromChild *IncludeConfig,
) (*TerragruntConfig, error)

func PartialParseConfigFile(
    ctx context.Context,
    pctx *ParsingContext,
    l log.Logger,
    configPath string,
    include *IncludeConfig,
) (*TerragruntConfig, error)

type TerragruntConfig struct { /* Terraform, Include, Dependencies, Inputs, RemoteState, ... */ }
```

Both live in `pkg/config` (not `internal/`) — see section 4 for why that doesn't matter.

**The blocker:** both functions take `pctx *ParsingContext`, and the only public constructor is:

```go
// github.com/gruntwork-io/terragrunt/pkg/config/parsing_context.go
func NewParsingContext(
    ctx context.Context,
    l log.Logger,
    v *venv.Venv,          // github.com/gruntwork-io/terragrunt/internal/venv
    opts ...Option,
) (context.Context, *ParsingContext)
```

`v *venv.Venv` is the poison pill — see section 4. `ParsingContext` itself is worse than the constructor signature suggests: its exported struct fields are typed with half a dozen more `internal/*` types (`TerraformCliArgs *iacargs.IacArgs`, `EngineConfig *engine.EngineConfig`, `IAMRoleOptions iam.RoleOptions`, `StrictControls strict.Controls`, `ProviderCacheOptions pcoptions.ProviderCacheOptions`, `ParserOptions []hclparse.Option`), so even a hand-rolled struct literal (skipping `NewParsingContext` entirely) cannot be built from outside the module — you cannot name these types without importing `internal/*`, and Go's toolchain refuses that import from a foreign module path.

Source: `pkg/config/config.go` (parse entry points, TerragruntConfig at line 145), `pkg/config/parsing_context.go` (ParsingContext struct, NewParsingContext), both read directly from `github.com/gruntwork-io/terragrunt` at tag `v1.1.4` via the GitHub API, 2026-09-01.

## 2. Version and Go requirement

- Latest release: **v1.1.4**, 2026-08-27.
- `go.mod`: `go 1.27` (no separate `toolchain` directive). Confirmed by fetching `go.mod` directly from the repo.
- **This is a second, independent reason the import is a bad fit even before the `internal/` blocker**: the project's stated stack targets Go 1.24. Terragrunt v1.1.x requires a **Go 1.27 toolchain** to build. Go 1.27 was released 2026-08-19 (three feature releases ahead of 1.24, which shipped Feb 2025) — this is current information, not a training-data guess. Building gruntled against terragrunt would force an upgrade of the whole project's Go toolchain as a side effect of one dependency, which is its own smell independent of the `internal/` problem.

## 3. Partial-parse mode — does it exist? Yes, and it's well-designed. It doesn't matter.

`pkg/config/config_partial.go` defines exactly the mechanism a roadmap would want:

```go
type PartialDecodeSectionType int

const (
    DependenciesBlock PartialDecodeSectionType = iota
    DependencyBlock
    TerraformBlock
    TerraformSource
    TerragruntFlags
    TerragruntVersionConstraints
    RemoteStateBlock
    FeatureFlagsBlock
    EngineBlock
    ExcludeBlock
    ErrorsBlock
    TerraformExtraArgs
)

func PartialParseConfigFile(ctx, pctx, l, configPath, include) (*TerragruntConfig, error)
func PartialParseConfigString(...) (*TerragruntConfig, error)
```

Consumers set `pctx.PartialParseDecodeList = []PartialDecodeSectionType{DependencyBlock, TerraformBlock, ...}` before calling — this is precisely "decode only `dependency`/`terraform`/etc., skip the rest," which would have let gruntled skip locals evaluation entirely. Note there is no dedicated `IncludeBlock` or `InputsBlock` constant; `include` resolution happens via a separate code path (`DecodeBaseBlocks`, `partialParseIncludedConfig`) and `inputs` appears to only fully materialize in the non-partial decode — worth re-verifying if this ever becomes reachable.

None of this changes the verdict: `PartialParseConfigFile` takes the same poisoned `*ParsingContext` as the full parse. The partial-decode design is good engineering, aimed at Terragrunt's own internal callers, not at library consumers.

## 4. Are the parsing packages under `internal/`? — the actual answer is more damaging than a yes/no

`pkg/config` itself is **not** under `internal/` — Go tooling and `pkg.go.dev` will happily list it as importable. But every function you would actually call takes or returns a type from `internal/`:

| Public function/field (in `pkg/config`, importable path) | Type required | Declared in |
|---|---|---|
| `NewParsingContext(ctx, l, v, opts...)` param `v` | `*venv.Venv` | `internal/venv` |
| `ParsingContext.TerraformCliArgs` | `*iacargs.IacArgs` | `internal/iacargs` |
| `ParsingContext.EngineConfig` | `*engine.EngineConfig` | `internal/engine` |
| `ParsingContext.IAMRoleOptions` | `iam.RoleOptions` | `internal/iam` |
| `ParsingContext.StrictControls` | `strict.Controls` | `internal/strict` |
| `ParsingContext.ProviderCacheOptions` | `pcoptions.ProviderCacheOptions` | `internal/providercache/options` |
| `hclparse.Parser.ParseFromFile(fsys, configPath)` param `fsys` | `vfs.FS` | `internal/vfs` |
| `pkg/options.TerragruntOptions` (the older, more commonly cited object) | embeds `internal/venv`, `internal/engine`, `internal/iam`, `internal/vexec`, `internal/vfs`, `internal/filter`, `internal/report`, `internal/cloner`, `internal/tips`, ... | multiple `internal/*` |

Go enforces `internal/` visibility by import **path**, not by package location: only code whose own import path is rooted at `github.com/gruntwork-io/terragrunt` (i.e., lives inside this module) may import `github.com/gruntwork-io/terragrunt/internal/...`. `github.com/<you>/gruntled` is a different module rooted at a different path — the compiler rejects the import outright (`use of internal package ... not allowed`), before any question of API stability even arises. There is no field-visibility or reflection trick around this; it's enforced by `go build`/`go vet` at the import-graph level.

**Confirmed this is the shipping state, not a one-off gap**: I checked three independent surfaces (`pkg/config/config.go`, `pkg/config/parsing_context.go`, `pkg/config/hclparse/parser.go`, and `pkg/options/options.go`) and all four import `internal/venv` (or its siblings) in their exported signatures. The `internal/venv` package was introduced deliberately and recently (PRs #6089, #6404, #6406, #6412, June–July 2026) as an explicit architectural choice — "Threading venv through CLI" — to funnel every side-effecting operation (filesystem, subprocess exec, HTTP, SOPS decryption, env vars) through one injectable bundle. It is good internal architecture. It is also, as a side effect (probably not even the primary intent), what makes external library use impossible.

## 5. API stability track record — official, in writing

Terragrunt's own docs settle this without needing to infer anything from source:

> **Golang Library Compatibility** — "Using Terragrunt as a Go library has no backwards compatibility guarantees. Most Go code in the Terragrunt repository lives in `internal`, and maintainers don't expect external parties to depend on Terragrunt packages directly. When packages are generally useful to internal Gruntwork parties, they will be migrated to `pkg`. Breaking changes to packages in `pkg` are still possible at any time... When external parties need a stable dependency on shared code, dedicated libraries will be created in separate, versioned repositories (e.g., `terragrunt-engine-go`)."
> — `docs/src/content/docs/07-process/01-1-0-guarantees.mdx`, current `main`, fetched 2026-09-01.

This is a direct, current statement from the maintainers, not an inference. It matches the on-the-ground evidence: GitHub issue [#4004](https://github.com/gruntwork-io/terragrunt/issues/4004) ("Want to use terragrunt library directly in golang code," opened 2025) was closed without maintainers offering a supported path — the one substantive reply asked *why* the user wanted direct integration, rather than pointing at a stable API. PR [#5564](https://github.com/gruntwork-io/terragrunt/pull/5564) ("docs: Adding callout for the lack of library compatibility guarantees," merged 2026-02-18) exists specifically to make this explicit after presumably enough people asked.

**Conclusion: minor releases don't just risk breaking these signatures — the maintainers explicitly disclaim any obligation not to, and today's release already blocks the import at compile time regardless of future breakage.**

## 6. Transitive dependency weight (moot given the verdict, quantified anyway per the question)

From `go.mod` at `v1.1.4`: **298 total `require` entries** (103 direct, 195 indirect). This single `go.mod` pulls in, among others:
- Full cloud SDKs for all three major providers: `aws/aws-sdk-go`, `aws/aws-sdk-go-v2` (+ ~6 subservices: dynamodb, iam, s3, sts, config, credentials), `Azure/azure-sdk-for-go` (6 subpackages: azcore, azidentity, armauthorization, armresources, armstorage, azblob), `cloud.google.com/go/auth`, `cloud.google.com/go/storage` — this alone contradicts the project's "No cloud SDKs" constraint even before the compile-time blocker.
- A full terminal-UI stack: `charm.land/bubbletea/v2`, `bubbles/v2`, `glamour/v2`, `lipgloss/v2` (for the `catalog` TUI command) — dead weight for a library consumer that never touches that command.
- `hashicorp/hcl` v1 *and* `hashicorp/hcl/v2` v2.24.0 (both, simultaneously).
- No cgo indicators found (`mattn/go-shellwords`, `mattn/go-zglob`, `mattn/go-isatty`/`go-colorable`/`go-runewidth` are pure-Go terminal/glob utilities, not cgo bindings) — this one dimension is not a problem.

Even setting the `internal/` blocker aside entirely, importing this module would triple-plus gruntled's dependency surface with three cloud SDKs and a TUI framework it will never call, directly contradicting the "No cloud SDKs" / "single binary, air-gap-demonstrable" constraints in PROJECT.md.

## 7. Licence

**MIT** (`LICENSE.txt`, confirmed via `gh api repos/gruntwork-io/terragrunt` → `license.spdx_id: MIT`). MIT is a permissive license fully compatible with Apache-2.0 redistribution — the Apache Software Foundation's own third-party license policy lists MIT as "Category A" (may be included in Apache-licensed products without further legal review). Not the blocker; included for completeness since it was asked. This also means small, targeted pieces of Terragrunt's *logic* (e.g. the `find_in_parent_folders` algorithm, if copied rather than imported) could legally be reimplemented/adapted with attribution — see the recommended alternative below.

## 8. Side-effect suppression — the mechanism exists, but it's also gated behind the same wall

Terragrunt's own design already anticipated needing to suppress side effects during parsing, and the mechanism is genuinely well thought out:

- `run_cmd` and `get_env` (the two HCL functions capable of causing effects/reading environment) are implemented as thin wrappers that shell out through `pctx.Venv.Exec` (`internal/vexec.Exec`, an interface) and read from `pctx.Venv.Env` (`map[string]string`), respectively — both injectable. Production wiring uses `venv.OSVenv()` (real subprocess exec, real `os.Environ()`); tests substitute `Venv.WithHandler(h vexec.Handler)` (an in-memory fake) or a synthetic `Env` map. In principle, an embedder could supply a `Venv` whose `Exec` always returns "disabled" and whose `Env` is empty, exactly matching gruntled's "disable this path, mark the unit `unknown`" requirement.
- Dependency-output resolution (`dependency.X.outputs.Y` evaluating to an actual *value*, which requires either running `terragrunt output` in the target unit or reading remote state over the network) is separately gate-able: `ParsingContext.SkipOutputsResolution` / `WithSkipOutputsResolution()` and `ParsingContext.NoDependencyFetchOutputFromState` exist specifically to turn this off.
- Filesystem and HTTP access are likewise behind `Venv.FS` (`internal/vfs.FS`) and `Venv.HTTP` (`internal/vhttp.Client`), both injectable in principle (an in-memory `vfs.FS` would also incidentally give read-only, no-`.terragrunt-cache`-writes guarantees for free).

**All of this is unreachable from outside the module**, for the identical reason as section 4: `venv.Venv`, `vexec.Exec`, `vfs.FS`, `vhttp.Client` are all `internal/*` types. The suppression mechanism is real, well-designed, and completely inaccessible to gruntled. It is worth noting for what it tells us about scope, though: Terragrunt's own maintainers had to solve "parse without running `run_cmd`/hitting the network" as a real engineering problem, which confirms this is a genuine hazard to design around in gruntled's own parser too (see Recommended Stack below — never implement `run_cmd`/`get_env` at all, rather than implement-then-disable).

## Recommended Stack

Reject the terragrunt-as-library plan. Adopt directly what PROJECT.md already lists as the eventual stack for the "Later" milestone ("Own HCL parser: parse each include once and share it") — but pull it forward into v0.1/M1, since the "import terragrunt for free correctness" alternative does not exist. Reframe the risk, too: **v0.1's own scope already limits what needs evaluating**. `GRT001` compares *names* referenced in `dependency.X.outputs.Y` expressions against the module's extracted `.tf` surface — it does not need the actual *value* of any output, so gruntled never needs Terragrunt's expression evaluator, remote-state readers, or `dependency` output-fetching machinery at all. That was always out of scope (PROJECT.md "Out of Scope": no expression evaluation). This shrinks "reimplement Terragrunt's HCL semantics" from "reimplement an evaluator" down to "structurally decode a fixed set of blocks and resolve `include` chains" — a meaningfully smaller and more bounded problem than the original risk assessment implied.

### Core Technologies

| Technology | Version | Purpose | Why Recommended |
|------------|---------|---------|-----------------|
| `hashicorp/hcl/v2` | v2.24.0 | HCL2 tokenizing/parsing (`hclsyntax`), traversal | Current stable, exactly the version Terragrunt itself pins — same semantics gruntled would inherit "for free" via import, obtained instead by depending on the same upstream parser directly, with none of the `internal/` contamination |
| `zclconf/go-cty` | v1.16.3 (pulled transitively by hcl/v2 v2.24.0; latest standalone is v1.19.0, requires Go 1.25 — do not force-upgrade past what hcl/v2's `go.mod` pins unless gruntled's own Go version moves too) | HCL's value system (`cty.Value`), and `cty/function/stdlib` for the ~30 built-in functions (`upper`, `lower`, `jsonencode`, `element`, `merge`, `format`, etc.) that Terragrunt's own HCL functions are themselves built on | Public, independent of Terragrunt's `internal/` restructuring; reusing `function/stdlib` avoids reimplementing the generic HCL function library, leaving only Terragrunt-*specific* functions (`find_in_parent_folders`, `path_relative_to_include`, `get_terragrunt_dir`, etc.) to hand-write |
| `spf13/cobra` | v1.10.2 | CLI command tree (`check`, later `watch`/`report`/`blast`/`graph`) | Already in PROJECT.md's stated stack; de facto standard for Go CLIs, minimal Go requirement (`go 1.15` in its own `go.mod`), no conflict |
| `fsnotify/fsnotify` | v1.10.1 | Filesystem watching for the later daemon milestone | Already in PROJECT.md's stated stack; standard, `go 1.23` minimum |

### Supporting Libraries (for the hand-rolled Terragrunt parser — new, not previously in PROJECT.md)

| Library | Version | Purpose | When to Use |
|---------|---------|---------|-------------|
| `hashicorp/hcl/v2/hclsyntax` | (bundled with hcl/v2) | Native HCL2 syntax parsing, partial-body decoding via `hcl.Body.PartialContent` | Structural decode of `dependency`, `terraform { source = }`, `include`, `inputs`, `locals` blocks without evaluating everything |
| `hashicorp/hcl/v2/hclparse` | (bundled with hcl/v2) | File-to-`hcl.Body` loading with diagnostics | Same role Terragrunt's own `pkg/config/hclparse` plays — reimplement the thin wrapper directly, it is a handful of lines |

Do **not** implement `run_cmd` or `get_env` at all in gruntled's parser, rather than implementing them and then disabling — this matches the project's own "no external processes, no network" constraint exactly, and Terragrunt's own experience (having to build an entire injectable `Venv` abstraction just to make these two functions test-safe) is a warning sign about how much surface they otherwise touch. Any HCL expression that calls `run_cmd()`/`get_env()`/references an unresolvable remote source should short-circuit gruntled's evaluator and mark the unit `unknown` — which is already the project's own zero-false-positive philosophy (PROJECT.md, section 7 in the design doc).

## Alternatives Considered

| Recommended | Alternative | When to Use Alternative |
|-------------|-------------|--------------------------|
| Hand-rolled structural parser on `hashicorp/hcl/v2` | Import `gruntwork-io/terragrunt/pkg/config` directly | Never, as of v1.1.4 — will not compile from outside the module (section 4). Re-evaluate only if Terragrunt ships a dedicated, versioned parsing library outside `internal/` (their own docs mention this as their preferred path for external consumers, e.g. `terragrunt-engine-go` is the precedent for how they'd do it — no such repo exists yet for config parsing) |
| Hand-rolled structural parser | Shell out to the real `terragrunt` CLI (`hcl validate --json`, `terragrunt render --json`) as a subprocess | Explicitly ruled out by PROJECT.md's "no external processes" constraint; would also reintroduce Terragrunt's own O(n²) `locals`/`include` re-evaluation cost, which is the exact problem gruntled exists to avoid |
| Vendoring/copying small, MIT-licensed algorithms (e.g. `find_in_parent_folders` directory-walk logic) with attribution | Vendoring the entire `internal/` tree via `go mod vendor` + manual patch to make it importable | Copying small, well-isolated algorithms is legally clean under MIT and low-maintenance; forking/patching the whole internal tree to defeat the `internal/` boundary is fragile, hostile to upstream updates, and defeats the purpose of "correct semantics for free" that motivated the import in the first place |

## What NOT to Use

| Avoid | Why | Use Instead |
|-------|-----|--------------|
| `github.com/gruntwork-io/terragrunt/pkg/config` (`ParseConfigFile`, `PartialParseConfigFile`) | Does not compile from an external module — every public entry point requires an `internal/*` type (`*venv.Venv` at minimum). Confirmed against `v1.1.4` source, not inferred | `hashicorp/hcl/v2` + hand-rolled structural decode (this document, "Recommended Stack") |
| `github.com/gruntwork-io/terragrunt/pkg/options` (`TerragruntOptions`) | Same problem, arguably worse — its own `go.mod` imports show it embedding *more* `internal/*` packages than `pkg/config` does | n/a — not needed; gruntled doesn't need Terragrunt's CLI-option surface at all |
| Pinning gruntled's Go toolchain to whatever `internal/` patch would require | Chases a moving, explicitly-unsupported target (section 5); Terragrunt requiring Go 1.27 while gruntled targets 1.24 is a second, independent friction point even if the `internal/` wall were somehow bypassed | Keep gruntled's Go version decision independent of Terragrunt's; re-evaluate PROJECT.md's "Go 1.24" against current Go 1.27 (Aug 2026) separately — flagged here as an aside, not fully researched, since it wasn't this document's question |

## Stack Patterns by Variant

**If v0.1/M1 needs `dependency`/`include`/`terraform{source}`/`inputs` structurally, without evaluating locals functions:**
- Use `hcl.Body.PartialContent()` against a `hclsyntax.Schema` mirroring exactly the block/attribute names Terragrunt documents for those four constructs
- Because full-body decode (`gohcl.DecodeBody`) would require resolving every expression including ones calling undefined-in-gruntled functions, which is exactly the complexity being avoided

**If a `locals` or `inputs` expression calls a function gruntled hasn't implemented, or references an unresolvable remote `source`:**
- Mark the unit `unknown`, skip checks that depend on it, continue indexing the rest of the repository
- Because this is the project's own non-negotiable "zero false positives" rule, and matches how Terragrunt's own `Venv`-gated design treats side-effecting calls it can't safely evaluate either

## Version Compatibility

| Package A | Compatible With | Notes |
|-----------|-----------------|-------|
| `hashicorp/hcl/v2@v2.24.0` | `zclconf/go-cty@v1.16.3` (its own pinned version) | Do not independently bump go-cty to v1.19.0 — that release requires Go 1.25, ahead of gruntled's stated Go 1.24 target; let hcl/v2's own `go.mod` pin the version unless the project's Go version is deliberately raised |
| `gruntwork-io/terragrunt@v1.1.4` | Go 1.27 toolchain (hard requirement in its `go.mod`) | Confirms this dependency is a poor fit even ignoring the `internal/` blocker — gruntled currently targets Go 1.24 |

## Sources

- `github.com/gruntwork-io/terragrunt` source at tag `v1.1.4`, fetched directly via GitHub Contents API on 2026-09-01: `pkg/config/config.go`, `pkg/config/config_partial.go`, `pkg/config/parsing_context.go`, `pkg/config/options.go`, `pkg/config/hclparse/parser.go`, `pkg/options/options.go`, `internal/venv/venv.go`, `go.mod` — HIGH confidence, primary source
- `docs/src/content/docs/07-process/01-1-0-guarantees.mdx` on `main`, fetched 2026-09-01 — official maintainer statement on library compatibility — HIGH confidence, primary source
- GitHub issue [gruntwork-io/terragrunt#4004](https://github.com/gruntwork-io/terragrunt/issues/4004) "Want to use terragrunt library directly in golang code" — corroborating community evidence — MEDIUM confidence (anecdotal, but consistent with the source-level finding)
- GitHub PR [gruntwork-io/terragrunt#5564](https://github.com/gruntwork-io/terragrunt/pull/5564) "docs: Adding callout for the lack of library compatibility guarantees," merged 2026-02-18 — HIGH confidence
- Commit history for `internal/venv/venv.go` (PRs #6089, #6404, #6406, #6412, June–July 2026) via `gh api repos/gruntwork-io/terragrunt/commits` — HIGH confidence, shows the `internal/` restructuring is recent and deliberate
- Go module proxy (`proxy.golang.org`) `@latest` queries for `hashicorp/hcl/v2`, `spf13/cobra`, `fsnotify/fsnotify`, `zclconf/go-cty`, plus their respective `go.mod` files fetched from GitHub tags — HIGH confidence, primary source, current versions as of 2026-09-01
- WebSearch "Go 1.27 release date golang.org 2026" confirming Go 1.27 shipped 2026-08-19 — MEDIUM confidence (WebSearch, not independently cross-checked against go.dev directly, but consistent with Go's well-established ~6-month release cadence)
- `gh api repos/gruntwork-io/terragrunt` license endpoint (`license.spdx_id: MIT`) and `LICENSE.txt` presence — HIGH confidence, primary source

---
*Stack research for: gruntled — Terragrunt repository static analysis CLI*
*Researched: 2026-09-01*
