# Phase 6: Machine-Readable Output - Context

**Gathered:** 2026-09-30
**Status:** Ready for planning

<domain>
## Phase Boundary

Two machine-readable documents over the existing graph and final rule set:
`gruntled graph --json` (the repository graph) and `gruntled check --format sarif`
(diagnostics as SARIF 2.1.0 that `github/codeql-action/upload-sarif` accepts). Exit codes of
`check` do not change, and `graph` writes nothing inside the analysed repository. Version
injection, releases, pre-commit and user-facing CI recipes are Phase 7.

</domain>

<decisions>
## Implementation Decisions

The user picked all four areas and asked Claude to answer the questions on its own. A question
agent wrote 44 questions with code-grounded recommendations. Every item below is *[auto]*: Claude
reviewed each one and accepted or changed it.

**Carried forward (locked):** repo-relative paths only; byte-identical output from two
differently-named checkout dirs; no writes inside the analysed repo (tested); presenters are pure
and live in `internal/interfaces/presenter`; `cmd/gruntled` is only the composition root; JSON is
built from structs only (never maps), empty lists are `[]` and never null, HTML is not escaped;
stdlib `flag`; no net/exec linked into the binary; exit codes 0/1/2/3.

### Graph JSON shape
- Top level, in this order: `version`, `kind`, `units`, `modules`, `edges`,
  `unresolved_dependencies`, `summary`. `kind` is `"graph"`. `version` is the graph document's own
  schema version (starts at 1) and is independent of check JSON's version. Check JSON is left
  unchanged: no `kind` is added there, and it has no version bump.
- `summary` reuses the existing tally (`presenter/summary.go`): units, resolved, module_unknown,
  config_unknown, unknown_modules. It has no error/warning counts, because graph runs no analyzers.
- Edges: one entry per `RepositoryGraph.Edges()` item. Edges are raw and lossless: block edges
  and paths edges stay separate, and nothing is deduplicated. Each edge has `kind`
  (`block`/`paths`), `from`, `to`, `position`, `name` (block edges only; omitted on paths edges),
  `target_state` (`has-config`/`no-config`/`dir-missing`/`unknown`), `enabled` and `skip_outputs`
  as tristate strings (`"true"`/`"false"`/`"unknown"`).
- `Edge` does not carry a name, options or target state today (`graph.go:267-273`). Extend the
  domain `Edge`. The presenter does not re-join through `Unit.Dependencies()`, so the graph
  stays the single source.
- Deferred, and additive later with no version bump: `mock_outputs`,
  `mock_merge_strategy_with_state` and `mock_outputs_allowed_terraform_commands` on edges.
- `unresolved_dependencies`: a separate list of `{from, kind, name?, position, reason}` for
  unresolved `dependency` blocks, unknown `PathDependency` entries and a non-tuple `paths`. This
  keeps `edges[].to` never null, and it is the Phase 5 "so Phase 6 can show it" item. Order: `from`,
  then `position`.
- Units: `{path, status, module?, reason?, references: []}`, where
  `references = [{dependency, output, position}]`. Only `module` and `reason` use `omitempty`.
- Modules: `{path, known, variables: [], outputs: [], reason?}`. `known` is a boolean, not a null
  surface. Surface lists are already sorted and unique.
- Positions: a nested `{file, line, column}` object that is reused everywhere. The column is
  1-based and counted in bytes (documented).
- Ordering is the graph's own: units and modules by path, edges in `Edges()` order.
- Reason text is the domain string as-is. It is documented as human text and not stable.
- Stability contract: a "Graph JSON" section in `docs/cli.md` with a field table. The rule is that
  additive fields do not bump the version and any removal or rename does. There is no JSON Schema
  file for graph in this phase.

### Graph command surface
- `gruntled graph --json [path]`. It takes `--json` only, as the roadmap says, and has no
  `--format`. Without `--json` it is a usage error (exit 2), and the help text says "--json is
  required; text output is reserved".
- The path argument follows the same rules as `check`: default `.`, flags before or after the
  path, `--`, at most one path. Extract a shared parse/open helper from the monolithic `runCheck`
  (`main.go:74-154`).
- Exit codes: 0 on success, even when there are unknowns or config-unknown (parse failure) units,
  because graph describes and does not judge. 2 is a usage error. 3 means the analysis could not
  run or the stdout write failed. Output is buffered before it is written, the same as check.
- Graph runs no analyzers and embeds no diagnostics. Build the graph through the indexing path.
  Use `checking.Check` only if no clean graph-only entry exists (Claude's choice).
- Nothing is written to stderr on success. stdout carries only machine output.
- Help: add a `graph` line to `topUsage`, and add a `graphUsage` constant with an exit-code block
  shown by `gruntled graph -h`. Update `usage.txtar` and the `docs/cli.md` Usage section.
- Tests: add `graph --json` to the existing `TestDeterministicAcrossCheckouts` and `TestNoWrites`
  loops, and to `TestRunStdoutWriteFailure`. Add a testscript per exit code, and a new
  `testdata/script/graph_golden.txtar` holding the full expected JSON for one fixture that has a
  block edge, a paths edge, an unresolved dependency, a module-unknown unit, a config-unknown unit
  and a cycle.

### SARIF content
- One run, and `$schema` points at the OASIS SARIF 2.1.0 JSON schema URL. There are no timestamps,
  no `startTimeUtc` and no GUIDs. When there are no results, `results` is `[]`. The encoding is
  struct-only with `SetEscapeHTML(false)`.
- `tool.driver`: `name: "gruntled"`, `informationUri` is the GitHub repo URL, and `version` is
  passed in from main through a small `ToolInfo` value. For now main passes `"dev"`. Phase 7
  replaces that value with the ldflags version and changes nothing in the presenter.
- Rules: one per `GRT` code in code order: GRT001, GRT002, GRT003 and GRT100. GRT100 IS emitted
  today (a broken `terragrunt.hcl`/include), so leaving it out would give results with an
  undefined `ruleId`. The roadmap's "one rule per GRT code" covers it.
- Rule metadata: `id`; a PascalCase `name`; `shortDescription` and `fullDescription` taken from the
  `docs/cli.md` Diagnostics headings; `helpUri` pointing at the `docs/cli.md` anchor on GitHub;
  `help.text`; `defaultConfiguration.level` mapped from severity; `properties.tags:
  ["terragrunt", "correctness"]` and `precision: "very-high"`. There is no `security-severity`,
  because these findings are not security alerts. A doc test keeps the rule titles in sync with
  `docs/cli.md`.
- Result: `ruleId`, `ruleIndex`, and a `level` mapped from `Severity.String()`.
  `message.text` is `d.Message()`, with ` (unit X)` appended when a unit is set, the same as the
  text presenter. This separates the duplicates that a shared include produces.
- Location: a relative `uri` with `uriBaseId: "%SRCROOT%"`. `originalUriBaseIds` is NOT emitted,
  because an absolute path breaks the repo-relative and determinism locks. Percent-encode the URI
  path segments (RFC 3986) in the presenter using only the allowed packages. `RepoPath` is already
  `/`-separated, so no Windows conversion is needed.
- Region: `startLine` plus `startColumn`, both 1-based, and the run declares
  `columnKind: "unicodeCodePoints"`. Columns are byte offsets, so the value is exact for ASCII HCL
  and can drift on lines with non-ASCII characters before the anchor. This is documented as a
  known limitation, and converting in infrastructure is deferred.
- `partialFingerprints` are NOT emitted. `upload-sarif` computes `primaryLocationLineHash` from
  the checked-out source. This avoids widening the presenter's import allowlist (no `crypto/*` or
  `hash/*` there) and avoids a second identity that differs from the diagnostic Key. The docs
  state this policy.
- `relatedLocations` are not emitted in this phase (cycle members and GRT001's target module).
  `Diagnostic` carries no member list, and adding one is a domain change (deferred).
- Unknown units and modules go in `invocations[0].toolExecutionNotifications` at level `note`,
  ordered by path, with repo-relative locations and no alerts. `executionSuccessful` is always
  `true`, and `exitCode` is omitted, because the exit code depends on the results.
- Exit 3 (the analysis could not run): no SARIF is printed, and there is only a stderr message.
  This is unchanged behaviour. Exit 1 on an error diagnostic, 0 when clean, and 2 on usage are all
  unchanged. `--format` accepts `text|json|sarif`, and the usage, help and docs are updated.
- `automationDetails` is not emitted. The upload action's `category` input covers that.
- Add `sarif` to the determinism, no-writes and stdout-failure test loops, plus a SARIF golden
  testscript.

### upload-sarif acceptance proof
- Local: a structural test in `cmd/gruntled` (not in the presenter package, whose test allowlist
  is only `testing`/`reflect`) asserts GitHub's ingestion constraints:
  - `version` is `"2.1.0"` and `$schema` is present;
  - there is exactly one run;
  - every `ruleId` is in `driver.rules`, and `ruleIndex` matches it;
  - `level` is in {error, warning, note};
  - every `uri` is relative and has `uriBaseId: "%SRCROOT%"`;
  - `startLine` and `startColumn` are ≥ 1;
  - every rule has `help.text` and `shortDescription`.
- CI: a job validates the golden SARIF against a vendored `sarif-schema-2.1.0.json`, using a
  pinned `go run` JSON-schema validator. This follows the existing staticcheck/govulncheck
  `go run` pattern. It adds no dependency to `go.mod`, and the vendored schema is a test asset.
- Real upload: a Phase 6 CI job runs `gruntled check --format sarif` on a dedicated fixture with
  deliberate GRT001-GRT003 diagnostics and uploads the result with
  `github/codeql-action/upload-sarif` (pinned by SHA). The job has
  `permissions: security-events: write` and `category: gruntled-fixture`, and it runs only on pushes
  to master, because fork PR tokens cannot upload. The check step tolerates exit 1 and
  fails on any other code. This is an internal proof job, not the user recipe. Phase 7 builds the
  documented recipe on top of it. Keep the fixture under `cmd/gruntled/testdata/` so its alerts
  are clearly fixture alerts.
- Evidence: record the green upload run URL in the Phase 6 VERIFICATION. Document the "SARIF"
  section (rule table, location/column policy, fingerprint policy, unknowns as notifications) and
  the "Graph JSON" section in `docs/cli.md`. Extend `validation_doc_test.go` to pin the SARIF rule
  names. Make one small README edit to the usage lines only.

### Claude's Discretion
- Package and file split inside `presenter` (`graph.go`, `sarif.go`), DTO names, the shared
  argument-parsing helper shape, whether graph uses `indexing` directly, the exact PascalCase rule
  names and descriptions (they must match the docs), which validator module is pinned, and the
  fixture contents.

</decisions>

<code_context>
## Existing Code Insights

### Reusable Assets
- `presenter/json.go`: struct-only encoder with `SetEscapeHTML(false)`, `[]`-not-null pattern and
  schema-version constant. It is the template for both new presenters.
- `presenter/summary.go` `tally`: unit/module counts for the graph `summary`.
- `RepositoryGraph.Edges()` (`repograph/graph.go:293-322`): deterministic edges. It skips
  unresolved deps, which is why `unresolved_dependencies` must come from `Unit.Dependencies()`
  and `Unit.PathDependencies()` in the domain.
- `checking.Report.Graph` already exposes the graph (`checking/check.go:19`).
- Test loops that loop over formats: `e2e_test.go:139` (determinism), `e2e_test.go:267` (no
  writes), `main_test.go:118` (stdout write failure).

### Established Patterns
- Architecture allowlist `scripts/check-architecture.sh:278-321`: `internal/interfaces` can use the
  pure domain list plus `fmt`, `io` and `encoding/json` only. There is no `crypto`, `hash`,
  `net/url` or `os`, so URI percent-encoding is hand-written with `strings`. Presenter tests can
  only use `testing` and `reflect`, so schema and structural tests live in `cmd/gruntled`.
- Columns are byte offsets (`infrastructure/hclconv/hclconv.go:1-5`).
- Golden harness `testdata/golden/*.txtar` parses text diagnostics only
  (`golden_test.go:320`). JSON and SARIF goldens go in `testdata/script/*.txtar`.
- Docs are test-pinned (`validation_doc_test.go`).

### Integration Points
- `cmd/gruntled/main.go`: `topUsage`, the `check` subcommand switch (`:65`), the `--format`
  validation (`:105`) and the new `graph` case.
- `.github/workflows/ci.yml`: new schema-validation step and master-only upload job.
- `docs/cli.md`: Usage, Exit codes, Output formats (new "Graph JSON" and "SARIF" subsections),
  Known limitations (column caveat, no fingerprints, no relatedLocations).

</code_context>

<specifics>
## Specific Ideas

- The user asked for an autonomous question agent with at least 40 questions, the same approach
  as Phase 5's *[auto]* items. It produced 44 questions, and each one was reviewed and answered
  above.
- Changes from the agent's recommendations: no `partialFingerprints` (the agent proposed a
  hand-rolled FNV-1a); edge options limited to `enabled` and `skip_outputs` (the agent proposed
  all of them); and `unresolved_dependencies` sourced from the domain, not from a presenter-side
  join.

</specifics>

<deferred>
## Deferred Ideas

- Text or DOT output for `gruntled graph` (the reserved non-`--json` mode).
- `mock_outputs` and the other mock options on graph edges (additive, no version bump).
- A JSON Schema file for the graph document.
- `partialFingerprints` (a stable `gruntled/v1` hash) if GitHub's line-hash proves noisy.
- `relatedLocations` for GRT003 cycle members and GRT001 target modules (needs a domain change).
- Converting byte columns to UTF-16 code units or code points in infrastructure.
- User-facing GitHub Actions and GitLab recipes (Phase 7).

</deferred>

---

*Phase: 06-machine-readable-output*
*Context gathered: 2026-09-30*
