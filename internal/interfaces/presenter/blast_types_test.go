package presenter_test

import (
	"bytes"
	"encoding/json"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/GiulioSavini/gruntled/internal/domain/impact"
	"github.com/GiulioSavini/gruntled/internal/interfaces/presenter"
)

func typeChange(kind impact.TypeChangeKind, name, old, new string) impact.TypeChange {
	return impact.TypeChange{Kind: kind, Name: name, Old: old, New: new}
}

// typedVPC is modules/vpc gaining required variable name, changing the
// type of variable cidr, and flipping output id's sensitive flag.
func typedVPC(t *testing.T) impact.SurfaceChange {
	c := vpcChange(t)
	c.RemovedOutputs = []string{}
	c.TypeChanges = []impact.TypeChange{
		typeChange(impact.VariableRequired, "name", "absent", "required"),
		typeChange(impact.VariableType, "cidr", "string", "list(string)"),
		typeChange(impact.OutputType, "id", "string", "number"),
		typeChange(impact.OutputSensitive, "id", "false", "true"),
	}
	return c
}

// typedResult has live/a at distance 1 and live/b at 2 from typedVPC, plus
// modules/net with a name-only change and no type changes.
func typedResult(t *testing.T) impact.Result {
	vpc := typedVPC(t)
	net := impact.SurfaceChange{Module: rp(t, "modules/net"), AddedVariables: []string{}, RemovedVariables: []string{"x"}, AddedOutputs: []string{}, RemovedOutputs: []string{}, TypeChanges: []impact.TypeChange{}}
	return impact.Result{
		Baseline: true,
		Broken:   []impact.BrokenUnit{},
		Impacted: []impact.ImpactedUnit{
			{Unit: rp(t, "live/a"), Change: vpc, Reach: impact.Reach{Distance: 1, Source: rp(t, "live/a")}},
			{Unit: rp(t, "live/b"), Change: vpc, Reach: impact.Reach{Distance: 2, Source: rp(t, "live/a"), Via: rp(t, "live/a")}},
			{Unit: rp(t, "live/n"), Change: net, Reach: impact.Reach{Distance: 1, Source: rp(t, "live/n")}},
		},
		Changes: []impact.SurfaceChange{net, vpc},
	}
}

func TestBlastTextTypeChanges(t *testing.T) {
	want := `baseline: base
Broken (0):
Impacted (3):
  live/a (distance 1, module modules/vpc: +variable name, required variable name, type variable cidr, type output id, sensitive output id)
  live/b (distance 2, from module modules/vpc, path live/b -> live/a)
  live/n (distance 1, module modules/net: -variable x)
Type changes (4):
  modules/vpc variable name: now required
  modules/vpc variable cidr: type string -> list(string)
  modules/vpc output id: type string -> number
  modules/vpc output id: sensitive false -> true
`
	if got := renderBlastText(t, typedResult(t), "base"); got != want {
		t.Errorf("BlastText =\n%s\nwant\n%s", got, want)
	}

	// No type changes: no section, as before.
	res := typedResult(t)
	res.Impacted = res.Impacted[2:]
	res.Changes = res.Changes[:1]
	if got := renderBlastText(t, res, "base"); strings.Contains(got, "Type changes") {
		t.Errorf("section printed without type changes:\n%s", got)
	}

	// A type-only change: the reasons are the only tokens.
	only := impact.SurfaceChange{Module: rp(t, "modules/m"), AddedVariables: []string{}, RemovedVariables: []string{}, AddedOutputs: []string{}, RemovedOutputs: []string{},
		TypeChanges: []impact.TypeChange{typeChange(impact.VariableType, "v", "string", "number")}}
	res = impact.Result{Baseline: true, Broken: []impact.BrokenUnit{}, Changes: []impact.SurfaceChange{only},
		Impacted: []impact.ImpactedUnit{{Unit: rp(t, "live/m"), Change: only, Reach: impact.Reach{Distance: 1, Source: rp(t, "live/m")}}}}
	if got := renderBlastText(t, res, "b"); !strings.Contains(got, "  live/m (distance 1, module modules/m: type variable v)\n") ||
		!strings.HasSuffix(got, "Type changes (1):\n  modules/m variable v: type string -> number\n") {
		t.Errorf("type-only change:\n%s", got)
	}

	// No baseline: neither Impacted nor the section.
	res = typedResult(t)
	res.Baseline = false
	if got := renderBlastText(t, res, "b"); strings.Contains(got, "Type changes") {
		t.Errorf("section printed without a baseline:\n%s", got)
	}
}

type decodedTypeChange struct {
	Kind, Name, Old, New string
}

func TestBlastJSONTypeChanges(t *testing.T) {
	out := renderBlastJSON(t, typedResult(t))
	var doc struct {
		Version  int               `json:"version"`
		Impacted []json.RawMessage `json:"impacted"`
		Changes  []struct {
			Module      string               `json:"module"`
			TypeChanges *[]decodedTypeChange `json:"type_changes"`
		} `json:"changes"`
	}
	if err := json.Unmarshal([]byte(out), &doc); err != nil {
		t.Fatal(err)
	}
	if doc.Version != 2 || len(doc.Changes) != 2 {
		t.Fatalf("version %d, %d changes", doc.Version, len(doc.Changes))
	}
	if doc.Changes[0].Module != "modules/net" || doc.Changes[0].TypeChanges == nil || len(*doc.Changes[0].TypeChanges) != 0 {
		t.Fatalf("modules/net type_changes = %v, want []", doc.Changes[0].TypeChanges)
	}
	if !strings.Contains(out, `"type_changes": []`) {
		t.Fatal(`no literal "type_changes": [] in the document`)
	}
	want := []decodedTypeChange{
		{"variable_required", "name", "absent", "required"},
		{"variable_type", "cidr", "string", "list(string)"},
		{"output_type", "id", "string", "number"},
		{"output_sensitive", "id", "false", "true"},
	}
	if got := doc.Changes[1].TypeChanges; got == nil || !reflect.DeepEqual(*got, want) {
		t.Fatalf("modules/vpc type_changes = %v, want %v", got, want)
	}
	// impacted[] is byte-identical to the same Result without TypeChanges.
	plain := typedResult(t)
	for i := range plain.Impacted {
		plain.Impacted[i].Change.TypeChanges = nil
	}
	for i := range plain.Changes {
		plain.Changes[i].TypeChanges = nil
	}
	var plainDoc struct {
		Impacted []json.RawMessage `json:"impacted"`
	}
	if err := json.Unmarshal([]byte(renderBlastJSON(t, plain)), &plainDoc); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(doc.Impacted, plainDoc.Impacted) {
		t.Fatalf("impacted[] changed with type changes:\n%s\nvs\n%s", doc.Impacted, plainDoc.Impacted)
	}
}

func TestBlastTypeStringOncePerModule(t *testing.T) {
	old := "object({" + strings.Repeat(`"a"=string,`, (1<<20)/11) + `"z"=string})`
	cur := strings.Replace(old, `"z"=string`, `"z"=number`, 1)
	ch := impact.SurfaceChange{Module: rp(t, "modules/big"), AddedVariables: []string{}, RemovedVariables: []string{}, AddedOutputs: []string{}, RemovedOutputs: []string{},
		TypeChanges: []impact.TypeChange{typeChange(impact.VariableType, "v", old, cur)}}
	res := impact.Result{Baseline: true, Broken: []impact.BrokenUnit{}, Changes: []impact.SurfaceChange{ch}}
	for i := range 1000 {
		u := "live/u" + strconv.Itoa(i)
		res.Impacted = append(res.Impacted, impact.ImpactedUnit{Unit: rp(t, u), Change: ch, Reach: impact.Reach{Distance: 1, Source: rp(t, u)}})
	}
	const perLine = 200
	jsonForm := func(s string) string { return strings.ReplaceAll(s, `"`, `\"`) }
	textForm := func(s string) string { return s }
	for _, c := range []struct {
		name string
		out  string
		form func(string) string
	}{
		{"text", renderBlastText(t, res, "b"), textForm},
		{"json", renderBlastJSON(t, res), jsonForm},
	} {
		name, out := c.name, c.out
		if n := strings.Count(out, c.form(old)); n != 1 {
			t.Errorf("%s: old type string printed %d times, want 1", name, n)
		}
		if n := strings.Count(out, c.form(cur)); n != 1 {
			t.Errorf("%s: new type string printed %d times, want 1", name, n)
		}
		if bound := 2*len(old)*2 + 1000*perLine; len(out) > bound {
			t.Errorf("%s: %d bytes, over %d", name, len(out), bound)
		}
	}
}

func TestBlastTextEscapesTypeChanges(t *testing.T) {
	ch := impact.SurfaceChange{Module: rp(t, "modules/"+crafted), AddedVariables: []string{}, RemovedVariables: []string{}, AddedOutputs: []string{}, RemovedOutputs: []string{},
		TypeChanges: []impact.TypeChange{
			typeChange(impact.VariableRequired, "r"+crafted, "absent", "required"),
			typeChange(impact.VariableType, "v"+crafted, "string"+crafted, "number"+crafted),
			typeChange(impact.OutputType, "o"+crafted, "a"+crafted, "b"+crafted),
			typeChange(impact.OutputSensitive, "s"+crafted, "false", "true"),
		}}
	unit := "live/" + crafted
	res := impact.Result{Baseline: true, Broken: []impact.BrokenUnit{}, Changes: []impact.SurfaceChange{ch},
		Impacted: []impact.ImpactedUnit{{Unit: rp(t, unit), Change: ch, Reach: impact.Reach{Distance: 1, Source: rp(t, unit)}}}}

	got := renderBlastText(t, res, "base")
	assertTerminalSafe(t, []byte(got))
	m := "modules/" + craftedTerm
	want := `baseline: base
Broken (0):
Impacted (1):
  live/` + craftedTerm + ` (distance 1, module ` + m + `: required variable r` + craftedTerm + `, type variable v` + craftedTerm + `, type output o` + craftedTerm + `, sensitive output s` + craftedTerm + `)
Type changes (4):
  ` + m + ` variable r` + craftedTerm + `: now required
  ` + m + ` variable v` + craftedTerm + `: type string` + craftedTerm + ` -> number` + craftedTerm + `
  ` + m + ` output o` + craftedTerm + `: type a` + craftedTerm + ` -> b` + craftedTerm + `
  ` + m + ` output s` + craftedTerm + `: sensitive false -> true
`
	if got != want {
		t.Errorf("BlastText =\n%q\nwant\n%q", got, want)
	}

	out := []byte(renderBlastJSON(t, res))
	assertJSONEscaped(t, out, jstring("changes", 0, "type_changes", 1, "name"), "v"+crafted)
	assertJSONEscaped(t, out, jstring("changes", 0, "type_changes", 1, "old"), "string"+crafted)
	assertJSONEscaped(t, out, jstring("changes", 0, "type_changes", 2, "new"), "b"+crafted)
	var b bytes.Buffer
	if err := presenter.BlastJSON(&b, res); err != nil {
		t.Fatal(err)
	}
}
