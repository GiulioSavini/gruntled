# Phase 7: Distribution & CI Integration - Context

**Gathered:** 2026-10-01
**Status:** Ready for planning
**Mode:** User unavailable. Claude acted as user's proxy: generated 40 questions, answered each
with the best-justified choice grounded in PROJECT.md, REQUIREMENTS.md, ROADMAP.md, prior phase
CONTEXT.md decisions and the actual code/CI/docs in this repository. Every decision below is
*[proxy]* — flagged items need a real human nod before or during planning.

<domain>
## Phase Boundary

A team can install gruntled from a tagged release and wire it into pre-commit and CI by copying a
documented snippet. Four requirements, already locked by ROADMAP Phase 7: REL-02 (`--version`),
REL-01 (tagged static-binary releases + checksums), INT-03 (pre-commit hook), INT-04 (GitHub
Actions recipe that runs green in this repo's own CI, plus a documented GitLab CI recipe). No new
diagnostics, no daemon, no signing/SBOM/provenance, no package-manager channels — those are either
already shipped (Phases 5-6) or explicitly deferred (REQUIREMENTS.md "Future"/"Out of Scope").

</domain>

<decisions>
## Implementation Decisions

### Correction to task framing (verified against live GitHub, not assumed)
- The repository `GiulioSavini/gruntled` is **public** (`gh repo view` → `visibility: PUBLIC`),
  confirmed live and consistent with STATE.md's Phase 6 note: "06-05: repo made public for code
  scanning." The brief's premise that the repo is private and that this blocks Code Scanning is
  stale/incorrect — carried over from before 06-05. GitHub code scanning upload is therefore
  **not** blocked and needs no workaround (no artifact-only fallback, no "private repo" caveat in
  docs). **Flagged for human confirmation** since it reverses an explicit instruction in the task.

### `--version` / REL-02
- Add a `case "-version", "--version":` arm to the existing top-level `switch args[0]` in
  `cmd/gruntled/main.go` (next to the existing `-h, -help, --help, help` arm) — same pattern,
  same leniency (trailing args ignored, exit 0). No `-v` short alias (reserved for a future
  verbose flag; no short aliases exist elsewhere in the CLI today).
- Output is one line: `gruntled <version> (<commit>)`. No build date/time — keeps the string
  itself reproducible and avoids adding a field nobody asked for (REL-02 only requires version
  and commit).
- `version` and `commit` are `var`s in package `main`, set via `-ldflags "-X main.version=...
  -X main.commit=..."`. This is build-metadata plumbing in the composition root, not domain logic
  — it does not violate the "domain pure / ports in application / adapters in infrastructure"
  layering.
- Defaults when built without ldflags (`go build ./cmd/gruntled`, `go install`, local dev):
  `version = "dev"`, `commit = "none"`. `"dev"` matches the literal already hardcoded at the
  Phase 6 SARIF call site (`main.go:191`, `ToolInfo{Version: "dev"}`) so nothing changes for an
  unbuilt-from-release binary.
- The same `version` var replaces the `"dev"` literal passed into `presenter.ToolInfo{Version:
  version}` at that exact call site — this is the forward-reference Phase 6's own CONTEXT already
  recorded: *"For now main passes `dev`. Phase 7 replaces that value with the ldflags version and
  changes nothing in the presenter."*

### Release artifacts & build matrix / REL-01
- **Target matrix is 6 targets**: `linux/amd64`, `linux/arm64`, `darwin/amd64`, `darwin/arm64`,
  `windows/amd64`, `windows/arm64`.
  USER DECISION 2026-10-01: windows/arm64 ADDED as 6th target (matches ROADMAP/REQUIREMENTS
  literal wording). Target list = linux/amd64, linux/arm64, darwin/amd64, darwin/arm64,
  windows/amd64, windows/arm64.
- `CGO_ENABLED=0` explicit in the release build env — all deps (`hcl/v2`, `go-cty`) are pure Go,
  so this changes nothing functionally but makes "static binaries" (REL-01's own word) an explicit,
  documented build property instead of an accident.
- Archive format: `.tar.gz` for `linux`/`darwin`, `.zip` for `windows` — per-OS convention so a
  Windows user is never asked to find a `tar` binary.
- Asset naming: `gruntled_<version>_<os>_<arch>.tar.gz|zip` (version in the filename, standard Go
  release-tool convention, avoids stale-cache collisions across versions).
- Binary name inside the archive: plain `gruntled` / `gruntled.exe` — the archive filename already
  carries os/arch; the docs only ever need one invocation (`./gruntled check`).
- Checksums: one `checksums.txt` (plain `sha256sum`-format: `<hash>  <filename>`) covering every
  archive — REL-01 says "a checksums file," singular; `sha256sum -c checksums.txt` verifies
  everything in one command.
- Build tool: **hand-rolled matrix build in a new `release.yml` workflow**, `go build` + ldflags
  per target, no goreleaser. Reasons: (1) goreleaser is a new, fairly large CI dependency the
  project has never used; (2) the existing style for build-adjacent tooling is already
  `go run <pinned-module>@<version>` one-liners (staticcheck, govulncheck, jv) or raw shell loops
  (the cross-build step already in `ci.yml`) — a hand-rolled matrix continues that style instead
  of introducing a new tool and a new config file format; (3) "CI simple but correct, few jobs
  each able to really fail" favors fewer moving parts.
- The no-net/no-exec binary proof for the 6 release targets already exists and already runs per
  `GOOS`/`GOARCH` (`scripts/check-architecture.sh` Step 9, `binary-no-net-no-exec`, documented in
  `docs/cli.md`). `release.yml` does not reimplement this; it gates the release job on the
  existing `check`/`architecture` CI jobs having passed for that commit (`needs:` / requiring the
  tag's commit to be on `master` with green CI), so ROADMAP's "no-net/no-exec binary proof still
  passes on them" is satisfied by composition, not duplication.

### Release workflow mechanics / REL-01, REL-02
- Trigger: `on: push: tags: ['v*']` — a single workflow, one job chain, no manual dispatch needed
  for the common path.
- Tag format: `vX.Y.Z` (Go-ecosystem convention; stays pre-1.0 per PROJECT.md's current v0.2
  milestone, no `/vN` module-path concern yet).
- Version string for ldflags = `${{ github.ref_name }}` (the tag itself, e.g. `v0.2.0`) — exact
  and deterministic, not `git describe` (which can append `-dirty`/commit-distance suffixes and
  would make the same tag produce different strings on different checkouts, at odds with this
  project's determinism culture).
- Commit string for ldflags = short SHA (`${{ github.sha }}` truncated to 7-12 chars, e.g. via
  `git rev-parse --short HEAD`) — keeps `--version`'s one-line output short and human-scannable.
- Release creation via `gh release create <tag> --generate-notes <assets...>` (GitHub CLI,
  preinstalled on `ubuntu-latest`) — no third-party release-creation action to pin/trust, one
  fewer dependency, consistent with "no fake checks, no gonfie pipeline" directive.
- Changelog/release notes: GitHub's auto-generated notes (`--generate-notes`, derived from merged
  PRs/commits since the previous tag). No hand-maintained `CHANGELOG.md` — the repo has never had
  one, and introducing a new file to keep in sync by hand is extra maintenance ROADMAP doesn't ask
  for.
- Action pinning in `release.yml`: **every** action pinned by full commit SHA with a version
  comment (`actions/checkout@<sha> # v7.x.x`, `actions/setup-go@<sha> # v7.x.x`), going further
  than `ci.yml`'s current mixed state (tag-pinned `actions/checkout@v7`/`actions/setup-go@v7`,
  SHA-pinned only `github/codeql-action/upload-sarif`). This honors the explicit SHA-pinning
  preference for all new work in this phase. `ci.yml` itself is left untouched — retrofitting its
  existing pins is outside Phase 7's boundary and is **captured as a deferred idea**, not done
  silently.
- Signing / SBOM / provenance (cosign, SLSA attestations, GitHub artifact attestations): **out of
  scope**, deferred. Nothing in ROADMAP's Phase 7 success criteria, REQUIREMENTS.md REL-01/REL-02,
  or REQUIREMENTS.md's "Out of Scope" table mentions it; adding it now would be a new capability
  on top of the fixed phase boundary.

### Pre-commit integration / INT-03
- `.pre-commit-hooks.yaml` at repo root, one hook: `id: gruntled-check`, `name: gruntled check`,
  `description` one line, `language: golang`, `entry: gruntled check`, `pass_filenames: false`.
  `language: golang` is pre-commit's own documented support for Go-language hooks: `go install`s
  the binary from this repository at the pinned `rev`, needing no release-binary download logic
  and no custom install script.
- **`always_run: true`, no `files:` filter.** This reverses the obvious-looking first instinct
  (filter to `*.hcl`/`terragrunt.hcl` changes). gruntled's whole core value (PROJECT.md: "tell the
  user... that `dependency.X.outputs.Y` does not exist in the module it points to") is triggered
  just as often by editing a module's `.tf` file (renaming an `output`) as by editing a
  `terragrunt.hcl`. A file filter on `*.hcl` would silently skip the exact bug class gruntled
  exists to catch when only the module side changed. `pass_filenames: false` is required
  regardless, since gruntled takes a directory argument, not a file list.
- Default path: repo root (`.`), no hard-coded `args:` in `.pre-commit-hooks.yaml`. The documented
  recipe shows a user how to add `args: [subdir/]` for a monorepo subpath if they need it, but the
  shipped hook itself stays generic (matches gruntled's own CLI convention of defaulting to `.`).
- `--format`: not set (defaults to `text`) — pre-commit surfaces the hook's stdout/stderr directly
  to a human at the terminal; INT-03's own acceptance wording ("`pre-commit run` fails on a broken
  unit") is a human-facing check, not a machine-consumed one.
- `stages`: left at pre-commit's default (runs on `git commit`) — no explicit addition of `push`
  or `manual` stages; nothing in INT-03 asks for those.
- No hook-side exit-code translation. pre-commit already treats any non-zero exit as "hook
  failed" and prints the hook's own output verbatim, so gruntled's existing exit codes (1 =
  findings, 2 = usage error, 3 = internal failure) all correctly fail the hook as-is. The
  recipe doc explains what each code means so a user isn't confused by a 2 or 3 showing up instead
  of a diagnostic list, but no new code is written to handle this.
- Users pin `rev: vX.Y.Z` in their own `.pre-commit-config.yaml`, same tag the release workflow
  creates — the recipe doc shows the exact snippet with a concrete tag placeholder.

### CI integration recipes / INT-04
- New `docs/ci.md`, parallel to the existing `docs/cli.md` / `docs/validation.md` split-by-topic
  convention — not stuffed into README's existing "CI" section (which describes *this* repo's own
  internal CI and must stay clearly distinct from the user-facing recipe, exactly as Phase 6's
  `sarif-upload` job comment already warns: *"this is an internal proof job, not the user
  recipe"*). README gets a one-line pointer to the new doc, same style as its existing
  "Further reading" links.
- GitHub Actions recipe installs via `go install github.com/GiulioSavini/gruntled/cmd/
  gruntled@<tag>` on `ubuntu-latest` (Go preinstalled) — no release-binary download/platform-
  detection logic in the recipe itself. Simpler, one line, always matches the tag.
- Two documented GitHub Actions variants: (1) a minimal `check` job (plain text, fails the job on
  exit 1) as the first/default example; (2) a `check --format sarif` + `upload-sarif` job as a
  second "for GitHub code scanning" example, carrying forward INT-02's work. Neither forces SARIF
  complexity (the `security-events: write` permission, `category`, URI-prefix caveat) on a reader
  who just wants a failing CI check.
- The SARIF variant explicitly documents the Phase 6 finding recorded in STATE.md ("SARIF URIs
  relative to analysed dir; subdirectory analysis needs URI prefix or run from repo root"):
  recommend running gruntled from the repo root so URIs need no rewriting, and give the same `jq`
  prefixing one-liner from `ci.yml`'s `sarif-upload` job as a documented fallback for subdirectory
  analysis.
- `ci.yml` gains one new job, `recipe-check`, that runs the exact minimal recipe from `docs/ci.md`
  against this repository itself — satisfying ROADMAP's literal "runs green in this repository's
  own CI" rather than asserting it once by hand. It stays separate from the existing internal
  `sarif-upload` fixture-proof job (different purpose: one proves the recipe works, the other
  proves SARIF upload is accepted). This keeps job count growth to the same granularity the file
  already uses (one job per distinct thing being proven).
- Actions in the new `recipe-check` job are SHA-pinned, consistent with the release.yml decision
  above and the stated global preference, even though neighboring `ci.yml` jobs use tag pins.
  `docs/ci.md`'s own example YAML also shows SHA-pinned actions with a version comment, as a
  visible best-practice example for adopters copying it into their own repo.
- GitLab CI recipe is a **documented reference snippet only** (`go install` + `gruntled check`,
  fail the job on non-zero exit) — ROADMAP's wording distinguishes "runs green in this
  repository's own CI" (GitHub-specific) from "a GitLab CI recipe is documented" (no green-run
  requirement stated), so no disposable GitLab project/runner is provisioned to prove it.
- GitLab-native Code Quality report format (`gl-code-quality-report.json`, GitLab's SARIF
  analogue): **out of scope, deferred**. INT-04's wording only asks for a documented recipe, not a
  new output format; building one now would be a new capability, not a clarification of the
  existing boundary.

### Claude's Discretion
- Exact prose/section ordering inside `docs/ci.md` and the pre-commit recipe section of the README
  (or wherever it's cross-linked from).
- Whether `version`/`commit` live in `main.go` directly or a tiny sibling `version.go` in the same
  `cmd/gruntled` package — purely a file-split call, not an architectural one.
- Exact `release.yml` job/step names, and whether the 6-target matrix build (6 legs / matrix
  entries) is one job with a `strategy.matrix` or a shell loop mirroring `ci.yml`'s existing cross-build loop (either keeps
  the target list and action pins identical; matrix is likely cleaner since each leg can publish
  its own artifact in parallel).
- Exact wording of the `--generate-notes` release body vs adding one static sentence of
  boilerplate above it (e.g. linking to `docs/ci.md`/`.pre-commit-hooks.yaml`).
- Whether checksums are computed via `sha256sum` (Linux runner) or Go's `crypto/sha256` in a tiny
  helper step — either produces the same file content.

</decisions>

<code_context>
## Existing Code Insights

### Reusable assets
- `cmd/gruntled/main.go`'s top-level `switch args[0]` already has the exact pattern `--version`
  should follow (`case "-h", "-help", "--help", "help":`), and the SARIF call site
  (`presenter.SARIF(&buf, rep.Graph, rep.Diagnostics, presenter.ToolInfo{Version: "dev"})` at
  `main.go:191`) is the one line Phase 7 must change to wire the real version through — Phase 6's
  own CONTEXT already names this as the Phase 7 handoff.
- `scripts/check-architecture.sh` Step 9 (`binary-no-net-no-exec`) already runs `go list -deps`
  per `GOOS`/`GOARCH` for exactly the 6-target list (`linux/amd64`, `linux/arm64`, `darwin/amd64`,
  `darwin/arm64`, `windows/amd64`, `windows/arm64`) — this is the existing proof `release.yml` should depend on
  rather than duplicate.
- `ci.yml`'s existing `cross-build` step (loop over the same targets, 6 once windows/arm64 is added, `go build -o /dev/null`)
  is the direct template for `release.yml`'s real matrix build, minus the `/dev/null` and plus
  ldflags + archiving + checksums.
- `ci.yml`'s `sarif-schema` and `sarif-upload` jobs are the existing pattern for "pin a
  third-party action by SHA with a version comment" (`github/codeql-action/upload-sarif@<sha> #
  v4.38.2`) and for "`go run <pinned module>@<version>`" tool invocation (staticcheck,
  govulncheck, jv) — both patterns continue into `release.yml`/`recipe-check`.

### Established patterns
- Stdlib `flag` only, no cobra (locked in PROJECT.md Key Decisions) — `--version` must be handled
  in the existing hand-rolled `switch`, not via a flag library addition.
- Composition root (`cmd/gruntled`) is the only layer allowed to hold build-metadata plumbing like
  ldflags-injected vars; domain/application/infrastructure stay untouched by this phase.
- "CI simple but correct": every job in `ci.yml` today does one real, falsifiable thing (gofmt,
  vet, staticcheck, govulncheck, test, cross-build, sarif-schema validation, sarif-upload,
  architecture, architecture self-test) — `release.yml` and the new `recipe-check` job should keep
  that one-job-one-falsifiable-thing granularity rather than merging concerns.
- Mixed action-pinning already exists in `ci.yml` (tag-pinned `actions/checkout@v7`/`actions/
  setup-go@v7`, SHA-pinned `codeql-action`). New Phase 7 workflows go further (SHA-pin
  everything) per the explicit instruction, without retrofitting `ci.yml`'s existing pins.

### Integration points
- `release.yml` is a new workflow file, triggered independently of `ci.yml` (tag push vs.
  branch push/PR), but its release job should `needs:`/require the tagged commit's `ci.yml` run on
  `master` to be green, so the no-net/no-exec and test proofs cover the exact binaries shipped.
- `docs/ci.md` is new; cross-linked from README's existing "Further reading" list and from
  `docs/cli.md` if that file ever references CI/pre-commit (currently it doesn't — no edit needed
  there beyond what INT-03/04 require).
- `.pre-commit-hooks.yaml` lives at repo root (pre-commit's own convention — it is not under
  `docs/` or `scripts/`).

</code_context>

<specifics>
## Specific Ideas

No specific product references were given (user unavailable; this is proxy-authored). The closest
things to "specific ideas" are carried-forward constraints already locked elsewhere:
- `--version` output should be as terse as every other piece of gruntled's user-facing text —
  one line, no fluff, matching the project's own "eliminate conversational fluff" ethos that
  extends from CLI help text (`docs/cli.md`, `main.go`'s usage constants) to this new flag.
- The release pipeline should feel like other single-static-binary Go CLIs the user already
  interacts with in this ecosystem (checksums.txt + per-platform archives is the de facto norm for
  tools like `ripgrep`/`fd`/`terragrunt` itself) rather than inventing a bespoke layout.

</specifics>

<deferred>
## Deferred Ideas

- Signing / SBOM / provenance: cosign signatures, SLSA build provenance, GitHub artifact
  attestations for release binaries.
- Package-manager channels (Homebrew, apt, Scoop, winget) — REQUIREMENTS.md "Out of Scope" already
  says "add a channel only when someone asks."
- Container image — REQUIREMENTS.md "Out of Scope": "a static binary runs in any CI image
  already."
- Retrofitting `ci.yml`'s existing `actions/checkout@v7` / `actions/setup-go@v7` tag-pins to SHA
  pins, to make the whole repo's action-pinning style consistent (Phase 7 only SHA-pins its own
  *new* workflows/jobs).
- A hand-maintained `CHANGELOG.md`, if auto-generated GitHub release notes ever prove insufficient.
- GitLab-native Code Quality report format (`gl-code-quality-report.json`) as a new output format,
  analogous to the SARIF presenter but for GitLab's own code-quality widget.
- A disposable GitLab CI project/runner to actually prove the GitLab recipe green (today it is
  documentation-only, matching ROADMAP's narrower wording for GitLab vs. GitHub).
- `GRT004`-`GRT006`, daemon/watch, blast radius — already deferred to v0.3 in REQUIREMENTS.md,
  restated here only to confirm Phase 7 discussion did not touch them.

</deferred>

---

*Phase: 07-distribution-ci-integration*
*Context gathered: 2026-10-01*
