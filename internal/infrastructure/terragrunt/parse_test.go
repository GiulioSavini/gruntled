package terragrunt

import (
	"slices"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/GiulioSavini/gruntled/internal/domain/diagnostic"
	"github.com/GiulioSavini/gruntled/internal/domain/repograph"
	"github.com/GiulioSavini/gruntled/internal/infrastructure/hclconv"
)

// --- fileCache: parse-once, missing files, syntax errors -------------------

func TestFileCacheParseOnce(t *testing.T) {
	cfs := newCountingFS(fstest.MapFS{
		"a.hcl": &fstest.MapFile{Data: []byte(`x = 1` + "\n")},
	})
	cache := newFileCache(cfs)
	p := repograph.MustRepoPath("a.hcl")

	first := cache.get(p)
	second := cache.get(p)
	// A different caller asking through the same cache instance must also
	// see the cached result, not trigger a second read.
	third := cache.get(p)

	if first != second || second != third {
		t.Fatalf("get() returned different *parsedFile pointers across calls")
	}
	if got := cfs.count("a.hcl"); got != 1 {
		t.Fatalf(`countingFS reads["a.hcl"] = %d, want 1`, got)
	}
}

func TestFileCacheMissingFile(t *testing.T) {
	cache := newFileCache(fstest.MapFS{})
	pf := cache.get(repograph.MustRepoPath("missing.hcl"))

	if pf == nil {
		t.Fatal("get() returned nil")
	}
	if pf.readErr == nil {
		t.Fatal("readErr = nil, want non-nil for a missing file")
	}
	if pf.syntax != nil {
		t.Fatalf("syntax = %v, want nil for a missing file", pf.syntax)
	}
	if len(pf.includes) != 0 || len(pf.terraforms) != 0 || len(pf.deps) != 0 || len(pf.generates) != 0 || len(pf.refs) != 0 {
		t.Fatalf("facts populated for a missing file: %+v", pf)
	}
}

func TestFileCacheMidEditSyntaxError(t *testing.T) {
	// An unclosed block: hclsyntax.ParseConfig returns a non-nil partial
	// file plus an error diagnostic, never a panic.
	src := "terraform {\n  source = \"x\"\n"
	cache := newFileCache(fstest.MapFS{"t.hcl": &fstest.MapFile{Data: []byte(src)}})
	pf := cache.get(repograph.MustRepoPath("t.hcl"))

	if pf.readErr != nil {
		t.Fatalf("readErr = %v, want nil for a file that was read but has a syntax error", pf.readErr)
	}
	if pf.syntax == nil {
		t.Fatal("syntax = nil, want a GRT100 diagnostic")
	}
	if pf.syntax.Code() != diagnostic.CodeSyntaxError {
		t.Fatalf("syntax.Code() = %v, want %v", pf.syntax.Code(), diagnostic.CodeSyntaxError)
	}
	if unit, ok := pf.syntax.Unit(); ok {
		t.Fatalf("syntax.Unit() = (%v, true), want file-level (ok=false)", unit)
	}
	if pf.syntax.Pos().Column() < 1 {
		t.Fatalf("syntax.Pos().Column() = %d, want >= 1", pf.syntax.Pos().Column())
	}
	if len(pf.includes) != 0 || len(pf.terraforms) != 0 || len(pf.deps) != 0 || len(pf.generates) != 0 || len(pf.refs) != 0 {
		t.Fatalf("facts populated despite a syntax error, even though hcl returned a partial body: %+v", pf)
	}
}

func TestFileCacheNonUTF8SyntaxError(t *testing.T) {
	src := []byte("a = \xff\xfe\n")
	cache := newFileCache(fstest.MapFS{"t.hcl": &fstest.MapFile{Data: src}})
	pf := cache.get(repograph.MustRepoPath("t.hcl"))

	if pf.readErr != nil {
		t.Fatalf("readErr = %v, want nil for a file that was read but is not valid UTF-8", pf.readErr)
	}
	if pf.syntax == nil {
		t.Fatal("syntax = nil, want a GRT100 diagnostic for non-UTF-8 source")
	}
	if len(pf.includes) != 0 || len(pf.terraforms) != 0 || len(pf.deps) != 0 || len(pf.generates) != 0 || len(pf.refs) != 0 {
		t.Fatalf("facts populated for non-UTF-8 source: %+v", pf)
	}
}

func TestFileCacheSyntaxDiagnosticsSortedAndOnlyTouched(t *testing.T) {
	broken := []byte("terraform {\n")
	cache := newFileCache(fstest.MapFS{
		"b.hcl":  &fstest.MapFile{Data: broken},
		"a.hcl":  &fstest.MapFile{Data: broken},
		"c.hcl":  &fstest.MapFile{Data: broken}, // never get() below: must not be reported
		"ok.hcl": &fstest.MapFile{Data: []byte("x = 1\n")},
	})
	cache.get(repograph.MustRepoPath("b.hcl"))
	cache.get(repograph.MustRepoPath("a.hcl"))
	cache.get(repograph.MustRepoPath("ok.hcl"))

	diags := cache.syntaxDiagnostics()
	if len(diags) != 2 {
		t.Fatalf("got %d diagnostics, want 2 (c.hcl untouched, ok.hcl has no error): %+v", len(diags), diags)
	}
	if diags[0].Pos().File().String() != "a.hcl" || diags[1].Pos().File().String() != "b.hcl" {
		t.Fatalf("diagnostics not sorted by path: %s then %s", diags[0].Pos().File().String(), diags[1].Pos().File().String())
	}
}

// TestFileCacheLimits proves G7b at the fileCache level: a file over
// hclconv.MaxFileBytes or nested deeper than hclconv.MaxNestingDepth is
// never parsed. It gets a limitReason, no readErr, no syntax diagnostic
// (it is not known to be invalid) and no facts. A file sitting exactly at
// the depth limit still parses normally.
func TestFileCacheLimits(t *testing.T) {
	atLimit := "x = " + strings.Repeat("(", hclconv.MaxNestingDepth) + "dependency.a.outputs.b" + strings.Repeat(")", hclconv.MaxNestingDepth) + "\n"
	cache := newFileCache(fstest.MapFS{
		"large.hcl": &fstest.MapFile{Data: []byte(strings.Repeat("#", hclconv.MaxFileBytes+1))},
		"deep.hcl":  &fstest.MapFile{Data: []byte("x = " + strings.Repeat("(", hclconv.MaxNestingDepth+1) + "1" + strings.Repeat(")", hclconv.MaxNestingDepth+1) + "\n")},
		"limit.hcl": &fstest.MapFile{Data: []byte(atLimit)},
	})

	for _, tc := range []struct {
		file string
		want string
	}{
		{"large.hcl", ReasonConfigTooLarge},
		{"deep.hcl", ReasonConfigTooDeep},
	} {
		pf := cache.get(repograph.MustRepoPath(tc.file))
		if pf.limitReason != tc.want {
			t.Fatalf("%s: limitReason = %q, want %q", tc.file, pf.limitReason, tc.want)
		}
		if pf.readErr != nil {
			t.Fatalf("%s: readErr = %v, want nil", tc.file, pf.readErr)
		}
		if pf.syntax != nil {
			t.Fatalf("%s: syntax = %v, want nil", tc.file, pf.syntax)
		}
		if len(pf.includes) != 0 || len(pf.terraforms) != 0 || len(pf.deps) != 0 || len(pf.generates) != 0 || len(pf.refs) != 0 {
			t.Fatalf("%s: facts populated for a refused file", tc.file)
		}
	}

	pf := cache.get(repograph.MustRepoPath("limit.hcl"))
	if pf.limitReason != "" || pf.readErr != nil || pf.syntax != nil {
		t.Fatalf("limit.hcl: (limitReason %q, readErr %v, syntax %v), want a normal parse", pf.limitReason, pf.readErr, pf.syntax)
	}
	if len(pf.refs) != 1 {
		t.Fatalf("limit.hcl: %d refs, want 1", len(pf.refs))
	}

	if diags := cache.syntaxDiagnostics(); len(diags) != 0 {
		t.Fatalf("syntaxDiagnostics() = %v, want none", diags)
	}
}

// --- structural facts: include, terraform, dependency, generate, refs ------

func TestParseIncludes(t *testing.T) {
	src := `
include "root" {
  path = find_in_parent_folders("root.hcl")
}

include {
  path            = "x"
  merge_strategy  = "deep"
}

include "a" "b" {
}
`
	cache := newFileCache(fstest.MapFS{"t.hcl": &fstest.MapFile{Data: []byte(src)}})
	pf := cache.get(repograph.MustRepoPath("t.hcl"))
	if pf.readErr != nil || pf.syntax != nil {
		t.Fatalf("unexpected readErr=%v syntax=%v", pf.readErr, pf.syntax)
	}
	if len(pf.includes) != 3 {
		t.Fatalf("got %d includeDecl, want 3: %+v", len(pf.includes), pf.includes)
	}

	first, second, third := pf.includes[0], pf.includes[1], pf.includes[2]
	if !slices.Equal(first.labels, []string{"root"}) {
		t.Fatalf("includes[0].labels = %v, want [root]", first.labels)
	}
	if first.path == nil {
		t.Fatal("includes[0].path = nil, want non-nil")
	}

	if !slices.Equal(second.labels, []string{}) && second.labels != nil {
		t.Fatalf("includes[1].labels = %v, want empty", second.labels)
	}
	if second.mergeStrategy == nil {
		t.Fatal("includes[1].mergeStrategy = nil, want non-nil")
	}

	if !slices.Equal(third.labels, []string{"a", "b"}) {
		t.Fatalf("includes[2].labels = %v, want [a b]", third.labels)
	}
	if third.path != nil {
		t.Fatal("includes[2].path != nil, want nil (no path attribute)")
	}
}

func TestParseTerraformBlocks(t *testing.T) {
	src := `
terraform {
  source = "../mod"
}

terraform {
}
`
	cache := newFileCache(fstest.MapFS{"t.hcl": &fstest.MapFile{Data: []byte(src)}})
	pf := cache.get(repograph.MustRepoPath("t.hcl"))
	if pf.readErr != nil || pf.syntax != nil {
		t.Fatalf("unexpected readErr=%v syntax=%v", pf.readErr, pf.syntax)
	}
	if len(pf.terraforms) != 2 {
		t.Fatalf("got %d terraformDecl, want 2 (the loader rejects duplicates, this layer just reports them): %+v", len(pf.terraforms), pf.terraforms)
	}
	if pf.terraforms[0].source == nil {
		t.Fatal("terraforms[0].source = nil, want non-nil")
	}
	if pf.terraforms[1].source != nil {
		t.Fatal("terraforms[1].source != nil, want nil (empty block)")
	}
}

func TestParseDependencyBlocks(t *testing.T) {
	src := `
dependency "vpc" {
  config_path = "../vpc"
}

dependency "x" {
  expansion {
    for_each = []
  }
  config_path = "../x"
}

dependency {
  config_path = "../y"
}
`
	cache := newFileCache(fstest.MapFS{"t.hcl": &fstest.MapFile{Data: []byte(src)}})
	pf := cache.get(repograph.MustRepoPath("t.hcl"))
	if pf.readErr != nil || pf.syntax != nil {
		t.Fatalf("unexpected readErr=%v syntax=%v", pf.readErr, pf.syntax)
	}
	if len(pf.deps) != 3 {
		t.Fatalf("got %d depDecl, want 3: %+v", len(pf.deps), pf.deps)
	}

	vpc, x, bare := pf.deps[0], pf.deps[1], pf.deps[2]
	if !slices.Equal(vpc.labels, []string{"vpc"}) || vpc.hasExpansion {
		t.Fatalf("deps[0] = %+v, want labels [vpc], hasExpansion=false", vpc)
	}
	if vpc.configPath == nil {
		t.Fatal("deps[0].configPath = nil, want non-nil")
	}
	if !slices.Equal(x.labels, []string{"x"}) || !x.hasExpansion {
		t.Fatalf("deps[1] = %+v, want labels [x], hasExpansion=true", x)
	}
	if !slices.Equal(bare.labels, []string{}) && bare.labels != nil {
		t.Fatalf("deps[2].labels = %v, want empty", bare.labels)
	}
	if bare.hasExpansion {
		t.Fatal("deps[2].hasExpansion = true, want false")
	}
}

func TestParseGenerateBlocks(t *testing.T) {
	src := `
generate "provider" {
  contents = "provider \"aws\" {}"
}
`
	cache := newFileCache(fstest.MapFS{"t.hcl": &fstest.MapFile{Data: []byte(src)}})
	pf := cache.get(repograph.MustRepoPath("t.hcl"))
	if pf.readErr != nil || pf.syntax != nil {
		t.Fatalf("unexpected readErr=%v syntax=%v", pf.readErr, pf.syntax)
	}
	if len(pf.generates) != 1 {
		t.Fatalf("got %d generateDecl, want 1: %+v", len(pf.generates), pf.generates)
	}
	if !slices.Equal(pf.generates[0].labels, []string{"provider"}) {
		t.Fatalf("generates[0].labels = %v, want [provider]", pf.generates[0].labels)
	}
	if pf.generates[0].contents == nil {
		t.Fatal("generates[0].contents = nil, want non-nil")
	}
}

func TestParseRefsMatchExtractRefs(t *testing.T) {
	src := `inputs = { a = dependency.vpc.outputs.id }` + "\n"
	body, b := parseBody(t, "t.hcl", src)
	file := repograph.MustRepoPath("t.hcl")
	want, err := extractRefs(file, b, body)
	if err != nil {
		t.Fatalf("extractRefs: unexpected error: %v", err)
	}

	cache := newFileCache(fstest.MapFS{"t.hcl": &fstest.MapFile{Data: b}})
	pf := cache.get(file)
	if pf.readErr != nil || pf.syntax != nil {
		t.Fatalf("unexpected readErr=%v syntax=%v", pf.readErr, pf.syntax)
	}
	if len(pf.refs) != len(want) || len(want) != 1 {
		t.Fatalf("pf.refs = %v, want %v", pf.refs, want)
	}
	if pf.refs[0].Dependency() != want[0].Dependency() || pf.refs[0].Output() != want[0].Output() {
		t.Fatalf("pf.refs[0] = (%s,%s), want (%s,%s)", pf.refs[0].Dependency(), pf.refs[0].Output(), want[0].Dependency(), want[0].Output())
	}
}

// --- dependencyOptions: DIAG-03 fact table ----------------------------------

func TestDependencyOptionsNoAttributes(t *testing.T) {
	body, _ := parseBody(t, "d.hcl", "\n")
	got := dependencyOptions(body)
	want := repograph.DefaultDependencyOptions()
	if got.Enabled != want.Enabled {
		t.Fatalf("Enabled = %v, want %v", got.Enabled, want.Enabled)
	}
	if got.SkipOutputs != want.SkipOutputs {
		t.Fatalf("SkipOutputs = %v, want %v", got.SkipOutputs, want.SkipOutputs)
	}
	if got.MockMergeWithState != want.MockMergeWithState {
		t.Fatalf("MockMergeWithState = %v, want %v", got.MockMergeWithState, want.MockMergeWithState)
	}
	assertNameListEqual(t, got.MockOutputs, want.MockOutputs)
	assertNameListEqual(t, got.MockAllowedCommands, want.MockAllowedCommands)
}

func TestDependencyOptionsEnabledSkipOutputs(t *testing.T) {
	tests := []struct {
		name string
		src  string
		want repograph.Tristate
		get  func(repograph.DependencyOptions) repograph.Tristate
	}{
		{"enabled literal false", "enabled = false\n", repograph.TristateFalse, func(o repograph.DependencyOptions) repograph.Tristate { return o.Enabled }},
		{"enabled non-literal", "enabled = local.on\n", repograph.TristateUnknown, func(o repograph.DependencyOptions) repograph.Tristate { return o.Enabled }},
		{"skip_outputs literal true", "skip_outputs = true\n", repograph.TristateTrue, func(o repograph.DependencyOptions) repograph.Tristate { return o.SkipOutputs }},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			body, _ := parseBody(t, "d.hcl", tc.src)
			got := tc.get(dependencyOptions(body))
			if got != tc.want {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
		})
	}
}

func TestDependencyOptionsMockOutputs(t *testing.T) {
	knownEmpty, err := repograph.KnownNames(nil)
	if err != nil {
		t.Fatalf("KnownNames(nil): %v", err)
	}
	knownTwo, err := repograph.KnownNames([]string{"vpc_id", "subnet"})
	if err != nil {
		t.Fatalf("KnownNames: %v", err)
	}

	tests := []struct {
		name string
		src  string
		want repograph.NameList
	}{
		{"two literal keys, unquoted and quoted", `mock_outputs = { vpc_id = "m", "subnet" = [] }` + "\n", knownTwo},
		{"empty object", `mock_outputs = {}` + "\n", knownEmpty},
		{"non-literal: local ref", `mock_outputs = local.m` + "\n", repograph.UnknownNames()},
		{"non-literal: merge call", `mock_outputs = merge({a = 1}, local.m)` + "\n", repograph.UnknownNames()},
		{"computed key", `mock_outputs = { (local.k) = 1 }` + "\n", repograph.UnknownNames()},
		{"duplicate key", `mock_outputs = { a = 1, a = 2 }` + "\n", repograph.UnknownNames()},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			body, _ := parseBody(t, "d.hcl", tc.src)
			got := dependencyOptions(body).MockOutputs
			assertNameListEqual(t, got, tc.want)
		})
	}

	t.Run("absent", func(t *testing.T) {
		body, _ := parseBody(t, "d.hcl", "\n")
		got := dependencyOptions(body).MockOutputs
		if !got.IsAbsent() {
			t.Fatalf("MockOutputs.IsAbsent() = false, want true")
		}
	})
}

func assertNameListEqual(t *testing.T, got, want repograph.NameList) {
	t.Helper()
	if got.IsUnknown() != want.IsUnknown() {
		t.Fatalf("IsUnknown() = %v, want %v", got.IsUnknown(), want.IsUnknown())
	}
	if got.IsAbsent() != want.IsAbsent() {
		t.Fatalf("IsAbsent() = %v, want %v", got.IsAbsent(), want.IsAbsent())
	}
	gotNames, gotOK := got.Names()
	wantNames, wantOK := want.Names()
	if gotOK != wantOK {
		t.Fatalf("Names() ok = %v, want %v (got=%v want=%v)", gotOK, wantOK, got, want)
	}
	if gotOK && !slices.Equal(gotNames, wantNames) {
		t.Fatalf("Names() = %v, want %v", gotNames, wantNames)
	}
}

func TestDependencyOptionsMockMergeWithState(t *testing.T) {
	tests := []struct {
		name string
		src  string
		want repograph.Tristate
	}{
		{"merge_with_state literal true", "mock_outputs_merge_with_state = true\n", repograph.TristateTrue},
		{"strategy no_merge", `mock_outputs_merge_strategy_with_state = "no_merge"` + "\n", repograph.TristateFalse},
		{"strategy shallow", `mock_outputs_merge_strategy_with_state = "shallow"` + "\n", repograph.TristateTrue},
		{"strategy deep_map_only", `mock_outputs_merge_strategy_with_state = "deep_map_only"` + "\n", repograph.TristateTrue},
		{"strategy unrecognized literal", `mock_outputs_merge_strategy_with_state = "weird"` + "\n", repograph.TristateUnknown},
		{"strategy non-literal", "mock_outputs_merge_strategy_with_state = local.s\n", repograph.TristateUnknown},
		{
			"merge true wins over strategy no_merge",
			"mock_outputs_merge_with_state = true\n" + `mock_outputs_merge_strategy_with_state = "no_merge"` + "\n",
			repograph.TristateTrue,
		},
		{
			"strategy non-literal, merge literal false -> unknown",
			"mock_outputs_merge_with_state = false\nmock_outputs_merge_strategy_with_state = local.s\n",
			repograph.TristateUnknown,
		},
		{"neither attribute present", "\n", repograph.TristateFalse},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			body, _ := parseBody(t, "d.hcl", tc.src)
			got := dependencyOptions(body).MockMergeWithState
			if got != tc.want {
				t.Fatalf("MockMergeWithState = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestDependencyOptionsAllowedCommands(t *testing.T) {
	known, err := repograph.KnownNames([]string{"validate", "plan", "apply"})
	if err != nil {
		t.Fatalf("KnownNames: %v", err)
	}
	knownEmpty, err := repograph.KnownNames(nil)
	if err != nil {
		t.Fatalf("KnownNames(nil): %v", err)
	}

	tests := []struct {
		name string
		src  string
		want repograph.NameList
	}{
		{"literal list", `mock_outputs_allowed_terraform_commands = ["validate", "plan", "apply"]` + "\n", known},
		{"empty literal list", `mock_outputs_allowed_terraform_commands = []` + "\n", knownEmpty},
		{"non-literal: local ref", "mock_outputs_allowed_terraform_commands = local.cmds\n", repograph.UnknownNames()},
		{"mixed literal and non-literal", `mock_outputs_allowed_terraform_commands = ["plan", local.c]` + "\n", repograph.UnknownNames()},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			body, _ := parseBody(t, "d.hcl", tc.src)
			got := dependencyOptions(body).MockAllowedCommands
			assertNameListEqual(t, got, tc.want)
		})
	}

	t.Run("absent", func(t *testing.T) {
		body, _ := parseBody(t, "d.hcl", "\n")
		got := dependencyOptions(body).MockAllowedCommands
		if !got.IsAbsent() {
			t.Fatalf("MockAllowedCommands.IsAbsent() = false, want true")
		}
	})
}

// TestDependencyOptionsPrimaryCorpusShape is the DEP-08 shape: two
// mock_outputs keys, mock_outputs_merge_with_state = true, and a 3-command
// allowlist, all captured together on one block.
func TestDependencyOptionsPrimaryCorpusShape(t *testing.T) {
	src := `
mock_outputs = {
  role_name = "mock"
  role_arn  = "mock"
}
mock_outputs_merge_with_state = true
mock_outputs_allowed_terraform_commands = ["validate", "plan", "apply"]
`
	body, _ := parseBody(t, "d.hcl", src)
	got := dependencyOptions(body)

	names, ok := got.MockOutputs.Names()
	if !ok {
		t.Fatal("MockOutputs not Known")
	}
	if !slices.Equal(names, []string{"role_arn", "role_name"}) {
		t.Fatalf("MockOutputs.Names() = %v, want [role_arn role_name]", names)
	}
	if got.MockMergeWithState != repograph.TristateTrue {
		t.Fatalf("MockMergeWithState = %v, want true", got.MockMergeWithState)
	}
	cmds, ok := got.MockAllowedCommands.Names()
	if !ok {
		t.Fatal("MockAllowedCommands not Known")
	}
	if !slices.Equal(cmds, []string{"apply", "plan", "validate"}) {
		t.Fatalf("MockAllowedCommands.Names() = %v, want [apply plan validate]", cmds)
	}
}
