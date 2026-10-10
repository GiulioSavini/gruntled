---
phase: 12-gap-closure-watcher-directory-ignore-and-v0-3-audit-findings
plan: 04
subsystem: presenter-output
tags: [presenter, terminal-escaping, json, sarif, blast, watch, security]
requires:
  - phase: 09
    provides: BlastText / BlastJSON
  - phase: 11
    provides: report snapshot rendered through the shared presenter (check/report parity)
provides:
  - escapeTerm(s string) string: visible escapes for C0/DEL/invalid UTF-8 (\xNN) and C1/Cf/U+2028/9 (\uNNNN, \UNNNNNNNN)
  - escapeJSON(b []byte) []byte: lossless \uXXXX post-pass for DEL, C1, Cf, U+2028/9 on encoding/json output
  - SanitizeReason also maps Cf, Zl, Zp to a space
  - TestTextOutputsEscapeControls end-to-end proof on check, watch, report, blast (+ --base) and the four JSON documents
affects: [12-05]
tech-stack:
  added: []
  patterns: [escape at the shared presenter, not at call sites; zero-alloc fast path returning the input]
key-files:
  created:
    - internal/interfaces/presenter/escape.go
    - internal/interfaces/presenter/escape_test.go
    - cmd/gruntled/escape_test.go
  modified:
    - internal/interfaces/presenter/text.go
    - internal/interfaces/presenter/blast.go
    - internal/interfaces/presenter/json.go
    - internal/interfaces/presenter/sarif.go
    - internal/interfaces/presenter/graph.go
    - internal/interfaces/presenter/status.go
    - internal/interfaces/presenter/status_test.go
    - internal/interfaces/presenter/presenter_test.go
    - internal/interfaces/presenter/blast_test.go
    - docs/cli.md
key-decisions:
  - "Both helpers return their input unchanged (no allocation) when nothing needs escaping, also for non-ASCII names, so normal output is byte-identical"
  - "escapeJSON copies a non-UTF-8 byte unchanged (encoding/json never writes one) instead of guessing an escape"
  - "isFormatOrSeparator (Cf, U+2028, U+2029) is the one predicate shared by escapeTerm, escapeJSON and SanitizeReason"
requirements-completed: [DAEMON-04, BLAST-01]
duration: 12min
completed: 2026-10-10
---

# Phase 12 Plan 04: Terminal Escaping Summary

**Repository-controlled strings can no longer drive the terminal. Every text path (check, `report` text, `watch` stdout, blast text including the `--base` label) prints C0, DEL, C1, invalid UTF-8, bidi/zero-width (Cf) and U+2028/9 as visible escapes. The four JSON documents (check json, SARIF, graph --json, blast json) write the runes encoding/json leaves raw as `\uXXXX`, and they decode to the same values. Output for normal names is byte-identical: no golden or testscript changed. Closes v0.3-sec LOW-3 (bus #8), 09-sec#2, 09-sec#3 (bus #16), 11-sec#1, 11-sec#2 and sec #38.**

## Performance

- Duration: ~12 min
- Tasks: 3
- Files: 3 created, 10 modified

## Accomplishments

- `escapeTerm` (escape.go:36) scans bytes and returns `s` when nothing needs escaping. Printable ASCII is skipped bytewise and other runes are decoded. The slow path writes invalid bytes, C0 and DEL as `\xNN`, and C1, Cf, U+2028 and U+2029 as `\uNNNN`, or `\UNNNNNNNN` above U+FFFF. Hex is lowercase and a backslash is copied verbatim.
- `escapeJSON` (escape.go:108) has a fast path when no byte is >= 0x7f, or when no decoded rune is in the escape set. In that case it returns `b` itself. Otherwise it rewrites DEL, C1, Cf and U+2028/9 as `\uXXXX`, using a `utf16.EncodeRune` surrogate pair above U+FFFF.
- Wiring: Text (text.go:28,36,39) and BlastText (blast.go:82,93,98,106,117,119,142) cover the label, subject, file, message, unit, module and change names. `w.Write(escapeJSON(b.Bytes()))` is in json.go:124, sarif.go:333, graph.go:178 and blast.go:195. Check (render.go), report (snapshot bytes) and watch stdout (watch.go:248) inherit the escaping without any cmd change.
- SanitizeReason (status.go:71) maps `unicode.IsControl(r) || isFormatOrSeparator(r)` to a space.
- docs/cli.md: a "Control characters" paragraph and escape table under Text (default), plus the backslash caveat that points to JSON/SARIF for exact bytes, and a note that the same escaping applies to watch stdout, report text and blast text. Blast text says the baseline label is escaped. JSON has a sentence on `\uXXXX` that covers Graph JSON, Blast JSON and SARIF.
- Red output, for the record:
  ```
  daaacb7  undefined: escapeTerm / escapeJSON (build failure); with escape_test.go moved aside:
           --- FAIL: TestSanitizeReason/format_and_separators
             SanitizeReason("a‮b​c d") = "a‮b​c d", want "a b c d"
           --- FAIL: TestSanitizeReason/bidi_isolate_and_bom
  8cce242  --- FAIL: TestBlastTextEscapesControls      output holds raw U+001B
           --- FAIL: TestTextEscapesControls           output holds raw U+001B
           --- FAIL: TestBlastJSONEscapesTerminalRunes output holds raw U+009B
           --- FAIL: TestJSONEscapesTerminalRunes      output holds raw U+009B
           --- FAIL: TestSARIFEscapesTerminalRunes     output holds raw U+009B
           --- FAIL: TestGraphEscapesTerminalRunes     output holds raw U+009B
  e2e on the pre-fix tree (8cce242, temp worktree):
           --- FAIL: TestTextOutputsEscapeControls
             escape_test.go:94: check: stdout holds raw U+001B
  ```
- Fuzz (bounded, `-fuzztime 20s`, corpus left in GOCACHE and not committed): FuzzEscapeTerm ran 248296 execs and PASSED. FuzzEscapeJSON ran 141564 execs and PASSED. Neither found a crasher.

## Task Commits

1. Task 1 red: escapeTerm/escapeJSON/SanitizeReason tests + fuzz targets, `daaacb7`
2. Task 1 green: escape.go + SanitizeReason, `08f0c41`
3. Task 2 red: presenter text + JSON encoder escape tests, `8cce242`
4. Task 2 green: Text/BlastText escapeTerm, escapeJSON post-pass on the four encoders, `1508fbf`
5. Task 3: e2e TestTextOutputsEscapeControls, `1329b18`
6. Task 3: docs/cli.md, `22dcb3a`

## Verification

- `go vet ./...`: ok
- `GOOS=windows go vet ./...`: ok
- `go test -count=1 ./...`: every package ok (cmd/gruntled 7.5s, presenter 0.01s, watch 1.2s, ...)
- `go test -count=1 -run 'TestTextOutputsEscapeControls|TestHelpMatchesDocs|TestReadme|Doc' ./cmd/gruntled/`: ok. All 7 e2e subtests pass: watch, report, blast, check_json, check_sarif, graph_json, blast_json.
- `go test -count=1 -run 'Golden|TestScripts|Corpus|Sarif|E2E|Blast' ./cmd/gruntled/`: ok. **No golden, testscript or doc example changed.**
- `bash scripts/check-architecture.sh`: `architecture: OK (4 domain packages, 5 application packages, 1 interfaces packages)`, exit 0. The presenter imports only the stdlib (`unicode`, `unicode/utf8`, `unicode/utf16`, `strings`).

## Threat Mitigations

| ID | Disposition | Where |
|----|-------------|-------|
| T-12-04-1 | mitigated | escape.go:36 escapeTerm; text.go:28,36,39; blast.go:93-142. Tests: TestTextEscapesControls, TestBlastTextEscapesControls, and TestTextOutputsEscapeControls on check, watch, report and blast |
| T-12-04-2 | mitigated | escape.go:16 isFormatOrSeparator (Cf, U+2028/9), used by escapeTerm, escapeJSON and SanitizeReason (status.go:71). Tests: TestEscapeTerm, TestSanitizeReason |
| T-12-04-3 | mitigated | blast.go:82 `escapeTerm(baselineLabel)`. e2e: the first line is `baseline: <tmp>/base\x1b[2Jx` |
| T-12-04-4 | mitigated | escaping lives in the shared presenter. The e2e `report` subtest asserts that report text == check text, byte for byte |
| T-12-04-5 | accepted | backslash is not escaped. This is documented in docs/cli.md under Text (default) |
| T-12-04-6 | accepted | stderr echoes the user's own arguments. watch.go:185 is owned by 12-05 T2 and is untouched here |
| T-12-04-7 | mitigated | escapeJSON at escape.go:108, wired at json.go:124, sarif.go:333, graph.go:178 and blast.go:195. Tests: TestEscapeJSON, TestEscapeJSONRoundTrip, FuzzEscapeJSON, the per-encoder presenter tests, and the e2e JSON subtests (no raw 0x1b/0x07/0x7f/C1/U+202E, decoded names exact) |
| T-12-04-8 | accepted | Mn, Hangul fillers, non-ASCII Zs and private use are left as they are |

## Deviations from Plan

**1. [TDD order] The e2e test was committed after the Task 2 fix**
- Task 3's test (1329b18) comes after the presenter fix (1508fbf), as the plan orders the tasks. Its red run was made against the pre-fix tree 8cce242 in a temporary git worktree. The output is above: `check: stdout holds raw U+001B`.

**2. [TDD hygiene] The Task 1 red is a build failure**
- escapeTerm and escapeJSON are unexported and new, so the red commit does not compile. The SanitizeReason red was also shown as an assertion failure by running with escape_test.go moved aside.

**3. [Test fixture] The producer unit in the e2e repo has a crafted name (`v‮p`)**
- Blast's Impacted section lists only the producer whose surface changed. Giving it a bidi override puts a crafted unit and module name in Impacted (text and JSON) as well as in Broken.

**4. [Extra assertions]** The zero-alloc check also covers a non-ASCII name. SanitizeReason has a second case for an LRI/PDI/BOM/U+2029 mix. TestEscapeJSON has a case showing that a literal `\\u202e` (escaped backslash) is untouched.

## Known Risks / Unverified

- `-race` could not run locally (`go: -race requires cgo; enable cgo by setting CGO_ENABLED=1`). CI runs it.
- The e2e test skips on windows, because NTFS forbids control characters in names. The presenter unit tests run everywhere.
- During execution the orchestrator committed fa04e9d (ci + watch test deflake) on master. It touches none of these files.

## Self-Check: PASSED
