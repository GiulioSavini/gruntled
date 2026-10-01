# gruntled CLI reference

gruntled reads a Terragrunt repository and reports `dependency.X.outputs.Y`
references that name an output the target module does not declare. It never
runs Terraform, Terragrunt or any other program.

## Usage

```
gruntled check [--format text|json|sarif] [path]
gruntled graph --json [path]
gruntled --version
```

`path` defaults to `.`. Examples:

```
gruntled check
gruntled check live/
gruntled check live/ --format json
gruntled check live/ --format sarif > gruntled.sarif
gruntled check --format=json -- -oddly-named-dir
gruntled graph --json live/
```

- Flags may appear before or after the path. The Go standard library `flag`
  package stops at the first positional argument, so gruntled parses again
  after each positional.
- `--` ends flag parsing. Everything after it is a path, even if it starts with `-`.
- More than one path is a usage error (exit 2).
- `gruntled -h` lists the commands. `gruntled check -h` and `gruntled graph -h`
  print the usage texts below and exit 0.

```
usage: gruntled <command> [arguments]

Commands:
  check   check a Terragrunt repository for broken dependency output references
  graph   print the repository graph as JSON (--json)

Flags:
  --version   print the version and commit, then exit

Run "gruntled check -h" or "gruntled graph -h" for details.
```

```
usage: gruntled check [--format text|json|sarif] [path]

Check the Terragrunt repository at path (default ".") for dependency
output references that name an output the target module does not declare.
Flags may appear before or after path; "--" ends flag parsing.

Flags:
  --format text|json|sarif   output format (default "text")

Exit codes:
  0  analysis completed, no error diagnostics
  1  analysis completed, at least one error diagnostic (GRT001-GRT003, GRT100)
  2  usage error: unknown command or flag, invalid --format, more than one path
  3  analysis could not run: path missing, not a directory or unreadable, or an internal failure
```

`gruntled --version` (also `-version`) prints one line to stdout and exits 0;
any arguments after it are ignored:

```
gruntled v0.2.0 (abc1234)
```

Release builds set both values at link time with
`-ldflags "-X main.version=<tag> -X main.commit=<short sha>"`. A binary built
without those flags, including one from `go build` or
`go install github.com/GiulioSavini/gruntled/cmd/gruntled@vX.Y.Z`, prints
`gruntled dev (none)`. The same version string is the SARIF
`tool.driver.version`.

`gruntled graph` builds the repository graph without running any analyzer
and prints it as one JSON document. `--json` is required: without it the
command exits 2, because plain-text graph output is reserved for a later
release.

```
usage: gruntled graph --json [path]

Print the dependency graph of the Terragrunt repository at path (default ".")
as a JSON document. No analyzers run; unknown units and modules are reported
in the document, not as failures.
Flags may appear before or after path; "--" ends flag parsing.

Flags:
  --json   print the graph as JSON (--json is required; text output is reserved)

Exit codes:
  0  graph printed, even with unknown units
  2  usage error: unknown flag, missing --json, more than one path
  3  analysis could not run: path missing, not a directory or unreadable, an internal failure, or stdout write failed
```

## Exit codes

| Code | Meaning |
|------|---------|
| 0 | Analysis completed. No error diagnostic. |
| 1 | Analysis completed. At least one error diagnostic (`GRT001`, `GRT002`, `GRT003`, `GRT100`). |
| 2 | Usage error: no or unknown command, unknown flag, invalid `--format`, more than one path. |
| 3 | Analysis could not run: path missing, not a directory or unreadable, an internal failure, or a failed write to stdout. |

The exit codes of `gruntled check` are the same for every `--format`
(`text`, `json`, `sarif`). The same table as printed by `gruntled check -h`:

```
  0  analysis completed, no error diagnostics
  1  analysis completed, at least one error diagnostic (GRT001-GRT003, GRT100)
  2  usage error: unknown command or flag, invalid --format, more than one path
  3  analysis could not run: path missing, not a directory or unreadable, or an internal failure
```

`gruntled graph` exits 0, 2 or 3 only: it runs no analyzers, so it never
reports findings, and a graph with unknown units or modules is still exit 0.
Exit 2 also covers a missing `--json`; exit 3 also covers a failed write to
stdout. As printed by `gruntled graph -h`:

```
  0  graph printed, even with unknown units
  2  usage error: unknown flag, missing --json, more than one path
  3  analysis could not run: path missing, not a directory or unreadable, an internal failure, or stdout write failed
```

CI can tell findings in the repository (1) apart from gruntled failing to run (3).

## Output formats

### Text (default)

stdout carries one line per diagnostic, in canonical order, and nothing else:

```
path:line:col: CODE message
```

followed by ` (unit U)` when the diagnostic belongs to a unit. Columns are
1-based byte columns. Paths are relative to the checked repository and always
use `/`, on every platform. A reference in a file included by several units
is reported once per including unit, at the same position; the unit suffix
tells the lines apart.

A one-line summary goes to stderr:

```
gruntled: checked N units (K unknown): E errors, W warnings
```

or `gruntled: no Terragrunt units found` when there was nothing to analyse.

Example. Given `app/terragrunt.hcl`:

```hcl
dependency "vpc" {
  config_path                             = "../vpc"
  mock_outputs                            = { vpc_idd = "mock" }
  mock_outputs_merge_with_state           = true
  mock_outputs_allowed_terraform_commands = ["init", "plan", "apply", "destroy", "validate"]
}
inputs = { id = dependency.vpc.outputs.vpc_idd }
```

and a `vpc` unit whose module declares only `output "vpc_id"`, stdout is:

```
app/terragrunt.hcl:7:17: GRT001 dependency "vpc" output "vpc_idd" is not declared by module "vpc" (target unit "vpc"); mock_outputs supplies it, so apply would silently use the mock value (unit app)
```

### JSON

`--format json` writes a single JSON document to stdout. There is no stderr
summary in JSON mode; the document carries it. The schema is versioned; this
is version 1.

| Key | Content |
|-----|---------|
| `version` | Schema version, `1`. |
| `diagnostics[]` | `code`, `severity`, `file`, `line`, `column`, `unit` (omitted for file-level diagnostics), `message`. Canonical order. |
| `unknown_units[]` | Units gruntled could not fully analyse: `path`, `status` (`module-unknown` or `config-unknown`), `reason`. Sorted by path. |
| `unknown_modules[]` | Modules whose outputs could not be read: `path`, `reason`. Sorted by path. |
| `summary` | `units`, `resolved`, `module_unknown`, `config_unknown`, `unknown_modules`, `errors`, `warnings`. |

Empty lists print as `[]`, never `null`. HTML characters (`<`, `>`, `&`) are
not escaped. `file`, `path` and `unit` follow the same repo-relative `/` rule
as text mode.

Example: the repository above plus a `dns` unit with a remote
`terraform { source = "git::https://..." }`:

```json
{
  "version": 1,
  "diagnostics": [
    {
      "code": "GRT001",
      "severity": "error",
      "file": "app/terragrunt.hcl",
      "line": 7,
      "column": 17,
      "unit": "app",
      "message": "dependency \"vpc\" output \"vpc_idd\" is not declared by module \"vpc\" (target unit \"vpc\"); mock_outputs supplies it, so apply would silently use the mock value"
    }
  ],
  "unknown_units": [
    {
      "path": "dns",
      "status": "module-unknown",
      "reason": "remote-source"
    }
  ],
  "unknown_modules": [],
  "summary": {
    "units": 3,
    "resolved": 2,
    "module_unknown": 1,
    "config_unknown": 0,
    "unknown_modules": 0,
    "errors": 1,
    "warnings": 0
  }
}
```

### Graph JSON

`gruntled graph --json` writes the repository graph as a single JSON
document to stdout and nothing to stderr. It runs no analyzer, so it carries
no diagnostics. The schema is versioned; this is version 1.

| Key | Content |
|-----|---------|
| `version` | Schema version, `1`. |
| `kind` | Always `"graph"`. |
| `units[]` | Every unit, sorted by path. |
| `modules[]` | Every module a unit points at, sorted by path. |
| `edges[]` | Every resolved dependency, block and paths alike. Sorted by `from`, then position, then kind, then `to`. |
| `unresolved_dependencies[]` | Dependencies whose target gruntled could not resolve. Sorted by `from`, then position. |
| `summary` | `units`, `resolved`, `module_unknown`, `config_unknown`, `unknown_modules`. |

A `position` is always the full object `{ "file", "line", "column" }`, with
the same repo-relative `/` paths and 1-based byte columns as `check`.

`units[]`:

| Field | Content |
|-------|---------|
| `path` | Unit directory. |
| `status` | `resolved`, `module-unknown` or `config-unknown`. |
| `module` | Module path. Omitted for `module-unknown` and `config-unknown` units. |
| `reason` | Why the unit is unknown. Omitted for `resolved`. |
| `references[]` | `dependency.X.outputs.Y` reads: `dependency`, `output`, `position`. |

`modules[]`:

| Field | Content |
|-------|---------|
| `path` | Module directory. |
| `known` | `true` when gruntled read the module's surface. |
| `variables[]`, `outputs[]` | Declared names, sorted. `[]` when unknown. |
| `reason` | Why the surface is unknown. Omitted when `known` is `true`. |

`edges[]`:

| Field | Content |
|-------|---------|
| `kind` | `block` (a `dependency` block) or `paths` (a `dependencies { paths }` entry). |
| `from`, `to` | Unit paths. `to` is the resolved directory, which may hold no unit. |
| `position` | The `config_path` value, or the paths entry. |
| `name` | The `dependency` block label. Omitted for `paths` edges. |
| `target_state` | `has-config`, `no-config`, `dir-missing` or `unknown`. |
| `enabled` | Tristate: `true`, `false` or `unknown` (not a literal). `paths` edges are always `"true"`. |
| `skip_outputs` | Tristate, as `enabled`. `paths` edges are always `"false"`. |

`unresolved_dependencies[]`: `from`, `kind`, `name` (omitted for `paths`),
`position`, `reason`. A `dependency` block whose `config_path` is not a
literal lands here, and so does a dynamic `paths` attribute. A `paths`
literal that is not a list yields no edges and nothing here.

Stability:

- Adding a field or a key is not a breaking change and does not bump
  `version`. Consumers must ignore fields they do not know.
- Removing or renaming a field, or changing its type or meaning, bumps
  `version`.
- `reason` values are human-readable text, not a stable enum. Do not match
  on them.
- Deferred, and additive later: `mock_outputs`, `mock_merge_strategy_with_state`
  and `mock_outputs_allowed_terraform_commands` on edges.

Empty lists print as `[]`, never `null`. HTML characters are not escaped.

### SARIF

`--format sarif` writes one SARIF 2.1.0 log with exactly one run to stdout,
for GitHub code scanning and other SARIF consumers. There is no stderr
summary. Exit codes are the same as for `text` and `json`. On exit 3 nothing
is printed to stdout, only the stderr message.

Rules, in `tool.driver.rules` in code order (`ruleIndex` indexes this list):

| Id | Name | `shortDescription` | Level |
|----|------|--------------------|-------|
| `GRT001` | `DependencyOutputNotDeclared` | dependency output not declared | `error` |
| `GRT002` | `DependencyTargetHasNoUnit` | dependency target has no unit | `error` |
| `GRT003` | `DependencyCycle` | dependency cycle | `error` |
| `GRT100` | `HclSyntaxError` | HCL syntax error | `error` |

`shortDescription` is the title of the rule's heading under
[Diagnostics](#diagnostics), and `helpUri` links to it. A result's `level`
is its severity: `error` → `error`, `warning` → `warning`, anything else
`note`. Its `message.text` is the diagnostic message, with ` (unit U)`
appended as in text mode, so the per-unit copies of a finding in a shared
include stay apart.

Locations:

- `uri` is relative to the analysed path, percent-encoded (RFC 3986, UTF-8,
  `/` kept), with `uriBaseId: "%SRCROOT%"`. No `originalUriBaseIds` is
  emitted, because an absolute path would break determinism. GitHub resolves
  `%SRCROOT%` to the checkout root, so either run gruntled on the repository
  root, or set the upload action's `checkout_path` to the analysed directory.
- `region` has `startLine` and `startColumn`, both 1-based. The run declares
  `columnKind: "unicodeCodePoints"`, but gruntled reports byte columns: exact
  on ASCII lines, off on lines with non-ASCII characters before the anchor.
- No `partialFingerprints`. `upload-sarif` computes `primaryLocationLineHash`
  from the checked-out source; a second fingerprint from gruntled would be an
  identity that differs from the diagnostic's own.
- No `relatedLocations` (cycle members, GRT001's target module): the
  diagnostic does not carry them yet.

Unknown units and modules are not results. They are `note`-level entries in
`invocations[0].toolExecutionNotifications`, sorted by path: a unit points at
`<dir>/terragrunt.hcl` (no region), a module has no location.
`executionSuccessful` is always `true` and `exitCode` is omitted. `results`
and `toolExecutionNotifications` are `[]` when empty, never `null`. The log
has no timestamps, GUIDs or `automationDetails` (use the upload action's
`category`), so the same repository gives identical bytes.

## Diagnostics

### GRT001: dependency output not declared (error)

A `dependency.X.outputs.Y` reference names an output `Y` that the module
behind dependency `X` does not declare. Resolution takes two hops: the
`dependency "X"` block's `config_path` leads to the target unit, and the
target unit's `terraform { source }` (or its own directory when there is no
`source`) leads to the module whose `output` blocks are read. If either hop
cannot be resolved with certainty, gruntled stays silent (see below).

Message:

```
dependency "<label>" output "<Y>" is not declared by module "<module>" (target unit "<target>")
```

with the suffix `; mock_outputs supplies it, so apply would silently use the mock value`
when mock masking is certain.

### GRT100: HCL syntax error (error)

A Terragrunt or Terraform file does not parse. One diagnostic per file,
reporting the first error only, at its byte column. The unit or module that
depends on the file becomes unknown, so GRT001 stays silent for it.

### GRT002: dependency target has no unit (error)

A `dependency` block's `config_path`, or an entry of a `dependencies { paths }`
list, resolves to a directory that holds no unit: it does not exist, or it
has no `terragrunt.hcl` (or `terragrunt.hcl.json`). Terragrunt refuses to run
such a dependency, whatever `skip_outputs` or `mock_outputs` say.

Messages:

```
dependency "<label>" config_path resolves to "<dir>": directory does not exist
dependency "<label>" config_path resolves to "<dir>": directory has no terragrunt.hcl
dependencies path "<entry>" resolves to "<dir>": directory does not exist
dependencies path "<entry>" resolves to "<dir>": directory has no terragrunt.hcl
```

Example:

```
live/app/terragrunt.hcl:2:17: GRT002 dependency "gone" config_path resolves to "live/gone": directory does not exist (unit live/app)
```

The position is the `config_path` value, or the paths entry. For each
dependency, the first matching row decides:

| # | Situation | Result |
|---|-----------|--------|
| 1 | The target is not a literal gruntled can resolve (`local.x`, `get_env()`, a function call, an empty block `config_path = ""`, a stack, a non-default file, outside the repository) | silent |
| 2 | The target directory holds a `terragrunt.hcl`, or gruntled could not read it (permission, not a directory, symlink escaping the root) | silent |
| 3 | A block whose `enabled` is not literally `true` (absent counts as `true`; `false`, a non-literal and a value from a deep-merged label do not). Paths entries have no `enabled` and skip this row | silent |
| 4 | The directory does not exist | GRT002 "directory does not exist" |
| 5 | The directory exists without `terragrunt.hcl` (for example a module-only directory) | GRT002 "directory has no terragrunt.hcl" |

- `skip_outputs` and `mock_outputs` never gate GRT002.
- The diagnostic belongs to the unit that declares the dependency. A block
  or paths list coming from a shared include gives one diagnostic per
  including unit, all at the same `file:line:col` in the include.
- `dependencies { paths }` entries are checked like `config_path`. Paths lists
  from includes are merged as a union; a target listed twice is reported
  once. A block and a paths entry naming the same missing directory both
  fire.

Rejected alternatives:

- **Suppress when `skip_outputs = true` or mocks are set.** Terragrunt still
  needs the directory to hold a unit, so the run would fail anyway.
- **Use `terragrunt hcl validate` as the oracle.** It needs Terragrunt and
  runs processes, which gruntled never does.

### GRT003: dependency cycle (error)

Units depend on each other in a cycle, which Terragrunt refuses to order.
There is one diagnostic per cycle (strongly connected component), attributed
to its lexically smallest unit, at that unit's first edge into the cycle.

Messages:

```
dependency cycle: "<a>" -> "<b>" -> "<a>"
dependency cycle among: "<a>", "<b>", "<c>"
```

Example:

```
ring/p/terragrunt.hcl:2:17: GRT003 dependency cycle: "ring/p" -> "ring/q" -> "ring/p" (unit ring/p)
```

| # | Situation | Result |
|---|-----------|--------|
| 1 | An edge whose `enabled` is not literally `true` | not an edge |
| 2 | An edge whose target is unresolved or not a unit of the repository (missing, no config) | not an edge |
| 3 | `skip_outputs = true` or mocks on an edge | still an edge: Terragrunt orders the units anyway |
| 4 | A `dependency` block and a `dependencies` path to the same unit | one edge |
| 5 | A unit depends on itself (`config_path = "."`, `"../<self>"`, or a paths entry `""`) | GRT003 `dependency cycle: "a" -> "a"` |
| 6 | Every unit in the cycle has exactly one successor inside it | GRT003 ring: `"a" -> "b" -> "a"`, walking from the smallest unit |
| 7 | Any other cycle, including one where a unit also loops on itself | GRT003 `among`: every member, sorted, no cap |

The message is part of the diagnostic's identity, so adding or removing a
unit in a cycle gives a new diagnostic.

Rejected alternatives:

- **One diagnostic per unit in the cycle.** It repeats the same finding N
  times; one per cycle is what the user has to fix.
- **Print the ring for every cycle.** A cycle with branches has no single
  ring, and picking one would hide the other edges.
- **Canonicalise symlink aliases** so two paths to the same directory become
  one unit. Deferred: it needs symlink resolution for every target, and the
  miss is only a false negative.

### Reserved codes

Every other code is reserved. GRT004 to GRT006 are planned for later
versions.

## How GRT001 treats mock_outputs, enabled and skip_outputs (DIAG-03)

For each reference, the first matching row decides:

| # | Situation | Result |
|---|-----------|--------|
| 1 | The unit has no `dependency` block with that label | silent |
| 2 | `enabled` is not literally `true` (it is `false`, or not a literal); absent means `true` | silent |
| 3 | `skip_outputs` is not literally `false` (it is `true`, or not a literal); absent means `false` | silent |
| 4 | `config_path` does not resolve to a unit in the repository | silent |
| 5 | The target unit's module, or its outputs, cannot be read | silent |
| 6 | The module declares the output | no diagnostic |
| 7 | Otherwise | GRT001, severity error |

Rows 2 and 3 are silent because Terragrunt then never reads the module's
outputs: `outputs` is `mock_outputs` or nothing.

**The rule: `mock_outputs` never suppresses GRT001 and never downgrades it.**
`mock_outputs`, `mock_outputs_merge_with_state`,
`mock_outputs_merge_strategy_with_state` and
`mock_outputs_allowed_terraform_commands` do not change whether GRT001 fires
or its severity. When the output is missing:

- If a mock covers it and `apply` may use mocks, Terragrunt's `apply` fills the
  missing output with the mock value. With merge-with-state, state wins and a
  mock only fills keys state lacks; a module with zero outputs has empty state,
  so mocks are used outright. Production silently gets a mock value. That is
  exactly the bug gruntled exists to catch.
- Without such a mock, `apply` fails.

Both are bugs. Mock facts only choose the message suffix.

The suffix appears only when all of these are literal and certain:

- `mock_outputs` has a key named after the output;
- `apply` is allowed: `mock_outputs_allowed_terraform_commands` is absent, is a
  literal empty list (both mean every command), or literally contains `"apply"`;
- merge-with-state is literally on, or the target module declares zero outputs.

Any non-literal fact drops the suffix. It never changes whether GRT001 fires.

`mock_outputs_merge_strategy_with_state`, when present, takes precedence over
the deprecated `mock_outputs_merge_with_state`, as in Terragrunt: `"no_merge"`
means off, `"shallow"` and `"deep_map_only"` mean on, anything else is unknown.

Rejected alternatives:

- **Suppress** GRT001 when a mock covers the output. This hides the worst
  variant, a silent mock value in production. It would also make the Phase 4
  mutation experiment impossible on the primary corpus, where every reference
  has this shape.
- **Downgrade to warning.** Warnings exit 0, so CI would pass a repository that
  deploys mock values.

Other silences come from the graph, not from this rule, and are handled when
the graph is built: references in unknown units; references in lazily evaluated
positions (ternary branches, `&&`/`||` operands, `for` bodies, `try()` and
`can()`), where HCL drops errors from the branch it does not take; dependencies
whose target is a Terragrunt stack (`terragrunt.stack.hcl`); and units whose
`terragrunt.hcl` is itself included by another unit, which are checked once per
including unit instead.

## Guarantees

- **Deterministic.** The same repository gives identical bytes on stdout and
  the same exit code from any checkout path. Paths in output are always
  repo-relative.
- **No network, no external processes.** The binary links no `net`,
  `os/exec`, `plugin` or `crypto/tls` package, and no linked third-party file
  calls `os.StartProcess` or `syscall.ForkExec`/`Exec`. The
  `binary-no-net-no-exec` rule in `scripts/check-architecture.sh` enforces
  this. The proof covers every release target (linux/amd64, linux/arm64,
  darwin/amd64, darwin/arm64, windows/amd64, windows/arm64), because the rule runs
  `go list -deps` once per GOOS/GOARCH, not only on the build host. The import
  half is exact; the process-spawn half is a source scan for qualified calls,
  so it is a strong guard, not a formal proof.
- **No writes.** gruntled opens the repository with `os.OpenRoot`, which also
  refuses paths that escape the repository, and hands the parsers only
  `root.FS()`, an `fs.FS` that has no write methods. A test runs `check` on a
  read-only tree and verifies nothing changed.
- **No cache, no telemetry.** Nothing is stored between runs and nothing is sent anywhere.

## Known limitations

- A diagnostic's identity includes its position, so a line shift above a
  reference changes it.
- Terragrunt stacks that have not been generated are invisible.
- Undeclared dependency labels are not reported.
- A config-unknown unit carries no dependency edges: a cycle through it is
  not reported and its own dependencies are not checked for GRT002.
- Symlink aliases are not canonicalised: an edge to a directory reached
  through a symlink (or a vendored or cached copy) is not matched to the unit,
  so a cycle through it is missed.
- `exclude {}` blocks are not modelled: an excluded unit still counts in
  GRT002 and GRT003.
- `dependencies { paths }` from includes are merged as a union (both shallow
  and deep merge), as Terragrunt does; `no_merge` includes contribute nothing.
- SARIF columns are byte columns although the run declares
  `unicodeCodePoints`: a line with non-ASCII characters before the anchor
  gets a column that is too large.
- SARIF has no `partialFingerprints`; GitHub's line hash identifies alerts,
  so editing the flagged line can reopen an alert as new.
- SARIF has no `relatedLocations`: cycle members and GRT001's target module
  appear only in the message.
