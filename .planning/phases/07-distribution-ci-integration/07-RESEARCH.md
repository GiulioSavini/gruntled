# Phase 7: Distribution & CI Integration - Research

**Researched:** 2026-10-01
**Domain:** Go release packaging, GitHub Actions, pre-commit hook repo, CI recipe docs
**Confidence:** MEDIUM-HIGH (repo facts verified locally; pre-commit behaviour verified from pre-commit source; GitLab image tag unverified)

<user_constraints>
## User Constraints (from CONTEXT.md)

All decisions below are *[proxy]*-authored (user was unavailable), copied in condensed-but-faithful form. Full text: `07-CONTEXT.md`.

### Locked Decisions
- Repo `GiulioSavini/gruntled` is PUBLIC (verified live again by me: `gh repo view` -> PUBLIC). The "private repo blocks Code Scanning" premise is stale. No artifact-only fallback, no private-repo caveat in docs.
- `--version`: new `case "-version", "--version":` arm in the top-level `switch args[0]` in `cmd/gruntled/main.go`; trailing args ignored, exit 0; no `-v`. Output one line: `gruntled <version> (<commit>)`, no date. `version`/`commit` are package-`main` `var`s set by `-ldflags "-X main.version=... -X main.commit=..."`; defaults `"dev"` / `"none"`. The same `version` var replaces the `"dev"` literal at `main.go:191` (`presenter.ToolInfo{Version: version}`).
- Targets: the 5 established: linux/amd64, linux/arm64, darwin/amd64, darwin/arm64, windows/amd64 (NOT windows/arm64; flagged for human confirmation, see Open Questions). `CGO_ENABLED=0` explicit. `.tar.gz` for linux/darwin, `.zip` for windows. Asset name `gruntled_<version>_<os>_<arch>.tar.gz|zip`. Binary inside archive: `gruntled` / `gruntled.exe`. One `checksums.txt` in `sha256sum` format covering every archive.
- Hand-rolled build in new `release.yml`, no goreleaser. Trigger `on: push: tags: ['v*']`, tags `vX.Y.Z`. ldflags version = `${{ github.ref_name }}` (not `git describe`); commit = short SHA. Release via `gh release create <tag> --generate-notes <assets...>`; no CHANGELOG.md. Every action in new workflows/jobs SHA-pinned with version comment; `ci.yml` existing pins NOT retrofitted. No signing/SBOM/provenance.
- The no-net/no-exec proof (`scripts/check-architecture.sh` Step 9) is not reimplemented; release is gated on it by composition.
- Pre-commit: `.pre-commit-hooks.yaml` at repo root, one hook `id: gruntled-check`, `name: gruntled check`, one-line `description`, `language: golang`, `entry: gruntled check`, `pass_filenames: false`, `always_run: true`, no `files:` filter, no hard-coded `args`, no `--format`, default stages, no exit-code translation. Users pin `rev: vX.Y.Z`.
- CI recipes: new `docs/ci.md` (README gets a one-line pointer in "Further reading"; README "CI" section stays about this repo's internal CI). GitHub recipe installs with `go install github.com/GiulioSavini/gruntled/cmd/gruntled@<tag>`. Two GitHub variants: (1) minimal text `check` job; (2) `check --format sarif` + `upload-sarif`. SARIF variant documents the URI-relative-to-analysed-dir finding: run from repo root, `jq` prefix one-liner as subdirectory fallback. `ci.yml` gains one job `recipe-check` running the minimal recipe against this repo, SHA-pinned actions. docs example YAML also SHA-pinned with version comments. GitLab recipe: documented snippet only (`go install` + `gruntled check`), no green run required.

### Claude's Discretion
- Prose/ordering of `docs/ci.md` and the pre-commit section.
- `version`/`commit` in `main.go` vs a sibling `version.go`.
- `release.yml` job/step names; matrix vs shell loop.
- Release body wording / one static boilerplate sentence.
- Checksums via `sha256sum` or Go helper.

### Deferred Ideas (OUT OF SCOPE)
windows/arm64 as 6th target; signing/SBOM/provenance; Homebrew/apt/Scoop/winget; container image; retrofitting ci.yml pins; hand-maintained CHANGELOG.md; GitLab Code Quality report format; disposable GitLab project to prove the recipe; GRT004-006/daemon/blast.
</user_constraints>

<phase_requirements>
## Phase Requirements

| ID | Description | Research Support |
|----|-------------|-----------------|
| REL-02 | `gruntled --version` prints version + commit injected at build time | Pattern 1 (ldflags `-X main.version`), unit test + release smoke step |
| REL-01 | Static binaries linux/darwin/windows amd64+arm64 on tagged release + checksums | Pattern 2 (`scripts/build-release.sh`), release.yml, verification gate (Pitfalls 1, 2) |
| INT-03 | One hook entry in `.pre-commit-config.yaml` served by this repo's `.pre-commit-hooks.yaml`; `pre-commit run` fails on broken unit | Pattern 3, `try-repo` CI proof, Pitfall 5 |
| INT-04 | Documented GH Actions recipe (check + SARIF) runs in own CI; GitLab recipe documented | Pattern 4, Pitfall 3 (repo root is NOT clean), Pitfall 4 |
</phase_requirements>

## Summary

Phase 7 adds no domain logic. It is build metadata in the composition root, one new shell script + one new workflow for releases, one 10-line YAML file for pre-commit, one doc, and two proof steps in `ci.yml`. Everything is stdlib-Go/bash/GitHub-CLI; no new Go dependency is needed.

Three facts verified in the repo change how the plan must be shaped. (1) `gruntled check .` on this repository exits 1, because `cmd/gruntled/testdata/sarif-fixture` contains deliberately broken units (GRT001/002/003) and the walker does not skip `testdata`; a `recipe-check` that "runs the recipe against this repo's root" can never be green as written. A clean fixture is required. (2) A tag-triggered workflow cannot `needs:` a job in another workflow; "gated on ci.yml green" must be done inside `release.yml` by re-running the proofs (cheap: the architecture script + tests). (3) `go.mod` says `go 1.27`; pre-commit's `language: golang` and the GitHub runner's preinstalled Go can be older, so toolchain selection must be handled explicitly in docs and CI.

**Primary recommendation:** Put all packaging in `scripts/build-release.sh <version> <commit> <outdir>` (builds 5 targets, archives, `checksums.txt`), call it from both `release.yml` (real) and `ci.yml`'s `check` cross-build step (dry run), so the release path is exercised on every push instead of first failing at tag time. Add a clean fixture; make `recipe-check` run the recipe against the clean fixture and `pre-commit try-repo` against clean+broken fixtures.

## Standard Stack

### Core
| Tool | Version | Purpose | Why |
|------|---------|---------|-----|
| Go toolchain | go.mod `go 1.27` (local go1.27.0) | `go build -trimpath -ldflags` | already the project toolchain |
| bash + tar + zip + sha256sum + jq | ubuntu-latest preinstalled | archives/checksums | same style as existing scripts |
| GitHub CLI `gh` | preinstalled on ubuntu-latest | `gh release create --generate-notes` | locked decision; no third-party release action |
| actions/checkout | v7.0.1 = `3d3c42e5aac5ba805825da76410c181273ba90b1` | checkout | verified via `gh api` today |
| actions/setup-go | v7.0.0 = `b7ad1dad31e06c5925ef5d2fc7ad053ef454303e` | Go from go.mod | verified via `gh api` today |
| github/codeql-action/upload-sarif | v4.38.2 = `2892aa5e19bbd11bc0cff5427e3b750a04d9e3c2` | SARIF upload | already pinned in ci.yml |
| pre-commit | 4.6.2 (latest release) | hook runner for the CI proof only | `pre-commit try-repo` |

(SHAs resolved by `gh api repos/<r>/commits/<tag>` on 2026-10-01. Annotated-tag caveat: `commits/<tag>` returns the commit SHA, which is what `uses:` needs.)

### Alternatives Considered
| Instead of | Could Use | Tradeoff |
|------------|-----------|----------|
| shell script | goreleaser | locked out (new tool/config) |
| `strategy.matrix` + upload/download-artifact | one job, script loop | loop needs 0 extra actions, one checksums step, no artifact plumbing; matrix is allowed by CONTEXT but adds 2 pinned actions. **Recommend the loop.** |

No install step needed (no new Go deps).

## Architecture Patterns

### Recommended layout of new/changed files
```
cmd/gruntled/main.go            # var version/commit; --version arm; ToolInfo{Version: version}
cmd/gruntled/main_test.go       # --version tests
cmd/gruntled/testdata/clean-fixture/   # NEW: valid 2-unit repo (dependency + module with the output)
scripts/build-release.sh        # NEW: <version> <commit> <outdir>
.github/workflows/release.yml   # NEW
.github/workflows/ci.yml        # + recipe-check job; cross-build step calls build-release.sh
.pre-commit-hooks.yaml          # NEW
docs/ci.md                      # NEW; README one-line pointer; docs/cli.md documents --version
```

### Pattern 1: ldflags version (REL-02)
```go
// cmd/gruntled/main.go (composition root)
// Set at release build time: -ldflags "-X main.version=v0.2.0 -X main.commit=abc1234".
var (
	version = "dev"
	commit  = "none"
)

case "-version", "--version":
	fmt.Fprintf(stdout, "gruntled %s (%s)\n", version, commit)
	return exitOK
```
Notes: `-X` only works on package-level `string` **vars** initialised with a constant string (not `const`, not function-call initialisers). Print to **stdout** (convention; the help arm uses stderr, do not copy that). Optional enhancement (discretion, flag in plan): when `version == "dev"`, fall back to `debug.ReadBuildInfo().Main.Version` (non-`(devel)`), so `go install ...@v0.2.0` (the recipe's install path!) prints `v0.2.0` instead of `dev`. `vcs.revision` is NOT stamped on module-path installs, so commit stays `none`. Source: Go `runtime/debug` docs / linker docs [MEDIUM; standard, not re-fetched].

### Pattern 2: `scripts/build-release.sh` (REL-01)
```bash
#!/usr/bin/env bash
set -euo pipefail
version=$1 commit=$2 out=$3
cd "$(dirname "$0")/.."
export CGO_ENABLED=0 GOWORK=off
rm -rf "$out"; mkdir -p "$out"
for target in linux/amd64 linux/arm64 darwin/amd64 darwin/arm64 windows/amd64; do
  os=${target%/*} arch=${target#*/}
  stage=$(mktemp -d); bin=gruntled; [ "$os" = windows ] && bin=gruntled.exe
  GOOS=$os GOARCH=$arch go build -trimpath \
    -ldflags "-s -w -X main.version=$version -X main.commit=$commit" \
    -o "$stage/$bin" ./cmd/gruntled
  cp LICENSE "$stage/"
  name=gruntled_${version}_${os}_${arch}
  if [ "$os" = windows ]; then (cd "$stage" && zip -q "$OLDPWD/$out/$name.zip" "$bin" LICENSE)
  else tar -C "$stage" -czf "$out/$name.tar.gz" "$bin" LICENSE; fi
done
(cd "$out" && sha256sum gruntled_* > checksums.txt && sha256sum -c checksums.txt)
```
(Sketch; planner should resolve `$out` to an absolute path and keep `release_targets` identical to `check-architecture.sh` line 486 and the ci.yml loop — three copies of the list exist; add a comment cross-reference in each, as Step 9 already does.)
Naming: version in filename includes the leading `v` if `ref_name` is used (`gruntled_v0.2.0_linux_amd64.tar.gz`). That is consistent but differs from goreleaser's habit of stripping `v`; pick one and document it in `docs/ci.md`. Recommend keeping the `v` (one variable, no stripping logic).

### Pattern 3: release.yml (REL-01/REL-02)
```yaml
name: release
on:
  push:
    tags: ['v*']
permissions:
  contents: read
env:
  GOTOOLCHAIN: local
jobs:
  release:
    runs-on: ubuntu-latest
    permissions:
      contents: write          # only this job; gh release create needs it
    steps:
      - uses: actions/checkout@3d3c42e5aac5ba805825da76410c181273ba90b1 # v7.0.1
        with: { fetch-depth: 0 }
      - uses: actions/setup-go@b7ad1dad31e06c5925ef5d2fc7ad053ef454303e # v7.0.0
        with: { go-version-file: go.mod }
      - name: tag format and branch
        run: |
          [[ "$GITHUB_REF_NAME" =~ ^v[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.-]+)?$ ]]
          git fetch origin master
          git merge-base --is-ancestor "$GITHUB_SHA" origin/master   # tag must be on master
      - name: proofs      # re-run what the binaries depend on
        run: |
          bash scripts/check-architecture.sh
          go test -count=1 ./...
      - name: build
        run: bash scripts/build-release.sh "$GITHUB_REF_NAME" "$(git rev-parse --short=12 HEAD)" "$RUNNER_TEMP/dist"
      - name: smoke
        run: |
          tar -xzf "$RUNNER_TEMP"/dist/gruntled_${GITHUB_REF_NAME}_linux_amd64.tar.gz -C "$RUNNER_TEMP"
          out=$("$RUNNER_TEMP/gruntled" --version)
          [[ "$out" == "gruntled $GITHUB_REF_NAME ("* ]]
      - name: publish
        env: { GH_TOKEN: "${{ github.token }}" }
        run: |
          pre=(); [[ "$GITHUB_REF_NAME" == *-* ]] && pre=(--prerelease)
          gh release create "$GITHUB_REF_NAME" --generate-notes --verify-tag "${pre[@]}" "$RUNNER_TEMP"/dist/*
```
Why re-run proofs instead of `needs:`: `needs:` is intra-workflow only. "Require master CI green" via API polling is more moving parts than re-running a ~1 min script. The architecture script runs Step 9 for all 5 targets, so "no-net/no-exec still passes on them" holds for exactly the tagged commit. `go test -race` is optional here (CI already ran it); `-count=1` without race is cheaper.

### Pattern 4: pre-commit hook (INT-03)
```yaml
# .pre-commit-hooks.yaml
- id: gruntled-check
  name: gruntled check
  description: Check Terragrunt dependency output references
  language: golang
  entry: gruntled check
  pass_filenames: false
  always_run: true
```
How pre-commit builds a `golang` hook (verified from pre-commit `languages/golang.py`): installs Go (system Go if on PATH, else downloads latest from dl.google.com), then `go install ./...` in the hook repo clone; for non-system Go it sets `GOTOOLCHAIN=local`. All main packages under `./...` are installed: this repo has `cmd/gruntled` and `scripts/archscan` (non-test `main.go`), so an extra `archscan` binary lands in the hook env bin dir — harmless but note it. `entry: gruntled check` resolves because the env bin dir is on PATH. User config:
```yaml
repos:
  - repo: https://github.com/GiulioSavini/gruntled
    rev: v0.2.0
    hooks:
      - id: gruntled-check
        # args: [infra/]     # monorepo subpath; args are appended after the entry
```
Consider `minimum_pre_commit_version: '3.0.0'` in the hook (optional). Hook fields (`id,name,entry,language,pass_filenames,always_run,description`) are the documented pre-commit hook schema [MEDIUM-HIGH].

### Pattern 5: CI proof job (INT-04, INT-03)
```yaml
  recipe-check:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@3d3c42e5... # v7.0.1
      - uses: actions/setup-go@b7ad1dad... # v7.0.0
        with: { go-version-file: go.mod }
      - name: install (recipe)            # recipe uses go install ...@tag; here: this checkout
        run: go install ./cmd/gruntled
      - name: check (recipe)
        run: gruntled check cmd/gruntled/testdata/clean-fixture   # exit 0 required
      - name: recipe fails on a broken unit
        run: '! gruntled check cmd/gruntled/testdata/sarif-fixture'   # must be exit 1 exactly: see Pitfall 6
      - name: pre-commit hook
        run: |
          pipx run --spec pre-commit==4.6.2 pre-commit try-repo ... (clean passes, broken fails)
```
`go install` puts the binary in `$(go env GOPATH)/bin`, which setup-go puts on PATH [MEDIUM; verify during implementation, else `export PATH`].

### Anti-Patterns to Avoid
- `gruntled check .` against this repo's root (always exit 1, see Pitfall 3).
- `|| true` or `continue-on-error` to make a job "green"; each proof step must fail when the thing is broken (user preference).
- Asserting only `!cmd` for the broken case: exit 2/3 also satisfy `!`. Capture rc and require `== 1`, as the existing `sarif-schema` step does.
- `git describe` for version (locked out).

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| Release creation/notes | custom API calls / release actions | `gh release create --generate-notes` | locked; preinstalled |
| Checksums verification | Go helper | `sha256sum -c checksums.txt` | same format users run |
| Hook installation | install-script/download logic | pre-commit `language: golang` | locked |
| SARIF path rewriting in docs | new flag in gruntled | documented `jq` one-liner / run from repo root | locked; new flag = new capability |
| Pre-commit proof | parse the YAML in Go | `pre-commit try-repo` | exercises the real consumer |

## Common Pitfalls

### Pitfall 1: tag-triggered release cannot depend on ci.yml
**What goes wrong:** `needs: [check]` does not resolve across workflows; "gate" silently absent.
**How to avoid:** re-run `check-architecture.sh` + tests inside `release.yml` (Pattern 3). Plan must state this explicitly since CONTEXT says "`needs:` / requiring green CI".
**Warning signs:** release.yml referencing jobs it does not define (actionlint rejects it).

### Pitfall 2: release path first exercised at tag time
**What goes wrong:** zip/tar/ldflags bug found only when cutting v0.2.0; a half-created release or a burned tag.
**How to avoid:** `build-release.sh` also runs in ci.yml (replace the `/dev/null` cross-build loop, or add a step) with version `v0.0.0-ci`; run it locally in the plan's verification. Use `gh release create --verify-tag`; build everything before the single publish step so failure leaves no partial release.

### Pitfall 3: this repo is not clean
`go run ./cmd/gruntled check .` -> exit 1 (verified: 3 errors from `sarif-fixture`). Same for the pre-commit hook run in this repo and for any `gruntled check .` in `docs/ci.md` "run in this repo" claims. Add `cmd/gruntled/testdata/clean-fixture` (a `live/<unit>/terragrunt.hcl` with a `dependency` whose `outputs.X` exists in the module `outputs.tf`; model it on the passing synthrepo/fixture patterns). Do not add a `.gruntledignore` (new capability).

### Pitfall 4: SARIF URI base
STATE 06-05: URIs are relative to the analysed dir; `checkout_path` does not rewrite them. In the doc recipe running `gruntled check --format sarif .` at repo root is correct. Upload step must run even when check exits 1: gate it with `if: ${{ !cancelled() }}` (or `always()`), and make the check step not abort before the file is written (shell redirect writes it before exit). Document exit 2/3 produce no valid SARIF. Needs `permissions: security-events: write` + `contents: read` and a `category`. Private forks without Advanced Security can't upload: one sentence, not a workaround.

### Pitfall 5: Go toolchain version in consumers
`go.mod` is `go 1.27`. (a) Recipe: a runner whose Go is older triggers an automatic toolchain download (default `GOTOOLCHAIN=auto`; needs network). In the doc recipe use `actions/setup-go` with `go-version: stable` (or `1.27`) rather than "Go is preinstalled". (b) pre-commit: if a system Go is on PATH it is used and `GOTOOLCHAIN` is not forced, so an old system Go auto-downloads (works, slow, needs network); with no system Go pre-commit downloads the latest Go (>=1.27 required). In this repo's `ci.yml` the `GOTOOLCHAIN: local` env is global; keep setup-go with `go-version-file` first, or the try-repo step may fail with "go.mod requires go >= 1.27".
(c) `go install pkg@tag` prints version `dev` unless the ReadBuildInfo fallback (Pattern 1) is added; users following the recipe would otherwise see `gruntled dev (none)`.

### Pitfall 6: exit-code semantics in CI
Exit 1 = findings, 2 = usage, 3 = internal. Recipes should let any non-zero fail the job (default shell `-e`). The own-CI "must fail" assertion should require exactly 1.

### Pitfall 7: Windows archive tooling and naming
`zip` is installed on ubuntu-latest [MEDIUM]; use `-q` and archive from inside the stage dir so the zip has no path prefix. `.exe` suffix only for windows. Test: `unzip -l` lists `gruntled.exe`.

### Pitfall 8: `-X` silently ignored
A typo in `-X main.verison=...` or making it `const` produces `dev` with no error. The smoke step (Pattern 3) and unit test pin this. `-X` import path is `main` for the main package (not the module path).

### Pitfall 9: pinning drift
Prior pins in `ci.yml` are tags (`@v7`). The new SHAs above are v7.0.1/v7.0.0; they are what `@v7` resolves to today. Keep the `# vX.Y.Z` comment. The `upload-sarif` SHA in docs must match the one in ci.yml (copy, don't re-resolve).

## Code Examples

### GitHub recipe for docs/ci.md (variant 1 + 2)
```yaml
name: gruntled
on: [push, pull_request]
permissions: { contents: read }
jobs:
  check:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@3d3c42e5aac5ba805825da76410c181273ba90b1 # v7.0.1
      - uses: actions/setup-go@b7ad1dad31e06c5925ef5d2fc7ad053ef454303e # v7.0.0
        with: { go-version: stable }
      - run: go install github.com/GiulioSavini/gruntled/cmd/gruntled@v0.2.0
      - run: gruntled check .
  # variant 2: code scanning
  scan:
    runs-on: ubuntu-latest
    permissions: { contents: read, security-events: write }
    steps:
      - (checkout, setup-go, install as above)
      - run: gruntled check --format sarif . > gruntled.sarif
      - if: ${{ !cancelled() }}
        uses: github/codeql-action/upload-sarif@2892aa5e19bbd11bc0cff5427e3b750a04d9e3c2 # v4.38.2
        with: { sarif_file: gruntled.sarif, category: gruntled }
```
Caveat to confirm in implementation: `!cancelled()` upload after a failed step means a file may be missing when check exits 2/3; fine (upload fails loudly).

### GitLab recipe
```yaml
gruntled:
  image: golang:1.27        # LOW: tag existence not verified
  script:
    - go install github.com/GiulioSavini/gruntled/cmd/gruntled@v0.2.0
    - $(go env GOPATH)/bin/gruntled check .
```
Failing exit code fails the job by default. Mention optional download-binary path using `checksums.txt` for Go-less images (curl + `sha256sum -c`).

### Tests
```go
// cmd/gruntled/main_test.go style: call run(args, &out, &errb)
func TestVersion(t *testing.T) { /* run([]string{"--version"}) -> exit 0, stdout "gruntled dev (none)\n";
   also "-version", and trailing args ignored; override package vars for a non-default case */ }
```

## State of the Art

| Old | Current | Impact |
|-----|---------|--------|
| unpinned/tag-pinned actions | full-SHA pins with comment | matches user preference |
| goreleaser for any Go CLI | small script acceptable for 5 targets | locked |
| `pre-commit` `language: system` + manual install | `language: golang` builds from hook repo | no binary download logic |

## Open Questions

1. **windows/arm64 (HIGH IMPACT on verification)**
   - Known: ROADMAP success criterion 2 and REL-01 literally read "linux, darwin and windows on amd64 and arm64" (6 targets). CONTEXT (proxy) locks 5.
   - Risk: the verifier reading the criterion literally will fail the phase.
   - Recommendation: planner honours the lock but makes the target list a single bash array in `build-release.sh`, and asks for a human nod; adding it = one entry here + one in `check-architecture.sh` `release_targets` + the ci.yml loop (and its self-test if it enumerates targets). Cost is trivial; I would add it (smaller risk than the literal-criterion mismatch). Windows/arm64 builds with pure Go at go1.27 [MEDIUM; confirm via `GOOS=windows GOARCH=arm64 go build` locally].
2. **Repo public vs task brief:** I re-verified PUBLIC; CONTEXT's correction is right. No action.
3. **Where does `recipe-check` get exactly "the recipe"?** Recipe uses `go install ...@tag` and `check .`; CI uses `go install ./cmd/gruntled` and `check <clean-fixture>`. Optional guard: a Go test (like `TestValidationDocPins`) asserting docs/ci.md contains the `gruntled check` command lines used in the job. Recommend a small one; otherwise the "runs green in own CI" claim is only approximate.
4. **ReadBuildInfo fallback for `go install@tag`:** not in CONTEXT; recommended small addition, decide in plan.
5. **First real tag:** who cuts `v0.2.0`? The phase can only prove the pipeline by dry run (script locally + CI); an actual tag push is an externally visible action needing the user's go-ahead.

## Validation Architecture

### Test Framework
| Property | Value |
|----------|-------|
| Framework | Go `testing` + testscript (existing), bash scripts, GitHub Actions |
| Config file | none; `go.mod` go 1.27 |
| Quick run command | `go test ./cmd/gruntled -run 'Version\|Sarif' -count=1` |
| Full suite command | `go test -race -count=1 ./... && bash scripts/check-architecture.sh && bash scripts/test-check-architecture.sh` |

### Phase Requirements -> Test Map
| Req | Behavior | Test Type | Command | Exists? |
|-----|----------|-----------|---------|---------|
| REL-02 | `--version`/`-version` prints `gruntled dev (none)` exit 0; trailing args ignored; SARIF `ToolInfo.Version` = version | unit | `go test ./cmd/gruntled -run TestVersion -count=1` | Wave 0 |
| REL-02 | ldflags actually inject | script | `go build -ldflags "-X main.version=v9.9.9 -X main.commit=abc1234" -o $T/g ./cmd/gruntled && $T/g --version \| grep -x 'gruntled v9.9.9 (abc1234)'` | Wave 0 (put in build-release smoke / ci step) |
| REL-01 | 5 archives + checksums.txt, `sha256sum -c` passes, zip has .exe, `file`/`go version` shows static | script | `bash scripts/build-release.sh v0.0.0-ci abc1234 $T && ls $T` | Wave 0 |
| REL-01 | no-net/no-exec on all targets | existing | `bash scripts/check-architecture.sh` | yes |
| INT-03 | hook fails on broken, passes on clean | CI integration | `pre-commit try-repo <repo> gruntled-check --all-files` in temp git repos from clean/sarif fixtures (pre-commit not installed locally: CI-only, or `pipx run`) | Wave 0 |
| INT-03 | hooks YAML well-formed | unit | Go test parsing `.pre-commit-hooks.yaml` is NOT possible without a YAML dep (go.mod has none): do not add one; rely on try-repo | n/a |
| INT-04 | recipe passes on clean, exit exactly 1 on broken | CI | `recipe-check` job | Wave 0 |
| INT-04 | docs/ci.md pins match ci.yml (SHAs, command lines) | unit | `go test ./cmd/gruntled -run TestCIDoc -count=1` | Wave 0 (optional) |
| INT-04 | GitLab recipe documented | manual-only | review docs/ci.md (no runner, by decision) | n/a |
| REL-01 | real tag -> release with assets | manual-only | needs a pushed tag (user approval); verify with `gh release view` + `sha256sum -c` | n/a |

### Sampling Rate
- Per task commit: `go test ./cmd/gruntled -count=1` and, for script tasks, `bash scripts/build-release.sh ...`
- Per wave merge: full suite command above
- Phase gate: full suite green + ci.yml green on master (incl. `recipe-check`) before `/gsd:verify-work`

### Wave 0 Gaps
- [ ] `cmd/gruntled/main_test.go` - TestVersion cases
- [ ] `cmd/gruntled/testdata/clean-fixture/` - must be verified exit 0 (`go run ./cmd/gruntled check <dir>`) and must not be picked up by golden/corpus tests that enumerate testdata (check `golden_test.go`/`sarif_test.go` globs before adding)
- [ ] `scripts/build-release.sh` (+ local run)
- [ ] optional `actionlint` run locally if available (`go run github.com/rhysd/actionlint/cmd/actionlint@<pinned>`) to catch workflow syntax errors before pushing; do not add it as a CI job unless desired

## Sources

### Primary (HIGH confidence)
- Local repo: `.github/workflows/ci.yml`, `cmd/gruntled/main.go` (switch at ~L80, SARIF call L191), `scripts/check-architecture.sh` Step 9 (`release_targets`, L486), `go run ./cmd/gruntled check .` (exit 1, 3 errors), `gh repo view` (PUBLIC)
- `gh api` releases/commits for actions SHAs and pre-commit v4.6.2 (2026-10-01)
- pre-commit source `pre_commit/languages/golang.py` (raw.githubusercontent.com, fetched today): system-Go detection, download URL, `GOTOOLCHAIN=local` for non-system

### Secondary (MEDIUM)
- Go linker `-X` semantics, `runtime/debug.ReadBuildInfo` behaviour for module installs, pre-commit hook schema, ubuntu-latest preinstalled tools: training knowledge, standard and stable, not re-fetched

### Tertiary (LOW)
- `golang:1.27` Docker tag existence; `pipx run --spec` availability on runner; setup-go putting `GOPATH/bin` on PATH: verify at implementation

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH - all tools already used in repo or verified live
- Architecture: MEDIUM-HIGH - patterns standard; release.yml is unexecuted until first push/tag
- Pitfalls: HIGH for repo-specific (clean fixture, cross-workflow needs, windows/arm64 criterion); MEDIUM for pre-commit toolchain interplay

**Research date:** 2026-10-01
**Valid until:** 2026-10-31 (action SHAs: re-resolve if plan slips)
