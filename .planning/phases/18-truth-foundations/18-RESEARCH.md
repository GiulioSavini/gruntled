---
phase: 18-truth-foundations
type: research
created: 2026-10-16
code: master 18a1ba5
requirements: [INST-01, INST-02, HYG-03]
sources:
  - .planning/research/SUMMARY.md (Hard Rules, Version row, Phase 18 entry), STACK.md (ReadBuildInfo experiment), PITFALLS.md (Pitfall 1)
  - .planning/REQUIREMENTS.md (INST-01, INST-02, HYG-03, Hard rules), ROADMAP.md phase 18, PROJECT.md Key Decisions (ldflags-only row, "Revisit")
  - code at 18a1ba5: cmd/gruntled/{main,render,instance}.go, main_test.go, ci_doc_test.go, release_test.go, e2e_test.go,
    internal/interfaces/presenter/sarif.go, internal/infrastructure/ipc/ipc.go, scripts/build-release.sh,
    .github/workflows/{release,ci}.yml, docs/ci.md, docs/cli.md, README.md, .pre-commit-hooks.yaml, .planning/MILESTONES.md
  - one experiment: `go build ./cmd/gruntled && go version -m` on the checkout (go1.27.2)
---

# Phase 18: Research

## 0. Summary

Three small, independent deliverables. No new module, no new package layer: everything lands in
`cmd/gruntled`, `scripts/`, the two workflows and the docs. The only product-code change is a
version resolver in `cmd/gruntled` (plus a `runtime/debug` import, which links no `net`, checked
with `go list -deps runtime/debug`). `check` bytes cannot move: the resolver returns `dev` for every
test binary and every checkout build, which is what the goldens already see.

## 1. Facts at 18a1ba5

### 1.1 Version today
- `cmd/gruntled/main.go:44-47`: `var version = "dev"; commit = "none"` (ldflags `-X`).
- Three read sites, all `version` directly: `main.go:151` (`--version` prints
  `gruntled %s (%s)\n`), `render.go:24` (`presenter.ToolInfo{Version: version}`, the SARIF
  `tool.driver.version` for `check`, `report` and the daemon), `instance.go:122`
  (`ipc.Info{..., Version: version, ...}`).
- Tests: `main_test.go:153-179` `TestVersion` (rows 160-162 are the `dev (none)` contract, row 163
  injected), `TestVersionSARIF`, `TestVersionLdflags` (real `go build -ldflags`). `setVersion` swaps
  the two package vars.
- `scripts/build-release.sh` injects `-X main.version=$1 -X main.commit=$2` and smoke-compares
  `--version` to `gruntled $version ($commit)`; `release.yml` passes `$GITHUB_REF_NAME` and
  `git rev-parse --short=12 HEAD`; `ci.yml` runs the same script with `v0.0.0-ci` on every push.

### 1.2 What the toolchain stamps (experiment, go1.27.2)
`go build ./cmd/gruntled` in the checkout, then `go version -m`:
```
mod   github.com/GiulioSavini/gruntled  v0.3.1-0.20261016194242-18a1ba5e3a35
build vcs=git
build vcs.revision=18a1ba5e3a35a4fbac7061ecf77163f599de686d
build vcs.time=2026-10-16T19:42:42Z
build vcs.modified=false
```
A dirty tree appends `+dirty` to `Main.Version` and sets `vcs.modified=true`. A `go test` binary
and a build outside a VCS report `(devel)`. `go install pkg@vX.Y.Z` (module-cache build) reports
`Main.Version = vX.Y.Z` and **no** `vcs.*` settings (STACK.md experiment, HIGH). So the one
predicate that is true for `go install @tag` and false for every local build is:
clean release semver, module path matches, and no `vcs.*` setting.

Tags that exist: `v0.2.0`, `v0.3.0`. `v0.4.0` is untagged (STATE.md), so `@latest` resolves to a
pseudo-version today and must keep printing `dev (none)`.

### 1.3 Doc pins today
- `docs/ci.md`: `v0.2.0` at lines 7, 16, 42 (asset example), 50 (`tag=v0.2.0`), 84
  (`rev: v0.2.0`), 89, 138, 166, 203; the limitation paragraph at 24-26.
- `docs/cli.md:87` example `gruntled v0.2.0 (abc1234)`; limitation sentences at 91-95.
- `README.md`: no install snippet at all (only `## Building, testing and checking locally` with
  dev commands; `v0.3.0` at 491-495 is the compat-v1 protocol reference, not an install pin).
- `.pre-commit-hooks.yaml`: hook definition only; the `rev:` lives in the consumer snippet in
  `docs/ci.md`.
- `cmd/gruntled/ci_doc_test.go` `TestCIDoc` checks `go install github.com/GiulioSavini/gruntled/cmd/gruntled@`
  as a prefix only, so a stale pin passes today.

### 1.4 Release notes today
`release.yml` `publish` step: `gh release create "$GITHUB_REF_NAME" --generate-notes --verify-tag`.
The guard step already validates the tag against `^v[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.-]+)?$`
and `merge-base --is-ancestor origin/master`. The tag reaches `run:` blocks only through the
`$GITHUB_REF_NAME` env var, never through `${{ }}` interpolation (keep it that way).

### 1.5 CHANGELOG source
`.planning/MILESTONES.md` has the four shipped milestones with dates: v0.1 2026-09-29,
v0.2 2026-10-08, v0.3 2026-10-10, v0.4 2026-10-13, each with "Key accomplishments" and
"Known limitations". That is the backfill source.

## 2. Decisions

| ID | Decision | Why |
|----|----------|-----|
| D-18-01 | `resolveVersion(ldVersion, ldCommit string, bi *debug.BuildInfo, ok bool) (version, commit string)` is a pure function in `cmd/gruntled/version.go`; `readBuildInfo = debug.ReadBuildInfo` is a package var the tests swap; `versionInfo()` wraps both and is the only thing the three sites call | Table-testable without depending on how the test binary was built; PITFALLS 1 |
| D-18-02 | Precedence: ldflags value wins when `version != "dev"` (commit printed as given). Else accept `bi.Main.Version` only if `bi.Main.Path == modulePath`, it matches `^v\d+\.\d+\.\d+(-[0-9A-Za-z.-]+)?$`, contains no `+`, is not a pseudo-version (`-\d{14}-[0-9a-f]{12}$` or `\.0\.\d{14}-[0-9a-f]{12}$`), is not `(devel)`, and no `bi.Settings` key starts with `vcs.`. Then commit is `none`. Otherwise `dev`, `none` | INST-01; the vcs gate is what keeps checkout builds at `dev (none)` (PROJECT.md decision's reason preserved) |
| D-18-03 | `modulePath` is a const in version.go, pinned to go.mod's `module` line by a test | No parsing of go.mod at runtime |
| D-18-04 | `setVersion` in tests also pins `readBuildInfo` to a reader returning `(nil, false)` | Keeps rows 160-162 environment-independent; a new table test covers the reader |
| D-18-05 | `VERSION` file holds exactly `v0.4.0\n` (the last shipped milestone, the tag Giulio would create). Rule, written in `docs/ci.md` and the CHANGELOG header: VERSION is bumped, and the Unreleased section renamed, in the commit that gets tagged; `release.yml` fails when `VERSION` != tag | INST-02; v0.5.0 does not exist; no workflow tags |
| D-18-06 | Doc-pin checker is a pure function `checkVersionPins(version string, files map[string]string) []string` over fixed patterns: `go install github.com/GiulioSavini/gruntled/cmd/gruntled@<v>`, `rev: <v>`, `tag=<v>`, `gruntled_<v>_linux_amd64.tar.gz`, `gruntled <v> (none)`; every occurrence of each pattern with any `v\d+\.\d+\.\d+[-A-Za-z0-9.]*` must equal VERSION, and each pattern must occur at least once in its designated file (non-vacuity). Negative test feeds a drifted copy and expects exactly one named failure | SC2 "proven by a negative case" without editing real files |
| D-18-07 | README gains a short `## Install` section (go install @VERSION, release-archive pointer to docs/ci.md), pinned by the same checker | INST-02 names README install snippets; today there are none to pin |
| D-18-08 | `docs/cli.md` example becomes three lines: release `gruntled v0.4.0 (<12-hex>)` shape described in prose, `gruntled v0.4.0 (none)` for `go install @v0.4.0`, `gruntled dev (none)` for plain `go build`; the limitation sentences go | SC4 |
| D-18-09 | `CHANGELOG.md` is Keep a Changelog 1.1.0: `# Changelog`, intro, `## [Unreleased]`, `## [0.4.0] - 2026-10-13` ... `## [0.1.0] - 2026-09-29`, with `### Added/Changed/Fixed` groups and link references at the bottom (`[Unreleased]: .../compare/v0.4.0...HEAD`, `[0.4.0]: .../compare/v0.3.0...v0.4.0`, `[0.1.0]: .../releases/tag/v0.1.0`). Section headings carry no `v`; tag `vX.Y.Z` maps to `## [X.Y.Z]` | HYG-03 |
| D-18-10 | `scripts/release-notes.sh <tag> [changelog]` prints the body of the matching section (lines after `## [X.Y.Z] - date` up to the next `## [`), exit 1 with a message when absent, exit 2 on a malformed tag (same regex as the guard). Line matching is a fixed-string prefix compare in bash, never a regex built from the tag | HYG-03; no tag-controlled regex |
| D-18-11 | `ci.yml` `check` job gets a `release-notes dry run` step: positive (`$(cat VERSION)`) must print a non-empty body, negative (`v0.0.0-nosuch`) must exit 1; `release.yml` runs the guard (VERSION == tag), then the notes extraction into `$RUNNER_TEMP/notes.md` before `build`, and publishes with `--notes-file` instead of `--generate-notes`. `TestCIDoc` pins both | SC3 "CI dry run with a negative case", nothing tags or releases |
| D-18-12 | The real `go install @tag` end-to-end is not testable offline; covered by INST-07 `verify-published` in Phase 23 | accept, named in 18-01 threat model |

## 3. Plan map and waves

| Plan | Wave | Files | Requirement |
|------|------|-------|-------------|
| 18-01 version resolver | 1 | cmd/gruntled/version.go, version_test.go, main.go, render.go, instance.go, main_test.go | INST-01 |
| 18-02 VERSION + doc pins + release guard | 1 (parallel with 18-01: disjoint files) | VERSION, cmd/gruntled/version_doc_test.go, docs/ci.md, docs/cli.md, README.md, release.yml (guard step only), ci_doc_test.go (guard subtest) | INST-02 |
| 18-03 CHANGELOG + release notes | 2 (after 18-02: shares release.yml, ci_doc_test.go, docs/ci.md, VERSION) | CHANGELOG.md, scripts/release-notes.sh, cmd/gruntled/changelog_test.go, release.yml (notes + publish), ci.yml (dry-run step), ci_doc_test.go | HYG-03 |

18-02 writes the `docs/cli.md` version paragraph that 18-01 implements; both are consistent by
construction (18-02 pins `gruntled v0.4.0 (none)` as text; 18-01 makes the binary print it).
If executed in the same wave, each touches different lines of different files.

## 4. Environment
`export PATH=$HOME/.local/go/bin:$HOME/.local/bin:$HOME/go/bin:$PATH GOMAXPROCS=2 GOFLAGS=-p=2`.
No local `-race`. `bash scripts/check-architecture.sh` must stay green (Step 9 six targets; the
new import is `runtime/debug` and `regexp`, both std, neither links `net`).
