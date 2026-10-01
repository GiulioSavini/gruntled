#!/usr/bin/env bash
# build-release.sh <version> <commit> <outdir>
#
# Builds static gruntled binaries for every release target, packs each into
# an archive (.tar.gz for linux/darwin, .zip for windows) holding the binary
# and LICENSE with no path prefix, writes checksums.txt and verifies it, then
# smoke-runs the host target's binary to prove the -ldflags injection worked.
#
# Archive names keep the leading "v" of the version:
#   gruntled_<version>_<os>_<arch>.tar.gz|zip
#
# Used by ci.yml on every push/PR (version v0.0.0-ci) and by the release
# workflow on tags, so the packaging path is exercised long before a tag.
set -euo pipefail

if [ "$#" -ne 3 ] || [ -z "$1" ] || [ -z "$2" ] || [ -z "$3" ]; then
  echo "usage: $0 <version> <commit> <outdir>" >&2
  exit 2
fi
version=$1
commit=$2
mkdir -p "$3"
outdir=$(cd "$3" && pwd)

if ! command -v zip >/dev/null 2>&1; then
  echo "build-release.sh: 'zip' not found; it is required for the windows archives" >&2
  exit 1
fi

cd "$(dirname "$0")/.."

export CGO_ENABLED=0 GOWORK=off

# Keep identical to release_targets in scripts/check-architecture.sh
# Step 9 (binary-no-net-no-exec): every target shipped must be proven.
# cmd/gruntled/release_test.go fails if the two lists differ.
release_targets="linux/amd64 linux/arm64 darwin/amd64 darwin/arm64 windows/amd64 windows/arm64"

work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT

for target in $release_targets; do
  goos=${target%/*}
  goarch=${target#*/}
  stage="$work/$goos-$goarch"
  mkdir -p "$stage"
  bin=gruntled
  [ "$goos" = windows ] && bin=gruntled.exe
  GOOS="$goos" GOARCH="$goarch" go build -trimpath \
    -ldflags "-s -w -X main.version=$version -X main.commit=$commit" \
    -o "$stage/$bin" ./cmd/gruntled
  cp LICENSE "$stage/LICENSE"
  name="gruntled_${version}_${goos}_${goarch}"
  if [ "$goos" = windows ]; then
    (cd "$stage" && zip -q -X "$outdir/$name.zip" "$bin" LICENSE)
  else
    (cd "$stage" && tar -czf "$outdir/$name.tar.gz" "$bin" LICENSE)
  fi
  echo "built $name"
done

(cd "$outdir" && sha256sum gruntled_* >checksums.txt && sha256sum -c checksums.txt)

host="$(go env GOHOSTOS)/$(go env GOHOSTARCH)"
case " $release_targets " in
  *" $host "*) ;;
  *)
    echo "build-release.sh: host $host is not a release target; cannot smoke-test --version" >&2
    exit 1
    ;;
esac
h_os=${host%/*}
h_arch=${host#*/}
smoke="$work/smoke"
mkdir -p "$smoke"
if [ "$h_os" = windows ]; then
  (cd "$smoke" && unzip -q "$outdir/gruntled_${version}_${h_os}_${h_arch}.zip")
  smoke_bin="$smoke/gruntled.exe"
else
  tar -xzf "$outdir/gruntled_${version}_${h_os}_${h_arch}.tar.gz" -C "$smoke"
  smoke_bin="$smoke/gruntled"
fi
got=$("$smoke_bin" --version)
want="gruntled $version ($commit)"
if [ "$got" != "$want" ]; then
  echo "build-release.sh: --version smoke failed: got '$got', want '$want'" >&2
  exit 1
fi
echo "smoke: $got"
