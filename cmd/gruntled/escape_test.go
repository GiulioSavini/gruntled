package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"unicode"
	"unicode/utf8"
)

// Crafted unit directory names a pulled branch could carry.
const (
	// escUnit sets the terminal title (OSC 0 ... BEL) when printed raw.
	escUnit = "a\x1b]0;PWNED\x07b"
	// escUnitText is escUnit as the text presenter prints it.
	escUnitText = `a\x1b]0;PWNED\x07b`
	// bidiUnit reverses the rest of the line (U+202E RIGHT-TO-LEFT OVERRIDE).
	bidiUnit = "r\u202el"
	// c1Unit holds a UTF-8 encoded C1 CSI (U+009B) and DEL.
	c1Unit = "c\u009b2Jd\u007fe"
	// producer is the unit (and module) every consumer reads; its name
	// holds a bidi override too, so it shows up escaped in Impacted.
	producer = "v\u202ep"
	// escBase is the --base directory name; it clears the screen raw.
	escBase = "base\x1b[2Jx"
	// rmUnit reads producer's output "old", which only the baseline
	// declares: blast reports it as GRT004 with producer in the message.
	rmUnit = "rm"
	// rmGRT004 is that GRT004's message. strconv.Quote already writes the
	// U+202E of producer as the 6-byte literal \u202e (sec #134), so this is
	// what both blast text and the decoded blast JSON carry.
	rmGRT004 = `dependency "vpc" output "old" was removed from module "v\u202ep" (target unit "v\u202ep")`
	// deepUnit is clean and depends on c1Unit, which reads the producer: in
	// blast it is Impacted at distance 3 through crafted hops (sec #194).
	// Its config_path spells c1Unit with HCL escapes, so its own file holds
	// no raw control bytes and it never turns config-unknown (sec #208).
	deepUnit = "deep"
	deepHCL  = "dependency \"c\" {\n  config_path = \"../c\\u009b2Jd\\u007fe\"\n}\n"
	// typesUnit is a clean unit whose own module changes type-level facts
	// of crafted names (Phase 15, SEC-01); rawbadUnit's module file holds a
	// raw 0xff byte in the current tree.
	typesUnit  = "types"
	rawbadUnit = "rawbad"
	// The crafted names as decoded, and as escapeTerm prints them.
	typesVar      = "v\x1bx\u009by\u202ez"
	typesVarText  = `v\x1bx\u009by\u202ez`
	typesOut      = "o\x1b\u202e"
	typesOutText  = `o\x1b\u202e`
	typesJSON     = "j\x1b\u202e"
	typesJSONText = `j\x1b\u202e`
	typesFFFD     = "k\ufffd"
)

// consumerOf is a unit that reads output ref of the producer unit.
func consumerOf(ref string) string {
	return "dependency \"vpc\" {\n  config_path = \"../" + producer + "\"\n}\ninputs = { id = dependency.vpc.outputs." + ref + " }\n"
}

// craftedRepo writes a repository with a GRT100 unit at escUnit and GRT001
// consumers at bidiUnit, c1Unit and rmUnit. ok writes the clean variant used
// as a blast baseline: every unit parses and resolves, and the producer
// declares one more output ("old", which rmUnit reads), so its unit is
// Impacted against it and rmUnit's finding is a GRT004 in blast.
func craftedRepo(t *testing.T, dir string, ok bool) {
	t.Helper()
	files := map[string]string{
		producer + "/terragrunt.hcl": "",
		producer + "/main.tf":        "output \"vpc_id\" { value = \"x\" }\n",
		escUnit + "/terragrunt.hcl":  "inputs = {\n",
		bidiUnit + "/terragrunt.hcl": consumerOf("vpc_idd"),
		bidiUnit + "/main.tf":        "variable \"id\" {}\n",
		c1Unit + "/terragrunt.hcl":   consumerOf("vpc_idd"),
		c1Unit + "/main.tf":          "variable \"id\" {}\n",
		rmUnit + "/terragrunt.hcl":   consumerOf("old"),
		rmUnit + "/main.tf":          "variable \"id\" {}\n",
		deepUnit + "/terragrunt.hcl": deepHCL,
		deepUnit + "/main.tf":        "# reads no output\n",
	}
	// Phase 15 (SEC-01): a module with crafted variable and output names
	// whose type-level facts change. The names are written with HCL and
	// JSON \u escapes, so this file holds no raw control or bidi rune.
	// Variable w's object attribute name holds U+202E, which typeexpr
	// rejects (quoted key), so w's type is unknown on both sides.
	files[typesUnit+"/terragrunt.hcl"] = ""
	files[typesUnit+"/main.tf"] = `variable "v\u001bx\u009by\u202ez" { type = number }
output "o\u001b\u202e" {
  value     = 1
  sensitive = true
}
variable "w" { type = object({ "a\u202eb" = number }) }
`
	files[typesUnit+"/vars.tf.json"] = `{"variable": {"j\u001b\u202e": {}}}` + "\n"
	// sec #249a: a raw 0xff byte in a .tf.json key decodes to U+FFFD.
	files[typesUnit+"/bad.tf.json"] = "{\"variable\": {\"k\xff\": {}}}\n"
	// sec #249a: a raw 0xff byte in a .tf is a GRT100; the surface is
	// unknown, so no type change is reported for rawbad.
	files[rawbadUnit+"/terragrunt.hcl"] = ""
	files[rawbadUnit+"/main.tf"] = "variable \"x\" {\n  type    = number\n  default = \"\xff\"\n}\n"
	if ok {
		files[typesUnit+"/main.tf"] = `variable "v\u001bx\u009by\u202ez" { type = string }
output "o\u001b\u202e" { value = 1 }
variable "w" { type = object({ "a\u202eb" = string }) }
`
		delete(files, typesUnit+"/vars.tf.json")
		delete(files, typesUnit+"/bad.tf.json")
		files[rawbadUnit+"/main.tf"] = "variable \"x\" {\n  type = string\n}\n"
		files[producer+"/main.tf"] = "output \"vpc_id\" { value = \"x\" }\noutput \"old\" { value = \"y\" }\n"
		files[escUnit+"/terragrunt.hcl"] = ""
		files[bidiUnit+"/terragrunt.hcl"] = consumerOf("vpc_id")
		files[c1Unit+"/terragrunt.hcl"] = consumerOf("vpc_id")
	}
	writeFiles(t, dir, files)
}

// assertNoRawControls fails when out holds anything a terminal could act
// on: a C0 control other than '\n', DEL, invalid UTF-8, a C1 control
// (including its UTF-8 form 0xc2 0x80-0x9f), a format (Cf) rune or
// U+2028/U+2029.
func assertNoRawControls(t *testing.T, what, out string) {
	t.Helper()
	if !utf8.ValidString(out) {
		t.Fatalf("%s: stdout is not valid UTF-8:\n%q", what, out)
	}
	for _, r := range out {
		if r == '\n' {
			continue
		}
		if r < 0x20 || (r >= 0x7f && r <= 0x9f) || r == 0x2028 || r == 0x2029 || unicode.Is(unicode.Cf, r) {
			t.Fatalf("%s: stdout holds raw %U:\n%q", what, r, out)
		}
	}
}

// TestTextOutputsEscapeControls proves end to end that a crafted directory
// name cannot write control sequences to the terminal through check,
// watch stdout, report, blast (with its --base label) and the JSON, SARIF
// and graph documents, and that report keeps byte parity with check.
func TestTextOutputsEscapeControls(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("control characters are invalid in NTFS file names")
	}
	dir := t.TempDir()
	craftedRepo(t, dir, false)

	check, checkErr, code := runCLI(t, "check", dir)
	if code != exitFindings {
		t.Fatalf("check: exit %d, stderr:\n%s", code, checkErr)
	}
	assertNoRawControls(t, "check", check)
	if strings.Contains(check, "GRT004") {
		t.Errorf("check stdout holds GRT004:\n%s", check)
	}
	for _, want := range []string{
		escUnitText + "/terragrunt.hcl:",
		`r\u202el/terragrunt.hcl:`,
		`c\u009b2Jd\x7fe/terragrunt.hcl:`,
		// check never emits GRT004: the removed output is a GRT001 here
		rmUnit + `/terragrunt.hcl:4:17: GRT001 dependency "vpc" output "old" is not declared by module "v\u202ep" (target unit "v\u202ep")`,
	} {
		if !strings.Contains(check, want) {
			t.Errorf("check stdout lacks %q:\n%s", want, check)
		}
	}

	t.Run("watch", func(t *testing.T) {
		d := startWatch(t, dir, testDeps(t), "--poll")
		eventually(t, func() bool { return d.stdout.String() == check }, "watch stdout == check stdout",
			func() string { return d.stdout.String() + "\nstderr:\n" + d.stderr.String() })
		assertNoRawControls(t, "watch", d.stdout.String())
	})

	t.Run("report", func(t *testing.T) {
		deps := testDeps(t)
		deps.goos = "linux"
		startWatch(t, dir, deps, "--poll")
		var last cliResult
		eventually(t, func() bool {
			last = runReportAt(deps.env, "linux", "text", dir)
			return last.code == exitFindings
		}, "first report", func() string { return last.String() })
		if last.stdout != check {
			t.Fatalf("report text differs from check\nreport:\n%q\ncheck:\n%q", last.stdout, check)
		}
		assertNoRawControls(t, "report", last.stdout)
	})

	base := filepath.Join(t.TempDir(), escBase)
	if err := os.Mkdir(base, 0o755); err != nil {
		t.Fatal(err)
	}
	craftedRepo(t, base, true)

	// Every line blast prints for the crafted names, except the label line
	// (also required of report --blast, whose label differs). The Phase 15
	// type-level tokens and the Type changes section escape every name too.
	blastWant := []string{
		"  " + escUnitText + "/terragrunt.hcl\n",
		"  " + `c\u009b2Jd\x7fe` + "\n",
		"  " + `r\u202el` + "\n",
		`  v\u202ep (distance 1, module v\u202ep: -output old)` + "\n",
		`  deep (distance 3, from module v\u202ep, path deep -> c\u009b2Jd\x7fe -> v\u202ep)` + "\n",
		"  " + rmUnit + "\n",
		"    " + rmUnit + "/terragrunt.hcl:4:17: GRT004 " + rmGRT004 + "\n",
		"type variable " + typesVarText,
		"sensitive output " + typesOutText,
		"required variable " + typesJSONText,
		"required variable " + typesFFFD,
		"\nType changes (",
		"  " + typesUnit + " variable " + typesJSONText + ": now required\n",
		"  " + typesUnit + " variable " + typesFFFD + ": now required\n",
		"  " + typesUnit + " variable " + typesVarText + ": type string -> number\n",
		"  " + typesUnit + " output " + typesOutText + ": sensitive false -> true\n",
		"    " + rawbadUnit + "/main.tf:",
	}

	t.Run("blast", func(t *testing.T) {
		out, stderr, code := runCLI(t, "blast", "--base", base, dir)
		if code != exitFindings {
			t.Fatalf("blast: exit %d, stderr:\n%s", code, stderr)
		}
		assertNoRawControls(t, "blast", out)
		first, _, _ := strings.Cut(out, "\n")
		if want := "baseline: " + filepath.Dir(base) + `/base\x1b[2Jx`; first != want {
			t.Fatalf("blast first line %q, want %q", first, want)
		}
		for _, want := range blastWant {
			if !strings.Contains(out, want) {
				t.Errorf("blast stdout lacks %q:\n%s", want, out)
			}
		}
		// Phase 15: w (unparseable on both sides) and rawbad (unknown
		// surface in cur) report nothing type-level.
		if strings.Contains(out, "variable w") || strings.Contains(out, " "+rawbadUnit+" variable") || strings.Contains(out, "("+"distance 1, module "+rawbadUnit) {
			t.Errorf("blast reports a type change for w or rawbad:\n%s", out)
		}
		if strings.Contains(out, "\n  "+deepUnit+"\n") || strings.Contains(out, deepUnit+"/terragrunt.hcl:") {
			t.Errorf("deep must be Impacted, not Broken:\n%s", out)
		}
		if strings.Contains(out, rmUnit+"/terragrunt.hcl:4:17: GRT001") {
			t.Errorf("blast reports both codes at the rm site:\n%s", out)
		}
	})

	// blastJSONHas holds when a blast document decodes back to the exact
	// crafted names: blast json and report --blast json share it.
	blastJSONHas := func(v any) bool {
		return anyItem(v, "broken", func(m map[string]any) bool { return m["unit"] == c1Unit }) &&
			anyItem(v, "impacted", func(m map[string]any) bool { return m["unit"] == producer && m["module"] == producer }) &&
			anyItem(v, "broken", func(m map[string]any) bool {
				return m["unit"] == rmUnit && anyItem(m, "findings", func(f map[string]any) bool {
					return f["code"] == "GRT004" && f["message"] == rmGRT004
				}) && !anyItem(m, "findings", func(f map[string]any) bool { return f["code"] == "GRT001" })
			}) &&
			anyItem(v, "impacted", func(m map[string]any) bool {
				return m["unit"] == deepUnit && m["distance"] == float64(3) && m["via"] == c1Unit && m["source"] == producer && m["module"] == producer
			}) &&
			!anyItem(v, "broken", func(m map[string]any) bool { return m["unit"] == deepUnit }) &&
			anyItem(v, "changes", func(m map[string]any) bool {
				tc := func(kind, name, old, new string) bool {
					return anyItem(m, "type_changes", func(c map[string]any) bool {
						return c["kind"] == kind && c["name"] == name && c["old"] == old && c["new"] == new
					})
				}
				return m["module"] == typesUnit &&
					tc("variable_type", typesVar, "string", "number") &&
					tc("output_sensitive", typesOut, "false", "true") &&
					tc("variable_required", typesJSON, "absent", "required") &&
					tc("variable_required", typesFFFD, "absent", "required") &&
					!tc("variable_type", "w", "", "")
			}) &&
			!anyItem(v, "changes", func(m map[string]any) bool { return m["module"] == rawbadUnit })
	}

	// JSON documents: every name escaped, decoded back exactly.
	jsonRuns := []struct {
		name string
		args []string
		code int
		has  func(v any) bool
	}{
		{"check json", []string{"check", "--format", "json", dir}, exitFindings, func(v any) bool {
			return anyItem(v, "diagnostics", func(m map[string]any) bool {
				return m["file"] == c1Unit+"/terragrunt.hcl" && m["unit"] == c1Unit
			})
		}},
		{"check sarif", []string{"check", "--format", "sarif", dir}, exitFindings, func(v any) bool {
			runs, _ := v.(map[string]any)["runs"].([]any)
			if len(runs) != 1 {
				return false
			}
			return anyItem(runs[0], "results", func(m map[string]any) bool {
				msg, _ := m["message"].(map[string]any)
				text, _ := msg["text"].(string)
				return strings.HasSuffix(text, " (unit "+c1Unit+")")
			})
		}},
		{"graph json", []string{"graph", "--json", dir}, exitOK, func(v any) bool {
			return anyItem(v, "units", func(m map[string]any) bool { return m["path"] == c1Unit })
		}},
		{"blast json", []string{"blast", "--format", "json", "--base", base, dir}, exitFindings, blastJSONHas},
	}
	for _, r := range jsonRuns {
		t.Run(r.name, func(t *testing.T) {
			out, stderr, code := runCLI(t, r.args...)
			if code != r.code {
				t.Fatalf("exit %d, want %d; stderr:\n%s", code, r.code, stderr)
			}
			assertNoRawControls(t, r.name, out)
			for _, esc := range []string{`\u009b`, `\u007f`, `\u202e`} {
				if !strings.Contains(out, esc) {
					t.Errorf("stdout lacks %s:\n%s", esc, out)
				}
			}
			var v any
			if err := json.Unmarshal([]byte(out), &v); err != nil {
				t.Fatalf("Unmarshal: %v\n%s", err, out)
			}
			if !r.has(v) {
				t.Fatalf("decoded document does not carry the exact crafted names:\n%s", out)
			}
		})
	}

	// SEC-01 report --blast half (16-04): a daemon started on a copy of the
	// crafted baseline, rewritten to the crafted tree, serves a blast view
	// that escapes every name like blast does, and equals blast --base over
	// the crafted baseline after normalisation.
	live := t.TempDir()
	craftedRepo(t, live, true)
	deps := testDeps(t)
	deps.goos = "linux"
	startWatch(t, live, deps, "--poll")
	report := func(args ...string) cliResult { return runReportArgs(deps.env, "linux", args...) }
	var first cliResult
	eventually(t, func() bool { first = report(live); return first.code != exitFailure },
		"first report on the crafted baseline", func() string { return first.String() })
	craftedRepo(t, live, false)
	waitBlastFinal(t, live, report)

	t.Run("report --blast", func(t *testing.T) {
		got := report("--blast", live)
		if got.code != exitFindings || got.stderr != "" {
			t.Fatalf("report --blast: %s", got)
		}
		assertNoRawControls(t, "report --blast", got.stdout)
		_, rest, _ := strings.Cut(got.stdout, "\n")
		for _, want := range blastWant {
			if !strings.Contains(rest, want) {
				t.Errorf("report --blast stdout lacks %q:\n%s", want, got.stdout)
			}
		}
		assertBlastEquivalent(t, report, base, live)
	})

	t.Run("report --blast json", func(t *testing.T) {
		got := report("--blast", "--format", "json", live)
		if got.code != exitFindings || got.stderr != "" {
			t.Fatalf("report --blast json: %s", got)
		}
		assertNoRawControls(t, "report --blast json", got.stdout)
		for _, esc := range []string{`\u009b`, `\u007f`, `\u202e`} {
			if !strings.Contains(got.stdout, esc) {
				t.Errorf("stdout lacks %s:\n%s", esc, got.stdout)
			}
		}
		var v any
		if err := json.Unmarshal([]byte(got.stdout), &v); err != nil {
			t.Fatalf("Unmarshal: %v\n%s", err, got.stdout)
		}
		if !blastJSONHas(v) {
			t.Fatalf("decoded report --blast document does not carry the exact crafted names:\n%s", got.stdout)
		}
		assertBlastEquivalent(t, report, base, live)
	})
}

// anyItem reports whether some object in the list v[key] satisfies ok.
func anyItem(v any, key string, ok func(map[string]any) bool) bool {
	m, _ := v.(map[string]any)
	list, _ := m[key].([]any)
	for _, it := range list {
		if o, isObj := it.(map[string]any); isObj && ok(o) {
			return true
		}
	}
	return false
}
