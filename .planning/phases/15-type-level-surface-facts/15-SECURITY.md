---
phase: 15-type-level-surface-facts
audited: 2026-10-11
auditor: gruntled-sec (code-audit, bus findings #317-#320, verdict #321)
scope: "git diff e6fca35..2e6c6f8 -- ':!.planning'"
verdict: OPEN_THREATS
threats_total: 32
threats_mitigated: 28
threats_accepted: 2
threats_na: 0
threats_open: 2
findings: { blocker: 1, high: 1, medium: 0, low: 1, info: 1 }
---

# Phase 15 Security

Verdict **OPEN_THREATS (2)**: one BLOCKER new in this phase (F1) and one HIGH that already
existed in v0.3.0 (F2). One fix closes both. The recursion work holds. Every crash shape from
plan review (#245, #260, #261, #271, #272, round 4) runs on a binary built from 2e6c6f8 through
`check`, `blast --base` and `blast --base --format json` (26 shapes x 3 = 78 runs). None crashed.
The slowest took 1.1 s and the largest peak RSS was 740 MB, and every hostile module stays
silent (Impacted 0, exit 0). The new problem is evaluation cost, not depth. `typeexpr`
evaluates `optional()` defaults and `LiteralBool` evaluates `sensitive`. A nil EvalContext does
not stop for-expressions over literal lists, and nested fors multiply. A 1,339-byte `.tf` makes
`check` take 45 s, and the list form took 6.7 GB RSS and OOM-killed the audit VM. The same trees
take 0.00 s on v0.3.0.

Mutation: 33 valid mutants in a throwaway copy (`git archive 2e6c6f8`), 31 killed by the
expected tests, 2 survived (F3: the controlDepth floor and the `else` rule are not pinned).
Mutants that failed to build were rewritten until they compiled. One mutant was dropped as
inconclusive: reading the JSON type string via `expr.Value(nil)` first is equivalent on every
tested input.

Baseline at 2e6c6f8:
- `go vet ./...` clean.
- Targeted tests green: impact, repograph, presenter, hclconv, tfsurface, terragrunt (hostile
  4 MB tests skipped locally under the OOM rule; CI is green on them).
- `cmd/gruntled` checks green: `TestScripts` 25/25 (incl. `blast_types`, `blast_impacted`),
  `TestTextOutputsEscapeControls`, `TestBlastDepth1MatchesV1`, `TestRuleRegistryDoc`.
- `go mod verify` OK, `govulncheck` (v1.8.0) clean, go.mod/go.sum unchanged in the range.
- `check-architecture.sh` OK. The typeexpr rule was probed with a `presenter` test import and a
  `//go:build windows` file in a `tfsurface/sub` package: both fail the rule.
- `go list -deps ./cmd/gruntled` shows no `net`, `net/*`, `os/exec`, `plugin` or `crypto/tls` on
  linux/darwin/windows x amd64/arm64. The only dependency added since v0.3.0 is
  `hcl/v2/ext/typeexpr`.
- `compare-ref.sh v0.3.0`: 244/244 identical.

15-00 cherry-pick: c1440d7, dfe565b and 3279bec apply cleanly onto v0.3.0 in a temporary
worktree (removed afterwards). On that worktree:
- `go vet ./internal/infrastructure/...` is clean, and the hclconv, terragrunt and tfsurface
  tests pass.
- A binary built there refuses the #261 repro (4.05 MB `terragrunt.hcl`, nested `%{if}` x270k)
  and the #271 `%{<NL>for x in [1]}` x165k repro as `config-too-deep`: exit 0 in 0.95 s and
  1.09 s.
- Plain v0.3.0 dies with `fatal error: stack overflow` on both (3.6 s and 1.4 GB, 3.3 s and
  1.4 GB).

## Threat register

| ID | Disposition | Status | Evidence |
|----|-------------|--------|----------|
| T-15-00-1 | mitigate | CLOSED | `ctl` in the depth sum `limits.go:267`, `controlDepth` `:281-297`, called at `:208`. Dropping `+ctl` is caught by `TestCheckNativeDepthTemplateControl`. #261 repro refused on HEAD and on v0.3.0+cherry-pick (see above) |
| T-15-00-2 | accept | CLOSED | 50,000 siblings and 990 nested are accepted (`limits_test.go` accepts table); a refusal is the existing unknown reason |
| T-15-00-3 | mitigate | CLOSED (test gap F3) | floor `limits.go:289`, `%%{` lex assertion in the test. The code is correct, but no test catches the no-floor mutant (F3) |
| T-15-00-4 | accept | CLOSED | lexer pre-scan cost: 0.5-1.1 s, 480-740 MB on the 4 MB repros |
| T-15-00-5 | mitigate | CLOSED | recursion audit spot-checked: no recursion crash found in 26 shapes. Non-recursive evaluation cost is a different class (F1) |
| T-15-00-6 | mitigate | CLOSED | trivia skip `limits.go:283`; anything else increments `:295-297`. Trivia-neutral and unknown-keyword-neutral mutants are caught. Four trivia forms x {terragrunt.hcl, module local, optional() default} at about 4 MB on the binary: no crash |
| T-15-01-1 | mitigate | CLOSED | name views independent of facts (`repograph/surface.go`); views mutant caught by `TestNewSurfaceFacts`; compare-ref 244/244 |
| T-15-01-2 | mitigate | CLOSED | `hclconv.LiteralBool` `literal.go:15-21` is the v0.3.0 `eval.go:75` body; a string-"true" mutant is caught by `TestLiteralBool`. Its pre-existing evaluation cost is F2 |
| T-15-01-3 | mitigate | CLOSED | `NewSurface` facts zero = `TristateUnknown`/"" (`options.go:11`); `TestNewSurfaceFactsUnknownByDefault` |
| T-15-02-1 | mitigate | **OPEN** (F1) | depth part closed (`typesig.go:137/140/143`, renderer cap `:50`, single builder). Stall part open: nested for-expressions in an `optional()` default are evaluated by `typeConstraint` `typesig.go:147`, 45 s for 1,339 bytes |
| T-15-02-2 | mitigate | CLOSED | `TypeString` unused; capsule/unknown kind -> false `typesig.go:105-106`; renderer depth mutant caught by `TestTypeSigCapsuleAndDepth` |
| T-15-02-3 | mitigate | CLOSED | sorted attribute names `typesig.go:85`; the no-sort mutant is caught by `TestReadSurfaceMetamorphic` |
| T-15-02-4 | mitigate | CLOSED | override -> unknown `facts.go:107`; per-fact agreement `:110-119`; mutants caught by `TestReadSurfaceOverride` and `TestReadSurfaceDuplicates` |
| T-15-02-5 | accept | **OPEN** (F1) | The acceptance was based on "nil EvalContext: variables and functions error", which is true, but for-expressions over literals evaluate with nil ctx and multiply (F1). Must become mitigate |
| T-15-02-6 | mitigate | CLOSED | Step 6b `check-architecture.sh:395-436` with non-vacuous guard; two probes fail as expected; six targets 0 forbidden packages |
| T-15-02-7 | mitigate | CLOSED | names from the same blocks `reader.go:184-193`; compare-ref 244/244 |
| T-15-02-8 | mitigate | CLOSED | 64 KiB cap `typesig.go:137`, bracket scan `:140`, `CheckNativeExprDepth` `:143`, encoding/json extraction `:158-172`. Each guard mutant is caught (`TestTypeSigJSONNewlineTernary` benign/2,000, `TestTypeSigJSONDeepString` list x500). The #245 and #260 JSON shapes run on the binary in 0.06-0.13 s and up to 28 MB |
| T-15-02-9 | mitigate | CLOSED | narrow recover `typesig.go:204-211`; mutant caught by `TestReadSurfacePanickingParser`; doc says it cannot catch a stack overflow |
| T-15-02-10 | mitigate | CLOSED | `CheckNativeExprDepth` root newlines off `limits.go:185-187`; config-mode mutants caught by `TestCheckNativeExprDepth` and `TestTypeSigJSONNewlineTernary`; #272 repro (4.14 MB) 0.14 s, 29 MB |
| T-15-03-1 | mitigate | CLOSED | Broken = `groupBroken(NewFindings(baseD, curD))` `impact.go:134`, independent of SurfaceDiff; facts read only in `impact/surface.go:131-172` and the presenter (grep); `TestTypeFactsNeverBroken`, property test `typefacts_test.go:343` |
| T-15-03-2 | mitigate | CLOSED | silence on "" / Unknown `surface.go:139,144,147,157,160`; five unknown-compared mutants caught by `TestTypeChangeMatrix`; unknown surface skipped `:99-106` |
| T-15-03-3 | mitigate | CLOSED | `slices.SortFunc` `surface.go:166`; no-sort mutant caught by `TestTypeChangeOrder` |
| T-15-03-4 | mitigate | CLOSED | `TypeChanges` shared by the module's SurfaceChange; `Empty` counts them (mutant caught) |
| T-15-04-1 | mitigate | CLOSED | `escapeTerm` `blast.go:269,299,306,316,318`; each of four removal mutants caught by `TestBlastTextEscapesTypeChanges`; JSON via `escapeJSON` pass; SEC-01 e2e `TestTextOutputsEscapeControls` green |
| T-15-04-2 | mitigate | CLOSED | tokens without type strings `blast.go:263-270`, section once `:285-321`; per-unit type-string mutant caught by `TestBlastTextTypeChanges` |
| T-15-04-3 | mitigate | CLOSED | `TestBlastDepth1MatchesV1` green (deletion counts per case) |
| T-15-04-4 | mitigate | CLOSED | `blast_types.txtar` silent cases green |
| T-15-04-5 | mitigate | CLOSED | every `blast_types.txtar` case exits 0 with Broken (0); `HasErrors` reads Broken only |
| T-15-05-1 | mitigate | CLOSED | `escape_test.go` e2e green (text and JSON, invalid UTF-8 cases) |
| T-15-05-2 | mitigate | CLOSED | `TestBlastSizeBound` (CI): bytes bounded from the rendered old+new, `strings.Count(text, new) == 1`. RSS bound is loose (F4) |
| T-15-05-3 | mitigate | CLOSED | same-size in-run control (#311), absolute values logged only, `-short` skip |
| T-15-05-4 | mitigate | CLOSED | gate re-run by sec: arch, six targets, mod verify, compare-ref |

## Findings

| # | Severity | Ref | Finding | Disposition |
|---|----------|-----|---------|-------------|
| F1 | BLOCKER | `tfsurface/facts.go:77`, `typesig.go:147`, `facts.go:94` -> `hclconv/literal.go:16` | Unbounded CPU/RAM in `check`, `report`, `watch` and the daemon from a module file of about 1.3 KB. The cause is new in this phase. `typeexpr.TypeConstraintWithDefaults` evaluates every `optional()` default with `Value(nil)`, and `LiteralBool` evaluates an output's `sensitive`. Nested for-expressions over literal lists need no variables or functions, so they run, and each level multiplies the cost. Measurements (4 levels x 100 elements): `%{for}` template default in `.tf`: 1,339 bytes, 45.6 s, 561 MB. The same in `.tf.json`: 45.0 s, 755 MB. `optional(any, [for ...])`: 54 s, 6.7 GB RSS (OOM-killed the 8 GB VM). `sensitive = [for ...] == []`: over 90 s. v0.3.0 on the same trees: 0.00-0.04 s. Five levels would take about 75 min, six about days. `blast --base` pays the cost for both trees, and `ReadSurface` ignores ctx, so `watch` cannot cancel it | Fix: (1) before `parseTypeFact` on a native expression, `hclsyntax.VisitAll` it; any `*hclsyntax.ForExpr` (`[for]`, `{for}`, template `%{for}`) makes the type fact unknown and typeexpr is not called. For a `.tf.json` type string, after the existing guards, lex the decoded string and refuse on any `for` identifier token. (2) `hclconv.LiteralBool` returns `TristateUnknown` without calling `Value` when a native expression contains a `ForExpr`. Every v0.3 literal result stays the same, so compare-ref is unaffected. (3) Tests at 5 x 100 for list, object and template `%{for}` (quoted and heredoc), `.tf` and `.tf.json` defaults, and output `sensitive`: each under 1 s, fact unknown, name kept, typeexpr not called; plus one `blast --base` e2e. Add row T-15-02-11; T-15-02-5 becomes mitigate |
| F2 | HIGH (pre-existing, v0.3.0) | `terragrunt/depfacts.go:23,26,92` -> `hclconv/literal.go:16` (v0.3.0 `eval.go:75`) | `dependency "b" { enabled = [for ...[for ...[for ...]]] == [] }` in a `terragrunt.hcl` is evaluated in full. At 1e6 iterations it takes 0.86 s on v0.3.0 and 0.69 s on HEAD, and each extra level multiplies the cost by 100, as in F1. This affects check, watch and the daemon | Same fix as F1 (2). Test: 5 x 100 in `enabled` and `skip_outputs`, under 1 s, edge facts unknown, no false GRT001. Cherry-pickable to v0.3.1 with 15-00 |
| F3 | LOW | `hclconv/limits_test.go:580-630`, rule at `limits.go:289-291` | Two mutants survive every hclconv test. (a) The floor: `max(ctl-1,0)` changed to `ctl-1`. T-15-00-3 says a test pins it, but none does. (b) `else` changed to decrement, which would reopen #261: nested `%{if a}%{else}` nets 0 per level. The code is correct today | Add reject cases for both `depthCheckers`: `nestTemplate("%{if a}%{else}", "x", "%{endif}", deep)`, and `strings.Repeat("%{endif}", 5)` followed by `deep` nested ifs |
| F4 | INFO | `cmd/gruntled/blast_bound_test.go:165` | The BLAST-11 RSS bound is control + 64 x typeBytes (about +256 MiB), and the measured big run stayed at or under the control. It catches per-unit copies of the type string (5,000 x 4 MiB would be about 20 GB) but nothing smaller. Accepted in #311. Self-correction: plan review round 4 (#280/#281) checked recursion only and accepted T-15-02-5, which is how F1 got through | None required |

## Crash repros (binary from 2e6c6f8; check / blast --base / blast --base --format json)

| Shape (bus ref) | Size | Result |
|-----------------|------|--------|
| `.tf.json` type `list(`x400k (#245) | 2.4 MB | rc 0, 0.07 s, 17 MB, silent |
| `.tf.json` type `!`x2M, `-`x2M, `a?b:`x1M (#260) | 2-4 MB | rc 0, 0.06-0.12 s, up to 25 MB |
| `.tf.json` nested-if `optional()` default x260k (#260) | 3.9 MB | rc 0, 0.13 s, 28 MB |
| native `.tf` nested-if `optional()` default x270k (#260) | 4.05 MB | rc 0, 0.65 s, 657 MB, `module-file-too-deep` |
| `terragrunt.hcl` nested `%{if}` x270k (#261) | 4.05 MB | rc 0, 0.51-0.68 s, 731 MB, `config-too-deep` (v0.3.0: stack overflow) |
| `%{<NL>for`, `%{/*c*/if`, `%{#c<NL>if`, `%{~/**/if` in terragrunt.hcl / module local / `optional()` default (#271) | about 4 MB each | 36 runs, rc 0, 0.49-0.73 s, up to 740 MB |
| `.tf.json` type `a?b:<NL>`x690k (#272) | 4.14 MB | rc 0, 0.15 s, 29 MB |
| `.tf.json` heredoc with nested `%{if}` x250k and x3k (round 4) | 3.75 MB / 45 KB | rc 0, 0.16 s / 0.01 s |
| `.tf.json` `sensitive`: nested `%{if}` x270k, `${!`x2M`}`, `${a?b:<NL>`x690k`}` (round 4) | 2-4 MB | rc 0, 0.08-0.17 s, up to 22 MB |
| native `.tf` `sensitive` nested `%{if}` x270k (round 4) | 4.05 MB | rc 0, 0.84-1.09 s, 730 MB, too deep |
