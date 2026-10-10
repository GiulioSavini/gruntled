# Pitfalls Research

**Domain:** Adding baseline-aware diagnostics (GRT004, daemon blast, `rebase`, transitive Impacted, type-level surface changes) to an existing static, zero-false-positive Terragrunt analyser
**Milestone:** v0.4 Blast-aware Diagnostics (subsequent milestone; engine, daemon and blast v0.3 already shipped)
**Researched:** 2026-10-10
**Confidence:** HIGH for pitfalls grounded in the current code (read directly: `impact`, `analysis/grt001.go`, `tfsurface`, `ipc`, `watch/run.go`, `cmd/gruntled/report.go`, `refs.go`, `depfacts.go`); MEDIUM for Terraform/Terragrunt semantics taken from vendor docs; LOW items are flagged inline.

Phase names below are logical, not numbered. Suggested roadmap order: **Types facts** (surface model) -> **GRT004 + transitive** (pure domain, `impact`) -> **Daemon blast + rebase** (state + IPC) -> **Close** (corpus, mutation proofs, independent audit). The roadmapper maps them to numbers (v0.3 ended at phase 12).

## Facts established during this research (they change the feature definitions)

1. **`moved` does not apply to outputs or variables.** Terraform's `moved` block refactors resources and modules only; the docs do not list outputs or variables. There is no language-level rename mechanism for an output, so an output rename is always "one removed + one added". Do not build any `moved`-aware suppression for GRT004 (MEDIUM: negative finding from docs, not from source).
2. **An `output` block now accepts an optional `type` argument** (docs list `type` as optional on output; `deprecated` is "child modules only", v1.15+). Before that, output types were inferred from `value` and are not statically knowable. Consequence: "output type changed" is only decidable when BOTH sides declare an explicit, parseable `type`. For the vast majority of outputs (no `type`) the right answer is "unknown, silent". `absent -> present` is not a change signal. (MEDIUM: docs; check against the Terraform/OpenTofu versions the corpus pins.)
3. **A variable is required iff it has no `default` attribute.** `default = null` makes it optional. `nullable = false` does not make it required. `type` omitted means `any`. (HIGH: Terraform docs, "Variables without a default value are required".)
4. **`deprecated` exists on variables and outputs** (newer Terraform). A deprecated output is still declared: it must never be treated as removed.
5. **The existing Impacted semantics are "units that instantiate the changed module"**, not "units that depend on it" (`impact.Compute` matches `u.Module()` against the changed module). Transitive Impacted therefore starts from instantiating units and walks REVERSE dependency edges. Mixing the two directions is the most likely design bug in this milestone.
6. **`refs.go` deliberately records no reference for `try()`, `can()`, lazy branches, whole-object `dependency.x.outputs` and expanded dependencies.** Anything that filters edges by "has a reference" inherits those blind spots (false negatives, silent).
7. **`report` is documented as read-only** ("no directory, file, socket or process is ever created"; `respond` "never touches the index"). `rebase` is the first mutating client operation and breaks that invariant on purpose; docs, usage text and tests that pin it must be updated deliberately.

## Critical Pitfalls

### Pitfall 1: GRT004 and GRT001 both fire for the same reference (double reporting)

**What goes wrong:** An output is removed while a consumer still references it. `UnknownOutputs` already emits GRT001 for that reference on the current tree. A new GRT004 pass then emits a second error for the same `(unit, dependency, output)`. In `blast`, `NewFindings(base, cur)` reports both as Broken: two errors at one location, SARIF duplicates, wrong counts, and "refines GRT001" turns into "repeats GRT001".

**Why it happens:** `impact.Compute` builds Broken from `NewFindings` over the full current diagnostic set. GRT001 is computed per tree by `checking.Check` before any baseline is known, so it is already in `curD` when the diff runs.

**How to avoid:**
- Decide once, in requirements: **exactly one finding per reference**. GRT004 *supersedes* GRT001 for that reference, in the blast view only. Apply the replacement to `curD` before `NewFindings`, keyed by `(unit, dependency, output)`, never by message text.
- `check` and plain `report` keep emitting GRT001 only. GRT004 needs two trees, so it cannot live in `analysis` (`analyzers` in `checking.Check` take one graph) and must not enter `checking.Check`. Put it in `domain/impact`.
- Precise GRT004 predicate (silent when any clause is unsure): the reference existed in the base graph; base resolved its dependency to module M with a known surface containing the output; cur resolves the same reference to the same module path M with a known surface lacking it; rows 1-5 of the GRT001 decision table pass in cur. Anything else stays GRT001 (a new consumer written against a removed output, or a consumer re-pointed at a module that never had it, is a plain GRT001).
- Reuse the GRT001 predicate (extract it into a shared function), do not re-implement rows 1-6. A copy will drift on `enabled`, `skip_outputs` and unknown surfaces.
- Anchor the diagnostic at the consumer reference. The removed output has no position in the current tree (its line is gone), and a consumer list inside the message would make the message, which is part of `FindingKey`, change whenever a consumer is added. One GRT004 per consumer reference; the "names its consumers" requirement is met by the grouped presentation (Broken groups by unit already), not by a consumer list in the message.

**Warning signs:** Two diagnostics with identical `unit:line:col`; `blast` summary `broken` count exceeding the number of distinct references; a golden where GRT001 and GRT004 appear together; a GRT004 message containing a list.

**Phase to address:** GRT004 + transitive (domain). Decide the supersede rule in requirements before planning.

---

### Pitfall 2: GRT004 weakens the locked mock/skip decisions

**What goes wrong:** Someone "refines" GRT004 so that a consumer with `mock_outputs` covering the removed output, or with `skip_outputs = true` / `enabled = false`, behaves differently from GRT001. Either `mock_outputs` suppresses GRT004 (violating the locked decision and silently hiding the exact bug), or `enabled=false` / `skip_outputs=true` units get a GRT004 where GRT001 is silent (a false positive: Terragrunt never reads the outputs).

**Why it happens:** GRT004 is written as a new diff feature and its author reads the diff, not the GRT001 decision table.

**How to avoid:** GRT004 inherits the whole table: `enabled` not literally true -> silent; `skip_outputs` not literally false -> silent; unknown module/surface -> silent; mock never suppresses and never downgrades; mock facts only select the message suffix. One shared predicate, one test matrix that runs every row for both codes and asserts "GRT004 fires iff GRT001 would have fired and the baseline had the output".

**Warning signs:** A GRT004 test fixture without a `mock_outputs` case; a `severity` other than error; GRT004 firing on a `try(dependency.x.outputs.y, null)` consumer (that shape is deliberately not a reference).

**Phase to address:** GRT004 + transitive. Verify with a row-by-row parity test and a mutation proof (flip each row, test must fail).

---

### Pitfall 3: Output renamed or moved is treated as something Terraform can express

**What goes wrong:** Time is spent on rename detection (`moved` support, "added one removed one, so suggest rename"), or a "did you mean" hint goes into the message.

**Why it happens:** Resource refactoring has `moved`; people assume outputs do too. Rename hints feel helpful.

**How to avoid:** There is no output `moved`. Treat a rename as remove + add. Do not put suggestions in messages: the project rule already says the message carries no volatile content because it is part of `FindingKey`. A rename hint is only safe in a presentation-only field outside the Key, and even then only when exactly one output was added and one removed; defer it.

**Warning signs:** The word "rename" in a diagnostic message; a heuristic comparing names by edit distance.

**Phase to address:** GRT004 + transitive (requirements: state explicitly "no rename detection").

---

### Pitfall 4: A "required variable added" becomes Broken (include-merged `inputs`, tfvars, env)

**What goes wrong:** A new required variable is reported as an error or as Broken because the unit does not set it. In reality the value can come from the unit's `inputs` after `include` merge, a root config `inputs`, `terraform.tfvars` / `*.auto.tfvars` copied from the module directory, `extra_arguments` with `-var-file`/`required_var_files`, a `generate` block that writes a tfvars file, or `TF_VAR_*`. The project already deferred GRT005/GRT006 for exactly this include-merge false-positive risk.

**Why it happens:** "New required variable" sounds like a breaking change, and the milestone lists it next to GRT004.

**How to avoid:** Type-level changes are **Impacted-only, informational, never a diagnostic, never Broken, never an exit-code input**. They are reason tags on the existing Impacted entry (`added required variable "x"`), not a new tier and not a new code. Promoting any of them to Broken requires modelling `inputs` after include-merge, which is GRT005 and out of scope. Put this in the requirements as a rule, with a test that a unit missing a newly required variable is not in `Broken` and does not change the exit code.

**Warning signs:** A new `Code` constant for a type change; `HasErrors()` depending on a type-level fact; a fixture where the unit sets the variable through an included `inputs` and still shows up as Broken.

**Phase to address:** Types facts, enforced again in the presenter/exit-code phase.

---

### Pitfall 5: False "type changed" from formatting, `any`, `optional()`, attribute order

**What goes wrong:** A type change is reported because of whitespace, comments, line breaks, HEREDOC/quoting, `object({b=..., a=...})` attribute order, `optional(string)` vs `optional(string, null)`, a changed `optional()` default value, `type` absent vs `any`, or `.tf` vs `.tf.json` spelling. Each is noise that destroys the Impacted signal (the project's own decision: "reporting twelve units because a comment changed destroys trust").

**Why it happens:** The natural implementation compares the source text of the `type` expression (`src[expr.Range()]`) or its `fmt` output. That fails every case above. Even a normalised string built from `cty.Type.GoString()` or `FriendlyName()` is wrong: `FriendlyName` prints just "object"; `typeexpr.TypeString` does not handle `any` or `optional()`.

**How to avoid:**
- Parse with `github.com/hashicorp/hcl/v2/ext/typeexpr` (`TypeConstraintWithDefaults`; same module as the existing `hcl/v2` dependency, AST walk only, no evaluation, no new module). Compare the resulting `cty.Type` with `Equals` (object attribute order is irrelevant by construction; optional attributes are part of the type) and discard the `Defaults` tree, so a changed `optional()` default is not a type change.
- The domain must not import HCL/cty (architecture rule). Convert to a **canonical string in the infrastructure layer** with an own recursive renderer: attribute names sorted, `optional` marked, `any` explicit, no whitespace. Domain compares strings only. Pin the renderer with a golden table and a property test: any permutation of object attributes yields the same string.
- Treat absent `type` as `any`.
- Any parse failure (legacy quoted `type = "string"`, JSON-syntax oddities, unknown constructor) makes that variable's type **unknown**; unknown on either side means no report. Do not downgrade the whole module surface to unknown (that would silently drop name-level changes, a regression of v0.3 behaviour): keep names known and type facts independently optional.
- Direction: `any` replacing `T` widens a variable and is not a breaking change for callers; `T` replacing `any` narrows it. Do not attempt subtyping. Cheapest safe rule: skip the report for variable `T -> any`; report every other `!Equals` as Impacted.
- Outputs: only compare when both sides have an explicit parseable `type` (Fact 2). Output `sensitive` counts only when both sides are literal booleans; a non-literal (`sensitive = var.x`) is unknown. Reuse the existing literal-bool helper from `depfacts.go`, do not evaluate.

**Warning signs:** A test fixture diff that only reformats a file and shows Impacted; a type string containing a Go `%v` rendering; rendering that differs between two runs (map iteration); `.tf.json` fixtures absent.

**Phase to address:** Types facts. Needs a metamorphic test: reformatting, reordering attributes, adding comments and converting `.tf` to the equivalent `.tf.json` must produce an empty diff.

---

### Pitfall 6: `default = null` vs absent `default`; duplicate and union declarations

**What goes wrong:** "Required" is computed by evaluating the default (`default == null` -> treated as required), or by `Attributes["default"].Expr` being a null literal. A `.tf`/`.tofu` pair, which the reader reads as a UNION, or the same variable declared twice with different attributes, then yields a last-file-wins type nondeterministically (the current reader collects names into a `map`; adding attributes to that map copies the problem).

**Why it happens:** `ReadSurface` currently stores only names in sets; there is no per-variable record to disagree with itself.

**How to avoid:** `required = no default attribute present in the block`, read from the block body's attribute set (HCL and JSON alike; in JSON `"default": null` is a present attribute). When the same variable name is declared more than once across the kept files with differing facts, mark each differing fact **unknown** (never pick). The `.tf`/`.tofu` union stays: it can over-count names, which never creates a false report, but for type facts a conflict is unknown. Also remember `nullable`/`sensitive`/`ephemeral` are separate facts; do not fold them into "required".

**Warning signs:** Test names containing "default null"; a reader that iterates `kept` files and overwrites a map entry; flaky output across `-count=N` runs.

**Phase to address:** Types facts.

---

### Pitfall 7: The surface model change leaks into existing outputs and contracts

**What goes wrong:** `repograph.Surface` gains per-variable data. `graph --json`, `blast --json` and the SARIF/doc-sync tests change shape, or equality of `Surface` values (used by tests and by `SurfaceDiff`) starts depending on description/default fields, so "only names count" silently stops being true.

**Why it happens:** `Surface` is the shared currency of `tfsurface`, `indexing`, `graph` and `impact`; widening it touches all of them. `SurfaceDiff` documents "a changed default, type or description is not a surface change".

**How to avoid:** Keep `Surface.Variables()/Outputs()` returning names exactly as today. Add a separate, optional `TypeFacts` structure on the module (unknown by default), consumed only by a new `impact` function. Do not emit it in `graph --json` (or bump that schema deliberately). Keep `SurfaceChange.Empty()` meaning "names changed"; add a separate `TypeChanges` field and a separate `Empty` for the combined case. Re-run the unchanged v0.3 goldens as a regression gate: they must stay byte-identical.

**Warning signs:** Any v0.3 golden or `TestGraph*` diff in the PR; `SurfaceChange` equality used in a `map` key (the new slice field makes it uncomparable).

**Phase to address:** Types facts (first plan: change nothing observable, prove goldens equal).

---

### Pitfall 8: Transitive impact explodes and the signal becomes useless

**What goes wrong:** A change to a foundational module (vpc, iam, a shared `tags` module) puts nearly every unit in the repository in Impacted. The output is thousands of lines, nobody reads it, and the one-hop precision that v0.3 established is gone. A second form: the transitive list changes the existing `blast` output for current users.

**Why it happens:** Reverse reachability over a real Terragrunt graph is a wide cone. And it is an over-approximation by construction: unit A depending on B is affected by a change in B's module only if B's *outputs* change as a result, which cannot be known without evaluating expressions.

**How to avoid:**
- **Label honestly.** Transitive entries are "may be impacted"; direct entries stay exactly as v0.3. Keep the direct list's JSON shape; add `depth` and `via` (the connecting path, as a list of unit paths) to a **separate** collection or as additive fields, and bump `blastSchemaVersion` only if an existing field changes meaning.
- **Which edges propagate** (state in requirements, test per row): block edges with `enabled` literally not `false` and `skip_outputs` not `true`; `dependencies { paths }` edges do not propagate (no outputs are read); unknown `enabled`/`skip_outputs` do not propagate (silent when unsure). Do not filter by "has a reference": `refs.go` has no reference for whole-object uses, `try()` and lazy branches, so such a filter would silently drop real edges (Fact 6).
- **Unit statuses:** a unit with an unknown module can be reached (its dependency edges are still recorded) but is never a source; a config-unknown unit has no recorded dependencies, so it ends a path silently. Document both as false-negative sources.
- **Bound the presentation, not the data.** Text output shows direct entries fully, groups transitive entries by depth with counts and a deterministic cap ("and N more"); JSON is complete. Offer a depth flag; the default in the text view must not be unbounded.
- **Shortest path, deterministic tie-break:** BFS from sources in sorted order, neighbours in sorted path order, first discovery wins, so the connecting path is the lexicographically smallest among shortest ones. Never iterate a Go map for neighbours.
- **Broken units are traversed but not listed** (keeps Broken/Impacted disjoint, as the current `Result` contract says) and a unit appears once, at its minimum depth.

**Warning signs:** On the corpus, a single-output edit that lists a majority of 65 units; text output length growing with repo size; a path that differs between two runs; `Impacted` containing a unit present in `Broken`.

**Phase to address:** GRT004 + transitive. Measure on the pinned corpus (iso20022 has 62 of 65 units with `dependency` blocks) before fixing the default depth.

---

### Pitfall 9: Cycles (GRT003) and BFS

**What goes wrong:** The traversal loops forever or reports paths that repeat a unit; or a cycle member is reported with a path through itself; or a deep graph blows the stack in a recursive implementation.

**Why it happens:** GRT003 reports cycles but the graph still contains them; `Edges()` returns every edge including cycle edges.

**How to avoid:** Iterative BFS with a visited set keyed by `RepoPath` (never recursion; the project already has a recursion-depth hardening culture). A unit is visited once, so cycles terminate by construction; a source that is also reachable from another source keeps depth 0. Test: a cycle A->B->A where A's module changes lists B at depth 1 and does not relist A; a self-loop (`dependencies.paths = [""]` is a GRT003 self-loop) terminates; a diamond chooses the deterministic path. Add the cycle shape to the property-test generator (Retrospective lesson: a property test is only as good as its generator).

**Warning signs:** A test that completes only because the fixture has no cycle; recursion in the traversal code.

**Phase to address:** GRT004 + transitive.

---

### Pitfall 10: Units whose module is `unknown`, and re-pointed units

**What goes wrong:** An unknown-module unit is treated as "using the changed module", or treated as never-affected. Separately, a unit whose `terraform.source` changed between base and cur is judged only by the current module mapping, so a module swap (a big change) produces nothing.

**Why it happens:** `Compute` maps units to modules with the current graph only (`u.Module()` of `curG`); `SurfaceDiff` skips modules unknown on either side.

**How to avoid:** Keep the rule: unknown is never compared and never a source (silent). For GRT004, a consumer whose target module is unknown in cur or base is silent (already implied by the predicate in Pitfall 1). Document explicitly that a re-pointed unit is not covered by v0.4 (false negative, silent) and add one golden that pins the current behaviour so a later change is deliberate. Do not "fix" this inside the milestone without a requirement.

**Warning signs:** A fixture with `source` changed that expects a finding; any code branch that treats `ok == false` from `u.Module()` as "changed".

**Phase to address:** GRT004 + transitive (golden); noted under Gaps for a later milestone.

---

### Pitfall 11: Daemon baseline silently stale, polluted, or lost

**What goes wrong:**
- The baseline is "the first index after `watch` started". If the user started the daemon on a dirty tree, or on a tree that mid-edit had a module with a temporarily missing output, the baseline is wrong. All pre-existing findings are masked as "not Broken" (Compare treats base findings as known), and a surface that was unknown in the baseline is never compared. Both fail silent.
- A long-running daemon's baseline drifts from `main`; after days "Impacted" lists the whole sprint's work.
- A daemon crash or restart resets the baseline to the then-current tree, and `report --blast` now reports "nothing impacted" with no sign anything was lost.

**Why it happens:** An in-memory baseline has no provenance. The cheap implementation stores the first `checking.Report` and renders a blast diff against it.

**How to avoid:**
- Print the provenance with every `report --blast`: baseline source (`daemon start` / `rebase`), the generation number it was taken at and the current generation. Use generations, not wall-clock times, in the deterministic payload (the existing `Snapshot.Generation` already counts successful indexes). A baseline from the initial index is labelled as such.
- **Never auto-rebase.** The only transitions are start and an explicit `rebase`.
- Retain only domain values (`checking.Report`: graph + diagnostics set). Never retain a second loader or parse cache: it pins every parsed AST for the daemon's lifetime, and `blasting.Sources` already warns a loader must not be shared between trees. Memory is two graphs plus two diagnostic sets; measure on a synthetic repo of a few thousand units (`synthrepo`) and put the figure in the plan.
- Optional seeding from a directory (`watch --base <dir>`, reusing the v0.3 `--base` mechanism) solves the dirty-start problem; if included, build the baseline `Report` once with its own loader, keep the `Report`, drop the loader.
- Property: `blast --base D` and `report --blast` with baseline = tree D must be byte-identical (same trick as `report` == `check`).

**Warning signs:** `report --blast` right after daemon start printing a clean result with no baseline label; RSS growing after many saves (baseline referencing the loader); a restart test that doesn't assert the label.

**Phase to address:** Daemon blast + rebase.

---

### Pitfall 12: `rebase` races with in-flight reindex and with the reader

**What goes wrong:**
1. `rebase` is handled on the socket goroutine and mutates the baseline while `Run`'s single indexer goroutine publishes: a torn read of baseline vs snapshot, or `rebase` captures a `Report` the daemon deliberately did not publish (`Run` skips publishing when newer changes are pending because the result "may come from a torn read mid-save").
2. The user reads `report --blast` at generation N, then runs `rebase`; a save lands in between and the baseline becomes generation N+1, accepting a change the user never reviewed.
3. `rebase` while the state is `indexing` or `failed` (last good report is stale versus the broken tree).
4. Rebase silently turns current errors into "pre-existing", hiding them from blast.

**Why it happens:** `respond` is pure over an immutable `*Snapshot`; a mutating op does not fit that shape, and the "last published" notion lives in the `watchPublisher` in `cmd/gruntled/watch.go`, not in the IPC layer.

**How to avoid:**
- Serialise state changes through the publisher: baseline lives next to `last`/`gen` under the publisher's lock (or a channel into the `Run` goroutine); the socket handler calls a function, it never touches indexer state. Snapshots stay immutable; a rebase installs a new pointer atomically (`atomic.Pointer`, already used in `instance.go`).
- Rebase target = the **last published** report. Include the target in the request as an optional expected generation (compare-and-swap); respond with the generation actually used; print it. Without the flag, the response still says which generation, so a surprise is visible.
- Refuse (nonzero exit, explicit message) while `indexing` or `failed`.
- Print how many current errors are being accepted by the rebase.
- Test with the existing deterministic `Run` harness (fake Indexer/Watcher/clock): interleave a pending-skipped result with a rebase and assert the baseline is never the skipped report; run under `-race` in CI.

**Warning signs:** A rebase handler that reads `p.last` without the lock; no test where a reindex is in flight; the response not carrying a generation.

**Phase to address:** Daemon blast + rebase.

---

### Pitfall 13: IPC compatibility (old client / new daemon, new client / old daemon) and the zero-value trap

**What goes wrong:** The worst case is not an error but a wrong answer: a new client asks an old daemon for `--blast`, receives a snapshot whose blast fields are absent, decodes them as empty, and prints "nothing impacted". A second case: a new op sent to an old daemon returns `unknown op`, a message that is cryptic and not actionable. A third: `Snapshot` grows (pre-rendered blast text/JSON per format), approaching the 64 MiB response cap and enlarging the windows report-file dump written on every index.

**Why it happens:** `encoding/json` ignores unknown fields and zero-fills missing ones. `ProtocolVersion` is a single integer (`1`) checked by equality; extending the snapshot additively does not trip it.

**How to avoid:**
- Decide the rule once: **any change that old peers would misread bumps `ProtocolVersion`** (the existing `VersionError` already prints "restart the daemon with the same gruntled"). A new `blast` capability, a `rebase` op and a changed snapshot are all in that class, so bump to 2. The v1 daemon rejects a v2 request with its own versioned reply, and `decodeResponse` turns it into the `VersionError`.
- If staying on v1 for compatibility, blast data must be a pointer/`omitempty` field and the client must treat "absent" as "daemon does not support `--blast`" with exit 3, never as "empty". Also compare `Info.Version`.
- The plain `report` op's bytes must stay identical for all three formats (regression test pinned to a recorded v0.3 response).
- Render blast in the publisher at publish time and store it in the immutable snapshot (the server keeps never touching the index); do not compute it lazily per request on the socket goroutine. Budget the extra bytes against `maxResponse` and the dump; add a size test on a large synthetic repo.
- `rebase` over the report file (windows) is impossible: the dump is read-only by design and `net` is excluded from the binary. Return an explicit "rebase is not supported on windows" (exit 3), documented, instead of a silent no-op. `report --blast` on windows reads the blast from the dump. This is a scoped decision the requirements must state.
- Every string in the new response (paths, names, type strings, `via` paths) goes through `escapeTerm` / the JSON escaper; the sec agent audit in v0.3 found presenter blind spots exactly here.

**Warning signs:** A client test that never talks to a fake v1 daemon; a response struct with non-pointer blast fields; `report --blast` exit 0 against a daemon started by the previous release.

**Phase to address:** Daemon blast + rebase. Include a compatibility matrix test (v1 daemon fake x new client, new daemon x v1 client request).

---

### Pitfall 14: Determinism and path discipline in the new outputs

**What goes wrong:** New collections (transitive entries, `via` paths, type-change lists, GRT004 ordering) are emitted in map or discovery order; a path in a message is absolute or contains the baseline directory; type strings embed host-dependent detail; the daemon blast differs from the CLI blast byte for byte.

**Why it happens:** New code, new maps. The project rule is "every collection must be sorted explicitly" and "all paths repo-relative".

**How to avoid:** Sort at the producing function, not in the presenter. `via` paths are lists of `RepoPath`, rendered with the existing presenters. GRT004 diagnostics enter `diagnostic.Set`, so canonical ordering is free; keep them in the Set. Run each new output twice (and in a shuffled-input property test) and compare bytes. The baseline label must never include the `--base` absolute directory (v0.3 used a label; keep it a label).

**Warning signs:** `range` over a map in `impact`; an absolute path in any golden; a flaky golden under `-count=20`.

**Phase to address:** every phase; verified in Close.

## Moderate Pitfalls

### Pitfall 15: Pre-existing findings and rebase interplay

**What goes wrong:** After `rebase`, GRT004 for a still-dangling reference disappears from blast because the base now lacks the output, while the plain `report` still shows GRT001. Users read this as "it fixed itself".
**Prevention:** Intended and consistent with "pre-existing findings are never Broken", but document it, and make `report --blast` print the number of pre-existing errors (e.g. "N errors existed at baseline") so the masking is visible. Test: GRT004 present before rebase, absent from Broken after, GRT001 still in `report`.

### Pitfall 16: Equal diagnostics from key collisions

**What goes wrong:** Two references to the same output from one unit (different positions) produce identical `FindingKey` (position-free) and the multiset logic treats the second as pre-existing or new inconsistently.
**Prevention:** The v0.3 multiset handling already covers identical keys; keep GRT004 message deterministic and add a fixture with two references to the same removed output in one unit.

### Pitfall 17: Output removed but still "declared" through union or deprecated

**What goes wrong:** GRT004 or the Removed list fires when an output was merely moved to another file (names are a set, so no) or marked `deprecated` (still declared, no).
**Prevention:** Names come from the union of kept files; add fixtures for move-between-files, `.tf` -> `.tofu` rename of the file, and a `deprecated` output. A module going from known to unknown (syntax error mid-edit) must produce nothing, not "all outputs removed". This is the single most likely daemon false positive: a save at an inconsistent moment. The existing "unknown on either side yields nothing" rule must stay unconditional for GRT004.

### Pitfall 18: Direction mix-up in transitive traversal

**What goes wrong:** Walking forward (dependencies) instead of reverse (dependents) lists the units the changed unit depends on, which cannot be impacted by it.
**Prevention:** Build the reverse adjacency from `Edges()` once; name the types `dependents`; a test with an asymmetric chain (A depends on B depends on C; change C's module: B at depth 1, A at depth 2, never the reverse).

### Pitfall 19: Docs, help and exit-code text drift

**What goes wrong:** `report` usage still says "GRT001-GRT003, GRT100"; SARIF rule list lacks GRT004; `docs/cli.md`, `docs/validation.md` and the README go stale. The repo has doc-sync tests (`TestSARIFDoc`, `TestCIDoc`, help-vs-docs, `validation_doc_test`) that will fail late.
**Prevention:** Add GRT004 to the rule registry, SARIF rules, usage text and docs in the same plan as the code; add GRT004 to the list of codes pinned by the doc tests. Finish doc edits before verification runs (Retrospective lesson 3). GRT004 only appears in blast output; say so in the docs so nobody expects it from `check`.

## Minor Pitfalls

### Pitfall 20: Hostile or enormous type expressions

**What goes wrong:** A pathological `type` expression (deep nesting, huge object) reaches `typeexpr` and recurses.
**Prevention:** The reader already runs `CheckNativeDepth` / `CheckJSONDepth` and size caps before parsing; type walking happens only after that. Add a depth cap on the canonical renderer and treat exceeding it as unknown.

### Pitfall 21: Static-proof regressions

**What goes wrong:** A new import (`ext/typeexpr`, a time or formatting helper) pulls `net` or `os/exec` into one of the six release targets.
**Prevention:** `ext/typeexpr` is in the existing `hcl/v2` module; still, run the six-target no-net/no-exec proof on the first plan that imports it, not at the end.

## Technical Debt Patterns

| Shortcut | Immediate Benefit | Long-term Cost | When Acceptable |
|----------|-------------------|----------------|-----------------|
| Compare `type` source text or `fmt` strings | 10 lines of code | False "type changed" on any reformat; erodes trust | Never |
| Widen `Surface` with per-variable fields | One struct to pass around | Leaks into `graph --json`, goldens, equality semantics | Never; use a separate optional facts structure |
| GRT004 as a copy of GRT001 plus a diff condition | Fast | Rows drift; mock/skip semantics diverge | Never; share the predicate |
| GRT004 inside `checking.Check` / `analyzers` | Reuses wiring | `check` then needs a baseline, breaking `report == check` | Never |
| Compute blast lazily on the socket goroutine | No snapshot growth | Violates "server never touches the index"; races with reindex | Never |
| Bump nothing, rely on additive JSON | No compatibility work | Old client prints "no impact" against a daemon that cannot say | Never for blast; acceptable for purely cosmetic fields |
| Unbounded transitive text output | Simple presenter | Unreadable on foundational-module changes | Only in JSON |
| Keep the baseline loader instead of the Report | Can re-derive anything | Pins every parsed AST for the daemon's lifetime | Never |
| Windows `rebase` via a request file | Feature parity | Needs a writable control channel; new attack surface in the runtime dir | Defer; document unsupported |

## Integration Gotchas

| Integration | Common Mistake | Correct Approach |
|-------------|----------------|------------------|
| `impact.Compute` and GRT001 | Add GRT004 findings next to existing GRT001 | Replace GRT001 by `(unit, dependency, output)` before `NewFindings` |
| `tfsurface.Reader` | Add attributes to the name sets | Separate per-variable fact record, conflicts become unknown |
| `watchPublisher` / `Run` | Mutate baseline from the socket goroutine | Publisher-owned baseline, atomic pointer swap, rebase serialised |
| `ipc.respond` | Add a mutating op without changing the "never touches the index" contract | Handler calls a publisher function; snapshots stay immutable |
| `ProtocolVersion` | Additive fields at v1 | Bump, or make absence explicit and refuse |
| Windows dump | Assume rebase works | Explicit unsupported error; blast rendered into the dump |
| Presenters | New text fields emitted raw | Every new string through `escapeTerm` / JSON escape |
| `blastSchemaVersion` | Change meaning of `impacted` | Additive fields only, else bump |
| `refs.go` shapes | Use references to decide edge liveness | Use `enabled` / `skip_outputs` only |

## Performance Traps

| Trap | Symptoms | Prevention | When It Breaks |
|------|----------|------------|----------------|
| Blast recomputed on every `report --blast` request | Latency scales with requests | Compute once at publish, store in snapshot | Many editors/prompts polling |
| Re-parsing `.tf` for type facts on every save | Save-to-result latency grows | Reuse the existing parse/surface path; only changed module dirs invalidate | Thousands of modules |
| Per-source BFS instead of one multi-source BFS | O(sources x edges) | One multi-source BFS, sorted frontier | Foundational module with many instantiating units |
| Baseline plus current plus per-format snapshots | Memory growth, dump size | Report values only; measure with `synthrepo`; check `maxResponse` | ~5k units (assumption; measure) |
| Path strings built per entry | Allocation spike in text output | Build `via` lists once per node (parent pointers), join at render | Deep chains |

## Security Mistakes

| Mistake | Risk | Prevention |
|---------|------|------------|
| `rebase` accepted from any local user | Another user resets your baseline | Socket permissions are already per-user in the runtime dir; keep the socket 0600 and add a test that `rebase` is refused for a mismatching peer if the transport exposes credentials; otherwise document the trust boundary |
| Unescaped variable/type/path text in blast | Terminal control or bidi injection from repository content (v0.3 sec finding) | All new fields through `escapeTerm` and JSON escaping; extend the existing escape tests with the new fields |
| Request size/flags for the new op | Memory abuse via the socket | Keep `maxRequest` 4 KiB; reject unknown fields' size growth; expected-generation is a number, not free text |
| New file written for rebase (windows) | Writes under the repo or a predictable path | Do not add; unsupported on windows |
| Baseline label includes the `--base` absolute path | Absolute path leak breaks reproducibility | Label only |

## UX Pitfalls

| Pitfall | User Impact | Better Approach |
|---------|-------------|-----------------|
| "Impacted" and "may be impacted" look the same | Users trust the over-approximation | Separate section, depth, and the connecting path on every transitive entry |
| Silent "nothing impacted" when the baseline was lost | False reassurance | Always print baseline provenance and generations |
| Rebase gives no feedback | User unsure what was accepted | Print generation and number of errors accepted |
| GRT004 message lists every consumer | Message churn, huge lines | One diagnostic per consumer reference, grouped by presentation |
| Transitive output of thousands of lines | Ignored | Counts and a cap in text, full data in JSON |
| Type change shown without before/after | Hard to act on | Show `old -> new` canonical type strings (escaped) |

## "Looks Done But Isn't" Checklist

- [ ] **GRT004:** often missing the supersede rule for GRT001 - verify no reference ever yields two diagnostics.
- [ ] **GRT004:** often missing the mock/skip/enabled parity - verify a matrix that runs every GRT001 decision-table row for both codes.
- [ ] **GRT004:** often fires on a module that went unknown mid-edit - verify a syntax-error-in-module fixture yields nothing.
- [ ] **Type changes:** often missing metamorphic tests - verify reformat, reorder, comment and `.tf` <-> `.tf.json` produce an empty diff.
- [ ] **Type changes:** often reported for `default = null` or `optional()` default edits - verify they are not type changes.
- [ ] **Type changes:** verify none reaches `Broken`, a diagnostic code, or the exit status.
- [ ] **Transitive:** often missing cycle, self-loop and diamond fixtures - verify termination and a deterministic path.
- [ ] **Transitive:** verify direction with an asymmetric chain and that paths-only edges do not propagate.
- [ ] **Transitive:** verify default text output on the corpus is bounded and the direct list is unchanged from v0.3.
- [ ] **Daemon blast:** verify `report --blast` == `blast --base D` byte for byte when the baseline is tree D.
- [ ] **Daemon blast:** verify provenance (source, generations) is printed and no wall-clock time is in the deterministic payload.
- [ ] **Rebase:** verify it never installs an unpublished/torn report, refuses during `indexing`/`failed`, reports generation and accepted error count, and passes `-race`.
- [ ] **IPC:** verify plain `report` bytes are unchanged, v1 daemon x new client fails loudly, and the windows path is an explicit error for `rebase`.
- [ ] **Docs:** verify usage text, `docs/cli.md`, SARIF rules and doc-sync tests list GRT004 and the new flags/ops before verification runs.
- [ ] **Generators:** verify the property/stateful tests generate cycles, unknown modules, `enabled=false`, `skip_outputs`, mocks, `.tf.json` and union `.tofu` files, and prove non-vacuity with counters and a mutation (Retrospective lesson 1).

## Recovery Strategies

| Pitfall | Recovery Cost | Recovery Steps |
|---------|---------------|----------------|
| False positive GRT004/type change released | MEDIUM | Make the case silent first, add the fixture, patch release; the corpus oracle (terragrunt v1.1.6) decides ambiguous cases |
| Double GRT001+GRT004 shipped | LOW | Apply the supersede rule in `impact` and bump nothing else; golden update |
| Protocol mismatch shipped at v1 | MEDIUM | Bump `ProtocolVersion`, release; old clients get a `VersionError` |
| Baseline retains loader (memory growth) | LOW | Store `Report` only; no data migration (in-memory) |
| Transitive output too noisy | LOW | Change default depth/cap in the presenter; data model unchanged |
| Rebase race found late | MEDIUM | Move baseline into the publisher lock; add the deterministic harness test |

## Pitfall-to-Phase Mapping

| Pitfall | Prevention Phase | Verification |
|---------|------------------|--------------|
| 1 GRT004/GRT001 double report | GRT004 + transitive | Parity and uniqueness test; golden with both codes absent together |
| 2 Mock/skip/enabled semantics | GRT004 + transitive | Row-by-row matrix; mutation per row |
| 3 Rename/moved | GRT004 + transitive (requirements) | No rename logic; grep test for suggestions in messages |
| 4 Required variable as Broken | Types facts | Test: unit not Broken, exit code unchanged |
| 5 False type change | Types facts | Metamorphic test; canonical renderer golden; permutation property |
| 6 default null / union / duplicates | Types facts | Fixtures for `default=null`, `.tf`+`.tofu`, duplicate declarations; `-count=N` stability |
| 7 Surface model leak | Types facts | v0.3 goldens byte-identical |
| 8 Transitive explosion | GRT004 + transitive | Corpus measurement; bounded text; direct list unchanged |
| 9 Cycles | GRT004 + transitive | Cycle, self-loop, diamond fixtures; no recursion |
| 10 Unknown / re-pointed units | GRT004 + transitive | Goldens pin silent behaviour |
| 11 Stale/lost baseline | Daemon blast + rebase | Provenance output; restart test; `blast --base` equivalence; memory figure |
| 12 Rebase races | Daemon blast + rebase | Deterministic `Run` harness + `-race`; CAS generation |
| 13 IPC compatibility | Daemon blast + rebase | Matrix test; v0.3 `report` bytes pinned; windows explicit error |
| 14 Determinism / paths | All, verified in Close | Twice-run and shuffled-input property tests; no absolute path in goldens |
| 15-19 Moderate | Phase owning the feature | See each entry |
| 20-21 Minor | Types facts / first importing plan | Depth cap test; six-target proof |

## Gaps / Needs Phase-Specific Research

- Exact Terraform/OpenTofu version in which output `type` and `deprecated` became valid, and how OpenTofu treats them (docs fetched for Terraform only). Affects whether output type comparison is ever worth shipping in v0.4. Recommend deciding "output type: compare only when both sides explicit" and testing with the corpus pins.
- Behaviour of `ext/typeexpr` on `.tf.json` string-form types and on legacy bare `list`/`map`: the package is documented as an AST walk, but the exact error classes should be confirmed by a spike (no Go toolchain was available in this research environment, so it could not be executed). Anything erroring maps to unknown.
- Whether `report --blast` should support all three formats (text, json, sarif) or text and json only (SARIF has no Impacted notion). Requirements decision.
- Whether the daemon should accept `watch --base <dir>` for seeding (Pitfall 11). Requirements decision; it is the only fix for dirty-start pollution.
- Memory and snapshot-size figures on a multi-thousand-unit synthetic repo have not been measured; the 5k figure above is an assumption.

## Sources

- Code read directly (HIGH): `/home/giulio/gruntled/internal/domain/impact/{impact,surface}.go`, `/home/giulio/gruntled/internal/domain/analysis/grt001.go`, `/home/giulio/gruntled/internal/application/{checking,blasting,watching}`, `/home/giulio/gruntled/internal/infrastructure/{tfsurface/reader.go,ipc/ipc.go,watch/run.go,terragrunt/refs.go,terragrunt/depfacts.go}`, `/home/giulio/gruntled/cmd/gruntled/report.go`, `/home/giulio/gruntled/internal/interfaces/presenter/blast.go`.
- Project history (HIGH): `/home/giulio/gruntled/.planning/PROJECT.md`, `/home/giulio/gruntled/.planning/RETROSPECTIVE.md` (v0.1-v0.3 lessons: generator quality, cross-phase audit, doc-sync, oracle validation).
- Terraform variable block reference (MEDIUM, vendor docs): https://developer.hashicorp.com/terraform/language/block/variable
- Terraform output block reference (MEDIUM): https://developer.hashicorp.com/terraform/language/block/output
- Terraform `moved` block reference (MEDIUM, negative finding): https://developer.hashicorp.com/terraform/language/block/moved
- HCL `ext/typeexpr` package (MEDIUM, summary of package docs): https://pkg.go.dev/github.com/hashicorp/hcl/v2/ext/typeexpr
- Terragrunt dependency block (`enabled`, `skip_outputs`, `mock_outputs`) (MEDIUM): https://terragrunt.gruntwork.io/docs/reference/config-blocks-and-attributes

---
*Pitfalls research for: blast-aware diagnostics on a static Terragrunt analyser (gruntled v0.4)*
*Researched: 2026-10-10*
