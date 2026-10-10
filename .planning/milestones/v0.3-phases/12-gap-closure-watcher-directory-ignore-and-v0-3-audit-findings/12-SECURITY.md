---
phase: 12-gap-closure-watcher-directory-ignore-and-v0-3-audit-findings
audited: 2026-10-10
auditor: gruntled-sec (code-audit, bus verdict #94, count correction #95)
scope: "git diff 455afe2~1..f1c23bd -- ':!.planning'"
verdict: SECURED
threats_total: 31
threats_mitigated: 20
threats_accepted: 10
threats_na: 1
threats_open: 0
findings: { blocker: 0, high: 0, medium: 0, low: 1, info: 4 }
---

# Phase 12 Security

Verdict **SECURED**, 0 open threats. Every `mitigate` row was verified at file:line and by
mutation: each mitigation was removed in a throwaway copy and its tests failed.

Baseline at f1c23bd: `go vet` clean, `go test ./...` green, `-race` green in CI (run
38043996664, linux/macos/windows), no `net`/`net/*`/`os/exec`/`plugin`/`crypto/tls` in
`go list -deps` on the six release targets, `check-architecture.sh` OK, `go mod verify` OK,
`govulncheck` clean (GO-2026-6629 gone with x/text v0.41.0).

## Threat register

| ID | Disposition | Status | Evidence |
|----|-------------|--------|----------|
| T-12-01-1 | mitigate | CLOSED | `ignore.go:36-60` type-aware `IgnoredEntry`; real types from `poll.go:120`, `native_unix.go:131,164,256`; `TestWatchPatternDirParity`; live repro below |
| T-12-01-2 | mitigate | CLOSED | file + `.#` symlink rule `ignore.go:53-58`; contract "vim-style save", "emacs lock symlink" (skipped on darwin native, see F1) |
| T-12-01-3 | mitigate | CLOSED | Lstat only on editor-pattern names `native_unix.go:247` |
| T-12-01-4 | mitigate (narrowed) | CLOSED | `lstatBeforeWatch` `native_unix.go:143-149`; seam test `native_unix_test.go:217`; residual window accepted |
| T-12-01-5 | accept | CLOSED | `patternDirs` bounded and pruned (`native_unix.go:168`) |
| T-12-01-6 | mitigate | CLOSED | `known \|\| Lstat dir` `native_unix.go:233,250`; `TestNativePatternDirReplacedByFile`, contract "mkdir … then rename it away" |
| T-12-02-1 | mitigate | CLOSED | `parsedFile.canon` `parse.go:132`; Invalidate by key or canon `loader.go:95-111`; `TestAliasCache*` |
| T-12-02-2 | mitigate | CLOSED | `getCanon` reuse only on equal canon `parse.go:223`, `loader.go:478`; `TestAliasCacheRetargetChain` |
| T-12-02-3 | accept | CLOSED | canon already computed; ≤2× ancestor walk |
| T-12-02-4 | n/a | CLOSED | canonicalPath unchanged, fails closed |
| T-12-02-5 | accept | CLOSED | canon/read race: same uid, sub-ms, self-heals on next edit, 30 s safety net |
| T-12-03-1 | mitigate | CLOSED | `instance.go:70-83` refusal before `EnsureRepoDir`; `TestWatchRuntimeDirInsideRepo` |
| T-12-03-2 | mitigate | CLOSED | same guard; nothing created on refusal (PoC) |
| T-12-03-3 | mitigate | CLOSED | `statusfile/path.go:89-110` `os.SameFile` on ancestors; `TestInsideBySameFile`; drvfs case-variant PoC refused |
| T-12-03-4/5/6 | accept | CLOSED | startup-only cost; user's own environment; SameFile false positive / slow automount fail closed (exit 2) |
| T-12-04-1 | mitigate | CLOSED | `escapeTerm` in `text.go:28-39`, `blast.go:82-142`; `TestTextOutputsEscapeControls` |
| T-12-04-2 | mitigate | CLOSED | Cf, U+2028/9 `escape.go:17`; `SanitizeReason` `status.go:71` |
| T-12-04-3 | mitigate | CLOSED | `--base` label escaped `blast.go:82` |
| T-12-04-4 | mitigate | CLOSED | shared presenter; report text/json/sarif byte-identical to check |
| T-12-04-5/6/8 | accept | CLOSED | backslash ambiguity documented `docs/cli.md:314-318`; check/graph/blast stderr only echoes the user's own argument; display-only classes (Mn, Zs, Hangul filler, private use) |
| T-12-04-7 | mitigate | CLOSED | `escapeJSON` at `json.go:124`, `sarif.go:333`, `graph.go:178`, `blast.go:195`; round-trip + fuzz tests |
| T-12-05-1 | mitigate | CLOSED | `TestIncrementalModelDetectsTypeBlindIgnore`; mutation runs fail `TestIncrementalEqualsFull` |
| T-12-05-2/3 | mitigate | CLOSED | x/text v0.41.0, `go mod verify`, six-target deny-list, govulncheck |
| T-12-05-4 | mitigate | CLOSED | `TestHelpMatchesDocs`; docs updated |
| T-12-05-5/6 | accept | CLOSED | 64 MiB IPC response cap (`docs/cli.md`), Windows `%LocalAppData%` ACL reliance (`docs/cli.md`) |
| T-12-05-7 | mitigate | CLOSED | `printRunError` via `SanitizeReason` `watch.go:195`; `watch_stderr_test.go` |

## Findings

| # | Severity | Ref | Finding | Disposition |
|---|----------|-----|---------|-------------|
| F1 | LOW | `docs/cli.md` Known limitations | fsnotify v1.10.1 kqueue `dirChange` stops at the first dangling symlink; on darwin native, files after it are not seen when created and rename-replaced files are not re-watched, so changes wait for the 30 s safety net. Pre-existing, upstream. | Doc widened (orchestrator, phase close); upstream fsnotify#787 with the rename case added. Accepted: late by ≤30 s, darwin native only, `check`/`--poll` unaffected. |
| F2 | INFO | `native_unix.go:168` | `patternDirs` not cleared on inotify overflow: only an extra reindex | Accepted |
| F3 | INFO | `escape.go:110` | escape cost linear (ASCII 948 MB/s, worst 59 MB/s, ≤3× growth) | Accepted |
| F4 | INFO | `.github/workflows/ci.yml` | `GOTOOLCHAIN=go1.27.1` downloads a toolchain in CI for staticcheck only; verified via sum.golang.org | Accepted until a staticcheck release reads go1.27.2 export data |
| F5 | INFO | `go.mod` | x/text bump raised x/sys v0.46.0 → v0.47.0 (linked on linux/darwin); also x/mod 0.38, x/sync 0.22, x/tools 0.48 (not linked) | Recorded; govulncheck clean, Step 9 green |

## v0.3 audit PoCs re-run at f1c23bd

- Directory names that look like editor files (`live/2024`, `x.tmp`, `live/2025~` → `gone4913`): 7 steps, `report` equals `check` (stdout and exit code) on native and `--poll`. In v0.3 the daemon stayed stale.
- Symlink alias cache (file link, directory link, two-hop chain retarget): every step matches `check` on both backends.
- Runtime dir inside the repo (`XDG_RUNTIME_DIR` under the repo, via symlink alias, drvfs case variants): exit 2, nothing created; `--status-file` case variant refused.
- Terminal escapes (ESC/BEL, C1, RLO, DEL directory names): 0 raw control bytes across check/graph/blast/report text+json+sarif, watch stdout/stderr and the status file; JSON decodes to the exact names.
