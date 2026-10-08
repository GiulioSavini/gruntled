# Architecture Research: gruntled

**Domain:** Go DDD/hexagonal CLI — static analysis of Terragrunt repositories
**Researched:** 2026-09-01
**Confidence:** HIGH (Part A, verified against primary sources) / MEDIUM-HIGH (Part B, verified against real reference projects + Go tooling docs)

---

## Part A — Surface Extraction

### A.1 — Is `terraform-config-inspect` still the right library?

**Yes. Verdict: use it. Confidence: HIGH — verified against the real repository, not memory.**

Verified facts (via `go.mod`, source files, GitHub API, `pkg.go.dev`, fetched 2026-09-01):

| Fact | Value | Source |
|---|---|---|
| Module path | `github.com/hashicorp/terraform-config-inspect` | go.mod, root of repo |
| Latest pseudo-version | `v0.0.0-20260709150029-2fb54c236733` | pkg.go.dev |
| Last commit | 2026-07-09 (code), repo `pushed_at` 2026-09-01 | GitHub API |
| Archived? | No (`"archived": false`) | GitHub API `/repos/hashicorp/terraform-config-inspect` |
| Open issues / forks / stars | 34 / 84 / 439 | GitHub API |
| License | **MPL-2.0** (`LICENSE` file, `Copyright IBM Corp. 2018, 2025`) | repo root |
| Go version | `go 1.18` (module declares this floor; works fine under Go 1.24) | go.mod |
| HCL dependency | `github.com/hashicorp/hcl/v2 v2.20.1` | go.mod |
| Formal release tags | **None** — consumed via pseudo-version, no `git tag` releases exist | GitHub API `/tags` returned empty |

Note the copyright line: HashiCorp's Terraform ecosystem repos now carry `Copyright IBM Corp.` (post-acquisition), but the module path, org, and maintenance activity are unchanged. Not archived, actively pushed to as of the research date. The maintainers' own stance (README, git blame consistent 2018→2025): "feature-complete," bug-fixes and API-usability changes only, explicitly **hesitant about breaking changes** because "this library is used by a number of existing tools and systems" (it backs `terraform-docs`, among others). This is exactly the risk profile you want for a dependency: stable API, low churn, not going away.

**MPL-2.0 vs. gruntled's Apache-2.0 target:** compatible. MPL-2.0 is file-level weak copyleft — it only requires that modifications to MPL-2.0-licensed *files themselves* stay MPL-2.0 if redistributed; it does not propagate to the code that imports it as a library, and it does not prevent gruntled's own code from being Apache-2.0. No action needed beyond standard third-party notice/attribution in a `NOTICE` or `go.sum`-derived license report.

**Exact API** (verified against `load.go`, `variable.go`, `output.go` source, fetched raw from GitHub):

```go
func LoadModule(dir string) (*Module, Diagnostics)
func LoadModuleFromFilesystem(fs FS, dir string) (*Module, Diagnostics)
func IsModuleDir(dir string) bool
func IsModuleDirOnFilesystem(fs FS, dir string) bool

type Module struct {
    Path              string
    Variables         map[string]*Variable
    Outputs           map[string]*Output
    RequiredCore      []string
    RequiredProviders map[string]*ProviderRequirement
    ProviderConfigs   map[string]*ProviderConfig
    ManagedResources  map[string]*Resource
    DataResources     map[string]*Resource
    ModuleCalls       map[string]*ModuleCall
    Diagnostics       Diagnostics
}

type Variable struct {
    Name, Type, Description string
    Default                 interface{} // lossy, JSON-serializable only
    Required, Sensitive     bool
    Deprecated              string
    Pos                     SourcePos
}

type Output struct {
    Name, Description, Deprecated string
    Sensitive                     bool
    Type                          string // present but not populated by real `output` blocks (Terraform output blocks have no `type` argument) — treat as always empty in practice
    Pos                           SourcePos
}
```

`LoadModuleFromFilesystem` takes an injectable `FS` interface (defaults to `NewOsFs()`), which is directly useful for testing the `tfsurface` adapter against in-memory fixtures without touching disk.

### A.2 — `.tf.json`, initialization, network

All three confirmed by reading `load.go` and `load_hcl.go` directly:

- **`.tf.json` — supported.** `fileExt()` explicitly matches `.tf`, `.tf.json`, plus the newer `.tfcomponent.hcl` / `.tfstack.hcl` (Terraform Stacks format, irrelevant to gruntled's scope). `loadModule` branches on `strings.HasSuffix(filename, ".json")` to route through the JSON-body HCL parser rather than the native syntax parser — same struct output either way.
- **No initialization required.** The loader never touches `.terraform/`, provider schemas, or lock files. It reads directory entries and parses source text only. This matches gruntled's read-only constraint exactly.
- **No network access.** Confirmed by reading the full `load.go`: `dirFiles()` calls `fs.ReadDir(dir)` and nothing else does I/O. There is no HTTP client, no provider-schema fetch, anywhere in the load path.
- **Override file handling** is built in: `foo_override.tf` / `override.tf` are detected and processed after primary files, matching Terraform's own override semantics — one less edge case gruntled would otherwise have to reimplement.

### A.3 — What it does NOT give us

Verified by reading `load_hcl.go` directly (both the native-HCL and legacy-HCL code paths use the identical logic):

```go
var typeExprAsStr string
valDiags := gohcl.DecodeExpression(attr.Expr, nil, &typeExprAsStr)
if !valDiags.HasErrors() {
    typeExpr = typeExprAsStr
} else {
    rng := attr.Expr.Range()
    typeExpr = string(rng.SliceBytes(file.Bytes))
}
v.Type = typeExpr
```

- **`Variable.Type` is the raw source text of the type expression, not a parsed `cty.Type`.** `type = object({ id = string, tags = optional(map(string)) })` comes back as that literal string. No structural decomposition, no way to ask "does this variable accept a string" without gruntled parsing that string itself.
- **`nullable` is not read at all.** Grepped the entire `load_hcl.go` (625 lines) for `nullable` — zero matches. The struct has no `Nullable` field. If gruntled ever needs nullable-aware semantics, it has to re-parse the variable block itself.
- **Optional object attributes (`optional(...)`) are opaque** — they exist only inside the raw `Type` string, undecoded.
- **`validation` blocks inside `variable`** are not exposed (irrelevant to gruntled's scope — no expression evaluation is a stated non-goal).
- **Ephemeral variables/resources (Terraform 1.10+)** — not mentioned anywhere in the struct or source reviewed. **UNVERIFIED** whether `ephemeral` is captured or silently dropped; low practical risk for v0.1 since GRT001 doesn't need it, but flag for whoever implements GRT005/006 later.

**Forward pointer for later diagnostics (GRT005/GRT006, which do need real type matching):** `hashicorp/hcl/v2/ext/typeexpr` is the correct tool to parse `Variable.Type`'s raw string into an actual `cty.Type` (this is literally what Terraform core itself uses to parse `type = ...` constraints, including `optional()` and defaults). Pipeline: `hclsyntax.ParseExpression(v.Type, ...)` → `typeexpr.TypeConstraint(expr)`. Not needed for v0.1 (GRT001 only checks output-name existence, not types).

### A.4 — terraform-config-inspect vs. raw `hcl/v2` + `hclparse`

**Recommendation: use terraform-config-inspect for v0.1. Do not hand-roll a raw-HCL variable/output reader.**

| | terraform-config-inspect | Raw `hcl/v2` + `hclparse` |
|---|---|---|
| Effort | Near zero — one function call, one adapter to translate `*tfconfig.Module` → domain `Surface` VO | Must reimplement: `.tf` + `.tf.json` dual-path parsing (`hcl/v2` does have a `hcl/json` subpackage implementing the same `hcl.Body` interface, so this is *feasible*, not exotic — but still your code to write and test), override-file merging (`*_override.tf`, `override.tf`), editor swap-file / dotfile skipping, legacy pre-0.12 HCL fallback |
| Robustness | Years of edge-case bug fixes across a library used by `terraform-docs` and others (84 forks, 439 stars, actively triaged) | Starts from zero; every edge case terraform-config-inspect already handles becomes a gruntled bug to discover in the wild — directly contradicts the zero-false-positive principle |
| Speed | Thin wrapper around `hcl/v2` parsing — negligible overhead (a handful of map allocations per module) | Same underlying parser, so no meaningful speed advantage |
| Scope fit | Returns exactly variable names/raw-types and output names, plus resources/providers/module-calls gruntled doesn't need yet but may reuse for GRT002-006 without a new adapter | Would need to be scoped down manually to avoid growing into a second parser to maintain |
| Domain purity | Fine — HCL never appears outside `infrastructure/tfsurface`; the adapter converts to a domain VO at the boundary | Same discipline required either way |

The "own HCL parser" line in `PROJECT.md`'s Later section ("parse each include once and share it — the speed claim") is about **Terragrunt `include` files**, which Terragrunt's O(n²) documented behavior re-evaluates per unit. It is **not** about module `.tf` files: a given module directory is read once per distinct resolved path regardless of how many units reference it (dedupe by path in the indexing use case), so there is no O(n²) hot loop here to justify hand-rolling a parser. Reusing terraform-config-inspect costs nothing on the speed axis that matters for v0.1's benchmark claim.

### A.5 — Resolving the module a Terragrunt unit points to

Terragrunt's `terraform { source = ... }` uses the same source-address mechanism as Terraform's own `module` block `source` argument — confirmed directly from Terragrunt's docs (fetched 2026-09-01, `docs.terragrunt.com/reference/config-blocks-and-attributes`, which the old `terragrunt.gruntwork.io` URL now 308-redirects to): *"support[s] the same syntax as the module source parameter for OpenTofu/Terraform `module` blocks **except for the Terraform registry**... including local file paths, Git URLs, and Git URLs with `ref` parameters,"* plus two Terragrunt-specific protocol additions: `tfr://REGISTRY_HOST/MODULE_SOURCE[?version=VERSION]` and `oci://REGISTRY_HOST/REPOSITORY[//SUBDIR][?tag=TAG|?digest=DIGEST]`.

Underlying mechanism (per Terragrunt docs/DeepWiki cross-check, MEDIUM confidence — not primary-sourced to Go code, but consistent across sources): **Terragrunt delegates all of this to `hashicorp/go-getter`**, the same library Terraform itself uses for `-from-module` and provider downloads. This matters directly for gruntled: `go-getter` exposes a pure, offline classification function that gruntled can reuse without ever calling the network-touching part of the library.

**Verified (via `pkg.go.dev` + `go-getter` source):**

```go
func Detect(src string, pwd string, ds []Detector) (string, error)
```

`Detect` is documented as pure string transformation — *"Detect turns a source string into another source string if it is detected to be of a known pattern"* — no network I/O. It runs a source string through an ordered list of `Detector`s (`FileDetector`, `GitHubDetector`, `GitLabDetector`, `BitBucketDetector`, `S3Detector`, `GCSDetector`, `GitDetector` for SSH-style addresses) and normalizes it to a scheme-qualified URL (`file://...` for local paths). `SourceDirSubdir` then splits a `//subdir` suffix. Neither function performs I/O beyond `Detect`'s local-path branch resolving against `pwd`. **`tfr://` and `oci://` are not go-getter's concern** — those are Terragrunt-specific getters layered on top, per Terragrunt's own docs; the string-level detection rule below still classifies them correctly as remote without needing go-getter to know about them.

**Recommended classification algorithm** (defensive, matches the zero-false-positive principle — when ambiguous, prefer `unknown` over guessing local):

1. `source` contains a forced-getter prefix (`::`, e.g. `git::`, `hg::`, `s3::`, `gcs::`, `http::`, `https::`) → **remote**.
2. `source` starts with a non-`file` scheme (`git://`, `ssh://`, `http://`, `https://`, `s3://`, `gcs://`, `tfr://`, `oci://`) → **remote**.
3. `source` matches SCP-like git syntax (`user@host:path`) → **remote**.
4. `source` begins with `./`, `../`, or is an absolute path (`/...`) → **local**: resolve relative to the directory containing the unit's `terragrunt.hcl`, `filepath.Clean` it.
5. Anything else (bare shorthand: `github.com/org/repo//modules/x`, `terraform-aws-modules/vpc/aws`) → **remote**. This branch is load-bearing and directly backed by Terraform's own documented rule (`developer.hashicorp.com/terraform/language/modules/sources`, verified via WebSearch + cross-checked against the Terragrunt docs statement that it mirrors Terraform's module-source syntax): *"A local path must begin with either `./` or `../` to indicate that a local path is intended, to distinguish from a module registry address."* Anything not matching rule 4 is therefore never local by construction — no heuristic guessing needed.
6. Even after classifying as **local**, `os.Stat` the resolved path. If it doesn't exist or isn't a directory, mark the unit `unknown` rather than emitting a diagnostic based on a wrong assumption — belt-and-braces consistent with "ten false negatives beat one false positive."

**Implementation choice:** reuse `go-getter`'s `Detect` + `SourceDirSubdir` rather than hand-rolling the regex/prefix table, and call **only** `Detect`, never `Get`/`Client.Get`. This is safe against the "no network at runtime" constraint because Go doesn't execute unreferenced code — the networking getters (`get_http.go`, `get_git.go`, etc.) are simply never invoked. It is also low-cost: `go-getter` is already a transitive dependency the moment gruntled imports `gruntwork-io/terragrunt` as a library (per the M1 decision in `PROJECT.md`), so this reuses an existing dependency rather than adding a new attack surface, and it stays bit-for-bit consistent with what real Terragrunt would decide (no drift between gruntled's classification and Terragrunt's actual behavior).

**Open question flagged for a short spike, not resolved here (UNVERIFIED):** since gruntled already plans to import `gruntwork-io/terragrunt` as a library for unit/include parsing, it's worth checking during implementation whether the library exposes its own resolved-source-classification step (it must have one internally, since it decides local-copy vs. go-getter-download) that gruntled could call directly instead of reimplementing rules 1-6 against raw `go-getter`. If it does, prefer that — it guarantees byte-identical behavior to real Terragrunt with less code. If the library's internal function isn't exported or is entangled with actual downloading, fall back to the go-getter `Detect`-based approach above. This should be a half-day spike at the start of the `infrastructure/terragrunt` work (Level 6 in the build order below), not a blocking research question — either path is buildable now.

---

## Part B — System Structure

### B.6 — Package layout: validate the proposed design

**The layout proposed in `docs/superpowers/specs/2026-09-01-gruntled-design.md` §5.2 is sound. Validated against real Go projects, with one concrete addition recommended.**

```
cmd/gruntled/                    composition root
internal/
  domain/                        zero external dependencies
    repograph/                   Unit, Module, Surface (VO), Reference (VO), RepositoryGraph (aggregate root)
    diagnostic/                  Diagnostic (VO), DiagnosticKey, Set with pure Diff
    blast/                       domain service: Broken / Impacted
    analysis/                    domain service: analyzers (GRT001, ...)
  application/                   use cases; know ports, not adapters
    indexing/ watching/ querying/
    ports/                       ConfigParser · SurfaceReader · ModuleResolver · FileWatcher · Notifier · IndexStore · Clock
  infrastructure/                adapters — hcl, go-getter, fsnotify, sockets live ONLY here
    terragrunt/                  ACL: terragrunt.hcl + shared includes -> domain
    tfsurface/                   ACL: .tf -> Surface (wraps terraform-config-inspect)
    sourceresolve/                ACL: source string -> local path | remote (unknown) (wraps go-getter Detect)
    fswatcher/ notifier/ indexstore/ ipc/
  interfaces/
    cli/                          cobra
    presenter/                    human · json · sarif
```

**Reference validation:**

1. **`internal/` as an import barrier is a real, tooling-enforced mechanism, not just convention.** Verified directly against Go's own documentation (`go.dev/doc/go1.4#internalpackages`): *"When the `go` command sees an import of a package with `internal` in its path, it verifies that the package doing the import is within the tree rooted at the parent of the `internal` directory."* This is enforced by the `go` command itself since Go 1.4 (main repo) / Go 1.5 (any repo), not a linter opinion. It correctly prevents code *outside gruntled's module* from importing any of these packages — but it does **not**, by itself, stop `internal/domain` from importing `internal/infrastructure/tfsurface` or `hcl/v2` directly, since both are inside the same tree. Recommendation: add a cheap CI check (`go list -deps ./internal/domain/...` grepped for `hcl`, `terraform-config-inspect`, `go-getter`, `fsnotify` — fail the build if any appear) to make the "domain must not import HCL" rule structurally enforced, not just documented. This is a two-line CI job, consistent with the project's stated preference for simple, correct CI rather than elaborate pipelines.

2. **`cmd/<binary>/` as a thin composition root, `internal/` for everything else** is the layout used by `kubectl` and `docker cli` — `cmd/kubectl/kubectl.go` and `cmd/docker/docker.go` are both short `main` functions that call into a library package (`k8s.io/kubectl/pkg/cmd`, `github.com/docker/cli/cli/command`) and do nothing else. `gruntled`'s `cmd/gruntled/main.go` as "the only point that wires adapters together" matches this precisely.

3. **DDD/hexagonal `domain/ application/ infrastructure/` layering with ports as interfaces owned by `application/`** is exactly the structure of `ThreeDotsLabs/wild-workouts-go-ddd-example` (verified: `internal/trainings/{domain,app,adapters,ports,service}` on GitHub), a widely cited Go DDD/Clean-Architecture reference with an accompanying series of articles. One structural difference worth calling out rather than copying blindly: wild-workouts is a **multi-service** system, so it repeats this `{domain,app,adapters,ports}` quartet **per bounded context**, each in its own Go module (`internal/trainings/go.mod`, `internal/trainer/go.mod`, `internal/users/go.mod`) glued together as separate deployables. gruntled is a **single binary** with exactly two bounded contexts (Terragrunt Wiring core, Module Surface supporting) that never deploy separately — so flattening to one `domain/ application/ infrastructure/ interfaces/` tree at the top, with the two contexts expressed as *subpackages* (`domain/repograph` for the core, `infrastructure/tfsurface` for the supporting context's only HCL-touching adapter) rather than as separate modules, is the *correct* simplification for this scale, not a shortcut. Don't introduce per-context Go modules — that's solving a multi-service deployment problem gruntled doesn't have.

4. **Contrast worth noting honestly:** HashiCorp's own tools (`terraform-config-inspect` itself, `terraform`, `terragrunt`) do **not** use DDD/hexagonal layering — they use pragmatic package-by-feature layouts with no enforced domain/infrastructure boundary. gruntled's choice to go hexagonal is a deliberate, stated architectural constraint from `PROJECT.md` ("the domain layer must not import HCL"), not the Go-ecosystem default. That's fine — it's justified here because HCL parsing is genuinely a swappable adapter (terraform-config-inspect could be replaced by raw `hcl/v2` later without touching `domain/` or `application/` if the CI import-check above is in place) — but it means gruntled can't lean on "this is how the ecosystem does it" as a justification; the justification is the specific "domain must not import HCL" invariant the project chose.

5. **Recommended addition to §5.2's port list:** add `ports.ModuleResolver` (source string + unit directory → `ModuleRef{Kind: Local|Remote, Path}`) as its own named port, implemented by `infrastructure/sourceresolve` (§A.5 above). The current three named ports (`ConfigParser`, `SurfaceReader`, `FileWatcher`) conflate "parse `terragrunt.hcl`" with "decide whether `source` is even readable" — these are genuinely different responsibilities (one needs `go-getter`/string logic, the other needs `terraform-config-inspect`) and keeping them as separate ports keeps each adapter and its test suite small and independently replaceable, consistent with the rest of the design's granularity.

### B.7 — Suggested build order for v0.1

Ordered by actual dependency, not by the milestone checklist order. Steps in the same numbered tier have no dependency on each other and can be built in either order (or in parallel).

**Tier 0 — pure domain types (zero external dependencies, build and test first):**
`internal/domain/repograph` (Unit, Module, Surface, Reference VOs, RepositoryGraph aggregate skeleton with explicit-sort invariants baked in from day one — guarantee ① in the design doc depends on this being right from the start, not retrofitted) and `internal/domain/diagnostic` (Diagnostic VO, stable DiagnosticKey, Set with a pure Diff). Nothing here imports HCL, `go-getter`, or the filesystem.

**Tier 1 — de-risk the "no test corpus" problem immediately, in parallel with Tier 2/3:**
A **minimal** synthetic-repo generator (see §B.8) producing a handful of fixtures: one clean unit, one unit with a bad `dependency.X.outputs.Y`, one with an HCL syntax error. `PROJECT.md` flags "no local test corpus" as a real risk ("the largest Terragrunt repository on the developer's machine has one unit") — building this first, even in skeleton form, means every subsequent tier has real fixtures to test against instead of ad hoc hand-written HCL snippets duplicated across packages.

**Tier 2 — isolated adapters, each testable alone (depends only on Tier 0's VOs as their output type):**
- `infrastructure/tfsurface` wrapping terraform-config-inspect (§A.1-A.4). Unit-tested directly against `.tf` fixtures from Tier 1 — no `terragrunt.hcl` needed.
- `infrastructure/sourceresolve` wrapping `go-getter.Detect` (§A.5). Unit-tested against string-only table tests (no filesystem needed for the classification logic; `os.Stat` guard tested separately with a temp dir).

**Tier 3 — domain services (depends only on Tier 0, can be built in parallel with Tier 2):**
`domain/analysis` — GRT001 written and tested against **hand-built** `RepositoryGraph` fixtures (construct the aggregate directly in Go, no parsing involved at all). GRT100 is not really a graph-walking analyzer — it's HCL syntax diagnostics that fall out of `ConfigParser` during Tier 4; the `analysis` package should just merge parser-produced diagnostics with computed ones rather than re-deriving syntax errors. This tier is genuinely independent of Tiers 2 and 4 and is the actual "core value" of the product — worth having a green test for it as early as possible even before the wiring exists to feed it real data.

**Tier 4 — the real integration risk, do this after 2/3 exist so there's something to validate it against immediately:**
`infrastructure/terragrunt` — ACL parsing `terragrunt.hcl` + shared includes into domain `Unit` objects via the `gruntwork-io/terragrunt` library (per the M1 key decision), wired to `ports.ModuleResolver` for `source =`. Do the go-getter-vs-terragrunt-library-internals spike (§A.5, open question) at the start of this tier, not before — it only needs answering once real `terragrunt.hcl` parsing is underway.

**Tier 5 — orchestration:**
`application/indexing` use case: `ConfigParser` → `ModuleResolver` → `SurfaceReader` (for local modules only; remote/unresolvable sources mark the unit `unknown` and skip surface reading) → assemble `RepositoryGraph` → run `domain/analysis` → produce `(RepositoryGraph, diagnostic.Set)`. This is the first point all the ports come together; it should have almost no logic of its own beyond sequencing, since the interesting logic already lives in Tiers 0/2/3/4.

**Tier 6 — surface:**
`interfaces/cli` (`gruntled check`), `interfaces/presenter` (human formatter — JSON/SARIF are explicitly Later), `cmd/gruntled/main.go` wiring concrete adapters to ports. Exit codes and stdout/stderr separation (design doc §8) belong here.

**Tier 7 — scale up and validate the milestone's actual success criterion:**
Grow the Tier 1 generator to the full N-unit / nested-include / dependency-graph shape (§B.8) and add the benchmark harness (design doc §9: `gruntled check` timed against `terragrunt hcl validate --inputs --strict` as the comparison baseline). Note: shelling out to the real `terragrunt` binary is fine **inside this dev/benchmark test suite** — the "no external processes at runtime" constraint applies to the shipped product, not to the developer-facing benchmark tooling that establishes the speed claim. Finish with validation against one real public Terragrunt repository — the actual v0.1 success criterion in `PROJECT.md`, and the last thing to attempt, since everything before it exists specifically to make that one real-world run trustworthy.

Dependency summary as a one-line graph:
```
domain (Tier 0)
  ├─→ tfsurface, sourceresolve (Tier 2, adapters only depend on domain VOs as output types)
  ├─→ analysis (Tier 3, pure domain service)
  └─→ terragrunt ACL (Tier 4, needs sourceresolve + terragrunt-lib)
         └─→ indexing use case (Tier 5, needs all three adapters + analysis)
                └─→ cli + presenter + cmd/ (Tier 6)
                       └─→ full-scale generator + benchmark + real-repo validation (Tier 7)
generator (Tier 1, minimal version needed by Tiers 2-4's tests; grown fully in Tier 7)
```

### B.8 — Synthetic repo generator: one structure, two purposes

**Design: a deterministic, spec-driven builder package, consumed two different ways depending on scale.**

```go
package synthrepo // internal/testsupport/synthrepo

type Spec struct {
    Units             int
    IncludeDepth      int          // nesting levels of shared include files
    DependencyFanout  int          // avg dependency{} blocks per unit
    InjectErrors      []ErrorKind  // BadOutputRef, SyntaxError, CyclicDependency, UnresolvableRemoteSource, ...
    Seed              int64        // deterministic PRNG seed — same Spec+Seed => byte-identical tree
}

type Manifest struct {
    UnitPaths         []string
    ExpectedDiagnostics []ExpectedDiagnostic // {UnitPath, Code, ...} — golden-test oracle
}

func Generate(spec Spec, destDir string) (Manifest, error)
```

Key design choices, each justified by a concrete downstream consumer:

- **Determinism (seeded PRNG, explicit sort of every generated collection) is not optional.** It mirrors guarantee ① from the design doc (indexing the same repo twice is byte-identical) and means the generator itself can be golden-tested — dogfooding the project's own idempotency principle.
- **The `Manifest` returned alongside the generated tree is the golden-test oracle**, not a hand-maintained `.golden` file. For generated fixtures, the test is "run `gruntled check` against the generated tree, compare its `diagnostic.Set` to `Manifest.ExpectedDiagnostics`" — this scales to hundreds of injected-error fixtures without hundreds of hand-written `.golden` files. Small, illustrative **hand-written** fixtures (the three called out in Tier 1, plus a cyclic-dependency case and an unresolvable-remote-source case per design doc §9) still get plain `testdata/*.golden` files as today — the generator doesn't replace those, it complements them for the combinatorial cases (N units, N error kinds) that don't need bespoke authorship.
- **Two consumption modes, same `Generate` function:**
  - **Golden tests:** small `Spec` (≤20 units), generated once via a `go:generate` directive or a `make fixtures` target, **committed to `testdata/golden/`**. Keeps `go test ./...` fast and avoids flaky/slow generation inside the normal test loop.
  - **Benchmarking:** large `Spec` (N=100s-1000s of units, deep `IncludeDepth`, `InjectErrors: nil` for a clean happy-path throughput measurement), generated **on the fly into a temp dir** inside a `go test -bench` benchmark function — never committed to git, since these trees are throughput probes, not correctness oracles, and would otherwise bloat the repository.
- **Package location:** `internal/testsupport/synthrepo`, importable from any `_test.go` file across the module (test-only code, correctly excluded from the shipped binary via normal Go build rules — `_test.go` files never compile into the release artifact regardless of package name, but placing it outside `domain/ application/ infrastructure/ interfaces/` also keeps it from ever being mistaken for production wiring). If a standalone CLI wrapper is wanted for manual repro or ad hoc benchmarking outside `go test` (`go run ./tools/gen -units 500`), add a thin `tools/gen/main.go` that imports `synthrepo` — this is a separate `go build` target within the same module, not something the shipped `gruntled` binary links against, so it doesn't compromise the single-binary guarantee.

---

## Sources

**Primary (fetched/read directly, HIGH confidence):**
- `github.com/hashicorp/terraform-config-inspect` — `go.mod`, `LICENSE`, `tfconfig/variable.go`, `tfconfig/output.go`, `tfconfig/load.go`, `tfconfig/load_hcl.go` (raw source, fetched 2026-09-01)
- GitHub API: `/repos/hashicorp/terraform-config-inspect` (archived/license/activity), `/repos/hashicorp/terraform-config-inspect/tags`, `/repos/hashicorp/terraform-config-inspect/commits`
- `pkg.go.dev/github.com/hashicorp/terraform-config-inspect/tfconfig` — published version, API surface
- `pkg.go.dev/github.com/hashicorp/hcl/v2/hclparse` — `NewParser`, `ParseHCLFile`, `ParseHCL` signatures
- `pkg.go.dev/github.com/hashicorp/go-getter#Detect` — `Detect` signature and pure-function behavior
- `go.dev/doc/go1.4#internalpackages` — `internal/` tooling-level enforcement
- `github.com/ThreeDotsLabs/wild-workouts-go-ddd-example` (GitHub API tree listing of `internal/trainings/`)

**Secondary (WebSearch/WebFetch summaries, cross-checked against primary sources above where load-bearing, MEDIUM confidence):**
- `docs.terragrunt.com/reference/config-blocks-and-attributes` (fetched 2026-09-01; note old `terragrunt.gruntwork.io` URL now 308-redirects here) — `source` attribute syntax, `tfr://` and `oci://` protocols
- `developer.hashicorp.com/terraform/language/modules/sources` (via WebSearch) — local-path-must-start-with-`./`-or-`../` rule
- DeepWiki `gruntwork-io/terragrunt` pages — Terragrunt's use of `go-getter` for source download (secondary characterization, not verified against Terragrunt's own Go source directly — flagged as MEDIUM confidence, not primary-verified)

**UNVERIFIED (explicitly flagged, low practical risk for v0.1):**
- Whether `terraform-config-inspect` captures Terraform 1.10+ `ephemeral` variable/resource markers — not found in the reviewed source, not confirmed absent either.
- Whether the `gruntwork-io/terragrunt` library exposes its own internal source-classification function reusable without invoking download — flagged as a short implementation-time spike in §A.5/Tier 4, not resolved by this research pass.

---
*Architecture research for: gruntled (Go DDD/hexagonal Terragrunt static analyser)*
*Researched: 2026-09-01*
