package main

// TestSARIFStructure asserts, on real `check --format sarif` output, the
// constraints GitHub code scanning applies when it ingests a SARIF file.
// TestSARIFDoc keeps docs/cli.md in step with the emitted rule table.

import (
	"encoding/json"
	"os"
	"regexp"
	"slices"
	"strings"
	"testing"
)

// sarifAllRules has one finding per rule code, an unknown unit under a path
// with a space and an unknown module.
var sarifAllRules = map[string]string{
	"vpc/terragrunt.hcl":         "",
	"vpc/main.tf":                `output "vpc_id" { value = "x" }` + "\n",
	"my app/terragrunt.hcl":      "dependency \"vpc\" {\n  config_path = \"../vpc\"\n}\ninputs = { id = dependency.vpc.outputs.vpc_idd }\n",
	"my app/main.tf":             "variable \"id\" {}\n",
	"orphan/terragrunt.hcl":      "dependency \"gone\" {\n  config_path = \"../gone\"\n}\n",
	"orphan/main.tf":             "# no outputs\n",
	"ring/a/terragrunt.hcl":      "dependency \"b\" {\n  config_path = \"../b\"\n}\n",
	"ring/a/main.tf":             "# no outputs\n",
	"ring/b/terragrunt.hcl":      "dependency \"a\" {\n  config_path = \"../a\"\n}\n",
	"ring/b/main.tf":             "# no outputs\n",
	"broken/terragrunt.hcl":      "dependency \"vpc\" {\n  config_path = \"../vpc\"\n",
	"remote unit/terragrunt.hcl": "terraform {\n  source = \"git::https://example.com/x.git\"\n}\n",
	"db/terragrunt.hcl":          "terraform {\n  source = \"../modules/missing\"\n}\n",
}

// sarifRuleNames pins the rule table: id, PascalCase name, in code order.
var sarifRuleNames = [][2]string{
	{"GRT001", "DependencyOutputNotDeclared"},
	{"GRT002", "DependencyTargetHasNoUnit"},
	{"GRT003", "DependencyCycle"},
	{"GRT100", "HclSyntaxError"},
}

type jsonObj = map[string]any

func sarifRun(t *testing.T) (log jsonObj, run jsonObj) {
	t.Helper()
	dir := t.TempDir()
	writeFiles(t, dir, sarifAllRules)
	stdout, stderr, code := runCLI(t, "check", "--format", "sarif", dir)
	if code != exitFindings {
		t.Fatalf("exit %d, want %d; stderr:\n%s", code, exitFindings, stderr)
	}
	if stderr != "" {
		t.Fatalf("stderr not empty: %q", stderr)
	}
	if err := json.Unmarshal([]byte(stdout), &log); err != nil {
		t.Fatalf("decode SARIF: %v\n%s", err, stdout)
	}
	runs := arr(t, log["runs"], "runs")
	if len(runs) != 1 {
		t.Fatalf("got %d runs, want exactly 1", len(runs))
	}
	return log, obj(t, runs[0], "runs[0]")
}

func obj(t *testing.T, v any, what string) jsonObj {
	t.Helper()
	m, ok := v.(jsonObj)
	if !ok {
		t.Fatalf("%s: got %T, want object", what, v)
	}
	return m
}

// arr fails on anything but a JSON array, so null is caught.
func arr(t *testing.T, v any, what string) []any {
	t.Helper()
	a, ok := v.([]any)
	if !ok {
		t.Fatalf("%s: got %T, want array", what, v)
	}
	return a
}

func str(t *testing.T, v any, what string) string {
	t.Helper()
	s, ok := v.(string)
	if !ok || s == "" {
		t.Fatalf("%s: got %#v, want non-empty string", what, v)
	}
	return s
}

func checkArtifactLocation(t *testing.T, loc jsonObj, what string) {
	t.Helper()
	al := obj(t, obj(t, loc["physicalLocation"], what+".physicalLocation")["artifactLocation"], what+".artifactLocation")
	uri := str(t, al["uri"], what+".uri")
	if strings.HasPrefix(uri, "/") {
		t.Errorf("%s: uri %q is absolute", what, uri)
	}
	first := strings.Index(uri, "/")
	if first < 0 {
		first = len(uri)
	}
	if strings.Contains(uri[:first], ":") {
		t.Errorf("%s: uri %q has a scheme or drive", what, uri)
	}
	if strings.Contains(uri, " ") || strings.Contains(uri, `\`) {
		t.Errorf("%s: uri %q is not percent-encoded or uses backslashes", what, uri)
	}
	if got := al["uriBaseId"]; got != "%SRCROOT%" {
		t.Errorf("%s: uriBaseId %#v, want %%SRCROOT%%", what, got)
	}
}

func TestSARIFStructure(t *testing.T) {
	log, run := sarifRun(t)

	if v := log["version"]; v != "2.1.0" {
		t.Errorf("version %#v, want 2.1.0", v)
	}
	str(t, log["$schema"], "$schema")
	if _, ok := run["originalUriBaseIds"]; ok {
		t.Error("run has originalUriBaseIds; URIs must stay relative to the checkout")
	}

	driver := obj(t, obj(t, run["tool"], "tool")["driver"], "driver")
	rules := arr(t, driver["rules"], "rules")
	ids := make([]string, len(rules))
	for i, r := range rules {
		ro := obj(t, r, "rule")
		ids[i] = str(t, ro["id"], "rule.id")
		str(t, obj(t, ro["shortDescription"], ids[i]+".shortDescription")["text"], ids[i]+".shortDescription.text")
		str(t, obj(t, ro["help"], ids[i]+".help")["text"], ids[i]+".help.text")
	}

	levels := []string{"error", "warning", "note"}
	results := arr(t, run["results"], "results")
	seen := map[string]bool{}
	for i, r := range results {
		ro := obj(t, r, "result")
		id := str(t, ro["ruleId"], "ruleId")
		seen[id] = true
		idx, ok := ro["ruleIndex"].(float64)
		if !ok || int(idx) < 0 || int(idx) >= len(ids) || ids[int(idx)] != id {
			t.Errorf("result %d: ruleIndex %#v does not index rule %s", i, ro["ruleIndex"], id)
		}
		if !slices.Contains(levels, str(t, ro["level"], "level")) {
			t.Errorf("result %d: level %v not in %v", i, ro["level"], levels)
		}
		locs := arr(t, ro["locations"], "locations")
		if len(locs) == 0 {
			t.Errorf("result %d (%s) has no location", i, id)
		}
		for _, l := range locs {
			lo := obj(t, l, "location")
			checkArtifactLocation(t, lo, id)
			reg := obj(t, obj(t, lo["physicalLocation"], "physicalLocation")["region"], id+".region")
			for _, k := range []string{"startLine", "startColumn"} {
				if n, ok := reg[k].(float64); !ok || n < 1 {
					t.Errorf("result %d (%s): %s = %#v, want >= 1", i, id, k, reg[k])
				}
			}
		}
		if _, ok := ro["partialFingerprints"]; ok {
			t.Errorf("result %d has partialFingerprints", i)
		}
		if _, ok := ro["relatedLocations"]; ok {
			t.Errorf("result %d has relatedLocations", i)
		}
	}
	for _, rn := range sarifRuleNames {
		if !seen[rn[0]] {
			t.Errorf("fixture produced no %s result", rn[0])
		}
	}

	invs := arr(t, run["invocations"], "invocations")
	if len(invs) != 1 {
		t.Fatalf("got %d invocations, want 1", len(invs))
	}
	notes := arr(t, obj(t, invs[0], "invocation")["toolExecutionNotifications"], "toolExecutionNotifications")
	if len(notes) == 0 {
		t.Error("no notifications for the unknown unit and module")
	}
	for i, n := range notes {
		no := obj(t, n, "notification")
		if lv := str(t, no["level"], "notification.level"); lv != "note" {
			t.Errorf("notification %d: level %q, want note", i, lv)
		}
		if v, ok := no["locations"]; ok {
			for _, l := range arr(t, v, "notification.locations") {
				checkArtifactLocation(t, obj(t, l, "notification.location"), "notification")
			}
		}
	}
}

// TestSARIFStructureClean: a repository with no findings still has results
// and notifications as arrays, never null.
func TestSARIFStructureClean(t *testing.T) {
	dir := t.TempDir()
	writeFiles(t, dir, map[string]string{"vpc/terragrunt.hcl": "", "vpc/main.tf": "# empty\n"})
	stdout, _, code := runCLI(t, "check", "--format", "sarif", dir)
	if code != exitOK {
		t.Fatalf("exit %d, want %d", code, exitOK)
	}
	var log jsonObj
	if err := json.Unmarshal([]byte(stdout), &log); err != nil {
		t.Fatal(err)
	}
	run := obj(t, arr(t, log["runs"], "runs")[0], "run")
	if len(arr(t, run["results"], "results")) != 0 {
		t.Error("clean repository has results")
	}
	inv := obj(t, arr(t, run["invocations"], "invocations")[0], "invocation")
	arr(t, inv["toolExecutionNotifications"], "toolExecutionNotifications")
}

var docRuleHeading = regexp.MustCompile(`^### (GRT\d{3}): (.+) \((error|warning|note)\)$`)

func TestSARIFDoc(t *testing.T) {
	b, err := os.ReadFile("../../docs/cli.md")
	if err != nil {
		t.Fatal(err)
	}
	doc := strings.ReplaceAll(string(b), "\r\n", "\n")

	type heading struct{ id, title string }
	var docs []heading
	for l := range strings.SplitSeq(doc, "\n") {
		if m := docRuleHeading.FindStringSubmatch(l); m != nil {
			docs = append(docs, heading{m[1], m[2]})
		}
	}
	slices.SortFunc(docs, func(a, b heading) int { return strings.Compare(a.id, b.id) })

	_, run := sarifRun(t)
	rules := arr(t, obj(t, obj(t, run["tool"], "tool")["driver"], "driver")["rules"], "rules")
	var emitted []heading
	for _, r := range rules {
		ro := obj(t, r, "rule")
		emitted = append(emitted, heading{
			str(t, ro["id"], "id"),
			str(t, obj(t, ro["shortDescription"], "shortDescription")["text"], "shortDescription.text"),
		})
	}
	if !slices.Equal(docs, emitted) {
		t.Errorf("docs/cli.md rule headings %v != emitted rules %v", docs, emitted)
	}

	if len(rules) != len(sarifRuleNames) {
		t.Fatalf("emitted %d rules, want %d", len(rules), len(sarifRuleNames))
	}
	for i, rn := range sarifRuleNames {
		ro := obj(t, rules[i], "rule")
		if ro["id"] != rn[0] || ro["name"] != rn[1] {
			t.Errorf("rule %d = %v/%v, want %s/%s", i, ro["id"], ro["name"], rn[0], rn[1])
		}
	}

	// The SARIF section documents every rule by id and name.
	_, sarifSec, ok := strings.Cut(doc, "\n### SARIF\n")
	if !ok {
		t.Fatal("docs/cli.md has no ### SARIF section")
	}
	sarifSec, _, _ = strings.Cut(sarifSec, "\n## ")
	for _, rn := range sarifRuleNames {
		row := "| `" + rn[0] + "` | `" + rn[1] + "` |"
		if !strings.Contains(sarifSec, row) {
			t.Errorf("SARIF section lacks rule row starting %q", row)
		}
	}
	if !strings.Contains(doc, "\n### Graph JSON\n") {
		t.Error("docs/cli.md has no ### Graph JSON section")
	}
}
