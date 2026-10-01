---
phase: 07-distribution-ci-integration
plan: 04
subsystem: ci
tags: [pre-commit, github-actions, gitlab-ci, sarif, docs, INT-03, INT-04]
requires:
  - "07-01: clean-fixture (exit 0), --version semantics (go install prints dev (none))"
  - "07-02: archive naming gruntled_<tag>_<os>_<arch>, 6 targets, checksums.txt"
provides:
  - ".pre-commit-hooks.yaml: hook gruntled-check (language golang, always_run, pass_filenames false)"
  - "docs/ci.md: install/checksums, exit codes, pre-commit, GitHub Actions (plain + SARIF), GitLab CI"
  - "ci.yml recipe-check job: recipe commands on clean/broken fixtures + pre-commit try-repo proof"
  - "TestCIDoc: drift guard between docs/ci.md, ci.yml and the hook file"
affects: [07-05 first push must show recipe-check green on master]
tech-stack:
  added: []
  patterns: ["doc-vs-workflow drift guard via plain string checks on job blocks (no YAML dep)"]
key-files:
  created:
    - .pre-commit-hooks.yaml
    - docs/ci.md
    - cmd/gruntled/ci_doc_test.go
  modified:
    - .github/workflows/ci.yml
    - README.md
decisions:
  - "docs/ci.md does not promise a version string for go install @tag: it states `gruntled dev (none)`; only release binaries carry `gruntled <tag> (<commit>)`"
  - "GitLab recipe uses golang:1.27 (tag verified on Docker Hub 2026-10-01, last_updated 2026-09-30)"
  - "recipe-check pins checkout/setup-go by SHA; existing jobs keep their @v7 tags untouched (plan: do not modify existing jobs)"
  - "TestCIDoc scopes pin checks to the job that uses them (recipe-check for checkout/setup-go, sarif-upload for upload-sarif)"
metrics:
  duration: 8min
  completed: 2026-10-01
  tasks: 3
  files: 5
---

# Phase 7 Plan 04: pre-commit hook, CI recipes and recipe-check Summary

Users now have a single `gruntled-check` pre-commit hook, served by `.pre-commit-hooks.yaml` and built from source (`language: golang`, runs every time). `docs/ci.md` holds user recipes for: release-binary download with `sha256sum -c --ignore-missing`, exit codes, pre-commit, GitHub Actions (plain and SARIF code scanning) and GitLab CI. A new `recipe-check` job in ci.yml runs those recipes for real. `TestCIDoc` fails if the doc's pins or commands drift from ci.yml or the hook file.

## Tasks

| # | Task | Commit |
|---|------|--------|
| 1 | `.pre-commit-hooks.yaml`, `docs/ci.md` (5 sections), README Further reading link | 2dc0032 |
| 2 (RED) | `cmd/gruntled/ci_doc_test.go` TestCIDoc. RED: recipe-check job missing (pins, recipe-check subtests); hook/doc subtests already green | 7774308 |
| 3 (GREEN) | ci.yml `recipe-check` job; TestCIDoc green; docs GOTOOLCHAIN note; README CI section | ba92378 |

## Verification

- `go test ./cmd/gruntled -run TestCIDoc -count=1 -v`: all 5 subtests pass.
- `go test -count=1 ./...` all green (no `-race`: no C compiler locally; CI runs `-race`). `go vet ./...`, `gofmt -l .` clean. `scripts/check-architecture.sh` OK. `test-check-architecture.sh` was not re-run, because check-architecture.sh did not change.
- actionlint v1.7.12 (`go run ...@v1.7.12`) reports nothing on ci.yml.
- I reproduced steps 1-4 of recipe-check locally with a `go install ./cmd/gruntled` binary on PATH: clean `check` rc=0, SARIF variant rc=0 (5144 bytes), broken fixture rc=1.
- **Step 5 (pre-commit) was verified locally** with `uvx --from pre-commit==4.6.2 pre-commit try-repo`. I ran it against a `--depth 1` clone of HEAD, which mimics the shallow CI checkout. Go 1.27 was on PATH and `GOTOOLCHAIN=local`, as CI sets it. The clean fixture gave `Passed` with rc=0. The broken fixture gave `Failed` with rc=1 and printed GRT001/GRT002/GRT003.
- Docker Hub `library/golang/tags/1.27` returns `name: 1.27`, so the GitLab snippet uses `golang:1.27`.
- The first real `recipe-check` run on GitHub (`pipx install` on the runner, `$GITHUB_PATH`) happens after the push in 07-05.

## Deviations from Plan

**1. [Rule 1 - Doc accuracy] GOTOOLCHAIN=local caveat in the pre-commit section**
- **Found during:** Task 3 local try-repo run
- **Issue:** This machine's system Go is 1.24.1 and normally auto-switches to 1.27. With `GOTOOLCHAIN=local` exported, pre-commit's `go install ./...` failed with `go.mod requires go >= 1.27`. With the default (`auto`), the same run passed.
- **Fix:** docs/ci.md now says that an older Go plus `GOTOOLCHAIN=local` fails with that message. CI is not affected because setup-go installs 1.27 from go.mod first.
- **Commit:** ba92378

**2. [Rule 2] README CI section lists recipe-check**
- The CI section said "two independent jobs". It now says "independent jobs" and has a `recipe-check` bullet. The Further reading link was added as the plan asked.

**3. [Plan note] go install version wording**
- The plan text said that `go install` "reports module version". Per 07-01 (no ReadBuildInfo fallback), docs/ci.md instead says that `go install` builds print `gruntled dev (none)`.

## Self-Check: PASSED
