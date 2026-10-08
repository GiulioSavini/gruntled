# Phase 9: Blast Radius - Research

**Researched:** 2026-10-08
**Domain:** pure-Go diff of two analysed Terragrunt trees (findings + module surfaces), new CLI subcommand
**Confidence:** HIGH (all findings come from reading this repo's code; no external library involved)

<user_constraints>
## User Constraints (no CONTEXT.md; unattended run; decisions from research/SUMMARY.md "Decisions taken")

### Locked Decisions
- Baseline is a second directory: `gruntled blast --base <dir> <path>` (CI uses `git worktree`).
- Impacted = one hop, direct consumers only (no transitive walk).
- Surface change = variable/output NAMES added or removed (no types/required/defaults).
- No git, no exec, no network (binary-no-net-no-exec proof must still pass).
- Out of scope (REQUIREMENTS.md): git-based blast, daemon baseline (`report --blast`, `rebase`), transitive Impacted, type-level surface changes, GRT004-006.

### Claude's Discretion
- Finding identity across trees, Broken subject for unit-less findings, exit codes, json shape/versioning, text layout, package placement.

### Deferred Ideas (OUT OF SCOPE)
- Daemon in-memory baseline, transitive Impacted, type-level surface diff, MORE-03..05.
</user_constraints>

<phase_requirements>
## Phase Requirements

| ID | Description | Research Support |
|----|-------------|-----------------|
| BLAST-01 | `blast --base <dir> <path>`: disjoint sorted Broken / Impacted, text+json; no baseline => Broken only, labelled "no baseline" | Sections: Finding identity, No-baseline semantics, Output shapes, CLI wiring, Architecture placement |
| BLAST-02 | Impacted only when a directly consumed module gained/lost a variable/output name; pre-existing findings never Broken | Sections: Surface diff, Finding identity (position-free key) |
</phase_requirements>

## Summary

Everything needed already exists: `checking.Check(ctx, units, surfaces)` returns `Report{Graph, Diagnostics}` for one tree; `repograph.Surface` has sorted `Variables()/Outputs()`; `Unit.Module()` gives the repo-relative module path a unit consumes (`graph.Units()`, `graph.Modules()`, `Module.Surface()` returns `(Surface, known)`). Blast is: run `Check` on both trees, then a pure comparison. No new parsing, no new dependency.

The one real design trap: `diagnostic.Key` includes Line and Column (and `diagnostic.Diff` uses it). Inserting a line above an existing finding in `<path>` shifts its position, so `Diff` would call a pre-existing finding "new" and report it Broken, violating SC2/BLAST-02. Blast must use a position-free identity. Messages (GRT001/002/003) contain only names (dep, output, module path, target unit, cycle member paths), never line numbers or absolute roots, and all paths are repo-relative `RepoPath`, so `(Code, Unit, File, Message)` is stable across two trees with different absolute roots.

**Primary recommendation:** domain pure functions (new package `internal/domain/impact`) + thin application use case `internal/application/blasting` that calls `checking.Check` twice + two presenter functions + `runBlast` in main.go. Exit 1 iff Broken is non-empty.

## Standard Stack

No new libraries. Go stdlib only (`slices`, `maps`, `sort`, `strings`, `encoding/json` in presenter). Test: existing `testscript` (`cmd/gruntled/testdata/script/*.txtar`), `pgregory.net/rapid` (test-only, already present), plain `testing`.

## Architecture Patterns

### Placement (verified against scripts/check-architecture.sh)
```
internal/domain/impact/        NEW pure: FindingKey, NewFindings(base,cur), SurfaceDiff, Compute -> Result
internal/application/blasting/ NEW use case: Blast(ctx, cur, base *Sources) (Result, error)
internal/interfaces/presenter/blast.go  NEW: BlastText, BlastJSON (stdlib fmt/io/encoding/json only)
cmd/gruntled/main.go           + case "blast", runBlast, blastUsage
docs/cli.md                    + blast section, Known limitations entry (names only)
```
- Domain allowlist: errors, strings, strconv, sort, slices, maps, path, cmp, iter, bytes, math... No `fmt`, `context`, `os`. Domain may import only `internal/domain/...`. The new package must not use `fmt.Errorf` (use `errors.New` + concatenation like the rest).
- Application allowlist = domain list + `context`; imports only domain/application. Use `ports.UnitLoader` + `ports.SurfaceReader` pairs; define `type Sources struct{ Units ports.UnitLoader; Surfaces ports.SurfaceReader }` in blasting. Error type: custom `*blasting.Error{Stage, Err}` mirroring `checking.Error` (stages "current"/"baseline"), no fmt.
- Presenter allowlist adds fmt, io, encoding/json. Build output in a `bytes.Buffer` like existing presenters; main uses `writeOut`.
- The architecture script's non-vacuous guards and layout rules are unaffected (new dirs live under existing layers). Do NOT run scripts/test-check-architecture.sh (constraint); `scripts/check-architecture.sh` itself is fine.
- Put the position-free key in the domain `diagnostic` package? Prefer adding to `impact` (keeps `Key`/`Diff` untouched, which GRT/daemon code and tests rely on). `impact` imports `diagnostic` and `repograph`.

### Finding identity (resolves "same finding in both trees")
`type FindingKey struct{ Code diagnostic.Code; Unit repograph.RepoPath; File string; Message string }` built from `d.Key()` fields minus Line/Column (and, like `Key`, ignoring severity). Broken findings = cur diagnostics whose FindingKey not in base set. A finding in both is never Broken even if its line moved (SC2). Known, documented tradeoff: two findings identical except for line collapse to one key (set semantics); acceptable, same-message duplicates in one file/unit are the same defect. Do not use multiset counts (over-engineering).
Verified: GRT001 msg = dep name, output, module path, target unit path (+ fixed mock suffix); GRT002 msg names target; GRT003 msg lists unit paths. None embed positions. GRT100 (syntax error from `hclconv`) is created with `diagnostic.New` (no unit) and a message from HCL; check that the HCL message does not embed line/column when planning (open question 2).

### Broken subject (set of "units")
Broken is a set of units, each with its findings. Subject of a finding = `d.Unit()` if present, else the finding's position file (a `RepoPath`, e.g. GRT100 in a shared file). Sorted by `RepoPath.Compare`; findings per subject in `diagnostic.Compare` order. Disjointness: Impacted = consumers with changed module surface MINUS any subject already in Broken (a Broken unit is not repeated as Impacted). Both lists strictly sorted by path, no duplicates.

### Surface diff / Impacted (BLAST-02)
- For each module path M present in BOTH graphs with `Surface()` known in both: `added/removed` variables and outputs via sorted-merge or set diff of `Variables()`/`Outputs()`. Empty diff => contributes nothing (SC3 "unchanged surface impacts nothing"; comment/body edits invisible by construction since surface is parsed names).
- Impacted candidates = units in the CURRENT graph with `u.Module()` == M (status resolved; `Module()` returns ok=false for unknown). One hop only: no walk over dependency edges.
- Module present in only one tree, or unknown (`UnknownReason != ""`) in either: not diffed, nothing Impacted (cannot claim a name change from no surface). A unit that newly points at a different module is itself edited and is out of this definition. Document in docs/cli.md.
- Each Impacted entry carries the changed module path and the added/removed names (useful output, free to compute).

### No-baseline semantics (SC4 ambiguity resolved)
Without `--base`, the baseline is "empty": every current finding is absent from it, so Broken = all subjects with any current finding (identical set to what `check` reports, grouped by unit). Impacted is not computable: do not print an empty Impacted as if it were "none".
- Text: first line `baseline: none (no baseline)` then `Broken (N):` section only; no `Impacted` section; with baseline first line `baseline: <dir>` (the flag value as given; never an absolute path derived by the tool).
- JSON: `"baseline": false`, `"note": "no baseline"`, `"impacted": null`? Repo convention says lists are never null. Use `"impacted": []` plus `"baseline": false` and `"note": "no baseline"` so consumers key off `baseline`. With baseline: `"baseline": true`, note omitted.
- Exit code identical rule in both modes (below).

### Exit codes (consistent with check: 0/1/2/3)
- 0: analysis ran; Broken is empty (Impacted alone never fails: it is a risk signal, not a defect).
- 1: Broken non-empty AND at least one Broken finding has error severity (all current codes are errors, so effectively Broken non-empty; implement via severity to match `check`'s `HasErrors`).
- 2: usage: unknown flag, bad/missing `--format` value, more than one path, `--base` given without value.
- 3: could not run: path or base dir missing/unreadable (`openRepo` failure for either), loader/surface internal error (either tree), write failure.
Note `Check` itself returns exit 0/1 semantics only through diagnostics; loader errors come back as `error` => 3.

### Output shapes
Flags: `--base <dir>`, `--format text|json` (default text). Do not offer sarif. Reuse `parseArgs(name, usage, args, stderr, define)` with `fs.String("base","","...")`; empty base string = no baseline. Reject `--format sarif` as usage (2).
Text (everything on stdout, no stderr summary, deterministic):
```
baseline: ../base
Broken (1):
  live/app
    live/app/terragrunt.hcl:12:5: GRT001 dependency "vpc" output "id" is not declared by module "modules/vpc" (target unit "live/vpc")
Impacted (1):
  live/db (module modules/vpc: -output id, +variable name)
```
Empty sections print `Broken (0):` / `Impacted (0):` lines so output is never silently empty.
JSON (structs only, fixed field order, `SetEscapeHTML(false)`, indent like graph.go):
```json
{"version":1,"kind":"blast","baseline":true,
 "broken":[{"unit":"live/app","findings":[{"code":"GRT001","severity":"error","file":"...","line":12,"column":5,"message":"..."}]}],
 "impacted":[{"unit":"live/db","module":"modules/vpc","added_variables":[],"removed_variables":[],"added_outputs":[],"removed_outputs":["id"]}],
 "summary":{"broken":1,"impacted":1}}
```
Versioning: mirror graph.go: `const blastSchemaVersion = 1`, `const blastKind = "blast"`, `kind` distinguishes it from check report (which has no kind) and graph. All lists `[]` never null. Positions use the CURRENT tree's repo-relative paths.

### Anti-Patterns
- Reusing `diagnostic.Diff` (line-sensitive) for Broken.
- Reporting an Impacted unit also as Broken, or printing absolute roots.
- Computing surface diff by diffing file text (comment edits would count).
- Walking `Edges()` transitively for Impacted.
- Adding git/exec; any `os` import in application/domain.

## Don't Hand-Roll

| Problem | Don't Build | Use Instead |
|---------|-------------|-------------|
| Analysing a tree | second pipeline | `checking.Check` (run twice; Report.Graph + Diagnostics) |
| Sorted unique names / set ops | custom sort | `slices.Sort`, `slices.BinarySearch`, `Surface.HasVariable/HasOutput` |
| Path ordering | string compare | `RepoPath.Compare` |
| Arg parsing, root opening, buffered write | new helpers | `parseArgs`, `openRepo`, `writeOut` in main.go |
| Dir-based test baselines | git in tests | `writeFiles`/txtar with two directories |

## Common Pitfalls

1. **Line-shift false Broken** (above). Verify with a test: same finding, extra blank line above it in `<path>` => Broken empty.
2. **GRT100 message may embed positions or absolute-ish text.** Check `hclconv.go:80` message construction; if it includes line/column or a diag summary with filenames, normalise or accept it as unstable (GRT100 would then shift on edits). Add a test with a syntax error present in both trees.
3. **Disjointness bug**: forgetting to subtract Broken subjects from Impacted. Property test: `Broken ∩ Impacted = ∅`, both sorted/unique.
4. **Base == path / base missing**: missing base => exit 3 with clear stderr (not silently "no baseline"). Same dir given for both => empty Broken, empty Impacted, exit 0 (valid).
5. **Unknown modules** treated as empty surface would make every consumer Impacted; require known on both sides.
6. **Resolved vs unknown units**: `Unit.Module()` ok=false for config-unknown units; skip them.
7. **Finding removed in cur / fixed**: never reported (blast is only what the change breaks).
8. **JSON null lists**: allocate `[]T{}`.
9. **Docs tests**: `ci_doc_test.go`, `validation_doc_test.go` pin doc content; run them after editing docs/ (full `go test ./cmd/gruntled`).
10. Names-only limits must be stated in docs: an added required variable or type change is invisible; a unit not edited but consuming a module is only "Impacted" by name change.

## Code Examples

```go
// internal/domain/impact (sketch, no fmt)
type FindingKey struct {
	Code    diagnostic.Code
	Unit    repograph.RepoPath
	File    string
	Message string
}

func KeyOf(d diagnostic.Diagnostic) FindingKey {
	k := d.Key()
	return FindingKey{k.Code, k.Unit, k.File, k.Message}
}

// NewFindings: diagnostics of cur whose KeyOf is absent from base.
func NewFindings(base, cur diagnostic.Set) []diagnostic.Diagnostic {
	seen := make(map[FindingKey]struct{}, base.Len())
	for _, d := range base.All() {
		seen[KeyOf(d)] = struct{}{}
	}
	var out []diagnostic.Diagnostic
	for _, d := range cur.All() { // already canonical order
		if _, ok := seen[KeyOf(d)]; !ok {
			out = append(out, d)
		}
	}
	return out
}
```
```go
// application: no baseline => base graph/diags nil
func Blast(ctx context.Context, cur Sources, base *Sources) (impact.Result, error) {
	curRep, err := checking.Check(ctx, cur.Units, cur.Surfaces)
	if err != nil { return impact.Result{}, &Error{Stage: "current", Err: err} }
	if base == nil { return impact.NoBaseline(curRep.Diagnostics), nil }
	baseRep, err := checking.Check(ctx, base.Units, base.Surfaces)
	if err != nil { return impact.Result{}, &Error{Stage: "baseline", Err: err} }
	return impact.Compute(baseRep.Graph, baseRep.Diagnostics, curRep.Graph, curRep.Diagnostics), nil
}
```
main.go: open both roots with `openRepo`, `defer Close`, build `terragrunt.NewLoader(root.FS())` + `tfsurface.NewReader(root.FS())` per tree (one loader per tree; do not share).

## State of the Art
Phase 8 added `Loader.Invalidate`/persistent parse store; irrelevant to one-shot blast (fresh loaders, cache cold). Daemon baseline is deferred; keep `impact.Compute` a pure function of (graphs, diagnostic sets) so Phase 10/11 or later can feed it an in-memory baseline unchanged.

## Open Questions
1. Brand-new or deleted module: kept as "not diffed". Alternative (treat absent as empty surface) would flag every consumer of a new module. Recommendation: not diffed; document.
2. Exact GRT100 message stability (see Pitfall 2) - planner task: read `hclconv.go` ~line 60-90 and add a regression test.
3. Subject for unit-less findings = file path: if the planner prefers, list them in a separate `unattributed` key instead; recommendation stays with file path (keeps exactly two sets, per SC1).

## Validation Architecture

### Test Framework
| Property | Value |
|----------|-------|
| Framework | Go `testing` + `testscript` (rogpeppe/go-internal, txtar) + `pgregory.net/rapid` v1.3.0 (test-only) |
| Config file | none |
| Quick run command | `go test ./internal/domain/impact/ ./internal/application/blasting/ ./internal/interfaces/presenter/ -count=1` and `go test ./cmd/gruntled -run 'TestScripts/blast' -count=1` |
| Full suite command | `go vet ./... && go test -count=1 ./... && scripts/check-architecture.sh` (no `-race`: no C compiler) |

### Phase Requirements -> Test Map
| Req ID | Behavior | Test Type | Automated Command | File Exists? |
|--------|----------|-----------|-------------------|-------------|
| BLAST-01 | two sorted disjoint sets, text+json; unit-less finding subject | unit+rapid | `go test ./internal/domain/impact/ -run 'TestCompute\|TestDisjoint'` | Wave 0 |
| BLAST-01 | no --base => Broken only, "no baseline" in text and json (`baseline:false`) | script | `go test ./cmd/gruntled -run TestScripts/blast_nobase` | Wave 0 |
| BLAST-01 | exit codes 0/1/2/3, missing base => 3, bad format => 2, flags after path | script | `go test ./cmd/gruntled -run TestScripts/blast_exitcodes` | Wave 0 |
| BLAST-01 | JSON golden: version 1, kind blast, `[]` never null, deterministic | presenter golden/script | `go test ./internal/interfaces/presenter/ -run Blast` | Wave 0 |
| BLAST-01 | application: base error => Stage "baseline", cur error => "current", nil base ok | unit | `go test ./internal/application/blasting/` | Wave 0 |
| BLAST-02 | finding in both (even line-shifted) never Broken; new finding Broken | unit+script | `go test ./internal/domain/impact/ -run TestNewFindings` | Wave 0 |
| BLAST-02 | add/remove variable or output => direct consumers Impacted; comment-only edit / reordered names => none; unknown module => none; one hop only (consumer-of-consumer not listed) | unit+script | `go test ./internal/domain/impact/ -run TestSurface` ; `TestScripts/blast_impacted` | Wave 0 |

### Sampling Rate
- Per task commit: quick run command
- Per wave merge: full suite command
- Phase gate: full suite + `gofmt -l .` empty + `go mod tidy -diff` clean + binary no-net/no-exec proof (`scripts/check-architecture.sh`) green before verify-work

### Wave 0 Gaps
- [ ] `internal/domain/impact/*_test.go` (table + rapid: disjoint, sorted, line-shift invariance, identical trees => empty)
- [ ] `internal/application/blasting/blasting_test.go` (fake ports as in `checking/check_test.go`)
- [ ] `internal/interfaces/presenter/blast_test.go`
- [ ] `cmd/gruntled/testdata/script/blast_*.txtar` (nobase, exitcodes, impacted, shifted-line); use two dirs in the txtar workdir
- [ ] docs/cli.md blast section (run doc tests afterwards)
- Framework install: none needed

## Sources
### Primary (HIGH, read in-repo)
- internal/domain/diagnostic/{diagnostic,set}.go (Key includes Line/Column; Diff)
- internal/domain/analysis/grt001-003.go (message contents), internal/domain/repograph/{surface,graph,unit}.go
- internal/application/{checking,indexing,ports}, cmd/gruntled/main.go (parseArgs, exit codes, runCheck/runGraph)
- internal/interfaces/presenter/{graph,json,text}.go (schema versioning pattern)
- scripts/check-architecture.sh (allowlists), .planning/research/{SUMMARY,ARCHITECTURE,PITFALLS}.md, REQUIREMENTS.md

## Metadata
**Confidence:** stack HIGH (no deps); architecture HIGH (allowlists read from script); pitfalls HIGH except GRT100 message stability (MEDIUM, unverified).
**Research date:** 2026-10-08. **Valid until:** until Phase 8-11 change diagnostic.Key or Check wiring.
