package main

// MORE-07: GRT004 exists only in the blast view. check (text, json, sarif),
// report through a live daemon, graph --json and the watch status file
// never print it, on trees where blast does.

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rogpeppe/go-internal/txtar"
)

// more07App reads dependency.db.outputs.id on lines 8 and 9.
const more07App = `terraform {
  source = "../../modules/app"
}
dependency "db" {
  config_path = "../db"
}
inputs = {
  a = dependency.db.outputs.id
  b = dependency.db.outputs.id
}
`

// more07Tree is live/app -> live/db (modules/vpc). vpcOutputs is modules/vpc's
// main.tf. live/orphan adds an unrelated GRT002.
func more07Tree(vpcOutputs string) map[string]string {
	return map[string]string{
		"live/app/terragrunt.hcl":    more07App,
		"live/db/terragrunt.hcl":     "terraform {\n  source = \"../../modules/vpc\"\n}\n",
		"live/orphan/terragrunt.hcl": "terraform {\n  source = \"../../modules/app\"\n}\ndependency \"gone\" {\n  config_path = \"../gone\"\n}\n",
		"modules/app/main.tf":        "variable \"a\" {}\nvariable \"b\" {}\n",
		"modules/vpc/main.tf":        vpcOutputs,
	}
}

func TestNoGRT004OutsideBlast(t *testing.T) {
	// shortBase, not t.TempDir: the test name holds "GRT004", which would
	// leak into any output that echoes a path (the blast baseline line).
	base, cur := shortBase(t), shortBase(t)
	writeFiles(t, base, more07Tree("output \"id\" { value = \"x\" }\noutput \"name\" { value = \"x\" }\n"))
	writeFiles(t, cur, more07Tree("output \"name\" { value = \"x\" }\n"))

	// Non-vacuity: the pair is GRT004-shaped.
	out, stderr, code := runCLI(t, "blast", "--base", base, cur)
	if code != exitFindings || strings.Count(out, "GRT004") != 2 || strings.Contains(out, "GRT001") {
		t.Fatalf("blast: exit %d, want 1 with two GRT004 and no GRT001\nstdout:\n%s\nstderr:\n%s", code, out, stderr)
	}

	checks := map[string]cliResult{}
	for _, f := range reportFormats {
		r := runCheckAt(f, cur)
		checks[f] = r
		if r.code != exitFindings {
			t.Fatalf("check %s: %s", f, r)
		}
		if strings.Contains(r.stdout, "GRT004") {
			t.Errorf("check %s prints GRT004:\n%s", f, r.stdout)
		}
		if n := strings.Count(r.stdout, "GRT001"); n < 2 {
			t.Errorf("check %s: %d GRT001, want both sites:\n%s", f, n, r.stdout)
		}
	}
	for _, site := range []string{"live/app/terragrunt.hcl:8:7: GRT001", "live/app/terragrunt.hcl:9:7: GRT001"} {
		if !strings.Contains(checks["text"].stdout, site) {
			t.Errorf("check text lacks %q:\n%s", site, checks["text"].stdout)
		}
	}
	if !strings.Contains(checks["text"].stdout, "GRT002") {
		t.Errorf("check text lacks the unrelated GRT002:\n%s", checks["text"].stdout)
	}

	// SARIF rule table is exactly the four check rules.
	var log jsonObj
	if err := json.Unmarshal([]byte(checks["sarif"].stdout), &log); err != nil {
		t.Fatalf("sarif: %v", err)
	}
	runs := arr(t, log["runs"], "runs")
	rules := arr(t, obj(t, obj(t, obj(t, runs[0], "run")["tool"], "tool")["driver"], "driver")["rules"], "rules")
	if len(rules) != len(sarifRuleNames) {
		t.Fatalf("sarif rules = %d, want %d", len(rules), len(sarifRuleNames))
	}
	for i, r := range rules {
		if id := str(t, obj(t, r, "rule")["id"], "id"); id != sarifRuleNames[i][0] {
			t.Errorf("rule %d = %s, want %s", i, id, sarifRuleNames[i][0])
		}
	}

	gout, gerr, gcode := runCLI(t, "graph", "--json", cur)
	if gcode != exitOK || strings.Contains(gout, "GRT004") {
		t.Errorf("graph --json: exit %d, GRT004 present = %v; stderr:\n%s", gcode, strings.Contains(gout, "GRT004"), gerr)
	}

	for _, goos := range reportGOOS {
		t.Run("report "+goos, func(t *testing.T) {
			deps := testDeps(t)
			deps.goos = goos
			d := startWatch(t, cur, deps, "--poll")
			eventually(t, func() bool { return runReportAt(deps.env, goos, "text", cur).code == exitFindings },
				"first report", func() string { return runReportAt(deps.env, goos, "text", cur).String() })
			for _, f := range reportFormats {
				got := runReportAt(deps.env, goos, f, cur)
				if got != checks[f] {
					t.Fatalf("report %s differs from check\nreport %s\ncheck %s", f, got, checks[f])
				}
				if strings.Contains(got.stdout, "GRT004") {
					t.Errorf("report %s prints GRT004", f)
				}
			}
			eventually(t, func() bool { return strings.Contains(d.readStatus(), "GRT001×2") }, "status GRT001×2",
				func() string { return d.readStatus() })
			if st := d.readStatus(); strings.Contains(st, "GRT004") {
				t.Errorf("status file holds GRT004: %q", st)
			}
		})
	}
}

// TestCheckCannotProduceGRT004: check never prints GRT004 on any tree of
// blast_grt004.txtar, where blast does.
func TestCheckCannotProduceGRT004(t *testing.T) {
	ar, err := txtar.ParseFile(filepath.Join("testdata", "script", "blast_grt004.txtar"))
	if err != nil {
		t.Fatal(err)
	}
	root := shortBase(t)
	trees := map[string]bool{}
	for _, f := range ar.Files {
		top, _, ok := strings.Cut(f.Name, "/")
		if !ok || !(strings.HasPrefix(top, "base") || strings.HasPrefix(top, "cur")) {
			continue
		}
		trees[top] = true
		p := filepath.Join(root, filepath.FromSlash(f.Name))
		mustDo(t, os.MkdirAll(filepath.Dir(p), 0o755))
		mustDo(t, os.WriteFile(p, f.Data, 0o644))
	}
	if len(trees) < 20 {
		t.Fatalf("only %d trees extracted from blast_grt004.txtar", len(trees))
	}
	for tree := range trees {
		r := runCheckAt("json", filepath.Join(root, tree))
		if r.code != exitOK && r.code != exitFindings {
			t.Errorf("%s: %s", tree, r)
		}
		if strings.Contains(r.stdout, `"GRT004"`) {
			t.Errorf("%s: check prints GRT004:\n%s", tree, r.stdout)
		}
	}
}
