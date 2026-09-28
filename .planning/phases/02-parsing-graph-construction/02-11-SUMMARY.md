---
phase: 02-parsing-graph-construction
plan: 11
subsystem: terragrunt-loader
tags: [go, terragrunt, hcl, gap-closure, stack-overflow, input-limits, fuzzing]

requires:
  - phase: 02-parsing-graph-construction
    provides: "hclconv limits API (02-10), loader guards and include-target bookkeeping (02-07), lazy-evaluation refs guard (02-06)"
provides:
  - "ReasonConfigTooLarge / ReasonConfigTooDeep: unit and include files over hclconv limits are never parsed"
  - "parsedFile.limitReason, checked after readErr and before syntax in resolveUnit step 2 and resolveIncludes"
  - "Two-input FuzzLoadUnits (body, shared) reaching root.hcl includes and the dependency target's module surface"
  - "Edge-case catalogue rows for every gap-closure reason; STACK-09 recorded as a documented false negative"
affects: [03-grt001-diagnostic-cli]

tech-stack:
  added: []
  patterns:
    - "Guarded parse in fileCache: hclconv.ReadFileLimited, then CheckNativeDepth, then hclsyntax.ParseConfig (same order as tfsurface)"
    - "Precedence per file: unreadable > too-large > too-deep > syntax-error"

key-files:
  created:
    - internal/infrastructure/terragrunt/limits_test.go
  modified:
    - internal/infrastructure/terragrunt/parse.go
    - internal/infrastructure/terragrunt/loader.go
    - internal/infrastructure/terragrunt/reasons.go
    - internal/infrastructure/terragrunt/parse_test.go
    - internal/infrastructure/terragrunt/loader_test.go
    - internal/infrastructure/terragrunt/fuzz_test.go
    - .planning/phases/02-parsing-graph-construction/02-TERRAGRUNT-EDGECASES.md

key-decisions:
  - "A refused file sets limitReason only: no readErr, no syntax diagnostic, no facts. No GRT100, because the file is not known to be invalid"
  - "Only CheckNativeDepth runs in the terragrunt fileCache: .json includes are refused (include-json-unsupported) before cache.get, so no JSON ever reaches it"
  - "A limited include file is still recorded as located (include-target), since the located bookkeeping runs before the limit check"

patterns-established:
  - "Every file the loader or the surface reader parses goes through the hclconv size and depth guard first"

requirements-completed: [PARSE-03, PARSE-04, PARSE-05]

duration: 8min
completed: 2026-09-28
---

# Phase 2 Plan 11: Unit/include file limits, two-input fuzz and catalogue catch-up Summary

**Oversize (>4 MiB) or overdeep (>1000 levels) terragrunt.hcl and include files now make units config-unknown `config-too-large`/`config-too-deep` instead of killing the process. FuzzLoadUnits also fuzzes a shared root.hcl and the dependency's main.tf, and the edge-case catalogue matches the gap-closure code.**

## Performance

- **Duration:** ~8 min
- **Started:** 2026-09-28T08:24:35Z
- **Completed:** 2026-09-28T08:32:30Z
- **Tasks:** 3/3
- **Files modified:** 8 (1 created)

## Accomplishments

- `fileCache.parse` reads through `hclconv.ReadFileLimited` and pre-scans with `hclconv.CheckNativeDepth`. A refused file gets `parsedFile.limitReason` and hclsyntax never sees it. Parse-once still holds (`TestLoaderParseOnceIncludes`, `TestParseOnce`, `TestFileCacheParseOnce` green; the shared deep root.hcl is read exactly once).
- `resolveUnit` step 2 and `resolveIncludes` map `limitReason` to config-unknown, after the `readErr` check and before the syntax check. 02-07's `located` bookkeeping and the `.json` refusal are unchanged and still run first.
- `FuzzLoadUnits(body, shared string)`: `shared` is written to `root.hcl` and `live/vpc/main.tf`. All 19 existing seeds were kept with the old root.hcl content, and 7 new pairs were added. A fifth invariant checks that reference positions inside root.hcl fall within `shared`'s bytes.
- Catalogue: STACK-09 is now `Unknown (include-target; documented false negative)` with a G3 revision paragraph, and STACK-06 has a G2 revision paragraph. New rows and sections: SRC-14 (invalid-generate), SRC-15 (module-file-unreadable), INC-13 (include-json-unsupported), DEP-14 (config-path-nondefault-file), DEP-15 (config-path-invalid), STACK-11 (config-too-large/deep) and STACK-12 (module-file-too-large/deep). A "Gap closure (02-06..02-11)" list maps G1..G14 to their plans.

## Task Commits

1. **Task 1: Size and depth limits for unit and include files (G7b)**
   - RED `362bf8e` test(02-11): add failing unit and include size/depth limit cases
   - GREEN `171044a` fix(02-11): refuse oversize and overdeep unit and include files before parsing
2. **Task 2: Two-input FuzzLoadUnits (G10)**: `2405133` test(02-11): fuzz shared root.hcl and dependency module file
3. **Task 3: Edge-case catalogue catches up with gap closure**: `9f632fc` docs(02-11): catalogue gap-closure reasons and STACK-09 false negative

## Observed RED crash

Before the fix, `TestDeepNestingNoCrash` (200k parens in `live/app/terragrunt.hcl`) killed the test binary with `fatal error: stack overflow`. The recursion was in `hclsyntax.(*parser).parseExpressionTerm` → `parseExpressionWithTraversals` → `parseBinaryOps`. `TestFileCacheLimits` and the four new `TestUnknownReasons` rows failed normally. They use inputs one level or one byte over the limit, so they don't crash. `TestDeepModuleFileEndToEnd` already passed thanks to 02-10's reader guard and serves as an end-to-end regression.

## Fuzz

- The new seeds reach real code. This was checked with a temporary `t.Logf` that has since been removed. With seed (a), live/app resolves with dependency vpc and vpc's module surface is known. With seed (b), live/app gets a second reference `db.y` at `root.hcl:4:16`, declared in the include. Seed (d) gives config-too-deep with no diagnostics, and seed (c) gives GRT100 for both shared files.
- `testdata/fuzz` did not exist before (`ls internal/infrastructure/terragrunt/testdata/fuzz` found nothing), so changing the arity broke no stored input.
- `go test -run '^$' -fuzz '^FuzzLoadUnits$' -fuzztime 60s -parallel 4`: **119,278 execs**, 92 interesting inputs, no crasher, and no testdata written.

## Corpus results

- Primary (`TestCorpusSmoke`): PASS and unchanged.
- Secret (`TestIncludeTargetSecretCorpus`): PASS.

## Memory

The terragrunt test binary peaks at ~165 MiB RSS (168996 KB, `/usr/bin/time -v`) with the new 200k-level tests. The hclconv binary is unchanged.

## Deviations from Plan

### Auto-fixed Issues

None for code. Process notes:

- **parsedFile.limitReason declared in the RED commit.** The field and the two reason constants went in with the failing tests, still unset, so the package compiled and `TestFileCacheLimits`/`TestUnknownReasons` could fail on assertions instead of a build error. GREEN only set and consumed the field.
- **Doc tweak:** `ReasonUnreadableConfig`'s comment now says a failure of the `fs.Stat` inside `ReadFileLimited` also counts, because that is now a possible readErr source.
- **Not pushed; no CI run id.** The plan's verification step asks for `git pull --rebase && git push` and a `gh run watch`. The orchestrator instructions for this worktree forbid pushing and merging. `-race` was not run locally because there is no gcc. CI still has to run on the merged master.

## Verification

In the worktree with `GOTOOLCHAIN=go1.27.0`, all of these passed: `gofmt -l .` (empty), `go mod tidy -diff`, `go build ./...`, `go vet ./...`, staticcheck v0.8.1, `go test -count=1 ./...`, govulncheck v1.8.0 ("No vulnerabilities found"), cross-builds for linux/darwin amd64+arm64 and windows/amd64, `scripts/check-architecture.sh` and `scripts/test-check-architecture.sh`.

## Next Phase Readiness

- No terragrunt.hcl, include or module file can crash the loader anymore. Phase 3's GRT001 can rely on every refused file being unknown with a named reason.
- STATE.md, ROADMAP.md and REQUIREMENTS.md are left to the orchestrator.

## Self-Check: PASSED

- Files exist: limits_test.go, parse.go, loader.go, reasons.go, parse_test.go, loader_test.go, fuzz_test.go, 02-TERRAGRUNT-EDGECASES.md
- Commits present on worktree-02-11: 362bf8e, 171044a, 2405133, 9f632fc
