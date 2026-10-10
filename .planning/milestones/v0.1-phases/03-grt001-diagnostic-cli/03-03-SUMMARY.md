---
phase: 03-grt001-diagnostic-cli
plan: 03
subsystem: cli, composition-root, e2e-tests
tags: [go, cli, stdlib-flag, exit-codes, testscript, diag-03, grt001, grt100]
requires:
  - phase: 03-grt001-diagnostic-cli (03-01)
    provides: "checking.Check, analysis.UnknownOutputs, strategy-over-bool MockMergeWithState"
  - phase: 03-grt001-diagnostic-cli (03-02)
    provides: "presenter.Text/JSON/Summary, binary-no-net-no-exec rule"
  - phase: 03-grt001-diagnostic-cli (03-04)
    provides: "docs/cli.md exit-code lines the checkUsage text must match"
provides:
  - "gruntled check [--format text|json] [path]: os.OpenRoot -> terragrunt.NewLoader + tfsurface.NewReader -> checking.Check -> presenters"
  - "run(args, stdout, stderr) int with exit codes 0/1/2/3; flag re-parse loop so flags work after the path"
  - "testscript harness with gruntled and gruntled-exit (exact exit-code assertion) programs"
  - "11 txtar scripts: usage, exitcodes, flags_after_path, text/json goldens, empty, grt100, four DIAG-03 scripts"
affects: [03-05, 04-real-repo-validation-experiment, README]
tech-stack:
  added: ["github.com/rogpeppe/go-internal v1.16.0 (test-only, testscript)"]
  patterns:
    - "Composition root renders into a bytes.Buffer, then one stdout.Write; summary to stderr in text mode only"
    - "gruntled-exit <code> helper program lets every script assert the exact numeric exit code"
key-files:
  created:
    - cmd/gruntled/main_test.go
    - cmd/gruntled/testdata/script/usage.txtar
    - cmd/gruntled/testdata/script/exitcodes.txtar
    - cmd/gruntled/testdata/script/flags_after_path.txtar
    - cmd/gruntled/testdata/script/text_golden.txtar
    - cmd/gruntled/testdata/script/json_golden.txtar
    - cmd/gruntled/testdata/script/empty.txtar
    - cmd/gruntled/testdata/script/grt100.txtar
    - cmd/gruntled/testdata/script/diag03_corpus_shape.txtar
    - cmd/gruntled/testdata/script/diag03_issue2163.txtar
    - cmd/gruntled/testdata/script/diag03_no_mocks.txtar
    - cmd/gruntled/testdata/script/diag03_silent_rows.txtar
  modified:
    - cmd/gruntled/main.go
    - go.mod
    - go.sum
    - README.md
key-decisions:
  - "An empty text render performs no stdout Write at all, so a clean run never touches stdout"
  - "Usage errors print a specific one-line reason (more than one path / invalid --format) followed by the full checkUsage text"
  - "Row 5c (config-unknown target via dynamic include) uses a dynamic prefix with a fixed file name (\"${local.dir}/common.hcl\"), so the referencing unit is not itself marked include-target and the silence really comes from the target"
requirements-completed: [CLI-01, CLI-02, DIAG-01, DIAG-02, DIAG-03, DIAG-04]
duration: 40min
completed: 2026-09-29
---

# Phase 3 Plan 03: gruntled check composition root and end-to-end scripts Summary

**`gruntled check` wired on stdlib `flag` with exit codes 0/1/2/3 and flags after the path, proven end to end by 11 testscript scripts (exact exit code per row), byte-exact text/JSON goldens, GRT100 for unit/include/module files, and every DIAG-03 row including the corpus mock shape.**

## Tasks

1. **Composition root, flags, exit codes, harness** (TDD)
   - RED `bcbc018`: go-internal v1.16.0 added (test-only; tidy adds only go-internal and its go.sum lines), main_test.go with `TestMain`/`testscript.Main`, `gruntled-exit`, `TestScripts`, `TestRunExitCodes` (16 rows), `TestRunStdoutWriteFailure` (text with a finding and JSON on a clean repo; asserts the failing writer was called). Scripts usage, exitcodes, flags_after_path. Build failed on undefined `run`.
   - GREEN `6aee8ef`: main.go with the exact topUsage/checkUsage texts, re-parse loop (research Pattern 8), `os.OpenRoot` + `root.FS()`, buffer + single write, summary to stderr in text mode, `HasErrors()` -> 1.
2. **End-to-end diagnostic scripts** `5047b2f`: text_golden, json_golden, empty, grt100, diag03_corpus_shape (with the `no_merge` precedence block), diag03_issue2163, diag03_no_mocks, diag03_silent_rows (rows 1, 2a, 2b, 3a, 3b, 4, 5a, 5b, 5c).
3. **README** `1f568ac` (orchestrator instruction): Status and usage now describe the working CLI; the "Planned interface (Phase 3)" section became "Usage" with the real line format and exit-code table; diagnostic-codes paragraph and architecture diagram updated.

## Golden verification (hand-counted)

- `app/terragrunt.hcl:4:17`: `inputs = { id = ` is 16 bytes.
- `root.hcl:4:18`: `inputs = { net = ` is 17 bytes; reported twice (units api, web) in unit order.
- Corpus shape `7:17`: line 7 is `inputs`, after the 5-line block and its `}`; `no_merge` variant is `8:17` (one extra attribute line); issue2163 is `6:17`.
- Summary `checked 5 units (1 unknown): 3 errors` matches units vpc, app, dns, web, api with dns remote-source.
- Mutation check: changing 7:17 to 7:18 makes diag03_corpus_shape fail, so cmp is live.
- docs/cli.md JSON example reproduced byte-for-byte by the binary (with `app/main.tf` present); every `check -h` line appears verbatim in docs/cli.md.
- No script contains the old `: error GRT` / `[unit ` format.

## Corpus runs (built binary)

- Primary corpus: exit 0, 0 lines on stdout, stderr `gruntled: checked 65 units (3 unknown): 0 errors, 0 warnings`.
- Temp copy with `output "role_name"` renamed in `iac.src/s3_runtime/state.tf`: exit 1, exactly 8 GRT001 lines (all with the mock-masking suffix), stderr `8 errors`, no temp path on stdout.

## Deviations from Plan

- **[Rule 1 - fixture] Row 5c fixture changed.** `find_in_parent_folders(local.common)` has a dynamic file name, so the include-target guard (rule 3) also marked the include-free referencing unit `app` as config-unknown; the row would have been silent for the wrong reason. Switched to `"${local.dir}/common.hcl"` and the script now asserts via JSON that only `vpc` is unknown (`include-dynamic-path`) and `app` is not. No production code changed.
- README edit done here (per orchestrator) although 03-05 also lists README; 03-05 only needs to re-check it.

## Verification

GOTOOLCHAIN=go1.27.0: `gofmt -l .` empty, `go mod tidy -diff`, `go vet ./...`, staticcheck v0.8.1, govulncheck v1.8.0 (no vulnerabilities), `go test -count=1 ./...`, `check-architecture.sh` OK, env-gated corpus tests (TestCorpusSmoke, TestDependencyOptionsPrimaryCorpusShape, TestIncludeTargetSecretCorpus, TestIncludeTargetCorpusReproduction) pass, five cross-builds succeed, `test-check-architecture.sh` passes (56/56).

## Self-Check: PASSED

- FOUND: bcbc018, 6aee8ef, 5047b2f, 1f568ac
- FOUND: cmd/gruntled/main.go, main_test.go, 11 txtar scripts
