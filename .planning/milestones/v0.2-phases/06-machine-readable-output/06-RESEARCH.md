# Phase 6: Machine-Readable Output - Research

**Researched:** 2026-10-01
**Domain:** Go stdlib JSON presenters (graph document, SARIF 2.1.0), CLI wiring, GitHub code scanning ingestion
**Confidence:** HIGH on code seams and design; MEDIUM on GitHub ingestion details (proved only by the real upload job)

<user_constraints>
## User Constraints (from CONTEXT.md)

### Locked Decisions

**Carried forward (locked):** repo-relative paths only; byte-identical output from two differently-named checkout dirs; no writes inside the analysed repo (tested); presenters are pure and live in `internal/interfaces/presenter`; `cmd/gruntled` is only the composition root; JSON is built from structs only (never maps), empty lists are `[]` and never null, HTML is not escaped; stdlib `flag`; no net/exec linked into the binary; exit codes 0/1/2/3.

**Graph JSON shape**
- Top level, in this order: `version`, `kind`, `units`, `modules`, `edges`, `unresolved_dependencies`, `summary`. `kind` is `"graph"`. `version` is the graph document's own schema version (starts at 1) and is independent of check JSON's version. Check JSON is left unchanged: no `kind` is added there, and it has no version bump.
- `summary` reuses the existing tally (`presenter/summary.go`): units, resolved, module_unknown, config_unknown, unknown_modules. It has no error/warning counts, because graph runs no analyzers.
- Edges: one entry per `RepositoryGraph.Edges()` item. Edges are raw and lossless: block edges and paths edges stay separate, and nothing is deduplicated. Each edge has `kind` (`block`/`paths`), `from`, `to`, `position`, `name` (block edges only; omitted on paths edges), `target_state` (`has-config`/`no-config`/`dir-missing`/`unknown`), `enabled` and `skip_outputs` as tristate strings (`"true"`/`"false"`/`"unknown"`).
- `Edge` does not carry a name, options or target state today (`graph.go:267-273`). Extend the domain `Edge`. The presenter does not re-join through `Unit.Dependencies()`, so the graph stays the single source.
- Deferred, and additive later with no version bump: `mock_outputs`, `mock_merge_strategy_with_state` and `mock_outputs_allowed_terraform_commands` on edges.
- `unresolved_dependencies`: a separate list of `{from, kind, name?, position, reason}` for unresolved `dependency` blocks, unknown `PathDependency` entries and a non-tuple `paths`. This keeps `edges[].to` never null, and it is the Phase 5 "so Phase 6 can show it" item. Order: `from`, then `position`.
- Units: `{path, status, module?, reason?, references: []}`, where `references = [{dependency, output, position}]`. Only `module` and `reason` use `omitempty`.
- Modules: `{path, known, variables: [], outputs: [], reason?}`. `known` is a boolean, not a null surface. Surface lists are already sorted and unique.
- Positions: a nested `{file, line, column}` object that is reused everywhere. The column is 1-based and counted in bytes (documented).
- Ordering is the graph's own: units and modules by path, edges in `Edges()` order.
- Reason text is the domain string as-is. It is documented as human text and not stable.
- Stability contract: a "Graph JSON" section in `docs/cli.md` with a field table. Additive fields do not bump the version; any removal or rename does. No JSON Schema file for graph in this phase.

**Graph command surface**
- `gruntled graph --json [path]`. Takes `--json` only, no `--format`. Without `--json` it is a usage error (exit 2), and the help text says "--json is required; text output is reserved".
- Path argument follows the same rules as `check`: default `.`, flags before or after the path, `--`, at most one path. Extract a shared parse/open helper from the monolithic `runCheck` (`main.go:74-154`).
- Exit codes: 0 on success, even with unknown or config-unknown units (graph describes, does not judge). 2 usage error. 3 analysis could not run or stdout write failed. Output buffered before written, same as check.
- Graph runs no analyzers and embeds no diagnostics. Build the graph through the indexing path. Use `checking.Check` only if no clean graph-only entry exists (Claude's choice).
- Nothing on stderr on success. stdout carries only machine output.
- Help: add a `graph` line to `topUsage`, a `graphUsage` constant with an exit-code block shown by `gruntled graph -h`. Update `usage.txtar` and the `docs/cli.md` Usage section.
- Tests: add `graph --json` to `TestDeterministicAcrossCheckouts` and `TestNoWrites` loops, and to `TestRunStdoutWriteFailure`. A testscript per exit code, and a new `testdata/script/graph_golden.txtar` with the full expected JSON for one fixture that has a block edge, a paths edge, an unresolved dependency, a module-unknown unit, a config-unknown unit and a cycle.

**SARIF content**
- One run; `$schema` points at the OASIS SARIF 2.1.0 JSON schema URL. No timestamps, no `startTimeUtc`, no GUIDs. No results => `results` is `[]`. Struct-only encoding with `SetEscapeHTML(false)`.
- `tool.driver`: `name: "gruntled"`, `informationUri` is the GitHub repo URL, `version` passed in from main through a small `ToolInfo` value. For now main passes `"dev"`. Phase 7 replaces it with the ldflags version.
- Rules: one per `GRT` code in code order: GRT001, GRT002, GRT003, GRT100. GRT100 IS emitted today, so it must be a rule.
- Rule metadata: `id`; PascalCase `name`; `shortDescription` and `fullDescription` from the `docs/cli.md` Diagnostics headings; `helpUri` to the `docs/cli.md` anchor on GitHub; `help.text`; `defaultConfiguration.level` mapped from severity; `properties.tags: ["terragrunt", "correctness"]` and `precision: "very-high"`. No `security-severity`. A doc test keeps rule titles in sync with `docs/cli.md`.
- Result: `ruleId`, `ruleIndex`, `level` from `Severity.String()`. `message.text` is `d.Message()`, with ` (unit X)` appended when a unit is set (same as the text presenter).
- Location: relative `uri` with `uriBaseId: "%SRCROOT%"`. `originalUriBaseIds` NOT emitted. Percent-encode URI path segments (RFC 3986) in the presenter using only allowed packages. `RepoPath` is already `/`-separated.
- Region: `startLine` plus `startColumn`, 1-based, and the run declares `columnKind: "unicodeCodePoints"`. Columns are byte offsets: exact for ASCII HCL, can drift with non-ASCII before the anchor. Documented as known limitation; infrastructure conversion deferred.
- `partialFingerprints` NOT emitted (upload-sarif computes `primaryLocationLineHash`). Docs state this policy.
- `relatedLocations` not emitted.
- Unknown units and modules go in `invocations[0].toolExecutionNotifications` at level `note`, ordered by path, repo-relative locations, no alerts. `executionSuccessful` always `true`, `exitCode` omitted.
- Exit 3: no SARIF printed, stderr message only. Exit 1 on error diagnostic, 0 clean, 2 usage: unchanged. `--format` accepts `text|json|sarif`; usage, help and docs updated.
- `automationDetails` not emitted.
- Add `sarif` to the determinism, no-writes and stdout-failure test loops, plus a SARIF golden testscript.

**upload-sarif acceptance proof**
- Local: a structural test in `cmd/gruntled` (not in the presenter package) asserts: `version` is `"2.1.0"` and `$schema` present; exactly one run; every `ruleId` in `driver.rules` and `ruleIndex` matches; `level` in {error, warning, note}; every `uri` relative with `uriBaseId: "%SRCROOT%"`; `startLine`/`startColumn` >= 1; every rule has `help.text` and `shortDescription`.
- CI: a job validates the golden SARIF against a vendored `sarif-schema-2.1.0.json`, using a pinned `go run` JSON-schema validator (follows staticcheck/govulncheck pattern; no `go.mod` dependency; vendored schema is a test asset).
- Real upload: a Phase 6 CI job runs `gruntled check --format sarif` on a dedicated fixture with deliberate GRT001-GRT003 diagnostics and uploads via `github/codeql-action/upload-sarif` (pinned by SHA). `permissions: security-events: write`, `category: gruntled-fixture`, only on pushes to master. Check step tolerates exit 1, fails on any other code. Internal proof job, not the user recipe. Fixture under `cmd/gruntled/testdata/`.
- Evidence: record the green upload run URL in Phase 6 VERIFICATION. Document "SARIF" and "Graph JSON" sections in `docs/cli.md`. Extend `validation_doc_test.go` to pin SARIF rule names. One small README edit to usage lines only.

### Claude's Discretion
Package and file split inside `presenter` (`graph.go`, `sarif.go`), DTO names, the shared argument-parsing helper shape, whether graph uses `indexing` directly, the exact PascalCase rule names and descriptions (must match docs), which validator module is pinned, and the fixture contents.

### Deferred Ideas (OUT OF SCOPE)
- Text or DOT output for `gruntled graph`.
- `mock_outputs` and other mock options on graph edges.
- A JSON Schema file for the graph document.
- `partialFingerprints`.
- `relatedLocations` for GRT003 / GRT001.
- Converting byte columns to UTF-16 / code points in infrastructure.
- User-facing GitHub Actions and GitLab recipes (Phase 7).
</user_constraints>

<phase_requirements>
## Phase Requirements

| ID | Description | Research Support |
|----|-------------|-----------------|
| INT-01 | `gruntled graph --json` emits one deterministic document: schema version, units, modules, edges, unknown reasons, repo-relative paths | Domain `Edge` must be extended (name, target state, skip_outputs); `unresolved_dependencies` sourced from `Unit.Dependencies()`/`PathDependencies()`; `indexing.Build` gives graph without analyzers; json.go is the template; testscript golden + determinism/no-writes loops |
| INT-02 | `gruntled check --format sarif` emits SARIF 2.1.0 accepted by upload-sarif, one rule per GRT code, repo-relative locations | Hand-written struct DTOs, 4 rules, `%SRCROOT%` URIs, vendored OASIS schema validated with pinned `jv`, real upload job with `checkout_path` |
</phase_requirements>

## Summary

Everything needed already exists in the codebase; this phase is two new presenters plus CLI wiring, one small domain extension (Edge) and CI/doc work. `presenter/json.go` is a direct template (struct-only DTOs, `bytes.Buffer` + `json.Encoder` with `SetEscapeHTML(false)` and indent, buffered write). `indexing.Build(ctx, units, surfaces)` returns `Result{Graph, Diagnostics}` without running analyzers, so `graph` should call it directly (no need for `checking.Check`). Imports allowed in presenters: domain, `fmt`, `io`, `encoding/json`, plus the pure domain list (`strings`, `strconv`, `sort`, `slices`, `bytes`, `unicode/utf8`...). So URI percent-encoding uses `strings.Builder` + `strconv`/hex lookup; no `net/url`.

The one non-obvious trap: SARIF URIs are relative to the *analysed path*, not the git repository root. If the CI proof job runs `gruntled check --format sarif cmd/gruntled/testdata/<fixture>`, URIs are fixture-relative, so `upload-sarif` must be given `checkout_path: cmd/gruntled/testdata/<fixture>` (documented input) or alerts will point to non-existent files. This also matters for Phase 7 recipes.

**Primary recommendation:** Implement `presenter.Graph(w, g)` and `presenter.SARIF(w, g, diags, ToolInfo)` as pure struct-DTO encoders cloned from `json.go`; extend domain `Edge` with name/targetState/skipOutputs; refactor `runCheck` into `parseArgs` + `openRepo` helpers; prove acceptance with (1) structural Go test, (2) vendored-schema validation via `go run github.com/santhosh-tekuri/jsonschema/cmd/jv@v0.7.0`, (3) master-only upload job.

## Standard Stack

### Core
| Library | Version | Purpose | Why Standard |
|---------|---------|---------|--------------|
| Go stdlib `encoding/json` | go 1.27 (go.mod) | Both documents | Already used by `json.go`; allowlisted for interfaces |
| Go stdlib `flag` | - | `graph` subcommand | Locked; same loop as `check` |
| `rogpeppe/go-internal/testscript` | v1.16.0 (in go.mod) | `graph_golden.txtar`, sarif golden, exit-code scripts | Existing harness (`exec gruntled-exit N ...`, `cmp stdout want`) |

### Supporting (CI only, not in go.mod)
| Tool | Version | Purpose | When |
|------|---------|---------|------|
| `github.com/santhosh-tekuri/jsonschema/cmd/jv` | v0.7.0 (verified resolves via `go run ...@v0.7.0`; usage `jv [OPTIONS] SCHEMA [INSTANCE...]`, supports draft-07 which SARIF schema uses) | Validate golden SARIF against vendored schema | CI step |
| OASIS `sarif-schema-2.1.0.json` | from `oasis-tcs/sarif-spec` main, `sarif-2.1/schema/` (112,768 bytes, blob sha 0f58372b...) | Vendored test asset in `cmd/gruntled/testdata/` | schema job |
| `github/codeql-action/upload-sarif` | v4.38.2 = commit `2892aa5e19bbd11bc0cff5427e3b750a04d9e3c2` (annotated tag dereferenced; re-verify at implementation) | Real upload proof | master-only job |
| `actions/checkout` | v7.0.1 = `3d3c42e5aac5ba805825da76410c181273ba90b1`; ci.yml currently uses `@v7` tag | match existing | job |

### Alternatives Considered
| Instead of | Could Use | Tradeoff |
|------------|-----------|----------|
| `jv` | Microsoft SARIF Multitool (needs .NET/npm) | Heavier, GitHub docs mention it, but breaks the `go run` pattern |
| `jv` | `check-jsonschema` (pip) | Extra toolchain |

**Installation:** none (no go.mod change). Validator invocation:
```bash
go run github.com/santhosh-tekuri/jsonschema/cmd/jv@v0.7.0 cmd/gruntled/testdata/sarif-schema-2.1.0.json cmd/gruntled/testdata/<golden>.sarif
```
Note: golden SARIF lives inside a txtar; extract it for CI by running the binary on a fixture (`go run ./cmd/gruntled check --format sarif <fixture> > out.sarif; jv schema out.sarif`) rather than parsing txtar. Exit code 1 must be tolerated.

## Architecture Patterns

### Recommended Project Structure
```
cmd/gruntled/
├── main.go                      # topUsage/graphUsage, runCheck, runGraph, shared parseArgs/openRepo
├── sarif_test.go                # structural ingestion test (new)
├── testdata/script/graph_golden.txtar, sarif_golden.txtar, graph_exitcodes.txtar
├── testdata/sarif-schema-2.1.0.json
└── testdata/sarif-fixture/      # deliberate GRT001-003 for the upload job
internal/domain/repograph/graph.go     # Edge gains name, targetState, skipOutputs (+ accessors)
internal/interfaces/presenter/graph.go # Graph(w, g)
internal/interfaces/presenter/sarif.go # SARIF(w, g, diags, ToolInfo); percentEncode helper
```

### Pattern 1: Struct-only encoder (copy of json.go)
```go
// Source: internal/interfaces/presenter/json.go
var b bytes.Buffer
enc := json.NewEncoder(&b)
enc.SetEscapeHTML(false)
enc.SetIndent("", "  ")
if err := enc.Encode(doc); err != nil { return err }
_, err := w.Write(b.Bytes())
```
Always `make([]T, 0, n)` so empty slices encode `[]`. Only `omitempty` on the fields CONTEXT lists (`module`, `reason`, edge `name`, unresolved `name`). Beware: `omitempty` on a nested struct (position) does nothing; use pointer or no omit.

### Pattern 2: Domain Edge extension
`Edges()` at `graph.go:293-322` builds `Edge{kind, from, to, pos, enabled}` from `Dependency` (`d.Name()`, `d.TargetState()`, `d.Options().SkipOutputs`, `d.Options().Enabled`) and `PathDependency` (`pd.TargetState()`). Add unexported fields `name string`, `state TargetState`, `skipOutputs Tristate` plus accessors `Name()`, `TargetState()`, `SkipOutputs()`. For paths edges: name "", skipOutputs `TristateFalse`?? Decide: CONTEXT says `skip_outputs` is a tristate string on every edge; for paths edges emit `"false"` (no such attribute; outputs not read). Document this. Existing constructors in `edges_test.go` use unexported fields; check test compile after extending. Sort order is unchanged.

### Pattern 3: unresolved_dependencies from Unit
Iterate `g.Units()`; for each `u.Dependencies()` where `d.Target()` not ok emit `{from: u.Path, kind:"block", name: d.Name(), position: d.PathPos() (or Pos(); pick the config_path position, consistent with Edge.Pos = pathPos), reason: d.UnresolvedReason()}`; for each `u.PathDependencies()` where target not ok emit `{kind:"paths", position: pd.Pos(), reason: pd.UnresolvedReason()}`. Sort by `from`, then `position` (units already sorted by path; deps sorted by Pos inside unit via `sortAndValidateDepsRefs`, but block and paths lists are separate, so merge-sort per unit with `Position.Compare`). A non-tuple `paths` is represented by an unresolved PathDependency (verify in `infrastructure/terragrunt` how it is encoded before planning that task: `grep -n NewUnresolvedPathDependency internal/infrastructure/terragrunt/*.go`).

### Pattern 4: CLI refactor
Extract from `runCheck`: `parseArgs(name string, usage string, args []string, stderr, define func(*flag.FlagSet)) (paths []string, code int, ok bool)` implementing the existing parse-again loop, then `openRepo(dir) (*os.Root, error)`. `runGraph`: `--json` bool; not set => stderr "gruntled: graph: --json is required; text output is reserved" + usage, exit 2. Build with `indexing.Build(ctx, terragrunt.NewLoader(fsys), tfsurface.NewReader(fsys))` (an `*indexing.Error` => exit 3). Reuse the buffer-then-write tail (extract `writeOut`). No stderr on success, so do not call `presenter.Summary`.
`check` validation: `format` in {text,json,sarif}; error message becomes "want text, json or sarif"; update `TestHelpMatchesDocs` expectations (`e2e_test.go:396`) and `usage.txtar`.

### Pattern 5: SARIF document shape
```jsonc
{
  "$schema": "https://raw.githubusercontent.com/oasis-tcs/sarif-spec/main/sarif-2.1/schema/sarif-schema-2.1.0.json",
  "version": "2.1.0",
  "runs": [{
    "tool": {"driver": {"name":"gruntled","version":"dev","informationUri":"https://github.com/GiulioSavini/gruntled","rules":[ {id,name,shortDescription:{text},fullDescription:{text},helpUri,help:{text},defaultConfiguration:{level},properties:{tags:[...],precision:"very-high"}} ]}},
    "columnKind": "unicodeCodePoints",
    "invocations": [{"executionSuccessful": true, "toolExecutionNotifications": [{"level":"note","message":{"text":"..."},"locations":[{"physicalLocation":{"artifactLocation":{"uri":"a/b","uriBaseId":"%SRCROOT%"}}}]}]}],
    "results": [{"ruleId":"GRT001","ruleIndex":0,"level":"error","message":{"text":"... (unit x)"},"locations":[{"physicalLocation":{"artifactLocation":{"uri":"...","uriBaseId":"%SRCROOT%"},"region":{"startLine":3,"startColumn":5}}}]}]
  }]
}
```
Notes: `invocations[].toolExecutionNotifications` with no `locations` is legal; `executionSuccessful` is the only required invocation property. `defaultConfiguration.level` for all four rules is "error" (all emitted at error today); `result.level` still set explicitly from severity. `invocations` must be emitted even with no unknowns? Emit `invocations` always with `toolExecutionNotifications: []` for stability (SARIF allows empty arrays).
Rule-index lookup: static slice of rule definitions in code order; `ruleIndex` by code. A diagnostic whose code is not in the table is an internal bug: return an error (exit 3) rather than emit an undefined ruleId.
Notification message: `unit "<path>" is <status>: <reason>` / `module "<path>" surface is unknown: <reason>`; the location is the unit's `terragrunt.hcl` or module dir. Check what location a unit/module has: units have a `Path()` (a directory) not a file, so use the directory path as the artifact URI, or omit location; an `artifactLocation` pointing at a directory is odd. LOW-risk recommendation: point at `<unitdir>/terragrunt.hcl` for units (verify the filename is constant in the loader) and omit location for modules (put the module path in the message). Decide in plan; CONTEXT says repo-relative locations.

### Pattern 6: Percent-encoding
Per path segment (split on `/`): keep RFC 3986 unreserved `A-Za-z0-9-._~`, encode every other byte as `%XX` uppercase hex (works for UTF-8 bytes directly). Do not encode `/`. Implementation with `strings.Builder` and a `const hex = "0123456789ABCDEF"`. Test via cmd-level tests or presenter_test (allowed: `testing`, `reflect`).

### Anti-Patterns to Avoid
- **Maps in DTOs:** key order non-deterministic.
- **`omitempty` on slices/ints:** lines/columns/lists must always be present.
- **Absolute paths or `originalUriBaseIds`:** breaks determinism lock.
- **Re-joining edges via `Unit.Dependencies()` in the presenter:** locked against.
- **Writing SARIF on exit 3:** analysis failure prints nothing to stdout.

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| SARIF schema validation | Custom validator | `jv@v0.7.0` + vendored OASIS schema | Schema is 112 KB, draft-07 |
| Txtar golden compare | Custom diff | testscript `cmp stdout want` | Existing harness |
| Argument loop | Second copy for graph | Shared `parseArgs` extracted from `runCheck` | Locked |
| Fingerprints | FNV hash | Nothing; upload-sarif adds `primaryLocationLineHash` | Locked |
| JSON key ordering | map + sort | Struct field order | Locked |

## Common Pitfalls

### Pitfall 1: URIs relative to analysed path, not git root
**What goes wrong:** `check --format sarif sub/dir` yields URIs relative to `sub/dir`; GitHub resolves against repo root and shows alerts on wrong/nonexistent files, fingerprint computation cannot find files.
**How to avoid:** upload job sets `checkout_path: cmd/gruntled/testdata/sarif-fixture` (input of upload-sarif; GitHub docs also list `checkout_path`/`checkout_uri`/`invocations[0].workingDirectory.uri` as the repo-root conversion mechanism). Alternatively run from a cwd equal to the fixture. Document for Phase 7. MEDIUM: confirm that alerts land on fixture files in the real run.

### Pitfall 2: Position of unresolved deps and `omitempty`
`name` omitted for paths kinds only; `Position` must always print all three fields. Zero Position (`IsZero`) should not occur for unresolved deps; check that `NewUnresolvedDependency` always receives non-zero positions (validateDependencyCommon).

### Pitfall 3: GitHub docs list region.endLine/endColumn and partialFingerprints as "required"
Docs page (docs.github.com SARIF support) lists them in a requirements bullet, but also says upload-sarif computes fingerprints for you; many third-party tools upload with only `startLine`. CONTEXT chose to omit. Treat as the main residual risk; the real upload job is the arbiter. Fallback if GitHub rejects missing endLine/endColumn: emit `endLine=startLine` and `endColumn` unknown, so avoid unless forced (domain Position has no end). MEDIUM confidence.

### Pitfall 4: columnKind mismatch
GitHub docs say UTF-16 code units is the standard. CONTEXT picks `unicodeCodePoints` (closer to bytes than utf16 for non-BMP only; both differ from bytes on any non-ASCII). Documented limitation; no change to code. Do not use `utf16CodeUnits` without conversion either.

### Pitfall 5: Determinism of `Edges()` and ties
Sort is from, pos, kind, to; with the new fields add no new key but equal edges (same from/pos/kind/to) are identical in output except name/state, which derive from the same dependency, so stable. `slices.SortFunc` is not stable; make sure two edges can't tie with different name (same pos implies same dependency). OK.

### Pitfall 6: Exit codes with `--format sarif`
Reuse the tail unchanged: `HasErrors()` => 1. Do not call `presenter.Summary` for sarif (stderr empty like json). `TestRunStdoutWriteFailure` needs a sarif case on a clean repo and one with findings.

### Pitfall 7: Doc-sync tests
`TestHelpMatchesDocs` (`e2e_test.go:396`) compares help with `docs/cli.md`; after editing `checkUsage`/adding `graphUsage` update docs in the same task or the test fails. Rule title doc test: parse `### GRTnnn: <title> (<severity>)` headings in `docs/cli.md` and compare with the presenter rule table (the presenter table cannot be read from cmd test unless exported; export rule metadata via a small exported function, e.g. `presenter.SARIFRules()` returning name/short description, or compare against emitted SARIF output from a run, which is simplest and needs no new export).

### Pitfall 8: Master-only job and fork PRs
Condition: `if: github.event_name == 'push' && github.ref == 'refs/heads/master'`; job-level `permissions: {contents: read, security-events: write}` (add `actions: read` if repo is private). Check step: `set +e; gruntled check ...; rc=$?; [ $rc -eq 0 ] || [ $rc -eq 1 ] || exit $rc` (and the SARIF must have been written; stdout redirect to file). Workflow-level `permissions: contents: read` already exists so the job needs its own override.

## Code Examples

### Edge construction (extended)
```go
// Source: internal/domain/repograph/graph.go (Edges), to be extended
edges = append(edges, Edge{kind: EdgeBlock, from: u.path, to: to, pos: d.pathPos,
    enabled: d.opts.Enabled, name: d.name, state: d.state, skipOutputs: d.opts.SkipOutputs})
edges = append(edges, Edge{kind: EdgePaths, from: u.path, to: to, pos: pd.pos,
    enabled: TristateTrue, state: pd.state, skipOutputs: TristateFalse})
```
(verify the Dependency/PathDependency private field names for name/state before coding.)

### testscript golden pattern
```
# graph golden
exec gruntled-exit 0 graph --json .
cmp stdout want.json
! stderr .
```
(Mirror `json_golden.txtar`: `exec gruntled-exit 1 check . --format json`, `cmp stdout want.json`, `! stderr .`.)

### Determinism loop extension
`e2e_test.go:139` iterates `[]string{"text","json"}` with `record("check","--format",format,d)` and requires `exitFindings`. For graph the exit is 0, so graph needs its own loop (or a table of `{args builder, wantCode}`), not a plain extra format string. Same for `TestNoWrites` (`:267`) and the write-failure table (`main_test.go:118`, add `{"graph json", {"graph","--json",repo(...)}}` and `{"sarif with a finding", ...}`).

## State of the Art

| Old | Current | Impact |
|-----|---------|--------|
| upload-sarif v3 | v4 (v4.38.2; v3.38.2 also still tagged) | pin v4 by SHA |
| Fingerprints required from tool | upload-sarif computes `primaryLocationLineHash` when missing | locked decision is viable |

## Open Questions

1. **Does GitHub accept results with only startLine/startColumn (no endLine/endColumn)?**
   - Known: common practice and upload-sarif's fingerprinting handle it; docs bullet is ambiguous.
   - Recommendation: implement as decided; the real upload job proves it; record the run URL in VERIFICATION.
2. **Location for unit/module notifications** (directories, not files). Recommendation: unit => `<dir>/terragrunt.hcl` if the loader's filename is fixed; module => no location, path in message text.
3. **Does a non-tuple `paths` surface as an unresolved PathDependency?** Check infra before planning the `unresolved_dependencies` task.
4. **`checkout_path` behaviour with a fixture subdirectory** (Pitfall 1). Verify in the real run; fallback is to run `gruntled` with cwd = repo root and fixture path of `.`-relative URIs by invoking a copy of the fixture at repo root in CI (avoid if possible).
5. **Is the repository public with code scanning enabled?** If private, GitHub Advanced Security is needed; confirm before the job is planned.

## Validation Architecture

### Test Framework
| Property | Value |
|----------|-------|
| Framework | Go `testing` + `rogpeppe/go-internal/testscript` v1.16.0 |
| Config file | none (`cmd/gruntled/main_test.go` registers `gruntled-exit`) |
| Quick run command | `go test ./internal/interfaces/... ./internal/domain/repograph/... ./cmd/gruntled -run 'Graph|SARIF|Sarif|Scripts|Deterministic|NoWrites|StdoutWrite|HelpMatchesDocs' -count=1` |
| Full suite command | `go test -race -count=1 ./... && bash scripts/check-architecture.sh` |

### Phase Requirements -> Test Map
| Req ID | Behavior | Test Type | Automated Command | File Exists? |
|--------|----------|-----------|-------------------|-------------|
| INT-01 | graph JSON shape/golden (block, paths, unresolved, module-unknown, config-unknown, cycle) | testscript | `go test ./cmd/gruntled -run TestScripts/graph_golden -count=1` | Wave 0 |
| INT-01 | Edge carries name/state/skipOutputs | unit | `go test ./internal/domain/repograph -run Edges -count=1` | extend edges_test.go |
| INT-01 | presenter: `[]` not null, key order, omitempty fields | unit | `go test ./internal/interfaces/presenter -run Graph -count=1` | Wave 0 |
| INT-01 | byte-identical across checkouts; nothing written | e2e | `go test ./cmd/gruntled -run 'TestDeterministicAcrossCheckouts|TestNoWrites' -count=1` | extend |
| INT-01 | `--json` required => exit 2; exits 0 with unknowns; 3 on bad path | testscript | `go test ./cmd/gruntled -run TestScripts/graph_exitcodes -count=1` | Wave 0 |
| INT-01 | stdout failure => 3 | unit | `go test ./cmd/gruntled -run TestRunStdoutWriteFailure -count=1` | extend |
| INT-02 | SARIF golden + exit codes unchanged (0/1) | testscript | `go test ./cmd/gruntled -run TestScripts/sarif_golden -count=1` | Wave 0 |
| INT-02 | structural ingestion constraints | unit | `go test ./cmd/gruntled -run TestSARIFStructure -count=1` | Wave 0 |
| INT-02 | percent-encoding, rule table, ruleIndex | unit | `go test ./internal/interfaces/presenter -run SARIF -count=1` | Wave 0 |
| INT-02 | rule names/titles match docs/cli.md | doc test | `go test ./cmd/gruntled -run 'SARIFDoc|ValidationDocPins|HelpMatchesDocs' -count=1` | extend |
| INT-02 | schema validity vs OASIS schema | CI/manual | `go run github.com/santhosh-tekuri/jsonschema/cmd/jv@v0.7.0 cmd/gruntled/testdata/sarif-schema-2.1.0.json <out.sarif>` | Wave 0 (CI step) |
| INT-02 | upload-sarif accepts | CI only (manual evidence) | master-push workflow run URL in VERIFICATION | Wave 0 (workflow) |
| Both | architecture allowlists (no new imports in presenter) | script | `bash scripts/check-architecture.sh` | exists |

### Sampling Rate
- **Per task commit:** quick run command above
- **Per wave merge:** full suite command
- **Phase gate:** full suite green, schema validation green, upload run URL recorded, before `/gsd:verify-work`

### Wave 0 Gaps
- [ ] `cmd/gruntled/testdata/script/graph_golden.txtar`, `graph_exitcodes.txtar`, `sarif_golden.txtar`
- [ ] `cmd/gruntled/sarif_test.go` (structure test using `encoding/json` into generic maps; allowed in cmd tests)
- [ ] `cmd/gruntled/testdata/sarif-schema-2.1.0.json` (vendored from oasis-tcs/sarif-spec) and `testdata/sarif-fixture/`
- [ ] presenter tests for graph/sarif in `presenter_test.go` (stdlib `testing`/`reflect` only)
- [ ] Manual-only: real upload (needs GitHub; justified, cannot run locally)

## Sources

### Primary (HIGH confidence)
- Local code: `cmd/gruntled/main.go`, `presenter/json.go`, `presenter/summary.go`, `repograph/graph.go:255-322`, `repograph/options.go`, `application/checking/check.go`, `application/indexing/build.go`, `scripts/check-architecture.sh:270-321`, `.github/workflows/ci.yml`, existing tests
- GitHub API (gh): codeql-action tags/SHAs, actions/checkout v7.0.1, jsonschema tags, OASIS schema file listing
- `go run ...jv@v0.7.0 -h` executed locally (resolves and runs)

### Secondary (MEDIUM confidence)
- https://docs.github.com/en/code-security/code-scanning/integrating-with-code-scanning/sarif-support-for-code-scanning (limits: 10 MB gz, 20 runs, 25,000 results/run (top 5,000 shown), 1,000 locations; version "2.1.0" only; levels note/warning/error; `checkout_path` for relative URI resolution; shortDescription/fullDescription max 1024 chars; tags max 20). Keep rule descriptions under 1024 chars.

### Tertiary (LOW confidence)
- Behaviour of upload-sarif without endLine/endColumn and with directory-typed notification locations: from training knowledge, unverified; proven only by the real upload.

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH, all tools checked
- Architecture: HIGH, code seams read
- Pitfalls: MEDIUM, GitHub ingestion specifics untested

**Research date:** 2026-10-01
**Valid until:** 2026-10-31 (re-verify action SHAs at implementation)
