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

addstub() {
  local copy="$1" org="$2"
  mkdir -p "$copy/zzstub/${org}"
  cat >"$copy/zzstub/${org}/go.mod" <<EOF
module github.com/${org}/zzprobe

go 1.27
EOF
  cat >"$copy/zzstub/${org}/p.go" <<EOF
package zzprobe

const X = 1
EOF
  (cd "$copy" && go mod edit \
    -require="github.com/${org}/zzprobe@v0.0.0" \
    -replace="github.com/${org}/zzprobe=./zzstub/${org}")
}

run_case() {
  local name="$1"
  local copy="$2"
  local expect="$3" # "zero", or a space-separated list of rule names that must fail
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
    # A non-zero exit is not enough: the failure must come from every rule
    # this case targets, or the case proves nothing about those rules.
    if [ "$rc" -eq 0 ]; then
      echo "FAIL $name (expected rule(s) '${expect}' to fail, got exit $rc)"
      cat /tmp/tca-out.$$ /tmp/tca-err.$$ >&2
      rm -f /tmp/tca-out.$$ /tmp/tca-err.$$
      exit 1
    fi
    for rule in $expect; do
      if ! grep -q -x -F "=== RULE FAILED: ${rule} ===" /tmp/tca-err.$$; then
        echo "FAIL $name (expected rule ${rule} to fail, got exit $rc)"
        cat /tmp/tca-out.$$ /tmp/tca-err.$$ >&2
        rm -f /tmp/tca-out.$$ /tmp/tca-err.$$
        exit 1
      fi
    done
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

# --- app-fmt: application file imports fmt -----------------------------
copy=$(mkcopy)
cat >"$copy/internal/application/ports/zz_probe.go" <<'EOF'
package ports

import "fmt"

func zzProbe() string { return fmt.Sprint("hello") }
EOF
run_case "app-fmt" "$copy" application-stdlib-allowlist

# --- app-xtest-os: application external test package imports os --------
copy=$(mkcopy)
cat >"$copy/internal/application/ports/zz_probe_test.go" <<'EOF'
package ports_test

import (
	_ "os"
	"testing"
)

func TestZZProbe(t *testing.T) {}
EOF
run_case "app-xtest-os" "$copy" application-stdlib-allowlist

# --- app-external-dep: application imports internal/infrastructure -----
copy=$(mkcopy)
mkdir -p "$copy/internal/infrastructure/zzprobe"
cat >"$copy/internal/infrastructure/zzprobe/p.go" <<'EOF'
package zzprobe

const X = 1
EOF
cat >"$copy/internal/application/ports/zz_probe.go" <<EOF
package ports

import _ "${MODULE}/internal/infrastructure/zzprobe"
EOF
run_case "app-external-dep" "$copy" application-external-deps

# --- app-testsupport-in-test: application test imports internal/testsupport
copy=$(mkcopy)
cat >"$copy/internal/application/ports/zz_probe_test.go" <<EOF
package ports_test

import (
	_ "${MODULE}/internal/testsupport/synthrepo"
	"testing"
)

func TestZZProbe(t *testing.T) {}
EOF
run_case "app-testsupport-in-test" "$copy" application-external-deps

# --- app-windows-file: os imported from a file linux never compiles ------
copy=$(mkcopy)
cat >"$copy/internal/application/ports/zz_probe_windows.go" <<'EOF'
package ports

import _ "os"
EOF
run_case "app-windows-file" "$copy" application-platform-neutral

# --- app-vacuous: internal/application exists but holds no packages ------
copy=$(mkcopy)
rm -rf "$copy/internal/application"
mkdir -p "$copy/internal/application"
run_case "app-vacuous" "$copy" application-non-vacuous-guard

# --- hcl-in-cmd: cmd/gruntled imports hashicorp/hcl directly --------------
copy=$(mkcopy)
addstub "$copy" hashicorp
cat >"$copy/cmd/gruntled/zz_probe.go" <<'EOF'
package main

import _ "github.com/hashicorp/zzprobe"
EOF
run_case "hcl-in-cmd" "$copy" hcl-only-in-infrastructure

# --- zclconf-in-testsupport-test: testsupport test imports zclconf/go-cty
copy=$(mkcopy)
addstub "$copy" zclconf
cat >"$copy/internal/testsupport/synthrepo/zz_probe_test.go" <<'EOF'
package synthrepo_test

import _ "github.com/zclconf/zzprobe"
EOF
run_case "zclconf-in-testsupport-test" "$copy" hcl-only-in-infrastructure

# --- hcl-in-infrastructure-allowed: infrastructure importing HCL is fine -
copy=$(mkcopy)
addstub "$copy" hashicorp
mkdir -p "$copy/internal/infrastructure/zzprobe"
cat >"$copy/internal/infrastructure/zzprobe/p.go" <<'EOF'
package zzprobe

import _ "github.com/hashicorp/zzprobe"
EOF
run_case "hcl-in-infrastructure-allowed" "$copy" zero

# --- layout-unknown-dir: a new top-level internal/<x> dir, code + hole ---
copy=$(mkcopy)
mkdir -p "$copy/internal/analysis"
cat >"$copy/internal/analysis/a.go" <<EOF
package analysis

import (
	_ "os"
	_ "${MODULE}/internal/infrastructure/hclconv"
)
EOF
run_case "layout-unknown-dir" "$copy" "internal-layout infrastructure-importers"

# --- layout-stray-go-file: a .go file directly in internal/ --------------
copy=$(mkcopy)
cat >"$copy/internal/zz_probe.go" <<'EOF'
package internal
EOF
run_case "layout-stray-go-file" "$copy" internal-layout

# --- layout-interfaces-allowed: internal/interfaces is reserved, allowed -
copy=$(mkcopy)
mkdir -p "$copy/internal/interfaces/zzprobe"
cat >"$copy/internal/interfaces/zzprobe/p.go" <<'EOF'
package zzprobe

const X = 1
EOF
run_case "layout-interfaces-allowed" "$copy" zero

# --- layout-non-go-dir-allowed: a non-Go directory under internal/ -------
copy=$(mkcopy)
mkdir -p "$copy/internal/zznotes"
cat >"$copy/internal/zznotes/README.md" <<'EOF'
notes
EOF
run_case "layout-non-go-dir-allowed" "$copy" zero

# --- infra-from-testsupport: testsupport package imports infrastructure --
copy=$(mkcopy)
mkdir -p "$copy/internal/testsupport/zzprobe"
cat >"$copy/internal/testsupport/zzprobe/p.go" <<EOF
package zzprobe

import _ "${MODULE}/internal/infrastructure/hclconv"
EOF
run_case "infra-from-testsupport" "$copy" infrastructure-importers

# --- infra-from-tagged-file: same import, but only from a _windows.go ----
copy=$(mkcopy)
mkdir -p "$copy/internal/testsupport/zzprobe"
cat >"$copy/internal/testsupport/zzprobe/p.go" <<'EOF'
package zzprobe
EOF
cat >"$copy/internal/testsupport/zzprobe/zz_windows.go" <<EOF
package zzprobe

import _ "${MODULE}/internal/infrastructure/hclconv"
EOF
run_case "infra-from-tagged-file" "$copy" infrastructure-importers

# --- infra-from-cmd-allowed: cmd/... may import infrastructure -----------
copy=$(mkcopy)
cat >"$copy/cmd/gruntled/zz_probe.go" <<EOF
package main

import _ "${MODULE}/internal/infrastructure/hclconv"
EOF
run_case "infra-from-cmd-allowed" "$copy" zero

# --- testsupport-in-prod: prod package imports internal/testsupport ------
copy=$(mkcopy)
cat >"$copy/internal/infrastructure/tfsurface/zz_probe.go" <<EOF
package tfsurface

import _ "${MODULE}/internal/testsupport/synthrepo"
EOF
run_case "testsupport-in-prod" "$copy" testsupport-only-in-tests

# --- testsupport-in-tagged-prod: same, but only from a _windows.go -------
copy=$(mkcopy)
cat >"$copy/internal/infrastructure/tfsurface/zz_probe_windows.go" <<EOF
package tfsurface

import _ "${MODULE}/internal/testsupport/synthrepo"
EOF
run_case "testsupport-in-tagged-prod" "$copy" testsupport-only-in-tests

# --- hcl-windows-file-in-cmd: HCL imported from a file linux never compiles
copy=$(mkcopy)
addstub "$copy" hashicorp
cat >"$copy/cmd/gruntled/zz_windows.go" <<'EOF'
package main

import _ "github.com/hashicorp/zzprobe"
EOF
run_case "hcl-windows-file-in-cmd" "$copy" hcl-only-in-infrastructure

# --- hcl-tagged-in-testsupport: go-cty imported behind a //go:build tag ----
copy=$(mkcopy)
addstub "$copy" zclconf
mkdir -p "$copy/internal/testsupport/zz"
cat >"$copy/internal/testsupport/zz/zz.go" <<'EOF'
//go:build integration

package zz

import _ "github.com/zclconf/zzprobe"
EOF
run_case "hcl-tagged-in-testsupport" "$copy" hcl-only-in-infrastructure

# --- hcl-aliased-import-block: aliased spec inside an import block ---------
copy=$(mkcopy)
addstub "$copy" hashicorp
cat >"$copy/cmd/gruntled/zz_probe_windows.go" <<'EOF'
package main

import (
	x "github.com/hashicorp/zzprobe"
)

var _ = x.X
EOF
run_case "hcl-aliased-import-block" "$copy" hcl-only-in-infrastructure

# --- hcl-tagged-in-infrastructure-allowed: tagged HCL import in infra ------
copy=$(mkcopy)
addstub "$copy" hashicorp
mkdir -p "$copy/internal/infrastructure/zzprobe"
cat >"$copy/internal/infrastructure/zzprobe/zz_windows.go" <<'EOF'
package zzprobe

import _ "github.com/hashicorp/zzprobe"
EOF
run_case "hcl-tagged-in-infrastructure-allowed" "$copy" zero

# --- hcl-mention-in-comment-allowed: naming HCL in a comment is fine -------
copy=$(mkcopy)
cat >"$copy/internal/domain/repograph/zz_probe.go" <<'EOF'
package repograph

// see "github.com/hashicorp/hcl/v2" for the grammar
EOF
run_case "hcl-mention-in-comment-allowed" "$copy" zero

# --- interfaces-infra-dep: presenter imports internal/infrastructure -----
# infrastructure-importers (Step 7) already forbids the import; the new
# interfaces-external-deps rule must fire too.
copy=$(mkcopy)
mkdir -p "$copy/internal/infrastructure/zzprobe"
cat >"$copy/internal/infrastructure/zzprobe/p.go" <<'EOF'
package zzprobe

const X = 1
EOF
cat >"$copy/internal/interfaces/presenter/zz_probe.go" <<EOF
package presenter

import _ "${MODULE}/internal/infrastructure/zzprobe"
EOF
run_case "interfaces-infra-dep" "$copy" "interfaces-external-deps infrastructure-importers"

# --- interfaces-testsupport-in-test: presenter test imports testsupport --
copy=$(mkcopy)
cat >"$copy/internal/interfaces/presenter/zz_probe_test.go" <<EOF
package presenter_test

import (
	_ "${MODULE}/internal/testsupport/synthrepo"
	"testing"
)

func TestZZProbe(t *testing.T) {}
EOF
run_case "interfaces-testsupport-in-test" "$copy" interfaces-external-deps

# --- interfaces-os: presenter imports os ----------------------------------
copy=$(mkcopy)
cat >"$copy/internal/interfaces/presenter/zz_probe.go" <<'EOF'
package presenter

import _ "os"
EOF
run_case "interfaces-os" "$copy" interfaces-stdlib-allowlist

# --- interfaces-windows-file: os imported from a file linux never compiles
copy=$(mkcopy)
cat >"$copy/internal/interfaces/presenter/zz_probe_windows.go" <<'EOF'
package presenter

import _ "os"
EOF
run_case "interfaces-windows-file" "$copy" interfaces-platform-neutral

# --- interfaces-vacuous: internal/interfaces exists but holds no packages -
# Once cmd/gruntled imports the presenter (03-03), compile-gate also fails
# here, but the guard prints first because it runs in Step 0, and run_case
# greps for the guard's own line.
copy=$(mkcopy)
rm -rf "$copy/internal/interfaces"
mkdir -p "$copy/internal/interfaces"
run_case "interfaces-vacuous" "$copy" interfaces-non-vacuous-guard

# --- binary-os-exec: cmd/gruntled links os/exec ---------------------------
copy=$(mkcopy)
cat >"$copy/cmd/gruntled/zz_probe.go" <<'EOF'
package main

import _ "os/exec"
EOF
run_case "binary-os-exec" "$copy" binary-no-net-no-exec

# --- binary-net: cmd/gruntled links net -----------------------------------
copy=$(mkcopy)
cat >"$copy/cmd/gruntled/zz_probe.go" <<'EOF'
package main

import _ "net"
EOF
run_case "binary-net" "$copy" binary-no-net-no-exec

# --- binary-start-process: os.StartProcess, invisible to an import list ---
copy=$(mkcopy)
cat >"$copy/cmd/gruntled/zz_probe.go" <<'EOF'
package main

import "os"

func zzProbe() { _, _ = os.StartProcess("x", nil, &os.ProcAttr{}) }
EOF
run_case "binary-start-process" "$copy" binary-no-net-no-exec

# --- binary-exec-windows-file: os/exec only in a _windows.go file ---------
# A linux go list never sees this file; only the windows/amd64 iteration of
# the release-target loop does, so this proves the loop covers non-host
# targets.
copy=$(mkcopy)
cat >"$copy/cmd/gruntled/zz_probe_windows.go" <<'EOF'
package main

import _ "os/exec"
EOF
run_case "binary-exec-windows-file" "$copy" binary-no-net-no-exec

# --- binary-exec-in-test-allowed: test-only os/exec is not in the binary --
copy=$(mkcopy)
cat >"$copy/cmd/gruntled/zz_probe_test.go" <<'EOF'
package main

import (
	_ "os/exec"
	"testing"
)

func TestZZProbe(t *testing.T) {}
EOF
run_case "binary-exec-in-test-allowed" "$copy" zero

# --- hcl-comment-in-import-block: a /* */ comment before the spec (G21) ----
# The old line-based scan required a spec line to begin with the quote, so
# a comment in front of it hid the import. The Go parser does not care.
copy=$(mkcopy)
cat >"$copy/cmd/gruntled/zz_probe_windows.go" <<'EOF'
package main

import ( /* x */ _ "github.com/hashicorp/hcl/v2" )
EOF
run_case "hcl-comment-in-import-block" "$copy" hcl-only-in-infrastructure

# --- hcl-semicolon-import-block: two specs on one line, split by ; (G21) --
copy=$(mkcopy)
cat >"$copy/cmd/gruntled/zz_probe_windows.go" <<'EOF'
package main

import ( _ "fmt"; _ "github.com/hashicorp/hcl/v2" )
EOF
run_case "hcl-semicolon-import-block" "$copy" hcl-only-in-infrastructure

# --- infra-comment-in-import-block: same comment bypass, Step 7 ----------
copy=$(mkcopy)
mkdir -p "$copy/internal/testsupport/zz"
cat >"$copy/internal/testsupport/zz/zz_windows.go" <<EOF
package zz

import ( /* x */ _ "${MODULE}/internal/infrastructure/hclconv" )
EOF
run_case "infra-comment-in-import-block" "$copy" infrastructure-importers

# --- testsupport-semicolon-in-tagged-prod: same ; bypass, Step 8 ---------
copy=$(mkcopy)
mkdir -p "$copy/internal/infrastructure/zzprobe"
cat >"$copy/internal/infrastructure/zzprobe/zz_windows.go" <<EOF
package zzprobe

import ( _ "fmt"; _ "${MODULE}/internal/testsupport/synthrepo" )
EOF
run_case "testsupport-semicolon-in-tagged-prod" "$copy" testsupport-only-in-tests

# --- hcl-raw-string-in-func-allowed: an import in a raw string is fine ----
copy=$(mkcopy)
cat >"$copy/internal/domain/repograph/zz_probe.go" <<'EOF'
package repograph

func zzProbe() string {
	return `import _ "github.com/hashicorp/hcl/v2"`
}
EOF
run_case "hcl-raw-string-in-func-allowed" "$copy" zero

# --- hcl-in-testdata-allowed: testdata/ is ignored like the go tool does --
copy=$(mkcopy)
mkdir -p "$copy/internal/domain/repograph/testdata"
cat >"$copy/internal/domain/repograph/testdata/zz.go" <<'EOF'
package zz

import _ "github.com/hashicorp/hcl/v2"
EOF
run_case "hcl-in-testdata-allowed" "$copy" zero

echo "all architecture self-tests passed"
