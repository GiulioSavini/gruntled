---
phase: 03-grt001-diagnostic-cli
plan: 02
subsystem: interfaces-presenters, architecture-enforcement
tags: [go, presenter, json, text, bash, ci, architecture, hexagonal, cli-04]

# Dependency graph
requires:
  - phase: 02-parsing-graph-construction (02-09)
    provides: check-architecture.sh helpers (check_stdlib_allowlist, check_platform_neutral, check_external_deps) and the run_case/mkcopy self-test harness
  - phase: 01/02 domain
    provides: diagnostic.Set, diagnostic.Diagnostic, repograph.RepositoryGraph/Unit/Module/RepoPath
provides:
  - "presenter.Text(w, diags): `path:line:col: CODE message` per diagnostic, ` (unit U)` suffix, canonical Set order"
  - "presenter.JSON(w, g, diags): schema version 1 document (diagnostics, unknown_units, unknown_modules, summary), structs only, [] never null, no HTML escaping"
  - "presenter.Summary(w, g, diags): `gruntled: checked N units (K unknown): E errors, W warnings` or `gruntled: no Terragrunt units found`"
  - "rules interfaces-non-vacuous-guard, interfaces-stdlib-allowlist, interfaces-platform-neutral, interfaces-external-deps"
  - "rule binary-no-net-no-exec (Step 9): no net, net/*, os/exec, plugin, crypto/tls and no os.StartProcess/syscall.ForkExec|Exec|StartProcess in linked non-std files, for linux/amd64, linux/arm64, darwin/amd64, darwin/arm64, windows/amd64"
  - "compile gate now also vets ./internal/interfaces/..."
affects: [03-03, 03-05]

tech-stack:
  added: []
  patterns:
    - "Presenters are pure functions over domain values writing to io.Writer; output built in a bytes.Buffer then written once"
    - "Binary capability rule evaluated per release target via GOOS/GOARCH go list -e -deps, so build-constrained files cannot hide an import"

key-files:
  created:
    - internal/interfaces/presenter/doc.go
    - internal/interfaces/presenter/text.go
    - internal/interfaces/presenter/json.go
    - internal/interfaces/presenter/summary.go
    - internal/interfaces/presenter/presenter_test.go
  modified:
    - scripts/check-architecture.sh
    - scripts/test-check-architecture.sh

key-decisions:
  - "Text omits severity, per the locked CONTEXT format `path:line:col: CODE message` (03-RESEARCH Pattern 6 showed a severity-bearing variant; CONTEXT wins). JSON carries severity"
  - "interfaces allowlist is exactly domain allowlist + fmt, io, encoding/json; no extra package was needed (text.go uses strconv and bytes, already in the domain allowlist)"
  - "Summary and JSON share one tally() so their counts can never disagree"
  - "Text on an empty Set performs no Write at all (zero bytes, and no spurious writer error)"

requirements-completed: [CLI-01, CLI-03, CLI-04, DIAG-04]

duration: 45min
completed: 2026-09-28
---

# Phase 3 Plan 2: Presenters and CLI-04 / Interfaces Arch Rules Summary

**Pure text, JSON (schema v1) and summary presenters over domain types, plus four interfaces layering rules and a `binary-no-net-no-exec` rule that checks every release target, each proven by a self-test that asserts its exact rule name.**

## Performance

- Duration: about 45 min (the architecture self-test alone takes about 13 min on this WSL host)
- Tasks: 2/2
- Files: 5 created, 2 modified

## Accomplishments

- `internal/interfaces/presenter`: `Text`, `JSON`, `Summary` with the signatures 03-03 consumes. Byte-for-byte golden tests: empty Set, unit suffix, file-level GRT100, shared include (two units, same position, Set order), empty JSON document, mixed JSON (resolved, config-unknown `syntax-error`, module-unknown `remote-source`, unknown module `no-terraform-files`, `<>&` not escaped, GRT100 without `unit`), determinism, both summary lines, and writer-error propagation for every presenter.
- The package imports only bytes, encoding/json, fmt, io, strconv and internal/domain. No os, even in tests.
- `check-architecture.sh`: the Phase 3 header paragraph; interfaces guard in Step 0; interfaces added to the compile gate; Step 2c `interfaces-stdlib-allowlist`; `interfaces-platform-neutral` in Step 3; `interfaces-external-deps` in Step 4; Step 9 `binary-no-net-no-exec` looping over the five release targets, with its known blind spots written down (aliased or dot imports, x/sys spawners, cgo, asm, reflection; comments can over-match). The OK line now also reports the interfaces package count.
- `test-check-architecture.sh`: 10 new cases appended after the 36 existing ones, which are unchanged: interfaces-infra-dep (asserts both interfaces-external-deps and infrastructure-importers), interfaces-testsupport-in-test, interfaces-os, interfaces-windows-file, interfaces-vacuous, binary-os-exec, binary-net, binary-start-process, binary-exec-windows-file (proves the non-host targets are checked), binary-exec-in-test-allowed (expects zero failures).

## Task Commits

1. Task 1 (TDD) presenters
   - RED `25bf08a` test(03-02): add failing golden tests for text, JSON and summary presenters
   - GREEN `416bec4` feat(03-02): implement pure text, JSON and summary presenters
2. Task 2 arch rules + self-tests: `43f843a` feat(03-02): add interfaces layering rules and binary-no-net-no-exec over all release targets

## Deviations from Plan

None. The plan was executed as written. Two notes:
- The plan's `<verify>` blocks `cd` into the main checkout. As instructed, every command ran in the worktree `/home/giulio/gruntled/.claude/worktrees/03-02` instead.
- The arch script now takes about 25 s on this host, up from about 17 s, because of the five cross-target `go list` passes. The self-test takes about 13 min in total. Correctness is unaffected; on CI it is a cost only.

## Issues Encountered

None.

## Verification

With GOTOOLCHAIN=go1.27.0, all of these were green: `gofmt -l .` (empty), `go mod tidy -diff`, `go vet ./...`, staticcheck v0.8.1, `go test -count=1 ./...`, `bash scripts/check-architecture.sh` (`architecture: OK (2 domain packages, 2 application packages, 1 interfaces packages)`) and `bash scripts/test-check-architecture.sh` (46/46 PASS).

## Next Phase Readiness

03-03 can wire `presenter.Text/JSON/Summary` into `cmd/gruntled`. Once it does, `interfaces-vacuous` also trips compile-gate, but the guard's line is still printed first, and the case asserts only that line. Keep `release_targets` in Step 9 identical to the 03-03/03-05 cross-build list.

## Self-Check: PASSED
