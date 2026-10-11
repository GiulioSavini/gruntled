package impact_test

import (
	"reflect"
	"slices"
	"strconv"
	"testing"

	"github.com/GiulioSavini/gruntled/internal/domain/diagnostic"
	"github.com/GiulioSavini/gruntled/internal/domain/impact"
	"github.com/GiulioSavini/gruntled/internal/domain/repograph"
)

const (
	tT = repograph.TristateTrue
	tF = repograph.TristateFalse
	tU = repograph.TristateUnknown
)

type (
	vd = repograph.VariableDecl
	od = repograph.OutputDecl
)

func factsMod(t tb, path string, vars []vd, outs []od) repograph.Module {
	t.Helper()
	s, err := repograph.NewSurfaceFacts(vars, outs)
	if err != nil {
		t.Fatalf("NewSurfaceFacts: %v", err)
	}
	m, err := repograph.NewModule(p(path), s)
	if err != nil {
		t.Fatalf("NewModule: %v", err)
	}
	return m
}

// diffFacts runs SurfaceDiff over one module "m" with the given decls.
func diffFacts(t *testing.T, bv []vd, bo []od, cv []vd, co []od) []impact.SurfaceChange {
	t.Helper()
	return impact.SurfaceDiff(graph(t, nil, factsMod(t, "m", bv, bo)), graph(t, nil, factsMod(t, "m", cv, co)))
}

func tc(kind impact.TypeChangeKind, name, old, new string) impact.TypeChange {
	return impact.TypeChange{Kind: kind, Name: name, Old: old, New: new}
}

func TestTypeChangeKindString(t *testing.T) {
	for k, want := range map[impact.TypeChangeKind]string{
		impact.VariableRequired: "variable_required",
		impact.VariableType:     "variable_type",
		impact.OutputType:       "output_type",
		impact.OutputSensitive:  "output_sensitive",
	} {
		if got := k.String(); got != want {
			t.Errorf("%d.String() = %q, want %q", int(k), got, want)
		}
	}
}

func TestTypeChangeMatrix(t *testing.T) {
	cases := []struct {
		name   string
		bv     []vd
		bo     []od
		cv     []vd
		co     []od
		want   []impact.TypeChange
		names  bool // a name change is expected too
		silent bool // no SurfaceChange at all
	}{
		{name: "new variable required", cv: []vd{{Name: "x", Required: tT, Type: "any"}},
			want: []impact.TypeChange{tc(impact.VariableRequired, "x", "absent", "required")}, names: true},
		{name: "new variable optional", cv: []vd{{Name: "x", Required: tF, Type: "any"}}, names: true},
		{name: "new variable required unknown", cv: []vd{{Name: "x", Required: tU}}, names: true},
		{name: "optional to required", bv: []vd{{Name: "x", Required: tF, Type: "string"}}, cv: []vd{{Name: "x", Required: tT, Type: "string"}},
			want: []impact.TypeChange{tc(impact.VariableRequired, "x", "optional", "required")}},
		{name: "required to optional", bv: []vd{{Name: "x", Required: tT}}, cv: []vd{{Name: "x", Required: tF}}, silent: true},
		{name: "required unknown base", bv: []vd{{Name: "x", Required: tU}}, cv: []vd{{Name: "x", Required: tT}}, silent: true},
		{name: "required unknown cur", bv: []vd{{Name: "x", Required: tF}}, cv: []vd{{Name: "x", Required: tU}}, silent: true},
		{name: "variable type change", bv: []vd{{Name: "x", Required: tT, Type: "string"}}, cv: []vd{{Name: "x", Required: tT, Type: "list(string)"}},
			want: []impact.TypeChange{tc(impact.VariableType, "x", "string", "list(string)")}},
		{name: "variable type to any (D-15-04)", bv: []vd{{Name: "x", Required: tT, Type: "list(string)"}}, cv: []vd{{Name: "x", Required: tT, Type: "any"}},
			want: []impact.TypeChange{tc(impact.VariableType, "x", "list(string)", "any")}},
		{name: "variable type unknown base", bv: []vd{{Name: "x", Required: tT, Type: ""}}, cv: []vd{{Name: "x", Required: tT, Type: "string"}}, silent: true},
		{name: "variable type unknown cur", bv: []vd{{Name: "x", Required: tT, Type: "string"}}, cv: []vd{{Name: "x", Required: tT, Type: ""}}, silent: true},
		{name: "variable type equal", bv: []vd{{Name: "x", Required: tT, Type: "string"}}, cv: []vd{{Name: "x", Required: tT, Type: "string"}}, silent: true},
		{name: "output type newly declared", bo: []od{{Name: "o", Type: "", Sensitive: tF}}, co: []od{{Name: "o", Type: "string", Sensitive: tF}}, silent: true},
		{name: "output type dropped", bo: []od{{Name: "o", Type: "string", Sensitive: tF}}, co: []od{{Name: "o", Type: "", Sensitive: tF}}, silent: true},
		{name: "output type change", bo: []od{{Name: "o", Type: "string", Sensitive: tF}}, co: []od{{Name: "o", Type: "number", Sensitive: tF}},
			want: []impact.TypeChange{tc(impact.OutputType, "o", "string", "number")}},
		{name: "sensitive false to true", bo: []od{{Name: "o", Sensitive: tF}}, co: []od{{Name: "o", Sensitive: tT}},
			want: []impact.TypeChange{tc(impact.OutputSensitive, "o", "false", "true")}},
		{name: "sensitive true to false", bo: []od{{Name: "o", Sensitive: tT}}, co: []od{{Name: "o", Sensitive: tF}},
			want: []impact.TypeChange{tc(impact.OutputSensitive, "o", "true", "false")}},
		{name: "sensitive unknown base", bo: []od{{Name: "o", Sensitive: tU}}, co: []od{{Name: "o", Sensitive: tT}}, silent: true},
		{name: "sensitive unknown cur", bo: []od{{Name: "o", Sensitive: tF}}, co: []od{{Name: "o", Sensitive: tU}}, silent: true},
		{name: "removed variable", bv: []vd{{Name: "x", Required: tT, Type: "string"}}, names: true},
		{name: "removed output", bo: []od{{Name: "o", Type: "string", Sensitive: tT}}, names: true},
		{name: "new output", co: []od{{Name: "o", Type: "string", Sensitive: tT}}, names: true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := diffFacts(t, c.bv, c.bo, c.cv, c.co)
			if c.silent {
				if len(got) != 0 {
					t.Fatalf("SurfaceDiff = %+v, want none", got)
				}
				return
			}
			if len(got) != 1 {
				t.Fatalf("SurfaceDiff = %+v, want one change", got)
			}
			ch := got[0]
			if ch.TypeChanges == nil {
				t.Fatal("TypeChanges must be non-nil")
			}
			want := c.want
			if want == nil {
				want = []impact.TypeChange{}
			}
			if !reflect.DeepEqual(ch.TypeChanges, want) {
				t.Fatalf("TypeChanges = %+v, want %+v", ch.TypeChanges, want)
			}
			hasNames := len(ch.AddedVariables)+len(ch.RemovedVariables)+len(ch.AddedOutputs)+len(ch.RemovedOutputs) > 0
			if hasNames != c.names {
				t.Fatalf("name change = %v, want %v (%+v)", hasNames, c.names, ch)
			}
		})
	}
}

func TestTypeChangeOrder(t *testing.T) {
	bv := []vd{{Name: "b", Required: tF, Type: "string"}, {Name: "a", Required: tT, Type: "string"}, {Name: "c", Required: tF, Type: "number"}}
	cv := []vd{{Name: "b", Required: tT, Type: "number"}, {Name: "a", Required: tT, Type: "bool"}, {Name: "c", Required: tT, Type: "number"}, {Name: "n", Required: tT}}
	bo := []od{{Name: "z", Type: "string", Sensitive: tF}, {Name: "y", Type: "string", Sensitive: tT}}
	co := []od{{Name: "z", Type: "number", Sensitive: tT}, {Name: "y", Type: "bool", Sensitive: tF}}
	want := []impact.TypeChange{
		tc(impact.VariableRequired, "b", "optional", "required"),
		tc(impact.VariableRequired, "c", "optional", "required"),
		tc(impact.VariableRequired, "n", "absent", "required"),
		tc(impact.VariableType, "a", "string", "bool"),
		tc(impact.VariableType, "b", "string", "number"),
		tc(impact.OutputType, "y", "string", "bool"),
		tc(impact.OutputType, "z", "string", "number"),
		tc(impact.OutputSensitive, "y", "true", "false"),
		tc(impact.OutputSensitive, "z", "false", "true"),
	}
	r := xorshift(1503)
	shuffle := func(n int, swap func(i, j int)) {
		for i := n - 1; i > 0; i-- {
			swap(i, r.intn(i+1))
		}
	}
	for i := range 50 {
		shuffle(len(bv), func(a, b int) { bv[a], bv[b] = bv[b], bv[a] })
		shuffle(len(cv), func(a, b int) { cv[a], cv[b] = cv[b], cv[a] })
		shuffle(len(bo), func(a, b int) { bo[a], bo[b] = bo[b], bo[a] })
		shuffle(len(co), func(a, b int) { co[a], co[b] = co[b], co[a] })
		got := diffFacts(t, bv, bo, cv, co)
		if len(got) != 1 || !reflect.DeepEqual(got[0].TypeChanges, want) {
			t.Fatalf("shuffle %d: %+v, want %+v", i, got, want)
		}
	}
}

func TestSurfaceDiffTypeOnlyModule(t *testing.T) {
	got := diffFacts(t,
		[]vd{{Name: "x", Required: tT, Type: "string"}}, nil,
		[]vd{{Name: "x", Required: tT, Type: "number"}}, nil)
	if len(got) != 1 || got[0].Empty() || len(got[0].TypeChanges) != 1 {
		t.Fatalf("SurfaceDiff = %+v, want one non-empty type-only change", got)
	}
	if (impact.SurfaceChange{TypeChanges: []impact.TypeChange{{}}}).Empty() {
		t.Fatal("a change holding only TypeChanges must not be Empty")
	}
}

// nameOnlyDiff is the pre-phase SurfaceDiff of one module pair, rebuilt
// here as the reference.
func nameOnlyDiff(bv, bo, cv, co []string) (av, rv, ao, ro []string) {
	minus := func(a, b []string) []string {
		out := []string{}
		for _, n := range a {
			if !slices.Contains(b, n) {
				out = append(out, n)
			}
		}
		slices.Sort(out)
		return out
	}
	return minus(cv, bv), minus(bv, cv), minus(co, bo), minus(bo, co)
}

func randNames(r *xorshift) []string {
	var out []string
	for _, n := range []string{"a", "b", "c", "d", "e"} {
		if r.intn(2) == 0 {
			out = append(out, n)
		}
	}
	return out
}

func TestSurfaceDiffNamesOnlyUnchanged(t *testing.T) {
	r := xorshift(15_03)
	changed := 0
	for i := range 2000 {
		bv, bo, cv, co := randNames(&r), randNames(&r), randNames(&r), randNames(&r)
		got := impact.SurfaceDiff(graph(t, nil, knownMod(t, "m", bv, bo)), graph(t, nil, knownMod(t, "m", cv, co)))
		av, rv, ao, ro := nameOnlyDiff(bv, bo, cv, co)
		empty := len(av)+len(rv)+len(ao)+len(ro) == 0
		if empty {
			if len(got) != 0 {
				t.Fatalf("case %d: %+v, want none", i, got)
			}
			continue
		}
		changed++
		want := impact.SurfaceChange{Module: p("m"), AddedVariables: av, RemovedVariables: rv, AddedOutputs: ao, RemovedOutputs: ro, TypeChanges: []impact.TypeChange{}}
		if len(got) != 1 || !reflect.DeepEqual(got[0], want) {
			t.Fatalf("case %d: %+v, want %+v", i, got, want)
		}
	}
	if changed < 100 {
		t.Fatalf("vacuous: only %d changed cases", changed)
	}
}

// --- Task 2: propagation and the Broken invariant -----------------------------

func TestComputeTypeOnlySeeds(t *testing.T) {
	units := []repograph.Unit{
		resolved(t, "a", "modules/m"),
		resolved(t, "b", "modules/app", dep(t, "b", "a", "a")),
	}
	baseG := graph(t, units, factsMod(t, "modules/m", []vd{{Name: "x", Required: tT, Type: "string"}}, nil), knownMod(t, "modules/app", nil, nil))
	curG := graph(t, units, factsMod(t, "modules/m", []vd{{Name: "x", Required: tT, Type: "number"}}, nil), knownMod(t, "modules/app", nil, nil))
	r := impact.Compute(baseG, diagnostic.NewSet(), curG, diagnostic.NewSet())
	if len(r.Changes) != 1 || r.Changes[0].Module != p("modules/m") {
		t.Fatalf("Changes = %+v", r.Changes)
	}
	if len(r.Impacted) != 2 {
		t.Fatalf("Impacted = %+v", r.Impacted)
	}
	a, b := r.Impacted[0], r.Impacted[1]
	if a.Unit != p("a") || a.Reach.Distance != 1 || !reflect.DeepEqual(a.Change.TypeChanges, []impact.TypeChange{tc(impact.VariableType, "x", "string", "number")}) {
		t.Fatalf("a = %+v", a)
	}
	if b.Unit != p("b") || b.Reach != (impact.Reach{Distance: 2, Source: p("a"), Via: p("a")}) {
		t.Fatalf("b = %+v", b)
	}
	if len(r.Broken) != 0 || r.HasErrors() {
		t.Fatalf("Broken = %+v", r.Broken)
	}
}

func TestTypeFactsNeverBroken(t *testing.T) {
	units := []repograph.Unit{
		resolved(t, "a", "modules/m"),
		resolved(t, "other", "modules/app"),
	}
	baseG := graph(t, units, factsMod(t, "modules/m", nil, nil), knownMod(t, "modules/app", nil, nil))
	// modules/m gains a required variable that unit a does not set.
	curG := graph(t, units, factsMod(t, "modules/m", []vd{{Name: "need", Required: tT, Type: "string"}}, nil), knownMod(t, "modules/app", nil, nil))

	r := impact.Compute(baseG, diagnostic.NewSet(), curG, diagnostic.NewSet())
	if len(r.Broken) != 0 || r.HasErrors() {
		t.Fatalf("Broken = %+v, HasErrors %v; a type-level fact must never break a unit", r.Broken, r.HasErrors())
	}
	if len(r.Impacted) != 1 || r.Impacted[0].Unit != p("a") {
		t.Fatalf("Impacted = %+v", r.Impacted)
	}

	grt := unitDiag(t, diagnostic.SeverityError, "other", 3, "unrelated GRT001")
	r = impact.Compute(baseG, diagnostic.NewSet(), curG, diagnostic.NewSet(grt))
	if len(r.Broken) != 1 || r.Broken[0].Subject != p("other") || !reflect.DeepEqual(r.Broken[0].Findings, []diagnostic.Diagnostic{grt}) || !r.HasErrors() {
		t.Fatalf("Broken = %+v", r.Broken)
	}
}

// randDecls draws a surface with random names and random facts.
func randDecls(r *xorshift) ([]vd, []od) {
	tris := []repograph.Tristate{tT, tF, tU}
	types := []string{"", "string", "number", "list(string)"}
	var vs []vd
	for _, n := range randNames(r) {
		vs = append(vs, vd{Name: n, Required: tris[r.intn(3)], Type: types[r.intn(len(types))]})
	}
	var os []od
	for _, n := range randNames(r) {
		os = append(os, od{Name: n, Type: types[r.intn(len(types))], Sensitive: tris[r.intn(3)]})
	}
	return vs, os
}

// mutateDecls copies a surface and changes some names and facts.
func mutateDecls(r *xorshift, vs []vd, os []od) ([]vd, []od) {
	if r.intn(4) == 0 {
		return randDecls(r)
	}
	tris := []repograph.Tristate{tT, tF, tU}
	types := []string{"", "string", "number", "list(string)"}
	cv := slices.Clone(vs)
	co := slices.Clone(os)
	for i := range cv {
		if r.intn(3) == 0 {
			cv[i].Required = tris[r.intn(3)]
		}
		if r.intn(3) == 0 {
			cv[i].Type = types[r.intn(len(types))]
		}
	}
	for i := range co {
		if r.intn(3) == 0 {
			co[i].Type = types[r.intn(len(types))]
		}
		if r.intn(3) == 0 {
			co[i].Sensitive = tris[r.intn(3)]
		}
	}
	return cv, co
}

func namesOf[T any](ds []T, name func(T) string) []string {
	var out []string
	for _, d := range ds {
		out = append(out, name(d))
	}
	return out
}

func brokenCore(bs []impact.BrokenUnit) []impact.BrokenUnit {
	out := make([]impact.BrokenUnit, len(bs))
	for i, b := range bs {
		out[i] = impact.BrokenUnit{Subject: b.Subject, Findings: b.Findings}
	}
	return out
}

// TestTypeFactsNeverChangeBrokenProperty (sec #247): the same graph pairs
// computed with facts and with names only (NewSurface: every fact unknown).
func TestTypeFactsNeverChangeBrokenProperty(t *testing.T) {
	r := xorshift(0x247)
	added, shortened, typeOnlySeedCases, equalCases := 0, 0, 0, 0
	for i := range 2000 {
		nMods := 1 + r.intn(4)
		nUnits := 2 + r.intn(7)
		type modDecls struct {
			bv, cv []vd
			bo, co []od
		}
		mods := make([]modDecls, nMods)
		for m := range mods {
			mods[m].bv, mods[m].bo = randDecls(&r)
			mods[m].cv, mods[m].co = mutateDecls(&r, mods[m].bv, mods[m].bo)
		}
		var units []repograph.Unit
		for u := range nUnits {
			name := "u" + strconv.Itoa(u)
			if r.intn(8) == 0 {
				units = append(units, configUnknown(t, name))
				continue
			}
			var deps []repograph.Dependency
			for j := range u {
				if r.intn(3) == 0 {
					deps = append(deps, dep(t, name, "d"+strconv.Itoa(j), "u"+strconv.Itoa(j)))
				}
			}
			units = append(units, resolved(t, name, "modules/m"+strconv.Itoa(r.intn(nMods)), deps...))
		}
		var baseDiags, curDiags []diagnostic.Diagnostic
		for u := range nUnits {
			sev := diagnostic.SeverityError
			if r.intn(2) == 0 {
				sev = diagnostic.SeverityWarning
			}
			d := unitDiag(t, sev, "u"+strconv.Itoa(u), 1+r.intn(3), "m")
			if r.intn(4) == 0 {
				baseDiags = append(baseDiags, d)
			}
			if r.intn(4) == 0 {
				curDiags = append(curDiags, d)
			}
		}

		build := func(facts, cur bool) *repograph.RepositoryGraph {
			var ms []repograph.Module
			for m, d := range mods {
				vs, os := d.bv, d.bo
				if cur {
					vs, os = d.cv, d.co
				}
				path := "modules/m" + strconv.Itoa(m)
				if facts {
					ms = append(ms, factsMod(t, path, vs, os))
				} else {
					ms = append(ms, knownMod(t, path,
						namesOf(vs, func(v vd) string { return v.Name }),
						namesOf(os, func(o od) string { return o.Name })))
				}
			}
			return graph(t, units, ms...)
		}
		bd, cd := diagnostic.NewSet(baseDiags...), diagnostic.NewSet(curDiags...)
		with := impact.Compute(build(true, false), bd, build(true, true), cd)
		names := impact.Compute(build(false, false), bd, build(false, true), cd)

		if !reflect.DeepEqual(brokenCore(with.Broken), brokenCore(names.Broken)) || with.HasErrors() != names.HasErrors() {
			t.Fatalf("case %d: Broken/HasErrors differ:\nwith  %+v\nnames %+v", i, with.Broken, names.Broken)
		}
		wd := map[repograph.RepoPath]impact.Reach{}
		for _, iu := range with.Impacted {
			wd[iu.Unit] = iu.Reach
		}
		short := false
		for _, iu := range names.Impacted {
			re, ok := wd[iu.Unit]
			if !ok {
				t.Fatalf("case %d: %s Impacted by names only, not with facts", i, iu.Unit)
			}
			if re.Distance > iu.Reach.Distance {
				t.Fatalf("case %d: %s distance %d with facts > %d names only", i, iu.Unit, re.Distance, iu.Reach.Distance)
			}
			if re.Distance < iu.Reach.Distance {
				short = true
			}
		}
		nameSeeds := map[repograph.RepoPath]bool{}
		for _, c := range names.Changes {
			nameSeeds[c.Module] = true
		}
		typeOnly := false
		for _, c := range with.Changes {
			if !nameSeeds[c.Module] {
				typeOnly = true
			}
		}
		if typeOnly {
			typeOnlySeedCases++
		} else {
			equalCases++
			reaches := func(res impact.Result) ([]impact.ImpactedUnit, []impact.Reach) {
				var ius []impact.ImpactedUnit
				for _, iu := range res.Impacted {
					ius = append(ius, impact.ImpactedUnit{Unit: iu.Unit, Reach: iu.Reach})
				}
				var brs []impact.Reach
				for _, b := range res.Broken {
					brs = append(brs, b.Reach)
				}
				return ius, brs
			}
			wi, wb := reaches(with)
			ni, nb := reaches(names)
			if !reflect.DeepEqual(wi, ni) || !reflect.DeepEqual(wb, nb) {
				t.Fatalf("case %d: no type-only seed, yet Reach differs:\nwith  %+v %+v\nnames %+v %+v", i, wi, wb, ni, nb)
			}
		}
		if len(with.Impacted) > len(names.Impacted) {
			added++
		}
		if short {
			shortened++
		}
	}
	t.Logf("facts added Impacted units in %d cases, shortened a distance in %d; %d type-only-seed cases, %d equal-seed cases", added, shortened, typeOnlySeedCases, equalCases)
	if added == 0 || shortened == 0 || equalCases == 0 {
		t.Fatalf("vacuous property: added %d, shortened %d, equal-seed cases %d", added, shortened, equalCases)
	}
}
