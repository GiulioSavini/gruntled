# Project Research Summary

**Project:** gruntled — milestone v0.4 Blast-aware Diagnostics
**Domain:** Static analysis / change-impact (blast radius) for Terragrunt repositories, with a warm-index daemon
**Researched:** 2026-10-10
**Confidence:** MEDIUM-HIGH

## Executive Summary

v0.4 turns gruntled from a single-tree checker into a baseline-aware tool: what broke and what is impacted relative to a baseline. Four target features: GRT004 (removed output still referenced), `report --blast` + `rebase` from the daemon's in-memory baseline, transitive Impacted with a connecting path, and type-level surface changes. No comparable tool (Terramate, Terragrunt `--filter`, Pants, Nx, Bazel, Atlantis, Spacelift) prints an explained path, detects removed-but-consumed outputs, or keeps a baseline in a long-running process — the warm index plus the baseline diff is the defensible position PROJECT.md already names.

The recommended approach adds zero module dependencies (`go.mod`/`go.sum` byte-identical): the one new import is `hcl/v2/ext/typeexpr` from an already-required module, with no banned package added to the six-target no-net/no-exec proof. Everything else is stdlib plus existing code. One pure shared core, `blasting.Between(base, cur Report)`, serves both `blast --base` and the daemon so they agree by construction (pinnable by a byte-equality property test). The daemon retains the immutable derived `checking.Report` by pointer as its baseline (never the Loader/parse cache) and pre-renders the blast view at publish. IPC `ProtocolVersion` 1→2.

Key risks are false-positive and trust risks: GRT004/GRT001 double-reporting, type-level changes leaking into Broken or the exit code, transitive cone noise, a stale/lost daemon baseline, rebase races, and old/new IPC peers misreading each other. Mitigation: a supersede rule, one shared GRT001 predicate, Impacted-only type facts, a restricted edge rule with bounded text output, explicit rebase with printed provenance, a protocol bump, and a publisher-owned lock.

## Reconciled Conflicts (binding for requirements)

**1. Output `type`.** FEATURES.md says outputs have no `type`; STACK, ARCHITECTURE and PITFALLS say Terraform 1.15.0 added an optional `type` (PR #36411). **Resolution:** compare an output's type only when BOTH baseline and current declare a parseable type; any other combination is silent; never infer from `value`. Absent→present is not a change signal; absent output type is not `any` (for variables absent `type` is `any`). FEATURES.md's anti-feature row on output type is superseded. Phase-research item: confirm the 1.15 release-note wording and OpenTofu parity.

**2. GRT004 vs GRT001.** GRT004 is produced only in the blast core (`blasting.Between`, shared by `blast --base` and the daemon), never by `check`/`report`, and not added to the `analyzers` slice, so `report == check` byte identity holds. It supersedes the matching GRT001 — exactly one finding per (unit, dependency, output), matched by structured key (never message text), applied to the current diagnostic set before `NewFindings`. It fires only when: the reference existed in the baseline; base resolved it to the same module with a known surface declaring the output; the current surface is known and lacks it; and GRT001's decision-table predicate passes in the current tree. The predicate is one shared function (`resolveReference`) extracted from `grt001.go` — `enabled`, `skip_outputs`, unknown module/surface, `mock_outputs` (never suppresses; mock facts only pick the message suffix). Otherwise it stays GRT001. Severity error, anchored at the consumer reference; the message carries no consumer list (grouped in the presenter via `Result.RemovedOutputs`). No rename detection/`moved` handling; a deprecated output is still declared; no SARIF rule for GRT004 in v0.4.

**3. Transitive edge rule.** Propagating edges: block edges with `enabled` literally true and `skip_outputs` not true; unknown tristates do not propagate; `dependencies { paths }` edges do not propagate; edges are not filtered by "has a reference" (`refs.go` blind spots). Multi-source level-synchronous BFS over reverse edges, each level sorted by `RepoPath`, first discovery wins → shortest path with lexicographic tie-break; iterative with a visited set (cycles and self-loops terminate). Seeds are direct consumers computed before the Broken filter; Broken units are traversed but not listed (Broken/Impacted stay disjoint). Each unit appears once, at minimum distance, with one path. `--depth N`; `--depth 1` == v0.3 (v0.3 goldens stay valid). Text bounded by default (counts plus a deterministic cap), JSON complete.

**4. Type-level changes.** Impacted-only facts: new required variable, variable losing its default, variable type old→new, output sensitivity flip (informational), output type when both sides declare it. Never diagnostics, never Broken, never exit-code affecting, no new `Code` (escalation is GRT005/006, deferred). Equality-only on a canonical string, no widening/narrowing judgement. Unknown on either side → silent. A type-parse failure makes only that fact unknown, never the module surface. Conflicting duplicate declarations (`.tf` + `.tofu`) → unknown. "Required" = no `default` attribute; `default = null` is a default.

**5. Daemon.** Baseline = first successful index, moved only by explicit `rebase`; never auto-rebased, never persisted. Provenance by generation counters, not clocks (`daemon baseline (generation N)`); no timestamps or absolute paths in any payload. `ProtocolVersion` 1→2 with the exact-match policy kept. New ops `blast` (read) and `rebase` (only mutating op, in memory). Plain `report` bytes unchanged. `rebase` unsupported on windows (explicit message, exit 3); `report --blast` works on windows via the report file. `rebase` refuses during `indexing`/`failed`, prints the generation and the count of accepted errors, and targets the last published report (never a skipped/torn one).

**Deferred:** `watch --base <dir>`, `blast --format sarif` (`report --blast --format sarif` is a usage error), output sensitivity beyond an informational fact, windows rebase, GRT005/GRT006, reference-gated propagation, rename hints.

## Key Findings

### Recommended Stack

No new module. The single new import is `hcl/v2/ext/typeexpr` (v2.25.0), only in `internal/infrastructure/tfsurface`. The `go list -deps` delta is exactly that one package on all six targets; `check-architecture.sh` needs no rule change. The domain holds an opaque canonical type string compared with `==` and never sees `cty`/HCL.

- `typeexpr.TypeConstraintWithDefaults` — parses `type` without evaluation; optional defaults discarded; any error diagnostic → unknown.
- Own ~40-line `cty.Type` canonical walker — `typeexpr.TypeString` is lossy for `optional()` (reproduced on v2.25.0) and panics on capsule types; pinned by a property test against `cty.Type.Equals`.
- `PartialContent` per `variable`/`output` for `type`, `default`, `sensitive`; the literal-bool rule moves from `terragrunt/eval.go` to `hclconv`.
- stdlib `slices`/`sort`/`encoding/json` — BFS, ordering, wire format; no graph library.

### Expected Features

**Must have:** GRT004 in the blast core with presenter-grouped consumers; transitive Impacted with canonical shortest path, distance, `--depth`; type facts (required variable, variable type old→new); daemon baseline, `report --blast` (text/json), `rebase` with a discard summary; deterministic ordering of all new lists.

**Should have:** the explained canonical path (no comparable tool prints one); blast answered from the warm index on every save; output sensitivity flip as an informational fact (folded into the type-facts phase).

**Defer:** `watch --base <dir>`, `blast --format sarif`, `.git/HEAD` changed hint, per-hop `file:line`, reference-gated propagation, GRT005/006, windows rebase, `nullable`/`validation`/`ephemeral` tracking.

### Architecture Approach

`blasting.Between(base, cur)` = `analysis.RemovedOutputs` → supersede → `impact.Compute`. The `watchPublisher` owns `base`, `lastRep` and a mutex; at each `EventReady` it computes and pre-renders the blast view into the immutable `Snapshot.Blast`. The socket server stays a byte reader behind a new `Handler` interface (`Serve` kept as a wrapper, so existing ipc tests are untouched). `blastSchemaVersion` bumped once, 1→2, for the whole milestone.

1. `repograph` — Surface + VariableDecl/OutputDecl/TypeConstraint; `Variables()/Outputs()` name views and `NewSurface` unchanged (v0.3 goldens byte-identical).
2. `analysis/grt004.go` + shared `resolveReference` — blast-only.
3. `impact` — SurfaceDiff extension, `transitive.go`, Compute: type diff, BFS, `Path`/`Distance`, `RemovedOutputs`.
4. `tfsurface` — reader attributes + `typestring.go`; the only place touching `typeexpr`/`cty`.
5. `ipc` v2, `watchPublisher`, `report --blast`, `rebase`.

### Critical Pitfalls

1. **GRT001+GRT004 double-report** — supersede by structured key, blast view only; test that no reference yields two diagnostics.
2. **GRT004 weakens mock/skip/enabled decisions** — one shared predicate, row-by-row parity matrix, mutation of each row.
3. **Type change becomes Broken or a diagnostic** — Impacted-only; test that a unit missing a new required variable is not Broken and the exit code is unchanged.
4. **False "type changed"** — `typeexpr` + canonical string + metamorphic test (reformat, reorder, comment, `.tf`→`.tf.json` gives an empty diff); a parse failure never disables GRT001.
5. **Transitive cone explosion / direction mix-up** — restricted edge rule, bounded text, asymmetric-chain test, corpus measurement (iso20022: 62/65 units have dependencies).
6. **Stale/lost baseline, rebase races, IPC misread** — printed provenance, no auto-rebase, serialised under the publisher lock (stdout outside it), protocol bump, `report` byte pin, `-race`.
7. **Module going unknown mid-edit must produce nothing** — the most likely daemon false positive; "unknown either side → silent" is unconditional for GRT004.

## Implications for Roadmap

ARCHITECTURE and PITFALLS differ only on whether the surface-model refactor comes first or third; the ARCHITECTURE order is recommended because the surface change is additive and can land in phase 3 without touching phases 1–2, and the daemon merges last so its byte-equality test covers everything.

### Phase 1: GRT004 + shared `Between` core
**Rationale:** smallest pure-domain, highest-value slice; closes MORE-03, creates the shared core, verifiable through the existing `blast --base` before any daemon work.
**Delivers:** `Between`, `resolveReference` extraction, `analysis/grt004.go`, `CodeRemovedOutput`, supersede, `Result.RemovedOutputs`, presenter grouping; docs/usage/rule registry/doc-sync pins in the same plan.
**Avoids:** pitfalls 1, 2, 3, 10, 16, 17, 19; golden pinning silent re-pointed units; test that `check` can never emit GRT004.

### Phase 2: Transitive Impacted + path + `--depth`
**Rationale:** pure domain and independent; the headline. Settling the Impacted entry shape (`Path`, `Distance`) before type facts means one schema bump.
**Delivers:** `impact/transitive.go`, `Path`/`Distance`, `--depth`, bounded text view, `blastSchemaVersion` 2.
**Avoids:** pitfalls 8, 9, 14, 18; generator with cycles, self-loops, diamonds, unknown modules, `enabled=false`, `skip_outputs`, non-vacuity counters and a mutation.

### Phase 3: Type-level surface facts
**Rationale:** widest model change, isolated; first plan changes nothing observable and proves v0.3 goldens byte-identical.
**Delivers:** `TypeConstraint` + decls, `tfsurface` attribute extraction, `typestring.go`, `hclconv.LiteralBool` move, `SurfaceDiff` extension, escaped `old -> new` presenter tokens, informational output sensitivity flip.
**Uses:** `typeexpr`; six-target proof run in the first plan that imports it.
**Avoids:** pitfalls 4, 5, 6, 7, 20, 21.

### Phase 4: Daemon baseline, `report --blast`, `rebase`, IPC v2
**Rationale:** composes phases 1–3 through the same `Between` + presenter, so `report --blast == blast --base <baseline copy>` covers everything; most concurrency and protocol risk.
**Delivers:** publisher state + mutex, `Snapshot.Blast`, `Handler`, ops, version bump, dump path, `sock_other.go` stub, the two commands, usage, docs.
**Avoids:** pitfalls 11, 12, 13, 15; v1-fake-daemon × new-client compatibility matrix, `-race` rebase test on the deterministic `Run` harness, `MemStats` probe on `synthrepo`, `report` byte pin.

### Phase 5: Cross-phase audit, corpus, docs
**Rationale:** the v0.3 audit found a BLOCKER and a MEDIUM that per-phase checks missed.
**Delivers:** GRT004 mutation run on the three corpora (0 FP unmutated, every injected removal caught), blast of a tree against itself is empty, six-target proof, doc pins, independent security pass on the socket op and presenter escaping.

### Research Flags

Need deeper research (`/gsd-plan-phase --research-phase`):
- **Phase 4** — first mutating socket op, locking, windows asymmetry, 64 MiB cap with pre-rendered blast, unmeasured memory.
- **Phase 3** — spike: `typeexpr` over `.tf.json` string form and legacy bare `list`/`map`, six-target proof, Terraform 1.15 output `type` release note + OpenTofu parity, `override.tf` and `.tf`+`.tofu` duplicates.

Standard patterns: Phase 1 (reuses the GRT001 table and mutation corpus), Phase 2 (standard BFS, decisions fixed above; measure the text bound on the corpus during planning), Phase 5 (v0.3 process).

## Confidence Assessment

| Area | Confidence | Notes |
|------|------------|-------|
| Stack | HIGH | `go list -deps` verified on six targets; lossy `TypeString` reproduced; output `type` since 1.15 is MEDIUM |
| Features | MEDIUM-HIGH | Comparable tools checked against vendor docs; output-type claim superseded by reconciliation 1 |
| Architecture | HIGH / MEDIUM | Integration points read from the code; memory figures derived, not measured |
| Pitfalls | HIGH / MEDIUM | Code-grounded items HIGH; Terraform semantics and the `moved` negative from docs |

**Overall confidence:** MEDIUM-HIGH

### Gaps to Address

- **Output `type` release confirmation** — read the 1.15 notes and check OpenTofu; behaviour is safe either way (both-declared only). Phase 3 spike.
- **`typeexpr` on `.tf.json` and legacy forms** — test matrix; anything that errors → unknown.
- **`T -> any` suppression** — PITFALLS suggests it, STACK says equality-only. Decided: equality-only (everything is informational).
- **GRT004 "reference existed in baseline"** — explicit requirement (MORE-03). Cost: a reference added in the same change that removes the output stays GRT001.
- **Default transitive text cap** — choose after measuring the 65-unit corpus.
- **Memory/snapshot size at thousands of units** — measure in phase 4; the 5k figures are assumptions.
- **Re-pointed units** (`terraform.source` changed) — silent false negative; pin with a golden and document.
- **Rebase peer credentials** — the existing 0600 socket in a 0700 dir is the trust boundary; document it; add a peer-credential test only if the transport exposes credentials.

## Sources

### Primary (HIGH)
- Repository code at v0.3.0; `.planning/PROJECT.md`, `RETROSPECTIVE.md`
- `hcl/v2@v2.25.0/ext/typeexpr` and `go-cty@v1.19.0` source; `go list -deps` on six targets
- Terraform variable / input-variable / outputs docs
- Vendor docs: Terramate, Terragrunt filter/graph/git, Pants, Nx, Bazel, Atlantis, Spacelift, buf, oasdiff, Watchman

### Secondary (MEDIUM)
- Terraform output block and `moved` references; Terragrunt dependency block reference; Digger issue #402; Terragrunt RFC #6111

### Tertiary (LOW)
- Terraform 1.15.0 output `type` (PR #36411, changelog, blog) via secondary summaries
- Baseline memory figures (derived); `ephemeral` output rules across versions

---
*Research completed: 2026-10-10*
*Ready for roadmap: yes*
