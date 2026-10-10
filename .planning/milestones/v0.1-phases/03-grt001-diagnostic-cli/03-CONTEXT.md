# Phase 3: GRT001 Diagnostic & CLI - Context

**Gathered:** 2026-09-25
**Status:** Ready for planning
**Source:** Orchestrator decisions (user delegated all choices) on 03-RESEARCH.md open questions

<domain>
## Phase Boundary

`gruntled check [path]` runs end to end over the Phase 2 graph and reports GRT001 and GRT100
diagnostics deterministically, with a documented exit-code table, no network, no subprocesses,
no writes. Phase 4 (corpus experiment, benchmark) is out of scope.
</domain>

<decisions>
## Implementation Decisions

### DIAG-03 (locked): mock_outputs never suppress and never downgrade
- Adopt research Pattern 1's decision table as written. Rows 1-5 are silent (undeclared label,
  enabled not literally true, skip_outputs not literally false, unresolved or non-unit target,
  unknown target surface). Row 7 is GRT001 at SeverityError, created with `NewForUnit`.
- Mocks only change the message suffix, and only when masking at apply is certain from literal
  facts. Unknown mock facts drop the suffix and never add a diagnostic.
- Why: when a mock covers a removed output, Terragrunt's apply silently uses the mock value in
  production, so it is a real bug, not a false positive. Suppressing it would also make VALID-03
  impossible on the primary corpus (all 22 refs have this shape). ROADMAP SC2's corpus-pattern
  case must be a named test that asserts GRT001 plus the suffix. A companion test asserts that
  "no mocks at all" on a declared output gives nothing.
- Document the rule and the rejected alternatives (suppress, downgrade) in `docs/cli.md`.
- The analyzer lives in the domain (`internal/domain/analysis`, pure allowlist, no fmt).

### Phase 2 follow-ups are NOT in this phase
- Research Patterns 2-4 cover three more false-positive sources: the lazy-evaluation guard
  (try/can, ternary, `&&`/`||`, for-bodies), the stack-target guard, and include-target units.
  They are Phase 2 correctness bugs and are closed as Phase 2 gap-closure plans before Phase 2
  is verified. Phase 3 assumes the graph already has them.
- Additional arch rules: `binary-no-net-no-exec` and `interfaces-external-deps`, each with a
  self-test case asserting its rule name.
- PROJECT.md "Key Decisions" records the DIAG-03 rule and its reasoning, and the tech stack line
  says stdlib flag. Flags given after the path must work, or produce a tested usage error.

### CLI
- stdlib `flag`, not cobra: fewer deps, and no net or exec linked into the binary. Update
  PROJECT.md's stack line to say so.
- `gruntled check [--format text|json] [path]`, where path defaults to `.`.
- Exit codes per research Pattern 7: 0 clean, 1 at least one error diagnostic, 2 usage error,
  3 analysis could not run. The table goes in `gruntled check -h`, in `docs/cli.md`, and in a
  testscript per row.
- Text mode: stdout carries only diagnostics (`path:line:col: CODE message`) and the summary
  line goes to stderr. JSON carries the summary.
- Paths are repo-relative only. A test runs from two differently-named checkout dirs and
  compares the byte-identical stdout and exit code.
- CLI-05: a test proves no writes inside the analysed repo (snapshot after any chmod, research
  Pitfall 11). An arch rule proves the binary links no net or os/exec
  (`binary-no-net-no-exec`), with a self-test case.
- Presenters live in `internal/interfaces/...` (pure; they may use fmt and io.Writer).
  `cmd/gruntled` is the composition root only.

### Process
- The README is owned by another agent. After the CLI lands, make only a small edit to its
  Status and usage section to match the real CLI. No attribution.
- Author Giulio Savini, no AI attribution. Full local suite before every push. CI must be green.

### Claude's Discretion
- JSON shape, message wording beyond what is above, and the package split.
</decisions>

<deferred>
## Deferred Ideas
- SARIF, graph --json, GRT002-006 (including undeclared labels and cycles), daemon/watch, cache.
</deferred>

---
*Phase: 03-grt001-diagnostic-cli — context gathered 2026-09-25 by the orchestrator*
