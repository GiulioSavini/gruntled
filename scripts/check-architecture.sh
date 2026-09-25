#!/usr/bin/env bash
# check-architecture.sh enforces ARCH-01: internal/domain must never touch
# the filesystem/network directly and must never depend on anything outside
# internal/domain, and the shipped binary must never link internal/testsupport.
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

# --- Step 2: domain-direct-io ----------------------------------------------
domain_imports=$(go list -f '{{range .Imports}}{{.}}{{"\n"}}{{end}}{{range .TestImports}}{{.}}{{"\n"}}{{end}}{{range .XTestImports}}{{.}}{{"\n"}}{{end}}' ./internal/domain/...)
direct_io=$(printf '%s\n' "$domain_imports" | sort -u | grep -E '^(os|os/.+|io/fs|io/ioutil|path/filepath|net|net/.+|syscall)$' || true)
if [ -n "$direct_io" ]; then
  echo "=== RULE FAILED: domain-direct-io ===" >&2
  echo "internal/domain (or one of its tests) imports a filesystem/network/syscall package directly:" >&2
  printf '%s\n' "$direct_io" >&2
  echo "The domain must be pure. Move filesystem/network access to internal/infrastructure." >&2
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
