#!/usr/bin/env bash
# test-check-architecture.sh proves that scripts/check-architecture.sh can
# genuinely fail: every rule it enforces is exercised, once, against a
# throwaway copy of the repo. The real working tree is never modified.
set -euo pipefail
cd "$(dirname "$0")/.."

MODULE=$(go list -m)
COPIES=()

cleanup() {
  for d in "${COPIES[@]}"; do
    rm -rf "$d"
  done
}
trap cleanup EXIT

mkcopy() {
  local d
  d=$(mktemp -d)
  cp -R go.mod "$d/go.mod"
  if [ -f go.sum ]; then
    cp -R go.sum "$d/go.sum"
  fi
  cp -R cmd "$d/cmd"
  cp -R internal "$d/internal"
  cp -R scripts "$d/scripts"
  COPIES+=("$d")
  echo "$d"
}

run_case() {
  local name="$1"
  local copy="$2"
  local expect="$3" # "zero" or "nonzero"
  local rc=0
  bash "$copy/scripts/check-architecture.sh" >/tmp/tca-out.$$ 2>/tmp/tca-err.$$ || rc=$?
  case "$expect" in
    zero)
      if [ "$rc" -eq 0 ]; then
        echo "PASS $name"
      else
        echo "FAIL $name (expected exit 0, got $rc)"
        cat /tmp/tca-out.$$ /tmp/tca-err.$$ >&2
        rm -f /tmp/tca-out.$$ /tmp/tca-err.$$
        exit 1
      fi
      ;;
    nonzero)
      if [ "$rc" -ne 0 ]; then
        echo "PASS $name"
      else
        echo "FAIL $name (expected non-zero exit, got 0)"
        cat /tmp/tca-out.$$ /tmp/tca-err.$$ >&2
        rm -f /tmp/tca-out.$$ /tmp/tca-err.$$
        exit 1
      fi
      ;;
  esac
  rm -f /tmp/tca-out.$$ /tmp/tca-err.$$
}

# --- clean: unmodified copy exits 0 ----------------------------------------
copy=$(mkcopy)
run_case "clean" "$copy" zero

# --- direct-os: domain file imports os -------------------------------------
copy=$(mkcopy)
cat >"$copy/internal/domain/repograph/zz_probe.go" <<'EOF'
package repograph

import _ "os"
EOF
run_case "direct-os" "$copy" nonzero

# --- xtest-os: domain external test package imports os ---------------------
copy=$(mkcopy)
cat >"$copy/internal/domain/repograph/zz_probe_test.go" <<'EOF'
package repograph_test

import (
	_ "os"
	"testing"
)

func TestZZProbe(t *testing.T) {}
EOF
run_case "xtest-os" "$copy" nonzero

# --- external-dep: domain imports a non-domain internal package ------------
copy=$(mkcopy)
mkdir -p "$copy/internal/infrastructure/zzprobe"
cat >"$copy/internal/infrastructure/zzprobe/p.go" <<'EOF'
package zzprobe

const X = 1
EOF
cat >"$copy/internal/domain/repograph/zz_probe.go" <<EOF
package repograph

import _ "${MODULE}/internal/infrastructure/zzprobe"
EOF
run_case "external-dep" "$copy" nonzero

# --- broken-code: domain file fails to compile ------------------------------
copy=$(mkcopy)
cat >"$copy/internal/domain/repograph/zz_probe.go" <<'EOF'
package repograph

func {
EOF
run_case "broken-code" "$copy" nonzero

# --- testsupport-linked: cmd/gruntled links internal/testsupport -----------
copy=$(mkcopy)
mkdir -p "$copy/internal/testsupport/zzprobe"
cat >"$copy/internal/testsupport/zzprobe/p.go" <<'EOF'
package zzprobe

const X = 1
EOF
cat >"$copy/cmd/gruntled/zz_probe.go" <<EOF
package main

import _ "${MODULE}/internal/testsupport/zzprobe"
EOF
run_case "testsupport-linked" "$copy" nonzero

# --- vacuous: internal/domain missing entirely ------------------------------
copy=$(mkcopy)
rm -rf "$copy/internal/domain"
run_case "vacuous" "$copy" nonzero

echo "all architecture self-tests passed"
