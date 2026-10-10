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
// are in diagnostic.Compare order. Reach is zero unless propagation
// traversed this Broken unit, so a path through it can be rebuilt.
type BrokenUnit struct {
	Subject  repograph.RepoPath
	Findings []diagnostic.Diagnostic
	Reach    Reach
}

// ImpactedUnit is a unit reached by propagation from a module whose surface
// changed: an instantiating unit (Reach.Distance 1) or a transitive
// dependent. Change is the change of Reach.Source's module.
type ImpactedUnit struct {
	Unit   repograph.RepoPath
	Change SurfaceChange
	Reach  Reach
}

// Result is a blast radius. Broken and Impacted are sorted by path, unique,
// disjoint (a Broken subject never appears in Impacted) and never nil.
// Changes is SurfaceDiff of the two graphs, sorted by Module, never nil.
// Baseline is false when no base tree was available, in which case Broken
// holds every current finding and Impacted and Changes are empty.
type Result struct {
	Baseline bool
	Broken   []BrokenUnit
	Impacted []ImpactedUnit
	Changes  []SurfaceChange
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

// WithMaxDistance returns r limited to propagation depth n (n >= 1; it
// panics on n < 1, callers validate): Impacted entries with Distance > n are
// dropped and Broken Reach beyond n is zeroed. Broken, Changes and Baseline
// are otherwise unchanged. BFS levels are exact minimum distances and every
// Via is one level closer, so this equals propagation limited to n levels.
// It never mutates r.
func (r Result) WithMaxDistance(n int) Result {
	if n < 1 {
		panic("impact: WithMaxDistance needs n >= 1")
	}
	out := Result{Baseline: r.Baseline, Changes: r.Changes, Broken: make([]BrokenUnit, len(r.Broken)), Impacted: []ImpactedUnit{}}
	for i, b := range r.Broken {
		if b.Reach.Distance > n {
			b.Reach = Reach{}
		}
		out.Broken[i] = b
	}
	for _, iu := range r.Impacted {
		if iu.Reach.Distance <= n {
			out.Impacted = append(out.Impacted, iu)
		}
	}
	return out
}

// Compute returns the blast radius from (baseG, baseD) to (curG, curD).
// Broken holds the findings new in cur, grouped by subject. Changes is
// SurfaceDiff(baseG, curG). Impacted holds every unit of the CURRENT graph
// reached by propagate from the seeds, the units whose resolved module is a
// changed module, over reverse dependency-block edges that Terragrunt reads
// (see propagates): instantiating units at distance 1, their dependents
// further out. Broken units are seeds or traversed like any other unit and
// carry their Reach, but are never listed in Impacted. A nil graph is
// treated as empty.
func Compute(baseG *repograph.RepositoryGraph, baseD diagnostic.Set, curG *repograph.RepositoryGraph, curD diagnostic.Set) Result {
	broken := groupBroken(NewFindings(baseD, curD))
	changes := SurfaceDiff(baseG, curG)
	impacted := []ImpactedUnit{}
	if len(changes) > 0 {
		byModule := make(map[repograph.RepoPath]SurfaceChange, len(changes))
		for _, c := range changes {
			byModule[c.Module] = c
		}
		var seeds []repograph.RepoPath
		moduleOf := map[repograph.RepoPath]repograph.RepoPath{}
		for _, u := range curG.Units() {
			mod, ok := u.Module()
			if !ok {
				continue
			}
			if _, ok := byModule[mod]; ok {
				seeds = append(seeds, u.Path())
				moduleOf[u.Path()] = mod
			}
		}
		reach := propagate(curG, seeds)
		isBroken := make(map[repograph.RepoPath]struct{}, len(broken))
		for i, b := range broken {
			isBroken[b.Subject] = struct{}{}
			if re, ok := reach[b.Subject]; ok {
				broken[i].Reach = re
			}
		}
		for u, re := range reach {
			if _, b := isBroken[u]; b {
				continue
			}
			impacted = append(impacted, ImpactedUnit{Unit: u, Change: byModule[moduleOf[re.Source]], Reach: re})
		}
		slices.SortFunc(impacted, func(a, b ImpactedUnit) int { return a.Unit.Compare(b.Unit) })
	}

	return Result{Baseline: true, Broken: broken, Impacted: impacted, Changes: changes}
}

// NoBaseline returns the degraded result used when no base tree exists:
// every current finding is Broken, nothing is Impacted, Baseline is false.
func NoBaseline(curD diagnostic.Set) Result {
	return Result{Baseline: false, Broken: groupBroken(curD.All()), Impacted: []ImpactedUnit{}, Changes: []SurfaceChange{}}
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
