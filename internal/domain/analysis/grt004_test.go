package analysis_test

import (
	"math/rand/v2"
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
	rng := rand.New(rand.NewPCG(13, 1))
	for range 2 {
		for range 10 {
			got, err := analysis.RemovedOutputs(removedFixture(t, []string{"a", "b"}, rng.Perm), removedFixture(t, []string{"b"}, rng.Perm))
			if err != nil {
				t.Fatalf("RemovedOutputs: %v", err)
			}
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("result depends on input order:\n got %v\nwant %v", got, want)
			}
		}
	}
}
