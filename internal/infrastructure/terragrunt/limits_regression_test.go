package terragrunt_test

import (
	"context"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/GiulioSavini/gruntled/internal/application/ports"
	"github.com/GiulioSavini/gruntled/internal/infrastructure/terragrunt"
)

// hostileBound is the test bound for loading one hostile ~4 MB file.
const hostileBound = 10 * time.Second

// TestNestedTemplateDirectiveUnit: nested %{if}/%{for} directives in a unit
// make that unit config-unknown (config-too-deep) instead of driving HCL
// evaluation into a fatal stack overflow (sec #261, #271). The hostile
// sources are built here, never committed.
func TestNestedTemplateDirectiveUnit(t *testing.T) {
	quoted := func(open, inner, close string, n int) string {
		return "dependency \"b\" {\n  config_path = \"" + strings.Repeat(open, n) + inner + strings.Repeat(close, n) + "\"\n}\n"
	}
	cases := []struct {
		name string
		hcl  string
	}{
		{"270,000 nested if (sec #261)", quoted("%{if a}", "../b", "%{endif}", 270_000)},
		{"1,001 nested if", quoted("%{if a}", "../b", "%{endif}", 1_001)},
		{"170,000 newline-for (sec #271)", quoted("%{\nfor x in[1]}", "../b", "%{endfor}", 170_000)},
		{"heredoc nested for", "inputs = {\n  x = <<EOT\n" + strings.Repeat("%{for v in l}\n", 150_000) + "x\n" + strings.Repeat("%{endfor}\n", 150_000) + "EOT\n}\n"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			fsys := fstest.MapFS{
				"live/a/terragrunt.hcl": {Data: []byte(c.hcl)},
				"live/b/terragrunt.hcl": {Data: []byte("")},
				"live/b/main.tf":        {Data: []byte("output \"o\" { value = \"x\" }\n")},
				"live/c/terragrunt.hcl": {Data: []byte("dependency \"b\" {\n  config_path = \"../b\"\n}\ninputs = { v = dependency.b.outputs.missing }\n")},
				"live/c/main.tf":        {Data: []byte("variable \"v\" {}\n")},
			}
			start := time.Now()
			res, err := terragrunt.NewLoader(fsys).LoadUnits(context.Background())
			took := time.Since(start)
			t.Logf("%d bytes loaded in %v", len(c.hcl), took)
			if err != nil {
				t.Fatalf("LoadUnits: %v", err)
			}
			if took > hostileBound {
				t.Fatalf("took %v, over the %v bound", took, hostileBound)
			}
			units := map[string]ports.UnitConfig{}
			for _, u := range res.Units {
				units[u.Path.String()] = u
			}
			if got := units["live/a"].ConfigUnknownReason; got != terragrunt.ReasonConfigTooDeep {
				t.Fatalf("live/a reason = %q, want %q", got, terragrunt.ReasonConfigTooDeep)
			}
			cu := units["live/c"]
			if cu.ConfigUnknownReason != "" || len(cu.Dependencies) != 1 || len(cu.References) != 1 ||
				cu.References[0].Output() != "missing" {
				t.Fatalf("live/c changed: %+v", cu)
			}
			if target, ok := cu.Dependencies[0].Target(); !ok || target.String() != "live/b" {
				t.Fatalf("live/c dependency target = %v (%v)", target, ok)
			}
			if units["live/b"].ConfigUnknownReason != "" {
				t.Fatalf("live/b became unknown: %+v", units["live/b"])
			}
		})
	}
}
