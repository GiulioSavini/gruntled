---
phase: 12-gap-closure-watcher-directory-ignore-and-v0-3-audit-findings
plan: 02
subsystem: terragrunt-loader
tags: [parse-cache, symlink, incremental, daemon-parity, security]
requires:
  - phase: 08
    provides: persistent parseStore and Loader.Invalidate
  - phase: 02
    provides: canonicalPath for includes (02-REVIEW G15)
provides:
  - canonical path recorded on every parse cache entry
  - Invalidate evicting by key or canonical path
  - per-load canonical re-check on cache hits (fileCache.getCanon)
  - TestAliasCache* real-FS and MapFS regressions
affects: [12-05]
tech-stack:
  added: []
  patterns: [cache entry carries the canonical path it was read from; hit only when canon matches]
key-files:
  created:
    - internal/infrastructure/terragrunt/alias_cache_test.go
  modified:
    - internal/infrastructure/terragrunt/parse.go
    - internal/infrastructure/terragrunt/loader.go
key-decisions:
  - "Canonical path stored as a field on parsedFile (canon), set before the entry is stored; one map, so prune and the clear-all path drop it with the entry"
  - "fileCache.get(p) kept for paths canonical by construction (unit files, canon = key); the include site calls getCanon(file, canon), so parse_test.go and cache_test.go stay untouched"
  - "Invalidate runs the ancestor walk on the canonical path only when it differs from the key"
requirements-completed: []
duration: 8min
completed: 2026-10-10
---

# Phase 12 Plan 02: Symlink Alias Parse Cache Summary

**Each parse cache entry now records the canonical path it was read from. Invalidate evicts by key or canonical path, and the include lookup reuses an entry only when the canonical path recomputed in this load matches. Editing a symlink target or retargeting a link in a chain no longer leaves the daemon serving the old parse (sec MEDIUM, bus #7).**

## Performance

- Duration: ~8 min
- Tasks: 2
- Files: 1 created, 2 modified

## Accomplishments

- `parsedFile.canon` (parse.go:132) holds the symlink-free in-repo path of the bytes. `parsedFile.path` is still the lexical path, so diagnostics and outputs do not change.
- `fileCache.getCanon(p, canon)` (parse.go:220): a hit requires `pf.canon == canon` (parse.go:223). On a mismatch it counts a miss, re-reads the file, and overwrites the entry. `get(p)` is now `getCanon(p, p.String())` for unit files.
- The include site in loader.go passes the `canon` already computed by `canonicalPath` (loader.go:478). This adds no syscalls.
- `Loader.Invalidate` evicts an entry when `underAny(set, key)` or `underAny(set, pf.canon)` holds (loader.go:96). The clear-all rule for out-of-contract paths is unchanged. Doc comments on parseStore, get/getCanon and Invalidate now describe the alias rule.
- Red output on HEAD (93642d4), for the record:
  ```
  --- FAIL: TestAliasCacheTargetEditRealFS   alias_cache_test.go:72: incremental result differs from fresh Loader (stale alias entry)
  --- FAIL: TestAliasCacheDirLinkRealFS      alias_cache_test.go:86: incremental result differs from fresh Loader (stale alias entry)
  --- FAIL: TestAliasCacheRetargetChain      alias_cache_test.go:110: incremental result differs from fresh Loader (stale alias entry)
  --- FAIL: TestAliasCacheMapFS              alias_cache_test.go:127: incremental result differs from fresh Loader (stale alias entry)
  TestAliasCacheNoopZeroMisses PASS
  ```
- Every scenario also asserts that the edit changes the fresh result, so the comparison is not vacuous.
- `fstest.MapFS` with `Mode: fs.ModeSymlink` works on go1.27 (MapFS implements ReadLinkFS). TestAliasCacheMapFS therefore runs on windows, where the multi-segment file-link real-FS test skips.

## Task Commits

1. Task 1: failing symlink-alias regressions: `93642d4`
2. Task 2: canonical path per cache entry, Invalidate and hit check: `371e6cd`

## Verification

- `go test -count=1 -run TestAliasCache -v ./internal/infrastructure/terragrunt/`: 5/5 PASS
- `go test -count=1 -run 'Golden|TestScripts|Corpus|E2E' ./cmd/gruntled/`: ok
- `go vet ./...`: clean. `GOOS=windows` and `GOOS=darwin` `go vet ./internal/infrastructure/terragrunt/`: clean
- `go test -count=1 ./...`: every package ok (cmd/gruntled 21.6s, terragrunt 4.5s, watch 0.9s, ...)
- `bash scripts/check-architecture.sh`: `architecture: OK (4 domain packages, 5 application packages, 1 interfaces packages)`
- Existing TestPersistentCache*, TestIncrementalEqualsFull, TestIncrementalModelNonVacuous and include_target* pass without changes.

## Threat Mitigations

| ID | Disposition | Where |
|----|-------------|-------|
| T-12-02-1 | mitigated | parse.go:132 (canon field), loader.go:96 (evict by key or canon); TestAliasCacheTargetEditRealFS, TestAliasCacheDirLinkRealFS, TestAliasCacheMapFS |
| T-12-02-2 | mitigated | parse.go:223 (hit only when stored canon == canon of this load), loader.go:478; TestAliasCacheRetargetChain |
| T-12-02-3 | accepted | canon was already computed per include; Invalidate walks the canonical path only when it differs from the key |
| T-12-02-4 | n/a | canonicalPath unchanged; it still fails closed before any cache read |
| T-12-02-5 | accepted | canon/read race on the miss path, as dispositioned in the plan; no hardening added |

## Deviations from Plan

**1. [Scope constraint] `get(p)` kept, include site uses `getCanon(p, canon)`**
- The plan changes `fileCache.get(p)` to `get(p, canon)`. That would require editing 15+ call sites in parse_test.go and cache_test.go, which are not in files_modified. Instead, `get(p)` stays as the unit-file form (canon = key) and `getCanon` is the two-argument form. Behaviour is the same. The key_link regex `cache\.get\(.*canon` does not match the call `cache.getCanon(file, canon)` literally. I asked the planner on the bus (#60); no answer came before handoff.

## Known Risks / Unverified

- `-race` could not run locally (`go: -race requires cgo`, no gcc). CI ubuntu runs it.
- The windows runtime of the real-FS tests is left to CI. TestAliasCacheTargetEditRealFS skips on windows, as its sibling does. The dir-link and retarget tests skip only if os.Symlink fails.
- `requirements-completed` is `[]` because DAEMON-02 also depends on 12-01 (the watcher directory-ignore BLOCKER).

## Self-Check: PASSED
