---
phase: 06-machine-readable-output
plan: 04
subsystem: cli
tags: [golden, sarif, graph, docs, int-01, int-02]
requires:
  - "gruntled graph --json (06-03)"
  - "gruntled check --format sarif (06-03)"
provides:
  - "graph_golden.txtar / sarif_golden.txtar byte pins"
  - "TestSARIFStructure, TestSARIFStructureClean, TestSARIFDoc"
  - "docs/cli.md ### Graph JSON and ### SARIF sections"
affects: [06-05]
tech-stack:
  added: []
  patterns: ["generic-map decoding of real CLI output for ingestion-constraint tests", "doc-sync test: docs headings == emitted rule table"]
key-files:
  created:
    - cmd/gruntled/testdata/script/graph_golden.txtar
    - cmd/gruntled/testdata/script/sarif_golden.txtar
    - cmd/gruntled/sarif_test.go
  modified:
    - docs/cli.md
decisions:
  - "Doc-sync test lives in sarif_test.go; validation_doc_test.go untouched (its pin helper not reused)"
  - "TestSARIFDoc also requires a SARIF-section rule row '| `GRTnnn` | `Name` |' per rule and a ### Graph JSON heading"
  - "SARIF golden pins percent-encoding twice: '%20' (result URI) and UTF-8 '%C3%A0' (notification URI)"
metrics:
  duration: 10min
  completed: 2026-10-01
  tasks: 2
  files: 4
---

# Phase 6 Plan 04: Goldens, SARIF Structure Test and Output Docs Summary

Graph JSON and SARIF output are byte-pinned by two testscripts, GitHub ingestion constraints are asserted on real SARIF output, and docs/cli.md documents both contracts with a test keeping rule titles and names in sync.

## Tasks

| Task | Name | Commits |
|------|------|---------|
| 1 | Golden testscripts for graph and SARIF | 57d4ecf |
| 2 | Structure test, doc sections, doc-sync test | 69a98de (test, RED), 99e733d (docs, GREEN) |

## Details

- graph_golden.txtar: block edge with `skip_outputs = true`, paths edge (no `name`, `skip_outputs` "false"), unresolved block dep (`config_path = local.dyn`, `config-path-dynamic`), module-unknown unit (remote source), config-unknown unit (syntax error), unknown module (missing local source, unit stays `resolved`), 2-unit cycle. Exit 0, empty stderr. Positions hand-checked.
- sarif_golden.txtar: GRT001 under `my app/` (`my%20app/terragrunt.hcl`), GRT002, GRT003 ring, module-unknown unit `città` (`citt%C3%A0/terragrunt.hcl` notification), module notification with no location. Exit 1, empty stderr. No absolute paths in either golden.
- sarif_test.go: TestSARIFStructure (fixture hits all four rules incl. GRT100): version, `$schema`, one run, ruleIndex indexes ruleId, levels, relative `%SRCROOT%` URIs (no leading `/`, no scheme/drive, no raw space or backslash), startLine/startColumn >= 1, help/shortDescription text, arrays never null, no originalUriBaseIds/partialFingerprints/relatedLocations, notifications at `note`. TestSARIFStructureClean: empty `results`/notifications are arrays. TestSARIFDoc: `### GRTnnn: title (severity)` headings sorted by code == emitted (id, shortDescription.text); pins the four PascalCase names; SARIF section has a row per rule.
- docs/cli.md: `### Graph JSON` (top-level + per-object field tables, tristates, target_state values, unresolved rules, stability rule, deferred mock_* fields) and `### SARIF` (rule table, level mapping, URI/checkout_path policy, byte columns, no fingerprints, no relatedLocations, notifications, exit 3 prints nothing). Three Known-limitations bullets added. Help-sensitive lines untouched.

## Deviations from Plan

None - plan executed as written. TestSARIFStructureClean added as a small extra check for the never-null rule on an empty run.

## Verification

- `go test -count=1 ./...`: all green (`-race` not runnable: no gcc).
- `bash scripts/check-architecture.sh`: OK.

## Self-Check: PASSED
