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
# COMPARE_REF_PERTURB=1 appends one byte to the first working-tree stdout
# before comparing: the self-test that proves the comparison can fail.
#
# Offline by construction: GOFLAGS=-mod=readonly and GOPROXY=off make a
# build that would need a download fail instead. The ref worktree and every
# temp file are removed on exit. <ref> must name a commit (git rev-parse
# --verify --end-of-options); txtar member names and symlink targets must stay
# inside the extracted tree, or the script stops before writing them.
set -euo pipefail

if [ "$#" -ne 1 ] || [ -z "$1" ]; then
  echo "usage: $0 <ref>" >&2
  exit 2
fi
ref=$1

cd "$(dirname "$0")/.."
root=$(pwd)
T=$(mktemp -d)
trap 'git -C "$root" worktree remove --force "$T/ref" >/dev/null 2>&1 || true; rm -rf "$T"' EXIT

export GOFLAGS=-mod=readonly GOPROXY=off

commit=$(git rev-parse --verify --quiet --end-of-options "$ref^{commit}") || {
  echo "compare-ref.sh: $ref is not a commit" >&2
  exit 2
}
git worktree add --quiet --detach "$T/ref" "$commit"
(cd "$T/ref" && go build -o "$T/gref" ./cmd/gruntled)
go build -o "$T/ghead" ./cmd/gruntled

# safe_rel <name>: fails unless name is a relative path that stays inside
# its root: not absolute, no drive letter or backslash, no ".." segment.
safe_rel() {
  case $1 in
    '' | /* | *\\* | [A-Za-z]:*) return 1 ;;
  esac
  case /$1/ in
    */../*) return 1 ;;
  esac
  return 0
}

# inside_after_link <link> <target>: fails when target, resolved lexically
# from link's directory, leaves the tree.
inside_after_link() {
  local depth=0 seg dir
  case $2 in /* | *\\* | [A-Za-z]:*) return 1 ;; esac
  dir=$(dirname -- "$1")
  [ "$dir" = . ] && dir=
  local parts
  IFS=/ read -r -a parts <<<"$dir/$2"
  for seg in "${parts[@]}"; do
    case $seg in
      '' | .) ;;
      ..) depth=$((depth - 1)); [ "$depth" -ge 0 ] || return 1 ;;
      *) depth=$((depth + 1)) ;;
    esac
  done
  return 0
}

# extract <txtar> <dest>: writes every file of the archive under dest, except
# want* expectation files and the _golden/ section; _golden/symlinks lines
# ("link -> target") become symlinks. Member names and link targets are
# checked before anything is created, directories are made from bash with
# a quoted argv (no shell built from a name), and awk only writes files.
extract() {
  local archive=$1 dest=$2 name line link target
  mkdir -p -- "$dest"
  while IFS= read -r name; do
    if ! safe_rel "$name"; then
      echo "compare-ref.sh: $archive: unsafe member name: $name" >&2
      exit 1
    fi
    mkdir -p -- "$(dirname -- "$dest/$name")"
  done < <(awk '
    function base(p) { sub(/.*\//, "", p); return p }
    /^-- .* --$/ {
      name = substr($0, 4, length($0) - 6)
      if (name ~ /^_golden\// || base(name) ~ /^want/) next
      print name
    }' "$archive")
  awk -v dest="$dest" -v links="$dest/.symlinks" '
    function base(p) { sub(/.*\//, "", p); return p }
    /^-- .* --$/ {
      if (out != "") close(out)
      name = substr($0, 4, length($0) - 6)
      out = ""; inlinks = 0
      if (name == "_golden/symlinks") { inlinks = 1; next }
      if (name ~ /^_golden\// || base(name) ~ /^want/) next
      out = dest "/" name
      printf "" > out
      next
    }
    inlinks { print > links; next }
    out != "" { print > out }
  ' "$archive"
  if [ -f "$dest/.symlinks" ]; then
    while IFS= read -r line; do
      [ -n "$line" ] || continue
      link=${line%% -> *}
      target=${line#* -> }
      if ! safe_rel "$link" || ! inside_after_link "$link" "$target"; then
        echo "compare-ref.sh: $archive: unsafe symlink: $line" >&2
        exit 1
      fi
      mkdir -p -- "$(dirname -- "$dest/$link")"
      ln -s -- "$target" "$dest/$link"
    done <"$dest/.symlinks"
    rm -f -- "$dest/.symlinks"
  fi
}

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

commands=("check --format text" "check --format json" "check --format sarif" "graph --json")
total=0
same=0
perturb=${COMPARE_REF_PERTURB:-}
for d in "${fixtures[@]}"; do
  label=${d#"$T/fx/"}
  for c in "${commands[@]}"; do
    total=$((total + 1))
    # shellcheck disable=SC2086 # c is a fixed word list
    rc_ref=0; (cd "$d" && "$T/gref" $c . >"$T/out.ref" 2>/dev/null) || rc_ref=$?
    # shellcheck disable=SC2086
    rc_head=0; (cd "$d" && "$T/ghead" $c . >"$T/out.head" 2>/dev/null) || rc_head=$?
    if [ -n "$perturb" ]; then
      printf 'x' >>"$T/out.head"
      perturb=
    fi
    if [ "$rc_ref" -eq "$rc_head" ] && cmp -s "$T/out.ref" "$T/out.head"; then
      same=$((same + 1))
      echo "same $label $c (exit $rc_head)"
    else
      echo "DIFF $label $c (exit ref $rc_ref, head $rc_head)"
    fi
  done
done

echo "$same/$total identical"
[ "$same" -eq "$total" ]
