package repograph

import "fmt"

// Position is a 1-based line and column (column counted in bytes) inside a
// repo-relative file. It is the single location type used across the whole
// domain, including Dependency, Reference and diagnostic.Diagnostic.
type Position struct {
	file   RepoPath
	line   int
	column int
}

// NewPosition validates its arguments and returns a Position. file must be
// non-zero, and line and column must each be at least 1.
func NewPosition(file RepoPath, line, column int) (Position, error) {
	if file.IsZero() {
		return Position{}, fmt.Errorf("repograph: invalid position: file must not be zero")
	}
	if line < 1 {
		return Position{}, fmt.Errorf("repograph: invalid position: line must be >= 1, got %d", line)
	}
	if column < 1 {
		return Position{}, fmt.Errorf("repograph: invalid position: column must be >= 1, got %d", column)
	}
	return Position{file: file, line: line, column: column}, nil
}

// File returns the repo-relative file the position is inside.
func (p Position) File() RepoPath {
	return p.file
}

// Line returns the 1-based line number.
func (p Position) Line() int {
	return p.line
}

// Column returns the 1-based, byte-counted column.
func (p Position) Column() int {
	return p.column
}

// Compare orders Position values by file, then line, then column.
func (p Position) Compare(q Position) int {
	if c := p.file.Compare(q.file); c != 0 {
		return c
	}
	if p.line != q.line {
		if p.line < q.line {
			return -1
		}
		return 1
	}
	if p.column != q.column {
		if p.column < q.column {
			return -1
		}
		return 1
	}
	return 0
}

// String renders the position as "file:line:column".
func (p Position) String() string {
	return fmt.Sprintf("%s:%d:%d", p.file.String(), p.line, p.column)
}
