package analysis_test

import (
	"reflect"
	"strings"
	"testing"

	"github.com/GiulioSavini/gruntled/internal/domain/analysis"
	"github.com/GiulioSavini/gruntled/internal/domain/diagnostic"
	"github.com/GiulioSavini/gruntled/internal/domain/repograph"
)

// TestRemovedOutputsDIAG03Parity: with a baseline that references vpc_idd
// and whose live/vpc module declares it, GRT004 fires exactly where GRT001
// fires for every DIAG-03 row, at the same site, with the same suffix
// decision.
func TestRemovedOutputsDIAG03Parity(t *testing.T) {
	for _, r := range diag03Rows() {
		t.Run(r.name, func(t *testing.T) {
			base := buildScenario(t, scenario{opts: withOpts(nil), outputs: []string{"vpc_id", "vpc_idd"}})
			cur := buildScenario(t, r.build(t))
			removed, err := analysis.RemovedOutputs(base, cur)
			if err != nil {
				t.Fatalf("RemovedOutputs: %v", err)
			}
			unknown, err := analysis.UnknownOutputs(cur)
			if err != nil {
				t.Fatalf("UnknownOutputs: %v", err)
			}
			if len(unknown) != r.want {
				t.Fatalf("UnknownOutputs = %d, want %d (DIAG-03 row drifted)", len(unknown), r.want)
			}
			if len(removed) != len(unknown) {
				t.Fatalf("RemovedOutputs = %d diagnostics, UnknownOutputs = %d: %v", len(removed), len(unknown), removed)
			}
			for i, d := range removed {
				g := unknown[i]
				if d.Code() != diagnostic.CodeRemovedOutput {
					t.Errorf("code = %v, want GRT004", d.Code())
				}
				if d.Severity() != diagnostic.SeverityError {
					t.Errorf("severity = %v, want error", d.Severity())
				}
				du, _ := d.Unit()
				gu, _ := g.Unit()
				if du != gu || d.Pos() != g.Pos() {
					t.Errorf("GRT004 at %v %v, GRT001 at %v %v", du, d.Pos(), gu, g.Pos())
				}
				if strings.HasSuffix(d.Message(), maskSuffix) != strings.HasSuffix(g.Message(), maskSuffix) {
					t.Errorf("suffix differs:\n  GRT004 %s\n  GRT001 %s", d.Message(), g.Message())
				}
				if strings.HasSuffix(d.Message(), maskSuffix) != r.suffix {
					t.Errorf("suffix = %v, want %v", !r.suffix, r.suffix)
				}
			}
		})
	}
}

// removedFixture builds a tree of n units live/uNN, each with dependency
// "vpc" -> live/vpc referencing outputs a and b, plus a shared include
// read by every unit; units and modules are passed in perm order.
func removedFixture(t *testing.T, vpcOutputs []string, perm func(n int) []int) *repograph.RepositoryGraph {
	t.Helper()
	const n = 6
	vpc := repograph.MustRepoPath("live/vpc")
	vpcMod := repograph.MustRepoPath("modules/vpc")
	shared := repograph.MustRepoPath("live/_common/vpc.hcl")
	var units []repograph.Unit
	for i := range n {
		p := repograph.MustRepoPath("live/u" + string(rune('0'+i)))
		file := repograph.MustRepoPath(p.String() + "/terragrunt.hcl")
		dep, err := repograph.NewDependency("vpc", vpc, mustPos(t, file, 1, 1), mustPos(t, file, 1, 1), repograph.TargetUnknown, repograph.DefaultDependencyOptions())
		if err != nil {
			t.Fatalf("NewDependency: %v", err)
		}
		refs := []repograph.Reference{
			mustRef(t, "vpc", "a", mustPos(t, file, 4, 3)),
			mustRef(t, "vpc", "b", mustPos(t, file, 5, 3)),
			mustRef(t, "vpc", "a", mustPos(t, shared, 2, 7)),
		}
		units = append(units, mustResolvedUnit(t, p, appModule, []repograph.Dependency{dep}, refs))
	}
	units = append(units, mustResolvedUnit(t, vpc, vpcMod, nil, nil))
	modules := []repograph.Module{mustModule(t, appModule, mustSurface(t)), mustModule(t, vpcMod, mustSurface(t, vpcOutputs...))}

	shuffledUnits := make([]repograph.Unit, len(units))
	for i, j := range perm(len(units)) {
		shuffledUnits[i] = units[j]
	}
	shuffledMods := make([]repograph.Module, len(modules))
	for i, j := range perm(len(modules)) {
		shuffledMods[i] = modules[j]
	}
	return mustGraph(t, shuffledUnits, shuffledMods)
}

func TestRemovedOutputsDeterministic(t *testing.T) {
	identity := func(n int) []int {
		out := make([]int, n)
		for i := range out {
			out[i] = i
		}
		return out
	}
	want, err := analysis.RemovedOutputs(removedFixture(t, []string{"a", "b"}, identity), removedFixture(t, []string{"b"}, identity))
	if err != nil {
		t.Fatalf("RemovedOutputs: %v", err)
	}
	if len(want) != 12 {
		t.Fatalf("got %d GRT004, want 12 (6 units x 2 sites of a)", len(want))
	}
	// The domain may not use randomness (check-architecture), so the
	// shuffles are fixed: every rotation of the input, forwards and
	// reversed, each run twice.
	for k := range 8 {
		for _, rev := range []bool{false, true} {
			perm := func(n int) []int {
				out := make([]int, n)
				for i := range out {
					j := (i + k) % n
					if rev {
						j = n - 1 - j
					}
					out[i] = j
				}
				return out
			}
			for range 2 {
				got, err := analysis.RemovedOutputs(removedFixture(t, []string{"a", "b"}, perm), removedFixture(t, []string{"b"}, perm))
				if err != nil {
					t.Fatalf("RemovedOutputs: %v", err)
				}
				if !reflect.DeepEqual(got, want) {
					t.Fatalf("result depends on input order (rotation %d, reversed %v):\n got %v\nwant %v", k, rev, got, want)
				}
			}
		}
	}
}

func mustDiag(t *testing.T, code diagnostic.Code, unit string, pos repograph.Position, msg string) diagnostic.Diagnostic {
	t.Helper()
	var d diagnostic.Diagnostic
	var err error
	if unit == "" {
		d, err = diagnostic.New(code, diagnostic.SeverityError, pos, msg)
	} else {
		d, err = diagnostic.NewForUnit(code, diagnostic.SeverityError, repograph.MustRepoPath(unit), pos, msg)
	}
	if err != nil {
		t.Fatalf("diagnostic: %v", err)
	}
	return d
}

func TestSupersedeUnknownOutputs(t *testing.T) {
	tg := repograph.MustRepoPath("live/app/terragrunt.hcl")
	root := repograph.MustRepoPath("live/app/root.hcl")
	p7 := mustPos(t, tg, 7, 20)
	p9 := mustPos(t, tg, 9, 20)
	r7 := mustPos(t, root, 7, 20)
	g001 := func(unit string, p repograph.Position) diagnostic.Diagnostic {
		return mustDiag(t, diagnostic.CodeUnknownOutput, unit, p, "dependency \"vpc\" output \"id\" is not declared")
	}
	g004 := func(unit string, p repograph.Position, msg string) diagnostic.Diagnostic {
		return mustDiag(t, diagnostic.CodeRemovedOutput, unit, p, msg)
	}
	others := []diagnostic.Diagnostic{
		mustDiag(t, diagnostic.CodeMissingDependencyTarget, "live/app", p7, "missing target"),
		mustDiag(t, diagnostic.CodeDependencyCycle, "live/app", p7, "cycle"),
		mustDiag(t, diagnostic.CodeSyntaxError, "", p7, "syntax"),
	}
	type row struct {
		name    string
		cur     []diagnostic.Diagnostic
		removed []diagnostic.Diagnostic
		want    []diagnostic.Diagnostic // nil means Equal(cur)
	}
	rows := []row{
		{name: "one GRT001 replaced",
			cur:     []diagnostic.Diagnostic{g001("live/app", p7)},
			removed: []diagnostic.Diagnostic{g004("live/app", p7, "removed")},
			want:    []diagnostic.Diagnostic{g004("live/app", p7, "removed")}},
		{name: "only the GRT001 at the same Pos is replaced",
			cur:     []diagnostic.Diagnostic{g001("live/app", p7), g001("live/app", p9)},
			removed: []diagnostic.Diagnostic{g004("live/app", p9, "removed")},
			want:    []diagnostic.Diagnostic{g001("live/app", p7), g004("live/app", p9, "removed")}},
		{name: "message never matched; other unit kept",
			cur:     []diagnostic.Diagnostic{g001("live/app", p7), g001("live/other", p7)},
			removed: []diagnostic.Diagnostic{g004("live/app", p7, "zzz totally unrelated text")},
			want:    []diagnostic.Diagnostic{g004("live/app", p7, "zzz totally unrelated text"), g001("live/other", p7)}},
		{name: "other codes never replaced, unmatched GRT004 dropped",
			cur:     others,
			removed: []diagnostic.Diagnostic{g004("live/app", p7, "removed")}},
		{name: "removed nil", cur: []diagnostic.Diagnostic{g001("live/app", p7)}},
		{name: "removed empty", cur: []diagnostic.Diagnostic{g001("live/app", p7)}, removed: []diagnostic.Diagnostic{}},
		{name: "same line:col in another file of the unit",
			cur:     []diagnostic.Diagnostic{g001("live/app", p7)},
			removed: []diagnostic.Diagnostic{g004("live/app", r7, "removed")}},
	}
	for _, r := range rows {
		t.Run(r.name, func(t *testing.T) {
			cur := diagnostic.NewSet(r.cur...)
			got := analysis.SupersedeUnknownOutputs(cur, r.removed)
			want := cur
			if r.want != nil {
				want = diagnostic.NewSet(r.want...)
			}
			if !got.Equal(want) {
				t.Errorf("SupersedeUnknownOutputs =\n  %v\nwant\n  %v", got.All(), want.All())
			}
			if got.Len() != cur.Len() {
				t.Errorf("Len = %d, want %d", got.Len(), cur.Len())
			}
		})
	}
}

// xorshift is a fixed pseudo-random sequence: the domain's tests may not
// import math/rand or rapid (check-architecture domain-stdlib-allowlist),
// so the property below is driven by this deterministic generator.
type xorshift uint64

func (x *xorshift) intn(n int) int {
	*x ^= *x << 13
	*x ^= *x >> 7
	*x ^= *x << 17
	return int(uint64(*x) % uint64(n))
}

type siteID struct {
	unit    repograph.RepoPath
	hasUnit bool
	pos     repograph.Position
}

func siteOf(d diagnostic.Diagnostic) siteID {
	u, ok := d.Unit()
	return siteID{unit: u, hasUnit: ok, pos: d.Pos()}
}

// TestSupersedeNeverAddsProperty: for random cur sets (at most one GRT001
// per (Unit, Pos), the References invariant) and random GRT004 lists (at
// most one per (Unit, Pos)), Supersede keeps Len and the (Unit, Pos)
// multiset, keeps every non-GRT001, and puts a GRT004 exactly where a
// GRT001 was.
func TestSupersedeNeverAddsProperty(t *testing.T) {
	units := []string{"live/a", "live/b"}
	files := []repograph.RepoPath{repograph.MustRepoPath("live/a/terragrunt.hcl"), repograph.MustRepoPath("live/_common/vpc.hcl")}
	msgs := []string{"m1", "m2", "dependency \"vpc\" output \"id\""}
	var positions []repograph.Position
	for _, f := range files {
		for _, line := range []int{7, 9} {
			for _, col := range []int{1, 20} {
				positions = append(positions, mustPos(t, f, line, col))
			}
		}
	}
	rng := xorshift(0x9e3779b97f4a7c15)
	replacedRuns, droppedRuns := 0, 0
	const cases = 3000
	for range cases {
		var curDs, removed []diagnostic.Diagnostic
		grt001At := map[siteID]bool{}
		for _, u := range units {
			for _, p := range positions {
				id := siteID{unit: repograph.MustRepoPath(u), hasUnit: true, pos: p}
				if rng.intn(2) == 0 {
					curDs = append(curDs, mustDiag(t, diagnostic.CodeUnknownOutput, u, p, msgs[rng.intn(len(msgs))]))
					grt001At[id] = true
				}
				if rng.intn(4) == 0 {
					curDs = append(curDs, mustDiag(t, diagnostic.CodeMissingDependencyTarget, u, p, msgs[rng.intn(len(msgs))]))
				}
				if rng.intn(3) == 0 {
					removed = append(removed, mustDiag(t, diagnostic.CodeRemovedOutput, u, p, msgs[rng.intn(len(msgs))]))
				}
			}
		}
		for _, p := range positions {
			if rng.intn(4) == 0 {
				curDs = append(curDs, mustDiag(t, diagnostic.CodeSyntaxError, "", p, msgs[rng.intn(len(msgs))]))
			}
		}
		// Shuffle removed so its order is not the generation order.
		for i := len(removed) - 1; i > 0; i-- {
			j := rng.intn(i + 1)
			removed[i], removed[j] = removed[j], removed[i]
		}

		cur := diagnostic.NewSet(curDs...)
		got := analysis.SupersedeUnknownOutputs(cur, removed)

		if got.Len() != cur.Len() {
			t.Fatalf("Len = %d, want %d", got.Len(), cur.Len())
		}
		count := func(s diagnostic.Set) map[siteID]int {
			m := map[siteID]int{}
			for _, d := range s.All() {
				m[siteOf(d)]++
			}
			return m
		}
		if !reflect.DeepEqual(count(got), count(cur)) {
			t.Fatalf("(Unit, Pos) multiset changed:\n got %v\n cur %v", got.All(), cur.All())
		}
		gotKeys := map[diagnostic.Key]bool{}
		for _, d := range got.All() {
			gotKeys[d.Key()] = true
		}
		for _, d := range cur.All() {
			if d.Code() != diagnostic.CodeUnknownOutput && !gotKeys[d.Key()] {
				t.Fatalf("non-GRT001 %v lost", d)
			}
		}
		wantReplaced, dropped := 0, 0
		for _, d := range removed {
			if grt001At[siteOf(d)] {
				wantReplaced++
			} else {
				dropped++
			}
		}
		gotReplaced := 0
		for _, d := range got.All() {
			if d.Code() != diagnostic.CodeRemovedOutput {
				continue
			}
			gotReplaced++
			if !grt001At[siteOf(d)] {
				t.Fatalf("GRT004 %v sits where cur has no GRT001", d)
			}
		}
		if gotReplaced != wantReplaced {
			t.Fatalf("GRT004 in result = %d, want %d", gotReplaced, wantReplaced)
		}
		if wantReplaced > 0 {
			replacedRuns++
		}
		if dropped > 0 {
			droppedRuns++
		}
	}
	if replacedRuns == 0 || droppedRuns == 0 {
		t.Fatalf("vacuous: %d runs with a replacement, %d with a dropped GRT004 (of %d)", replacedRuns, droppedRuns, cases)
	}
	t.Logf("%d cases: %d with >=1 replacement, %d with >=1 dropped GRT004", cases, replacedRuns, droppedRuns)
}
