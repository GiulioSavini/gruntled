#!/usr/bin/env bash
# compat-v1.sh [ref]
#
# DAEMON-09 against the real previous release (default v0.3.0, protocol v1):
# builds gruntled from <ref> in a temporary git worktree and from the working
# tree, then runs the mismatch matrix with real daemons:
#   (a) <ref> watch, working-tree report, report --blast and rebase;
#   (b) working-tree watch, <ref> report.
# Every client must exit 3 with an empty stdout and exactly the
# "daemon runs protocol vX, this gruntled speaks vY; restart the daemon"
# line on stderr: a mismatch is never an empty or wrong answer. The last
# matrix line asserts that no process mentioning the temp dir survived, so
# a leaked daemon fails the gate. It prints one line per check, "ok <what>"
# or "DIFF <what>", then "compat-v1: N/M", and exits 1 on any mismatch.
#
# Offline by construction: GOFLAGS=-mod=readonly, GOPROXY=off and
# GOTOOLCHAIN=local make a build that would need a download fail instead.
# <ref> must name a commit (git rev-parse --verify --end-of-options). Nothing
# is written outside the temp dir. On exit, ^C, TERM or HUP the daemons are
# stopped (TERM, up to 5 s, then KILL) before the worktree and the temp dir
# are removed: background jobs of a non-interactive bash ignore SIGINT.
set -euo pipefail

ref=${1:-v0.3.0}
if [ "$#" -gt 1 ] || [ -z "$ref" ]; then
  echo "usage: $0 [ref]" >&2
  exit 2
fi

cd "$(dirname "$0")/.."
root=$(pwd)
export GOFLAGS=-mod=readonly GOPROXY=off GOTOOLCHAIN=local

commit=$(git rev-parse --verify --quiet --end-of-options "$ref^{commit}") || {
  echo "compat-v1.sh: $ref is not a commit" >&2
  exit 2
}

T=$(mktemp -d)
pids=()
finished=0

cleanup() {
  local rc=$?
  trap - EXIT INT TERM HUP
  local pid i
  for pid in ${pids[@]+"${pids[@]}"}; do
    kill -TERM "$pid" 2>/dev/null || true
  done
  for i in $(seq 50); do
    alive=0
    for pid in ${pids[@]+"${pids[@]}"}; do
      if kill -0 "$pid" 2>/dev/null; then alive=1; fi
    done
    [ "$alive" -eq 1 ] || break
    sleep 0.1
  done
  for pid in ${pids[@]+"${pids[@]}"}; do
    kill -KILL "$pid" 2>/dev/null || true
    wait "$pid" 2>/dev/null || true
  done
  git -C "$root" worktree remove --force "$T/ref" >/dev/null 2>&1 || true
  git -C "$root" worktree prune >/dev/null 2>&1 || true
  rm -rf "$T" || true
  # Interrupted before the end: never report success.
  if [ "$finished" -ne 1 ] && [ "$rc" -eq 0 ]; then
    rc=1
  fi
  exit "$rc"
}
trap cleanup EXIT INT TERM HUP

git worktree add --quiet --detach "$T/ref" "$commit"
(cd "$T/ref" && go build -o "$T/gref" ./cmd/gruntled)
go build -o "$T/ghead" ./cmd/gruntled
mkdir -p "$T/repo"
cp -R cmd/gruntled/testdata/sarif-fixture/. "$T/repo/"

total=0
passed=0
pass() {
  total=$((total + 1))
  passed=$((passed + 1))
  echo "ok $1"
}
fail() {
  total=$((total + 1))
  echo "DIFF $1"
}

# client <rt> <bin> <args...>: runs one client under timeout 20 with the
# daemon's runtime dir; sets rc, out and err.
client() {
  local rt=$1 bin=$2
  shift 2
  set +e
  XDG_RUNTIME_DIR=$rt TMPDIR=$rt timeout 20 "$bin" "$@" >"$T/out" 2>"$T/err"
  rc=$?
  set -e
  out=$(cat "$T/out")
  err=$(cat "$T/err")
}

# start <name> <bin> <rt>: one daemon, started as a plain background job so
# that $! is the daemon itself.
start() {
  local name=$1 bin=$2 rt=$3
  mkdir -m 700 "$rt"
  XDG_RUNTIME_DIR=$rt TMPDIR=$rt "$bin" watch --poll --poll-interval 50ms --status-file "$T/st/status" "$T/repo" >/dev/null 2>"$T/$name.err" &
  pids+=($!)
}

# ready <what> <rt> <bin>: polls report until it exits 1 (the fixture has
# findings), for at most 30 s.
ready() {
  local what=$1 rt=$2 bin=$3 i
  for i in $(seq 300); do
    client "$rt" "$bin" report "$T/repo"
    if [ "$rc" -eq 1 ]; then
      pass "$what ready"
      return
    fi
    sleep 0.1
  done
  fail "$what ready: last report exit $rc, stderr: $err"
}

# stop: SIGTERM the last daemon and wait for it, up to 5 s, then KILL.
stop() {
  local pid=${pids[${#pids[@]}-1]} i
  kill -TERM "$pid" 2>/dev/null || true
  for i in $(seq 50); do
    kill -0 "$pid" 2>/dev/null || break
    sleep 0.1
  done
  kill -KILL "$pid" 2>/dev/null || true
  wait "$pid" 2>/dev/null || true
}

# expect <what> <want-stderr> <rt> <bin> <args...>: exit 3, empty stdout,
# stderr exactly want.
expect() {
  local what=$1 want=$2
  shift 2
  client "$@"
  if [ "$rc" -eq 3 ] && [ -z "$out" ] && [ "$err" = "$want" ]; then
    pass "$what"
  else
    fail "$what: exit $rc, stdout ${#out} bytes, stderr: $err"
  fi
}

v1v2="gruntled: daemon runs protocol v1, this gruntled speaks v2; restart the daemon"
v2v1="gruntled: daemon runs protocol v2, this gruntled speaks v1; restart the daemon"

# (a) the previous release's daemon, this client.
start a "$T/gref" "$T/r1"
ready "$ref watch" "$T/r1" "$T/gref"
expect "$ref watch, HEAD report" "$v1v2" "$T/r1" "$T/ghead" report "$T/repo"
expect "$ref watch, HEAD report --blast" "$v1v2" "$T/r1" "$T/ghead" report --blast "$T/repo"
expect "$ref watch, HEAD rebase" "$v1v2" "$T/r1" "$T/ghead" rebase "$T/repo"
stop

# (b) this daemon, the previous release's client.
start b "$T/ghead" "$T/r2"
ready "HEAD watch" "$T/r2" "$T/ghead"
expect "HEAD watch, $ref report" "$v2v1" "$T/r2" "$T/gref" report "$T/repo"
stop

if leaked=$(pgrep -f -- "$T"); then
  fail "no daemon left running: $leaked"
else
  pass "no daemon left running"
fi

echo "compat-v1: $passed/$total"
finished=1
[ "$passed" -eq "$total" ]
