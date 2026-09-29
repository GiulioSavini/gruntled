package terragrunt

import (
	"testing"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"

	"github.com/GiulioSavini/gruntled/internal/domain/repograph"
)

// generateContents parses a generate block and returns its contents
// expression, so the table exercises real heredoc and quoted forms.
func generateContents(t *testing.T, src string) hcl.Expression {
	t.Helper()
	f, diags := hclsyntax.ParseConfig([]byte(src), "t.hcl", hcl.InitialPos)
	if diags.HasErrors() {
		t.Fatalf("ParseConfig: %v", diags)
	}
	for _, b := range f.Body.(*hclsyntax.Body).Blocks {
		if b.Type == "generate" {
			if a, ok := b.Body.Attributes["contents"]; ok {
				return a.Expr
			}
			return nil
		}
	}
	t.Fatalf("no generate block in %q", src)
	return nil
}

// heredoc wraps text in a generate block whose contents is a heredoc.
func heredoc(text string) string {
	return "generate \"o\" {\n  path = \"o.tf\"\n  contents = <<EOT\n" + text + "\nEOT\n}\n"
}

func TestGenerateMayDeclareOutputsLiteral(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want bool
	}{
		{"g19 comment prefix", heredoc(`/* generated */ output "id" { value = 1 }`), true},
		{"one line after locals", heredoc(`locals { a = 1 } output "id" {}`), true},
		{"line start", heredoc(`  output "x" {`), true},
		{"json", heredoc(`{"output": {"id": {"value": 1}}}`), true},
		{"json unicode escape", heredoc(`{"output": {"id": {"value": 1}}}`), true},
		{"provider only", heredoc("provider \"aws\" {\n  region = \"eu-west-1\"\n}"), false},
		{"backend only", heredoc("terraform {\n  backend \"s3\" {}\n}"), false},
		{"comment mentioning outputs", heredoc(`# outputs live elsewhere`), true},
		{"quoted provider", "generate \"o\" {\n  contents = \"provider \\\"aws\\\" {}\"\n}\n", false},
		{"interpolation", "generate \"o\" {\n  contents = \"x ${local.y}\"\n}\n", true},
		{"function call", "generate \"o\" {\n  contents = file(\"x.tf\")\n}\n", true},
		{"no contents", "generate \"o\" {\n  path = \"o.tf\"\n}\n", true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := generateMayDeclareOutputs(generateContents(t, c.src)); got != c.want {
				t.Fatalf("generateMayDeclareOutputs = %v, want %v", got, c.want)
			}
		})
	}
}

// TestGenerateOutputG19Repro is the 02-REVIEW G19 repro end to end: a
// comment before the output block defeated the old line-start regex, so vpc
// resolved against a module without "id" and app got a false GRT001.
func TestGenerateOutputG19Repro(t *testing.T) {
	res := build(t, filesFS(map[string]string{
		"modules/vpc/main.tf": `output "other" { value = 1 }`,
		"live/vpc/terragrunt.hcl": "terraform { source = \"../../modules/vpc\" }\n" +
			"generate \"o\" {\n" +
			"  path      = \"o.tf\"\n" +
			"  if_exists = \"overwrite\"\n" +
			"  contents  = <<EOT\n" +
			"/* generated */ output \"id\" { value = 1 }\n" +
			"EOT\n" +
			"}\n",
		"live/app/terragrunt.hcl": "dependency \"vpc\" { config_path = \"../vpc\" }\n" +
			"inputs = { id = dependency.vpc.outputs.id }\n",
		"live/app/main.tf": "",
	}))
	vpc, ok := res.Graph.Unit(repograph.MustRepoPath("live/vpc"))
	if !ok {
		t.Fatalf("unit %q not found", "live/vpc")
	}
	if vpc.Status() != repograph.StatusModuleUnknown || vpc.UnknownReason() != ReasonGenerateMayDeclareOutputs {
		t.Fatalf("live/vpc = (%v, %q), want (%v, %q)", vpc.Status(), vpc.UnknownReason(), repograph.StatusModuleUnknown, ReasonGenerateMayDeclareOutputs)
	}
	if n := missingOutputs(res); n != 0 {
		t.Fatalf("references missing their declared output = %d, want 0", n)
	}
}
