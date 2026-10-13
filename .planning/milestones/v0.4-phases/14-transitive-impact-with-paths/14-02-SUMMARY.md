---
phase: 14-transitive-impact-with-paths
plan: 02
subsystem: presenter
tags: [blast, json-v2, text, paths, escaping]
requires:
  - phase: 14-01
    provides: impact.Reach, Result.Changes, transitive Compute
provides:
  - blast JSON version 2 (changes[], impacted distance/source/via, broken distance/source/via)
  - blast text with distance and bounded, escaped paths
  - cmd blast goldens and docs/cli.md Blast text/JSON on the v2 shape
affects: [14-03, 15, 16]
tech-stack:
  added: []
  patterns: ["transitive entries reference changes[] by module, O(1) per entry", "text path elision 6/4"]
key-files:
  created: []
  modified:
    - internal/interfaces/presenter/blast.go
    - internal/interfaces/presenter/blast_test.go
    - cmd/gruntled/testdata/script/blast_impacted.txtar
    - cmd/gruntled/testdata/script/blast_grt004.txtar
    - cmd/gruntled/escape_test.go
    - cmd/gruntled/rules_doc_test.go
    - docs/cli.md
key-decisions:
  - "A distance-1 entry, or a hand-built entry with zero Reach, is rendered in the distance-1 shape (name lists in JSON, the token line in text); Compute always sets Reach"
  - "The text path walk stops at the source, at a missing Via, or after Distance units (the head limit when elided), so a malformed chain cannot loop"
  - "The 'only direct consumers' wording in blastUsage (main.go:71) and in the docs/cli.md blast intro stays for 14-03, whose Task 1 and Task 2 rewrite usage, its docs mirror and the intro bullet; 14-02 changes only the Blast text and Blast JSON output sections"
requirements-completed: []
duration: 25min
completed: 2026-10-10
---

# Phase 14 Plan 02: Blast Output Version 2 Summary

**Blast JSON is now version 2 and only adds keys. There is a top-level `changes[]`, impacted entries gain `distance`, `source` and `via`, and the four name lists appear only at distance 1. Each transitive entry is therefore O(1) bytes. Blast text shows the distance for every Impacted unit and one escaped shortest path for transitive units, at most 6 units long. The cmd goldens, the crafted-name end-to-end test and the docs output sections are moved to this shape, as the plan intended.**

## Performance

- Duration: ~25 min
- Tasks: 3
- Files: 0 created, 7 modified

## Accomplishments

- **blast.go JSON:**
  - `blastSchemaVersion = 2` (:16).
  - `blastChange`, plus `Distance`/`Source`/`Via` on `blastBroken` (set when the unit was reached, :264) and on `blastImpacted`.
  - The name lists are `*[]string` and are set only when `Reach.Distance <= 1` (:282-283).
  - `changes[]` is built from `Result.Changes` (:291).
  - Key order follows the interfaces block. The `escapeJSON` whole-buffer pass is kept (:309).
- **blast.go text:**
  - A distance-1 line is the v0.3 line plus `distance 1, ` (:155).
  - A distance-2-or-more line is `(distance d, from module m, path u -> ... -> source)`.
  - `writePath` (:187) follows Via through Impacted and reached Broken units. It is bounded by `textPathMaxUnits, textPathHead = 6, 4` (:179); the loop condition is at :194 and the stop at the source or a missing Via at :196.
  - Every hop goes through `escapeTerm(hop...)` (:206).
- **Presenter tests (blast_test.go):**
  - TestBlastJSONGolden and TestBlastTextGolden move on purpose to the `blastChain` fixture (live/a d1, live/b d2, live/c d3).
  - New tests: TestBlastJSONV1KeysKeepType (key presence and type only, sec #195), TestBlastJSONBrokenReach, TestBlastJSONLinear, TestBlastTextPathElision (6, 7 and 50 units, reversed input), TestBlastTextPathThroughBroken, TestBlastTextMalformedVia (a two-unit Via loop terminates), TestBlastTextLinear.
  - Extended: NoBaseline (`changes: []`), NilListsNeverNull (a distance-2 entry has exactly 5 keys), and the escape tests (crafted `source`/`via`, `changes[].module`, and a crafted transitive text line).
- **cmd goldens:**
  - blast_impacted.txtar: the two distance-1 lines, the live/edge line `(distance 2, from module modules/vpc, path live/edge -> live/db)`, and in JSON `"version": 2`, `changes`, and a live/edge entry ending at `"via": "live/db"` with no lists.
  - blast_grt004.txtar: the case 1 and case 2 Impacted lines, the case 2 live/app path, and case 10 `"version": 2`. The Broken GRT004/GRT001 lines are untouched.
- **escape_test.go (sec #194, #208a):**
  - New clean unit `deep`. Its `config_path` spells c1Unit with HCL `\u009b`/`\u007f` escapes.
  - Blast text has `  deep (distance 3, from module v\u202ep, path deep -> c\u009b2Jd\x7fe -> v\u202ep)` and `  v\u202ep (distance 1, module v\u202ep: -output old)`, with no raw controls. deep is asserted to be absent from Broken.
  - Blast JSON: the deep entry has distance 3, `via` decodes to the raw c1Unit and `source`/`module` to the raw producer, and deep is not in `broken`.
- **docs/cli.md:**
  - Blast text gets the new example (live/edge at distance 2), the path direction, the 6/4 elision rule, the tie-break, and the JSON-is-unambiguous sentence.
  - Blast JSON says "version 2". It has the "Readers must check `version`; a v1 reader must reject version 2." sentence, the `module` definition, `source`, `via`, the list presence rule and `changes[]`.
  - TestRuleRegistryDoc pins three new sentences and the transitive example line. The GRT004 example and the code-value sentence are unchanged.

## Task Commits

1. Task 1 red: `ab81414` test(14-02): blast JSON version 2 with changes, distance, source and via
2. Task 1 test fix: `c4131ce` test(14-02): fix the JSON golden's message escaping and the linear bound's digit growth
3. Task 1 green: `1df337c` feat(14-02): blast JSON version 2 (changes, distance, source, via)
4. Task 2 red: `e6145ae` test(14-02): blast text with distance and elided, escaped paths
5. Task 2 test fix: `51a61d4` test(14-02): count the Broken live/b line, not every substring
6. Task 2 green: `a6d6959` feat(14-02): blast text with distance and bounded, escaped paths
7. Task 3 tests: `81653ce` test(14-02): blast goldens, crafted transitive path and docs pins move to the v2 shape
8. Task 3 docs: `8c19749` docs(14-02): Blast text and Blast JSON describe the version 2 shape

## Verification

Full gate, real output tail. The suite, vet, check-architecture, test-check-architecture and compare-ref v0.3.0 all pass:

```
ok  	github.com/GiulioSavini/gruntled/cmd/gruntled	11.699s
ok  	github.com/GiulioSavini/gruntled/internal/application/blasting	0.721s
ok  	github.com/GiulioSavini/gruntled/internal/application/checking	0.005s
ok  	github.com/GiulioSavini/gruntled/internal/application/indexing	0.004s
?   	github.com/GiulioSavini/gruntled/internal/application/ports	[no test files]
ok  	github.com/GiulioSavini/gruntled/internal/application/watching	0.005s
ok  	github.com/GiulioSavini/gruntled/internal/domain/analysis	0.241s
ok  	github.com/GiulioSavini/gruntled/internal/domain/diagnostic	0.005s
ok  	github.com/GiulioSavini/gruntled/internal/domain/impact	1.510s
ok  	github.com/GiulioSavini/gruntled/internal/domain/repograph	0.008s
ok  	github.com/GiulioSavini/gruntled/internal/infrastructure/hclconv	2.768s
ok  	github.com/GiulioSavini/gruntled/internal/infrastructure/ipc	0.066s
ok  	github.com/GiulioSavini/gruntled/internal/infrastructure/sourceresolve	0.006s
ok  	github.com/GiulioSavini/gruntled/internal/infrastructure/statusfile	0.103s
ok  	github.com/GiulioSavini/gruntled/internal/infrastructure/terragrunt	1.942s
ok  	github.com/GiulioSavini/gruntled/internal/infrastructure/tfsurface	1.356s
ok  	github.com/GiulioSavini/gruntled/internal/infrastructure/watch	1.177s
ok  	github.com/GiulioSavini/gruntled/internal/interfaces/presenter	0.062s
ok  	github.com/GiulioSavini/gruntled/internal/testsupport/synthrepo	0.058s
ok  	github.com/GiulioSavini/gruntled/scripts/archscan	0.041s
VET-OK
architecture: OK (4 domain packages, 5 application packages, 1 interfaces packages)
all architecture self-tests passed
244/244 identical
-- vs 67c9ace
 cmd/gruntled/escape_test.go                       |  20 +-
 cmd/gruntled/testdata/script/blast_grt004.txtar   |  15 +-
 cmd/gruntled/testdata/script/blast_impacted.txtar |  19 +-
 internal/interfaces/presenter/blast_test.go       | 384 +++++++++++++++++++---
 4 files changed, 374 insertions(+), 64 deletions(-)
```

The testdata diff against 67c9ace (phase 13 end) is the two intended scripts, which carry both 14-01's and 14-02's edits:

```
 cmd/gruntled/testdata/script/blast_grt004.txtar   | 15 +++++++++------
 cmd/gruntled/testdata/script/blast_impacted.txtar | 19 +++++++++++++------
 2 files changed, 22 insertions(+), 12 deletions(-)
```

check and graph output are still byte-identical to v0.3.0 (244/244).

`-race` was not run locally: there is no gcc and no cgo. CI runs it.

## Threat Mitigations

| ID | Disposition | Where |
|----|-------------|-------|
| T-14-02-1 | mitigated | blast.go:206 (`escapeTerm(hop...)`), the source and module on the same line through escapeTerm, :309 (escapeJSON pass); TestBlastTextEscapesControls (blast_test.go:443), TestBlastJSONEscapesTerminalRunes (:460); escape_test.go deep line and JSON decode |
| T-14-02-2 | mitigated | blast.go:282 (lists only at distance 1), via only (O(1) per entry); :179/:194 (text path capped at 6 units); TestBlastJSONLinear (:293), TestBlastTextLinear (:613) |
| T-14-02-3 | mitigated | docs/cli.md:523 (Readers must check version), :535 (`module` definition), text "from module" (blast.go writePath caller); pinned in rules_doc_test.go; TestBlastJSONV1KeysKeepType (:224) promises key presence and type only |
| T-14-02-4 | mitigated | blast.go:194-196 (walk bounded by Distance, stops at the source or a zero Via); TestBlastTextMalformedVia (:596) |
| T-14-02-5 | mitigated | the path follows Via only, which is deterministic from 14-01; TestBlastTextPathElision (:538) renders twice and from reversed Impacted order |

## Deviations from Plan

**1. [Rule 1 - test bug] Two red-test fixes before green**
- `c4131ce`: the hand-written JSON golden had doubled backslashes in the GRT001 message, and the linear bound did not allow for digit growth when the chain doubles (2,000 to 4,000 units adds a digit to names). The bound is now `2*b2 + 3*4000`, and the per-entry cap stays at 250 bytes.
- `51a61d4`: TestBlastTextPathThroughBroken counted every substring "  live/b" (the finding line contains it), not the Broken subject line.
- Neither fix changes what is asserted about behaviour.

**2. [Scope] The "only direct consumers" wording is not changed here**
- 14-02 limits docs edits to the Blast text and Blast JSON output sections and says the usage mirror is 14-03's. 14-03 Task 1 rewrites blastUsage, including that sentence and its docs mirror, and Task 2 rewrites the docs/cli.md blast intro bullet and Known limitations. So main.go:71 and the intro bullet still say "only direct consumers" until 14-03.

**3. [Golden, intended] blast_grt004 case 10 version line**
- Case 10 asserted `"version": 1`. It now asserts `"version": 2`, and its comment changed accordingly. This is the version bump the plan intends, but the plan's list of blast_grt004 edits does not name this line.

## Known Risks / Unverified

- `-race` not run locally (no gcc).
- Blast usage (`blast -h`) still says only direct consumers are listed (14-03).

## Self-Check: PASSED
