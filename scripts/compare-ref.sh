#!/usr/bin/env bash
# compare-ref.sh <ref>
#
# Byte-compares the gruntled built from <ref> (for example v0.3.0) with the
# one built from the working tree, on every check/graph fixture: the
# testdata/golden repositories, the text/json/sarif/graph golden scripts, the
# diag03 scripts, the GRT004 blast trees, clean-fixture and sarif-fixture.
# For each fixture and each of `check --format text|json|sarif` and
# `graph --json` it runs both binaries with the fixture as cwd on ".", then
# compares stdout bytes and exit codes. It prints one line per comparison,
# "same <fixture> <command>" or "DIFF <fixture> <command>", then
# "N/M identical", and exits 1 when anything differs. A difference fails the
# gate; it is never explained away.
#
# COMPARE_REF_CORPORA=<dir>[:<dir>...] adds each directory (for example the
# pinned corpora from scripts/fetch-corpora.sh) as a read-only fixture,
# labelled corpus/<basename>: cwd = the directory, argument ".", the same four
# commands. For corpus rows stdout, stderr and the exit code are compared.
# Each entry must exist and be a directory, else the script exits 2 before
# building anything. A tree digest of each corpus is taken before and after
# the runs, and any change prints "DIFF corpus/<name> digest" and fails the
# run. The digest never follows a symlink (find -P): regular files are
# hashed with their mode, links are recorded as "path -> target", other
# types by type, path and mode. Corpora are never extracted, copied or
# executed; only the two gruntled binaries read them.
#
# COMPARE_REF_PERTURB=1 appends one byte to the first working-tree stdout
# before comparing: the self-test that proves the comparison can fail.
#
# Offline by construction: GOFLAGS=-mod=readonly, GOPROXY=off and
# GOTOOLCHAIN=local make a build that would need a download fail instead. The ref worktree and every
# temp file are removed on exit. <ref> must name a commit (git rev-parse
# --verify --end-of-options); txtar member names and symlink targets must stay
# inside the extracted tree, or the script stops before writing them.
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

if [ "$#" -ne 1 ] || [ -z "$1" ]; then
  echo "usage: $0 <ref>" >&2
  exit 2
fi
ref=$1

# Resolved before the cd below, so relative entries mean the caller's cwd.
corpora=()
if [ -n "${COMPARE_REF_CORPORA:-}" ]; then
  IFS=: read -r -a entries <<<"$COMPARE_REF_CORPORA"
  for e in "${entries[@]}"; do
    if [ -z "$e" ] || ! r=$(realpath -e -- "$e" 2>/dev/null) || [ ! -d "$r" ]; then
      echo "compare-ref.sh: COMPARE_REF_CORPORA entry is not a directory: '$e'" >&2
      exit 2
    fi
    corpora+=("$r")
  done
fi

cd "$(dirname "$0")/.."
root=$(pwd)
T=$(mktemp -d)
trap 'git -C "$root" worktree remove --force "$T/ref" >/dev/null 2>&1 || true; rm -rf "$T"' EXIT

export GOFLAGS=-mod=readonly GOPROXY=off GOTOOLCHAIN=local

# tree_listing <dir>: one sorted line per entry under dir (excluding .git),
# never following a symlink: "f <mode> <sha256>  <path>" for regular files,
# "l <path> -> <target>" for links, "<type> <path> <mode>" for the rest.
tree_listing() {
  (
    cd -- "$1"
    find -P . -path ./.git -prune -o -type f -print0 | LC_ALL=C sort -z >"$T/dl.files"
    # xargs keeps argument order, so line n of both lists is the same file.
    xargs -0 -r stat -c '%a' -- <"$T/dl.files" >"$T/dl.mode"
    xargs -0 -r sha256sum -- <"$T/dl.files" >"$T/dl.sum"
    {
      paste -d' ' "$T/dl.mode" "$T/dl.sum" | sed 's/^/f /'
      find -P . -path ./.git -prune -o -type l -printf 'l %p -> %l\n'
      find -P . -path ./.git -prune -o ! -type f ! -type l -printf '%y %p %m\n'
    } | LC_ALL=C sort
  )
}

tree_digest() {
  tree_listing "$1" | sha256sum | cut -d' ' -f1
}

commit=$(git rev-parse --verify --quiet --end-of-options "$ref^{commit}") || {
  echo "compare-ref.sh: $ref is not a commit" >&2
  exit 2
}
git worktree add --quiet --detach "$T/ref" "$commit"
(cd "$T/ref" && go build -o "$T/gref" ./cmd/gruntled)
go build -o "$T/ghead" ./cmd/gruntled

fixtures=()
td=cmd/gruntled/testdata
for f in "$td"/golden/*.txtar; do
  d="$T/fx/golden/$(basename "$f" .txtar)"
  extract "$f" "$d"
  fixtures+=("$d")
done
for name in text_golden json_golden sarif_golden graph_golden diag03_corpus_shape diag03_issue2163 diag03_no_mocks diag03_silent_rows blast_grt004; do
  f="$td/script/$name.txtar"
  d="$T/fx/script/$name"
  extract "$f" "$d"
  if grep -Eq '^exec gruntled(-exit [0-9]+)? (check|graph)( [^ ]+)* \.( |$)' "$f"; then
    fixtures+=("$d")
  else
    for sub in "$d"/*/; do
      fixtures+=("${sub%/}")
    done
  fi
done
for name in clean-fixture sarif-fixture; do
  mkdir -p "$T/fx/dir"
  cp -R "$td/$name" "$T/fx/dir/$name"
  fixtures+=("$T/fx/dir/$name")
done

ncorpus=${#fixtures[@]}
for d in "${corpora[@]}"; do
  fixtures+=("$d")
done

commands=("check --format text" "check --format json" "check --format sarif" "graph --json")
total=0
same=0
perturb=${COMPARE_REF_PERTURB:-}
digest_fail=0
for i in "${!fixtures[@]}"; do
  d=${fixtures[$i]}
  corpus=
  label=${d#"$T/fx/"}
  if [ "$i" -ge "$ncorpus" ]; then
    corpus=1
    label=corpus/$(basename -- "$d")
    before=$(tree_digest "$d")
  fi
  for c in "${commands[@]}"; do
    total=$((total + 1))
    : >"$T/err.ref"
    : >"$T/err.head"
    if [ -n "$corpus" ]; then
      # shellcheck disable=SC2086 # c is a fixed word list
      rc_ref=0; (cd -- "$d" && "$T/gref" $c . >"$T/out.ref" 2>"$T/err.ref") || rc_ref=$?
      # shellcheck disable=SC2086
      rc_head=0; (cd -- "$d" && "$T/ghead" $c . >"$T/out.head" 2>"$T/err.head") || rc_head=$?
    else
      # shellcheck disable=SC2086
      rc_ref=0; (cd "$d" && "$T/gref" $c . >"$T/out.ref" 2>/dev/null) || rc_ref=$?
      # shellcheck disable=SC2086
      rc_head=0; (cd "$d" && "$T/ghead" $c . >"$T/out.head" 2>/dev/null) || rc_head=$?
    fi
    if [ -n "$perturb" ]; then
      printf 'x' >>"$T/out.head"
      perturb=
    fi
    if [ "$rc_ref" -eq "$rc_head" ] && cmp -s "$T/out.ref" "$T/out.head" &&
      cmp -s "$T/err.ref" "$T/err.head"; then
      same=$((same + 1))
      echo "same $label $c (exit $rc_head)"
    else
      echo "DIFF $label $c (exit ref $rc_ref, head $rc_head)"
    fi
  done
  if [ -n "$corpus" ]; then
    if [ "$(tree_digest "$d")" = "$before" ]; then
      echo "same $label digest $before"
    else
      echo "DIFF $label digest"
      digest_fail=1
    fi
  fi
done

echo "$same/$total identical"
[ "$same" -eq "$total" ] && [ "$digest_fail" -eq 0 ]
