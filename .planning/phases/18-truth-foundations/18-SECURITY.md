---
phase: 18-truth-foundations
audited: 2026-10-17
auditor: gruntled-sec (plan-review #484-#490 on ccd76c1; code-audit #510-#512 of ccd76c1..a3578cc)
scope: "git diff ccd76c1..a3578cc -- ':!.planning'"
verdict: SECURED
threats_total: 24
threats_mitigated: 14
threats_accepted: 10
threats_na: 0
threats_open: 0
findings: { blocker: 0, high: 0, medium: 0, low: 1, info: 1 }  # LOW #510 fixed after the audit (see Follow-up)
---

# Phase 18 Security

## Plan-review (ccd76c1, bus #484-#490)

Verdict **SECURED (0 blocking)**: 1 MEDIUM, 4 LOW, 2 INFO, all folded into the plans before execution.

| # | Sev | Where | Finding | Resolution |
|---|-----|-------|---------|------------|
| #484 | MEDIUM | 18-02 | Prose pins in `docs/ci.md` (lines 7, 89) matched no checker row | All-tokens rule over `docs/ci.md` with an exact-line allowlist for action pin comments; negative drifting line 7 only (`version_doc_test.go`) |
| #485 | LOW | 18-03 | ci.yml dry run accepted any non-zero exit | `set +e`, `rc -eq 1`, `grep -F` of the exact stderr; pinned by TestCIDoc |
| #486 | LOW | 18-01 | No length bound on the build-info version; output-injection threat implicit | `len > 64` rejected (`version.go:41,74`), rows for overlong/control-char/non-ASCII suffixes (`version_test.go:60-63`) |
| #487 | LOW | 18-02 | `${{` scanner only on release.yml | One scanner (`expressionsByContext`) over release.yml and ci.yml, synthetic negative |
| #488 | LOW | 18-03 | TestChangelog had no negatives | Pure `checkChangelog` + 8 one-failure negatives |
| #489 | INFO | 18-02/03 | bash `[[ =~ ]]` locale dependence; argv-literal wording | `env: LC_ALL: C` on the guard step, `export LC_ALL=C` in `release-notes.sh`; comment reworded |

## Code-audit (ccd76c1..a3578cc, bus #510-#512)

Verdict **SECURED (0 blocking)**: 0 BLOCKER, 0 HIGH, 0 MEDIUM, 1 LOW, 1 INFO.

- **#510 LOW** (`docs/cli.md:87,95`; `version_doc_test.go:35,83`). Two version tokens in `docs/cli.md`
  (`gruntled v0.4.0 (1a2b3c4d5e6f)`, `go install ...@v0.4.0`) sit outside every pin rule because the
  all-tokens loop iterates `docs/ci.md` only. Residual of #484. Fix: extend the loop to `docs/cli.md`
  (and the README Install block with an allowlist), add a negative drifting `docs/cli.md:87` only.
- **#511 INFO** (`version_test.go:99`). `go test -race` not runnable locally (no cgo). The
  `version`/`readBuildInfo` swaps in `setVersion`/`setBuildInfo` are safe: zero `t.Parallel` in
  `cmd/gruntled`; CI's race job covers it.

### Evidence

- `go vet ./cmd/gruntled/` on linux and `GOOS=windows`: ok.
- `go test -count=1 -short -run 'Version|Golden|SARIF|E2E|Changelog|ReleaseNotes|CIDoc|Readme' ./cmd/gruntled/`: ok;
  full `TestVersion|TestResolveVersion|TestModulePath` including `TestVersionPlainBuild` (real toolchain): ok.
- `go list -deps ./cmd/gruntled` on linux/amd64, windows/amd64, darwin/arm64: no `net`, `net/*`,
  `os/exec`, `text/template`, `golang.org/x/sys/windows`. Full `bash scripts/check-architecture.sh`:
  `architecture: OK` (Step 9 six-target proof included).
- `release-notes.sh` probes under a non-C caller locale: `v1.2.3` selects `[1.2.3]`, not `[1.2.3-rc.1]`
  or `[11.2.3]`; `v1.2.3-`, `v1.2.3-ä`, tab, trailing space exit 2; no file created.
- Docs: `grep v0.2.0 docs/ README.md` empty; the "`go install` prints `dev (none)`" limitation replaced
  (`docs/ci.md:31`, `docs/cli.md:87-89`); CHANGELOG dates equal `.planning/MILESTONES.md`; `VERSION` is
  exactly `v0.4.0\n`.
- Resolver: ldflags precedence `version.go:67`; module-path gate `:70`; ASCII regex `:50`; `+`/pseudo
  rejection `:79`; bare `vcs` and `vcs.` gate `:85`; length cap `:41/:74`; commit literal `none` `:89`;
  one `versionInfo` feeds `main.go:154`, `render.go:24`, `instance.go:227` (`daemonInfo`);
  `setVersion` pins `readBuildInfo` (`main_test.go:152`).
- Workflows: release.yml guard order regex `:47` → VERSION `:51-55` → fetch `:56`; `env: LC_ALL: C`
  `:44-45`; notes step `:61-70` before proofs; `--notes-file "$RUNNER_TEMP/notes.md"` `:108`; `${{` only
  at `:20` (concurrency) and `:102` (GH_TOKEN); permissions unchanged. ci.yml dry run `:93-112`
  (`rc -eq 1`, `test ! -s`, exact stderr), no `gh release`/`git tag`/secrets, top-level `contents: read`.

### Threat rows

| Row | Disposition | Evidence |
|-----|-------------|----------|
| T-18-01 | CLOSED | `version.go:70` `Main.Path == modulePath`; "other module" row; TestModulePathMatchesGoMod |
| T-18-02 | CLOSED | `version.go:50,53,74-87`; TestVersionBuildInfoNoLeak; check goldens untouched |
| T-18-03 | CLOSED | `version.go:89` returns only `v` and literal `none` |
| T-18-04 | ACCEPTED | as planned |
| T-18-05 | ACCEPTED | as planned |
| T-18-06 | CLOSED | `go list -deps` on 3 targets clean; check-architecture OK |
| T-18-07 | CLOSED | `main_test.go:152` setVersion → setBuildInfo(nil,false); TestVersionPlainBuild |
| T-18-08 | ACCEPTED | transferred to Phase 23 (INST-07) |
| T-18-09 | CLOSED | `version_doc_test.go:71-74` non-vacuity, `:83-91` all-tokens, negatives `:177-231`; residual #510 fixed below |
| T-18-10 | ACCEPTED | partial; `versionPins` table `:26` is where Phase 23 adds rows |
| T-18-11 | ACCEPTED | `docs/ci.md` VERSION rule (v0.4.0 untagged until Giulio tags) |
| T-18-12 | CLOSED | `release.yml:47-55`; TestCIDoc "release guard" order |
| T-18-13 | CLOSED | `${{` only at release.yml `:20,:102`; scanner over both files + synthetic negative |
| T-18-14 | ACCEPTED | as planned |
| T-18-15 | ACCEPTED | as planned |
| T-18-16 | CLOSED | `release-notes.sh:24` regex → exit 2, `:42` fixed-length compare, `:49` fixed awk |
| T-18-17 | CLOSED | `release.yml:65-70` env vars only; pinned |
| T-18-18 | CLOSED | want `## [X.Y.Z] - ` `:33`; prerelease row; 11.2.3/rc.1 probe |
| T-18-19 | ACCEPTED | intended |
| T-18-20 | CLOSED | `release.yml:108` `--notes-file` + `--verify-tag`; no `--generate-notes` (pinned) |
| T-18-21 | ACCEPTED | as planned |
| T-18-22 | CLOSED | checkChangelog + 8 negatives; ci.yml `rc -eq 1` + exact stderr; TestCIDoc pins |
| T-18-23 | CLOSED | ci.yml permissions unchanged; step has no `gh release`/`git tag`/secrets/`${{` |
| #486 (plan-review) | CLOSED | `version.go:41,74`; rows `version_test.go:60-63` |

## Follow-up

- #510 LOW: fixed in the `test(18-02): cover docs/cli.md and README version tokens` commit
  (see 18-01/18-03 SUMMARY handoff on the bus); the all-tokens loop now covers `docs/cli.md`
  and the README Install block, with a negative drifting `docs/cli.md:87` only.
- #511 INFO: accepted; CI race job covers `cmd/gruntled`.
