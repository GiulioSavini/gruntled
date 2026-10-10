package hclconv_test

import (
	"testing"

	"github.com/GiulioSavini/gruntled/internal/domain/repograph"
	"github.com/GiulioSavini/gruntled/internal/infrastructure/hclconv"
	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/hashicorp/hcl/v2/json"
)

func TestLiteralBool(t *testing.T) {
	native := []struct {
		expr string
		want repograph.Tristate
	}{
		{"true", repograph.TristateTrue},
		{"false", repograph.TristateFalse},
		{`"true"`, repograph.TristateUnknown},
		{"var.x", repograph.TristateUnknown},
		{"local.y", repograph.TristateUnknown},
		{"local.x", repograph.TristateUnknown},
		{"null", repograph.TristateUnknown},
		{"1", repograph.TristateUnknown},
		{`upper("x")`, repograph.TristateUnknown},
	}
	for _, tc := range native {
		t.Run(tc.expr, func(t *testing.T) {
			expr, diags := hclsyntax.ParseExpression([]byte(tc.expr), "test.hcl", hcl.InitialPos)
			if diags.HasErrors() {
				t.Fatalf("parse %q: %v", tc.expr, diags)
			}
			if got := hclconv.LiteralBool(expr); got != tc.want {
				t.Fatalf("LiteralBool(%q) = %v, want %v", tc.expr, got, tc.want)
			}
		})
	}

	jsonCases := []struct {
		src  string
		want repograph.Tristate
	}{
		{`{"a": true}`, repograph.TristateTrue},
		{`{"a": false}`, repograph.TristateFalse},
		{`{"a": "true"}`, repograph.TristateUnknown},
	}
	for _, tc := range jsonCases {
		t.Run("json "+tc.src, func(t *testing.T) {
			f, diags := json.Parse([]byte(tc.src), "main.tf.json")
			if diags.HasErrors() {
				t.Fatalf("parse: %v", diags)
			}
			attrs, diags := f.Body.JustAttributes()
			if diags.HasErrors() {
				t.Fatalf("attributes: %v", diags)
			}
			if got := hclconv.LiteralBool(attrs["a"].Expr); got != tc.want {
				t.Fatalf("LiteralBool(%s) = %v, want %v", tc.src, got, tc.want)
			}
		})
	}
}
