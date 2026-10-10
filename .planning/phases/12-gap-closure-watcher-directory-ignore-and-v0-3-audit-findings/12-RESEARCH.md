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
// entry; editor swap/backup/probe patterns apply only when typ has neither
// fs.ModeDir nor fs.ModeSymlink.
func IgnoredEntry(rel string, typ fs.FileMode) bool
func IgnoredDir(name string) bool // unchanged
```

`Ignored` is removed (only in-package callers) so nothing can keep calling the type-blind rule.
`pending.add(rel, typ)` filters with `IgnoredEntry`. Symlinks are never pattern-ignored: editors
create regular files, and a symlink the Loader may read through must reach Invalidate.

fsnotify events do not carry the entry type. `handleEvent` keeps today's cost for normal names and
pays an `os.Lstat` only when the base name matches an editor pattern. When the path is gone
(Remove/Rename) Lstat fails; the adapter then consults a set of the *pattern-named* directories it
installed watches on (`patternDirs map[string]struct{}`, owned by the event goroutine; filled by
`addTree`, pruned with descendants when such a dir disappears). Known → `fs.ModeDir` (reported,
Loader prefix-evicts); unknown → 0 (a vanished vim probe `4913` stays ignored, contract "vim-style
save" unchanged).

Carried INFO 10-sec#5 fits here at no cost: `addTree` Lstats `p` right before `addWatch` (p != dir)
and skips it when it is a symlink or no longer a directory.

**Rapid test layer.** `TestIncrementalEqualsFull` (internal/infrastructure/terragrunt/incremental_test.go)
drives the Loader with a model-built dirty set over `fstest.MapFS`; it never runs a watcher. It can
still close the seam that broke: route the model's dirty set through `watch.IgnoredEntry` (file
paths with typ 0, dirsOnly directory paths with `fs.ModeDir`) and add pattern-named directories
(`2024`, `x.tmp`, `bak~`) to the alphabet. With the old type-blind rule the dirsOnly rename of
`2024` is dropped and the property fails; with the new one it holds. `terragrunt` (test) → `watch`
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
`presenter.Summary` prints counts only and `presenter.Graph`/JSON/SARIF are JSON-escaped, so they need
nothing. `SanitizeReason` maps `unicode.IsControl` to space but lets Cf (bidi overrides U+202A–202E,
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
