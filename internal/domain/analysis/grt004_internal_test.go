package analysis

import (
	"slices"
	"strings"
	"testing"

	"github.com/GiulioSavini/gruntled/internal/domain/diagnostic"
	"github.com/GiulioSavini/gruntled/internal/domain/repograph"
)

// Tree specs for the GRT004 precondition matrix. The default tree is
// live/app (module modules/app) with dependency "vpc" -> live/vpc (module
// modules/vpc), referencing dependency.vpc.outputs.id at
// live/app/terragrunt.hcl:7:20; base modules/vpc declares id, cur does not.

type unitKind int

const (
	unitResolved unitKind = iota
	unitModuleUnknown
	unitConfigUnknown
)

type depSpec struct {
	name   string
	target string // "" builds an unresolved dependency
	opts   repograph.DependencyOptions
}

type refSpec struct {
	dep, output, file string
	line, col         int
}

type unitSpec struct {
	path   string
	kind   unitKind
	module string
	deps   []depSpec
	refs   []refSpec
}

type modSpec struct {
	path    string
	outputs []string
	unknown bool
}

type treeSpec struct {
	units []unitSpec
	mods  []modSpec
}

func (s *treeSpec) unit(path string) *unitSpec {
	for i := range s.units {
		if s.units[i].path == path {
			return &s.units[i]
		}
	}
	panic("no unit " + path)
}

func (s *treeSpec) mod(path string) *modSpec {
	for i := range s.mods {
		if s.mods[i].path == path {
			return &s.mods[i]
		}
	}
	panic("no module " + path)
}

func defaultTree(vpcOutputs ...string) treeSpec {
	return treeSpec{
		units: []unitSpec{
			{path: "live/app", module: "modules/app",
				deps: []depSpec{{name: "vpc", target: "live/vpc", opts: repograph.DefaultDependencyOptions()}},
				refs: []refSpec{{dep: "vpc", output: "id", file: "live/app/terragrunt.hcl", line: 7, col: 20}}},
			{path: "live/vpc", module: "modules/vpc"},
		},
		mods: []modSpec{
			{path: "modules/app"},
			{path: "modules/vpc", outputs: vpcOutputs},
		},
	}
}

func ipos(t *testing.T, file string, line, col int) repograph.Position {
	t.Helper()
	p, err := repograph.NewPosition(repograph.MustRepoPath(file), line, col)
	if err != nil {
		t.Fatalf("NewPosition: %v", err)
	}
	return p
}

func buildTree(t *testing.T, s treeSpec) *repograph.RepositoryGraph {
	t.Helper()
	var units []repograph.Unit
	for _, us := range s.units {
		path := repograph.MustRepoPath(us.path)
		var deps []repograph.Dependency
		for _, ds := range us.deps {
			p := ipos(t, us.path+"/terragrunt.hcl", 1, 1)
			var d repograph.Dependency
			var err error
			if ds.target == "" {
				d, err = repograph.NewUnresolvedDependency(ds.name, "config-path-not-literal", p, p, ds.opts)
			} else {
				d, err = repograph.NewDependency(ds.name, repograph.MustRepoPath(ds.target), p, p, repograph.TargetUnknown, ds.opts)
			}
			if err != nil {
				t.Fatalf("dependency: %v", err)
			}
			deps = append(deps, d)
		}
		var refs []repograph.Reference
		for _, rs := range us.refs {
			r, err := repograph.NewReference(rs.dep, rs.output, ipos(t, rs.file, rs.line, rs.col))
			if err != nil {
				t.Fatalf("NewReference: %v", err)
			}
			refs = append(refs, r)
		}
		var u repograph.Unit
		var err error
		switch us.kind {
		case unitResolved:
			u, err = repograph.NewResolvedUnit(path, repograph.MustRepoPath(us.module), deps, refs)
		case unitModuleUnknown:
			u, err = repograph.NewModuleUnknownUnit(path, "source-remote", deps, refs)
		case unitConfigUnknown:
			u, err = repograph.NewConfigUnknownUnit(path, "config-syntax-error")
		}
		if err != nil {
			t.Fatalf("unit %s: %v", us.path, err)
		}
		units = append(units, u)
	}
	var mods []repograph.Module
	for _, ms := range s.mods {
		path := repograph.MustRepoPath(ms.path)
		var m repograph.Module
		var err error
		if ms.unknown {
			m, err = repograph.NewUnknownModule(path, "module-file-syntax-error")
		} else {
			var surf repograph.Surface
			surf, err = repograph.NewSurface(nil, ms.outputs)
			if err != nil {
				t.Fatalf("NewSurface: %v", err)
			}
			m, err = repograph.NewModule(path, surf)
		}
		if err != nil {
			t.Fatalf("module %s: %v", ms.path, err)
		}
		mods = append(mods, m)
	}
	g, err := repograph.NewRepositoryGraph(units, mods)
	if err != nil {
		t.Fatalf("NewRepositoryGraph: %v", err)
	}
	return g
}

// site is an expected GRT004: unit, position and, when msg is non-empty,
// the exact message.
type site struct {
	unit, file string
	line, col  int
	msg        string
}

type matrixRow struct {
	name string
	base func() treeSpec
	cur  func() treeSpec
	want []site
	// curGRT001 is, when >= 0, how many GRT001 UnknownOutputs(cur) must
	// return (-1 skips the assertion).
	curGRT001 int
}

const removedMsg = `dependency "vpc" output "id" was removed from module "modules/vpc" (target unit "live/vpc")`

func appSite(line int) site {
	return site{unit: "live/app", file: "live/app/terragrunt.hcl", line: line, col: 20}
}

func withMsg(s site, msg string) site { s.msg = msg; return s }

func mutDep(s treeSpec, f func(o *repograph.DependencyOptions)) treeSpec {
	f(&s.unit("live/app").deps[0].opts)
	return s
}

func matrixRows() []matrixRow {
	base := func() treeSpec { return defaultTree("id") }
	cur := func() treeSpec { return defaultTree() }
	mockOpts := func(allowed []string) func(o *repograph.DependencyOptions) {
		return func(o *repograph.DependencyOptions) {
			names, err := repograph.KnownNames([]string{"id"})
			if err != nil {
				panic(err)
			}
			o.MockOutputs = names
			o.MockMergeWithState = repograph.TristateTrue
			if allowed != nil {
				l, err := repograph.KnownNames(allowed)
				if err != nil {
					panic(err)
				}
				o.MockAllowedCommands = l
			}
		}
	}
	return []matrixRow{
		{name: "removed", base: base, cur: cur, want: []site{withMsg(appSite(7), removedMsg)}, curGRT001: 1},
		{name: "still declared in cur", base: base, cur: func() treeSpec { return defaultTree("id") }, curGRT001: 0},
		{name: "reference added in this change", base: func() treeSpec {
			s := defaultTree("id")
			s.unit("live/app").refs = nil
			return s
		}, cur: cur, curGRT001: 1},
		{name: "base references id at another position", base: func() treeSpec {
			s := defaultTree("id")
			s.unit("live/app").refs[0].line, s.unit("live/app").refs[0].col = 3, 4
			return s
		}, cur: cur, want: []site{appSite(7)}, curGRT001: 1},
		{name: "dependency re-pointed to another module", base: func() treeSpec {
			s := defaultTree("id")
			s.units = append(s.units, unitSpec{path: "live/other", module: "modules/other"})
			s.mods = append(s.mods, modSpec{path: "modules/other"})
			return s
		}, cur: func() treeSpec {
			s := defaultTree("id")
			s.unit("live/app").deps[0].target = "live/other"
			s.units = append(s.units, unitSpec{path: "live/other", module: "modules/other"})
			s.mods = append(s.mods, modSpec{path: "modules/other"})
			return s
		}, curGRT001: 1},
		{name: "target unit's source changed", base: base, cur: func() treeSpec {
			s := defaultTree("id")
			s.unit("live/vpc").module = "modules/vpc2"
			s.mods = append(s.mods, modSpec{path: "modules/vpc2"})
			return s
		}, curGRT001: 1},
		{name: "dependency re-pointed within the same module", base: func() treeSpec {
			s := defaultTree("id")
			s.units = append(s.units, unitSpec{path: "live/vpc_b", module: "modules/vpc"})
			return s
		}, cur: func() treeSpec {
			s := defaultTree()
			s.unit("live/app").deps[0].target = "live/vpc_b"
			s.units = append(s.units, unitSpec{path: "live/vpc_b", module: "modules/vpc"})
			return s
		}, want: []site{withMsg(appSite(7), `dependency "vpc" output "id" was removed from module "modules/vpc" (target unit "live/vpc_b")`)}, curGRT001: 1},
		{name: "base module never declared id", base: func() treeSpec { return defaultTree("other") }, cur: func() treeSpec { return defaultTree("other") }, curGRT001: 1},
		{name: "base module surface unknown", base: func() treeSpec {
			s := defaultTree()
			s.mod("modules/vpc").unknown = true
			return s
		}, cur: cur, curGRT001: 1},
		{name: "base target module-unknown", base: func() treeSpec {
			s := defaultTree("id")
			s.unit("live/vpc").kind = unitModuleUnknown
			return s
		}, cur: cur, curGRT001: 1},
		{name: "base target config-unknown", base: func() treeSpec {
			s := defaultTree("id")
			s.unit("live/vpc").kind = unitConfigUnknown
			return s
		}, cur: cur, curGRT001: 1},
		{name: "base dependency unresolved", base: func() treeSpec {
			s := defaultTree("id")
			s.unit("live/app").deps[0].target = ""
			return s
		}, cur: cur, curGRT001: 1},
		{name: "base unit has no dependency vpc", base: func() treeSpec {
			s := defaultTree("id")
			s.unit("live/app").deps = nil
			return s
		}, cur: cur, curGRT001: 1},
		{name: "cur module surface unknown", base: base, cur: func() treeSpec {
			s := defaultTree()
			s.mod("modules/vpc").unknown = true
			return s
		}, curGRT001: 0},
		{name: "base enabled false, cur default", base: func() treeSpec {
			return mutDep(defaultTree("id"), func(o *repograph.DependencyOptions) { o.Enabled = repograph.TristateFalse })
		}, cur: cur, want: []site{withMsg(appSite(7), removedMsg)}, curGRT001: 1},
		{name: "cur enabled false", base: base, cur: func() treeSpec {
			return mutDep(defaultTree(), func(o *repograph.DependencyOptions) { o.Enabled = repograph.TristateFalse })
		}, curGRT001: 0},
		{name: "cur enabled unknown", base: base, cur: func() treeSpec {
			return mutDep(defaultTree(), func(o *repograph.DependencyOptions) { o.Enabled = repograph.TristateUnknown })
		}, curGRT001: 0},
		{name: "cur skip_outputs true", base: base, cur: func() treeSpec {
			return mutDep(defaultTree(), func(o *repograph.DependencyOptions) { o.SkipOutputs = repograph.TristateTrue })
		}, curGRT001: 0},
		{name: "cur skip_outputs unknown", base: base, cur: func() treeSpec {
			return mutDep(defaultTree(), func(o *repograph.DependencyOptions) { o.SkipOutputs = repograph.TristateUnknown })
		}, curGRT001: 0},
		{name: "cur mock covers id, merge true, apply allowed", base: base, cur: func() treeSpec {
			return mutDep(defaultTree(), mockOpts([]string{"plan", "apply"}))
		}, want: []site{withMsg(appSite(7), removedMsg+mockMaskSuffix)}, curGRT001: 1},
		{name: "cur mock covers id, apply not allowed", base: base, cur: func() treeSpec {
			return mutDep(defaultTree(), mockOpts([]string{"validate", "plan"}))
		}, want: []site{withMsg(appSite(7), removedMsg)}, curGRT001: 1},
		{name: "two references in one unit", base: func() treeSpec {
			s := defaultTree("id")
			s.unit("live/app").refs = append(s.unit("live/app").refs, refSpec{dep: "vpc", output: "id", file: "live/app/terragrunt.hcl", line: 9, col: 20})
			return s
		}, cur: func() treeSpec {
			s := defaultTree()
			s.unit("live/app").refs = append(s.unit("live/app").refs, refSpec{dep: "vpc", output: "id", file: "live/app/terragrunt.hcl", line: 9, col: 20})
			return s
		}, want: []site{withMsg(appSite(7), removedMsg), withMsg(appSite(9), removedMsg)}, curGRT001: 2},
		{name: "same line:col in two files of one unit", base: func() treeSpec {
			s := defaultTree("id")
			s.unit("live/app").refs = []refSpec{
				{dep: "vpc", output: "id", file: "live/app/root.hcl", line: 7, col: 20},
				{dep: "vpc", output: "never", file: "live/app/terragrunt.hcl", line: 7, col: 20},
			}
			return s
		}, cur: func() treeSpec {
			s := defaultTree()
			s.unit("live/app").refs = []refSpec{
				{dep: "vpc", output: "id", file: "live/app/root.hcl", line: 7, col: 20},
				{dep: "vpc", output: "never", file: "live/app/terragrunt.hcl", line: 7, col: 20},
			}
			return s
		}, want: []site{{unit: "live/app", file: "live/app/root.hcl", line: 7, col: 20, msg: removedMsg}}, curGRT001: 2},
		{name: "shared include read by two units", base: func() treeSpec {
			return sharedInclude("id")
		}, cur: func() treeSpec {
			return sharedInclude()
		}, want: []site{
			{unit: "live/a", file: "live/_common/vpc.hcl", line: 3, col: 10, msg: removedMsg},
			{unit: "live/b", file: "live/_common/vpc.hcl", line: 3, col: 10, msg: removedMsg},
		}, curGRT001: 2},
		{name: "unrelated never-declared output in cur only", base: base, cur: func() treeSpec {
			s := defaultTree("id")
			s.units = append(s.units, unitSpec{path: "live/z", module: "modules/app",
				deps: []depSpec{{name: "vpc", target: "live/vpc", opts: repograph.DefaultDependencyOptions()}},
				refs: []refSpec{{dep: "vpc", output: "ghost", file: "live/z/terragrunt.hcl", line: 2, col: 2}}})
			return s
		}, curGRT001: 1},
		{name: "empty graphs", base: func() treeSpec { return treeSpec{} }, cur: func() treeSpec { return treeSpec{} }, curGRT001: 0},
		{name: "base has no references", base: func() treeSpec {
			s := defaultTree("id")
			s.unit("live/app").refs = nil
			s.unit("live/app").deps = nil
			return s
		}, cur: cur, curGRT001: 1},
	}
}

// sharedInclude is live/a and live/b both reading live/_common/vpc.hcl,
// which references dependency.vpc.outputs.id at 3:10.
func sharedInclude(vpcOutputs ...string) treeSpec {
	unit := func(p string) unitSpec {
		return unitSpec{path: p, module: "modules/app",
			deps: []depSpec{{name: "vpc", target: "live/vpc", opts: repograph.DefaultDependencyOptions()}},
			refs: []refSpec{{dep: "vpc", output: "id", file: "live/_common/vpc.hcl", line: 3, col: 10}}}
	}
	return treeSpec{
		units: []unitSpec{unit("live/a"), unit("live/b"), {path: "live/vpc", module: "modules/vpc"}},
		mods:  []modSpec{{path: "modules/app"}, {path: "modules/vpc", outputs: vpcOutputs}},
	}
}

// gotSites renders ds as sites (with messages) for comparison.
func gotSites(t *testing.T, ds []diagnostic.Diagnostic) []site {
	t.Helper()
	var out []site
	for _, d := range ds {
		if d.Code() != diagnostic.CodeRemovedOutput {
			t.Errorf("code = %v, want GRT004", d.Code())
		}
		if d.Severity() != diagnostic.SeverityError {
			t.Errorf("severity = %v, want error", d.Severity())
		}
		u, ok := d.Unit()
		if !ok {
			t.Errorf("GRT004 without unit: %v", d)
		}
		out = append(out, site{unit: u.String(), file: d.Pos().File().String(), line: d.Pos().Line(), col: d.Pos().Column(), msg: d.Message()})
	}
	return out
}

// sitesMatch reports whether got equals want; a want with an empty msg
// matches any message.
func sitesMatch(got, want []site) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range want {
		w := want[i]
		if w.msg == "" {
			w.msg = got[i].msg
		}
		if got[i] != w {
			return false
		}
	}
	return true
}

func runRow(t *testing.T, r matrixRow, checks []removedOutputCheck) []site {
	t.Helper()
	ds, err := removedOutputs(buildTree(t, r.base()), buildTree(t, r.cur()), checks)
	if err != nil {
		t.Fatalf("removedOutputs: %v", err)
	}
	return gotSites(t, ds)
}

func TestRemovedOutputsMatrix(t *testing.T) {
	for _, r := range matrixRows() {
		t.Run(r.name, func(t *testing.T) {
			base, cur := buildTree(t, r.base()), buildTree(t, r.cur())
			ds, err := RemovedOutputs(base, cur)
			if err != nil {
				t.Fatalf("RemovedOutputs: %v", err)
			}
			if len(r.want) == 0 && ds != nil {
				t.Errorf("RemovedOutputs = %v, want nil", ds)
			}
			got := gotSites(t, ds)
			if !sitesMatch(got, r.want) {
				t.Errorf("RemovedOutputs sites =\n  %+v\nwant\n  %+v", got, r.want)
			}
			if r.curGRT001 >= 0 {
				u, err := UnknownOutputs(cur)
				if err != nil {
					t.Fatalf("UnknownOutputs: %v", err)
				}
				if len(u) != r.curGRT001 {
					t.Errorf("UnknownOutputs(cur) = %d diagnostics, want %d: %v", len(u), r.curGRT001, u)
				}
				// Every GRT004 sits on a GRT001 site of cur.
				for _, d := range ds {
					found := slices.ContainsFunc(u, func(g diagnostic.Diagnostic) bool {
						gu, _ := g.Unit()
						du, _ := d.Unit()
						return gu == du && g.Pos() == d.Pos()
					})
					if !found {
						t.Errorf("GRT004 %v has no GRT001 at its site", d)
					}
				}
			}
		})
	}
}

// TestRemovedOutputsChecksAreNecessary disables one check at a time and
// requires that some matrix row then gives a result other than expected,
// and in particular that the check's named killing row does.
func TestRemovedOutputsChecksAreNecessary(t *testing.T) {
	killer := map[string]string{
		"fires":        "still declared in cur",
		"baseHadRef":   "reference added in this change",
		"sameModule":   "dependency re-pointed to another module",
		"baseDeclared": "base module never declared id",
	}
	if len(killer) != len(removedOutputChecks) {
		t.Fatalf("killer table has %d entries, removedOutputChecks %d", len(killer), len(removedOutputChecks))
	}
	rows := matrixRows()
	for i, c := range removedOutputChecks {
		t.Run(c.name, func(t *testing.T) {
			want, ok := killer[c.name]
			if !ok {
				t.Fatalf("check %q has no named killing row", c.name)
			}
			mutant := slices.Delete(slices.Clone(removedOutputChecks), i, i+1)
			var killed []string
			for _, r := range rows {
				if !sitesMatch(runRow(t, r, mutant), r.want) {
					killed = append(killed, r.name)
				}
			}
			if len(killed) == 0 {
				t.Fatalf("check %q kills no row: it is vacuous and must be deleted", c.name)
			}
			if !slices.Contains(killed, want) {
				t.Errorf("check %q: killing row %q survives (killed: %s)", c.name, want, strings.Join(killed, "; "))
			}
			t.Logf("check %q killed by: %s", c.name, strings.Join(killed, "; "))
		})
	}
}
