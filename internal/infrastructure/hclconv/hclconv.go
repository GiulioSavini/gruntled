// Package hclconv is the single place where hcl positions and diagnostics
// become domain values. Columns are counted in bytes because the domain
// Position and the synthrepo manifest both count bytes, while hcl.Pos.Column
// counts grapheme clusters: recomputing from the byte offset is therefore
// mandatory, never hcl.Pos.Column itself.
//
// hclconv is also where the input limits guarding hcl's recursive parsers
// live (limits.go): hclsyntax.ParseConfig and hcl/json.Parse both recurse
// over their input, and Go cannot recover from the fatal stack overflow a
// deeply nested or oversize file can cause (02-REVIEW G7). Both HCL
// adapters in this module apply the same MaxFileBytes and MaxNestingDepth
// limits, so they live here rather than in either adapter.
package hclconv

import (
	"bytes"

	"github.com/hashicorp/hcl/v2"

	"github.com/GiulioSavini/gruntled/internal/domain/diagnostic"
	"github.com/GiulioSavini/gruntled/internal/domain/repograph"
)

// Position converts an hcl.Pos into a domain repograph.Position, whose
// column is a 1-based byte column recomputed from p.Byte: line = 1 + the
// count of '\n' bytes in src[:off], and column = off - (the offset just
// after the last '\n' before off) + 1, where off is p.Byte clamped to
// [0, len(src)]. hcl.Pos.Column (grapheme clusters) is never used.
func Position(file repograph.RepoPath, src []byte, p hcl.Pos) (repograph.Position, error) {
	off := p.Byte
	if off < 0 {
		off = 0
	}
	if off > len(src) {
		off = len(src)
	}
	line := 1 + bytes.Count(src[:off], []byte{'\n'})
	lastNL := bytes.LastIndexByte(src[:off], '\n')
	column := off - (lastNL + 1) + 1
	return repograph.NewPosition(file, line, column)
}

// FirstSyntaxError converts the error-severity diagnostic in diags whose
// Subject starts at the smallest byte offset into a single file-level
// (zero Unit) GRT100 diagnostic. Ties are broken by Summary, then Detail. A
// nil Subject sorts as if it started at offset 0, and its position becomes
// 1:1. ok is false when diags has no error-severity entry, in which case the
// returned Diagnostic is the zero value. The message is d.Summary, plus
// ": "+d.Detail when d.Detail is non-empty.
func FirstSyntaxError(file repograph.RepoPath, src []byte, diags hcl.Diagnostics) (diagnostic.Diagnostic, bool, error) {
	var first *hcl.Diagnostic
	firstOff := 0
	for _, d := range diags {
		if d.Severity != hcl.DiagError {
			continue
		}
		off := 0
		if d.Subject != nil {
			off = d.Subject.Start.Byte
		}
		if first == nil || off < firstOff || (off == firstOff && lessDiag(d, first)) {
			first = d
			firstOff = off
		}
	}
	if first == nil {
		return diagnostic.Diagnostic{}, false, nil
	}
	pos, err := repograph.NewPosition(file, 1, 1)
	if first.Subject != nil {
		pos, err = Position(file, src, first.Subject.Start)
	}
	if err != nil {
		return diagnostic.Diagnostic{}, false, err
	}
	msg := first.Summary
	if first.Detail != "" {
		msg += ": " + first.Detail
	}
	d, err := diagnostic.New(diagnostic.CodeSyntaxError, diagnostic.SeverityError, pos, msg)
	if err != nil {
		return diagnostic.Diagnostic{}, false, err
	}
	return d, true, nil
}

// lessDiag breaks a tie between two diagnostics whose Subject starts at the
// same byte offset: by Summary, then by Detail.
func lessDiag(a, b *hcl.Diagnostic) bool {
	if a.Summary != b.Summary {
		return a.Summary < b.Summary
	}
	return a.Detail < b.Detail
}
