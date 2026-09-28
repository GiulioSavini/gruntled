# Phase 4: Real-Repo Validation Experiment - Context

**Gathered:** 2026-09-28
**Status:** Ready for planning
**Source:** Orchestrator decisions (user delegated all choices) on the open questions in 04-RESEARCH.md

<domain>
## Phase Boundary

Phase 4 settles the v0.1 claim. It adds golden tests over hand-written and full-scale
synthetic fixture repositories (VALID-02). It runs `gruntled check` on the unmutated
primary corpus (VALID-03), then on copies of it with one injected mutation each
(VALID-04). It confirms that plain `terragrunt hcl validate` misses those mutations
(VALID-05), and it benchmarks both tools on the same repository (VALID-06). The results go
in a committed `docs/validation.md`.

This phase adds no production features. It adds test code, fixture data and one results
document. If the experiment fails, the project stops here. That is the outcome the
roadmap planned for, not a defect to fix inside this phase.

Preconditions (not work in this phase): Phase 3 is executed and verified, and the
Phase 2 gap-closure plans (02-06, 02-07 and anything else from 02-REVIEW.md) have landed.
Phase 4 drives `run()` in `cmd/gruntled` and the JSON schema v1 from 03-02/03-03.
</domain>

<decisions>
## Implementation Decisions

### Fixtures (locked)
- Full-scale synthrepo specs, as the research recommends: several hundred units, include
  depth up to 4, fanout up to 8, several seeds, and many `BadOutputRef` injections, plus
  one clean full-scale tree. The oracle is `Manifest.Expected`, compared for exact set
  equality.
- Hand-written golden fixtures are authored fresh for this project. Nothing is copied
  from `denis256/terragrunt-tests` or any other external repository.
- Expected diagnostics in golden fixtures are computed by hand, byte by byte. There is
  no `-update` flag, and expectations are never generated from gruntled's own output.

### Environment gating (locked)
- Exactly two environment variables: `GRUNTLED_CORPUS` (existing convention) points at
  the pinned primary-corpus checkout. `GRUNTLED_TERRAGRUNT_BIN` points at the pinned
  terragrunt binary.
- Corpus and terragrunt tests skip when their variables are unset. They are NOT added to
  CI. `.github/workflows/ci.yml` must not be edited.
- Golden tests (VALID-02) need no external asset. They are always on, so they run in CI's
  existing `go test ./...` step without any workflow change.
- Test code never hardcodes `~/.cache/gruntled-phase4/...` paths. Those paths appear only
  in docs/validation.md's reproduction commands.

### Comparator and assets (locked)
- The only comparator is plain `terragrunt hcl validate`. Never use `--inputs` or
  `--strict`: PREP.md §4 measured them failing on the clean corpus and writing a
  `.terragrunt-cache/` into it.
- The terragrunt binary comes only from `~/.cache/gruntled-phase4/bin`: v1.1.6, SHA256
  `d75a80bb264758ba00dabcb17f4b507fcdab4ca90d9e41f96750df036bf69b04`. Tests check that
  hash and version and fail (not skip) on a mismatch.
- Corpora stay outside the repository. The primary corpus is
  `aws-solutions-library-samples/guidance-for-iso20022-messaging-workflows-on-aws` at
  `e6c55d11fd1a01e75b78d7897be36c69fa26b8cc` (detached HEAD). Tests fail (not skip) if
  `GRUNTLED_CORPUS` is at a different commit.
- Mutations happen only on temporary copies (`t.TempDir()`). They never touch the corpus
  checkout or this repository. A tree digest of the corpus checkout, taken before and
  after, proves this.
- terragrunt also runs only against temporary copies, with a minimal explicit environment
  (PATH limited to the pinned bin dir, HOME and TMPDIR set to temp dirs, no inherited
  `TG_*` variables).

### Results (locked)
- The results live in a committed `docs/validation.md`. It records the terragrunt version
  and SHA256, the corpus commit SHA, machine info, each mutation (the exact edit), the
  exact expected and actual diagnostics, terragrunt's observed output, and the benchmark
  method with samples and medians.
- `os/exec` appears only in `_test.go` files. The `binary-no-net-no-exec` rule stays
  exactly as it is.

### Failure handling (locked)
- If the experiment FAILS, the plan stops and records the failure honestly. A failure
  means a false positive on the unmutated corpus, a missed or extra reference on a
  mutation, terragrunt reporting the injected break, or gruntled being slower. Nothing is
  tweaked to make the tests pass: no assertion is loosened, no oracle is edited to match
  gruntled, no benchmark flags are changed, and no production code is fixed in this
  phase.
- The one allowed repair is a harness bug that is shown independently of gruntled's
  output. Examples are a copy helper that flattens symlinks, or an oracle regexp that
  disagrees with `grep`. The evidence goes in the SUMMARY.

### Assertions vs PREP.md numbers
- gruntled-derived counts (65 units, 3 config-unknown, 22 references) may shift after the
  Phase 2 gap closure. They are logged and recorded, never asserted.
- Text facts of the pinned commit are fixed points and may be asserted as cross-checks:
  8 `dependency.s3.outputs.role_name` references and 3 `dependency.mq.outputs.region`
  references.

### Claude's Discretion
- The second (deletion) mutation: delete `output "region"` from `iac.mq/mq_broker/state.tf`,
  which breaks the 3 `dependency.mq.outputs.region` references in `iac.mq/ecr_mq_*`. It
  uses a different module from the rename mutation, per PREP.md §5.3.
- Benchmark method: both tools run as external processes on the same scratch copy, with
  3 warm-up runs each, then 21 interleaved samples each, compared by median. A
  supplementary in-process `b.Loop()` benchmark is included.
- Golden fixture format: txtar archives under `cmd/gruntled/testdata/golden/`, with a
  reserved `_golden/` section for expectations and symlinks.
- The secondary corpora (cds-snc/secret, cds-snc/gc-articles, denis256) are not used for
  any VALID-* claim. docs/validation.md says why (see PREP.md §3).
</decisions>

<deferred>
## Deferred Ideas
- Running the corpus or benchmark in CI, and a hyperfine-based benchmark.
- Secondary-corpus experiments and more mutation kinds beyond one rename and one deletion.
- Anything that changes gruntled's behaviour. That is out of scope for a terminal
  experiment phase.
</deferred>

---
*Phase: 04-real-repo-validation-experiment. Context gathered 2026-09-28 by the orchestrator.*
