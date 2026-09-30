package terragrunt

import (
	"io/fs"
	"testing"
	"testing/fstest"

	"github.com/GiulioSavini/gruntled/internal/domain/repograph"
)

func TestTargetStateMapFS(t *testing.T) {
	fsys := fstest.MapFS{
		"empty":                           &fstest.MapFile{Mode: fs.ModeDir | 0o755},
		"unit/terragrunt.hcl":             &fstest.MapFile{Data: []byte("")},
		"jsonunit/terragrunt.hcl.json":    &fstest.MapFile{Data: []byte("{}")},
		"upper/Terragrunt.hcl":            &fstest.MapFile{Data: []byte("")},
		"stack/terragrunt.stack.hcl":      &fstest.MapFile{Data: []byte("")},
		"other/root.hcl":                  &fstest.MapFile{Data: []byte("")},
		"other/main.tf":                   &fstest.MapFile{Data: []byte("")},
		"dirnamed/terragrunt.hcl/keep.tf": &fstest.MapFile{Data: []byte("")},
		"afile":                           &fstest.MapFile{Data: []byte("x")},
		"terragrunt.hcl":                  &fstest.MapFile{Data: []byte("")},
	}
	l := NewLoader(fsys)
	tests := []struct {
		dir  string
		want repograph.TargetState
	}{
		{"missing", repograph.TargetDirMissing},
		{"missing/deeper", repograph.TargetDirMissing},
		{"empty", repograph.TargetNoConfig},
		{"unit", repograph.TargetHasConfig},
		{"jsonunit", repograph.TargetHasConfig},
		{"upper", repograph.TargetNoConfig},
		{"stack", repograph.TargetNoConfig},
		{"other", repograph.TargetNoConfig},
		{"dirnamed", repograph.TargetNoConfig},
		{"afile", repograph.TargetUnknown},
		{".", repograph.TargetHasConfig},
	}
	for _, tt := range tests {
		t.Run(tt.dir, func(t *testing.T) {
			if got := l.classifyTarget(tt.dir); got != tt.want {
				t.Errorf("classifyTarget(%q) = %v, want %v", tt.dir, got, tt.want)
			}
		})
	}
}
