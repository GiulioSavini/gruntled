---
phase: 5
slug: graph-diagnostics
status: draft
nyquist_compliant: false
wave_0_complete: false
created: 2026-09-29
---

# Phase 5 — Validation Strategy

> Per-phase validation contract for feedback sampling during execution.

---

## Test Infrastructure

| Property | Value |
|----------|-------|
| **Framework** | Go `testing` (go 1.27.0), testscript/txtar goldens, synthrepo |
| **Config file** | none (env gates) |
| **Quick run command** | `go test -count=1 ./internal/domain/... ./internal/application/... ./internal/infrastructure/terragrunt/... && go test -count=1 ./cmd/gruntled -run 'TestGolden\|TestValidationDocPins'` |
| **Full suite command** | `test -z "$("$(go env GOROOT)/bin/gofmt" -l .)" && go mod tidy -diff && go vet ./... && GOTOOLCHAIN=go1.27.0 go run honnef.co/go/tools/cmd/staticcheck@v0.8.1 ./... && go test -count=1 ./... && bash scripts/check-architecture.sh && bash scripts/test-check-architecture.sh` |
| **Corpus command (env-gated)** | `GRUNTLED_CORPUS=$HOME/.cache/gruntled-phase4/corpus/primary GRUNTLED_CORPUS_SECRET=$HOME/.cache/gruntled-phase4/corpus/secret GRUNTLED_CORPUS_DENIS256=$HOME/.cache/gruntled-phase4/corpus/denis256 GRUNTLED_TERRAGRUNT_BIN=$HOME/.cache/gruntled-phase4/bin/terragrunt_linux_amd64 go test -count=1 -v -run 'TestCorpus\|TestDenis256\|TestSecret' ./cmd/gruntled` |
| **Estimated runtime** | ~60 seconds (full suite, excluding corpus) |

---

## Sampling Rate

- **After every task commit:** Run quick run command
- **After every plan wave:** Run full suite command
- **Before `/gsd:verify-work`:** Full suite green plus corpus command recorded in `docs/validation.md`
- **Max feedback latency:** 60 seconds

---

## Per-Task Verification Map

| Req | Behavior | Test Type | Automated Command | File Exists | Status |
|-----|----------|-----------|-------------------|-------------|--------|
| MORE-01 | GRT002 table rows (missing, no-config, has-config, unknown, disabled, non-literal, paths entry, skip_outputs) | unit | `go test ./internal/domain/analysis -run TestMissingTargets` | ❌ W0 | ⬜ pending |
| MORE-01 | Loader target classification (missing/empty/case-mismatch/dangling symlink/EACCES/parent-file) | unit | `go test ./internal/infrastructure/terragrunt -run 'TestTargetState\|TestPathDependencies'` | ❌ W0 | ⬜ pending |
| MORE-01 | `dependencies` merge = union, no_merge dropped, invalid block drops only path edges | unit | `go test ./internal/infrastructure/terragrunt -run TestPathDepsMerge` | ❌ W0 | ⬜ pending |
| MORE-01 | Goldens: `dependency_edges` updated, new `grt002_*.txtar` | golden | `go test ./cmd/gruntled -run TestGolden` | ❌ W0 | ⬜ pending |
| MORE-02 | Self-loop, 2-ring, 3-ring, non-ring SCC, dedup block+paths, disabled edge dropped, unknown-config invisible | unit | `go test ./internal/domain/analysis -run TestDependencyCycles` | ❌ W0 | ⬜ pending |
| MORE-02 | Permutation invariance, 10k-unit chain no overflow, repeated runs byte-identical | unit | `go test ./internal/domain/analysis -run 'TestCyclesDeterministic\|TestCyclesDeepChain'` | ❌ W0 | ⬜ pending |
| MORE-02 | `Edges()` deterministic and complete | unit | `go test ./internal/domain/repograph -run TestEdges` | ❌ W0 | ⬜ pending |
| MORE-01/02 | `Check` wires GRT001+GRT002+GRT003 into one Set | unit | `go test ./internal/application/checking` | extend | ⬜ pending |
| MORE-06 | Unmutated corpus: iso20022 + secret zero GRT002/GRT003; denis256 == exact oracle set | corpus | corpus command | extend | ⬜ pending |
| MORE-06 | Mutations each add exactly expected diagnostic, exit 1, GRT001 unchanged, revert byte-identical | corpus | corpus command | ❌ W0 | ⬜ pending |
| MORE-06 | `docs/validation.md` v0.2 sections and pins | doc | `go test ./cmd/gruntled -run TestValidationDocPins` | extend | ⬜ pending |
| ARCH | domain/application allowlists hold | script | `bash scripts/check-architecture.sh` | ✅ | ⬜ pending |

*Status: ⬜ pending · ✅ green · ❌ red · ⚠️ flaky*

---

## Wave 0 Requirements

- [ ] `internal/domain/analysis/grt002_test.go`, `grt003_test.go` (graph builders with path positions)
- [ ] `internal/domain/repograph` tests for new enum/constructors/`Edges()`
- [ ] `internal/infrastructure/terragrunt` tests for target state and `dependencies` parse/merge (`fstest.MapFS` + real-FS symlink case in `_unix_test.go`)
- [ ] Spike: `terragrunt run --all --non-interactive -- version` oracle on scratch copies of iso20022 and secret; record exact outputs
- [ ] Secret corpus pin const + env gate; new `corpusMutation` entries with `Old`/`New` exact-occurrence pins

---

## Manual-Only Verifications

| Behavior | Requirement | Why Manual | Test Instructions |
|----------|-------------|------------|-------------------|
| Corpus run on local checkouts | MORE-06 | Corpora are not in repo, env-gated | Run corpus command, paste results into `docs/validation.md` |

---

## Validation Sign-Off

- [ ] All tasks have `<automated>` verify or Wave 0 dependencies
- [ ] Sampling continuity: no 3 consecutive tasks without automated verify
- [ ] Wave 0 covers all MISSING references
- [ ] No watch-mode flags
- [ ] Feedback latency < 60s
- [ ] `nyquist_compliant: true` set in frontmatter

**Approval:** pending
