package diagnostic

import "slices"

// Set is a canonically ordered, duplicate-free collection of Diagnostic
// values. Two Sets built from the same diagnostics in any order are Equal
// and have identical All() order.
type Set struct {
	items []Diagnostic
}

// NewSet builds a Set from ds: it clones, sorts with Compare, and keeps
// only the first occurrence of each Key, so every Key appears at most once
// in the result. Key includes Unit, so two diagnostics that differ only in
// Unit are distinct and both kept. Diagnostics sharing a Key can differ
// only in Severity, and Compare orders SeverityError before
// SeverityWarning at the same position, code and unit, so a collision
// keeps the error.
//
// Deduplication tracks every Key already kept rather than comparing
// neighbours: Compare orders by Severity before Message, so two diagnostics
// with the same Key are not necessarily adjacent after sorting.
func NewSet(ds ...Diagnostic) Set {
	sorted := slices.Clone(ds)
	slices.SortFunc(sorted, Compare)

	seen := make(map[Key]struct{}, len(sorted))
	items := make([]Diagnostic, 0, len(sorted))
	for _, d := range sorted {
		k := d.Key()
		if _, dup := seen[k]; dup {
			continue
		}
		seen[k] = struct{}{}
		items = append(items, d)
	}
	return Set{items: items}
}

// All returns a copy of the Set's diagnostics in canonical order.
func (s Set) All() []Diagnostic {
	return slices.Clone(s.items)
}

// Len returns the number of diagnostics in the Set.
func (s Set) Len() int {
	return len(s.items)
}

// HasErrors reports whether the Set contains at least one
// SeverityError diagnostic.
func (s Set) HasErrors() bool {
	for _, d := range s.items {
		if d.Severity() == SeverityError {
			return true
		}
	}
	return false
}

// Equal reports whether s and o contain the same diagnostics in the same
// canonical order.
func (s Set) Equal(o Set) bool {
	if len(s.items) != len(o.items) {
		return false
	}
	for i, d := range s.items {
		if d != o.items[i] {
			return false
		}
	}
	return true
}
