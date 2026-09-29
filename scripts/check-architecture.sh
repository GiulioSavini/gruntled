#!/usr/bin/env bash
# check-architecture.sh enforces ARCH-01: internal/domain may import only an
# allowlist of pure standard-library packages, must be platform-neutral (no
# build constraints), and must never depend on anything outside
# internal/domain; the shipped binary must never link internal/testsupport.
#
# It also enforces the Phase 2 layering rules: internal/application may
# import only the domain allowlist plus context, must be platform-neutral,
# and may depend only on internal/domain and internal/application; HCL
# (hashicorp/hcl, zclconf/go-cty) may be imported only inside
# internal/infrastructure; every top-level internal/<x> directory must be
# one of the known layers (internal-layout); only cmd/... and
# internal/infrastructure/... may import internal/infrastructure/...
# (infrastructure-importers); and only _test.go files may import
# internal/testsupport (testsupport-only-in-tests).
#
# Phase 3 adds the presenter and binary rules: internal/interfaces must be
# non-empty (interfaces-non-vacuous-guard), may import only a pure stdlib
# allowlist (fmt, io and encoding/json on top of the domain allowlist:
# interfaces-stdlib-allowlist), must be platform-neutral
# (interfaces-platform-neutral), and may depend only on internal/domain,
# internal/application and internal/interfaces (interfaces-external-deps);
# and the shipped binary, for every release target, links no net, os/exec,
# plugin or crypto/tls package and no linked non-std file calls
# os.StartProcess or syscall.ForkExec/Exec (binary-no-net-no-exec).
#
# Exit 0 means every rule held. Exit 1 means at least one rule failed; every
# failing rule prints its own labelled block to stderr before the script
# exits, so a single run can report more than one violation.
set -euo pipefail
cd "$(dirname "$0")/.."

fail=0

# --- Step 0: non-vacuous guards --------------------------------------------
# These run before the compile gate on purpose: go list ./x/... on an
# EXISTING EMPTY directory prints "matched no packages" and exits 0 (it does
# not need x to compile), but internal/application imports internal/domain,
# so emptying internal/domain entirely would otherwise make
# internal/application fail to compile and trip compile-gate first, hiding
# the vacuous-guard violation these cases exist to catch. Both guards use
# fail=1 (not exit) so a genuinely broken tree still reaches compile-gate
# below and reports that failure too.
domain_pkgs=$(go list ./internal/domain/...)
domain_pkg_count=$(printf '%s\n' "$domain_pkgs" | grep -c . || true)
if [ "$domain_pkg_count" -lt 2 ]; then
  echo "=== RULE FAILED: non-vacuous-guard ===" >&2
  echo "found only ${domain_pkg_count} package(s) under ./internal/domain/..., expected at least 2." >&2
  echo "This check would pass vacuously; the path is probably wrong." >&2
  fail=1
fi

# application-non-vacuous-guard: at least one application package must
# exist, or every rule below it would pass vacuously.
app_pkgs=$(go list ./internal/application/... 2>/dev/null || true)
app_pkg_count=$(printf '%s\n' "$app_pkgs" | grep -c . || true)
if [ "$app_pkg_count" -lt 1 ]; then
  echo "=== RULE FAILED: application-non-vacuous-guard ===" >&2
  echo "found only ${app_pkg_count} package(s) under ./internal/application/..., expected at least 1." >&2
  echo "This check would pass vacuously; the path is probably wrong." >&2
  fail=1
fi

# interfaces-non-vacuous-guard: at least one interfaces (presenter)
# package must exist, or every interfaces rule below would pass vacuously.
interfaces_pkgs=$(go list ./internal/interfaces/... 2>/dev/null || true)
interfaces_pkg_count=$(printf '%s\n' "$interfaces_pkgs" | grep -c . || true)
if [ "$interfaces_pkg_count" -lt 1 ]; then
  echo "=== RULE FAILED: interfaces-non-vacuous-guard ===" >&2
  echo "found only ${interfaces_pkg_count} package(s) under ./internal/interfaces/..., expected at least 1." >&2
  echo "This check would pass vacuously; the path is probably wrong." >&2
  fail=1
fi

# --- internal-layout ---------------------------------------------------
# Every top-level entry of internal/ must be one of the known layers:
# domain, application, infrastructure, interfaces (reserved for Phase 3
# presenters), testsupport. A directory outside that set is a violation
# only if it holds a *.go file somewhere below it (a non-Go directory such
# as internal/notes/README.md is not a layering hole). A *.go file placed
# directly in internal/ is a violation too. This needs no compiler, so it
# runs here next to the other non-vacuous guards.
internal_layout_allowed="domain application infrastructure interfaces testsupport"
internal_layout_violations=""
while IFS= read -r entry; do
  [ -n "$entry" ] || continue
  name=$(basename "$entry")
  if [ -f "$entry" ]; then
    case "$name" in
    *.go)
      internal_layout_violations="${internal_layout_violations}${entry} (a .go file directly in internal/)
"
      ;;
    esac
    continue
  fi
  if [ -d "$entry" ]; then
    case " ${internal_layout_allowed} " in
    *" ${name} "*) continue ;;
    esac
    if [ -n "$(find "$entry" -type f -name '*.go' 2>/dev/null)" ]; then
      internal_layout_violations="${internal_layout_violations}${entry} (unknown top-level internal/ directory holding Go code)
"
    fi
  fi
done < <(find internal -mindepth 1 -maxdepth 1)
internal_layout_violations=$(printf '%s\n' "$internal_layout_violations" | grep -v '^$' || true)
if [ -n "$internal_layout_violations" ]; then
  echo "=== RULE FAILED: internal-layout ===" >&2
  echo "internal/ must hold only: domain, application, infrastructure, interfaces, testsupport (interfaces is reserved for Phase 3 presenters). Move the code under one of those, or make the directory non-Go content:" >&2
  printf '%s\n' "$internal_layout_violations" >&2
  fail=1
fi

# --- Step 1: compile gate ---------------------------------------------------
# go list can succeed (rc=0) on code with a syntax error and simply omit it
# from .Error, so every rule below that depends on go list must be preceded
# by a real compiler pass.
if ! go vet ./internal/domain/... ./internal/application/... ./internal/interfaces/... ./cmd/... >/tmp/check-architecture-vet.$$ 2>&1; then
  echo "=== RULE FAILED: compile-gate ===" >&2
  echo "domain, application, interfaces or cmd does not compile:" >&2
  cat /tmp/check-architecture-vet.$$ >&2
  rm -f /tmp/check-architecture-vet.$$
  exit 1
fi
rm -f /tmp/check-architecture-vet.$$

module=$(go list -m)
module_re=$(printf '%s' "$module" | sed 's/[.]/\\./g')

# --- shared helpers ----------------------------------------------------
# check_stdlib_allowlist enforces that every package under pkgs_pattern
# (prod code and tests) imports only the packages in allowed (plus
# test_extra in test files), except for imports matching exempt_re (imports
# of sibling layers this rule does not police, e.g. internal/domain from
# internal/application, which application-external-deps polices instead).
check_stdlib_allowlist() {
  local rule="$1" pkgs_pattern="$2" allowed="$3" test_extra="$4" exempt_re="$5" desc="$6"
  local prod_imports test_imports not_allowed
  prod_imports=$(go list -f '{{range .Imports}}{{.}}{{"\n"}}{{end}}' "$pkgs_pattern" | grep -v '^$' | sort -u || true)
  test_imports=$(go list -f '{{range .TestImports}}{{.}}{{"\n"}}{{end}}{{range .XTestImports}}{{.}}{{"\n"}}{{end}}' "$pkgs_pattern" | grep -v '^$' | sort -u || true)
  not_allowed=$(
    {
      printf '%s\n' "$prod_imports" | grep -v -x -F "$allowed" || true
      printf '%s\n' "$test_imports" | grep -v -x -F "$allowed
$test_extra" || true
    } | grep -v '^$' | grep -v -E "$exempt_re" | sort -u || true
  )
  if [ -n "$not_allowed" ]; then
    echo "=== RULE FAILED: ${rule} ===" >&2
    echo "$desc" >&2
    printf '%s\n' "$not_allowed" >&2
    fail=1
  fi
}

# check_platform_neutral enforces that no file under dir is build-
# constrained, cgo, or non-Go source.
check_platform_neutral() {
  local rule="$1" dir="$2"
  local constrained suffixed tagged platform
  constrained=$(go list -f '{{range .IgnoredGoFiles}}{{$.Dir}}/{{.}}{{"\n"}}{{end}}{{range .IgnoredOtherFiles}}{{$.Dir}}/{{.}}{{"\n"}}{{end}}{{range .CgoFiles}}{{$.Dir}}/{{.}}{{"\n"}}{{end}}{{range .SFiles}}{{$.Dir}}/{{.}}{{"\n"}}{{end}}{{range .CFiles}}{{$.Dir}}/{{.}}{{"\n"}}{{end}}' "./${dir}/..." | grep -v '^$' || true)
  suffixed=$(find "$dir" -type f -name '*.go' | grep -E "_(${goos_re}|${goarch_re})(_(${goarch_re}))?(_test)?\.go$" || true)
  tagged=$(find "$dir" -type f -name '*.go' -exec grep -l -E '^[[:space:]]*//[[:space:]]*(go:build|\+build)([[:space:]]|$)' {} + || true)
  platform=$(printf '%s\n%s\n%s\n' "$constrained" "$suffixed" "$tagged" | grep -v '^$' | sort -u || true)
  if [ -n "$platform" ]; then
    echo "=== RULE FAILED: ${rule} ===" >&2
    echo "${dir} has build-constrained, cgo or non-Go source file(s):" >&2
    printf '%s\n' "$platform" >&2
    echo "${dir} must compile identically on every platform. Remove the constraint or move the code out." >&2
    fail=1
  fi
}

# check_external_deps enforces that every non-stdlib transitive dependency
# of pkgs_pattern (prod + test) matches allow_re.
check_external_deps() {
  local rule="$1" pkgs_pattern="$2" allow_re="$3" desc="$4"
  local deps external
  deps=$(go list -deps -test -f '{{if not .Standard}}{{.ImportPath}}{{end}}' "$pkgs_pattern")
  external=$(printf '%s\n' "$deps" | grep -v '^$' | grep -v -E "$allow_re" || true)
  if [ -n "$external" ]; then
    echo "=== RULE FAILED: ${rule} ===" >&2
    echo "$desc" >&2
    printf '%s\n' "$external" >&2
    fail=1
  fi
}

# scan_imports runs the source-level import scanner for one rule and
# leaves its matches, one "./file.go:LINE: import "x"" per line, in
# scan_out. It exists because go list only sees the files the host
# platform compiles: a _GOOS/_GOARCH suffix or a //go:build tag hides a
# file from it entirely, so a rule that must hold on every platform needs
# this scan next to go list.
#
# The scanner is a Go program (scripts/archscan, go/parser with
# ImportsOnly), not a line-based scan: a /* */ comment or a ";" inside an
# import block defeated the old awk version (02-REVIEW G21). It reads every
# *.go file whatever its build constraints, prunes the directories the go
# tool ignores (names starting with "." or "_", testdata, vendor), and
# matches re against the unquoted import path. It lives under scripts/,
# outside internal/, and is never linked into cmd/gruntled.
#
# A scanner failure (it did not build, a file's imports did not parse, an
# I/O error) fails the rule that asked for it; it never passes silently.
# The result comes back in scan_out, not on stdout, because fail=1 set
# inside a $(...) subshell would be lost.
scan_out=""
scan_imports() {
  local rule="$1" re="$2"
  shift 2
  local errfile
  errfile=$(mktemp)
  scan_out=""
  if ! scan_out=$(go run ./scripts/archscan -match "$re" "$@" 2>"$errfile"); then
    echo "=== RULE FAILED: ${rule} ===" >&2
    echo "import scanner failed:" >&2
    cat "$errfile" >&2
    fail=1
  fi
  rm -f "$errfile"
}

# --- Step 2: domain-stdlib-allowlist ---------------------------------------
# The domain may import only the pure standard-library packages listed
# here (plus other internal/domain packages, which Step 3 polices). This is
# an allowlist on purpose: a blocklist silently lets through fmt/log
# (printing to stdout/stderr), crypto/rand, embed, unsafe, runtime and
# anything added to the standard library later. fmt is deliberately absent:
# the domain builds its messages with errors and strconv, so nothing in it
# can ever reach os.Stdout. testing and reflect are allowed in test files only.
domain_allowed='errors
strings
strconv
sort
slices
maps
path
cmp
iter
bytes
math
math/bits
unicode
unicode/utf8
unicode/utf16'
domain_test_allowed='testing
reflect'
check_stdlib_allowlist domain-stdlib-allowlist ./internal/domain/... \
  "$domain_allowed" "$domain_test_allowed" "^${module_re}/internal/domain/" \
  "internal/domain (or one of its tests) imports package(s) outside the pure allowlist. The domain must be pure: no I/O, no printing, no randomness, no unsafe. Move the code to internal/infrastructure, or justify extending the allowlist in review:"

# --- Step 2b: application-stdlib-allowlist ---------------------------------
# application may import the same pure allowlist as the domain, plus
# context (for cancellation, the one concession an orchestrating use case
# needs). No fmt: error wrapping uses errors and custom error types.
# Imports of internal/domain and internal/application are exempt here;
# application-external-deps polices those.
app_allowed="${domain_allowed}
context"
check_stdlib_allowlist application-stdlib-allowlist ./internal/application/... \
  "$app_allowed" "$domain_test_allowed" "^${module_re}/internal/(domain|application)/" \
  "internal/application (or one of its tests) imports package(s) outside the allowlist. application must not do I/O or printing; error wrapping uses errors and custom error types, not fmt.Errorf. Move the code to internal/infrastructure, or justify extending the allowlist in review:"

# --- Step 2c: interfaces-stdlib-allowlist ----------------------------------
# Presenters turn domain values into bytes on an io.Writer: the domain
# allowlist plus fmt, io and encoding/json. They must not touch the
# filesystem, the network, processes or the environment, so os,
# path/filepath, io/fs, net, os/exec and syscall stay out. Imports of
# internal/domain, internal/application and internal/interfaces are exempt
# here; interfaces-external-deps polices those.
interfaces_allowed="${domain_allowed}
fmt
io
encoding/json"
check_stdlib_allowlist interfaces-stdlib-allowlist ./internal/interfaces/... \
  "$interfaces_allowed" "$domain_test_allowed" "^${module_re}/internal/(domain|application|interfaces)/" \
  "internal/interfaces (or one of its tests) imports package(s) outside the allowlist. Presenters turn domain values into bytes on an io.Writer; they must not touch the filesystem, the network, processes or the environment (no os, path/filepath, io/fs, net, os/exec, syscall):"

# --- Step 3: platform-neutral -----------------------------------------------
# Everything above only sees the files that compile for the host platform.
# A foo_windows.go or a file with a //go:build line is invisible to go list
# on linux, so it could import os unseen. domain and application must be
# platform-neutral: no build constraints of any kind, no cgo, no assembly.
goos_re=$(go tool dist list | cut -d/ -f1 | sort -u | paste -sd'|' -)
goarch_re=$(go tool dist list | cut -d/ -f2 | sort -u | paste -sd'|' -)
check_platform_neutral domain-platform-neutral internal/domain
check_platform_neutral application-platform-neutral internal/application
check_platform_neutral interfaces-platform-neutral internal/interfaces

# --- Step 4: external-deps ---------------------------------------------------
check_external_deps domain-external-deps ./internal/domain/... \
  "^${module_re}/internal/domain/" \
  "internal/domain transitively depends on non-domain package(s). internal/domain must never reach hcl, terraform-config-inspect, go-getter, fsnotify or any other internal layer:"
check_external_deps application-external-deps ./internal/application/... \
  "^${module_re}/internal/(domain|application)/" \
  "internal/application transitively depends on package(s) outside domain/application. application must never reach internal/infrastructure or internal/testsupport directly:"
check_external_deps interfaces-external-deps ./internal/interfaces/... \
  "^${module_re}/internal/(domain|application|interfaces)/" \
  "internal/interfaces transitively depends on package(s) outside domain/application/interfaces. Presenters must never reach internal/infrastructure, internal/testsupport, HCL or any third-party library:"

# --- Step 5: binary-links-testsupport --------------------------------------
cmd_deps=$(go list -deps -f '{{.ImportPath}}' ./cmd/gruntled)
testsupport_link=$(printf '%s\n' "$cmd_deps" | grep -E "^${module_re}/internal/testsupport(/|$)" || true)
if [ -n "$testsupport_link" ]; then
  echo "=== RULE FAILED: binary-links-testsupport ===" >&2
  echo "cmd/gruntled links internal/testsupport, which must never ship in the binary:" >&2
  printf '%s\n' "$testsupport_link" >&2
  fail=1
fi

# --- Step 6: hcl-only-in-infrastructure -------------------------------------
# HCL (hashicorp/hcl/v2, zclconf/go-cty) may be imported only inside
# internal/infrastructure. Direct imports only: cmd/gruntled will
# legitimately link infrastructure transitively once Phase 3 wires the CLI.
# go list -e (not plain go list) so a probe that breaks some unrelated
# package under set -e does not abort the script before this rule runs.
# go list only sees the files the host platform compiles, so the source
# scan below (scan_imports, a go/parser-based scanner) adds every file
# hidden behind a _GOOS/_GOARCH suffix or a //go:build tag. It reads
# import declarations only, so a doc comment or a string that names the
# libraries stays allowed.
hcl_go_list=$(
  go list -e -f '{{.ImportPath}}{{"\t"}}{{join .Imports " "}} {{join .TestImports " "}} {{join .XTestImports " "}}' ./... |
    while IFS=$'\t' read -r pkg rest; do
      case "$pkg" in
      "${module}/internal/infrastructure" | "${module}/internal/infrastructure"/*) continue ;;
      esac
      for imp in $rest; do
        case "$imp" in
        github.com/hashicorp/* | github.com/zclconf/*)
          echo "${pkg} -> ${imp}"
          ;;
        esac
      done
    done
)
scan_imports hcl-only-in-infrastructure '^github\.com/(hashicorp|zclconf)/' -exclude internal/infrastructure/
hcl_source=$scan_out
hcl_violations=$(printf '%s\n%s\n' "$hcl_go_list" "$hcl_source" | grep -v '^$' | sort -u || true)
if [ -n "$hcl_violations" ]; then
  echo "=== RULE FAILED: hcl-only-in-infrastructure ===" >&2
  echo "package(s) outside internal/infrastructure import HCL directly:" >&2
  printf '%s\n' "$hcl_violations" >&2
  echo "HCL parsing (hashicorp/hcl, zclconf/go-cty) must stay inside internal/infrastructure." >&2
  fail=1
fi

# --- Step 7: infrastructure-importers ---------------------------------------
# Only cmd/... and internal/infrastructure/... may import
# internal/infrastructure/...; every other layer must go through the
# application ports, not infrastructure directly. Two engines: go list
# sees every import the host platform compiles (prod and test); the
# source scan (scan_imports) adds files no build on this platform makes
# visible (_windows.go, //go:build).
infra_go_list=$(
  go list -e -f '{{.ImportPath}}{{"\t"}}{{join .Imports " "}} {{join .TestImports " "}} {{join .XTestImports " "}}' ./... |
    while IFS=$'\t' read -r pkg rest; do
      case "$pkg" in
      "${module}/cmd" | "${module}/cmd"/* | "${module}/internal/infrastructure" | "${module}/internal/infrastructure"/*) continue ;;
      esac
      for imp in $rest; do
        case "$imp" in
        "${module}/internal/infrastructure" | "${module}/internal/infrastructure"/*)
          echo "${pkg} -> ${imp}"
          ;;
        esac
      done
    done
)
scan_imports infrastructure-importers "^${module_re}/internal/infrastructure(/|\$)" -exclude cmd/ -exclude internal/infrastructure/
infra_source=$scan_out
infra_violations=$(printf '%s\n%s\n' "$infra_go_list" "$infra_source" | grep -v '^$' | sort -u || true)
if [ -n "$infra_violations" ]; then
  echo "=== RULE FAILED: infrastructure-importers ===" >&2
  echo "package(s) outside cmd/... and internal/infrastructure/... import internal/infrastructure/... directly:" >&2
  printf '%s\n' "$infra_violations" >&2
  echo "Go through internal/application/ports instead." >&2
  fail=1
fi

# --- Step 8: testsupport-only-in-tests ---------------------------------------
# Only _test.go files may import internal/testsupport; no production
# package may link it, whether or not cmd reaches it (binary-links-
# testsupport above only catches the case where it does). Two engines: go
# list's .Imports only (never TestImports/XTestImports, which are
# expected to use testsupport); the source scan (scan_imports) over
# non-_test.go files adds files no build on this platform makes visible.
ts_go_list=$(
  go list -e -f '{{.ImportPath}}{{"\t"}}{{join .Imports " "}}' ./... |
    while IFS=$'\t' read -r pkg rest; do
      case "$pkg" in
      "${module}/internal/testsupport" | "${module}/internal/testsupport"/*) continue ;;
      esac
      for imp in $rest; do
        case "$imp" in
        "${module}/internal/testsupport" | "${module}/internal/testsupport"/*)
          echo "${pkg} -> ${imp}"
          ;;
        esac
      done
    done
)
scan_imports testsupport-only-in-tests "^${module_re}/internal/testsupport(/|\$)" -exclude internal/testsupport/ -skip-tests
ts_source=$scan_out
ts_violations=$(printf '%s\n%s\n' "$ts_go_list" "$ts_source" | grep -v '^$' | sort -u || true)
if [ -n "$ts_violations" ]; then
  echo "=== RULE FAILED: testsupport-only-in-tests ===" >&2
  echo "production (non-_test.go) code outside internal/testsupport imports internal/testsupport:" >&2
  printf '%s\n' "$ts_violations" >&2
  echo "internal/testsupport must be linked only by _test.go files." >&2
  fail=1
fi

# --- Step 9: binary-no-net-no-exec (CLI-04) -----------------------------------
# The shipped binary must not be able to do network or process I/O. Two
# halves, both evaluated for EVERY release target, not only the build
# host: go list only sees the files the current GOOS/GOARCH compiles, so a
# zz_windows.go importing os/exec is invisible to a linux go list and
# visible only to the windows/amd64 iteration below. Keep release_targets
# identical to the release cross-build target list (03-03/03-05).
#
# 1. Import deny-list over go list -deps: no net, net/*, os/exec, plugin or
#    crypto/tls. No -test: test-only deps (testscript) legitimately use
#    os/exec. -e as in Step 6, so a probe that breaks an unrelated package
#    does not abort the script under set -e.
# 2. Source scan for spawners. os.StartProcess lives in os itself, which an
#    import deny-list cannot forbid (every binary links os), so the non-std
#    GoFiles of each target are grepped for the qualified identifiers
#    os.StartProcess and syscall.ForkExec|Exec|StartProcess. Its limits,
#    stated plainly: it is a textual grep, so it does NOT catch an aliased
#    import (import o "os"; o.StartProcess), a dot import, golang.org/x/sys
#    spawners (unix.Exec, windows.CreateProcess), cgo, assembly or
#    reflection; and it can over-match a comment that names those
#    identifiers (a conservative false positive, fixed by rewording the
#    comment). The import deny-list half has no such gap. The self-test
#    cases binary-os-exec, binary-net, binary-start-process,
#    binary-exec-windows-file and binary-exec-in-test-allowed pin exactly
#    what is covered.
release_targets="linux/amd64 linux/arm64 darwin/amd64 darwin/arm64 windows/amd64"
bin_violations=""
for target in $release_targets; do
  t_goos=${target%/*}
  t_goarch=${target#*/}
  t_deps=$(GOOS="$t_goos" GOARCH="$t_goarch" go list -e -deps ./cmd/gruntled)
  t_forbidden=$(printf '%s\n' "$t_deps" | grep -E '^(net|net/.+|os/exec|plugin|crypto/tls)$' || true)
  t_spawners=$(
    GOOS="$t_goos" GOARCH="$t_goarch" go list -e -deps \
      -f '{{if not .Standard}}{{$d := .Dir}}{{range .GoFiles}}{{$d}}/{{.}}{{"\n"}}{{end}}{{end}}' ./cmd/gruntled |
      grep -v '^$' |
      xargs -r grep -l -E '\bos\.StartProcess\b|\bsyscall\.(ForkExec|Exec|StartProcess)\b' || true
  )
  if [ -n "$t_forbidden" ]; then
    bin_violations="${bin_violations}$(printf '%s\n' "$t_forbidden" | sed "s|^|${target}: package |")
"
  fi
  if [ -n "$t_spawners" ]; then
    bin_violations="${bin_violations}$(printf '%s\n' "$t_spawners" | sed "s|^|${target}: file |")
"
  fi
done
bin_violations=$(printf '%s\n' "$bin_violations" | grep -v '^$' | sort -u || true)
if [ -n "$bin_violations" ]; then
  echo "=== RULE FAILED: binary-no-net-no-exec ===" >&2
  echo "cmd/gruntled links a network or process-spawning package, or linked non-std code calls os.StartProcess/syscall.ForkExec/Exec, on at least one release target:" >&2
  printf '%s\n' "$bin_violations" >&2
  echo "gruntled must make no network calls and spawn no external processes (CLI-04); keep that statically provable." >&2
  fail=1
fi

if [ "$fail" -ne 0 ]; then
  exit 1
fi

echo "architecture: OK (${domain_pkg_count} domain packages, ${app_pkg_count} application packages, ${interfaces_pkg_count} interfaces packages)"
