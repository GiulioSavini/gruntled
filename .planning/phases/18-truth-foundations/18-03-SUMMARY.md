---
phase: 18-truth-foundations
plan: 03
subsystem: release
tags: [changelog, release-notes, release-workflow, ci]
requires: [18-02]
provides:
  - "CHANGELOG.md in Keep a Changelog form, backfilled v0.1.0..v0.4.0 with the MILESTONES dates"
  - "scripts/release-notes.sh <tag> [file]: prints the section for the tag, exit 1 if absent/empty, exit 2 on bad usage or tag"
  - "release.yml `notes` step and `gh release create --notes-file`; ci.yml `release-notes dry run`"
  - "TestChangelog, TestChangelogNegative, TestReleaseNotesScript, TestCIDoc rows for both workflows"
affects: [24-hygiene, 25-release]
tech-stack:
  added: []
  patterns: ["tag treated as data: regex-validated, then fixed-length substring compare", "release notes are reviewed text in the tagged commit"]
key-files:
  created:
    - CHANGELOG.md
    - scripts/release-notes.sh
    - cmd/gruntled/changelog_test.go
  modified:
    - cmd/gruntled/ci_doc_test.go
    - .github/workflows/release.yml
    - .github/workflows/ci.yml
    - docs/ci.md
key-decisions:
  - "Section header compared as the literal prefix `## [X.Y.Z] - ` (bracket and dash included), so v1.2.3 cannot match [1.2.3-rc.1] or [11.2.3]"
  - "No section means no release: the `notes` step runs right after `guard`, before proofs and build"
  - "ci.yml proves on every run that the VERSION section extracts and that a missing section exits exactly 1"
requirements-completed: [HYG-03]
duration: n/a
completed: 2026-10-17
---

# Phase 18 Plan 03: CHANGELOG and release notes Summary

**`CHANGELOG.md` (v0.1.0..v0.4.0 plus Unreleased) is now the source of the GitHub release notes. `scripts/release-notes.sh` extracts the section for a tag with the tag treated as data only; release.yml fails before building if the section is missing and publishes it with `--notes-file` instead of `--generate-notes`; ci.yml dry-runs both the positive and the negative extraction on every push without creating a tag or release.**

## Accomplishments

- **CHANGELOG.md** (2263e61): backfilled from MILESTONES.md bullets (process items excluded), newest == VERSION enforced by `TestChangelog` (changelog_test.go:223) and shape negatives in `TestChangelogNegative` (:237).
- **scripts/release-notes.sh** (96df6c4): `LC_ALL=C` (:16), tag validated against the guard regex first, exit 2 (:24-26); section found by `${line:0:${#want}}` = `$want` (:42), never by case/grep/awk on the tag; body trimmed (:49); empty or absent -> `release-notes: no section [X.Y.Z] in <file>`, exit 1 (:50-52).
- **TestReleaseNotesScript** (changelog_test.go:308): real changelog with the VERSION tag, prerelease vs release prefix, `*`, `$(touch x)`, `OR 1=1` tags with no file created, empty body, usage errors, missing file, `v0.0.0-nosuch`, malformed tag.
- **release.yml** (a3578cc): `notes` step after `guard` (`set -o pipefail`, `bash scripts/release-notes.sh "$GITHUB_REF_NAME" > "$RUNNER_TEMP/notes.md"`, `test -s`); publish uses `--notes-file "$RUNNER_TEMP/notes.md" --verify-tag`.
- **ci.yml** `release-notes dry run`: extracts the VERSION section, asserts `v0.0.0-nosuch` exits exactly 1 with empty stdout and the no-section message.
- **TestCIDoc** (ci_doc_test.go:310 "release notes", :343 "release-notes dry run") pins the exact step text, order, and absence of `secrets.`; the 18-02 scanner still proves no `${{` in any run block.
- **docs/ci.md** describes the notes behaviour.

## Task Commits

1. `ef7c047` test(18-03): add failing CHANGELOG shape test
2. `2263e61` docs(18-03): add CHANGELOG.md backfilled v0.1.0..v0.4.0
3. `801c9ec` test(18-03): add failing tests for release-notes.sh
4. `96df6c4` feat(18-03): add scripts/release-notes.sh
5. `06911f1` test(18-03): pin release notes extraction in release.yml and the ci.yml dry run
6. `a3578cc` ci(18-03): publish CHANGELOG section as release notes; dry-run extraction in ci

## Test evidence (master at 636d601)

```
go vet ./...                                   ok
go test -count=1 -short ./cmd/gruntled/        ok  github.com/GiulioSavini/gruntled/cmd/gruntled  20.492s
bash scripts/release-notes.sh v0.4.0 | head -3 (first lines of the v0.4.0 section: "### Added" / "- `GRT004` replaces ...")
bash scripts/release-notes.sh v0.0.0-nosuch    stderr: release-notes: no section [0.0.0-nosuch] in CHANGELOG.md   rc=1
go list -deps ./cmd/gruntled | grep -E '^net$|os/exec|text/template'    (empty)
bash scripts/check-architecture.sh             architecture: OK (4 domain packages, 5 application packages, 1 interfaces packages)
```

`scripts/test-check-architecture.sh` (the mutation self-test) is not fast (it rebuilds the binary per case; over 15 minutes locally) and had not finished when this summary was written; CI runs it. `go test -race` could not run locally (no gcc/cgo); CI's race job covers it.

## Deviations

- None from the plan. Sec findings applied: #485 (negative dry run, exit 1 exactly), #488 (tag as data, fixed-length compare, hostile-tag rows), #489 (`LC_ALL=C`).

## Threat mitigations

| ID | Disposition | Where |
|----|-------------|-------|
| T-18-16 tag as data | mitigate | release-notes.sh:24-26 (regex, exit 2), :42 (fixed-length compare); TestReleaseNotesScript hostile-tag rows (changelog_test.go:308+); release.yml passes the tag only as quoted `$GITHUB_REF_NAME` |
| T-18-17 expression injection | mitigate | new steps use `$GITHUB_REF_NAME`/`$RUNNER_TEMP` only; 18-02 scanner (ci_doc_test.go:276) over release.yml and ci.yml |
| T-18-18 prefix match | mitigate | release-notes.sh:42 compares `## [X.Y.Z] - ` including bracket and dash; prerelease row in TestReleaseNotesScript |
| T-18-20 unreviewed notes | mitigate | `--notes-file` of the checked-in section, `--verify-tag` kept (release.yml publish step) |
| T-18-22 test vacuity | mitigate | TestChangelog requires 0.1.0..0.4.0 with MILESTONES dates, newest == VERSION; ci.yml negative branch and TestCIDoc pin its text (ci_doc_test.go:343) |
| T-18-23 permissions | mitigate | both steps are local shell; no permission change; TestCIDoc asserts no `secrets.` in the step |
| T-18-19, -21 | accept | as planned |

## Open items

- A tag pushed before its CHANGELOG section exists fails the release by design (T-18-19); the fix is a commit then a new tag.
- `v0.4.0` itself is still untagged (see 18-02).
