---
phase: 01-domain-foundation-test-substrate
verified: 2026-09-25T10:55:10Z
status: passed
score: 4/4 must-haves verified
---

# Phase 1: Domain Foundation & Test Substrate Verification Report

**Phase Goal:** The domain layer (`Unit`, `Module`, `Surface`, `Reference`, `RepositoryGraph`, `Diagnostic`) exists, is provably free of HCL/filesystem imports, and a deterministic synthetic-repository generator exists so every subsequent phase has real fixtures instead of ad hoc HCL snippets.
**Verified:** 2026-09-25T10:55:10Z
**Status:** passed
**Re-verification:** No — initial verification

## Goal Achievement

### Observable Truths (ROADMAP Success Criteria)

| # | Truth | Status | Evidence |
|---|-------|--------|----------|
| 1 | The domain package's dependency list contains no HCL, filesystem, or infrastructure import, enforced by an automated CI check that fails the build if one appears | ✓ VERIFIED | `scripts/check-architecture.sh` (allowlist of pure stdlib packages, platform-neutral gate, external-deps gate, binary-links-testsupport gate); `scripts/test-check-architecture.sh` proves 13 distinct failure modes all genuinely fail (`clean`, `direct-os`, `xtest-os`, `fmt`, `log`, `unsafe`, `windows-file`, `build-tag`, `tagged-test`, `external-dep`, `broken-code`, `testsupport-linked`, `vacuous` — all PASS); CI job `architecture` runs both scripts on every push/PR, green on commit `a459430` |
| 2 | Analyzers and graph queries can be exercised in unit tests using hand-built domain objects only — no filesystem access, no HCL parsing | ✓ VERIFIED | `internal/domain/repograph/graph_test.go` builds a test-local `unknownOutputRefs` function chaining only `References -> DependencyTarget -> ModuleOf -> Surface().HasOutput` over a hand-built 3-unit graph, no filesystem, no HCL (`grep -n HasOutput` confirms line 288) |
| 3 | Given a `Spec` (unit count, include nesting depth, dependency fanout, seed), the generator produces a Terragrunt repository tree deterministically — the same `Spec` and seed produce a byte-identical tree on repeated runs | ✓ VERIFIED | `internal/testsupport/synthrepo/render_test.go::TestRender_PinnedDigest` (hard-coded SHA-256 digest, passes); `generate_test.go::TestGenerate_Deterministic` (two independent `t.TempDir()` runs byte-identical); ran `go test -count=3 ./...` locally, all packages pass repeatedly with no flakiness |
| 4 | The generator can inject at least one known error kind (e.g. a bad output reference) and return a manifest describing the diagnostics a correct analyzer should report against the generated tree | ✓ VERIFIED | `synthrepo.ErrorKind.BadOutputRef` injected via `Spec.Errors`; `generate_test.go::TestGenerate_ManifestIsExactOracle` independently re-scans generated HCL with a local regexp-based scanner and proves the `Manifest.Expected` set is exactly the broken references, no more, no less |

**Score:** 4/4 truths verified

### Required Artifacts

| Artifact | Expected | Status | Details |
|----------|----------|--------|---------|
| `go.mod` | module github.com/GiulioSavini/gruntled, go 1.27, no require block | ✓ VERIFIED | Present, exactly as specified, `go version` reports go1.27.0 |
| `cmd/gruntled/main.go` | composition-root stub | ✓ VERIFIED | Exists, builds |
| `internal/domain/repograph/{path,position,surface,module,unit,graph}.go` | value objects + RepositoryGraph aggregate | ✓ VERIFIED | All files present; `RepositoryGraph`, `NewRepositoryGraph`, `UnitReference`, `Unit`, `NewResolvedUnit`, `NewUnknownUnit`, `Dependency`, `Reference` all exported as specified |
| `internal/domain/diagnostic/{diagnostic,set}.go` | Diagnostic, Key, Set with pure Diff | ✓ VERIFIED | Present; `Set`, `NewSet`, `Diff` exported |
| `scripts/check-architecture.sh` | ARCH-01 enforcement | ✓ VERIFIED | Current version (post commit a459430) is a pure-stdlib allowlist + platform-neutrality gate + external-deps allowlist + binary-links-testsupport gate; exits 0 on current tree |
| `scripts/test-check-architecture.sh` | self-test proving each rule fails | ✓ VERIFIED | 13 cases, all PASS, working tree untouched (`git status --porcelain` clean before/after) |
| `.github/workflows/ci.yml` | 2 jobs (check, architecture), triggers on master push + PR | ✓ VERIFIED | Present; `check` job now also runs `go mod tidy -diff`, staticcheck, govulncheck, `-race`, cross-build (expanded beyond the original plan by commit `0b56ca2`, a deliberate post-plan hardening, not a gap); `architecture` job runs both scripts |
| `internal/testsupport/synthrepo/{spec,manifest,render,generate}.go` | Spec/Render/Generate/Manifest | ✓ VERIFIED | All present; `Spec`, `ErrorKind`, `BadOutputRef`, `Render`, `Tree`, `File`, `Generate`, `Manifest`, `ExpectedDiagnostic` all exported as specified |

### Key Link Verification

| From | To | Via | Status | Details |
|------|----|----|--------|---------|
| `diagnostic.go` | `repograph.Position` | `Diagnostic` carries `repograph.Position` | ✓ WIRED | `pos repograph.Position` field, `New(...)` param, `Pos()` accessor all present |
| `graph_test.go` | RepositoryGraph queries | pure output-ref check over hand-built graph | ✓ WIRED | `unknownOutputRefs` at line ~269-288 chains the exact query sequence |
| `ci.yml` | `scripts/check-architecture.sh` | architecture job run step | ✓ WIRED | line 61 |
| `ci.yml` | `scripts/test-check-architecture.sh` | architecture job run step | ✓ WIRED | line 63 |
| `manifest.go` | `diagnostic.CodeUnknownOutput` + `repograph.Position` | `ExpectedDiagnostic` fields use domain types | ✓ WIRED | `Code diagnostic.Code` field present; `Pos repograph.Position` field also present |
| `render.go` | `math/rand/v2` PCG | explicit `*rand.Rand` from `rand.NewPCG`, never package-level `rand.*` | ✓ WIRED | `rng := rand.New(rand.NewPCG(uint64(spec.Seed), 0x9E3779B97F4A7C15))`; `test-check-architecture.sh`/plan verify step also grepped for stray package-level `rand.*` calls and found none |

### Requirements Coverage

| Requirement | Source Plan | Description | Status | Evidence |
|-------------|-------------|--------------|--------|----------|
| ARCH-01 | 01-02-PLAN.md | Domain layer contains no HCL/filesystem/infra import, enforced by an automated check | ✓ SATISFIED | `scripts/check-architecture.sh` + CI `architecture` job, green on `a459430` |
| ARCH-02 | 01-01-PLAN.md | Analyzers and graph queries are pure functions over domain types, testable without a filesystem | ✓ SATISFIED | `graph_test.go` `unknownOutputRefs` test-local proof over hand-built objects |
| VALID-01 | 01-03-PLAN.md | A generator produces synthetic Terragrunt repositories from a spec — N units, nested includes, dependency chains — used for golden tests and benchmarks | ✓ SATISFIED | `internal/testsupport/synthrepo` package, pinned-digest determinism test, `BadOutputRef` injection with exact-oracle proof |

No orphaned requirements: REQUIREMENTS.md maps only ARCH-01, ARCH-02, VALID-01 to Phase 1, and all three are declared across the three plans' `requirements` frontmatter fields (01-01: ARCH-02, 01-02: ARCH-01, 01-03: VALID-01).

### Anti-Patterns Found

None. Scanned `internal/domain`, `internal/testsupport`, `cmd`, `scripts`, `.github` for `TODO|FIXME|XXX|HACK|PLACEHOLDER|placeholder|coming soon` — zero matches. No stub returns, no empty handlers (this phase has no UI/API surface, only domain types, a generator and shell scripts).

### Automated Verification Run (this session)

All commands run directly against the current tree at commit `8bf575e` (HEAD; the phase-1-relevant commit is `a459430`, itself green in CI):

- `go build ./...` — OK
- `go vet ./...` — OK
- `go test -count=1 ./...` and `-count=3 ./...` — all packages pass, repeatedly, no flakiness
- `"$(go env GOROOT)/bin/gofmt" -l .` — empty (clean)
- `go mod tidy -diff` — exit 0 (no drift)
- `bash scripts/check-architecture.sh` — `architecture: OK (2 domain packages)`
- `bash scripts/test-check-architecture.sh` — 13/13 PASS, `all architecture self-tests passed`
- `go test -race ./...` — could not run locally (`-race requires cgo; enable cgo by setting CGO_ENABLED=1`, and no gcc is installed in this environment); this is expected per the task instructions — CI (`ubuntu-latest`, which has gcc) runs `go test -race -count=1 ./...` in the `check` job, confirmed green on commit `fix(01): dedup diagnostic Set by Key, tighten domain arch check` (`a459430`, run `36126106981`, conclusion `success`)
- `go list -deps ./internal/domain/...` (non-standard only) — lists only the two intra-domain packages, no external or infra leakage
- `go list -deps ./cmd/gruntled | grep testsupport` — empty, confirms the shipped binary never links `internal/testsupport`
- `gh run list --limit 3` — commit `a459430` (the architecture-hardening fix directly relevant to Phase 1's ARCH-01 criterion) is `completed`/`success`; the current HEAD (`8bf575e`, a Phase 2 docs-only commit) was still `in_progress` at verification time and is out of scope for Phase 1

### Human Verification Required

None. All four success criteria and all three requirements are mechanically verifiable and were verified directly against the codebase and CI.

### Gaps Summary

No gaps. The phase goal is fully achieved: the domain layer is pure and CI-enforced (13 self-test cases, not merely the 7 originally planned — commit `a459430` broadened the enforcement from a denylist to an allowlist plus a platform-neutrality gate, closing gaps the original denylist would have missed, e.g. `fmt`, `log`, `unsafe`, `crypto/rand`, build-tagged files), graph queries are proven pure and testable with hand-built objects, and the synthetic-repository generator is deterministic and ships an exact diagnostic oracle. The one deviation from the original plans — CI's `check` job now also runs `go mod tidy -diff`, `staticcheck`, `govulncheck`, `-race`, and a 5-target cross-build (added in commit `0b56ca2`, beyond the plan's original 4-step job) — is a strict superset of what was planned and does not weaken any check.

---

_Verified: 2026-09-25T10:55:10Z_
_Verifier: Claude (gsd-verifier)_
