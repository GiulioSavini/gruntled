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
)

// consumerOf is a unit that reads output ref of the producer unit.
func consumerOf(ref string) string {
	return "dependency \"vpc\" {\n  config_path = \"../" + producer + "\"\n}\ninputs = { id = dependency.vpc.outputs." + ref + " }\n"
}

// craftedRepo writes a repository with a GRT100 unit at escUnit and GRT001
// consumers at bidiUnit and c1Unit. ok writes the clean variant used as a
// blast baseline: every unit parses and resolves, and the producer
// declares one more output, so its unit is Impacted against it.
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
	}
	if ok {
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
	for _, want := range []string{
		escUnitText + "/terragrunt.hcl:",
		`r\u202el/terragrunt.hcl:`,
		`c\u009b2Jd\x7fe/terragrunt.hcl:`,
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
		for _, want := range []string{
			"  " + escUnitText + "/terragrunt.hcl\n",
			"  " + `c\u009b2Jd\x7fe` + "\n",
			"  " + `r\u202el` + "\n",
			`  v\u202ep (module v\u202ep: -output old)` + "\n",
		} {
			if !strings.Contains(out, want) {
				t.Errorf("blast stdout lacks %q:\n%s", want, out)
			}
		}
	})

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
		{"blast json", []string{"blast", "--format", "json", "--base", base, dir}, exitFindings, func(v any) bool {
			return anyItem(v, "broken", func(m map[string]any) bool { return m["unit"] == c1Unit }) &&
				anyItem(v, "impacted", func(m map[string]any) bool { return m["unit"] == producer && m["module"] == producer })
		}},
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
