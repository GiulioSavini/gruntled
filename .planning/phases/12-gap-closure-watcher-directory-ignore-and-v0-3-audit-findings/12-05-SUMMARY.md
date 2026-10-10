---
phase: 12-gap-closure-watcher-directory-ignore-and-v0-3-audit-findings
plan: 05
subsystem: daemon-proof, docs, deps
tags: [rapid, property-test, incremental, watcher, symlink, docs, supply-chain, terminal-escaping]
requires:
  - phase: 12
    provides: "12-01 IgnoredEntry(rel, typ); 12-02 canonical-path parse cache eviction; 12-03 runtime-dir refusal; 12-04 SanitizeReason covers Cf/Zl/Zp"
provides:
  - "rapid incremental == full model with a typed dirty set filtered like the adapters (entry + ancestor-dir subtree skip)"
  - "pattern-named unit dirs (2024, x.tmp) and a symlink op in the model, with counters"
  - "TestIncrementalModelDetectsTypeBlindIgnore (permanent mutation guard)"
  - "watch run-error stderr line through presenter.SanitizeReason"
  - "docs/cli.md and watch -h aligned with 12-01..12-04; darwin kqueue limitation (fsnotify#787)"
  - "golang.org/x/text v0.41.0 (GO-2026-6629 out of the graph)"
affects: []
tech-stack:
  added: []
  patterns: [model filters dirty paths with the production ignore predicate; recording fatalf proves a check can fail]
key-files:
  created:
    - cmd/gruntled/watch_stderr_test.go
  modified:
    - internal/infrastructure/terragrunt/incremental_test.go
    - cmd/gruntled/watch.go
    - docs/cli.md
    - .planning/REQUIREMENTS.md
    - go.mod
    - go.sum
key-decisions:
  - "ign is a package-level test var defaulting to watch.IgnoredEntry; delivered(p, typ) = !ign(p, typ) && no proper ancestor a has ign(a, fs.ModeDir)"
  - "Link op draws its target mostly from existing regular include files, and seed writes b/common.hcl, so linkedIncludes is reliably > 0 (min 4 over 100 runs)"
  - "Ops that leave a symlink cycle are undone (guarded): fstest.MapFS has no loop limit and overflows the stack"
  - "The run-error line goes through a small helper printRunError, because no existing seam makes watch.Run fail with attacker-shaped text"
requirements-completed: [DAEMON-01, DAEMON-02]
duration: 25min
completed: 2026-10-10
---

# Phase 12 Plan 05: Property Test, Docs and Gate Summary

**The rapid incremental == full property now generates the two cases the v0.3 audit found it never generated. It covers directories named like editor files (`2024`, `x.tmp`) with the watcher's subtree skip modelled, and include files reached through in-repo symlinks. With the pre-12-01 type-blind predicate or with 12-02's canonical eviction disabled, the property fails. The watch run-error line is sanitised, the docs and DAEMON-04 now describe what shipped, x/text is v0.41.0, and the full gate is green.**

## Accomplishments

- **Typed dirty set, filtered like the adapters** (incremental_test.go:46-60). `ign` defaults to `watch.IgnoredEntry`. `delivered(p, typ)` drops an entry when `ign(p, typ)` holds, or when a proper ancestor `a` has `ign(a, fs.ModeDir)`. The second check models poll's walk and native's addTree returning SkipDir. `reindex` invalidates only the delivered paths. Entry types: file write/remove/rename use 0, `renameDir` dirsOnly uses `fs.ModeDir` for both dirs, and `link` uses `fs.ModeSymlink`.
- **Alphabet.** `modelUnitDirs` and `modelRenameDirs` gain `2024` and `x.tmp`.
- **Link op** (incremental_test.go:192, op at :440). It writes `fstest.MapFile{Mode: fs.ModeSymlink, Data: <target relative to the link dir>}` and dirties only the link path. A later edit of the target dirties only the target.
- **Counters**, asserted > 0 at :478:
  - `patternDirRenames` (:275): dirsOnly renames from or to `2024`/`x.tmp`.
  - `linkedIncludes` (:356): reindexes in which a resolved unit's direct include is a symlink entry. It mirrors find_in_parent_folders starting at the parent dir.
- **TestIncrementalModelNonVacuous** now has more tails:
  - `patternDirTail` (:557) for `2024` and `x.tmp`. It writes a kind-0 unit, reindexes, rewrites the unit as broken HCL (GRT100), and reindexes again. It guards that the two full renders differ, and each reindex compares against a fresh Loader.
  - A `renameDir("2024", "e", true)`.
  - `symlinkTail` (:569): `common.hcl` links to `b/common.hcl`, unit `a` is kind 4, reindex, edit `b/common.hcl`, reindex. It also guards that the output changes and that linkedIncludes > 0.
- **TestIncrementalModelDetectsTypeBlindIgnore** (:591), permanent. It swaps in `func(rel, _) bool { return watch.IgnoredEntry(rel, 0) }` (restored with t.Cleanup) and runs the 2024 and x.tmp tails with a recording `fatalf`. It asserts that the check failed on the reindex after the edit.
- Green run, default 100 checks: `reindexes=389 noop=47 resolvedIncludes=118 grt100=112 patternDirRenames=90 linkedIncludes=20`, 0.06s. With `-rapid.checks=1000`: `reindexes=4104 noop=489 resolvedIncludes=1232 grt100=1035 patternDirRenames=687 linkedIncludes=122`, PASS in 0.61s. Over 100 repeated default runs the lowest `linkedIncludes` was 4.

### Mutation proof (each edit reverted after the run; rapid failfiles deleted)

This rapid version prints `[rapid] failed after N tests` followed by the shrunk draw log. It does not print a "minimal failing" header.

1. **Type-blind `ign`.** One-line edit `var ign = func(rel string, _ fs.FileMode) bool { return watch.IgnoredEntry(rel, 0) }`. TestIncrementalEqualsFull **FAILED**:
   ```
   incremental_test.go:387: [rapid] failed after 7 tests: incremental != full rescan
       files: [b/common.hcl b/terragrunt.hcl modules/m1/main.tf modules/m2/main.tf root.hcl x.tmp/terragrunt.hcl]
   [rapid] draw action: "create"
   [rapid] draw path: "x.tmp/terragrunt.hcl"
   [rapid] draw unitKind: 1
   [rapid] draw sibling: "a"
   [rapid] draw action: "reindex"
   [rapid] draw action: "rename"
   [rapid] draw from: "a/terragrunt.hcl"
   [rapid] draw to: "x.tmp/terragrunt.hcl"
   [rapid] draw action: "reindex"
   incremental_test.go:325: incremental != full rescan
   --- FAIL: TestIncrementalEqualsFull (2.51s)
   ```
   The rename onto `x.tmp/terragrunt.hcl` is dropped by the ancestor skip, so the daemon keeps the old parse.
2. **Canonical eviction disabled** (loader.go:96 changed to `underAny(set, key) || (false && pf.canon != key && underAny(set, pf.canon))`). TestIncrementalEqualsFull **FAILED**:
   ```
   incremental_test.go:387: [rapid] failed after 29 tests: incremental != full rescan
   [rapid] draw action: "create"
   [rapid] draw path: "a/sub/terragrunt.hcl"
   [rapid] draw unitKind: 2
   [rapid] draw sibling: "a"
   [rapid] draw action: "link"
   [rapid] draw dangling: 0
   [rapid] draw link: "a/root.hcl"
   [rapid] draw target: "root.hcl"
   [rapid] draw action: "reindex"
   [rapid] draw action: "edit"
   [rapid] draw path: "root.hcl"
   [rapid] draw include: "locals {"
   [rapid] draw action: "reindex"
   ```
   `a/sub` includes `a/root.hcl`, which is a link to `root.hcl`. Only the target is dirtied, and the alias entry stays stale. TestIncrementalModelNonVacuous also FAILED under this mutation, through `symlinkTail`.
3. **Ancestor term removed from `delivered`** (`if false && ign(a, fs.ModeDir)`). TestIncrementalModelDetectsTypeBlindIgnore **FAILED**: `2024: type-blind ignore rule went undetected: incremental == full after editing 2024/terragrunt.hcl`. A per-entry-only filter cannot see the BLOCKER (sec #35).

### Task 2

- `printRunError` (watch.go:191-196) is called at watch.go:185. It prints `gruntled: %s` with `presenter.SanitizeReason(err.Error())`.
  - Red: `4fb3fad` fails to build (`undefined: printRunError`).
  - Mutation check: with the helper body changed back to a raw `%v`, TestWatchRunErrorSanitised FAILs 4 times (ESC, BEL, U+202E, U+2028 all present).
- watch -h exit 2 line (watch.go:42, docs/cli.md:192 and :271) now reads `... --status-file or runtime directory inside the repository`.
- docs/cli.md changes:
  - Incremental: a file reached through an in-repo symlink is re-read when its target changes.
  - Ignored paths (:787): editor patterns apply to files only, and symlinks are pattern-ignored only for `.#name`.
  - Status file: runtime-directory refusal (exit 2, nothing created). Inside-ness is decided by file identity, so a case variant counts as inside. When the default status file is also inside, the status-file message fires first.
  - report: the 64 MiB response cap (:925, 11-sec#3).
  - Guarantees: the windows `%LocalAppData%` ACL reliance (:956, 11-sec#4).
  - Known limitations: the darwin kqueue dangling-symlink issue with the fsnotify#787 link (:1009, orchestrator #75/#78).
- REQUIREMENTS.md DAEMON-04: "on windows `report` reads the report file the daemon rewrites in its runtime directory, only while the daemon holds its lock". The checkbox and traceability row are unchanged.

## Task Commits

1. `497d48f` test(12-05): rapid model generates pattern-named dirs and symlinked includes
2. `4fb3fad` test(12-05): add failing test for the sanitised watch run-error line
3. `412ee23` fix(12-05): sanitise the watch run-error line on stderr
4. `12ecefb` docs(12-05): document file-only editor patterns, runtime-dir refusal, report cap and windows ACL reliance
5. `c442ae3` chore(12-05): bump golang.org/x/text to v0.41.0
6. `326feaa` test(12-05): spell control and format runes as escapes in the stderr test (staticcheck ST1018)

## Gate (local, HEAD 326feaa)

```
$ go list -m golang.org/x/text
golang.org/x/text v0.41.0
$ test -z "$(gofmt -l .)"                 -> clean
$ go mod tidy -diff                       -> no diff, exit 0
$ T=$(mktemp -d); cp go.mod go.sum "$T"/ && go mod tidy && cmp go.mod "$T"/go.mod && cmp go.sum "$T"/go.sum
cmp: go.mod and go.sum unchanged by tidy
$ go build ./... && go vet ./...          -> build ok, vet clean
$ GOTOOLCHAIN=go1.27.1 go run honnef.co/go/tools/cmd/staticcheck@v0.8.1 ./...   -> clean, exit 0
$ govulncheck ./...
No vulnerabilities found.
  (same command on c442ae3~1, x/text v0.39.0, -show verbose: "Vulnerability #1: GO-2026-6629", unreachable)
$ go test -count=1 ./...
ok  cmd/gruntled 16.652s, ... terragrunt 1.436s, watch 1.165s, presenter 0.011s, archscan 0.022s (every package ok), exit 0
$ bash scripts/check-architecture.sh
architecture: OK (4 domain packages, 5 application packages, 1 interfaces packages)
$ bash scripts/test-check-architecture.sh
PASS nested-go-mod / nested-go-mod-dot-dir / go-work / unrelated-go-mod-in-dot-dir-allowed
all architecture self-tests passed
$ bash scripts/build-release.sh v0.0.0-gate 326feaa <scratch>/rel
built ... (6 targets); 4 tar.gz + 2 zip: OK; smoke: gruntled v0.0.0-gate (326feaa); exit 0
```

On success Step 9 (binary-no-net-no-exec) prints nothing beyond the final OK line. Its deny-list half, re-run per target by hand:
```
linux/amd64: deps=163 forbidden(net|net/*|os/exec|plugin|crypto/tls)=0 x/text pkgs=2
linux/arm64: deps=163 forbidden(net|net/*|os/exec|plugin|crypto/tls)=0 x/text pkgs=2
darwin/amd64: deps=161 forbidden(net|net/*|os/exec|plugin|crypto/tls)=0 x/text pkgs=2
darwin/arm64: deps=161 forbidden(net|net/*|os/exec|plugin|crypto/tls)=0 x/text pkgs=2
windows/amd64: deps=162 forbidden(net|net/*|os/exec|plugin|crypto/tls)=0 x/text pkgs=2
windows/arm64: deps=162 forbidden(net|net/*|os/exec|plugin|crypto/tls)=0 x/text pkgs=2
```

Tidy/MVS moved other modules (all indirect). They are required by x/text v0.41.0's go.mod (`x/tools v0.48.0`, `x/mod v0.38.0`, `x/sync v0.22.0`) and through them `x/sys v0.47.0`: mod 0.37→0.38, sync 0.21→0.22, sys 0.46→0.47, tools 0.47→0.48. No module was added.

## Requirements evidence

- **DAEMON-01** (watch reindexes on change). 12-01 made pattern-named directories watched, scanned and reindexed on both adapters (contract sub-tests, TestWatchPatternDirParity). This plan proves at the model level that the dirty set reaching Invalidate under the shipped predicate keeps incremental == full for those dirs (property + tails), and that the type-blind predicate breaks it (DetectsTypeBlindIgnore).
- **DAEMON-02** (incremental result identical to a full check). 12-02 added canonical eviction (TestAliasCache*). The property now generates symlinked includes and pattern-named dirs, is green at 100 and 1000 checks with non-zero counters, and fails under both regressions (mutations 1 and 2 above).

## Threat Mitigations

| ID | Disposition | Where |
|----|-------------|-------|
| T-12-05-1 | mitigated | incremental_test.go:46 (`ign`), :53-60 (`delivered`: entry + ancestor-dir skip), :192/:440 (link op), :275/:356/:478 (counters > 0), :557/:569 (fixed tails), :591 (TestIncrementalModelDetectsTypeBlindIgnore); mutation runs above |
| T-12-05-2 | mitigated | go.mod:21; tidy no-op proven by cmp against pre-tidy copies; per-target deny-list 0 forbidden on 6 targets; check-architecture OK; govulncheck clean |
| T-12-05-3 | mitigated | x/text v0.41.0 (c442ae3); GO-2026-6629 listed before, absent after |
| T-12-05-4 | mitigated | TestHelpMatchesDocs green with the new exit-2 line (watch.go:42, docs/cli.md:192/:271); doc text checked against instance.go:81 and watch.go:140 messages and ipc.go:43 (`maxResponse = 64 << 20`) |
| T-12-05-5 | accepted | 64 MiB cap documented (docs/cli.md:925) |
| T-12-05-6 | accepted | windows ACL reliance documented (docs/cli.md:956); to record in 12-SECURITY.md |
| T-12-05-7 | mitigated | watch.go:185 → printRunError watch.go:194-196 (SanitizeReason); TestWatchRunErrorSanitised (watch_stderr_test.go) |

## Deviations from Plan

**1. [Rule 3, harness limit] Symlink cycles undone in the model.** fstest.MapFS resolves symlinks recursively with no loop limit, and a link cycle (e.g. `root.hcl` ↔ `a/root.hcl`) overflows the goroutine stack. Each rapid op therefore runs inside `guarded` (incremental_test.go:202), which restores the map in place and truncates the dirty set when the op leaves a cycle (`linkCycle`, :215). ELOOP handling on a real filesystem is outside the model's reach.

**2. [Rule 2, test strength] Link target bias and seed.** With uniform draws, `linkedIncludes` was 0 in about 1 of 10 runs, which made the vacuity check flaky. The link op now draws its target from existing regular include files 3 times out of 4 (otherwise from all includes, so dangling links still occur), and `seed` writes `b/common.hcl`. Over 100 runs the lowest value was 4.

**3. [Scope] Tail edit kind.** The plan's example was a misspelled dependency output (GRT001). The tails use broken HCL (GRT100, `unitContent(unitKinds-1, ...)`) instead. Either way the output changes, and the guard asserts it.

**4. [No seam] Run-error test uses a helper.** No existing seam makes `watch.Run` return attacker-shaped text: the only error is the initial-index root walk error, and it names `.` rather than a repository path. Following the plan's fallback, no injection seam was added. The line moved into the unexported helper `printRunError`, and the test drives that helper with ESC, BEL, U+202E and U+2028. The grep in the verify step still matches watch.go:195.

**5. [Gate] staticcheck ST1018.** The first version of the stderr test had literal U+202E/U+2028 in string literals. It was fixed with escape sequences in 326feaa.

## Known Risks / Unverified

- `-race` could not run locally (no cgo/gcc). CI covers it on linux, macos and windows after the orchestrator pushes.
- Nothing pushed. CI is still needed for success criterion 4.

## Self-Check: PASSED
