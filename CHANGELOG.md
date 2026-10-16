# Changelog

All notable changes to gruntled are documented here. The format is
[Keep a Changelog 1.1.0](https://keepachangelog.com/en/1.1.0/) and the project
follows [Semantic Versioning](https://semver.org/spec/v2.0.0.html). The newest
released section always equals the `VERSION` file; both are bumped in the
commit that is tagged, and `release.yml` publishes the section matching the tag
as the release notes (and fails when it is missing).

## [Unreleased]

### Added
- `VERSION` file as the single version source; `docs/ci.md`, the README install
  snippet and the pre-commit `rev` are checked against it by a test;
  `release.yml` refuses a tag that differs from it
- This changelog; `release.yml` publishes the matching section as the release
  notes and fails when the section is missing

### Changed
- `gruntled --version`, the SARIF `tool.driver.version` and the daemon's
  advertised version resolve the tag of a
  `go install github.com/GiulioSavini/gruntled/cmd/gruntled@vX.Y.Z` build
  (`gruntled vX.Y.Z (none)`); release builds keep the ldflags version and
  commit; a plain `go build` keeps `gruntled dev (none)`

## [0.4.0] - 2026-10-13

### Added
- `GRT004` replaces `GRT001` at every reference to an output a change removed,
  through one shared blast core (`Between`) reused by `check`, `report`,
  `blast` and the daemon
- Transitive Impacted follows dependents past one hop, with a distance, one
  connecting path and `--depth`
- Type-level surface facts: new required variables and variable/output type or
  `sensitive` changes put instantiating units in Impacted, never Broken
- The daemon answers `report --blast` against its in-memory baseline over
  IPC v2, and `rebase [--expect N]` moves that baseline, which holds only
  `checking.Report`, never a loader or parse cache
- Corpus mutation proof: zero noise on three real corpora, 100% catch of the
  injected mutation set (4,345 mutations), network-isolated against a frozen,
  fingerprinted v0.3.0 reference binary

### Changed
- `check` stays byte-identical to v0.3.0
- A 64 MiB blast-response size bound and terminal/bidi escaping cover every new
  surface
- Known limitations: `report`/`rebase` unsupported over AF_UNIX on windows; the
  include-target conservative rule hides all 8 known broken references on the
  denis256 secondary corpus; `GRT005`/`GRT006` deferred

## [0.3.0] - 2026-10-10

### Added
- `gruntled blast --base <dir>`: disjoint, sorted Broken and Impacted sets
  (one-hop `variable`/`output` surface changes), text and json, multiset
  baseline so duplicate findings are not hidden
- `gruntled watch`: fsnotify (or `--poll`, and windows) with ~150 ms debounce,
  single indexer goroutine, 30 s safety net, atomic one-line status file at a
  per-repository path outside the repository
- `gruntled report` over a raw-syscall AF_UNIX socket (report file on windows),
  byte-identical to `check`; crash-safe single instance via lock + stale socket
  recovery
- Incremental index: a persistent parse store with `Invalidate`, proven equal
  to a full rescan by a stateful property test over an in-memory filesystem

### Changed
- Every text and JSON output escapes terminal control and bidi runes from
  repository names; the no-net/no-exec binary proof still holds on all six
  targets with watcher and socket linked
- Known limitations: darwin native watcher delays new files after a dangling
  symlink by up to 30 s (fsnotify#787); 64 MiB report response cap; Windows
  runtime dir relies on `%LocalAppData%` ACLs

### Fixed
- Incremental index equality for editor-pattern directory names and symlinked
  includes (found by the milestone audit, closed in phase 12)

## [0.2.0] - 2026-10-08

### Added
- `GRT002` (dependency to no unit) and `GRT003` (dependency cycle) as pure
  graph analyzers, validated on the pinned corpus and confirmed against
  terragrunt v1.1.6 and an independent oracle
- `gruntled graph --json`: one deterministic document with schema version,
  units, modules, edges and unresolved dependencies, all paths
  repository-relative
- `gruntled check --format sarif`: SARIF 2.1.0 with one rule per `GRT` code,
  validated in CI against the vendored OASIS schema and accepted by GitHub code
  scanning
- `gruntled-check` pre-commit hook and `docs/ci.md` recipes (release download,
  pre-commit, GitHub Actions plain and SARIF, GitLab CI), proven by a
  `recipe-check` CI job
- Release build: static binaries for 6 targets (linux/darwin/windows x
  amd64/arm64) with `checksums.txt`, `gruntled --version` injected at build
  time, tag-triggered `release.yml`

## [0.1.0] - 2026-09-29

### Added
- `gruntled check` with `GRT001` and `GRT100` on an in-house structural HCL
  decoder; validated on the real corpus with 0 false positives and every
  injected mutation caught

[Unreleased]: https://github.com/GiulioSavini/gruntled/compare/v0.4.0...HEAD
[0.4.0]: https://github.com/GiulioSavini/gruntled/compare/v0.3.0...v0.4.0
[0.3.0]: https://github.com/GiulioSavini/gruntled/compare/v0.2.0...v0.3.0
[0.2.0]: https://github.com/GiulioSavini/gruntled/compare/v0.1.0...v0.2.0
[0.1.0]: https://github.com/GiulioSavini/gruntled/releases/tag/v0.1.0
