---
phase: 18-truth-foundations
verified: 2026-10-17T00:00:00Z
status: passed
score: 7/7 must-haves verified (4 roadmap success criteria + INST-01, INST-02, HYG-03)
covered_files: [".github/workflows/ci.yml", ".github/workflows/release.yml", ".planning/phases/18-truth-foundations/18-01-PLAN.md", ".planning/phases/18-truth-foundations/18-01-SUMMARY.md", ".planning/phases/18-truth-foundations/18-02-PLAN.md", ".planning/phases/18-truth-foundations/18-02-SUMMARY.md", ".planning/phases/18-truth-foundations/18-03-PLAN.md", ".planning/phases/18-truth-foundations/18-03-SUMMARY.md", "CHANGELOG.md", "README.md", "VERSION", "cmd/gruntled/changelog_test.go", "cmd/gruntled/ci_doc_test.go", "cmd/gruntled/instance.go", "cmd/gruntled/main.go", "cmd/gruntled/render.go", "cmd/gruntled/version.go", "cmd/gruntled/version_doc_test.go", "cmd/gruntled/version_test.go", "docs/ci.md", "docs/cli.md", "scripts/release-notes.sh"]
covered_digest: "v3:sha256:20965e510e253c3e7aaec240db78d2e5fc25f51a5d688f8d695be1e9e7e70c51"
behavior_unverified: 0
overrides_applied: 0
---

# Phase 18: Truth Foundations Verification Report

**Phase goal:** User gets the same, correct version string from `--version`, SARIF and the daemon however gruntled was installed, and every install snippet and release note in the repository comes from one source of truth, so no later install channel can ship a stale pin.
**Verified:** 2026-10-17, code at master `506ff51`
**Status:** passed
**Re-verification:** No. First verification.

Method: read `version.go`, the three call sites, the docs, `release.yml`, `ci.yml`, `release-notes.sh`, and ran the commands below. SUMMARY claims were not used as evidence. Not run: `scripts/test-check-architecture.sh` (slow, CI-only); `go test -race` (no cgo locally, CI covers it).

## Goal Achievement

### Observable Truths (roadmap success criteria)

| # | Truth | Status | Evidence |
|---|-------|--------|----------|
| 1 | `go install ...@vX.Y.Z` prints `gruntled vX.Y.Z (none)`; release build keeps ldflags version and commit; plain `go build`, `(devel)`, `+dirty`, pseudo-version, any `vcs.revision` print `gruntled dev (none)`; the same string is SARIF `tool.driver.version` and `ipc.Info` version (table test over an injected reader). | VERIFIED | `version.go:59-90`: `versionInfo()` reads the swappable `readBuildInfo` and calls pure `resolveVersion`. ldflags win when version != `dev` (:67); otherwise build info is accepted only for this module path, release regexp, length <= 64, no `+`, no pseudo-version, no `vcs`/`vcs.*` setting, and the commit is the literal `none`. Call sites: `main.go:154` (`--version`), `render.go:24` (SARIF), `instance.go:227` (`ipc.Info`); no other reader of `version`/`commit`. Ran: `/tmp/g --version` = `gruntled dev (none)`; ldflags build `-X main.version=v9.9.9 -X main.commit=abcdef123456` = `gruntled v9.9.9 (abcdef123456)`. Tests PASS: `TestResolveVersion` (28-row table), `TestResolveVersionIsPure`, `TestVersionInfoFeedsAllSites`, `TestVersionBuildInfoNoLeak`, `TestVersionSARIF`, `TestVersion`, and with the real toolchain (non-short) `TestVersionLdflags`, `TestVersionPlainBuild`. The real `go install @tag` path is proven through the injected build-info reader only (a true module-proxy install needs a published tag, which does not exist yet). |
| 2 | One `VERSION` source feeds `docs/ci.md`, README install snippets and pre-commit `rev`; a doc-pin test fails on drift (negative case); `docs/ci.md` no longer pins v0.2.0; release guard fails when `VERSION` != tag. | VERIFIED | `cat VERSION` = `v0.4.0`. `grep -rn 'v0.2.0' docs README.md` = empty. `version_doc_test.go`: `checkVersionPins` over a fixed (file, pattern) table plus an all-tokens rule on docs/ci.md and non-vacuity failures; `TestVersionPins` PASS, `TestVersionPinsNegative` PASS (drifted pin, drifted README, deleted pattern, missing file, wrong VERSION, prose-token drift, foreign action-comment tokens, docs/cli.md and README drift per sec #510). `release.yml:50-54` compares `VERSION` to `$GITHUB_REF_NAME` before fetch/proof/build and exits 1; `TestCIDoc` pins text and order and that no `${{` is inside a run block. Sec #510 (docs/cli.md tokens outside pin rule) is fixed: test lines 83/145/256. |
| 3 | `CHANGELOG.md` is Keep a Changelog 1.1.0, v0.1.0..v0.4.0 backfilled from `MILESTONES.md` plus `Unreleased`; `release.yml` uses the matching section as release notes and fails for a tag with no section (CI dry run with a negative case). | VERIFIED | `CHANGELOG.md` headings: `[Unreleased]`, `[0.4.0] - 2026-10-13`, `[0.3.0]`, `[0.2.0]`, `[0.1.0]`. `TestChangelog`/`TestChangelogNegative` PASS (dates checked against MILESTONES.md, newest == VERSION). Ran: `bash scripts/release-notes.sh v0.4.0 | head -2` = `### Added` / the GRT004 line; `bash scripts/release-notes.sh v0.0.0-nosuch` = `release-notes: no section [0.0.0-nosuch] in CHANGELOG.md`, rc=1. `release.yml:67` runs the script with `pipefail` right after `guard` (before proofs and build); `:108` publishes with `--notes-file ... --verify-tag` (no `--generate-notes`). `ci.yml:93-112` dry-runs the positive extraction and asserts the negative exits exactly 1 with the exact stderr. `TestReleaseNotesScript` PASS (hostile tags treated as data, no file created). |
| 4 | `check` text/JSON/SARIF byte-identical to v0.4 goldens; six-target no-net/no-exec proof passes with the build-info reader linked; the "`go install` prints `dev (none)`" limitation is deleted from `docs/cli.md` and `docs/ci.md`. | VERIFIED | `git diff --stat ccd76c1..HEAD -- cmd/gruntled/testdata` is empty and no golden/testdata file appears in `git diff --name-only ccd76c1..HEAD`; `TestGoldenFixtures`, `TestGoldenSynthrepoFullScale`, `TestSARIFStructure*` PASS. `bash scripts/check-architecture.sh` = `architecture: OK` (Step 9 six-target binary proof included). `go list -deps ./cmd/gruntled` for all six release targets: no `net`, `os/exec`, `text/template`. `docs/ci.md:28-32` now says `go install` prints `gruntled v0.4.0 (none)` and only a plain `go build` prints `dev (none)`; `docs/cli.md:85-100` documents the three shapes. |

**Score:** 4/4 success criteria verified, 0 behavior-unverified.

Gate commands: `go vet ./... && go test -count=1 -short ./cmd/gruntled/` = ok (30.7 s).

### Requirements Coverage

| Requirement | Source Plan | Status | Evidence |
|-------------|-------------|--------|----------|
| INST-01 | 18-01 | SATISFIED | SC1. Commit placeholder `none` documented in docs/cli.md and docs/ci.md. |
| INST-02 | 18-02 | SATISFIED | SC2. REQUIREMENTS also lists install-script and image/formula snippets; none exist yet, and the pin table is built for Phase 23 to add rows. |
| HYG-03 | 18-03 | SATISFIED | SC3. |

No orphaned requirements: REQUIREMENTS.md maps only INST-01, INST-02 and HYG-03 to Phase 18, and each is claimed by a plan.

### Anti-Patterns Found

None in the phase's production files (`version.go`, `release-notes.sh`, workflows). No TBD/FIXME/XXX markers introduced.

### Gaps Summary

No gaps. Notes (non-blocking):

- REQUIREMENTS.md and ROADMAP.md still show these requirements and plans as unchecked/"Pending"; the orchestrator should tick them on phase close.
- `scripts/test-check-architecture.sh` and `go test -race` were not run locally; CI covers both.
- The real `go install github.com/GiulioSavini/gruntled/cmd/gruntled@v0.4.0` can only be observed after the tag is published (Phase 25); until then the behavior rests on the injected build-info table test.

---

_Verified: 2026-10-17_
_Verifier: Claude (gsd-verifier)_
