---
phase: 7
slug: distribution-ci-integration
status: draft
nyquist_compliant: false
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
| TBD | TBD | TBD | REL-02 | unit | `go test ./cmd/gruntled -run TestVersion -count=1` | ❌ W0 | ⬜ pending |
| TBD | TBD | TBD | REL-02 | script | `go build -ldflags "-X main.version=v9.9.9 -X main.commit=abc1234" ... && --version` matches `gruntled v9.9.9 (abc1234)` | ❌ W0 | ⬜ pending |
| TBD | TBD | TBD | REL-01 | script | `bash scripts/build-release.sh v0.0.0-ci abc1234 $T` (6 archives + checksums.txt, `sha256sum -c`) | ❌ W0 | ⬜ pending |
| TBD | TBD | TBD | REL-01 | existing | `bash scripts/check-architecture.sh` (no-net/no-exec on all 6 targets) | ✅ | ⬜ pending |
| TBD | TBD | TBD | INT-03 | CI integration | `pre-commit try-repo` on clean (pass) and broken (fail) fixtures | ❌ W0 | ⬜ pending |
| TBD | TBD | TBD | INT-04 | CI | `recipe-check` job: clean exit 0, broken exit exactly 1 | ❌ W0 | ⬜ pending |

*Status: ⬜ pending · ✅ green · ❌ red · ⚠️ flaky*

---

## Wave 0 Requirements

- [ ] `cmd/gruntled/main_test.go` — TestVersion cases
- [ ] `cmd/gruntled/testdata/clean-fixture/` — exit 0, not picked up by golden/corpus globs
- [ ] `scripts/build-release.sh` — 6-target packaging + checksums

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
