---
phase: 13-removed-output-diagnostic-shared-blast-core
audited: 2026-10-10
auditor: gruntled-sec (code-audit, bus verdict #180, findings #177-#179)
scope: "git diff 6276296..21e419c -- ':!.planning'"
verdict: SECURED
threats_total: 20
threats_mitigated: 17
threats_accepted: 3
threats_na: 0
threats_open: 0
findings: { blocker: 0, high: 0, medium: 0, low: 0, info: 3 }
---

# Phase 13 Security

Verdict **SECURED**, 0 open threats. Every `mitigate` row was verified at file:line and by
mutation: 22 mutations in a throwaway copy (`git archive 21e419c`), 21 killed by the expected
tests, 1 doc-test mutation survived (F1, INFO).

Baseline at 21e419c: `go vet` clean, `go test ./...` green, `-count=3 -shuffle=on` green on
analysis/blasting/diagnostic/presenter, no `net`/`net/*`/`os/exec`/`plugin` in `go list -deps`
on the six release targets, `check-architecture.sh` OK (Step 9) and its self-tests pass,
`go mod verify` OK, go.mod/go.sum unchanged in the range, `govulncheck` clean.
`compare-ref.sh v0.3.0`: 244/244 identical; `COMPARE_REF_PERTURB=1` 243/244, exit 1.

Accepted executor deviations (not findings): xorshift instead of rapid in domain/application
property tests; GRT004 message built from `resolveDependency(cur)`; `Between` skips the GRT004
step when either Report has no graph (GRT001 kept, conservative); `shortBase` temp dirs in
`more07_test.go`.

## Threat register

| ID | Disposition | Status | Evidence |
|----|-------------|--------|----------|
| T-13-01-1 | mitigate | CLOSED | `resolveReference` `grt001.go:107-120` (rows 1,4,5 then 2,3; all silent, order-free); `UnknownOutputs` `grt001.go:42-45`; `TestDIAG03` unchanged; `git diff v0.3.0 -- cmd/gruntled/testdata` touches only `blast_grt004.txtar` (new) and `blast_impacted.txtar` (GRT001→GRT004); mutations row 2 / row 3 removed killed by `TestDIAG03`, `TestRemovedOutputsDIAG03Parity`, `TestRemovedOutputsMatrix` |
| T-13-01-2 | mitigate | CLOSED | `fires` uses `resolveReference` `grt004.go:80,87`; mutation "fires via resolveDependency (ignores enabled/skip_outputs)" killed by `TestRemovedOutputsDIAG03Parity`, `TestRemovedOutputsMatrix`; dropping `fires` killed by `TestRemovedOutputsChecksAreNecessary` |
| T-13-01-3 | mitigate | CLOSED | independent facts `grt004.go:83-91` (`baseDep` never gated on `hadRef`); dropping `baseHadRef` / `sameModule` / `baseDeclared` each killed by `TestRemovedOutputsChecksAreNecessary` + `TestRemovedOutputsMatrix`; `sameModule` gated on `hadRef` killed; unknown module either side → `blast_grt004.txtar` cases 5a/5b |
| T-13-01-4 | mitigate | CLOSED | `siteKey{unit, pos}` `grt004.go:128-131`, code filters `grt004.go:147,160`, no message read; key without pos killed (`TestSupersedeUnknownOutputs`, `TestSupersedeNeverAddsProperty`, `TestBetweenOnlyReclassifiesGRT001`); key without file killed by row "same line:col in another file of the unit" `grt004_test.go:198` |
| T-13-01-5 | accept | CLOSED | one map over base refs, linear in cur refs `grt004.go:70-113` |
| T-13-01-6 | mitigate | CLOSED | message fields are `RepoPath` (`module.go:43`, `unit.go:476`; `NewRepoPath` rejects absolute and `..`, `path.go:26-38`) and `strconv.Quote` `grt004.go:101-104`; removing any of the four `strconv.Quote` calls killed by `TestRemovedOutputsMatrix`, `TestScripts`, `TestTextOutputsEscapeControls` |
| T-13-02-1 | mitigate | CLOSED | `SupersedeUnknownOutputs` drops unmatched GRT004 `grt004.go:168-170`, result never larger; "append unmatched" mutation killed by `TestSupersedeNeverAddsProperty`, `TestSupersedeUnknownOutputs`; `TestBetweenOnlyReclassifiesGRT001` `between_property_test.go:335`; GRT004 is `SeverityError` like GRT001, exit code unchanged |
| T-13-02-2 | mitigate | CLOSED | `Between` `blasting.go:77-87` sole composition point, `Blast` delegates `blasting.go:60`; bypass mutation killed by `TestBetweenEqualsBlast`, `TestBlastBrokenAndImpacted`, `TestNoGRT004OutsideBlast`, `TestScripts` |
| T-13-02-3 | mitigate | CLOSED | `escapeTerm(d.Message())` `blast.go:106`, `escapeJSON` `blast.go:195`; removal killed by `TestBlastTextGRT004` / `TestBlastJSONGRT004` (`blast_test.go:327,337`) and e2e `TestTextOutputsEscapeControls` (`escape_test.go:25-35`, RLO in producer name) |
| T-13-02-4 | mitigate | CLOSED | messages from `RepoPath` only (see T-13-01-6); `blast_grt004.txtar:16-21` exact message, `! stdout '^/'`, `! stdout '[A-Za-z]:\\'`; `--base` label only on escaped `baseline:` line `blast.go:82` (F3) |
| T-13-02-5 | mitigate | CLOSED | `blast_grt004.txtar:62-79` cases 5a (module unknown in base → GRT001) and 5b (unknown in cur → silent) |
| T-13-02-6 | accept | CLOSED | `Between` linear in references; property generators bounded |
| T-13-02-7 | accept | CLOSED | contract in `Between` doc `blasting.go:74-76`; `checking.Check` analyzers are one-graph only `check.go:47-51`; re-verify in Phase 16 (daemon baseline) |
| T-13-03-1 | mitigate | CLOSED | analyzer signature `func(*repograph.RepositoryGraph)` `check.go:47-51` cannot take two graphs; `TestNoGRT004OutsideBlast` (`more07_test.go:42`, non-vacuous: blast prints 2×GRT004; check text/json/sarif, SARIF rule table, `graph --json`, live `report` per GOOS, status file); `TestCheckCannotProduceGRT004` over ≥20 trees; GRT001 emitting GRT004 code killed by both + goldens; `compare-ref.sh v0.3.0` 244/244 |
| T-13-03-2 | mitigate | CLOSED | `TestRuleRegistryDoc` `rules_doc_test.go:48`; removing the docs sentence or the usage mirror (`main.go:68-70`) killed; README paragraph reflow survives (F1) |
| T-13-03-3 | mitigate | CLOSED | go.mod/go.sum unchanged 6276296..21e419c; `go mod verify` all modules verified |
| T-13-03-4 | mitigate | CLOSED | `check-architecture.sh` OK incl. Step 9 (`check-architecture.sh:461`), self-tests pass; `go list -deps` on linux/darwin/windows × amd64/arm64: no net, os/exec, plugin |
| T-13-03-5 | mitigate | CLOSED | `trap` `compare-ref.sh:32` removes worktree and `$T`; verified after normal exit, `SIGINT` and `SIGTERM` mid-build: no `/tmp/tmp.*` left, `git worktree list` only main, `git status` clean; working tree never written (only transient `.git/worktrees` metadata) |
| T-13-03-6 | mitigate | CLOSED | `export GOFLAGS=-mod=readonly GOPROXY=off` `compare-ref.sh:34` (also blocks GOTOOLCHAIN auto-download) |
| T-13-03-7 | mitigate | CLOSED | `docs/cli.md:510-512` "New code values can appear without a version bump … Gate on severity, not on a code list."; removal killed by `TestRuleRegistryDoc` |

## Findings

| # | Severity | Ref | Finding | Disposition |
|---|----------|-----|---------|-------------|
| F1 | INFO | `cmd/gruntled/rules_doc_test.go:93` | README guard is line-scoped: a paragraph naming GRT004 with "no `Code` constant" on a later line passes (mutation M15 survived). README is correct today and the GRT004 table row is still asserted. | Fixed in ddfbf2d: paragraph-scoped check, M15 now fails (`rules_doc_test.go:93-100`) |
| F2 | INFO | `scripts/compare-ref.sh:56` | `extract()` interpolates txtar member names into `system("mkdir -p ...")`, accepts absolute/`..` names and symlink targets; `<ref>` goes to `git worktree add` without `rev-parse --verify --end-of-options`. Inputs are the working tree's own testdata and a maintainer-typed ref; not run in CI. | Fixed in f4b1e1f: ref verified with `rev-parse --verify --end-of-options`, `safe_rel` and `inside_after_link` reject unsafe names and links, no shell built from names; 244/244 unchanged |
| F3 | INFO | `cmd/gruntled/testdata/script/blast_grt004.txtar:20` | T-13-02-4 assertion is `! stdout '^/'` + drive-letter (text, case 1) instead of the planned `\$WORK\|^/`. Mitigation holds structurally (RepoPath) and by exact-message asserts. | No action |

## Re-checks requested by the orchestrator

- GRT004 never adds a finding and never appears outside blast: `SupersedeUnknownOutputs` only replaces a matched GRT001 (same code, Unit, Pos); `CodeRemovedOutput` is referenced in production only by `grt004.go:108,160`, called only from `Between`; check's analyzers are one-graph.
- Supersede key is structural and includes the file: `siteKey{unit, pos}`, `Position{file, line, column}` (`position.go:11-15`); both key mutations killed.
- Escaping: `escapeTerm`/`escapeJSON` on the GRT004 message, `strconv.Quote` kept on all four fields (each removal killed).
- Determinism: GRT004 emitted in `cur.References()` order, maps used only for lookup, `diagnostic.NewSet` sorts canonically; `TestRemovedOutputsDeterministic`, `-count=3 -shuffle=on` green.
- `compare-ref.sh`: `set -euo pipefail`, worktree outside the repo under `mktemp -d`, offline env, cleanup verified on three exit paths.
