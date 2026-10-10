package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/rogpeppe/go-internal/txtar"
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

// snapshotDir holds the blast output of the pre-Phase-14 binary, written by
// scripts/blast-snapshots.sh from the commit on PROVENANCE line 1.
const snapshotDir = "testdata/blast_v1"

// impactedSet returns the impacted[].unit values of a blast JSON document.
func impactedSet(t *testing.T, doc map[string]any) []string {
	t.Helper()
	var out []string
	for _, e := range doc["impacted"].([]any) {
		out = append(out, e.(map[string]any)["unit"].(string))
	}
	return out
}

func decodeJSON(t *testing.T, s string) map[string]any {
	t.Helper()
	var v map[string]any
	if err := json.Unmarshal([]byte(s), &v); err != nil {
		t.Fatalf("decode: %v\n%s", err, s)
	}
	return v
}

// TestBlastDepth1MatchesV1 pins BLAST-05: blast --depth 1 equals the
// pre-Phase-14 output on every blast fixture that existed before Phase 14,
// apart from the "distance 1, " token in text and version 2 plus additive
// keys in JSON. The snapshots are checked for provenance first (sec #192).
func TestBlastDepth1MatchesV1(t *testing.T) {
	prov, err := os.ReadFile(filepath.Join(snapshotDir, "PROVENANCE"))
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSuffix(string(prov), "\n"), "\n")
	if len(lines[0]) != 40 {
		t.Fatalf("PROVENANCE line 1 %q is not a full commit hash", lines[0])
	}
	listed := map[string]bool{}
	for _, l := range lines[1:] {
		sum, name, ok := strings.Cut(l, "  ")
		if !ok {
			t.Fatalf("bad PROVENANCE line %q", l)
		}
		b, err := os.ReadFile(filepath.Join(snapshotDir, name))
		if err != nil {
			t.Fatal(err)
		}
		h := sha256.Sum256(b)
		if hex.EncodeToString(h[:]) != sum {
			t.Fatalf("%s does not match its PROVENANCE sha256", name)
		}
		listed[name] = true
	}
	entries, err := os.ReadDir(snapshotDir)
	if err != nil {
		t.Fatal(err)
	}
	var cases []string
	for _, e := range entries {
		if e.Name() != "PROVENANCE" && !listed[e.Name()] {
			t.Fatalf("%s is not in PROVENANCE", e.Name())
		}
		if c, ok := strings.CutSuffix(e.Name(), ".exit"); ok {
			cases = append(cases, c)
		}
	}
	if len(cases) < 20 {
		t.Fatalf("only %d snapshot cases", len(cases))
	}

	sawEdge := false
	for _, c := range cases {
		t.Run(c, func(t *testing.T) {
			read := func(ext string) string {
				b, err := os.ReadFile(filepath.Join(snapshotDir, c+ext))
				if err != nil {
					t.Fatal(err)
				}
				return string(b)
			}
			wantText, wantJSON, wantExit := read(".txt"), read(".json"), strings.TrimSpace(read(".exit"))
			// Non-vacuity: these are pre-Phase-14 outputs.
			if !strings.Contains(wantJSON, `"version": 1`) || strings.Contains(wantJSON, `"distance"`) || strings.Contains(wantJSON, `"changes"`) {
				t.Fatalf("snapshot JSON is not a version-1 document")
			}
			if strings.Contains(wantText, "distance ") {
				t.Fatalf("snapshot text carries a distance token")
			}

			script, pair, _ := strings.Cut(c, "__")
			base, cur, _ := strings.Cut(pair, "__")
			ar, err := txtar.ParseFile(filepath.Join("testdata", "script", script+".txtar"))
			if err != nil {
				t.Fatal(err)
			}
			root := t.TempDir()
			for _, f := range ar.Files {
				if strings.HasPrefix(f.Name, base+"/") || strings.HasPrefix(f.Name, cur+"/") {
					p := filepath.Join(root, filepath.FromSlash(f.Name))
					mustDo(t, os.MkdirAll(filepath.Dir(p), 0o755))
					mustDo(t, os.WriteFile(p, f.Data, 0o644))
				}
			}
			t.Chdir(root)

			gotText, _, code := runCLI(t, "blast", "--base", base, "--depth", "1", cur)
			if strconv.Itoa(code) != wantExit {
				t.Fatalf("exit %d, snapshot %s", code, wantExit)
			}
			if got := strings.ReplaceAll(gotText, "distance 1, ", ""); got != wantText {
				t.Fatalf("--depth 1 text differs beyond the distance token:\n got:\n%s\nwant:\n%s", got, wantText)
			}
			gotJSON, _, _ := runCLI(t, "blast", "--base", base, "--depth", "1", "--format", "json", cur)
			got, want := decodeJSON(t, gotJSON), decodeJSON(t, wantJSON)
			delete(got, "version")
			delete(got, "changes")
			delete(want, "version")
			for _, key := range []string{"impacted", "broken"} {
				for _, e := range got[key].([]any) {
					m := e.(map[string]any)
					delete(m, "distance")
					delete(m, "source")
					delete(m, "via")
				}
			}
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("--depth 1 JSON differs beyond version 2 and additive keys:\n got %v\nwant %v", got, want)
			}

			full, _, _ := runCLI(t, "blast", "--base", base, "--format", "json", cur)
			all := impactedSet(t, decodeJSON(t, full))
			for _, u := range impactedSet(t, want) {
				if !slices.Contains(all, u) {
					t.Fatalf("unlimited Impacted lost %s", u)
				}
			}
			if c == "blast_impacted__base__cur" {
				if strings.Contains(wantText, "live/edge") || !slices.Contains(all, "live/edge") {
					t.Fatalf("live/edge must be absent from the snapshot and Impacted without --depth")
				}
				sawEdge = true
			}
		})
	}
	if !sawEdge {
		t.Fatal("no blast_impacted__base__cur snapshot")
	}
}
