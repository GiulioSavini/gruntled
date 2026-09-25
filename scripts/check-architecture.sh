#!/usr/bin/env bash
# check-architecture.sh enforces ARCH-01: internal/domain may import only an
# allowlist of pure standard-library packages, must be platform-neutral (no
# build constraints), and must never depend on anything outside
# internal/domain; the shipped binary must never link internal/testsupport.
#
# Exit 0 means every rule held. Exit 1 means at least one rule failed; every
# failing rule prints its own labelled block to stderr before the script
# exits, so a single run can report more than one violation.
set -euo pipefail
cd "$(dirname "$0")/.."

fail=0

# --- Step 0: compile gate -------------------------------------------------
# go list can succeed (rc=0) on code with a syntax error and simply omit it
# from .Error, so every rule below that depends on go list must be preceded
# by a real compiler pass.
if ! go vet ./internal/domain/... ./cmd/... >/tmp/check-architecture-vet.$$ 2>&1; then
  echo "=== RULE FAILED: compile-gate ===" >&2
  echo "domain or cmd does not compile:" >&2
  cat /tmp/check-architecture-vet.$$ >&2
  rm -f /tmp/check-architecture-vet.$$
  exit 1
fi
rm -f /tmp/check-architecture-vet.$$

# --- Step 1: non-vacuous guard --------------------------------------------
pkgs=$(go list ./internal/domain/...)
pkg_count=$(printf '%s\n' "$pkgs" | grep -c . || true)
if [ "$pkg_count" -lt 2 ]; then
  echo "=== RULE FAILED: non-vacuous-guard ===" >&2
  echo "found only ${pkg_count} package(s) under ./internal/domain/..., expected at least 2." >&2
  echo "This check would pass vacuously; the path is probably wrong." >&2
  fail=1
fi

module=$(go list -m)
module_re=$(printf '%s' "$module" | sed 's/[.]/\\./g')

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
domain_prod_imports=$(go list -f '{{range .Imports}}{{.}}{{"\n"}}{{end}}' ./internal/domain/... | grep -v '^$' | sort -u)
domain_test_imports=$(go list -f '{{range .TestImports}}{{.}}{{"\n"}}{{end}}{{range .XTestImports}}{{.}}{{"\n"}}{{end}}' ./internal/domain/... | grep -v '^$' | sort -u)
not_allowed=$(
  {
    printf '%s\n' "$domain_prod_imports" | grep -v -x -F "$domain_allowed" || true
    printf '%s\n' "$domain_test_imports" | grep -v -x -F "$domain_allowed
$domain_test_allowed" || true
  } | grep -v '^$' | grep -v -E "^${module_re}/internal/domain/" | sort -u || true
)
if [ -n "$not_allowed" ]; then
  echo "=== RULE FAILED: domain-stdlib-allowlist ===" >&2
  echo "internal/domain (or one of its tests) imports package(s) outside the pure allowlist:" >&2
  printf '%s\n' "$not_allowed" >&2
  echo "The domain must be pure: no I/O, no printing, no randomness, no unsafe." >&2
  echo "Move the code to internal/infrastructure, or justify extending the allowlist in review." >&2
  fail=1
fi

# --- Step 2b: domain-platform-neutral ---------------------------------------
# Everything above only sees the files that compile for the host platform.
# A foo_windows.go or a file with a //go:build line is invisible to go list
# on linux, so it could import os unseen. The domain must be
# platform-neutral: no build constraints of any kind, no cgo, no assembly.
constrained=$(go list -f '{{range .IgnoredGoFiles}}{{$.Dir}}/{{.}}{{"\n"}}{{end}}{{range .IgnoredOtherFiles}}{{$.Dir}}/{{.}}{{"\n"}}{{end}}{{range .CgoFiles}}{{$.Dir}}/{{.}}{{"\n"}}{{end}}{{range .SFiles}}{{$.Dir}}/{{.}}{{"\n"}}{{end}}{{range .CFiles}}{{$.Dir}}/{{.}}{{"\n"}}{{end}}' ./internal/domain/... | grep -v '^$' || true)
goos_re=$(go tool dist list | cut -d/ -f1 | sort -u | paste -sd'|' -)
goarch_re=$(go tool dist list | cut -d/ -f2 | sort -u | paste -sd'|' -)
suffixed=$(find internal/domain -type f -name '*.go' | grep -E "_(${goos_re}|${goarch_re})(_(${goarch_re}))?(_test)?\.go$" || true)
tagged=$(find internal/domain -type f -name '*.go' -exec grep -l -E '^[[:space:]]*//[[:space:]]*(go:build|\+build)([[:space:]]|$)' {} + || true)
platform=$(printf '%s\n%s\n%s\n' "$constrained" "$suffixed" "$tagged" | grep -v '^$' | sort -u || true)
if [ -n "$platform" ]; then
  echo "=== RULE FAILED: domain-platform-neutral ===" >&2
  echo "internal/domain has build-constrained, cgo or non-Go source file(s):" >&2
  printf '%s\n' "$platform" >&2
  echo "The domain must compile identically on every platform. Remove the constraint or move the code out of the domain." >&2
  fail=1
fi

# --- Step 3: domain-external-deps ------------------------------------------
domain_deps=$(go list -deps -test -f '{{if not .Standard}}{{.ImportPath}}{{end}}' ./internal/domain/...)
external_deps=$(printf '%s\n' "$domain_deps" | grep -v '^$' | grep -v -E "^${module_re}/internal/domain/" || true)
if [ -n "$external_deps" ]; then
  echo "=== RULE FAILED: domain-external-deps ===" >&2
  echo "internal/domain transitively depends on non-domain package(s):" >&2
  printf '%s\n' "$external_deps" >&2
  echo "internal/domain must never reach hcl, terraform-config-inspect, go-getter, fsnotify or any other internal layer." >&2
  fail=1
fi

# --- Step 4: binary-links-testsupport --------------------------------------
cmd_deps=$(go list -deps -f '{{.ImportPath}}' ./cmd/gruntled)
testsupport_link=$(printf '%s\n' "$cmd_deps" | grep -E "^${module_re}/internal/testsupport(/|$)" || true)
if [ -n "$testsupport_link" ]; then
  echo "=== RULE FAILED: binary-links-testsupport ===" >&2
  echo "cmd/gruntled links internal/testsupport, which must never ship in the binary:" >&2
  printf '%s\n' "$testsupport_link" >&2
  fail=1
fi

if [ "$fail" -ne 0 ]; then
  exit 1
fi

echo "architecture: OK (${pkg_count} domain packages)"
