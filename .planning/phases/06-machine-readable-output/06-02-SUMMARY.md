---
phase: 06-machine-readable-output
plan: 02
subsystem: presenter
tags: [sarif, int-02, presenter]
requires: []
provides:
  - "presenter.SARIF(w, g, diags, ToolInfo) error"
  - "presenter.ToolInfo{Version string}"
affects: [06-03, 06-05]
tech-stack:
  added: []
  patterns: ["struct-only SARIF DTOs, static rule table indexed by code, resolve-then-write single buffered write"]
key-files:
  created:
    - internal/interfaces/presenter/sarif.go
    - internal/interfaces/presenter/sarif_test.go
    - internal/interfaces/presenter/sarif_internal_test.go
  modified: []
decisions:
  - "Rule shortDescription = docs heading title (e.g. 'dependency output not declared'); helpUri anchor derived from full heading via githubAnchor"
  - "Unit notifications point at <unit>/terragrunt.hcl (root -> terragrunt.hcl) with no region; module notifications carry no locations key"
  - "Notification locations and result region use omitempty only where absence is the contract (module locations, notification region)"
metrics:
  duration: 10min
  completed: 2026-10-01
  tasks: 2
  files: 3
---

# Phase 6 Plan 02: SARIF Presenter Summary

Pure `presenter.SARIF` writing one deterministic SARIF 2.1.0 run with four rules (GRT001, GRT002, GRT003, GRT100), percent-encoded `%SRCROOT%` URIs, note notifications for unknown units/modules, and an error (no output) for diagnostic codes with no rule.

## Tasks

| Task | Name | Commits |
|------|------|---------|
| 1 | Rule table, percent-encoder, DTOs | 7a0404e (test), 5ba14d1 (feat) |
| 2 | SARIF writer | 36dcfd1 (test), fd9a680 (feat) |

## What was built

- `sarif.go`: struct-only DTOs (fixed key order), `sarifRuleTable` in code order with PascalCase names, `sarifRules()`, `ruleIndexOf`, `githubAnchor`, `percentEncode` (per-byte, uppercase hex, `/` kept), `SARIF`, `ToolInfo`.
- Results: ruleId, ruleIndex, level from severity (error/warning, else note), message with ` (unit X)` suffix, location with startLine/startColumn.
- No timestamps, GUIDs, originalUriBaseIds, partialFingerprints, relatedLocations, automationDetails, exitCode.
- Doc comment records known limitations: byte columns under `unicodeCodePoints`, URIs relative to analysed path (uploader needs `checkout_path`).

## Deviations from Plan

**1. [Rule 3 - Blocking] Internal tests in a separate file**
- **Found during:** Task 1
- **Issue:** `percentEncode`, `ruleIndexOf` and the rule table are unexported; `sarif_test.go` is `package presenter_test` (needs presenter_test.go helpers), so it cannot reach them.
- **Fix:** Task 1 unit tests live in `sarif_internal_test.go` (`package presenter`, imports only `testing`/`reflect`/`strings` + domain).
- **Commit:** 7a0404e

## Known limitations

- A unit configured by `terragrunt.hcl.json` gets a notification location of `<unit>/terragrunt.hcl`: the graph does not record the config filename. Notifications only; results use real diagnostic positions.

## Verification

- `go test ./... -count=1`: all green.
- `bash scripts/check-architecture.sh`: OK, no new imports outside the interfaces allowlist.

## Self-Check: PASSED
