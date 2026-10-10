---
phase: 06-machine-readable-output
plan: 03
subsystem: cli
tags: [cli, graph, sarif, int-01, int-02, exit-codes]
requires:
  - "presenter.Graph (06-01)"
  - "presenter.SARIF, presenter.ToolInfo (06-02)"
provides:
  - "gruntled graph --json [path]"
  - "gruntled check --format text|json|sarif"
  - "cmd/gruntled parseArgs/openRepo/writeOut helpers shared by check and graph"
affects: [06-04, 06-05]
tech-stack:
  added: []
  patterns: ["shared parse-again flag loop with per-command define callback", "buffered single stdout write via writeOut"]
key-files:
  created:
    - cmd/gruntled/testdata/script/graph_exitcodes.txtar
  modified:
    - cmd/gruntled/main.go
    - cmd/gruntled/e2e_test.go
    - cmd/gruntled/main_test.go
    - cmd/gruntled/testdata/script/usage.txtar
    - cmd/gruntled/testdata/script/exitcodes.txtar
    - docs/cli.md
    - README.md
decisions:
  - "graph -h exit-code block has 3 lines (0/2/3); exit 3 line also names failed stdout write"
  - "graph builds via indexing.Build directly; Build diagnostics (e.g. GRT100) are not printed by graph"
  - "docs/cli.md help blocks generated from the real binary output, so they match byte-for-byte"
metrics:
  duration: 9min
  completed: 2026-10-01
  tasks: 2
  files: 8
---

# Phase 6 Plan 03: CLI wiring for graph --json and check --format sarif Summary

`gruntled graph --json [path]` prints the INT-01 graph document (exit 0/2/3 only, silent stderr on success) and `gruntled check --format sarif` prints the INT-02 log with exit codes identical to text/json; both share argument parsing, repo opening and stdout writing extracted from runCheck.

## Tasks

| Task | Name | Commit |
|------|------|--------|
| 1 | Refactor runCheck, add runGraph and sarif format | 417a324 |
| 2 | Tests, usage.txtar, graph_exitcodes.txtar, docs, README | ea3ccce |

## Details

- main.go: `parseArgs(name, usage, args, stderr, define) (dir, code, done)`, `openRepo(dir, stderr) (*os.Root, bool)`, `writeOut(stdout, stderr, *bytes.Buffer) bool`. Text/json behaviour unchanged except the invalid-format message `(want text, json or sarif)`.
- runGraph: `--json` bool; missing => `gruntled: graph: --json is required; text output is reserved` + usage, exit 2; `indexing.Build(ctx, terragrunt.NewLoader, tfsurface.NewReader)`; `presenter.Graph` into buffer.
- sarif branch: `presenter.SARIF(&buf, rep.Graph, rep.Diagnostics, presenter.ToolInfo{Version: "dev"})`, no stderr summary, exit 1 on errors, nothing written + exit 3 if SARIF errors.
- Tests: TestDeterministicAcrossCheckouts now table-driven over text/json/sarif (exit 1) and graph (exit 0); TestNoWrites adds sarif and graph; TestRunStdoutWriteFailure adds sarif and graph cases; TestHelpMatchesDocs checks check (4 lines) and graph (3 lines) against docs/cli.md.
- graph_exitcodes.txtar: config-unknown (syntax error) unit exit 0 with empty stderr, flags after path / `--` / `-json=true` byte-identical, finding repo exit 0, missing --json, two paths, unknown flag (2), missing path (3); sarif 0/1/2/3.

## Deviations from Plan

**1. [Rule 2 - Correctness] README "Not built yet" list**
- **Found during:** Task 2
- **Issue:** README listed `gruntled graph --json` and SARIF output as not built.
- **Fix:** Removed both items from the list (besides the usage-line edits the plan asked for).
- **Files modified:** README.md
- **Commit:** ea3ccce

**2. TDD ordering for Task 2**
- Task 2 is test/docs for code delivered in Task 1, so the new tests passed on first run; committed as one `test` commit, no separate RED/GREEN pair.

## Verification

- `go test -count=1 ./...`: all green.
- `go test -race`: not runnable here (no gcc for cgo); non-race suite used instead.
- `bash scripts/check-architecture.sh`: OK.

## Self-Check: PASSED
