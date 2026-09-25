package repograph_test

import (
	"reflect"
	"testing"

	"github.com/GiulioSavini/gruntled/internal/domain/repograph"
)

func TestNewRepoPath(t *testing.T) {
	valid := []string{"units/a", "a", "."}
	for _, s := range valid {
		p, err := repograph.NewRepoPath(s)
		if err != nil {
			t.Errorf("NewRepoPath(%q): unexpected error: %v", s, err)
			continue
		}
		if p.String() != s {
			t.Errorf("NewRepoPath(%q).String() = %q, want %q", s, p.String(), s)
		}
		if p.IsZero() {
			t.Errorf("NewRepoPath(%q).IsZero() = true, want false", s)
		}
	}

	invalid := []string{"", "/abs", "../x", "..", "a/../b", "a/", `a\b`}
	for _, s := range invalid {
		if _, err := repograph.NewRepoPath(s); err == nil {
			t.Errorf("NewRepoPath(%q): expected error, got nil", s)
		}
	}
}

func TestRepoPathZeroValue(t *testing.T) {
	var p repograph.RepoPath
	if !p.IsZero() {
		t.Errorf("zero-value RepoPath.IsZero() = false, want true")
	}
}

func TestMustRepoPath(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Errorf("MustRepoPath(%q): expected panic, got none", "../x")
		}
	}()
	repograph.MustRepoPath("../x")
}

func TestMustRepoPathValid(t *testing.T) {
	p := repograph.MustRepoPath("units/a")
	if p.String() != "units/a" {
		t.Errorf("MustRepoPath(%q).String() = %q, want %q", "units/a", p.String(), "units/a")
	}
}

func TestRepoPathCompare(t *testing.T) {
	a := repograph.MustRepoPath("a")
	b := repograph.MustRepoPath("b")
	if a.Compare(b) >= 0 {
		t.Errorf("a.Compare(b) = %d, want < 0", a.Compare(b))
	}
	if b.Compare(a) <= 0 {
		t.Errorf("b.Compare(a) = %d, want > 0", b.Compare(a))
	}
	if a.Compare(a) != 0 {
		t.Errorf("a.Compare(a) = %d, want 0", a.Compare(a))
	}
}

func TestNewPosition(t *testing.T) {
	file := repograph.MustRepoPath("a.hcl")
	var zero repograph.RepoPath

	if _, err := repograph.NewPosition(file, 0, 1); err == nil {
		t.Errorf("NewPosition with line=0: expected error, got nil")
	}
	if _, err := repograph.NewPosition(file, 1, 0); err == nil {
		t.Errorf("NewPosition with column=0: expected error, got nil")
	}
	if _, err := repograph.NewPosition(zero, 1, 1); err == nil {
		t.Errorf("NewPosition with zero RepoPath: expected error, got nil")
	}

	p, err := repograph.NewPosition(file, 3, 5)
	if err != nil {
		t.Fatalf("NewPosition: unexpected error: %v", err)
	}
	if p.File() != file || p.Line() != 3 || p.Column() != 5 {
		t.Errorf("NewPosition: got file=%v line=%d column=%d, want file=%v line=3 column=5", p.File(), p.Line(), p.Column(), file)
	}
	if p.String() != "a.hcl:3:5" {
		t.Errorf("Position.String() = %q, want %q", p.String(), "a.hcl:3:5")
	}
}

func TestPositionCompare(t *testing.T) {
	fileA := repograph.MustRepoPath("a.hcl")
	fileB := repograph.MustRepoPath("b.hcl")

	pA1 := mustPosition(t, fileA, 1, 1)
	pA2 := mustPosition(t, fileA, 1, 2)
	pA3 := mustPosition(t, fileA, 2, 1)
	pB1 := mustPosition(t, fileB, 1, 1)

	if pA1.Compare(pA2) >= 0 {
		t.Errorf("pA1.Compare(pA2) = %d, want < 0 (column order)", pA1.Compare(pA2))
	}
	if pA2.Compare(pA3) >= 0 {
		t.Errorf("pA2.Compare(pA3) = %d, want < 0 (line order)", pA2.Compare(pA3))
	}
	if pA3.Compare(pB1) >= 0 {
		t.Errorf("pA3.Compare(pB1) = %d, want < 0 (file order)", pA3.Compare(pB1))
	}
	if pA1.Compare(pA1) != 0 {
		t.Errorf("pA1.Compare(pA1) = %d, want 0", pA1.Compare(pA1))
	}
}

func mustPosition(t *testing.T, file repograph.RepoPath, line, column int) repograph.Position {
	t.Helper()
	p, err := repograph.NewPosition(file, line, column)
	if err != nil {
		t.Fatalf("NewPosition(%v, %d, %d): unexpected error: %v", file, line, column, err)
	}
	return p
}

func TestNewSurface(t *testing.T) {
	s, err := repograph.NewSurface([]string{"b", "a"}, []string{"y", "x"})
	if err != nil {
		t.Fatalf("NewSurface: unexpected error: %v", err)
	}

	vars := s.Variables()
	if len(vars) != 2 || vars[0] != "a" || vars[1] != "b" {
		t.Errorf("Variables() = %v, want [a b]", vars)
	}
	outs := s.Outputs()
	if len(outs) != 2 || outs[0] != "x" || outs[1] != "y" {
		t.Errorf("Outputs() = %v, want [x y]", outs)
	}

	if !s.HasOutput("x") {
		t.Errorf("HasOutput(%q) = false, want true", "x")
	}
	if s.HasOutput("z") {
		t.Errorf("HasOutput(%q) = true, want false", "z")
	}
	if !s.HasVariable("a") {
		t.Errorf("HasVariable(%q) = false, want true", "a")
	}
	if s.HasVariable("z") {
		t.Errorf("HasVariable(%q) = true, want false", "z")
	}
}

func TestNewSurfaceRejectsDuplicateOrEmpty(t *testing.T) {
	if _, err := repograph.NewSurface([]string{"a", "a"}, nil); err == nil {
		t.Errorf("NewSurface with duplicate variable: expected error, got nil")
	}
	if _, err := repograph.NewSurface(nil, []string{"x", "x"}); err == nil {
		t.Errorf("NewSurface with duplicate output: expected error, got nil")
	}
	if _, err := repograph.NewSurface([]string{""}, nil); err == nil {
		t.Errorf("NewSurface with empty variable name: expected error, got nil")
	}
	if _, err := repograph.NewSurface(nil, []string{""}); err == nil {
		t.Errorf("NewSurface with empty output name: expected error, got nil")
	}
}

func TestSurfaceReturnedSlicesAreDefensiveCopies(t *testing.T) {
	s, err := repograph.NewSurface([]string{"a", "b"}, []string{"x", "y"})
	if err != nil {
		t.Fatalf("NewSurface: unexpected error: %v", err)
	}

	vars := s.Variables()
	vars[0] = "mutated"
	if s.Variables()[0] != "a" {
		t.Errorf("mutating Variables() result affected Surface: got %q, want %q", s.Variables()[0], "a")
	}

	outs := s.Outputs()
	outs[0] = "mutated"
	if s.Outputs()[0] != "x" {
		t.Errorf("mutating Outputs() result affected Surface: got %q, want %q", s.Outputs()[0], "x")
	}
}

func TestSurfaceInputSlicesAreCopied(t *testing.T) {
	vars := []string{"a", "b"}
	outs := []string{"x", "y"}
	s, err := repograph.NewSurface(vars, outs)
	if err != nil {
		t.Fatalf("NewSurface: unexpected error: %v", err)
	}
	vars[0] = "mutated"
	outs[0] = "mutated"
	if s.Variables()[0] != "a" {
		t.Errorf("mutating input slice after construction affected Surface variables: got %q", s.Variables()[0])
	}
	if s.Outputs()[0] != "x" {
		t.Errorf("mutating input slice after construction affected Surface outputs: got %q", s.Outputs()[0])
	}
}

func TestNewModule(t *testing.T) {
	path := repograph.MustRepoPath("modules/vpc")
	surface, err := repograph.NewSurface([]string{"cidr"}, []string{"subnet_id"})
	if err != nil {
		t.Fatalf("NewSurface: unexpected error: %v", err)
	}

	m, err := repograph.NewModule(path, surface)
	if err != nil {
		t.Fatalf("NewModule: unexpected error: %v", err)
	}
	if m.Path() != path {
		t.Errorf("Module.Path() = %v, want %v", m.Path(), path)
	}
	gotSurface, ok := m.Surface()
	if !ok || !gotSurface.HasOutput("subnet_id") {
		t.Errorf("Module.Surface() = (%v, %v), want a surface with output %q", gotSurface, ok, "subnet_id")
	}
	if m.UnknownReason() != "" {
		t.Errorf("known Module.UnknownReason() = %q, want empty", m.UnknownReason())
	}

	var zero repograph.RepoPath
	if _, err := repograph.NewModule(zero, surface); err == nil {
		t.Errorf("NewModule with zero RepoPath: expected error, got nil")
	}
}

func TestNewUnknownModule(t *testing.T) {
	path := repograph.MustRepoPath("modules/m")

	m, err := repograph.NewUnknownModule(path, "no-terraform-files")
	if err != nil {
		t.Fatalf("NewUnknownModule: unexpected error: %v", err)
	}
	if _, ok := m.Surface(); ok {
		t.Errorf("unknown Module.Surface() ok = true, want false")
	}
	if m.UnknownReason() != "no-terraform-files" {
		t.Errorf("UnknownReason() = %q, want %q", m.UnknownReason(), "no-terraform-files")
	}

	var zero repograph.RepoPath
	if _, err := repograph.NewUnknownModule(zero, "reason"); err == nil {
		t.Errorf("NewUnknownModule with zero RepoPath: expected error, got nil")
	}
	if _, err := repograph.NewUnknownModule(path, ""); err == nil {
		t.Errorf("NewUnknownModule with empty reason: expected error, got nil")
	}
}

func TestNewDependencyAndReference(t *testing.T) {
	target := repograph.MustRepoPath("units/vpc")
	pos := mustPosition(t, repograph.MustRepoPath("units/app/terragrunt.hcl"), 4, 1)
	opts := repograph.DefaultDependencyOptions()

	dep, err := repograph.NewDependency("vpc", target, pos, opts)
	if err != nil {
		t.Fatalf("NewDependency: unexpected error: %v", err)
	}
	gotTarget, ok := dep.Target()
	if dep.Name() != "vpc" || !ok || gotTarget != target || dep.Pos() != pos {
		t.Errorf("NewDependency: got name=%q target=(%v,%v) pos=%v", dep.Name(), gotTarget, ok, dep.Pos())
	}
	if dep.UnresolvedReason() != "" {
		t.Errorf("resolved Dependency.UnresolvedReason() = %q, want empty", dep.UnresolvedReason())
	}
	if !reflect.DeepEqual(dep.Options(), opts) {
		t.Errorf("Dependency.Options() = %+v, want %+v", dep.Options(), opts)
	}

	if _, err := repograph.NewDependency("", target, pos, opts); err == nil {
		t.Errorf("NewDependency with empty name: expected error, got nil")
	}
	if _, err := repograph.NewDependency("vpc", repograph.RepoPath{}, pos, opts); err == nil {
		t.Errorf("NewDependency with zero target: expected error, got nil")
	}

	ref, err := repograph.NewReference("vpc", "subnet_id", pos)
	if err != nil {
		t.Fatalf("NewReference: unexpected error: %v", err)
	}
	if ref.Dependency() != "vpc" || ref.Output() != "subnet_id" || ref.Pos() != pos {
		t.Errorf("NewReference: got dependency=%q output=%q pos=%v", ref.Dependency(), ref.Output(), ref.Pos())
	}

	if _, err := repograph.NewReference("", "subnet_id", pos); err == nil {
		t.Errorf("NewReference with empty dependency: expected error, got nil")
	}
	if _, err := repograph.NewReference("vpc", "", pos); err == nil {
		t.Errorf("NewReference with empty output: expected error, got nil")
	}
}

func TestNewUnresolvedDependency(t *testing.T) {
	pos := mustPosition(t, repograph.MustRepoPath("units/app/terragrunt.hcl"), 4, 1)
	opts := repograph.DefaultDependencyOptions()

	d, err := repograph.NewUnresolvedDependency("vpc", "config-path-dynamic", pos, opts)
	if err != nil {
		t.Fatalf("NewUnresolvedDependency: unexpected error: %v", err)
	}
	if _, ok := d.Target(); ok {
		t.Errorf("unresolved Dependency.Target() ok = true, want false")
	}
	if d.UnresolvedReason() != "config-path-dynamic" {
		t.Errorf("UnresolvedReason() = %q, want %q", d.UnresolvedReason(), "config-path-dynamic")
	}
	if !reflect.DeepEqual(d.Options(), opts) {
		t.Errorf("Options() = %+v, want %+v", d.Options(), opts)
	}

	if _, err := repograph.NewUnresolvedDependency("vpc", "", pos, opts); err == nil {
		t.Errorf("NewUnresolvedDependency with empty reason: expected error, got nil")
	}
	if _, err := repograph.NewUnresolvedDependency("", "reason", pos, opts); err == nil {
		t.Errorf("NewUnresolvedDependency with empty name: expected error, got nil")
	}
}

func TestUnitStatusString(t *testing.T) {
	if repograph.StatusResolved.String() != "resolved" {
		t.Errorf("StatusResolved.String() = %q, want %q", repograph.StatusResolved.String(), "resolved")
	}
	if repograph.StatusModuleUnknown.String() != "module-unknown" {
		t.Errorf("StatusModuleUnknown.String() = %q, want %q", repograph.StatusModuleUnknown.String(), "module-unknown")
	}
	if repograph.StatusConfigUnknown.String() != "config-unknown" {
		t.Errorf("StatusConfigUnknown.String() = %q, want %q", repograph.StatusConfigUnknown.String(), "config-unknown")
	}
}

func TestNewResolvedUnitSortsAndClones(t *testing.T) {
	unitPath := repograph.MustRepoPath("units/app")
	modulePath := repograph.MustRepoPath("modules/app")
	pos1 := mustPosition(t, unitPath, 1, 1)
	pos2 := mustPosition(t, unitPath, 2, 1)
	pos3 := mustPosition(t, unitPath, 3, 1)

	depB := mustDependency(t, "b", repograph.MustRepoPath("units/b"), pos1)
	depA := mustDependency(t, "a", repograph.MustRepoPath("units/a"), pos1)
	deps := []repograph.Dependency{depB, depA}

	refLate := mustReference(t, "b", "out", pos3)
	refEarly := mustReference(t, "a", "out", pos1)
	refMid := mustReference(t, "a", "out2", pos2)
	refs := []repograph.Reference{refLate, refEarly, refMid}

	u, err := repograph.NewResolvedUnit(unitPath, modulePath, deps, refs)
	if err != nil {
		t.Fatalf("NewResolvedUnit: unexpected error: %v", err)
	}

	gotDeps := u.Dependencies()
	if len(gotDeps) != 2 || gotDeps[0].Name() != "a" || gotDeps[1].Name() != "b" {
		t.Errorf("Dependencies() not sorted by name: %+v", gotDeps)
	}

	gotRefs := u.References()
	if len(gotRefs) != 3 || gotRefs[0] != refEarly || gotRefs[1] != refMid || gotRefs[2] != refLate {
		t.Errorf("References() not sorted by position: %+v", gotRefs)
	}

	if u.Status() != repograph.StatusResolved {
		t.Errorf("Status() = %v, want StatusResolved", u.Status())
	}
	mod, ok := u.Module()
	if !ok || mod != modulePath {
		t.Errorf("Module() = (%v, %v), want (%v, true)", mod, ok, modulePath)
	}

	d, ok := u.Dependency("a")
	if !ok || d.Name() != "a" {
		t.Errorf("Dependency(%q) = (%+v, %v), want name=a, true", "a", d, ok)
	}
	if _, ok := u.Dependency("nope"); ok {
		t.Errorf("Dependency(%q): got ok=true, want false", "nope")
	}

	// Mutating the input slices after construction must not affect the Unit.
	deps[0] = mustDependency(t, "z", repograph.MustRepoPath("units/z"), pos1)
	refs[0] = mustReference(t, "z", "out", pos1)
	if u.Dependencies()[1].Name() != "b" {
		t.Errorf("mutating input deps slice affected Unit: got %q, want %q", u.Dependencies()[1].Name(), "b")
	}
}

func TestNewResolvedUnitRejectsDuplicateDependencyName(t *testing.T) {
	unitPath := repograph.MustRepoPath("units/app")
	modulePath := repograph.MustRepoPath("modules/app")
	pos := mustPosition(t, unitPath, 1, 1)

	dep1 := mustDependency(t, "vpc", repograph.MustRepoPath("units/vpc"), pos)
	dep2 := mustDependency(t, "vpc", repograph.MustRepoPath("units/other"), pos)

	if _, err := repograph.NewResolvedUnit(unitPath, modulePath, []repograph.Dependency{dep1, dep2}, nil); err == nil {
		t.Errorf("NewResolvedUnit with duplicate dependency name: expected error, got nil")
	}
}

func TestNewResolvedUnitRejectsZeroPaths(t *testing.T) {
	var zero repograph.RepoPath
	unitPath := repograph.MustRepoPath("units/app")
	modulePath := repograph.MustRepoPath("modules/app")

	if _, err := repograph.NewResolvedUnit(zero, modulePath, nil, nil); err == nil {
		t.Errorf("NewResolvedUnit with zero unit path: expected error, got nil")
	}
	if _, err := repograph.NewResolvedUnit(unitPath, zero, nil, nil); err == nil {
		t.Errorf("NewResolvedUnit with zero module path: expected error, got nil")
	}
}

func TestNewConfigUnknownUnit(t *testing.T) {
	unitPath := repograph.MustRepoPath("units/legacy")

	u, err := repograph.NewConfigUnknownUnit(unitPath, "syntax-error")
	if err != nil {
		t.Fatalf("NewConfigUnknownUnit: unexpected error: %v", err)
	}
	if u.Status() != repograph.StatusConfigUnknown {
		t.Errorf("Status() = %v, want StatusConfigUnknown", u.Status())
	}
	if _, ok := u.Module(); ok {
		t.Errorf("Module() ok = true, want false")
	}
	if u.UnknownReason() != "syntax-error" {
		t.Errorf("UnknownReason() = %q, want %q", u.UnknownReason(), "syntax-error")
	}
	if len(u.Dependencies()) != 0 {
		t.Errorf("Dependencies() = %v, want empty", u.Dependencies())
	}
	if len(u.References()) != 0 {
		t.Errorf("References() = %v, want empty", u.References())
	}

	if _, err := repograph.NewConfigUnknownUnit(unitPath, ""); err == nil {
		t.Errorf("NewConfigUnknownUnit with empty reason: expected error, got nil")
	}
}

func TestNewModuleUnknownUnit(t *testing.T) {
	unitPath := repograph.MustRepoPath("units/app")
	pos1 := mustPosition(t, unitPath, 1, 1)
	pos2 := mustPosition(t, unitPath, 2, 1)

	depB := mustDependency(t, "b", repograph.MustRepoPath("units/b"), pos1)
	depA := mustDependency(t, "a", repograph.MustRepoPath("units/a"), pos1)
	refA := mustReference(t, "a", "out", pos1)
	refB := mustReference(t, "b", "out", pos2)

	u, err := repograph.NewModuleUnknownUnit(unitPath, "remote-source",
		[]repograph.Dependency{depB, depA}, []repograph.Reference{refB, refA})
	if err != nil {
		t.Fatalf("NewModuleUnknownUnit: unexpected error: %v", err)
	}
	if u.Status() != repograph.StatusModuleUnknown {
		t.Errorf("Status() = %v, want StatusModuleUnknown", u.Status())
	}
	if _, ok := u.Module(); ok {
		t.Errorf("Module() ok = true, want false")
	}
	if u.UnknownReason() != "remote-source" {
		t.Errorf("UnknownReason() = %q, want %q", u.UnknownReason(), "remote-source")
	}

	gotDeps := u.Dependencies()
	if len(gotDeps) != 2 || gotDeps[0].Name() != "a" || gotDeps[1].Name() != "b" {
		t.Errorf("Dependencies() not sorted by name: %+v", gotDeps)
	}
	gotRefs := u.References()
	if len(gotRefs) != 2 || gotRefs[0] != refA || gotRefs[1] != refB {
		t.Errorf("References() not sorted by position: %+v", gotRefs)
	}

	if _, err := repograph.NewModuleUnknownUnit(unitPath, "", nil, nil); err == nil {
		t.Errorf("NewModuleUnknownUnit with empty reason: expected error, got nil")
	}

	dup1 := mustDependency(t, "vpc", repograph.MustRepoPath("units/vpc"), pos1)
	dup2 := mustDependency(t, "vpc", repograph.MustRepoPath("units/other"), pos1)
	if _, err := repograph.NewModuleUnknownUnit(unitPath, "remote-source", []repograph.Dependency{dup1, dup2}, nil); err == nil {
		t.Errorf("NewModuleUnknownUnit with duplicate dependency name: expected error, got nil")
	}
}

func mustDependency(t *testing.T, name string, target repograph.RepoPath, pos repograph.Position) repograph.Dependency {
	t.Helper()
	d, err := repograph.NewDependency(name, target, pos, repograph.DefaultDependencyOptions())
	if err != nil {
		t.Fatalf("NewDependency(%q): unexpected error: %v", name, err)
	}
	return d
}

func mustReference(t *testing.T, dependency, output string, pos repograph.Position) repograph.Reference {
	t.Helper()
	r, err := repograph.NewReference(dependency, output, pos)
	if err != nil {
		t.Fatalf("NewReference(%q, %q): unexpected error: %v", dependency, output, err)
	}
	return r
}

func TestErrorMessagesStartWithPackageName(t *testing.T) {
	_, err := repograph.NewRepoPath("../x")
	if err == nil || !errorHasPrefix(err, "repograph:") {
		t.Errorf("NewRepoPath error = %v, want prefix %q", err, "repograph:")
	}
}

func errorHasPrefix(err error, prefix string) bool {
	if err == nil {
		return false
	}
	s := err.Error()
	return len(s) >= len(prefix) && s[:len(prefix)] == prefix
}
