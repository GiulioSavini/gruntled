package tfsurface_test

import (
	"context"
	"fmt"
	"io/fs"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/GiulioSavini/gruntled/internal/application/ports"
	"github.com/GiulioSavini/gruntled/internal/domain/repograph"
	"github.com/GiulioSavini/gruntled/internal/infrastructure/hclconv"
	"github.com/GiulioSavini/gruntled/internal/infrastructure/tfsurface"
)

func readSurface(t *testing.T, fsys fs.FS, module string) ports.SurfaceResult {
	t.Helper()
	r := tfsurface.NewReader(fsys)
	res, err := r.ReadSurface(context.Background(), repograph.MustRepoPath(module))
	if err != nil {
		t.Fatalf("ReadSurface(%q): unexpected error: %v", module, err)
	}
	return res
}

func assertSurface(t *testing.T, res ports.SurfaceResult, wantVars, wantOuts []string) {
	t.Helper()
	if res.UnknownReason != "" {
		t.Fatalf("UnknownReason = %q, want \"\" (diags: %v)", res.UnknownReason, res.Diagnostics)
	}
	if got := res.Surface.Variables(); !equalNames(got, wantVars) {
		t.Fatalf("Variables() = %v, want %v", got, wantVars)
	}
	if got := res.Surface.Outputs(); !equalNames(got, wantOuts) {
		t.Fatalf("Outputs() = %v, want %v", got, wantOuts)
	}
}

func equalNames(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}

func TestReadSurfaceUnquotedLabelAndVariable(t *testing.T) {
	fsys := fstest.MapFS{
		"mod/main.tf": &fstest.MapFile{Data: []byte(`
output name { value = 1 }
variable "x" {}
`)},
	}
	res := readSurface(t, fsys, "mod")
	assertSurface(t, res, []string{"x"}, []string{"name"})
}

func TestReadSurfaceJSONObjectForm(t *testing.T) {
	fsys := fstest.MapFS{
		"mod/main.tf.json": &fstest.MapFile{Data: []byte(`{"output": {"a": {"value": 1}}, "variable": {"v": {}}}`)},
	}
	res := readSurface(t, fsys, "mod")
	assertSurface(t, res, []string{"v"}, []string{"a"})
}

func TestReadSurfaceMixedTfAndJSONUnion(t *testing.T) {
	fsys := fstest.MapFS{
		"mod/main.tf":       &fstest.MapFile{Data: []byte(`output "a" { value = 1 }`)},
		"mod/extra.tf.json": &fstest.MapFile{Data: []byte(`{"output": {"b": {"value": 1}}}`)},
	}
	res := readSurface(t, fsys, "mod")
	assertSurface(t, res, nil, []string{"a", "b"})
}

func TestReadSurfaceOpenTofuPrecedence(t *testing.T) {
	fsys := fstest.MapFS{
		"mod/foo.tf":   &fstest.MapFile{Data: []byte(`output "a" { value = 1 }`)},
		"mod/foo.tofu": &fstest.MapFile{Data: []byte(`output "b" { value = 1 }`)},
	}
	res := readSurface(t, fsys, "mod")
	assertSurface(t, res, nil, []string{"b"})
}

func TestReadSurfaceOpenTofuJSONPrecedence(t *testing.T) {
	fsys := fstest.MapFS{
		"mod/x.tf.json":   &fstest.MapFile{Data: []byte(`{"output": {"a": {"value": 1}}}`)},
		"mod/x.tofu.json": &fstest.MapFile{Data: []byte(`{"output": {"b": {"value": 1}}}`)},
	}
	res := readSurface(t, fsys, "mod")
	assertSurface(t, res, nil, []string{"b"})
}

func TestReadSurfaceOverrideDedup(t *testing.T) {
	fsys := fstest.MapFS{
		"mod/main.tf":     &fstest.MapFile{Data: []byte(`output "a" { value = 1 }`)},
		"mod/override.tf": &fstest.MapFile{Data: []byte(`output "a" { value = 2 }`)},
	}
	res := readSurface(t, fsys, "mod")
	assertSurface(t, res, nil, []string{"a"})
}

func TestReadSurfaceIgnoredNames(t *testing.T) {
	fsys := fstest.MapFS{
		"mod/main.tf":    &fstest.MapFile{Data: []byte(`output "keep" { value = 1 }`)},
		"mod/.hidden.tf": &fstest.MapFile{Data: []byte(`output "z" { value = 1 }`)},
		"mod/main.tf~":   &fstest.MapFile{Data: []byte(`output "z" { value = 1 }`)},
		"mod/#main.tf#":  &fstest.MapFile{Data: []byte(`output "z" { value = 1 }`)},
	}
	res := readSurface(t, fsys, "mod")
	assertSurface(t, res, nil, []string{"keep"})
}

func TestReadSurfaceDirectoryNamedLikeAModuleFileIsSkipped(t *testing.T) {
	fsys := fstest.MapFS{
		"mod/main.tf":       &fstest.MapFile{Data: []byte(`output "keep" { value = 1 }`)},
		"mod/dir.tf":        &fstest.MapFile{Mode: fs.ModeDir},
		"mod/dir.tf/nested": &fstest.MapFile{Data: []byte("x")},
	}
	res := readSurface(t, fsys, "mod")
	assertSurface(t, res, nil, []string{"keep"})
}

func TestReadSurfaceModuleDirNotFound(t *testing.T) {
	fsys := fstest.MapFS{
		"other/main.tf": &fstest.MapFile{Data: []byte(`output "a" { value = 1 }`)},
	}
	res := readSurface(t, fsys, "mod")
	if res.UnknownReason != tfsurface.ReasonModuleDirNotFound {
		t.Fatalf("UnknownReason = %q, want %q", res.UnknownReason, tfsurface.ReasonModuleDirNotFound)
	}
	if len(res.Diagnostics) != 0 {
		t.Fatalf("Diagnostics = %v, want none", res.Diagnostics)
	}
}

func TestReadSurfaceNoTerraformFiles(t *testing.T) {
	fsys := fstest.MapFS{
		"mod/README.md": &fstest.MapFile{Data: []byte("hello")},
	}
	res := readSurface(t, fsys, "mod")
	if res.UnknownReason != tfsurface.ReasonNoTerraformFiles {
		t.Fatalf("UnknownReason = %q, want %q", res.UnknownReason, tfsurface.ReasonNoTerraformFiles)
	}
}

func TestReadSurfaceOneBrokenFileAmongTwo(t *testing.T) {
	fsys := fstest.MapFS{
		"mod/good.tf": &fstest.MapFile{Data: []byte(`output "a" { value = 1 }`)},
		"mod/bad.tf": &fstest.MapFile{Data: []byte(`
output "x" {
  value = 1
`)},
	}
	res := readSurface(t, fsys, "mod")
	if res.UnknownReason != tfsurface.ReasonSyntaxError {
		t.Fatalf("UnknownReason = %q, want %q", res.UnknownReason, tfsurface.ReasonSyntaxError)
	}
	if len(res.Diagnostics) != 1 {
		t.Fatalf("len(Diagnostics) = %d, want 1 (%v)", len(res.Diagnostics), res.Diagnostics)
	}
	if file := res.Diagnostics[0].Pos().File().String(); file != "mod/bad.tf" {
		t.Fatalf("Diagnostics[0].Pos().File() = %q, want %q", file, "mod/bad.tf")
	}
}

func TestReadSurfaceTwoBrokenFiles(t *testing.T) {
	broken := []byte(`
output "x" {
  value = 1
`)
	fsys := fstest.MapFS{
		"mod/bad1.tf": &fstest.MapFile{Data: broken},
		"mod/bad2.tf": &fstest.MapFile{Data: broken},
	}
	res := readSurface(t, fsys, "mod")
	if res.UnknownReason != tfsurface.ReasonSyntaxError {
		t.Fatalf("UnknownReason = %q, want %q", res.UnknownReason, tfsurface.ReasonSyntaxError)
	}
	if len(res.Diagnostics) != 2 {
		t.Fatalf("len(Diagnostics) = %d, want 2 (%v)", len(res.Diagnostics), res.Diagnostics)
	}
	if f0, f1 := res.Diagnostics[0].Pos().File().String(), res.Diagnostics[1].Pos().File().String(); f0 != "mod/bad1.tf" || f1 != "mod/bad2.tf" {
		t.Fatalf("Diagnostics files = %q, %q, want mod/bad1.tf, mod/bad2.tf (sorted)", f0, f1)
	}
}

func TestReadSurfaceInvalidModuleBlock(t *testing.T) {
	fsys := fstest.MapFS{
		"mod/main.tf": &fstest.MapFile{Data: []byte(`output {}`)},
	}
	res := readSurface(t, fsys, "mod")
	if res.UnknownReason != tfsurface.ReasonInvalidModuleBlock {
		t.Fatalf("UnknownReason = %q, want %q", res.UnknownReason, tfsurface.ReasonInvalidModuleBlock)
	}
}

func TestReadSurfaceRepoRootModule(t *testing.T) {
	fsys := fstest.MapFS{
		"main.tf": &fstest.MapFile{Data: []byte(`output "a" { value = 1 }`)},
	}
	res := readSurface(t, fsys, ".")
	assertSurface(t, res, nil, []string{"a"})
}

func TestReadSurfaceContextCancelled(t *testing.T) {
	fsys := fstest.MapFS{
		"mod/main.tf": &fstest.MapFile{Data: []byte(`output "a" { value = 1 }`)},
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	r := tfsurface.NewReader(fsys)
	if _, err := r.ReadSurface(ctx, repograph.MustRepoPath("mod")); err == nil {
		t.Fatal("ReadSurface with a cancelled context: err = nil, want non-nil")
	}
}

// --- G7: module-file size cap and nesting-depth guard ----------------------

// readFailFS wraps an fstest.MapFS so that ReadFile fails for exactly one
// named file, while Stat (promoted, unmodified, from the embedded MapFS)
// still succeeds: this exercises ReasonModuleFileUnreadable without
// needing a real dangling or escaping symlink (realfs_test.go covers
// those).
type readFailFS struct {
	fstest.MapFS
	failFile string
}

func (f readFailFS) ReadFile(name string) ([]byte, error) {
	if name == f.failFile {
		return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrPermission}
	}
	return f.MapFS.ReadFile(name)
}

// deepParenValue returns an `output "x" { value = ... }` block whose value
// nests n parens deep, entirely generated in memory (never written to the
// repo as a fixture).
func deepParenValue(n int) string {
	return `output "x" {` + "\n  value = " + strings.Repeat("(", n) + "1" + strings.Repeat(")", n) + "\n}\n"
}

// deepJSONValue returns a .tf.json document with an output named "x" whose
// value nests n JSON objects deep.
func deepJSONValue(n int) string {
	nested := strings.Repeat(`{"a":`, n) + "1" + strings.Repeat("}", n)
	return `{"output": {"x": {"value": ` + nested + `}}}`
}

// TestReadSurfaceDeepNestingNoCrash is the G7 regression: before the fix,
// this killed the test binary with a fatal stack overflow (150k levels
// crashed in the 02-10 planning probe; 200k is used here for headroom).
func TestReadSurfaceDeepNestingNoCrash(t *testing.T) {
	fsys := fstest.MapFS{
		"m/main.tf": &fstest.MapFile{Data: []byte(deepParenValue(200_000))},
	}
	res := readSurface(t, fsys, "m")
	if res.UnknownReason != tfsurface.ReasonModuleFileTooDeep {
		t.Fatalf("UnknownReason = %q, want %q", res.UnknownReason, tfsurface.ReasonModuleFileTooDeep)
	}
	if len(res.Diagnostics) != 0 {
		t.Fatalf("Diagnostics = %v, want none", res.Diagnostics)
	}
}

// TestReadSurfaceDeepJSONNoCrash is the JSON counterpart of the G7
// regression: hcl/json.Parse recurses just as hclsyntax does.
func TestReadSurfaceDeepJSONNoCrash(t *testing.T) {
	fsys := fstest.MapFS{
		"m/main.tf.json": &fstest.MapFile{Data: []byte(deepJSONValue(200_000))},
	}
	res := readSurface(t, fsys, "m")
	if res.UnknownReason != tfsurface.ReasonModuleFileTooDeep {
		t.Fatalf("UnknownReason = %q, want %q", res.UnknownReason, tfsurface.ReasonModuleFileTooDeep)
	}
}

// TestReadSurfaceOversizeFile proves a file over hclconv.MaxFileBytes is
// refused before parsing, with no diagnostic (it is not a syntax error).
func TestReadSurfaceOversizeFile(t *testing.T) {
	base := "output \"x\" {}\n# "
	pad := strings.Repeat("a", hclconv.MaxFileBytes+1-len(base))
	fsys := fstest.MapFS{
		"m/main.tf": &fstest.MapFile{Data: []byte(base + pad)},
	}
	res := readSurface(t, fsys, "m")
	if res.UnknownReason != tfsurface.ReasonModuleFileTooLarge {
		t.Fatalf("UnknownReason = %q, want %q", res.UnknownReason, tfsurface.ReasonModuleFileTooLarge)
	}
	if len(res.Diagnostics) != 0 {
		t.Fatalf("Diagnostics = %v, want none", res.Diagnostics)
	}
}

// TestReadSurfaceAtDepthLimitStillReads proves a module file at exactly
// hclconv.MaxNestingDepth still parses normally: the bound is inclusive.
// The output block's own enclosing braces count as one nesting level (as
// CheckNativeDepth's own boundary test at package hclconv proves in
// isolation), so the value nests one fewer paren to land the WHOLE file
// exactly at the limit.
func TestReadSurfaceAtDepthLimitStillReads(t *testing.T) {
	fsys := fstest.MapFS{
		"m/main.tf": &fstest.MapFile{Data: []byte(deepParenValue(hclconv.MaxNestingDepth - 1))},
	}
	res := readSurface(t, fsys, "m")
	assertSurface(t, res, nil, []string{"x"})
}

// TestReadSurfaceLimitPrecedence proves the reader's final precedence:
// unreadable > too-large > too-deep > syntax-error (> invalid-block,
// unchanged from before this plan).
func TestReadSurfaceLimitPrecedence(t *testing.T) {
	t.Run("unreadable beats too-large", func(t *testing.T) {
		fsys := readFailFS{
			MapFS: fstest.MapFS{
				"m/bad.tf": &fstest.MapFile{Data: []byte(`output "a" { value = 1 }`)},
				"m/big.tf": &fstest.MapFile{Data: []byte(strings.Repeat("a", hclconv.MaxFileBytes+1))},
			},
			failFile: "m/bad.tf",
		}
		res := readSurface(t, fsys, "m")
		if res.UnknownReason != tfsurface.ReasonModuleFileUnreadable {
			t.Fatalf("UnknownReason = %q, want %q", res.UnknownReason, tfsurface.ReasonModuleFileUnreadable)
		}
	})

	t.Run("too-large beats too-deep", func(t *testing.T) {
		fsys := fstest.MapFS{
			"m/big.tf":  &fstest.MapFile{Data: []byte(strings.Repeat("a", hclconv.MaxFileBytes+1))},
			"m/deep.tf": &fstest.MapFile{Data: []byte(deepParenValue(200_000))},
		}
		res := readSurface(t, fsys, "m")
		if res.UnknownReason != tfsurface.ReasonModuleFileTooLarge {
			t.Fatalf("UnknownReason = %q, want %q", res.UnknownReason, tfsurface.ReasonModuleFileTooLarge)
		}
	})

	t.Run("too-deep beats syntax-error", func(t *testing.T) {
		fsys := fstest.MapFS{
			"m/deep.tf": &fstest.MapFile{Data: []byte(deepParenValue(200_000))},
			"m/bad.tf": &fstest.MapFile{Data: []byte(`
output "x" {
  value = 1
`)},
		}
		res := readSurface(t, fsys, "m")
		if res.UnknownReason != tfsurface.ReasonModuleFileTooDeep {
			t.Fatalf("UnknownReason = %q, want %q", res.UnknownReason, tfsurface.ReasonModuleFileTooDeep)
		}
	})

	t.Run("syntax-error only, unchanged regression", func(t *testing.T) {
		fsys := fstest.MapFS{
			"m/bad.tf": &fstest.MapFile{Data: []byte(`
output "x" {
  value = 1
`)},
		}
		res := readSurface(t, fsys, "m")
		if res.UnknownReason != tfsurface.ReasonSyntaxError {
			t.Fatalf("UnknownReason = %q, want %q", res.UnknownReason, tfsurface.ReasonSyntaxError)
		}
		if len(res.Diagnostics) != 1 {
			t.Fatalf("len(Diagnostics) = %d, want 1 (%v)", len(res.Diagnostics), res.Diagnostics)
		}
	})
}

// --- G17: ternary nesting and long operator chains -------------------------

// ternaryAlwaysOnN matches the terragrunt package's always-on G17 size:
// the largest ternary count that keeps the file within 1 MiB. No ternary
// file that small crashes hclsyntax (the 02-12 probe put the crash point
// between 500k and 600k levels), so this asserts refusal.
const ternaryAlwaysOnN = (1<<20 - 64) / 4

func TestReadSurfaceDeepTernary(t *testing.T) {
	shapes := map[string]func(int) string{
		"true-chain": func(n int) string { return strings.Repeat("1?", n) + "1" + strings.Repeat(":1", n) },
		"else-chain": func(n int) string { return strings.Repeat("a?b:", n) + "1" },
	}
	for name, chain := range shapes {
		for _, n := range []int{hclconv.MaxNestingDepth + 1, ternaryAlwaysOnN} {
			t.Run(fmt.Sprintf("%s n=%d", name, n), func(t *testing.T) {
				fsys := fstest.MapFS{
					"mod/main.tf": &fstest.MapFile{Data: []byte("output \"x\" {\n  value = " + chain(n) + "\n}\n")},
				}
				res := readSurface(t, fsys, "mod")
				if res.UnknownReason != tfsurface.ReasonModuleFileTooDeep {
					t.Fatalf("UnknownReason = %q, want %q", res.UnknownReason, tfsurface.ReasonModuleFileTooDeep)
				}
				if len(res.Diagnostics) != 0 {
					t.Fatalf("Diagnostics = %v, want none", res.Diagnostics)
				}
			})
		}
	}
}

func TestReadSurfaceLongChain(t *testing.T) {
	fsys := fstest.MapFS{
		"mod/main.tf": &fstest.MapFile{Data: []byte("output \"x\" {\n  value = " + strings.Repeat("1+", hclconv.MaxExpressionChain+1) + "1\n}\n")},
	}
	res := readSurface(t, fsys, "mod")
	if res.UnknownReason != tfsurface.ReasonModuleFileTooDeep {
		t.Fatalf("UnknownReason = %q, want %q", res.UnknownReason, tfsurface.ReasonModuleFileTooDeep)
	}
}
