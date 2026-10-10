# Requirements: gruntled

**Defined:** 2026-10-10
**Core Value:** Tell the user, before they run anything slow, that `dependency.X.outputs.Y` does not exist in the module it points to.

## v0.4 Requirements

Milestone v0.4 Blast-aware Diagnostics. Each maps to one roadmap phase.

### Terms

- **Instantiating unit**: a unit whose resolved module in the current tree is the changed module (the v0.3 Impacted set).
- **Dependent**: a unit with a `dependency` block edge to another unit.
- **Seeds**: the instantiating units of every module with a name-level or type-level surface change. Transitive impact walks dependents outward from the seeds.

### Removed outputs

- [ ] **MORE-03**: In `blast --base` and `report --blast`, `GRT004` fires at exactly the reference sites where the current tree raises `GRT001`, and replaces each of them (one finding per site, never both codes at one site), when the baseline had a reference with the same (unit, dependency name, output) (position-free) and both trees resolve the dependency to the same module path, known on both sides, whose baseline surface declares the output. It only reclassifies an existing `GRT001`, so it can never add a finding, and it shares `GRT001`'s decision table (`mock_outputs` never suppresses; `skip_outputs`/`enabled`/unknown keep it silent). Severity error, exit 1 in `blast` and `report --blast`. The message names the output, the module and the target unit, carries no consumer list, and uses repo-relative paths.
- [ ] **MORE-07**: `check` (text/json/sarif), plain `report`, `graph --json` and the status file never emit `GRT004` or type facts and stay byte-identical to v0.3.0 (pinned goldens).

### Transitive impact

- [ ] **BLAST-03**: Impacted includes the dependents reachable from the seeds by reverse `dependency` block edges whose `enabled` is literally true or absent and whose `skip_outputs` is literally false or absent (the same facts as `GRT001` rows 2-3), and whose target is a unit of the current graph. Any non-literal value stops propagation (a documented lower bound). `dependencies { paths }` edges do not propagate. Cycles, self-loops and diamonds terminate; Broken units are traversed but not listed as Impacted.
- [ ] **BLAST-04**: Instantiating units have distance 1; a dependent k reverse edges from the nearest seed has distance 1+k. Each Impacted unit shows its distance and one shortest path: the one whose unit sequence, read from the impacted unit back to its seed, is smallest under `RepoPath.Compare` at the first differing hop. Independent of map and input order, proven by a brute-force oracle property test over random graphs with cycles, self-loops, diamonds and shuffled input. Blast JSON schema version 2, additive fields only.
- [ ] **BLAST-05**: `blast --depth N` limits propagation; `--depth 1` limits Impacted to the instantiating units and, with only name-level changes, equals the v0.3 set. N is an integer >= 1, otherwise exit 2. Default: unlimited.

### Type-level surface

- [ ] **BLAST-06**: A variable that is new without a `default`, or that loses its `default`, makes the module's instantiating units Impacted with a "now required" reason. Required = no `default` attribute present in the merged declaration; `default = null` is a default; `nullable`, `validation`, `ephemeral` and `deprecated` never count.
- [ ] **BLAST-07**: A variable whose `type` constraint changes makes instantiating units Impacted with a "type changed" reason. Constraints are compared structurally in canonical form: formatting, attribute order and `optional()` defaults never count; absent `type` equals `any`. Type facts are extracted only from files that passed the existing size and depth pre-scan; the canonical renderer is iterative or depth-capped (over the cap or an unrecognised cty kind is unknown, never a panic), with a fuzz test in CI.
- [ ] **BLAST-08**: An output whose declared `type` changes (only when both sides declare a parseable type, never inferred from `value`), or whose `sensitive` flips (absent = literal false; non-literal is unknown and silent), makes instantiating units Impacted with that reason.
- [ ] **BLAST-09**: Type-level reasons are Impacted facts only: they never create a diagnostic, never make a unit Broken and never change the exit code. With `--depth 1`, every v0.3 blast golden produces the same Broken and Impacted sets and the same exit code; text differs only by the distance/path tokens, JSON only by `schema_version` 2 and the additive fields.
- [ ] **BLAST-10**: Each type-level fact (required, variable type, output type, output sensitive) is per name and is unknown when the name is declared in more than one kept file, or in any Terraform override file (`override.tf`, `*_override.tf` and their `.tf.json`/`.tofu`/`.tofu.json` forms), with differing facts, or is non-literal or unparseable. Unknown on either side reports nothing. Names stay known, so `GRT001` and name-level Impacted are unchanged.
- [ ] **BLAST-11**: `blast` and `report --blast` memory and output size are O(units + edges + surface text). Each type-level change is rendered once per module. The per-unit path representation is linear (JSON: distance, source, `via` = predecessor; text: full path up to a fixed hop count with deterministic elision). A test on a synthetic 5,000-unit linear chain and a 4 MiB type expression bounds RSS and output bytes.

### Daemon blast

- [ ] **DAEMON-07**: `gruntled report --blast [--format text|json]` prints the daemon's last published blast view against its baseline (the first successful index), showing the baseline and current generations. Exit 0/1 as `blast` (1 iff Broken has an error), 2 for `--format sarif` or `--depth`, 3 as `report`. On windows it reads the view from the report dump. After a failed reindex it prints the previous view with the existing stderr warning. A property test proves it equals `blast --base <copy of the tree at baseline time>`, comparing bytes after replacing only the baseline label line (text) and the baseline object (JSON).
- [ ] **DAEMON-08**: `gruntled rebase [--expect N]` sends one fixed-shape request (op plus an optional numeric expected generation; `maxRequest` stays 4 KiB) over the same 0600/0700 socket; the trust boundary is the daemon owner's uid, documented. It changes only in-memory state, never writes any file (repository, runtime dir, status file) and never starts a daemon. The swap is serialised with publish and installs only the last published report. It refuses with exit 3 while indexing or failed, or when `--expect N` differs. It prints the new generation and the number of error findings in Broken at that moment. A client timeout says the outcome is unknown. On windows it exits 3 as unsupported. Docs that call `report` read-only are updated.
- [ ] **DAEMON-09**: The IPC protocol version is bumped to 2; a client/daemon version mismatch is an explicit error (exit 3), never an empty "nothing impacted" answer. Compatibility matrix test: fake v1 daemon × v2 client, v2 daemon × v1 request, v1 report dump. Plain `report` bytes are unchanged.
- [ ] **DAEMON-10**: The baseline keeps only the `checking.Report` (never a loader, `fs.FS` or parse cache). The `report` op response does not carry blast bytes. A blast response over 64 MiB is an explicit exit-3 error naming the cap. Memory is measured on `synthrepo` at 65/500/5,000 units and recorded.

### Security

- [ ] **SEC-01**: Every string added to text output in v0.4 passes through `presenter.escapeTerm`, and every JSON document keeps the whole-buffer `escapeJSON` pass. The canonical type renderer quotes names with `strconv.Quote`. The phase-12 e2e escape test is extended to `blast` and `report --blast` with a module whose variable name, output name and object attribute name contain C0, C1, U+202E and invalid UTF-8.

### Proof

- [ ] **REL-03**: On each corpus: `check` is byte-identical to v0.3.0; `blast --base <same tree>` has empty Broken and Impacted; reformat-only mutations (whitespace, comments, attribute reorder, `optional()` default, `.tf`↔`.tf.json`, output moved to another file, `deprecated` added) give empty Impacted; the mutation set {referenced output removed → `GRT004` at every site and no `GRT001`, unreferenced output removed, required variable added, default removed, variable type changed, output type declared and changed, `sensitive` flipped, propagation to depth ≥ 2} is caught 100%. The six-target no-net/no-exec proof runs in the first plan that imports `ext/typeexpr`.

## Future Requirements

Deferred. Tracked, not in the current roadmap.

- **MORE-04**: `GRT005` — `inputs` key matching no `variable` (include-merge false-positive risk)
- **MORE-05**: `GRT006` — `variable` without default that no unit sets
- `watch --base <dir>` to seed the daemon baseline from another tree (dirty-start baseline)
- `blast --format sarif` (GRT004 as a PR annotation)
- `report` / `rebase` over AF_UNIX on windows (needs a scoped `net` exception to the proof)
- Reference-gated propagation (needs whole-object `dependency.X.outputs` use recorded)
- Per-hop `file:line` in Impacted paths

## Out of Scope

| Feature | Reason |
|---------|--------|
| Breaking vs compatible verdict on type changes | Terraform converts values implicitly and consumer values are never evaluated; a verdict would risk false positives |
| Inferring output types from `value` | Requires expression evaluation |
| Rename detection / "did you mean" for outputs | `moved` does not apply to outputs; the message is part of the finding key |
| `nullable`, `validation`, `ephemeral` tracking | Not surface facts a consumer's wiring depends on; evaluation-adjacent |
| Git-based baseline (HEAD, merge-base) | External process or git object reader; `--base <dir>` with `git worktree` covers it |
| Auto-rebase or persisting the baseline to disk | Results would depend on history; on-disk state is out of scope since v0.1 |
| Graph libraries, go-git, new modules | Stdlib BFS suffices; zero new dependencies (`hcl/v2/ext/typeexpr` is in the existing module) |

## Traceability

Which phases cover which requirements. Updated during roadmap creation.

| Requirement | Phase | Status |
|-------------|-------|--------|

**Coverage:**
- v0.4 requirements: 19 total
- Mapped to phases: 0
- Unmapped: 19 ⚠️

---
*Requirements defined: 2026-10-10*
*Last updated: 2026-10-10 after sec review (bus #103-#119)*
