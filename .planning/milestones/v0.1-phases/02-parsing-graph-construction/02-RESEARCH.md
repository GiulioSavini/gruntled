# Phase 2: Parsing & Graph Construction - Research

**Researched:** 2026-09-25
**Domain:** Structural HCL decoding of Terragrunt configs, Terragrunt include/path-function semantics, Terraform/OpenTofu module surface extraction, and hexagonal graph assembly in Go
**Confidence:** HIGH. Terragrunt semantics were read from Terragrunt's own source at `main@5dc737a` (2026-09-24). hcl/v2 and Go stdlib behaviour was verified by running scratch programs. The corpus claims were verified by prototyping the two-hop resolution against the real primary corpus.

<user_constraints>
## User Constraints

No CONTEXT.md exists for Phase 2. The orchestrator gave these constraints as **locked**, and they carry over from PROJECT.md and the Phase 1 CONTEXT:

### Locked Decisions
- gruntled uses its **own HCL parsing** (`hashicorp/hcl/v2` is fine) and does **NOT** import the Terragrunt library.
- **No cache**, meaning no on-disk index. **No external binaries.** No network at runtime.
- gruntled **never evaluates expression values**. The one sanctioned exception is PARSE-02: it may evaluate the six pure path functions inside path-bearing attributes. See Architecture Pattern 3.
- **DDD/hexagonal**:
  - HCL lives only in `internal/infrastructure`.
  - Ports live in `internal/application` (`ports/`).
  - `internal/domain` stays pure.
  - This is CI-enforced by `scripts/check-architecture.sh`, which comes with a self-test proving each rule can fail.
- Layout per Phase 1 CONTEXT (research/ARCHITECTURE.md §B.6): `cmd/gruntled` is the composition root only. Phase 2 creates only the packages it needs.
- Zero false positives. When unsure, the unit becomes `unknown` and the analyzer stays silent.
- Deterministic output: sort explicitly, and use repo-relative paths only.
- Repo conventions: commits are authored by Giulio Savini <giuliosavini@proton.me>, with no Co-Authored-By trailer. Every change is committed, pushed and tested.

### Claude's Discretion
- Exact port shapes, package split inside `internal/infrastructure`, DTO field names, and unknown-reason strings.
- Which of the edge constructs are resolved and which become `unknown` (always biased toward `unknown`).

### Deferred Ideas (OUT OF SCOPE)
- The GRT001 analyzer, the `mock_outputs` rule (DIAG-03), the CLI, and presenters belong to Phase 3.
- The corpus experiment and benchmark belong to Phase 4.
- Terragrunt Stacks parsing (`terragrunt.stack.hcl`), the daemon/watch mode, and `read_terragrunt_config`/`locals` evaluation are out of scope.
</user_constraints>

<phase_requirements>
## Phase Requirements

| ID | Description | Research Support |
|----|-------------|-----------------|
| PARSE-01 | Decode `terragrunt.hcl` structurally, reading only `include`, `terraform.source`, `dependency` | Pattern 2 (parse-once cache + `hclsyntax` AST). Pattern 3 (closed EvalContext on path attributes only). Pattern 5 (reference extraction by AST walk) |
| PARSE-02 | Evaluate the six pure path functions | Pattern 4. The semantics table was derived from Terragrunt `pkg/config/config_helpers.go`, with virtual-root absolute paths |
| PARSE-03 | Resolve `include`, including nested includes and merge strategy, parsing each file once | Pattern 2 (cache keyed by repo path). Pattern 6 (merge precedence child > last include > first include; `no_merge` excluded). "Nested include" semantics are clarified under Pitfall 3 |
| PARSE-04 | Mark a unit `unknown` on any offline-unresolvable construct, and emit no diagnostic for it | The unknown-reason catalogue (Pattern 7). Only the three path-bearing attributes need resolving, and everything else stays AST-only |
| PARSE-05 | Report a diagnostic instead of crashing on invalid HCL, including mid-edit files | Verified: `hclsyntax.ParseConfig` returns a non-nil partial file plus diagnostics for mid-edit and non-UTF-8 input. Pattern 8 covers conversion to a `GRT100` domain Diagnostic with byte columns. A fuzz target is recommended |
| PARSE-06 | Skip `.terragrunt-cache`, `.terraform`, vendored dirs and symlinks while walking | Pattern 1 (`fs.WalkDir` over `os.Root.FS()` never follows symlinks, verified). Skip set is taken from Terragrunt `util.SkipDirIfIgnorable` |
| GRAPH-01 | Unit → module via `terraform.source` when present | Pattern 9 (source classifier mirroring Terragrunt's detector chain, with `//` subdir handling) |
| GRAPH-02 | Unit → own directory when `source` is absent | Pattern 9. This is the corpus shape: all 65 units, one of them with an empty `terragrunt.hcl` |
| GRAPH-03 | Classify remote sources without downloading, and mark the unit `unknown` | Pattern 9. Hand-rolled string classifier plus an in-repo existence check as a safety net |
| GRAPH-04 | Resolve `dependency` through both hops | Pattern 10 (application-layer assembly). `config_path` resolves against the **child** unit dir even when declared in an include (verified in the Terragrunt source) |
| GRAPH-05 | Extract `variable`/`output` names from `.tf` and `.tf.json` | Pattern 11 (hand-rolled `hcl/v2` surface reader, also `.tofu`/`.tofu.json`, following in-repo file symlinks). The corpus depends on the symlinks: see Pitfall 1 |
</phase_requirements>

## Summary

Phase 2 turns a Terragrunt repository on disk into the Phase 1 `repograph.RepositoryGraph`. It is built from:
- `hashicorp/hcl/v2` v2.25.0, together with `go-cty` v1.19.0 for the six path functions;
- the Go 1.27 standard library `os.Root`/`io/fs`;
- three infrastructure adapters, `terragrunt`, `sourceresolve` and `tfsurface`;
- one application use case, `indexing`, which assembles the graph behind two ports.

No other third-party dependency is needed. Specifically, **do not** add `terraform-config-inspect` or `go-getter`. The reasons are under Standard Stack.

The Terragrunt semantics that matter were read directly from Terragrunt's source:
- Included files are evaluated in the **child's** context. `get_terragrunt_dir()`, `find_in_parent_folders()` and relative `dependency.config_path` all resolve against the child unit's directory.
- **Nested includes are not supported by Terragrunt**, which raises `TooManyLevelsOfInheritanceError`. An include file containing an `include` block therefore makes the unit `unknown`. "Nested includes" in PARSE-03 must mean *multiple include blocks per unit, at different directory depths*, each shared by many units.
- Merge precedence is **child > last include > … > first include**, and `no_merge` includes contribute nothing.
- Terragrunt's source classification runs every source through a go-getter detector chain whose **last detector turns any unmatched string into a local path**. So `source = "units/chicken"` (no `./`) is local in Terragrunt, unlike Terraform. This corrects research/ARCHITECTURE.md §A.5 rule 5.
- Terragrunt **copies the unit directory's files over the module copy**. A unit whose `source` points elsewhere but whose own directory holds `.tf` files has a surface that is not the module's.

Four findings from the empirical checks change the plan:
1. **The primary corpus symlinks `global.tf` into every one of its 65 module directories.** 68 symlinks in total, each target inside the repo. PARSE-06 ("symlinks never walked into") must therefore apply to the **unit-discovery walk only**. The surface reader must follow in-repo *file* symlinks through `os.Root`, which refuses links that escape the root (verified). If it rejected symlinked `.tf` files or marked such modules `unknown`, every unit in the corpus would go `unknown` and Phase 4 could never catch a mutation. With this design, a prototype over the corpus resolved **22/22 references with 0 missing**. After renaming `output "role_name"` it reported **8 missing** (VALID-03/04 shape proven early).
2. **`hcl.Pos.Column` counts grapheme clusters, not bytes**, but the domain `Position` column is byte-counted (the synthrepo manifest uses bytes too). Column must be recomputed from `Pos.Byte` (verified: `"ééé"` prefix gave Column 21 against a byte column of 23).
3. **`hclsyntax` walks a body's `Attributes` by ranging over a map**, so `VisitAll`/`Walk` order is random. Every collected slice must be sorted (verified in the hcl source).
4. Several inputs are legitimately silent:
   - a reference wrapped in `try()`/`can()`;
   - references to the whole object (`dependency.x.outputs`, `lookup(dependency.x.outputs, …)`, splats, dynamic indexes);
   - expanded dependencies (`dependency.a["k"].outputs.y`, Terragrunt v1.2.0 `expansion` block).

   They must not produce a `Reference`. An `outputs["literal"]` index is a normal reference.

**Primary recommendation:** Keep the pipeline in three stages:
1. **Terragrunt adapter.** It walks the tree, parses each file once, evaluates the three path-bearing attributes, merges includes and extracts references.
2. **Surface reader.** It reads each distinct module directory once and yields a Surface or an unknown reason.
3. **Pure application assembly.** It builds the graph from the two ports' DTOs and is testable with fakes.

Bias every ambiguity toward `unknown`, and prove parse-once, determinism and the corpus shape with tests.

## Standard Stack

### Core
| Library | Version | Purpose | Why Standard |
|---------|---------|---------|--------------|
| `github.com/hashicorp/hcl/v2` | **v2.25.0** (2026-09-15, `go 1.25.0`) | `hclsyntax.ParseConfig`, `hcljson.Parse`, `Body.PartialContent`, `hclsyntax.Walk`, `Expression.Value` | The parser Terraform and Terragrunt themselves use. Terragrunt pins v2.24.0. v2.25.0 adds "Prepare for Unicode 17 support in Go 1.27" (#814), which matches gruntled's `go 1.27` |
| `github.com/zclconf/go-cty` | v1.19.0 (pinned by hcl v2.25.0) | `cty/function.New` for the six path functions, and reading `cty.String` values | Required by the hcl evaluator. Already a transitive dependency |
| Go stdlib `os.Root` / `io/fs` / `testing/fstest` | Go 1.27 toolchain (go.mod `go 1.27`) | `os.OpenRoot(repo).FS()` implements `StatFS`, `ReadFileFS`, `ReadDirFS` and `ReadLinkFS` and refuses symlink escape. `fs.WalkDir` gives lexical order and does not follow symlinks. `fstest.MapFS` supports symlinks, `Lstat` and `ReadLink` | Verified with `go doc` and scratch programs on go1.27.0. Everything stays repo-relative by construction, so no absolute path can leak |

### Supporting
| Library | Version | Purpose | When to Use |
|---------|---------|---------|-------------|
| `github.com/hashicorp/hcl/v2/json` | bundled | Parse `.tf.json`/`.tofu.json` for surface extraction | Only in `tfsurface` |
| `internal/testsupport/synthrepo` (Phase 1, plan 01-03) | in-repo | `Render(spec)` gives a pure in-memory `Tree`, which is easy to turn into an `fstest.MapFS`. `Generate` writes to disk for `os.Root` tests | Used by the parse-once, determinism and scale tests. It may be imported only from `_test.go` files |

### Alternatives Considered
| Instead of | Could Use | Tradeoff |
|------------|-----------|----------|
| Hand-rolled surface reader on `hcl/v2` | `hashicorp/terraform-config-inspect` (recommended in research/ARCHITECTURE.md §A.4) | **Rejected for Phase 2.** Its `fileExt` recognises only `.tf`, `.tf.json`, `.tfcomponent.hcl` and `.tfstack.hcl`, and **ignores OpenTofu `.tofu`/`.tofu.json`**. A module that declares outputs in `.tofu` would be under-counted, which is a false positive. It also returns a partial `Module` on syntax errors (under-counting again), pulls legacy `hashicorp/hcl` v1 and `go 1.18`-era deps, and its `FS` interface predates `io/fs` (needs `WrapFS`). The replacement is about 80 lines on the same parser |
| Hand-rolled source classifier | `hashicorp/go-getter/v2` `Detect` | **Rejected.** Terragrunt's real chain lives in `internal/getter` and `internal/tf` (not importable). It adds its own BitBucket detector and prefixed GCS/S3 detectors. go-getter/v2 would add about 10 modules (netrc, compress, xz, …) for one pure function and still diverge. The classifier is small, and the existence check makes any misclassification fail safe: both directions end in `unknown` |
| `hcl.EvalContext` for path attributes | Custom mini-evaluator over `TemplateExpr`/`FunctionCallExpr` | The closed EvalContext is already safe. With `Variables: nil` you get "Variables not allowed", and with only six functions any other call gives "Call to unknown function" (both verified). It also handles templates and conditionals correctly for free |
| `hclparse.Parser` | Direct `hclsyntax.ParseConfig` | `hclparse` reads through `os` by filename and has its own cache. Use `hclsyntax.ParseConfig(src, repoRelPath, hcl.InitialPos)` over `fs.FS` so ranges carry repo-relative filenames |

**Installation** (in the repo, Wave 0):
```bash
go get github.com/hashicorp/hcl/v2@v2.25.0
go mod tidy
```

## Architecture Patterns

### Recommended Project Structure
```
internal/
  application/
    ports/ports.go            # UnitLoader, SurfaceReader + DTOs (domain types only; no hcl, no io/fs)
    indexing/build.go         # Build(ctx, UnitLoader, SurfaceReader) -> (*repograph.RepositoryGraph, []diagnostic.Diagnostic, error)
    indexing/build_test.go    # fakes only, no filesystem
  infrastructure/
    terragrunt/               # ACL: walk + parse-once cache + path functions + include merge + refs
      walk.go  parse.go  pathfuncs.go  eval.go  include.go  refs.go  position.go  loader.go
      testdata/ (optional)    # prefer fstest.MapFS fixtures inline
    sourceresolve/classify.go # pure: source string -> Local{root,subdir} | Remote | Invalid
    tfsurface/reader.go       # module dir -> Surface | unknown reason + GRT100 diags
scripts/check-architecture.sh # + new rules (see Pattern 12) and self-test cases
```
The structure has two ports, not three. Source classification needs the evaluation context, which uses the virtual root (Pattern 4), and that context is internal to the Terragrunt anti-corruption layer. `sourceresolve` is therefore a pure collaborator package called by `terragrunt`, not a port.

The module-directory existence check falls out of `SurfaceReader`. A missing or empty directory gives an unknown reason.

### Pattern 1: Unit discovery walk
**What:** Walk the repository with `fs.WalkDir(fsys, ".", …)` over `os.OpenRoot(repo).FS()`.
- A unit is a directory containing a regular file named `terragrunt.hcl`, or `terragrunt.hcl.json`, which becomes `unknown` (reason `json-config-unsupported`).
- Skip these directory names: `.git`, `.terraform`, `.terragrunt-cache` (Terragrunt's own `util.SkipDirIfIgnorable` set), plus `vendor` for PARSE-06's "vendored module directories".
- Any entry with `d.Type()&fs.ModeSymlink != 0` is ignored. `WalkDir` reports a symlinked directory as a non-directory entry and never descends into it (verified).

**Do NOT skip `.terragrunt-stack`.** PROJECT.md says generated stack units are "ordinary `terragrunt.hcl` files on disk, which gruntled's tree walk then finds for free". Terragrunt's own discovery includes them too. research/PITFALLS.md §6 says to skip it, but that contradicts PROJECT.md, and PROJECT.md is authoritative.

`WalkDir` is lexical, so results are already deterministic. Do not re-sort, and never collect into a map and range over it.

```go
// Source: io/fs.WalkDir docs (go1.27) + terragrunt internal/util/file.go SkipDirIfIgnorable
var skipDirs = map[string]bool{".git": true, ".terraform": true, ".terragrunt-cache": true, "vendor": true}

func discoverUnits(fsys fs.FS) ([]string, error) {
	var units []string
	err := fs.WalkDir(fsys, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err // or record + continue; never panic
		}
		if d.Type()&fs.ModeSymlink != 0 {
			return nil // never walk into / count symlinks (PARSE-06)
		}
		if d.IsDir() {
			if p != "." && skipDirs[d.Name()] {
				return fs.SkipDir
			}
			return nil
		}
		if d.Name() == "terragrunt.hcl" || d.Name() == "terragrunt.hcl.json" {
			units = append(units, path.Dir(p)) // "." for a root-level unit
		}
		return nil
	})
	return units, err
}
```
If a directory has both `terragrunt.hcl.json` and `terragrunt.hcl`, Terragrunt picks the JSON one (`DefaultTerragruntConfigPaths` lists JSON first), so the unit is `unknown`.

### Pattern 2: Parse-once cache (the speed claim, PARSE-03)
**What:** One `map[string]*parsedFile` keyed by the cleaned repo-relative path. Every file (unit config or include) is read with `fs.ReadFile` and parsed with `hclsyntax.ParseConfig(src, repoRel, hcl.InitialPos)` **once**. Structural facts are extracted once and stored as unevaluated `hcl.Expression`s:
- include blocks;
- `terraform{source}`;
- dependency blocks with their raw `config_path`, `enabled`, `skip_outputs` and mock attributes;
- generate blocks;
- references.

Only the cheap step of evaluating path expressions runs per unit.

```go
type parsedFile struct {
	path   string          // repo-relative, slash-separated
	src    []byte          // kept for byte-column conversion
	body   *hclsyntax.Body // nil if the file could not be parsed at all
	syntax hcl.Diagnostics // errors only; non-empty => every unit touching this file is unknown
	// extracted facts (unevaluated), each slice sorted by source position
	includes   []includeDecl
	terraforms []*hclsyntax.Block // >1 => unknown (Terragrunt decode error)
	deps       []depDecl
	generates  []genDecl
	refs       []rawRef
}

func (l *Loader) file(p string) *parsedFile {
	if pf, ok := l.cache[p]; ok {
		return pf
	}
	src, err := fs.ReadFile(l.fsys, p)
	pf := &parsedFile{path: p, src: src}
	if err == nil {
		f, diags := hclsyntax.ParseConfig(src, p, hcl.InitialPos)
		pf.syntax = diags.Errs() // keep error-severity only
		if !diags.HasErrors() {
			pf.body = f.Body.(*hclsyntax.Body)
			pf.extract()
		}
	}
	l.cache[p] = pf
	return pf
}
```
Keep the loader single-threaded in v0.1. If it is ever parallelised, guard each key with `sync.Once` and still sort outputs.

**Test (the PARSE-03 success criterion):** wrap the FS in a counter whose `ReadFile(name)` increments `count[name]`. Build over `synthrepo.Render(Spec{Units: 50, IncludeDepth: 3, …})` as an `fstest.MapFS`, then assert `count["root.hcl"] == 1` and that every count is `<= 1`. The counter must implement `fs.ReadFileFS`, `fs.StatFS` and `fs.ReadDirFS`, or `fs.ReadFile` falls back to `Open`.

### Pattern 3: Closed evaluation of path-bearing attributes only
**What:** gruntled evaluates exactly these attributes: `include.path`, `terraform.source` and `dependency.config_path`. It also reads literal-only flags (`include.merge_strategy`, and `dependency.enabled`/`skip_outputs`) in a tri-state known/unknown form. Everything else (`inputs`, `locals`, `remote_state`, `generate.contents`, `mock_outputs` values) is **AST-only**.

The EvalContext has `Variables: nil`. Its `Functions` holds exactly the six path functions and nothing else: no stdlib, no `run_cmd`, no `get_env`.

A result is usable only if it has no error diagnostics, `IsKnown()`, `!IsNull()` and type `cty.String`. Anything else makes the unit `unknown`. This matches Terragrunt, which evaluates include blocks before locals ("you cannot reference locals in the include block config", per the `ParseConfig` doc comment).

```go
// Source: verified in scratch program against hcl v2.25.0
func evalPath(expr hcl.Expression, fns map[string]function.Function) (string, bool) {
	v, diags := expr.Value(&hcl.EvalContext{Functions: fns}) // Variables nil => "Variables not allowed"
	if diags.HasErrors() || !v.IsWhollyKnown() || v.IsNull() || !v.Type().Equals(cty.String) {
		return "", false // local.x, get_env(), run_cmd(), format(), dirname(), get_repo_root() ... => unknown
	}
	return v.AsString(), true
}
```
Verified outcomes:
- `"${get_terragrunt_dir()}/../modules//vpc"` evaluates.
- `"a" == "a" ? "../x" : "../y"` gives `"../x"`.
- `"${local.x}/y"` fails with "Variables not allowed".
- `format(...)`, `get_env(...)` and `run_cmd(...)` fail with "Call to unknown function".

### Pattern 4: The six path functions (PARSE-02), with Terragrunt-exact semantics
Derived from `pkg/config/config_helpers.go` (`GetTerragruntDir`, `getOriginalTerragruntDir`, `GetParentTerragruntDir`, `findInParentFoldersImpl`, `PathRelativeToInclude`, `PathRelativeFromInclude`, `getSelectedIncludeBlock`) and `include.go` (`getTrackInclude`). The key fact: **when an included file is parsed, `pctx.TerragruntConfigPath` is still the child's path.** Included files are evaluated "as if" they were the child, apart from the include-tracking state.

Three evaluation scopes exist:
- **S0:** the child's own `include { path = … }` attributes. `TrackInclude` is nil here, because Terragrunt calls `WithTrackInclude(nil)` before `DecodeBaseBlocks`.
- **S1:** the child body.
- **S2:** an expression written inside an included file, which Terragrunt calls `TrackInclude.Original = that include`.

| Function | S0 (include.path) | S1 (child body) | S2 (inside included file X) |
|---|---|---|---|
| `get_terragrunt_dir()` | V(unit) | V(unit) | V(unit) |
| `get_original_terragrunt_dir()` | V(unit) | V(unit) | V(unit) (differs only under `read_terragrunt_config`, which is unsupported) |
| `find_in_parent_folders(name?, fallback?)` | search starts at the **parent** of unit dir | same | same (the docs say it "searches relative to the child") |
| `path_relative_to_include(name?)` | `"."` | 0 includes: `"."`. 1 include: `rel(dir(inc), unit)` (arg ignored). ≥2: exactly 1 arg naming a label, else unknown | `rel(dir(X), unit)` |
| `path_relative_from_include(name?)` | `"."` | 0: `"."`. 1: `rel(unit, dir(inc))`. ≥2: select by label | `rel(unit, dir(X))` |
| `get_parent_terragrunt_dir(name?)` | V(unit) | V(clean(unit + from)) | V(dir(X)) |

Definitions:
- V(p) is a **virtual absolute path**: `vroot + "/" + p`, with `const vroot = "/__gruntled_repo_root__"`. Terragrunt returns absolute paths, and configs concatenate them (`"${get_terragrunt_dir()}/../vpc"`). A sentinel root keeps values checkout-independent, so no absolute path can leak.
- When resolving any evaluated path:
  - a prefix of `vroot + "/"` (or exactly `vroot`) gives a repo path;
  - any other absolute path means `unknown` ("absolute path outside repo");
  - a relative path is `path.Join(unitDir, p)`, and a result of `..` or `../…` means `unknown` ("escapes repository").
- **Do not** use `path.Clean` on a `/`-rooted string to detect escape, because `path.Clean("/a/../../x") == "/x"` silently clamps.

`find_in_parent_folders` details:
- With no argument, probe `terragrunt.hcl.json`, then `terragrunt.hcl`, in each ancestor.
- With a name, probe `path.Join(dir, name)`. The name may contain `/`, and it may name a directory, because Terragrunt uses `vfs.Exists`.
- Walk up to and including the repo root `"."`.
- If nothing is found inside the repo, return the fallback if one was given. Otherwise the unit is `unknown` (Terragrunt would keep searching above the repo, which gruntled must not do). Return V(found).

```go
// Source: logic mirrors terragrunt pkg/config/config_helpers.go findInParentFoldersImpl
func (s *scope) findInParentFolders(args []cty.Value) (cty.Value, error) {
	if len(args) > 2 {
		return cty.NilVal, errUnsupported
	}
	name, fallback, hasFallback := "", "", len(args) == 2
	if len(args) >= 1 {
		name = args[0].AsString()
	}
	if hasFallback {
		fallback = args[1].AsString()
	}
	dir := s.unitDir
	for dir != "." {
		dir = path.Dir(dir) // path.Dir("a") == "."
		candidates := []string{path.Join(dir, name)}
		if name == "" || name == "terragrunt.hcl" {
			candidates = []string{path.Join(dir, "terragrunt.hcl.json"), path.Join(dir, "terragrunt.hcl")}
		}
		for _, c := range candidates {
			if _, err := fs.Stat(s.fsys, c); err == nil {
				return cty.StringVal(virtual(c)), nil
			}
		}
	}
	if hasFallback {
		return cty.StringVal(fallback), nil
	}
	return cty.NilVal, errNotFoundInRepo // => unit unknown
}
```
Register each function with `function.New(&function.Spec{VarParam: &function.Parameter{Type: cty.String}, Type: function.StaticReturnType(cty.String), Impl: …})`. Functions that take no parameter get `Params: nil` and no `VarParam`, so a call with arguments fails and the unit goes `unknown`.

### Pattern 5: Reference extraction (AST only)
**What:** Use `hclsyntax.Walk` with an Enter/Exit walker over the **whole body** of each merged file. The walker collects every `*hclsyntax.ScopeTraversalExpr` whose traversal matches this shape:

```
dependency . <TraverseAttr name> . outputs . (<TraverseAttr out> | <TraverseIndex "literal">) [ ... anything ... ]
```

It keeps a depth counter for enclosing `try(...)` and `can(...)` calls and emits nothing while inside one. Collected references carry the file they appear in, so a reference in `root.hcl` belongs to each child that merges `root.hcl`, with a Position in `root.hcl`.

```go
// Source: shapes verified against hcl v2.25.0 in a scratch program
type refWalker struct{ guard int; out []rawRef }

func (w *refWalker) Enter(n hclsyntax.Node) hcl.Diagnostics {
	switch e := n.(type) {
	case *hclsyntax.FunctionCallExpr:
		if e.Name == "try" || e.Name == "can" {
			w.guard++
		}
	case *hclsyntax.ScopeTraversalExpr:
		if w.guard == 0 {
			if dep, out, ok := outputRef(e.Traversal); ok {
				w.out = append(w.out, rawRef{dep: dep, out: out, start: e.SrcRange.Start})
			}
		}
	}
	return nil
}
func (w *refWalker) Exit(n hclsyntax.Node) hcl.Diagnostics {
	if e, ok := n.(*hclsyntax.FunctionCallExpr); ok && (e.Name == "try" || e.Name == "can") {
		w.guard--
	}
	return nil
}

func outputRef(t hcl.Traversal) (dep, out string, ok bool) {
	if len(t) < 4 || t.RootName() != "dependency" {
		return "", "", false
	}
	name, ok1 := t[1].(hcl.TraverseAttr) // dependency.a["k"] (expansion) => not TraverseAttr => skip
	outs, ok2 := t[2].(hcl.TraverseAttr)
	if !ok1 || !ok2 || outs.Name != "outputs" {
		return "", "", false
	}
	switch s := t[3].(type) {
	case hcl.TraverseAttr:
		return name.Name, s.Name, true
	case hcl.TraverseIndex:
		if s.Key.IsKnown() && !s.Key.IsNull() && s.Key.Type() == cty.String {
			return name.Name, s.Key.AsString(), true
		}
	}
	return "", "", false
}
// then: slices.SortFunc(w.out, byByteOffset) — Attributes are walked in map order.
```
Verified shapes:

| Source | AST | Result |
|---|---|---|
| `dependency.vpc.outputs.id` | 4-step traversal | ref(vpc, id) |
| `dependency.vpc.outputs.nested.deep` | 5 steps | ref(vpc, nested) |
| `dependency.vpc.outputs["cidr"]` | `TraverseIndex` at step 3 | ref(vpc, cidr) |
| `"${dependency.vpc.outputs.tmpl}-x"`, and inside `for` | normal traversal | ref |
| `try(dependency.vpc.outputs.maybe, null)` | inside `try` | **skip** (would be a false positive: `try` tolerates missing outputs) |
| `dependency.vpc.outputs` (whole), `lookup(dependency.vpc.outputs, "k", …)` | 3 steps | skip |
| `dependency.vpc.outputs[local.key]` | `IndexExpr` around a 3-step traversal | skip |
| `dependency.vpc.outputs.*.id` | `SplatExpr` around a 3-step traversal | skip |
| `dependency.aurora["web"].outputs.id` | `TraverseIndex` at step 2 | skip (expansion) |

### Pattern 6: Include resolution and merge (PARSE-03)
The rules come from `include.go` (`handleInclude`, `handleIncludeForDependency`, `Merge`, `mergeDependencyBlocks`, `getTrackInclude`) and the `blocks.mdx` docs.

- A unit can have several `include` blocks. Labels must be unique, and at most one may be bare (label `""`). A violation means `unknown`.
- `include.path` is evaluated in scope S0. A relative result is joined to the **child** dir (`getTrackInclude`). It must resolve inside the repo to an existing file, or the unit is `unknown`.
- **An included file that itself has an `include` block** is a Terragrunt error (`TooManyLevelsOfInheritanceError`, and the docs say "only supports a single level of include"). The unit becomes `unknown` with reason `nested-include`, and no diagnostic is emitted.
- `merge_strategy` must be a literal. `""` and `"shallow"` mean shallow, and `"deep"` and `"no_merge"` mean what they say. Any other value (including `deep_map_only`, which Terragrunt rejects for includes) means `unknown`.
- **Precedence: child > last include > … > first include.** Terragrunt merges with `slices.Backward(includeList)`, and the source wins. `no_merge` includes contribute **nothing**: no source, no dependencies, no generate blocks, no references.
  - **`terraform.source`**: take the highest-precedence file whose `terraform` block has a `source` attribute. A `terraform` block without `source` does not override. Evaluate it in the scope (S1 or S2) of the file it is written in.
  - **`dependency` blocks** are merged by label. Under shallow merge the highest-precedence block wins whole. Under deep merge, `config_path` comes from the highest-precedence block that sets it. `config_path` is evaluated in its file's scope and then joined relative to the **child** dir (`getCleanedTargetConfigPath(cfgPath, pctx.TerragruntConfigPath)`). A duplicate label *within one file* (Terragrunt warns and "last wins", per the `duplicate-dependency-labels` strict control) means `unknown`, so it is never guessed.
  - A dependency with an `expansion` block (Terragrunt v1.2.0) makes the unit `unknown`.
  - A `config_path` that is dynamic (locals, `each.key`) or outside the repo makes the unit `unknown`.
  - Target unit dir: if the resolved path is a directory, use it. Otherwise use `path.Dir` (for example `../vpc/terragrunt.hcl`).
  - A target that is not a discovered unit is **not** an error. The domain already allows it, and the analyzer stays silent (GRT002 is a later check). This also covers `config_path` pointing at a `terragrunt.stack.hcl` directory.
  - **`generate` blocks** are merged by label with the same precedence.
- A unit directory containing `terragrunt.autoinclude.hcl` means `unknown`. Terragrunt auto-merges it (`mergeAutoIncludeIfPresent`, a stacks feature).

### Pattern 7: Unknown-reason catalogue (PARSE-04)
Use stable kebab-case constants in `internal/infrastructure/terragrunt` so reasons are testable and greppable. When several reasons apply, report the first one hit in a fixed check order. Every unknown unit is silent: it gets no GRT001 and has no dependencies or references, which is how `NewUnknownUnit` already behaves.

| Reason | Trigger |
|---|---|
| `syntax-error` | unit file or a merged include file has HCL errors (GRT100 is emitted **for the file**, once) |
| `json-config-unsupported` | `terragrunt.hcl.json` |
| `autoinclude-unsupported` | `terragrunt.autoinclude.hcl` in unit dir |
| `invalid-include` | duplicate labels, more than one bare include, or a non-literal or unknown `merge_strategy` |
| `include-not-found` / `nested-include` | include path unresolvable or missing in repo / included file has `include` |
| `dynamic-path` | any of the three path attributes fails closed evaluation |
| `path-outside-repo` | absolute non-virtual path, or a result that escapes the root |
| `remote-source` | classifier says remote (GRAPH-03) |
| `invalid-dependency` | duplicate label in one file, `expansion` block, or missing `config_path` |
| `generate-may-declare-outputs` | effective `generate` with non-template `contents` (for example `file()`, `templatefile()` or `local.x`), or a template whose literal parts match `(?m)^\s*output\b` or `"output"\s*:` |
| `unit-dir-overlays-module` | `source` points outside the unit dir **and** the unit dir holds `.tf`/`.tf.json`/`.tofu`/`.tofu.json` files (Terragrunt `CopyFolderContents(UnitDir → WorkingDir)` overlays them) |
| `module:<reason>` | the module's surface is unknown (see Pattern 11) |

Scope rule: a construct that sits outside the three path-bearing attributes never makes a unit `unknown`. This covers `run_cmd`/`get_env` in `locals`/`inputs`, `remote_state`, and hooks. gruntled never evaluates those attributes, so there is nothing to resolve.

`generate` blocks are the one exception. They can inject declarations the static walk cannot see (research/PITFALLS.md §4). The literal `provider`/`variable` heredocs in `cds-snc/secret` do not trigger the rule, and the primary corpus has no `generate` at all.

### Pattern 8: HCL diagnostics become GRT100 with byte columns (PARSE-05)
```go
// Source: hcl.Pos docs — "Column ... in unicode characters ... counting grapheme clusters";
// "Byte is the byte offset" (verified: `"ééé"` prefix => Column 21, byte column 23)
func toPosition(file repograph.RepoPath, src []byte, p hcl.Pos) (repograph.Position, error) {
	off := min(max(p.Byte, 0), len(src))
	col := off - (bytes.LastIndexByte(src[:off], '\n') + 1) + 1 // 1-based byte column
	return repograph.NewPosition(file, max(p.Line, 1), col)
}

func syntaxDiagnostic(file repograph.RepoPath, src []byte, d *hcl.Diagnostic) (diagnostic.Diagnostic, error) {
	pos, _ := repograph.NewPosition(file, 1, 1) // Subject can be nil
	if d.Subject != nil {
		pos, _ = toPosition(file, src, d.Subject.Start)
	}
	return diagnostic.New(diagnostic.CodeSyntaxError, diagnostic.SeverityError, pos, d.Summary+": "+d.Detail)
}
```
Verified: a mid-edit file (unclosed block, trailing `outputs.`) and non-UTF-8 bytes all return a **non-nil** partial `*hcl.File` plus error diagnostics. There is no panic.

**Do not analyze the partial body.** It could omit `output` blocks, which would under-count the surface and cause a false positive. A file with errors makes every unit that touches it `unknown`.

Recommendation: emit only the **first** error diagnostic per file (sorted by position), because hcl cascades errors. It is deterministic and less noisy. The Phase 3 presenter may revisit this.

Add a native fuzz target (`FuzzLoadUnit`) over the adapter with a seed corpus of the broken fixtures. `denis256/terragrunt-tests` has 8 intentionally broken files: `broken-dependencies/`, `include-error/` and `encryption/`. The seeds run in plain `go test`.

### Pattern 9: Source classification (GRAPH-01/02/03)
Mirror Terragrunt's detector chain (`internal/getter/detect.go` `defaultDetectors`: GitHub, Git, BitBucket, GitLab, GCS, S3, File). Apply the checks in order:
1. Empty after trim: **invalid**, which means `unknown`.
2. Forced getter `^[A-Za-z0-9]+::` (`git::`, `s3::`, `gcs::`, `hg::`, `http::`, `cas::`): **remote**.
3. A URL scheme `^[A-Za-z][A-Za-z0-9+.-]*://` (`git://`, `ssh://`, `http(s)://`, `s3://`, `gcs://`, `tfr://`, `oci://`) is **remote**. `file://` gives **unknown** (real absolute path).
4. Scheme-less host shorthands: prefix `github.com/`, `gitlab.com/`, `bitbucket.org/`, SCP-like `^[^/@:]+@[^/:]+:` (`git@github.com:org/repo`), or containing `amazonaws.com/` or `googleapis.com/`: **remote**.
5. Contains `?`: **unknown**. A query string on a local path is not worth modelling.
6. Otherwise **local**, because Terragrunt's `FileDetector` catches everything else, including bare `modules/vpc` and registry-looking `hashicorp/consul/aws`. Split at the first `//` into root and subdir. The module dir is `resolve(unitDir, root)` joined with the subdir, then cleaned. `"."`, `".//"` and `"./"` give the unit dir itself.

The **safety net** is `SurfaceReader`. A local module dir that does not exist inside the repo, or holds no Terraform files, gives `unknown`. A misclassification in either direction therefore ends in `unknown`, never in a diagnostic. With `source` absent (GRAPH-02), the module is the unit dir.

Corpus evidence: `cds-snc/secret` uses `source = "../../aws//lambda"`, which gives module `aws/lambda` (unit dir ≠ module dir, the shape in research/PITFALLS.md §5). The primary corpus has no `source`.

### Pattern 10: Application assembly (GRAPH-04), pure and fake-tested
```go
// internal/application/ports — domain types only
type UnitLoader interface {
	LoadUnits(ctx context.Context) (LoadResult, error)
}
type LoadResult struct {
	Units       []UnitConfig            // sorted by Path
	Diagnostics []diagnostic.Diagnostic // GRT100 for unit/include files
}
type UnitConfig struct {
	Path          repograph.RepoPath
	UnknownReason string             // non-empty => unknown; remaining fields ignored
	Module        repograph.RepoPath // resolved local module dir (== Path when no source)
	Dependencies  []DependencyConfig
	References    []repograph.Reference
}
type DependencyConfig struct {
	Name   string
	Target repograph.RepoPath // target *unit* dir (hop 1)
	Pos    repograph.Position
	// Literal facts captured for Phase 3 DIAG-03 (see Open Question 1):
	Enabled, SkipOutputs Tristate
	MockOutputKeys       []string // nil = absent; sorted; only when mock_outputs is an object literal
	MockKeysKnown        bool
	MockMergeWithState   Tristate // mock_outputs_merge_with_state / _strategy_with_state != no_merge
	MockAllowedCommands  []string // literal list or nil
}
type SurfaceReader interface {
	ReadSurface(ctx context.Context, module repograph.RepoPath) (SurfaceResult, error)
}
type SurfaceResult struct {
	Surface       repograph.Surface
	UnknownReason string
	Diagnostics   []diagnostic.Diagnostic // GRT100 for broken .tf files
}
```
`indexing.Build` runs these steps:
1. `LoadUnits`.
2. Collect the **distinct** module paths of non-unknown units, sort them, and call `ReadSurface` once each.
3. For each unit:
   - a unit that is unknown, or whose module is unknown, becomes `NewUnknownUnit(path, reason)`;
   - otherwise `NewDependency(name, target, pos)` for each dependency, then `NewResolvedUnit(path, module, deps, refs)`.
4. Pass the modules of resolved units only to `NewRepositoryGraph`, which requires every resolved unit's module to be present.
5. Diagnostics go through `diagnostic.NewSet` for canonical order and dedup.

Hop 2 (target unit → module → surface) is answered by the existing graph queries `DependencyTarget` and `ModuleOf`. Nothing new is needed in the domain.

### Pattern 11: Surface reader (GRAPH-05)
For a module dir:
1. Call `fs.ReadDir`. An error gives `module-dir-not-found`.
2. Keep names ending in `.tf`, `.tf.json`, `.tofu` or `.tofu.json`. Drop Terraform's ignored names: prefix `.`, suffix `~`, or `#…#`, per `isIgnoredFile`.
3. Apply OpenTofu precedence: "If both `foo.tf` and `foo.tofu` exist … OpenTofu will only load `foo.tofu`" (opentofu.org/docs/language/files). The same applies to `.tf.json` and `.tofu.json`.
4. Handle each entry:
   - A regular file is read.
   - A **symlink is followed with `fs.ReadFile`**, which `os.Root` confines to the repo. An escape or dangling link errors, which gives `module-file-unreadable`.
   - An entry that `fs.Stat` reports as a directory is skipped.
5. Zero files gives `no-terraform-files`. This also covers a root `terragrunt.hcl` "unit" such as `cds-snc/secret/terragrunt/terragrunt.hcl`, which is really an include target.
6. Parse with `hclsyntax.ParseConfig` or `hcljson.Parse`. Any error gives `syntax-error` plus GRT100.
7. Call `PartialContent` with schema `{variable[name], output[name]}`. A `PartialContent` error diagnostic (for example `output {}` with no label) gives `unknown`.
8. Union the names through a set, because override files legitimately re-declare names. Then call `repograph.NewSurface`, which **rejects duplicates**.

Verified: `output name { }` (unquoted label), `.tf.json` object form, and mixed files all yield correct labels.

### Pattern 12: Architecture enforcement additions
Extend `scripts/check-architecture.sh` and add one self-test case per new rule in `scripts/test-check-architecture.sh`. This follows the project's "few jobs, each can genuinely fail" rule.
- **application-direct-io**: the direct imports of `./internal/application/...` (including tests) must not contain `os`, `io/fs`, `io/ioutil`, `path/filepath`, `net`, `net/*` or `syscall`. Use the same regex as the domain rule.
- **application-external-deps**: `go list -deps -test ./internal/application/...` non-standard deps must match `^<module>/internal/(domain|application)/`. This is an allowlist, like the domain rule, so it also catches `internal/infrastructure` and `hashicorp/hcl`.
- **hcl-only-in-infrastructure**: for every package outside `internal/infrastructure/`, **direct** `Imports`/`TestImports`/`XTestImports` must not match `^github.com/hashicorp/|^github.com/zclconf/`. Check direct imports, not `-deps`, because `cmd/gruntled` legitimately links infrastructure transitively.
- Add a non-vacuous guard: at least one application package must exist.
- The self-test `mkcopy` already copies `go.sum`, which it needs now that hcl is a dependency. The module cache makes the copies build offline.

### Anti-Patterns to Avoid
- **Reading `.tf` straight from `config_path`'s directory.** Always go target unit → its module (research/PITFALLS.md §5). `cds-snc/secret` breaks this assumption.
- **Treating `hcl.Pos.Column` as the byte column.** It breaks every non-ASCII line, and it mismatches the synthrepo manifest oracle.
- **Ranging over `Body.Attributes`, or relying on `Walk`/`VisitAll` order.** Map order is random, so always sort by `Pos.Byte`, then file.
- **Evaluating `inputs`/`locals`/`generate` with the EvalContext.** Only the three path attributes are evaluated. `run_cmd`/`get_env` must never exist as functions.
- **`path.Clean` on virtual-absolute paths to detect escape.** It clamps at `/` silently.
- **Using the partial body of a file with syntax errors.** It under-counts the surface and causes false positives.
- **Rejecting symlinked `.tf` files, or marking their modules unknown.** It kills the whole primary corpus (Pitfall 1).
- **Converting paths with `filepath`.** Inside `fs.FS` everything is slash-separated and repo-relative. Use `path`, and let `repograph.NewRepoPath` validate.

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| HCL tokenizing/parsing, block labels, heredocs, JSON syntax | regex or line scanning | `hclsyntax.ParseConfig`, `hcljson.Parse`, `Body.PartialContent` | Unquoted labels (`output name {}`) exist in the corpus (research/PITFALLS.md §1). Regex misses comments, heredocs and multi-line blocks |
| Expression evaluation for path attributes | a custom interpreter | `hcl.Expression.Value` with a closed `hcl.EvalContext` | Templates, escapes, conditionals and error diagnostics come for free. The closed context is the safety boundary |
| Symlink containment / "stay inside the repo" | manual `Lstat` + `EvalSymlinks` + prefix checks | `os.OpenRoot(repo).FS()` | Go 1.24+ refuses escapes ("path escapes from parent", verified), including absolute-target links. It is TOCTOU-safe |
| Lexically ordered walk | manual `ReadDir` recursion plus sort | `fs.WalkDir` | Documented lexical order. It does not follow symlinks |
| In-memory FS fixtures, including symlinks | temp dirs for every unit test | `testing/fstest.MapFS` (`Mode: fs.ModeSymlink`, `Data` = link target) | Verified in Go 1.27: `ReadFile` follows the link, and `WalkDir` reports it as a symlink |
| Canonical diagnostic ordering and dedup | ad-hoc sort | `diagnostic.NewSet` (Phase 1) | Already implements `Compare` and dedup by `Key` |

Hand-rolled on purpose, with justification:
- The source classifier. Terragrunt's chain is `internal/`, and go-getter adds deps. The existence check makes errors fail safe.
- The surface reader. `terraform-config-inspect` ignores `.tofu`, returns partial results on errors and carries legacy deps.

**Key insight:** whatever is hand-rolled here sits behind a "fail to `unknown`" net. Every hand-written decision point must lean toward `unknown`, and each must have a table test showing its uncertain input takes that path.

## Common Pitfalls

### Pitfall 1: The primary corpus depends on symlinked `.tf` files
**What goes wrong:** The team implements PARSE-06 as "never follow any symlink". In the corpus, `iac.src/*/global.tf -> ../global.tf` appears in 66 module dirs, plus 2 more, for 68 symlinks. Either the variables vanish (harmless for GRT001 today), or, if the "safe" choice is to mark those modules unknown, **every one of the 65 units becomes unknown** and Phase 4 VALID-04 can never pass.
**Why it happens:** "Symlinks" is ambiguous. The requirement is about the **walk**.
**How to avoid:** Never descend into symlinked directories during unit discovery. Do follow file symlinks when *reading* a module dir or an include file through `os.Root`. Mark the module `unknown` only when a link escapes or dangles.
**Warning signs:** Corpus smoke test shows `unknown` units with reason `module-*`.
**Evidence:** Prototype over the corpus with `os.Root`: 65 units, 22 refs OK, 0 missing, 0 unknown. After renaming `output "role_name"` in `iac.src/s3_runtime/state.tf`: 8 missing, as expected.

### Pitfall 2: Byte columns vs grapheme columns
**What goes wrong:** GRT001 and GRT100 positions drift on any line containing non-ASCII characters before the reference, and they disagree with the `synthrepo` manifest, which uses byte columns.
**How to avoid:** Pattern 8 `toPosition`, used for **every** hcl position (Dependency `DefRange`, reference `SrcRange.Start`, diagnostic `Subject`). The unit-test input is `a = "ééé" == "" ? dependency.x.outputs.y : ""`, which must give column 23, not 21.

### Pitfall 3: "Nested includes" misread as recursive include chains
**What goes wrong:** Implementing recursive include resolution produces behaviour Terragrunt itself rejects (`TooManyLevelsOfInheritanceError`, docs: "Terragrunt only supports a single level of `include` blocks").
**How to avoid:** Support **several** includes per unit, at any directory depth, sharing parsed files across units. An include inside an included file makes the unit `unknown` (`nested-include`). Test both.

### Pitfall 4: Included files evaluated in the wrong directory
**What goes wrong:** `dependency "vpc" { config_path = "../vpc" }` written in `_envcommon/app.hcl` is resolved relative to `_envcommon/` instead of the child unit, giving wrong targets and potential false positives.
**How to avoid:** Every relative path and `get_terragrunt_dir()`/`find_in_parent_folders()` resolves against the **child** unit dir. Only `path_relative_*` and `get_parent_terragrunt_dir` depend on which include file the expression lives in (Pattern 4 table). The source for this is Terragrunt `ParseConfigFile` for includes, which keeps `pctx.TerragruntConfigPath` as the child. The docs also say `find_in_parent_folders` "searches relative to the child".

### Pitfall 5: Non-deterministic collection order
**What goes wrong:** Reference, dependency or diagnostic order changes run to run, because `hclsyntax` ranges over `Body.Attributes`, which is a map (verified in `structure.go`).
**How to avoid:** Sort every extracted slice by (file, byte offset), then by name. Domain constructors sort again, but DTOs, diagnostics and the per-module surface map must be sorted before use. Add a test that builds the same repo twice, and from two differently-named temp dirs, and compares a canonical dump.

### Pitfall 6: `try()`, `can()` and whole-object references counted as references
**What goes wrong:** `try(dependency.x.outputs.maybe, null)` is valid Terraform/Terragrunt even when the output is absent. Reporting it is a false positive.
**How to avoid:** Use the Pattern 5 guard counter. Only emit on the exact traversal shapes listed there.

### Pitfall 7: Unit directory overlay and `generate`
**What goes wrong:** Terragrunt copies the unit dir's files over the module copy (`internal/runner/run/download_source.go` → `util.CopyFolderContents(opts.UnitDir, terraformSource.WorkingDir, …)`), and `generate` writes files into the same place. The effective surface can then differ from the module dir's surface, both larger and smaller, because a same-named file replaces the module's file.
**How to avoid:** Mark the unit `unknown` (reasons `unit-dir-overlays-module` and `generate-may-declare-outputs`). This is rare in practice: none in the primary corpus, 1 case in `denis256/terragrunt-tests` (`issue-2977/module_b`), and `cds-snc/secret` generates only provider and variable heredocs.

### Pitfall 8: Classifying bare sources like Terraform does
**What goes wrong:** Research/ARCHITECTURE.md §A.5 rule 5 says a non-`./` path is remote. That is Terraform's rule. Terragrunt's `FileDetector` catches every unmatched string as local, and real repos use `source = "units/chicken"` (seen in `denis256/terragrunt-tests`).
**How to avoid:** Use the Pattern 9 order. Registry-looking strings (`hashicorp/consul/aws`) become local-then-missing, which gives `unknown`: the correct outcome through the safety net.

### Pitfall 9: Surface duplicates crash `NewSurface`
**What goes wrong:** Override files (`override.tf`, `*_override.tf`) and `.tf`/`.tofu` pairs re-declare the same names. `repograph.NewSurface` returns an error on duplicates.
**How to avoid:** Put the names in a set first, and apply the `.tofu` precedence before parsing.

### Pitfall 10: The root `terragrunt.hcl` as a unit
**What goes wrong:** Legacy layouts such as `cds-snc/secret/terragrunt/terragrunt.hcl` include a root `terragrunt.hcl` via bare `find_in_parent_folders()`. The walk finds it as a "unit".
**How to avoid:** Its module (its own dir) has no Terraform files, so it becomes `unknown` (`module:no-terraform-files`) and stays silent. There is no special case, but add it as a fixture.

## Code Examples

### Relative path between two repo dirs (for `path_relative_*`)
```go
// Mirrors filepath.Rel for cleaned, slash-separated, repo-relative dirs ("." = root).
func relDir(from, to string) string {
	split := func(p string) []string {
		if p == "." {
			return nil
		}
		return strings.Split(p, "/")
	}
	f, t := split(from), split(to)
	i := 0
	for i < len(f) && i < len(t) && f[i] == t[i] {
		i++
	}
	parts := make([]string, 0, len(f)-i+len(t)-i)
	for range f[i:] {
		parts = append(parts, "..")
	}
	parts = append(parts, t[i:]...)
	if len(parts) == 0 {
		return "."
	}
	return strings.Join(parts, "/")
}
// Terragrunt docs example: from "terragrunt" (root.hcl dir) to "terragrunt/secrets/mysql":
// path_relative_to_include() = "secrets/mysql"; path_relative_from_include() = "../.."
// => source "${path_relative_from_include()}/../sources//${path_relative_to_include()}"
//    = "../../../sources//secrets/mysql" resolved from the unit dir => module "sources/secrets/mysql".
```

### Resolving an evaluated path against the unit dir (escape-safe)
```go
const vroot = "/__gruntled_repo_root__"

func resolve(unitDir, p string) (string, bool) {
	var rel string
	switch {
	case p == vroot:
		rel = "."
	case strings.HasPrefix(p, vroot+"/"):
		rel = path.Clean(strings.TrimPrefix(p, vroot+"/")) // relative clean: ".." stays visible
	case path.IsAbs(p):
		return "", false // real absolute path: outside the analysed tree => unknown
	default:
		rel = path.Join(unitDir, p) // path.Join cleans; ".." survives at the front if it escapes
	}
	if rel == ".." || strings.HasPrefix(rel, "../") {
		return "", false
	}
	return rel, true
}
```

### Converting a synthrepo tree into a MapFS (test helper)
```go
func mapFS(t synthrepo.Tree) fstest.MapFS {
	m := fstest.MapFS{}
	for _, f := range t {
		m[f.Path] = &fstest.MapFile{Data: f.Content}
	}
	return m
}
```

## State of the Art

| Old Approach | Current Approach | When Changed | Impact |
|--------------|------------------|--------------|--------|
| Root `terragrunt.hcl` found by bare `find_in_parent_folders()` | `root.hcl` + `find_in_parent_folders("root.hcl")`. No-argument use is deprecated behind the `root-terragrunt-hcl` strict control | Terragrunt 0.7x–1.x | Still support no-arg (it probes `terragrunt.hcl.json` then `terragrunt.hcl`). `cds-snc/secret` uses it |
| Dependency blocks with a fixed name | `expansion { for_each / count }` on `dependency`, referenced as `dependency.a["k"].outputs.x` | Terragrunt v1.2.0 (graduated from experiment) | Unit becomes `unknown`. The traversal shape is skipped |
| Duplicate `dependency` labels silently shadowed | `duplicate-dependency-labels` strict control (warns by default) | Terragrunt v1.1.4 | Unit becomes `unknown` |
| Unit config only from `terragrunt.hcl` + includes | `terragrunt.autoinclude.hcl` auto-merged | Terragrunt 1.x stacks | Unit becomes `unknown` if the file is present |
| Terraform-only `.tf` | OpenTofu `.tofu`/`.tofu.json` take precedence over same-named `.tf` | OpenTofu 1.8 | The surface reader must read them |
| `terraform-config-inspect` for surface | direct `hcl/v2` | This research | Covers `.tofu`, strict on errors, fewer deps |

**Deprecated/outdated for this phase:**
- The STATE.md blocker "spike: is Terragrunt's source classification reusable, or `go-getter.Detect`?" is **resolved**. The classification lives in `internal/tf/source.go` and `internal/getter/detect.go`, both not importable. Hand-roll it per Pattern 9, with the existence safety net. research/ARCHITECTURE.md §A.5 (go-getter reuse, Terraform local-path rule) and §A.4 (use tfconfig) are superseded for Phase 2.

## Open Questions

1. **Where do dependency mock and flag facts live? (DIAG-03 is Phase 3)**
   - What we know: the primary corpus depends on them. It has 22 references, all behind `mock_outputs` with `mock_outputs_merge_with_state = true` and `apply` among the allowed commands, and many ordering-only deps with `skip_outputs = true`. The parser is the only code that reads HCL.
   - What's unclear: whether Phase 2 should extend the domain `Dependency`.
   - Recommendation: capture them as literal facts in the ports DTO in Phase 2 (`DependencyConfig`, Pattern 10), with a tri-state for non-literal values. Do **not** change the domain in Phase 2. Phase 3 promotes exactly what its rule needs into the domain once DIAG-03 is decided. That avoids speculative domain API while never re-opening the parser.

2. **Functions beyond the six (`dirname`, `get_repo_root`)**
   - What we know: in `denis256/terragrunt-tests` path attributes, `get_repo_root()` appears 64 times (mostly stacks tests) and `dirname()` 9 times. The Gruntwork `_envcommon` idiom is `"${dirname(find_in_parent_folders("root.hcl"))}/_envcommon/x.hcl"`. The primary and secondary corpora use neither.
   - Recommendation: stay with exactly the six (locked scope). These units become `unknown` (`dynamic-path`), which is a false negative only. Revisit after Phase 4 if coverage matters. `dirname` is pure and trivial to add later. `get_repo_root` needs a `.git` lookup.

3. **What counts as a "vendored module directory"?**
   - Recommendation: skip `vendor/` during unit discovery (plus `.terraform`, which holds `.terraform/modules`). This is safe because module dirs are still read when a unit's `source` points into them, so skipping can only drop units, never create diagnostics. Confirm with the user if they meant something else.

4. **`.terragrunt-stack`: skip or walk?**
   - research/PITFALLS.md §6 says skip, while PROJECT.md "Out of Scope" relies on the walk finding generated units. Recommendation: walk it (PROJECT.md is authoritative, and Terragrunt discovery includes it). Units there usually have remote sources, so they become `unknown`.

5. **One GRT100 per file, or every error diagnostic?**
   - Recommendation: first error per file in Phase 2 (deterministic, low noise). Phase 3 (DIAG-02) owns the final presentation.

6. **Deep-merge `dependency` details**
   - Recommendation: implement `config_path` precedence exactly (Pattern 6) and capture mock keys from the highest-precedence block only. If deep-merge *mock* semantics matter to DIAG-03, mark units with deep-merged mocks `mock-keys-unknown` in the DTO rather than modelling the merge.

## Validation Architecture

### Test Framework
| Property | Value |
|----------|-------|
| Framework | Go `testing` (go1.27.0 toolchain via `go 1.27` in go.mod), table-driven tests, `testing/fstest.MapFS`, native fuzzing |
| Config file | none (go.mod only) |
| Quick run command | `go test -count=1 ./internal/application/... ./internal/infrastructure/...` |
| Full suite command | `go vet ./... && go test -count=1 ./... && bash scripts/check-architecture.sh && bash scripts/test-check-architecture.sh` |

### Phase Requirements → Test Map
| Req ID | Behavior | Test Type | Automated Command | File Exists? |
|--------|----------|-----------|-------------------|-------------|
| PARSE-01 | Only include, terraform.source and dependency are read. `inputs`/`locals` containing `run_cmd`/`get_env` do not make a unit unknown. References extracted per Pattern 5 table | unit | `go test ./internal/infrastructure/terragrunt -run 'TestRefs\|TestStructuralOnly' -count=1` | ❌ Wave 0 |
| PARSE-02 | Six functions × three scopes (Pattern 4 table), including the docs examples (`sources//secrets/mysql`), fallback, no-arg probe order, not-found-in-repo becomes unknown | unit (table) | `go test ./internal/infrastructure/terragrunt -run TestPathFuncs -count=1` | ❌ Wave 0 |
| PARSE-03 | Multi-include precedence, `no_merge`/`deep`, nested include becomes unknown. **Parse-once:** counting FS over synthrepo 50 units has `root.hcl` read exactly 1× | unit + integration | `go test ./internal/infrastructure/terragrunt -run 'TestInclude\|TestParseOnce' -count=1` | ❌ Wave 0 |
| PARSE-04 | Each unknown reason in Pattern 7 has a fixture, and the unit is `StatusUnknown` with no deps or refs | unit | `go test ./internal/infrastructure/terragrunt -run TestUnknownReasons -count=1` | ❌ Wave 0 |
| PARSE-05 | Mid-edit, non-UTF-8 and broken include/module `.tf` give one GRT100 with a byte column, units unknown, no panic. Fuzz seeds run | unit + fuzz seed | `go test ./internal/infrastructure/... -run 'TestSyntax\|FuzzLoad' -count=1` | ❌ Wave 0 |
| PARSE-06 | Decoy `terragrunt.hcl` in `.terragrunt-cache/`, `.terraform/`, `vendor/` and a symlinked dir is not discovered. A symlinked `terragrunt.hcl` is not discovered | unit (MapFS with symlinks) | `go test ./internal/infrastructure/terragrunt -run TestWalk -count=1` | ❌ Wave 0 |
| GRAPH-01 | `source = "../../aws//lambda"` gives module `aws/lambda`. The virtual-root source via `get_parent_terragrunt_dir` works | unit | `go test ./internal/infrastructure/... -run 'TestSource\|TestClassify' -count=1` | ❌ Wave 0 |
| GRAPH-02 | No source, empty `terragrunt.hcl`: module = unit dir | unit | same as above | ❌ Wave 0 |
| GRAPH-03 | Classifier table (git::, github.com/, git@…:, tfr://, oci://, s3::, https://, bare missing, bare existing, `?ref`, file://) gives remote or unknown, with zero FS/network for remote | unit (pure table) | `go test ./internal/infrastructure/sourceresolve -count=1` | ❌ Wave 0 |
| GRAPH-04 | Two-hop with unit dir ≠ module dir. Dependency declared in an include resolves against the child. Target not a unit is kept, and the analyzer path stays silent | unit (application fakes) + integration | `go test ./internal/application/indexing ./internal/infrastructure/terragrunt -run TestTwoHop -count=1` | ❌ Wave 0 |
| GRAPH-05 | Unquoted labels, `.tf.json`, `.tofu` precedence, override dedup, ignored files, **symlinked `.tf` followed**, escaping link gives unknown (real FS via `os.Root` in `t.TempDir()`), no files gives unknown | unit + real-FS | `go test ./internal/infrastructure/tfsurface -count=1` | ❌ Wave 0 |
| (determinism) | Same repo built twice, and from two differently-named temp dirs (`synthrepo.Generate`), gives an identical canonical dump. No absolute path appears | integration | `go test ./internal/application/indexing -run TestDeterministic -count=1` | ❌ Wave 0 |
| (architecture) | New rules (application IO, application deps, hcl-only-in-infrastructure) each proven to fail by self-test | script | `bash scripts/test-check-architecture.sh` | ✅ extend |
| (corpus smoke, optional) | With `GRUNTLED_CORPUS=/path/to/iso20022` set: 65 units, 0 unknown, 22 references | integration (env-gated, skipped by default) | `GRUNTLED_CORPUS=… go test ./internal/application/indexing -run TestCorpusSmoke -count=1` | ❌ Wave 0 |

### Sampling Rate
- **Per task commit:** `go test -count=1 ./internal/application/... ./internal/infrastructure/...` (under 10 s)
- **Per wave merge:** `go vet ./... && go test -count=1 ./... && bash scripts/check-architecture.sh`
- **Phase gate:** full suite plus `bash scripts/test-check-architecture.sh` green, and the corpus smoke run once by hand with `GRUNTLED_CORPUS` before `/gsd:verify-work`

### Wave 0 Gaps
- [ ] `go get github.com/hashicorp/hcl/v2@v2.25.0` (adds `go.sum`), then verify `bash scripts/test-check-architecture.sh` still passes, since its copies need `go.sum`.
- [ ] `internal/application/ports/ports.go`, the DTOs and interfaces the tests compile against.
- [ ] Shared test helpers: `mapFS(synthrepo.Tree)`, a counting FS (ReadFileFS + StatFS + ReadDirFS), and a canonical graph dump (units, modules, refs, diags as sorted strings).
- [ ] Architecture rules and self-test cases (Pattern 12), added before any infrastructure package exists, so that the rules are proven first.
- [ ] Dependency on plan 01-03 (`synthrepo`) being complete. Its `Render`/`Tree`/`Manifest` contract is used by the parse-once and determinism tests.

## Sources

### Primary (HIGH confidence)
- `github.com/gruntwork-io/terragrunt` @ `5dc737a` (2026-09-24), read directly:
  - `pkg/config/config_helpers.go`: the six functions, `getCleanedTargetConfigPath`, `getSelectedIncludeBlock`
  - `pkg/config/include.go`: `parseIncludedConfig`, `handleInclude`, `Merge`, `mergeDependencyBlocks`, `handleIncludeForDependency`, `getTrackInclude`/`TooManyLevelsOfInheritanceError`
  - `pkg/config/config.go`: `ParseConfigFile`, `ParseConfig` stage order, `DefaultTerragruntConfigPaths`, `FindConfigFilesInPath`, `mergeAutoIncludeIfPresent`
  - `pkg/config/dependency.go`: the `Dependency` struct, including `enabled`, `skip_outputs`, mock attributes and `expansion`
  - `internal/tf/source.go`: `ToSourceURL`, `SplitSourceURL`, `IsLocalSource`
  - `internal/getter/detect.go`: `defaultDetectors` with the `FileDetector` catch-all
  - `internal/runner/run/download_source.go`: unit dir copied over the module
  - `internal/util/file.go`: `SkipDirIfIgnorable`
  - docs `04-reference/01-hcl/{02-blocks,04-functions}.mdx`, changelog `v1.2.0/iterate-blocks-with-expansion.mdx`, strict control `duplicate-dependency-labels.mdx`
- `github.com/hashicorp/hcl/v2` v2.25.0: `go.mod`, the release notes (GitHub release 2026-09-15), `hclsyntax/structure.go` (map-order walk), and `go doc` for `hcl.Pos`, `hclsyntax.Walker`/`Walk`, `ParseConfig` and `TemplateExpr.IsStringLiteral`
- Go 1.27 `go doc`: `io/fs.ReadLinkFS`, `io/fs.WalkDir` (lexical, no symlink follow), `os.Root.FS` (implements ReadLinkFS), `testing/fstest.MapFS` (symlinks)
- Scratch programs run on go1.27.0 with hcl v2.25.0. They verified:
  - traversal shapes;
  - grapheme vs byte columns;
  - closed-EvalContext errors;
  - mid-edit and non-UTF-8 parsing;
  - unquoted and JSON labels;
  - MapFS and `os.Root` symlink behaviour ("path escapes from parent");
  - corpus prototype: 65 units, 22/22 refs resolved, mutation gives 8 missing.
- Go module proxy `@latest`: hcl/v2 v2.25.0, go-cty v1.19.0, go-getter/v2 v2.2.4 (its go.mod), terraform-config-inspect `v0.0.0-20260904064934-75d64de68c31` (its go.mod)
- `terraform-config-inspect` `tfconfig/load.go` (`fileExt`, `isIgnoredFile`) and `tfconfig/filesystem.go` (the legacy `FS` interface)

### Secondary (MEDIUM confidence)
- opentofu.org/docs/language/files (via WebFetch): `.tofu`/`.tofu.json` loading and precedence over same-named `.tf`
- Corpus surveys (shallow clones, 2026-09-25):
  - `guidance-for-iso20022-messaging-workflows-on-aws` @ `e6c55d1`: 65 units, no include, no source, no generate, 68 symlinked `global.tf`, 22 refs, 11 files with `mock_outputs`
  - `cds-snc/secret`: bare include, `source "../../aws//x"`, generate heredocs
  - `denis256/terragrunt-tests`: 1149 units, function usage counts, 8 intentionally broken files

### Tertiary (LOW confidence)
- None of the recommendations rest on unverified claims. The one inference is Pattern 9's host-detector patterns (`amazonaws.com/`, `googleapis.com/`), which are summarised from go-getter's S3/GCS detectors rather than read line by line. Misclassification fails safe through the existence check.

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH. Versions come from the proxy, the go.mod files were read, and the APIs were exercised in scratch code.
- Architecture: HIGH for the Terragrunt semantics (read from source). MEDIUM-HIGH for port shapes, which are a design recommendation consistent with the Phase 1 domain API.
- Pitfalls: HIGH. Each was reproduced or read in source. The corpus symlink finding was verified by prototype.

**Research date:** 2026-09-25
**Valid until:** about 2026-10-25. Terragrunt ships config features often (for example `expansion` and `autoinclude` in 1.1–1.2), so re-check `pkg/config` if the plan slips past a Terragrunt minor release.
