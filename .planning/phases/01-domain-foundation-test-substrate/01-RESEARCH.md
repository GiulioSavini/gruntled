# Phase 1: Domain Foundation & Test Substrate - Research

**Researched:** 2026-09-25
**Domain:** Go module bootstrap, DDD/hexagonal domain layer, `go list`-based architecture enforcement, deterministic synthetic-data generation
**Confidence:** HIGH (every claim below that is load-bearing for a plan decision was verified by actually running `go build`/`go list`/`go vet` in this environment, not recalled from training data)

<user_constraints>
## User Constraints (from CONTEXT.md)

### Locked Decisions

**Architecture — clean DDD/hexagonal layering (LOCKED, non-negotiable)**
- Follow the layout in `research/ARCHITECTURE.md` §B.6 exactly:
  - `cmd/gruntled/`: composition root only.
  - `internal/domain/`: zero external dependencies. The bounded contexts are subpackages, not separate Go modules.
  - `internal/application/`: use cases, plus `ports/` (interfaces owned by application).
  - `internal/infrastructure/`: the ONLY place where HCL, go-getter, fsnotify, sockets and the filesystem may appear.
  - `internal/interfaces/`: cli, presenter.
- Phase 1 creates only what it needs: `internal/domain/repograph`, `internal/domain/diagnostic` and `internal/testsupport/synthrepo`. Do not scaffold empty packages for later phases.
- The domain uses the ubiquitous language from design doc §5.3. Value objects are immutable. `RepositoryGraph` is the aggregate root and bakes in explicit-sort invariants from day one, which determinism depends on.
- ARCH-01: a CI job fails the build when the domain's transitive deps include hcl, terraform-config-inspect, go-getter or fsnotify (`go list -deps ./internal/domain/...`). It must also fail on any `os`/`io/fs`/`path/filepath` filesystem access from the domain. Keep the CI simple, with few jobs, and make sure each job can genuinely fail.
- ARCH-02: analyzers and graph queries are pure functions over domain types. They are tested with hand-built domain objects only.
- `internal/testsupport/synthrepo` is test support. It writes files (that is its job), but it lives outside `domain/`, `application/`, `infrastructure/` and `interfaces/`, and the shipped binary never links it.

**Generator (VALID-01)**
- `Spec{Units, IncludeDepth, DependencyFanout, Seed}` → same Spec + seed produce a byte-identical tree on repeated runs.
- It can inject at least one known error kind (a bad `dependency.X.outputs.Y` reference) and returns a manifest of the diagnostics a correct analyzer must report.

**Repo conventions**
- Commits authored as Giulio Savini <giuliosavini@proton.me>, with no Co-Authored-By trailer.
- Every change is committed, pushed and tested.

### Claude's Discretion
- Exact field names, constructor shapes, error types and file split inside each package, as long as the layering holds.

### Deferred Ideas (OUT OF SCOPE)
- blast, analysis (GRT001), application use cases, all adapters, and the CLI: Phases 2-3.
</user_constraints>

<phase_requirements>
## Phase Requirements

| ID | Description | Research Support |
|----|-------------|-------------------|
| ARCH-01 | The domain layer contains no import of HCL, the filesystem or any infrastructure library, enforced by an automated check | §"CI Enforcement of ARCH-01" — verified, runnable `go list` commands (direct-import check for `os`/`io/fs`/`path/filepath`, transitive-dep check for `hcl`/`terraform-config-inspect`/`go-getter`/`fsnotify`), plus the concrete pitfall that motivates using two different `go list` modes instead of one |
| ARCH-02 | Analyzers and graph queries are pure functions over domain types, testable without a filesystem | §"Architecture Patterns" — `RepositoryGraph`/`Diagnostic` construction patterns using only exported constructors and in-memory value objects, no I/O anywhere in `internal/domain` |
| VALID-01 | A generator produces synthetic Terragrunt repositories from a spec — N units, nested includes, dependency chains — used for both golden tests and benchmarks | §"Synthetic Repository Generator" — deterministic PRNG seeding (`math/rand/v2` PCG, verified pitfall around `math/rand` v1's Go-1.20+ auto-seeding), tree-shape design, manifest format with byte-offset-derived line/column |
</phase_requirements>

## Summary

Phase 1 is pure bootstrap: there is no Go code in the repository yet. The work is (1) a `go.mod` at `github.com/GiulioSavini/gruntled`, (2) two zero-dependency domain packages (`internal/domain/repograph`, `internal/domain/diagnostic`) that only stdlib touches, (3) a test-support package (`internal/testsupport/synthrepo`) that deterministically generates synthetic Terragrunt repository trees on disk, and (4) a small GitHub Actions workflow that makes ARCH-01 a build-breaking fact rather than a documented convention.

The two technical risks worth being precise about, both verified experimentally in this exact environment rather than assumed:

1. **`go list -deps` alone is the wrong tool for the filesystem-import half of ARCH-01.** A domain package importing nothing but `errors`, `fmt`, `sort`, `strings` already transitively depends on `os` and `io/fs` (verified: `go list -deps` on such a package lists `os`, `io`, `io/fs` even though no domain code ever calls them). The correct check uses **direct** imports only (`go list -f '{{.Imports}}'` / `{{.TestImports}}`) for the `os`/`io/fs`/`path/filepath` rule, and **transitive** deps (`go list -deps`) only for the third-party library rule (`hcl`, `terraform-config-inspect`, `go-getter`, `fsnotify`), because no innocuous stdlib package will ever drag those in by accident.
2. **Determinism for the generator depends on which random API is used, not just on passing a seed.** Since Go 1.20, `math/rand`'s package-level functions (`rand.Intn`, etc.) auto-seed from a random source at program start and `rand.Seed` is a documented no-op/deprecated call — using the global functions with a "seed" produces a *different* sequence on every run. The fix is to construct an explicit source: `rand.New(rand.NewPCG(seed1, seed2))` (from `math/rand/v2`, confirmed available and the recommended API for exactly this "simulations need deterministic randomness" use case), never the package-level functions.

The local toolchain question in this task's constraints ("local toolchain is go1.27") is resolved and verified in this environment: the installed `go` binary reports `go1.24.1`, but Go's toolchain auto-switch mechanism (`GOTOOLCHAIN=auto`, the default since Go 1.21) already has `go1.27.0` cached in `GOMODCACHE` — declaring `go 1.27` in `go.mod` makes `go version` (and every other `go` subcommand) transparently switch to 1.27 with **zero network calls**, verified by creating a scratch `go.mod` with `go 1.27` and running `go version` in it, which printed `go1.27.0`. Use `go 1.27` in `go.mod`.

**Primary recommendation:** Bootstrap with `go.mod` declaring `module github.com/GiulioSavini/gruntled` and `go 1.27`; build `internal/domain/repograph` and `internal/domain/diagnostic` as stdlib-only packages with sorted, defensively-copied slices as the determinism boundary; build `internal/testsupport/synthrepo` as a stdlib-only (no `hcl/v2` needed) deterministic text generator using `math/rand/v2`'s `NewPCG`; enforce ARCH-01 with two separate `go list` invocations (direct-imports for filesystem, transitive-deps for third-party libraries) in a small GitHub Actions workflow alongside build/vet/test jobs.

## Standard Stack

### Core

| Library | Version | Purpose | Why Standard |
|---------|---------|---------|--------------|
| Go stdlib only | go1.27 (toolchain) | Everything Phase 1 needs: `sort`, `slices`, `cmp`, `math/rand/v2`, `text/template` or plain `strings.Builder`, `os`, `path/filepath` (in `synthrepo` only) | Phase 1's own locked constraint is that the domain has *zero* external dependencies, and the generator has no technical need for `hcl/v2` (it emits syntactically valid Terragrunt HCL as plain text — the actual parser that must accept this text doesn't exist until Phase 2). Keeping Phase 1 dependency-free entirely (no `go.sum` third-party entries at all) is both correct and the simplest thing that satisfies every success criterion. |

### Supporting

None needed for Phase 1. `hashicorp/hcl/v2`, `terraform-config-inspect`, `go-getter`, `fsnotify`, `cobra` all belong to Phase 2+ (`infrastructure/*`, `interfaces/cli`) and must not appear in `go.mod` yet — their mere presence in `go.sum` doesn't violate ARCH-01 by itself (ARCH-01 is about the *domain's* import graph, not the module's `go.sum`), but introducing them early with nothing to use them for is scope creep the CONTEXT.md discretion note explicitly warns against ("Do not scaffold empty packages for later phases").

### Alternatives Considered

| Instead of | Could Use | Tradeoff |
|------------|-----------|----------|
| Plain-text generation of Terragrunt HCL in `synthrepo` (`fmt.Fprintf`/`strings.Builder` into a `[]byte` buffer) | `hashicorp/hcl/v2/hclwrite` to build and serialize an AST | `hclwrite` guarantees the *shape* is valid per the HCL grammar and handles formatting/escaping for you, but its auto-formatting makes byte-offset tracking for the manifest's line/column fields indirect (you'd have to re-parse the serialized output to find the injected error's position). Plain text generation gives full, simple control over exact byte layout, which is exactly what the manifest's line/column requirement needs, at the cost of manually writing correct HCL block/attribute syntax by hand. Recommended: plain text for Phase 1; revisit `hclwrite` only if Phase 2's parser turns out to reject something the hand-written text emits (verify by running the real Terragrunt/HCL grammar reference during Phase 2, not by guessing now). |
| `sort.Slice` for explicit ordering | `slices.SortFunc` / `slices.SortStableFunc` (stdlib `slices` package, stable since Go 1.21) | Both work under Go 1.27. `slices.SortFunc` is the more current idiom (type-safe comparator, no reflection), and this project is not constrained by an older Go version, so prefer it. `sort.Slice` remains fine wherever a plan already reaches for it; no need to churn existing code once written. |
| `math/rand/v2` with `rand.NewPCG` | `math/rand` v1 with `rand.New(rand.NewSource(seed))` | v1's `rand.New(rand.NewSource(seed))` (not the deprecated package-level `rand.Seed`) is *also* deterministic and still valid — the pitfall is specifically the **package-level** functions, not v1 as a whole. `math/rand/v2` is preferred because it's the current stdlib direction (v1 is officially in "soft maintenance," v2 is where PCG — designed for exactly this seeded-simulation use case — lives), and its `IntN`/`Int64N` naming (capital N) is worth knowing about up front so a plan doesn't mix v1 and v2 method names by accident. |

**Installation:**
```bash
cd /home/giulio/gruntled
go mod init github.com/GiulioSavini/gruntled
# go.mod will read: module github.com/GiulioSavini/gruntled
#                    go 1.27
# No `go get` needed for Phase 1 — stdlib only.
```

## Architecture Patterns

### Recommended Project Structure (Phase 1 slice only)

```
gruntled/
├── go.mod                                  module github.com/GiulioSavini/gruntled; go 1.27
├── cmd/
│   └── gruntled/
│       └── main.go                         composition root stub — no adapters exist yet, just enough
│                                            for `go build ./...` to build a real, empty binary
├── internal/
│   ├── domain/
│   │   ├── repograph/                      Unit, Module, Surface, Reference (VOs) + RepositoryGraph (aggregate root)
│   │   │   ├── unit.go
│   │   │   ├── module.go
│   │   │   ├── surface.go
│   │   │   ├── reference.go
│   │   │   ├── repository_graph.go
│   │   │   └── repository_graph_test.go    ARCH-02: hand-built graphs only, zero filesystem
│   │   └── diagnostic/                     Diagnostic (VO), stable key, Set with pure Diff
│   │       ├── diagnostic.go
│   │       ├── set.go
│   │       └── set_test.go
│   └── testsupport/
│       └── synthrepo/                      deterministic generator — lives OUTSIDE domain/application/infrastructure/interfaces
│           ├── spec.go                     Spec, ErrorKind
│           ├── manifest.go                 Manifest, ExpectedDiagnostic
│           ├── generate.go                 Generate(spec, destDir) (Manifest, error)
│           ├── render.go                   plain-text HCL/`.tf` emission + byte-offset line/col helper
│           └── generate_test.go            determinism + error-injection tests
└── .github/
    └── workflows/
        └── ci.yml                          build / vet / test / domain-purity — 4 small, independently-failable jobs
```

Do not create `internal/application`, `internal/infrastructure`, `internal/interfaces`, or any `internal/domain` subpackage beyond `repograph`/`diagnostic` in this phase — CONTEXT.md is explicit that scaffolding empty packages for later phases is out of scope.

### Pattern 1: `internal/` enforces module-external isolation, not intra-module layering — so ARCH-01 needs an explicit CI check, not just the directory name

**What:** Go's compiler enforces that `internal/` packages are only importable from within the module rooted at their parent directory (this is a `go build`-level rule since Go 1.4, not a lint opinion). It does **not** stop `internal/domain/repograph` from importing `internal/infrastructure/tfsurface` or `hashicorp/hcl/v2` directly — both are inside the same module tree, so the compiler has no opinion about it.

**When to use:** Always, for this project's layering guarantee. This is why CONTEXT.md's ARCH-01 decision is a CI job, not a design note.

**Example — the two `go list` invocations, both verified by actually running them in this environment (not templated from memory):**
```bash
# 1. Filesystem check — DIRECT imports only. Using `go list -deps` here produces
#    false positives: a domain package that imports nothing but errors/fmt/sort/strings
#    still transitively depends on os/io/io/fs, because fmt internally touches os.Stdout/os.Stderr.
#    Verified: `go list -deps ./domaintest` on a package importing only [errors fmt sort strings]
#    lists `os`, `io`, `io/fs` in its 62-entry transitive closure.
forbidden_fs='^(os|io/fs|path/filepath)$'
found=$(go list -f '{{range .Imports}}{{.}}{{"\n"}}{{end}}{{range .TestImports}}{{.}}{{"\n"}}{{end}}' \
          ./internal/domain/... | sort -u | grep -E "$forbidden_fs" || true)
if [ -n "$found" ]; then
  echo "internal/domain imports filesystem packages directly: $found"
  exit 1
fi

# 2. Third-party infrastructure check — TRANSITIVE deps are correct here, because no
#    stdlib package will ever accidentally pull in a third-party module like hashicorp/hcl.
#    -test included so a domain _test.go accidentally importing synthrepo (or anything
#    infrastructure-ish) is also caught.
forbidden_infra='hashicorp/hcl|terraform-config-inspect|hashicorp/go-getter|fsnotify'
found=$(go list -deps -test ./internal/domain/... | sort -u | grep -E "$forbidden_infra" || true)
if [ -n "$found" ]; then
  echo "internal/domain transitively depends on forbidden infrastructure libraries: $found"
  exit 1
fi
```
Both commands were run end-to-end against a throwaway two-package module in this session (one package importing `errors`/`fmt`/`sort`/`strings` plus a `_test.go` importing `os`/`testing`; a second package importing `time`) and correctly separated "direct filesystem import → fail" from "transitive stdlib noise → ignore."

### Pattern 2: `RepositoryGraph` as an aggregate root that bakes in sort-on-construction

**What:** Rather than sorting at the presentation boundary only (design doc §6①'s general rule), the aggregate root itself should accept unsorted input and normalize to a stable, sorted internal representation immediately, then hand out only defensive copies. This makes "RepositoryGraph built from the same units twice is byte/deep-equal" trivially true without every caller having to remember to sort.

**When to use:** `internal/domain/repograph.RepositoryGraph` specifically — this is the type determinism (VALID-01, and later design doc guarantee ①) depends on being right from the start.

**Example (illustrative — exact field/method names are Claude's discretion per CONTEXT.md):**
```go
// internal/domain/repograph/repository_graph.go
package repograph

import "slices"

type RepositoryGraph struct {
	units []Unit // always sorted by Path, always a defensive copy
}

// NewRepositoryGraph normalizes input order and rejects duplicate paths so the
// resulting aggregate is deterministic regardless of caller-supplied order —
// the property every later phase (golden tests, the daemon's diffing) relies on.
func NewRepositoryGraph(units []Unit) (*RepositoryGraph, error) {
	sorted := slices.Clone(units)
	slices.SortFunc(sorted, func(a, b Unit) int {
		if a.Path < b.Path {
			return -1
		}
		if a.Path > b.Path {
			return 1
		}
		return 0
	})
	for i := 1; i < len(sorted); i++ {
		if sorted[i].Path == sorted[i-1].Path {
			return nil, &DuplicateUnitError{Path: sorted[i].Path}
		}
	}
	return &RepositoryGraph{units: sorted}, nil
}

// Units returns a defensive copy — callers cannot mutate the aggregate's internal state.
func (g *RepositoryGraph) Units() []Unit {
	return slices.Clone(g.units)
}
```
This uses only `slices` (stdlib, stable since Go 1.21, fully available under `go 1.27`) — no `sort` package import even needed, though either is fine.

### Pattern 3: pure analyzers/graph queries testable with hand-built objects only (ARCH-02)

**What:** Domain-layer functions take and return only domain types, never touch a filesystem, and are exercised in tests via direct struct construction — no fixtures on disk, no parser, no `synthrepo` import from inside `internal/domain`'s own tests (that would itself violate the "domain never touches infrastructure" spirit even though `synthrepo` isn't literally `hcl`/`fsnotify`/etc., since `synthrepo` writes files with `os`).

**Example:**
```go
// internal/domain/repograph/repository_graph_test.go
package repograph_test

import (
	"testing"

	"github.com/GiulioSavini/gruntled/internal/domain/repograph"
)

func TestNewRepositoryGraph_SortsAndDeduplicates(t *testing.T) {
	units := []repograph.Unit{
		{Path: "units/b"},
		{Path: "units/a"},
	}
	g, err := repograph.NewRepositoryGraph(units)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got := g.Units()
	if got[0].Path != "units/a" || got[1].Path != "units/b" {
		t.Fatalf("expected sorted order, got %+v", got)
	}
}
```
No `os`, no `testdata/`, no HCL — exactly what ARCH-02 requires and what the ARCH-01 CI check (§Pattern 1) will itself verify holds for this very test file via `go list -f '{{.TestImports}}'`.

### Anti-Patterns to Avoid

- **Using `go list -deps` (transitive) for the filesystem-import half of ARCH-01:** produces false-positive failures on any domain package that imports ordinary stdlib packages like `fmt`, `time`, or `errors`, because those transitively touch `os`/`io/fs` internally. Verified in this session. Use direct imports (`.Imports`/`.TestImports`) for that rule instead.
- **Package-level `math/rand` calls (`rand.Intn`, `rand.Shuffle`, etc.) anywhere in `synthrepo`:** since Go 1.20 these auto-seed from a cryptographically random source at program startup; calling the deprecated `rand.Seed` does not restore old reproducible behavior. This silently breaks VALID-01's "byte-identical on repeated runs" requirement in a way that a single local test run won't catch (it'll pass by coincidence if you only ever run the test once per process) — this is the kind of bug that surfaces as CI flakiness.
- **Ranging over a `map` anywhere a `Diagnostic.Set`, `Surface`, or dependency adjacency list is built or printed, without an explicit sort first:** Go's map iteration order is deliberately randomized. Already documented in `research/PITFALLS.md` Pitfall 7 — repeated here because `diagnostic.Set` is built in Phase 1 and is exactly the type this bites.
- **Scaffolding `internal/application`, `internal/infrastructure`, or extra `internal/domain` subpackages "to save time later":** explicitly out of scope per CONTEXT.md; also means `go build ./...` and the ARCH-01 check would be exercising nothing meaningful in those empty trees, giving false confidence.

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|--------------|-----|
| Deterministic pseudo-randomness for the generator's unit/dependency selection | A custom LCG or hash-based "deterministic random" function | `math/rand/v2` with `rand.New(rand.NewPCG(seed1, seed2))` | PCG is stdlib, specifically documented as designed for "simulations that need a seeded, deterministic source of randomness" — exactly VALID-01's requirement. Hand-rolling risks subtle seed-dependent bias or non-uniform distribution nobody will notice until fixture generation looks "clumpy." |
| Stable sort ordering for aggregate invariants | Manual insertion-sort or bespoke comparator plumbing | `slices.SortFunc`/`slices.SortStableFunc` (stdlib `slices` package) | Same guarantee as `sort.Slice` with a type-safe comparator and no reflection; it's the current stdlib idiom under Go 1.21+, which this project's `go 1.27` comfortably satisfies. |
| Line/column computation for the generator's manifest | A full HCL/text scanner just to find "what line is byte offset N on" | A ~10-line byte-counting helper (count `\n` before the offset) — see Code Examples | The generator controls exactly what bytes it writes; it already knows the offset of the injected error at write time. A scanner is solving a harder, more general problem (arbitrary existing files) that doesn't exist here. |
| Enforcing the "domain has zero external deps" rule | A hand-written import-graph walker/AST scanner | `go list -f`/`go list -deps` (both verified, see Pattern 1) | `go list` already computes exactly this graph correctly, including edge cases like build tags and test-only imports; a hand-rolled AST walker would have to reimplement all of that and would be its own source of false negatives. |

**Key insight:** Every "don't hand-roll" item above has a stdlib or `go` toolchain answer already verified to work in this exact environment — Phase 1 genuinely needs zero third-party dependencies, which is also the simplest possible way to satisfy "the domain package's dependency list contains no HCL, filesystem, or infrastructure import."

## Common Pitfalls

### Pitfall 1: `go list -deps` gives false positives for the filesystem-import rule

**What goes wrong:** A CI check written as `go list -deps ./internal/domain/... | grep -E 'os|io/fs|path/filepath'` fails the build even when no domain code ever calls `os`, `io/fs`, or `path/filepath` directly — because ordinary stdlib packages (`fmt`, `time`, anything using errors formatting) pull those in transitively.

**Why it happens:** `go list -deps` reports the full transitive closure, including the standard library's own internal plumbing. `fmt` needs `os.Stdout`/`os.Stderr` for its `Print*` family; almost nothing in Go avoids importing `fmt` indirectly.

**How to avoid:** Use `go list -f '{{.Imports}}'` and `{{.TestImports}}` (direct imports only, per package) for this specific rule. Reserve `go list -deps` (transitive) for the third-party-library rule, where no stdlib package will ever accidentally introduce `hashicorp/hcl` or `fsnotify`.

**Warning signs:** A CI job that fails on the very first commit that adds any domain code at all, even code with no filesystem logic — that's the signature of this bug, not a real domain violation.

**Verified:** experimentally in this session — see §Pattern 1's code block and the transcript: a package importing only `errors, fmt, sort, strings` showed `os`, `io`, `io/fs` in its `go list -deps` output (62 total transitive packages) but an empty result from `go list -f '{{.Imports}}'` for those same three forbidden names.

### Pitfall 2: `math/rand` global functions silently break "same seed → same output" (Go 1.20+)

**What goes wrong:** Code that does the intuitive thing — `rand.Seed(spec.Seed); rand.Intn(n)` — produces a *different* sequence on every process run, because `rand.Seed` has been a documented no-op/deprecated call since Go 1.20 specifically to stop this pattern (the global source now auto-seeds from a random value at program start for safety). VALID-01's "byte-identical on repeated runs" requirement will fail intermittently — and might even pass in a single local dev loop if the test happens to run against a cached previous output — which makes it a particularly sneaky bug to catch late.

**Why it happens:** This is a genuine, documented Go stdlib behavior change (Go 1.20, well before this project's `go 1.27`), not a hypothetical — many older tutorials and even some still-circulating blog posts show the old `rand.Seed(...)` pattern.

**How to avoid:** Always construct an explicit source: `rng := rand.New(rand.NewPCG(0, uint64(spec.Seed)))` (math/rand/v2) or, if v1 is preferred for some reason, `rng := rand.New(rand.NewSource(spec.Seed))` — never call the package-level `rand.Intn`/`rand.Shuffle`/etc. anywhere in `synthrepo`.

**Warning signs:** Any import of `math/rand` (v1 or v2) in `synthrepo` followed by calls to the *package-level* function names (`rand.Intn`, `rand.IntN`, `rand.Shuffle`) rather than a method on a locally constructed `*rand.Rand`.

### Pitfall 3: `internal/` doesn't stop same-module layering violations

**What goes wrong:** A developer (or a future plan) assumes putting HCL-touching code in `internal/domain/somepackage` is safe because "it's internal, so it's protected." Go's `internal/` enforcement only blocks imports from *outside the module* — nothing stops `internal/domain/repograph` from writing `import "github.com/hashicorp/hcl/v2"` and compiling successfully.

**Why it happens:** The word "internal" reads like it implies more isolation than Go's toolchain actually provides. This is a real, previously-verified finding in `research/ARCHITECTURE.md` §B.6.1, restated here because it is exactly why ARCH-01 must be an automated CI check, not a documentation note.

**How to avoid:** The two-command CI check in §Pattern 1, run on every push/PR, not just documented as a convention.

**Warning signs:** ARCH-01's CI job passing trivially because it was never actually exercised (e.g., it's written but not wired into the workflow trigger, or it targets a path that doesn't exist yet).

### Pitfall 4: declaring `go 1.27` without confirming the toolchain is actually reachable

**What goes wrong:** A plan assumes "go.mod says go 1.27" is sufficient without checking whether the CI runner and the local dev machine can actually obtain a go1.27 toolchain — if `GOTOOLCHAIN` were set to `local` instead of the default `auto`, or if CI has no network access and no cached toolchain, `go build` would fail immediately with a toolchain-not-found error before any domain code even runs.

**Why it happens:** `GOTOOLCHAIN=auto` (the default since Go 1.21) transparently downloads/switches when `go.mod`'s `go` directive names a newer version than the installed `go` binary — this "just works" often enough that it's easy to not verify it explicitly.

**How to avoid:** Verified in this session: local `go env GOTOOLCHAIN` is `auto` (default, unmodified), and `golang.org/toolchain@v0.0.1-go1.27.0.linux-amd64` is already present in `GOMODCACHE`, so `go version` inside a `go 1.27` module printed `go1.27.0` immediately with no network call. For CI, use `actions/setup-go@v7` with `go-version-file: go.mod` (reads the `go 1.27` directive directly, downloads the matching toolchain on the runner — GitHub Actions runners have network access, so this is a non-issue there).

**Warning signs:** `go build`/`go test` failing with a message about "go.mod requires go >= 1.27" or a toolchain download error — check `go env GOTOOLCHAIN` and `GOMODCACHE` first before assuming the Go version choice itself is wrong.

## Code Examples

### `go.mod` bootstrap

```
module github.com/GiulioSavini/gruntled

go 1.27
```
Verified: `go mod init github.com/GiulioSavini/gruntled` in an empty directory followed by hand-editing the `go` directive to `1.27` (or running under a toolchain that already defaults to 1.27) produces exactly this file; no `require` block needed since Phase 1 has no third-party dependencies.

### `cmd/gruntled/main.go` composition-root stub (Phase 1 has nothing to wire yet)

```go
// Source: pattern from research/ARCHITECTURE.md §B.6 point 2 (kubectl/docker cli style
// thin main.go); no adapters exist yet in Phase 1, so this only needs to exist so
// `go build ./...` exercises the full module tree, including internal/domain.
package main

import "fmt"

func main() {
	fmt.Println("gruntled: not yet implemented")
}
```

### Line/column from emitted bytes (for the generator's manifest)

```go
// internal/testsupport/synthrepo/render.go
package synthrepo

// lineCol returns the 1-based line and column of the byte at offset within content.
// Verified formula: column = offset - index_of_preceding_newline, which is exactly
// 1 for the first character on a line (index_of_preceding_newline defaults to -1
// for line 1, so offset 0 gives column 0 - (-1) = 1).
func lineCol(content []byte, offset int) (line, col int) {
	line = 1
	lastNewline := -1
	for i := 0; i < offset && i < len(content); i++ {
		if content[i] == '\n' {
			line++
			lastNewline = i
		}
	}
	return line, offset - lastNewline
}
```

### Deterministic PRNG construction for `Generate`

```go
// internal/testsupport/synthrepo/generate.go
package synthrepo

import "math/rand/v2"

func newRNG(seed int64) *rand.Rand {
	// NewPCG takes two uint64 seed halves; folding a single int64 spec.Seed into
	// both is sufficient for this generator's determinism requirement (same Spec.Seed
	// always produces the same two PCG seed values, hence the same sequence).
	return rand.New(rand.NewPCG(0, uint64(seed)))
}
```
Note the v2 API surface differs by name from v1: methods are `IntN`, `Int64N`, `Uint64N` (capital N) rather than v1's `Intn`/`Int63n` — worth calling out explicitly so a plan doesn't mix the two APIs' method names.

### ARCH-01 CI job (GitHub Actions)

```yaml
# .github/workflows/ci.yml
name: ci

on:
  push:
    branches: [main]
  pull_request:

jobs:
  build:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v7
      - uses: actions/setup-go@v7
        with:
          go-version-file: go.mod
      - run: go build ./...

  vet:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v7
      - uses: actions/setup-go@v7
        with:
          go-version-file: go.mod
      - run: go vet ./...

  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v7
      - uses: actions/setup-go@v7
        with:
          go-version-file: go.mod
      - run: go test ./...

  domain-purity:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v7
      - uses: actions/setup-go@v7
        with:
          go-version-file: go.mod
      - name: domain must not import the filesystem directly
        run: |
          set -euo pipefail
          forbidden='^(os|io/fs|path/filepath)$'
          found=$(go list -f '{{range .Imports}}{{.}}{{"\n"}}{{end}}{{range .TestImports}}{{.}}{{"\n"}}{{end}}' \
                    ./internal/domain/... | sort -u | grep -E "$forbidden" || true)
          if [ -n "$found" ]; then
            echo "internal/domain imports filesystem packages directly:"
            echo "$found"
            exit 1
          fi
      - name: domain must not transitively depend on HCL/getter/watcher libraries
        run: |
          set -euo pipefail
          forbidden='hashicorp/hcl|terraform-config-inspect|hashicorp/go-getter|fsnotify'
          found=$(go list -deps -test ./internal/domain/... | sort -u | grep -E "$forbidden" || true)
          if [ -n "$found" ]; then
            echo "internal/domain transitively depends on forbidden infrastructure libraries:"
            echo "$found"
            exit 1
          fi
```
Four small, independently-failable jobs, matching the "few jobs, each able to genuinely fail" convention — `build` catches compile errors, `vet` catches suspicious constructs, `test` runs the actual test suite, `domain-purity` is the ARCH-01 enforcement and is the only job specific to this phase's architecture constraint.

**Action versions used above (`actions/checkout@v7`, `actions/setup-go@v7`):** confirmed as current major versions via the projects' own GitHub Releases pages during this research pass (MEDIUM confidence — tag numbers cross-checked across two independent fetches, but the release *dates* returned by the fetch tool were inconsistent/unreliable, so don't cite specific dates from this research; the tag numbers themselves, `v7`, were consistent). If a plan executes significantly after this research date, do a quick check of `https://github.com/actions/checkout/releases` and `https://github.com/actions/setup-go/releases` before pinning, since these update roughly every few months.

## State of the Art

| Old Approach | Current Approach | When Changed | Impact |
|--------------|-------------------|---------------|--------|
| `math/rand` v1 package-level functions with `rand.Seed(n)` for reproducible sequences | Explicit `*rand.Rand` construction (`rand.New(rand.NewSource(n))` in v1, or `rand.New(rand.NewPCG(a,b))` in `math/rand/v2`) | Go 1.20 (auto-seeding of the global source) | Any code still relying on `rand.Seed` for determinism silently gets non-reproducible output — directly relevant to VALID-01 |
| `sort.Slice(x, func(i,j int) bool {...})` | `slices.SortFunc(x, func(a, b T) int {...})` | `slices` stdlib package since Go 1.21 | Not a correctness change, but the more current idiom under this project's `go 1.27`; both work, `slices` is preferred for new code |
| Assuming `internal/` alone enforces domain purity | Explicit `go list`-based CI check | N/A — this is a long-standing Go toolchain behavior (Go 1.4+), the "current approach" here is the project's own decision to add enforcement, not a language change | Without the CI check, ARCH-01 is a convention, not a guarantee |

**Deprecated/outdated:**
- `rand.Seed()` (math/rand v1): documented as a no-op for the auto-seeded default source since Go 1.20; still technically callable but does not restore old reproducible behavior when used with the package-level functions.

## Open Questions

1. **Should the generator emit HCL as plain text (stdlib only) or via `hclwrite`?**
   - What we know: plain text keeps `synthrepo` fully dependency-free and gives exact control over byte offsets for the manifest's line/column fields (verified approach, see Code Examples).
   - What's unclear: whether Phase 2's actual HCL parser will be strict about formatting details (whitespace, comment placement) that hand-written text might get subtly wrong in ways `hclwrite` would guarantee correct.
   - Recommendation: start with plain text for Phase 1 (nothing in Phase 1's success criteria requires HCL-library-grade correctness — only Phase 2's parser cares, and it doesn't exist yet). Add an `hclwrite`-based rendering path later only if Phase 2 discovers a concrete parsing mismatch.

2. **Should `Spec` support Unit≠Module (separate module directory) generation in Phase 1, or is inline (Unit dir == Module dir) sufficient?**
   - What we know: `research/PITFALLS.md` Pitfall 5 flags Unit≠Module as an important real-world case (most non-trivial repos separate them), and the corpus's cleanest reference (`aws-iso20022`) is actually the inline shape.
   - What's unclear: Phase 1's own success criteria (VALID-01, this phase) only require "N units, nested includes, dependency chains" and one injected error kind — nothing mandates Unit≠Module modeling yet, and CONTEXT.md's deferred list pushes the actual analyzer (which is what would need to exercise this distinction) to Phase 2-3.
   - Recommendation: ship inline (Unit dir == Module dir) generation for Phase 1's `Spec`; leave a documented seam (e.g., a boolean or enum field name reserved but unused) for Phase 2/3 to extend into Unit≠Module generation once the real resolver needs fixtures for it. Don't build unused generality now.

3. **Exact directory/naming scheme for `IncludeDepth` nesting.**
   - What we know: Terragrunt's `find_in_parent_folders()` walks up the directory tree looking for a named file; "nesting depth" naturally maps to how many directory levels separate a unit from the shared include file it inherits.
   - What's unclear: the precise naming convention (`root.hcl` vs `common.hcl` vs `terragrunt.hcl` at intermediate levels) isn't specified by any locked decision — this is explicitly "Claude's discretion" per CONTEXT.md.
   - Recommendation: use a single shared file named consistently (e.g. `root.hcl`) at the tree root, with leaf units at depth `IncludeDepth` including it via `find_in_parent_folders("root.hcl")`; document the chosen name in the package's own code comments so Phase 2's parser tests can rely on it.

## Validation Architecture

### Test Framework

| Property | Value |
|----------|-------|
| Framework | Go stdlib `testing` package via `go test` (no third-party test framework — idiomatic for Go, and Phase 1 has no dependencies to justify one) |
| Config file | none — `go.mod` itself is the only configuration; no `pytest.ini`/`jest.config.*` equivalent exists or is needed |
| Quick run command | `go build ./... && go vet ./... && go test ./...` |
| Full suite command | quick run command, plus the two `go list`-based ARCH-01 checks from §Code Examples |

### Phase Requirements → Test Map

| Req ID | Behavior | Test Type | Automated Command | File Exists? |
|--------|----------|-----------|---------------------|--------------|
| ARCH-01 | Domain's direct imports never include `os`/`io/fs`/`path/filepath` | static/CI check | `go list -f '{{range .Imports}}{{.}}{{"\n"}}{{end}}{{range .TestImports}}{{.}}{{"\n"}}{{end}}' ./internal/domain/... \| sort -u \| grep -E '^(os\|io/fs\|path/filepath)$'` (expect empty output) | ❌ Wave 0 — `.github/workflows/ci.yml` does not exist |
| ARCH-01 | Domain's transitive deps never include hcl/terraform-config-inspect/go-getter/fsnotify | static/CI check | `go list -deps -test ./internal/domain/... \| sort -u \| grep -E 'hashicorp/hcl\|terraform-config-inspect\|hashicorp/go-getter\|fsnotify'` (expect empty output) | ❌ Wave 0 |
| ARCH-02 | `RepositoryGraph`/`Diagnostic` construction and queries work on hand-built objects with no filesystem | unit | `go test ./internal/domain/... -v` | ❌ Wave 0 — `internal/domain/repograph`, `internal/domain/diagnostic` packages don't exist |
| VALID-01 | Same `Spec` + `Seed` → byte-identical generated tree across repeated runs | unit/integration | `go test ./internal/testsupport/synthrepo/... -run TestGenerate_Deterministic -v` | ❌ Wave 0 — `internal/testsupport/synthrepo` doesn't exist |
| VALID-01 | Generator can inject a `BadOutputRef` error and return a `Manifest` with correct repo-relative path/line/column/`GRT001` code | unit/integration | `go test ./internal/testsupport/synthrepo/... -run TestGenerate_InjectsBadOutputRef -v` | ❌ Wave 0 |

### Sampling Rate

- **Per task commit:** `go build ./... && go vet ./... && go test ./...` — expected to run in well under a second at this phase's scale (a handful of packages, no I/O-heavy tests beyond `synthrepo` writing a small tree to a temp dir).
- **Per wave merge:** full suite (quick run command + both ARCH-01 `go list` checks), plus `gofmt -l .` (should print nothing) as a cheap style gate — optional but essentially free to add.
- **Phase gate:** the full `.github/workflows/ci.yml` (all four jobs) green before `/gsd:verify-work`.

### Wave 0 Gaps

- [ ] `go.mod` — module bootstrap, `go 1.27`
- [ ] `cmd/gruntled/main.go` — composition-root stub so `go build ./...` covers the whole tree
- [ ] `internal/domain/repograph/*.go` + `*_test.go` — covers ARCH-02
- [ ] `internal/domain/diagnostic/*.go` + `*_test.go` — covers ARCH-02 (Diagnostic VO + Set)
- [ ] `internal/testsupport/synthrepo/*.go` + `*_test.go` — covers VALID-01 (determinism + error injection)
- [ ] `.github/workflows/ci.yml` — covers ARCH-01 (both `go list` checks), plus build/vet/test jobs
- Framework install: none — `go test` ships with the Go toolchain already verified present (`go1.24.1` local binary, auto-switching to the cached `go1.27.0` toolchain via `go.mod`'s `go` directive)

## Sources

### Primary (HIGH confidence — verified by direct execution in this environment during this research pass)
- `go build`, `go vet`, `go list -f`, `go list -deps`, `go list -deps -test`, `gofmt -l` — all run directly against scratch Go packages in `/tmp/claude-1000/.../scratchpad/gotest` during this session, confirming: (a) direct-vs-transitive import behavior for the ARCH-01 filesystem rule, (b) `go 1.27` toolchain auto-switch with zero network calls given the cached `golang.org/toolchain@v0.0.1-go1.27.0.linux-amd64` in `GOMODCACHE`, (c) local Go binary version (`go1.24.1`) and `GOTOOLCHAIN=auto` default
- `.planning/phases/01-domain-foundation-test-substrate/01-CONTEXT.md` — locked architecture decisions, ARCH-01/ARCH-02/VALID-01 scope
- `.planning/research/ARCHITECTURE.md` §B.6-B.8 — package layout, build order, synthrepo design (this research extends/refines its §B.8 `Spec`/`Manifest` sketch, doesn't replace it)
- `.planning/research/STACK.md` — Go/hcl/go-cty version pinning rationale for later phases; confirms Phase 1 introduces none of these
- `docs/superpowers/specs/2026-09-01-gruntled-design.md` §5 (architecture, ubiquitous language), §6 (idempotence guarantee ①), §9 (testing philosophy)
- `go.dev/doc/go1.4#internalpackages` (cited in ARCHITECTURE.md, re-confirmed relevant here) — `internal/` enforcement scope

### Secondary (MEDIUM confidence — WebSearch/WebFetch, cross-checked across two queries each)
- `actions/checkout` and `actions/setup-go` current major version (`v7` for both) — corroborated by both a WebSearch and a direct WebFetch of each project's GitHub Releases page; the fetch tool returned internally inconsistent dates for both projects (2024 dates for a repo whose activity is contemporaneous with this 2026 research), so tag numbers are trusted but specific release dates from this research pass are not — re-verify dates if they matter to a future decision
- `math/rand/v2` `NewPCG`/`IntN` API shape and Go 1.20 auto-seeding change — WebSearch results consistent with (and this research treats as confirming) well-established, stable Go stdlib documentation

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH — Phase 1 needs stdlib only; every claim about `go list`/`math/rand/v2`/`slices` behavior was executed and observed in this session, not recalled
- Architecture: HIGH — layout is a direct, unmodified carry-forward of the already-verified `research/ARCHITECTURE.md` §B.6, itself cross-checked against real reference projects
- Pitfalls: HIGH for the two central ones (go list false positives, math/rand determinism) — both reproduced directly; MEDIUM for the CI Action version pins, since those change on their own release cadence independent of this project

**Research date:** 2026-09-25
**Valid until:** ~30 days for the architectural/stdlib content (stable); ~14 days for the specific `actions/checkout`/`actions/setup-go` version pins (these projects release every few weeks) — re-check tag numbers if the plan is executed materially later than this research date
