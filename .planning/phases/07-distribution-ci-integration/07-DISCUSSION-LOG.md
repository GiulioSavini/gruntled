# Phase 7: Distribution & CI Integration - Discussion Log

**Date:** 2026-10-01
**Mode:** User unavailable interactively. Claude acted as user's proxy for this discuss-phase
session: generated 40 questions across 5 phase-specific gray areas and answered each itself,
grounded in PROJECT.md, REQUIREMENTS.md, ROADMAP.md Phase 7, prior phase `<decisions>` (Phases
1-6 CONTEXT.md files), and direct inspection of the code/CI/docs in `/home/giulio/gruntled`.
Every answer below is `[proxy]`. Items that materially change the task's own framing or that a
human should actively confirm are marked **FLAGGED**.

Areas: (A) `--version` / REL-02, (B) Release artifacts & build matrix / REL-01, (C) Release
workflow mechanics / REL-01+REL-02, (D) pre-commit integration / INT-03, (E) CI recipes
(GitHub Actions + GitLab) / INT-04.

---

## Area A — `--version` / REL-02

**A1. Where is `--version` dispatched?**
Options: (a) new arm in the existing top-level `switch args[0]` in `main.go`, next to `-h`;
(b) a new `flag.FlagSet` just for it.
**Answer:** (a). **Rationale:** `main.go` already has `case "-h", "-help", "--help", "help":` in
the top switch — a sibling `case "-version", "--version":` is the smallest possible diff and
matches the file's own established idiom exactly.

**A2. Output format?**
Options: (a) one line `gruntled <version> (<commit>)`; (b) multi-line with Go version/OS/arch/
build date.
**Answer:** (a). **Rationale:** REL-02 only asks for "release version and commit." The project's
whole tone (terse help text, no fluff) argues against inventing extra fields; a build date would
also make the same tag produce different `--version` strings on different build days, at odds
with the project's determinism culture.

**A3. What gets ldflags-injected?**
Options: (a) version + commit only; (b) version + commit + build date; (c) + "built by".
**Answer:** (a). **Rationale:** matches REL-02's literal wording; no unrequested fields.

**A4. Where do `version`/`commit` vars live (DDD placement)?**
Options: (a) `var`s in package `main`; (b) a new `internal/infrastructure/buildinfo` package.
**Answer:** (a). **Rationale:** this is build-metadata plumbing for the composition root, not
business logic — `cmd/gruntled` is documented as "composition root only," and a trivial var block
set by `-ldflags -X` doesn't need a dedicated internal package or port/adapter split.

**A5. Default values with no ldflags (local `go build`/`go install`)?**
Options: (a) `"dev"`/`"none"`; (b) `"unknown"`/`"unknown"`; (c) empty string + special message.
**Answer:** (a). **Rationale:** `"dev"` is already the hardcoded literal at the Phase 6 SARIF call
site (`main.go:191`) — reusing it means a locally-built binary's `--version` and its SARIF
`tool.driver.version` agree, and nothing already-tested changes by surprise.

**A6. Does `--version` feed `presenter.ToolInfo{Version: ...}` (the SARIF tool-driver version)?**
Options: (a) yes, same var; (b) no, keep SARIF's version independent.
**Answer:** (a). **Rationale:** Phase 6's own CONTEXT explicitly names this as the Phase 7 handoff:
*"For now main passes `dev`. Phase 7 replaces that value with the ldflags version and changes
nothing in the presenter."* Not doing this would leave that forward-reference unfulfilled.

**A7. Exit behavior / trailing args after `--version`?**
Options: (a) ignore trailing args, exit 0 always; (b) usage error if anything follows.
**Answer:** (a). **Rationale:** mirrors the existing `-h` arm, which doesn't inspect `args[1:]`
either — keeps the two "informational, not analysis" commands symmetrical.

**A8. Short alias `-v`?**
Options: (a) no short alias, only `-version`/`--version`; (b) also accept `-v`.
**Answer:** (a). **Rationale:** no command in this CLI has a short alias today (`-h`/`-help`/
`--help`/`help` are all spelled out); `-v` is better reserved for a possible future `--verbose`.

---

## Area B — Release artifacts & build matrix / REL-01

**B1. Exact target matrix?**
Options: (a) the 5 targets already used everywhere in this repo's CI/plans/docs (`linux/amd64`,
`linux/arm64`, `darwin/amd64`, `darwin/arm64`, `windows/amd64`); (b) literal ROADMAP/REQUIREMENTS
wording ("windows on amd64 and arm64") → 6 targets, adding `windows/arm64`.
**Answer:** (a). **FLAGGED. Rationale:** every single prior reference in this codebase — `ci.yml`'s
cross-build step, README's "Building, testing and checking locally," and over a dozen Phase 2/3
plan `<verify>` commands, including 03-02-PLAN.md's explicit *"for EVERY release target (linux/
amd64, linux/arm64, darwin/amd64, darwin/arm64, windows/amd64)"* — treats this 5-target list as
the complete, deliberate set. The ROADMAP/REQUIREMENTS prose almost certainly means "both
architectures, across linux and darwin, plus windows" in template-speak rather than a conscious
decision to add `windows/arm64`; nothing anywhere motivates that 6th target. Still, this reads the
requirement text narrower than it's literally written, so it needs a human nod.

**B2. CGO?**
Options: (a) explicit `CGO_ENABLED=0`; (b) leave default (which is already effectively 0 for pure-
Go deps on these targets, but implicit).
**Answer:** (a). **Rationale:** `hcl/v2` and `go-cty` are pure Go, so this changes no behavior —
but REL-01 says "static binary," and making `CGO_ENABLED=0` explicit in the workflow documents the
guarantee instead of relying on an accident of the dependency graph.

**B3. Archive format?**
Options: (a) `.tar.gz` for linux/darwin, `.zip` for windows; (b) `.zip` everywhere for uniformity.
**Answer:** (a). **Rationale:** per-OS convention (ripgrep, fd, terragrunt itself all do this) so a
Windows user never has to find a `tar` binary, and a Unix user gets the smaller/standard format.

**B4. Asset naming?**
Options: (a) `gruntled_<version>_<os>_<arch>.tar.gz|zip`; (b) `gruntled-<os>-<arch>` (no version in
filename).
**Answer:** (a). **Rationale:** version-in-filename is the standard Go release-tool convention and
avoids stale-cache collisions when re-downloading across versions.

**B5. Binary filename inside the archive?**
Options: (a) plain `gruntled`/`gruntled.exe`; (b) `gruntled-<os>-<arch>`/`.exe`.
**Answer:** (a). **Rationale:** the archive name already encodes os/arch; keeping the extracted
binary plainly named means every doc only ever needs one invocation, `./gruntled check`.

**B6. Checksums file shape?**
Options: (a) one `checksums.txt` in `sha256sum`-compatible format covering every archive; (b) a
`.sha256` sidecar per asset.
**Answer:** (a). **Rationale:** REL-01 says "a checksums file" (singular); one file lets a user run
`sha256sum -c checksums.txt` once instead of per-asset verification.

**B7. Build tooling?**
Options: (a) hand-rolled matrix build (shell loop or `strategy.matrix`) with plain `go build` +
ldflags, no new tool; (b) adopt goreleaser.
**Answer:** (a). **Rationale:** "CI simple but correct, few jobs, no fake checks" argues against
adding a new, fairly large build tool and its own config file format when the existing style
(`go run <pinned module>@<version>` one-liners, a raw shell loop already in `ci.yml`'s
`cross-build` step) already does 90% of what's needed with zero new dependencies.

**B8. How is the no-net/no-exec proof applied to the actual release binaries?**
Options: (a) reuse the existing `check-architecture.sh` Step 9 (`binary-no-net-no-exec`), which
already runs `go list -deps` per `GOOS`/`GOARCH` for exactly these 5 targets, by gating the
release job on `ci.yml` being green for that commit; (b) re-run/re-implement an equivalent check
inside `release.yml` against the built binaries themselves.
**Answer:** (a). **Rationale:** the proof is a source-level static check (not something that needs
the actual binary bytes), already exists, already covers exactly this target list, and is already
documented in `docs/cli.md`. Duplicating it in `release.yml` would be a second, divergent
implementation of the same guarantee.

---

## Area C — Release workflow mechanics / REL-01 + REL-02

**C1. Trigger?**
Options: (a) `push: tags: ['v*']`; (b) `release: types: [published]` (create the GitHub Release by
hand first, workflow attaches assets after); (c) `workflow_dispatch` only.
**Answer:** (a). **Rationale:** single linear workflow — push a tag, get a release with assets, no
separate manual "create release" step and no need to also wire a second trigger type.

**C2. Tag format?**
Options: (a) `vX.Y.Z`; (b) bare `X.Y.Z`.
**Answer:** (a). **Rationale:** Go-ecosystem convention (`go install pkg@vX.Y.Z`); project is pre-
1.0 so no `/vN` module-path concern yet.

**C3. Version string source for ldflags?**
Options: (a) `${{ github.ref_name }}` (the tag itself); (b) `git describe --tags` (can append
`-N-g<sha>`/`-dirty`).
**Answer:** (a). **Rationale:** exact and deterministic — the same tag always produces the same
string. `git describe` can vary by checkout state, which conflicts with this project's explicit
determinism culture ("same input, same output").

**C4. Commit string source/length?**
Options: (a) short SHA (7-12 chars); (b) full 40-char SHA.
**Answer:** (a). **Rationale:** keeps the one-line `--version` output scannable; a full SHA is
needless length for a human-read string.

**C5. Release-creation mechanism?**
Options: (a) `gh release create` (GitHub CLI, preinstalled on `ubuntu-latest`); (b) a third-party
action (`softprops/action-gh-release` or similar).
**Answer:** (a). **Rationale:** zero extra action to vet/pin/trust; one fewer third-party
dependency in a workflow that already has write permissions (`contents: write`), consistent with
"no fake checks / few dependencies."

**C6. Changelog approach?**
Options: (a) `gh release create --generate-notes` (auto-generated from merged PRs/commits); (b) a
hand-maintained `CHANGELOG.md`; (c) a Conventional-Commits-driven changelog generator tool.
**Answer:** (a). **Rationale:** the repo has never had a changelog file; GitHub's built-in notes
need zero new maintenance burden and zero new tooling, matching "CI simple but correct."

**C7. Action pinning strategy for the new `release.yml`?**
Options: (a) SHA-pin every action, including `actions/checkout`/`actions/setup-go` (going further
than `ci.yml`'s current mixed state); (b) match `ci.yml`'s existing mixed pattern (tag-pin official
actions, SHA-pin third-party ones only).
**Answer:** (a). **Rationale:** the task's explicit instruction is "SHA-pinned actions" as a user
preference; a release workflow with `contents: write` is exactly the kind of higher-stakes
pipeline where that preference should apply fully, even though it's stricter than `ci.yml`'s own
precedent. `ci.yml` itself is left untouched (its retrofit is a deferred idea, not silently done
as part of this phase).

**C8. Signing / SBOM / provenance (cosign, SLSA attestations, GitHub artifact attestations)?**
Options: (a) defer entirely, out of scope; (b) add GitHub's native artifact attestation (one extra
action) since it's low-effort; (c) add full cosign signing.
**Answer:** (a). **Rationale:** none of ROADMAP's Phase 7 success criteria, REL-01/REL-02's
wording, or REQUIREMENTS.md's "Out of Scope" table mention this. Per the scope guardrail, adding
it now — even the "low-effort" GitHub-native option — would be a new capability layered on a fixed
phase boundary, not a clarification of one already in scope. Captured as a deferred idea.

---

## Area D — Pre-commit integration / INT-03

**D1. Hook `id`/`name`?**
Options: (a) `id: gruntled-check`, `name: gruntled check`; (b) `id: gruntled` (bare).
**Answer:** (a). **Rationale:** names the actual subcommand being run and leaves room for a future
`gruntled-graph` hook without a naming clash.

**D2. Hook mechanism (`language`)?**
Options: (a) `language: golang`, `entry: gruntled check` — pre-commit's own Go-language support,
which `go install`s the binary from this repo at the pinned `rev`; (b) `language: system` pointing
at a pre-installed binary the user must provide themselves; (c) a custom script that downloads the
matching release archive from Area B.
**Answer:** (a). **Rationale:** this is pre-commit's standard, documented way of shipping a Go
tool as a hook (the same mechanism `golangci-lint`'s own `.pre-commit-hooks.yaml` uses) — no
platform-detection or download logic to write or maintain, and it tracks the exact pinned `rev`
automatically via `go install`.

**D3. File filtering — restrict to `*.hcl`/`terragrunt.hcl` changes, or run unconditionally?**
Options: (a) `always_run: true`, no `files:` regex, `pass_filenames: false`; (b) `files:
'(^|/)terragrunt\.hcl$'`, gated on `.hcl` changes only.
**Answer:** (a). **Rationale:** this is the one place a naive first instinct (filter to `.hcl`
changes) would be wrong. gruntled's entire core value (PROJECT.md: catching `dependency.X.
outputs.Y` references that no longer resolve) is triggered just as often by editing a module's
`.tf` file — e.g. renaming an `output` — as by editing a `terragrunt.hcl`. Filtering to `.hcl`
changes would silently skip exactly the bug class the tool exists to catch whenever only the
module side of the break changed. `pass_filenames: false` is required either way since gruntled
takes a directory, not a file list.

**D4. Default analysis path / configurability?**
Options: (a) default to repo root `.`, no hard-coded `args:`, document `args: [subdir/]` as an
override for monorepos; (b) hard-code a path in `.pre-commit-hooks.yaml` itself.
**Answer:** (a). **Rationale:** matches gruntled's own CLI default (`.`) and keeps the shipped hook
generic rather than opinionated about repo layout.

**D5. `--format` for the hook?**
Options: (a) default (`text`), no `--format` arg; (b) `--format json`.
**Answer:** (a). **Rationale:** pre-commit surfaces a hook's own stdout/stderr to a human at the
terminal; INT-03's acceptance wording ("`pre-commit run` fails on a broken unit") is a human-
facing check, not a machine-consumed pipeline stage.

**D6. `stages` key?**
Options: (a) leave at pre-commit's default (the `pre-commit`/commit stage only); (b) add `push`
and/or `manual` stages too.
**Answer:** (a). **Rationale:** nothing in INT-03 asks for push- or manual-stage behavior; adding
stages nobody requested is scope creep on the hook's own config surface.

**D7. Exit-code handling / translation inside the hook?**
Options: (a) none — rely on pre-commit treating any non-zero exit as "hook failed," document what
each of gruntled's exit codes (1/2/3) means in the recipe prose; (b) wrap `gruntled check` in a
script that normalizes exit 2/3 to something else.
**Answer:** (a). **Rationale:** all three non-zero cases (findings, usage error, internal failure)
should correctly fail the hook as-is — there's no case where a non-zero exit should be treated as
success. A wrapper script would add a moving part to explain a distinction that doesn't need
explaining in behavior, only in docs.

**D8. Versioning of the hook itself?**
Options: (a) users pin `rev: vX.Y.Z` in their own `.pre-commit-config.yaml`, same tag the release
workflow (Area C) creates; (b) a separate versioning scheme just for the hook file.
**Answer:** (a). **Rationale:** `.pre-commit-hooks.yaml` lives at repo root and pre-commit resolves
`rev` to any tag's commit automatically — no extra versioning mechanism needed, and it stays in
lockstep with the same releases REL-01 produces.

---

## Area E — CI integration recipes (GitHub Actions + GitLab CI) / INT-04

**E1. Where do the documented recipes live?**
Options: (a) new `docs/ci.md`, parallel to `docs/cli.md`/`docs/validation.md`; (b) inline in
README's existing "CI" section; (c) a separate `examples/` directory of raw YAML files.
**Answer:** (a). **Rationale:** matches the existing docs/ split-by-topic convention, and keeps the
user-facing recipe clearly distinct from README's "CI" section, which describes *this* repo's own
internal CI — a distinction Phase 6 already explicitly cared about (`sarif-upload`'s own code
comment: "this is an internal proof job, not the user recipe").

**E2. How does the GitHub Actions recipe install gruntled?**
Options: (a) `go install github.com/GiulioSavini/gruntled/cmd/gruntled@<tag>`; (b) download the
matching release archive from Area B and extract it; (c) build from a checked-out clone.
**Answer:** (a). **Rationale:** one line, no platform-detection/asset-name logic in the recipe
itself, always matches the tag exactly, and `ubuntu-latest` ships Go already — "simple but
correct" directly favors this over writing download/extract logic into a doc example.

**E3. SARIF or plain-text recipe, or both?**
Options: (a) document both — a minimal plain-text variant first, a SARIF+`upload-sarif` variant
second; (b) only the SARIF variant, since Phase 6 already built it; (c) only plain text.
**Answer:** (a). **Rationale:** a reader who just wants a CI check that fails the build shouldn't
have to understand `security-events: write` permissions, `category`, or the URI-prefix caveat; a
reader who wants GitHub code scanning integration gets the full SARIF path. Both are real,
documented options rather than forcing one.

**E4. Does the SARIF recipe need to address the URI-prefix caveat from `ci.yml`'s `sarif-upload`
job (STATE.md: "SARIF URIs relative to analysed dir; subdirectory analysis needs URI prefix or run
from repo root")?**
Options: (a) yes, state it plainly and give the same `jq` one-liner as a fallback; (b) leave it
undocumented and let users discover it themselves.
**Answer:** (a). **Rationale:** this is a real, already-encountered gap; carrying the exact Phase 6
finding forward avoids every adopter rediscovering it independently.

**E5. Does the GitHub Actions recipe actually have to run green in this repo's own CI (ROADMAP
success criterion #4), and if so, how?**
Options: (a) add a new `recipe-check` job to `ci.yml` that runs the documented minimal recipe
verbatim against this repo; (b) treat the existing `sarif-upload` job as sufficient proof; (c)
verify once by hand and don't encode it in CI at all.
**Answer:** (a). **Rationale:** ROADMAP's wording is literal ("runs green in this repository's own
CI"), and the existing `sarif-upload` job is explicitly an internal fixture proof, not the user
recipe — conflating them would blur a distinction Phase 6 deliberately drew. A dedicated job also
means any future doc drift (recipe changes but the job doesn't, or vice versa) is caught
automatically rather than silently.

**E6. Action pinning for the new `recipe-check` job and the `docs/ci.md` example YAML?**
Options: (a) SHA-pin, consistent with Area C's release.yml decision and the stated global
preference, even next to `ci.yml`'s existing tag-pinned jobs; (b) match `ci.yml`'s existing
tag-pinned style for consistency within that one file.
**Answer:** (a). **Rationale:** the explicit SHA-pinning preference applies to all new work in this
phase; the doc's own example YAML shown to third parties benefits from demonstrating the
best-practice pin style, not just matching this repo's partially-legacy `ci.yml`.

**E7. What does "a GitLab CI recipe is documented" require — a runnable, actually-green pipeline,
or a reference snippet?**
Options: (a) a documented reference `.gitlab-ci.yml` snippet only (`go install` + `gruntled
check`, fail on non-zero exit), no GitLab project provisioned; (b) stand up a disposable GitLab
project/runner to prove it green, matching the GitHub bar.
**Answer:** (a). **Rationale:** ROADMAP's own wording draws the line explicitly: GitHub gets "runs
green in this repository's own CI," GitLab gets only "a GitLab CI recipe is documented" — no
green-run requirement stated for GitLab. Standing up GitLab infrastructure to prove something the
requirement doesn't ask for would be scope creep in the other direction (over-delivering, still
outside the stated boundary).

**E8. Should the GitLab recipe include GitLab's own Code Quality report format (its SARIF
analogue, `gl-code-quality-report.json`)?**
Options: (a) out of scope, defer — document only a plain pass/fail job; (b) build a GitLab
Code-Quality presenter alongside the existing SARIF one.
**Answer:** (a). **Rationale:** INT-04's wording only asks for a documented recipe, with no mention
of a GitLab-native report format; adding one would mean a new output-format presenter — the same
kind of new capability the scope guardrail reserves for its own phase, not a clarification of this
one.

---

## Summary of flagged items (need explicit human confirmation)

1. **Repo visibility.** The task's premise ("repo on GitHub is PRIVATE, blocks Code Scanning") is
   factually wrong as of now — `gh repo view` shows `GiulioSavini/gruntled` is **public**, and
   STATE.md records it was made public during Phase 6 (06-05) specifically for code scanning. All
   Phase 7 decisions above assume public/unblocked. If this is a surprise, it needs a look before
   planning proceeds on these assumptions.
2. **Release target matrix (5 vs 6).** Decided to match the established 5-target precedent
   (`windows/amd64`, no `windows/arm64`) over ROADMAP/REQUIREMENTS' literal "amd64 and arm64"
   wording for all three OSes. This is a plausible reading of loose template prose, not a certainty
   — worth one explicit nod before Phase 7 planning locks the release matrix.

