#!/usr/bin/env bash
# release-notes.sh <tag> [changelog]
#
# Prints the CHANGELOG section for tag vX.Y.Z[-suffix] (the body under the
# heading "## [X.Y.Z] - YYYY-MM-DD", up to the next "## [" heading or the
# first link-reference line, surrounding blank lines dropped) to stdout.
#
# Exit 1 when the section is absent or empty (empty notes are a missing
# section), 2 on a malformed tag or usage. The tag is validated against the
# same regex as the release guard and then compared as a fixed-length
# substring, so no character of it is ever read by case, [[ ]], grep or awk.
#
# release.yml feeds the result to gh release create --notes-file; ci.yml
# dry-runs the positive and the negative case on every push and PR.
set -euo pipefail
export LC_ALL=C # [[ =~ ]] bracket ranges stay ASCII

if [ "$#" -lt 1 ] || [ "$#" -gt 2 ]; then
  echo "usage: $0 <tag> [changelog]" >&2
  exit 2
fi
tag=$1
file=${2:-CHANGELOG.md}
if ! [[ "$tag" =~ ^v[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.-]+)?$ ]]; then
  echo "release-notes: tag '$tag' does not match vX.Y.Z[-suffix]" >&2
  exit 2
fi
if [ ! -f "$file" ] || [ ! -r "$file" ]; then
  echo "release-notes: cannot read $file" >&2
  exit 1
fi

want="## [${tag#v}] - "
body=$(
  found=0
  while IFS= read -r line || [ -n "$line" ]; do
    if [ "$found" -eq 1 ]; then
      case "$line" in
        "## ["* | "["*"]: http"*) break ;;
      esac
      printf '%s\n' "$line"
    elif [ "${line:0:${#want}}" = "$want" ]; then
      found=1
    fi
  done < "$file"
)
# Drop leading and trailing blank lines (fixed awk program; the tag is not
# part of it).
body=$(printf '%s\n' "$body" | awk '{ l[NR] = $0 } END { s = 1; n = NR; while (s <= n && l[s] ~ /^[[:space:]]*$/) s++; while (n >= s && l[n] ~ /^[[:space:]]*$/) n--; for (i = s; i <= n; i++) print l[i] }')
if [ -z "${body//[[:space:]]/}" ]; then
  echo "release-notes: no section [${tag#v}] in $file" >&2
  exit 1
fi
printf '%s\n' "$body"
