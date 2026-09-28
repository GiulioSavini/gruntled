# Terragrunt Edge-Case Catalogue — Phase 2 (Parsing & Graph Construction)

**Scope:** every real-world shape of `terraform { source }`, `include` /
`read_terragrunt_config`, `dependency` / `dependencies`, output-reference expressions,
and stack/cache/root-file layout that Phase 2's walker, HCL decoder and graph builder
must have a defined, tested answer for before GRT001 (Phase 3) can be trusted.

**Top priority: zero false positives** (`PROJECT.md` Constraints). Every case below is
resolved to exactly one of three behaviours:

| Behaviour | Meaning in Phase 2 |
|---|---|
| **Resolve** | Statically determined with certainty; feeds the `RepositoryGraph` as a real edge/name, and — if it's an `outputs.*` reference — becomes a candidate GRT001 check in Phase 3 |
| **Unknown** | Cannot be determined offline without evaluating state, running a process, or reaching the network; the owning unit (or just that one reference) is marked `unknown` per PARSE-04. Phase 3 must never emit GRT001 off an `unknown` unit or reference |
| **Report** | Fully and unambiguously determinable statically, and *wrong* — either a syntax error Phase 2 itself reports (`GRT100`, PARSE-05), or a structural anomaly (undefined dependency label, `include` nesting >1 level) that Phase 2 must not silently drop or silently treat as resolved; it is surfaced on the domain object for Phase 3 (or a later `GRT0xx`, per `REQUIREMENTS.md` v2 `MORE-0x`) to turn into a diagnostic |

Ten false negatives (`Unknown`) beat one false positive. When a case is genuinely
ambiguous between `Resolve` and `Unknown`, this catalogue picks `Unknown`.

Every entry also says whether **the Phase 1 synthetic generator** (`VALID-01`, spec:
unit count, include nesting depth, dependency fanout, seed) should cover it, versus
being a **hand-written fixture only** (mined largely from `denis256/terragrunt-tests`
per `PITFALLS.md` Part A #7). The generator is deterministic and spec-driven — it
should cover the common, structural shapes that scale with N units/depth/fanout; rare
or adversarial shapes belong in hand-written golden fixtures instead.

Cross-references: `.planning/PROJECT.md` (Constraints, Key Decisions), `.planning/
REQUIREMENTS.md` (PARSE-01..06, GRAPH-01..05), `.planning/research/PITFALLS.md`
(Pitfalls 1–7), `.planning/research/FEATURES.md` (function catalogue, Lead Finding).

---

## Quick reference

| ID | Case | Static? | Behaviour | Generator |
|---|---|---|---|---|
| SRC-01 | Local relative path | Yes | Resolve | Yes |
| SRC-02 | Absent `source` (unit dir is the module) | Yes | Resolve | Yes |
| SRC-03 | Local path with `//` subdir marker | Yes | Resolve | Yes |
| SRC-04 | Absolute local path | Yes | Resolve | No |
| SRC-05 | Local path via pure path function(s) | Yes | Resolve | Yes |
| SRC-06 | Git source (`git::ssh://`, `git::https://`) | No | Unknown | Yes (one case) |
| SRC-07 | GitHub shorthand source | No | Unknown | No |
| SRC-08 | Terraform/OpenTofu registry source | No | Unknown | Yes (one case) |
| SRC-09 | Source with `?ref=`/query pinning | No | Unknown | No |
| SRC-10 | Source built from a `local.*` reference | No | Unknown | No |
| SRC-11 | Source via `get_env()` with literal default | Yes | Resolve | No |
| SRC-12 | Source via `get_env()` without default | No | Unknown | No |
| SRC-13 | Local path resolving to a nonexistent directory | Yes | Report | No |
| SRC-14 | Malformed or in-file duplicate `generate` block (`invalid-generate`, G9) | No | Unknown | No |
| SRC-15 | Unit directory unlistable during the overlay check (`module-file-unreadable`, G6) | No | Unknown | No |
| INC-01 | Basic `include` via `find_in_parent_folders()` | Yes | Resolve | Yes |
| INC-02 | `include` with explicit literal `path` | Yes | Resolve | Yes |
| INC-03 | Multiple `include` blocks, distinct labels | Yes | Resolve | Yes |
| INC-04 | `merge_strategy = "no_merge"` | Yes | Resolve | Yes |
| INC-05 | `merge_strategy = "shallow"` (default) | Yes | Resolve | Yes |
| INC-06 | `merge_strategy = "deep"` | Yes | Resolve | Yes |
| INC-07 | `remote_state`/`generate` shallow-under-`deep` quirk | Yes | Resolve | No |
| INC-08 | `expose = true`, referenced as `include.x.locals.y` | Partial | Resolve/Unknown | No |
| INC-09 | Include chain depth > 1 (parent itself includes) | Yes | Report | No |
| INC-10 | `include.path` via `get_env()`, no default | No | Unknown | No |
| INC-11 | `read_terragrunt_config()` in `locals` (not gating source/config_path) | Yes (structurally) | Resolve (out of scope for the traversal it produces) | No |
| INC-12 | Parent config with its own `dependency` blocks | Yes | Resolve | No |
| INC-13 | Include of a `.json` file, explicit or via `find_in_parent_folders()` (`include-json-unsupported`, G5) | No | Unknown | No |
| DEP-01 | Basic `dependency` with literal `config_path` | Yes | Resolve | Yes |
| DEP-02 | `config_path` via pure path function(s) | Yes | Resolve | Yes |
| DEP-03 | `config_path` via ambiguous function composition | No | Unknown | No |
| DEP-04 | `config_path` pointing at a nonexistent directory | Yes | Report | No |
| DEP-05 | Multiple `dependency` blocks, distinct labels | Yes | Resolve | Yes |
| DEP-06 | `dependencies { paths = [...] }` (ordering-only) | Yes | Resolve (no outputs edge) | Yes |
| DEP-07 | `mock_outputs` present | Yes (graph unaffected) | Resolve | Yes |
| DEP-08 | `mock_outputs_merge_with_state = true` + `apply` allowed | Yes (graph unaffected) | Resolve | No |
| DEP-09 | `skip_outputs = true` | Yes | Resolve | No |
| DEP-10 | Direct dependency cycle (A → B → A) | Yes | Report (no crash) | No |
| DEP-11 | Self-referential `config_path = "."` | Yes | Report (no crash) | No |
| DEP-12 | Reference to an undeclared dependency label | Yes | Report | Yes |
| DEP-13 | `dependency.X.config_path` inherited from parent via `deep` merge | Yes | Resolve | No |
| DEP-14 | `config_path` naming a non-default file (`config-path-nondefault-file`, G4) | No | Unknown | No |
| DEP-15 | `config_path` not a valid repo path, e.g. a backslash (`config-path-invalid`, G8) | No | Unknown | No |
| OUT-01 | Simple `dependency.x.outputs.y` | Yes | Resolve | Yes |
| OUT-02 | Map/index access `outputs.y["k"]` or `outputs.y[0]` | Yes (name only) | Resolve | Yes |
| OUT-03 | Splat `outputs.y[*]` / `outputs.y.*.id` | Yes (name only) | Resolve | No |
| OUT-04 | Bracket form `outputs["y"]` | Yes | Resolve | No |
| OUT-05 | Wrapped in `try(...)` | Yes (extraction); suppression deferred | Resolve (flagged) | Yes |
| OUT-06 | `lookup(dependency.x.outputs, "y", default)` | Yes, if key is a literal | Resolve | No |
| OUT-07 | Whole-object `dependency.x.outputs` (no attr) | Yes (nothing to check) | Resolve (no-op) | No |
| OUT-08 | Dynamic key `outputs[local.k]` | No | Unknown | No |
| OUT-09 | Reference in a `for` expression | Yes | Resolve (collection); no reference from body/key/condition (lazy: gap G1) | No |
| OUT-10 | Reference in a ternary (both branches) | Yes | No reference (lazy: gap G1) | No |
| OUT-11 | Reference inside `locals`, not `inputs` | Yes | Resolve | Yes |
| OUT-12 | Reference to output on a dependency inherited via `include` | Yes | Resolve | No |
| OUT-13 | Reference to output on a dependency with `skip_outputs = true` | Yes | Resolve | No |
| OUT-14 | Merge/spread of whole outputs map (`merge(dependency.x.outputs, ...)`) | Yes (nothing to check) | Resolve (no-op) | No |
| STACK-01 | `.terragrunt-cache/` present on disk | Yes | Skip (walk) | Yes |
| STACK-02 | `.terraform/` present on disk | Yes | Skip (walk) | No |
| STACK-03 | Decoy `terragrunt.hcl` inside `.terragrunt-cache/` | Yes | Skip (walk) | Yes |
| STACK-04 | Symlinked directory or file | Yes | Skip (walk) | No |
| STACK-05 | Symlink cycle | Yes | Skip (walk), no hang | No |
| STACK-06 | Ungenerated `terragrunt.stack.hcl` | Yes | Skip, surfaced as a count | No |
| STACK-07 | Generated `.terragrunt-stack/` with real `terragrunt.hcl` files | Yes | Resolve (ordinary units) | No |
| STACK-08 | Root config named `root.hcl` vs `terragrunt.hcl` | Yes | Resolve | Yes |
| STACK-09 | A `terragrunt.hcl` that is both included and independently runnable | Yes | Unknown (include-target; documented false negative) | No |
| STACK-10 | Custom `download_dir` (literal vs env-driven) | Partial | Resolve (literal) / documented gap (env) | No |
| STACK-11 | Unit or include file over 4 MiB or nested over 1000 levels (`config-too-large` / `config-too-deep`, G7b) | No | Unknown | No |
| STACK-12 | Module file over 4 MiB or nested over 1000 levels (`module-file-too-large` / `module-file-too-deep`, G7a) | No | Unknown | No |

---

## 1. `terraform { source = ... }` resolution

### SRC-01 — Local relative path

```hcl
terraform {
  source = "../../modules/vpc"
}
```

- Static resolution: Yes — plain relative path, no functions.
- Behaviour: **Resolve**. Module dir = `filepath.Join(unitDir, source)`.
- Generator: Yes — this is the base case; every synthetic unit with a dependency
  fanout needs at least one unit resolved this way.

### SRC-02 — Absent `source` (unit directory *is* the module)

```hcl
# terragrunt.hcl — no terraform block at all
include "root" {
  path = find_in_parent_folders()
}
```

- Static resolution: Yes.
- Behaviour: **Resolve** — GRAPH-02. Module dir = unit dir. This is the *dominant*
  real-world shape: the entire primary corpus (65/65 units, `aws-iso20022`) uses it —
  see `PITFALLS.md` Part A #1. Any Phase 2 design that treats `source` presence as the
  only resolution path is already wrong on the primary corpus.
- Generator: Yes — must be the *default* unit shape the generator produces, not an
  edge case bolted on afterward.

### SRC-03 — Local path with `//` subdir marker

```hcl
terraform {
  source = "../../aws//lambda"
}
```

- Static resolution: Yes.
- Behaviour: **Resolve**. Split on the first `//`; the left side is a filesystem path,
  the right side is a subdirectory within it. Confirmed real usage: `cds-snc/secret`
  (`PITFALLS.md` Part A #2). `//` is not exclusively a remote-source marker — it
  appears on local sources too, and a naive "treat `//` as remote" heuristic would
  misclassify this as unresolvable and silently under-report a resolvable unit.
- Generator: Yes — cheap to add, closes a real false-`Unknown` risk (a false negative
  by policy is safe, but this one is easy to get right and worth locking in with a
  golden test).

### SRC-04 — Absolute local path

```hcl
terraform {
  source = "/opt/shared-modules/vpc"
}
```

- Static resolution: Yes — Terragrunt treats any non-URL-shaped string as a
  filesystem path, absolute or relative.
- Behaviour: **Resolve** if the path exists on disk (even outside the repo root — a
  shared-modules mount is a legitimate pattern); if it does not exist, see SRC-13.
- Generator: No — rare, and adds no coverage beyond SRC-01's path-join logic. Hand
  fixture only, to prove the "don't assume repo-root-relative" assumption is tested.

### SRC-05 — Local path built from pure path functions

```hcl
terraform {
  source = "${get_terragrunt_dir()}/../../modules/vpc"
}
```

- Static resolution: Yes — `get_terragrunt_dir()` is one of the six PARSE-02
  functions; the rest is string concatenation with a literal.
- Behaviour: **Resolve**.
- Notes: `find_in_parent_folders("modules")` in a `source` position is a real but
  rarer variant (`find_in_parent_folders` normally locates a *file*, not a directory,
  by walking up; using it to locate a module directory by name is a non-standard but
  seen pattern). Resolve it the same way *only if* the function call is one of the
  exact six PARSE-02 signatures with literal arguments — never attempt to
  general-purpose-evaluate an expression tree beyond that fixed set.
- Generator: Yes — `get_terragrunt_dir()`/`find_in_parent_folders()` composition is
  exactly the mechanism PARSE-02 exists for; needs golden coverage independent of the
  `include.path` use of the same functions (INC-01/INC-02).

### SRC-06 — Git source (SSH or HTTPS)

```hcl
terraform {
  source = "git::ssh://git@github.com/example-org/modules.git//vpc?ref=v1.2.3"
}
```

- Static resolution: No — requires network access to even confirm the ref exists,
  let alone read `.tf` files from it. Categorically out of scope (`PROJECT.md`: "no
  network access at runtime").
- Behaviour: **Unknown**. Mark the owning unit `unknown` with reason `remote-source`
  (per the `PITFALLS.md` UX pitfall recommendation: every `unknown` should carry a
  machine-readable reason, not bare silence).
- Notes: Any `dependency` block pointing *at* this unit (i.e. treating it as a target)
  is unaffected — GRAPH-04's first hop (`config_path` → unit) still resolves; it's the
  *second* hop (unit → module) that goes `unknown`. Do not let one unit's unresolved
  source poison the dependency graph edges of units that merely depend on it — GRT001
  on *that specific reference* becomes unknown, not the whole repository.
- Generator: Yes, one representative case — needed to prove GRAPH-03's "classify
  remote without downloading" and to prove the blast radius of `unknown` is properly
  contained to just that unit/reference, not the whole graph.

### SRC-07 — GitHub shorthand source

```hcl
terraform {
  source = "github.com/example-org/modules//vpc?ref=v1.0.0"
}
```

- Static resolution: No — same class as SRC-06, different surface syntax (Terraform's
  generic `go-getter` shorthand, no `git::` prefix).
- Behaviour: **Unknown**, reason `remote-source`.
- Generator: No — same resolution logic as SRC-06 exercises this; a second generator
  case adds no new code path. Hand fixture only, for the detector's syntax coverage.

### SRC-08 — Terraform/OpenTofu registry source

```hcl
terraform {
  source = "terraform-aws-modules/vpc/aws"
}
```

- Static resolution: No — registry sources have no filesystem or git representation
  at all without a network call to the registry API.
- Behaviour: **Unknown**, reason `remote-source`.
- Notes: Distinguishing a registry source from a local path is itself a parsing
  decision: a bare `<namespace>/<name>/<provider>` string with exactly two slashes and
  no `//`, no leading `.`/`/`, and no URL scheme. Get this classifier wrong in the
  direction of "treat as local" and you get a crash or a bogus module dir instead of a
  correctly-`unknown` unit — worth its own unit test on the classifier, independent of
  the golden fixtures.
- Generator: Yes, one representative case — proves the source-classifier doesn't
  misfire on registry-shaped strings that superficially resemble two-segment relative
  paths.

### SRC-09 — Source with query-string pinning

```hcl
terraform {
  source = "git::https://example.com/modules.git?ref=v2.0.0&depth=1"
}
```

- Static resolution: No.
- Behaviour: **Unknown**. The query string (`ref`, `depth`, or anything else) never
  changes the classification — it's still remote regardless of what's pinned.
- Generator: No — subsumed by SRC-06's classification logic.

### SRC-10 — Source built from a `local.*` reference

```hcl
locals {
  module_base = "../../modules"
}
terraform {
  source = "${local.module_base}/vpc"
}
```

- Static resolution: No, under the "never evaluate an expression to a value" rule
  (`PROJECT.md` Key Decisions). Chasing `local.module_base` back to its own defining
  expression is itself unbounded — that `local` could in turn reference another
  `local`, an `include`d value, or a function outside the fixed six. Confirmed
  real-world existence of this general shape (locals feeding generated content) in
  `cds-snc/secret`'s `generate` blocks (`PITFALLS.md` Pitfall 4), even though that
  specific repo uses it for `generate` contents rather than `source`.
- Behaviour: **Unknown**, reason `dynamic-source-expression`.
- Notes: Resist the temptation to special-case "a `local` that is itself a bare
  string literal" — that reintroduces exactly the kind of expanding-scope expression
  evaluator the structural-decode-only decision exists to avoid, for a shape not yet
  confirmed common in the corpus. If real-repo validation (Phase 4) shows this pattern
  is common enough to matter, revisit deliberately rather than opportunistically.
- Generator: No — deliberately kept `Unknown`; a hand fixture proves the negative
  (gruntled stays silent) rather than needing spec-driven scale.

### SRC-11 — Source via `get_env()` with a literal default

```hcl
terraform {
  source = get_env("MODULE_SOURCE", "../../modules/vpc")
}
```

- Static resolution: Yes, for the default — per `FEATURES.md`'s Question 3
  recommendation: never read the real process environment (breaks CLI-03
  determinism — same input must mean same repository content, not same environment);
  when a literal `default` argument is present, evaluate to that literal.
- Behaviour: **Resolve** (using the literal default, never the real env var).
- Generator: No — low observed prevalence for gating `source` specifically
  (`FEATURES.md` Question 2: side-effecting/environment functions overwhelmingly feed
  `inputs`, not path-resolution positions). Hand fixture only.

### SRC-12 — Source via `get_env()` without a default

```hcl
terraform {
  source = get_env("MODULE_SOURCE")
}
```

- Static resolution: No — no literal to fall back to, and reading the real
  environment is explicitly rejected for determinism.
- Behaviour: **Unknown**, reason `environment-dependent-source`.
- Generator: No — hand fixture only, proves the "no default → unknown" half of
  SRC-11's policy.

### SRC-13 — Local path resolving to a nonexistent directory

```hcl
terraform {
  source = "../../modules/typo-vpc"
}
```

- Static resolution: Yes — the string is fully literal; what's not resolvable is the
  *target*, not the expression.
- Behaviour: **Report**. This is fully and unambiguously known statically (the path
  doesn't exist) and is a genuine wiring defect independent of GRT001 — Phase 2 must
  not crash, must not silently treat it as `unknown` (that would hide a real,
  100%-certain defect behind the same marker used for genuine uncertainty), and must
  not silently invent an empty module surface (which would make every `dependency`
  pointing at it wrongly report GRT001 on every single output, drowning the real
  signal in noise). Surface it as a distinct "module not found" condition on the
  domain `Module`/`Unit` object for Phase 3 to turn into its own diagnostic (or fold
  into GRT001 deliberately — that decision belongs to Phase 3, not Phase 2; Phase 2's
  job is to not lose the information).
- Generator: No — hand fixture only; this is a deliberately-broken fixture, not a
  spec-scale shape.

---

### SRC-14 — Malformed or in-file duplicate `generate` block

```hcl
generate {                 # no label
  path     = "x.tf"
  contents = ""
}
generate "a" "b" { ... }   # two labels
generate "p" { ... }
generate "p" { ... }       # same label twice in one file
```

- Static resolution: No. Terragrunt itself rejects all three shapes, so the unit's
  effective generated files are not known.
- Behaviour: **Unknown**. The unit is config-unknown `invalid-generate`
  (`ReasonInvalidGenerate`, gap G9, 02-07). The same label in the child and in an
  include is still a valid merge (highest precedence wins) and stays resolved.
- Tests: `TestUnknownReasons` rows `invalid-generate/no-label`, `/two-labels`,
  `/duplicate-in-file`, `/duplicate-in-include`, plus the
  `regression-same-label-child-and-include-is-valid` row.
- Generator: No.

### SRC-15 — Unit directory unlistable during the overlay check

- Static resolution: No. When the source points outside the unit directory, the loader
  lists the unit directory to see whether its own `.tf` files overlay the module. If that
  `fs.ReadDir` fails, whether it overlays is unknown.
- Behaviour: **Unknown**. The unit is module-unknown `module-file-unreadable`
  (`ReasonModuleFileUnreadable`, gap G6, 02-07). It was previously assumed not to
  overlay, which could hide real outputs. The string matches tfsurface's own
  `module-file-unreadable`, used when a kept module file cannot be read.
- Tests: `TestUnknownReasons` row `module-file-unreadable` (loader, via
  `readDirFailFS`); tfsurface's `TestReadSurfaceLimitPrecedence` for the reader variant.
- Generator: No.

## 2. `include` / `read_terragrunt_config`

### INC-01 — Basic `include` via `find_in_parent_folders()`

```hcl
include "root" {
  path = find_in_parent_folders()
}
```

- Static resolution: Yes — PARSE-02's core case; walk up from the unit directory for
  the nearest file matching the default name(s) (see STACK-08 for the `root.hcl` vs
  `terragrunt.hcl` naming nuance).
- Behaviour: **Resolve**.
- Generator: Yes — this is the default shape for every generated unit's root include;
  the "nested includes" spec parameter (`ROADMAP.md` Phase 1) is expressed through
  this exact pattern at every depth level the generator produces.

### INC-02 — `include` with an explicit literal path

```hcl
include "root" {
  path = "${get_terragrunt_dir()}/../../root.hcl"
}
```

- Static resolution: Yes.
- Behaviour: **Resolve**.
- Generator: Yes — exercises PARSE-02 function composition inside `include.path`
  specifically (distinct code path from `source`/`config_path` composition — the same
  functions, different call site, all three must be tested independently since a
  shared-but-untested implementation is exactly how this kind of bug survives review).

### INC-03 — Multiple `include` blocks, distinct labels

```hcl
include "root" {
  path = find_in_parent_folders()
}
include "region" {
  path = find_in_parent_folders("region.hcl")
}
```

- Static resolution: Yes.
- Behaviour: **Resolve** — each label resolved and merged independently per its own
  `merge_strategy` (`FEATURES.md` Question 3).
- Generator: Yes — the generator's "nested includes" spec parameter should include at
  least one multi-label case, since real Gruntwork-style repos commonly layer
  `account.hcl`/`region.hcl`/`root.hcl`.

### INC-04 — `merge_strategy = "no_merge"`

```hcl
include "root" {
  path            = find_in_parent_folders()
  merge_strategy  = "no_merge"
}
```

- Static resolution: Yes.
- Behaviour: **Resolve**. The parent is parsed (once, shared — PARSE-03) but *not*
  merged into the child's effective config; only `include.root.*` values are visible
  if `expose = true` is also set (see INC-08). Critically: a child's own `dependency`
  blocks are unaffected either way (they were never inherited under any strategy
  except via `deep`, see INC-06) — `no_merge` mainly matters for `inputs`/`locals`
  inheritance, which GRT001 itself doesn't need but a correct `RepositoryGraph` model
  must still represent accurately for future diagnostics (`inputs`-vs-`variable`
  checks, `MORE-04`/`MORE-05`, v2).
- Generator: Yes — the three `merge_strategy` values are a small, enumerable set;
  covering all three is cheap and directly protects GRAPH-04's correctness under
  inheritance.

### INC-05 — `merge_strategy = "shallow"` (the default)

```hcl
include "root" {
  path = find_in_parent_folders()
  # merge_strategy omitted -> "shallow"
}
```

- Static resolution: Yes.
- Behaviour: **Resolve**. Top-level attributes/blocks in the child override the
  parent's same-named attribute/block wholesale; `dependencies { }` (plural, ordering
  block — DEP-06) is the one documented exception that always deep-merges regardless
  of the declared strategy (`FEATURES.md` Question 3) — this asymmetry is easy to miss
  and must be modeled explicitly, not inferred from the general shallow rule.
- Generator: Yes.

### INC-06 — `merge_strategy = "deep"`

```hcl
include "root" {
  path           = find_in_parent_folders()
  merge_strategy = "deep"
}
```

- Static resolution: Yes.
- Behaviour: **Resolve**. Recursive merge — a child's `dependency` block set can be
  entirely inherited from the parent under `deep`, meaning GRAPH-04 cannot determine a
  unit's full dependency set from the child file alone; include resolution must happen
  *before* dependency resolution (`FEATURES.md` Question 3, "Dependency Notes").
- Generator: Yes — this is the one `merge_strategy` value that actually changes
  GRAPH-04's algorithm (parent-owned `dependency` blocks reaching the child), so it
  needs its own golden fixture, not just "included in the enum sweep."

### INC-07 — `remote_state`/`generate` stay shallow even under `deep`

```hcl
# root.hcl
remote_state {
  backend = "s3"
  config  = { bucket = "root-bucket" }
}
# child terragrunt.hcl
include "root" {
  path           = find_in_parent_folders()
  merge_strategy = "deep"
}
remote_state {
  config = { bucket = "child-bucket" }
}
```

- Static resolution: Yes — documented implementation limit, not ambiguous.
- Behaviour: **Resolve**, using shallow (wholesale-replace) semantics for
  `remote_state` and `generate` specifically, even though the enclosing
  `merge_strategy` says `deep` everywhere else.
- Notes: This has no direct bearing on GRT001 (`remote_state`/`generate` aren't in the
  PARSE-01 read set), but the include-merge model built here in Phase 2 is shared
  infrastructure — getting the exception wrong now means re-deriving it later for
  `generate`-surface tracking (Pitfall 4, `MORE-04`/`MORE-05`).
- Generator: No — hand fixture only; too narrow to warrant a spec parameter.

### INC-08 — `expose = true`, referenced as `include.x.locals.y`

```hcl
include "root" {
  path   = find_in_parent_folders()
  expose = true
}
dependency "vpc" {
  config_path = "${include.root.locals.modules_dir}/../vpc"
}
```

- Static resolution: Partial — depends entirely on whether
  `include.root.locals.modules_dir` is itself a string literal in the parent's
  `locals` block. If it is, chase it (one hop, into an *already-parsed-and-shared*
  parent config — this is cheap and bounded, unlike SRC-10's arbitrary-depth `local`
  chase, because `include` nesting is capped at exactly one level — INC-09). If the
  parent's `locals.modules_dir` is itself computed from a function outside the
  PARSE-02 six, or from another `local`, stop and go `Unknown`.
- Behaviour: **Resolve** if the exposed value chases to a literal in one hop;
  **Unknown** otherwise.
- Generator: No — low prevalence estimate (`FEATURES.md`), and the resolvable case
  reduces to "chase one literal in an already-parsed file," which existing
  include-sharing tests (INC-03) already cover indirectly. Hand fixture only, to prove
  the cutoff (one hop, not recursive).

### INC-09 — Include chain depth > 1 (parent itself has an `include` block)

```hcl
# grandparent/terragrunt.hcl (a "parent" config)
include "even_more_root" {
  path = find_in_parent_folders("truly-root.hcl")
}
```

- Static resolution: Yes — Terragrunt itself rejects this at runtime ("Terragrunt
  only supports a single level of `include` blocks", `FEATURES.md` Question 3), so
  it's a fully known, unambiguous invalid configuration, not an uncertain one.
- Behaviour: **Report**. Do not silently resolve two levels deep (that would make
  gruntled *more* permissive than real Terragrunt and risk reporting a graph that
  could never actually run), and do not go `Unknown` either (this isn't uncertainty,
  it's a known-invalid shape). Treat it the same class as PARSE-05's "invalid HCL
  instead of a crash" — a config-level error, not a values-level one.
- Generator: No — hand fixture only; deliberately-invalid shape.

### INC-10 — `include.path` via `get_env()`, no default

```hcl
include "root" {
  path = get_env("TG_ROOT_CONFIG_PATH")
}
```

- Static resolution: No.
- Behaviour: **Unknown**. The owning unit's entire effective config is unknown (its
  `dependency` set may be entirely parent-inherited per INC-06) — mark the whole unit
  `unknown`, reason `environment-dependent-include`, not just this one attribute.
- Generator: No — hand fixture only.

### INC-11 — `read_terragrunt_config()` used in `locals`, not gating `include`/`source`/`config_path`

```hcl
locals {
  shared_tags = read_terragrunt_config("${get_terragrunt_dir()}/../tags.hcl").locals.tags
}
```

- Static resolution: Yes, structurally — PARSE-01 reads only `include`,
  `terraform.source` and `dependency` blocks; a `read_terragrunt_config()` call
  sitting in an unrelated `locals` block never has to be evaluated at all, because
  nothing Phase 2 needs (an include target, a source path, a `config_path`, or an
  `outputs.*` traversal) depends on `local.shared_tags`.
- Behaviour: **Resolve** in the specific sense of "correctly ignored" — the file
  parses without error, and this construct contributes nothing to the graph. This is
  the direct, load-bearing consequence of the structural-decode-only design decision
  (`PROJECT.md`): the *presence* of an unevaluable function call elsewhere in the file
  must never poison an otherwise-fully-static `include`/`source`/`dependency` triad.
- Generator: No — implicitly covered by never generating spurious failures on
  fixtures that contain unrelated `locals`; not itself a distinct spec parameter.

### INC-12 — Parent config that itself has `dependency` blocks

```hcl
# root.hcl
dependency "shared_kms" {
  config_path = "../kms"
}
```

- Static resolution: Yes — but with a documented restriction to mirror, not invent:
  when the *parent* contains `dependency` blocks, Terragrunt restricts what's visible
  to descendants' `include`/`dependency` resolution to only the parent's `locals` and
  `include` (explicitly to avoid circular bootstrapping — you can't need an applied
  dependency's output to compute your own dependency graph, `FEATURES.md`
  Question 3).
- Behaviour: **Resolve**, applying that same restriction rather than trying to be
  cleverer than Terragrunt about what a child can see from such a parent.
- Generator: No — narrow, protocol-level nuance; hand fixture only, to prevent a
  future "helpful" refactor from accidentally letting a child see more of such a
  parent than real Terragrunt would.

---

### INC-13 — Include of a `.json` file

```hcl
include "root" {
  path = "../root.hcl.json"
}
# or: find_in_parent_folders() finding terragrunt.hcl.json ahead of terragrunt.hcl
```

- Static resolution: No. This domain does not parse JSON Terragrunt configs, and
  handing a JSON file to the native HCL parser produced a false GRT100.
- Behaviour: **Unknown**. The including unit is config-unknown
  `include-json-unsupported` (`ReasonIncludeJSONUnsupported`, gap G5, 02-07), with no
  GRT100.
- Tests: `TestUnknownReasons` rows `include-json-unsupported/explicit` and
  `/find-in-parent` (both assert no diagnostics).
- Generator: No.

## 3. `dependency` / `dependencies` blocks

### DEP-01 — Basic `dependency` with a literal `config_path`

```hcl
dependency "vpc" {
  config_path = "../vpc"
}
```

- Static resolution: Yes.
- Behaviour: **Resolve** — GRAPH-04's base case, two hops: `config_path` → unit,
  then that unit's `source` (or absence, SRC-02) → module.
- Generator: Yes — the core mechanism the "dependency fanout" spec parameter
  (`ROADMAP.md` Phase 1) exists to exercise.

### DEP-02 — `config_path` via pure path function(s)

```hcl
dependency "vpc" {
  config_path = "${get_terragrunt_dir()}/../vpc"
}
```

- Static resolution: Yes.
- Behaviour: **Resolve**.
- Generator: Yes — third independent call site for PARSE-02 functions (alongside
  SRC-05 and INC-02); each needs its own test since they're different AST positions
  even though the function set is shared.

### DEP-03 — `config_path` via ambiguous/unsupported function composition

```hcl
dependency "vpc" {
  config_path = "${find_in_parent_folders("modules")}/vpc/../${path_relative_to_include()}"
}
```

- Static resolution: No — composition of multiple path functions with relative
  segments in a shape not confidently reducible to a single literal without a fuller
  expression evaluator than PARSE-02 commits to.
- Behaviour: **Unknown**, reason `unresolvable-config-path`. Prefer staying silent
  over guessing at a plausible-looking but wrong resolution — a wrong `config_path`
  resolution is worse than a missed one, because it produces a *confidently wrong*
  graph edge that could manufacture a false GRT001.
- Generator: No — hand fixture only, deliberately adversarial.

### DEP-04 — `config_path` pointing at a nonexistent directory

```hcl
dependency "vpc" {
  config_path = "../vcp"  # typo
}
```

- Static resolution: Yes (the string), no (the target).
- Behaviour: **Report**. Same reasoning as SRC-13 — fully known and definitely
  broken, not uncertain. `GRT002` (`config_path` pointing at no unit) is explicitly
  v2/`MORE-01` scope, so Phase 2 does not need to *emit* that diagnostic — but it must
  preserve the information (an "unresolved dependency target" marker on the graph
  edge) rather than crash, silently drop the edge, or treat it as `unknown` in a way
  indistinguishable from a genuinely-remote/dynamic case. Any `outputs.*` reference
  through this dependency must not trigger GRT001 in Phase 3 (there's no module
  surface to check against) — it should surface as its own condition instead.
- Generator: No — hand fixture only.

### DEP-05 — Multiple `dependency` blocks, distinct labels

```hcl
dependency "vpc" {
  config_path = "../vpc"
}
dependency "kms" {
  config_path = "../kms"
}
```

- Static resolution: Yes.
- Behaviour: **Resolve**.
- Generator: Yes — the direct expression of the "dependency fanout" spec parameter.

### DEP-06 — `dependencies { paths = [...] }` (plural, ordering-only)

```hcl
dependencies {
  paths = ["../vpc", "../kms"]
}
```

- Static resolution: Yes.
- Behaviour: **Resolve**, but note it contributes *only* an apply-ordering edge, never
  an `outputs.*` accessor — there is no `dependencies.vpc.outputs.*` syntax; this
  block cannot itself be the source of a GRT001-relevant reference. Confirmed real
  co-occurrence with real `dependency` blocks in `cds-snc/secret` (`PITFALLS.md`
  Part A #2) — a repo can and does use both forms side by side for different targets.
- Generator: Yes — cheap to add, and its "no outputs edge" property is exactly the
  kind of thing worth locking into a golden test so a future refactor doesn't
  accidentally start treating `dependencies` entries as GRT001-checkable.

### DEP-07 — `mock_outputs` present

```hcl
dependency "vpc" {
  config_path  = "../vpc"
  mock_outputs = {
    vpc_id = "vpc-mock"
  }
  mock_outputs_allowed_terraform_commands = ["validate", "plan"]
}
```

- Static resolution: Yes — for the *graph*, `mock_outputs` changes nothing about
  hops one or two.
- Behaviour: **Resolve**, identically to DEP-01. Per Pitfall 3 (`PITFALLS.md`), the
  presence of a mock must never suppress or alter GRT001 in Phase 3 — but that's a
  Phase 3/DIAG-03 decision. Phase 2's only job here is to make sure `mock_outputs`
  parses structurally without perturbing the graph edge, and that the domain model
  doesn't conflate "has a mock for key Y" with "module Y is confirmed to exist."
- Generator: Yes — `mock_outputs` is called out in `PROJECT.md` as "the main
  false-positive risk"; the generator should be able to produce a unit whose
  dependency has `mock_outputs` covering a key that the target module genuinely does
  *not* declare, specifically so Phase 3's golden tests can assert GRT001 still fires.

### DEP-08 — `mock_outputs_merge_with_state = true` with `apply` allowed

```hcl
dependency "vpc" {
  config_path                             = "../vpc"
  mock_outputs                            = { vpc_id = "vpc-mock" }
  mock_outputs_merge_with_state           = true
  mock_outputs_allowed_terraform_commands = ["validate", "plan", "apply"]
}
```

- Static resolution: Yes, for the graph.
- Behaviour: **Resolve**, identically to DEP-01/DEP-07. This exact shape is called
  out in `ROADMAP.md` Phase 3 success criteria as a pattern that must be *among the
  tested cases* for DIAG-03, because "a missing output genuinely works at runtime"
  here (state merge can supply the real value even when the mock's static shape looks
  incomplete). Phase 2's contribution is only to make sure this attribute combination
  parses cleanly and is preserved on the domain `Dependency` object so Phase 3 has it
  available to reason about — Phase 2 itself makes no suppression decision.
- Generator: No — this exact interaction belongs to Phase 3's DIAG-03 test matrix;
  Phase 2 only needs it to parse without error (implicitly covered by DEP-07's
  broader `mock_outputs` parsing).

### DEP-09 — `skip_outputs = true`

```hcl
dependency "vpc" {
  config_path  = "../vpc"
  skip_outputs = true
}
```

- Static resolution: Yes.
- Behaviour: **Resolve**, unchanged from DEP-01 — `skip_outputs` only affects whether
  Terragrunt *fetches* outputs at plan/apply time for performance, not whether the
  dependency's target module is statically resolvable. If the same unit also contains
  an `outputs.*` traversal against this dependency, that combination is internally
  contradictory at the Terragrunt-runtime level, but statically the module surface
  check is still meaningful and should still run — a real repo pattern from
  `denis256/terragrunt-tests/skip_outputs/` (`PITFALLS.md` Part A #7) is the fixture
  source.
- Generator: No — hand fixture only, mined from `denis256/terragrunt-tests`.

### DEP-10 — Direct dependency cycle (A → B → A)

```hcl
# a/terragrunt.hcl
dependency "b" { config_path = "../b" }
# b/terragrunt.hcl
dependency "a" { config_path = "../a" }
```

- Static resolution: Yes — fully knowable once both units are indexed.
- Behaviour: **Report**, in the sense of "detected and surfaced, never a crash or a
  hang." `GRT003` (dependency cycle) is explicitly `MORE-02`/v2 scope, so Phase 2 does
  not need to emit that diagnostic code — but the graph builder must detect the cycle
  during construction (a simple DFS with a visiting-set) and represent it on the
  domain model rather than infinite-looping or stack-overflowing. This is a graph
  builder correctness requirement independent of whether the diagnostic ships in v1.
- Generator: No — hand fixture only; deliberately pathological, mined from
  `denis256/terragrunt-tests/cycles/`.

### DEP-11 — Self-referential `config_path = "."`

```hcl
# a/terragrunt.hcl
dependency "self" {
  config_path = "."
}
```

- Static resolution: Yes.
- Behaviour: **Report**, same handling as DEP-10 (a cycle of length one) — must not
  hang or crash.
- Generator: No — hand fixture only.

### DEP-12 — Reference to an undeclared dependency label

```hcl
dependency "vpc" {
  config_path = "../vpc"
}
# elsewhere in the same file:
inputs = {
  subnet = dependency.vpcc.outputs.subnet_id  # "vpcc" was never declared
}
```

- Static resolution: Yes — fully, unambiguously knowable: no `dependency "vpcc"`
  block exists in this unit's effective (post-include-merge) config at all. This is
  categorically different from every "remote/dynamic source" `Unknown` case above —
  there is no uncertainty here, only a typo.
- Behaviour: **Report**. Whether this becomes GRT001 itself (arguably `Y` "does not
  exist" because `X` doesn't exist) or a sibling code is a Phase 3 naming decision,
  but Phase 2 must not drop it — a reference to a traversal root that resolves to no
  declared dependency at all must be surfaced on the domain model, not silently
  skipped as if it were merely `unknown`.
- Generator: Yes — this is a cheap, high-value injectable mutation (alongside the
  Phase 1 generator's existing "bad output ref" injection kind) — reuse the same
  injection machinery for "reference an undeclared dependency label" as a second,
  distinct mutation kind, since it exercises a different part of the graph builder
  (traversal-root resolution) than a renamed/deleted output does.

### DEP-13 — `dependency` block inherited from parent via `deep` merge

```hcl
# root.hcl
dependency "shared_kms" {
  config_path = "${get_parent_terragrunt_dir()}/kms"
}
# child/terragrunt.hcl
include "root" {
  path           = find_in_parent_folders()
  merge_strategy = "deep"
}
# no dependency block of its own — inherits "shared_kms" entirely
```

- Static resolution: Yes, but only after include resolution (INC-06) runs first —
  this is precisely why `FEATURES.md`'s dependency notes insist include resolution
  must precede dependency resolution.
- Behaviour: **Resolve**, using the child's own directory as the base for any
  relative-path functions inside the inherited block, evaluated in the parent's
  original expression but re-anchored per Terragrunt's own documented semantics for
  `get_parent_terragrunt_dir()`/`get_terragrunt_dir()` under inheritance.
- Generator: No — this composes INC-06 + DEP-01; hand fixture only, since it's a
  cross-cutting correctness case rather than a new independent mechanism.

---

### DEP-14 — `config_path` naming a non-default file

```hcl
dependency "vpc" {
  config_path = "../vpc/alt.hcl"
}
```

- Static resolution: No. Terragrunt reads THAT file as the target's config, and it may
  set a different source than the directory's own `terragrunt.hcl`, so mapping it to the
  directory would be a guess. `config_path = "../vpc/terragrunt.hcl"` still maps to
  `vpc`.
- Behaviour: **Unknown**, for that dependency only: unresolved
  `config-path-nondefault-file` (`ReasonConfigPathNondefaultFile`, gap G4, 02-07).
- Tests: `TestUnknownReasons` rows `config-path-nondefault-file/named-file` and
  `/json-config`, plus the default-file regression row.
- Generator: No.

### DEP-15 — `config_path` that is not a valid repo path

```hcl
dependency "vpc" {
  config_path = "..\\vpc"
}
```

- Static resolution: No. A backslash survives path resolution as part of a segment,
  and `repograph.NewRepoPath` rejects the result.
- Behaviour: **Unknown**, for that dependency only: unresolved `config-path-invalid`
  (`ReasonConfigPathInvalid`, gap G8, 02-07). Sibling dependencies and the unit itself
  stay resolved; before G8 the whole unit became config-unknown.
- Tests: `TestUnknownReasons` row `config-path-invalid` (sibling `good` stays resolved).
- Generator: No.

## 4. Output-reference expressions

These are the traversals GRT001 (Phase 3) ultimately checks. Phase 2's job is
*extraction*: walking each unit's full effective HCL expression tree (not just
`inputs` — see OUT-11) for every `dependency.<label>.outputs...` shape, without
evaluating any of it, and recording enough structure (root label, accessed name if
statically known, and any wrapping context like `try()`) for Phase 3 to act on.

### OUT-01 — Simple attribute access

```hcl
inputs = {
  vpc_id = dependency.vpc.outputs.vpc_id
}
```

- Static resolution: Yes.
- Behaviour: **Resolve** — extract `(dependency = "vpc", output = "vpc_id")`. The
  canonical GRT001 case.
- Generator: Yes — this is the reference shape the Phase 1 generator's "bad output
  ref" injection (`ROADMAP.md` Phase 1, success criterion 4) already targets.

### OUT-02 — Map/index access on an output

```hcl
inputs = {
  private_subnet = dependency.vpc.outputs.subnet_ids["private"]
}
```

- Static resolution: Yes, for the name that matters to GRT001 (`subnet_ids`) — the
  index/key itself is a *value*-level detail GRT001 was never designed to check
  (that would require evaluating the target module's actual output value, out of
  scope per `PROJECT.md`).
- Behaviour: **Resolve** — extract `(dependency = "vpc", output = "subnet_ids")`,
  discard the index.
- Generator: Yes — cheap, and a naive extractor that only matches a bare
  `dependency.X.outputs.Y` traversal (no trailing index) will silently *miss* this
  reference entirely, which is a false negative (safe by policy) but defeats the
  point of the check on a common real-world shape; worth locking in positively.

### OUT-03 — Splat access

```hcl
inputs = {
  subnet_ids = dependency.vpc.outputs.subnet_ids[*]
}
```

- Static resolution: Yes, for the name (`subnet_ids`); same reasoning as OUT-02.
- Behaviour: **Resolve**.
- Generator: No — same extraction logic as OUT-02; hand fixture only, for AST-shape
  coverage (splat is a distinct HCL expression node from index access).

### OUT-04 — Bracket form on the `outputs` object itself

```hcl
inputs = {
  vpc_id = dependency.vpc.outputs["vpc_id"]
}
```

- Static resolution: Yes — the bracket argument is a string literal.
- Behaviour: **Resolve** — extract `(dependency = "vpc", output = "vpc_id")`,
  identical result to OUT-01, different AST shape (`IndexExpr` with a literal key
  instead of a `TraverseAttr`). Both forms must be supported by the same extractor.
- Generator: No — hand fixture only, for AST-shape coverage.

### OUT-05 — Wrapped in `try(...)`

```hcl
inputs = {
  optional_flag = try(dependency.vpc.outputs.enable_nat_gateway, false)
}
```

- Static resolution: Yes, for extraction — the traversal is present in the AST
  regardless of which function it's an argument to; a whole-body walk finds it the
  same way it finds an unwrapped reference.
- Behaviour: **Resolve** the extraction, but flag it as `wrapped_in_try = true` on the
  extracted reference. Whether `try()` should *suppress* GRT001 (the author
  explicitly anticipated the output might be absent, similar in spirit to
  `mock_outputs`) or leave it unchanged (the output not existing at all is still a
  genuine wiring defect, `try()` only guards against *runtime* errors, not against
  gruntled's static check) is a Phase 3/DIAG-03-adjacent design decision this
  catalogue deliberately does not make — Phase 2's responsibility stops at faithfully
  recording that the wrapping exists, so Phase 3 has the information available either
  way.
- Generator: Yes — cheap to add as a variant of the OUT-01 injection, and this
  flag needs to exist and be populated correctly before Phase 3 can make its
  suppression decision at all; better to have the data available and unused than to
  retrofit the extractor later.

### OUT-06 — `lookup(dependency.x.outputs, "y", default)`

```hcl
inputs = {
  vpc_id = lookup(dependency.vpc.outputs, "vpc_id", "vpc-fallback")
}
```

- Static resolution: Yes, if the key argument is a string literal — this is a
  different AST shape from OUT-01/OUT-04: the first argument to `lookup()` is the
  *whole-object* traversal `dependency.vpc.outputs` (no trailing attribute), and the
  name to check (`"vpc_id"`) is a separate literal argument, not part of the
  traversal at all. An extractor built only around traversal-walking (OUT-01–OUT-04)
  will not find this shape by construction — it requires a second, function-call-aware
  pattern: "first arg is `dependency.<label>.outputs` bare, second arg is a string
  literal."
- Behaviour: **Resolve** if the key is a literal; **Unknown** for that specific
  reference if the key is itself an expression (e.g. `lookup(dependency.vpc.outputs,
  local.key_name, null)`) — don't guess.
- Generator: No — hand fixture only; distinct enough AST shape to warrant explicit
  test coverage but low enough real-world prevalence (no confirmed occurrence in the
  surveyed corpus) that spec-scale generation isn't justified yet.

### OUT-07 — Whole-object reference, no specific attribute

```hcl
inputs = dependency.vpc.outputs
```

- Static resolution: Yes — there is no specific `Y` name asserted here at all.
- Behaviour: **Resolve** as a no-op for GRT001 purposes: correctly parse and
  correctly produce *zero* extracted references from this expression, rather than
  either crashing (treating `outputs` itself as an attribute name to look up, which
  would be a phantom false positive — no module declares an output literally named
  `outputs`) or spuriously flagging the whole dependency as suspicious.
- Generator: No — hand fixture only, specifically to prove the extractor doesn't
  mis-parse the whole-object case into a bogus single reference.

### OUT-08 — Dynamic key on `outputs[...]`

```hcl
inputs = {
  value = dependency.vpc.outputs[local.output_key]
}
```

- Static resolution: No — the index expression is not a literal.
- Behaviour: **Unknown** for this specific reference only (does not need to mark the
  whole unit `unknown` — every *other* extracted reference in the same file is
  independently resolvable and should still be checked).
- Generator: No — hand fixture only.

### OUT-09 — Reference inside a `for` expression

```hcl
inputs = {
  first_two_subnets = [for s in dependency.vpc.outputs.subnet_ids : s][0:2]
}
```

- Static resolution: Yes — the traversal source of the `for` is a normal, static
  attribute access; gruntled never executes the loop, it only walks the AST for
  traversal nodes, so laziness/eagerness of `for`-loop bodies (relevant to
  side-effecting functions, per `FEATURES.md`'s `sops_decrypt_file`-in-ternary note)
  is not a factor here at all — there is nothing to "evaluate," only to find.
- Behaviour: **Resolve** — extract `(dependency = "vpc", output = "subnet_ids")`.
- Generator: No — hand fixture only; same extraction mechanism as OUT-01, different
  surrounding syntax, worth one explicit AST-shape test.

**Reversed by gap G1 (02-06):** the row above covers only the `for` expression's
COLLECTION (the part after `in`), which HCL always evaluates and always surfaces
diagnostics for, so a reference there stays a Reference. A reference in the `for`
body's key, value or `if`-condition is different: hclsyntax runs those sub-expressions
once per source element, so zero times when the collection is empty at plan time, and a
missing output there need not be a runtime error. `refWalker` (refs.go) now suppresses
those three positions the same way it suppresses `try()`/`can()`, and
`TestExtractRefsLazyEvaluation` (refs_test.go) proves it, including the `%{ for }`
template-directive form (parses to `*hclsyntax.TemplateJoinExpr` wrapping the same
`*hclsyntax.ForExpr`).

### OUT-10 — Reference inside a ternary (both branches)

```hcl
inputs = {
  gateway_id = local.use_nat ? dependency.nat.outputs.gateway_id : dependency.igw.outputs.gateway_id
}
```

- Static resolution: Yes, for extraction of *both* branches — per `FEATURES.md`'s
  confirmed finding that HCL's full-body decode eagerly evaluates both branches of a
  ternary regardless of the runtime condition, a structural AST walk must likewise
  treat both traversals as present references, not attempt to pick a branch based on
  `local.use_nat`'s value (which Phase 2 never evaluates anyway, per the
  structural-decode-only design decision).
- Behaviour: **Resolve** both — extract `(dependency = "nat", output = "gateway_id")`
  and `(dependency = "igw", output = "gateway_id")` independently. A GRT001 mismatch
  on either target module must be reported regardless of which branch would "really"
  run at apply time — gruntled never knows which branch runs, and correctly does not
  need to.
- Generator: No — hand fixture only.

**Reversed by gap G1 (02-06):** this row's "eagerly evaluates both branches" premise
was wrong at the diagnostics layer. hclsyntax's `ConditionalExpr.Value` (v2.24.0 and
v2.25.0) does evaluate both `TrueResult` and `FalseResult` internally to compute
`trueVal`/`falseVal`, but it appends only the selected branch's diagnostics
(`trueDiags` or `falseDiags`) to the result; the other branch's are discarded even when
it references a missing output. try()/can() already established that gruntled treats
"HCL will not surface this as a runtime error" as "not a Reference" — the same
zero-false-positives rule now applies to both ternary branches, condition excepted (the
condition is always evaluated and its diagnostics always surface, so a reference there
is still a Reference). `refWalker` suppresses `ConditionalExpr.TrueResult` and
`.FalseResult` the same way it suppresses a `try()`/`can()` argument.
`TestExtractRefsLazyEvaluation` (refs_test.go) and the flipped OUT-10 row in
`TestExtractRefsShapes` prove it, including the `%{ if }` template-directive form
(parses to the same `*hclsyntax.ConditionalExpr`).

**Also gap G1: `&&`/`||` short-circuiting.** `hclsyntax.BinaryOpExpr` with
`Op == OpLogicalAnd` or `OpLogicalOr` uses a `ShortCircuit` evaluation that returns only
the controlling operand's diagnostics: `false && dependency.x.outputs.missing` and
`dependency.x.outputs.missing && false` both drop the reference operand's diagnostics
regardless of which side it is on. A reference in either operand of `&&` or `||` is
therefore NOT a Reference (same fail-silent treatment; any other `BinaryOpExpr`, such as
arithmetic or comparison, is unaffected and stays checked).

### OUT-11 — Reference inside `locals`, not `inputs`

```hcl
locals {
  vpc_id = dependency.vpc.outputs.vpc_id
}
inputs = {
  vpc_id = local.vpc_id
}
```

- Static resolution: Yes, for the traversal itself (`dependency.vpc.outputs.vpc_id`
  inside the `locals` block) — but only if the extractor's structural walk covers
  the *entire* unit body, not just the `inputs` block. PARSE-01 names `include`,
  `terraform.source` and `dependency` as the blocks gruntled *decodes* for graph
  construction; the separate GRT001-reference-extraction walk (searching for
  `outputs.*` traversals) must not be scoped down to `inputs` alone, or this common,
  idiomatic pattern (compute once in `locals`, reference from `inputs`) is silently
  missed entirely — a false negative, safe by policy, but one that would defeat a
  meaningful fraction of real-world GRT001 coverage.
- Behaviour: **Resolve** — extract from the `locals` block exactly as from `inputs`.
  This does not conflict with "never evaluate expression values" (`PROJECT.md`): the
  walk inspects the *syntax tree* of every attribute's expression looking for
  traversal nodes, it never evaluates `local.vpc_id`'s resulting value or chases it
  through to `inputs`.
- Generator: Yes — this needs explicit, positive golden coverage; it's the kind of
  scope gap ("we only extract from `inputs`") that looks correct against a narrow
  fixture set and quietly under-covers real repos.

### OUT-12 — Output reference to a dependency inherited via `include`

```hcl
# root.hcl
dependency "shared_kms" {
  config_path = "${get_parent_terragrunt_dir()}/kms"
}
# child/terragrunt.hcl (merge_strategy = deep, per DEP-13)
inputs = {
  kms_key_arn = dependency.shared_kms.outputs.arn
}
```

- Static resolution: Yes, but only after DEP-13's include-then-dependency resolution
  order is respected — the `dependency "shared_kms"` block doesn't exist in the
  child's own file at all.
- Behaviour: **Resolve**, using the effective (post-merge) dependency set.
- Generator: No — composes INC-06/DEP-13/OUT-01; hand fixture only.

### OUT-13 — Output reference on a `skip_outputs = true` dependency

```hcl
dependency "vpc" {
  config_path  = "../vpc"
  skip_outputs = true
}
inputs = {
  vpc_id = dependency.vpc.outputs.vpc_id
}
```

- Static resolution: Yes, for the traversal (structurally identical to OUT-01).
- Behaviour: **Resolve** the extraction and the module-surface check exactly as
  DEP-09 describes — this combination is unusual (arguably self-contradictory at
  Terragrunt-runtime) but does not make the static check any less meaningful, and
  gruntled should not special-case it into `Unknown` just because it looks odd.
- Generator: No — hand fixture only.

### OUT-14 — Merge/spread of the whole outputs map

```hcl
inputs = merge(
  dependency.vpc.outputs,
  { extra_tag = "value" }
)
```

- Static resolution: Yes — same reasoning as OUT-07: no specific `Y` name is
  asserted anywhere in this expression.
- Behaviour: **Resolve** as a no-op — zero extracted references, no crash, no
  phantom "outputs" attribute lookup.
- Generator: No — hand fixture only, a second AST shape (function-call argument
  rather than a bare RHS) exercising the same "whole object, nothing to check"
  invariant as OUT-07.

---

## 5. Stacks, `.terragrunt-cache`, and root-config naming

### STACK-01 — `.terragrunt-cache/` present on disk

```
repo/
  unit/
    terragrunt.hcl
    .terragrunt-cache/
      <hash>/<version>/  (full copy of a fetched remote module, no terragrunt.hcl)
```

- Static resolution: Yes — the directory name is the documented Terragrunt default
  (`PITFALLS.md` Pitfall 6).
- Behaviour: **Skip** (walk exclusion, PARSE-06) — never descend into it, regardless
  of what it contains.
- Generator: Yes — the generator should be able to emit a fake `.terragrunt-cache/`
  alongside a real unit, so the walker's exclusion is exercised at generator scale,
  not just in one hand-written fixture.

### STACK-02 — `.terraform/` present on disk

```
repo/
  unit/
    terragrunt.hcl
    .terraform/
      modules/  (init-time artifacts, no terragrunt.hcl)
```

- Static resolution: Yes — same class as STACK-01, different default directory name.
- Behaviour: **Skip** (PARSE-06).
- Generator: No — identical exclusion mechanism to STACK-01; hand fixture only.

### STACK-03 — Decoy `terragrunt.hcl` inside `.terragrunt-cache/`

```
repo/
  unit/
    terragrunt.hcl
    .terragrunt-cache/
      <hash>/<version>/
        terragrunt.hcl   # a remote *module* that happens to itself be Terragrunt-structured
```

- Static resolution: Yes.
- Behaviour: **Skip** — path-prefix exclusion must apply unconditionally,
  independent of the excluded directory's contents; this is the one fixture
  `PITFALLS.md` explicitly calls out as necessary to prove exclusion isn't merely
  "skip empty-looking cache dirs."
- Generator: Yes — directly named in `PITFALLS.md`'s Pitfall 6 verification note as
  the fixture that proves the exclusion is unconditional; cheap to add to the
  generator's output alongside STACK-01.

### STACK-04 — Symlinked directory or file

```
repo/
  unit/
    terragrunt.hcl
  shared -> /outside/the/repo/modules   (symlink)
```

- Static resolution: Yes — symlinks are cheaply detectable via `Lstat`/`DirEntry`
  type before following them.
- Behaviour: **Skip** (PARSE-06 names symlinks explicitly). Never follow a symlinked
  directory or file during the walk; if a `dependency`/`source` path happens to
  *point through* a symlink (rather than the walk encountering one directly), that's
  a separate, narrower question (path resolution, not walk traversal) and is not
  itself required to be excluded — the PARSE-06 requirement is about the *tree walk*
  discovering units, not about every possible filesystem path a config might name.
- Generator: No — hand fixture only; Go's `filepath.WalkDir` behavior around symlinks
  needs a real filesystem fixture, not something naturally expressible by the
  synthetic generator's spec (unit count/depth/fanout) without extra machinery.

### STACK-05 — Symlink cycle

```
repo/
  a -> b
  b -> a
```

- Static resolution: Yes — detectable the same way as STACK-04 (don't follow
  symlinks at all, full stop) — this isn't actually a harder case than STACK-04 if
  the walker simply never follows symlinks; it's listed separately because "never
  follow symlinks" and "detect and break cycles when following symlinks" are two
  different design choices, and only the former is compatible with PARSE-06's literal
  wording ("symlinks... never walked into").
- Behaviour: **Skip**. Confirms the walker's symlink handling is "never follow," not
  "follow with cycle detection" — the simpler and correct choice per PARSE-06.
- Generator: No — hand fixture only, adversarial by construction.

### STACK-06 — Ungenerated `terragrunt.stack.hcl`

```
repo/
  stacks/prod/terragrunt.stack.hcl   # no .terragrunt-stack/ committed anywhere
```

- Static resolution: Yes, that it exists and was deliberately not parsed — `.stack.hcl` parsing is explicitly out of scope for v0.1 (`PROJECT.md` Out of Scope,
  `FEATURES.md` Question 4 recommendation).
- Behaviour: **Skip** the file's *content* entirely (never attempt to decode
  `unit`/`autoinclude` blocks inside it), but the walk should still notice the file
  exists and count it, so Phase 3's CLI layer can distinguish "0 units found because
  the repo is empty" from "0 units found because this repo uses ungenerated Stacks
  and gruntled can't see them yet" (`PITFALLS.md` Pitfall 2's UX recommendation). This
  is a structural fact Phase 2 is well-positioned to capture cheaply (a count on the
  walk result) even though acting on it in the CLI is Phase 3's job.
- Generator: No — hand fixture only; out-of-scope-by-design feature, not something
  the spec-driven generator should produce as a first-class shape in v0.1.

**Revised by gap G2 (02-07):** the walk-level Skip above still holds, but a
`dependency` can also point at a stack. A dependency whose `config_path` names a
directory holding `terragrunt.stack.hcl` (even when it also holds a `terragrunt.hcl`),
or names a `terragrunt.stack.hcl` file directly, is unresolved `config-path-stack`
(`ReasonConfigPathStack`) and never a target. Terragrunt's `getTerragruntOutput` tries
the stack's outputs first (03-RESEARCH Pattern 3), so the dependency's outputs are not
the directory's unit module's outputs. Tests: `TestUnknownReasons` rows
`config-path-stack/dir-only-stack`, `/dir-with-both` and `/file`.

### STACK-07 — Generated `.terragrunt-stack/` with real, materialized `terragrunt.hcl` files

```
repo/
  stacks/prod/terragrunt.stack.hcl
  stacks/prod/.terragrunt-stack/
    vpc/terragrunt.hcl     # ordinary, real, fully-formed unit config
    mysql/terragrunt.hcl
```

- Static resolution: Yes.
- Behaviour: **Resolve** as ordinary units — `.terragrunt-stack/` is *not* one of the
  PARSE-06 excluded directory names (only `.terragrunt-cache`, `.terraform`, vendored
  module dirs and symlinks are excluded); this is the mechanism by which `PROJECT.md`
  justifies deferring Stacks support ("the tree walk finds them for free"). Do not
  add `.terragrunt-stack` to the exclusion list by analogy with `.terragrunt-cache` —
  they look superficially similar in name but serve opposite roles (one holds decoys
  to skip, the other holds real units to find).
- Generator: No — the generator can already produce this shape trivially, since
  post-generation it's indistinguishable from any other directory of real units; no
  Stacks-specific generator support is needed to exercise this path. Hand fixture
  only, if exercised at all before Phase 4's real-repo validation (`terragrunt-
  infrastructure-catalog-example`'s `units/*` after a local `stack generate` run
  would be the natural real-world source, per `PITFALLS.md` Part A #3).

### STACK-08 — Root config named `root.hcl` vs `terragrunt.hcl`

```
repo/
  root.hcl              # current Terragrunt convention: no `terraform` block, include-only
  team-a/terragrunt.hcl # include "root" { path = find_in_parent_folders() }
```

versus the older convention:

```
repo/
  terragrunt.hcl         # same role: include-only, no `terraform` block
  team-a/terragrunt.hcl
```

- Static resolution: Yes — role (parent/include-only config vs. runnable unit) must
  be determined by *content* (presence or absence of a `terraform` block referencing
  a module, per GRAPH-01/GRAPH-02's own Unit definition), never by filename. Both
  `root.hcl` and `terragrunt.hcl` are accepted filenames for `find_in_parent_folders()`'s
  default lookup in current Terragrunt versions, specifically because a root
  `terragrunt.hcl` at the repo root, discoverable by any glob-based unit finder, used
  to risk being mistaken for a runnable unit itself.
- Behaviour: **Resolve**. A directory containing a parse-able Terragrunt config file
  (either name) with no `terraform` block and no local `.tf` files is a parent/include
  config, not a Unit — exclude it from the `Unit` list the walk produces, while still
  parsing and sharing it (PARSE-03) as an include target for any child that references
  it. Do not assume the filename alone determines this; a repo can name its root
  config `terragrunt.hcl` and still intend it as include-only, and (per STACK-09) a
  config file can legitimately be both an include target *and* a runnable unit at
  once.
- Generator: Yes — both filenames should be exercised at least once so a
  filename-based shortcut (e.g. "any file named `root.hcl` is never a unit") never
  gets baked in by accident; the content-based rule is the one that must hold.

### STACK-09 — A `terragrunt.hcl` that is both included and independently runnable

```hcl
# platform/terragrunt.hcl — has its own terraform block...
terraform {
  source = "../modules/platform-baseline"
}
# ...and is also included by children:
# platform/team-a/terragrunt.hcl
include "platform" {
  path = find_in_parent_folders()
}
```

- Static resolution: Yes.
- Behaviour: **Resolve** as both simultaneously — a real Unit (it has its own
  `terraform.source`, GRAPH-01) *and* a shared include target (PARSE-03) for its
  children. These are orthogonal roles; being an include target must never disqualify
  a config from also being counted as a Unit, and vice versa.
- Generator: No — hand fixture only; a valid but unusual combination worth one
  explicit regression test rather than spec-scale generation.

**Revised by gap G3 (02-07):** a `terragrunt.hcl` that other units include is now
config-unknown `include-target` (`ReasonIncludeTarget`). Its references are still
checked, once per including unit, because include-file facts are attributed to the
including unit. Analysing it standalone resolved them against the wrong directory: the
cds-snc/secret corpus false GRT001 at `live/terragrunt.hcl:4:21` (03-RESEARCH Pattern 4).
The cost is that a parent config that is ALSO run standalone is never checked on its
own. That is a documented false negative, accepted under "ten false negatives beat one
false positive". Tests: `TestUnknownReasons` row `include-target`,
`TestIncludeTargetCorpusReproduction`, `TestIncludeTargetExplicitPath`,
`TestIncludeTargetKeepsEarlierConfigUnknownReason` and the env-gated
`TestIncludeTargetSecretCorpus`.

### STACK-10 — Custom `download_dir`

```hcl
# root.hcl
download_dir = ".my-cache"
```

versus (not statically visible at all):

```
TG_DOWNLOAD_DIR=.my-cache terragrunt run-all plan
```

- Static resolution: Partial — resolvable when `download_dir` is set as a literal
  string attribute in a parsed config file (chase it the same way as any other
  literal attribute read during PARSE-01/PARSE-03); **not** resolvable when set only
  via the `TG_DOWNLOAD_DIR`/`TERRAGRUNT_DOWNLOAD_DIR` environment variable or a
  `--terragrunt-download-dir` CLI flag at invocation time, neither of which is
  visible to a pure file-tree read (`PITFALLS.md` Pitfall 6).
- Behaviour: **Resolve** and add to the exclusion set when the literal attribute is
  present in a parsed config; when it is not, fall back to excluding only the default
  `.terragrunt-cache` name and treat the env-var/CLI-flag case as a documented,
  known gap — not a crash, not a silent misreport, just an acknowledged blind spot
  consistent with the project's "ten false negatives beat one false positive"
  priority (a custom-cache directory with a `.tf`-containing decoy that isn't
  excluded could in principle inflate the unit count, which is a false *negative* in
  the opposite direction — never a false positive on GRT001 itself, since decoy
  directories don't participate in dependency resolution unless something explicitly
  points a `dependency`/`source` at them, which would be a separate, extremely
  unusual repo misconfiguration).
- Generator: No — hand fixture only; low real-world prevalence, and the literal-vs-
  environment split is best proven with one targeted case each rather than spec-scale
  coverage.

### STACK-11 — Oversize or overdeep unit or include file

```hcl
# live/app/terragrunt.hcl, or a root.hcl it includes
inputs = { x = ((((((( ... 200000 levels ... ))))))) }
```

- Static resolution: No. hclsyntax parses recursively, and a deeply nested file
  overflows the goroutine stack. Go cannot recover from that: `fatal error: stack
  overflow` killed the whole process (02-REVIEW G7).
- Behaviour: **Unknown**. A unit file or include file larger than
  `hclconv.MaxFileBytes` (4 MiB) or nested deeper than `hclconv.MaxNestingDepth` (1000)
  is never parsed. Every unit reading it is config-unknown `config-too-large`
  (`ReasonConfigTooLarge`) or `config-too-deep` (`ReasonConfigTooDeep`), gap G7b, 02-11.
  No GRT100 is emitted, because the file is not known to be invalid. A shared include
  is still read once.
- Tests: `TestDeepNestingNoCrash`, `TestDeepSharedIncludeNoCrash`,
  `TestFileCacheLimits`, and `TestUnknownReasons` rows `config-too-large/unit-file`,
  `/include`, `config-too-deep/unit-file`, `/include`.
- Generator: No; inputs are generated in the test, never committed.

### STACK-12 — Oversize or overdeep module file

- Static resolution: No, for the same reason as STACK-11, on the module side.
- Behaviour: **Unknown**. A kept module file (`.tf`, `.tf.json`, `.tofu`,
  `.tofu.json`) over the same limits makes the module's surface unknown
  `module-file-too-large` or `module-file-too-deep` (tfsurface
  `ReasonModuleFileTooLarge` / `ReasonModuleFileTooDeep`, gap G7a, 02-10), with no
  diagnostic. A reference into that module is not checked.
- Tests: tfsurface `TestReadSurfaceDeepNestingNoCrash`, `TestReadSurfaceDeepJSONNoCrash`,
  `TestReadSurfaceOversizeFile`, `TestReadSurfaceLimitPrecedence`; end to end,
  terragrunt `TestDeepModuleFileEndToEnd`.
- Generator: No.

---

## Cross-cutting notes for implementation

- **Extraction is a separate walk from graph construction.** Building the
  `RepositoryGraph` (GRAPH-01..05) needs only `include`, `terraform.source` and
  `dependency` blocks (PARSE-01). Finding `dependency.X.outputs.Y` references for
  GRT001 needs a walk over the *entire* effective HCL body of a unit — `inputs`,
  `locals`, and (in principle) any other block — since real repos put these
  traversals in `locals` as often as `inputs` (OUT-11). Conflating the two into one
  narrowly-scoped decode is the single most likely way this phase under-covers real
  repositories without ever producing a visible test failure, because every hand-
  written fixture that only puts the traversal in `inputs` would still pass.
- **`Unknown` has a blast radius that must be as small as correctness allows.** A
  remote source on one unit (SRC-06) should make only *that* unit's module surface
  `unknown` — not poison every other unit that happens to share an `include`d root
  config with it, and not poison units that merely *depend on* it (GRAPH-04's first
  hop, `config_path` → unit, still succeeds even when the second hop fails). Every
  `unknown` marking should carry a machine-readable reason string, per the UX pitfall
  in `PITFALLS.md`, so Phase 3/CLI can eventually distinguish `remote-source` from
  `dynamic-source-expression` from `environment-dependent-include` in verbose output.
- **`Report`-class findings in this catalogue are not all Phase-2-owned diagnostics.**
  Only `GRT100` (invalid HCL, PARSE-05) belongs to Phase 2 itself. Everything else
  marked `Report` above (DEP-04, DEP-10, DEP-11, DEP-12, INC-09, SRC-13) must be
  *preserved on the domain model* — as a distinct condition, not silently folded into
  either a normal resolved edge or an `unknown` marker — so that Phase 3 (or a later
  v2 `GRT00x`) can decide how and whether to surface it. Losing this information
  during Phase 2 graph construction cannot be recovered later without re-parsing.

### Gap closure (02-06..02-11)

The 02-REVIEW gaps and the plan that closed each:

- G1 (lazy-evaluation reference guard: unselected ternary branch, short-circuited
  `&&`/`||`, `for` body): 02-06. OUT-09/OUT-10 reversals above.
- G2 (stack target, `config-path-stack`): 02-07. STACK-06 revision.
- G3 (include-target units, `include-target`): 02-07. STACK-09 revision.
- G4 (non-default config file, `config-path-nondefault-file`): 02-07. DEP-14.
- G5 (JSON include, `include-json-unsupported`): 02-07. INC-13.
- G6 (overlay ReadDir failure, `module-file-unreadable`): 02-07. SRC-15.
- G7a (module file size/depth, `module-file-too-large` / `module-file-too-deep`): 02-10.
  STACK-12.
- G7b (unit/include file size/depth, `config-too-large` / `config-too-deep`): 02-11.
  STACK-11.
- G8 (invalid config_path, `config-path-invalid`): 02-07. DEP-15.
- G9 (malformed generate, `invalid-generate`): 02-07. SRC-14.
- G10 (two-input fuzz target over root.hcl and the dependency's module file): 02-11.
- G11 (wrong loader.go comment): 02-07.
- G12 (zero values invalid everywhere in repograph): 02-08.
- G13, G14 (architecture check: unpoliced `internal/*` dirs, build-constrained
  files): 02-09.
