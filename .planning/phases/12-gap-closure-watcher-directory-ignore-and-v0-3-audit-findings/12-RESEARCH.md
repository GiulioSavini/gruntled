---
phase: 12-gap-closure-watcher-directory-ignore-and-v0-3-audit-findings
type: research
created: 2026-10-10
sources:
  - .planning/v0.3-MILESTONE-AUDIT.md
  - agent bus #5 #7 #8 #11-#16 #18 #19 (integration-checker + sec cross-phase audit)
---

# Phase 12 — Research (short)

Gap closure only: no new feature, no new requirement. Every item below was read in the code at
HEAD `bf9143e`; line numbers are from that commit.

## 1. Watcher applies file patterns to directories (BLOCKER, DAEMON-01/02)

`watch.Ignored(rel)` (ignore.go:21) = "some component is `.git`/`.terraform`/`.terragrunt-cache`"
OR `ignoredBase(last component)`. `ignoredBase` holds editor *file* patterns (`*~`, `*.sw[ponx]`,
`.#*`, `#*#`, `*.tmp`, `___jb_*___`, `.DS_Store`, 4–5 digit vim probe).

Three call sites cannot tell the base is a directory:

| Site | Effect on dir `live/2024` |
|------|---------------------------|
| native_unix.go:113 `addTree` | `Ignored("live/2024")` → `SkipDir`: never watched |
| native_unix.go:184 `handleEvent` | Create of `live/2024` dropped before the `addTree` call |
| poll.go:110 `scanner.walk` (poll + native safety net) | subtree never scanned |
| pending.go:33 `pending.add` | a dir path (e.g. dirsOnly rename) dropped from the dirty set |

Files *inside* (`live/2024/terragrunt.hcl`) are not themselves matched (only the last component
is), so the fix is purely "which rule applies to which entry type".

**Design.** One exported predicate keyed by entry type:

```go
// IgnoredEntry reports whether the repo-relative slash path rel must never reach
// the dirty set. typ is the entry's type bits (fi.Mode().Type() / d.Type()):
// fs.ModeDir, fs.ModeSymlink, ..., or 0 for a regular file or an unknown/gone entry.
// Ignored-directory components (.git, .terraform, .terragrunt-cache) apply to every
// entry; directories are never pattern-ignored; symlinks only by the Emacs lock
// rule (".#" prefix); every other entry by all editor patterns.
func IgnoredEntry(rel string, typ fs.FileMode) bool
func IgnoredDir(name string) bool // unchanged
```

`Ignored` is removed (only in-package callers) so nothing can keep calling the type-blind rule.
`pending.add(rel, typ)` filters with `IgnoredEntry`. Symlinks are pattern-ignored only by the `.#`
rule: Emacs creates its lock file `.#name` as a dangling symlink (target `user@host.pid:boot`) when a
buffer is first modified and removes it on save, so without that rule every edit session would cost
a reindex (sec #36). Any other symlink (e.g. a link named `2024` to a directory) the Loader may read
through must reach Invalidate.

fsnotify events do not carry the entry type. `handleEvent` keeps today's cost for normal names and
pays an `os.Lstat` only when the base name matches an editor pattern. It also consults a set of the
*pattern-named* directories it installed watches on (`patternDirs map[string]struct{}`, owned by the
event goroutine; filled by `addTree`, subtree root included when addTree runs from handleEvent for a
runtime-created dir). typ = `fs.ModeDir` when rel is in the set OR Lstat says dir (over-approximation,
never stale: a recorded dir moved out and replaced by a same-named file before the event is handled
is still reported, sec #37); otherwise Lstat's type, or 0 when gone (a vanished vim probe `4913`
stays ignored, contract "vim-style save" unchanged). Any Remove/Rename prunes rel and every key under
rel/ from the set.

Carried INFO 10-sec#5 fits here at no cost: `addTree` Lstats `p` right before `addWatch` (p != dir)
and skips it when it is a symlink or no longer a directory.

**Rapid test layer.** `TestIncrementalEqualsFull` (internal/infrastructure/terragrunt/incremental_test.go)
drives the Loader with a model-built dirty set over `fstest.MapFS`; it never runs a watcher. It can
still close the seam that broke, but only if it models the adapters' *subtree* skip, not just a
per-path filter: the BLOCKER lived in poll.go walk / native addTree returning SkipDir for `2024`,
so `2024/terragrunt.hcl` was never seen, while its own base name matches nothing (sec #35). The
model drops a dirty path p when `IgnoredEntry(p, typ)` OR some proper ancestor a has
`IgnoredEntry(a, fs.ModeDir)` (file paths typ 0, dirsOnly directory paths `fs.ModeDir`), and adds
pattern-named directories (`2024`, `x.tmp`) to the alphabet. With the old type-blind rule the
ancestor term drops every edit under `2024/`, so "write 2024/terragrunt.hcl, reindex, change it so
the output differs, reindex" diverges from a fresh load; with the new rule it holds. (A per-path
filter alone, or a dirsOnly rename of `2024`, passes on the old rule too: discovery simply loses the
2024 units and the new name is a cache miss.) `terragrunt` (test) → `watch`
is allowed (infrastructure → infrastructure) and has no cycle (`go list -deps ./internal/infrastructure/watch`
reaches no terragrunt package). The real-FS staleness (dirs never walked) is covered by the
contract suite (native + poll) and a cmd-level parity test.

## 2. Parse cache keyed by alias path (sec MEDIUM, DAEMON-02)

`fileCache.get(p)` (parse.go:195) keys the persistent `parseStore` by the lexical path. The only
non-canonical keys come from includes: loader.go:433 already computes `canon` with `canonicalPath`
(requires `canonOK`, the file to exist and be regular) and then calls `cache.get(file)` with the
lexical `file` (loader.go:458). Unit files are canonical by construction (discoverUnits never enters
symlinked dirs or counts symlinked configs).

Watchers report the target path (native: the event is on the target inode's directory; poll: Lstat
of the link is unchanged), so `Invalidate("live/parent/terragrunt.hcl")` never evicts key
`live/alias.hcl`.

**Design.** Store the canonical path on each entry and use it twice:

1. `Invalidate` evicts an entry when its key OR its canonical path equals or lies under an
   invalidated path (same ancestor walk as today, run on both strings).
2. On a hit, the entry is reused only if its stored canonical path equals the canonical path the
   current load computed; otherwise it is a miss (re-read, re-stored). This covers a retargeted link
   anywhere in a chain (`live/link1 -> link2 -> parent`, link2 retargeted): the event names `live/link2`,
   which is an ancestor of neither key nor old canonical, but loader.go:433 recomputes canon on every
   load, so the mismatch forces a re-read. Zero extra syscalls: canon is already computed.

Unit files pass `canon == key`. `parsedFile.path` (used in diagnostics) stays the lexical path, so
no output changes.

## 3. Runtime dir inside the watched root (sec LOW)

watch.go:134 checks only the status path. `acquireInstance` (instance.go:68) calls
`statusfile.EnsureRepoDir(root, env)` without asking whether `statusfile.Dir(root, env)` lies inside
`root`. In report-file mode the daemon rewrites `<rt>/report` after every index → poll sees it →
reindex loop (sec PoC: Generation=150 in 1.5 s). Fix: before `EnsureRepoDir`, compute `Dir`, call
`statusfile.Inside(root, dir)`, refuse with exit 2 and a message naming the directory and the env
variable that chose it, creating nothing.

## 4. `statusfile.Inside` is case-sensitive (sec LOW)

`Inside` compares `filepath.Rel` of two EvalSymlinks'd strings. On APFS/NTFS/drvfs a case variant of
the root is "outside". Fix: keep the string check as a fast `true`, then walk `p`'s existing
ancestors (deepest first, up to the volume root) and return true when `os.SameFile(stat(ancestor),
stat(root))`. Identity, not spelling: case-, alias- and bind-mount-proof. A package-level `statFn`
seam lets a linux test simulate two spellings of one directory; a real case-variant test runs where
the temp FS is case-insensitive (macos, windows CI) and skips elsewhere.

## 5. Terminal control sequences on text paths (sec LOW, consolidates 09-sec#2/#3, 11-sec#1/#2)

Text paths that print repo-controlled strings: `presenter.Text` (check, report snapshot text, watch
stdout via watch.go:248) and `presenter.BlastText` (incl. the verbatim `--base` label at blast.go:79).
`presenter.Summary` prints counts only. `presenter.Graph`/JSON/SARIF/BlastJSON go through
encoding/json, which escapes only < 0x20, quote, backslash, U+2028/2029 and invalid UTF-8: DEL, C1
(U+0080–U+009F; U+009B is CSI to xterm-family terminals) and every Cf rune are emitted raw (sec #38).
They get a lossless post-pass (`escapeJSON`) that rewrites those runes as `\uXXXX` inside the
encoder buffer; the runes can only occur inside JSON strings, so the decoded value is identical and
output for normal names is byte-identical. `SanitizeReason` maps `unicode.IsControl` to space but lets Cf (bidi overrides U+202A–202E,
U+2066–2069, zero-width U+200B–200F, U+FEFF), Zl U+2028 and Zp U+2029 through.

**Design.** One helper in the presenter (stdlib only, allowed by the interfaces allowlist):
`escapeTerm(s string) string` with a no-allocation fast path when nothing needs escaping. Escaped:
invalid UTF-8 bytes and C0/DEL as `\xNN`; C1 (U+0080–U+009F), Cf, Zl, Zp as `\uNNNN`
(`\UNNNNNNNN` above U+FFFF). Lowercase hex. Everything else, backslash included, is verbatim, so
every existing golden and testscript stays byte-identical. `SanitizeReason` additionally maps Cf/Zl/Zp
to a space.

## 6. golang.org/x/text v0.39.0 → v0.41.0

Indirect (via hcl). GO-2026-6629 not reachable per govulncheck. `go get golang.org/x/text@v0.41.0 &&
go mod tidy`; may pull a newer x/tools/x/mod indirectly; Step 9 proof and archscan must stay green.
Needs the module proxy; if offline, the executor reports a blocker instead of vendoring.

## Carried INFO dispositions

| Item | Disposition |
|------|-------------|
| 10-sec#5 Lstat before addWatch | fixed in 12-01 (cheap, same function) |
| 11-sec#3 single 64 MiB IPC cap | documented in docs/cli.md (12-05); protocol unchanged (same-uid peers only) |
| 11-sec#4 Windows runtime dir: no owner/ACL check | documented in docs/cli.md Guarantees (12-05); accepted, relies on per-user %LocalAppData% ACL |
| EventIndexing only published for the initial index | accepted/deferred: cosmetic (status shows the previous result during a reindex, then the new one); no correctness impact |
| DAEMON-04 wording ("windows report reads the status file") | REQUIREMENTS.md wording aligned to shipped behaviour (report file under the lock) in 12-05 |

## Plan-review dispositions (sec #35–#42, revision round 1)

| Finding | Disposition |
|---------|-------------|
| #35 MEDIUM rapid model cannot see the subtree skip | fixed in 12-05 T1: dirty filter models entry + ancestor-dir skip; fixed 2024/x.tmp tail; permanent TestIncrementalModelDetectsTypeBlindIgnore; shrunk mutation sequence in SUMMARY |
| #36 LOW Emacs lock files are dangling symlinks | fixed in 12-01: `.#` rule also applies to symlinks; contract "emacs lock symlink" |
| #37 LOW handleEvent Lstat overrides patternDirs | fixed in 12-01 T2: ModeDir when recorded OR Lstat dir; root recorded; prefix prune on Remove/Rename; contract + TestNativePatternDirReplacedByFile |
| #38 LOW JSON outputs carry DEL/C1/Cf raw | fixed in 12-04: escapeJSON post-pass on JSON/SARIF/graph/blast JSON with round-trip unit + fuzz test |
| #39 LOW tidy gate always fails after go get | fixed in 12-05 T3: cmp against pre-tidy copies; go list -m and Step 9 tail quoted |
| #40 INFO canon/read race | accepted: T-12-02-5 |
| #41 INFO SameFile false positive / slow automount | accepted: T-12-03-6 (fails closed) |
| #42 INFO display-only rune classes; watch.go:185 raw %v | accepted: T-12-04-8; watch.go:185 through SanitizeReason in 12-05 T2 (T-12-05-7) |
