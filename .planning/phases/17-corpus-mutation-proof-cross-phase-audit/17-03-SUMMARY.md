---
phase: 17-corpus-mutation-proof-cross-phase-audit
plan: 03
subsystem: cmd/gruntled (tests), scripts
tags: [corpus, rel-03, mutation, dual-oracle, gate]
requires: [17-02]
provides:
  - "TestCorpusMutation17/<corpus>/<class>: M1-M7, M9 with exact equality, M8 depth counters, all pinned"
  - "M1 dual oracle against a fingerprinted v0.3.0 binary"
  - "scripts/corpus-gate.sh: the serial offline REL-03 gate (31 shards)"
  - "sec #453 LOW items: category-to-reason binding, pinned derived subjects, eligibility gap by reason"
affects: [17-04, 17-05]
tech-stack:
  added: []
  patterns: ["onApplied hook for an oracle that needs the mutated tree", "unshare -rn re-exec with a /proc/net/dev proof"]
key-files:
  created:
    - cmd/gruntled/corpus_mutation_test.go
    - scripts/corpus-gate.sh
  modified:
    - cmd/gruntled/corpus_model_test.go
    - cmd/gruntled/corpus_harness_test.go
key-decisions:
  - "M8 reads the expected results of the same descriptor lists the class shards prove equal to gruntled. Re-running every class inside the M8 shard would double the 50-minute gate"
  - "M9: a module that links the shared file but is instantiated only by excluded units is dropped from the expected changes. The accepted rows say those units carry no module. This is removal, not correction: if gruntled did list such a module, exact equality would fail"
  - "Files shared through a symlink are edited only by M9. A variable declared only in a linked file is counted as declared-in-shared-link (iso20022: 372), not as a site of M4/M5"
  - "The gate caps each shard's address space (ulimit -v 4 GB) on top of the 1500 MB floor. A development run of an unfixed expectBlast had grown to 6.4 GB before it was killed"
requirements-completed: [REL-03]
duration: 4h
completed: 2026-10-12
---

# Phase 17 Plan 03: Mutation Set and Corpus Gate Summary

**REL-03 clause 3 is recorded. One offline run of `scripts/corpus-gate.sh`, isolated in an empty network namespace, passed 31 of 31 shards in 50 minutes. Those shards cover compare-ref 256/256, the self-blast, the model with its pinned exclusions, R1-R9, and 4,345 mutations. Each mutation was caught with exact equality: 1,337 on iso20022, 27 on secret and 2,981 on denis256.**

- **Dual oracle:** every M1 GRT004 set equals the GRT001 set of a fingerprinted v0.3.0 binary.
- **Propagation:** reaches distance 5 on denis256.
- **Pins:** sites, exclusions and depth counters are pinned per corpus and class.
- **sec #453:** its three LOW items were applied before the recorded run.

## Performance

- Duration: about 4 h. The recorded gate took 3,020 s; the development runs took about 1 h.
- Tasks: 3, plus the sec #453 items
- Files: 2 created, 2 modified

## Accomplishments

### sec #453 LOW items (8abad29), before the recorded run

1. **`categoryBinding`.** For each category it pins the allowed (difference kind | gruntled reason) pairs. Every covered difference is checked against them. For example, include target allows only `config-unknown include-target` on module-of, edge and ref, plus `module-known|absent`. The pins were taken from the observed set after a review: every pair carries the reason its category predicts. A mutant that binds include target to `syntax-error` fails secret.
2. **`corpusDerived`.** It pins the derived-coverage subjects: `{issue-4209/base}` on denis256 and none elsewhere.
3. **`corpusEligibilityGap`.** It logs (`CORPUS-MODEL-GAP`) and pins the gap by reason. "818 known vs 811 eligible" is a net figure:
   - 15 gruntled-known modules are not eligible. 14 are `generate-may-change-surface`: a seed's generate block has a non-literal path or contents, or a path that collides with a module file. This is a conservative choice on the model side. The 15th, issue-2298/app, is `not-a-model-module`: the model does not evaluate its include.
   - 8 eligible modules are not gruntled-known; they belong to excluded units.

   The catRefOther (module, output) pairs leave M1/M2 as `excluded_by_touch`: 8 on denis256, the pairs of the 16 rows that fall in included modules.

### Task 1: M1 with the v0.3.0 dual oracle, and M2 (5348dae red, 5c114bc pins)

- The descriptors come from the 17-02 model, in sorted (module, file, byte offset) order. One working copy per class goes through `runMutation`, the driver the self-test proves able to fail.
- `refBin` (corpus_mutation_test.go) accepts GRUNTLED_REF_BIN only with:
  - the exact label `gruntled v0.3.0 (e870e2984760f0a674bc87697e3f69329613758e)`;
  - `rebase` exiting 2 with `gruntled: unknown command "rebase"`;
  - `blast --depth 1` exiting 2 with `flag provided but not defined: -depth`.
- A missing or wrong binary fails the test; it never skips. Both negative rows were run on secret/M1:
  - unset gives `GRUNTLED_REF_BIN is not set: … run scripts/corpus-gate.sh, which builds and exports it`;
  - a HEAD build with the v0.3.0 ldflags prints the right label but fails with `fails the fingerprint: rebase exit 3, stderr line 1 "gruntled: no watch dae…"`.
- `dualCompare` is a pure function. It requires v0.3.0's GRT001 set for the removed (module, output) to equal HEAD's GRT004 set by (unit, file, line, col), and the messages to match after rewriting `was removed from` to `is not declared by`. An empty set fails.
- The always-on self-test row `fault/dual oracle differs by one site` shows three things: agreeing sets pass, a one-site difference fails, and an empty v0.3.0 set fails. The comparator fault "GRT001 left at a GRT004 site" was already a 17-02 self-test row.
- M1: iso20022 4, secret 3 and denis256 87 mutations, caught with the dual oracle equal on each. On iso20022 the 4 pairs carry the model's 22 plain sites, which is exact Broken equality. denis256 has 8 excluded by touch.
- M2: 392, 1 and 387, with exit 0 and Broken [].

### Task 2: M3-M7, M9 and SENTINEL (5348dae, 5c114bc)

**SENTINEL, red first.** With `m17SentinelCanon` blanked, secret/M5 failed on every site and printed `new "object({\"gruntled_m17\"=string})", want ""`.

Hand check against the D-15 rules in internal/infrastructure/tfsurface/typesig.go:30-100:
- The source `object({gruntled_m17 = string})` has one attribute.
- The renderer writes `object({`, then the sorted names, each `strconv.Quote`d (`"gruntled_m17"`), then `=` with no whitespace, then the primitive `string`. There is no `optional(` marker because the attribute is not optional. Then it closes with `})`.
- The result is `object({"gruntled_m17"=string})`, which equals the value HEAD printed.

The constant's comment points to this summary and to docs/validation.md, which 17-04 adds.

M3 adds `variable "gruntled_m17_req" {}` in gruntled_m17.tf. M4 deletes the default. M5 sets or adds the sentinel type, for variables in the simple type grammar only (a recursive-descent parser, no HCL import). M6 is two-sided: base' gets `type = string` and cur' gets `type = number`, and base' vs the pristine checkout must be empty. M7 flips `sensitive`. M9 applies M4 and M5 to the shared symlink target. A text-token check runs once per class.

Results: every class exits 0 with Broken [] except M1. iso20022 M4 has 0 sites by design: every default is in the shared global.tf, which M9 covers (12 caught across 62 linker modules each).

### Task 3: M8, corpus-gate.sh and the recorded run (5c114bc, 853bb17)

M8 counts the mutations whose expected result has an entry at distance ≥ 2, 3, 4 and 5, from the same descriptor lists. The counts are pinned: iso20022 67/0/0/0, secret 14/0/0/0, denis256 667/147/67/12.

scripts/corpus-gate.sh, in order:
1. If CORPUS_GATE_INNER is unset and CORPUS_GATE_NONET is not 0, it runs `exec unshare -rn -- env CORPUS_GATE_INNER=1 bash "$0"` when `unshare -rn true` works.
2. It proves the isolation from /proc/net/dev. CORPUS_GATE_NONET=1 with no namespace exits 1.
3. It exports `GOMAXPROCS=2 GOFLAGS='-mod=readonly -p=1' GOPROXY=off GOTOOLCHAIN=local`.
4. It requires that `v0.3.0^{commit}` equals e870e29…; a moved tag exits 1.
5. It creates the temp dir and worktree, and builds v0.3.0 with `-X main.commit=$sha` (the resolved sha).
6. It runs 31 shards one at a time. Before each it prints `free -m` and checks MemAvailable against the 1500 MB floor. Each shard runs under `timeout` with `-timeout 90m` and `ulimit -v`.
7. trap EXIT INT TERM HUP removes the worktree and the temp dir.

Self-check rows:

| Row | Result |
|-----|--------|
| `CORPUS_GATE_DRYRUN=1` | prints the reference build and 31 shards, runs nothing |
| `CORPUS_GATE_NONET=1`, PATH without unshare | `CORPUS_GATE_NONET=1 but an empty network namespace is not available (unshare -rn)`, exit 1 |
| moved tag (scratch clone, `git tag -f v0.3.0 HEAD~1`) | `v0.3.0 resolves to 245c286…, pinned e870e29… (moved tag)`, exit 1, before any worktree |
| unshare available | `corpus-gate: network=isolated (/proc/net/dev: lo)` |

**The recorded run** (15:18:50 to 16:09:10, alone on the machine, wall 3,020 s), started with:

```sh
GRUNTLED_CORPUS=… GRUNTLED_CORPUS_SECRET=… GRUNTLED_CORPUS_DENIS256=… bash scripts/corpus-gate.sh
```

```
corpus-gate: network=isolated (/proc/net/dev: lo)
corpus-gate: GRUNTLED_REF_BIN gruntled v0.3.0 (e870e2984760f0a674bc87697e3f69329613758e)
256/256 identical
corpus-gate: shard compare-ref ok 8s
corpus-gate: shard TestCorpusBlastSelf ok 5s
corpus-gate: shard TestCorpusBlastModel ok 8s
corpus-gate: shard TestCorpusReformat ok 30s
…
corpus-gate: 31/31
```

`free -m` stayed between 4,022 and 5,864 MB free (6,754 to 7,029 MB available). Peak RSS was about 150 MB per shard.

| Corpus | Class | sites | caught | excluded_by_touch | other exclusions | time |
|--------|-------|------:|-------:|------------------:|------------------|-----:|
| iso20022 | M1 | 4 | 4 | 0 | | 3 s |
| iso20022 | M2 | 392 | 392 | 0 | | 65 s |
| iso20022 | M3 | 62 | 62 | 0 | | 9 s |
| iso20022 | M4 | 0 | 0 | 0 | declared-in-shared-link 372 | 1 s |
| iso20022 | M5 | 75 | 75 | 0 | declared-in-shared-link 372 | 12 s |
| iso20022 | M6 | 396 | 396 | 0 | | 125 s |
| iso20022 | M7 | 396 | 396 | 0 | | 67 s |
| iso20022 | M9 | 12 | 12 | 0 | linker-of-excluded-units 3 | 3 s |
| secret | M1 | 3 | 3 | 0 | | 1 s |
| secret | M2 | 1 | 1 | 0 | | 0 s |
| secret | M3 | 3 | 3 | 0 | | 1 s |
| secret | M4 | 4 | 4 | 0 | | 0 s |
| secret | M5 | 8 | 8 | 0 | | 1 s |
| secret | M6 | 4 | 4 | 0 | | 1 s |
| secret | M7 | 4 | 4 | 0 | | 1 s |
| secret | M9 | 0 | 0 | 0 | no shared file | 0 s |
| denis256 | M1 | 87 | 87 | 8 | | 96 s |
| denis256 | M2 | 387 | 387 | 0 | | 289 s |
| denis256 | M3 | 795 | 795 | 0 | | 591 s |
| denis256 | M4 | 41 | 41 | 0 | | 34 s |
| denis256 | M5 | 708 | 708 | 0 | type-grammar 1 | 540 s |
| denis256 | M6 | 481 | 481 | 0 | type-declared 1 | 746 s |
| denis256 | M7 | 482 | 482 | 0 | | 378 s |
| denis256 | M9 | 0 | 0 | 0 | no shared file | 1 s |

```
CORPUS-MUTATION iso20022 M8 depth>=2=67 depth>=3=0 depth>=4=0 depth>=5=0
CORPUS-MUTATION secret M8 depth>=2=14 depth>=3=0 depth>=4=0 depth>=5=0
CORPUS-MUTATION denis256 M8 depth>=2=667 depth>=3=147 depth>=4=67 depth>=5=12
```

After the run, `git worktree list` showed only `/home/giulio/gruntled 853bb17 [master]`. No gruntled process or temp directory was left.

## Task Commits

| Task | Commit | Message |
|------|--------|---------|
| sec #453 | 8abad29 | test(17-03): bind exclusion categories to gruntled reasons and kinds, pin derived subjects and the eligibility gap |
| 1-2 (red) | 5348dae | test(17-03): TestCorpusMutation17 M1-M9 with the v0.3.0 dual oracle, red with zero pins |
| 1-3 | 5c114bc | test(17-03): pin sites, excluded_by_touch and exclusions per corpus and class, and the M8 depth counters |
| 3 | 853bb17 | feat(17-03): corpus-gate.sh, the serial offline REL-03 gate with the pinned v0.3.0 reference build |

## Gate

```sh
bash scripts/check-architecture.sh                 # architecture: OK (4 domain packages, 5 application packages, 1 interfaces packages)
go vet ./cmd/gruntled/                             # clean
go test -count=1 -short ./cmd/gruntled/            # ok 8.653s (no corpus env; TestCorpusMutation17 SKIPs x3)
go test -count=1 -run TestCorpusHarnessSelfTest ./cmd/gruntled/   # ok (27 subtests incl. the dual-oracle row)
bash scripts/corpus-gate.sh                        # corpus-gate: 31/31, network=isolated
```

Not run: -race, -fuzz, `go test ./...`, and test-check-architecture.sh (no architecture script changed).

## Threat Mitigations

| ID | Status | Where |
|----|--------|-------|
| T-17-03-1 | mitigated | expectBlast from the model and the descriptor only; dual oracle `dualCompare`/`dualOracle` in corpus_mutation_test.go; SENTINEL hand check above |
| T-17-03-2 | mitigated | `corpusMutationPins`, `corpusDepthPins`, `corpusIncluded`; caught == sites asserted per class |
| T-17-03-9 | mitigated | the four exactness exceptions are named in the corpus_mutation_test.go header and the harness header; 17-04 adds the docs claim table |
| T-17-03-3 | mitigated | the gate checks the pinned tag commit and uses the resolved sha in ldflags; `checkRefBin` requires the label and both fingerprints; negative rows recorded above |
| T-17-03-4 | mitigated | `exec unshare -rn` first, /proc/net/dev proof, `network=isolated` in the record; CORPUS_GATE_NONET=1 row |
| T-17-03-5 | mitigated | the gate exports GOMAXPROCS/GOFLAGS; serial shards; 1500 MB floor; `ulimit -v`; timeouts; about 150 MB per shard |
| T-17-03-6 | mitigated | work copies in t.TempDir; sha256 restore; digestGuard every 100 edits and at the end; checkout corpusDigest before and after |
| T-17-03-7 | mitigated | the only exec in the 17-03 tests and the gate's anchored shards is GRUNTLED_REF_BIN (fingerprinted); corpora ELF-stripped. The env-gated phase 5 terragrunt oracle (corpus_graph_test.go) runs terragrunt on corpus copies, outside the gate |
| T-17-03-8 | mitigated | trap EXIT INT TERM HUP removes the worktree, prunes and deletes the temp dir; `git worktree list` clean after the run |

## Deviations from Plan

1. **[M8] Computed from the expectations, not by re-running the classes.** The plan says the M8 shard "re-runs the classes" when it runs alone. In the gate every shard is its own process, so that would double the run. The counters come from expectBlast over the same descriptor lists, whose distance, source and via the class shards assert equal to gruntled.
2. **[Runtime] The gate takes 50 minutes, not about 35.** denis256 M3, M5 and M6 dominate, and M6 runs two blasts per site.
3. **[M9 linkers] Linkers whose seeds are all excluded units are dropped** from the expected changes (3 on iso20022). Without this, every M9 site would be excluded by touch.
4. **[Shared files]** Only M9 edits a file linked by a module. Variables declared only through a link are counted as `declared-in-shared-link`.
5. **[Memory] One development-run incident.** expectBlast ranged over the map it filled. With 62 modules (M9) the map grew without bound. The test reached 6.4 GB RSS and was killed by hand; the VM was not affected. It was fixed in 5348dae, and every later corpus run used `ulimit -v`, which the gate now also applies.
6. **[Commit] gofmt.** 5348dae went in without gofmt on corpus_mutation_test.go; 5c114bc formats it.

## Known Risks / Unverified

- The M6 base'-vs-pristine check uses the read-only checkout as the pristine tree. That is safe because the checkout digest is checked before and after, but the check reads the corpus.
- The gate needs `unshare -rn` (user namespaces) for `network=isolated`. On a host without it, the record shows `available` unless CORPUS_GATE_NONET=1.
- Nothing is pushed, so there is no CI run of the always-on rows yet.

## Self-Check: PASSED
