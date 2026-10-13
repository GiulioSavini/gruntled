# gruntled CLI reference

gruntled reads a Terragrunt repository and reports `dependency.X.outputs.Y`
references that name an output the target module does not declare. It never
runs Terraform, Terragrunt or any other program.

## Usage

```
gruntled check [--format text|json|sarif] [path]
gruntled graph --json [path]
gruntled blast [--base dir] [--format text|json] [path]
gruntled watch [--poll] [--poll-interval d] [--debounce d] [--status-file p] [--print-status-path] [path]
gruntled report [--blast] [--format text|json|sarif] [path]
gruntled rebase [--expect N] [path]
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
gruntled blast --base ../base .
gruntled blast --base ../base --format json live/
gruntled watch live/
gruntled watch --print-status-path live/
gruntled report live/
gruntled report --format sarif live/ > gruntled.sarif
gruntled report --blast live/
gruntled rebase --expect 7 live/
```

- Flags may appear before or after the path. The Go standard library `flag`
  package stops at the first positional argument, so gruntled parses again
  after each positional.
- `--` ends flag parsing. Everything after it is a path, even if it starts with `-`.
- More than one path is a usage error (exit 2).
- `gruntled -h` lists the commands. `gruntled check -h`, `gruntled graph -h`,
  `gruntled blast -h`, `gruntled watch -h`, `gruntled report -h` and
  `gruntled rebase -h` print the usage texts below and exit 0.

```
usage: gruntled <command> [arguments]

Commands:
  check   check a Terragrunt repository for broken dependency output references
  graph   print the repository graph as JSON (--json)
  blast   report which units a change breaks (Broken) or puts at risk (Impacted) against --base
  watch   reindex on every save and keep a one-line status file outside the repository
  report  print the running watch daemon's result, like check, or its blast view (--blast)
  rebase  move the running watch daemon's blast baseline to its last result

Flags:
  --version   print the version and commit, then exit

Run "gruntled check -h", "gruntled graph -h", "gruntled blast -h",
"gruntled watch -h", "gruntled report -h" or "gruntled rebase -h" for
details.
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

`gruntled blast` answers "what does this change break, and what does it put
at risk?" by comparing the tree at `path` (the change) with a baseline tree
at `--base` (before the change). It runs the same analysis as `check` on
both trees and reports two disjoint, path-sorted lists:

- **Broken**: units whose findings are new in `path`. A finding is matched
  across the trees by code, unit, file and message, without line and column,
  so a pre-existing finding that only moved to another line is not new. A
  finding that has no unit (GRT100 on a file no unit reads) is listed under
  its file. A reference to an output that the baseline's module declared and
  the current module removed is reported as
  [GRT004](#grt004-dependency-output-removed-error-blast-only) instead of
  GRT001, at the same site.
- **Impacted**: units of `path` that the change reaches, each with its
  distance and one shortest path. An instantiating unit, one that uses a
  module whose declared variable or output names differ between the trees,
  is at distance 1. So is a unit whose module only changed a type-level
  fact: a new required variable, a variable type, a declared output type or
  an output's `sensitive` flag. A dependent of a reached unit is one further
  away. A dependent is reached only through a `dependency` block whose
  `enabled` is absent or literally `true` and whose `skip_outputs` is absent
  or literally `false`. Broken units are traversed, so their dependents are
  reached, but they are listed only under Broken. `--depth N` lists Impacted
  units up to distance N; `--depth 1` lists the instantiating units only.
  Impacted beyond distance 1 is a may-be-affected set: dependents read
  outputs, so a change to a module's variables reaches them only through the
  outputs of the units that use it.

gruntled never runs git or any other program. Check the baseline out
yourself, for example in CI:

```
git worktree add ../base origin/main
gruntled blast --base ../base .
```

Without `--base` there is nothing to compare against: every current finding
is Broken, Impacted is not computed, and the output says so ("no baseline").
A `--base` that cannot be opened is exit 3, never a silent fallback to no
baseline. `--format` is `text` or `json`; `sarif` is not supported for
`blast` and is a usage error.

```
usage: gruntled blast [--base dir] [--format text|json] [--depth N] [path]

Compare the Terragrunt repository at path (default ".") with the baseline
tree at --base. Broken lists the units with findings that are new in path
(a finding that only moved to another line is not new). A reference to an
output that the baseline module declared and path removed is reported as
GRT004 instead of GRT001. Impacted lists the units that use a module whose
variable or output names changed (distance 1) and the units that depend on
them through dependency blocks whose enabled and skip_outputs are absent or
literally true and false, each with its distance and one shortest path;
Broken units are not listed again. --depth N stops at distance N. Without
--base every finding is Broken and Impacted is not computed ("no
baseline"). gruntled never runs git: check the baseline out yourself, for
example with git worktree.
Flags may appear before or after path; "--" ends flag parsing.

Flags:
  --base dir           baseline tree to compare against (default: none)
  --format text|json   output format (default "text")
  --depth N            list Impacted units up to distance N (default: no limit)

Exit codes:
  0  comparison completed, no error diagnostic in Broken
  1  comparison completed, at least one error diagnostic in Broken
  2  usage error: unknown flag, invalid --format or --depth, more than one path
  3  comparison could not run: path or --base missing, not a directory or unreadable, an internal failure, or stdout write failed
```

`gruntled watch` keeps the analysis of `check` up to date while you edit.
It indexes the repository fully once, then after every save reindexes only
the files that changed and keeps a one-line status file outside the
repository that a shell prompt, tmux status bar or editor can read. It runs
until SIGINT or SIGTERM (Ctrl-C), then writes a final `stopped` line and
exits 0. Details are in [gruntled watch](#gruntled-watch).

```
usage: gruntled watch [--poll] [--poll-interval d] [--debounce d] [--status-file p] [--print-status-path] [path]

Watch the Terragrunt repository at path (default "."): index it fully, then
reindex only the changed files after each save, until interrupted
(SIGINT/SIGTERM). After every index a one-line status is written
atomically to a status file outside the repository; whenever the diagnostics
change, the full current set is printed to stdout in the same text format as
check.
Flags may appear before or after path; "--" ends flag parsing.

Flags:
  --poll                 use stat polling instead of the native watcher (always on windows)
  --poll-interval d      polling period (default 500ms)
  --debounce d           quiet period after the last change before reindexing (default 150ms)
  --status-file p        write the status line to p instead of the default path; p must be outside the repository
  --print-status-path    print the status file path for path, then exit

Exit codes:
  0  clean shutdown after SIGINT/SIGTERM, already watching (another daemon runs for path), or --print-status-path / -h
  2  usage error: unknown flag, invalid duration, more than one path, --status-file or runtime directory inside the repository
  3  watch could not start: path missing, not a directory or unreadable, no watcher, or the initial index failed
```

`gruntled report` prints what a running `gruntled watch` last published for
the same repository, without analysing anything itself. Details are in
[gruntled report](#gruntled-report).

```
usage: gruntled report [--blast] [--format text|json|sarif] [path]

Print the diagnostics of the watch daemon running for the Terragrunt
repository at path (default "."), in the same format and bytes as check.
The daemon is only read; report never starts one. It prints the last
published result: after a failed reindex that is the previous result,
with a warning on stderr.
With --blast it prints the daemon's blast view instead: the blast of the
current tree against the daemon's in-memory baseline, in the same format
as blast --base (text or json).
Flags may appear before or after path; "--" ends flag parsing.

Flags:
  --blast                    print the daemon's blast view (text or json)
  --format text|json|sarif   output format (default "text")

Exit codes:
  0  report printed, no error diagnostic
  1  report printed, at least one error diagnostic (GRT001-GRT003, GRT100; in Broken with --blast)
  2  usage error: unknown flag, invalid --format, sarif with --blast, --depth, more than one path
  3  no daemon is running for the repository, the daemon is still indexing, protocol mismatch, blast view over the 64 MiB cap or unavailable, or the daemon could not be queried
```

`gruntled rebase` moves the running daemon's blast baseline. Details are in
[gruntled rebase](#gruntled-rebase).

```
usage: gruntled rebase [--expect N] [path]

Move the baseline of the watch daemon running for the Terragrunt
repository at path (default ".") to its last published result, so that
report --blast compares against it from now on. Only the daemon's
in-memory baseline changes: nothing is written and nothing is reindexed.
It prints the number of error findings in Broken that the new baseline
accepts. rebase needs the daemon socket: it is not available on windows.
Flags may appear before or after path; "--" ends flag parsing.

Flags:
  --expect N   rebase only if the daemon is at generation N (an integer >= 1)

Exit codes:
  0  baseline moved
  2  usage error: unknown flag, invalid --expect, more than one path
  3  no daemon is running for the repository, the daemon refused (still indexing, failed reindex, generation mismatch, blast view unavailable), protocol mismatch, no usable answer, or not available on this platform
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

`gruntled blast` exits 1 only when Broken holds at least one error
diagnostic, like `check`. Every current rule is an error, but a Broken list
holding only warnings would exit 0. Pre-existing findings never fail `blast`.
Exit 3 also covers a `--base` directory that cannot be opened. As printed by
`gruntled blast -h`:

```
  0  comparison completed, no error diagnostic in Broken
  1  comparison completed, at least one error diagnostic in Broken
  2  usage error: unknown flag, invalid --format, more than one path
  3  comparison could not run: path or --base missing, not a directory or unreadable, an internal failure, or stdout write failed
```

CI can tell findings in the repository (1) apart from gruntled failing to run (3).

`gruntled watch` exits 0, 2 or 3 only: findings never end the daemon, they
are reported in the status file and on stdout. As printed by
`gruntled watch -h`:

```
  0  clean shutdown after SIGINT/SIGTERM, already watching (another daemon runs for path), or --print-status-path / -h
  2  usage error: unknown flag, invalid duration, more than one path, --status-file or runtime directory inside the repository
  3  watch could not start: path missing, not a directory or unreadable, no watcher, or the initial index failed
```

`gruntled report` exits 0 and 1 exactly as `check` would for the result it
prints; with `--blast`, 1 means an error diagnostic in Broken, as for
`blast`. Exit 3 means there is no result to print: no daemon runs for the
repository, it has not finished its first index, it speaks another protocol
version, the blast view is over the cap or could not be computed, or the
daemon could not be queried. As printed by `gruntled report -h`:

```
  0  report printed, no error diagnostic
  1  report printed, at least one error diagnostic (GRT001-GRT003, GRT100; in Broken with --blast)
  2  usage error: unknown flag, invalid --format, sarif with --blast, --depth, more than one path
  3  no daemon is running for the repository, the daemon is still indexing, protocol mismatch, blast view over the 64 MiB cap or unavailable, or the daemon could not be queried
```

`gruntled rebase` exits 0 when the baseline moved, 2 on a usage error and 3
otherwise: a refusal (nothing changed), no daemon, a protocol mismatch, no
usable answer (the outcome is unknown), or windows. As printed by
`gruntled rebase -h`:

```
  0  baseline moved
  2  usage error: unknown flag, invalid --expect, more than one path
  3  no daemon is running for the repository, the daemon refused (still indexing, failed reindex, generation mismatch, blast view unavailable), protocol mismatch, no usable answer, or not available on this platform
```

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

Control characters. Paths, messages and unit names come from the repository,
so before printing gruntled replaces everything a terminal could act on with a
visible escape (hex is lowercase):

| Input | Printed as |
|-------|------------|
| invalid UTF-8 byte | `\xNN` |
| C0 control (U+0000-U+001F: ESC, BEL, tab, newline...) and DEL (U+007F) | `\xNN` |
| C1 control (U+0080-U+009F) | `\uNNNN` |
| format character (Unicode Cf: bidi overrides and isolates, zero-width characters, BOM, soft hyphen, tags), U+2028, U+2029 | `\uNNNN`, or `\UNNNNNNNN` above U+FFFF |

A directory named `a<ESC>]0;x<BEL>b` prints as `a\x1b]0;x\x07b`, and every
diagnostic stays on one line. Names made of printable characters, non-ASCII
letters included, print unchanged. A backslash is not escaped, so a literal
`\x1b` in a name reads the same as an escaped ESC: text output is for people;
use `--format json` or `--format sarif` for exact bytes. The same escaping
applies to `gruntled watch` stdout, `gruntled report` text and
[blast text](#blast-text).

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
as text mode. DEL, C1 controls (U+0080-U+009F), format characters (Unicode
Cf) and U+2028/U+2029 are written as `\uXXXX` escapes (a surrogate pair above
U+FFFF), so the document is safe to print to a terminal and decodes to the
same values; [Graph JSON](#graph-json), [Blast JSON](#blast-json) and
[SARIF](#sarif) are escaped the same way.

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

### Blast text

`gruntled blast` (default `--format text`) writes to stdout only; there is no
stderr summary. The first line names the baseline as given to `--base`.
Each Broken subject is followed by its new findings in the `check` text
layout without the `(unit U)` suffix. Each Impacted unit carries its
distance. A unit at distance 1 uses the changed module; its line names the
module and its name changes, in the order removed variables, added
variables, removed outputs, added outputs, then its type-level reasons:
`required variable <name>`, `type variable <name>`, `type output <name>`,
`sensitive output <name>`. A unit at distance 2 or more does
not use the module: the change reached it from that module through
dependency blocks, and its line names the module and one shortest path, from
the unit back to the unit that instantiates the changed module:

```
baseline: ../base
Broken (1):
  live/app
    live/app/terragrunt.hcl:7:20: GRT004 dependency "db" output "id" was removed from module "modules/vpc" (target unit "live/db")
Impacted (3):
  live/cache (distance 1, module modules/vpc: +variable name, -output id, required variable name)
  live/db (distance 1, module modules/vpc: +variable name, -output id, required variable name)
  live/edge (distance 2, from module modules/vpc, path live/edge -> live/db)
Type changes (1):
  modules/vpc variable name: now required
```

The `Type changes (N):` section comes last and is printed only when at least
one changed module has a type-level change. It lists each change once per
module, so a type string never repeats per unit:

- `<module> variable <name>: now required`: a new variable without a
  `default`, or an existing one that lost its `default`.
- `<module> variable <name>: type <old> -> <new>`: the canonical type; a
  variable without `type` is `any`.
- `<module> output <name>: type <old> -> <new>`: only when both trees declare
  the output's `type`.
- `<module> output <name>: sensitive <old> -> <new>`: `false` or `true`; an
  absent `sensitive` is `false`.

Type-level changes are Impacted reasons only: they never create a
diagnostic, never make a unit Broken and never change the exit code.

A path runs through Broken units too; they are listed only under Broken. A
path of more than 6 units prints its first 4 units, then
`... (<k> more) ->`, then the instantiating unit, so every line stays short.
When several shortest paths exist, the one printed is the smallest by path
order at the first unit where they differ, read from the unit back. The JSON
`source` and `via` keys are the unambiguous form of a path: text joins hops
with ` -> `, which a directory name may itself contain.

Empty sections still print their header (`Broken (0):`), except Type
changes. Without `--base` the first line is `baseline: none (no baseline)`
and there is no Impacted or Type changes section.

The baseline label is escaped like every path and name, with the
[text control-character rules](#text-default).

The blast view of a watch daemon (`gruntled report --blast`, see
[gruntled report](#gruntled-report)) has the same layout. Its first line
names the daemon's baseline instead of a directory: the generation of the
published result the baseline was taken from, how it was taken
(`watch start` for the daemon's first index, `rebase` after
[gruntled rebase](#gruntled-rebase)), and the generation it is compared with:

```
baseline: daemon generation 1 (watch start), current generation 1
Broken (0):
Impacted (0):
```

### Blast JSON

`gruntled blast --format json` writes one JSON document to stdout. The
schema is versioned; this is version 2, with the same stability rules as
Graph JSON. Version 2 only adds keys to version 1. Readers must check
`version`; a v1 reader must reject version 2. New code values can appear
without a version bump: since GRT004, a removed output is reported as GRT004
where earlier versions said GRT001. Gate on severity, not on a code list.

| Key | Content |
|-----|---------|
| `version` | Schema version, `2`. |
| `kind` | Always `"blast"`. |
| `baseline` | `true` when `--base` was given, `false` otherwise. |
| `note` | `"no baseline"`. Present only when `baseline` is `false`. |
| `broken[]` | `unit` (the unit, or the file for a unit-less finding) and `findings[]`, each `code`, `severity`, `file`, `line`, `column`, `message`. When propagation passed through the unit, also `distance`, `source` and `via`. Sorted by `unit`. |
| `impacted[]` | `unit`; `module`, the changed module whose change reached this unit; for distance 1 it is also the unit's own module; `distance`; `source`, the unit that instantiates `module` at the start of the path (the unit itself at distance 1); `via`, the next unit on the path toward `source` (absent at distance 1). At distance 1 only, also `added_variables`, `removed_variables`, `added_outputs`, `removed_outputs` (sorted names); at distance 2 or more the change is the `changes[]` entry for `module`. Sorted by `unit`. Always `[]` without a baseline. |
| `changes[]` | One entry per changed module: `module`, `added_variables`, `removed_variables`, `added_outputs`, `removed_outputs`, `type_changes`. Sorted by `module`. Always `[]` without a baseline. |
| `changes[].type_changes[]` | The module's type-level changes, each `kind`, `name`, `old`, `new`, sorted by kind in the order below, then `name`. `[]` when none. `variable_required`: `old` is `absent` (a new variable) or `optional` (it lost its `default`), `new` is `required`. `variable_type`, `output_type`: canonical type strings. `output_sensitive`: `false` or `true`. Type strings appear only here, never in `impacted[]`. |
| `daemon_baseline` | Only in a watch daemon's blast view (`gruntled report --blast --format json`), right after `baseline`: `source` (`"watch start"` or `"rebase"`), `generation` (the baseline's generation) and `current_generation`. Never in `gruntled blast` output. |
| `summary` | `broken` and `impacted` counts. |

Type-level changes are Impacted reasons only: they never create a
diagnostic, never make a unit Broken and never change the exit code.

The `daemon_baseline` member does not change the schema version, which stays
`2`; a reader that ignores unknown keys reads a daemon view like any blast
document:

```json
{
  "version": 2,
  "kind": "blast",
  "baseline": true,
  "daemon_baseline": {
    "source": "watch start",
    "generation": 1,
    "current_generation": 3
  },
  "broken": [],
  ...
}
```

A unit never appears in both `broken` and `impacted`. Following `via` from
any `impacted[]` or `broken[]` entry reaches its `source` through entries of
the same document. Every list present prints as `[]` when empty, never
`null`. HTML characters are not escaped.

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

### GRT004: dependency output removed (error, blast only)

Only `gruntled blast --base` produces GRT004. It reports a
`dependency.X.outputs.Y` reference whose output `Y` the baseline's module
declared and the current module no longer declares. It replaces the GRT001
that `check` reports at the same reference site: never both codes at one
site, never an extra finding, and the exit code stays 1. A reference site is
GRT004 instead of GRT001 only when all four hold:

| # | Condition | When it fails |
|---|-----------|---------------|
| 1 | GRT001 fires at the site in the current tree (see the [DIAG-03 table](#how-grt001-treats-mock_outputs-enabled-and-skip_outputs-diag-03)) | no finding, or another code |
| 2 | The baseline has a reference with the same unit, dependency name and output (its line may differ) | GRT001: the reference was added in the same change |
| 3 | In the baseline the unit's dependency resolves to a unit whose module and module surface are known, with the same module path as in the current tree | GRT001: re-pointed to another module, or the module is unknown in the baseline |
| 4 | The baseline module declares the output | GRT001: it was never declared |

The dependency may be re-pointed to another unit of the same module; it is
still GRT004. `enabled`, `skip_outputs` and `mock_outputs` follow GRT001's
table in the current tree: `mock_outputs` never suppresses GRT004 and only
selects the same suffix.

Message:

```
dependency "<label>" output "<Y>" was removed from module "<module>" (target unit "<target>")
```

with the suffix `; mock_outputs supplies it, so apply would silently use the mock value`
under exactly GRT001's condition. `check`, `report`, `graph`, the watch
status file and SARIF never show GRT004; they keep reporting GRT001 at that
site.

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

Every other code is reserved. GRT005 and GRT006 are planned for later
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

## gruntled watch

`gruntled watch [path]` is a foreground daemon. It never forks or
daemonizes itself; run it in a terminal, a tmux pane, in the background or
under a service manager (see [Running in the background](#running-in-the-background)).

### Flags

| Flag | Default | Meaning |
|------|---------|---------|
| `--poll` | off | Use stat polling instead of the native watcher. Always on for windows. |
| `--poll-interval d` | `500ms` | Period of the stat scan when polling. Must be positive. |
| `--debounce d` | `150ms` | Trailing quiet period: reindex once no change arrived for this long (a burst is still flushed after at most 1 s). Must be positive. |
| `--status-file p` | see below | Write the status line to `p`. A relative `p` is made absolute first. `p` must be outside the repository, also after resolving symlinks, or the command exits 2. |
| `--print-status-path` | off | Print the status file path for `path` to stdout and exit 0, without starting the daemon or creating the file. |

Durations use Go syntax (`250ms`, `2s`).

### Behaviour

- **Start.** The watcher is set up first, then the repository is indexed in
  full, so a change made during the initial index is not lost. A failed
  initial index is exit 3.
- **Incremental.** Each debounced batch of changed paths is handed to the
  parse cache, which drops only the entries under those paths; the rest of
  the repository is served from memory. The result is by construction the
  same as a fresh `gruntled check`. A file reached through an in-repo
  symlink is re-read when its target changes, even though only the target
  is reported as changed.
- **Debounce.** 150 ms after the last change (trailing), capped at 1 s from
  the first change of a burst. An editor's save-and-rename sequence gives one
  reindex.
- **Ignored paths.** Anything under `.git`, `.terraform` or
  `.terragrunt-cache`, and editor swap, backup and temp files (`*~`,
  `*.swp`/`.swo`/`.swn`/`.swx`, `.#*`, `#*#`, `*.tmp`, JetBrains
  `___jb_tmp___`/`___jb_old___`, `.DS_Store`) never trigger a reindex.
  The editor patterns apply to files only: a directory named like one
  (`2024`, `x.tmp`, `bak~`) is watched and indexed like any other. Symlinks
  are pattern-ignored only for Emacs lock files (`.#name`); a symlink named
  like any other pattern triggers a reindex.
- **New and deleted directories** are picked up: a new unit directory is
  watched and indexed, a deleted one disappears from the results.
- **Backends.** On linux and darwin the native watcher (inotify, kqueue) is
  used, with a stat re-scan every 30 s as a safety net for lost kernel
  events; an event queue overflow triggers a full revalidation. If the OS
  watch limit is exhausted, gruntled prints
  `gruntled: native watcher unavailable (...); falling back to polling` on
  stderr and polls. On windows, and with `--poll`, it stat-polls every
  `--poll-interval`. Polling compares size and modification time, so an edit
  that keeps both identical is invisible to it.
- **stderr** carries one banner line,
  `gruntled: watching <root> (fsnotify|poll); status: <status file>`, the
  fallback notice and hints, a failed reindex, and the first failure to write
  the status file (later ones are not repeated; a write failure never stops
  the daemon).

### stdout

After an index, if the set of diagnostics differs from the last one printed,
gruntled prints the new set in the same text format as `check` (one
`path:line:col: CODE message` line per diagnostic, repo-relative paths). The
first set is printed if it is non-empty. A save that does not change the
diagnostics prints nothing, and a clean repository prints nothing at all.
There is no summary line and no JSON or SARIF mode.

### Status file

The status file holds exactly one line, newline-terminated. The stamp is
local time, `HH:MM:SS`, of the moment the line was written:

```
gruntled: indexing... @ 14:02:09
gruntled: ok @ 14:02:11
gruntled: 1 error (GRT001×1) @ 14:02:11
gruntled: 3 errors (GRT001×2 GRT003×1) @ 14:02:11
gruntled: failed (<reason>) @ 14:02:11
gruntled: stopped @ 14:05:40
```

- Only error diagnostics are counted, grouped per code in ascending code
  order; `×` is U+00D7. A failure reason has whitespace collapsed and is cut
  to 120 characters.
- `indexing...` is written while the initial index runs, `stopped` on a
  clean shutdown. A failed reindex writes `failed (...)` and the daemon
  keeps running; the next successful index replaces it.
- **Atomic.** Every update writes a temporary file in the same directory
  and renames it over the status file, so a reader sees the previous line or
  the new one, never a partial line.
- **Permissions.** The directory is created with mode 0700 and the file with
  0600. An existing directory that group or others can access is refused
  (the write fails and is reported once on stderr; not checked on windows).
- **Path.** By default `<base>/gruntled/<hash12>/status`, where
  - `base` is `$XDG_RUNTIME_DIR` on linux when it is set and absolute,
    otherwise `os.UserCacheDir` (`$XDG_CACHE_HOME` or `~/.cache` on linux,
    `~/Library/Caches` on darwin, `%LocalAppData%` on windows), otherwise
    `os.TempDir`;
  - `hash12` is the first 12 hex characters of the SHA-256 of the absolute,
    symlink-resolved repository path. There is no case folding: on a
    case-insensitive filesystem two spellings of the same directory that
    symlink resolution does not canonicalise get different status files.
- The status file is never inside the repository: a `--status-file` (or a
  default path) that lands inside it, also through a symlink, is exit 2.
- The same holds for the per-repository runtime directory (lock, socket,
  report file) chosen from `$XDG_RUNTIME_DIR` or the user cache directory:
  if it lies inside the repository, watch exits 2 before creating anything,
  with `gruntled: runtime directory <dir> is inside the repository <root>; ...`
  (or the status file message above, when the default status file lands
  there too and no `--status-file` moves it out). Inside-ness is decided by file identity, so on a
  case-insensitive filesystem a case variant of the repository path counts as
  inside.
- `gruntled watch --print-status-path [path]` prints the path, so a prompt
  or status bar does not have to recompute it:

```
# shell prompt / tmux status-right
cat "$(gruntled watch --print-status-path .)" 2>/dev/null
```

- A daemon that crashed or was killed with SIGKILL leaves its last line
  behind; nothing marks it stale yet.

### Running in the background

There is no daemonize flag. Use the shell, tmux or a service manager:

```
gruntled watch ~/live > /dev/null &        # background job, status file only
tmux new-window -d 'gruntled watch ~/live' # stdout visible in a tmux window
```

A systemd user unit (`~/.config/systemd/user/gruntled-live.service`):

```
[Unit]
Description=gruntled watch for ~/live

[Service]
ExecStart=%h/go/bin/gruntled watch %h/live
Restart=on-failure

[Install]
WantedBy=default.target
```

`systemctl --user enable --now gruntled-live` starts it; `systemctl --user
stop` sends SIGTERM, which writes `stopped` and exits 0.

## gruntled report

`gruntled report [--blast] [--format text|json|sarif] [path]` prints the result of the
`gruntled watch` daemon running for the repository at `path`, in the same
formats and with the same bytes as `gruntled check`: stdout is identical and,
for `text`, so is the summary on stderr. The daemon renders every format once
per index with the same code `check` uses; `report` only copies those bytes.

- **Read-only.** `report` and `report --blast` only read the daemon:
  rebase is the only operation that changes the daemon, and only its in-memory baseline.
  `report` never starts a daemon or creates a directory, file or socket. With
  no daemon it prints `gruntled: no watch daemon is running for <root>` and
  exits 3.
- **Last published result.** `report` shows the result of the last completed
  index. Before the first index finishes it exits 3 with
  `gruntled: the daemon is still indexing; try again`. After a failed
  reindex it prints the previous result and, on stderr,
  `gruntled: last reindex failed: <reason>; showing the previous result`;
  the exit code follows the result printed.
- **Blast view.** `report --blast` prints the daemon's blast view in `text`
  or `json` (`--format sarif` is a usage error, exit 2): the blast of the
  last published result against the daemon's baseline, rendered at every
  index with the same comparison and the same
  [text](#blast-text) and [JSON](#blast-json) layouts as
  `gruntled blast --base`. Only the first line
  (`baseline: daemon generation G0 (source), current generation G`) and the
  JSON `daemon_baseline` member differ. The
  baseline is the daemon's first index until
  [gruntled rebase](#gruntled-rebase) moves it. stdout is exactly the view,
  there is no summary on stderr, and the exit code is 1 when Broken holds
  an error diagnostic. A view the daemon could not compute exits 3 with
  `gruntled: the daemon could not compute the blast view: <reason>`.
  `--depth` belongs to `gruntled blast --base` and is a usage error here.
- **linux and darwin.** `report` asks the daemon over a unix socket in the
  repository's runtime directory (next to the status file), with a 2 s
  timeout. The protocol is version 2; a daemon built with another protocol
  version is refused with
  `gruntled: daemon runs protocol vX, this gruntled speaks vY; restart the daemon`.
- **windows.** There is no socket: the daemon rewrites a report file in the
  runtime directory after each index, and `report` reads it only while the
  daemon holds its lock, so a file left behind by a dead daemon is reported
  as no daemon. There is no live query and no version negotiation beyond the
  version recorded in the file. `report --blast` reads the view from the
  same file; rebase is not available on windows.
- **Response cap.** One daemon response (socket reply or report file) is
  capped at 64 MiB. The plain report response carries every format at once
  (text, summary, JSON and SARIF, base64-encoded), so a repository whose
  combined renders exceed about 48 MiB gets a transport error from `report`
  (exit 3). `check` has no such cap. report never carries the blast view: the
  blast view has its own 64 MiB cap, decided once per index. Over it,
  `report --blast` exits 3 with
  `gruntled: the blast view is <n> bytes, over the 64 MiB response cap; run gruntled blast --base <dir>`.
  On windows the report file drops an oversize blast view first, so plain
  `report` keeps working.

## gruntled rebase

`gruntled rebase [--expect N] [path]` makes the last published result of
the `gruntled watch` daemon running for the repository at `path` its new
blast baseline, so `report --blast` shows only what changes from now on. On
success it prints

```
gruntled: baseline moved to generation N; M error findings in Broken accepted
```

and exits 0. M is the number of error findings the blast view held in
Broken: they are now part of the baseline and no longer reported, so read
`report --blast` before you rebase.

- **In memory only.** The baseline lives in the daemon's memory. rebase
  writes no file (not the status file, not the report file), reindexes
  nothing and does not change the generation. A restarted daemon takes its
  first index as the baseline again.
- **--expect N.** The daemon refuses unless its published result is
  generation N, the one shown on the first line of `report --blast`, so a
  save that landed after you read the view is never accepted unseen. A
  mismatch prints
  `gruntled: the daemon is at generation M, not N; nothing changed` and exits
  3. N is a decimal integer >= 1 without sign or leading zero; anything else
  is a usage error (exit 2).
- **Refusals.** Every refusal exits 3 and changes nothing: the daemon is
  still indexing, the last reindex failed (fix the tree and save, then
  rebase), or the blast view could not be computed.
- **Timeout.** rebase waits 10 s for the answer. When the request was sent
  but no usable answer came back (timeout, a closed connection, an
  unreadable reply) the outcome is unknown, and rebase prints
  `gruntled: no usable answer from the daemon; the baseline may or may not have moved; run gruntled report --blast to check`
  and exits 3 (on a timeout the message adds `(no answer within 10s)`).
- **Trust boundary.** rebase talks to the daemon over its unix socket, which
  is mode 0600 in a runtime directory that is mode 0700 and owned by the
  user; both the daemon and the client check the directory owner and mode,
  so only the daemon owner's user can connect (and root). The socket peer's
  credentials are not checked.
- **windows.** rebase is not available on windows: it needs the daemon
  socket. It exits 3 with
  `gruntled: rebase needs the daemon socket, which this platform does not have; restart gruntled watch to reset the baseline`
  before touching anything.
- **GRT004 and GRT001.** The baseline is a full `check` result, so a
  removed output keeps its GRT004 in the blast view until you rebase. Note that
  after a rebase, a reference that still names a removed output leaves Broken, while report keeps its GRT001.

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
  so it is a strong guard, not a formal proof. The local unix socket used by
  `watch` and `report` on linux and darwin is opened with the standard
  `syscall` package, not `net`, so the proof holds with it linked in.
- **No writes.** gruntled opens the repository with `os.OpenRoot`, which also
  refuses paths that escape the repository, and hands the parsers only
  `root.FS()`, an `fs.FS` that has no write methods. A test runs `check` on a
  read-only tree and verifies nothing changed. `report`, `report --blast` and
  `rebase` write nothing anywhere; rebase changes only the daemon's memory.
- **No cache, no telemetry.** Nothing is stored between runs and nothing is sent anywhere.
  `gruntled watch` keeps its parse cache and its blast baseline (one
  `check` result) in memory only; what it writes
  (status file, lock file, unix socket on linux and darwin, report file on
  windows) lives in the per-repository runtime directory, always outside the
  repository. On windows that directory is under `%LocalAppData%` and relies
  on its per-user ACL: gruntled does not verify the directory's owner or ACL
  there.

## Known limitations

- The daemon's blast baseline is its first index: a tree that was already
  dirty when `watch` started counts as the baseline. The baseline is lost
  when the daemon restarts. `gruntled rebase` repairs both.

- A diagnostic's identity includes its position, so a line shift above a
  reference changes it. `blast` is the exception: it matches findings without
  line and column, so two findings that differ only by position collapse
  into one, and only a finding with a new code, unit, file or message is
  Broken.
- `blast` type-level reasons read `default` presence only (never its
  value), treat an absent variable `type` as `any`, compare an output `type`
  only when both sides declare it, and stay silent on override files,
  conflicting duplicate declarations and non-literal values. They give no
  compatible or breaking verdict. A new required variable is an Impacted
  reason, never Broken, because inputs can come from includes, tfvars or the
  environment. A changed default value, description or output value is
  invisible.
- `blast` Impacted is a lower bound: a non-literal `enabled` or
  `skip_outputs` and a `dependencies { paths }` entry stop propagation, and
  propagation is not gated on which outputs a dependent reads.
- `blast` does not diff a module that is new, deleted, or has an unknown
  surface in either tree; consumers of a deleted local module show up only
  through other findings.
- `blast` attributes a unit-less finding (GRT100 in a file no unit reads) to
  its file, not to a unit.
- `blast` reports GRT004 only under all four of its conditions. A reference
  added in the same change, a dependency re-pointed to another module, and a
  module unknown in either tree stay GRT001 (or silent, as in `check`).
- GRT004 has no SARIF rule: `blast` has no SARIF output, and `check` never
  emits GRT004.
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
- `watch`: a daemon that crashed or was killed with SIGKILL leaves its last
  status line behind, which then looks current. There is no liveness check
  yet.
- `watch --poll` (and windows) compares size and modification time only: an
  edit that keeps both identical is not seen until the next change.
- `watch` under WSL on a Windows drive (`/mnt/c/...`): inotify does not see
  edits made from the Windows side. gruntled prints a hint; use `--poll`.
- `report` on windows has no live query: it reads the report file the daemon
  rewrites after each index, so it shows the last published result and
  cannot ask the daemon anything else.
- `watch` on darwin with the native watcher: while a directory holds a
  dangling symlink (an Emacs lock file `.#name`, or a broken link committed
  to the repository), kqueue directory scanning stops at that link. Files
  that sort after it are not seen when created, and a file replaced by a
  rename-on-save editor (vim's default) is not watched again, so its later
  saves are seen only by the 30 s safety-net re-scan. Results are late by
  at most 30 s, never wrong for longer; `check` and `--poll` are not
  affected. Upstream: https://github.com/fsnotify/fsnotify/issues/787.
