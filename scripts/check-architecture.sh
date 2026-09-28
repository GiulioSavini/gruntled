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
if ! go vet ./internal/domain/... ./internal/application/... ./cmd/... >/tmp/check-architecture-vet.$$ 2>&1; then
  echo "=== RULE FAILED: compile-gate ===" >&2
  echo "domain, application or cmd does not compile:" >&2
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

# scan_import_lines prints "file:line" for every import spec, in any *.go
# file, whose path matches target_re right after the opening quote (" or
# `). It exists because go list only sees the files the host platform
# compiles: a _GOOS/_GOARCH suffix or a //go:build tag hides a file from it
# entirely, so a rule that must hold on every platform needs this
# source-level scan next to go list.
#
# Directories the go tool itself ignores are pruned: names starting with
# "." or "_", testdata and vendor (so .git and a .claude/worktrees copy of
# the repo are never scanned). Extra args are passed straight to find as
# file predicates, letting each caller exclude what its rule allows.
#
# Only import declarations are read, never arbitrary lines: a single-line
# `import [name|_|.] "x"`, or the specs of an `import ( ... )` block. The
# scan of a file stops at its first func/type/var/const declaration (Go
# allows imports only before those), so a string literal or a raw-string
# Go snippet in a test cannot match. Comments never match either, because
# a spec line must begin (after an optional name) with the quote.
scan_import_lines() {
  local target_re="$1"
  shift
  # shellcheck disable=SC2016 # the awk program is single-quoted on purpose
  SCAN_TARGET_RE="$target_re" find . \
    \( -type d \( -name '.?*' -o -name '_*' -o -name testdata -o -name vendor \) \) -prune -o \
    -type f -name '*.go' "$@" -exec awk '
      FNR == 1 { inblock = 0; incomment = 0; done = 0
                 spec = "^[ \t]*([A-Za-z_][A-Za-z0-9_]*[ \t]+|[_.][ \t]*)?[\"`]" ENVIRON["SCAN_TARGET_RE"] }
      done { next }
      { line = $0; sub(/\r$/, "", line) }
      incomment { if (index(line, "*/") > 0) incomment = 0; next }
      inblock {
        if (line ~ /^[ \t]*\)/) { inblock = 0; next }
        if (line ~ spec) print FILENAME ":" FNR
        next
      }
      line ~ /^[ \t]*import[ \t]*\(/ {
        rest = line; sub(/^[ \t]*import[ \t]*\(/, "", rest)
        if (rest ~ spec) print FILENAME ":" FNR
        if (index(rest, ")") == 0) inblock = 1
        next
      }
      line ~ /^[ \t]*import[ \t"`]/ {
        rest = line; sub(/^[ \t]*import/, "", rest)
        if (rest ~ spec) print FILENAME ":" FNR
        next
      }
      line ~ /^[ \t]*(func|type|var|const)([ \t(]|$)/ { done = 1; next }
      line ~ /^[ \t]*\/\*/ { if (index(substr(line, index(line, "/*") + 2), "*/") == 0) incomment = 1 }
    ' {} +
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

# --- Step 3: platform-neutral -----------------------------------------------
# Everything above only sees the files that compile for the host platform.
# A foo_windows.go or a file with a //go:build line is invisible to go list
# on linux, so it could import os unseen. domain and application must be
# platform-neutral: no build constraints of any kind, no cgo, no assembly.
goos_re=$(go tool dist list | cut -d/ -f1 | sort -u | paste -sd'|' -)
goarch_re=$(go tool dist list | cut -d/ -f2 | sort -u | paste -sd'|' -)
check_platform_neutral domain-platform-neutral internal/domain
check_platform_neutral application-platform-neutral internal/application

# --- Step 4: external-deps ---------------------------------------------------
check_external_deps domain-external-deps ./internal/domain/... \
  "^${module_re}/internal/domain/" \
  "internal/domain transitively depends on non-domain package(s). internal/domain must never reach hcl, terraform-config-inspect, go-getter, fsnotify or any other internal layer:"
check_external_deps application-external-deps ./internal/application/... \
  "^${module_re}/internal/(domain|application)/" \
  "internal/application transitively depends on package(s) outside domain/application. application must never reach internal/infrastructure or internal/testsupport directly:"

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
# scan below adds every file hidden behind a _GOOS/_GOARCH suffix or a
# //go:build tag. It reads import declarations only, so a doc comment
# that names the libraries stays allowed.
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
hcl_source=$(scan_import_lines 'github\.com/(hashicorp|zclconf)/' -not -path './internal/infrastructure/*')
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
# source scan adds files no build on this platform makes visible
# (_windows.go, //go:build).
infra_import_re="${module_re}/internal/infrastructure"
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
infra_source=$(scan_import_lines "${infra_import_re}(/|[\"\`])" -not -path './cmd/*' -not -path './internal/infrastructure/*')
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
# expected to use testsupport); the source scan over non-_test.go files
# adds files no build on this platform makes visible.
testsupport_import_re="${module_re}/internal/testsupport"
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
ts_source=$(scan_import_lines "${testsupport_import_re}(/|[\"\`])" -not -path './internal/testsupport/*' -not -name '*_test.go')
ts_violations=$(printf '%s\n%s\n' "$ts_go_list" "$ts_source" | grep -v '^$' | sort -u || true)
if [ -n "$ts_violations" ]; then
  echo "=== RULE FAILED: testsupport-only-in-tests ===" >&2
  echo "production (non-_test.go) code outside internal/testsupport imports internal/testsupport:" >&2
  printf '%s\n' "$ts_violations" >&2
  echo "internal/testsupport must be linked only by _test.go files." >&2
  fail=1
fi

if [ "$fail" -ne 0 ]; then
  exit 1
fi

echo "architecture: OK (${domain_pkg_count} domain packages, ${app_pkg_count} application packages)"
