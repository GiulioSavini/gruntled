// Package presenter turns domain values (a diagnostic.Set and a
// repograph.RepositoryGraph) into bytes on an io.Writer: plain text, a
// versioned JSON document, and a one-line summary.
//
// Presenters are pure adapters. They receive no filesystem path, only
// RepoPath values that are repo-relative and '/'-separated by construction,
// so an absolute path of the checkout can never leak into the output
// (DIAG-04), and the same input always yields the same bytes (CLI-03).
//
// The package may import fmt, io and encoding/json on top of the pure
// domain allowlist, and internal/domain. It must never import os,
// path/filepath, io/fs, internal/infrastructure or HCL; the
// interfaces-stdlib-allowlist and interfaces-external-deps rules in
// scripts/check-architecture.sh enforce this.
package presenter
