#!/usr/bin/env bash
# test-txtar-extract.sh
#
# Negative tests for the txtar extraction shared by compare-ref.sh and
# blast-snapshots.sh (scripts/lib/txtar-extract.sh) and for their ref
# checks. Each hostile archive must make extract fail (non-zero) without
# writing anything outside its target directory; each hostile ref must be
# refused before any worktree or build. A benign archive must extract, so
# the rejections are not vacuous. Prints "ok <case>" per case and exits
# non-zero on the first unexpected result.
#
# TXTAR_EXTRACT_LIB overrides the library path (used once, before the
# library existed, to run these cases against the functions extracted from
# compare-ref.sh). TXTAR_REF_SCRIPTS overrides the scripts whose ref check
# is tested (default: compare-ref.sh blast-snapshots.sh).
#
# Requirements: bash, git, Go, GNU coreutils `realpath` (for `-m`) and
# `sha256sum`, as on Linux and in CI. macOS/BSD ship neither in that form:
# install coreutils and put its gnubin directory first on PATH (so
# `realpath` and `sha256sum` are the GNU `grealpath` and `gsha256sum`), or
# run the script on Linux or in CI. Without them the script fails closed:
# it exits non-zero and writes nothing outside its temp directory.
set -euo pipefail

cd "$(dirname "${BASH_SOURCE[0]}")/.."
lib=${TXTAR_EXTRACT_LIB:-scripts/lib/txtar-extract.sh}
# shellcheck source=lib/txtar-extract.sh
. "$lib"

W=$(mktemp -d)
trap 'rm -rf "$W"' EXIT

fail() {
  echo "FAIL $*" >&2
  exit 1
}

# listing prints every path under $W except the target dirs and inputs.
listing() {
  (cd "$W" && find . -path ./in -prune -o -path './s/*/dest' -prune -o -print | LC_ALL=C sort)
}

# reject <case> <archive text>: extract must fail and write nothing outside
# $W/s/<case>/dest.
reject() {
  local name=$1 archive="$W/in/$1.txtar" before after rc=0
  mkdir -p "$W/in" "$W/s/$name"
  printf '%s' "$2" >"$archive"
  before=$(listing)
  (set -euo pipefail; extract "$archive" "$W/s/$name/dest") >/dev/null 2>&1 || rc=$?
  after=$(listing)
  [ "$rc" -ne 0 ] || fail "$name: hostile archive accepted"
  [ "$before" = "$after" ] || fail "$name: wrote outside the target dir: $(diff <(echo "$before") <(echo "$after") || true)"
  echo "ok $name"
}

# Benign archive: nested dirs, an empty file, a symlink inside the tree.
mkdir -p "$W/in"
printf -- '-- a/b/c.hcl --\nx = 1\n-- empty.tf --\n-- want.txt --\nskip\n-- _golden/symlinks --\nlinks/c -> ../a/b/c.hcl\n' >"$W/in/benign.txtar"
(set -euo pipefail; extract "$W/in/benign.txtar" "$W/s/benign/dest")
[ -f "$W/s/benign/dest/a/b/c.hcl" ] && [ -f "$W/s/benign/dest/empty.tf" ] && [ ! -e "$W/s/benign/dest/want.txt" ] &&
  [ "$(cat "$W/s/benign/dest/links/c")" = "x = 1" ] || fail "benign archive did not extract"
echo "ok benign"

reject dotdot-name $'-- ../x --\nescaped\n'
reject absolute-name $'-- /abs --\nescaped\n'
reject inner-dotdot-name $'-- a/../../x --\nescaped\n'
reject empty-name $'--  --\nescaped\n'
# A member name cannot contain a newline (a txtar marker is one line); the
# nearest input is a marker split over two lines, which is file content, not
# a member: extract must not create a name holding a newline. The archive
# also carries an unsafe member so the case must still be rejected.
reject newline-name $'-- a\nb --\nx\n-- ../x --\ny\n'
reject backslash-name $'-- a\\..\\..\\x --\nescaped\n'
reject link-escapes $'-- f --\n-- _golden/symlinks --\nl -> ../../etc\n'
reject link-absolute $'-- f --\n-- _golden/symlinks --\nl -> /etc\n'
reject link-name-escapes $'-- f --\n-- _golden/symlinks --\n../l -> f\n'
# Chain: each link alone looks inside, but d -> . makes d/l sit at the root,
# where ../x leaves the tree.
reject link-chain $'-- f --\n-- _golden/symlinks --\nd -> .\nd/l -> ../x\n'
# The target goes through a symlinked directory: lexically d/../x is x,
# but d -> . makes it ../x.
reject link-through-symlink $'-- f --\n-- _golden/symlinks --\nd -> .\nl -> d/../x\n'
# Late link (sec #232): l is checked while d does not exist yet; d -> .
# created afterwards makes l resolve to <dest>/../x.
reject late-link $'-- f --\n-- _golden/symlinks --\nl -> d/../x\nd -> .\n'
# The same, chained far enough to reach an absolute file: each d/.. is
# lexically a no-op but climbs one real level once d -> . exists.
chain=$(printf 'd/../%.0s' $(seq 40))
reject late-link-etc "$(printf -- '-- f --\n-- _golden/symlinks --\nl -> %setc/hostname\nd -> .\n' "$chain")"
# Duplicate link name onto a directory link: without -n, ln follows d and
# writes <dest>/z -> ../z.
reject duplicate-link $'-- f --\n-- _golden/symlinks --\nx/y/d -> ../..\nx/y/d -> ../z\n'
# A link whose parent path goes through an existing symlink.
reject link-under-link $'-- f --\n-- _golden/symlinks --\nd -> sub\nd/l -> f\n-- sub/g --\n'
# Chain: link1 -> link2 -> outside.
reject link-chain-outside $'-- f --\n-- _golden/symlinks --\nlink1 -> link2\nlink2 -> ../outside\n'

scripts=${TXTAR_REF_SCRIPTS:-compare-ref.sh blast-snapshots.sh}
worktrees_before=$(git worktree list --porcelain)
for s in $scripts; do
  [ -f "scripts/$s" ] || fail "scripts/$s missing"
  for ref in --upload-pack=x HEAD:foo no-such-ref-txtar-test; do
    rc=0
    args=("$ref")
    [ "$s" = compare-ref.sh ] || args+=("$W/out")
    err=$(bash "scripts/$s" "${args[@]}" 2>&1 >/dev/null) || rc=$?
    [ "$rc" -ne 0 ] || fail "$s $ref: accepted"
    case $err in *"is not a commit"*) ;; *) fail "$s $ref: not refused by the ref check: $err" ;; esac
    [ ! -e "$W/out" ] || fail "$s $ref: wrote $W/out"
    echo "ok $s ref $ref"
  done
done
[ "$worktrees_before" = "$(git worktree list --porcelain)" ] || fail "a ref case left a worktree"
echo "all txtar-extract cases passed"
