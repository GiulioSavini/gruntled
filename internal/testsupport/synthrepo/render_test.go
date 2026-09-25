package synthrepo

import (
	"reflect"
	"strings"
	"testing"
)

func TestSpecValidate(t *testing.T) {
	base := Spec{Units: 3, IncludeDepth: 2, DependencyFanout: 1, Seed: 1}

	cases := []struct {
		name string
		spec Spec
	}{
		{"units-zero", Spec{Units: 0, IncludeDepth: 2, DependencyFanout: 1}},
		{"units-negative", Spec{Units: -1, IncludeDepth: 2, DependencyFanout: 1}},
		{"include-depth-zero", Spec{Units: 3, IncludeDepth: 0, DependencyFanout: 1}},
		{"include-depth-negative", Spec{Units: 3, IncludeDepth: -1, DependencyFanout: 1}},
		{"fanout-negative", Spec{Units: 3, IncludeDepth: 2, DependencyFanout: -1}},
		{"unknown-error-kind", Spec{Units: 3, IncludeDepth: 2, DependencyFanout: 1, Errors: []ErrorKind{ErrorKind(99)}}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if err := c.spec.Validate(); err == nil {
				t.Fatalf("Validate() on %+v: expected error, got nil", c.spec)
			}
		})
	}

	if err := base.Validate(); err != nil {
		t.Fatalf("Validate() on well-formed spec: unexpected error: %v", err)
	}
}

func TestRender_TooManyBadOutputRefs(t *testing.T) {
	spec := Spec{Units: 1, IncludeDepth: 2, DependencyFanout: 3, Seed: 1, Errors: []ErrorKind{BadOutputRef}}
	_, _, err := Render(spec)
	if err == nil {
		t.Fatalf("Render() with Units=1 and one BadOutputRef: expected error (no references exist), got nil")
	}
}

func TestRender_Deterministic(t *testing.T) {
	spec := Spec{Units: 8, IncludeDepth: 3, DependencyFanout: 2, Seed: 123, Errors: []ErrorKind{BadOutputRef, BadOutputRef}}

	tree1, manifest1, err := Render(spec)
	if err != nil {
		t.Fatalf("Render() first call: unexpected error: %v", err)
	}
	tree2, manifest2, err := Render(spec)
	if err != nil {
		t.Fatalf("Render() second call: unexpected error: %v", err)
	}

	if !reflect.DeepEqual(tree1, tree2) {
		t.Fatalf("Render() gave different Tree across two calls with the same Spec")
	}
	if !reflect.DeepEqual(manifest1, manifest2) {
		t.Fatalf("Render() gave different Manifest across two calls with the same Spec")
	}
}

func TestRender_DifferentSeedsDifferentDigests(t *testing.T) {
	base := Spec{Units: 6, IncludeDepth: 2, DependencyFanout: 2}

	spec1 := base
	spec1.Seed = 1
	spec2 := base
	spec2.Seed = 2

	tree1, _, err := Render(spec1)
	if err != nil {
		t.Fatalf("Render() seed 1: unexpected error: %v", err)
	}
	tree2, _, err := Render(spec2)
	if err != nil {
		t.Fatalf("Render() seed 2: unexpected error: %v", err)
	}

	if tree1.Digest() == tree2.Digest() {
		t.Fatalf("Render() with different seeds gave the same digest %q", tree1.Digest())
	}
}

// pinnedDigest is the hard-coded SHA-256 digest of the tree produced by
// Render(Spec{Units:12, IncludeDepth:3, DependencyFanout:2, Seed:42,
// Errors:[BadOutputRef]}). Update ONLY on an intentional output-format
// change; a spontaneous mismatch means nondeterminism.
const pinnedDigest = "eab97b9b703cc449d49f74232ceaa6aa8ee36d427e7de4d259d3bcfc0f4e34aa"

func TestRender_PinnedDigest(t *testing.T) {
	spec := Spec{Units: 12, IncludeDepth: 3, DependencyFanout: 2, Seed: 42, Errors: []ErrorKind{BadOutputRef}}

	tree, _, err := Render(spec)
	if err != nil {
		t.Fatalf("Render(): unexpected error: %v", err)
	}

	got := tree.Digest()
	if got != pinnedDigest {
		t.Fatalf("Render() digest = %q, want pinned %q (if this is an intentional output-format change, update pinnedDigest; if not, this means nondeterminism)", got, pinnedDigest)
	}
}

func TestRender_Shape(t *testing.T) {
	spec := Spec{Units: 12, IncludeDepth: 3, DependencyFanout: 2, Seed: 7}
	tree, manifest, err := Render(spec)
	if err != nil {
		t.Fatalf("Render(): unexpected error: %v", err)
	}

	rootCount, tgCount, mainCount := 0, 0, 0
	byUnit := map[string][]File{}
	for _, f := range tree {
		switch {
		case f.Path == "root.hcl":
			rootCount++
		case strings.HasSuffix(f.Path, "/terragrunt.hcl"):
			tgCount++
			unit := strings.TrimSuffix(f.Path, "/terragrunt.hcl")
			byUnit[unit] = append(byUnit[unit], f)
			segs := strings.Split(unit, "/")
			if len(segs) != spec.IncludeDepth {
				t.Errorf("unit %q has %d path segments, want IncludeDepth=%d", unit, len(segs), spec.IncludeDepth)
			}
		case strings.HasSuffix(f.Path, "/main.tf"):
			mainCount++
		default:
			t.Errorf("unexpected file in tree: %q", f.Path)
		}
	}

	if rootCount != 1 {
		t.Errorf("root.hcl count = %d, want 1", rootCount)
	}
	if tgCount != spec.Units {
		t.Errorf("terragrunt.hcl count = %d, want %d", tgCount, spec.Units)
	}
	if mainCount != spec.Units {
		t.Errorf("main.tf count = %d, want %d", mainCount, spec.Units)
	}

	if len(manifest.Units) != spec.Units {
		t.Errorf("manifest.Units length = %d, want %d", len(manifest.Units), spec.Units)
	}
	for i := 1; i < len(manifest.Units); i++ {
		if manifest.Units[i-1].Compare(manifest.Units[i]) > 0 {
			t.Errorf("manifest.Units is not sorted: %q > %q", manifest.Units[i-1].String(), manifest.Units[i].String())
		}
	}

	// unit-000 must have no dependency blocks.
	for unit, files := range byUnit {
		if !strings.HasSuffix(unit, "unit-000") {
			continue
		}
		for _, f := range files {
			if strings.Contains(string(f.Content), "dependency \"") {
				t.Errorf("unit index 0 (%q) has dependency blocks, want none", unit)
			}
		}
	}

	// No unit has more than DependencyFanout dependency blocks.
	for unit, files := range byUnit {
		for _, f := range files {
			count := strings.Count(string(f.Content), "dependency \"")
			if count > spec.DependencyFanout {
				t.Errorf("unit %q has %d dependency blocks, want <= DependencyFanout=%d", unit, count, spec.DependencyFanout)
			}
		}
	}
}

func TestRender_CleanSpecEmptyManifestExpected(t *testing.T) {
	spec := Spec{Units: 5, IncludeDepth: 2, DependencyFanout: 2, Seed: 9}
	_, manifest, err := Render(spec)
	if err != nil {
		t.Fatalf("Render(): unexpected error: %v", err)
	}
	if len(manifest.Expected) != 0 {
		t.Errorf("manifest.Expected length = %d, want 0 for a clean spec", len(manifest.Expected))
	}
	if len(manifest.Units) != spec.Units {
		t.Errorf("manifest.Units length = %d, want %d", len(manifest.Units), spec.Units)
	}
}

func TestRelPath(t *testing.T) {
	cases := []struct {
		from, to, want string
	}{
		{"unit-001", "unit-000", "../unit-000"},
		{"g0/g1/unit-005", "g0/g0/unit-002", "../../g0/unit-002"},
		{"g1/unit-3", "g0/unit-1", "../../g0/unit-1"},
	}
	for _, c := range cases {
		if got := relPath(c.from, c.to); got != c.want {
			t.Errorf("relPath(%q, %q) = %q, want %q", c.from, c.to, got, c.want)
		}
	}
}

func TestLineCol(t *testing.T) {
	cases := []struct {
		content  string
		offset   int
		wantLine int
		wantCol  int
	}{
		{"abc", 0, 1, 1},
		{"ab\ncdef", 3, 2, 1},
		{"ab\ncdef", 5, 2, 3},
	}
	for _, c := range cases {
		line, col := lineCol([]byte(c.content), c.offset)
		if line != c.wantLine || col != c.wantCol {
			t.Errorf("lineCol(%q, %d) = (%d, %d), want (%d, %d)", c.content, c.offset, line, col, c.wantLine, c.wantCol)
		}
	}
}
