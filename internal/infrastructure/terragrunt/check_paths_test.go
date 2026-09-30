package terragrunt

import (
	"context"
	"slices"
	"testing"

	"github.com/GiulioSavini/gruntled/internal/application/checking"
	"github.com/GiulioSavini/gruntled/internal/infrastructure/tfsurface"
)

// TestPathDepsEndToEnd runs `dependencies { paths }` through the real loader,
// indexing.Build and checking.Check, pinning the GRT002/GRT003 output. It
// lives here, not in internal/application/checking, because the
// architecture rule infrastructure-importers forbids application packages
// (tests included) from importing this package.
func TestPathDepsEndToEnd(t *testing.T) {
	cases := []struct {
		name  string
		files map[string]string
		want  []string
	}{
		{
			name:  "missing target",
			files: map[string]string{"x/terragrunt.hcl": `dependencies { paths = ["../nodir"] }`},
			want: []string{
				`GRT002 x/terragrunt.hcl:1:25 dependencies path "../nodir" resolves to "nodir": directory does not exist`,
			},
		},
		{
			name: "mutual paths",
			files: map[string]string{
				"p/terragrunt.hcl": `dependencies { paths = ["../q"] }`,
				"q/terragrunt.hcl": `dependencies { paths = ["../p"] }`,
			},
			want: []string{`GRT003 p/terragrunt.hcl:1:25 dependency cycle: "p" -> "q" -> "p"`},
		},
		{
			name: "block and paths to same target",
			files: map[string]string{
				"s/terragrunt.hcl":   "dependencies { paths = [\"../acm\"] }\ndependency \"acm\" { config_path = \"../acm\" }\n",
				"acm/terragrunt.hcl": "",
			},
			want: nil,
		},
		{
			name: "block and paths to same target, closed",
			files: map[string]string{
				"s/terragrunt.hcl":   "dependencies { paths = [\"../acm\"] }\ndependency \"acm\" { config_path = \"../acm\" }\n",
				"acm/terragrunt.hcl": `dependency "s" { config_path = "../s" }`,
			},
			want: []string{`GRT003 acm/terragrunt.hcl:1:32 dependency cycle: "acm" -> "s" -> "acm"`},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fsys := filesFS(tc.files)
			rep, err := checking.Check(context.Background(), NewLoader(fsys), tfsurface.NewReader(fsys))
			if err != nil {
				t.Fatalf("Check: %v", err)
			}
			var got []string
			for _, d := range rep.Diagnostics.All() {
				got = append(got, string(d.Code())+" "+d.Pos().String()+" "+d.Message())
			}
			if !slices.Equal(got, tc.want) {
				t.Errorf("diagnostics:\n got  %q\n want %q", got, tc.want)
			}
		})
	}
}
