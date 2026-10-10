# scripts/lib/txtar-extract.sh: safe txtar extraction shared by
# compare-ref.sh and blast-snapshots.sh. Functions only: no set, no trap and
# nothing runs when it is sourced; callers own `set -euo pipefail`, their
# trap and the ref check. Source it as
#   . "$(dirname "${BASH_SOURCE[0]}")/lib/txtar-extract.sh"
# scripts/test-txtar-extract.sh holds its negative tests.
#
# Symlink targets are checked lexically, then again from the link's real
# parent directory, and the created link is resolved with `realpath -m`
# (GNU coreutils): a missing realpath fails closed.

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
  local archive=$1 dest=$2 name line link target destreal parent rel resolved
  mkdir -p -- "$dest"
  while IFS= read -r name; do
    if ! safe_rel "$name"; then
      echo "txtar-extract: $archive: unsafe member name: $name" >&2
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
        echo "txtar-extract: $archive: unsafe symlink: $line" >&2
        exit 1
      fi
      mkdir -p -- "$(dirname -- "$dest/$link")"
      # The lexical check above trusts the path; an earlier link can make
      # a directory of it point elsewhere. Re-check the link's real parent,
      # then the fully resolved target, against the real dest.
      destreal=$(cd -- "$dest" && pwd -P)
      parent=$(cd -- "$(dirname -- "$dest/$link")" && pwd -P)
      case $parent/ in
        "$destreal"/*) ;;
        *)
          echo "txtar-extract: $archive: symlink parent leaves the tree: $line" >&2
          exit 1
          ;;
      esac
      rel=${parent#"$destreal"}
      rel=${rel#/}
      if ! inside_after_link "${rel:+$rel/}$(basename -- "$link")" "$target"; then
        echo "txtar-extract: $archive: unsafe symlink: $line" >&2
        exit 1
      fi
      ln -s -- "$target" "$dest/$link"
      resolved=$(realpath -m -- "$dest/$link")
      case $resolved/ in
        "$destreal"/*) ;;
        *)
          rm -f -- "$dest/$link"
          echo "txtar-extract: $archive: symlink resolves outside the tree: $line" >&2
          exit 1
          ;;
      esac
    done <"$dest/.symlinks"
    rm -f -- "$dest/.symlinks"
  fi
}
