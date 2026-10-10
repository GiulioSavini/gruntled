---
phase: 15-type-level-surface-facts
plan: 00
subsystem: hclconv
tags: [security, dos, stack-overflow, template-directives, cherry-pick]
requires: []
provides:
  - "CheckNativeDepth counts nested %{if}/%{for} template control blocks"
  - "loader and surface-reader regressions for the sec #261 and #271 repros"
affects: [15-02]
tech-stack:
  added: []
  patterns: ["over-count on doubt in the lexer-based depth pre-scan"]
key-files:
  created:
    - internal/infrastructure/terragrunt/limits_regression_test.go
    - internal/infrastructure/tfsurface/limits_regression_test.go
  modified:
    - internal/infrastructure/hclconv/limits.go
    - internal/infrastructure/hclconv/limits_test.go
    - internal/infrastructure/terragrunt/fuzz_test.go
key-decisions:
  - "controlDepth takes the first token after the %{ that is not a comment or newline; endif/endfor close one block (floored at 0), else is neutral, anything else opens one"
  - "The code commit is fix(15-00) rather than feat: it closes a pre-existing v0.3.0 crash and may ship as v0.3.1"
  - "The three code commits touch only the plan's five files; this SUMMARY is a separate docs commit (sec #273)"
requirements-completed: []
duration: 15min
completed: 2026-10-10
---

# Phase 15 Plan 00: Template Directive Depth Summary

**The native depth pre-scan now counts open `%{if}`/`%{for}` template control blocks. On v0.3.0, a ~4 MB file of nested directives killed check, report, watch and the daemon with a fatal stack overflow (sec #261). The four keyword-after-trivia forms from sec #271 are covered too. A hostile unit becomes config-unknown (`config-too-deep`) and a hostile module gets an unknown surface (`module-file-too-deep`), each in under a second. The three code commits cherry-pick cleanly onto v0.3.0 and pass its tests there.**

## Performance

- Duration: ~15 min
- Tasks: 2
- Files: 2 created, 3 modified

## Accomplishments

- **limits.go:**
  - `ctl` counter (:178), updated at every `TokenTemplateControl` (:186) and added to the depth sum compared with MaxNestingDepth (:245).
  - `controlDepth` (:259): it skips TokenComment and TokenNewline (:261); `endif`/`endfor` → `max(ctl-1, 0)` (:267); `else` is neutral (:268); anything else → `ctl+1`, including a missing token.
  - The directive's own frame push and pop are unchanged.
  - The doc comment gains a fourth bullet: the rule, the evaluation recursion and the #261/#271 history.
- **TestCheckNativeDepthTemplateControl** (limits_test.go:568):
  - Rejects at 1,001 levels: nested if, nested for, the `~` strip form, mixed if/for, the heredoc form, and the sec #271 forms `%{/*c*/if`, `%{#c<NL>if`, `%{<NL>if`, `%{~/**/if` and `%{<NL>for`.
  - Accepts: 990 nested ifs, 50,000 sibling if/else/endif blocks, a realistic multi-line heredoc template, a stray `%{endif}` before 990 ifs (the floor at 0), and `%%{if}` × 1,001. A lexer check asserts `%%{` produces no TokenTemplateControl.
  - All existing CheckNativeDepth tests are unchanged and green.
- **TestNestedTemplateDirectiveUnit** (terragrunt, `fstest.MapFS` built in-test):
  - Hostile live/a variants: 270,000 nested `%{if a}` in `config_path` (4,050,042 bytes), 1,001 nested ifs, sec #271's `"%{<NL>for x in[1]}"` × 170,000 (4,080,042 bytes), and a heredoc with 150,000 nested `%{for}` (3.6 MB).
  - In each, live/a is config-unknown with `config-too-deep`. live/b is unaffected. live/c keeps its dependency on live/b and its reference to the missing output, so its GRT001 facts are unchanged.
  - Times: 0.71 s, 1.2 ms, 0.56 s, 0.37 s; the bound is 10 s.
- **TestNestedTemplateDirectiveModule** (tfsurface): the 270,000 nested-if and the 170,000 newline-for `default` strings give `module-file-too-deep`; a sibling module keeps its surface. Times: 0.67 s, 0.65 s.
- **FuzzLoadUnits** gains a seed with 1,100 nested `%{if a}` in a `config_path`.

### Red evidence

With limits.go restored to its pre-fix version (from c1440d7):
- `TestNestedTemplateDirectiveUnit/270,000 nested if` died with `runtime: goroutine stack exceeds 1000000000-byte limit` / `fatal error: stack overflow`. This is the v0.3.0 crash.
- `TestNestedTemplateDirectiveModule/170,000 newline-for` failed its assertion without crashing: the reader did not report module-file-too-deep.
- All ten reject cases of TestCheckNativeDepthTemplateControl failed (commit c1440d7).

## Task Commits

The code commits touch only the plan's files:
1. `c1440d7` test(15-00): nested template control blocks count toward the native depth (limits_test.go)
2. `dfe565b` fix(15-00): count nested template control blocks in CheckNativeDepth (limits.go)
3. `3279bec` test(15-00): nested template directives through the unit loader and the surface reader (fuzz_test.go, both limits_regression_test.go)

This summary is its own `docs(15-00): summary` commit.

## Verification

```
ok  	github.com/GiulioSavini/gruntled/cmd/gruntled	10.119s
ok  	github.com/GiulioSavini/gruntled/internal/application/blasting	0.460s
ok  	github.com/GiulioSavini/gruntled/internal/application/checking	0.003s
ok  	github.com/GiulioSavini/gruntled/internal/application/indexing	0.003s
?   	github.com/GiulioSavini/gruntled/internal/application/ports	[no test files]
ok  	github.com/GiulioSavini/gruntled/internal/application/watching	0.005s
ok  	github.com/GiulioSavini/gruntled/internal/domain/analysis	0.180s
ok  	github.com/GiulioSavini/gruntled/internal/domain/diagnostic	0.006s
ok  	github.com/GiulioSavini/gruntled/internal/domain/impact	1.016s
ok  	github.com/GiulioSavini/gruntled/internal/domain/repograph	0.003s
ok  	github.com/GiulioSavini/gruntled/internal/infrastructure/hclconv	2.264s
ok  	github.com/GiulioSavini/gruntled/internal/infrastructure/ipc	0.059s
ok  	github.com/GiulioSavini/gruntled/internal/infrastructure/sourceresolve	0.004s
ok  	github.com/GiulioSavini/gruntled/internal/infrastructure/statusfile	0.079s
ok  	github.com/GiulioSavini/gruntled/internal/infrastructure/terragrunt	3.011s
ok  	github.com/GiulioSavini/gruntled/internal/infrastructure/tfsurface	2.138s
ok  	github.com/GiulioSavini/gruntled/internal/infrastructure/watch	1.282s
ok  	github.com/GiulioSavini/gruntled/internal/interfaces/presenter	0.045s
ok  	github.com/GiulioSavini/gruntled/internal/testsupport/synthrepo	0.046s
ok  	github.com/GiulioSavini/gruntled/scripts/archscan	0.053s
VET-OK
architecture: OK (4 domain packages, 5 application packages, 1 interfaces packages)
all architecture self-tests passed
VET-OK
architecture: OK (4 domain packages, 5 application packages, 1 interfaces packages)
all architecture self-tests passed
```

`bash scripts/compare-ref.sh v0.3.0`: `244/244 identical`.

### Cherry-pick onto v0.3.0 (sec #273)

The check ran in a detached worktree of v0.3.0 (e870e29): cherry-pick c1440d7, dfe565b and 3279bec, run the tests, remove the worktree. No branch was pushed or kept.

```
 3 files changed, 128 insertions(+)
 create mode 100644 internal/infrastructure/terragrunt/limits_regression_test.go
 create mode 100644 internal/infrastructure/tfsurface/limits_regression_test.go
cherry-pick rc=0
217a9b1 test(15-00): nested template directives through the unit loader and the surface reader
c4df923 fix(15-00): count nested template control blocks in CheckNativeDepth
20f7fdf test(15-00): nested template control blocks count toward the native depth
e870e29 chore: remove REQUIREMENTS.md for v0.3 milestone
ok  	github.com/GiulioSavini/gruntled/internal/infrastructure/hclconv	5.397s
ok  	github.com/GiulioSavini/gruntled/internal/infrastructure/ipc	0.079s
ok  	github.com/GiulioSavini/gruntled/internal/infrastructure/sourceresolve	0.005s
ok  	github.com/GiulioSavini/gruntled/internal/infrastructure/statusfile	0.095s
ok  	github.com/GiulioSavini/gruntled/internal/infrastructure/terragrunt	6.627s
ok  	github.com/GiulioSavini/gruntled/internal/infrastructure/tfsurface	5.019s
ok  	github.com/GiulioSavini/gruntled/internal/infrastructure/watch	1.055s
v030 tests rc=0
?   	github.com/GiulioSavini/gruntled/internal/application/ports	[no test files]
/home/giulio/gruntled  3279bec [master]
```

All three commits applied without conflicts. The internal/infrastructure tests pass on v0.3.0 plus the picks, and so does the whole `go test ./...` there; its only non-`ok` line is the `ports` package, which has no test files. The worktree was removed, and `git worktree list` shows only the main tree.

`-race` was not run locally: there is no gcc and no cgo. CI runs it.

## Threat Mitigations

| ID | Disposition | Where |
|----|-------------|-------|
| T-15-00-1 | mitigated | limits.go:178/:186/:245 (ctl in the depth sum), :259 controlDepth; TestCheckNativeDepthTemplateControl (limits_test.go:568); TestNestedTemplateDirectiveUnit and TestNestedTemplateDirectiveModule (quoted and heredoc forms) |
| T-15-00-2 | accepted | sibling blocks never accumulate (50,000-sibling accept case); only more than 1,000 nested blocks are refused, as an existing unknown reason |
| T-15-00-3 | mitigated | limits.go:267 (floor at 0); the stray-endif accept case; the `%%{` accept case plus the lexer assertion that it yields no TokenTemplateControl |
| T-15-00-4 | accepted | lexing cost is pre-existing; measured 0.37-0.71 s per 3.6-4.08 MB file |
| T-15-00-5 | mitigated | 15-RESEARCH section 10 audit (spot-checked by sec #273); this plan covers the one escape it found |
| T-15-00-6 | mitigated | limits.go:261 (comment and newline tokens skipped before the keyword); anything but endif/endfor/else increments; the four sec #271 forms at 1,001 levels and the 4.08 MB repro through LoadUnits and ReadSurface |

## Deviations from Plan

**1. [Naming] Code commit type**
- The implementation commit is `fix(15-00)`, not `feat(15-00)`, because it closes a pre-existing crash and may become v0.3.1.

**2. [Observation] The tfsurface red was a wrong result, not a crash**
- Before the fix, the 170,000 newline-for module case failed its assertion (no module-file-too-deep) without crashing, while the loader case crashed. Both are red before the fix and green after.

## Known Risks / Unverified

- `-race` not run locally (no gcc).

## Self-Check: PASSED
