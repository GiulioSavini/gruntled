# Running gruntled in CI and pre-commit

This page shows how to run `gruntled check` on your own Terragrunt repository:
how to install it, what the exit codes mean for a CI job, a pre-commit hook,
GitHub Actions workflows (plain and with code scanning) and a GitLab CI job.

In the examples `v0.2.0` stands for the release you want. Pin a real tag from
the [releases page](https://github.com/GiulioSavini/gruntled/releases) and
bump it on purpose, like any other tool version.

## 1. Install

### With Go

```
go install github.com/GiulioSavini/gruntled/cmd/gruntled@v0.2.0
```

This needs Go at least as new as the `go` line in gruntled's `go.mod`
(currently `go 1.27`). An older Go downloads the newer toolchain
automatically if network access is allowed (the default `GOTOOLCHAIN=auto`).
The binary lands in `$(go env GOPATH)/bin`.

A binary built this way prints `gruntled dev (none)` for `gruntled --version`:
the version string is injected only into release binaries. The code is the
one at the tag you installed; only the label is missing.

### Release binaries

Each release has one archive per target plus a `checksums.txt`:

| OS | Arch | Asset |
|----|------|-------|
| linux | amd64 | `gruntled_<tag>_linux_amd64.tar.gz` |
| linux | arm64 | `gruntled_<tag>_linux_arm64.tar.gz` |
| darwin | amd64 | `gruntled_<tag>_darwin_amd64.tar.gz` |
| darwin | arm64 | `gruntled_<tag>_darwin_arm64.tar.gz` |
| windows | amd64 | `gruntled_<tag>_windows_amd64.zip` |
| windows | arm64 | `gruntled_<tag>_windows_arm64.zip` |

`<tag>` is the full tag with its leading `v`, for example
`gruntled_v0.2.0_linux_amd64.tar.gz`. Each archive holds the binary
(`gruntled`, or `gruntled.exe` on windows) and `LICENSE`, with no directory
prefix. The binaries are static (`CGO_ENABLED=0`) and print
`gruntled <tag> (<commit>)` for `--version`.

Download and verify, then unpack:

```
tag=v0.2.0
base=https://github.com/GiulioSavini/gruntled/releases/download/$tag
curl -fsSLO "$base/gruntled_${tag}_linux_amd64.tar.gz"
curl -fsSLO "$base/checksums.txt"
sha256sum -c --ignore-missing checksums.txt
tar -xzf "gruntled_${tag}_linux_amd64.tar.gz" gruntled
./gruntled --version
```

`--ignore-missing` makes `sha256sum` check only the archives you downloaded;
it still fails if that archive does not match. On macOS use
`shasum -a 256 -c --ignore-missing checksums.txt`.

## 2. Exit codes

| Code | Meaning |
|------|---------|
| 0 | No error diagnostic. |
| 1 | At least one error diagnostic (a broken reference was found). |
| 2 | Usage error (unknown flag, invalid `--format`, more than one path). |
| 3 | Analysis could not run (path missing or unreadable, internal failure). |

Every non-zero code fails the CI job or the pre-commit hook. There is no
"warn only" mode and nothing to translate: a finding is exit 1, and a
misconfigured invocation (2) or an analysis that could not run (3) also fails
rather than passing silently. The full table is in [cli.md](cli.md#exit-codes).

## 3. pre-commit

Add this to your repository's `.pre-commit-config.yaml`:

```yaml
repos:
  - repo: https://github.com/GiulioSavini/gruntled
    rev: v0.2.0
    hooks:
      - id: gruntled-check
```

Replace `v0.2.0` with a real release tag (`pre-commit autoupdate` also works).

The hook runs `gruntled check` on the repository root, once per commit.

- **It always runs, whatever files changed** (`always_run: true`,
  `pass_filenames: false`). The bug gruntled catches usually comes from a
  change in a module's `.tf` file, such as a renamed or removed `output`.
  The broken reference sits in a `terragrunt.hcl` that the commit did not
  touch. A hook filtered to staged `terragrunt.hcl` files would miss exactly
  that case, so the hook checks the whole repository every time.
- **Monorepo subpath:** to check only one directory, pass it as an argument:

  ```yaml
      hooks:
        - id: gruntled-check
          args: [infra/]
  ```

- **Go toolchain:** the hook is `language: golang`, so pre-commit builds it
  from source the first time (`go install ./...` in its clone of this
  repository). This also installs a small helper binary, `archscan`, from
  `scripts/archscan` into the hook's private environment. It is harmless and
  never on your own `PATH`. If Go is on your `PATH`, pre-commit uses it. It
  must be at least the version in gruntled's `go.mod` (`go 1.27`), or have
  network access to download that toolchain. With an older Go and
  `GOTOOLCHAIN=local` set, the install fails with
  `go.mod requires go >= 1.27`. If Go is not installed,
  pre-commit downloads a current Go by itself.

## 4. GitHub Actions

Run the workflows from the repository root (`gruntled check .`). The actions
are pinned to full commit SHAs, with the version in a comment.

### Variant 1: fail the job on a broken reference

```yaml
name: gruntled
on: [push, pull_request]
permissions:
  contents: read
jobs:
  check:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@3d3c42e5aac5ba805825da76410c181273ba90b1 # v7.0.1
      - uses: actions/setup-go@b7ad1dad31e06c5925ef5d2fc7ad053ef454303e # v7.0.0
        with:
          go-version: stable
      - run: go install github.com/GiulioSavini/gruntled/cmd/gruntled@v0.2.0
      - run: gruntled check .
```

`setup-go` with `go-version: stable` avoids depending on whichever Go the
runner image has preinstalled, which can be older than gruntled's `go.mod`.

### Variant 2: code scanning (SARIF)

Findings appear as code scanning alerts on the lines of the broken
references, and the job still fails on exit 1:

```yaml
name: gruntled
on: [push, pull_request]
permissions:
  contents: read
jobs:
  scan:
    runs-on: ubuntu-latest
    permissions:
      contents: read
      security-events: write
    steps:
      - uses: actions/checkout@3d3c42e5aac5ba805825da76410c181273ba90b1 # v7.0.1
      - uses: actions/setup-go@b7ad1dad31e06c5925ef5d2fc7ad053ef454303e # v7.0.0
        with:
          go-version: stable
      - run: go install github.com/GiulioSavini/gruntled/cmd/gruntled@v0.2.0
      - run: gruntled check --format sarif . > gruntled.sarif
      - if: ${{ !cancelled() }}
        uses: github/codeql-action/upload-sarif@2892aa5e19bbd11bc0cff5427e3b750a04d9e3c2 # v4.38.2
        with:
          sarif_file: gruntled.sarif
          category: gruntled
```

Notes:

- `if: ${{ !cancelled() }}` uploads the report even though the `check` step
  failed with exit 1, which is the case where you want the alerts.
- Artifact URIs in the SARIF file are relative to the directory gruntled
  analysed. Run it on `.` from the repository root and they resolve as-is. If
  you analyse a subdirectory, prefix the URIs before uploading, for example
  for `infra/`:

  ```
  gruntled check --format sarif infra/ > gruntled.sarif
  jq --arg p infra/ '(.. | .artifactLocation? | objects | .uri) |= $p + .' \
    gruntled.sarif > upload.sarif
  ```

  and upload `upload.sarif` instead.
- On exit 2 or 3 gruntled writes no valid SARIF. The upload step then fails
  as well, which is intended: the job never goes green on a run that did not
  analyse anything.
- Code scanning is available on public repositories and on private ones
  with GitHub Advanced Security (Code Security). Without it, use variant 1.

## 5. GitLab CI

```yaml
gruntled:
  image: golang:1.27
  script:
    - go install github.com/GiulioSavini/gruntled/cmd/gruntled@v0.2.0
    - $(go env GOPATH)/bin/gruntled check .
```

A non-zero exit code fails the job, as in GitHub Actions. For an image
without Go, download a release archive and check it against `checksums.txt`
as in [Release binaries](#release-binaries).

This snippet is documentation only: gruntled's own CI runs on GitHub and does
not execute it. gruntled has no GitLab Code Quality output format; use the
default text output in the job log (or `--format json` as an artifact).
