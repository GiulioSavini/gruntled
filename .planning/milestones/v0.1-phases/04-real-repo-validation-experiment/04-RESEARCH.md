# Phase 4: Real-Repo Validation Experiment - Research

**Researched:** 2026-09-25
**Domain:** validation/benchmark harness for a Go static-analysis CLI (`gruntled`) against real and synthetic Terragrunt repositories
**Confidence:** MEDIUM-HIGH (built on directly-measured PREP.md data and read Phase 2/3 source; the one LOW-confidence item is flagged explicitly below)

<phase_requirements>
## Phase Requirements

| ID | Description | Research Support |
|----|-------------|-------------------|
| VALID-02 | Golden tests cover fixture repositories (hand-written + full-scale synthrepo), asserting the exact expected diagnostic set | Architecture Patterns "Golden test harness"; reuses `synthrepo.Manifest.Expected` + the projection/sort/diff pattern already proven by Phase 3's `TestOracle`/`TestMutationDiff` (03-05-PLAN.md) |
| VALID-03 | `gruntled check` reports zero diagnostics on the unmutated primary corpus | PREP.md §2-3 (verified: 65 units, 22 refs, corpus commit `e6c55d11f...`); reuses the `GRUNTLED_CORPUS` env-gate pattern already in `TestCorpusSmoke` (`internal/infrastructure/terragrunt/integration_test.go:496`) |
| VALID-04 | Every reference broken by an injected output rename/deletion is reported | PREP.md §5 (exact ground truth: `role_name`→8 refs in 8 files, `iac.src/s3_runtime/state.tf`); Common Pitfalls #1, #2, #5 |
| VALID-05 | `terragrunt hcl validate` is confirmed NOT to report the injected mutation | PREP.md §3-4 (plain `hcl validate` exit 0 on mutated corpus, confirmed directly); Common Pitfalls #4, #10 |
| VALID-06 | A reproducible benchmark shows `gruntled check` faster than `terragrunt hcl validate` | PREP.md §3 baseline timings + §4.5 noise-floor caveat; Architecture Patterns "Benchmark harness" (Go `b.Loop()`, confirmed current idiom for go.mod's `go 1.27`) |
</phase_requirements>

## Summary

Phase 4 does not need a new technology stack — it needs a disciplined **test/experiment harness** built entirely from what Phases 1-3 already produced (`synthrepo`, `checking.Check`, the JSON presenter, `diagnostic.Diff`) plus the pinned, checksummed `terragrunt` binary that `~/.cache/gruntled-phase4/PREP.md` already prepared and measured. Every piece of ground truth this phase needs — corpus shape, the exact mutation and its 8 broken references, the fact that plain `terragrunt hcl validate` does not catch it, and honest baseline timings — was already measured by hand in PREP.md and should be treated as verified fact, not re-derived from scratch. Phase 4's job is to turn that manual investigation into **committed, automated (but not CI-triggered), reproducible** Go tests and a results document.

The one real risk is staleness: PREP.md's exact corpus numbers (65 units, 3 config-unknown, 22 references) were measured against the current loader, but `02-REVIEW.md` lists six confirmed false-positive-risk gaps (G1-G6) in Phase 2 that must land as gap-closure plans before Phase 3 is verified (per `03-CONTEXT.md`: "Phase 2 follow-ups are NOT in this phase... closed as Phase 2 gap-closure plans before Phase 2 is verified"). If those gaps change the graph shape even slightly (e.g. G3's include-target-unit handling), the exact unit/reference counts in PREP.md could shift by execution time. Plans should therefore prefer **structural assertions and independently-recomputed oracles** (grep-based ground truth, exactly as PREP.md itself did for the mutation) over copy-pasting PREP.md's specific numbers into hardcoded test expectations wherever avoidable — except for the corpus's git commit SHA and the pinned tool versions/hashes, which are genuinely fixed points.

**Primary recommendation:** Build three tiers of tests, all as ordinary `_test.go` files following the exact `GRUNTLED_CORPUS`-style env-gate pattern already established in `internal/infrastructure/terragrunt/integration_test.go`: (1) always-on, CI-safe golden tests (hand-written + full-scale synthrepo, no external corpus, no external binary) that assert exact diagnostic sets; (2) env-gated real-corpus tests (`GRUNTLED_CORPUS`) for VALID-03/04 that copy the pinned corpus to a scratch temp dir before mutating; (3) env-gated comparison/benchmark tests (`GRUNTLED_CORPUS` + a new `GRUNTLED_TERRAGRUNT_BIN`) for VALID-05/06 that shell out to the pinned `terragrunt` binary via `os/exec` — permitted here because `binary-no-net-no-exec` only scans `cmd/gruntled`'s non-test dependency graph, not test files. None of tiers 2-3 are added to `.github/workflows/ci.yml`; they run locally against the assets PREP.md already pinned, and their results are hand-recorded into a new `docs/validation.md`.

## Standard Stack

### Core
| Tool | Version | Purpose | Why Standard |
|------|---------|---------|---------------|
| Go `testing` (stdlib) | go 1.27 (per go.mod) | golden tests, corpus tests, benchmarks | Already the project's only test framework; zero new dependencies |
| Go `testing.B.Loop()` | available since Go 1.24 | reproducible in-process benchmark of `checking.Check` | Confirmed current idiom: prevents compiler over-optimization of the loop body, excludes setup/cleanup from timing automatically, replaces the older manual `for i := 0; i < b.N` pattern — see [go.dev/blog/testing-b-loop](https://go.dev/blog/testing-b-loop) |
| `os/exec` (stdlib, test-only) | stdlib | shell out to the pinned `terragrunt` binary for the comparison/benchmark | Only usable in `_test.go` files — `binary-no-net-no-exec`'s `bin_deps=$(go list -deps ./cmd/gruntled)` intentionally omits `-test`, and 03-02-PLAN.md's rule comment says so explicitly ("test-only deps such as testscript legitimately use os/exec") |
| `terragrunt` v1.1.6 (external, pinned) | pinned by PREP.md | the comparator for VALID-05/06 | Verified SHA256 against the release's own `SHA256SUMS`; lives only at `~/.cache/gruntled-phase4/bin/terragrunt`, never installed system-wide |

### Supporting
| Tool | Version | Purpose | When to Use |
|------|---------|---------|-------------|
| `synthrepo` (in-repo) | current | golden tests at "full scale" + a fresh mutation for the benchmark's synthetic leg if desired | Already the Phase 1 test substrate; `Manifest.Expected` is the exact oracle |
| `diagnostic.Diff` (in-repo) | current | before/after mutation diff, corpus and golden alike | Already used by 03-05's `TestMutationDiff`; `func Diff(prev, next Set) (added, removed Set)` |
| `os.OpenRoot`/`fs.FS` (stdlib) | stdlib | read-only corpus access for the unmutated VALID-03 check | Already the pattern in `cmd/gruntled/main.go`'s `runCheck` and `TestCorpusSmoke` |

### Alternatives Considered
| Instead of | Could use | Tradeoff |
|------------|-----------|----------|
| Go `testing.B` benchmark for gruntled's own timing | `hyperfine` | Rejected per the task's hard rule (no system-wide installs) and PREP.md's own finding that hyperfine was not installed and was not added; Go's stdlib benchmark is reproducible and needs nothing extra |
| `os/exec`-based Go test for the terragrunt comparison | A standalone shell script (`scripts/bench-corpus.sh`) | Both are legitimate; a shell script is simpler to read but loses Go's `-benchtime=Nx` control and structured JSON output (`go test -bench . -json`). Recommend a small Go test/benchmark file for the comparison logic (median, sample count, exit-code capture) with a thin script only if the executor prefers manual runs outside `go test` |
| Rename mutation as primary VALID-04 evidence | Deletion | PREP.md §5 recommends rename-first: deletion risks colliding with the corpus's pre-existing "module has no outputs, but mock outputs provided" warnings (11 files use `mock_outputs_merge_with_state`), making the signal ambiguous. A rename keeps output *count* identical and isolates the name-mismatch signal cleanly. Still do one deletion case, on a *different* module, per PREP.md's explicit recommendation |

**No `go get`/`go install` needed.** All of Phase 4's Go-side work uses only the stdlib and already-vendored packages (`hashicorp/hcl/v2`, `zclconf/go-cty` are unaffected; no new `go.mod` entries required for the corpus/benchmark tests themselves — `os/exec` and `testing` are stdlib).

## Architecture Patterns

### Recommended Project Structure
```
cmd/gruntled/
├── golden_test.go          # VALID-02: hand-written + full-scale synthrepo, always-on (no env gate)
├── testdata/golden/         # hand-written fixture repos (txtar or plain files under t.TempDir(), your choice — see Open Question 3)
├── corpus_test.go           # VALID-03/04: GRUNTLED_CORPUS-gated, copies corpus to scratch, mutates, diffs
├── terragrunt_gap_test.go   # VALID-05: GRUNTLED_CORPUS + GRUNTLED_TERRAGRUNT_BIN-gated, exec.Command the pinned binary
└── bench_test.go            # VALID-06: GRUNTLED_CORPUS + GRUNTLED_TERRAGRUNT_BIN-gated Benchmark functions
docs/
└── validation.md            # results doc: tool versions, SHA256, corpus commits, benchmark numbers, mutation ground truth
```
All four new `_test.go` files live in `package main` (or `main_test`) alongside the existing `cmd/gruntled/main_test.go` and `e2e_test.go` from Phase 3 — this is where `checking.Check`, the presenters and `run()` are already wired together, and it is the one place `os/exec` is architecturally permitted without weakening `binary-no-net-no-exec`.

### Pattern 1: Env-gated corpus test (already established, reuse verbatim)
**What:** Skip a test unless an environment variable names a real, locally-prepared asset. This is the exact mechanism Phase 2 already uses.
**When to use:** Any test needing the real corpus or the pinned terragrunt binary — never make `go test ./...` require network or an external binary.
**Example (existing code, Phase 2):**
```go
// Source: internal/infrastructure/terragrunt/integration_test.go:496-520
func TestCorpusSmoke(t *testing.T) {
	corpus := os.Getenv("GRUNTLED_CORPUS")
	if corpus == "" {
		t.Skip("GRUNTLED_CORPUS not set")
	}
	root, err := os.OpenRoot(corpus)
	...
}
```
Phase 4 should add exactly one more env var, e.g. `GRUNTLED_TERRAGRUNT_BIN`, for the pinned binary path (`~/.cache/gruntled-phase4/bin/terragrunt` locally; never hardcode that path in test code — it is prep-cache-specific to this machine). Both gates compose: a comparison test skips unless *both* are set.

### Pattern 2: Scratch-copy before mutation (PREP.md's own discipline)
**What:** Never mutate the pinned corpus checkout in place. Copy it to a fresh `t.TempDir()`, mutate the copy, and leave the original untouched — exactly what PREP.md itself did ("in a scratch copy, not the corpus checkout — no lasting changes were made to `~/.cache/gruntled-phase4/corpus/primary`").
**When to use:** Every VALID-04/05/06 test that needs a mutated tree.
**Example (pattern, not existing code — to be written this phase):**
```go
// Pattern: copy corpus to scratch, then mutate only the scratch copy.
func copyCorpus(t *testing.T, src string) (dst string) {
	t.Helper()
	dst = t.TempDir()
	err := filepath.WalkDir(src, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel, _ := filepath.Rel(src, p)
		data, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		return os.WriteFile(target, data, 0o644)
	})
	if err != nil {
		t.Fatalf("copyCorpus: %v", err)
	}
	return dst
}
```
Skip `.git` when walking (the corpus is a detached-HEAD checkout; copying `.git` is unnecessary weight and risks confusing tooling that walks hidden dirs — though gruntled's own walker already skips no dotdirs by name, only `.terragrunt-cache`/`.terraform`/symlinks per PARSE-06, so `.git` would otherwise be walked as an ordinary directory with no `terragrunt.hcl` inside it; harmless but wasteful to copy).

### Pattern 3: Independently-recomputed oracle (not the prototype's cached numbers)
**What:** Before asserting an expected diagnostic set against the real corpus, recompute the ground truth directly from the pinned commit with a `grep`-equivalent scan — exactly as PREP.md did (`grep -rn 'dependency\.<unit>\.outputs\.<old_name>' --include=terragrunt.hcl .`) — rather than trusting a previously-recorded count.
**When to use:** VALID-04's expected-broken-reference set. Can be done once in Go with `filepath.WalkDir` + `strings.Contains`/`regexp`, independent of gruntled's own parser, so the oracle is not circular.
**Why:** This is the single most important trustworthiness property of the whole experiment — "gruntled agrees with gruntled" proves nothing; "gruntled agrees with an independently-computed textual scan" does.

### Pattern 4: Golden-test oracle comparison (reuse Phase 3's exact idiom)
**What:** Decode `gruntled check --format json`'s output, project the GRT001 diagnostics to `{file, line, column, unit}`, sort with the same comparator as `Manifest.Expected`, and compare for exact set equality (not superset/subset).
**When to use:** VALID-02's synthrepo-at-scale case; this is already proven at 60 units by Phase 3's `TestOracle` (`cmd/gruntled/e2e_test.go`, from 03-05-PLAN.md) — Phase 4 should either raise the scale and reuse the same helper, or extract a shared helper function so `TestOracle` (Phase 3, CI-facing) and Phase 4's golden test don't duplicate the projection/sort logic. Confirm with the CLI-01 JSON schema v1 fields: `code, severity, file, line, column, unit (omitempty), message`.
**Example (existing code, Phase 3):**
```go
// Source: cmd/gruntled/e2e_test.go (03-05-PLAN.md Task 1, TestOracle)
// runCLI("check", "--format", "json", dir) -> code 1
// decode JSON, project GRT001 subset to {file,line,column,unit}
// compare against Manifest.Expected projected the same way, both sorted identically
```

### Pattern 5: Benchmark with `b.Loop()` for gruntled, external-process sampling for terragrunt
**What:** `checking.Check` runs in-process, so time it with Go's modern benchmark loop. `terragrunt hcl validate` is an external binary, so time it by shelling out N times and computing the median by hand (Go's `testing.B` *can* wrap `exec.Command` inside `b.Loop()`, but process-spawn benchmarks are noisier than in-process ones and a hand-rolled median-of-N with explicit sample printing is more transparent for a results document than a `ns/op` figure buried in `go test -bench` output).
**Example (pattern for the in-process leg):**
```go
// Pattern for cmd/gruntled/bench_test.go — verified current idiom (Go 1.24+, go.mod declares go 1.27):
// Source: https://go.dev/blog/testing-b-loop
func BenchmarkGruntledCheck(b *testing.B) {
	corpus := os.Getenv("GRUNTLED_CORPUS")
	if corpus == "" {
		b.Skip("GRUNTLED_CORPUS not set")
	}
	root, err := os.OpenRoot(corpus)
	if err != nil {
		b.Fatalf("OpenRoot: %v", err)
	}
	defer root.Close()
	fsys := root.FS()
	for b.Loop() {
		if _, err := checking.Check(context.Background(), terragrunt.NewLoader(fsys), tfsurface.NewReader(fsys)); err != nil {
			b.Fatalf("Check: %v", err)
		}
	}
}
```
```go
// Pattern for the terragrunt leg — median-of-N via exec.Command, gated on both env vars:
func timeTerragruntValidate(t testing.TB, bin, dir string, n int) (median time.Duration, samples []time.Duration) {
	for i := 0; i < n; i++ {
		start := time.Now()
		cmd := exec.Command(bin, "hcl", "validate", "--working-dir", dir)
		cmd.Env = append(os.Environ(), "PATH="+filepath.Dir(bin)) // pinned bin only, per hard rule
		_ = cmd.Run() // exit code is expected/checked separately; timing only here
		samples = append(samples, time.Since(start))
	}
	sort.Slice(samples, func(i, j int) bool { return samples[i] < samples[j] })
	return samples[len(samples)/2], samples
}
```
Record machine info (`runtime.NumCPU()`, `runtime.GOOS`/`GOARCH`, and ideally a `uname -a`/CPU model string read once) alongside the numbers in `docs/validation.md`, per PREP.md's own header ("Machine: Linux WSL2, Intel i5-1335U, 12 logical cores, 7.6GiB RAM") — the benchmark is only reproducible if the machine is documented, since VALID-06 makes no absolute-time claim, only a relative one on the same machine.

### Anti-Patterns to Avoid
- **Wiring the corpus/terragrunt tests into `.github/workflows/ci.yml`:** violates the hard rule ("no outward-facing GitHub actions") and the existing CI design (2 jobs, `check` + `architecture`, no network, no external binaries). Env-gate keeps `go test ./...` green in CI without any workflow change.
- **Hardcoding `~/.cache/gruntled-phase4/bin/terragrunt` or `~/.cache/gruntled-phase4/corpus/primary` as string literals in test code:** these are this-machine paths from PREP.md's prep step, not portable. Always take them from env vars (`GRUNTLED_CORPUS`, `GRUNTLED_TERRAGRUNT_BIN`), consistent with the existing `GRUNTLED_CORPUS` convention.
- **Using `terragrunt hcl validate --inputs --strict` as the VALID-05 comparator:** PREP.md §4.1 measured it failing (exit 1) on the *unmutated* primary corpus for reasons unrelated to `dependency.outputs` (11 "unused input" errors escalated by `--strict`). Using it would make "confirmed not to report the injected mutation" meaningless, since it already reports unrelated failures. Use plain `terragrunt hcl validate` only (confirmed exit 0 on unmutated, exit 0 with zero `role_name` mentions on mutated).

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|--------------|-----|
| "Does gruntled's diagnostic set exactly equal the expected set" | A custom deep-equal/pretty-diff routine | `diagnostic.Diff(prev, next Set) (added, removed Set)` (already exists) | Already used by 03-05's `TestMutationDiff`; canonical order and dedup-by-Key semantics are already correct there |
| A synthetic repository generator for scale testing | A bespoke fixture-tree builder | `internal/testsupport/synthrepo` (`Spec`, `Render`, `Generate`, `Manifest.Expected`) | Already deterministic, already the Phase 1 test substrate, already proven at 60 units in Phase 3's `TestOracle` |
| Comparing gruntled vs. terragrunt output | A custom Terragrunt HCL re-implementation to "simulate" validate | Actually shell out to the real, pinned `terragrunt` binary via `os/exec` in test code | The whole point of VALID-05 is confirming what the *real* tool does; a simulation would beg the question. `os/exec` is permitted here because it's test-only |
| Reading/copying the corpus without accidentally writing into the pinned checkout | Ad hoc shell scripts run by hand outside `go test` | A small Go helper (`copyCorpus`) inside the test file, using `t.TempDir()` | Keeps the whole experiment reproducible via `go test`, consistent with how Phase 3's `TestNoWrites` (03-05-PLAN.md) already snapshots/copies fixture trees in Go, not shell |

**Key insight:** Every piece of machinery Phase 4 needs (generator, diff, env-gate pattern, read-only tree access) already exists in this codebase from Phases 1-3. The only genuinely new code is: (a) a corpus-copy helper, (b) an independently-recomputed grep-style oracle for the real corpus mutation, (c) an `os/exec` wrapper around the pinned terragrunt binary, and (d) the results document. Resist the temptation to build anything more general — this phase is terminal; if it fails, the project stops, so minimal, transparent, directly-verifiable code matters more than reusability.

## Common Pitfalls

### Pitfall 1: Mutating the pinned corpus checkout in place
**What goes wrong:** A mutation test that edits `~/.cache/gruntled-phase4/corpus/primary` directly leaves the "pinned" corpus no longer pinned; re-running VALID-03 afterward could see the mutation and wrongly appear to fail (a false regression) or, worse, silently normalize a corrupted baseline.
**Why it happens:** It's the path of least resistance — the corpus is already on disk, `sed -i` is one command.
**How to avoid:** Always copy to `t.TempDir()` (Pattern 2) before any mutation. PREP.md itself modeled this discipline explicitly ("in a scratch copy, not the corpus checkout").
**Warning signs:** A second, unrelated test run producing different results than the first; `git status` inside the corpus clone showing unexpected modifications.

### Pitfall 2: Stale expected-count assertions vs. Phase 2 gap-closure
**What goes wrong:** Hardcoding "65 units, 3 config-unknown, 22 references" from PREP.md into a Phase 4 test, then Phase 2's gap-closure plans (G1-G6 in `02-REVIEW.md`) land before Phase 3/4 execute and shift those counts (e.g. G3's include-target-unit handling could remove or reclassify a unit; G4's config_path-non-default-file guard could change a dependency's resolution).
**Why it happens:** PREP.md's numbers were measured against the loader as it existed 2026-09-25, before `03-CONTEXT.md`'s stated precondition ("the Phase 2 gap-closure plans... are merged before Phase 3 starts") is necessarily fulfilled.
**How to avoid:** For VALID-03 assert the *structural* invariant (zero diagnostics, specifically zero `GRT001`/`GRT100`), not a specific unit count. For VALID-04, recompute the expected broken-reference set independently at test time (Pattern 3) rather than hardcoding "8 refs in 8 files" — though that number is worth cross-checking against, as a sanity assertion with a clear failure message pointing back to this note, not a silent hardcode.
**Warning signs:** A VALID-03/04 test failing immediately after a Phase 2 gap-closure plan lands, with a diff that looks like a legitimate loader improvement rather than a regression.

### Pitfall 3: `--inputs --strict` masquerading as the "stronger" comparator
**What goes wrong:** Someone re-adds `--inputs --strict` believing it makes VALID-05's "gap" claim stronger (it validates more). PREP.md measured the opposite: it fails on the *unmutated* corpus for unrelated reasons and it writes 3.5MB / 131 subdirectories into the repo (violating the same no-side-effects principle CLI-05 holds gruntled to).
**Why it happens:** `--inputs --strict` sounds thorough, and PROJECT.md's own "What already exists" section name-checks it as covering some wiring-adjacent classes.
**How to avoid:** Use only plain `terragrunt hcl validate` for VALID-05, exactly as PREP.md §4.1/§4.2/§5 concludes.
**Warning signs:** The "clean corpus" baseline no longer reporting exit 0, or a `.terragrunt-cache/` directory appearing inside a corpus checkout after a test run.

### Pitfall 4: Deletion mutation colliding with pre-existing "no outputs" noise
**What goes wrong:** Deleting all outputs from a module target makes it indistinguishable from the corpus's existing "module has no outputs, but mock outputs provided" pattern (already present and warned-about in every PREP.md run, 11 files use `mock_outputs_merge_with_state`), muddying whether a detected diagnostic is really evidence of catching *this* mutation.
**Why it happens:** Deletion is the more "obvious" mutation to write first.
**How to avoid:** Per PREP.md §5 recommendation 1-3: rename is the primary VALID-04 mutation (on `iac.src/s3_runtime`'s `role_name`, with the pre-verified 8-reference ground truth); do one deletion mutation too, but on a *different* module, to exercise the "no longer exists at all" path distinctly, and expect its diagnostic set/count to be separately derived, not assumed identical in shape to the rename case.
**Warning signs:** A deletion-mutation test's expected count silently matching the rename case's count without independent verification.

### Pitfall 5: Benchmark noise swamping a real signal
**What goes wrong:** With only ~0.13-0.19s of wall time on the primary corpus (PREP.md §3), process-start overhead and OS scheduling noise can dominate a small number of runs, making "faster" or "slower" statistically meaningless.
**Why it happens:** The corpus is small (65 units); both tools complete in well under a quarter second.
**How to avoid:** PREP.md §4.5 explicitly recommends "hyperfine's default warm-up + ≥10 runs, or an equivalent" over its own 5-run fallback for the *actual* Phase 4 numbers. Use `testing.B`'s `b.Loop()` for the in-process gruntled leg (Go's benchmark framework auto-calibrates run count for a stable measurement) and at least 10-20 samples with median (not mean) for the external terragrunt leg, since medians resist outlier process-spawn hiccups better than means.
**Warning signs:** Repeated runs of the benchmark giving inconsistent "winner" results.

### Pitfall 6: Using `denis256/terragrunt-tests` as a timing/comparison corpus
**What goes wrong:** That corpus (1149 units, deliberately broken fixtures) makes Terragrunt v1.1.6 **hard-crash** (`terragrunt.ERROR "not a string"`) partway through — PREP.md only got one usable timing sample (65.9s) before concluding it's unsuitable for the speed benchmark.
**Why it happens:** It's the largest available corpus, so it's tempting to use for "scale" testing.
**How to avoid:** Use it only for VALID-02's hand-written/robustness golden fixtures (gruntled must not crash and must report a diagnostic, not silently accept broken HCL — this exercises PARSE-05-style robustness), never for VALID-05/06's speed/gap comparison. The primary corpus is the only one PREP.md validated as clean-compatible with the pinned Terragrunt version for that purpose.
**Warning signs:** A benchmark test hanging or crashing instead of completing.

### Pitfall 7: Terragrunt version drift eliminating comparator repos
**What goes wrong:** Two of the four candidate repos (`secret`, most of `gc-articles`'s 36 errors) fail under the pinned Terragrunt v1.1.6 purely because they use the legacy unnamed `include { path = ... }` syntax, which v1.1.6 rejects (labels are now mandatory) — unrelated to GRT001 entirely.
**Why it happens:** Those repos predate a Terragrunt syntax-compatibility break; gruntled's own parser (per PARSE-01/PARSE-03) doesn't require the label, so gruntled can analyze them even though the pinned `terragrunt` binary can't validate them.
**How to avoid:** Use only the primary corpus (`aws-solutions-library-samples/guidance-for-iso20022-messaging-workflows-on-aws`) for the VALID-05 "terragrunt confirms the gap" comparison, since it's the only one both tools can actually run against cleanly. `secret`/`gc-articles` remain usable only as small secondary gruntled-only sanity checks if desired (Claude's Discretion, not required by any VALID-* requirement).
**Warning signs:** A comparator repo failing to validate at all, for a reason that has nothing to do with `dependency.outputs`.

## Code Examples

Verified patterns, all from files already in this repository (read directly, HIGH confidence — no external library involved):

### Env-gated corpus test skeleton
```go
// Source: internal/infrastructure/terragrunt/integration_test.go:496-520 (existing, Phase 2)
func TestCorpusSmoke(t *testing.T) {
	corpus := os.Getenv("GRUNTLED_CORPUS")
	if corpus == "" {
		t.Skip("GRUNTLED_CORPUS not set")
	}
	root, err := os.OpenRoot(corpus)
	if err != nil {
		t.Fatalf("OpenRoot(%s): %v", corpus, err)
	}
	defer root.Close()
	res := build(t, root.FS())
	// ... assertions on res.Graph
}
```

### checking.Check call shape (what every corpus/benchmark test drives)
```go
// Source: .planning/phases/03-grt001-diagnostic-cli/03-01-PLAN.md interfaces block
// internal/application/checking
type Report struct { Graph *repograph.RepositoryGraph; Diagnostics diagnostic.Set }
func Check(ctx context.Context, units ports.UnitLoader, surfaces ports.SurfaceReader) (Report, error)
```

### JSON schema v1 fields (for decoding CLI output in corpus/golden tests)
```go
// Source: .planning/phases/03-grt001-diagnostic-cli/03-02-PLAN.md (json.go behavior block)
// {"version":1,"diagnostics":[{"code":...,"severity":...,"file":...,"line":...,"column":...,"unit":...(omitempty),"message":...}],
//  "unknown_units":[{"path":...,"status":...,"reason":...}],"unknown_modules":[{"path":...,"reason":...}],
//  "summary":{"units":...,"resolved":...,"module_unknown":...,"config_unknown":...,"unknown_modules":...,"errors":...,"warnings":...}}
```

## State of the Art

| Old Approach | Current Approach | When Changed | Impact |
|--------------|-------------------|---------------|--------|
| `for i := 0; i < b.N; i++ { ... }` manual benchmark loop | `for b.Loop() { ... }` | Go 1.24 (2025); go.mod here declares `go 1.27`, so this is available and is the current idiom | More accurate measurement (excludes setup/cleanup automatically, resists compiler over-optimizing the loop body) — use for the in-process `checking.Check` benchmark |
| `hyperfine` for CLI wall-clock benchmarking | A manual N-run + median script/Go helper, per this task's hard rule and PREP.md's own fallback | N/A (project-specific constraint: no system-wide installs) | Slightly more code, fully reproducible without any tool install |
| `terragrunt hcl validate --inputs --strict` as the "richer" comparator | Plain `terragrunt hcl validate` only | Discovered during PREP.md's 2026-09-25 measurement, not a Terragrunt version change | `--inputs --strict` is unusable as a fair baseline (fails on the clean corpus for unrelated reasons, writes inside the repo) |

**Deprecated/outdated:** Nothing Terragrunt-side is deprecated for this phase's purposes; the relevant "state of the art" shift is entirely about which flags/tools to use for a *fair, reproducible* comparison, not about any library API.

## Open Questions

1. **Have the Phase 2 gap-closure plans (G1-G6, `02-REVIEW.md`) landed by the time Phase 4 executes?**
   - What we know: `03-CONTEXT.md` states they must land before Phase 3 starts, and Phase 3 is planned (not yet executed) as of this research. `STATE.md`/`ROADMAP.md` mark Phase 2 "complete" but do not confirm the gap-closure plans' status distinctly from the five numbered 02-0X plans already read.
   - What's unclear: whether "Phase 2 complete" already includes G1-G6, or whether a separate gap-closure wave is still pending.
   - Recommendation: treat PREP.md's exact corpus counts (65 units, 3 config-unknown, 22 references, 8-reference mutation) as a **cross-check**, not a hardcoded assertion. Assert structural properties (zero errors on unmutated; the independently-grep-derived reference set exactly matched on mutated) so the tests remain correct regardless of exactly when the gap-closure work lands. If the executing planner confirms G1-G6 are already merged and stable by Phase 4 time, hardcoding the specific numbers as an additional sanity check (with a comment citing this research) is reasonable belt-and-braces.

2. **What "full scale" means for the synthrepo golden test (VALID-02).**
   - What we know: the primary real corpus has 65 units / 22 references; Phase 3's `TestOracle` already exercises `Spec{Units: 60, IncludeDepth: 3, DependencyFanout: 3, Seed: 7, Errors: [4×BadOutputRef]}` at CI-facing scale.
   - What's unclear: no CONTEXT.md exists for this phase (none was gathered), so there's no locked decision on the exact scale Phase 4 should add beyond what Phase 3 already covers for VALID-02's "generated at full scale" wording.
   - Recommendation (Claude's Discretion, since nothing locks it): pick a spec noticeably larger than Phase 3's CI-facing 60-unit case — e.g. `Units: 200-500`, `IncludeDepth: 4`, `DependencyFanout: 5`, several `BadOutputRef` instances — to genuinely exercise "full scale" distinctly from Phase 3's smaller CI oracle, and keep it env-gate-free (synthrepo needs no external corpus/binary) so it still runs in CI's `check` job. Confirm `Spec.Validate()`/`Render`'s own limits are not exceeded (e.g. too many `BadOutputRef` entries vs. available references) by checking the returned error, as 03-05-PLAN.md's TestOracle note already anticipates ("If Render rejects the spec... lower the counts and note the change").

3. **Should hand-written golden fixtures reuse real snippets from `denis256/terragrunt-tests`, or be authored fresh?**
   - What we know: Phase 2's fuzz seeds already inline 8 real files from that repo (`fuzz_test.go`, per `02-05-SUMMARY.md`); that repo is MIT-licensed and used only as local fixtures, not redistributed as a corpus.
   - What's unclear: whether Phase 4's "hand-written" golden fixtures (VALID-02) should follow that same precedent (a few excerpted real files) or be purpose-built minimal repos, given that repo's crash-proneness at scale (Pitfall 6) and CI must stay network-free.
   - Recommendation: author small, purpose-built fixtures inline (txtar via `testscript.Params{Dir: "testdata/golden"}` reusing Phase 3's existing txtar infrastructure, or `os.WriteFile` into `t.TempDir()` as `e2e_test.go` already does) covering the DIAG-03 shapes and structural edge cases explicitly, rather than depending on an external repo's exact contents for a golden-test oracle. A couple of excerpted snippets (not a clone) for one robustness case, mirroring the fuzz-seed precedent, are reasonable if a specific gap needs real-world shape.

4. **Env var name for the pinned terragrunt binary.**
   - What we know: no existing convention beyond `GRUNTLED_CORPUS`.
   - Recommendation: `GRUNTLED_TERRAGRUNT_BIN`, matching the existing `GRUNTLED_` prefix, pointing at the pinned `terragrunt` binary path (`~/.cache/gruntled-phase4/bin/terragrunt` locally, never hardcoded).

5. **Exact location/name of the results document.**
   - What we know: `docs/` currently only has a `superpowers/` subdirectory; `docs/cli.md` is created by Phase 3 (03-04-PLAN.md) as the CLI reference, owned entirely by this project (not the README-owning agent).
   - Recommendation: `docs/validation.md`, matching `docs/cli.md`'s location/style (plain English, no marketing tone, no AI attribution), recording: pinned tool versions + SHA256 (from PREP.md §1), corpus repository + commit SHAs (from PREP.md §2), the mutation ground truth (PREP.md §5), and the benchmark numbers with machine info (PREP.md's own header line as a template). This is a new, phase-4-owned file — no other agent claims it.

## Validation Architecture

### Test Framework
| Property | Value |
|----------|-------|
| Framework | Go stdlib `testing` (go 1.27, per go.mod); `testing.B.Loop()` for benchmarks; existing `rogpeppe/go-internal` testscript already a test-only dependency from Phase 3 (03-03-PLAN.md), reusable for txtar-based golden fixtures if desired |
| Config file | none — `go test` needs no config; env gates (`GRUNTLED_CORPUS`, new `GRUNTLED_TERRAGRUNT_BIN`) replace config files, matching the existing `TestCorpusSmoke` convention |
| Quick run command | `go test -count=1 ./cmd/gruntled -run 'TestGolden' -v` (VALID-02 only, always runs, no env needed) |
| Full suite command | `test -z "$("$(go env GOROOT)/bin/gofmt" -l .)" && go mod tidy -diff && go vet ./... && go run honnef.co/go/tools/cmd/staticcheck@v0.8.1 ./... && go test -count=1 ./... && bash scripts/check-architecture.sh && bash scripts/test-check-architecture.sh` (the same pipeline every prior phase uses; env-gated corpus/benchmark tests skip silently inside this when the env vars are unset) |

### Phase Requirements → Test Map
| Req ID | Behavior | Test Type | Automated Command | File Exists? |
|--------|----------|-----------|---------------------|--------------|
| VALID-02 | Golden tests (hand-written + full-scale synthrepo) assert the exact expected diagnostic set | unit/golden | `go test -count=1 ./cmd/gruntled -run TestGolden -v` | ❌ Wave 0 (`cmd/gruntled/golden_test.go`, plus fixture data) |
| VALID-03 | `gruntled check` reports zero diagnostics on the unmutated primary corpus | integration (env-gated) | `GRUNTLED_CORPUS=~/.cache/gruntled-phase4/corpus/primary go test -count=1 ./cmd/gruntled -run TestCorpusClean -v` | ❌ Wave 0 (`cmd/gruntled/corpus_test.go`) |
| VALID-04 | Every reference broken by an injected rename/deletion is reported, exactly | integration (env-gated) | `GRUNTLED_CORPUS=~/.cache/gruntled-phase4/corpus/primary go test -count=1 ./cmd/gruntled -run TestCorpusMutation -v` | ❌ Wave 0 (same file, additional test func) |
| VALID-05 | `terragrunt hcl validate` (plain) does not report the injected mutation | integration (env-gated, needs pinned binary) | `GRUNTLED_CORPUS=... GRUNTLED_TERRAGRUNT_BIN=~/.cache/gruntled-phase4/bin/terragrunt go test -count=1 ./cmd/gruntled -run TestCorpusMutationTerragruntGap -v` | ❌ Wave 0 (`cmd/gruntled/terragrunt_gap_test.go`) |
| VALID-06 | Reproducible benchmark shows gruntled faster on the same repo | benchmark (env-gated) | `GRUNTLED_CORPUS=... GRUNTLED_TERRAGRUNT_BIN=... go test -run '^$' -bench . -benchtime=20x ./cmd/gruntled` | ❌ Wave 0 (`cmd/gruntled/bench_test.go`) |

### Sampling Rate
- **Per task commit:** `go test -count=1 ./cmd/gruntled -run TestGolden -v` (VALID-02, self-contained, seconds) plus the project's usual full local suite before every push (per user memory: always test before push).
- **Per wave merge:** Full local suite (table above) — env-gated tests skip harmlessly if the executor hasn't set `GRUNTLED_CORPUS`/`GRUNTLED_TERRAGRUNT_BIN` in that shell, but MUST be run at least once per phase with both env vars set, against the exact PREP.md-pinned assets, before the phase gate.
- **Phase gate:** Full suite green (env vars unset, matching CI) **and** a full env-gated run (VALID-03 through VALID-06, env vars set to the PREP.md-pinned corpus/binary) with results transcribed into `docs/validation.md`, before `/gsd:verify-work`. This mirrors exactly what Phase 3's 03-05-PLAN.md Task 2 already did manually for its own smaller phase-gate corpus check — Phase 4 formalizes that same motion into committed, rerunnable test code.

### Wave 0 Gaps
- [ ] `cmd/gruntled/golden_test.go` — VALID-02: hand-written fixtures + a full-scale synthrepo case, always-on (no env gate), reusing the projection/sort/diff pattern from Phase 3's `TestOracle`
- [ ] `cmd/gruntled/testdata/golden/` (or equivalent inline txtar/fixture data) — the hand-written fixture repos for VALID-02
- [ ] `cmd/gruntled/corpus_test.go` — VALID-03 (`TestCorpusClean`) + VALID-04 (`TestCorpusMutation`), `GRUNTLED_CORPUS`-gated, includes the `copyCorpus` scratch-copy helper and the independently-recomputed grep-style oracle (Pattern 3)
- [ ] `cmd/gruntled/terragrunt_gap_test.go` — VALID-05, `GRUNTLED_CORPUS` + `GRUNTLED_TERRAGRUNT_BIN`-gated, `os/exec` wrapper around the pinned binary, plain `hcl validate` only (never `--inputs --strict`)
- [ ] `cmd/gruntled/bench_test.go` — VALID-06, both env vars gated, `b.Loop()` for the in-process gruntled leg, median-of-N `exec.Command` sampling for the terragrunt leg, machine info recorded alongside
- [ ] `docs/validation.md` — the results document (tool versions + SHA256 from PREP.md §1, corpus commits from PREP.md §2, mutation ground truth from PREP.md §5, benchmark numbers + machine info)
- [ ] No framework install needed — Go stdlib `testing` is already present; no new `go.mod` entries required

## Sources

### Primary (HIGH confidence)
- `~/.cache/gruntled-phase4/PREP.md` — directly-measured tool versions, SHA256 hashes, corpus commits/shapes, baseline timings, the exact mutation ground truth, and the `--inputs --strict` disqualification, all measured on this machine 2026-09-25
- `.planning/ROADMAP.md`, `.planning/REQUIREMENTS.md`, `.planning/PROJECT.md`, `.planning/STATE.md` — Phase 4 success criteria, VALID-02..06 wording, corpus identification, DIAG-03/mock_outputs findings
- `.planning/phases/03-grt001-diagnostic-cli/03-CONTEXT.md`, `03-01-PLAN.md`, `03-02-PLAN.md`, `03-03-PLAN.md`, `03-04-PLAN.md`, `03-05-PLAN.md` — the exact CLI contract (`checking.Check`, presenter signatures, JSON schema v1, exit codes, testscript/txtar conventions) Phase 4 drives
- `.planning/phases/02-parsing-graph-construction/02-05-SUMMARY.md`, `02-REVIEW.md` — the real corpus's 3 config-unknown units and their cause, plus the six confirmed Phase 2 false-positive gaps (G1-G6) that must land before this phase's numbers are trustworthy
- `internal/testsupport/synthrepo/spec.go`, `manifest.go` — the generator's exact `Spec`/`Manifest`/`ExpectedDiagnostic` contract
- `internal/infrastructure/terragrunt/integration_test.go` (`TestCorpusSmoke`) — the existing env-gate pattern to replicate
- `.github/workflows/ci.yml`, `go.mod` — confirms the 2-job, no-network CI shape and the `go 1.27` toolchain (relevant to `b.Loop()` availability)
- [go.dev/blog/testing-b-loop](https://go.dev/blog/testing-b-loop) — confirms `testing.B.Loop()` is the current (Go 1.24+) idiom for reproducible in-process benchmarks, directly relevant since go.mod declares `go 1.27`

### Secondary (MEDIUM confidence)
None needed — every finding above was verifiable against either a file already in this repository/prep cache or the official Go blog.

### Tertiary (LOW confidence)
None retained; the one genuinely uncertain item (whether Phase 2's gap-closure plans have landed by Phase 4 execution time) is documented as Open Question 1 with a concrete mitigation (structural assertions + independently-recomputed oracle) rather than presented as a fact.

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH — entirely stdlib plus already-pinned, checksum-verified external tools from PREP.md; no new dependency decisions needed
- Architecture: HIGH — every pattern either already exists in this codebase (env-gate, corpus copy discipline modeled by PREP.md, oracle projection/diff from Phase 3) or is a direct, verified stdlib idiom (`b.Loop()`)
- Pitfalls: HIGH for anything sourced from PREP.md's direct measurements (mutation ground truth, `--inputs --strict` disqualification, Terragrunt version drift, denis256 crash); MEDIUM for the Phase 2 gap-closure timing pitfall, since its resolution status wasn't directly observable from the files available to this research

**Research date:** 2026-09-25
**Valid until:** Re-verify PREP.md's pinned SHA256/corpus commits if more than ~30 days pass before Phase 4 executes (upstream releases move); re-verify the exact corpus unit/reference counts against Phase 2's actual gap-closure state immediately before writing the Phase 4 plan, per Open Question 1.
