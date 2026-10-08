// Package impact computes the blast radius of a change between a base tree
// and a current tree: which units (or files) a change broke, and which
// units consume a module whose surface changed. It is pure: callers feed it
// two already-analysed snapshots (graph + diagnostics), so the same logic
// serves a git-ref baseline today and an in-memory baseline later.
package impact

import (
	"slices"

	"github.com/GiulioSavini/gruntled/internal/domain/diagnostic"
	"github.com/GiulioSavini/gruntled/internal/domain/repograph"
)

// FindingKey is a finding's position-free identity. It is diagnostic.Key
// minus Line and Column: inserting lines above an existing finding moves it
// without making it new, so blast radius must not call it Broken.
//
// Message is kept verbatim for every code, GRT100 included: the hclconv
// stability test (TestGRT100MessageStability) pins that hcl syntax-error
// messages embed no line, column or file name.
type FindingKey struct {
	Code    diagnostic.Code
	Unit    repograph.RepoPath
	File    string
	Message string
}

// KeyOf returns d's position-free identity.
func KeyOf(d diagnostic.Diagnostic) FindingKey {
	k := d.Key()
	return FindingKey{Code: k.Code, Unit: k.Unit, File: k.File, Message: k.Message}
}

// NewFindings returns the diagnostics of cur whose FindingKey occurs more
// often in cur than in base, in cur's canonical order: base is a multiset, so
// a second identical finding added elsewhere is new. A finding that only
// moved, or only changed severity, is not new. Never nil.
func NewFindings(base, cur diagnostic.Set) []diagnostic.Diagnostic {
	seen := make(map[FindingKey]int, base.Len())
	for _, d := range base.All() {
		seen[KeyOf(d)]++
	}
	out := []diagnostic.Diagnostic{}
	for _, d := range cur.All() {
		k := KeyOf(d)
		if seen[k] > 0 {
			seen[k]--
			continue
		}
		out = append(out, d)
	}
	return out
}

// BrokenUnit groups the new findings of one subject: the unit a finding was
// raised for, or its file when the finding is file-level (GRT100). Findings
// are in diagnostic.Compare order.
type BrokenUnit struct {
	Subject  repograph.RepoPath
	Findings []diagnostic.Diagnostic
}

// ImpactedUnit is a direct consumer of a module whose surface changed.
type ImpactedUnit struct {
	Unit   repograph.RepoPath
	Change SurfaceChange
}

// Result is a blast radius. Broken and Impacted are sorted by path, unique,
// disjoint (a Broken subject never appears in Impacted) and never nil.
// Baseline is false when no base tree was available, in which case Broken
// holds every current finding and Impacted is empty.
type Result struct {
	Baseline bool
	Broken   []BrokenUnit
	Impacted []ImpactedUnit
}

// HasErrors reports whether any Broken finding has SeverityError.
func (r Result) HasErrors() bool {
	for _, b := range r.Broken {
		for _, d := range b.Findings {
			if d.Severity() == diagnostic.SeverityError {
				return true
			}
		}
	}
	return false
}

// Compute returns the blast radius from (baseG, baseD) to (curG, curD).
// Broken holds the findings new in cur, grouped by subject. Impacted holds,
// for every module whose surface names changed, the units of the CURRENT
// graph that use that module: one hop only, dependency edges are never
// followed. A nil graph is treated as empty.
func Compute(baseG *repograph.RepositoryGraph, baseD diagnostic.Set, curG *repograph.RepositoryGraph, curD diagnostic.Set) Result {
	broken := groupBroken(NewFindings(baseD, curD))
	isBroken := make(map[repograph.RepoPath]struct{}, len(broken))
	for _, b := range broken {
		isBroken[b.Subject] = struct{}{}
	}

	impacted := []ImpactedUnit{}
	changes := SurfaceDiff(baseG, curG)
	if len(changes) > 0 {
		byModule := make(map[repograph.RepoPath]SurfaceChange, len(changes))
		for _, c := range changes {
			byModule[c.Module] = c
		}
		seen := map[repograph.RepoPath]struct{}{}
		for _, u := range curG.Units() {
			mod, ok := u.Module()
			if !ok {
				continue
			}
			c, ok := byModule[mod]
			if !ok {
				continue
			}
			if _, b := isBroken[u.Path()]; b {
				continue
			}
			if _, dup := seen[u.Path()]; dup {
				continue
			}
			seen[u.Path()] = struct{}{}
			impacted = append(impacted, ImpactedUnit{Unit: u.Path(), Change: c})
		}
		slices.SortFunc(impacted, func(a, b ImpactedUnit) int { return a.Unit.Compare(b.Unit) })
	}

	return Result{Baseline: true, Broken: broken, Impacted: impacted}
}

// NoBaseline returns the degraded result used when no base tree exists:
// every current finding is Broken, nothing is Impacted, Baseline is false.
func NoBaseline(curD diagnostic.Set) Result {
	return Result{Baseline: false, Broken: groupBroken(curD.All()), Impacted: []ImpactedUnit{}}
}

// subjectOf is the unit a finding was raised for, else its file.
func subjectOf(d diagnostic.Diagnostic) repograph.RepoPath {
	if u, ok := d.Unit(); ok {
		return u
	}
	return d.Pos().File()
}

// groupBroken groups ds by subject, subjects sorted by path, findings in
// diagnostic.Compare order. Never nil.
func groupBroken(ds []diagnostic.Diagnostic) []BrokenUnit {
	idx := map[repograph.RepoPath]int{}
	out := []BrokenUnit{}
	for _, d := range ds {
		s := subjectOf(d)
		i, ok := idx[s]
		if !ok {
			i = len(out)
			idx[s] = i
			out = append(out, BrokenUnit{Subject: s})
		}
		out[i].Findings = append(out[i].Findings, d)
	}
	for i := range out {
		slices.SortFunc(out[i].Findings, diagnostic.Compare)
	}
	slices.SortFunc(out, func(a, b BrokenUnit) int { return a.Subject.Compare(b.Subject) })
	return out
}
