#!/usr/bin/env bash
# test-fetch-corpora.sh
#
# Offline self-test for scripts/fetch-corpora.sh. It builds file:// fixture
# repositories in a temp dir and points the script at them through
# FETCH_CORPORA_MANIFEST (lines "name url sha"), so no case touches the
# network. Cases:
#   1 correct sha: HEAD is the sha, the ELF file is gone, the symlink to it
#     and the plain file stay, the export line is printed
#   2 unknown sha: exit 1, no dest and no temp dir left
#   3 dest at another commit: exit 1, dest untouched
#   4 rerun on a good dest: exit 0, dest unchanged
#   5 malformed sha in the manifest: exit 2 before any git call
#   6 a .gitattributes filter defined in HOME-level git config never runs
#   7 an init.templateDir hook from HOME-level git config never runs
#   8 an escaping symlink (relative or absolute target): exit 1, no dest
# Cases 6 and 7 first prove that plain git under the same HOME does run the
# filter and the hook, so their "marker absent" checks are not vacuous.
# Prints "ok <case>" per case and "all fetch-corpora cases passed".
#
# Requirements: bash, git, GNU coreutils (realpath, sha256sum), as on Linux.
set -euo pipefail

cd "$(dirname "${BASH_SOURCE[0]}")/.."
script=$PWD/scripts/fetch-corpora.sh
realgit=$(command -v git)

W=$(mktemp -d)
trap 'chmod -R u+w "$W" 2>/dev/null || true; rm -rf "$W"' EXIT
W=$(realpath -e -- "$W")

fail() {
  echo "FAIL $*" >&2
  exit 1
}

# fx runs git for fixture setup only, isolated from the user's config.
fx() {
  env -u GIT_DIR -u GIT_WORK_TREE GIT_CONFIG_NOSYSTEM=1 GIT_CONFIG_GLOBAL=/dev/null \
    "$realgit" -c user.name=fixture -c user.email=fixture@example.invalid \
    -c init.defaultBranch=main -c protocol.file.allow=always "$@"
}

# newrepo <dir>: an empty repo that serves any sha (fetch by sha).
newrepo() {
  mkdir -p "$1"
  fx -C "$1" init -q --template=
  fx -C "$1" config uploadpack.allowAnySHA1InWant true
}

commit() {
  fx -C "$1" add -A
  fx -C "$1" commit -q -m "$2"
  fx -C "$1" rev-parse HEAD
}

# digest <dir>: paths, types, link targets and file hashes, never following
# links, excluding nothing (the .git dir is included on purpose).
digest() {
  (cd "$1" && find -P . -printf '%y %p %m %l\n' | LC_ALL=C sort &&
    find -P . -type f -print0 | LC_ALL=C sort -z | xargs -0 -r sha256sum) | sha256sum
}

# A home whose git config would run code on checkout and on init.
H=$W/home
mkdir -p "$H/tmpl/hooks"
cat >"$H/tmpl/hooks/post-checkout" <<EOF
#!/bin/sh
touch "$W/marker-template"
EOF
chmod +x "$H/tmpl/hooks/post-checkout"
cat >"$H/.gitconfig" <<EOF
[init]
	templateDir = $H/tmpl
[filter "x"]
	smudge = touch $W/marker-filter && cat
	clean = cat
[protocol "file"]
	allow = always
EOF

# run <manifest-text> <dest>: the script under the hostile HOME, with a git
# wrapper on PATH that records every call.
mkdir -p "$W/bin"
cat >"$W/bin/git" <<EOF
#!/bin/sh
echo "\$*" >>"$W/git-calls"
exec "$realgit" "\$@"
EOF
chmod +x "$W/bin/git"
run() {
  printf '%s\n' "$1" >"$W/manifest"
  rm -f "$W/git-calls"
  env -u GIT_CONFIG_GLOBAL -u GIT_CONFIG_NOSYSTEM -u GIT_LFS_SKIP_SMUDGE \
    HOME="$H" XDG_CONFIG_HOME="$H/.config" PATH="$W/bin:$PATH" \
    FETCH_CORPORA_MANIFEST="$W/manifest" bash "$script" "$2" >"$W/out" 2>&1
}

# ok0 <case> <manifest-text> <dest>: run and require exit 0.
ok0() {
  local rc=0
  run "$2" "$3" || rc=$?
  [ "$rc" -eq 0 ] || { cat "$W/out" >&2; fail "$1: exit $rc, want 0"; }
}

no_tmp_left() {
  if [ -d "$1" ] && find "$1" -maxdepth 1 -name '.tmp-*' | grep -q .; then
    fail "$2: temp dir left in $1"
  fi
}

# Fixture 1: a plain file, a fake ELF binary and a symlink to it.
S1=$W/src1
newrepo "$S1"
echo 'resource "x" "y" {}' >"$S1/main.tf"
mkdir -p "$S1/bin"
printf '\177ELF\002\001\001\000fake-binary-payload' >"$S1/bin/terragrunt"
chmod +x "$S1/bin/terragrunt"
ln -s bin/terragrunt "$S1/tg"
sha1=$(commit "$S1" one)
echo 'variable "v" {}' >"$S1/vars.tf"
sha2=$(commit "$S1" two)

# Case 1: correct sha.
D1=$W/d1
ok0 "case 1" "c1 file://$S1 $sha1" "$D1"
[ "$(fx -C "$D1/c1" rev-parse HEAD)" = "$sha1" ] || fail "case 1: HEAD"
[ ! -e "$D1/c1/bin/terragrunt" ] || fail "case 1: ELF file not stripped"
[ -L "$D1/c1/tg" ] || fail "case 1: symlink to the ELF file removed"
[ "$(readlink "$D1/c1/tg")" = bin/terragrunt ] || fail "case 1: symlink target changed"
[ -f "$D1/c1/main.tf" ] || fail "case 1: plain file removed"
[ ! -e "$D1/c1/vars.tf" ] || fail "case 1: content of another commit"
grep -qx "export GRUNTLED_CORPUS_C1=$D1/c1" "$W/out" || { cat "$W/out" >&2; fail "case 1: export line"; }
grep -q 'stripped 1 ELF' "$W/out" || { cat "$W/out" >&2; fail "case 1: strip count"; }
no_tmp_left "$D1" "case 1"
echo "ok 1 correct sha: HEAD pinned, ELF stripped, symlink and plain file kept"

# Case 2: unknown sha.
D2=$W/d2
bad=0123456789abcdef0123456789abcdef01234567
rc=0; run "c1 file://$S1 $bad" "$D2" || rc=$?
[ "$rc" -eq 1 ] || { cat "$W/out" >&2; fail "case 2: exit $rc, want 1"; }
[ ! -e "$D2/c1" ] || fail "case 2: dest created"
no_tmp_left "$D2" "case 2"
echo "ok 2 unknown sha: exit 1, no dest"

# Case 3: dest exists at another commit.
D3=$W/d3
ok0 "case 3 setup" "c1 file://$S1 $sha2" "$D3"
before=$(digest "$D3/c1")
rc=0; run "c1 file://$S1 $sha1" "$D3" || rc=$?
[ "$rc" -eq 1 ] || { cat "$W/out" >&2; fail "case 3: exit $rc, want 1"; }
[ "$(fx -C "$D3/c1" rev-parse HEAD)" = "$sha2" ] || fail "case 3: HEAD moved"
[ "$(digest "$D3/c1")" = "$before" ] || fail "case 3: dest changed"
no_tmp_left "$D3" "case 3"
echo "ok 3 dest at another commit: exit 1, untouched"

# Case 4: rerun on a good dest.
before=$(digest "$D1/c1")
ok0 "case 4" "c1 file://$S1 $sha1" "$D1"
[ "$(digest "$D1/c1")" = "$before" ] || fail "case 4: dest changed on rerun"
grep -qx "export GRUNTLED_CORPUS_C1=$D1/c1" "$W/out" || fail "case 4: export line"
echo "ok 4 rerun: exit 0, unchanged"

# Case 5: malformed sha, refused before any git call.
D5=$W/d5
for s in abc "${sha1}0" "$(echo "$sha1" | tr a-f A-F)" "-$sha1"; do
  rc=0; run "c1 file://$S1 $s" "$D5" || rc=$?
  [ "$rc" -eq 2 ] || { cat "$W/out" >&2; fail "case 5 ($s): exit $rc, want 2"; }
  [ ! -e "$W/git-calls" ] || fail "case 5 ($s): git was called: $(cat "$W/git-calls")"
  [ ! -e "$D5" ] || fail "case 5 ($s): dest created"
done
rc=0; run "c1 file://$S1 $sha1
c2 file://$S1 nothex" "$D5" || rc=$?
[ "$rc" -eq 2 ] && [ ! -e "$W/git-calls" ] || fail "case 5: a bad later line must stop before the first fetch"
echo "ok 5 malformed sha: exit 2 before any git call"

# Case 6: filter from HOME-level config.
S6=$W/src6
newrepo "$S6"
echo '*.txt filter=x' >"$S6/.gitattributes"
echo hello >"$S6/a.txt"
sha6=$(commit "$S6" filter)
rm -f "$W/marker-filter" "$W/marker-template"
env -u GIT_CONFIG_GLOBAL -u GIT_CONFIG_NOSYSTEM HOME="$H" XDG_CONFIG_HOME="$H/.config" \
  "$realgit" clone -q "file://$S6" "$W/plain6" 2>/dev/null
[ -e "$W/marker-filter" ] || fail "case 6: plain git did not run the filter, the case is vacuous"
[ -e "$W/marker-template" ] || fail "case 7: plain git did not run the template hook, the case is vacuous"
rm -f "$W/marker-filter" "$W/marker-template"
D6=$W/d6
ok0 "case 6" "c6 file://$S6 $sha6" "$D6"
[ ! -e "$W/marker-filter" ] || fail "case 6: the HOME-level filter ran"
grep -q 'filter attributes 1' "$W/out" || { cat "$W/out" >&2; fail "case 6: filter attribute not reported"; }
echo "ok 6 HOME-level filter not run (reported)"

# Case 7: template hook from HOME-level config.
[ ! -e "$W/marker-template" ] || fail "case 7: the HOME-level template hook ran"
[ -z "$(ls -A "$D6/c6/.git/hooks" 2>/dev/null)" ] || fail "case 7: hooks copied into the repo"
echo "ok 7 HOME-level init.templateDir hook not run"

# Case 8: escaping symlinks.
for kind in relative absolute; do
  S8=$W/src8-$kind
  newrepo "$S8"
  mkdir -p "$S8/a"
  echo x >"$S8/a/f.tf"
  if [ "$kind" = relative ]; then ln -s ../../outside "$S8/a/out"; else ln -s /etc/hostname "$S8/a/out"; fi
  sha8=$(commit "$S8" "escape $kind")
  D8=$W/d8-$kind
  rc=0; run "c8 file://$S8 $sha8" "$D8" || rc=$?
  [ "$rc" -eq 1 ] || { cat "$W/out" >&2; fail "case 8 ($kind): exit $rc, want 1"; }
  [ ! -e "$D8/c8" ] || fail "case 8 ($kind): dest created"
  no_tmp_left "$D8" "case 8 ($kind)"
done
echo "ok 8 escaping symlink (relative, absolute): exit 1, no dest"

echo "all fetch-corpora cases passed"
