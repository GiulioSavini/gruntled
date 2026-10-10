package tfsurface_test

import (
	"context"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/GiulioSavini/gruntled/internal/domain/repograph"
	"github.com/GiulioSavini/gruntled/internal/infrastructure/tfsurface"
)

// TestNestedTemplateDirectiveModule: a module .tf with nested %{if}/%{for}
// directives gets an unknown surface (module-file-too-deep) instead of a
// fatal stack overflow (sec #261, #271); a sibling module is unaffected.
// The hostile sources are built here, never committed.
func TestNestedTemplateDirectiveModule(t *testing.T) {
	cases := []struct {
		name string
		tf   string
	}{
		{"270,000 nested if (sec #261)", "variable \"v\" {\n  default = \"" + strings.Repeat("%{if a}", 270_000) + "x" + strings.Repeat("%{endif}", 270_000) + "\"\n}\n"},
		{"170,000 newline-for (sec #271)", "variable \"v\" {\n  default = \"" + strings.Repeat("%{\nfor x in[1]}", 170_000) + "x" + strings.Repeat("%{endfor}", 170_000) + "\"\n}\n"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			fsys := fstest.MapFS{
				"modules/bad/main.tf":  {Data: []byte(c.tf)},
				"modules/good/main.tf": {Data: []byte("variable \"a\" {}\noutput \"o\" { value = \"x\" }\n")},
			}
			r := tfsurface.NewReader(fsys)
			start := time.Now()
			res, err := r.ReadSurface(context.Background(), repograph.MustRepoPath("modules/bad"))
			took := time.Since(start)
			t.Logf("%d bytes read in %v", len(c.tf), took)
			if err != nil {
				t.Fatalf("ReadSurface: %v", err)
			}
			if took > 10*time.Second {
				t.Fatalf("took %v, over the 10s bound", took)
			}
			if res.UnknownReason != tfsurface.ReasonModuleFileTooDeep {
				t.Fatalf("UnknownReason = %q, want %q", res.UnknownReason, tfsurface.ReasonModuleFileTooDeep)
			}
			good, err := r.ReadSurface(context.Background(), repograph.MustRepoPath("modules/good"))
			if err != nil || good.UnknownReason != "" || !good.Surface.HasOutput("o") || !good.Surface.HasVariable("a") {
				t.Fatalf("sibling module changed: %+v, %v", good, err)
			}
		})
	}
}
