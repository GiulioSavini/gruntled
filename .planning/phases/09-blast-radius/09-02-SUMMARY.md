---
phase: 09-blast-radius
plan: 02
subsystem: application, interfaces
tags: [blast-radius, use-case, presenter, json, text]
requires:
  - internal/domain/impact (Compute, NoBaseline, Result)
  - checking.Check
provides:
  - internal/application/blasting (Sources, Blast, Error{Stage current|baseline})
  - presenter.BlastText(w, res, baselineLabel), presenter.BlastJSON(w, res)
affects: [09-03]
tech-stack:
  added: []
  patterns: [staged custom error without fmt, struct-only JSON document with fixed key order]
key-files:
  created:
    - internal/application/blasting/blasting.go
    - internal/application/blasting/blasting_test.go
    - internal/interfaces/presenter/blast.go
    - internal/interfaces/presenter/blast_test.go
  modified: []
decisions:
  - Blast checks the current tree first, so a broken current tree is reported as Stage "current" even when the baseline is broken too
  - Change tokens are ordered -variable, +variable, -output, +output (the plan's stated ordering; the RESEARCH sample line was inconsistent)
  - Without a baseline both presenters drop Impacted even if the caller passed some (text omits the section; JSON writes [])
  - Findings under a Broken subject are printed without the "(unit U)" suffix used by Text, since the subject line already names it
requirements-completed: []  # BLAST-01 stays pending: use case and presenters done, CLI wiring lands in 09-03
metrics:
  duration: 4min
  completed: 2026-10-08
  tasks: 2
  files: 4
---

# Phase 9 Plan 02: Blast Use Case and Presenters Summary

`blasting.Blast` runs `checking.Check` on the current tree and, if given, a baseline tree, then returns `impact.Compute` (or `impact.NoBaseline` when base is nil), with failures wrapped as `*blasting.Error{Stage: "current"|"baseline"}`. `BlastText`/`BlastJSON` render the result deterministically.

## Tasks

| # | Task | Commit |
|---|------|--------|
| 1 | blasting use case | eb403d0 |
| 2 | BlastText and BlastJSON presenters | e164993 |

## Output shapes as built

Text:
```
baseline: ../base
Broken (N):
  <subject>
    file:line:col: CODE message
Impacted (M):
  <unit> (module <m>: -variable a, +variable b, -output c, +output d)
```
No baseline: first line `baseline: none (no baseline)`, no Impacted section. Empty sections print `Broken (0):` / `Impacted (0):`.

JSON: `{version:1, kind:"blast", baseline, [note:"no baseline" only when baseline false], broken:[{unit, findings:[{code,severity,file,line,column,message}]}], impacted:[{unit,module,added_variables,removed_variables,added_outputs,removed_outputs}], summary:{broken,impacted}}`. Every list is `[]` when empty, including nil slices from the domain. SetEscapeHTML(false), two-space indent like graph.go.

## Verification

- `go test ./internal/application/... ./internal/interfaces/... -count=1`: pass
- `go vet ./...`: clean
- `scripts/check-architecture.sh`: exit 0 (application tests use only context/errors/reflect/testing)
- `gofmt -l`: empty

## Deviations from Plan

**1. TDD commit granularity:** as in 09-01, tests and implementation were committed together per task, not as separate RED and GREEN commits.

Otherwise the plan ran as written.

## Self-Check: PASSED
