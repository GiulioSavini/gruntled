---
phase: 12-gap-closure-watcher-directory-ignore-and-v0-3-audit-findings
verified: 2026-10-10T11:08:33Z
status: passed
score: 4/4 must-haves verified
covered_files:
  - .planning/phases/12-gap-closure-watcher-directory-ignore-and-v0-3-audit-findings/12-01-PLAN.md
  - .planning/phases/12-gap-closure-watcher-directory-ignore-and-v0-3-audit-findings/12-01-SUMMARY.md
  - .planning/phases/12-gap-closure-watcher-directory-ignore-and-v0-3-audit-findings/12-02-PLAN.md
  - .planning/phases/12-gap-closure-watcher-directory-ignore-and-v0-3-audit-findings/12-02-SUMMARY.md
  - .planning/phases/12-gap-closure-watcher-directory-ignore-and-v0-3-audit-findings/12-03-PLAN.md
  - .planning/phases/12-gap-closure-watcher-directory-ignore-and-v0-3-audit-findings/12-03-SUMMARY.md
  - .planning/phases/12-gap-closure-watcher-directory-ignore-and-v0-3-audit-findings/12-04-PLAN.md
  - .planning/phases/12-gap-closure-watcher-directory-ignore-and-v0-3-audit-findings/12-04-SUMMARY.md
  - .planning/phases/12-gap-closure-watcher-directory-ignore-and-v0-3-audit-findings/12-05-PLAN.md
  - .planning/phases/12-gap-closure-watcher-directory-ignore-and-v0-3-audit-findings/12-05-SUMMARY.md
  - cmd/gruntled/escape_test.go
  - cmd/gruntled/instance.go
  - cmd/gruntled/instance_test.go
  - cmd/gruntled/watch.go
  - cmd/gruntled/watch_patterndir_test.go
  - cmd/gruntled/watch_stderr_test.go
  - docs/cli.md
  - go.mod
  - go.sum
  - internal/infrastructure/statusfile/inside_internal_test.go
  - internal/infrastructure/statusfile/path.go
  - internal/infrastructure/terragrunt/alias_cache_test.go
  - internal/infrastructure/terragrunt/incremental_test.go
  - internal/infrastructure/terragrunt/loader.go
  - internal/infrastructure/terragrunt/parse.go
  - internal/infrastructure/watch/contract_test.go
  - internal/infrastructure/watch/ignore.go
  - internal/infrastructure/watch/native_unix.go
  - internal/infrastructure/watch/pending.go
  - internal/infrastructure/watch/poll.go
  - internal/interfaces/presenter/blast.go
  - internal/interfaces/presenter/escape.go
  - internal/interfaces/presenter/graph.go
  - internal/interfaces/presenter/json.go
  - internal/interfaces/presenter/sarif.go
  - internal/interfaces/presenter/status.go
  - internal/interfaces/presenter/text.go
covered_digest: "v3:sha256:db79c6f9385addb6212b9b60f9317426722ed4a12f702e3d7a90d8fdfe02142d"
behavior_unverified: 0
overrides_applied: 0
re_verification:
  previous_status: passed
  previous_score: 4/4
  previous_verified: 2026-10-10T10:21:02Z
  reason: "stale digest: docs/cli.md changed in 8a94bb8 (darwin kqueue Known limitation bullet widened after the sec code-audit)"
  gaps_closed: []
  gaps_remaining: []
  regressions: []
---

# Phase 12: Gap Closure (Watcher Directory Ignore & v0.3 Audit Findings) Verification Report

**Phase Goal:** The daemon stays exactly as fresh as `check` for every directory name, and the open findings from the v0.3 milestone audit are closed or explicitly accepted.
**Verified:** 2026-10-10T11:08:33Z, HEAD 8a94bb8 (initial verification 2026-10-10T10:21:02Z at f1c23bd)
**Status:** passed
**Re-verification:** Yes. The initial report (passed, 4/4) went stale because one covered file, `docs/cli.md`, changed afterwards. This is a scoped re-check of that change, with a regression check on everything else.

## Re-verification (2026-10-10T11:08:33Z, HEAD 8a94bb8)

**What changed since f1c23bd.** `git diff --stat f1c23bd HEAD` lists only `docs/cli.md` (+8/-6) and planning files: REQUIREMENTS, ROADMAP, STATE, v0.3-MILESTONE-AUDIT, and the new 12-SECURITY.md, 12-VALIDATION.md and 12-VERIFICATION.md. No Go source, test, `go.mod` or `go.sum` changed. The working tree is clean, and HEAD = `origin/master` = 8a94bb8. So the code evidence below still holds unchanged. The only new content to verify is the doc bullet.

**The doc change.** In commit 8a94bb8 the darwin bullet under Known limitations (docs/cli.md:1004-1011) went from "a new file that sorts after a dangling symlink is not seen" to a wider version. It now also covers broken links committed to the repo and rename-on-save re-watch loss, and gives a 30 s worst case. I checked each claim against fsnotify v1.10.1 (`backend_kqueue.go`, the version pinned in go.mod) and against gruntled's code:

| Claim in docs/cli.md | Source | Holds? |
|----------------------|--------|--------|
| A dangling symlink stops kqueue directory scanning | `dirChange` (backend_kqueue.go:622-653) loops over `os.ReadDir` and calls `sendCreateIfNew` → `internalWatch` → `addWatch(name, noteAllEvents, listDir=true)`. With `listDir` set the link is not resolved by hand, but `unix.Open(name, O_EVTONLY\|O_CLOEXEC)` (system_darwin.go:8) follows it and gets ENOENT. `dirChange` maps `os.ErrNotExist` to `return nil` (line 646-647), so the rest of the listing is dropped. The initial `watchDirectoryFiles` (line 582) skips the link and keeps going, which matches the doc's "when created" wording. | Yes |
| Covers any dangling link, Emacs `.#name` or a broken link committed to the repo | The scan is inside fsnotify, below gruntled's `IgnoredEntry` filter. gruntled only calls `fw.Add(dir)` (native_unix.go:26), so every entry in a watched directory goes through `dirChange` whether or not gruntled ignores it. | Yes |
| Files that sort after it are not seen when created | `os.ReadDir` returns entries sorted by name, and the early return skips the rest. | Yes |
| A file replaced by a rename-on-save editor is not watched again, so later saves wait for the safety net | The old file's watch fires NOTE_RENAME. The event is delivered, `w.remove` drops the watch and `markSeen(false)` is recorded (lines 500-503), so the first save is still seen. The re-watch of the new file only happens in `dirChange`, which stops at the link. After that the replaced file has no kqueue watch. | Yes, for a file that sorts after the link (see Info below) |
| Late by at most 30 s; `check` and `--poll` not affected | `DefaultSafetyNet = 30 * time.Second` (native_unix.go:22), and production `NewNative` uses it (line 63). The ticker runs `s.diff()`, which compares size, mtime and mode (poll.go:81), so a rename-replaced file (new mtime) is picked up. `check` does not watch, and `--poll` uses the stat scanner with no kqueue. | Yes (plus debounce and index time) |
| Upstream fsnotify#787 | Same link as before. The contract test skip from fa04e9d links it too. | Yes |

Info, not a gap: the rename-on-save clause has no "sorts after it" qualifier, so for a broken link that sorts after the edited file it describes more delay than really happens. It errs on the cautious side, and for the Emacs case (`.#` sorts before every letter and digit) it is exact. No change needed.

**Commands run at 8a94bb8:**
- `go test -count=1 -run 'TestHelpMatchesDocs|TestReadme|Doc' -v ./cmd/gruntled/`: PASS for TestCIDoc (5 subtests), TestHelpMatchesDocs, TestReadmeDocumentsV03, TestSARIFDoc and TestValidationDocPins. Package `ok`.
- `go vet ./cmd/gruntled/`: exit 0.
- **CI run 38046342590** (push, master) at headSha 8a94bb8ed14d0ff4c7e07cd9a535de818be3ab88: conclusion success. Jobs `check`, `architecture`, `recipe-check`, `sarif-upload`, `test-os (macos-latest)` and `test-os (windows-latest)` all succeeded. The previous run, 38043996664 at f1c23bd, was also green.
- `verification.fingerprint` over the same covered set gives the new digest in the frontmatter. The covered file list is the same as before.

**New phase artifacts:** 12-SECURITY.md (verdict SECURED, see Security below) and 12-VALIDATION.md (`status: validated`, `nyquist_compliant: true`, `wave_0_complete: true`). Two bookkeeping items from my initial Info list are now resolved: VALIDATION is no longer draft, and ROADMAP and REQUIREMENTS were updated in 8a94bb8.

**Regression check on truths 1-4:** No code changed, so the evidence below still holds. Truth 4's CI evidence now also includes run 38046342590 at 8a94bb8. I did not re-run the live audit flow or the mutations: no Go file changed.

## Observable Truths

| # | Truth (ROADMAP success criterion) | Status | Evidence |
|---|-----------------------------------|--------|----------|
| 1 | Editor swap/backup/probe patterns (`4913`-style digits, `*.tmp`, `*~`, `#…#`, `.#…`) apply only to files. Directories such as `live/2024/` are walked, watched (native and `--poll`) and their edits reindexed. | VERIFIED | **Code:** `watch.IgnoredEntry(rel, typ)` (ignore.go). Only `.git`, `.terraform` and `.terragrunt-cache` components are ignored for every type. `typ&fs.ModeDir` returns false before any pattern applies. Symlinks are ignored only by the `.#` rule. The type-blind `Ignored` was removed, so no caller is left on the old rule. All entry points pass the real type: `pending.add(rel, typ)`; poll `walk` uses `d.Type()` and `diff()` returns `[]change{rel,typ}` (the type of a removed entry comes from `s.prev`); native `addTree` uses `d.Type()`; `handleEvent` takes the type from `patternDirs` or an Lstat for pattern-named paths. The safety net goes through `diff()`. **Live, native and `--poll`, built binary, copy of `cmd/gruntled/testdata/clean-fixture`:** I created `live/2024/terragrunt.hcl` with output `vpc_idd`. Status became `gruntled: 1 error (GRT001×1)`. Fixing it gave `gruntled: ok`. Breaking it again in the now-existing dir gave `1 error` again. At each step `report` text/json/sarif matched `check` byte for byte, with the same exit codes. A new unit in dirs named `x.tmp`, `#d#`, `bak~`, `4913` and `.#e` was picked up, and so was its removal, on both backends. **Tests:** `TestIgnoredEntry`, and the contract sub-tests "edit inside pattern-named dirs", "mkdir pattern-named dir", "rename pattern-named dir away", "mkdir … after start, then rename it away" and "emacs lock symlink" on both `TestWatcherContractNative` and `TestWatcherContractPoll`. Also `TestNativePatternDir{Watched,SafetyNet,ReplacedByFile}`, `TestNativeAddTreeSkipsSwappedDir` and `TestPollScannerDiffTypes`. All PASS. "vim-style save" still passes, so the patterns still apply to files. *Re-check at 8a94bb8: no source changed.* |
| 2 | A regression test reproduces the stale-daemon case (edit inside a digit-named directory) on the native and poll paths, and the rapid incremental == full test generates directory names that match file ignore patterns. | VERIFIED | `TestWatchPatternDirParity/{poll,native}` (cmd/gruntled/watch_patterndir_test.go) runs the real daemon over `live/2024/app` and `x.tmp/app`. It edits `vpc_idd`→`vpc_id`, waits for the status to equal a fresh `check`, and guards that every step changes the status. PASS. The rapid model has `modelUnitDirs`/`modelRenameDirs` ∋ `2024`, `x.tmp`. Its dirty set is filtered with the production `watch.IgnoredEntry` plus the ancestor-dir SkipDir (`delivered`). The counter `patternDirRenames` is asserted > 0; my run printed `patternDirRenames=62 linkedIncludes=8`. `TestIncrementalModelNonVacuous` and `TestIncrementalModelDetectsTypeBlindIgnore` PASS. **Mutation 1, done by me in a scratch copy:** I removed the `ModeDir` exemption from `IgnoredEntry`. Then `TestWatchPatternDirParity` FAILED on both poll and native (`timed out … last state: gruntled: ok`), `TestIncrementalEqualsFull` FAILED (`[rapid] failed after 1 tests: incremental != full rescan`), `TestIncrementalModelNonVacuous` FAILED, and 4 contract sub-tests FAILED on each adapter. *Re-check at 8a94bb8: no test or source changed.* |
| 3 | Every BLOCKER/HIGH/MEDIUM finding of the v0.3 cross-phase security audit is fixed with a test, or accepted with a documented reason. | VERIFIED | The only MEDIUM (symlink alias cache) is fixed and tested; mutations 2 and 3 confirm the tests catch a regression. All three LOWs are also fixed with tests. Mapping below. Nothing at MEDIUM or above is left that would need an acceptance entry. 12-SECURITY.md now independently confirms 0 open threats with no finding above LOW. |
| 4 | `go test ./...`, `go vet ./...`, `scripts/check-architecture.sh` and CI on linux/macos/windows are green. | VERIFIED | **Local (go1.27.2) at f1c23bd:** `go test -count=1 ./...` ok in all 20 packages; `go vet ./...` exit 0; `check-architecture.sh` reports `architecture: OK (4 domain, 5 application, 1 interfaces)`; `test-check-architecture.sh` all PASS; `gofmt -l .` clean; `go list -m golang.org/x/text` is v0.41.0. **CI run 38043996664 at headSha f1c23bd:** conclusion success. Jobs `check` (vet, staticcheck, govulncheck, `go test -race -count=1 ./...` all ok, release-build), `architecture`, `recipe-check`, `sarif-upload`, `test-os (macos-latest)` and `test-os (windows-latest)` are all success. **Re-check:** CI run 38046342590 at headSha 8a94bb8 also succeeded on all six jobs. Doc tests and `go vet ./cmd/gruntled/` pass locally at 8a94bb8. |

**Score:** 4/4 truths verified (0 present-but-behavior-unverified)

### v0.3 audit security findings mapped to fixes (SC3)

| Finding | Sev | Fix (code at HEAD) | Test / proof | Disposition |
|---------|-----|--------------------|--------------|-------------|
| Parse cache keyed by the symlink alias path, so editing the target leaves it stale (parse.go:195) | MEDIUM | `parsedFile.canon` records the canonical path that was read. `Loader.Invalidate` evicts when `underAny(set, key) \|\| (pf.canon != key && underAny(set, pf.canon))` (loader.go:96). `fileCache.getCanon` treats an entry as a hit only when `pf.canon == canon` for this load (parse.go:223); the include site calls it with the canon it already computed (loader.go:478). | `TestAliasCacheTargetEditRealFS`, `…DirLinkRealFS`, `…RetargetChain`, `…MapFS`, `…NoopZeroMisses` PASS. The rapid `link` op is counted (`linkedIncludes` > 0), and the `symlinkTail` is in NonVacuous. **Mutation 2 (mine):** with the canon term removed from Invalidate, TargetEdit, DirLink, MapFS and NonVacuous FAIL; the property FAILED in 1 of 3 default runs. **Mutation 3 (mine):** with getCanon ignoring canon, RetargetChain FAILS. **Live, both backends:** `live/app2/common.hcl -> ../../shared/common.hcl`. Editing `shared/common.hcl` flipped the status to `1 error`, and the fix flipped it back to `ok`. `report` matched `check` in all three formats. | Fixed + tested |
| LOW-1: runtime dir (lock/sock/report) not checked against the repo, so a report-file rewrite feeds a poll reindex loop | LOW | `acquireInstance` (cmd/gruntled/instance.go) calls `statusfile.Inside(root, rt)` and refuses with exit 2 before `EnsureRepoDir`. It covers socket and report-file mode. | `TestWatchRuntimeDirInsideRepo/{socket,report-file}` PASS. **Live:** `XDG_RUNTIME_DIR=<repo>/cache gruntled watch --status-file <outside> <repo>` exits 2 with "runtime directory … is inside the repository …", and `<repo>/cache` is not created. | Fixed + tested |
| LOW-2: `statusfile.Inside` is case-sensitive, so on APFS/NTFS/drvfs state can land inside the repo | LOW | `Inside` keeps the name fast path. Otherwise it compares every existing ancestor of `p` with the root by `os.SameFile` (statusfile/path.go). | `TestInsideBySameFile/*` and `TestInsideSkipsUnreadableAncestor` (statFn seam) PASS. `TestDirInsideCaseVariant` skips on ext4 (case-sensitive), but I ran it on a real case-insensitive FS (`TMPDIR=/mnt/c/Users/giuli/AppData/Local/Temp/…`, WSL drvfs) and it PASSED. statusfile is green on CI macos/windows. | Fixed + tested |
| LOW-3: terminal escape injection on every text path (check, report, blast, watch stdout), including carried 09-sec#2/#3 and 11-sec#1/#2 | LOW | `escapeTerm` is applied in `presenter.Text` and `BlastText` (label, subject, file, message, unit, module, change names). `escapeJSON` post-processes JSON, SARIF, graph and blast-json output. `SanitizeReason` also maps Cf/Zl/Zp. The watch run-error line goes through `printRunError` → SanitizeReason (watch.go:195). `Summary` prints counts only. | `TestEscapeTerm`, `TestEscapeJSON*`, `Fuzz{EscapeTerm,EscapeJSON}` (seed corpus), `Test{Text,BlastText}EscapesControls`, `Test{JSON,SARIF,Graph,BlastJSON}EscapesTerminalRunes`, `TestSanitizeReason`, e2e `TestTextOutputsEscapeControls/{watch,report,blast,check_json,check_sarif,graph_json,blast_json}` and `TestWatchRunErrorSanitised` all PASS. **Live:** a unit dir named `e<ESC>[31mx` prints as `live/e\x1b[31mx/terragrunt.hcl:4:16: GRT001 …`. | Fixed + tested |
| x/text v0.39.0 GO-2026-6629 (not reachable) | dep | Bumped to v0.41.0 (go.mod) | `go list -m` shows v0.41.0; govulncheck in CI `check` job succeeded | Fixed |
| INFO 11-sec#3 (64 MiB IPC cap), 11-sec#4 (no Windows ACL check) | INFO | Not code changes | Documented in docs/cli.md:926 and :956 | Accepted. Below SC3's threshold; recorded in 12-SECURITY.md |

## Required Artifacts

| Artifact | Expected | Status | Details |
|----------|----------|--------|---------|
| `internal/infrastructure/watch/ignore.go` | type-aware `IgnoredEntry`, `editorPattern` | VERIFIED | Substantive. Used by pending.go, poll.go, native_unix.go and the rapid model |
| `internal/infrastructure/watch/{pending,poll,native_unix}.go` | entry type threaded through every adapter | VERIFIED | `add(rel, typ)`, `change{rel,typ}`, `patternDirs`, Lstat before `addWatch` |
| `internal/infrastructure/terragrunt/{parse,loader}.go` | canonical path per cache entry, evicted and re-checked by it | VERIFIED | `canon` field, `getCanon`, canonical term in Invalidate |
| `internal/infrastructure/statusfile/path.go` | `Inside` by file identity | VERIFIED | SameFile ancestor walk |
| `cmd/gruntled/instance.go` | runtime-dir refusal before anything is created | VERIFIED | Runs before `EnsureRepoDir` |
| `internal/interfaces/presenter/escape.go` | `escapeTerm`, `escapeJSON` | VERIFIED | Wired into text.go, blast.go, json.go, sarif.go, graph.go and status.go |
| `cmd/gruntled/watch_patterndir_test.go`, `terragrunt/alias_cache_test.go`, `cmd/gruntled/escape_test.go`, `statusfile/inside_internal_test.go`, `cmd/gruntled/watch_stderr_test.go` | regression tests | VERIFIED | Exist and pass. Three of them proved to catch regressions under my mutations |
| `docs/cli.md` | file-only patterns, symlink re-read, runtime-dir refusal, escapes, 64 MiB cap, Windows ACL, darwin kqueue limitation | VERIFIED | Lines 192/271 (exit 2), 778, 787, 856, 926, 956, and 1004-1011 (darwin bullet, widened in 8a94bb8 and checked against fsnotify v1.10.1 above). `TestHelpMatchesDocs` PASS at 8a94bb8 |

## Key Link Verification

| From | To | Via | Status |
|------|----|-----|--------|
| poll scanner / native addTree / handleEvent / safety net | `pending.add` | `IgnoredEntry(rel, realType)` | WIRED. Live edits in `live/2024` reach the daemon on both backends |
| watcher dirty paths | `Loader.Invalidate` | watching.Indexer (unchanged from Phase 10) | WIRED. Status and report converge to `check` |
| symlink target event | alias cache entry | `underAny(set, pf.canon)` | WIRED. Live target edit is picked up |
| include resolution | cache hit | `cache.getCanon(file, canon)` (loader.go:478) | WIRED. Mutation 3 is caught |
| `acquireInstance` | `statusfile.Inside` | before `EnsureRepoDir` | WIRED. Live exit 2, nothing created |
| `presenter.Text` / `BlastText` / JSON encoders | `escapeTerm` / `escapeJSON` | shared presenter, inherited by check, report and watch stdout | WIRED. Live escape output; e2e report == check |

## Data-Flow Trace (Level 4)

| Artifact | Data | Source | Real data | Status |
|----------|------|--------|-----------|--------|
| status file / `report` | diagnostics after an edit in `live/2024` | watcher → Invalidate → LoadUnits → analysis | yes. Byte-identical to `check` text/json/sarif at every step (native and poll) | FLOWING |

## Behavioral Spot-Checks

| Behavior | Command | Result | Status |
|----------|---------|--------|--------|
| Audit flow, native | `gruntled watch <copy>`; create, fix and re-break `live/2024/terragrunt.hcl`; compare `report` and `check` | ok → 1 error → ok → 1 error; 3 formats IDENTICAL at each step; SIGTERM exit 0; afterwards `report` exits 3 | PASS |
| Audit flow, poll | same with `--poll` | same | PASS |
| Other pattern dirs + symlink alias | `x.tmp`, `#d#`, `bak~`, `4913`, `.#e`; edit of a linked include target | all picked up and removed, parity held, on both backends | PASS |
| Runtime dir inside repo | `XDG_RUNTIME_DIR=<repo>/cache gruntled watch …` | exit 2, no `<repo>/cache` | PASS |
| Escape | `check` on a unit dir containing ESC | `\x1b[31m` printed literally | PASS |
| No leftover daemons | `pgrep -af "gruntled watch"` | none | PASS |
| Docs match code (re-check, 8a94bb8) | `go test -count=1 -run 'TestHelpMatchesDocs\|TestReadme\|Doc' ./cmd/gruntled/` | ok, 5 tests PASS | PASS |
| CI at HEAD (re-check) | `gh run view 38046342590` | success, headSha 8a94bb8, 6/6 jobs success | PASS |

## Probe Execution

Not applicable. The phase declares no `scripts/*/tests/probe-*.sh`.

## Requirements Coverage

| Requirement | Source Plans | Description | Status | Evidence |
|-------------|--------------|-------------|--------|----------|
| DAEMON-01 | 12-01, 12-05 | watch reindexes only changed files, ignoring `.git`/`.terraform`/`.terragrunt-cache` and editor swap/backup files, and picks up new and deleted directories | SATISFIED (audit gap closed) | Truth 1, Truth 2, live repro on both backends |
| DAEMON-02 | 12-01, 12-02, 12-05 | incremental reindex == full rescan, proven by rapid property | SATISFIED (both audit gaps closed: pattern dirs and symlink alias) | Truth 2, MEDIUM row, mutations 1 to 3 |

Plans 12-03 and 12-04 also declare DAEMON-03, DAEMON-04, DAEMON-05 and BLAST-01, because those hardenings touch them. None of the four regressed: the full suite and CI are green, and the e2e report-equals-check parity holds. DAEMON-04's wording now describes the Windows report file. No orphaned requirements: REQUIREMENTS.md maps no other ID to Phase 12.

## Anti-Patterns Found

| File | Line | Pattern | Severity | Impact |
|------|------|---------|----------|--------|
| None | | No TBD/FIXME/XXX/TODO/HACK/placeholder in lines added by phase 12 (diff 49cc144..HEAD). The 8a94bb8 doc hunk adds none either | | |

Informational (non-blocking):
- The rapid property alone catches the alias regression only sometimes: at the default 100 checks, 1 of my 3 runs under mutation 2, because `linkedIncludes` is about 4 to 20 per run. The deterministic `TestAliasCache*` and the NonVacuous `symlinkTail` catch it every time, so DAEMON-02 holds. Raising `-rapid.checks` in CI would make the property stronger.
- Commit fa04e9d skips the native "emacs lock symlink" contract sub-test on darwin. The cause is upstream fsnotify kqueue (fsnotify#787): `dirChange` stops at the first dangling symlink. This is documented at docs/cli.md:1004-1011, widened in 8a94bb8 to cover committed broken links and rename-on-save re-watch loss, and it is accepted as sec F1 (LOW). It is outside SC1, which is about directory names, and it is a darwin-only freshness delay bounded by the 30 s safety net.
- New, from the re-check: the doc's rename-on-save clause has no "that sort after it" qualifier, so it slightly overstates the effect when the broken link sorts after the edited file. It errs on the cautious side; no action needed.

## Human Verification Required

None blocking. VALIDATION's two manual items are covered. `-race` is green in the CI `check` job. The case-variant `Inside` test passed here on a real case-insensitive filesystem (WSL drvfs). CI logs are not verbose, so I could not confirm from them that it ran unskipped on macos/windows, but the statusfile package is green there.

## Gaps Summary

None. Both audit gaps are closed in the code and confirmed live on native and `--poll`: directories named like editor files are now watched, and symlink-alias cache entries are now evicted. A regression in either one is caught by the tests, as the mutations showed. The MEDIUM and all three LOW sec findings are fixed with tests. The re-check found that the only covered file changed since the initial report is the docs/cli.md darwin bullet, that the bullet matches fsnotify v1.10.1 and gruntled's safety net, and that the doc tests and CI (run 38046342590) are green at 8a94bb8.

## Security

Verdict **SECURED** (gruntled-sec code-audit, 2026-10-10, bus #94, count correction #95; scope `455afe2~1..f1c23bd`).

- Mitigations: **20/20** verified at file:line and by mutation; 10 accepted with reasons; 1 not applicable; **0 open**.
- Findings: 0 blocker, 0 high, 0 medium, **1 LOW** and **4 INFO**.
  - F1 LOW: darwin native kqueue limitation (fsnotify v1.10.1 `dirChange` stops at the first dangling symlink, fsnotify#787). The docs/cli.md Known limitations bullet was widened at phase close (8a94bb8) and checked by this re-verification. Accepted: results are late by up to 30 s, only on darwin native; `check` and `--poll` are not affected.
  - F2 to F5 INFO: `patternDirs` not cleared on inotify overflow (only causes an extra reindex), linear escape cost, the CI staticcheck toolchain pin, and the x/sys bump from x/text. All accepted or recorded.
- All v0.3 audit PoCs fail against f1c23bd.

Full register: `12-SECURITY.md`.

---

_Verified: 2026-10-10T11:08:33Z (re-verification; initial 2026-10-10T10:21:02Z)_
_Verifier: Claude (gsd-verifier)_
