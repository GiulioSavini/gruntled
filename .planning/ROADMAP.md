# Roadmap: gruntled

## Milestones

- ✅ **v0.1 Validated Engine** - Phases 1-4 (shipped 2026-09-29) — archived in `milestones/v0.1-ROADMAP.md`
- ✅ **v0.2 CI-Ready** - Phases 5-7 (shipped 2026-10-08) — archived in `milestones/v0.2-ROADMAP.md`
- ✅ **v0.3 Watch & Blast** - Phases 8-12 (shipped 2026-10-10) — archived in `milestones/v0.3-ROADMAP.md`
- 🚧 **v0.4 Blast-aware Diagnostics** - Phases 13-17 (in progress)

## Phases

<details>
<summary>✅ v0.1 Validated Engine (Phases 1-4) - SHIPPED 2026-09-29</summary>

See `milestones/v0.1-ROADMAP.md`.

</details>

<details>
<summary>✅ v0.2 CI-Ready (Phases 5-7) - SHIPPED 2026-10-08</summary>

- [x] Phase 5: Graph Diagnostics (7/7 plans) — completed 2026-09-30
- [x] Phase 6: Machine-Readable Output (5/5 plans) — completed 2026-10-01
- [x] Phase 7: Distribution & CI Integration (5/5 plans) — completed 2026-10-01

See `milestones/v0.2-ROADMAP.md`.

</details>

<details>
<summary>✅ v0.3 Watch & Blast (Phases 8-12) - SHIPPED 2026-10-10</summary>

- [x] Phase 8: Incremental Index Foundation (2/2 plans) — completed 2026-10-08
- [x] Phase 9: Blast Radius (3/3 plans) — completed 2026-10-08
- [x] Phase 10: Watch Daemon & Status File (7/7 plans) — completed 2026-10-08
- [x] Phase 11: Report, Single Instance & Release Proof (7/7 plans) — completed 2026-10-08
- [x] Phase 12: Gap Closure — Watcher Directory Ignore & v0.3 Audit Findings (5/5 plans) — completed 2026-10-10

See `milestones/v0.3-ROADMAP.md`.

</details>

### 🚧 v0.4 Blast-aware Diagnostics (In Progress)

**Milestone Goal:** Use the difference against a baseline, not only the current tree: the daemon answers blast on every save, impact follows the graph past one hop and through type changes, and a removed output that is still referenced becomes an error that names its consumers.

- [ ] **Phase 13: Removed-Output Diagnostic & Shared Blast Core** - `GRT004` replaces `GRT001` at every reference to an output the change removed; `check` stays byte-identical to v0.3.0
- [ ] **Phase 14: Transitive Impact with Paths** - Impacted follows dependents past one hop, with a distance, one connecting path and `--depth`
- [ ] **Phase 15: Type-Level Surface Facts** - New required variables and variable/output type or `sensitive` changes put instantiating units in Impacted, never in Broken
- [ ] **Phase 16: Daemon Baseline, `report --blast` & `rebase`** - The daemon answers blast against its in-memory baseline over IPC v2, and the baseline moves only on `rebase`
- [ ] **Phase 17: Corpus Mutation Proof & Cross-Phase Audit** - No noise and 100% mutation catch on the three corpora, plus an independent audit across phase boundaries

## Phase Details

### Phase 13: Removed-Output Diagnostic & Shared Blast Core

**Goal**: User sees an output that was removed from a module but is still referenced by `dependency.X.outputs.Y` reported as an error at each consuming reference (`GRT004`), by `gruntled blast --base`, through one shared comparison core that every later blast surface reuses.
**Depends on**: Phase 12 (shipped v0.3 `blast --base`)
**Requirements**: MORE-03, MORE-07
**Success Criteria** (what must be TRUE):
  1. Removing from a module an output that a unit still references makes `gruntled blast --base <dir> <path>` report one `GRT004` error at each such reference site and no `GRT001` at those sites, with exit code 1; the message names the output, the module and the target unit and uses repo-relative paths only
  2. `GRT004` only reclassifies an existing `GRT001`: for the same two trees the Broken units and the number of findings per unit equal what v0.3.0 `blast` printed, no site ever shows both codes, and removing an output that nothing references adds no finding
  3. Wherever a precondition fails the site behaves exactly as `check` does in v0.3.0: a reference added in the same change, a dependency re-pointed to another module, a module unknown on either side, or an output the baseline surface did not declare keeps `GRT001`; `enabled = false`, `skip_outputs = true` or a non-literal value for either stays silent; `mock_outputs` never suppresses the error
  4. `check` (text, JSON, SARIF), plain `report`, `graph --json` and the status file never print `GRT004` and stay byte-identical to v0.3.0 on pinned goldens
**Plans**: TBD

### Phase 14: Transitive Impact with Paths

**Goal**: User sees a surface change reach every unit that depends on an affected unit, directly or through a chain, and each impacted unit shows how far it is from the change and the path that connects it.
**Depends on**: Phase 13
**Requirements**: BLAST-03, BLAST-04, BLAST-05
**Success Criteria** (what must be TRUE):
  1. With units A, B and C where B depends on A and C depends on B, and A instantiates the changed module, `gruntled blast --base` lists A (distance 1), B (distance 2) and C (distance 3) as Impacted, each with one shortest path read from that unit back to the unit that instantiates the module
  2. Only live `dependency` block edges propagate: `enabled = false`, `skip_outputs = true`, any non-literal value for either, and `dependencies { paths }` edges stop propagation; Broken units are traversed but listed only under Broken, so Broken and Impacted stay disjoint and sorted
  3. Cycles, self-loops and diamonds terminate; each unit appears once, at its minimum distance, with the path that is smallest under `RepoPath.Compare` at the first differing hop, identical for any input or map order (proven against a brute-force oracle on random graphs with shuffled input)
  4. `blast --depth N` limits propagation (default unlimited); `--depth 1` with only name-level changes yields the v0.3 Impacted set; `--depth 0`, a negative or a non-integer value exits 2
  5. JSON carries `schema_version` 2 with additive fields only (distance, source and `via` = predecessor, so size is linear in chain length); text prints the full path up to a fixed hop count and elides the rest deterministically (the 5,000-unit size bound is proven in Phase 15 under BLAST-11)
**Plans**: TBD

### Phase 15: Type-Level Surface Facts

**Goal**: User sees a new required variable, a changed variable or output type, or a flipped `sensitive` put the units that instantiate the module in Impacted with a named reason, without it ever becoming a diagnostic or a Broken unit.
**Depends on**: Phase 14 (the Impacted entry shape is settled first, so the blast schema is bumped once)
**Requirements**: BLAST-06, BLAST-07, BLAST-08, BLAST-09, BLAST-10, BLAST-11, SEC-01
**Success Criteria** (what must be TRUE):
  1. A variable that is new without a `default`, or that loses its `default`, puts its module's instantiating units in Impacted with a "now required" reason; `default = null` counts as a default, and a change to `nullable`, `validation`, `ephemeral` or `deprecated` alone changes nothing
  2. A variable whose `type` constraint changes puts the instantiating units in Impacted with a "type changed" reason, as does an output whose declared `type` changes (only when both sides declare a parseable type) or whose `sensitive` flips; formatting, comments, attribute order, `optional()` defaults and `.tf` to `.tf.json` conversion give an empty Impacted set
  3. Type-level reasons never create a diagnostic, never make a unit Broken and never change the exit code: a unit missing a newly required variable is not Broken, and with `--depth 1` every v0.3 blast golden gives the same Broken and Impacted sets and exit code (text differs only by distance/path tokens, JSON only by `schema_version` 2 and the additive fields)
  4. An ambiguous fact reports nothing: a name declared in more than one kept file with differing facts, any Terraform override file, a non-literal or unparseable value, or an unknown fact on either side stays silent, while names stay known so `GRT001` and name-level Impacted are unchanged
  5. Hostile or huge input is safe and bounded: variable, output and object attribute names containing C0, C1, U+202E and invalid UTF-8 print escaped in `blast` text and JSON; a 4 MiB type expression and a 5,000-unit linear chain keep RSS and output size within the asserted bounds; the canonical type renderer never panics (fuzz test in CI); the six-target no-net/no-exec proof passes on the first commit that imports `hcl/v2/ext/typeexpr`
**Plans**: TBD
**Research flag**: yes — spike before planning: `typeexpr` over `.tf.json` string form and legacy bare `list`/`map`; Terraform 1.15 output `type` release note and OpenTofu parity; `override.tf` and `.tf` + `.tofu` duplicate declarations; the six-target proof with `ext/typeexpr` linked

### Phase 16: Daemon Baseline, `report --blast` & `rebase`

**Goal**: User with `gruntled watch` running can ask what the current tree breaks and impacts against the baseline the daemon took at its first index, and can move that baseline deliberately.
**Depends on**: Phase 15 (composes Phases 13-15 through the same comparison core and presenters)
**Requirements**: DAEMON-07, DAEMON-08, DAEMON-09, DAEMON-10
**Success Criteria** (what must be TRUE):
  1. After a save, `gruntled report --blast [--format text|json]` prints the daemon's latest Broken and Impacted against the baseline (the first successful index), showing the baseline and current generations; apart from the baseline label line (text) or baseline object (JSON) its bytes equal `blast --base <copy of the tree at baseline time>` (property test), including for a module with hostile names (the daemon-path half of the SEC-01 escape test)
  2. `report --blast` exits 0, or 1 only when Broken has an error; 2 for `--format sarif` or `--depth`; 3 when there is no daemon, it is indexing or failed, or versions mismatch; after a failed reindex it prints the previous view with the existing stderr warning; on windows it reads the view from the report dump
  3. `gruntled rebase [--expect N]` moves the baseline to the last published report in memory only: it prints the new generation and the number of error findings in Broken at that moment, refuses with exit 3 while indexing or failed or when `--expect N` differs, says the outcome is unknown on a client timeout, exits 3 as unsupported on windows, writes no file and never starts a daemon; until the next save `report --blast` then shows empty sets; docs no longer call `report` read-only
  4. The IPC protocol is version 2: a client/daemon version mismatch is an explicit error with exit 3, never an empty "nothing impacted" answer; the compatibility matrix (fake v1 daemon with v2 client, v2 daemon with v1 request, v1 report dump) passes and plain `report` bytes are unchanged
  5. The baseline holds only the `checking.Report` (never a loader, `fs.FS` or parse cache); a blast response over 64 MiB is an explicit exit-3 error naming the cap; daemon memory on `synthrepo` at 65, 500 and 5,000 units is measured and recorded
**Plans**: TBD
**Research flag**: yes — first mutating socket op and its serialisation with publish; windows asymmetry (`report --blast` via dump, `rebase` unsupported); 64 MiB cap with a pre-rendered blast view; unmeasured memory at thousands of units; peer-credential trust boundary of the 0600 socket

### Phase 17: Corpus Mutation Proof & Cross-Phase Audit

**Goal**: User can trust the milestone on real repositories and across phase boundaries: no false positives, no noise from reformatting, every injected change caught.
**Depends on**: Phase 16
**Requirements**: REL-03
**Success Criteria** (what must be TRUE):
  1. On each corpus (iso20022, secret, denis256) `check` output is byte-identical to v0.3.0 and `blast --base <same tree>` reports empty Broken and Impacted
  2. Reformat-only mutations (whitespace, comments, attribute reorder, `optional()` default, `.tf` to `.tf.json`, output moved to another file, `deprecated` added) each give an empty Impacted set
  3. Every mutation in the set is caught: referenced output removed (`GRT004` at every site and no `GRT001`), unreferenced output removed, required variable added, default removed, variable type changed, output type declared and changed, `sensitive` flipped, and propagation to depth 2 or more
  4. An independent security review across the blast core, the socket ops and output escaping leaves no open BLOCKER or MEDIUM (any found is closed in this phase), the six-target no-net/no-exec proof and the doc-sync pins pass on the final tree, and the docs match the shipped behaviour of `GRT004`, `--depth`, `report --blast` and `rebase`
**Plans**: TBD

## Progress

| Phase | Milestone | Plans Complete | Status | Completed |
|-------|-----------|----------------|--------|-----------|
| 5. Graph Diagnostics | v0.2 | 7/7 | Complete | 2026-09-30 |
| 6. Machine-Readable Output | v0.2 | 5/5 | Complete | 2026-10-01 |
| 7. Distribution & CI Integration | v0.2 | 5/5 | Complete | 2026-10-01 |
| 8. Incremental Index Foundation | v0.3 | 2/2 | Complete | 2026-10-08 |
| 9. Blast Radius | v0.3 | 3/3 | Complete | 2026-10-08 |
| 10. Watch Daemon & Status File | v0.3 | 7/7 | Complete | 2026-10-08 |
| 11. Report, Single Instance & Release Proof | v0.3 | 7/7 | Complete | 2026-10-08 |
| 12. Gap Closure — Watcher Dir Ignore & Audit Findings | v0.3 | 5/5 | Complete | 2026-10-10 |
| 13. Removed-Output Diagnostic & Shared Blast Core | v0.4 | 0/TBD | Not started | - |
| 14. Transitive Impact with Paths | v0.4 | 0/TBD | Not started | - |
| 15. Type-Level Surface Facts | v0.4 | 0/TBD | Not started | - |
| 16. Daemon Baseline, `report --blast` & `rebase` | v0.4 | 0/TBD | Not started | - |
| 17. Corpus Mutation Proof & Cross-Phase Audit | v0.4 | 0/TBD | Not started | - |
