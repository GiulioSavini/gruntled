#!/usr/bin/env bash
# fetch-corpora.sh [dest]
#
# One-time fetch of the three pinned validation corpora into dest (default
# $HOME/.cache/gruntled-phase4/corpus): primary (iso20022), secret and
# denis256, each a detached checkout of its pinned commit. This is the only
# step of the corpus validation that uses the network; after it, no step
# needs the network.
#
# Each corpus is fetched by sha (git fetch --depth 1 <url> <sha>) into a
# temp dir under dest, checked out detached, and verified: HEAD must equal
# the pinned sha and the object must be a commit. A symlink whose target is
# absolute or resolves outside the corpus fails the fetch (none expected).
# Gitlinks and .gitattributes filter= lines are reported. Only then is the
# temp dir renamed into place (mv -T), so a failure leaves no dest behind.
# An existing dest at the pinned HEAD is reused (re-verified, re-stripped);
# one at any other HEAD is refused and never reset.
#
# This script executes, sources and builds nothing from a corpus: it never
# cds into a corpus, and every git call is isolated from system, global and
# template config (GIT_CONFIG_NOSYSTEM, GIT_CONFIG_GLOBAL=/dev/null, an
# empty --template), with hooks, fsmonitor, submodule recursion and LFS
# smudge off and only https allowed. That isolation is the control against
# running corpus content.
#
# Every regular file whose first four bytes are 0x7f 'E' 'L' 'F' is deleted
# from the working tree (never under .git, never through a symlink). This is
# a size measure: denis256 checks in about 1.77 GB of terragrunt binaries
# that no test reads. It is not a security control.
#
# Output: per corpus HEAD, link, gitlink, filter and strip counts, then one
# "export GRUNTLED_CORPUS…=<dir>" line per corpus.
#
# FETCH_CORPORA_MANIFEST=<file> (test only, see test-fetch-corpora.sh)
# replaces the built-in list with lines "name url sha" and also allows
# file:// URLs. A malformed manifest exits 2 before any git call.
#
# Exit: 0 ok, 1 verification failure, 2 usage or manifest error.
# Requirements: bash, git, GNU coreutils (realpath, stat, od), findutils.
set -euo pipefail

if [ "$#" -gt 1 ]; then
  echo "usage: $0 [dest]" >&2
  exit 2
fi
dest=${1:-$HOME/.cache/gruntled-phase4/corpus}

builtin_manifest='primary https://github.com/aws-solutions-library-samples/guidance-for-iso20022-messaging-workflows-on-aws e6c55d11fd1a01e75b78d7897be36c69fa26b8cc
secret https://github.com/cds-snc/secret 341e8a95b0d9bd658793094a248527cc3ebae6f2
denis256 https://github.com/denis256/terragrunt-tests 726485e699a70c02dabbde629f66c0119e197357'

test_mode=
if [ -n "${FETCH_CORPORA_MANIFEST:-}" ]; then
  test_mode=1
  manifest=$(cat -- "$FETCH_CORPORA_MANIFEST")
else
  manifest=$builtin_manifest
fi

# Validate the whole manifest before any git call or any write.
names=() urls=() shas=()
while IFS= read -r line; do
  case $line in '' | '#'*) continue ;; esac
  read -r name url sha extra <<<"$line"
  if [ -n "${extra:-}" ] || [ -z "${sha:-}" ]; then
    echo "fetch-corpora.sh: manifest line needs exactly 'name url sha': $line" >&2
    exit 2
  fi
  if ! [[ $name =~ ^[a-z0-9][a-z0-9_]*$ ]]; then
    echo "fetch-corpora.sh: bad corpus name: $name" >&2
    exit 2
  fi
  if ! [[ $sha =~ ^[0-9a-f]{40}$ ]]; then
    echo "fetch-corpora.sh: $name: sha must be 40 lowercase hex chars: $sha" >&2
    exit 2
  fi
  if [[ $url =~ ^https://[^[:space:]]+$ ]] || { [ -n "$test_mode" ] && [[ $url =~ ^file:///[^[:space:]]+$ ]]; }; then
    :
  else
    echo "fetch-corpora.sh: $name: url not allowed: $url" >&2
    exit 2
  fi
  names+=("$name") urls+=("$url") shas+=("$sha")
done <<<"$manifest"
if [ "${#names[@]}" -eq 0 ]; then
  echo "fetch-corpora.sh: empty manifest" >&2
  exit 2
fi

mkdir -p -- "$dest"
dest=$(realpath -e -- "$dest")

file_allow=()
if [ -n "$test_mode" ]; then
  file_allow=(-c protocol.file.allow=always)
fi

# g runs git isolated from every config but the repo's own, which this
# script creates. GIT_CEILING_DIRECTORIES stops discovery at dest, so a
# missing .git never resolves to a parent repository.
g() {
  env -u GIT_DIR -u GIT_WORK_TREE -u GIT_INDEX_FILE -u GIT_OBJECT_DIRECTORY \
    -u GIT_ALTERNATE_OBJECT_DIRECTORIES -u GIT_CONFIG -u GIT_CONFIG_PARAMETERS \
    -u GIT_CONFIG_COUNT -u GIT_TEMPLATE_DIR -u GIT_EXEC_PATH -u GIT_ASKPASS \
    -u SSH_ASKPASS -u GIT_SSH -u GIT_SSH_COMMAND -u GIT_PROXY_COMMAND \
    GIT_CONFIG_NOSYSTEM=1 GIT_CONFIG_GLOBAL=/dev/null GIT_TERMINAL_PROMPT=0 \
    GIT_LFS_SKIP_SMUDGE=1 GIT_CEILING_DIRECTORIES="$dest" \
    git -c core.hooksPath=/dev/null -c core.fsmonitor=false \
    -c submodule.recurse=false -c protocol.allow=never \
    -c protocol.https.allow=always "${file_allow[@]}" \
    -c advice.detachedHead=false "$@"
}

tmp=
trap 'if [ -n "$tmp" ]; then rm -rf -- "$tmp"; fi' EXIT

# globesc escapes find -path pattern characters in a literal path.
globesc() {
  printf '%s' "$1" | sed 's/[][*?\\]/\\&/g'
}

# verify <name> <wt> <sha>: HEAD and object type, then links, gitlinks and
# filter attributes. Exits 1 on a mismatch or an escaping symlink.
verify() {
  local name=$1 wt=$2 sha=$3 head typ root links=0 esc=0 l t r gitlinks filters
  head=$(g -C "$wt" rev-parse --verify --quiet HEAD) || head=
  typ=$(g -C "$wt" cat-file -t "$sha" 2>/dev/null) || typ=
  if [ "$head" != "$sha" ] || [ "$typ" != commit ]; then
    echo "fetch-corpora.sh: $name: HEAD ${head:-none} ($typ), want commit $sha" >&2
    exit 1
  fi
  echo "$name: HEAD $head (commit)"
  root=$(realpath -e -- "$wt")
  local skip
  skip=$(globesc "$wt/.git")
  while IFS= read -r -d '' l; do
    links=$((links + 1))
    t=$(readlink -- "$l")
    case $t in
      /*)
        echo "fetch-corpora.sh: $name: absolute symlink ${l#"$wt"/} -> $t" >&2
        esc=$((esc + 1))
        continue
        ;;
    esac
    if ! r=$(realpath -m -- "$l" 2>/dev/null); then
      echo "fetch-corpora.sh: $name: unresolvable symlink ${l#"$wt"/} -> $t" >&2
      esc=$((esc + 1))
      continue
    fi
    case $r in
      "$root" | "$root"/*) ;;
      *)
        echo "fetch-corpora.sh: $name: escaping symlink ${l#"$wt"/} -> $t" >&2
        esc=$((esc + 1))
        ;;
    esac
  done < <(find -P "$wt" -path "$skip" -prune -o -type l -print0)
  echo "$name: symlinks $links, escaping $esc"
  if [ "$esc" -ne 0 ]; then
    exit 1
  fi
  gitlinks=$(g -C "$wt" ls-tree -r HEAD | awk '$1 == "160000"' | wc -l)
  echo "$name: gitlinks $gitlinks"
  filters=$(find -P "$wt" -path "$skip" -prune -o -type f -name .gitattributes -print0 |
    xargs -0 -r grep -h 'filter=' | wc -l)
  echo "$name: filter attributes $filters"
}

# strip <name> <wt>: delete regular files that start with the ELF magic.
strip_elf() {
  local name=$1 wt=$2 n=0 bytes=0 f m s skip
  skip=$(globesc "$wt/.git")
  while IFS= read -r -d '' f; do
    m=$(head -c4 -- "$f" | od -An -tx1 | tr -d ' \n')
    if [ "$m" = 7f454c46 ]; then
      s=$(stat -c %s -- "$f")
      rm -f -- "$f"
      n=$((n + 1))
      bytes=$((bytes + s))
    fi
  done < <(find -P "$wt" -path "$skip" -prune -o -type f -size +3c -print0)
  echo "$name: stripped $n ELF files, $bytes bytes"
}

exports=()
for i in "${!names[@]}"; do
  name=${names[$i]} url=${urls[$i]} sha=${shas[$i]}
  final=$dest/$name
  if [ -L "$final" ] || { [ -e "$final" ] && [ ! -d "$final/.git" ]; }; then
    echo "fetch-corpora.sh: $name: $final exists and is not a checkout; refusing" >&2
    exit 1
  fi
  if [ -e "$final" ]; then
    head=$(g -C "$final" rev-parse --verify --quiet HEAD) || head=
    if [ "$head" != "$sha" ]; then
      echo "fetch-corpora.sh: $name: $final is at ${head:-no commit}, want $sha; refusing (never reset)" >&2
      exit 1
    fi
    echo "$name: reusing $final"
    verify "$name" "$final" "$sha"
    strip_elf "$name" "$final"
  else
    tmp=$(mktemp -d -- "$dest/.tmp-$name.XXXX")
    g init -q --template= -- "$tmp"
    echo "$name: fetching $sha from $url"
    g -C "$tmp" fetch -q --depth 1 --no-tags --no-recurse-submodules -- "$url" "$sha" || {
      echo "fetch-corpora.sh: $name: fetch of $sha failed" >&2
      exit 1
    }
    g -C "$tmp" checkout -q --detach FETCH_HEAD
    verify "$name" "$tmp" "$sha"
    strip_elf "$name" "$tmp"
    mv -T -- "$tmp" "$final"
    tmp=
  fi
  var=GRUNTLED_CORPUS
  if [ "$name" != primary ]; then
    var=GRUNTLED_CORPUS_$(printf '%s' "$name" | tr '[:lower:]' '[:upper:]')
  fi
  exports+=("export $var=$final")
done

printf '%s\n' "${exports[@]}"
