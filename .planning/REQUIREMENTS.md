# Requirements: gruntled

**Defined:** 2026-09-29
**Core Value:** Tell the user, before they run anything slow, that `dependency.X.outputs.Y` does not exist in the module it points to.

v0.1 requirements (PARSE, GRAPH, DIAG-01..04, CLI, VALID, ARCH) are archived in
`milestones/v0.1-REQUIREMENTS.md`. The zero-false-positive, determinism, no-network and
read-only constraints in PROJECT.md apply to every requirement below.

## v0.2 Requirements

### More diagnostics

- [x] **MORE-01**: User sees `GRT002` when a `dependency` block's literal `config_path` resolves to a directory that contains no unit; a non-literal or unresolvable `config_path` stays silent
- [x] **MORE-02**: User sees `GRT003` once per dependency cycle, with cycle members listed in a deterministic order starting from the lexically smallest unit path
- [x] **MORE-06**: On the three-repo corpus, `GRT002` and `GRT003` report nothing on unmutated iso20022 and cds-snc/secret, report on denis256 (a deliberately broken suite) exactly the set an independent oracle derives, and catch every injected mutation (a `config_path` pointed at a missing directory, a back-edge that closes a cycle), recorded in `docs/validation.md`

### Integration

- [ ] **INT-01**: User runs `gruntled graph --json` and gets one deterministic document with a schema version, units, modules, dependency edges and unknown reasons, all paths repository-relative
- [ ] **INT-02**: User runs `gruntled check --format sarif` and gets a SARIF 2.1.0 document that `github/codeql-action/upload-sarif` accepts, with one rule per `GRT` code and repository-relative locations
- [ ] **INT-03**: User adds gruntled to `.pre-commit-config.yaml` with a single hook entry, served by a `.pre-commit-hooks.yaml` in this repository
- [ ] **INT-04**: User follows a documented GitHub Actions recipe (check plus SARIF upload) and a GitLab CI recipe; the GitHub recipe runs in this repository's own CI

### Release

- [ ] **REL-01**: User downloads a static binary for linux, darwin and windows on amd64 and arm64 from a tagged GitHub release, with a checksums file
- [ ] **REL-02**: User runs `gruntled --version` and gets the release version and commit, injected at build time

## Future Requirements

Deferred. Tracked, not in the current roadmap.

### Daemon (v0.3)

- **DAEMON-01**: `gruntled watch` keeps an in-memory index and reindexes only what changed on save
- **DAEMON-02**: incremental reindexing produces exactly what a full rescan would produce, verified by property-based testing
- **DAEMON-03**: the daemon writes a one-line status file readable from a shell prompt, tmux or an editor status bar
- **DAEMON-04**: `gruntled report` queries the running daemon over a unix socket
- **DAEMON-05**: starting the daemon while one is already running attaches to it rather than starting a second

### Blast radius (v0.3)

- **BLAST-01**: `gruntled blast <path>` reports Broken and Impacted as distinct sets
- **BLAST-02**: a unit is Impacted by a module change only when that change alters the module's `variable` or `output` surface

### More diagnostics

- **MORE-03**: `GRT004` — output removed from a module but still referenced downstream (needs a diff between two states; belongs with blast)
- **MORE-04**: `GRT005` — `inputs` key matching no `variable` in the module
- **MORE-05**: `GRT006` — `variable` without default that no unit sets

## Out of Scope

| Feature | Reason |
|---------|--------|
| `GRT005` / `GRT006` in v0.2 | Overlap `terragrunt hcl validate --inputs`, and `inputs` merged across includes carry false-positive risk |
| Package managers (Homebrew, apt, Scoop) | Release binaries first; add a channel only when someone asks |
| Container image | A static binary runs in any CI image already |
| Validating Terraform itself (provider schemas, resources, expressions) | A far larger problem already served by `tofu validate` and `tflint`. `.tf` files are read only for module surface |
| Importing Terragrunt as a Go library | Verified impossible: the only `ParsingContext` constructor needs `*venv.Venv` from `internal/venv`. Would also pull 672 modules and full cloud SDKs |
| Calling external binaries | Breaks the single-binary, no-external-process guarantee that makes air-gap and idempotency demonstrable |
| Network access at runtime | No remote source downloading, no telemetry, no update checks |
| Terragrunt functions that shell out, read env, decrypt or reach the network | Never implemented at all — the owning unit becomes `unknown` |
| `terragrunt.stack.hcl` | `stack generate` materialises ordinary units on disk, which the tree walk finds anyway |
| On-disk index cache | Schema versioning, invalidation and corruption traded against a sub-second startup |
| Desktop notifications | `notify-send` is absent on the target machine; PowerShell toasts break the no-external-process rule |
| LSP / editor integration | The status file is readable from a prompt, tmux or an editor without implementing a protocol |
| Planning or applying infrastructure | gruntled is strictly read-only |

## Traceability

Which phases cover which requirements. Updated during roadmap creation.

| Requirement | Phase | Status |
|-------------|-------|--------|
| MORE-01 | Phase 5 | Complete |
| MORE-02 | Phase 5 | Complete |
| MORE-06 | Phase 5 | Complete |
| INT-01 | Phase 6 | Pending |
| INT-02 | Phase 6 | Pending |
| INT-03 | Phase 7 | Pending |
| INT-04 | Phase 7 | Pending |
| REL-01 | Phase 7 | Pending |
| REL-02 | Phase 7 | Pending |

**Coverage:**
- v0.2 requirements: 9 total
- Mapped to phases: 9
- Unmapped: 0

---
*Requirements defined: 2026-09-29*
*Last updated: 2026-09-29 after starting milestone v0.2*
