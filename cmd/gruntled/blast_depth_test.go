package main

import (
	"encoding/json"
	"strings"
	"testing"
)

// chainTree is live/a (modules/m) <- live/b <- live/c <- live/d, each
// dependency a default block that reads no output. outputs is modules/m's
// main.tf.
func chainTree(outputs string) map[string]string {
	dep := func(name, to string) string {
		return "terraform {\n  source = \"../../modules/app\"\n}\ndependency \"" + name + "\" {\n  config_path = \"../" + to + "\"\n}\n"
	}
	return map[string]string{
		"live/a/terragrunt.hcl": "terraform {\n  source = \"../../modules/m\"\n}\n",
		"live/b/terragrunt.hcl": dep("a", "a"),
		"live/c/terragrunt.hcl": dep("b", "b"),
		"live/d/terragrunt.hcl": dep("c", "c"),
		"modules/m/main.tf":     outputs,
		"modules/app/main.tf":   "# no outputs\n",
	}
}

func impactedUnits(out string) []string {
	var units []string
	_, imp, _ := strings.Cut(out, "Impacted (")
	for _, l := range strings.Split(imp, "\n")[1:] {
		if strings.HasPrefix(l, "  ") {
			units = append(units, strings.Fields(l)[0])
		}
	}
	return units
}

func TestBlastDepthFlag(t *testing.T) {
	base, cur := shortBase(t), shortBase(t)
	writeFiles(t, base, chainTree("output \"id\" { value = \"x\" }\noutput \"gone\" { value = \"y\" }\n"))
	writeFiles(t, cur, chainTree("output \"id\" { value = \"x\" }\n"))

	blast := func(args ...string) string {
		t.Helper()
		out, stderr, code := runCLI(t, append([]string{"blast", "--base", base}, args...)...)
		if code != exitOK {
			t.Fatalf("blast %v: exit %d, stderr:\n%s", args, code, stderr)
		}
		return out
	}
	for _, c := range []struct {
		args []string
		want string
	}{
		{[]string{cur}, "live/a live/b live/c live/d"},
		{[]string{"--depth", "1", cur}, "live/a"},
		{[]string{"--depth", "2", cur}, "live/a live/b"},
		{[]string{"--depth", "3", cur}, "live/a live/b live/c"},
	} {
		if got := strings.Join(impactedUnits(blast(c.args...)), " "); got != c.want {
			t.Errorf("blast %v: Impacted %q, want %q", c.args, got, c.want)
		}
	}
	if a, b := blast("--depth=3", cur), blast(cur, "--depth", "3"); a != b {
		t.Errorf("--depth=3 before the path and --depth 3 after it differ:\n%s\n%s", a, b)
	}
	if !strings.Contains(blast(cur), "  live/d (distance 4, from module modules/m, path live/d -> live/c -> live/b -> live/a)") {
		t.Errorf("unlimited blast lacks live/d at distance 4")
	}

	var doc struct {
		Impacted []struct {
			Unit     string `json:"unit"`
			Distance int    `json:"distance"`
		} `json:"impacted"`
	}
	if err := json.Unmarshal([]byte(blast("--format", "json", "--depth", "2", cur)), &doc); err != nil {
		t.Fatal(err)
	}
	if len(doc.Impacted) != 2 {
		t.Errorf("json --depth 2: %d entries, want 2", len(doc.Impacted))
	}
	for _, e := range doc.Impacted {
		if e.Distance > 2 {
			t.Errorf("json --depth 2 lists %s at distance %d", e.Unit, e.Distance)
		}
	}

	// Without --base, --depth is accepted and changes nothing.
	plain, _, code := runCLI(t, "blast", cur)
	withDepth, _, code2 := runCLI(t, "blast", "--depth", "1", cur)
	if code != code2 || plain != withDepth {
		t.Errorf("--depth without --base changed the output:\n%s\n%s", plain, withDepth)
	}
}
