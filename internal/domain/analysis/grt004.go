package analysis

// GRT004 (RemovedOutputs) reclassifies a GRT001 when two trees are compared:
// a reference site of cur gets GRT004 instead of GRT001 iff all four checks
// of removedOutputChecks hold for it:
//
//   - fires: GRT001 fires at the site in cur (resolveReference ok and the cur
//     surface lacks the output);
//   - baseHadRef: base has a reference with the same unit, dependency name
//     and output (position-free);
//   - sameModule: the dependency resolves in base through rows 1, 4 and 5
//     (resolveDependency) and its module path equals the cur module path;
//   - baseDeclared: that base module's known surface declares the output.
//
// GRT004 only ever replaces a GRT001 (SupersedeUnknownOutputs) and only in
// blast; it is never one of checking's one-graph analyzers.

import (
	"strconv"

	"github.com/GiulioSavini/gruntled/internal/domain/diagnostic"
	"github.com/GiulioSavini/gruntled/internal/domain/repograph"
)

// siteFacts are computed independently of each other, so disabling one
// check is never masked by another (the necessity test relies on it).
type siteFacts struct {
	fires        bool // resolveReference(cur) ok && !cur surface.HasOutput(Y)
	baseHadRef   bool // base.References() has (unit, dep, Y)
	sameModule   bool // resolveDependency(base) ok && resolveDependency(cur) ok && module paths equal
	baseDeclared bool // resolveDependency(base) ok && base surface.HasOutput(Y)
}

// removedOutputCheck is one named GRT004 precondition over siteFacts.
type removedOutputCheck struct {
	name string
	ok   func(siteFacts) bool
}

// removedOutputChecks is the GRT004 precondition: a site gets GRT004 iff
// every check holds.
var removedOutputChecks = []removedOutputCheck{
	{name: "fires", ok: func(f siteFacts) bool { return f.fires }},
	{name: "baseHadRef", ok: func(f siteFacts) bool { return f.baseHadRef }},
	{name: "sameModule", ok: func(f siteFacts) bool { return f.sameModule }},
	{name: "baseDeclared", ok: func(f siteFacts) bool { return f.baseDeclared }},
}

// refKey is a reference's position-free identity across two trees.
type refKey struct {
	unit   repograph.RepoPath
	dep    string
	output string
}

// RemovedOutputs returns one GRT004 per reference site of cur, in
// cur.References() order, where every check in removedOutputChecks holds.
// It returns nil when there is none. The message names the dependency
// label, the output, the module and the target unit (repo-relative, no
// list, no position) and carries GRT001's mock suffix under exactly the
// same condition (mockMasksAtApply on the cur options and cur surface).
func RemovedOutputs(base, cur *repograph.RepositoryGraph) ([]diagnostic.Diagnostic, error) {
	return removedOutputs(base, cur, removedOutputChecks)
}

// removedOutputs is RemovedOutputs with an explicit check list (the seam the
// necessity test uses). RemovedOutputs == removedOutputs(base, cur,
// removedOutputChecks).
func removedOutputs(base, cur *repograph.RepositoryGraph, checks []removedOutputCheck) ([]diagnostic.Diagnostic, error) {
	baseRefs := make(map[refKey]struct{})
	for _, ur := range base.References() {
		baseRefs[refKey{unit: ur.Unit, dep: ur.Reference.Dependency(), output: ur.Reference.Output()}] = struct{}{}
	}

	var out []diagnostic.Diagnostic
	for _, ur := range cur.References() {
		dep := ur.Reference.Dependency()
		output := ur.Reference.Output()

		curRef, curRefOK := resolveReference(cur, ur)
		curDep, curDepOK := resolveDependency(cur, ur.Unit, dep)
		// Computed for every site, never gated on baseHadRef.
		baseDep, baseOK := resolveDependency(base, ur.Unit, dep)
		_, hadRef := baseRefs[refKey{unit: ur.Unit, dep: dep, output: output}]

		f := siteFacts{
			fires:        curRefOK && !curRef.surface.HasOutput(output),
			baseHadRef:   hadRef,
			sameModule:   baseOK && curDepOK && baseDep.module.Path() == curDep.module.Path(),
			baseDeclared: baseOK && baseDep.surface.HasOutput(output),
		}
		if !allHold(checks, f) {
			continue
		}
		// The message needs the cur module and target. fires and sameModule
		// each imply curDepOK, so with the full check list (or any list
		// missing one check) this never skips a site the checks accepted.
		if !curDepOK {
			continue
		}
		msg := "dependency " + strconv.Quote(curDep.dep.Name()) +
			" output " + strconv.Quote(output) +
			" was removed from module " + strconv.Quote(curDep.module.Path().String()) +
			" (target unit " + strconv.Quote(curDep.target.Path().String()) + ")"
		if mockMasksAtApply(curDep.dep.Options(), output, curDep.surface) {
			msg += mockMaskSuffix
		}
		d, err := diagnostic.NewForUnit(diagnostic.CodeRemovedOutput, diagnostic.SeverityError, ur.Unit, ur.Reference.Pos(), msg)
		if err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, nil
}

func allHold(checks []removedOutputCheck, f siteFacts) bool {
	for _, c := range checks {
		if !c.ok(f) {
			return false
		}
	}
	return true
}
