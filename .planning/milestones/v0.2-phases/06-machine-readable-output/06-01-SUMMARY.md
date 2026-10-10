---
phase: 06-machine-readable-output
plan: 01
subsystem: presenter
tags: [json, graph, int-01, domain-edge]
requires: []
provides:
  - "repograph.Edge.Name/TargetState/SkipOutputs"
  - "presenter.Graph(w, g) error"
affects: [06-03]
tech-stack:
  added: []
  patterns: ["struct-only JSON DTOs, buffered single write, [] never null"]
key-files:
  created:
    - internal/interfaces/presenter/graph.go
    - internal/interfaces/presenter/graph_test.go
  modified:
    - internal/domain/repograph/graph.go
    - internal/domain/repograph/edges_test.go
decisions:
  - "Paths edge: Name empty, SkipOutputs TristateFalse, TargetState from the paths entry"
  - "Graph summary = tally over an empty diagnostic set, no errors/warnings fields"
  - "Unresolved block deps use PathPos (as Edge.Pos); merged with unresolved paths entries per unit by Position.Compare (stable)"
metrics:
  duration: 6min
  completed: 2026-10-01
  tasks: 2
  files: 4
---

# Phase 6 Plan 01: Graph Edge Extension and Graph Presenter Summary

`repograph.Edge` now carries block label, observed target state and skip_outputs; `presenter.Graph` writes the INT-01 document (version, kind, units, modules, edges, unresolved_dependencies, summary) deterministically, edges taken straight from `RepositoryGraph.Edges()`.

## Tasks

| Task | Name | Commits |
|------|------|---------|
| 1 | Extend domain Edge with name, target state, skip_outputs | 0119d21 (test), 75fa510 (feat) |
| 2 | presenter.Graph document | 8c79f35 (test), 15a3d6b (feat) |

## Details

- Edge: new unexported fields `name`, `state`, `skipOutputs`; accessors documented with paths-edge conventions. Sort order untouched.
- edges_test.go: `edgeMetas` asserts name/state/skip for block edges (incl. a `skip_outputs` unknown dep) and a paths edge with `TargetNoConfig`.
- graph.go: DTOs `graphDoc`, `graphPos` (always full), `graphUnit`, `graphReference`, `graphModule`, `graphEdge`, `graphUnresolvedDep`, `graphSummary`; omitempty only on unit module/reason, module reason, edge name, unresolved name.
- graph_test.go: empty golden, mixed golden (block edge, paths edge, unresolved block dep, unresolved paths entry, module-unknown, config-unknown, known + unknown module), no-null, no HTML escaping, determinism, writer error.

## Deviations from Plan

None - plan executed exactly as written. (Presenter test imports `errors`/`strings` like the existing presenter_test.go; allowed by the test allowlist.)

## Verification

- `go test ./... -count=1` green
- `bash scripts/check-architecture.sh` OK

## Self-Check: PASSED
