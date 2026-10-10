---
phase: 07-distribution-ci-integration
verified: 2026-10-01T00:00:00Z
status: passed
score: 4/4 must-haves verified
---

# Phase 7: Distribution & CI Integration Verification Report

**Phase Goal:** A team can install gruntled from a tagged release and wire it into pre-commit and CI by copying a documented snippet.
**Status:** passed. **Re-verification:** No.

## Observable Truths

| # | Truth | Status | Evidence |
|---|-------|--------|----------|
| 1 | `--version` prints version and commit, build-time injected | VERIFIED | `cmd/gruntled/main.go` has `version`/`commit` vars, prints `gruntled %s (%s)`. `scripts/build-release.sh` injects via `-ldflags -X main.version -X main.commit`. Downloaded v0.2.0 linux_amd64 binary prints `gruntled v0.2.0 (27cc60399795)`. |
| 2 | Release has 6 static targets plus checksums, and the no-net/no-exec proof covers them | VERIFIED | `gh release view v0.2.0` lists 6 archives (linux, darwin, windows x amd64, arm64) plus `checksums.txt`. The linux_amd64 checksum verified OK. `build-release.sh` sets `CGO_ENABLED=0`. `check-architecture.sh` Step 9 iterates all 6 release targets. `release.yml` re-runs it plus the tests before building. The release run is green. |
| 3 | One hook entry served by `.pre-commit-hooks.yaml`, fails on a broken unit | VERIFIED | `.pre-commit-hooks.yaml` defines `gruntled-check` (golang, `gruntled check`, always_run). The CI `recipe-check` job runs `pre-commit try-repo` and requires exit 0 on the clean fixture and 1 on the broken fixture. CI is green. |
| 4 | GitHub Actions recipe (check + SARIF) green in own CI, GitLab recipe documented | VERIFIED | `docs/ci.md` has sections 4 (GitHub variants 1 and 2, with pinned upload-sarif) and 5 (GitLab CI). CI jobs `recipe-check` and `sarif-upload` exercise the commands. The `ci` run on master is green (36865410093 and others). A `TestCIDoc` test keeps the doc and workflow in step. |

**Score:** 4/4

## Requirements Coverage

| Req | Plans | Status | Evidence |
|-----|-------|--------|----------|
| REL-01 | 07-02, 07-03, 07-05 | SATISFIED | 6 archives plus checksums on v0.2.0 |
| REL-02 | 07-01, 07-03, 07-05 | SATISFIED | ldflags injection, binary smoke |
| INT-03 | 07-04, 07-05 | SATISFIED | `.pre-commit-hooks.yaml`, try-repo CI proof |
| INT-04 | 07-04, 07-05 | SATISFIED | `docs/ci.md`, CI jobs |

No orphaned requirements: REQUIREMENTS.md maps exactly REL-01, REL-02, INT-03 and INT-04 to Phase 7, and all appear in plan frontmatter.

## Checks Run

- `go test -count=1 ./...` passes on all packages (without `-race`: no C compiler locally; CI runs `-race`).
- `scripts/test-check-architecture.sh` was not run, by instruction (passed at 07-02, unchanged since). It is run in CI `architecture` job.

## Anti-Patterns

None blocking found in the release workflow, CI workflow, hook file or build script.

## Notes (non-blocking)

- The `recipe-check` job installs from the local checkout (`go install ./cmd/gruntled`) rather than `@<tag>`. This stands in for the recipe's `go install ...@tag`. This is documented in the workflow.
- `sarif-upload` is master-push only, so it does not run on PRs.

## Human Verification

None required. The release and CI evidence is real and green.

_Verifier: Claude (gsd-verifier)_
