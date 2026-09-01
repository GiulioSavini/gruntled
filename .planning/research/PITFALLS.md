# Pitfalls Research: gruntled (Terragrunt static analyser)

**Domain:** Static analysis of Terragrunt/Terraform configuration (wiring correctness, not cloud validation)
**Researched:** 2026-09-01
**Confidence:** HIGH for Part A (every repository below was cloned and inspected directly, not guessed from search snippets). MEDIUM-HIGH for Part B (grounded in real code found during the Part A survey, official HCL/Terragrunt docs, and one verified upstream issue/PR pair).

---

## PART A — Test Corpus (read this first — it blocks the v0.1 experiment)

### Method

Every repository below was `git clone`d into a scratch directory and inspected with scripts, not just read about. For each candidate I resolved `dependency.X.config_path` to the *target unit*, then resolved that unit's `terraform.source` to the *actual module directory* (local sources only), and diffed every `dependency.X.outputs.Y` reference against the `output` blocks actually declared in that module's `.tf` files. This two-hop resolution matters — see Pitfall 1 below, which exists because my first, naive, one-hop version of this script produced false positives on real code.

### Ranked candidates

**1. `aws-solutions-library-samples/guidance-for-iso20022-messaging-workflows-on-aws`** — best fit for the experiment
- URL: https://github.com/aws-solutions-library-samples/guidance-for-iso20022-messaging-workflows-on-aws
- Licence: MIT-0 (confirmed via GitHub API `license.spdx_id`)
- Units: **65** directories containing `terragrunt.hcl`
- Last push: 2026-04-21 (verified via `pushed_at`, not just the shallow-clone tip)
- `dependency`/`outputs`: **62** files contain `dependency "X" { ... }` blocks; **22** `dependency.X.outputs.Y` references; **11** files use `mock_outputs` (with `mock_outputs_merge_with_state` and `mock_outputs_allowed_terraform_commands`)
- Local vs remote modules: **entirely local, and unusually so** — no unit has a `terraform { source = ... }` at all. Each unit directory contains its `.tf` files directly alongside `terragrunt.hcl` (inline module pattern). This means module-surface extraction requires **zero path resolution** — the module is the unit directory. This is the single best property this repo has for a v0.1 test: no `source` resolution logic is even exercised, so a failure to find a genuine bug can't be blamed on source-path handling.
- HCL functions used: `find_in_parent_folders()` (inside `after_hook.execute`, a side-effecting hook — gruntled must not execute it, only read the path expression), `after_hook` blocks (side-effecting at Terragrunt runtime, irrelevant to static analysis but must parse without error)
- **Verified clean**: I cross-checked all 22 `outputs.*` references against the real `output` blocks in every target unit. Zero mismatches. This repo is *currently* wiring-correct — useful as the "zero false positives" half of the experiment, but it will not hand you a naturally occurring bug (see "Where the genuine bug comes from" below).

**2. `cds-snc/secret`** — best secondary corpus, actively maintained
- URL: https://github.com/cds-snc/secret
- Licence: MIT
- Units: 4 (`terragrunt/acm`, `terragrunt/ecr`, `terragrunt/lambda`, root)
- Last push: **2026-09-01** (pushed the same day as this research — genuinely live, not abandoned)
- `dependency`/`outputs`: 2 files with `dependency` blocks, both also use the lighter `dependencies { paths = [...] }` block (ordering-only, no outputs) alongside real `dependency.X.outputs.Y` references; both dependency blocks use `mock_outputs` + `mock_outputs_allowed_terraform_commands = ["plan-all", "validate"]`
- Local vs remote: local — `terraform { source = "../../aws//lambda" }` style relative double-slash paths
- Notable: root `terragrunt.hcl` uses two `generate` blocks (`provider`, `common_variables`) that inject a `provider.tf` and a `.tf` file containing **`variable` blocks with no quotes around the label** (`variable account_id { ... }`) — see Pitfall 1. Confirmed clean of GRT001 issues after accounting for unquoted labels.
- Too small on its own to be the headline "large repository" demonstration, but its small size makes it the easiest one to hand-mutate for the "genuine bug" requirement (see below), and it independently confirms the `generate`+unquoted-label findings from repo #1.

**3. `gruntwork-io/terragrunt-infrastructure-catalog-example`** — reference architecture, but for the *newer* Stacks feature, not classic units
- URL: https://github.com/gruntwork-io/terragrunt-infrastructure-catalog-example
- Licence: MPL-2.0. Stars: 55. Last push: 2026-07-06.
- Structure: `units/*` (12 standalone unit templates, each with `terraform { source = "../..//modules/X" }` pointing at real local `modules/*` with real `.tf` outputs) and `stacks/*` (`terragrunt.stack.hcl` files that wire units together via `dependency` blocks nested inside `autoinclude { }`, e.g. `dependency.db.outputs.db_security_group_id`)
- Critical finding: **the `dependency` blocks with real `outputs.*` references live in `.stack.hcl` files, not in any `units/*/terragrunt.hcl` on disk.** A unit's own `terragrunt.hcl` (e.g. `units/mysql/terragrunt.hcl`) has no `dependency` block at all — Terragrunt injects it at `terragrunt stack generate` time. See Pitfall 2 (Terragrunt Stacks) — this repo is a good source for *future* Stacks-aware fixtures, not for the v0.1 classic-unit walk.

**4. `gruntwork-io/terragrunt-infrastructure-live-example`** — the famous one, and a trap
- URL: https://github.com/gruntwork-io/terragrunt-infrastructure-live-example
- Licence: Apache-2.0. Stars: 863. **Archived: true** (confirmed via API — the repo is now read-only, superseded by the Stacks example above).
- Units: 6 (`{qa,stage,prod} × {mysql, webserver-cluster}`)
- **`dependency`/`outputs`: zero.** I checked the *entire* git history (86 commits back to the 2016 initial commit, including all reachable commits, not just the current tip) — this repository has never, on its main line of history, contained a `dependency` block referencing another unit's outputs. (Three commits do contain `dependency "` on an abandoned experimental branch, `yori-dry-experiment`, from 2020-2021, never merged.) Each unit deploys independently.
- **This is the repository every blog post and every "terragrunt example" search result points to, and it is useless for testing GRT001.** This is worth stating explicitly in the roadmap so nobody spends time pointing gruntled at it expecting a hit.

**5. `gruntwork-io/terragrunt-infrastructure-live-stacks-example`** — the current official reference, and structurally unusable by v0.1
- URL: https://github.com/gruntwork-io/terragrunt-infrastructure-live-stacks-example
- Licence: MPL-2.0. Stars: 116. Last push: 2026-07-07.
- **Zero `terragrunt.hcl` files anywhere in the git checkout.** The repo contains only `terragrunt.stack.hcl`, `account.hcl`, `region.hcl`, `root.hcl`. Units are materialized by `terragrunt stack generate` into `.terragrunt-stack/`, which is listed in `.gitignore`.
- Consequence: a tool that discovers units by walking the tree for `terragrunt.hcl` (as gruntled's design defines "Unit") finds **nothing** in this repo unless someone has already run `terragrunt` locally, leaving an untracked `.terragrunt-stack/` directory on disk. Do not use this as a v0.1 corpus. Flag explicitly for later Stacks support (see Pitfall 2).

**6. `khuedoan/cloudlab`** — real, popular, but wrong shape for this experiment
- URL: https://github.com/khuedoan/cloudlab
- Licence: GPL-3.0 (note if ever vendoring anything from it — fine as a read-only external analysis target, not for copying code). Stars: 53. Last push: 2026-08-03.
- 10 units, 5 with `dependency` blocks, 13 `outputs.*` references, **all module sources are remote or computed via `${find_in_parent_folders(...)}`** — a dynamic, non-literal `source` expression. Every dependency in this repo resolves to `unknown` under gruntled's own stated policy (unresolvable source → mark unit unknown, skip dependent checks). Good as a *negative* test case for "does gruntled correctly stay silent here" but produces zero GRT001 signal by construction.

**7. `denis256/terragrunt-tests`** — not a corpus, but the best fixture-mining source found
- URL: https://github.com/denis256/terragrunt-tests
- Licence: MIT. 3,474 files, hundreds of scenario directories, each reproducing a specific upstream Terragrunt GitHub issue (`issue-2163`, `issue-910`, `issue-2405`, `issue-2718`, etc.) or a specific behaviour (`cycles/`, `broken-locals/`, `mock-output/`, `module-mock-output/`, `autoinclude-bugs/case1-object-key-leak`, `autoinclude-bugs/case2-dead-branch-sideeffect`, `panics/01-stack-numeric-interpolation-panic` through `panics/07-deep-nested-quadratic-hang`, `manifest-traversal/attacker-*`, `skip_outputs/`).
- This is **not a live infrastructure repository** — it's a maintained bug-reproduction corpus, and using it as "the real public Terragrunt repository" for the milestone's success criterion would be a stretch of the spirit of the requirement. But it is exactly the raw material the design document already asks for under "Golden tests over generated and hand-written fixtures" (M1 checklist) and for the `panics/` robustness testing implied by "a file HCL salvato a metà produce una diagnostica di sintassi, mai un panic." Mine it for fixtures; don't cite it as the falsifying repo.

### Where the genuine bug actually comes from

None of the actively maintained real repositories surveyed (aws-iso20022, cds-snc/secret) currently contain a live `dependency.X.outputs.Y` mismatch — which is expected: repos that are applied regularly get their wiring bugs caught by `terraform apply` failing loudly, so surviving bugs of this exact shape are rare in maintained code. Three practical paths, in order of how well they satisfy "a real public Terragrunt repository":

1. **Recommended: clone a real repo (aws-iso20022 or cds-snc/secret) and apply one deliberate, documented, single-line mutation** — rename or delete one `output` block in the target module, leaving the `dependency.X.outputs.Y` reference pointing at the now-missing name. The repository, its dependency graph, and 99% of its content are unmodified and real; only the one line that creates the bug is synthetic. This is standard mutation-testing practice and produces exactly the "genuine wiring error `terragrunt hcl validate` does not report" scenario the milestone needs, reproducibly and byte-for-byte matching what a real regression looks like. Confirm with the roadmap owner that this satisfies the letter of "real public repository," since the bug itself is injected — functionally it is indistinguishable from an organic regression.
2. Search further and wider (only 10 unique repositories surfaced from GitHub code search for `"dependency" "outputs" filename:terragrunt.hcl` on the first two pages; a systematic scan of the next several pages, plus repos using `dependency` blocks without the literal word "outputs" nearby, was not exhausted in this pass — treat the ranked list above as a strong starting set, not an exhaustive one).
3. Watch for a repo where an output was recently renamed in one commit but a downstream `dependency.X.outputs.Y` reference in a *different* unit wasn't updated in the same commit — this is the real-world shape of the bug GRT001 targets, and it is more likely to be found by scanning commit diffs across many repos than by reading any single snapshot.

### Verified, and worth correcting in `PROJECT.md`

`PROJECT.md` cites [gruntwork-io/terragrunt#5811](https://github.com/gruntwork-io/terragrunt/issues/5811) as evidence that "Terragrunt's module-output probe... is currently broken." I checked: **the issue is closed**, fixed by [PR #5827](https://github.com/gruntwork-io/terragrunt/pull/5827), merged 2026-04-10, released in v1.0.1 and present in the current v1.1.4. This is good news for gruntled's positioning, not bad news — read the fix carefully:

> "The fix provides a dynamic placeholder for dependency outputs (and inputs) during validation so that attribute access evaluates to **unknown** rather than failing."

`terragrunt hcl validate` no longer *crashes* on `dependency.X.outputs.Y`, but by design it treats **every** `outputs.*` access as an opaque unknown value — it still does not, and structurally cannot without applying dependencies, check that `Y` is a real output of the target module. GRT001 is not competing with a broken probe that might get fixed out from under it; it is filling a gap the fix explicitly declined to close. Update the `PROJECT.md` context table to reflect the current (fixed, but still non-validating) state.

---

## PART B — Domain Pitfalls

### Pitfall 1: Unquoted HCL block labels are valid and appear in real code

**What goes wrong:**
A module surface extractor that assumes `output "name" { ... }` (quoted label) will silently miss `output name { ... }` (bare identifier label) — and report a real output as missing, which is exactly the false positive the zero-false-positive rule forbids.

**Why it happens:**
The HCL native-syntax grammar explicitly permits either form for block labels: `Block = Identifier (StringLit|Identifier)* "{" Newline Body "}" Newline;` (confirmed against the [hashicorp/hcl spec](https://github.com/hashicorp/hcl/blob/main/hclsyntax/spec.md)). Terraform's own convention and `terraform fmt` output nearly always show quoted labels, so most developers — and most regex-based or hand-rolled extractors — assume quotes are mandatory. They are not. I found this pattern in production code twice independently during the corpus survey: `cds-snc/secret/aws/acm/outputs.tf` (`output domain_cert_arn { ... }`, no quotes) and throughout `denis256/terragrunt-tests` (`output data { value = "module1" }`).

**How to avoid:**
Extract the module surface using `hashicorp/hcl/v2`'s actual parser (`hclsyntax.Body.Blocks`, reading `Block.Labels[0]`), never regex. The design document already commits to `hashicorp/hcl/v2` for this reason — the risk is specifically in any interim quick script, test fixture, or fallback path that reaches for a regex "just this once."

**Warning signs:**
Any code path in `tfsurface` that uses `regexp` instead of the HCL AST. A golden fixture with an unquoted `output`/`variable` label that passes today but wasn't deliberately added as a regression test.

**Phase to address:** M1 (Module Surface reader). Add an unquoted-label fixture to the golden test set before shipping GRT001.

---

### Pitfall 2: Terragrunt Stacks make "Unit = directory containing terragrunt.hcl" wrong for a growing share of new repositories

**What goes wrong:**
Gruntled's ubiquitous language defines a Unit as "directory containing `terragrunt.hcl` that invoca un modulo." Terragrunt Stacks (`terragrunt.stack.hcl`, generating `unit { source = ... }` blocks) is now the *officially recommended* pattern — both `terragrunt-infrastructure-live-stacks-example` and `terragrunt-infrastructure-catalog-example` (Gruntwork's current reference architectures, replacing the deprecated 863-star classic example) use it exclusively. A repository built this way has **zero `terragrunt.hcl` files checked into git** — they are generated into `.terragrunt-stack/` (conventionally `.gitignore`d) only after `terragrunt stack generate` runs. A purely static, read-only, no-external-process walk of such a repo — gruntled's core constraint — finds no units, no dependencies, nothing to check, and produces zero diagnostics. That's not a false positive, but it is a silent, total, and non-obvious failure to do anything at all, on repos that are increasingly the canonical way to write Terragrunt.

**Why it happens:**
Stacks is new enough (2026) that it postdates most existing tutorials and Q&A content on the topic, but it's already what Gruntwork ships as its own best-practice examples. `dependency` blocks in a Stacks repo live nested inside `unit { autoinclude { dependency "X" { config_path = unit.Y.path ... } } }` inside `.stack.hcl` — a materially different grammar shape from a classic unit's top-level `dependency` block.

**How to avoid:**
Out of scope for v0.1 by design (the design doc's "Unit" definition is deliberately classic-Terragrunt-only), but the roadmap should say so explicitly rather than let it be discovered by a confused early user. Two concrete things to do now: (1) detect the presence of `.stack.hcl` files during indexing and emit an informational note ("N stack files found, not analyzed — Terragrunt Stacks support not yet implemented") rather than silently reporting zero units as if the repo were clean; (2) do not silently treat a stacks-shaped repo as "verified clean" in any demo or benchmark — it was never analyzed.

**Warning signs:**
A demo or benchmark repo that reports zero diagnostics. Before trusting that as "clean," check whether it's actually "not analyzed" — `find . -name terragrunt.hcl | wc -l` should be non-zero.

**Phase to address:** Flag now in `PROJECT.md`/roadmap as an explicit out-of-scope-for-v0.1 note with a warning message, not silence. Full Stacks support (parsing `.stack.hcl`, resolving `unit.X.path`, nested `autoinclude`) is a `Later` milestone; `denis256/terragrunt-tests/autoinclude-bugs/` and `stacks-test/` are ready-made fixtures for it when the time comes.

---

### Pitfall 3: `mock_outputs` masks exactly the class of bug GRT001 exists to catch — do not let its presence suppress the check

**What goes wrong:**
The tempting design shortcut: "if `dependency.X.outputs.Y` has a corresponding `mock_outputs.Y` entry, treat `Y` as a legitimate output name and skip the real-module-surface check for it." This is backwards. `mock_outputs` exists so that `plan`/`validate` can proceed *without ever resolving the real dependency* — Terragrunt substitutes the mocked value and, per the official docs, only "throws an error [if] no outputs are available in the target module" **and** the running command isn't in `mock_outputs_allowed_terraform_commands`. Terragrunt's own documentation does not describe any mechanism that checks a mocked key against the target module's real declared outputs — mocking and validation are orthogonal. A developer can write `mock_outputs = { asp_id = "..." }` for a key that the target module's `.tf` files never declare as an output at all, and `terragrunt plan`/`validate` will succeed indefinitely, right up until someone runs `apply` against the real (un-mocked) dependency chain, at which point Terragrunt fails with "output not found" — but only then.

**Why it happens:**
[gruntwork-io/terragrunt#2163](https://github.com/gruntwork-io/terragrunt/issues/2163), reproduced verbatim in `denis256/terragrunt-tests/issue-2163/`, is close to this exact shape: an `app/terragrunt.hcl` with `dependency "app_service_plan01" { mock_outputs = { asp_id = "..." } mock_outputs_allowed_terraform_commands = ["validate", "plan"] }`, where `module/main.tf` (the actual target) never declares `output "asp_id"` at all — it has no `output` blocks whatsoever. `terragrunt plan` succeeds because the mock is used and the real module is never consulted. This is a minimal, real, publicly-filed reproduction of the drift GRT001 is built to catch, hiding in plain sight behind a mock.

**How to avoid:**
GRT001 must check `dependency.X.outputs.Y` against the target module's real `output` declarations **regardless of whether `mock_outputs` also defines `Y`.** The presence of a mock is not evidence the output is real; it's evidence the author expects it to eventually be real, which is exactly the assumption worth verifying statically. Do not read `mock_outputs_allowed_terraform_commands` as a signal to suppress the check either — it only governs which Terragrunt *commands* tolerate a missing dependency, not whether the output name is legitimate.

**Warning signs:**
Any analyzer code with a conditional like `if hasMockOutput(dep, key) { skip }`. A golden fixture (build one from `denis256/terragrunt-tests/issue-2163/` or `mock-output/`) where GRT001 should fire despite `mock_outputs` being present, and doesn't.

**Phase to address:** M1, GRT001 design/implementation. Add this as an explicit test case: dependency with `mock_outputs` covering a key, target module missing that output for real → GRT001 must still fire.

---

### Pitfall 4: `generate` blocks inject declarations invisible to a static `.tf` walk — measured, not just theorized

**What goes wrong:**
A `generate "name" { path = "x.tf" contents = <<EOF ... EOF }` block writes a `.tf` file into the unit's working directory (specifically, into the copy of the module inside `.terragrunt-cache` once Terragrunt actually runs) — this file exists **only after Terragrunt executes**, never as a committed file gruntled can find by walking the repository. If that generated file declares `variable` or `output` blocks, gruntled's static extraction under-counts the module's real surface (a false positive: "this input/output doesn't exist" when it will, at runtime, because Terragrunt generates it).

**How large this actually is, measured across the corpus surveyed:**
`generate` blocks were found in 3 of the ~9 non-fixture repos checked (`cds-snc/secret`: 1 file, injecting `provider.tf` and, separately, `common_variables.tf` with 5 unquoted `variable` blocks; `SAFEHR-data/FlowEHR`: 5 files; `aws-iso20022`, the largest and cleanest repo surveyed: **zero**). In every real occurrence found, `generate` was used for `provider`/`backend`/`remote_state` boilerplate or, in one case, to inject shared `variable` declarations — **not once** to inject an `output` block into a dependency's target module. So: for `GRT001` specifically (output existence), the risk observed in the wild is low so far. For a *future* `GRT005`/`GRT006` (input/variable surface checks, listed as `Later`), the risk is already real and demonstrated: `cds-snc/secret`'s `common_variables.tf` is exactly the shape that would produce a false positive for a naive variable-surface checker that only reads committed `.tf` files, because those five `variable` blocks are never checked into git anywhere.

**Prevention:**
Whenever a unit's `terragrunt.hcl` (or an `include` it inherits) contains a `generate` block, mark that unit's module surface `unknown` for any check whose correctness depends on complete enumeration of declarations that plausibly could be generated (inputs first, once GRT005/GRT006 exist; outputs too, to be conservative, even though no real-world case of generated outputs was found). Do not attempt to evaluate the `contents` HEREDOC as HCL and merge it into the static surface — that reintroduces the exact "invisible declaration" risk one layer down (a generated file whose `contents` itself references `local.foo` computed from something gruntled can't evaluate).

**Phase to address:** M1 — detect `generate` blocks during indexing and propagate an `unknown`/reduced-confidence marker even though GRT001 itself is about outputs (low observed risk); design the marker now so GRT005/GRT006 (`Later`) don't have to retrofit it.

---

### Pitfall 5: two-hop resolution — dependency target is a Unit, not a Module; don't conflate them

**What goes wrong:**
`dependency.X.config_path` points at another **Unit** (a directory with its own `terragrunt.hcl`), not directly at a **Module**. The Unit's own `terraform.source` (local, relative, possibly with a `//` subdirectory marker) is what actually names the Module directory whose `.tf` files must be read for GRT001. A checker — or an analyzer implementation — that resolves `config_path` and then looks for `output` blocks directly in that directory will find nothing whenever the target unit uses `source = "../../modules/x"` (module lives elsewhere) and will wrongly report every output as missing.

**Why it happens:**
This is an easy trap because it's invisible until tested against a repo that actually separates units from modules (many small example repos put `.tf` files directly next to `terragrunt.hcl`, masking the bug). I hit this myself during Part A of this research: my first version of the corpus-checking script produced spurious "MISSING" results against `cds-snc/secret` and several `denis256/terragrunt-tests` fixtures purely because it skipped the source-resolution hop — see "Method" above.

**Prevention:**
Resolve in two explicit steps and test both independently: (1) `config_path` → target Unit; (2) target Unit's `terraform.source` → Module directory (only for local/relative sources; remote sources → `unknown` per the design's existing policy). Never let an analyzer read `.tf` files from the Unit directory when the Unit has a `source` pointing elsewhere.

**Warning signs:**
A golden fixture set that only uses inline modules (Unit dir == Module dir) — this is the aws-iso20022 shape and will not exercise this bug at all. Deliberately include at least one fixture where Unit and Module are different directories (the `cds-snc/secret` shape, or `terragrunt-infrastructure-catalog-example`'s `units/X` → `../..//modules/X` shape).

**Phase to address:** M1, `repograph`/module-resolution logic and its test suite specifically.

---

### Pitfall 6: `.terragrunt-cache`, `.terragrunt-stack`, and a configurable `download_dir` must all be excluded from the walk — not just the default name

**What goes wrong:**
`.terragrunt-cache` (default download directory, holding full copies of every remote module ever fetched, keyed by source+version hash) and `.terragrunt-stack` (default Stacks-generated-unit directory) are conventionally `.gitignore`d, but nothing stops either from existing on disk in a working copy gruntled is asked to analyze (e.g. a developer runs gruntled inside their own checkout after having run `terragrunt` at least once). Walking into `.terragrunt-cache` means encountering the *same* module, potentially at multiple pinned versions, duplicated dozens of times, none of which are actual Units (no `terragrunt.hcl` inside — it's a plain `.tf` module checkout) but which do contain `.tf` files that could pollute Module surface resolution if the walk isn't careful about what counts as an "Module directory" (only ones explicitly referenced by a Unit's `source`, never anything discovered by directory walk). Confirmed both directory names are the documented Terragrunt defaults, and both are independently overridable: `.terragrunt-cache` via `download_dir` in config, `TG_DOWNLOAD_DIR`/`TERRAGRUNT_DOWNLOAD_DIR` env vars, or `--terragrunt-download-dir`; `.terragrunt-stack` is the fixed default for `terragrunt stack generate` output location (also user-nameable via a stack-level setting).

**Prevention:**
Skip both default directory names unconditionally when walking for Units. Additionally, if a Unit's config sets a custom `download_dir`, exclude that path too (read it, don't execute it). Never treat a directory found only by "it has a `terragrunt.hcl` in it" as a Unit without also checking it isn't nested inside one of these cache roots — a cached/vendored copy of a remote module could in principle carry along a `terragrunt.hcl` if the remote repo itself is Terragrunt-structured (rare but not impossible with `//` subdir sourcing into another live repo).

**Warning signs:**
Unit count in a benchmark or demo that's suspiciously larger than `find . -name terragrunt.hcl -not -path '*/.terragrunt-cache/*' -not -path '*/.terragrunt-stack/*' | wc -l` on the same tree — a sign the walk isn't excluding cache directories.

**Phase to address:** M1, indexing/walk implementation, with a golden fixture that includes a fake `.terragrunt-cache/` populated with a decoy `terragrunt.hcl` to prove it's skipped.

---

### Pitfall 7: Go determinism traps — map iteration is genuinely random; directory walk order is not

**What goes wrong:**
The design document (§6①) already names this as a first-class guarantee ("indicizzazione deterministica... in Go l'iterazione delle map è randomizzata di proposito"), which is correct and worth reinforcing precisely, because it's easy to over- or under-apply the fix:

- **Genuinely non-deterministic, must be sorted explicitly:** iterating any `map[K]V` — the Go language specification deliberately randomizes map iteration order to stop anyone from depending on it. Any place a `Set` of diagnostics, a `Surface` of variable/output names, or a dependency adjacency structure is backed by a map and later ranged over for output, printing, or hashing needs an explicit `sort.Slice`/`sort.Strings` before it's observable.
- **Genuinely non-deterministic if introduced later, currently avoided:** parallel indexing. v0.1's spec doesn't mention goroutines for the walk, but the "Later" milestone's incremental reindexing and any future parallel-parse-for-speed optimization will introduce goroutines whose completion order is not guaranteed — results collected into a shared slice/channel must be sorted (by stable Unit path, not by arrival order) before being compared or printed.
- **Already deterministic, do not "fix" it unnecessarily:** `filepath.WalkDir` (and the older `filepath.Walk`) — per the Go standard library documentation, "the files are walked in lexical order, which makes the output deterministic." Don't spend effort re-sorting walk results; do rely on this for the byte-identical-index guarantee, but verify it's `WalkDir`/`Walk` doing the walking and not, say, `os.ReadDir` results being reordered downstream by something that reintroduces randomness (e.g., building a `map[string]os.DirEntry` from them and then ranging over the map).
- **A real, separate trap not mentioned in the design doc: absolute paths leaking into output.** Any `Diagnostic` or `graph --json` field populated from `filepath.Abs()`, the CLI's working directory, or a path constructed by concatenating `os.Getwd()` will differ between two checkouts of the same repo (CI runner path vs. a developer's home directory) even though the *logical* content is identical — breaking the "index two identical repos, compare byte-for-byte" test (§6①) and producing noisy diffs for anyone comparing CI output across machines. Store and print paths relative to the repository root, resolved once at startup, never re-derived per-diagnostic from an absolute base.

**Prevention:**
Bake an explicit, single "make deterministic" boundary into the domain model: every exported/printed collection (`Diagnostic` list, `Surface` variable/output name lists, dependency edge lists in `graph --json`) is a `[]T` built by one explicit sort call, never a map ranged over directly at the presentation boundary. Add the double-indexing byte-diff test (already planned per §6①) as an actual CI step, not just documentation, and add a second test that runs the *same* repo from two different absolute filesystem paths (e.g. two `git clone`s into differently-named directories) and diffs the output — this is the test that specifically catches the absolute-path leak, and the map-only double-run test does not catch it.

**Warning signs:** A golden test that passes when run twice from the same checkout but a CI pipeline that reports spurious diffs when the runner's checkout path changes (common — GitHub Actions/GitLab CI checkout paths are rarely stable across runs).

**Phase to address:** M1, `diagnostic.Set` and any presenter code; make the "two absolute paths, same relative content" test part of the M1 acceptance criteria alongside the already-planned double-index-and-diff test.

---

## Technical Debt Patterns

| Shortcut | Immediate Benefit | Long-term Cost | When Acceptable |
|----------|-------------------|-----------------|------------------|
| Regex-based `.tf`/`.hcl` scanning instead of the real HCL AST, "just for a quick corpus survey script" | Fast to write, no dependency on `hashicorp/hcl` in a throwaway tool | Silently misses unquoted labels (Pitfall 1), multiline blocks, comments containing `output "..."` literals — produces both false positives and false negatives that look like real findings | Only in disposable, one-off research/verification scripts (like the ones used for this document) that are never shipped; never in `tfsurface` or any analyzer |
| Treating `mock_outputs` presence as proof an output is legitimate | Fewer analyzer edge cases to write against | Defeats the entire purpose of GRT001 (Pitfall 3) | Never |
| Skipping the two-hop Unit→Module resolution and reading `.tf` files straight from a dependency's `config_path` directory | Simpler code, works on inline-module repos (like aws-iso20022) | Silent false positives the moment a repo separates units from modules (Pitfall 5), which most non-trivial repos do | Never in shipped code; acceptable only for a throwaway script explicitly scoped to inline-module repos |
| Hardcoding `.terragrunt-cache`/`.terragrunt-stack` as the only excluded directory names | One-line exclusion list | Misses a repo with a custom `download_dir` (Pitfall 6) | Acceptable for M1 if the custom-`download_dir` case is explicitly logged as a known gap, not silently mishandled |

## Performance Traps

| Trap | Symptoms | Prevention | When It Breaks |
|------|----------|------------|-----------------|
| Walking into `.terragrunt-cache` on a large repo that's been `terragrunt`-run many times | Index time balloons, unit count wildly inflated | Exclude by name unconditionally (Pitfall 6) | Any repo where `.terragrunt-cache` wasn't cleaned; can be gigabytes on a repo with many pinned module versions |
| Treating each `include` as re-parsed per unit (the exact O(n²) the design doc already targets) | `check` time grows superlinearly with unit count | Parse each include once, share across units (already the M1+ core technical bet per design §5.4) | Documented at 30-50 modules / 8+ minutes for `run-all plan`; gruntled's own parse should stay sub-quadratic even if it inherits `gruntwork-io/terragrunt`'s cost in M1 |

## UX Pitfalls

| Pitfall | User Impact | Better Approach |
|---------|-------------|-------------------|
| Reporting a stacks-shaped repo (Pitfall 2) as "0 issues found" indistinguishably from a genuinely clean classic repo | User trusts a green result on a repo that was never actually analyzed | Emit an explicit informational note distinguishing "0 units found, nothing to check" from "N units checked, 0 issues" |
| Firing GRT001 on a `dependency` whose target unit's module source is remote-but-resolvable-in-principle (e.g. a pinned git tag) without clearly saying *why* it's `unknown` | User can't tell if gruntled is broken or being appropriately cautious | Every `unknown` marking should carry a machine-readable reason (`remote-source`, `generate-block-present`, `dynamic-source-expression`) surfaced in `--json`/verbose output, not just silence |

## "Looks Done But Isn't" Checklist

- [ ] **Module surface extraction:** often missing unquoted block labels — verify against a fixture using `output name { }` syntax, not just `output "name" { }`
- [ ] **GRT001 + mock_outputs interaction:** often implicitly (and wrongly) suppressed by mocks — verify with a fixture where `mock_outputs` is present *and* the real output is genuinely absent, and confirm GRT001 still fires
- [ ] **Unit discovery / walk:** often missing exclusion of `.terragrunt-cache` and `.terragrunt-stack` — verify with a fixture repo containing a decoy `terragrunt.hcl` inside a fake `.terragrunt-cache/`
- [ ] **Determinism claim:** often verified only by "run twice from the same checkout" — verify also from two differently-named checkouts of the same content (catches absolute-path leaks that a same-directory rerun cannot)
- [ ] **Dependency→Module resolution:** often implicitly assumes Unit dir == Module dir — verify with a fixture where they differ (the `cds-snc/secret` / catalog-example shape)

## Recovery Strategies

| Pitfall | Recovery Cost | Recovery Steps |
|---------|----------------|------------------|
| Regex-based surface extraction shipped by mistake, unquoted-label false positives reported in the wild | LOW | Swap in real HCL AST parsing; add the unquoted-label fixture; no data model change needed since `Surface` is already just names |
| GRT001 wrongly suppressed by `mock_outputs` presence | LOW | Remove the conditional; add the `issue-2163`-shaped fixture as a regression test |
| Stacks repos silently reported as "0 issues" pre-1.0 | MEDIUM | Requires a UX change (informational note) plus retroactively re-labeling any published benchmark/demo run against a Stacks repo as invalid |
| Absolute paths already baked into a published `graph --json` schema | HIGH | Schema change, needs a version bump and migration note for any downstream consumer of `graph --json` (§8 already commits to schema versioning for the index, extend the same discipline to `graph --json`) |

## Pitfall-to-Phase Mapping

| Pitfall | Prevention Phase | Verification |
|---------|-------------------|----------------|
| 1. Unquoted HCL block labels | M1, Module Surface reader | Golden fixture with `output name { }` (no quotes) parses correctly |
| 2. Terragrunt Stacks invisibility | Roadmap note now; full support `Later` | Informational message on any repo containing `.stack.hcl`, never silent zero |
| 3. `mock_outputs` masking real bugs | M1, GRT001 analyzer | Golden fixture from `terragrunt-tests/issue-2163` shape: mock present, real output absent, GRT001 still fires |
| 4. `generate`-injected declarations | M1 for detection/marking; enforcement expands at GRT005/GRT006 (`Later`) | Fixture with a `generate` block injecting a `variable`; confirm unit marked reduced-confidence, not silently treated as fully known |
| 5. Unit vs Module conflation | M1, `repograph` resolution logic | Fixture where Unit dir != Module dir (catalog-example shape) resolves outputs correctly |
| 6. `.terragrunt-cache`/`.terragrunt-stack` walk pollution | M1, indexing walk | Fixture repo with decoy `terragrunt.hcl` inside `.terragrunt-cache/`, confirm excluded |
| 7. Determinism (maps, absolute paths) | M1, presenter/diagnostic boundary | Byte-diff test across two differently-named checkouts of the same repo, not just two runs in place |

## Sources

- [gruntwork-io/terragrunt-infrastructure-live-example](https://github.com/gruntwork-io/terragrunt-infrastructure-live-example) — cloned, full history inspected (86 commits)
- [gruntwork-io/terragrunt-infrastructure-live-stacks-example](https://github.com/gruntwork-io/terragrunt-infrastructure-live-stacks-example) — cloned
- [gruntwork-io/terragrunt-infrastructure-catalog-example](https://github.com/gruntwork-io/terragrunt-infrastructure-catalog-example) — cloned
- [gruntwork-io/terragrunt-infrastructure-modules-example](https://github.com/gruntwork-io/terragrunt-infrastructure-modules-example) — cloned
- [aws-solutions-library-samples/guidance-for-iso20022-messaging-workflows-on-aws](https://github.com/aws-solutions-library-samples/guidance-for-iso20022-messaging-workflows-on-aws) — cloned, systematically checked
- [cds-snc/secret](https://github.com/cds-snc/secret) — cloned, systematically checked
- [khuedoan/cloudlab](https://github.com/khuedoan/cloudlab) — cloned
- [denis256/terragrunt-tests](https://github.com/denis256/terragrunt-tests) — cloned, structure surveyed
- [gruntwork-io/terragrunt#5811](https://github.com/gruntwork-io/terragrunt/issues/5811) — issue, verified closed
- [gruntwork-io/terragrunt#5827](https://github.com/gruntwork-io/terragrunt/pull/5827) — merged fix, body quoted directly
- [gruntwork-io/terragrunt#2163](https://github.com/gruntwork-io/terragrunt/issues/2163) — mock_outputs/real-module-validation gap, reproduced in `denis256/terragrunt-tests/issue-2163/`
- [HCL native syntax spec, hashicorp/hcl](https://github.com/hashicorp/hcl/blob/main/hclsyntax/spec.md) — Block grammar rule for label quoting
- [Terragrunt docs, dependency block / mock_outputs](https://docs.terragrunt.com/reference/config-blocks-and-attributes/#dependency)
- [Terragrunt docs, cache directory / download_dir](https://docs.terragrunt.com/reference/terragrunt-cache/)
- [Go stdlib, `filepath.WalkDir`](https://pkg.go.dev/path/filepath#WalkDir) — lexical-order determinism guarantee
- `.planning/PROJECT.md` and `docs/superpowers/specs/2026-09-01-gruntled-design.md` — project context read prior to research

---
*Pitfalls research for: gruntled (Terragrunt static analyser)*
*Researched: 2026-09-01*
