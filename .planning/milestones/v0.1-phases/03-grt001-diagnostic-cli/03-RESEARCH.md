# Phase 3: GRT001 Diagnostic & CLI - Research

**Researched:** 2026-09-25
**Domain:** Terragrunt runtime semantics of `dependency.X.outputs` (mock_outputs, merge-with-state, skip_outputs, enabled), HCL lazy-evaluation semantics, a pure GRT001 analyzer, and a deterministic, side-effect-free Go CLI over the Phase 2 graph
**Confidence:** HIGH. The DIAG-03 rule was derived from Terragrunt's source, read at `main@5dc737a` (2026-09-24) and diffed against release `v1.1.6` (2026-09-21). The HCL branch semantics were read in `hcl/v2` v2.24.0 (Terragrunt's pin) and v2.25.0 (ours). Every CLI and dependency claim was checked with a scratch prototype on go1.27.0. The prototype wired the real Phase 2 adapters and ran against the real primary corpus.

<user_constraints>
## User Constraints

No CONTEXT.md exists for Phase 3 yet. The constraints below are the orchestrator's brief plus the locked constraints carried over from PROJECT.md, REQUIREMENTS.md and the Phase 2 CONTEXT. A `/gsd:discuss-phase` pass may still lock the open choices listed under Open Questions.

### Locked Decisions
- **Zero false positives.** When the analyzer is not certain, it stays silent. *Unknown facts mean "may suppress".* Ten false negatives beat one false positive.
- **No network calls, no external processes, no writes inside the analysed repository, no on-disk cache.** This covers CLI-04 and CLI-05, plus PROJECT.md's Out of Scope list.
- **Deterministic output.** Same input gives byte-identical stdout, in the same order, with the same exit code, from any checkout path. Output paths are repo-relative only (CLI-03, DIAG-04).
- **DDD/hexagonal layering, CI-enforced by `scripts/check-architecture.sh`:**
  - `internal/domain` is pure: stdlib allowlist, no `fmt`.
  - `internal/application` uses the domain allowlist plus `context`, no `fmt`, and depends only on domain and application.
  - HCL (`hashicorp/*`, `zclconf/*`) is imported only inside `internal/infrastructure`.
  - `cmd/gruntled` is the composition root and must never link `internal/testsupport`.
  - Existing rules must not be weakened. Every new rule gets a self-test case that fails on that rule's name.
- **Keep dependencies minimal.**
- **Go 1.27.** go.mod says `go 1.27`, and CI uses `GOTOOLCHAIN: local` with `go-version-file: go.mod`.
- **The DIAG-03 rule is decided deliberately in this phase**, and it is documented and tested. The tested cases must include the corpus pattern: `mock_outputs_merge_with_state = true` with `apply` among the allowed commands. The absence of mocks must never manufacture a diagnostic (ROADMAP Phase 3 SC2).
- **Phase 2 is still executing.** Its unfinished plan 02-05 (`loader.go`, `merge.go`, `reasons.go`, integration and fuzz tests) is the intended design. Phase 3 builds on `ports.UnitLoader`/`SurfaceReader`, `indexing.Build`, and the domain as they exist now.
- **Repo conventions.**
  - Commits are authored by Giulio Savini <giuliosavini@proton.me>, with no AI attribution or trailer.
  - Every change is committed, pushed and tested locally before push.
  - CI has few jobs, and every job must be able to genuinely fail.
  - Code is formatted with `"$(go env GOROOT)/bin/gofmt"`.
  - `staticcheck` flags unused unexported code, tests included.

### Claude's Discretion
- CLI framework: stdlib `flag` or `spf13/cobra`.
- Output formats and their exact shapes.
- Exit-code numbering.
- Package split for the analyzer, use case, presenters and CLI.
- Diagnostic message wording.
- Test tooling: golden files, testscript.

### Deferred Ideas (OUT OF SCOPE)
- SARIF output (REQUIREMENTS v2 INT-02).
- `graph --json` (INT-01), and pre-commit/CI recipes (INT-03).
- GRT002–GRT006. This includes an "undeclared dependency label" diagnostic (DEP-12) and cycles (GRT003).
- The daemon: `watch`, `report`, status file. Also `blast`.
- The corpus experiment and the `terragrunt hcl validate` benchmark (Phase 4).
- The on-disk index cache.
</user_constraints>

<phase_requirements>
## Phase Requirements

| ID | Description | Research Support |
|----|-------------|-----------------|
| DIAG-01 | Report `GRT001` when a `dependency.X.outputs.Y` reference names an output the target module does not declare | Pattern 1 (pure analyzer over `RepositoryGraph`). Prototype: corpus gives 0 diagnostics; renaming `output "role_name"` gives exactly 8 |
| DIAG-02 | Report `GRT100` for HCL syntax errors, with file and line | Already produced by Phase 2 (`hclconv.FirstSyntaxError`, one per file, byte columns). Phase 3 presents it, merges it into one `diagnostic.Set`, maps it to exit 1, and tests it end to end (Pattern 8) |
| DIAG-03 | Apply a documented, tested rule for whether `mock_outputs` suppresses `GRT001` | Pattern 1 decision table, derived from Terragrunt source. **Mocks never suppress.** `enabled != true` or `skip_outputs != false` always suppresses. Pattern 2 adds lazy-evaluation guards, and Patterns 3 and 4 add two new silent cases |
| DIAG-04 | Every diagnostic carries a repo-relative path (never absolute), line, column and stable code | `repograph.RepoPath` makes this true by construction, and `os.OpenRoot(dir).FS()` means the absolute path never enters the pipeline. Tests: stdout never contains the temp-dir path, and JSON `file` never starts with `/` |
| CLI-01 | `gruntled check [path]` prints diagnostics in human-readable form | Pattern 6 (text presenter, `file:line:col: severity CODE: message`) |
| CLI-02 | Exit 0 when clean and non-zero on error, with documented codes | Pattern 7 (codes 0/1/2/3, documented in `--help` and docs, asserted by testscript) |
| CLI-03 | Byte-identical output, in identical order, for identical input | `diagnostic.NewSet` gives canonical order, and encoding uses structs only. Verified: the mutated corpus from two differently-named directories gave `cmp`-identical stdout |
| CLI-04 | No network calls and no external processes | Pattern 9: a static `binary-no-net-no-exec` arch rule. Verified: the wired binary links no `net`, `os/exec`, `plugin` or `crypto/tls`, and no linked non-std package calls `StartProcess`/`ForkExec`. **Adding cobra would link `net` (through pflag)** |
| CLI-05 | Writes nothing inside the analysed repository | `os.Root` is opened read-only and `fs.FS` has no write API. Test: a snapshot before and after, taken on a read-only tree, with HOME/TMPDIR pointed at empty dirs. Verified on the mutated corpus |
</phase_requirements>

## Summary

The Phase 2 graph already carries every fact GRT001 needs:
- two-hop resolution;
- tri-state dependency options;
- module-unknown and config-unknown states;
- byte columns.

The analyzer is therefore a small pure function in `internal/domain/analysis`. The hard part of this phase is **deciding exactly when a missing output is certainly a bug**. That was derived from Terragrunt's own resolution code (`pkg/config/dependency.go`), whose output semantics are identical in `v1.1.6` and on `main`:
- `enabled = false` makes `outputs` equal to `mock_outputs` (or absent).
- `skip_outputs = true` makes `outputs` equal to `mock_outputs` (when the command is allowed) or absent. So in both cases the module's surface is never consulted, and GRT001 must be silent.
- Otherwise `outputs` is the target's state outputs. With a merge strategy other than `no_merge`, and the command allowed, the mock keys are merged in **only where state lacks them** (`shallowMergeCtyMaps`: state wins).

The consequence is that in the corpus pattern, renaming an output makes `apply` silently substitute the mock value (`"rp2-cicd-assume-role"`) instead of failing. That is not "genuinely works". It is a silent wrong value in production, and it is exactly the defect GRT001 exists to catch. **Recommendation: `mock_outputs` never suppresses GRT001. It only adds an explanatory suffix to the message.** VALID-04 (Phase 4) depends on this: all 22 corpus references are mock-covered with merge-with-state and `apply` allowed. The prototype reported 8/8 after the mutation, which would be 0/8 if mocks suppressed.

Three false-positive sources are **not** covered by Phase 2 and must be fixed in this phase:
1. **Lazy evaluation.**
   - `hclsyntax.ConditionalExpr` evaluates both branches but reports only the selected branch's diagnostics.
   - `&&`/`||` short-circuit and drop the other operand's diagnostics.
   - `for` bodies are never evaluated over an empty collection.

   All of these were verified in hcl v2.24.0 and v2.25.0. So `c ? dependency.x.outputs.missing : null` can be valid at runtime. Phase 2's `refWalker` currently emits such references (its tests OUT-10 and the non-ASCII column test assert it).
2. **Stack targets.** When the target directory contains `terragrunt.stack.hcl`, Terragrunt resolves `outputs` from the stack (`tryGetStackOutput`, in v1.1.6 and main), not from the unit's module.
3. **Include-target "units".** A legacy parent `terragrunt.hcl` that children include is also discovered as a unit. Its `config_path`s then resolve against its own directory. This was reproduced: a spurious GRT001 on the parent while the real child was clean.

The CLI should use the **stdlib `flag` package**. Adding `spf13/cobra` v1.10.2 links `net`, `net/url` and `net/netip` (pflag's IP flag types) and `text/template` into the binary. That destroys the simple, CI-checkable proof of CLI-04 ("the binary links no network package") for a one-subcommand v0.1.

Test the CLI end to end with `rogpeppe/go-internal/testscript` v1.16.0. It is a test-only dependency, never linked into the binary, and was verified on go1.27.0. Pair it with plain Go tests for the synthrepo oracle, checkout-path determinism and the no-writes snapshot.

**Primary recommendation:** Build the pipeline in five pieces:
- a pure `analysis.UnknownOutputs(graph)` implementing the Pattern 1 decision table;
- an `application/checking.Check` use case;
- pure text/JSON presenters in `internal/interfaces/presenter`;
- a `run(args, stdout, stderr) int` composition root in `cmd/gruntled` on stdlib `flag`, with exit codes 0/1/2/3;
- three small infrastructure guards (lazy-evaluation refs, stack targets, include-target units).

Enforce CLI-04 and the presenter layering with two new arch rules and their self-tests.

## Standard Stack

### Core
| Library | Version | Purpose | Why Standard |
|---------|---------|---------|--------------|
| Go stdlib `flag` | go1.27.0 | `gruntled check [--format text\|json] [path]` parsing | Zero dependencies. `flag.ContinueOnError` gives control over exit codes. v0.1 has exactly one subcommand. Keeps CLI-04 statically provable (Pattern 9) |
| Go stdlib `os.OpenRoot` + `(*os.Root).FS()` | go1.27.0 | Read-only, escape-proof repository FS handed to the Phase 2 adapters | Already the Phase 2 contract. Paths stay repo-relative by construction (DIAG-04). No write API (CLI-05) |
| Go stdlib `encoding/json` (v1) | go1.27.0 | `--format json` | Deterministic for structs. Use `Encoder.SetEscapeHTML(false)` + `SetIndent("", "  ")`. `encoding/json/v2` also compiles on go1.27.0 without GOEXPERIMENT (verified), but v1 is frozen by the Go 1 compatibility promise and the output is identical for these shapes. Use v1 |
| Existing: `hashicorp/hcl/v2` v2.25.0, `zclconf/go-cty` v1.19.0 | pinned in go.mod | Only in `internal/infrastructure` (the refs guard in Pattern 2 uses `hclsyntax` node types) | No change |

### Supporting
| Library | Version | Purpose | When to Use |
|---------|---------|---------|-------------|
| `github.com/rogpeppe/go-internal/testscript` | **v1.16.0** (2026-07-01, `go 1.25` in its go.mod) | txtar end-to-end CLI tests: real exit codes, stdout/stderr, cwd-relative `.` | `cmd/gruntled/main_test.go` only (test-only; `go list -deps ./cmd/gruntled` does not include it). Verified on go1.27.0: `testscript.Main(m, map[string]func(){...})` plus `testscript.Run(t, Params{Dir: "testdata/script", RequireExplicitExec: true})`. Adds `golang.org/x/sys v0.46.0 // indirect` to go.mod. `RunMain` is deprecated, so use `Main` |
| `internal/testsupport/synthrepo` | in-repo | `Generate(spec, dir)` into two differently-named temp dirs (determinism), `Manifest.Expected` as the exact GRT001 oracle | `_test.go` files only (arch rule `binary-links-testsupport` already polices the binary) |

### Alternatives Considered
| Instead of | Could Use | Tradeoff |
|------------|-----------|----------|
| stdlib `flag` | `spf13/cobra` v1.10.2 (+ `spf13/pflag` v1.0.9, `inconshreveable/mousetrap` v1.1.0 on Windows) | **Rejected for v0.1.** It is listed in PROJECT.md's stack line, so it needs user sign-off (Open Question 2). Measured on go1.27.0: it pulls `net`, `net/url`, `net/netip` (pflag `ip.go`), `text/template` and `encoding/base64` into the binary. The binary does not call the network, but "links no `net` package" stops being a one-line CI proof. Its defaults also fight determinism and exit codes (usage printed on error, auto `completion` command, `Execute()` returns an error, not a code). Revisit when `watch`/`report`/`blast`/`graph` land (v2) |
| testscript | In-process `run()` table tests + golden files with a `-update` flag | Zero dependencies and fully viable. It loses the real `os.Exit` path and txtar's self-contained fixtures. Use it if the user vetoes the test-only dependency |
| Hand-written JSON | `owenrumney/go-sarif/v3` v3.3.1 | SARIF is deferred (INT-02). When it lands, SARIF 2.1.0's minimal result shape is small enough for `encoding/json` structs. No library is needed |

**Installation:**
```bash
GOTOOLCHAIN=go1.27.0 go get github.com/rogpeppe/go-internal@v1.16.0   # test-only
go mod tidy
```

## Architecture Patterns

### Recommended Project Structure
```
cmd/gruntled/
  main.go                  # composition root: run(args, stdout, stderr) int; flag parsing; os.OpenRoot; wiring; exit codes
  main_test.go             # TestMain -> testscript.Main; TestScripts; Go tests (oracle, determinism, no-writes, no abs path)
  testdata/script/*.txtar  # CLI contract: exit codes, usage, text/json golden, GRT100, DIAG-03 cases
internal/
  domain/analysis/
    grt001.go              # UnknownOutputs(*repograph.RepositoryGraph) ([]diagnostic.Diagnostic, error): pure, DIAG-03 table
    grt001_test.go         # hand-built graphs only, one row per decision-table case
  application/checking/
    check.go               # Check(ctx, ports.UnitLoader, ports.SurfaceReader) (Report, error)
    check_test.go          # fakes only
  interfaces/presenter/
    text.go  json.go       # pure: (io.Writer, checking.Report) -> error; no os, no infrastructure
    presenter_test.go      # hand-built Reports, golden strings
  infrastructure/terragrunt/
    refs.go                # + lazy-evaluation guard (Pattern 2)
    loader.go / merge.go   # + stack-target guard (Pattern 3), include-target units (Pattern 4)
    reasons.go             # + ReasonConfigPathStack, ReasonIncludeTarget
scripts/check-architecture.sh       # + interfaces-external-deps, binary-no-net-no-exec
scripts/test-check-architecture.sh  # + one self-test case per new rule
docs/cli.md                         # documented exit codes, formats, JSON schema v1, DIAG-03 rule
```

`internal/interfaces` already appears in the synthrepo package doc and in research/ARCHITECTURE.md §B.6. Keep the flag parsing and wiring in `cmd/gruntled` (composition root). Presenters are pure and therefore unit-testable without a filesystem.

### Pattern 1: GRT001 analyzer and the DIAG-03 decision table

**What happens at runtime** (Terragrunt `pkg/config/dependency.go`; the functions below are identical in `v1.1.6` and `main@5dc737a` apart from an unrelated config_path nil check):
- `getTerragruntOutputIfAppliedElseConfiguredDefault`: if disabled, return `MockOutputs` (possibly nil, in which case `outputs` is absent).
- `shouldGetOutputs` = `!SkipOutput && enabled && skip_outputs != true`. When false, outputs are `MockOutputs` if `shouldReturnMockOutputs`, else absent. **The module's outputs are never read.**
- If state outputs are non-empty and `shouldMergeMockOutputsWithState` (strategy ≠ `no_merge` **and** the command is allowed) and mocks are set, the result is `shallowMergeCtyMaps(state, mocks)`: **state wins, and a mock only fills keys state lacks**. With `deep_map_only` it is a top-level union. With `deep` the strategy is invalid and Terragrunt errors.
- If state outputs are empty (`"{}"`: the target is not applied, **or it declares zero outputs**), the result is the mocks if allowed, else an error.
- "Command allowed" is `mock_outputs_allowed_terraform_commands == nil || len == 0 || contains(cmd)`. **A literal empty list means all commands.**
- Mock merge strategy: `mock_outputs_merge_strategy_with_state`, when set, overrides the deprecated `mock_outputs_merge_with_state`.

**The rule.** For each reference, in `g.References()` order (already total: Pos, Unit, Dependency, Output), the first matching row wins:

| # | Condition | Result | Why |
|---|-----------|--------|-----|
| 0 | Reference is inside `try()`/`can()` or a lazily-evaluated position | not a `Reference` at all (infrastructure) | Pattern 2. HCL drops diagnostics from unselected branches |
| 1 | `unit.Dependency(label)` not found (DEP-12, undeclared label) | silent | A real error, but not GRT001's claim. It belongs to a future GRT00x |
| 2 | `opts.Enabled != TristateTrue` (false or unknown) | silent | Disabled means `outputs` is `mock_outputs` only. Unknown means it may be disabled |
| 3 | `opts.SkipOutputs != TristateFalse` (true or unknown) | silent | The module's outputs are never read (**reverses catalogue OUT-13/DEP-09**, with source evidence) |
| 4 | `g.DependencyTarget(unit, label)` false (unresolved, target not a unit, stack target per Pattern 3) | silent | Nothing to check against |
| 5 | `g.ModuleOf(target)` false (target config- or module-unknown) or `Surface()` not ok | silent | Surface unknown |
| 6 | `surface.HasOutput(Y)` | no diagnostic | Declared |
| 7 | otherwise | **GRT001, `SeverityError`, `NewForUnit(unit, ref.Pos())`** | `mock_outputs` never suppresses: see below |

`mock_outputs`, `mock_outputs_merge_with_state` and `mock_outputs_allowed_terraform_commands` **do not change whether GRT001 fires, or its severity**. They only select the message suffix:
- A missing output with a mock covering it and `apply` allowed makes `apply` silently inject the mock value. This covers both merge-with-state and zero-output modules.
- A missing output without that mock makes `apply` fail. That is the `denis256/terragrunt-tests/issue-2163` shape: mocks with allowed `["validate","plan"]`, and the module declares no such output.
- Both are bugs, and neither is uncertain.

Unknown mock facts therefore cannot create a false positive. They only drop the suffix.

Rejected alternatives (documented in `docs/cli.md`):
- **Suppress** when a mock covers Y and merge applies at apply. This makes VALID-04 impossible on the primary corpus: all 22 references have that shape. It also hides the worst variant of the bug, a silent mock value in production.
- **Downgrade to warning.** Warnings exit 0 (Pattern 7), so CI would pass a repo that deploys mock values. The Key is unaffected (Severity is excluded from Key), but the signal is lost.

```go
// internal/domain/analysis/grt001.go (domain allowlist: strconv, slices; no fmt)
func UnknownOutputs(g *repograph.RepositoryGraph) ([]diagnostic.Diagnostic, error) {
	var out []diagnostic.Diagnostic
	for _, ur := range g.References() {
		unit, ok := g.Unit(ur.Unit)
		if !ok {
			continue
		}
		dep, ok := unit.Dependency(ur.Reference.Dependency())
		if !ok { // row 1: undeclared label is not GRT001
			continue
		}
		opts := dep.Options()
		if opts.Enabled != repograph.TristateTrue || opts.SkipOutputs != repograph.TristateFalse { // rows 2-3
			continue
		}
		target, ok := g.DependencyTarget(ur.Unit, dep.Name()) // row 4
		if !ok {
			continue
		}
		mod, ok := g.ModuleOf(target.Path()) // row 5
		if !ok {
			continue
		}
		surf, ok := mod.Surface()
		if !ok || surf.HasOutput(ur.Reference.Output()) { // rows 5-6
			continue
		}
		msg := "dependency " + strconv.Quote(dep.Name()) + " output " + strconv.Quote(ur.Reference.Output()) +
			" is not declared by module " + strconv.Quote(mod.Path().String()) +
			" (target unit " + strconv.Quote(target.Path().String()) + ")"
		if mockMasksAtApply(opts, ur.Reference.Output(), surf) {
			msg += "; mock_outputs supplies it, so apply would silently use the mock value"
		}
		d, err := diagnostic.NewForUnit(diagnostic.CodeUnknownOutput, diagnostic.SeverityError, ur.Unit, ur.Reference.Pos(), msg)
		if err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, nil
}

// mockMasksAtApply is true only when every fact is a certain literal. Unknown => plain message.
func mockMasksAtApply(o repograph.DependencyOptions, output string, s repograph.Surface) bool {
	if o.MockOutputs.Contains(output) != repograph.TristateTrue {
		return false
	}
	applyAllowed := o.MockAllowedCommands.IsAbsent() // absent => every command allowed
	if names, ok := o.MockAllowedCommands.Names(); ok {
		applyAllowed = len(names) == 0 || slices.Contains(names, "apply") // known EMPTY list => every command
	}
	return applyAllowed && (o.MockMergeWithState == repograph.TristateTrue || len(s.Outputs()) == 0)
}
```

### Pattern 2: Lazy-evaluation guard in reference extraction (infrastructure)

**What:** Extend `refWalker` in `internal/infrastructure/terragrunt/refs.go` so a reference is not emitted when it sits inside a sub-expression that HCL may evaluate without surfacing its errors. This is the same fail-silent mechanism as the existing `try`/`can` guard.

**Evidence** (hclsyntax v2.24.0 and v2.25.0):
- `ConditionalExpr.Value` evaluates both branches, then appends only `trueDiags` or `falseDiags` for the selected branch.
- `OpLogicalAnd`/`OpLogicalOr` have a `ShortCircuit` that returns only the controlling side's diagnostics. `false && x` and `x && false` both drop `x`'s errors.
- A `for` expression's key, value and `if` sub-expressions run once per element, so zero times over an empty collection. The collection expression is always evaluated and is **not** guarded.
- Template directives parse to the same nodes (verified): `%{ if }` gives `*ConditionalExpr`, and `%{ for }` gives `*TemplateJoinExpr{*ForExpr}`.

Only the named sub-expressions become lazy. The ternary **condition** and the `for` **collection** stay checked.

```go
// refs.go additions (sketch). Walk calls Enter(n), children, Exit(n), so push/pop balance.
func lazyRanges(n hclsyntax.Node) []hcl.Range {
	switch e := n.(type) {
	case *hclsyntax.ConditionalExpr:
		return []hcl.Range{e.TrueResult.Range(), e.FalseResult.Range()}
	case *hclsyntax.ForExpr:
		rs := []hcl.Range{e.ValExpr.Range()}
		if e.KeyExpr != nil {
			rs = append(rs, e.KeyExpr.Range())
		}
		if e.CondExpr != nil {
			rs = append(rs, e.CondExpr.Range())
		}
		return rs
	case *hclsyntax.BinaryOpExpr:
		if e.Op == hclsyntax.OpLogicalAnd || e.Op == hclsyntax.OpLogicalOr {
			return []hcl.Range{e.LHS.Range(), e.RHS.Range()}
		}
	}
	return nil
}
// Enter: w.lazy = append(w.lazy, lazyRanges(n)...)
// Exit:  w.lazy = w.lazy[:len(w.lazy)-len(lazyRanges(n))]
// ScopeTraversalExpr: emit only if w.guard == 0 && no r in w.lazy has r.Start.Byte <= start.Byte < r.End.Byte
```

**Phase 2 tests this changes.** Update these deliberately and record the change as a reversal of catalogue OUT-10:
- `refs_test.go` row `"OUT-10 both ternary branches"` now expects no references.
- Row `"OUT-09 for expression"` keeps its reference, because it sits in the collection.
- The non-ASCII byte-column test (`a = "ééé" == "" ? dependency.x.outputs.y : ""`, refs_test.go:173) must move the reference out of the ternary. For example `a = ["ééé", dependency.x.outputs.y]`, then recompute the expected byte column.
- The same line appears in 02-05's planned `TestWholeBodyRefs`.

The corpus is unaffected: all 22 of its references are plain `inputs` values.

### Pattern 3: Stack-target guard (infrastructure)

**What:** In the loader's dependency resolution (02-05 `resolveOneDependency`), when the resolved target directory contains a regular file `terragrunt.stack.hcl`, keep the dependency but make it unresolved with a new reason `config-path-stack`. `DependencyTarget` then returns false and GRT001 stays silent.

**Evidence:**
- `getTerragruntOutput` first calls `tryGetStackOutput`.
- `resolveStackFilePath` maps `.../terragrunt.hcl` or a directory to `<dir>/terragrunt.stack.hcl`. If that file exists, `outputs` becomes the stack's nested unit outputs, not the unit module's.
- This is present in `v1.1.6` (3 occurrences) and `main`.
- A `config_path` whose base name is `terragrunt.stack.hcl` resolves to the same directory, so the same check covers it.

### Pattern 4: Include-target units (infrastructure)

**What:** In `Loader.LoadUnits`, after every unit is resolved, collect the set of include files that any unit actually resolved. Any discovered unit whose own `<dir>/terragrunt.hcl` is in that set becomes config-unknown with a new reason, `include-target`.

**Why (reproduced with the prototype):** Take `live/terragrunt.hcl` (legacy root, included by `live/prod/app` via `find_in_parent_folders()`) with `dependency "vpc" { config_path = "../vpc" }` plus `dependency.vpc.outputs.id`:
- The child resolves correctly to `live/prod/vpc` (declares `id`) and stays silent.
- The parent, analysed as its own unit, resolves `../vpc` against `live/`, reaching the repo-root `vpc` unit. It printed `live/terragrunt.hcl:4:21: error GRT001 ... [unit live]`.

The parent's references are already checked, correctly, once per including unit (Key includes Unit), so this loses no real finding. It only removes the self-interpretation. Catalogue STACK-09 ("included and independently runnable") becomes a documented false negative.

The legacy root-`terragrunt.hcl` layout appears in the secondary corpus `cds-snc/secret`.

### Pattern 5: `checking` use case (application, pure)

```go
// internal/application/checking/check.go (allowlist: context, errors, slices...; no fmt)
type Report struct {
	Graph       *repograph.RepositoryGraph
	Diagnostics diagnostic.Set // GRT100 (loader + surfaces) ∪ GRT001, canonical order, deduped by Key
}

func Check(ctx context.Context, units ports.UnitLoader, surfaces ports.SurfaceReader) (Report, error) {
	res, err := indexing.Build(ctx, units, surfaces)
	if err != nil {
		return Report{}, err // *indexing.Error, mapped to exit 3 by the CLI
	}
	grt001, err := analysis.UnknownOutputs(res.Graph)
	if err != nil {
		return Report{}, &Error{Stage: "analyze", Err: err}
	}
	return Report{Graph: res.Graph, Diagnostics: diagnostic.NewSet(append(res.Diagnostics.All(), grt001...)...)}, nil
}
```
Summary counts (units total, resolved, module-unknown, config-unknown; unknown-surface modules) are derived by the presenters from `Report.Graph`. The use case stays minimal.

### Pattern 6: Presenters, text and JSON (interfaces, pure)

**Text (stdout, one line per diagnostic, canonical Set order):**
```
<file>:<line>:<col>: <severity> <CODE>: <message>[ (unit <unit>)]
live/app/terragrunt.hcl:7:17: error GRT001: dependency "vpc" output "vpc_idd" is not declared by module "live/vpc" (target unit "live/vpc") (unit live/app)
root.hcl:3:1: error GRT100: Unclosed configuration block: ...
```
- `file:line:col:` is the errorformat editors and GitHub problem matchers parse.
- The unit is printed for every unit-attributed diagnostic. A reference in a shared include produces one diagnostic per including unit **at the same position**, and without the unit the lines look like duplicates.
- Paths are the `RepoPath` strings: always `/`-separated, including on Windows.
- **Summary goes to stderr**, not stdout. Examples: `gruntled: checked 65 units (0 unknown): 0 errors, 0 warnings`, or `gruntled: no Terragrunt units found`. The latter distinguishes "nothing analysed" from "clean", per the PITFALLS UX note. Both are deterministic anyway.

**JSON (stdout, one document, schema version 1):**
```go
type jsonReport struct {
	Version        int             `json:"version"` // 1
	Diagnostics    []jsonDiag      `json:"diagnostics"`
	UnknownUnits   []jsonUnknown   `json:"unknown_units"`   // path, status ("module-unknown"|"config-unknown"), reason
	UnknownModules []jsonUnknown   `json:"unknown_modules"` // modules with unknown surface: path, reason
	Summary        jsonSummary     `json:"summary"`         // units, resolved, module_unknown, config_unknown, errors, warnings
}
type jsonDiag struct {
	Code     string `json:"code"`
	Severity string `json:"severity"`
	File     string `json:"file"`
	Line     int    `json:"line"`
	Column   int    `json:"column"`
	Unit     string `json:"unit,omitempty"`
	Message  string `json:"message"`
}
```
Rules:
- Structs only, never maps.
- Slices initialised with `make(..., 0, n)` so empty collections print `[]`, never `null`.
- `enc.SetEscapeHTML(false)`, `enc.SetIndent("", "  ")`.
- `Encode` writes the trailing newline.
- Every list is sorted: diagnostics in Set order, unknowns by path.

The JSON shape lets Phase 4 assert "0 diagnostics, 0 unknown units" on the corpus through the CLI, and it maps 1:1 to SARIF `result` fields later.

### Pattern 7: Exit codes (documented table)

| Code | Meaning |
|------|---------|
| 0 | Analysis completed. No error-severity diagnostic (warnings, if any ever exist, do not fail) |
| 1 | Analysis completed. At least one error-severity diagnostic (`GRT001`, `GRT100`) |
| 2 | Usage error: no/unknown command, unknown flag, invalid `--format`, more than one path |
| 3 | Analysis could not run: path missing, not a directory, unreadable; `indexing.Error`; stdout write failure |

Code 2 matches Go's `flag` convention. Separating 1 (findings) from 3 (tool failure) lets CI distinguish "your config is wrong" from "gruntled could not run", which is the shellcheck/tflint style. Print the table in `gruntled check -h` and in `docs/cli.md`, and assert every row in testscript.

### Pattern 8: Composition root with stdlib `flag`

```go
// cmd/gruntled/main.go
func main() { os.Exit(run(os.Args[1:], os.Stdout, os.Stderr)) }

func runCheck(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("check", flag.ContinueOnError)
	fs.SetOutput(stderr)
	format := fs.String("format", "text", "output format: text or json")
	var paths []string
	for rest := args; ; { // stdlib flag stops at the first positional: re-parse so `check . --format json` works
		if err := fs.Parse(rest); err != nil {
			if errors.Is(err, flag.ErrHelp) {
				return exitOK
			}
			return exitUsage
		}
		if fs.NArg() == 0 {
			break
		}
		if consumed := len(rest) - fs.NArg(); consumed > 0 && rest[consumed-1] == "--" {
			paths = append(paths, fs.Args()...) // after "--" everything is positional; do not re-parse
			break
		}
		paths = append(paths, fs.Arg(0))
		rest = fs.Args()[1:]
	}
	if len(paths) > 1 || (*format != "text" && *format != "json") {
		fmt.Fprintln(stderr, "gruntled: usage: gruntled check [--format text|json] [path]")
		return exitUsage
	}
	dir := "."
	if len(paths) == 1 {
		dir = paths[0]
	}
	root, err := os.OpenRoot(dir) // read-only; a file or missing path fails here
	if err != nil {
		fmt.Fprintf(stderr, "gruntled: cannot open repository: %v\n", err)
		return exitFailure
	}
	defer root.Close()
	fsys := root.FS()
	rep, err := checking.Check(context.Background(), terragrunt.NewLoader(fsys), tfsurface.NewReader(fsys))
	if err != nil {
		fmt.Fprintf(stderr, "gruntled: %v\n", err)
		return exitFailure
	}
	var buf bytes.Buffer // render fully, then one Write: nothing partial on stdout
	if *format == "json" {
		err = presenter.JSON(&buf, rep)
	} else {
		err = presenter.Text(&buf, rep)
	}
	if err != nil {
		return exitFailure
	}
	if _, err := stdout.Write(buf.Bytes()); err != nil {
		return exitFailure
	}
	presenter.Summary(stderr, rep)
	if rep.Diagnostics.HasErrors() {
		return exitFindings
	}
	return exitOK
}
```
This was verified in the prototype with testscript:
- `check . --format json` works, and `check -- ./` works.
- `check a b` exits 2.
- `check . --format yaml` exits 2.
- A missing path exits 3.

`cmd/gruntled` imports `internal/infrastructure/*` directly (allowed: `hcl-only-in-infrastructure` checks direct HCL imports only). The existing `check-architecture.sh` passed on the prototype wiring.

### Pattern 9: Architecture rules for this phase

Add both rules to the existing `architecture` job. That adds no new CI job, and each rule gets a self-test.

1. **`binary-no-net-no-exec`** (CLI-04):
   - `go list -deps ./cmd/gruntled` (no `-test`) must not contain `^(net|net/.+|os/exec|plugin|crypto/tls)$`.
   - The Go files of every non-standard linked package (`go list -deps -f '{{if not .Standard}}...{{.Dir}} {{.GoFiles}}{{end}}'`) must not match `\bos\.StartProcess\b|\bsyscall\.(ForkExec|Exec|StartProcess)\b`. `os.StartProcess` lives in `os`, which the import deny-list cannot see.
   - Currently green (verified): the linked non-std set is only gruntled, hcl/v2, go-cty, go-textseg, levenshtein, go-wordwrap and x/text.
   - Self-test cases: `cmd/gruntled/zz_probe.go` importing `os/exec`, then importing `net`, then calling `os.StartProcess`. Each must fail on this rule's name.
2. **`interfaces-external-deps`**: non-std deps of `./internal/interfaces/...` (with `-test`) must match `^<module>/internal/(domain|application|interfaces)/`. Presenters never reach infrastructure or HCL.
   - Guard it with the same non-vacuous pattern once the package exists.
   - Self-test: a presenter probe importing `internal/infrastructure/...`.

The self-test `mkcopy` already copies `cmd/` (including `testdata/`) and `go.sum`. go-internal will be in the module cache after `go get`, so copies build offline.

### Pattern 10: End-to-end testing

- **testscript (`cmd/gruntled/testdata/script/*.txtar`)**:
  - Hand-written fixtures inline.
  - `! exec gruntled check .` + `cmp stdout want.txt` + `stderr 'pattern'`.
  - One script per concern:
    - exit codes and usage;
    - text golden;
    - json golden;
    - GRT100 (mid-edit file);
    - the DIAG-03 cases (Pattern 1 rows, including the corpus shape and issue-2163);
    - the lazy-evaluation, stack-target and include-target silences;
    - the "no units found" message.
  - Set `RequireExplicitExec: true`. testscript's `stderr`/`stdout` take **single-quoted** regexps (a double-quoted pattern failed in the prototype).
- **Go tests in the same package**, calling `run()` in-process:
  - `TestOracle`: `synthrepo.Generate(Spec{Units: 60, IncludeDepth: 3, DependencyFanout: 3, Seed: 7, Errors: [BadOutputRef×4]}, dir)`, then `run(check --format json dir)`. The decoded GRT001 set `{file,line,column,unit}` must equal `Manifest.Expected` exactly, and the exit code must be 1.
  - `TestDeterministicAcrossCheckouts`: Generate the same Spec into `<tmp>/checkout-a` and `<tmp>/another-name`. Run each twice, in both formats. All stdout bytes must be equal, with no temp-dir path and no `__gruntled_repo_root__` in them. Also running from the directory with `.` must match running with the path argument. Also `diagnostic.Diff` of the two runs' Sets is empty.
  - `TestNoWrites`:
    - Snapshot `(path, size, mode, mtime, sha256)` of every entry.
    - `chmod` the tree read-only (skip when `os.Geteuid()==0` or on Windows), then snapshot again.
    - `t.Setenv` HOME, XDG_CACHE_HOME and TMPDIR to fresh empty dirs, then run `check`.
    - Assert the snapshot is unchanged and those dirs are still empty.
    - Do not compare atime (reads may update it).
  - `TestMutationDiff`: `Diff(clean, mutated)` has added = exactly the injected references, removed = empty. Adding an unrelated output to the target module changes no Key.

### Anti-Patterns to Avoid
- **`if mockCovers(dep, Y) { skip }`.** It kills VALID-04 on the corpus and hides silent mock values in production (research/PITFALLS.md Pitfall 3).
- **Reading `MockAllowedCommands.Contains("apply")` without special-casing a known empty list.** Terragrunt treats `[]` as all commands, but `NameList.Contains` on a known-empty list returns False.
- **Putting volatile text in the message.** Message is part of `diagnostic.Key`. A "did you mean X?" or "available outputs: …" suffix changes the Key whenever an unrelated output is added, causing Diff churn for the future daemon. Keep the message a pure function of: label, output, module path, target unit path, and the certain mock-masking fact.
- **Printing the analysed path, `os.Getwd()` or `filepath.Abs` anywhere on stdout.**
- **Using `fmt` or `os` in `internal/domain` / `internal/application`.** The allowlist forbids it. Messages are built with `strconv.Quote` and concatenation.
- **Ranging over a map when building JSON or the summary.**
- **Letting cobra/pflag into the binary.** See Standard Stack.
- **Treating a ternary condition or a `for` collection as lazy.** Those are always evaluated. Guarding them would add needless false negatives.

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| Canonical ordering and dedup of diagnostics | ad-hoc sort | `diagnostic.NewSet` / `Set.All()` / `Set.HasErrors()` | Already total (Pos, Code, Unit, Severity, Message) and Key-deduped |
| Two-hop resolution | re-walking config paths in the analyzer | `g.DependencyTarget` + `g.ModuleOf` + `Module.Surface()` | Phase 2 already encodes every unknown state |
| Repo-relative paths / escape safety | `filepath.Rel` / `Abs` | `os.OpenRoot(dir).FS()` + `repograph.RepoPath` | Paths never become absolute. Escapes are refused |
| Change detection between runs | custom comparison | `diagnostic.Diff` | Severity-blind, Unit-aware, tested |
| CLI subprocess harness | custom `os/exec` test harness | `testscript` (test-only) | Real exit codes, txtar fixtures, cwd handling |
| JSON encoding | string concatenation | `encoding/json` with structs | Correct escaping. Deterministic |
| Lazy-evaluation detection | evaluating conditions | AST ranges of `ConditionalExpr`/`ForExpr`/`BinaryOpExpr` | Never evaluate. The static shape is enough to stay silent |

**Key insight:** every silence decision in this phase is a *static fact about shape or literals*, never an evaluation. When a fact is not a literal, the answer is "silent", except that mock facts never suppress. They are informational only.

## Common Pitfalls

### Pitfall 1: Mocks read as proof an output exists
**What goes wrong:** GRT001 is suppressed whenever `mock_outputs` covers Y.
**Why it happens:** The ROADMAP wording "where a missing output genuinely works at runtime" reads as a suppression mandate. But Terragrunt's `shallowMergeCtyMaps(state, mocks)` fills a removed output with the mock value, so `apply` "works" by deploying a mock.
**How to avoid:** Implement Pattern 1 exactly. Add a testscript case with the exact corpus block (`mock_outputs = {role_name, region}`, `mock_outputs_merge_with_state = true`, `allowed = ["init","plan","apply","destroy","validate"]`) and a renamed output. It must produce GRT001 with the mock-masking suffix and exit 1.
**Warning signs:** The Phase 4 mutation run reports 0.

### Pitfall 2: Ternary, `&&`/`||` and `for`-body references reported
**What goes wrong:** `contains(keys(dependency.x.outputs), "y") ? dependency.x.outputs.y : null` gets GRT001.
**Why it happens:** Phase 2 extracts references from both branches (catalogue OUT-10). HCL drops the unselected branch's errors.
**How to avoid:** Pattern 2. Update the Phase 2 test rows it invalidates.

### Pitfall 3: `skip_outputs`/`enabled` treated as irrelevant
**What goes wrong:** Catalogue OUT-13 says to keep checking `skip_outputs = true` references. But with `skip_outputs` Terragrunt reads only `mock_outputs` (or nothing), never the module. The corpus has 5 units with `skip_outputs = true`.
**How to avoid:** Rows 2 and 3 of Pattern 1. Unknown also suppresses.

### Pitfall 4: A known-empty allowed-commands list
**What goes wrong:** `mock_outputs_allowed_terraform_commands = []` is read as "no command allowed".
**Why it happens:** Terragrunt code: `len(*...) == 0` means allowed.
**How to avoid:** Use Pattern 1's `mockMasksAtApply` and add a test row. Because of the chosen rule this only affects the message, but it must still be right.

### Pitfall 5: `merge_with_state = true` plus `merge_strategy_with_state = "no_merge"`
**What goes wrong:** Phase 2's `mockMergeWithState` returns True if *either* attribute says so. Terragrunt gives the strategy precedence, which means no merge.
**Impact:** Message suffix only, and it errs toward the "masks" wording. It is not a false-positive risk. Note it in `docs/cli.md`, or fix it in `depfacts.go` (strategy present → it decides).

### Pitfall 6: Stdlib `flag` stops at the first positional
**What goes wrong:** `gruntled check ./repo --format json` treats `--format` and `json` as paths.
**How to avoid:** Use Pattern 8's re-parse loop, and stop re-parsing after an explicit `--`. Reject more than one path with exit 2 (the prototype's first version silently accepted `check a b`).

### Pitfall 7: Absolute paths leaking
**What goes wrong:** An error message, the summary or JSON includes `dir` or `filepath.Abs(dir)`.
**How to avoid:** Stdout contains only RepoPath strings. Stderr may echo the user's own argument only on the exit-3 path. Test by grepping stdout for the temp-dir path in the determinism test.

### Pitfall 8: Include-target and stack-target units
**What goes wrong:** See Patterns 3 and 4. A standalone interpretation of a parent config, or a stack aggregate, is checked against the wrong surface.
**How to avoid:** Add the two new reasons, a fixture for each, and add them to 02-05's `TestUnknownReasons` go/parser coverage check (every `Reason*` constant must have a fixture).

### Pitfall 9: Diff churn from message content
**What goes wrong:** Messages embed things that change without the finding changing.
**How to avoid:** Pattern 1's message only. A line shift still changes the Key (Position is identity). Document that as a known limitation for the v2 daemon.

### Pitfall 10: `-race` needs cgo
**What goes wrong:** `go test -race` fails locally without a C toolchain ("-race requires cgo").
**How to avoid:** Run `-race` in CI only (ubuntu has gcc), as the 02-05 plan already notes. Local verification runs without `-race`.

### Pitfall 11: Snapshot taken before chmod in the no-writes test
**What goes wrong:** Mode bits differ because the test itself changed them (observed in the prototype).
**How to avoid:** chmod first, then snapshot, then run, then snapshot again. Restore write permission in `t.Cleanup` so `t.TempDir` removal works.

### Pitfall 12: Phase 2 not finished
**What goes wrong:** Phase 3 plans compile against `terragrunt.NewLoader`, which lives in uncommitted 02-05 files at research time.
**How to avoid:** Wave 0 depends on 02-05 being complete and green.

## Code Examples

### testscript harness (verified on go1.27.0)
```go
// cmd/gruntled/main_test.go
func TestMain(m *testing.M) {
	testscript.Main(m, map[string]func(){
		"gruntled": func() { os.Exit(run(os.Args[1:], os.Stdout, os.Stderr)) },
	})
}

func TestScripts(t *testing.T) {
	testscript.Run(t, testscript.Params{Dir: "testdata/script", RequireExplicitExec: true})
}
```

### DIAG-03 corpus-shape script (sketch)
```
# corpus pattern: mock covers the missing output, merge-with-state, apply allowed -> still GRT001
! exec gruntled check .
cmp stdout want.txt
stderr '1 errors'

-- vpc/terragrunt.hcl --
-- vpc/main.tf --
output "vpc_id" { value = "x" }
-- app/terragrunt.hcl --
dependency "vpc" {
  config_path                             = "../vpc"
  mock_outputs                            = { vpc_idd = "mock" }
  mock_outputs_merge_with_state           = true
  mock_outputs_allowed_terraform_commands = ["init", "plan", "apply", "destroy", "validate"]
}
inputs = { id = dependency.vpc.outputs.vpc_idd }
-- app/main.tf --
variable "id" {}
-- want.txt --
app/terragrunt.hcl:7:17: error GRT001: dependency "vpc" output "vpc_idd" is not declared by module "vpc" (target unit "vpc"); mock_outputs supplies it, so apply would silently use the mock value (unit app)
```
(Line 7, byte column 17: `inputs = { id = ` is 16 bytes. The prototype printed the same position for this fixture.)

### arch rule sketch
```bash
# --- binary-no-net-no-exec -------------------------------------------------
bin_deps=$(go list -deps ./cmd/gruntled)
forbidden=$(printf '%s\n' "$bin_deps" | grep -E '^(net|net/.+|os/exec|plugin|crypto/tls)$' || true)
spawners=$(go list -deps -f '{{if not .Standard}}{{$d := .Dir}}{{range .GoFiles}}{{$d}}/{{.}}{{"\n"}}{{end}}{{end}}' ./cmd/gruntled |
  grep -v '^$' | xargs grep -l -E '\bos\.StartProcess\b|\bsyscall\.(ForkExec|Exec|StartProcess)\b' || true)
if [ -n "$forbidden$spawners" ]; then
  echo "=== RULE FAILED: binary-no-net-no-exec ===" >&2
  printf '%s\n' $forbidden $spawners >&2
  fail=1
fi
```

## State of the Art

| Old Approach | Current Approach | When Changed | Impact |
|--------------|------------------|--------------|--------|
| `mock_outputs_merge_with_state = true` | `mock_outputs_merge_strategy_with_state = "shallow" \| "deep_map_only" \| "no_merge"` (strategy wins; `deep` is invalid for mocks) | Terragrunt 0.5x–1.x | Phase 2 already reads both. Pitfall 5 covers precedence |
| `dependency.X.outputs` from the unit only | from the **stack** when `<target>/terragrunt.stack.hcl` exists (`tryGetStackOutput`) | present in v1.1.6 | Pattern 3 |
| `terragrunt hcl validate` evaluates `dependency.X.outputs.Y` against real or mocked values | outputs become `cty.DynamicVal` during validate (`pctx.SkipOutput`) | #5811, closed 2026-04-10 | Confirms the GRT001 gap (VALID-05) |
| `testscript.RunMain(m, map[string]func() int)` | `testscript.Main(m, map[string]func())` | go-internal v1.14+ | Use `Main` |
| `encoding/json` only | `encoding/json/v2` available by default on go1.27.0 (verified compile, no GOEXPERIMENT) | Go 1.25 experiment, default by 1.27 | Stay on v1 for stability. Its output is identical here |

**Deprecated/outdated for this phase:**
- **02-TERRAGRUNT-EDGECASES.md OUT-13/DEP-09** ("do not special-case `skip_outputs`") is superseded by Terragrunt source evidence (Pattern 1 row 3).
- **OUT-10** ("resolve both ternary branches") is superseded by HCL source evidence (Pattern 2).
- **PITFALLS.md Pitfall 3** is confirmed and strengthened.
- **PROJECT.md "Tech stack: … spf13/cobra"** conflicts with the minimal-dependency brief and with a static CLI-04 proof. Open Question 2 covers it.

## Open Questions

1. **DIAG-03: confirm "mocks never suppress, error severity"** (the most consequential decision this phase makes)
   - What we know: Terragrunt source shows the corpus pattern makes `apply` silently use the mock value for a removed output. Suppression makes VALID-04 impossible on the primary corpus (prototype: 8/8 reported).
   - What's unclear: whether the user reads ROADMAP SC2's "genuinely works at runtime" as a mandate to suppress.
   - Recommendation: lock "never suppress, never downgrade, message suffix when masking is certain" in `/gsd:discuss-phase`, and document the rejected alternatives in `docs/cli.md`.
2. **stdlib `flag` vs cobra (PROJECT.md lists cobra)**
   - Recommendation: stdlib `flag` for v0.1, with the evidence under Standard Stack. Update PROJECT.md's stack line when this is locked. Reconsider when v2 adds three or more subcommands. The `binary-no-net-no-exec` rule would then need a `net` exception for pflag, or a cobra alternative.
3. **Include-target units (Pattern 4)**
   - What we know: the false positive was reproduced. The fix makes a both-included-and-runnable config (STACK-09) silent for its own standalone interpretation.
   - Recommendation: adopt it. The references are still checked per including unit.
4. **Where the Phase 2 changes land**
   - Patterns 2–4 touch `refs.go`, `refs_test.go`, `loader.go`/`merge.go` and `reasons.go`, which Phase 2 owns and is still writing.
   - Recommendation: make them the first Phase 3 plan (wave 1), after 02-05 is committed. Record them as deliberate reversals of catalogue OUT-10/OUT-13.
5. **Summary line: stderr or stdout in text mode**
   - Recommendation: stderr. Stdout stays pure diagnostics, which is greppable and diff-able, and JSON carries the summary.

## Validation Architecture

### Test Framework
| Property | Value |
|----------|-------|
| Framework | Go `testing` (go1.27.0), table tests with hand-built graphs, `rogpeppe/go-internal/testscript` v1.16.0 (test-only), `synthrepo` fixtures |
| Config file | none (go.mod only). Scripts in `cmd/gruntled/testdata/script/` |
| Quick run command | `go test -count=1 ./internal/domain/analysis ./internal/application/checking ./internal/interfaces/... ./cmd/gruntled` |
| Full suite command | `go vet ./... && go test -count=1 ./... && test -z "$("$(go env GOROOT)/bin/gofmt" -l .)" && go mod tidy -diff && GOTOOLCHAIN=go1.27.0 go run honnef.co/go/tools/cmd/staticcheck@v0.8.1 ./... && bash scripts/check-architecture.sh && bash scripts/test-check-architecture.sh` (`-race` runs in CI) |

### Phase Requirements → Test Map
| Req ID | Behavior | Test Type | Automated Command | File Exists? |
|--------|----------|-----------|-------------------|-------------|
| DIAG-01 | Missing output gives GRT001 at the reference's byte position, per unit. Declared output gives nothing. Synthrepo oracle is exact | unit + e2e | `go test ./internal/domain/analysis -run TestUnknownOutputs -count=1` and `go test ./cmd/gruntled -run 'TestOracle\|TestScripts/grt001' -count=1` | ❌ Wave 0 |
| DIAG-02 | Mid-edit `terragrunt.hcl` / include / module `.tf` gives one GRT100 per file, exit 1, repo-relative file:line:col | e2e | `go test ./cmd/gruntled -run TestScripts/grt100 -count=1` | ❌ Wave 0 |
| DIAG-03 | Pattern 1 rows 1–7 plus `mockMasksAtApply` cases: no mocks; issue-2163 (mocks, `[validate, plan]`); corpus shape; mock keys unknown; known-empty allowed list; zero-output module; enabled false/unknown; skip_outputs true/unknown; declared output with mocks absent/present | unit (table) + e2e | `go test ./internal/domain/analysis -run TestDIAG03 -count=1` and `go test ./cmd/gruntled -run TestScripts/diag03 -count=1` | ❌ Wave 0 |
| DIAG-03 (silences) | Ternary branch, `&&`/`\|\|` operand, `for` body and `%{if}` template references are not emitted; condition and collection still are. Stack target is silent. Include-target unit is silent | unit | `go test ./internal/infrastructure/terragrunt -run 'TestRefs\|TestLazy\|TestUnknownReasons\|TestIncludeTarget\|TestStackTarget' -count=1` | ❌ Wave 0 (refs_test.go rows updated) |
| DIAG-04 | No absolute path in stdout/JSON. JSON `file` never starts with `/` | e2e | `go test ./cmd/gruntled -run TestDeterministicAcrossCheckouts -count=1` | ❌ Wave 0 |
| CLI-01 | Text format golden. Summary on stderr. "no units found" message | e2e | `go test ./cmd/gruntled -run 'TestScripts/(text\|empty)' -count=1` | ❌ Wave 0 |
| CLI-02 | Exit 0 clean, 1 findings, 2 usage (`check a b`, bad `--format`, unknown command/flag), 3 missing path or path is a file. `--help` shows the table | e2e | `go test ./cmd/gruntled -run TestScripts/exitcodes -count=1` | ❌ Wave 0 |
| CLI-03 | Two checkout names × two runs × two formats give identical bytes. `Diff` of the runs is empty | e2e | `go test ./cmd/gruntled -run TestDeterministicAcrossCheckouts -count=1` | ❌ Wave 0 |
| CLI-04 | Binary links no `net`/`os/exec`/`plugin`/`crypto/tls`. No linked package calls StartProcess/ForkExec. Self-test proves the rule can fail | script | `bash scripts/check-architecture.sh && bash scripts/test-check-architecture.sh` | ✅ extend |
| CLI-05 | Read-only tree plus snapshot unchanged. HOME/XDG_CACHE_HOME/TMPDIR still empty | e2e | `go test ./cmd/gruntled -run TestNoWrites -count=1` | ❌ Wave 0 |
| (layering) | Presenters never import infrastructure or HCL | script | `bash scripts/test-check-architecture.sh` | ✅ extend |

### Sampling Rate
- **Per task commit:** the quick run command (well under 10 s; the prototype's full `check` on the 65-unit corpus took 71 ms).
- **Per wave merge:** the full suite command.
- **Phase gate:**
  - the full suite plus CI green;
  - one manual run against a local clone of the primary corpus: `gruntled check <corpus>` exits 0 with 0 diagnostics; after `sed` renames `output "role_name"` in `iac.src/s3_runtime`, it exits 1 with 8 GRT001s. Record this in the SUMMARY.

### Wave 0 Gaps
- [ ] Plan 02-05 committed and green. `terragrunt.NewLoader`, `reasons.go` and `TestUnknownReasons` exist.
- [ ] `GOTOOLCHAIN=go1.27.0 go get github.com/rogpeppe/go-internal@v1.16.0 && go mod tidy`. Then check that `go mod tidy -diff` is clean and `scripts/test-check-architecture.sh` still passes (its copies need go.sum).
- [ ] `internal/domain/analysis/grt001_test.go`: a hand-built-graph helper (units, modules, deps with `DependencyOptions` rows).
- [ ] `internal/application/checking/check_test.go`: small fakes (the indexing test fakes live in `_test.go` and are not importable; duplicate them, they are tiny).
- [ ] `cmd/gruntled/main_test.go` + `testdata/script/`.
- [ ] New arch rules and self-test cases, added before `internal/interfaces` gains code, so each is proven first.

## Sources

### Primary (HIGH confidence)
- `github.com/gruntwork-io/terragrunt` at `main@5dc737a` (2026-09-24), read via `gh api`:
  - `pkg/config/dependency.go`: `getTerragruntOutputIfAppliedElseConfiguredDefault`, `shouldGetOutputs`, `isEnabled`, `shouldReturnMockOutputs`, `shouldMergeMockOutputsWithState`, `getMockOutputsMergeStrategy`, `getTerragruntOutput` (`isEmpty := jsonBytes == "{}"`), `setRenderedOutputs`, `dependencyBlocksToCtyValue` (DynamicVal under SkipOutput), `tryGetStackOutput`, `resolveStackFilePath`, `DeepMerge` of mocks.
  - `pkg/config/cty_helpers.go`: `shallowMergeCtyMaps` (state wins, mocks fill missing), `deepMergeCtyMapsMapOnly`.
  - `pkg/config/config.go`: the `ParseConfig` stage-order doc comment.
  - `pkg/config/locals.go`: `canEvaluateLocals`, which shows `locals` cannot reference `dependency`.
- `github.com/gruntwork-io/terragrunt` release `v1.1.6` (2026-09-21): the same functions diffed against main. Only an unrelated `configPathString` nil guard differs. `tryGetStackOutput` is present.
- `github.com/hashicorp/hcl/v2` v2.24.0 (Terragrunt's pin) and v2.25.0 (ours), module cache: `hclsyntax/expression.go` `ConditionalExpr.Value` (selected-branch diagnostics only) and `hclsyntax/expression_ops.go` `OpLogicalAnd`/`OpLogicalOr` `ShortCircuit`. The AST shapes of `%{if}`/`%{for}` were verified with a scratch program.
- Go module proxy `@latest` (2026-09-25): cobra v1.10.2 (go.mod `go 1.15`, requires pflag v1.0.9, mousetrap v1.1.0, go-md2man, yaml), pflag v1.0.10, go-internal v1.16.0 (go.mod `go 1.25`), go-sarif/v3 v3.3.1, go-cmp v0.7.0, hcl/v2 v2.25.0.
- Scratch prototypes on go1.27.0, not committed; they live in `/tmp/gproto`, `/tmp/cobratest` and `/tmp/jsontest`:
  - The GRT001 analyzer and stdlib-flag CLI, wired to the real Phase 2 adapters. Against primary corpus `e6c55d1`: 65 units, 0 diagnostics, 71 ms. With `role_name` renamed: 8 GRT001, exit 1, stdout `cmp`-identical from two directory names. On a read-only tree the snapshot was unchanged and HOME stayed empty. The include-target false positive was reproduced.
  - `go list -deps ./cmd/gruntled`: no net, os/exec or crypto/tls, and no StartProcess in the non-std deps.
  - Cobra: `net`, `net/url`, `net/netip` and `text/template` are linked, via pflag.
  - testscript `Main`/`Run` passes, including interleaved flags, `--` and usage exits.
  - `encoding/json/v2` compiles without GOEXPERIMENT.

### Secondary (MEDIUM confidence)
- `.planning/research/PITFALLS.md` Pitfall 3 and the corpus survey (issue-2163 fixture in `denis256/terragrunt-tests/issue-2163/`), cross-checked against the Terragrunt source above.
- Exit-code conventions (0/1 findings, 2 usage, separate failure code): Go `flag` uses 2 for usage errors, and shellcheck/tflint separate "issues" from "tool errors". Summarised from general knowledge; the exact numbering is a design choice, not an external standard.

### Tertiary (LOW confidence)
- None of the recommendations rest on unverified claims.

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH. Versions come from the proxy, and each choice was compiled and run on go1.27.0. The cobra dependency footprint was measured.
- Architecture: HIGH for the DIAG-03 semantics (Terragrunt source, two versions) and the lazy-evaluation facts (hcl source, two versions). MEDIUM-HIGH for the package split, which is a design recommendation consistent with the existing arch rules. The prototype passed `check-architecture.sh`.
- Pitfalls: HIGH. Each was reproduced in the prototype or read in source (the include-target false positive, flag interleaving, the snapshot/chmod ordering, `-race` needing cgo).

**Research date:** 2026-09-25
**Valid until:** about 2026-10-25. Re-check `pkg/config/dependency.go` if Terragrunt ships a minor release before planning completes: mock/stack semantics moved between 1.0 and 1.1.
