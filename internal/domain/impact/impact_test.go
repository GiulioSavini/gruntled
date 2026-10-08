package impact_test

import (
	"strconv"
	"testing"

	"github.com/GiulioSavini/gruntled/internal/domain/diagnostic"
	"github.com/GiulioSavini/gruntled/internal/domain/impact"
	"github.com/GiulioSavini/gruntled/internal/domain/repograph"
)

func pos(t tb, file string, line int) repograph.Position {
	t.Helper()
	ps, err := repograph.NewPosition(p(file), line, 1)
	if err != nil {
		t.Fatalf("NewPosition: %v", err)
	}
	return ps
}

func unitDiag(t tb, sev diagnostic.Severity, unit string, line int, msg string) diagnostic.Diagnostic {
	t.Helper()
	d, err := diagnostic.NewForUnit(diagnostic.CodeUnknownOutput, sev, p(unit), pos(t, unit+"/terragrunt.hcl", line), msg)
	if err != nil {
		t.Fatalf("NewForUnit: %v", err)
	}
	return d
}

func fileDiag(t tb, file string, line int, msg string) diagnostic.Diagnostic {
	t.Helper()
	d, err := diagnostic.New(diagnostic.CodeSyntaxError, diagnostic.SeverityError, pos(t, file, line), msg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return d
}

func resolved(t tb, path, module string, deps ...repograph.Dependency) repograph.Unit {
	t.Helper()
	u, err := repograph.NewResolvedUnit(p(path), p(module), deps, nil)
	if err != nil {
		t.Fatalf("NewResolvedUnit: %v", err)
	}
	return u
}

func configUnknown(t tb, path string) repograph.Unit {
	t.Helper()
	u, err := repograph.NewConfigUnknownUnit(p(path), "broken")
	if err != nil {
		t.Fatalf("NewConfigUnknownUnit: %v", err)
	}
	return u
}

func dep(t tb, from, name, target string) repograph.Dependency {
	t.Helper()
	ps := pos(t, from+"/terragrunt.hcl", 1)
	d, err := repograph.NewDependency(name, p(target), ps, ps, repograph.TargetHasConfig, repograph.DefaultDependencyOptions())
	if err != nil {
		t.Fatalf("NewDependency: %v", err)
	}
	return d
}

func TestNewFindings(t *testing.T) {
	moved := unitDiag(t, diagnostic.SeverityError, "live/a", 3, "m1")
	movedCur := unitDiag(t, diagnostic.SeverityError, "live/a", 9, "m1")
	fixed := unitDiag(t, diagnostic.SeverityError, "live/b", 1, "fixed")
	sevBase := unitDiag(t, diagnostic.SeverityWarning, "live/c", 1, "sev")
	sevCur := unitDiag(t, diagnostic.SeverityError, "live/c", 1, "sev")
	fresh := unitDiag(t, diagnostic.SeverityError, "live/d", 1, "new")

	base := diagnostic.NewSet(moved, fixed, sevBase)
	cur := diagnostic.NewSet(movedCur, sevCur, fresh)
	got := impact.NewFindings(base, cur)
	if len(got) != 1 || got[0] != fresh {
		t.Fatalf("want only fresh, got %v", got)
	}

	same := impact.NewFindings(cur, cur)
	if same == nil || len(same) != 0 {
		t.Fatalf("identical: want empty non-nil, got %#v", same)
	}
	if impact.KeyOf(moved) != impact.KeyOf(movedCur) {
		t.Fatal("KeyOf must ignore line")
	}
}

func TestCompute(t *testing.T) {
	baseG := graph(t,
		[]repograph.Unit{
			resolved(t, "live/app", "modules/vpc"),
			resolved(t, "live/db", "modules/vpc"),
			resolved(t, "live/web", "modules/web", dep(t, "live/web", "app", "live/app")),
			configUnknown(t, "live/odd"),
		},
		knownMod(t, "modules/vpc", []string{"cidr"}, []string{"id"}),
		knownMod(t, "modules/web", nil, nil),
	)
	curG := graph(t,
		[]repograph.Unit{
			resolved(t, "live/app", "modules/vpc"),
			resolved(t, "live/db", "modules/vpc"),
			resolved(t, "live/web", "modules/web", dep(t, "live/web", "app", "live/app")),
			configUnknown(t, "live/odd"),
		},
		knownMod(t, "modules/vpc", []string{"cidr"}, []string{"vpc_id"}),
		knownMod(t, "modules/web", nil, nil),
	)
	pre := unitDiag(t, diagnostic.SeverityError, "live/db", 4, "pre-existing")
	preMoved := unitDiag(t, diagnostic.SeverityError, "live/db", 7, "pre-existing")
	brokenDB := unitDiag(t, diagnostic.SeverityError, "live/db", 2, "id gone")
	syntax := fileDiag(t, "live/root.hcl", 5, "Unclosed configuration block")

	r := impact.Compute(baseG, diagnostic.NewSet(pre), curG, diagnostic.NewSet(preMoved, brokenDB, syntax))
	if !r.Baseline {
		t.Fatal("Compute must set Baseline")
	}
	if len(r.Broken) != 2 || r.Broken[0].Subject != p("live/db") || r.Broken[1].Subject != p("live/root.hcl") {
		t.Fatalf("Broken = %+v", r.Broken)
	}
	if len(r.Broken[0].Findings) != 1 || r.Broken[0].Findings[0] != brokenDB {
		t.Fatalf("live/db findings = %v", r.Broken[0].Findings)
	}
	// live/db is Broken so removed from Impacted; live/web (consumer of a
	// consumer) and live/odd (config-unknown) are never Impacted.
	if len(r.Impacted) != 1 || r.Impacted[0].Unit != p("live/app") {
		t.Fatalf("Impacted = %+v", r.Impacted)
	}
	if r.Impacted[0].Change.Module != p("modules/vpc") {
		t.Fatalf("Change = %+v", r.Impacted[0].Change)
	}
	if !r.HasErrors() {
		t.Fatal("HasErrors must be true")
	}

	same := impact.Compute(curG, diagnostic.NewSet(brokenDB), curG, diagnostic.NewSet(brokenDB))
	if same.Broken == nil || same.Impacted == nil || len(same.Broken) != 0 || len(same.Impacted) != 0 {
		t.Fatalf("identical trees: want empty non-nil, got %#v", same)
	}
	if same.HasErrors() {
		t.Fatal("identical trees must not HasErrors")
	}
}

func TestHasErrorsWarningOnly(t *testing.T) {
	w := unitDiag(t, diagnostic.SeverityWarning, "live/a", 1, "w")
	if impact.NoBaseline(diagnostic.NewSet(w)).HasErrors() {
		t.Fatal("warning-only must not HasErrors")
	}
}

func TestNoBaseline(t *testing.T) {
	a1 := unitDiag(t, diagnostic.SeverityError, "live/a", 1, "x")
	a2 := unitDiag(t, diagnostic.SeverityWarning, "live/a", 2, "y")
	f := fileDiag(t, "root.hcl", 1, "bad")
	r := impact.NoBaseline(diagnostic.NewSet(f, a2, a1))
	if r.Baseline {
		t.Fatal("Baseline must be false")
	}
	if r.Impacted == nil || len(r.Impacted) != 0 {
		t.Fatalf("Impacted = %#v", r.Impacted)
	}
	if len(r.Broken) != 2 || r.Broken[0].Subject != p("live/a") || len(r.Broken[0].Findings) != 2 || r.Broken[1].Subject != p("root.hcl") {
		t.Fatalf("Broken = %+v", r.Broken)
	}
	if empty := impact.NoBaseline(diagnostic.NewSet()); empty.Broken == nil || len(empty.Broken) != 0 {
		t.Fatalf("empty: %#v", empty.Broken)
	}
}

// lcg is a tiny deterministic generator: the domain allowlist forbids
// math/rand and rapid even in tests, so the property loop below drives its
// choices from a fixed-seed linear congruential sequence instead.
type lcg uint64

func (g *lcg) intn(n int) int {
	*g = *g*6364136223846793005 + 1442695040888963407
	return int((uint64(*g) >> 33) % uint64(n))
}

// TestDisjoint checks the Result invariants on many generated trees: Broken
// subjects and Impacted units strictly sorted, hence unique, and disjoint.
func TestDisjoint(t *testing.T) {
	names := []string{"a", "b", "c"}
	for seed := range 2000 {
		g := lcg(seed)
		itoa := strconv.Itoa
		nUnits := 1 + g.intn(6)
		nMods := 1 + g.intn(3)
		pick := func() []string {
			var out []string
			for _, n := range names {
				if g.intn(2) == 1 {
					out = append(out, n)
				}
			}
			return out
		}
		mkMods := func() []repograph.Module {
			ms := make([]repograph.Module, nMods)
			for i := range ms {
				ms[i] = knownMod(t, "modules/m"+itoa(i), pick(), pick())
			}
			return ms
		}
		units := make([]repograph.Unit, nUnits)
		for i := range units {
			units[i] = resolved(t, "live/u"+itoa(i), "modules/m"+itoa(g.intn(nMods)))
		}
		mkDiags := func() diagnostic.Set {
			ds := make([]diagnostic.Diagnostic, g.intn(7))
			for i := range ds {
				u := "live/u" + itoa(g.intn(nUnits))
				line := 1 + g.intn(5)
				msg := names[g.intn(len(names))]
				if g.intn(2) == 1 {
					ds[i] = fileDiag(t, u+"/terragrunt.hcl", line, msg)
				} else {
					ds[i] = unitDiag(t, diagnostic.SeverityError, u, line, msg)
				}
			}
			return diagnostic.NewSet(ds...)
		}
		baseG := graph(t, units, mkMods()...)
		curG := graph(t, units, mkMods()...)
		r := impact.Compute(baseG, mkDiags(), curG, mkDiags())

		broken := map[repograph.RepoPath]bool{}
		for i, b := range r.Broken {
			if i > 0 && r.Broken[i-1].Subject.Compare(b.Subject) >= 0 {
				t.Fatalf("seed %d: Broken not strictly sorted: %v", seed, r.Broken)
			}
			broken[b.Subject] = true
		}
		for i, u := range r.Impacted {
			if i > 0 && r.Impacted[i-1].Unit.Compare(u.Unit) >= 0 {
				t.Fatalf("seed %d: Impacted not strictly sorted: %v", seed, r.Impacted)
			}
			if broken[u.Unit] {
				t.Fatalf("seed %d: %s both Broken and Impacted", seed, u.Unit)
			}
			if u.Change.Empty() {
				t.Fatalf("seed %d: %s Impacted by empty change", seed, u.Unit)
			}
		}
	}
}
