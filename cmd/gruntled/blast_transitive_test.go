package main

import (
	"encoding/json"
	"path"
	"regexp"
	"slices"
	"testing"

	"github.com/GiulioSavini/gruntled/internal/testsupport/synthrepo"
)

var configPathLine = regexp.MustCompile(`(?m)^  config_path = "([^"]+)"$`)

// TestBlastTransitiveSynthrepo: on a synthrepo tree (each unit its own
// module, every dependency a default block) where one unit's module loses
// an output nobody reads, Impacted is exactly the set of units that reach
// that unit through reverse dependency edges, at their BFS distances, computed
// here from the rendered config_path lines, not from gruntled.
func TestBlastTransitiveSynthrepo(t *testing.T) {
	tree, _, err := synthrepo.Render(synthrepo.Spec{Units: 300, IncludeDepth: 2, DependencyFanout: 3, Seed: 14})
	if err != nil {
		t.Fatal(err)
	}
	// The generator's own dependency list: unit dir -> dependency dirs.
	deps := map[string][]string{}
	var units []string
	for _, f := range tree {
		if path.Base(f.Path) != "terragrunt.hcl" {
			continue
		}
		dir := path.Dir(f.Path)
		units = append(units, dir)
		for _, m := range configPathLine.FindAllStringSubmatch(string(f.Content), -1) {
			deps[dir] = append(deps[dir], path.Join(dir, m[1]))
		}
	}
	slices.Sort(units)
	dependents := map[string][]string{}
	for u, ds := range deps {
		for _, d := range ds {
			dependents[d] = append(dependents[d], u)
		}
	}
	// cone is the BFS over reverse edges from one unit.
	cone := func(from string) map[string]int {
		dist := map[string]int{from: 1}
		for frontier := []string{from}; len(frontier) > 0; {
			var next []string
			for _, u := range frontier {
				for _, d := range dependents[u] {
					if _, ok := dist[d]; !ok {
						dist[d] = dist[u] + 1
						next = append(next, d)
					}
				}
			}
			frontier = next
		}
		return dist
	}
	// The seed is the unit whose cone is closest to half the tree, so the
	// exact-set comparison below can fail in both directions.
	var seed string
	var dist map[string]int
	for _, u := range units {
		if c := cone(u); dist == nil || abs(len(c)-150) < abs(len(dist)-150) {
			seed, dist = u, c
		}
	}
	if len(dist) < 20 || len(dist) > 280 {
		t.Fatalf("%d units reach %s: the cone must be a strict, non-trivial subset", len(dist), seed)
	}

	cur, base := shortBase(t), shortBase(t)
	writeTree(t, tree, cur)
	writeTree(t, tree, base)
	// base declares one more output on the seed, which nothing reads.
	writeFiles(t, base, map[string]string{seed + "/zz_extra.tf": "output \"never_read\" {\n  value = \"x\"\n}\n"})

	out, stderr, code := runCLI(t, "blast", "--base", base, "--format", "json", cur)
	if code != exitOK {
		t.Fatalf("blast: exit %d, stderr:\n%s", code, stderr)
	}
	var doc struct {
		Broken   []json.RawMessage `json:"broken"`
		Impacted []struct {
			Unit     string `json:"unit"`
			Distance int    `json:"distance"`
			Source   string `json:"source"`
			Via      string `json:"via"`
		} `json:"impacted"`
	}
	if err := json.Unmarshal([]byte(out), &doc); err != nil {
		t.Fatal(err)
	}
	if len(doc.Broken) != 0 {
		t.Fatalf("Broken = %d entries, want 0", len(doc.Broken))
	}
	if len(doc.Impacted) != len(dist) {
		t.Fatalf("Impacted %d units, reverse reachability %d", len(doc.Impacted), len(dist))
	}
	byUnit := map[string]int{}
	for _, e := range doc.Impacted {
		byUnit[e.Unit] = e.Distance
	}
	far := 0
	for _, e := range doc.Impacted {
		want, ok := dist[e.Unit]
		if !ok || e.Distance != want {
			t.Fatalf("%s at distance %d, BFS says %d (reached %v)", e.Unit, e.Distance, want, ok)
		}
		if e.Source != seed {
			t.Fatalf("%s source %s, want %s", e.Unit, e.Source, seed)
		}
		if e.Distance == 1 {
			continue
		}
		far = max(far, e.Distance)
		// The path is valid edge by edge: via is a dependency of the unit,
		// one level closer.
		if !slices.Contains(deps[e.Unit], e.Via) || byUnit[e.Via] != e.Distance-1 {
			t.Fatalf("%s via %s: not a dependency at distance %d", e.Unit, e.Via, e.Distance-1)
		}
	}
	if far < 3 {
		t.Fatalf("deepest distance %d: the tree is too shallow to test paths", far)
	}

	text, _, _ := runCLI(t, "blast", "--base", base, cur)
	// A line is at most "  <unit> (distance N, from module <m>, path " plus 6
	// hops of at most ~20 bytes: 300 bytes is a generous fixed bound.
	if len(text) > 300*len(dist)+200 {
		t.Errorf("text is %d bytes for %d units: over the per-line bound", len(text), len(dist))
	}
	t.Logf("%d of 300 units reach %s, deepest distance %d, text %d bytes", len(dist), seed, far, len(text))
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}
