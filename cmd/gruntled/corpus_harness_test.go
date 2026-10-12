package main

// REL-03 (17-02 Task 2): the mutation driver, the expected-result builder
// and the comparator, and TestCorpusHarnessSelfTest, which proves them on a
// hand-built tree with no corpus (it runs in CI).
//
// expectBlast derives the whole expected blast document from the model
// (computed once from the pristine tree's source text) and the mutation
// descriptor the generator chose; it never reads graph --json or gruntled's
// output. compareBlast is exact set equality on the JSON (the authority),
// with four named exceptions (17-03): type_changes.old is compared by rule
// (exact for absent/primitive types, otherwise only != new and != ""),
// GRT004 messages by prefix, text checked elsewhere, and the M5 sentinel's
// canonical string pinned from HEAD after a hand check.

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// reporter is the part of testing.TB the driver reports through, so the
// self-test can run the real driver against a recorder.
type reporter interface {
	Errorf(format string, args ...any)
}

type recorder struct{ msgs []string }

func (r *recorder) Errorf(format string, args ...any) {
	r.msgs = append(r.msgs, fmt.Sprintf(format, args...))
}

// m17Prefix starts every name the generator introduces, so it cannot
// collide with a corpus name.
const (
	m17Prefix   = "gruntled_m17"
	m17ReqVar   = m17Prefix + "_req"
	m17File     = m17Prefix + ".tf"
	m17Sentinel = "object({" + m17Prefix + " = string})"
)

// m17SentinelCanon is TypeSig's canonical string for m17Sentinel, pinned
// from HEAD and checked by hand against the D-15 rendering rules: an object
// type renders its attributes sorted, each name quoted with strconv.Quote,
// as "name"=type, joined by commas, inside object({...}).
const m17SentinelCanon = `object({"gruntled_m17"=string})`

// mutation is one generated change. File is root-relative; for M9 it is the
// shared symlink target and Modules lists every module linking it.
type mutation struct {
	Class   string // M1..M7, M9
	Module  string
	File    string
	Name    string
	Detail  string // M5: the old type text ("" when absent); M7: absent|true|false; M9: M4|M5 (+ old type after ':')
	Modules []string
}

func (m mutation) String() string {
	return fmt.Sprintf("%s %s %s %s %s", m.Class, m.Module, m.File, m.Name, m.Detail)
}

// harness holds one corpus run: the model of the pristine tree and the two
// working copies (base is edited only by two-sided mutations).
type harness struct {
	model     *corpusModel
	base, cur string
	sites     int
	caught    int
	gBase     *digestGuard
	gCur      *digestGuard
}

func newHarness(t testing.TB, model *corpusModel, base, cur string) *harness {
	t.Helper()
	return &harness{model: model, base: base, cur: cur,
		gBase: &digestGuard{t: t, work: base, pristine: corpusDigest(t, base), n: 100},
		gCur:  &digestGuard{t: t, work: cur, pristine: corpusDigest(t, cur), n: 100}}
}

// blockIn finds the named block of kind in root/file.
func blockIn(root, file, kind, name string) (string, tfBlock, error) {
	b, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(file)))
	if err != nil {
		return "", tfBlock{}, err
	}
	src := string(b)
	blocks, ok := findBlocks(src)
	if !ok {
		return "", tfBlock{}, fmt.Errorf("%s does not scan", file)
	}
	for _, bl := range blocks {
		if bl.Kind == kind && bl.Name == name {
			return src, bl, nil
		}
	}
	return "", tfBlock{}, fmt.Errorf("%s %q not found in %s", kind, name, file)
}

// plan returns the edits of m on the base and the current copy.
func (h *harness) plan(m mutation) (base, cur []edit, err error) {
	switch m.Class {
	case "M1", "M2":
		src, b, err := blockIn(h.cur, m.File, "output", m.Name)
		if err != nil {
			return nil, nil, err
		}
		return nil, []edit{opDeleteBlock(m.File, src, b)}, nil
	case "M3":
		return nil, []edit{opNewFile(path.Join(m.Module, m17File), "variable \""+m17ReqVar+"\" {}\n")}, nil
	case "M4", "M5":
		return h.planVar(m.Class, m)
	case "M6":
		bsrc, bb, err := blockIn(h.base, m.File, "output", m.Name)
		if err != nil {
			return nil, nil, err
		}
		csrc, cb, err := blockIn(h.cur, m.File, "output", m.Name)
		if err != nil {
			return nil, nil, err
		}
		return []edit{opInsertAttr(m.File, bsrc, bb, "type = string")}, []edit{opInsertAttr(m.File, csrc, cb, "type = number")}, nil
	case "M7":
		src, b, err := blockIn(h.cur, m.File, "output", m.Name)
		if err != nil {
			return nil, nil, err
		}
		switch m.Detail {
		case "absent":
			return nil, []edit{opInsertAttr(m.File, src, b, "sensitive = true")}, nil
		case "true":
			return nil, []edit{opSetAttr(m.File, src, b, "sensitive", "false")}, nil
		case "false":
			return nil, []edit{opSetAttr(m.File, src, b, "sensitive", "true")}, nil
		}
	case "M9":
		sub, _, _ := strings.Cut(m.Detail, ":")
		return h.planVar(sub, m)
	}
	return nil, nil, fmt.Errorf("mutation %s: unknown class or detail", m)
}

func (h *harness) planVar(op string, m mutation) (base, cur []edit, err error) {
	src, b, err := blockIn(h.cur, m.File, "variable", m.Name)
	if err != nil {
		return nil, nil, err
	}
	switch op {
	case "M4":
		a, ok := b.attr("default")
		if !ok {
			return nil, nil, fmt.Errorf("%s: variable %q has no default", m.File, m.Name)
		}
		if b.OneLine {
			return nil, []edit{{File: m.File, New: []byte(src[:a.Start] + src[a.End:])}}, nil
		}
		return nil, []edit{opDeleteAttr(m.File, src, a)}, nil
	case "M5":
		return nil, []edit{opSetAttr(m.File, src, b, "type", m17Sentinel)}, nil
	}
	return nil, nil, fmt.Errorf("variable op %q", op)
}

// blastWant is the expected result of one mutation.
type blastWant struct {
	Doc  blastDoc
	Exit int
	// OldExact marks type_changes whose old value must match exactly
	// (module|kind|name); the others must only differ from new and be
	// non-empty.
	OldExact map[string]bool
}

// expectBlast builds the expected blast of m from the model alone.
func expectBlast(model *corpusModel, m mutation) blastWant {
	w := blastWant{Doc: blastDoc{Version: 2, Kind: "blast", Baseline: true,
		Broken: []blastBroken{}, Impacted: []blastImpacted{}, Changes: []blastChange{}}, OldExact: map[string]bool{}}
	mods := []string{m.Module}
	if m.Class == "M9" {
		mods = m.Modules
	}
	var d blastChange
	tc := func(kind, name, old, new string, exact bool) {
		d.TypeChanges = append(d.TypeChanges, blastTypeChange{Kind: kind, Name: name, Old: old, New: new})
		if exact {
			w.OldExact[kind+"|"+name] = true
		}
	}
	class, sub := m.Class, ""
	if class == "M9" {
		sub, _, _ = strings.Cut(m.Detail, ":")
		class = sub
	}
	switch class {
	case "M1", "M2":
		d.RemovedOutputs = []string{m.Name}
	case "M3":
		d.AddedVariables = []string{m17ReqVar}
		tc("variable_required", m17ReqVar, "absent", "required", true)
	case "M4":
		tc("variable_required", m.Name, "optional", "required", true)
	case "M5":
		old := m.Detail
		if m.Class == "M9" {
			_, old, _ = strings.Cut(m.Detail, ":")
		}
		switch old {
		case "":
			tc("variable_type", m.Name, "any", m17SentinelCanon, true)
		case "string", "number", "bool":
			tc("variable_type", m.Name, old, m17SentinelCanon, true)
		default:
			tc("variable_type", m.Name, "", m17SentinelCanon, false)
		}
	case "M6":
		tc("output_type", m.Name, "string", "number", true)
	case "M7":
		if m.Detail == "true" {
			tc("output_sensitive", m.Name, "true", "false", true)
		} else {
			tc("output_sensitive", m.Name, "false", "true", true)
		}
	}
	for _, mod := range mods {
		c := d
		c.Module = mod
		c.AddedVariables = nonNil(c.AddedVariables)
		c.RemovedVariables = nonNil(c.RemovedVariables)
		c.AddedOutputs = nonNil(c.AddedOutputs)
		c.RemovedOutputs = nonNil(c.RemovedOutputs)
		if c.TypeChanges == nil {
			c.TypeChanges = []blastTypeChange{}
		}
		w.Doc.Changes = append(w.Doc.Changes, c)
		for k := range w.OldExact {
			w.OldExact[mod+"|"+k] = true
		}
	}

	// Seeds and the cone.
	var seeds []string
	modOf := map[string]string{}
	for _, mod := range mods {
		for _, s := range model.Seeds[mod] {
			seeds = append(seeds, s)
			modOf[s] = mod
		}
	}
	cone := model.bfs(seeds)

	// Broken: M1's firing plain sites (GRT004 replaces GRT001).
	broken := map[string][]blastFinding{}
	if m.Class == "M1" {
		for _, s := range model.Sites {
			if s.Silent || s.Shape != "plain" || s.Output != m.Name || model.Units[s.TargetUnit] == nil || model.Units[s.TargetUnit].Module != m.Module {
				continue
			}
			broken[s.Unit] = append(broken[s.Unit], blastFinding{Code: "GRT004", Severity: "error", File: s.File, Line: s.Line, Column: s.Col,
				Message: fmt.Sprintf("dependency %q output %q was removed from module %q", s.Label, m.Name, m.Module)})
		}
	}
	for _, u := range slices.Sorted(maps(broken)) {
		b := blastBroken{Unit: u, Findings: broken[u]}
		if r, ok := cone[u]; ok {
			dist, src := r.Distance, r.Source
			b.Distance, b.Source = &dist, &src
			if r.Via != "" {
				via := r.Via
				b.Via = &via
			}
		}
		w.Doc.Broken = append(w.Doc.Broken, b)
	}
	for _, u := range slices.Sorted(maps(cone)) {
		if _, isBroken := broken[u]; isBroken {
			continue
		}
		r := cone[u]
		im := blastImpacted{Unit: u, Module: modOf[r.Source], Distance: r.Distance, Source: r.Source}
		if r.Via != "" {
			via := r.Via
			im.Via = &via
		}
		if r.Distance == 1 {
			im.AddedVariables, im.RemovedVariables = d.AddedVariables, d.RemovedVariables
			im.AddedOutputs, im.RemovedOutputs = d.AddedOutputs, d.RemovedOutputs
		}
		w.Doc.Impacted = append(w.Doc.Impacted, im)
	}
	w.Doc.Summary = blastSummary{Broken: len(w.Doc.Broken), Impacted: len(w.Doc.Impacted)}
	if len(w.Doc.Broken) > 0 {
		w.Exit = 1
	}
	return w
}

func nonNil(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

func ptrStr(p *string) string {
	if p == nil {
		return "<absent>"
	}
	return *p
}

func ptrInt(p *int) string {
	if p == nil {
		return "<absent>"
	}
	return fmt.Sprint(*p)
}

// compareBlast lists every difference between the expected and the actual
// result: exit code, version, kind, baseline, note, summary, and exact
// sets of broken units and findings, impacted units with their facts, and
// changes with their lists and type_changes.
func compareBlast(w blastWant, got blastDoc, gotExit int) []string {
	var diffs []string
	add := func(format string, a ...any) { diffs = append(diffs, fmt.Sprintf(format, a...)) }
	want := w.Doc
	if gotExit != w.Exit {
		add("exit %d, want %d", gotExit, w.Exit)
	}
	if got.Version != want.Version || got.Kind != want.Kind || got.Baseline != want.Baseline || got.Note != nil {
		add("header version=%d kind=%q baseline=%v note=%v, want %d %q %v none", got.Version, got.Kind, got.Baseline, got.Note, want.Version, want.Kind, want.Baseline)
	}
	if got.Summary != want.Summary {
		add("summary %+v, want %+v", got.Summary, want.Summary)
	}
	if got.Broken == nil || got.Impacted == nil || got.Changes == nil {
		add("a list is null")
	}
	// Broken.
	gb := map[string]blastBroken{}
	for _, b := range got.Broken {
		gb[b.Unit] = b
	}
	wb := map[string]blastBroken{}
	for _, b := range want.Broken {
		wb[b.Unit] = b
	}
	for u, b := range wb {
		g, ok := gb[u]
		if !ok {
			add("broken %s missing", u)
			continue
		}
		if ptrInt(g.Distance) != ptrInt(b.Distance) || ptrStr(g.Source) != ptrStr(b.Source) || ptrStr(g.Via) != ptrStr(b.Via) {
			add("broken %s distance/source/via %s/%s/%s, want %s/%s/%s", u, ptrInt(g.Distance), ptrStr(g.Source), ptrStr(g.Via), ptrInt(b.Distance), ptrStr(b.Source), ptrStr(b.Via))
		}
		key := func(f blastFinding) string {
			return fmt.Sprintf("%s %s %s:%d:%d", f.Code, f.Severity, f.File, f.Line, f.Column)
		}
		gf := map[string]blastFinding{}
		for _, f := range g.Findings {
			gf[key(f)] = f
		}
		if len(g.Findings) != len(b.Findings) {
			add("broken %s: %d findings, want %d", u, len(g.Findings), len(b.Findings))
		}
		for _, f := range b.Findings {
			gg, ok := gf[key(f)]
			switch {
			case !ok:
				add("broken %s: finding %s missing", u, key(f))
			case !strings.HasPrefix(gg.Message, f.Message):
				add("broken %s: finding %s message %q, want prefix %q", u, key(f), gg.Message, f.Message)
			}
		}
	}
	for u := range gb {
		if _, ok := wb[u]; !ok {
			add("broken %s unexpected", u)
		}
	}
	// Impacted.
	gi := map[string]blastImpacted{}
	for _, im := range got.Impacted {
		gi[im.Unit] = im
	}
	wi := map[string]blastImpacted{}
	for _, im := range want.Impacted {
		wi[im.Unit] = im
	}
	for u, im := range wi {
		g, ok := gi[u]
		if !ok {
			add("impacted %s missing", u)
			continue
		}
		if g.Module != im.Module || g.Distance != im.Distance || g.Source != im.Source || ptrStr(g.Via) != ptrStr(im.Via) {
			add("impacted %s module/distance/source/via %s/%d/%s/%s, want %s/%d/%s/%s", u, g.Module, g.Distance, g.Source, ptrStr(g.Via), im.Module, im.Distance, im.Source, ptrStr(im.Via))
		}
		for _, l := range [][3]any{{"added_variables", g.AddedVariables, im.AddedVariables}, {"removed_variables", g.RemovedVariables, im.RemovedVariables},
			{"added_outputs", g.AddedOutputs, im.AddedOutputs}, {"removed_outputs", g.RemovedOutputs, im.RemovedOutputs}} {
			if !slices.Equal(l[1].([]string), l[2].([]string)) {
				add("impacted %s %s %v, want %v", u, l[0], l[1], l[2])
			}
		}
	}
	for u := range gi {
		if _, ok := wi[u]; !ok {
			add("impacted %s unexpected", u)
		}
	}
	// Changes.
	gc := map[string]blastChange{}
	for _, c := range got.Changes {
		gc[c.Module] = c
	}
	for _, c := range want.Changes {
		g, ok := gc[c.Module]
		if !ok {
			add("changes %s missing", c.Module)
			continue
		}
		for _, l := range [][3]any{{"added_variables", g.AddedVariables, c.AddedVariables}, {"removed_variables", g.RemovedVariables, c.RemovedVariables},
			{"added_outputs", g.AddedOutputs, c.AddedOutputs}, {"removed_outputs", g.RemovedOutputs, c.RemovedOutputs}} {
			if !slices.Equal(l[1].([]string), l[2].([]string)) {
				add("changes %s %s %v, want %v", c.Module, l[0], l[1], l[2])
			}
		}
		if len(g.TypeChanges) != len(c.TypeChanges) {
			add("changes %s: %d type_changes, want %d (%+v)", c.Module, len(g.TypeChanges), len(c.TypeChanges), g.TypeChanges)
		}
		for _, wt := range c.TypeChanges {
			i := slices.IndexFunc(g.TypeChanges, func(x blastTypeChange) bool { return x.Kind == wt.Kind && x.Name == wt.Name })
			if i < 0 {
				add("changes %s: type_change %s %s missing", c.Module, wt.Kind, wt.Name)
				continue
			}
			gt := g.TypeChanges[i]
			if gt.New != wt.New {
				add("changes %s: type_change %s %s new %q, want %q", c.Module, wt.Kind, wt.Name, gt.New, wt.New)
			}
			if w.OldExact[c.Module+"|"+wt.Kind+"|"+wt.Name] {
				if gt.Old != wt.Old {
					add("changes %s: type_change %s %s old %q, want %q", c.Module, wt.Kind, wt.Name, gt.Old, wt.Old)
				}
			} else if gt.Old == "" || gt.Old == gt.New {
				add("changes %s: type_change %s %s old %q must be non-empty and differ from new %q", c.Module, wt.Kind, wt.Name, gt.Old, gt.New)
			}
		}
	}
	for m := range gc {
		if !slices.ContainsFunc(want.Changes, func(c blastChange) bool { return c.Module == m }) {
			add("changes %s unexpected", m)
		}
	}
	slices.Sort(diffs)
	return diffs
}

// decodeBlastErr is decodeBlast without t: strict, unknown keys and
// trailing data are errors.
func decodeBlastErr(raw string) (blastDoc, error) {
	dec := json.NewDecoder(strings.NewReader(raw))
	dec.DisallowUnknownFields()
	var doc blastDoc
	if err := dec.Decode(&doc); err != nil {
		return doc, err
	}
	if dec.More() {
		return doc, fmt.Errorf("trailing data")
	}
	return doc, nil
}

// runMutation is the per-mutation driver TestCorpusMutation17 calls: it
// applies m to the working copies, runs blast --base base cur (JSON, in
// process), lets hook alter gruntled's decoded document and exit code (the
// self-test's fault injection; nil in real runs), compares with
// expectBlast, records sites and caught, reports every difference through
// r, and restores both copies.
func runMutation(r reporter, h *harness, m mutation, hook func(*blastDoc, *int)) bool {
	h.sites++
	bEdits, cEdits, err := h.plan(m)
	if err != nil {
		r.Errorf("%s: plan: %v", m, err)
		return false
	}
	var restores []func() error
	undo := func() {
		for i := len(restores) - 1; i >= 0; i-- {
			if err := restores[i](); err != nil {
				r.Errorf("%s: restore: %v", m, err)
			}
		}
		h.gBase.Tick()
		h.gCur.Tick()
	}
	defer undo()
	for _, e := range bEdits {
		rs, err := tryApplyEdit(h.base, e)
		if err != nil {
			r.Errorf("%s: base edit: %v", m, err)
			return false
		}
		restores = append(restores, rs)
	}
	for _, e := range cEdits {
		rs, err := tryApplyEdit(h.cur, e)
		if err != nil {
			r.Errorf("%s: edit: %v", m, err)
			return false
		}
		restores = append(restores, rs)
	}
	var out, errb bytes.Buffer
	code := run([]string{"blast", "--base", h.base, "--format", "json", h.cur}, &out, &errb)
	if errb.Len() > 0 {
		r.Errorf("%s: blast wrote to stderr: %s", m, errb.String())
	}
	doc, err := decodeBlastErr(out.String())
	if err != nil {
		r.Errorf("%s: blast JSON: %v\n%s", m, err, out.String())
		return false
	}
	if hook != nil {
		hook(&doc, &code)
	}
	if diffs := compareBlast(expectBlast(h.model, m), doc, code); len(diffs) > 0 {
		r.Errorf("%s: %d differences:\n  %s", m, len(diffs), strings.Join(diffs, "\n  "))
		return false
	}
	h.caught++
	return true
}

// surfaceView is what assertSurfaceUnchanged compares: per module its known
// flag and names, per unit its status, reason and module, the summary, and
// the GRT100 positions of check.
type surfaceView struct {
	Modules map[string]string
	Units   map[string]string
	GRT100  []string
}

func surfaceOf(r reporter, root string) surfaceView {
	v := surfaceView{Modules: map[string]string{}, Units: map[string]string{}}
	var out, errb bytes.Buffer
	run([]string{"graph", "--json", root}, &out, &errb)
	var g graphFull
	if err := json.Unmarshal(out.Bytes(), &g); err != nil {
		r.Errorf("graph --json on %s: %v", root, err)
		return v
	}
	for _, m := range g.Modules {
		vs, os_ := slices.Clone(m.Variables), slices.Clone(m.Outputs)
		slices.Sort(vs)
		slices.Sort(os_)
		v.Modules[m.Path] = fmt.Sprintf("known=%v vars=%v outs=%v reason=%s", m.Known, vs, os_, m.Reason)
	}
	for _, u := range g.Units {
		v.Units[u.Path] = u.Status + " " + u.Reason + " " + u.Module
	}
	out.Reset()
	errb.Reset()
	run([]string{"check", "--format", "json", root}, &out, &errb)
	var rep corpusReport
	if err := json.Unmarshal(out.Bytes(), &rep); err != nil {
		r.Errorf("check --format json on %s: %v", root, err)
		return v
	}
	for _, d := range rep.Diagnostics {
		if d.Code == "GRT100" {
			v.GRT100 = append(v.GRT100, fmt.Sprintf("%s:%d:%d", d.File, d.Line, d.Column))
		}
	}
	slices.Sort(v.GRT100)
	return v
}

// assertSurfaceUnchanged is the non-vacuity guard of a reformat: cur must
// have the same modules (known flag, variable and output names), the same
// unit statuses, and no GRT100 that base does not have.
func assertSurfaceUnchanged(r reporter, base, cur string) {
	a, b := surfaceOf(r, base), surfaceOf(r, cur)
	for p, s := range a.Modules {
		if b.Modules[p] != s {
			r.Errorf("guard: module %s: %s, base %s", p, b.Modules[p], s)
		}
	}
	for p, s := range b.Modules {
		if _, ok := a.Modules[p]; !ok {
			r.Errorf("guard: module %s appeared: %s", p, s)
		}
	}
	for p, s := range a.Units {
		if b.Units[p] != s {
			r.Errorf("guard: unit %s: %q, base %q", p, b.Units[p], s)
		}
	}
	for _, g := range b.GRT100 {
		if !slices.Contains(a.GRT100, g) {
			r.Errorf("guard: new GRT100 at %s", g)
		}
	}
}

// ---------------------------------------------------------------------------
// TestCorpusHarnessSelfTest (always on).

// selfTestFiles is the hand-built tree. Answers derived by hand:
//
//	units: live/net, live/ops (module modules/net); live/app, live/web
//	(module modules/app). Live edges: app -> net, web -> app; ops -> web
//	has skip_outputs = true (not live). Reverse: net <- app <- web.
//	Sites: live/app:10:8 dependency.net.outputs.id (plain, fires);
//	live/web:10:13 dependency.app.outputs.url (guarded by try, silent);
//	live/ops:11:9 dependency.web.outputs.url (skip_outputs, silent).
//	Removing output id of modules/net: changes [{modules/net, removed
//	outputs [id]}]; Broken live/app (GRT004 at live/app/terragrunt.hcl:10:8,
//	distance 2, source live/net, via live/net); Impacted live/net and
//	live/ops at distance 1, live/web at distance 3 (source live/net, via
//	live/app, traversing the Broken unit); exit 1.
//	modules/shared/common.tf is linked as shared.tf by both modules, so a
//	change to its variable region changes both modules (M9).
var selfTestFiles = map[string]string{
	"modules/net/main.tf": `variable "cidr" {
  type    = string
  default = "10.0.0.0/16"
}

variable "name" {}

output "id" {
  value = "x"
}

output "arn" {
  value       = "y"
  description = <<EOT
heredoc { with } braces and output "fake" {
EOT
}

output "spare" { value = 1 }
`,
	"modules/app/main.tf": `variable "id" {
  type = string
}

output "url" {
  value     = "u"
  sensitive = false
}
`,
	"modules/shared/common.tf": `variable "region" {
  default = "eu"
}
`,
	"live/net/terragrunt.hcl": "terraform {\n  source = \"../../modules/net\"\n}\n",
	"live/app/terragrunt.hcl": "terraform {\n  source = \"../../modules/app\"\n}\n\ndependency \"net\" {\n  config_path = \"../net\"\n}\n\ninputs = {\n  id = dependency.net.outputs.id\n}\n",
	"live/web/terragrunt.hcl": "terraform {\n  source = \"../../modules/app\"\n}\n\ndependency \"app\" {\n  config_path = \"../app\"\n}\n\ninputs = {\n  url = try(dependency.app.outputs.url, \"\")\n}\n",
	"live/ops/terragrunt.hcl": "terraform {\n  source = \"../../modules/net\"\n}\n\ndependency \"web\" {\n  config_path  = \"../web\"\n  skip_outputs = true\n}\n\ninputs = {\n  url = dependency.web.outputs.url\n}\n",
}

var selfTestLinks = map[string]string{
	"modules/net/shared.tf": "../shared/common.tf",
	"modules/app/shared.tf": "../shared/common.tf",
	"modules/netalias":      "net",
	"modules/net/escape":    "../../../outside.tf",
}

func buildSelfTestTree(t *testing.T) string {
	t.Helper()
	root := filepath.Join(t.TempDir(), "tree")
	for rel, text := range selfTestFiles {
		p := filepath.Join(root, filepath.FromSlash(rel))
		mustDo(t, os.MkdirAll(filepath.Dir(p), 0o755))
		mustDo(t, os.WriteFile(p, []byte(text), 0o644))
	}
	for rel, target := range selfTestLinks {
		mustDo(t, os.Symlink(target, filepath.Join(root, filepath.FromSlash(rel))))
	}
	return root
}

// selfTestMutations has one mutation per class.
var selfTestMutations = []mutation{
	{Class: "M1", Module: "modules/net", File: "modules/net/main.tf", Name: "id"},
	{Class: "M2", Module: "modules/net", File: "modules/net/main.tf", Name: "spare"},
	{Class: "M3", Module: "modules/net"},
	{Class: "M4", Module: "modules/net", File: "modules/net/main.tf", Name: "cidr"},
	{Class: "M5", Module: "modules/net", File: "modules/net/main.tf", Name: "cidr", Detail: "string"},
	{Class: "M5", Module: "modules/net", File: "modules/net/main.tf", Name: "name", Detail: ""},
	{Class: "M6", Module: "modules/app", File: "modules/app/main.tf", Name: "url"},
	{Class: "M7", Module: "modules/app", File: "modules/app/main.tf", Name: "url", Detail: "false"},
	{Class: "M7", Module: "modules/net", File: "modules/net/main.tf", Name: "spare", Detail: "absent"},
	{Class: "M9", File: "modules/shared/common.tf", Name: "region", Detail: "M4", Modules: []string{"modules/app", "modules/net"}},
	{Class: "M9", File: "modules/shared/common.tf", Name: "region", Detail: "M5:", Modules: []string{"modules/app", "modules/net"}},
}

func TestCorpusHarnessSelfTest(t *testing.T) {
	pristine := buildSelfTestTree(t)
	model := buildCorpusModel(t, pristine)
	if got := model.Eligible; !slices.Equal(got, []string{"modules/app", "modules/net"}) {
		t.Fatalf("self-test model eligible %v", got)
	}
	newH := func(t *testing.T) *harness {
		return newHarness(t, model, corpusCopy(t, pristine), corpusCopy(t, pristine))
	}

	// (d) The expected-result builder agrees with gruntled on one mutation of
	// each class (answers in the selfTestFiles comment).
	t.Run("expect matches blast", func(t *testing.T) {
		h := newH(t)
		for _, m := range selfTestMutations {
			runMutation(t, h, m, nil)
		}
		if h.caught != h.sites || h.sites != len(selfTestMutations) {
			t.Errorf("caught %d of %d sites, want all %d", h.caught, h.sites, len(selfTestMutations))
		}
		h.gBase.Final()
		h.gCur.Final()
	})

	// (c) Each fault, injected through the real driver's hook, is recorded
	// as a failure and leaves caught < sites.
	at := func(d *blastDoc, unit string) *blastImpacted {
		for i := range d.Impacted {
			if d.Impacted[i].Unit == unit {
				return &d.Impacted[i]
			}
		}
		return nil
	}
	m1, m3, m5 := selfTestMutations[0], selfTestMutations[2], selfTestMutations[4]
	faults := []struct {
		name string
		m    mutation
		hook func(*blastDoc, *int)
	}{
		{"impacted unit dropped", m1, func(d *blastDoc, _ *int) {
			d.Impacted = slices.DeleteFunc(d.Impacted, func(i blastImpacted) bool { return i.Unit == "live/web" })
		}},
		{"impacted unit added", m1, func(d *blastDoc, _ *int) {
			d.Impacted = append(d.Impacted, blastImpacted{Unit: "live/zzz", Module: "modules/net", Distance: 1, Source: "live/zzz"})
		}},
		{"distance off by one", m1, func(d *blastDoc, _ *int) { at(d, "live/web").Distance-- }},
		{"wrong via", m1, func(d *blastDoc, _ *int) { v := "live/net"; at(d, "live/web").Via = &v }},
		{"wrong source", m1, func(d *blastDoc, _ *int) { at(d, "live/web").Source = "live/ops" }},
		{"wrong module", m1, func(d *blastDoc, _ *int) { at(d, "live/web").Module = "modules/app" }},
		{"wrong name in removed_outputs", m1, func(d *blastDoc, _ *int) { at(d, "live/net").RemovedOutputs = []string{"idx"} }},
		{"wrong name in added_variables", m3, func(d *blastDoc, _ *int) { d.Changes[0].AddedVariables = []string{"other"} }},
		{"GRT001 left at a GRT004 site", m1, func(d *blastDoc, _ *int) { d.Broken[0].Findings[0].Code = "GRT001" }},
		{"one GRT004 site missing", m1, func(d *blastDoc, _ *int) { d.Broken[0].Findings = d.Broken[0].Findings[:0] }},
		{"GRT004 at a silent site", m1, func(d *blastDoc, _ *int) {
			d.Broken = append(d.Broken, blastBroken{Unit: "live/ops", Findings: []blastFinding{{Code: "GRT004", Severity: "error", File: "live/ops/terragrunt.hcl", Line: 11, Column: 9,
				Message: `dependency "web" output "url" was removed from module "modules/app"`}}})
		}},
		{"extra changes entry", m1, func(d *blastDoc, _ *int) {
			d.Changes = append(d.Changes, blastChange{Module: "modules/app", AddedVariables: []string{}, RemovedVariables: []string{}, AddedOutputs: []string{}, RemovedOutputs: []string{"url"}, TypeChanges: []blastTypeChange{}})
		}},
		{"missing changes entry", m1, func(d *blastDoc, _ *int) { d.Changes = d.Changes[:0] }},
		{"wrong type_changes kind", m3, func(d *blastDoc, _ *int) { d.Changes[0].TypeChanges[0].Kind = "variable_type" }},
		{"wrong type_changes new", m5, func(d *blastDoc, _ *int) { d.Changes[0].TypeChanges[0].New = "string" }},
		{"type_changes old-rule violation", m3, func(d *blastDoc, _ *int) { d.Changes[0].TypeChanges[0].Old = "optional" }},
		{"wrong exit code", m1, func(_ *blastDoc, c *int) { *c = 0 }},
		{"summary mismatch", m1, func(d *blastDoc, _ *int) { d.Summary.Impacted++ }},
		{"wrong version, kind or baseline", m1, func(d *blastDoc, _ *int) { d.Version, d.Kind, d.Baseline = 1, "graph", false }},
	}
	for _, f := range faults {
		t.Run("fault/"+f.name, func(t *testing.T) {
			h := newH(t)
			rec := &recorder{}
			runMutation(rec, h, f.m, f.hook)
			if len(rec.msgs) == 0 || h.caught >= h.sites {
				t.Errorf("fault not caught: %d messages, caught %d of %d", len(rec.msgs), h.caught, h.sites)
			}
			// The same mutation without the fault is caught.
			rec2 := &recorder{}
			runMutation(rec2, h, f.m, nil)
			if len(rec2.msgs) != 0 || h.caught != 1 {
				t.Errorf("clean run not caught: %v", rec2.msgs)
			}
		})
	}
	for _, sub := range []string{"version", "kind", "baseline"} {
		t.Run("fault/wrong "+sub, func(t *testing.T) {
			h := newH(t)
			rec := &recorder{}
			runMutation(rec, h, m1, func(d *blastDoc, _ *int) {
				switch sub {
				case "version":
					d.Version = 1
				case "kind":
					d.Kind = "graph"
				case "baseline":
					d.Baseline = false
				}
			})
			if len(rec.msgs) == 0 || h.caught >= h.sites {
				t.Errorf("fault not caught")
			}
		})
	}

	// The non-vacuity guard trips on a reformat that breaks the parse.
	t.Run("fault/reformat breaks the parse", func(t *testing.T) {
		cur := corpusCopy(t, pristine)
		p := filepath.Join(cur, "modules", "app", "main.tf")
		b, err := os.ReadFile(p)
		mustDo(t, err)
		mustDo(t, os.WriteFile(p, bytes.Replace(b, []byte("output \"url\" {"), []byte("output \"url\" {{"), 1), 0o644))
		rec := &recorder{}
		assertSurfaceUnchanged(rec, pristine, cur)
		if len(rec.msgs) == 0 {
			t.Errorf("guard did not trip on a broken parse")
		}
		rec = &recorder{}
		assertSurfaceUnchanged(rec, pristine, corpusCopy(t, pristine))
		if len(rec.msgs) != 0 {
			t.Errorf("guard trips on an unchanged copy: %v", rec.msgs)
		}
	})

	// (b) applyEdit confinement.
	t.Run("refusals", func(t *testing.T) {
		work := corpusCopy(t, pristine)
		for _, e := range []edit{
			{File: "modules/net/escape", New: []byte("x")},
			{File: "modules/netalias/main.tf", New: []byte("x")},
			{File: "modules/netalias/new.tf", New: []byte("x"), Create: true},
			{File: "modules/net/escape", Materialise: true},
			{File: "../outside.tf", New: []byte("x"), Create: true},
			{File: "modules/net/shared.tf", New: []byte("x")},
		} {
			if _, err := tryApplyEdit(work, e); err == nil {
				t.Errorf("applyEdit accepted %+v", e)
			}
		}
		if got, want := corpusDigest(t, work), corpusDigest(t, pristine); got != want {
			t.Errorf("a refused edit changed the copy")
		}
	})

	// (a) Each editor operation changes exactly what it says; restore is
	// byte-exact.
	t.Run("editor ops", func(t *testing.T) {
		work := corpusCopy(t, pristine)
		want := corpusDigest(t, work)
		names := func(root, mod string) string { return surfaceOf(t, root).Modules[mod] }
		read := func(rel string) string {
			b, err := os.ReadFile(filepath.Join(work, filepath.FromSlash(rel)))
			mustDo(t, err)
			return string(b)
		}
		net := "modules/net/main.tf"
		block := func(kind, name string) (string, tfBlock) {
			src, b, err := blockIn(work, net, kind, name)
			mustDo(t, err)
			return src, b
		}
		type opCase struct {
			name  string
			edits func() []edit
			check func()
		}
		var cases []opCase
		cases = append(cases,
			opCase{"delete block", func() []edit { src, b := block("output", "arn"); return []edit{opDeleteBlock(net, src, b)} },
				func() {
					if got := names(work, "modules/net"); got != "known=true vars=[cidr name region] outs=[id spare] reason=" {
						t.Errorf("delete block: %s", got)
					}
				}},
			opCase{"new file with a variable", func() []edit { return []edit{opNewFile("modules/net/"+m17File, "variable \""+m17ReqVar+"\" {}\n")} },
				func() {
					if got := names(work, "modules/net"); got != "known=true vars=[cidr gruntled_m17_req name region] outs=[arn id spare] reason=" {
						t.Errorf("new file: %s", got)
					}
				}},
			opCase{"delete attribute", func() []edit {
				src, b := block("variable", "cidr")
				a, _ := b.attr("default")
				return []edit{opDeleteAttr(net, src, a)}
			}, func() {
				if strings.Contains(read(net), "10.0.0.0/16") || !strings.Contains(read(net), "type    = string") {
					t.Errorf("delete attribute:\n%s", read(net))
				}
			}},
			opCase{"replace attribute", func() []edit {
				src, b := block("variable", "cidr")
				return []edit{opSetAttr(net, src, b, "type", m17Sentinel)}
			},
				func() {
					if !strings.Contains(read(net), "type    = "+m17Sentinel+"\n  default") {
						t.Errorf("replace attribute:\n%s", read(net))
					}
				}},
			opCase{"add attribute to a one-line block", func() []edit {
				src, b := block("output", "spare")
				return []edit{opSetAttr(net, src, b, "sensitive", "true")}
			},
				func() {
					if !strings.Contains(read(net), "output \"spare\" {\n  value = 1\n  sensitive = true\n}") {
						t.Errorf("one-line add:\n%s", read(net))
					}
				}},
			opCase{"insert attribute after a heredoc", func() []edit {
				src, b := block("output", "arn")
				return []edit{opInsertAttr(net, src, b, "sensitive = true")}
			},
				func() {
					if !strings.Contains(read(net), "EOT\n  sensitive = true\n}") {
						t.Errorf("insert after heredoc:\n%s", read(net))
					}
				}},
			opCase{"move block to a new file", func() []edit {
				src, b := block("output", "id")
				return opMoveBlock(work, net, src, b, "modules/net/gruntled_moved.tf")
			}, func() {
				if got := names(work, "modules/net"); got != "known=true vars=[cidr name region] outs=[arn id spare] reason=" {
					t.Errorf("move: %s", got)
				}
				if !strings.Contains(read("modules/net/gruntled_moved.tf"), "output \"id\" {") || strings.Contains(read(net), "output \"id\"") {
					t.Errorf("move did not move")
				}
			}},
			opCase{"materialise a symlink", func() []edit { return []edit{opMaterialise("modules/net/shared.tf")} },
				func() {
					fi, err := os.Lstat(filepath.Join(work, "modules", "net", "shared.tf"))
					mustDo(t, err)
					if !fi.Mode().IsRegular() || read("modules/net/shared.tf") != selfTestFiles["modules/shared/common.tf"] {
						t.Errorf("materialise: mode %v", fi.Mode())
					}
					if got := names(work, "modules/net"); got != "known=true vars=[cidr name region] outs=[arn id spare] reason=" {
						t.Errorf("materialise: %s", got)
					}
				}},
			opCase{".tf to .tf.json", func() []edit {
				src := read(net)
				blocks, _ := findBlocks(src)
				es, conv, skip := opToJSON(work, net, src, blocks, "modules/net/gruntled_r5.tf.json")
				if conv != 4 || skip != 1 {
					t.Errorf(".tf.json: converted %d, skipped %d, want 4 and 1 (the heredoc block stays)", conv, skip)
				}
				return es
			}, func() {
				if got := names(work, "modules/net"); got != "known=true vars=[cidr name region] outs=[arn id spare] reason=" {
					t.Errorf(".tf.json: %s", got)
				}
				if strings.Contains(read(net), "variable \"cidr\"") || !strings.Contains(read(net), "output \"arn\"") {
					t.Errorf(".tf.json left:\n%s", read(net))
				}
			}},
		)
		for _, c := range cases {
			restore := applyEdits(t, work, c.edits())
			c.check()
			// The edit must also leave blast with no Broken unit, and the
			// attribute edits must show as exactly their type facts.
			restore()
			if got := corpusDigest(t, work); got != want {
				t.Fatalf("%s: restore is not byte-exact", c.name)
			}
		}
	})
}
