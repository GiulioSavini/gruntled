---
phase: 09-blast-radius
plan: 03
subsystem: cli, docs
tags: [blast-radius, cli, testscript, docs]
requires:
  - internal/application/blasting (Blast, Sources)
  - presenter.BlastText, presenter.BlastJSON
provides:
  - "gruntled blast [--base dir] [--format text|json] [path]"
  - docs/cli.md blast usage, Blast text, Blast JSON, exit codes, Known limitations
affects: []
tech-stack:
  added: []
  patterns: [one os.Root per tree with fresh loader and reader, exit code from Result.HasErrors like check]
key-files:
  created:
    - cmd/gruntled/testdata/script/blast_nobase.txtar
    - cmd/gruntled/testdata/script/blast_exitcodes.txtar
    - cmd/gruntled/testdata/script/blast_impacted.txtar
  modified:
    - cmd/gruntled/main.go
    - cmd/gruntled/testdata/script/usage.txtar
    - cmd/gruntled/e2e_test.go
    - docs/cli.md
    - .planning/phases/09-blast-radius/09-VALIDATION.md
decisions:
  - An unopenable --base exits 3 with "cannot open repository"; never a fallback to no baseline
  - '--base "" is treated as no --base (flag default is empty)'
  - blast writes no stderr summary; stdout only
  - Exit 1 only when Broken holds an error diagnostic (HasErrors); a warnings-only Broken set exits 0, documented in docs/cli.md
  - e2e_test.go {"blast", 4} committed with the docs (Task 2) so every commit stays green
requirements-completed: [BLAST-01, BLAST-02]
metrics:
  duration: 12min
  completed: 2026-10-08
  tasks: 2
  files: 8
---

# Phase 9 Plan 03: Blast CLI, Scripts and Docs Summary

`gruntled blast [--base dir] [--format text|json] [path]` opens path and base as separate `os.Root`s, runs `blasting.Blast`, renders `BlastText`/`BlastJSON`, and exits 0/1/2/3; three testscripts prove line-shift invariance, one-hop Impacted, disjointness and the no-baseline mode end to end.

## Tasks

| # | Task | Commit |
|---|------|--------|
| 1 | runBlast, usage text, scripts | d332f2f |
| 2 | docs/cli.md, validation map, phase gate | 54a4e9f |

## Coverage by script

- `blast_nobase`: no --base => `baseline: none (no baseline)`, no Impacted section; JSON `baseline:false`, `note`, `impacted: []`; clean tree exit 0.
- `blast_exitcodes`: same tree => 0; new GRT001 => 1; finding shifted by a blank line => 0; fixed => 0; flags after path / `--` / `=` forms byte-identical; exit 2 for sarif, xml, unknown flag, two paths, `--base` without value; exit 3 for missing path and missing base.
- `blast_impacted`: removing output `id` + adding variable `name` in modules/vpc => live/app Broken only (GRT001), live/cache and live/db Impacted with `+variable name, -output id`, live/other (other module) and live/edge (depends on live/db) absent; GRT100 present in both trees not Broken; comment/reorder edit => `Impacted (0):`; new + deleted module => nothing Impacted; JSON order, disjointness and summary asserted by regexp.

## Verification

Phase gate, run once: `go vet ./...` clean, `go test -count=1 ./...` all pass (incl. TestHelpMatchesDocs, TestSARIFDoc headings), `scripts/check-architecture.sh` OK, `gofmt -l .` empty, `go mod tidy -diff` clean. No go.mod changes.

## Deviations from Plan

**1. e2e_test.go moved to the Task 2 commit.** The plan lists `{"blast", 4}` under Task 1, but TestHelpMatchesDocs fails until docs/cli.md carries the blast exit lines; committing it with the docs keeps both commits green.

**2. Extra "Blast text" docs section** next to Blast JSON, holding the text layout sample the plan asked for.

Otherwise the plan ran as written.

## Self-Check: PASSED
