# Requirements: gruntled

**Defined:** 2026-09-01
**Core Value:** Tell the user, before they run anything slow, that `dependency.X.outputs.Y` does not exist in the module it points to.

## v1 Requirements

v1 is a **falsifiable experiment**. Scope is deliberately one diagnostic plus the
machinery to prove it correct on real code.

### Parsing

- [ ] **PARSE-01**: gruntled decodes `terragrunt.hcl` structurally, reading only `include`, `terraform.source` and `dependency` blocks, without evaluating expression values
- [ ] **PARSE-02**: gruntled evaluates the six pure path functions (`find_in_parent_folders`, `path_relative_to_include`, `path_relative_from_include`, `get_terragrunt_dir`, `get_parent_terragrunt_dir`, `get_original_terragrunt_dir`)
- [ ] **PARSE-03**: gruntled resolves `include` blocks, including nested includes and merge strategy, parsing each included file exactly once and sharing the result across every unit that includes it
- [ ] **PARSE-04**: gruntled marks a unit `unknown` when it encounters any construct it cannot resolve offline, and never reports a diagnostic for an `unknown` unit
- [ ] **PARSE-05**: gruntled reports a diagnostic instead of crashing when a file contains invalid HCL, including a file saved mid-edit
- [ ] **PARSE-06**: gruntled skips `.terragrunt-cache`, `.terraform`, vendored module directories and symlinks when walking a repository

### Graph

- [ ] **GRAPH-01**: gruntled resolves a unit to its module via `terraform.source` when present
- [ ] **GRAPH-02**: gruntled resolves a unit to its module as the unit's own directory when `terraform.source` is absent
- [ ] **GRAPH-03**: gruntled classifies a module source as remote without downloading it, and marks the owning unit `unknown`
- [ ] **GRAPH-04**: gruntled resolves a `dependency` block through both hops — from the dependency to the target unit, and from that unit to the module whose outputs it exposes
- [ ] **GRAPH-05**: gruntled extracts the public surface of a module — the names of its `variable` and `output` blocks — from its `.tf` and `.tf.json` files

### Diagnostics

- [ ] **DIAG-01**: gruntled reports `GRT001` when a `dependency.X.outputs.Y` reference names an output the target module does not declare
- [ ] **DIAG-02**: gruntled reports `GRT100` for HCL syntax errors, with file and line
- [ ] **DIAG-03**: gruntled applies a documented, tested rule for whether `mock_outputs` on a dependency suppresses `GRT001`
- [ ] **DIAG-04**: every diagnostic carries a repository-relative path — never an absolute path — plus line, column and a stable code

### CLI

- [ ] **CLI-01**: `gruntled check [path]` analyses a repository and prints diagnostics in a human-readable form
- [ ] **CLI-02**: `gruntled check` exits 0 when clean and non-zero when it reports an error, with documented exit codes
- [ ] **CLI-03**: `gruntled check` produces byte-identical output, in identical order, for identical input
- [ ] **CLI-04**: gruntled makes no network calls and starts no external processes at any point
- [ ] **CLI-05**: gruntled writes nothing inside the repository it analyses

### Validation

- [ ] **VALID-01**: a generator produces synthetic Terragrunt repositories from a spec — N units, nested includes, dependency chains — used for both golden tests and benchmarks
- [ ] **VALID-02**: golden tests cover the fixture repositories, asserting the exact expected diagnostic set
- [ ] **VALID-03**: gruntled reports zero diagnostics on the unmutated primary corpus (`guidance-for-iso20022-messaging-workflows-on-aws`)
- [ ] **VALID-04**: gruntled reports every reference broken by a deliberate output rename or deletion injected into the corpus
- [ ] **VALID-05**: `terragrunt hcl validate` is confirmed not to report those injected mutations, evidencing the gap
- [ ] **VALID-06**: a reproducible benchmark shows `gruntled check` faster than `terragrunt hcl validate` on the same repository

### Architecture

- [x] **ARCH-01**: the domain layer contains no import of HCL, the filesystem or any infrastructure library, enforced by an automated check
- [x] **ARCH-02**: analyzers and graph queries are pure functions over domain types, testable without a filesystem

## v2 Requirements

Deferred. Tracked, not in the current roadmap.

### Daemon

- **DAEMON-01**: `gruntled watch` keeps an in-memory index and reindexes only what changed on save
- **DAEMON-02**: incremental reindexing produces exactly what a full rescan would produce, verified by property-based testing
- **DAEMON-03**: the daemon writes a one-line status file readable from a shell prompt, tmux or an editor status bar
- **DAEMON-04**: `gruntled report` queries the running daemon over a unix socket
- **DAEMON-05**: starting the daemon while one is already running attaches to it rather than starting a second

### Blast radius

- **BLAST-01**: `gruntled blast <path>` reports Broken and Impacted as distinct sets
- **BLAST-02**: a unit is Impacted by a module change only when that change alters the module's `variable` or `output` surface

### More diagnostics

- **MORE-01**: `GRT002` — `config_path` pointing at no unit
- **MORE-02**: `GRT003` — dependency cycle between units
- **MORE-03**: `GRT004` — output removed from a module but still referenced downstream
- **MORE-04**: `GRT005` — `inputs` key matching no `variable` in the module
- **MORE-05**: `GRT006` — `variable` without default that no unit sets

### Integration

- **INT-01**: `gruntled graph --json` exports the graph as a reusable document
- **INT-02**: SARIF output
- **INT-03**: pre-commit hook and CI recipes

## Out of Scope

| Feature | Reason |
|---------|--------|
| Validating Terraform itself (provider schemas, resources, expressions) | A far larger problem already served by `tofu validate` and `tflint`. `.tf` files are read only for module surface |
| Importing Terragrunt as a Go library | Verified impossible: the only `ParsingContext` constructor needs `*venv.Venv` from `internal/venv`. Would also pull 672 modules and full cloud SDKs |
| Calling external binaries | Breaks the single-binary, no-external-process guarantee that makes air-gap and idempotency demonstrable |
| Network access at runtime | No remote source downloading, no telemetry, no update checks |
| Terragrunt functions that shell out, read env, decrypt or reach the network | Never implemented at all — the owning unit becomes `unknown`. Cheaper and safer than implementing then disabling |
| `terragrunt.stack.hcl` | `stack generate` materialises ordinary units on disk, which the tree walk finds anyway. Only ungenerated stacks are invisible — a documented absence, not a misreport |
| On-disk index cache | Schema versioning, invalidation and corruption traded against a sub-second startup. Add only if startup becomes slow |
| Desktop notifications | `notify-send` is absent on the target machine; PowerShell toasts cost ~1s and break the no-external-process rule |
| LSP / editor integration | The status file is readable from a prompt, tmux or an editor without implementing a protocol |
| Planning or applying infrastructure | gruntled is strictly read-only |

## Traceability

| Requirement | Phase | Status |
|-------------|-------|--------|
| PARSE-01 | Phase 2 | Pending |
| PARSE-02 | Phase 2 | Pending |
| PARSE-03 | Phase 2 | Pending |
| PARSE-04 | Phase 2 | Pending |
| PARSE-05 | Phase 2 | Pending |
| PARSE-06 | Phase 2 | Pending |
| GRAPH-01 | Phase 2 | Pending |
| GRAPH-02 | Phase 2 | Pending |
| GRAPH-03 | Phase 2 | Pending |
| GRAPH-04 | Phase 2 | Pending |
| GRAPH-05 | Phase 2 | Pending |
| DIAG-01 | Phase 3 | Pending |
| DIAG-02 | Phase 3 | Pending |
| DIAG-03 | Phase 3 | Pending |
| DIAG-04 | Phase 3 | Pending |
| CLI-01 | Phase 3 | Pending |
| CLI-02 | Phase 3 | Pending |
| CLI-03 | Phase 3 | Pending |
| CLI-04 | Phase 3 | Pending |
| CLI-05 | Phase 3 | Pending |
| VALID-01 | Phase 1 | Pending |
| VALID-02 | Phase 4 | Pending |
| VALID-03 | Phase 4 | Pending |
| VALID-04 | Phase 4 | Pending |
| VALID-05 | Phase 4 | Pending |
| VALID-06 | Phase 4 | Pending |
| ARCH-01 | Phase 1 | Complete |
| ARCH-02 | Phase 1 | Complete |

**Coverage:**
- v1 requirements: 28 total (corrected from initial count of 27 — 28 unique requirement IDs are listed above)
- Mapped to phases: 28
- Unmapped: 0 ✓

---
*Requirements defined: 2026-09-01*
