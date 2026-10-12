#!/usr/bin/env bash
# corpus-gate.sh
#
# The serial REL-03 corpus gate (17-03): it records, offline, that `check` is
# byte-identical to v0.3.0 on the three pinned corpora, that a self-blast is
# empty, that the independent model agrees with gruntled except for the
# pinned exclusions, that reformat-only transforms give an empty blast, and
# that every mutation of the set M1-M9 is caught with exact equality.
#
# Network: on linux the script first re-executes itself in an empty network
# namespace (`unshare -rn`, marker CORPUS_GATE_INNER=1) and proves it by
# /proc/net/dev listing only `lo` (never /sys/class/net, which still shows
# the host's interfaces). It prints `corpus-gate: network=isolated` or
# `network=available`. CORPUS_GATE_NONET=1 makes a failure to isolate fatal
# (exit 1, no fallback; the corpus CI workflow sets it); CORPUS_GATE_NONET=0
# skips the isolation; unset, the gate falls back and says so. The REL-03
# record must show `isolated`. The corpora must already be fetched
# (scripts/fetch-corpora.sh is the only network step).
#
# Reference binary: v0.3.0 is built from the local tag in a temporary
# worktree, after checking that `v0.3.0^{commit}` is still the pinned
# commit (a moved tag fails), with the resolved sha in the ldflags. It is
# exported as GRUNTLED_REF_BIN for M1's dual oracle.
#
# Memory (8 GB hosts): the gate exports GOMAXPROCS=2 and
# GOFLAGS='-mod=readonly -p=1' (plus GOPROXY=off, GOTOOLCHAIN=local) itself,
# runs one shard at a time, prints `free -m` before each, refuses to start a
# shard with less than 1500 MB available, caps each shard's address space,
# and runs each under `timeout` with `go test -timeout 90m`.
#
# Requires GRUNTLED_CORPUS, GRUNTLED_CORPUS_SECRET and
# GRUNTLED_CORPUS_DENIS256 (the export lines of fetch-corpora.sh).
# CORPUS_GATE_DRYRUN=1 prints the reference-build command and the shard list
# and runs nothing. Output: one line per shard, then `corpus-gate: N/N`;
# exit 1 on any failure. The worktree and temp dir are removed on exit.
set -euo pipefail

if [ "$#" -ne 0 ]; then
  echo "usage: $0 (configured by environment, see the header)" >&2
  exit 2
fi

# 1. Network isolation, before anything else.
if [ "${CORPUS_GATE_INNER:-}" != 1 ] && [ "${CORPUS_GATE_NONET:-}" != 0 ]; then
  if [ "$(uname -s)" = Linux ] && command -v unshare >/dev/null 2>&1 && unshare -rn true 2>/dev/null; then
    exec unshare -rn -- env CORPUS_GATE_INNER=1 bash "$0" "$@"
  fi
  if [ "${CORPUS_GATE_NONET:-}" = 1 ]; then
    echo "corpus-gate: CORPUS_GATE_NONET=1 but an empty network namespace is not available (unshare -rn)" >&2
    exit 1
  fi
fi
ifaces=$(awk 'NR > 2 { sub(":.*", "", $1); print $1 }' /proc/net/dev 2>/dev/null | sort | tr '\n' ' ')
if [ "$ifaces" = "lo " ]; then
  echo "corpus-gate: network=isolated (/proc/net/dev: lo)"
else
  if [ "${CORPUS_GATE_NONET:-}" = 1 ]; then
    echo "corpus-gate: CORPUS_GATE_NONET=1 but /proc/net/dev lists: $ifaces" >&2
    exit 1
  fi
  echo "corpus-gate: network=available (/proc/net/dev: $ifaces)"
fi

# 2. Go environment and memory rules, independent of the caller's shell.
export GOMAXPROCS=2 GOFLAGS='-mod=readonly -p=1' GOPROXY=off GOTOOLCHAIN=local
mem_floor_mb=1500
shard_vmem_kb=4000000

cd "$(dirname "${BASH_SOURCE[0]}")/.."
root=$(pwd)
pinned=e870e2984760f0a674bc87697e3f69329613758e

shards=("compare-ref" "TestCorpusBlastSelf" "TestCorpusBlastModel" "TestCorpusReformat")
for corpus in iso20022 secret denis256; do
  for class in M1 M2 M3 M4 M5 M6 M7 M9 M8; do
    shards+=("TestCorpusMutation17/$corpus/$class")
  done
done

build_cmd='go build -ldflags "-X main.version=v0.3.0 -X main.commit=<resolved v0.3.0 sha>" -o <tmp>/gruntled-v030 ./cmd/gruntled'
if [ "${CORPUS_GATE_DRYRUN:-}" = 1 ]; then
  echo "corpus-gate: reference build (worktree at v0.3.0 == $pinned): $build_cmd"
  for s in "${shards[@]}"; do
    echo "corpus-gate: shard $s"
  done
  echo "corpus-gate: dry run, ${#shards[@]} shards"
  exit 0
fi

for v in GRUNTLED_CORPUS GRUNTLED_CORPUS_SECRET GRUNTLED_CORPUS_DENIS256; do
  if [ -z "${!v:-}" ]; then
    echo "corpus-gate: $v is not set (run scripts/fetch-corpora.sh and its export lines)" >&2
    exit 2
  fi
done

# 3. The reference binary: the pinned v0.3.0 commit, offline.
sha=$(git rev-parse --verify --quiet --end-of-options 'v0.3.0^{commit}') || {
  echo "corpus-gate: tag v0.3.0 not found" >&2
  exit 1
}
if [ "$sha" != "$pinned" ]; then
  echo "corpus-gate: v0.3.0 resolves to $sha, pinned $pinned (moved tag)" >&2
  exit 1
fi

T=$(mktemp -d)
cleanup() {
  git -C "$root" worktree remove --force "$T/ref" >/dev/null 2>&1 || true
  git -C "$root" worktree prune >/dev/null 2>&1 || true
  rm -rf "$T"
}
trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM
trap 'exit 129' HUP

git worktree add --quiet --detach "$T/ref" "$sha"
(cd "$T/ref" && go build -ldflags "-X main.version=v0.3.0 -X main.commit=$sha" -o "$T/gruntled-v030" ./cmd/gruntled)
git worktree remove --force "$T/ref"
export GRUNTLED_REF_BIN="$T/gruntled-v030"
echo "corpus-gate: GRUNTLED_REF_BIN $("$GRUNTLED_REF_BIN" --version)"

# 4. Shards, one at a time.
pass=0
total=0
for s in "${shards[@]}"; do
  total=$((total + 1))
  avail=$(awk '/^MemAvailable:/ { print int($2 / 1024) }' /proc/meminfo)
  echo "corpus-gate: free -m: $(free -m | sed -n 2p | tr -s ' ')"
  if [ "$avail" -lt "$mem_floor_mb" ]; then
    echo "corpus-gate: shard $s refused: ${avail} MB available, floor ${mem_floor_mb} MB" >&2
    echo "corpus-gate: $pass/${#shards[@]}"
    exit 1
  fi
  start=$(date +%s)
  rc=0
  if [ "$s" = compare-ref ]; then
    out=$(COMPARE_REF_CORPORA="$GRUNTLED_CORPUS:$GRUNTLED_CORPUS_SECRET:$GRUNTLED_CORPUS_DENIS256" \
      timeout 1800 bash scripts/compare-ref.sh v0.3.0 2>&1) || rc=$?
    printf '%s\n' "$out" | grep -E '^(DIFF|same corpus/.* digest)|identical$' || true
  else
    test=${s%%/*}
    rest=${s#"$test"}
    pattern="^${test}\$"
    if [ -n "$rest" ]; then
      corpus=$(echo "$rest" | cut -d/ -f2)
      class=$(echo "$rest" | cut -d/ -f3)
      pattern="^${test}\$/^${corpus}\$/^${class}\$"
    fi
    out=$( (ulimit -v "$shard_vmem_kb"; timeout 5400 go test -p=1 -count=1 -timeout 90m -run "$pattern" -v ./cmd/gruntled/) 2>&1) || rc=$?
    printf '%s\n' "$out" | grep -E 'CORPUS-[A-Z-]+ |^(--- FAIL|FAIL|ok)|differences|uncovered|pinned' | sed 's/^ *[a-z_]*_test.go:[0-9]*: //' || true
    if [ "$rc" -eq 0 ] && printf '%s\n' "$out" | grep -qE -- '--- SKIP'; then
      echo "corpus-gate: shard $s skipped a corpus subtest" >&2
      rc=1
    fi
  fi
  secs=$(($(date +%s) - start))
  if [ "$rc" -eq 0 ]; then
    pass=$((pass + 1))
    echo "corpus-gate: shard $s ok ${secs}s"
  else
    echo "corpus-gate: shard $s FAIL (exit $rc) ${secs}s"
  fi
done

echo "corpus-gate: $pass/$total"
[ "$pass" -eq "$total" ]
