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

func TestBlockTargetState(t *testing.T) {
	fsys := filesFS(map[string]string{
		"u/terragrunt.hcl": `
dependency "miss" { config_path = "../nodir" }
dependency "unit" { config_path = "../vpc" }
dependency "tf" { config_path = "../plain" }
dependency "file" { config_path = "../x/terragrunt.hcl" }
dependency "fileexist" { config_path = "../vpc/terragrunt.hcl" }
dependency "filenocfg" { config_path = "../plain/terragrunt.hcl" }
dependency "stack" { config_path = "../stk" }
dependency "alt" { config_path = "../vpc/alt.hcl" }
dependency "esc" { config_path = "../../../vpc" }
dependency "dyn" { config_path = local.p }
`,
		"vpc/terragrunt.hcl":       "",
		"vpc/alt.hcl":              "",
		"plain/main.tf":            "",
		"stk/terragrunt.stack.hcl": "",
	})
	u := unitByPath(t, loadUnits(t, fsys), "u")

	resolved := []struct {
		name, target string
		state        repograph.TargetState
		line         int
	}{
		{"miss", "nodir", repograph.TargetDirMissing, 2},
		{"unit", "vpc", repograph.TargetHasConfig, 3},
		{"tf", "plain", repograph.TargetNoConfig, 4},
		{"file", "x", repograph.TargetDirMissing, 5},
		{"fileexist", "vpc", repograph.TargetHasConfig, 6},
		{"filenocfg", "plain", repograph.TargetNoConfig, 7},
	}
	for _, tt := range resolved {
		t.Run(tt.name, func(t *testing.T) {
			d, ok := findDep(u.Dependencies, tt.name)
			if !ok {
				t.Fatalf("dependency %q missing", tt.name)
			}
			target, ok := d.Target()
			if !ok {
				t.Fatalf("dependency %q unresolved: %s", tt.name, d.UnresolvedReason())
			}
			if target.String() != tt.target {
				t.Errorf("target = %q, want %q", target, tt.target)
			}
			if d.TargetState() != tt.state {
				t.Errorf("state = %v, want %v", d.TargetState(), tt.state)
			}
			// config_path = "..." value starts right after `config_path = `.
			wantCol := len(`dependency "`+tt.name+`" { config_path = `) + 1
			if p := d.PathPos(); p.Line() != tt.line || p.Column() != wantCol {
				t.Errorf("pathPos = %d:%d, want %d:%d", p.Line(), p.Column(), tt.line, wantCol)
			}
		})
	}

	unresolved := []struct{ name, reason string }{
		{"stack", ReasonConfigPathStack},
		{"alt", ReasonConfigPathNondefaultFile},
		{"esc", ReasonConfigPathOutsideRepo},
		{"dyn", ReasonConfigPathDynamic},
	}
	for _, tt := range unresolved {
		t.Run(tt.name, func(t *testing.T) {
			d, ok := findDep(u.Dependencies, tt.name)
			if !ok {
				t.Fatalf("dependency %q missing", tt.name)
			}
			if _, ok := d.Target(); ok {
				t.Fatalf("dependency %q resolved, want unresolved", tt.name)
			}
			if d.UnresolvedReason() != tt.reason {
				t.Errorf("reason = %q, want %q", d.UnresolvedReason(), tt.reason)
			}
			if d.TargetState() != repograph.TargetUnknown {
				t.Errorf("state = %v, want Unknown", d.TargetState())
			}
		})
	}
}
