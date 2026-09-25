// Package diagnostic holds the Diagnostic value object, its stable Key, and
// a Set with a pure Diff. It is the domain's stable diagnostic catalogue
// (ubiquitous language), not analyzer logic: the analyzers that produce
// Diagnostic values belong to Phase 3's internal/domain/analysis.
package diagnostic

import (
	"fmt"

	"github.com/GiulioSavini/gruntled/internal/domain/repograph"
)

// Code is a diagnostic's stable identifier, always "GRT" followed by
// exactly 3 ASCII digits.
type Code string

const (
	// CodeUnknownOutput means a dependency.X.outputs.Y reference names an
	// output the target module does not declare.
	CodeUnknownOutput Code = "GRT001"
	// CodeSyntaxError means the HCL being analyzed is invalid.
	CodeSyntaxError Code = "GRT100"
)

// ParseCode validates s and returns it as a Code. s is valid iff it is
// exactly "GRT" followed by 3 ASCII digits.
func ParseCode(s string) (Code, error) {
	c := Code(s)
	if !c.Valid() {
		return "", fmt.Errorf("diagnostic: invalid code %q: must be \"GRT\" followed by exactly 3 digits", s)
	}
	return c, nil
}

// Valid reports whether c is exactly "GRT" followed by 3 ASCII digits.
func (c Code) Valid() bool {
	s := string(c)
	if len(s) != 6 {
		return false
	}
	if s[0] != 'G' || s[1] != 'R' || s[2] != 'T' {
		return false
	}
	for _, b := range []byte(s[3:]) {
		if b < '0' || b > '9' {
			return false
		}
	}
	return true
}

// Severity is how serious a Diagnostic is.
type Severity int

const (
	// SeverityError blocks a correct plan/apply.
	SeverityError Severity = iota + 1
	// SeverityWarning does not block, but should be surfaced.
	SeverityWarning
)

// String renders the severity as "error" or "warning".
func (s Severity) String() string {
	switch s {
	case SeverityError:
		return "error"
	case SeverityWarning:
		return "warning"
	default:
		return fmt.Sprintf("Severity(%d)", int(s))
	}
}

// Diagnostic is a single finding at a Position in the repository, with a
// stable Code, a Severity and a human-readable Message.
type Diagnostic struct {
	code     Code
	severity Severity
	pos      repograph.Position
	message  string
}

// New validates its arguments and returns a Diagnostic. code must be
// Valid, severity must be one of the Severity constants, pos must be
// non-zero (its File must be non-zero), and message must be non-empty.
func New(code Code, severity Severity, pos repograph.Position, message string) (Diagnostic, error) {
	if !code.Valid() {
		return Diagnostic{}, fmt.Errorf("diagnostic: invalid code %q", string(code))
	}
	if severity != SeverityError && severity != SeverityWarning {
		return Diagnostic{}, fmt.Errorf("diagnostic: invalid severity %d", int(severity))
	}
	if pos.File().IsZero() {
		return Diagnostic{}, fmt.Errorf("diagnostic: invalid position: file must not be zero")
	}
	if message == "" {
		return Diagnostic{}, fmt.Errorf("diagnostic: message must not be empty")
	}
	return Diagnostic{code: code, severity: severity, pos: pos, message: message}, nil
}

// Code returns the diagnostic's stable code.
func (d Diagnostic) Code() Code {
	return d.code
}

// Severity returns the diagnostic's severity.
func (d Diagnostic) Severity() Severity {
	return d.severity
}

// Pos returns the diagnostic's position.
func (d Diagnostic) Pos() repograph.Position {
	return d.pos
}

// Message returns the diagnostic's human-readable message.
func (d Diagnostic) Message() string {
	return d.message
}

// Key returns the diagnostic's stable identity, used for Diff and for
// at-most-once notifications. Two diagnostics that differ only in Severity
// share a Key.
func (d Diagnostic) Key() Key {
	return Key{
		Code:    d.code,
		File:    d.pos.File().String(),
		Line:    d.pos.Line(),
		Column:  d.pos.Column(),
		Message: d.message,
	}
}

// Key is a Diagnostic's comparable, stable identity.
type Key struct {
	Code    Code
	File    string
	Line    int
	Column  int
	Message string
}

// Compare orders Diagnostic values by Pos (file, then line, then column),
// then Code, then Severity, then Message.
func Compare(a, b Diagnostic) int {
	if c := a.pos.Compare(b.pos); c != 0 {
		return c
	}
	if a.code != b.code {
		if a.code < b.code {
			return -1
		}
		return 1
	}
	if a.severity != b.severity {
		if a.severity < b.severity {
			return -1
		}
		return 1
	}
	if a.message != b.message {
		if a.message < b.message {
			return -1
		}
		return 1
	}
	return 0
}
