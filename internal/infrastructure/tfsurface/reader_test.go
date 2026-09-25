package tfsurface_test

import (
	"context"
	"io/fs"
	"testing"
	"testing/fstest"

	"github.com/GiulioSavini/gruntled/internal/application/ports"
	"github.com/GiulioSavini/gruntled/internal/domain/repograph"
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
