package hclconv_test

import (
	"testing"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"

	"github.com/GiulioSavini/gruntled/internal/domain/diagnostic"
	"github.com/GiulioSavini/gruntled/internal/infrastructure/hclconv"
)

// TestGRT100MessageStability pins the assumption blast radius relies on: a
// GRT100 message never embeds a line, column or file name, so the same
// syntax error shifted down by a few lines keeps a byte-identical message.
// Only the Position may differ. If an hcl upgrade starts embedding
// positions, this test fails and impact.KeyOf must stop using Message for
// GRT100.
func TestGRT100MessageStability(t *testing.T) {
	const shift = "\n# comment\n\n"
	cases := map[string]string{
		"unclosed block":         "terraform {\n  source = \"x\"\n",
		"missing equals":         "inputs {\n  name \"x\"\n}\n",
		"unterminated string":    "a = \"abc\n",
		"bad attribute value":    "a = = 1\n",
		"unexpected close brace": "}\n",
	}
	for name, src := range cases {
		t.Run(name, func(t *testing.T) {
			base := grt100(t, []byte(src))
			moved := grt100(t, []byte(shift+src))
			if base.Message() != moved.Message() {
				t.Fatalf("message changed with line shift:\n base:  %q\n moved: %q", base.Message(), moved.Message())
			}
			if base.Pos().Line() == moved.Pos().Line() {
				t.Fatalf("expected line to move, both at %d", base.Pos().Line())
			}
		})
	}
}

func grt100(t *testing.T, src []byte) diagnostic.Diagnostic {
	t.Helper()
	_, diags := hclsyntax.ParseConfig(src, "test.hcl", hcl.InitialPos)
	if !diags.HasErrors() {
		t.Fatalf("expected parse errors for %q", src)
	}
	d, ok, err := hclconv.FirstSyntaxError(mustFile(t), src, diags)
	if err != nil || !ok {
		t.Fatalf("FirstSyntaxError: ok=%v err=%v", ok, err)
	}
	return d
}
