# Phase 5: Graph Diagnostics - Research

**Researched:** 2026-09-29
**Domain:** Pure-Go graph analysis (missing-target check + SCC cycles) over the existing repograph, plus HCL `dependencies` modelling and real-repo validation
**Confidence:** HIGH on code integration and oracle behaviour (verified locally); MEDIUM on corpus-scale oracle runs (not yet executed)

<user_constraints>
## User Constraints (from CONTEXT.md)

CONTEXT.md is 220 lines and is the source of truth. The planner MUST read
`.planning/phases/05-graph-diagnostics/05-CONTEXT.md` and honor every `## Implementation Decisions`
item verbatim. Summary of the locked points (do not treat this summary as a substitute):

### Locked Decisions
- GRT002 fires only when a resolved dependency target dir is missing or has no `terragrunt.hcl` / `terragrunt.hcl.json` on disk; silent for non-literal, unresolvable, escapes-repo, non-literal or false `enabled`, deep-merged labels (Unknown options), and dirs holding a file the walk skipped. One code, two messages. Anchor = new path-value position on `Dependency`. Applies to literal `dependencies { paths }` entries too. `skip_outputs`/`mock_outputs` do not gate.
- Loader records a target-state enum (`Unknown` zero / `DirMissing` / `NoConfig` / `HasConfig`); "missing" only when `errors.Is(err, fs.ErrNotExist)`; case-exact via parent ReadDir; symlinks followed.
- GRT003: one diagnostic per SCC (self-loop = one-member SCC), edges = resolved deps with `enabled` literally true/absent (incl. `skip_outputs`), `dependencies` paths are edges; anchor Unit = lexically smallest member, position = its first in-SCC out-edge; ring message `dependency cycle: "a" -> "b" -> "a"`, otherwise `dependency cycle among: "a", "b"`; iterative Tarjan, allowlist stdlib only, 10k-deep chain test.
- New domain `PathDependency` on `Unit`, `RepositoryGraph.Edges()`, explicit path-position param on both `NewDependency` constructors, codes in `diagnostic.go`, analyzers in `internal/domain/analysis`, `checking.Check` runs all three, severity `error`.
- MORE-06 corpus validation: 4 mutations per repo (+1 on iso20022), pinned via `corpusMutation`, oracle = pinned terragrunt where it can run, textual oracle otherwise; results in `docs/validation.md`; golden `dependency_edges.txtar` updated.

### Claude's Discretion
Package split inside `internal/domain/analysis`, test helper shapes, enum names, adjacency representation, optional per-analyzer `Stage` labels.

### Deferred Ideas (OUT OF SCOPE)
Symlink-alias canonicalisation; modelling `exclude {}`; SCC message size cap; machine-readable graph output (Phase 6); releases/CI (Phase 7).
</user_constraints>

<phase_requirements>
## Phase Requirements

| ID | Description | Research Support |
|----|-------------|-----------------|
| MORE-01 | GRT002 on literal `config_path` resolving to a dir with no unit; silent on non-literal/unresolvable | Loader target-state classification (Architecture P1), analyzer decision table (P2), oracle text verified (Oracle section) |
| MORE-02 | GRT003 once per cycle, members from lexically smallest path, deterministic | Iterative Tarjan over `Edges()` (P3), ring/among rendering, determinism tests |
| MORE-06 | Corpus: clean on unmutated, every injected mutation caught, recorded in docs/validation.md | Corpus facts, oracle commands verified, harness extension (Validation Architecture) |
</phase_requirements>

## Summary

The work splits cleanly into three layers, all following patterns already in the repo. (1) Domain:
add `Dependency` path position + target state, a `PathDependency` type, `RepositoryGraph.Edges()`,
two codes, and two pure analyzers next to `analysis/grt001.go` (same shape: iterate graph, return
`[]diagnostic.Diagnostic` via `diagnostic.NewForUnit`). (2) Infrastructure: parse the top-level
`dependencies { paths = [...] }` block (parse.go currently ignores it; `depDecl` captures only
`dependency` blocks), evaluate each element with `evalPath`, share classification with
`resolveOneDependency`, and stat the resolved target through the loader's `fs.FS` to fill the
target-state enum. (3) Validation: extend the Phase 4 corpus harness and docs.

Three findings from this research CHANGE or SHARPEN CONTEXT.md and the planner must act on them:
(a) Terragrunt v1.1.6 merges `dependencies` blocks as a **union in BOTH shallow and deep include
merge**, not "highest-precedence wins" for shallow. (b) `terragrunt hcl validate` does **not** detect
a missing dependency target (exit 0), so it cannot be the GRT002 oracle; a working offline oracle
exists (see Oracle section). (c) The `terragrunt` on the default PATH is v0.99.3; the pinned oracle
is `~/.cache/gruntled-phase4/bin/terragrunt_linux_amd64` (v1.1.6).

**Primary recommendation:** Build bottom-up: domain types + `Edges()` -> loader (paths parse, target state, position) -> analyzers -> `Check` wiring -> goldens/docs -> env-gated corpus mutations. Use the `run --all ... -- version` oracle, not `hcl validate`.

## Standard Stack

No new dependencies. Everything is already in `go.mod`.

| Library | Version | Purpose | Why |
|---------|---------|---------|-----|
| Go stdlib `slices`, `sort`, `strconv`, `errors` | go 1.27.0 | Analyzers, Tarjan, messages | Domain allowlist (no `fmt`); enforced by `scripts/check-architecture.sh` |
| `hashicorp/hcl/v2` + `hclsyntax` | already vendored in go.mod | Parse `dependencies` block, list expression elements | HCL only in infrastructure |
| `io/fs` (`fs.ReadDir`, `fs.ErrNotExist`) | stdlib | Target-state classification in loader | Loader already holds `fs.FS` (`l.fsys`) |
| `rogpeppe/go-internal/txtar`, testscript | already in go.mod | Goldens | Existing Phase 3/4 pattern |

Do not add a graph library. Iterative Tarjan is ~60 lines, and domain allowlist forbids external deps.

## Architecture Patterns

### Recommended changes (file map)
```
internal/domain/diagnostic/diagnostic.go   + CodeMissingDependencyTarget "GRT002", CodeDependencyCycle "GRT003"
internal/domain/repograph/unit.go          Dependency: +pathPos, +targetState; new PathDependency; Unit.PathDependencies()
internal/domain/repograph/graph.go         + Edges() []Edge (kind, from, to, pos, enabled)
internal/domain/analysis/grt002.go         MissingTargets(g)
internal/domain/analysis/grt003.go         DependencyCycles(g)  (iterative Tarjan)
internal/application/ports/ports.go        UnitConfig: + PathDependencies (already include-merged)
internal/infrastructure/terragrunt/parse.go     depDecl + configPathPos; new pathsDecl parsed from `dependencies` blocks
internal/infrastructure/terragrunt/loader.go    shared resolve helper; classifyTarget(); path-dep resolution
internal/infrastructure/terragrunt/merge.go     union of path decls across effective files
internal/application/indexing/build.go     pass PathDependencies to constructors
internal/application/checking/check.go     run all three analyzers; update Report.Diagnostics doc
```

### Pattern 1: Target-state classification (loader only; domain cannot stat)
Existing `resolveOneDependency` already computes `targetDir` and stats `p` and `<dir>/terragrunt.stack.hcl`
via `fs.Stat(l.fsys, ...)`. Add a `classifyTarget(dir string) repograph.TargetState` called on the final
`targetDir` after the stack check:
- `fs.ReadDir(l.fsys, dir)`; if `errors.Is(err, fs.ErrNotExist)` -> `DirMissing`; any other error -> `Unknown`.
- Compare entry names case-exactly against `terragrunt.hcl` / `terragrunt.hcl.json`; for a match, `fs.Stat` (follows symlinks): success and not a dir -> `HasConfig`; `ErrNotExist` (dangling) counts as absent.
- If ReadDir succeeds but the path is a file (ENOTDIR-like) or no match -> `NoConfig` only when ReadDir succeeded on a directory; else `Unknown`.
- Missing path AND parent-of-file case: `config_path = "../x/terragrunt.hcl"` non-existent maps to its parent dir first (CONTEXT), then classify.
- Unresolved deps: always `Unknown`. Validate enum in constructors (`IsValid`, like `Tristate.IsValid`).

### Pattern 2: GRT002 analyzer decision table (mirror grt001.go doc-comment style)
Per unit in `g.Units()` order, per block dep then per path dep, first matching row wins:
1. unresolved, or `targetState` Unknown/HasConfig: silent.
2. block dep: `opts.Enabled` not `TristateTrue`... NOTE: CONTEXT says silent unless `enabled` is literally `true` **or absent**. Check how `DependencyOptions.Enabled` represents absent (look at `options.go`, `depfacts.go:18`) before writing the row; GRT001 row 2 uses `!= TristateTrue`, which may treat absent as true already. Do not assume; read those 20 lines and test both cases.
3. otherwise emit GRT002 at `dep.PathPos()` with message `dependency "vpc" config_path resolves to "live/dev/vpc": directory does not exist` / `... directory has no terragrunt.hcl`; paths: `dependencies path "../x" resolves to "a/x": ...`. Use `strconv.Quote`, no `fmt`.
Path deps have no `enabled` (a `dependencies` block has none): always eligible.

### Pattern 3: GRT003 (iterative Tarjan, deterministic)
- Nodes: all `g.Units()` (already RepoPath-sorted). Edges from `g.Edges()` filtered by the analyzer: enabled literally true/absent, target is a graph unit (else drop), source unit not config-unknown (they have no edges anyway).
- Dedupe out-edges by target; sort adjacency by `RepoPath.Compare`. Explicit stack of (node, next-child index); no recursion (10k chain test).
- SCC list: keep components with size > 1, or size 1 with a self-edge. Sort members; anchor unit = `members[0]`; position = min by `Position.Compare` over its edges into the SCC (self edge for one-member).
- Ring test: every member has exactly one distinct in-SCC successor -> walk from smallest member, render `"a" -> "b" -> "a"`; else `among:` sorted list. Self-loop inside a larger SCC: no extra diagnostic.
- Emit with `diagnostic.NewForUnit(CodeDependencyCycle, SeverityError, anchor, pos, msg)`.

### Pattern 4: `dependencies` block parse + merge (verified against terragrunt v1.1.6)
Parse in `parse.go` next to the `case "dependency":` arm (line ~219): for `block.Type == "dependencies"` read `paths` attr; if it is `*hclsyntax.TupleConsExpr`, keep each element expression (and its `Range().Start` position) so each is evaluated with `evalPath` in the same scope/`unitDir` as `config_path`; any other expression -> recorded Unknown; invalid structure (duplicate block, labels, missing/non-list `paths`) -> drop path edges only, unit stays resolved.
**Merge rule (CORRECTION to CONTEXT):** union with de-duplication in both strategies. Evidence, terragrunt v1.1.6 `pkg/config/include.go`: `Merge` (shallow, lines ~402-407): `if cfg.Dependencies == nil { cfg.Dependencies = source.Dependencies } else { cfg.Dependencies.Merge(source.Dependencies) }`, with the file's own comment "dependencies block is a special case and is merged deeply". `ModuleDependencies.Merge` in `pkg/config/config.go` appends source paths not already present. `DeepMerge` (~506-545) also unions (it drops parent paths that duplicate a `dependency` block path; harmless because edges are deduped by target, but a GRT002 could then attach to only one position). So: concatenate all effective files' (child + non-`no_merge` includes) literal paths, de-duplicate by resolved target, keep the highest-precedence position. Do NOT use "highest-precedence block wins" for shallow. Cite this in the plan; update CONTEXT wording in the plan's decision log.
Note: the `dependencies` block is documented as legacy in Terragrunt; still parsed in v1.1.6.

### Anti-Patterns to Avoid
- Folding path deps into `Dependency` (breaks name-uniqueness invariant in `sortAndValidateDepsRefs`).
- Using `DependencyTarget` for GRT002 (conflates "not a unit" with "skipped by the walk").
- Statting in the domain, or using `fmt` in analyzers (arch check fails).
- Recursion in Tarjan; map iteration order leaking into output.
- Silent constructor-signature drift: every `NewDependency`/`NewUnresolvedDependency` call site (loader, indexing, domain tests, `synthrepo`, presenter tests) must be updated; grep before starting (`grep -rn "NewDependency\|NewUnresolvedDependency" --include=*.go`).

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| Path evaluation of `paths` elements | New evaluator | `evalPath` + `resolvePath` (eval.go) | Six path functions, closed EvalContext, escape check already correct |
| Target classification (file/stack/escape) | Second copy | Extract shared helper from `resolveOneDependency` | Stack-wins and nondefault-file rules must not drift |
| Diagnostic ordering/dedup | Sorting in analyzer | `diagnostic.NewSet` (canonical) | Order-irrelevant by design |
| Cycle oracle | Custom checker in tests | `terragrunt run --all` (iso20022) / coreutils `tsort` | Independent oracle is the point |

## Common Pitfalls

### Pitfall 1: `hcl validate` is not an existence oracle
Verified on v1.1.6 with a hand-made tree: `terragrunt hcl validate` exits 0 with a missing `config_path` dir and no output. **Use** `terragrunt_linux_amd64 run --all --non-interactive --tf-path <tofu> -- version` in the tree: missing dir and empty dir both exit 1 with `You attempted to run terragrunt in a folder that does not contain a terragrunt.hcl file ... Path: "<abs>/<target>/terragrunt.hcl"`; the same for a `dependencies` path; `skip_outputs = true` does NOT suppress it (confirms CONTEXT [auto]); cycles and `config_path = "."` both exit 1 with `cycle detected during queue construction`. `terragrunt dag graph` prints `WARN Cycle detected in dependency graph` and exits 0 (weaker; `find --dag` prints the same WARN). `render --json` errors `resolving dependency "x" outputs: <abs>/nodir does not exist` only when outputs are actually evaluated. Terragrunt stops at the first error, so it validates one mutation at a time (fits the design). Not yet tried on the full corpora: `version` is offline but may create `.terragrunt-cache`; ALWAYS run on a scratch copy and delete it. Confidence: HIGH on the toy tree, LOW-MEDIUM on corpus scale (run and record; if it fails on iso20022's config-unknown units, fall back to textual oracle and document).

### Pitfall 2: Wrong terragrunt binary
`which terragrunt` is v0.99.3. Pinned v1.1.6 lives at `~/.cache/gruntled-phase4/bin/terragrunt_linux_amd64`, SHA in `docs/validation.md`; reuse the harness's `GRUNTLED_TERRAGRUNT_BIN` and SHA check (fail, not skip).

### Pitfall 3: Corpus reality differs from the stated SC3
Local pinned checkouts: primary/iso20022 `e6c55d11` (62 files with dependency blocks, no `dependencies`, no `exclude`); secret `341e8a95` (2 files with dependency blocks AND `dependencies { paths }`: `terragrunt/ecr` paths `["../acm"]` + block `../acm`; `terragrunt/lambda` paths `["../acm","../ecr"]` + blocks acm, ecr) so secret already has real edges and NO synthetic edge is needed; a block+paths pair to the same target must dedupe into one edge and must not double-count in cycle messages; denis256 `726485e6` (397 dependency files, 37 with `dependencies`, 11 with `exclude`, and real missing targets). denis256 needs an exact expected set (CONTEXT). Record the secret commit in a const and add `GRUNTLED_CORPUS_SECRET` gating (pin and fail on wrong commit; `TestIncludeTargetSecretCorpus` already uses this env var, reuse its name). Unmutated iso20022: `go run ./cmd/gruntled check` currently prints `checked 65 units (3 unknown): 0 errors`; that must stay true, and JSON must be byte-identical after revert.

### Pitfall 4: Message is part of the Key
Any hint text, member-count, or unstable ordering changes Key and breaks dedup/goldens. Keep messages exactly as in CONTEXT. Shared-include missing target yields one diagnostic per including unit at the same file:line:col (Key includes Unit): expected, not a bug.

### Pitfall 5: Windows/case/symlink determinism
Existence via ReadDir name comparison (not `Stat`) to be case-exact; `ErrNotExist` only for DirMissing; `os.Root`-escaping symlink errors are not `ErrNotExist` -> Unknown -> silent. Add a Unix-only symlink test (existing `*_unix_test.go` pattern) and keep build tags out of domain.

### Pitfall 6: Existing tests will change
`dependency_edges.txtar` (`config_path = "../nodir"` and a stack/no-config dir) will now emit GRT002 and exit 1; `denis256_test.go` fails on any non-GRT001 code; `validation_doc_test.go` pins doc contents; `docs/cli.md` "reserved" codes and "cycles not reported" limitation. Review each golden diff by hand; do not blanket `-update`.

## Code Examples

### Iterative Tarjan skeleton (stdlib only)
```go
// nodes sorted by RepoPath; adj[i] sorted, deduped, in-graph targets only.
index, low := make([]int, n), make([]int, n) // 0 = unvisited; store idx+1
onStack := make([]bool, n)
var stack []int
type frame struct{ v, next int }
counter := 0
for root := 0; root < n; root++ {
	if index[root] != 0 { continue }
	call := []frame{{root, 0}}
	counter++; index[root], low[root] = counter, counter
	stack = append(stack, root); onStack[root] = true
	for len(call) > 0 {
		f := &call[len(call)-1]
		if f.next < len(adj[f.v]) {
			w := adj[f.v][f.next]; f.next++
			if index[w] == 0 {
				counter++; index[w], low[w] = counter, counter
				stack = append(stack, w); onStack[w] = true
				call = append(call, frame{w, 0})
			} else if onStack[w] { low[f.v] = min(low[f.v], index[w]) }
			continue
		}
		v := f.v
		if low[v] == index[v] { /* pop stack until v -> one SCC */ }
		call = call[:len(call)-1]
		if len(call) > 0 { p := call[len(call)-1].v; low[p] = min(low[p], low[v]) }
	}
}
```
(Source: standard Tarjan; adapt, then sort each SCC and the SCC list by smallest member.)

### Oracle invocation (verified 2026-09-29 on v1.1.6)
```bash
TG=$HOME/.cache/gruntled-phase4/bin/terragrunt_linux_amd64
$TG run --all --non-interactive --tf-path $HOME/.cache/gruntled-phase4/bin/tofu -- version
# missing/empty target: rc=1, "...does not contain a terragrunt.hcl file... Path: \"<abs>/<t>/terragrunt.hcl\""
# cycle or self-loop:   rc=1, "cycle detected during queue construction"
```
Textual oracle for secret/denis256: `grep` the literal `config_path`/`paths` values, resolve against the file dir, `test -d`/`test -f terragrunt.hcl`; cycles via `tsort` on `unit target` pairs (tsort prints `tsort: input contains a loop` and the members).

## State of the Art

| Old | Current | Impact |
|-----|---------|--------|
| `hcl validate` as Phase 4 comparator | Not applicable to existence/cycles | Different oracle for Phase 5 (Pitfall 1) |
| Local `terragrunt` v0.99.3 | Pinned v1.1.6 | Always use pinned binary |
| Terragrunt source path `config/include.go` | `pkg/config/include.go` (v1.1.6) | Cite the right path in the plan |

## Open Questions

1. **Represention of absent `enabled`**: read `options.go`/`depfacts.go:18-38` to confirm whether absent maps to `TristateTrue` or Unknown before coding the GRT002/GRT003 gating rows. Recommendation: add table tests for absent, true, false, non-literal.
2. **`run --all -- version` at corpus scale** (writes cache, runtime, config-unknown iso20022 units). Recommendation: a Wave 0 spike on scratch copies; fall back to textual oracle and document.
3. **Should `indexing.Build` validate a target-state/pos on `PathDependency`?** Follow CONTEXT: constructors validate; zero pos rejected.
4. **Amend ROADMAP SC3 and REQUIREMENTS MORE-06 wording** (denis256 matches exact oracle set; iso20022 and secret clean) as CONTEXT directs.

## Validation Architecture

### Test Framework
| Property | Value |
|----------|-------|
| Framework | Go `testing` (go 1.27.0), testscript/txtar goldens, synthrepo |
| Config file | none (env gates) |
| Quick run command | `go test -count=1 ./internal/domain/... ./internal/application/... ./internal/infrastructure/terragrunt/... && go test -count=1 ./cmd/gruntled -run 'TestGolden\|TestValidationDocPins'` |
| Full suite command | `test -z "$("$(go env GOROOT)/bin/gofmt" -l .)" && go mod tidy -diff && go vet ./... && GOTOOLCHAIN=go1.27.0 go run honnef.co/go/tools/cmd/staticcheck@v0.8.1 ./... && go test -count=1 ./... && bash scripts/check-architecture.sh && bash scripts/test-check-architecture.sh` |
| Corpus (env-gated) | `GRUNTLED_CORPUS=$HOME/.cache/gruntled-phase4/corpus/primary GRUNTLED_CORPUS_SECRET=$HOME/.cache/gruntled-phase4/corpus/secret GRUNTLED_CORPUS_DENIS256=$HOME/.cache/gruntled-phase4/corpus/denis256 GRUNTLED_TERRAGRUNT_BIN=$HOME/.cache/gruntled-phase4/bin/terragrunt_linux_amd64 go test -count=1 -v -run 'TestCorpus\|TestDenis256\|TestSecret' ./cmd/gruntled` |

### Phase Requirements -> Test Map
| Req | Behavior | Type | Command | Exists |
|-----|----------|------|---------|--------|
| MORE-01 | Table test of GRT002 rows (missing, no-config, has-config, unknown, disabled, non-literal, paths entry, skip_outputs) | unit | `go test ./internal/domain/analysis -run TestMissingTargets` | Wave 0 |
| MORE-01 | Loader classification: missing/empty/case-mismatch/dangling symlink/EACCES/parent-file mapping | unit | `go test ./internal/infrastructure/terragrunt -run 'TestTargetState\|TestPathDependencies'` | Wave 0 |
| MORE-01 | `dependencies` merge = union, no_merge dropped, invalid block drops only path edges | unit | same package, `TestPathDepsMerge` | Wave 0 |
| MORE-01 | Golden: `dependency_edges` updated, plus new `grt002_*.txtar` (both messages, shared include x2 units) | golden | `go test ./cmd/gruntled -run TestGolden` | update + Wave 0 |
| MORE-02 | Self-loop, 2-ring, 3-ring, non-ring SCC, self-loop inside SCC, dedup block+paths, disabled edge dropped, unknown-config invisible | unit | `go test ./internal/domain/analysis -run TestDependencyCycles` | Wave 0 |
| MORE-02 | Input-order permutation invariance; 10k-unit chain does not overflow; repeated runs byte-identical | unit | same, `TestCyclesDeterministic`, `TestCyclesDeepChain` | Wave 0 |
| MORE-02 | `Edges()` deterministic and complete | unit | `go test ./internal/domain/repograph -run TestEdges` | Wave 0 |
| MORE-01/02 | `Check` wires all three, one Set | unit | `go test ./internal/application/checking` | extend |
| MORE-06 | Unmutated: iso20022 and secret zero GRT002/GRT003; denis256 == exact oracle set | corpus | Corpus command above | extend `TestCorpusClean`, `denis256_test.go` |
| MORE-06 | Per repo 4 mutations (+ module-only dir on iso20022) each adds exactly the expected diagnostic, exit 1, GRT001 counts unchanged, revert byte-identical, terragrunt oracle (iso20022) / textual oracle (others) | corpus | same | Wave 0 |
| MORE-06 | `docs/validation.md` v0.2 sections and pins | doc | `go test ./cmd/gruntled -run TestValidationDocPins` | extend |
| ARCH | domain/application allowlists, no fmt | script | `bash scripts/check-architecture.sh` | exists |

### Sampling Rate
- Per task commit: quick run command
- Per wave merge: full suite command
- Phase gate: full suite green plus the corpus command recorded in `docs/validation.md`

### Wave 0 Gaps
- [ ] `internal/domain/analysis/grt002_test.go`, `grt003_test.go` (helpers to build graphs with path positions)
- [ ] `internal/domain/repograph` tests for new enum/constructors/`Edges()`
- [ ] `internal/infrastructure/terragrunt` tests for target state and `dependencies` parse/merge (temp dirs via `fstest.MapFS` + real-FS symlink case in a `_unix_test.go`)
- [ ] Spike: run the `run --all -- version` oracle on scratch copies of iso20022 (and secret with the unnamed `include {}` named) and record the exact outputs
- [ ] Secret corpus pin const + env gate; new `corpusMutation` entries with `Old`/`New` exact-occurrence pins
- [ ] Framework install: none needed

## Sources

### Primary (HIGH)
- Local repo code read this session: `analysis/grt001.go`, `checking/check.go`, `repograph/unit.go` and `graph.go`, `terragrunt/loader.go` (resolveDependencies, resolveOneDependency), `eval.go`, `merge.go`, `parse.go`, `ports.go`, `scripts/check-architecture.sh`
- terragrunt v1.1.6 source: `https://raw.githubusercontent.com/gruntwork-io/terragrunt/v1.1.6/pkg/config/include.go` (Merge ~L314-410, DeepMerge ~L443-560) and `pkg/config/config.go` (`ModuleDependencies.Merge`)
- Hands-on runs of pinned terragrunt v1.1.6 (hcl validate, dag graph, find, render --json, run --all) on a scratch tree, 2026-09-29

### Secondary (MEDIUM)
- Local corpus greps at `~/.cache/gruntled-phase4/corpus/*` (counts of dependency/dependencies/exclude files)

### Tertiary (LOW)
- Corpus-scale behaviour of the `run --all -- version` oracle: unverified

## Metadata

**Confidence:** Standard stack HIGH (no new deps); Architecture HIGH (extends existing patterns); Pitfalls HIGH for oracle/merge (verified), MEDIUM for corpus-scale.
**Research date:** 2026-09-29
**Valid until:** 2026-10-29 (pinned versions; re-check only if terragrunt pin changes)
