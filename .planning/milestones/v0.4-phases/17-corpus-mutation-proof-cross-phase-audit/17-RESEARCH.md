---
phase: 17-corpus-mutation-proof-cross-phase-audit
type: research
created: 2026-10-12
code: master d99714e
requirements: [REL-03]
roadmap-sc: [SC1 corpus self-identity, SC2 reformat-only, SC3 mutation set, SC4 cross-phase audit + final gate + docs]
sources:
  - .planning/REQUIREMENTS.md (REL-03), ROADMAP.md phase 17, STATE.md, research/SUMMARY.md + PITFALLS.md
  - phases 13-16: every SUMMARY, VERIFICATION and SECURITY file
  - milestones/v0.1-phases/04-* (CONTEXT, VERIFICATION), v0.2-phases/05-VERIFICATION, v0.3-MILESTONE-AUDIT.md
  - docs/validation.md, docs/cli.md (Blast text/JSON, Known limitations)
  - code at d99714e: scripts/compare-ref.sh, scripts/compat-v1.sh, scripts/check-architecture.sh,
    cmd/gruntled/{corpus_test,corpus_graph_test,denis256_test,validation_doc_test,daemon_blast_test}.go,
    internal/infrastructure/watch/run.go, cmd/gruntled/rebase_test.go, .github/workflows/ci.yml
  - gsd-core workflows audit-milestone.md, complete-milestone.md, bin/lib/verify.cjs, bin/lib/config.cjs
measured: >
  Corpora shallow-fetched at the pinned SHAs into the session scratchpad (one-time network).
  HEAD and v0.3.0 binaries built once (GOMAXPROCS=2, -p=2, GOPROXY=off). One binary on one corpus
  at a time, under timeout, free -m checked first. No go test was run.
---

# Phase 17: Research

## 0. Summary

REL-03 is a proof phase. It adds no product feature. Its work is a test harness and
an oracle, both independent of gruntled's own code, run over three real corpora. After
that come an independent cross-phase audit and the bookkeeping that milestone closure
needs.

What I measured on d99714e:

| Check | primary (iso20022) | secret | denis256 |
|---|---|---|---|
| v0.3.0 vs HEAD: `check --format text/json/sarif` and `graph --json`, stdout, stderr and exit | 4/4 identical | 4/4 identical | 4/4 identical (exit 1) |
| `blast --base <faithful copy> .` | Broken 0, Impacted 0, changes 0, exit 0, 0.10 s, 13 MB | same, 0.01 s, 12 MB | same, 0.51 s, 25 MB |
| units / resolved / known modules instantiated | 65 / 62 / 62 | 4 / 3 / 3 | 1146 / 852 / 818 |
| propagating edges (block, enabled true, skip_outputs false, target has config) | 11 of 109 | 3 of 6 | 591 of 766 |
| max distance; modules whose cone reaches distance >= 2 | 2; 2 | 2; 2 | 5; 160 |
| referenced (module, output) pairs that would fire; sites | 4; 22 | 3; 4 | 99; 468 |

So the first half of SC1 already holds on HEAD (12/12). Phase 17 has to turn it into a
pinned, repeatable gate, not fix anything. The second half (self-blast empty) also holds.

The sizing numbers come from HEAD `graph --json`. They are for planning only. The
generator recomputes them, and the oracle never takes them from gruntled (see §4).

Decisions I recommend (§11):
1. The oracle is a **one-time independent model per corpus**. It holds the unit→module map,
   the live edges and the reference sites, and it is cross-checked once against `graph --json`,
   with every disagreement pinned by hand. Each mutation's expected result is then a pure
   function of (mutation descriptor, model). This is possible because every surface mutation
   touches only module `.tf`/`.tf.json` files and never a `terragrunt.hcl`, so the graph is
   invariant across the whole run.
2. GRT004 gets a **second oracle**: the frozen v0.3.0 binary's `check` on the mutated tree.
   Its GRT001 set must equal HEAD's GRT004 set at the same positions.
3. **Exhaustive**, not sampled: about 4,300 runs, about 30 min serial, under 100 MB. It runs
   env-gated and serial at the phase gate. Corpus tests are not in CI (a v0.1 decision), and
   adding them is an open question (§12).
4. **Daemon sample: yes, small and targeted.** One mutation per class per corpus, plus the
   iso20022 shared `global.tf` symlink-target edit, plus one rebase cycle, through a real
   `watch` daemon over the socket (linux).
5. The **cross-phase audit** has two parts. gruntled-sec does security (SC4), and
   gsd-integration-checker checks wiring and flows, as in the v0.3 audit. Their outputs feed
   `/gsd-audit-milestone`. `git.create_tag` defaults to **true** in gsd-core, so it must be
   turned off before complete-milestone.

## 1. Corpora: where they live and how to get them offline

| Name | Repository | Pinned commit | Env var | Licence |
|---|---|---|---|---|
| iso20022 (primary) | aws-solutions-library-samples/guidance-for-iso20022-messaging-workflows-on-aws | `e6c55d11fd1a01e75b78d7897be36c69fa26b8cc` | `GRUNTLED_CORPUS` | MIT-0 |
| secret | cds-snc/secret | `341e8a95b0d9bd658793094a248527cc3ebae6f2` | `GRUNTLED_CORPUS_SECRET` | MIT |
| denis256 | denis256/terragrunt-tests | `726485e699a70c02dabbde629f66c0119e197357` | `GRUNTLED_CORPUS_DENIS256` | MIT |

- **Not vendored.** The corpora are pinned by commit hash and fetched. The v0.1 decision
  (04-CONTEXT "Corpora stay outside the repository") is still in force. The test code never
  hardcodes a path. The documented location is `~/.cache/gruntled-phase4/corpus/{primary,secret,denis256}`
  (docs/validation.md "Reproduce" and the v0.2 "Command" section).
- **Pin enforcement already exists.** `corpusRequire` wants a detached HEAD equal to the pin.
  `secretRequire` and `denisRequire` go through `denisHeadCommit`, which resolves a ref, a loose
  ref or packed-refs. Each one skips when its env var is unset and fails (does not skip) on any
  other commit. A shallow `git fetch --depth 1 <url> <sha>` + `checkout --detach FETCH_HEAD`
  satisfies all three. I verified this: GitHub serves a fetch by SHA.
- **They are not on this machine.** `~/.cache/gruntled-phase4/` (corpora and the pinned
  terragrunt) does not exist at d99714e. No checkout was found under /home, /tmp, /root or
  /opt. The phase needs **network once**, for three shallow fetches. Sizes: primary 12 MB,
  secret 1.3 MB, denis256 2.2 GB working tree plus 438 MB `.git`. **1.77 GB of denis256 is 30
  checked-in ELF binaries** (`terragrunt`, `perf-tests/terragrunt.0.*`). The 3,433 other
  files are 12 MiB, and the HCL/TF part is 1.4 MB.
- **Copies.** `corpusCopy` (faithful: same bytes, perms and symlink targets, digest-checked)
  for iso20022 and secret. `corpusCopyFiltered(…, graphIsELF)` for denis256, the existing
  v0.2 pattern. The ELF-stripped copy gives the same self-blast result (measured: base =
  filtered copy, cur = full checkout, Impacted 0).
- **Offline after the fetch.** Every run then uses local files only:
  - `GOFLAGS=-mod=readonly GOPROXY=off GOTOOLCHAIN=local`, as in compare-ref.sh and compat-v1.sh;
  - v0.3.0 is built from the local tag through a temporary worktree (compare-ref pattern);
  - the corpora are read from the checkouts, and the tests write only to `t.TempDir`;
  - the pinned terragrunt is **not needed** for REL-03 (see §4.5).
  
  An optional offline proof for the gate is to run the test under `unshare -rn` (an empty
  network namespace) on linux. The six-target no-net/no-exec proof concerns the binaries, and
  `check-architecture.sh` Step 9 already covers them. The fetch is a maintainer step, the same
  as in v0.1/v0.2. If the plan adds a `scripts/fetch-corpora.sh`, it must verify the SHA after
  checkout and never run anything from the corpus.
- **How earlier phases used them.**
  - v0.1 phase 4: TestCorpusClean/TestCorpusMutation. The textual `corpusOracle` (regexps and
    no gruntled parser) covered 2 mutations: 8 and 3 GRT001 sites, all at col 16.
  - v0.1: TestDenis256Corpus, an exact hand-derived GRT001 set of 8.
  - v0.2 phase 5: TestCorpusGraphClean/Mutation over `graphRepos` with the textual
    `graphOracle` (oracleMask/oracleBlocks/oracleAttr/oracleIncludes/oracleUnitEdges),
    13 mutations. Every oracle disagreement was pinned by hand (`denisGraphOracleOnly`, 12 rows),
    and docs/validation.md is pinned by TestValidationDocPins.
  - Phases 11/12 did not run the corpora. 11-RESEARCH only proposed measuring snapshot cost on
    denis256. compare-ref.sh is fixture-only: 244 comparisons over testdata goldens, the
    diag03/blast_grt004 scripts and clean/sarif fixtures. It does **not** include the corpora.

## 2. REL-03 part 1: `check` identity and self-blast

**2.1 Byte identity with v0.3.0.** Extend `scripts/compare-ref.sh` with an optional corpus
list. An env var keeps the one-argument usage, for example
`COMPARE_REF_CORPORA="$GRUNTLED_CORPUS $GRUNTLED_CORPUS_SECRET $GRUNTLED_CORPUS_DENIS256"`.
Each corpus is a read-only fixture: cwd = checkout, argument `.`, the same 4 commands, stdout and
exit compared. I also compared stderr, and it is identical too. Adding stderr only for corpus
rows is optional; it is cheap and stronger. Expected count: 244 + 12 = 256. The perturb
self-test still applies. Cost: v0.3.0 and HEAD on denis256 take about 0.3 s per command.
Constraints the script already meets and must keep: the trap, the offline env, `rev-parse
--verify --end-of-options`, no extraction of corpus files (no txtar), and a corpus dir checked
to exist and to be a directory. A Go-side alternative (`TestCorpusCheckMatchesV030`, building
v0.3.0 inside the test) would duplicate compare-ref's worktree logic. **Recommend the script.**

**2.2 Self-blast.** A new env-gated `TestCorpusBlastSelf` with three subtests, run for each
corpus and both formats:
- `blast --base <checkout> <checkout>` (the same dir);
- `blast --base <faithful copy> <checkout>`;
- the same with `--format json`.

It asserts:
- exit 0;
- text: line 1 is the escaped baseline label, then exactly `Broken (0):\nImpacted (0):\n`,
  with no Type changes section;
- JSON: `broken`, `impacted` and `changes` are all `[]`, and summary is `{0,0}`;
- the checkout digest is unchanged (`corpusDigest`).

The baseline label is the `--base` argument as given, so tests compare from line 2 on (text)
and assert the JSON `baseline: true`.

## 3. REL-03 part 2: reformat-only transforms (each must give empty Impacted)

**Common harness.**
- base = a pristine copy and cur = base + transform. A two-sided case uses base' and cur'.
- Assert Broken 0, Impacted 0, `changes == []`, exit 0. `changes` is stricter than REL-03's
  "empty Impacted": a reformat must not even produce a module diff.
- **Non-vacuity guard.** The main way a reformat test passes falsely is a transform that breaks
  the parse, which makes the module unknown, and blast is then silent. So for every transformed
  tree:
  - `graph --json` `modules[]` (`known`, `variables`, `outputs`) must equal the base's, and
    `unknown_units`/`unknown_modules` must be unchanged;
  - `check` must show no new GRT100;
  - a per-transform counter (blocks or files touched) must be > 0 per corpus.
- Transforms edit only regular `.tf` files of **known, instantiated** modules (the eligible
  set, §4.1). They never touch `terragrunt.hcl` or a `.tf` file that is a symlink, unless the
  plan uses the materialisation rule below.

**Scanner.** Reuse `oracleMask` from corpus_graph_test.go. It gives the top-level brace depth or
-1 inside strings, templates, comments and heredocs. Add a block finder for
`^(variable|output)\s+"…"\s*\{` at depth 0 and an attribute splitter at depth 1. The splitter
puts attribute boundaries where the bracket depth of `( [ {` over code bytes returns to 0 at a
newline. **The generator may not import HCL.** check-architecture.sh Step 6 forbids
`hashicorp/hcl` and `zclconf` outside internal/infrastructure, tests included. That is also
what keeps it independent of gruntled's parser. Heredocs exist in 1/1/4 `.tf` files of the
three corpora; the mask handles them.

| # | Transform | Mechanical rule (deterministic, every eligible site) | Notes |
|---|---|---|---|
| R1 | whitespace | In every eligible file: a blank line after each top-level `}`, `=` aligned or padded to two spaces on depth ≥ 1 attribute lines, and tabs changed to spaces outside strings | Never inside mask -1 (strings, heredocs) |
| R2 | comments | `# gruntled-r2` before each `variable`/`output` block, `/* r2 */` after its `{`, `// r2` at the end of each attribute line | Only at code bytes; not after a heredoc opener |
| R3 | attribute reorder | Reverse the order of the depth-1 attributes in each variable/output block; nested blocks (`validation {}`) keep their place | The splitter must cover multi-line values |
| R4 | `optional()` default | **Two-sided.** No corpus has an `optional(` (0/0/0), so each eligible module gets an injected `variable "gruntled_opt" { type = object({a = optional(string)}) default = {} }` in base', and cur' changes it to `optional(string, "x")`. Variants: default added, default value changed | base' vs pristine is not under test |
| R5 | `.tf` → `.tf.json` | Move each variable/output block whose attribute values are single-line and free of heredocs or `%{` into `gruntled_r5.tf.json`: `type` becomes a string of the type expression, `default` becomes `"${<expr>}"` (presence is the fact), `sensitive` a JSON bool, `value` `"${<expr>}"`, `description` a string. Other blocks stay in `.tf`. The reverse direction is base = converted, cur = pristine | Counter: blocks converted > 0. On iso20022 this is the main `.tf.json` coverage (the corpora have 0 eligible `.tf.json` files today) |
| R6 | output moved to another file | Each output block moves to `gruntled_moved.tf` in the same module. Also do it for variables | Byte extent from the block finder |
| R7 | `deprecated` added | `deprecated = "gruntled-r7"` in every variable block and every output block | BLAST-06 says a `deprecated` change never counts |
| R8 | composition | R1∘R2∘R3∘R6∘R7 at once | Cheap, catches interactions |

**Symlinked module files (iso20022).**
- Each of the 62 iso20022 modules has a `global.tf` symlink to `iac.src/global.tf` (66 links to
  `../global.tf`, 2 to `../iac.src/global.tf`).
- That file declares 6 variables.
- Edit the target file **once** as a corpus-wide shared case.
- Also add one materialisation transform (R9), which replaces a symlink with a regular file
  holding the same bytes. It must give an empty Impacted. A materialised link is also how the
  mutation generator turns a shared variable into a module-local one.

**Cost.** One run per transform per corpus (corpus-wide): 9 × 3 × about 0.5 s at most. That is
negligible.

## 4. REL-03 part 3: the mutation set (caught 100%)

### 4.1 Eligibility and the independent model (the oracle)

All mutations edit only module files (`.tf`, `.tf.json`). Unit configs never change, so the
following are computed **once per corpus** on the pristine tree and reused for every mutation:

- **Units.** A unit is a directory with a `terragrunt.hcl`, skipping `.git`, `.terragrunt-cache`
  and hidden directories.
- **Unit → module map (seeds).** `terraform { source = "<lit>" }` comes from the unit file or an
  include the oracle can resolve (`oracleIncludes`). It is resolved like `oracleResolve`, with
  the `//` subdir split (secret: `"../../aws//ecr"` gives `aws/ecr`). An absent source means
  module = unit dir. On iso20022 all 62 resolved units have module == unit dir. A remote,
  dynamic or unresolvable source means the unit has no module (it is not a seed).
- **Live edges** (BLAST-03, written from the requirement text, not the code). A `dependency`
  block edge (not `dependencies { paths }`) is live when:
  - `enabled` is absent or literally `true`;
  - `skip_outputs` is absent or literally `false`;
  - `config_path` is a literal that resolves to a unit directory.

  Any other value stops propagation. This extends `oracleUnitEdges`, which today checks
  `enabled` but not `skip_outputs`. Note that 98 of iso20022's 109 edges are
  `skip_outputs = true`, so the extension matters.
- **Reference sites.** `dependency.<label>.outputs.<name>` in each unit file and its resolvable
  includes, mapped through the label's `config_path` to the target unit's module. Each site is
  classified as **plain** (`corpusOracle`'s `plainRe`: `attr = dependency.l.outputs.y` on its own
  line), **guarded** (inside `try(`, `can(`, a ternary, `&&`, a `for`; silent per the
  `lazy_guards` golden) or **other**. Then come the DIAG-03 literal rows of the edge: a
  disabled edge, `skip_outputs = true` or a non-literal value is silent.
- **Eligible modules.** These are known to the oracle, meaning:
  - every `.tf`/`.tf.json`/`.tofu` file in the dir parses as balanced blocks under `oracleMask`;
  - no `override*.tf*` file;
  - no `generate` block in any instantiating unit that could write a `.tf` file into it;
  - the module is instantiated by at least one seed.

**One-time cross-check against gruntled.** For each corpus, compare the model with HEAD
`graph --json` on the pristine tree: units, `module` per unit, edges with enabled/skip_outputs,
`known` modules, and references. Every difference is a row in a **pinned exception table**:
unit or module, the oracle's view, gruntled's view, and the reason, written by hand. That is the
`denisGraphOracleOnly` pattern from v0.2. The model then applies the table as a correction, so
per-mutation expectations stay exact. The table is invariant across mutations because unit
configs never change.

- **Exactly what gruntled is trusted for.** It is trusted only where a human has read and
  justified the row. One example is the 75 config-unknown denis256 units, which carry no edges
  (docs/cli.md Known limitations).
- **Expected table size.** iso20022 about 3 rows: the 3 `invalid-dependency` units. secret
  1 row: the `include-target` root. denis256 is unknown: units with `generate` blocks,
  include-target and dynamic paths may add tens of rows. **The first plan measures this** (§10
  risk R1).
- **Why the cross-check is not circular.** The check is "the model and gruntled agree, except
  for N hand-justified rows". The exclusions are audited. They are not silent. A module that
  gruntled wrongly marks unknown therefore shows up as a row someone must justify. It cannot
  quietly drop out of the mutation set.

**Expected result per mutation.** Here M is the mutated module, D the descriptor, Seeds(M) the
model's instantiating units and Rev the model's live reverse edges.

- `changes[]` has exactly one entry: module M, carrying exactly D's name-level lists and
  `type_changes` (kind, name and new value for the sentinel types; see §4.3).
- Broken: the units with model GRT004 sites (only for the referenced-output class), with
  findings exactly those sites, code GRT004, and no GRT001 anywhere new.
- Impacted:
  - distance 1 is Seeds(M) \ Broken;
  - distance k+1 is the BFS over Rev from Seeds(M), traversing Broken units without listing them;
  - `source` and `via` follow the canonical tie-break (smallest path at the first differing
    hop, read back from the unit). The test computes it with its own BFS plus a sorted
    predecessor choice, a few dozen lines in the cmd test. It is not imported from
    impact/oracle_test.go, which is package-internal anyway.
- Exit 1 only for the referenced-output class, else 0.
- The comparison is **exact set equality** on JSON. Text is checked once per class for its
  tokens and section; JSON is the authority.

### 4.2 How each mutation is generated

All mutations are byte edits found with the scanner from §3, and every edit is applied and then
restored in place on a single working copy per corpus. `corpusApply` returns the original
bytes, and the harness restores them and re-checks that file's sha256. This beats a fresh copy
per mutation: about 4,300 copies of 12 MiB is 50 GB of I/O. Every 100th mutation, and at the
end, a full `corpusDigest` of the working copy must equal the pristine copy's digest. Sites are
enumerated in sorted (module, file, byte offset) order. A name the generator introduces always
carries a sentinel prefix `gruntled_m17_` so it cannot collide with a corpus name.

| Class | Eligible sites | Edit | Expected D |
|---|---|---|---|
| M1 referenced output removed | each (module, output) with ≥ 1 model site that fires (plain, live, literal rows) | delete the output block's byte extent | `removed_outputs [y]`; GRT004 at **every** firing site (unit, file, line, col; message prefix `dependency "<l>" output "<y>" was removed from module "<M>"`); 0 GRT001 at those sites and 0 new GRT001 anywhere; guarded and silent sites: no finding; exit 1 |
| M2 unreferenced output removed | each output of an eligible module with no plain, guarded or other site anywhere | delete block | `removed_outputs [y]`; Broken 0; exit 0 |
| M3 required variable added | each eligible module (one per module) | append `variable "gruntled_m17_req" {}` to a new file `gruntled_m17.tf` | `added_variables [n]`, `type_changes [{variable_required, n, absent, required}]` |
| M4 default removed | each variable with a top-level `default` attribute (module-local regular file) | delete the `default` attribute's line span | `type_changes [{variable_required, v, optional, required}]`, no name change |
| M5 variable type changed | each variable whose type is absent or in the oracle's simple grammar (identifiers, `list/set/map/tuple/object/optional` with literal keys) | replace the `type` value with the sentinel `object({gruntled_m17 = string})`, or add it when absent | `type_changes [{variable_type, v, <old>, <sentinel canonical>}]`; `old` must be non-empty and differ (`any` when absent) |
| M6 output type declared and changed | each output of an eligible module (two-sided: no corpus output declares `type` except 1 on denis256) | base' adds `type = string`, cur' adds `type = number` | `type_changes [{output_type, o, string, number}]`. Also assert base' vs pristine is **empty** (a type declared on one side only is silent, BLAST-08) |
| M7 `sensitive` flipped | each output: absent → add `sensitive = true`; literal `true` → `false`; literal `false` → `true` | insert or replace | `type_changes [{output_sensitive, o, false/true, true/false}]` |
| M8 propagation depth ≥ 2 | not a separate edit: an assertion over every M1-M7 mutation whose module's cone reaches distance ≥ 2 (iso20022: s3_runtime and mq_broker; denis256: 160 modules, up to distance 5) | — | full Impacted with distance/source/via equals the model BFS. **Non-vacuity counters:** mutations with distance ≥ 2, at least 1 per corpus where the model allows it, and on denis256 at least one each at distances 3, 4 and 5 |
| M9 shared file (iso20022) | each variable in `iac.src/global.tf` (the symlink target) | M4/M5 edit on the target | one `changes[]` entry per module that links it (I measured 62 changes and 62 Impacted for `account` default removed) |

Notes:
- **M1 needs an inventory first.**
  - iso20022 has 4 such pairs (s3_runtime and mq_broker × role_name and region), with 22 sites,
    all plain at col 16 (the phase 4 oracle).
  - secret has 3 pairs and 4 sites.
  - denis256 has 99 pairs and 468 sites. Guarded and other shapes are likely there.
  
  If any site lands in "other", it goes into the exception table with its expected outcome
  derived by hand. This is the phase 4 "candidate list vs classification" lesson.
- **M1 dual oracle.** Run frozen v0.3.0 `check --format json` on the mutated tree. Its GRT001
  set, filtered to the removed (module, output), must equal HEAD's GRT004 set by
  (file, line, col, unit). Messages must be equal after rewriting `was removed from` to
  `is not declared by` (13-VERIFICATION truth 2 did exactly this by hand). v0.3.0 predates
  GRT004, so it cannot share a supersede bug. Cost: about 106 extra v0.3.0 runs.
- **M4.** iso20022 has **no module-local default** (all its defaults are in the shared
  `global.tf`, so it is covered by M9). secret has 4 and denis256 has 38.
- **M5.** A variable whose base type gruntled cannot read (non-literal, unparseable, or in
  conflicting duplicates) is legitimately silent (BLAST-10). The simple grammar keeps such
  variables out of the eligible set, and the count of excluded variables is logged and pinned.
- **M6, M7 and BLAST-10.** A name declared in two kept files with differing facts is unknown.
  The model flags duplicates per module and excludes them, with a counter.
- **Every mutation also re-asserts the non-vacuity guard from §3.** M stays known in
  `graph --json` and its names change exactly as D says, since gruntled reads names. This is a
  sanity check that the edit landed; it is not the oracle.

### 4.3 What stays non-circular

- **The change itself.** The expected `changes[]` comes from D, which the generator decided.
  It never comes from gruntled output.
- **Canonical type strings.** The oracle cannot render cty, and it must not import typeexpr.
  So it asserts `kind`, `name` and `new` (the sentinel's canonical string, hand-derived once
  and pinned, for example `object({"gruntled_m17"=string})` in whatever exact form TypeSig
  prints), plus `old != new` and `old != ""`. It also asserts `old == "any"` when the type was
  absent, and `old` equal to the source text for the bare primitives `string`, `number` and
  `bool`. Richer old types are compared only for inequality.
- **Seeds and edges** come from the model (§4.1) plus the hand-justified exception table.
- **Distance, source and via** come from the test's own BFS.
- **GRT004** comes from the textual site scan plus the frozen v0.3.0 binary.
- What remains trusted is that the pinned exception rows are correct. That trust is explicit,
  reviewed and pinned in docs/validation.md, as in v0.2.

### 4.4 Volume, runtime and memory

These are estimates from my counts with the regex inventory (module-local regular files of
eligible modules) and `graph --json`. The harness recounts.

| Class | iso20022 | secret | denis256 |
|---|---:|---:|---:|
| M1 referenced removed | 4 | 3 | 99 |
| M2 unreferenced removed | 392 | ~1 | ~376 |
| M3 required added | 62 | 3 | 818 |
| M4 default removed | 0 (+ M9) | 4 | 38 |
| M5 variable type | 75 (+ 6 M9) | 8 | ≤ 700 |
| M6 output type (two-sided, 2 runs) | 396 × 2 | ~2 × 2 | 475 × 2 |
| M7 sensitive | 396 | ~2 | 475 |
| **total runs** | **~1,730** | **~25** | **~3,460** |

- **Per run, measured with built binaries.** `blast --base` (which checks both trees) takes
  0.10 s on iso20022 and 0.51 s on denis256, at 13 and 25 MB max RSS. Run in process through
  `run([]string{"blast",…})` (the corpus-test pattern), it is a little faster, with no process
  spawn.
- **Serial wall time.** iso20022 about 3 min and denis256 about 30 min, plus v0.3.0 M1 runs
  (seconds) and §3 transforms (seconds). In total about **35 min on one core**. Peak memory is
  well under 200 MB: one tree pair at a time, reports dropped after each run.
- **`go test` timeout.** The default is 10 min, so the corpus run needs an explicit
  `-timeout 90m`. Sharding by class with `-run 'TestCorpusMutation17/<corpus>/<class>'` keeps
  each invocation under 10 min and lets a failure be re-run alone.
- **Optional speedup (only if needed): packing.** Apply mutations in K modules at once when
  their cones (seeds ∪ BFS ∪ referencing units) are pairwise disjoint. The expected result is
  then the union of the single expectations, with no attribution loss, because changes are per
  module and Impacted units are disjoint. A `GRUNTLED_M17_PACK=0` switch falls back to one at a
  time to localise a failure. I do **not** recommend it for the gate unless the serial run
  exceeds about 45 min.
- **Environment.** `GOMAXPROCS=2`, `-p=1` for this package, one corpus at a time, `free -m`
  before starting. Never alongside `go test ./...`, `-race` or the bound tests. Heavy RSS is
  not the issue here; wall time is. It fits the 8 GB WSL.

## 5. Through the daemon (`report --blast`, `rebase`)

**Recommendation: yes, a small targeted sample, not the full set.** Phase 16 already proved
`report --blast` ≡ `blast --base <copy>`:
- TestDaemonBlastEqualsBlastBase, a 200-case property (1,000 without `-short`);
- e2e on both transports;
- compat-v1.sh.

Re-running about 5,000 mutations through a daemon would only re-test `Between` and the
presenters, which are shared. The **corpus-specific risk is the watcher and incremental
path on real tree shapes**, which the property's generated trees do not have:
- 68 symlinks to a shared `global.tf` in iso20022 (the v0.3 symlink-alias cache gap, fixed in
  12-02);
- digit or editor-pattern directory names (the v0.3 DAEMON-01 gap, fixed in 12-01);
- denis256's 1,146 units and directory symlinks (`pr-3562/fixture2/t1`, `t2` → `../template`).

The sample:
- For each corpus, a built-binary `watch` on a filtered working copy, linux socket.
- For each class M1..M7, the first eligible site in sorted order, plus R8 (the composition) and,
  on iso20022, M9 (an edit to the symlink **target**, which must reach all 62 modules).
- After each edit, `waitBlastFinal`, then `assertBlastEquivalent` against
  `blast --base <pristine copy> <working copy>`. The helpers in daemon_blast_test.go use the
  exact-label normaliser with spelled-out labels from 16-04.
- Then revert and expect an empty view again. That is incremental == full on the way back.
- One rebase cycle per corpus:
  - mutate, then `rebase --expect <gen>`, which prints the accepted Broken count;
  - `report --blast` shows empty sets;
  - revert, and `report --blast` shows the inverse change, for example `+output y` for M1.

Size: about 10 edits × 3 corpora, each about 1-2 s (debounce plus reindex: 0.5 s on denis256).
In total about 1-2 min and one daemon at a time (denis256 daemon heap is roughly 2.4 × one
Report; the DAEMON-10 record is about 2.3 KB/unit, so a few MB). Windows (report dump path) is
covered by phase 16 CI and is not repeated on the corpora. Cleanup follows compat-v1.sh:
`trap … EXIT INT TERM HUP`, kill the daemon, `pgrep` must be empty, and `timeout` on every
client call.

## 6. Cross-phase audit (SC4)

**Who.** The same two-part shape as the v0.3 audit, which caught two silent divergences that
four phase-level verifications had passed:
1. **gruntled-sec**: an independent security review across the blast core, the socket ops and
   output escaping (SC4's three named areas), over `git diff v0.3.0..HEAD -- ':!.planning'`.
   The verdict goes in `17-SECURITY.md` (the phase file sec already produces). Any BLOCKER or
   MEDIUM is closed **in this phase** (SC4), as phase 12 did for v0.3.
2. **gsd-integration-checker**: wiring and E2E flows across 13→14→15→16. It writes a
   `17-INTEGRATION.md` (or the orchestrator embeds its report in the audit). `/gsd-audit-milestone`
   spawns the checker again anyway. The phase run proves the fixes, and the milestone audit
   re-confirms them.

**Invariants that span phase boundaries** (the audit's checklist; each with how to check it):

| Invariant | Spans | Check |
|---|---|---|
| GRT004 only reclassifies GRT001: one finding per site, never both, never adds; only `Between` creates it; never in check/report/graph/status | 13 → 14 (Broken traversed), 16 (daemon view, baseline never holds GRT004, T-13-02-7) | grep `CodeRemovedOutput` (only grt004.go), TestBetweenOnlyReclassifiesGRT001, TestNoGRT004OutsideBlast, corpus M1 dual oracle |
| Type facts are never Broken, never a diagnostic, never change the exit code | 15 → 14 (type-only seeds propagate), 16 (rebase prints the Broken error count, `HasErrors` from Broken only) | TestTypeFactsNeverChangeBrokenProperty, corpus M3-M7 exit 0, `report --blast` exit on M3 = 0 |
| Depth/paths: BFS over live block edges only, canonical tie-break, `--depth 1` = v0.3, Broken traversed but not listed | 14 → 15 (type-only seeds), 16 (`report --blast` refuses `--depth`, exit 2) | oracle tests, TestBlastDepth1MatchesV1, corpus M8 |
| Baseline equivalence: daemon view = `blast --base <copy>`; baseline = Report only; rebase installs the last published Report; atomic commit | 16 over 13-15 | TestDaemonBlastEqualsBlastBase, TestBaselineHoldsOnlyReport, corpus daemon sample (§5) |
| Escape pipeline: every new text string goes through `escapeTerm`, JSON through `escapeJSON`, the renderer through `strconv.Quote`; refusal text through `SanitizeReason` | 13 (GRT004 message) → 14 (path hops) → 15 (type tokens and section) → 16 (label, refusals) | TestTextOutputsEscapeControls (blast, blast_json, report --blast ×2), grep for presenter writes without escape |
| for-expression guard class: every `Value`/typeexpr evaluation site is preceded by `HasForExpr` or `hasForKeyword` | 15 (F1/F2/F5) → 16 (no new site) → 17 | `grep -rnE '\.Value\(\|typeConstraint\(' internal cmd --include=*.go \| grep -v _test.go` must give exactly the 4 known lines: `hclconv/literal.go`, `tfsurface/typesig.go`, `terragrunt/eval.go` ×2. Any new line is a finding |
| Bounded cost: blast/report --blast O(units+edges+surface); 64 MiB cap; seal once per publish | 15 (BLAST-11) → 16 | TestBlastSizeBound, TestReportBlastSizeBound (CI), TestBlastRequestAllocsConstant |
| Regressions vs v0.3.0 | all | compare-ref.sh v0.3.0 (244 → 256 with corpora), compat-v1.sh 7/7, blast-snapshots.sh, corpora 12/12 |
| No-net/no-exec on 6 targets; HCL only in infrastructure; typeexpr only in tfsurface | 15 → final tree | check-architecture.sh (Step 6, 6b, 9), test-check-architecture.sh |
| Docs = shipped behaviour (GRT004, `--depth`, `report --blast`, `rebase`) | 13-16 | TestRuleRegistryDoc, TestHelpMatchesDocs, TestDaemonDocs, TestCIDoc, TestValidationDocPins, README pins |

**Output.**
- `17-SECURITY.md`: sec's verdict, STRIDE rows of the phase plans, and cross-phase findings
  with dispositions.
- An integration report.
- Fix commits for anything BLOCKER or MEDIUM.
- The final gate log in `17-VERIFICATION.md`.

`/gsd-audit-milestone` then writes `.planning/v0.4-MILESTONE-AUDIT.md`. That is a milestone
step, not a phase plan.

## 7. Milestone closure prerequisites and leftover tech debt

Order: phase 17 executed → verify → secure → validate → `/gsd-audit-milestone` → (gap closure if
needed) → `/gsd-complete-milestone`. **Tagging is out of scope.**

1. **`git.create_tag`.** The gsd-core default is `true` (bin/lib/config.cjs:89), and
   `.planning/config.json` has no `git` key, so complete-milestone would create `v0.4` by
   default. Before complete-milestone, run
   `node ~/.claude/gsd-core/bin/gsd-tools.cjs query config-set git.create_tag false` (or decline that step). This is the
   orchestrator's or Giulio's call. The v0.4.0 tag stays Giulio's decision (memory: "push on
   master authorised, tag no").
2. **SUMMARY `requirements-completed` gaps.** audit-milestone cross-checks REQUIREMENTS,
   VERIFICATION and SUMMARY frontmatter. **BLAST-06, -07, -08, -09 and -10 appear in no SUMMARY's
   `requirements-completed`**: 15-01..15-04 all have `[]`, and only 15-05 claims SEC-01 and
   BLAST-11. The audit would report them as partial or orphaned. Fix: add BLAST-06..10 to
   15-03/15-04 (whichever completed each), the same way the v0.3 audit had to fix 09-03.
3. **VALIDATION status.** 13 and 14 are `validated`. **15 is `approved`, with
   `wave_0_complete: false`, and 16 is `executed`**. audit-milestone counts only
   `status: validated` as compliant. Run `/gsd-validate-phase 15` and `16` (and later 17) before
   the audit.
4. **gsd-tools verify-summary false positives.** These are `verify.cjs` heuristics, not product
   gaps, and they are cosmetic.

   | SUMMARY | What triggers it | Fix |
   |---|---|---|
   | 15-02, 13-01, 13-02 | Any backticked token with a dot, so `` `bash scripts/check-architecture.sh` `` is read as a missing file | Write commands in fenced blocks, or split them as `bash` plus a separate `` `scripts/…` `` |
   | 13-03 | `svc/api/common.tf -> ../../shared/common.tf` | Same fix |
   | 16-00 | `$RUNNER_TEMP/fuzz.log` | Same fix |
   | 16-04 | `modules/big/main.tf` (a test fixture path) | Same fix |
   | 13-01, 13-02, 15-00 | "Self-check section indicates failure": any `fail` anywhere after `## Self-Check` (for example in deviation text) | Move the Self-Check heading last, or accept |

   14-*, 15-01, 15-03..05 and 16-01..03 pass. **The 15-02 lines are 179 and 206.** I recommend
   a one-commit docs fix; the orchestrator may also simply acknowledge.
5. **run.go unreachable republish branch** (`internal/infrastructure/watch/run.go:128-131`).
   - With the current debouncer, `unpublished && !pend` is never true when a Ready arrives
     (16-02 deviation 2). TestRunSpuriousReadyAfterSkipEndsPublished shows it.
   - 16-SECURITY F3 (INFO) found no risk to T-13-02-7 or rebase.
   - Options: (a) keep it as defence in depth, with a comment naming the debouncer invariant
     that makes it unreachable today and the test that shows it, accepted in the milestone
     audit; (b) delete it, which removes the stale-status guard if the debouncer ever changes.
   - **Recommend (a).** A product-code change in a proof phase widens the audit diff for no user
     gain, and sec advised against a test pinning debouncer internals.
6. **Windows CI flakes.**
   - `TestRebaseWindowsUnsupported/report-file_daemon` failed once (run 38227051604) on code that
     passed elsewhere.
   - c083bef (on master) now reads metadata with `os.Lstat` and prints a line diff. That
     addresses the NTFS lazy directory-entry metadata cause, and the windows job on c083bef
     (run 38228443297) passed.
   - CI on d99714e (run 38229787854) was still running at research time.
   - What is left: confirm stability with repeated windows runs, for example a
     `workflow_dispatch` re-run ×5 or `-count=10 -run TestRebaseWindowsUnsupported` in one
     windows job. If it still flakes, compare names and sha256 only and drop mtime for held
     files on windows. That is safe because the behaviour (exit before any filesystem access,
     rebase.go:68-71) is proven by code order and by the stable `no runtime dir` subtest.
   - The other windows failures in the 16 window (`rebase_usage`, the held lock) were real test
     bugs fixed in 40dd3b2.
   - Phase 17 needs green CI to rely on, so this belongs in an early wave.
7. **Other carried items for the audit's `tech_debt`.** These are already accepted. Phase 17
   must not fix them unless the audit escalates them.
   - 15-SECURITY's v0.3.1 hand backport (the HasForExpr guard in v0.3.0 `eval.go` `literalBool`,
     `evalPath`, `literalString`; 15-00 cherry-picks cleanly). That is a release decision for
     Giulio.
   - 16-SECURITY M18: the EventFailed copy is caught only under `-race`.
   - The RSS bound in TestBlastSizeBound is loose (15 F4).
   - rebase.go:123 has a pre-send `%v` without SanitizeReason (INFO, local errors only).
   - ROADMAP and STATE bookkeeping drift that 16-VERIFICATION noted. ROADMAP now shows 16 complete,
     but STATE's "Current focus" line still says Phase 13.

## 8. Architecture constraints for the new code

- New tests go in `cmd/gruntled` (package main), env-gated, reusing the corpus helpers.
  - No `hashicorp/hcl`, `zclconf/go-cty` or typeexpr import, even in tests: check-architecture
    Step 6 and 6b scan sources, test files included. The generator and oracle are textual.
  - Interfaces/domain/application test allowlists do not apply in cmd. They are the reason
    earlier phases used in-test xorshift; here the enumeration is sorted and needs no RNG.
- `scripts/compare-ref.sh` changes keep these properties:
  - `set -euo pipefail`, the trap, the offline env and `--end-of-options`;
  - no Go file under scripts/ (sec #151);
  - corpus dirs are never written (digest before and after).
- No product code changes are expected, except a fix that the audit forces. compare-ref and
  the doc-sync tests must stay green on the final tree.
- docs/validation.md gets `## v0.4: blast on the real corpora (REL-03)`, with the pinned
  commits, class counts, the exception table, the recorded run and the command. A
  TestValidationDocPins extension pins the heading, the counts and each exception row, as v0.2
  did with `denisGraphOracleOnly`.

## 9. Validation Architecture

| Req / SC | Property | Test (new unless noted) | Where it runs | Command |
|---|---|---|---|---|
| REL-03 / SC1a | check text/json/sarif + graph --json byte-identical to v0.3.0 on 3 corpora | `scripts/compare-ref.sh v0.3.0` with `COMPARE_REF_CORPORA` (extended); perturb self-test | local gate (needs corpora); fixture half in CI as today | `COMPARE_REF_CORPORA="…" bash scripts/compare-ref.sh v0.3.0` → `256/256 identical` |
| REL-03 / SC1b | self-blast empty (same dir and copy, text+json) | `TestCorpusBlastSelf` | local, env-gated | `go test -count=1 -run TestCorpusBlastSelf ./cmd/gruntled` |
| REL-03 / SC1 oracle | model == graph --json modulo pinned exceptions | `TestCorpusBlastModel` (one per corpus) | local, env-gated | `-run TestCorpusBlastModel` |
| REL-03 / SC2 | R1-R9 give empty changes/Impacted/Broken, exit 0; non-vacuity (names/known unchanged, counters > 0) | `TestCorpusReformat/<corpus>/<R>` | local, env-gated | `-run TestCorpusReformat` |
| REL-03 / SC3 | M1-M7, M9 exact vs model; M8 counters; M1 dual oracle vs v0.3.0 | `TestCorpusMutation17/<corpus>/<class>` | local, env-gated, serial | `GOMAXPROCS=2 go test -p=1 -count=1 -timeout 90m -run 'TestCorpusMutation17/denis256/M3' ./cmd/gruntled` (per shard) |
| harness self-test | the oracle and harness can fail | recorded mutants: (a) a gruntled stub dropping one Impacted unit, (b) an off-by-one distance, (c) a reformat that breaks parse (must trip the non-vacuity guard), (d) an M1 with GRT001 left in; each must fail. Run as a unit test over a small synthrepo or a txtar tree, **always on, in CI** | CI | `-run TestCorpusHarnessSelfTest` |
| daemon sample | report --blast ≡ blast --base on corpus edits; rebase cycle | `TestCorpusDaemonSample` (linux) | local, env-gated | `-run TestCorpusDaemonSample` |
| SC4 | six-target proof, arch rules, doc pins | existing | CI + local | `bash scripts/check-architecture.sh`, `bash scripts/test-check-architecture.sh`, `go test -run 'TestRuleRegistryDoc\|TestHelpMatchesDocs\|TestDaemonDocs\|TestValidationDocPins\|TestCIDoc' ./cmd/gruntled` |
| SC4 | no open BLOCKER/MEDIUM | sec cross-phase audit + integration checker | review | 17-SECURITY.md verdict |
| regression | compat matrix, depth-1 snapshots | existing | local / CI | `bash scripts/compat-v1.sh v0.3.0`, `TestBlastDepth1MatchesV1` |

Sampling rate: the env-gated corpus tests are the gate. The harness self-test and every
existing test run in CI on each push. Nyquist: each REL-03 clause has at least one automated
command, and the self-test proves the corpus assertions can fail.

## 10. Candidate plan breakdown (≤ 3 tasks each)

| Plan | Wave | Content | Depends |
|---|---|---|---|
| 17-00 | 1 | **Tech debt / bookkeeping.** (T1) windows flake confirmation and hardening if still needed; (T2) SUMMARY frontmatter `requirements-completed` for BLAST-06..10, the verify-summary false-positive wording fixes, STATE current-focus fix; (T3) run.go republish branch comment (option a) | — |
| 17-01 | 1 | **Corpus gate part 1 + model.** (T1) compare-ref.sh corpus extension + perturb row; (T2) `TestCorpusBlastSelf`; (T3) the independent model (seeds, live edges, reference sites) + `TestCorpusBlastModel` + pinned exception tables (measure denis256 first) | — |
| 17-02 | 2 | **Generator + harness self-test + reformat.** (T1) textual scanner/editor (block and attribute extents, in-place apply/restore, digest guard) + `TestCorpusHarnessSelfTest` (always on); (T2) R1-R9 + non-vacuity guards | 17-01 |
| 17-03 | 3 | **Mutation set.** (T1) M1 (dual oracle with v0.3.0) + M2; (T2) M3-M7 + M9; (T3) M8 counters, sharding, recorded serial run | 17-02 |
| 17-04 | 3 | **Daemon sample + docs.** (T1) `TestCorpusDaemonSample`; (T2) docs/validation.md REL-03 section + TestValidationDocPins | 17-02 (T1); 17-03 for counts (T2) |
| 17-05 | 4 | **Cross-phase audit + final gate.** (T1) sec cross-phase security review and integration checker → findings; (T2) close any BLOCKER/MEDIUM (TDD); (T3) final gate: six-target, arch self-tests, doc pins, compare-ref 256/256, compat-v1, corpus suite, CI green | all |

17-00 and 17-01 can run in parallel (different files). Only one heavy corpus run at a time.

### Threat seeds for the plans' threat models

- **T: a corpus or harness writes outside the temp dir** (Tampering). Mitigate with
  `corpusDigest` before and after on each checkout, `t.TempDir` only, the apply/restore sha256
  check, and no `os/exec` of anything from the corpus.
- **T: the oracle is circular** (Repudiation of the proof). Mitigate: the model shares no code
  with internal/; the exception table is hand-justified and pinned in docs; M1 has the v0.3.0
  dual oracle; the harness self-test shows each assertion can fail.
- **T: vacuous pass**, for example a broken reformat makes a module unknown. Mitigate with the
  non-vacuity guards (known flag, unchanged names, no new GRT100, counters > 0).
- **T: hostile corpus content** (DoS / escape). The pinned corpora are third-party input. The
  phase 15 for-expression and depth guards apply, so that is accepted, with a timeout per run.
  Never execute corpus files, which matters because denis256 ships ELF binaries; the filtered
  copy drops them.
- **T: a network fetch replaced by a different commit** (Spoofing). Mitigate: SHA pins checked
  by the require helpers, fail rather than skip.
- **T: the daemon sample leaks a process or deletes the tree under a live daemon.** Mitigate
  with the compat-v1.sh cleanup pattern (16 sec F8).
- **T: compare-ref corpus extension** (Tampering / injection). Mitigate: dirs passed as
  arguments, quoted, read-only, never extracted or executed.

### Risks

- **R1. The denis256 exception table may be large**, from 75 config-unknown units,
  include-target, `generate` blocks and dynamic sources. If it passes about 50 rows, restrict
  denis256 to the subset of modules whose seeds and cones are fully oracle-resolved, and log the
  excluded count. A smaller universe is better than a table no one can review. Measure in
  17-01 T3 before committing to exhaustive denis256.
- **R2. Unusual reference shapes in denis256 M1** (guarded, whole-object, index forms). The
  "other" bucket needs hand rows. A missing shape is a false negative of the oracle, so
  cross-check sites against the v0.3.0 GRT001 set as well.
- **R3. The R5 converter is incomplete.** Converting only simple blocks keeps it honest. A
  counter > 0 prevents a vacuous pass. iso20022 and secret have single-line values almost
  everywhere.
- **R4. Runtime of about 35 min.** It needs `-timeout 90m` and sharding. Packing is the
  fallback.
- **R5. CI flake on windows blocks the "CI green" gate.** Handle it in 17-00 first.
- **R6. The corpora are absent locally.** The executor needs a one-time fetch (network) before
  17-01.
- **R7. Corpus licence and size.** Never commit corpus content. Fixtures stay hand-written, and
  only `file:line:col` strings go into docs (allowed by v0.1 precedent).

## 11. Decisions to confirm (orchestrator)

1. Exhaustive mutation set, env-gated and run serially at the gate. Not in CI (§12 Q1).
2. A one-time independent model with a pinned hand-justified exception table (v0.2 pattern),
   plus the v0.3.0 dual oracle for M1.
3. Assert `changes[]` empty for the reformat transforms (stricter than REL-03's "empty Impacted").
4. Two-sided injection for `optional()` defaults (R4) and output types (M6), because the
   corpora contain neither.
5. A targeted daemon sample (§5), not the full set.
6. Audit by gruntled-sec (security) plus gsd-integration-checker (wiring), with BLOCKER and
   MEDIUM closed in phase.
7. run.go republish branch: keep it and add a comment (option a).
8. Set `git.create_tag false` before `/gsd-complete-milestone`.

## 12. Open questions

1. **CI for corpora?** v0.1 locked corpus tests out of CI and ci.yml. A `workflow_dispatch`
   (or weekly) `corpus` job could fetch the three SHAs and run the about-35-min suite on a CI
   runner, which removes the local WSL load. Should it be added in this phase, or should the
   local recorded run stay the gate?
2. **The size of the denis256 universe** if the exception table is large (R1). Should a
   restricted, fully resolved subset with a logged exclusion count be accepted for REL-03's
   "caught 100%"?
3. **Does "caught" mean exact equality** (my recommendation: no missing and no extra Impacted,
   exact distances and paths), or only "the injected change appears"? Exact equality is what
   rules out noise. It is also what makes the oracle work necessary.
4. **Where do the canonical sentinel strings come from?** `new` in M5 is TypeSig's output for a
   hand-chosen sentinel. Is it acceptable to pin it once from HEAD output after hand
   verification, or must it be derived by hand from the D-15 rendering rules alone?
5. **Should the corpus fetch be a committed script** (`scripts/fetch-corpora.sh`, network, SHA
   checks) or stay as docs-only steps (the v0.1/v0.2 practice)?
6. **The v0.3.1 backport** (15-SECURITY F2/F5 and 15-00) is not phase 17 scope. Should it be
   listed in the milestone audit as a release follow-up for Giulio?
