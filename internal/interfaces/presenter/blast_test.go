package presenter_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/GiulioSavini/gruntled/internal/domain/diagnostic"
	"github.com/GiulioSavini/gruntled/internal/domain/impact"
	"github.com/GiulioSavini/gruntled/internal/interfaces/presenter"
)

// blastMixed has one unit-attributed and one file-level Broken subject and
// two Impacted units, one exercising every change-token group.
func blastMixed(t *testing.T) impact.Result {
	t.Helper()
	vpc := vpcChange(t)
	net := impact.SurfaceChange{
		Module:           rp(t, "modules/net"),
		AddedVariables:   []string{"c"},
		RemovedVariables: []string{"a", "b"},
		AddedOutputs:     []string{"e"},
		RemovedOutputs:   []string{"d"},
	}
	return impact.Result{
		Baseline: true,
		Broken: []impact.BrokenUnit{
			{Subject: rp(t, "live/app"), Findings: []diagnostic.Diagnostic{
				unitDiag(t, "live/app", "live/app/terragrunt.hcl", 12, 5, `dependency "vpc" output "id" is not declared by module "modules/vpc"`),
			}},
			{Subject: rp(t, "live/x/terragrunt.hcl"), Findings: []diagnostic.Diagnostic{
				fileDiag(t, "live/x/terragrunt.hcl", 3, 1, "Argument or block definition required"),
			}},
		},
		Impacted: []impact.ImpactedUnit{
			{Unit: rp(t, "live/db"), Change: vpc, Reach: impact.Reach{Distance: 1, Source: rp(t, "live/db")}},
			{Unit: rp(t, "live/net"), Change: net, Reach: impact.Reach{Distance: 1, Source: rp(t, "live/net")}},
		},
		Changes: []impact.SurfaceChange{net, vpc},
	}
}

// vpcChange is modules/vpc gaining variable name and losing output id.
func vpcChange(t *testing.T) impact.SurfaceChange {
	return impact.SurfaceChange{
		Module:           rp(t, "modules/vpc"),
		AddedVariables:   []string{"name"},
		RemovedVariables: []string{},
		AddedOutputs:     []string{},
		RemovedOutputs:   []string{"id"},
	}
}

// blastChain is blastMixed's Broken plus a transitive chain: live/a uses
// modules/vpc (distance 1), live/b depends on live/a, live/c on live/b.
func blastChain(t *testing.T) impact.Result {
	t.Helper()
	res := blastMixed(t)
	vpc := vpcChange(t)
	res.Impacted = []impact.ImpactedUnit{
		{Unit: rp(t, "live/a"), Change: vpc, Reach: impact.Reach{Distance: 1, Source: rp(t, "live/a")}},
		{Unit: rp(t, "live/b"), Change: vpc, Reach: impact.Reach{Distance: 2, Source: rp(t, "live/a"), Via: rp(t, "live/a")}},
		{Unit: rp(t, "live/c"), Change: vpc, Reach: impact.Reach{Distance: 3, Source: rp(t, "live/a"), Via: rp(t, "live/b")}},
	}
	res.Changes = []impact.SurfaceChange{vpc}
	return res
}

func renderBlastText(t *testing.T, res impact.Result, label string) string {
	t.Helper()
	var b bytes.Buffer
	if err := presenter.BlastText(&b, res, label); err != nil {
		t.Fatalf("BlastText: %v", err)
	}
	return b.String()
}

func renderBlastJSON(t *testing.T, res impact.Result) string {
	t.Helper()
	var b bytes.Buffer
	if err := presenter.BlastJSON(&b, res); err != nil {
		t.Fatalf("BlastJSON: %v", err)
	}
	return b.String()
}

func TestBlastTextGolden(t *testing.T) {
	want := `baseline: ../base
Broken (2):
  live/app
    live/app/terragrunt.hcl:12:5: GRT001 dependency "vpc" output "id" is not declared by module "modules/vpc"
  live/x/terragrunt.hcl
    live/x/terragrunt.hcl:3:1: GRT100 Argument or block definition required
Impacted (2):
  live/db (module modules/vpc: +variable name, -output id)
  live/net (module modules/net: -variable a, -variable b, +variable c, -output d, +output e)
`
	if got := renderBlastText(t, blastMixed(t), "../base"); got != want {
		t.Errorf("BlastText =\n%s\nwant\n%s", got, want)
	}
}

func TestBlastTextEmpty(t *testing.T) {
	res := impact.Result{Baseline: true, Broken: []impact.BrokenUnit{}, Impacted: []impact.ImpactedUnit{}}
	want := "baseline: base\nBroken (0):\nImpacted (0):\n"
	if got := renderBlastText(t, res, "base"); got != want {
		t.Errorf("BlastText = %q, want %q", got, want)
	}
}

func TestBlastTextNoBaseline(t *testing.T) {
	res := blastMixed(t)
	res.Baseline = false
	res.Impacted = []impact.ImpactedUnit{}
	want := `baseline: none (no baseline)
Broken (2):
  live/app
    live/app/terragrunt.hcl:12:5: GRT001 dependency "vpc" output "id" is not declared by module "modules/vpc"
  live/x/terragrunt.hcl
    live/x/terragrunt.hcl:3:1: GRT100 Argument or block definition required
`
	if got := renderBlastText(t, res, "ignored"); got != want {
		t.Errorf("BlastText =\n%s\nwant\n%s", got, want)
	}
}

func TestBlastJSONGolden(t *testing.T) {
	want := `{
  "version": 2,
  "kind": "blast",
  "baseline": true,
  "broken": [
    {
      "unit": "live/app",
      "findings": [
        {
          "code": "GRT001",
          "severity": "error",
          "file": "live/app/terragrunt.hcl",
          "line": 12,
          "column": 5,
          "message": "dependency \\"vpc\\" output \\"id\\" is not declared by module \\"modules/vpc\\""
        }
      ]
    },
    {
      "unit": "live/x/terragrunt.hcl",
      "findings": [
        {
          "code": "GRT100",
          "severity": "error",
          "file": "live/x/terragrunt.hcl",
          "line": 3,
          "column": 1,
          "message": "Argument or block definition required"
        }
      ]
    }
  ],
  "impacted": [
    {
      "unit": "live/a",
      "module": "modules/vpc",
      "distance": 1,
      "source": "live/a",
      "added_variables": [
        "name"
      ],
      "removed_variables": [],
      "added_outputs": [],
      "removed_outputs": [
        "id"
      ]
    },
    {
      "unit": "live/b",
      "module": "modules/vpc",
      "distance": 2,
      "source": "live/a",
      "via": "live/a"
    },
    {
      "unit": "live/c",
      "module": "modules/vpc",
      "distance": 3,
      "source": "live/a",
      "via": "live/b"
    }
  ],
  "changes": [
    {
      "module": "modules/vpc",
      "added_variables": [
        "name"
      ],
      "removed_variables": [],
      "added_outputs": [],
      "removed_outputs": [
        "id"
      ]
    }
  ],
  "summary": {
    "broken": 2,
    "impacted": 3
  }
}
`
	if got := renderBlastJSON(t, blastChain(t)); got != want {
		t.Errorf("BlastJSON =\n%s\nwant\n%s", got, want)
	}
}

// TestBlastJSONV1KeysKeepType: every v1 key is still present with its v1
// JSON type (sec #195). Nothing more is promised to a v1 reader: version is
// 2, and readers must check it.
func TestBlastJSONV1KeysKeepType(t *testing.T) {
	var v map[string]any
	if err := json.Unmarshal([]byte(renderBlastJSON(t, blastMixed(t))), &v); err != nil {
		t.Fatal(err)
	}
	if v["version"] != float64(2) || v["kind"] != "blast" || v["baseline"] != true {
		t.Fatalf("version/kind/baseline = %v/%v/%v", v["version"], v["kind"], v["baseline"])
	}
	for _, b := range v["broken"].([]any) {
		m := b.(map[string]any)
		if _, ok := m["unit"].(string); !ok {
			t.Errorf("broken unit not a string: %v", m)
		}
		if _, ok := m["findings"].([]any); !ok {
			t.Errorf("broken findings not a list: %v", m)
		}
	}
	for _, i := range v["impacted"].([]any) {
		m := i.(map[string]any)
		for _, k := range []string{"unit", "module"} {
			if _, ok := m[k].(string); !ok {
				t.Errorf("impacted %s not a string: %v", k, m)
			}
		}
		for _, k := range []string{"added_variables", "removed_variables", "added_outputs", "removed_outputs"} {
			if _, ok := m[k].([]any); !ok {
				t.Errorf("distance-1 impacted %s not a list: %v", k, m)
			}
		}
	}
	first := v["impacted"].([]any)[0].(map[string]any)
	if !reflect.DeepEqual(first["added_variables"], []any{"name"}) || !reflect.DeepEqual(first["removed_outputs"], []any{"id"}) {
		t.Errorf("distance-1 lists lost their v1 content: %v", first)
	}
	sum := v["summary"].(map[string]any)
	if _, ok := sum["broken"].(float64); !ok {
		t.Errorf("summary.broken not a number")
	}
	if _, ok := sum["impacted"].(float64); !ok {
		t.Errorf("summary.impacted not a number")
	}
}

func TestBlastJSONBrokenReach(t *testing.T) {
	res := blastMixed(t)
	res.Broken = append([]impact.BrokenUnit{{
		Subject:  rp(t, "live/aa"),
		Findings: []diagnostic.Diagnostic{unitDiag(t, "live/aa", "live/aa/terragrunt.hcl", 1, 1, "x")},
		Reach:    impact.Reach{Distance: 2, Source: rp(t, "live/db"), Via: rp(t, "live/db")},
	}}, res.Broken...)
	var v map[string]any
	if err := json.Unmarshal([]byte(renderBlastJSON(t, res)), &v); err != nil {
		t.Fatal(err)
	}
	broken := v["broken"].([]any)
	reached := broken[0].(map[string]any)
	if reached["distance"] != float64(2) || reached["source"] != "live/db" || reached["via"] != "live/db" {
		t.Errorf("reached Broken unit = %v", reached)
	}
	for _, b := range broken[1:] {
		m := b.(map[string]any)
		for _, k := range []string{"distance", "source", "via"} {
			if _, ok := m[k]; ok {
				t.Errorf("%v carries %q though it was not reached", m["unit"], k)
			}
		}
	}
}

func TestBlastJSONLinear(t *testing.T) {
	chain := func(n int) impact.Result {
		vpc := vpcChange(t)
		res := impact.Result{Baseline: true, Broken: []impact.BrokenUnit{}, Changes: []impact.SurfaceChange{vpc}}
		for i := range n {
			u := impact.ImpactedUnit{Unit: rp(t, "live/u"+strconv.Itoa(i)), Change: vpc, Reach: impact.Reach{Distance: i + 1, Source: rp(t, "live/u0")}}
			if i > 0 {
				u.Reach.Via = rp(t, "live/u"+strconv.Itoa(i-1))
			}
			res.Impacted = append(res.Impacted, u)
		}
		return res
	}
	b2, b4 := len(renderBlastJSON(t, chain(2000))), len(renderBlastJSON(t, chain(4000)))
	// One transitive entry: braces, five keys and three paths of at most 15
	// bytes, well under 250 bytes.
	if b2 > 2000*250 {
		t.Errorf("2,000-unit chain: %d bytes, over 250 per entry", b2)
	}
	if b4 > 2*b2+1024 {
		t.Errorf("doubling the chain: %d -> %d bytes, not linear", b2, b4)
	}
}

type blastDecoded struct {
	Version  int             `json:"version"`
	Kind     string          `json:"kind"`
	Baseline bool            `json:"baseline"`
	Note     *string         `json:"note"`
	Broken   json.RawMessage `json:"broken"`
	Impacted json.RawMessage `json:"impacted"`
	Changes  json.RawMessage `json:"changes"`
}

func decodeBlast(t *testing.T, s string) blastDecoded {
	t.Helper()
	var d blastDecoded
	if err := json.Unmarshal([]byte(s), &d); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if d.Version != 2 || d.Kind != "blast" {
		t.Errorf("version/kind = %d/%q, want 2/blast", d.Version, d.Kind)
	}
	if strings.Contains(s, "null") {
		t.Errorf("output contains null:\n%s", s)
	}
	return d
}

func TestBlastJSONNoBaseline(t *testing.T) {
	// Impacted set by a buggy caller must still not leak without a baseline.
	res := blastMixed(t)
	res.Baseline = false
	d := decodeBlast(t, renderBlastJSON(t, res))
	if d.Baseline {
		t.Errorf("baseline = true, want false")
	}
	if d.Note == nil || *d.Note != "no baseline" {
		t.Errorf("note = %v, want \"no baseline\"", d.Note)
	}
	if string(d.Impacted) != "[]" {
		t.Errorf("impacted = %s, want []", d.Impacted)
	}
	res.Changes = nil
	if d := decodeBlast(t, renderBlastJSON(t, res)); string(d.Changes) != "[]" {
		t.Errorf("changes = %s, want []", d.Changes)
	}
}

func TestBlastJSONBaselineHasNoNote(t *testing.T) {
	d := decodeBlast(t, renderBlastJSON(t, blastMixed(t)))
	if !d.Baseline || d.Note != nil {
		t.Errorf("baseline=%v note=%v, want true and absent", d.Baseline, d.Note)
	}
}

func TestBlastJSONNilListsNeverNull(t *testing.T) {
	bare := impact.SurfaceChange{Module: rp(t, "modules/vpc"), RemovedOutputs: []string{"id"}}
	res := impact.Result{
		Baseline: true,
		Impacted: []impact.ImpactedUnit{
			{Unit: rp(t, "live/db"), Change: bare, Reach: impact.Reach{Distance: 1, Source: rp(t, "live/db")}},
			{Unit: rp(t, "live/far"), Change: bare, Reach: impact.Reach{Distance: 2, Source: rp(t, "live/db"), Via: rp(t, "live/db")}},
		},
		Changes: []impact.SurfaceChange{bare},
	}
	out := renderBlastJSON(t, res)
	d := decodeBlast(t, out)
	if string(d.Broken) != "[]" {
		t.Errorf("broken = %s, want []", d.Broken)
	}
	var v map[string]any
	if err := json.Unmarshal([]byte(out), &v); err != nil {
		t.Fatal(err)
	}
	if far := v["impacted"].([]any)[1].(map[string]any); len(far) != 5 {
		t.Errorf("distance-2 entry has keys %v, want unit, module, distance, source, via only", far)
	}
	decodeBlast(t, renderBlastJSON(t, impact.Result{}))
}

func TestBlastDeterministic(t *testing.T) {
	res := blastMixed(t)
	if a, b := renderBlastJSON(t, res), renderBlastJSON(t, res); a != b {
		t.Errorf("BlastJSON not deterministic")
	}
	if a, b := renderBlastText(t, res, "x"), renderBlastText(t, res, "x"); a != b {
		t.Errorf("BlastText not deterministic")
	}
}

func TestBlastWriterErrorReturned(t *testing.T) {
	boom := errors.New("boom")
	if err := presenter.BlastText(failingWriter{err: boom}, blastMixed(t), "b"); !errors.Is(err, boom) {
		t.Errorf("BlastText err = %v, want boom", err)
	}
	if err := presenter.BlastJSON(failingWriter{err: boom}, blastMixed(t)); !errors.Is(err, boom) {
		t.Errorf("BlastJSON err = %v, want boom", err)
	}
}

// blastCrafted carries crafted in every repo-controlled field BlastText
// and BlastJSON print.
func blastCrafted(t *testing.T) impact.Result {
	t.Helper()
	unit := "live/" + crafted
	ch := impact.SurfaceChange{
		Module:           rp(t, "modules/"+crafted),
		AddedVariables:   []string{"v" + crafted},
		RemovedVariables: []string{},
		AddedOutputs:     []string{},
		RemovedOutputs:   []string{"o" + crafted},
	}
	return impact.Result{
		Baseline: true,
		Broken: []impact.BrokenUnit{
			{Subject: rp(t, unit), Findings: []diagnostic.Diagnostic{
				unitDiag(t, unit, unit+"/terragrunt.hcl", 2, 3, "bad "+crafted),
			}},
		},
		Impacted: []impact.ImpactedUnit{
			{Unit: rp(t, unit), Change: ch, Reach: impact.Reach{Distance: 1, Source: rp(t, unit)}},
			{Unit: rp(t, "live/z"), Change: ch, Reach: impact.Reach{Distance: 2, Source: rp(t, unit), Via: rp(t, unit)}},
		},
		Changes: []impact.SurfaceChange{ch},
	}
}

func TestBlastTextEscapesControls(t *testing.T) {
	got := renderBlastText(t, blastCrafted(t), "base\x1b[2J"+crafted)
	assertTerminalSafe(t, []byte(got))
	unit := "live/" + craftedTerm
	want := `baseline: base\x1b[2J` + craftedTerm + `
Broken (1):
  ` + unit + `
    ` + unit + `/terragrunt.hcl:2:3: GRT001 bad ` + craftedTerm + `
Impacted (1):
  ` + unit + ` (module modules/` + craftedTerm + `: +variable v` + craftedTerm + `, -output o` + craftedTerm + `)
`
	if got != want {
		t.Errorf("BlastText =\n%q\nwant\n%q", got, want)
	}
}

func TestBlastJSONEscapesTerminalRunes(t *testing.T) {
	out := []byte(renderBlastJSON(t, blastCrafted(t)))
	assertJSONEscaped(t, out, jstring("broken", 0, "unit"), "live/"+crafted)
	assertJSONEscaped(t, out, jstring("broken", 0, "findings", 0, "message"), "bad "+crafted)
	assertJSONEscaped(t, out, jstring("impacted", 0, "module"), "modules/"+crafted)
	assertJSONEscaped(t, out, jstring("impacted", 0, "added_variables", 0), "v"+crafted)
	assertJSONEscaped(t, out, jstring("impacted", 1, "source"), "live/"+crafted)
	assertJSONEscaped(t, out, jstring("impacted", 1, "via"), "live/"+crafted)
	assertJSONEscaped(t, out, jstring("changes", 0, "module"), "modules/"+crafted)
}

// blastGRT004 is a Broken GRT004 whose message carries crafted (raw ESC,
// U+009B, U+202E, ...) as the module and target names: the presenter must
// escape it like any other message, whatever the code.
func blastGRT004(t *testing.T) (impact.Result, string) {
	t.Helper()
	msg := `dependency "vpc" output "id" was removed from module "modules/` + crafted + `" (target unit "live/` + crafted + `")`
	d, err := diagnostic.NewForUnit(diagnostic.CodeRemovedOutput, diagnostic.SeverityError, rp(t, "live/app"), pos(t, "live/app/terragrunt.hcl", 7, 20), msg)
	if err != nil {
		t.Fatalf("NewForUnit: %v", err)
	}
	return impact.Result{
		Baseline: true,
		Broken:   []impact.BrokenUnit{{Subject: rp(t, "live/app"), Findings: []diagnostic.Diagnostic{d}}},
		Impacted: []impact.ImpactedUnit{},
	}, msg
}

func TestBlastTextGRT004(t *testing.T) {
	res, _ := blastGRT004(t)
	got := renderBlastText(t, res, "base")
	assertTerminalSafe(t, []byte(got))
	want := `    live/app/terragrunt.hcl:7:20: GRT004 dependency "vpc" output "id" was removed from module "modules/` + craftedTerm + `" (target unit "live/` + craftedTerm + `")` + "\n"
	if !strings.Contains(got, want) {
		t.Errorf("BlastText =\n%q\nwant a line\n%q", got, want)
	}
}

func TestBlastJSONGRT004(t *testing.T) {
	res, msg := blastGRT004(t)
	out := []byte(renderBlastJSON(t, res))
	assertJSONEscaped(t, out, jstring("broken", 0, "findings", 0, "message"), msg)
	if !bytes.Contains(out, []byte(`"code": "GRT004"`)) {
		t.Errorf("BlastJSON lacks \"code\": \"GRT004\":\n%s", out)
	}
	if !bytes.Contains(out, []byte(`"version": 2`)) {
		t.Errorf("BlastJSON is not version 2:\n%s", out)
	}
}
