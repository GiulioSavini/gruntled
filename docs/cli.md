# gruntled CLI reference

gruntled reads a Terragrunt repository and reports `dependency.X.outputs.Y`
references that name an output the target module does not declare. It never
runs Terraform, Terragrunt or any other program.

## Usage

```
gruntled check [--format text|json] [path]
```

`path` defaults to `.`. Examples:

```
gruntled check
gruntled check live/
gruntled check live/ --format json
gruntled check --format=json -- -oddly-named-dir
```

- Flags may appear before or after the path. The Go standard library `flag`
  package stops at the first positional argument, so gruntled parses again
  after each positional.
- `--` ends flag parsing. Everything after it is a path, even if it starts with `-`.
- More than one path is a usage error (exit 2).
- `gruntled -h` lists the commands. `gruntled check -h` prints the usage text
  below and exits 0.

```
usage: gruntled check [--format text|json] [path]

Check the Terragrunt repository at path (default ".") for dependency
output references that name an output the target module does not declare.
Flags may appear before or after path; "--" ends flag parsing.

Flags:
  --format text|json   output format (default "text")

Exit codes:
  0  analysis completed, no error diagnostics
  1  analysis completed, at least one error diagnostic (GRT001, GRT100)
  2  usage error: unknown command or flag, invalid --format, more than one path
  3  analysis could not run: path missing, not a directory or unreadable, or an internal failure
```

## Exit codes

| Code | Meaning |
|------|---------|
| 0 | Analysis completed. No error diagnostic. |
| 1 | Analysis completed. At least one error diagnostic (`GRT001`, `GRT100`). |
| 2 | Usage error: no or unknown command, unknown flag, invalid `--format`, more than one path. |
| 3 | Analysis could not run: path missing, not a directory or unreadable, an internal failure, or a failed write to stdout. |

The same table as printed by `gruntled check -h`:

```
  0  analysis completed, no error diagnostics
  1  analysis completed, at least one error diagnostic (GRT001, GRT100)
  2  usage error: unknown command or flag, invalid --format, more than one path
  3  analysis could not run: path missing, not a directory or unreadable, or an internal failure
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

### Reserved codes

Every other code is reserved. GRT002 to GRT006 are planned for later
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
  darwin/amd64, darwin/arm64, windows/amd64), because the rule runs
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
- Undeclared dependency labels and dependency cycles are not reported in v0.1.
