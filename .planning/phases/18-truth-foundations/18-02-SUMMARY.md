---
phase: 18-truth-foundations
plan: 02
subsystem: release
tags: [version, docs, release-workflow, doc-pins, ci]
requires: []
provides:
  - "VERSION at the repository root: exactly `v0.4.0` plus one newline, the single version source"
  - "checkVersionPins(version, files) with its fixed (file, pattern) table, the all-tokens rule over docs/ci.md and non-vacuity failures"
  - "TestVersionFileShape, TestVersionPins, TestVersionPinsNegative (seven negatives)"
  - "docs/ci.md, docs/cli.md and README pinned to VERSION; README `## Install` section; the three --version shapes documented"
  - "release.yml guard: tag regex, then VERSION == $GITHUB_REF_NAME, then fetch/ancestor; LC_ALL=C"
  - "TestCIDoc: release guard text and order, step order subsequence, no `${{` in run blocks of release.yml and ci.yml"
affects: [18-03, 23-install-channels]
tech-stack:
  added: []
  patterns: ["doc pins proven by a pure checker with a non-vacuity rule and drifted-copy negatives", "workflow text pinned by plain line scanning, no YAML parser"]
key-files:
  created:
    - VERSION
    - cmd/gruntled/version_doc_test.go
  modified:
    - cmd/gruntled/ci_doc_test.go
    - docs/ci.md
    - docs/cli.md
    - README.md
    - .github/workflows/release.yml
key-decisions:
  - "The all-tokens rule (sec #484) runs over docs/ci.md only; the sole foreign version tokens there are the `# vX.Y.Z` comments after the three action pins, allowlisted by exact `uses: <action>@<sha> # <version>` text taken from the existing ciPins table, so a changed action version or a foreign token anywhere else fails"
  - "checkVersionPins lives in the test file: it is test-only code and nothing in the binary references it"
  - "expressionsByContext is one scanner for both workflows (sec #487); the expected non-run `${{` lines are listed per file so the scanner is proven to see them, and a synthetic workflow proves it catches block and inline run lines"
  - "VERSION bump rule written once in docs/ci.md (intro) and enforced by the guard: VERSION is bumped in the commit that gets tagged; no workflow creates a tag or release"
requirements-completed: [INST-02]
duration: 25min
completed: 2026-10-16
---

# Phase 18 Plan 02: VERSION, doc pins and the release guard Summary

**`VERSION` is the one place the shipped version is written (`v0.4.0`, untagged yet). A pure checker proves that every install snippet in docs/ci.md, README and the docs/cli.md `--version` example equals it, that every version-shaped token in docs/ci.md equals it unless it is one of the three action pin comments, and that each designated pattern still occurs (a renamed snippet is a failure, not a pass). Seven negatives on drifted copies show the checker reads the pins and not its argument. docs/ci.md has no `v0.2.0` left and no longer claims `go install` prints `dev (none)`; both docs describe the three shapes. release.yml refuses a tag that differs from VERSION before any fetch, proof or build, under `LC_ALL=C`, and TestCIDoc pins the guard text, its order, the step order and the absence of `${{` in every run block of release.yml and ci.yml.**

## Performance

- Duration: ~25 min
- Tasks: 3
- Files: 2 created, 5 modified

## Accomplishments

### Task 1: VERSION and the doc-pin checker (cmd/gruntled/version_doc_test.go)
- **VERSION:** `v0.4.0\n`, 7 bytes. `versionFileRe` (:21) accepts exactly `vX.Y.Z[-suffix]\n`; TestVersionFileShape rejects a missing newline, CRLF, two newlines, a leading space, a bare `0.4.0` and `v0.4`.
- **Table:** `versionPins` (:26) holds the six (file, pattern) rows of the plan; `versionToken` (:19) is the shared capture group. Phase 23 adds its rows here.
- **checkVersionPins** (:59): a file absent from the map -> `<file>: not provided` (once per file); zero matches -> `<file>: no occurrence of <pattern>` (:73); every match whose `v` differs -> `<file>: <matched text> pins <v>, VERSION is <version>`. Then the all-tokens rule over docs/ci.md (sec #484): every `v[0-9]+\.[0-9]+\.[0-9]+[-0-9A-Za-z.]*` token on every line must equal VERSION unless `allowedForeignToken` (:46) accepts it, and that accepts only the exact `uses: <action>@<sha> # <version>` text of the three `ciPins` rows. Failures are sorted. Verified before writing it: the only version tokens in docs/ci.md besides `v0.2.0` were `# v7.0.1`, `# v7.0.0` (twice each) and `# v4.38.2`, all on `uses:` lines.
- **TestVersionPinsNegative:** "one drifted pin" (`rev: v0.0.1`: 2 failures, the snippet row and the all-tokens rule, both naming `docs/ci.md` and `v0.0.1`), "drift in README" (1 failure), "pattern deleted" (:188, `tag=` lines removed -> exactly `docs/ci.md: no occurrence of`), "missing file" (`docs/cli.md: not provided`), "checker is not fooled by the VERSION value" (:204, version `v0.0.1` against the real files -> at least 6 failures; it gives 24), "prose token drifts (sec #484)" (:210, only line 7 edited -> exactly 1 failure `docs/ci.md:7: ... v0.0.1`), "action pin comments are the only foreign tokens" (:222, a `v7.0.1` outside a `uses:` line fails; `# v7.0.2` on the checkout line fails).
- RED at a2c194d: TestVersionFileShape failed (no VERSION); after VERSION, TestVersionPins listed 18 drifts (nine `v0.2.0` tokens, seven snippet rows, the missing README pin and the missing `gruntled vX.Y.Z (none)` line in docs/cli.md).

### Task 2: docs
- **docs/ci.md:** all nine `v0.2.0` -> `v0.4.0`; the intro gains the rule (:10-13): the pins are the content of `VERSION`, checked by a test, bumped in the commit that is tagged. The limitation paragraph is now: `go install` prints `gruntled v0.4.0 (none)` (tag from the module information, commit `none`), a plain `go build` prints `gruntled dev (none)`. `grep -c 'v0.2.0'` = 0, `grep -c 'dev (none)'` = 1, the old "prints `gruntled dev (none)` for" sentence is gone. `sha256sum -c --ignore-missing checksums.txt` untouched.
- **docs/cli.md:** three-line example `gruntled v0.4.0 (1a2b3c4d5e6f)` / `gruntled v0.4.0 (none)` / `gruntled dev (none)` with the prose from the plan (ldflags always win; `go install @v0.4.0`; `(devel)`, pseudo-version, `+dirty` or VCS stamping -> `dev`), plus "and the `version` the watch daemon advertises to `report`". `including one from` is gone.
- **README.md:** `## Install` inserted between `## Status` and `### Usage`, go-install line pinned by the checker, pointer to the releases page and docs/ci.md. The compat-v1 `v0.3.0` lines (:501-505) are untouched.
- TestVersionPins, TestVersionPinsNegative, TestCIDoc, TestReadmeDocumentsV03, TestHelpMatchesDocs, TestTopUsageMatchesDocs, TestSARIFDoc, TestDaemonDocs, TestRuleRegistryDoc: ok.

### Task 3: release guard (release.yml:38-59, ci_doc_test.go)
- **Guard step:** comment states the three checks and that the tag is read only from `$GITHUB_REF_NAME`; `env: LC_ALL: C` (:43-44, sec #489); body: regex (:46), `want=$(tr -d '[:space:]' < VERSION)` (:50), `if [ "$want" != "$GITHUB_REF_NAME" ]` -> `VERSION is '$want' but the tag is '$GITHUB_REF_NAME'; bump VERSION in the tagged commit`, exit 1 (:51-54), then `git fetch origin master` and the ancestor check (:55-59). Nothing else in release.yml changed; permissions unchanged (`contents: read` top level, `contents: write` on the job as before).
- **TestCIDoc "release guard"** (:245): the five texts plus `LC_ALL: C`, `index(regex) < index("< VERSION") < index("git fetch origin master")` (:268), and no `${{` in the step.
- **"no expression injection in run blocks"** (:276, sec #487): `expressionsByContext` (:335) classifies every `${{` line of a workflow as inside a `run: |`/`>` block body (deeper indent than the `run:` key, blank lines included) or an inline `run:` line, or outside. Both workflows must have zero inside; the outside lines must be exactly (release.yml) `group: release-${{ github.ref }}`, `GH_TOKEN: ${{ github.token }}` and (ci.yml) `group: ci-${{ github.ref }}`, `cancel-in-progress: ${{ github.event_name == 'pull_request' }}`, `runs-on: ${{ matrix.os }}`, `sarif_file: ${{ runner.temp }}/upload.sarif`. A synthetic workflow with one block-run and one inline-run injection and one env expression must give 2 inside / 1 outside.
- **"release step order"** (:309): `stepNames` (:369) of job `release` must contain `guard, proofs, build, smoke, publish` as an ordered subsequence and start with `guard`; 18-03 can insert `notes` after `guard` without touching the assertion.
- RED at 221c674: "release guard" failed on the four new strings and the order (`VERSION check at -1`); the other two subtests passed against the existing files, with the synthetic negative proving the scanner is not vacuous.
- **Local dry run** (guard body up to the fetch, extracted from release.yml into the scratchpad, `LC_ALL=C`, run from the checkout):

```
--- GITHUB_REF_NAME=v0.4.0
exit=0
--- GITHUB_REF_NAME=v0.5.0
VERSION is 'v0.4.0' but the tag is 'v0.5.0'; bump VERSION in the tagged commit
exit=1
--- GITHUB_REF_NAME=v0.4.0;x
tag 'v0.4.0;x' does not match vX.Y.Z[-suffix]
exit=1
--- GITHUB_REF_NAME=vä.1.1
tag 'vä.1.1' does not match vX.Y.Z[-suffix]
exit=1
```

## Task Commits

1. `a2c194d` test(18-02): add failing doc-pin tests against VERSION
2. `22e7acc` feat(18-02): add VERSION as the single version source
3. `c34afcd` docs(18-02): pin install snippets to VERSION and drop the go-install limitation
4. `221c674` test(18-02): pin the release VERSION guard, step order and no-expression-in-run rule
5. `db0c69a` ci(18-02): release guard requires VERSION to equal the tag

This summary is its own `docs(18-02): summary` commit.

## Gate (at db0c69a, with 18-01's 36ad782 also on master)

Run with `GOMAXPROCS=2 GOFLAGS=-p=2`:

```
gofmt -l cmd/gruntled/            (empty)
go vet ./...                      ok
go test -count=1 -short ./cmd/gruntled/
ok  	github.com/GiulioSavini/gruntled/cmd/gruntled	30.134s
go test -count=1 -run 'TestVersion|TestCIDoc|Doc|TestReadme' ./cmd/gruntled/
ok  	github.com/GiulioSavini/gruntled/cmd/gruntled	2.265s
go test -count=1 ./...            all 20 packages ok (cmd/gruntled 188.3s)
grep -rn 'v0.2.0' docs/ README.md (nothing)
grep -n '\${{' .github/workflows/release.yml
19:  group: release-${{ github.ref }}
91:          GH_TOKEN: ${{ github.token }}
bash scripts/check-architecture.sh
architecture: OK (4 domain packages, 5 application packages, 1 interfaces packages)
```

No `-race` locally (18-RESEARCH section 4); the only Go changes are `_test.go` files, which CI runs under `-race`.

## Deviations

- **checkVersionPins committed with the RED test.** The plan splits "tests" (RED) from "checker" (GREEN), but the checker is test-only code in the same file and the tests cannot compile without it; the RED commit a2c194d fails for the reasons the plan names (no VERSION, stale pins, no README pin), and 22e7acc adds only VERSION.
- **"one drifted pin" expects 2 failures, not 1.** With the sec #484 all-tokens rule a drifted `rev:` line is reported twice (snippet row and token rule); the subtest asserts both name `docs/ci.md`, `rev: v0.0.1` and `v0.0.1`. The README and docs/cli.md drifts, outside docs/ci.md, still give exactly 1.
- **Sec findings applied:** #484 (all-tokens rule + line-7 negative + explicit allowlist), #487 (one scanner over both workflows with the expected non-run occurrences listed), #489 (`LC_ALL: C`).
- No `question` was needed; 18-01's files were not touched (its in-progress working tree was excluded from every `git add` by path).

## Threat mitigations

| ID | Disposition | Where |
|----|-------------|-------|
| T-18-09 doc-pin vacuity | mitigate | version_doc_test.go:73 (zero matches is a failure); negatives "pattern deleted" :188, "checker is not fooled by the VERSION value" :204; sec #484 all-tokens rule :59-95 with "prose token drifts" :210 and "action pin comments are the only foreign tokens" :222 |
| T-18-10 stale pin in a future channel | mitigate (partial) / transfer | the table `versionPins` version_doc_test.go:26 is the one place to add a row; Phase 23 adds install.sh, formula and image rows |
| T-18-11 users install an untagged v0.4.0 | accept | docs/ci.md:10-13 rule; open risk for the orchestrator until Giulio tags v0.4.0 |
| T-18-12 release of the wrong version | mitigate | release.yml:50-54 guard before fetch/proofs/build; ci_doc_test.go:245-273 pins text and order; step order :309 |
| T-18-13 expression injection | mitigate | release.yml reads the tag only from `$GITHUB_REF_NAME` (no `${{` in any run block, lines 19 and 91 are the only ones); ci_doc_test.go:276-307 over release.yml and ci.yml, scanner :335 |
| T-18-14 who can trigger | accept | repository settings (HYG-06, Phase 24) |
| T-18-15 which commit is v0.4.0 | accept | the tag is the identity; release binaries print the 12-hex commit |

## Open items for the orchestrator

- `go install github.com/GiulioSavini/gruntled/cmd/gruntled@v0.4.0` fails for readers until the `v0.4.0` tag exists (T-18-11); the README and docs now name it.
- 18-03 inserts `notes` after `guard` and changes `publish`; the "release step order" subsequence check and the run-block scanner already allow that.
