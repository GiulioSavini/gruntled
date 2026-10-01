package presenter

import (
	"reflect"
	"strings"
	"testing"

	"github.com/GiulioSavini/gruntled/internal/domain/diagnostic"
)

func TestPercentEncode(t *testing.T) {
	cases := []struct{ in, want string }{
		{"terragrunt.hcl", "terragrunt.hcl"},
		{"a/b c/é.hcl", "a/b%20c/%C3%A9.hcl"},
		{"live/app/terragrunt.hcl", "live/app/terragrunt.hcl"},
		{"AZaz09-._~", "AZaz09-._~"},
		{"100%", "100%25"},
		{"a#b?c", "a%23b%3Fc"},
		{"x+y=z", "x%2By%3Dz"},
		{"", ""},
	}
	for _, c := range cases {
		if got := percentEncode(c.in); got != c.want {
			t.Errorf("percentEncode(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestSarifRuleTableOrder(t *testing.T) {
	var ids, names []string
	for _, r := range sarifRuleTable {
		ids = append(ids, string(r.code))
		names = append(names, r.name)
	}
	wantIDs := []string{"GRT001", "GRT002", "GRT003", "GRT100"}
	if !reflect.DeepEqual(ids, wantIDs) {
		t.Fatalf("rule ids = %v, want %v", ids, wantIDs)
	}
	wantNames := []string{"DependencyOutputNotDeclared", "DependencyTargetHasNoUnit", "DependencyCycle", "HclSyntaxError"}
	if !reflect.DeepEqual(names, wantNames) {
		t.Fatalf("rule names = %v, want %v", names, wantNames)
	}
}

func TestSarifRuleTableText(t *testing.T) {
	wantURIs := []string{
		"https://github.com/GiulioSavini/gruntled/blob/master/docs/cli.md#grt001-dependency-output-not-declared-error",
		"https://github.com/GiulioSavini/gruntled/blob/master/docs/cli.md#grt002-dependency-target-has-no-unit-error",
		"https://github.com/GiulioSavini/gruntled/blob/master/docs/cli.md#grt003-dependency-cycle-error",
		"https://github.com/GiulioSavini/gruntled/blob/master/docs/cli.md#grt100-hcl-syntax-error-error",
	}
	rules := sarifRules()
	if len(rules) != len(wantURIs) {
		t.Fatalf("sarifRules() has %d rules, want %d", len(rules), len(wantURIs))
	}
	for i, r := range rules {
		if r.HelpURI != wantURIs[i] {
			t.Errorf("rule %s helpUri = %q, want %q", r.ID, r.HelpURI, wantURIs[i])
		}
		if r.DefaultConfiguration.Level != "error" {
			t.Errorf("rule %s default level = %q, want error", r.ID, r.DefaultConfiguration.Level)
		}
		if !reflect.DeepEqual(r.Properties.Tags, []string{"terragrunt", "correctness"}) || r.Properties.Precision != "very-high" {
			t.Errorf("rule %s properties = %+v", r.ID, r.Properties)
		}
		for _, s := range []string{r.ShortDescription.Text, r.FullDescription.Text, r.Help.Text} {
			if s == "" || len(s) >= 1024 || strings.TrimSpace(s) != s {
				t.Errorf("rule %s text %q: empty, untrimmed or >= 1024 chars", r.ID, s)
			}
		}
	}
}

func TestRuleIndexOf(t *testing.T) {
	for i, code := range []diagnostic.Code{diagnostic.CodeUnknownOutput, diagnostic.CodeMissingDependencyTarget, diagnostic.CodeDependencyCycle, diagnostic.CodeSyntaxError} {
		got, err := ruleIndexOf(code)
		if err != nil || got != i {
			t.Errorf("ruleIndexOf(%s) = %d, %v; want %d, nil", code, got, err, i)
		}
	}
	if _, err := ruleIndexOf(diagnostic.Code("GRT999")); err == nil {
		t.Fatal("ruleIndexOf(GRT999) returned no error")
	}
}
