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
  local expect="$3" # "zero", or the name of the rule that must fail
  local rc=0
  bash "$copy/scripts/check-architecture.sh" >/tmp/tca-out.$$ 2>/tmp/tca-err.$$ || rc=$?
  if [ "$expect" = zero ]; then
    if [ "$rc" -ne 0 ]; then
      echo "FAIL $name (expected exit 0, got $rc)"
      cat /tmp/tca-out.$$ /tmp/tca-err.$$ >&2
      rm -f /tmp/tca-out.$$ /tmp/tca-err.$$
      exit 1
    fi
  else
    # A non-zero exit is not enough: the failure must come from the rule
    # this case targets, or the case proves nothing about that rule.
    if [ "$rc" -eq 0 ] || ! grep -q -x -F "=== RULE FAILED: ${expect} ===" /tmp/tca-err.$$; then
      echo "FAIL $name (expected rule ${expect} to fail, got exit $rc)"
      cat /tmp/tca-out.$$ /tmp/tca-err.$$ >&2
      rm -f /tmp/tca-out.$$ /tmp/tca-err.$$
      exit 1
    fi
  fi
  echo "PASS $name"
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
run_case "direct-os" "$copy" domain-stdlib-allowlist

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
run_case "xtest-os" "$copy" domain-stdlib-allowlist

# --- fmt: domain prints via fmt (the old blocklist let this through) -------
copy=$(mkcopy)
cat >"$copy/internal/domain/repograph/zz_probe.go" <<'EOF'
package repograph

import "fmt"

func zzProbe() { fmt.Println("hello") }
EOF
run_case "fmt" "$copy" domain-stdlib-allowlist

# --- log: domain logs to stderr ----------------------------------------------
copy=$(mkcopy)
cat >"$copy/internal/domain/repograph/zz_probe.go" <<'EOF'
package repograph

import "log"

func zzProbe() { log.Print("hello") }
EOF
run_case "log" "$copy" domain-stdlib-allowlist

# --- unsafe: domain imports unsafe --------------------------------------------
copy=$(mkcopy)
cat >"$copy/internal/domain/repograph/zz_probe.go" <<'EOF'
package repograph

import "unsafe"

var zzProbe = unsafe.Sizeof(0)
EOF
run_case "unsafe" "$copy" domain-stdlib-allowlist

# --- windows-file: os imported from a file linux never compiles --------------
copy=$(mkcopy)
cat >"$copy/internal/domain/repograph/zz_probe_windows.go" <<'EOF'
package repograph

import _ "os"
EOF
run_case "windows-file" "$copy" domain-platform-neutral

# --- build-tag: os imported behind a //go:build line --------------------------
copy=$(mkcopy)
cat >"$copy/internal/domain/repograph/zz_probe.go" <<'EOF'
//go:build ignore

package repograph

import _ "os"
EOF
run_case "build-tag" "$copy" domain-platform-neutral

# --- tagged-test: a test file with a build constraint ------------------------
copy=$(mkcopy)
cat >"$copy/internal/domain/repograph/zz_probe_test.go" <<'EOF'
//go:build linux

package repograph_test
EOF
run_case "tagged-test" "$copy" domain-platform-neutral

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
run_case "external-dep" "$copy" domain-external-deps

# --- broken-code: domain file fails to compile ------------------------------
copy=$(mkcopy)
cat >"$copy/internal/domain/repograph/zz_probe.go" <<'EOF'
package repograph

func {
EOF
run_case "broken-code" "$copy" compile-gate

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
run_case "testsupport-linked" "$copy" binary-links-testsupport

# --- vacuous: internal/domain exists but holds no packages ------------------
# (Deleting the directory outright trips compile-gate first, which would not
# prove anything about the guard.)
copy=$(mkcopy)
rm -rf "$copy/internal/domain"
mkdir -p "$copy/internal/domain"
run_case "vacuous" "$copy" non-vacuous-guard

echo "all architecture self-tests passed"
