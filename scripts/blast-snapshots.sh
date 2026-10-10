#!/usr/bin/env bash
# blast-snapshots.sh <ref> <outdir>
#
# Writes the blast output of the gruntled built from <ref> on every
# base/cur pair that the blast scripts of <ref> compare (blast_impacted,
# blast_exitcodes, blast_grt004), as committed snapshots for
# TestBlastDepth1MatchesV1. Both the binary and the fixture trees come from
# the <ref> worktree, never from HEAD, so the snapshots describe a
# self-contained input (sec #208). For each pair it writes <case>.txt,
# <case>.json and <case>.exit, where <case> is <script>__<base>__<cur>, with
# the extraction root as cwd and relative paths only. Then PROVENANCE: line 1
# is the resolved commit hash, then "<sha256>  <file>" for every other file,
# sorted.
#
# Offline like compare-ref.sh: GOFLAGS=-mod=readonly GOPROXY=off, the ref
# must name a commit, and the worktree and temp files are removed on exit.
#
# Requirements: bash, git, Go, GNU coreutils `realpath` (for `-m`) and
# `sha256sum`, as on Linux and in CI. macOS/BSD ship neither in that form:
# install coreutils and put its gnubin directory first on PATH (so
# `realpath` and `sha256sum` are the GNU `grealpath` and `gsha256sum`), or
# run the script on Linux or in CI. Without them the script fails closed:
# it exits non-zero and writes nothing outside its temp directory.
set -euo pipefail

# Sourced before any cd, so BASH_SOURCE resolves from the caller's cwd.
# shellcheck source=lib/txtar-extract.sh
. "$(dirname "${BASH_SOURCE[0]}")/lib/txtar-extract.sh"

if [ "$#" -ne 2 ] || [ -z "$1" ] || [ -z "$2" ]; then
  echo "usage: $0 <ref> <outdir>" >&2
  exit 2
fi
ref=$1
outarg=$2

cd "$(dirname "${BASH_SOURCE[0]}")/.."
root=$(pwd)
T=$(mktemp -d)
trap 'git -C "$root" worktree remove --force "$T/ref" >/dev/null 2>&1 || true; rm -rf "$T"' EXIT

export GOFLAGS=-mod=readonly GOPROXY=off

commit=$(git rev-parse --verify --quiet --end-of-options "$ref^{commit}") || {
  echo "blast-snapshots.sh: $ref is not a commit" >&2
  exit 2
}
mkdir -p -- "$outarg"
out=$(cd -- "$outarg" && pwd)

git worktree add --quiet --detach "$T/ref" "$commit"
(cd "$T/ref" && go build -o "$T/gref" ./cmd/gruntled)

for script in blast_impacted blast_exitcodes blast_grt004; do
  d="$T/fx/$script"
  extract "$T/ref/cmd/gruntled/testdata/script/$script.txtar" "$d"
  # Every "blast --base B ... P" line whose two trees exist.
  grep -E '^exec gruntled(-exit [0-9]+)? blast ' "$T/ref/cmd/gruntled/testdata/script/$script.txtar" |
    while read -r -a w; do
      base='' path='' i=0
      while [ "$i" -lt "${#w[@]}" ]; do
        case ${w[$i]} in
          --base) i=$((i + 1)); base=${w[$i]:-} ;;
          --format) i=$((i + 1)) ;;
          --) ;;
          exec | gruntled | gruntled-exit | blast | [0-9]) ;;
          *) path=${w[$i]} ;;
        esac
        i=$((i + 1))
      done
      [ -n "$base" ] && [ -n "$path" ] && [ -d "$d/$base" ] && [ -d "$d/$path" ] || continue
      echo "$base $path"
    done | LC_ALL=C sort -u | while read -r base path; do
      case_=${script}__${base%/}__${path%/}
      rc=0
      (cd "$d" && "$T/gref" blast --base "$base" "$path" >"$out/$case_.txt") || rc=$?
      echo "$rc" >"$out/$case_.exit"
      (cd "$d" && "$T/gref" blast --base "$base" --format json "$path" >"$out/$case_.json") || true
    done
done

(
  cd "$out"
  echo "$commit"
  find . -type f ! -name PROVENANCE | sed 's|^\./||' | LC_ALL=C sort | while read -r f; do
    sha256sum -- "$f"
  done
) >"$T/PROVENANCE"
mv -- "$T/PROVENANCE" "$out/PROVENANCE"
