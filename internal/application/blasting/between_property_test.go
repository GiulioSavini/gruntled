package blasting_test

import (
	"context"
	"strconv"
	"testing"

	"github.com/GiulioSavini/gruntled/internal/application/blasting"
	"github.com/GiulioSavini/gruntled/internal/application/checking"
	"github.com/GiulioSavini/gruntled/internal/application/ports"
	"github.com/GiulioSavini/gruntled/internal/domain/diagnostic"
	"github.com/GiulioSavini/gruntled/internal/domain/impact"
	"github.com/GiulioSavini/gruntled/internal/domain/repograph"
)

// The property below is driven by a fixed xorshift sequence: application
// tests may import only the standard library plus testing and reflect
// (check-architecture application-stdlib-allowlist), so rapid and
// math/rand are out.
type xorshift uint64

func (x *xorshift) intn(n int) int {
	*x ^= *x << 13
	*x ^= *x >> 7
	*x ^= *x << 17
	return int(uint64(*x) % uint64(n))
}

const (
	gUnits   = 6
	gModules = 3
	gDeps    = 2 // dependency names d0, d1; references may also name d2 (undeclared)
	gOutputs = 3 // a, b, c
)

var gOutputNames = [gOutputs]string{"a", "b", "c"}

// Dependency options a generated dependency can carry.
const (
	optDefault = iota
	optEnabledFalse
	optEnabledUnknown
	optSkipTrue
	optSkipUnknown
	optMockA // mock_outputs covers a, merge_with_state true
	optMockB
	optCount
)

// gDep targets: 0..gUnits-1 is a unit, gUnits is a path that is no unit,
// gUnits+1 is an unresolved dependency.
type gDep struct {
	present bool
	target  int
	opt     int
}

type gUnit struct {
	modUnknown bool
	mod        int
	deps       [gDeps]gDep
	refs       [gDeps + 1][gOutputs]bool
}

type gMod struct {
	unknown bool
	outs    [gOutputs]bool
}

// gTree is a value type: assigning it copies everything.
type gTree struct {
	units [gUnits]gUnit
	mods  [gModules]gMod
}

func gUnitPath(i int) repograph.RepoPath {
	return repograph.MustRepoPath("live/u" + strconv.Itoa(i))
}

func gModPath(i int) repograph.RepoPath {
	return repograph.MustRepoPath("modules/m" + strconv.Itoa(i))
}

func gDepName(i int) string { return "d" + strconv.Itoa(i) }

// gRefPos is unique per (unit, dep, output), so add/remove never collide.
func gRefPos(t *testing.T, u, d, o int) repograph.Position {
	return mustPos(t, gUnitPath(u).String()+"/terragrunt.hcl", 10+d*gOutputs+o, 3)
}

func genTree(r *xorshift) gTree {
	var g gTree
	for m := range g.mods {
		g.mods[m].unknown = r.intn(8) == 0
		for o := range g.mods[m].outs {
			g.mods[m].outs[o] = r.intn(3) != 0
		}
	}
	for u := range g.units {
		gu := &g.units[u]
		gu.modUnknown = r.intn(8) == 0
		gu.mod = r.intn(gModules)
		for d := range gu.deps {
			gu.deps[d] = gDep{present: r.intn(4) != 0, target: r.intn(gUnits + 2), opt: genOpt(r)}
		}
		for d := range gu.refs {
			for o := range gu.refs[d] {
				gu.refs[d][o] = r.intn(3) == 0
			}
		}
	}
	return g
}

// genOpt is mostly the default, so that GRT001 sites are common.
func genOpt(r *xorshift) int {
	if r.intn(2) == 0 {
		return optDefault
	}
	return r.intn(optCount)
}

// editTree applies 1-3 random edits.
func editTree(r *xorshift, g gTree) gTree {
	for range 1 + r.intn(3) {
		u, m, d, o := r.intn(gUnits), r.intn(gModules), r.intn(gDeps), r.intn(gOutputs)
		switch r.intn(8) {
		case 0, 1: // drop an output (twice as likely: it is what GRT004 is about)
			g.mods[m].outs[o] = false
		case 2:
			g.mods[m].outs[o] = true
		case 3:
			g.units[u].deps[d].target = r.intn(gUnits + 2)
		case 4:
			if r.intn(4) == 0 {
				g.units[u].modUnknown = !g.units[u].modUnknown
			} else {
				g.units[u].mod = r.intn(gModules)
			}
		case 5:
			rd := r.intn(gDeps + 1)
			g.units[u].refs[rd][o] = !g.units[u].refs[rd][o]
		case 6:
			g.units[u].deps[d].opt = r.intn(optCount)
		case 7:
			g.mods[m].unknown = !g.mods[m].unknown
		}
	}
	return g
}

func gOptions(t *testing.T, opt int) repograph.DependencyOptions {
	o := repograph.DefaultDependencyOptions()
	switch opt {
	case optEnabledFalse:
		o.Enabled = repograph.TristateFalse
	case optEnabledUnknown:
		o.Enabled = repograph.TristateUnknown
	case optSkipTrue:
		o.SkipOutputs = repograph.TristateTrue
	case optSkipUnknown:
		o.SkipOutputs = repograph.TristateUnknown
	case optMockA, optMockB:
		names, err := repograph.KnownNames([]string{gOutputNames[opt-optMockA]})
		if err != nil {
			t.Fatalf("KnownNames: %v", err)
		}
		o.MockOutputs = names
		o.MockMergeWithState = repograph.TristateTrue
	}
	return o
}

func gSources(t *testing.T, g gTree) blasting.Sources {
	t.Helper()
	var units []ports.UnitConfig
	for u, gu := range g.units {
		uc := ports.UnitConfig{Path: gUnitPath(u), Module: gModPath(gu.mod)}
		if gu.modUnknown {
			uc.ModuleUnknownReason = "source-remote"
		}
		file := gUnitPath(u).String() + "/terragrunt.hcl"
		for d, gd := range gu.deps {
			if !gd.present {
				continue
			}
			p := mustPos(t, file, 1+d, 1)
			var dep repograph.Dependency
			var err error
			switch {
			case gd.target < gUnits:
				dep, err = repograph.NewDependency(gDepName(d), gUnitPath(gd.target), p, p, repograph.TargetUnknown, gOptions(t, gd.opt))
			case gd.target == gUnits:
				dep, err = repograph.NewDependency(gDepName(d), repograph.MustRepoPath("live/none"), p, p, repograph.TargetUnknown, gOptions(t, gd.opt))
			default:
				dep, err = repograph.NewUnresolvedDependency(gDepName(d), "config-path-not-literal", p, p, gOptions(t, gd.opt))
			}
			if err != nil {
				t.Fatalf("dependency: %v", err)
			}
			uc.Dependencies = append(uc.Dependencies, dep)
		}
		for d := range gu.refs {
			for o, on := range gu.refs[d] {
				if !on {
					continue
				}
				ref, err := repograph.NewReference(gDepName(d), gOutputNames[o], gRefPos(t, u, d, o))
				if err != nil {
					t.Fatalf("NewReference: %v", err)
				}
				uc.References = append(uc.References, ref)
			}
		}
		units = append(units, uc)
	}
	surfaces := map[string]ports.SurfaceResult{}
	for m, gm := range g.mods {
		if gm.unknown {
			surfaces[gModPath(m).String()] = ports.SurfaceResult{UnknownReason: "module-file-syntax-error"}
			continue
		}
		var outs []string
		for o, on := range gm.outs {
			if on {
				outs = append(outs, gOutputNames[o])
			}
		}
		surfaces[gModPath(m).String()] = ports.SurfaceResult{Surface: mustSurface(t, outs...)}
	}
	return blasting.Sources{Units: fakeLoader{res: ports.LoadResult{Units: units}}, Surfaces: fakeSurfaces{t: t, byModule: surfaces}}
}

// gResolve resolves unit u's dependency d in g the MORE-03 way: the block
// exists, its target is a unit of g, whose module is known and whose
// module surface is known. It returns the module index.
func gResolve(g gTree, u, d int) (int, bool) {
	if d >= gDeps || !g.units[u].deps[d].present {
		return 0, false
	}
	target := g.units[u].deps[d].target
	if target >= gUnits {
		return 0, false
	}
	tu := g.units[target]
	if tu.modUnknown || g.mods[tu.mod].unknown {
		return 0, false
	}
	return tu.mod, true
}

type gSite struct {
	unit repograph.RepoPath
	pos  repograph.Position
}

func siteOfD(d diagnostic.Diagnostic) gSite {
	u, _ := d.Unit()
	return gSite{unit: u, pos: d.Pos()}
}

// oracle predicts the GRT004 sites from the MORE-03 wording, directly over
// the generated trees: the site has a GRT001 in cur; base has the same
// (unit, dependency, output) reference; in base the dependency resolves to
// a unit with a known module and surface, with the same module path as in
// cur; and the base surface declares the output.
func oracle(t *testing.T, base, cur gTree, curD diagnostic.Set) map[gSite]bool {
	grt001 := map[gSite]bool{}
	for _, d := range curD.All() {
		if d.Code() == diagnostic.CodeUnknownOutput {
			grt001[siteOfD(d)] = true
		}
	}
	want := map[gSite]bool{}
	for u := range cur.units {
		for d := range cur.units[u].refs {
			for o, on := range cur.units[u].refs[d] {
				if !on || !base.units[u].refs[d][o] {
					continue
				}
				s := gSite{unit: gUnitPath(u), pos: gRefPos(t, u, d, o)}
				if !grt001[s] {
					continue
				}
				bm, ok := gResolve(base, u, d)
				if !ok {
					continue
				}
				cm, ok := gResolve(cur, u, d)
				if !ok || cm != bm {
					continue
				}
				if !base.mods[bm].outs[o] {
					continue
				}
				want[s] = true
			}
		}
	}
	return want
}

// removedUnreferenced reports whether some module lost an output that no
// current reference reads through a dependency resolving to that module.
func removedUnreferenced(base, cur gTree) bool {
	for m := range cur.mods {
		if base.mods[m].unknown || cur.mods[m].unknown {
			continue
		}
		for o := range cur.mods[m].outs {
			if !base.mods[m].outs[o] || cur.mods[m].outs[o] {
				continue
			}
			read := false
			for u := range cur.units {
				for d := range cur.units[u].refs {
					if !cur.units[u].refs[d][o] {
						continue
					}
					if cm, ok := gResolve(cur, u, d); ok && cm == m {
						read = true
					}
				}
			}
			if !read {
				return true
			}
		}
	}
	return false
}

// TestBetweenOnlyReclassifiesGRT001 compares Between with v0.3's blast
// (impact.Compute on the un-superseded sets) over generated tree pairs.
func TestBetweenOnlyReclassifiesGRT001(t *testing.T) {
	const cases = 3000
	r := xorshift(0x13013013)
	withGRT004, keptBeside, unreferenced := 0, 0, 0
	for i := range cases {
		baseT := genTree(&r)
		curT := editTree(&r, baseT)
		baseRep, err := checking.Check(context.Background(), gSources(t, baseT).Units, gSources(t, baseT).Surfaces)
		if err != nil {
			t.Fatalf("case %d: Check(base): %v", i, err)
		}
		curRep, err := checking.Check(context.Background(), gSources(t, curT).Units, gSources(t, curT).Surfaces)
		if err != nil {
			t.Fatalf("case %d: Check(cur): %v", i, err)
		}
		got, err := blasting.Between(baseRep, curRep)
		if err != nil {
			t.Fatalf("case %d: Between: %v", i, err)
		}
		v03 := impact.Compute(baseRep.Graph, baseRep.Diagnostics, curRep.Graph, curRep.Diagnostics)

		if len(got.Broken) != len(v03.Broken) {
			t.Fatalf("case %d: %d Broken subjects, v0.3 %d", i, len(got.Broken), len(v03.Broken))
		}
		gotSites := map[gSite]bool{}
		n001, n004 := 0, 0
		for j, gb := range got.Broken {
			vb := v03.Broken[j]
			if gb.Subject != vb.Subject || len(gb.Findings) != len(vb.Findings) {
				t.Fatalf("case %d: Broken[%d] = %v (%d), v0.3 %v (%d)", i, j, gb.Subject, len(gb.Findings), vb.Subject, len(vb.Findings))
			}
			codes := map[gSite]map[diagnostic.Code]bool{}
			for k, gd := range gb.Findings {
				vd := vb.Findings[k]
				if siteOfD(gd) != siteOfD(vd) {
					t.Fatalf("case %d: finding %d of %v at %v, v0.3 at %v", i, k, gb.Subject, gd.Pos(), vd.Pos())
				}
				if gd != vd {
					if vd.Code() != diagnostic.CodeUnknownOutput || gd.Code() != diagnostic.CodeRemovedOutput {
						t.Fatalf("case %d: %v differs from v0.3 %v other than GRT001 -> GRT004", i, gd, vd)
					}
					if gd.Severity() != diagnostic.SeverityError || vd.Severity() != diagnostic.SeverityError {
						t.Fatalf("case %d: severity changed: %v / %v", i, gd, vd)
					}
				}
				s := siteOfD(gd)
				if codes[s] == nil {
					codes[s] = map[diagnostic.Code]bool{}
				}
				codes[s][gd.Code()] = true
				switch gd.Code() {
				case diagnostic.CodeRemovedOutput:
					n004++
					gotSites[s] = true
				case diagnostic.CodeUnknownOutput:
					n001++
				}
			}
			for s, c := range codes {
				if c[diagnostic.CodeUnknownOutput] && c[diagnostic.CodeRemovedOutput] {
					t.Fatalf("case %d: site %v holds both GRT001 and GRT004", i, s)
				}
			}
		}
		if len(got.Impacted) != len(v03.Impacted) {
			t.Fatalf("case %d: Impacted %v, v0.3 %v", i, got.Impacted, v03.Impacted)
		}
		for j := range got.Impacted {
			if got.Impacted[j].Unit != v03.Impacted[j].Unit {
				t.Fatalf("case %d: Impacted %v, v0.3 %v", i, got.Impacted, v03.Impacted)
			}
		}
		if got.HasErrors() != v03.HasErrors() {
			t.Fatalf("case %d: HasErrors %v, v0.3 %v", i, got.HasErrors(), v03.HasErrors())
		}

		want := oracle(t, baseT, curT, curRep.Diagnostics)
		if len(want) != len(gotSites) {
			t.Fatalf("case %d: GRT004 sites %v, oracle %v", i, gotSites, want)
		}
		for s := range want {
			if !gotSites[s] {
				t.Fatalf("case %d: oracle predicts GRT004 at %v, got %v", i, s, gotSites)
			}
		}

		if n004 > 0 {
			withGRT004++
			if n001 > 0 {
				keptBeside++
			}
		}
		if removedUnreferenced(baseT, curT) {
			unreferenced++
		}
	}
	t.Logf("%d cases: %d with >=1 GRT004, %d with a GRT001 kept beside a GRT004, %d with an unreferenced output removed", cases, withGRT004, keptBeside, unreferenced)
	if withGRT004 == 0 || keptBeside == 0 || unreferenced == 0 {
		t.Fatalf("vacuous run: %d / %d / %d", withGRT004, keptBeside, unreferenced)
	}
}
