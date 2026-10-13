---
phase: 17-corpus-mutation-proof-cross-phase-audit
verified: 2026-10-13T00:00:00Z
status: passed
score: 4/4 success criteria verified (10/10 section-6 invariants backed by evidence)
head: 568e260
---

# Phase 17: Corpus Mutation Proof & Cross-Phase Audit Verification Report

**Phase Goal:** User can trust the milestone on real repositories and across phase boundaries: no false positives, no noise from reformatting, every injected change caught.
**Verified:** 2026-10-13 at HEAD 568e260 (master, pushed, ci run 38363329099 success, 0 tags at HEAD)
**Status:** passed
**Re-verification:** No (initial)

## Goal Achievement

### Observable Truths (ROADMAP success criteria)

| # | Truth | Status | Evidence |
|---|-------|--------|----------|
| 1 | Each corpus: `check` byte-identical to v0.3.0; `blast --base <same tree>` has empty Broken and Impacted | VERIFIED | corpus run 38264937436 log: shards `compare-ref ok`, `TestCorpusBlastSelf ok`. 17-05 gate re-ran compare-ref fresh on the final tree: `256/256 identical` |
| 2 | Reformat-only mutations give empty Impacted | VERIFIED | `TestCorpusReformat` (cmd/gruntled/corpus_reformat_test.go:463) covers R1 whitespace, R2 comments, R3 reorder, R4 optional() default, R5 .tf to .tf.json (and reverse), R6 output moved, R7 deprecated, plus R8/R9; shard ok in run 38264937436 and in the 17-03 record |
| 3 | Every mutation in the set caught (M1 GRT004 at every site and no GRT001, M2-M7, M9, propagation depth >= 2) | VERIFIED | 31/31 shards in run 38264937436 (all 24 class shards + 3 M8 shards listed in log). 17-03 table: iso20022 1,337, secret 27, denis256 2,981 = 4,345 sites, caught == sites in every row. The test enforces it: `if h.caught != h.sites { t.Errorf(...) }` (corpus_mutation_test.go:669). M8 depth counters pinned: denis256 667/147/67/12 at distance >= 2/3/4/5 |
| 4 | Independent review leaves no open BLOCKER/MEDIUM; six-target no-net/no-exec proof and doc pins pass on the final tree; docs match GRT004, `--depth`, `report --blast`, `rebase` | VERIFIED | See SC4 section below |

**Score:** 4/4

### SC4 detail

- **Independent review:** 17-INTEGRATION.md (gsd-integration-checker, 7708983) and 17-SECURITY.md (gruntled-sec, 88a62aa + re-audit 4f966d7), neither written by the executor.
- **No open BLOCKER/MEDIUM:**
  - Integration F1 MEDIUM (staticcheck red): fixed 0682227; ci green on a11dc37 (38264926783), baae464 and 568e260 (38363329099).
  - Integration F2 / security F1 MEDIUM (corpus.yml could not pass): fixed in 78fa801 (AppArmor userns sysctl) and acf2759 (`go mod download` + `go mod verify` before the offline gate). Closed by corpus run 38264937436 on a11dc37, conclusion success. I checked this run myself.
  - Integration F3/F4 LOW and security F2-F5 INFO: closed or accepted (security F3, the sysctl, accepted with a reason).
  - 17-SECURITY frontmatter still says "1 open condition". That condition was the run above, which is green. The frontmatter is stale, not an open finding.
- **Six-target proof:** `check-architecture.sh` OK fresh in the 17-05 gate. `test-check-architecture.sh` cited from 4f966d7; only docs and one test file changed after that.
- **Doc pins:** 7 PASS fresh (TestRuleRegistryDoc, TestHelpMatchesDocs, TestTopUsageMatchesDocs, TestDaemonDocs, TestCIDoc, TestValidationDocPins, TestReadmeDocumentsV03).
- **Docs vs behaviour:** the one known mismatch (validation.md:873, "corpus.yml runs the same two scripts") is now true, because corpus.yml passes. The topUsage `report --blast` line was fixed in 5ee63ff and is pinned by TestTopUsageMatchesDocs.
- **Final tree:** `git diff a11dc37..HEAD -- ':!.planning'` touches only `internal/infrastructure/terragrunt/incremental_test.go` (+8 lines, test only). So the a11dc37 corpus and ci runs cover the shipped product code exactly.

### Section-6 invariant table: evidence check

Each row was checked by both reviewers. Rows that cite code give file:line, and I re-checked the greps myself where cheap.

| # | Invariant | INTEGRATION | SECURITY | Independent check here |
|---|-----------|-------------|----------|------------------------|
| 1 | GRT004 only reclassifies GRT001, only in Between | holds: grt004.go:108 sole constructor, :170-172 drop, sole caller blasting.go:84, tests named | holds: same lines plus one-for-one removal, M1 dual oracle | M1 dual oracle against the fingerprinted v0.3.0 binary (17-03) |
| 2 | Type facts never Broken / diagnostic / exit | holds: impact.go:134, :88-97, exit sites main.go:390, report.go:203, render.go:81-87 | holds, plus corpus M3-M7 exit 0 | 17-03: every class except M1 has Broken [] |
| 3 | Depth/paths, `--depth 1` = v0.3, `report --blast --depth` exits 2 | holds: impact.go:155-166, main.go:374, report.go:87-91 | holds: parseDepth main.go:414-428 | M8 counters pinned |
| 4 | Baseline equivalence, rebase atomic | holds: render.go:69-79, watch.go:330-337, 382 | holds: product unchanged since phase 16 re-audit (26 mutants); TestCorpusDaemonSample | Daemon sample is a 17-04 local record, not a gate shard (see notes) |
| 5 | Escape pipeline | holds: blast.go:161-339, :433, SanitizeReason sites | holds: lists every unescaped write by type | n/a |
| 6 | for-expression guard: exact 4 sites | holds: each site with its guard line | holds: exact (file, function) set | **Re-ran the grep:** code sites only at typesig.go:161, literal.go:26, eval.go:28, eval.go:76 (the other matches are comments) |
| 7 | Bounded cost | "recorded; unverified at HEAD" (F3) | upgraded in the re-audit to **holds at HEAD**: ci 38264926783 `test` step runs TestBlastSizeBound/TestReportBlastSizeBound | ci green on a11dc37 and 568e260 |
| 8 | No regressions vs v0.3.0 | holds | holds | compare-ref 256/256 and compat-v1 7/7 fresh in 17-05; go.mod/go.sum unchanged since v0.3.0 (re-checked) |
| 9 | No-net/no-exec, HCL/typeexpr confinement | holds | holds | check-architecture OK fresh (17-05) |
| 10 | Docs = shipped behaviour | holds | holds, with the validation.md:873 exception, since closed | doc pins 7 PASS fresh |

All 10 rows are backed by code locations, named passing tests or run IDs. None is asserted without evidence. Row 7 was the only weak row, and it was closed by a green CI run that I confirmed exists.

### REL-03 corpus mutation proof: validity

- **Results:** two independent 31/31 runs.
  - 17-03 local record: 2026-10-12, wall 3,020 s, `network=isolated (/proc/net/dev: lo)`.
  - corpus.yml run 38264937436 on a11dc37 (GitHub ubuntu runner).
  - I checked the log: 31 `shard ... ok` lines and no non-ok shard. The shards are compare-ref, BlastSelf, BlastModel, Reformat, and M1-M7, M9, M8 for each of the 3 corpora.
- **"Network isolated":** `corpus-gate.sh` re-execs under `unshare -rn` (an empty network namespace). It proves isolation from `/proc/net/dev` (only `lo`), and that proof does not depend on the `CORPUS_GATE_INNER` marker. `CORPUS_GATE_NONET=1` without a namespace is fatal (proved by failed run 38262035294). The gate also runs with `GOPROXY=off -mod=readonly`, and the modules were checked by `go mod verify` beforehand. So nothing in the gate can reach the network or pull new code. The results come only from the pinned sources and the corpora at pinned commits.
- **"Real v0.3.0 ref binary":** the log prints `GRUNTLED_REF_BIN gruntled v0.3.0 (e870e2984760f0a674bc87697e3f69329613758e)`. That sha equals my local `git rev-parse v0.3.0^{commit}` and the `pinned=` line in scripts/corpus-gate.sh:71. `checkRefBin` (corpus_mutation_test.go:89) also requires a fingerprint. This is what makes the M1 dual oracle and compare-ref non-circular: the GRT004 sites are compared with GRT001 sites printed by a frozen binary that predates v0.4, not with v0.4's own output.
- **Non-circularity of the other classes:** the expectations come from an independent model that imports no internal or HCL package (enforced by check-architecture, T-17-02-1). The exclusions are pinned per corpus and class, and each is bound to a reason category. A `--- SKIP` shard counts as a failure (corpus-gate.sh:153).

### Required Artifacts

| Artifact | Status | Details |
|----------|--------|---------|
| scripts/corpus-gate.sh | VERIFIED | pinned ref sha, unshare, SKIP=fail; exercised green in CI |
| scripts/fetch-corpora.sh + test | VERIFIED | test-fetch-corpora passes (17-05 fresh) |
| cmd/gruntled/corpus_{blast,reformat,mutation,model,daemon}_test.go | VERIFIED | test functions exist; caught==sites assertion present |
| .github/workflows/corpus.yml | VERIFIED | green run 38264937436; pinned by TestCIDoc |
| 17-INTEGRATION.md, 17-SECURITY.md | VERIFIED | present, independent authorship, findings dispositioned |
| docs/validation.md REL-03 section | VERIFIED | pinned by TestValidationDocPins |

### Key Links

| From | To | Status |
|------|----|--------|
| corpus-gate.sh | v0.3.0 ref build (pinned sha) | WIRED (log line matches the tag sha) |
| corpus.yml | corpus-gate.sh, offline | WIRED (green run, network=isolated) |
| mutation tests | independent model expectations | WIRED (caught==sites enforced, pins) |
| Integration/Security findings | fix commits | WIRED (all 17 cited commits exist) |

### Requirements Coverage

| Requirement | Source Plan | Status | Evidence |
|-------------|-------------|--------|----------|
| REL-03 | 17-01..17-05 | SATISFIED | Truths 1-3 above. REQUIREMENTS.md:49/97 still show `[ ]` / Pending (integration F5): orchestrator bookkeeping, to be ticked on phase close |

No orphaned requirements: REL-03 is the only ID mapped to phase 17.

### Anti-Patterns / Caveats (none blocking)

| Item | Severity | Note |
|------|----------|------|
| "100% caught" means 100% of eligible sites | Info | Exclusions exist (iso20022: 372 declared-in-shared-link, 3 linker-of-excluded-units; denis256: type-grammar 1, type-declared 1, 8 excluded_by_touch). They are pinned and reasoned, and the shared-link variables are covered by M9. An adversarial reader should know the denominator is the model's eligible set, not every syntactically possible site |
| M8 depth counts come from expectations | Info | They are not re-run through gruntled in the M8 shard. They are valid because the class shards assert gruntled's distance and via equal to the same expectations |
| Daemon sample outside the gate | Info | TestCorpusDaemonSample is not a corpus.yml shard. Its evidence is the 17-04 local record plus unit-level TestDaemonBlastEqualsBlastBase in CI |
| Local corpus gate not re-run at HEAD | Info | Justified: product code at HEAD == a11dc37 (diff is test-only), and run 38264937436 is on a11dc37 |
| 17-SECURITY frontmatter "1 open condition" | Info | Stale; the condition (run 38264937436) is green |
| ROADMAP phase 17 `[ ]`, "Plans: TBD"; REL-03 Pending | Info | Bookkeeping for the orchestrator/milestone close |
| Carried tech debt (M18, loose RSS bound, rebase.go:123 `%v`, run.go branch) | Info | Already accepted, listed for the milestone audit |

### Human Verification Required

None needed for the phase goal. The tag decision (v0.4.0) and the v0.3.1 backport are release decisions for Giulio, not verification items.

### Gaps Summary

No gaps. The phase proves what it claims to prove:
- Two independent 31/31 gate runs: one local and one on a fresh CI runner, both network-isolated and both against the sha-pinned v0.3.0 reference binary.
- 4,345 mutations caught under exact equality, enforced in code.
- Two independent reviewers whose MEDIUM findings were fixed and confirmed by green runs that I checked.
- The final product tree is identical to the tree those runs covered.

An adversarial reviewer's strongest objection is that the denominator is the model's eligible set, with pinned exclusions. That objection is disclosed and pinned, and it does not undercut the claim.

---

_Verified: 2026-10-13_
_Verifier: Claude (gsd-verifier)_
