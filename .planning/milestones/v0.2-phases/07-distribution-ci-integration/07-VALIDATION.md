---
phase: 7
slug: distribution-ci-integration
status: approved
nyquist_compliant: true
wave_0_complete: false
created: 2026-10-01
---

# Phase 7 — Validation Strategy

> Per-phase validation contract for feedback sampling during execution.

---

## Test Infrastructure

| Property | Value |
|----------|-------|
| **Framework** | Go `testing` + testscript, bash scripts, GitHub Actions |
| **Config file** | none (`go.mod` go 1.27) |
| **Quick run command** | `go test ./cmd/gruntled -count=1` |
| **Full suite command** | `go test -race -count=1 ./... && bash scripts/check-architecture.sh && bash scripts/test-check-architecture.sh` |
| **Estimated runtime** | ~90 seconds |

---

## Sampling Rate

- **After every task commit:** Run `go test ./cmd/gruntled -count=1` (plus `bash scripts/build-release.sh ...` for script tasks)
- **After every plan wave:** Run full suite command
- **Before `/gsd:verify-work`:** Full suite green and ci.yml green on master (including `recipe-check`)
- **Max feedback latency:** 120 seconds

---

## Per-Task Verification Map

| Task ID | Plan | Wave | Requirement | Test Type | Automated Command | File Exists | Status |
|---------|------|------|-------------|-----------|-------------------|-------------|--------|
| 07-01-T1 | 01 | 1 | REL-02 | unit + ldflags | `go test ./cmd/gruntled -run 'TestVersion|Sarif' -count=1` (TestVersionLdflags builds with `-X main.version=v9.9.9 -X main.commit=abc1234`, expects `gruntled v9.9.9 (abc1234)`) | ❌ W0 | ⬜ pending |
| 07-01-T2 | 01 | 1 | REL-02 | unit | `go test -race -count=1 ./...` (fixture_test.go: clean-fixture exit 0, sarif-fixture exit 1) | ❌ W0 | ⬜ pending |
| 07-02-T1 | 02 | 2 | REL-01 | existing + self-test | `bash scripts/check-architecture.sh && bash scripts/test-check-architecture.sh` (no-net/no-exec on all 6 targets; case binary-exec-windows-arm64-file) | ✅ | ⬜ pending |
| 07-02-T2 | 02 | 2 | REL-01 | script + unit | `go test ./cmd/gruntled -run 'TestReleaseTargetsInSync|TestBuildRelease' -count=1` (6 archives + checksums.txt, `sha256sum -c`) | ❌ W0 | ⬜ pending |
| 07-02-T3 | 02 | 2 | REL-01 | CI static | `grep build-release.sh .github/workflows/ci.yml` + actionlint | ✅ | ⬜ pending |
| 07-03-T1 | 03 | 3 | REL-01, REL-02 | static | actionlint on release.yml + all `uses:` SHA-pinned | ❌ W0 | ⬜ pending |
| 07-04-T1 | 04 | 3 | INT-03, INT-04 | static | hook file + docs/ci.md present, README pointer | ❌ W0 | ⬜ pending |
| 07-04-T2 | 04 | 3 | INT-03, INT-04 | unit | `go test ./cmd/gruntled -run TestCIDoc -count=1` | ❌ W0 | ⬜ pending |
| 07-04-T3 | 04 | 3 | INT-03, INT-04 | CI integration | `recipe-check` job: clean exit 0, broken exit exactly 1, `pre-commit try-repo` pass/fail | ❌ W0 | ⬜ pending |
| 07-05-T1 | 05 | 4 | all | full suite | `go test -race -count=1 ./... && bash scripts/check-architecture.sh && bash scripts/test-check-architecture.sh` | ✅ | ⬜ pending |
| 07-05-T3 | 05 | 4 | REL-01, REL-02 | manual (user-approved push) | `gh release view <tag>`, `sha256sum -c checksums.txt`, downloaded `--version` | n/a | ⬜ pending |

*Status: ⬜ pending · ✅ green · ❌ red · ⚠️ flaky*

---

## Wave 0 Requirements

- [ ] `cmd/gruntled/main_test.go` — TestVersion cases
- [ ] `cmd/gruntled/testdata/clean-fixture/` — exit 0, not picked up by golden/corpus globs
- [ ] `scripts/build-release.sh` — 6-target packaging + checksums (07-02-T2)

---

## Manual-Only Verifications

| Behavior | Requirement | Why Manual | Test Instructions |
|----------|-------------|------------|-------------------|
| Real tag produces GitHub release with assets | REL-01 | Tag push is externally visible, needs user approval | Push tag, `gh release view <tag>`, download, `sha256sum -c checksums.txt` |
| GitLab CI recipe documented | INT-04 | No GitLab runner, by decision | Review `docs/ci.md` GitLab section |

---

## Validation Sign-Off

- [ ] All tasks have `<automated>` verify or Wave 0 dependencies
- [ ] Sampling continuity: no 3 consecutive tasks without automated verify
- [ ] Wave 0 covers all MISSING references
- [ ] No watch-mode flags
- [ ] Feedback latency < 120s
- [ ] `nyquist_compliant: true` set in frontmatter

**Approval:** pending
