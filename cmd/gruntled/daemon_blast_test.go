package main

// DAEMON-07 proofs (16-04): the daemon's blast view equals `blast --base`
// over a copy of its baseline, except for the label line (text) and the
// daemon_baseline member (JSON). The normaliser checks both against the
// expected baseline before it removes them (sec F7).

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/GiulioSavini/gruntled/internal/application/blasting"
	"github.com/GiulioSavini/gruntled/internal/application/checking"
	"github.com/GiulioSavini/gruntled/internal/application/watching"
	"github.com/GiulioSavini/gruntled/internal/infrastructure/ipc"
	"github.com/GiulioSavini/gruntled/internal/infrastructure/terragrunt"
	"github.com/GiulioSavini/gruntled/internal/infrastructure/tfsurface"
	"github.com/GiulioSavini/gruntled/internal/interfaces/presenter"
)

const (
	// daemonMemberLine opens the daemon_baseline block in a daemon view.
	daemonMemberLine = `  "daemon_baseline": {` + "\n"
	// baselineTrueLine is the line the block must directly follow.
	baselineTrueLine = `  "baseline": true,` + "\n"
	// normalisedLabel replaces the first text line of both documents.
	normalisedLabel = "baseline: X\n"
)

// normaliseDaemonBlast removes from a daemon blast view what only the
// daemon prints, after asserting it is exactly what want says:
//   - text: line 1 must be exactly "baseline: daemon generation <G0>
//     (<source>), current generation <G>" for want; it is replaced by
//     normalisedLabel;
//   - JSON: exactly one daemon_baseline block, directly after
//     `"baseline": true,`, holding exactly source, generation and
//     current_generation equal to want; it is deleted.
func normaliseDaemonBlast(out string, want presenter.DaemonBaseline) (string, error) {
	if strings.HasPrefix(out, "{") {
		lines := strings.SplitAfter(out, "\n")
		at, n := -1, 0
		for i, l := range lines {
			if strings.Contains(l, `"daemon_baseline"`) {
				at, n = i, n+1
			}
		}
		if n != 1 {
			return "", fmt.Errorf("%d daemon_baseline members, want exactly 1", n)
		}
		if lines[at] != daemonMemberLine {
			return "", fmt.Errorf("daemon_baseline line %q, want %q", lines[at], daemonMemberLine)
		}
		if at == 0 || lines[at-1] != baselineTrueLine {
			return "", fmt.Errorf("daemon_baseline does not directly follow %q", baselineTrueLine)
		}
		end := at + 4 // three members, then the closing line
		if end >= len(lines) || lines[end] != "  },\n" {
			return "", fmt.Errorf("daemon_baseline block is not exactly three members")
		}
		var got struct {
			Source            string `json:"source"`
			Generation        uint64 `json:"generation"`
			CurrentGeneration uint64 `json:"current_generation"`
		}
		dec := json.NewDecoder(strings.NewReader("{" + strings.Join(lines[at+1:end], "") + "}"))
		dec.DisallowUnknownFields()
		if err := dec.Decode(&got); err != nil {
			return "", fmt.Errorf("daemon_baseline block: %v", err)
		}
		if got.Source != want.Source || got.Generation != want.Generation || got.CurrentGeneration != want.CurrentGeneration {
			return "", fmt.Errorf("daemon_baseline %+v, want %+v", got, want)
		}
		return strings.Join(lines[:at], "") + strings.Join(lines[end+1:], ""), nil
	}
	first, rest, ok := strings.Cut(out, "\n")
	// Spelled out here, not through presenter.DaemonBaselineLabel, so a
	// wrong label is caught even when the presenter itself is wrong.
	wantLine := fmt.Sprintf("baseline: daemon generation %d (%s), current generation %d", want.Generation, want.Source, want.CurrentGeneration)
	if !ok || first != wantLine {
		return "", fmt.Errorf("first line %q, want %q", first, wantLine)
	}
	return normalisedLabel + rest, nil
}

// normaliseBlastBase normalises `blast --base` output the same way: its
// text label line (any path) is replaced, and its JSON must carry no
// daemon_baseline member.
func normaliseBlastBase(out string) (string, error) {
	if strings.HasPrefix(out, "{") {
		if strings.Contains(out, `"daemon_baseline"`) {
			return "", fmt.Errorf("blast --base JSON carries a daemon_baseline member")
		}
		return out, nil
	}
	first, rest, ok := strings.Cut(out, "\n")
	if !ok || !strings.HasPrefix(first, "baseline: ") {
		return "", fmt.Errorf("blast --base first line %q has no baseline label", first)
	}
	return normalisedLabel + rest, nil
}

func TestNormaliseDaemonBlast(t *testing.T) {
	a, b := abReports(t)
	res, err := blasting.Between(a, b)
	mustDo(t, err)
	d := presenter.DaemonBaseline{Source: "watch start", Generation: 1, CurrentGeneration: 2}
	var dText, dJSON, bText, bJSON strings.Builder
	mustDo(t, presenter.BlastText(&dText, res, presenter.DaemonBaselineLabel(d)))
	mustDo(t, presenter.BlastJSONDaemon(&dJSON, res, d))
	mustDo(t, presenter.BlastText(&bText, res, "/tmp/base"))
	mustDo(t, presenter.BlastJSON(&bJSON, res))

	for _, c := range []struct{ name, daemon, base string }{
		{"text", dText.String(), bText.String()},
		{"json", dJSON.String(), bJSON.String()},
	} {
		got, err := normaliseDaemonBlast(c.daemon, d)
		mustDo(t, err)
		want, err := normaliseBlastBase(c.base)
		mustDo(t, err)
		if got != want {
			t.Fatalf("%s: normalised daemon view differs from blast --base:\n%s\n---\n%s", c.name, got, want)
		}
	}
	// Exactly one block removed: the normalised daemon JSON is BlastJSON.
	if got, _ := normaliseDaemonBlast(dJSON.String(), d); got != bJSON.String() {
		t.Fatalf("normalised daemon JSON is not BlastJSON:\n%s", got)
	}

	block := func(s string) (pre, blk, post string) {
		i := strings.Index(s, daemonMemberLine)
		j := i + strings.Index(s[i:], "  },\n") + len("  },\n")
		return s[:i], s[i:j], s[j:]
	}
	pre, blk, post := block(dJSON.String())
	kindAt := strings.Index(pre, `  "kind"`)
	bad := []struct {
		name string
		out  string
		want presenter.DaemonBaseline
	}{
		{"text wrong generation", dText.String(), presenter.DaemonBaseline{Source: "watch start", Generation: 2, CurrentGeneration: 2}},
		{"text wrong current generation", dText.String(), presenter.DaemonBaseline{Source: "watch start", Generation: 1, CurrentGeneration: 3}},
		{"text wrong source", dText.String(), presenter.DaemonBaseline{Source: "rebase", Generation: 1, CurrentGeneration: 2}},
		{"text missing label line", dText.String()[strings.Index(dText.String(), "\n")+1:], d},
		{"text blast --base label", bText.String(), d},
		{"text label with a suffix", strings.Replace(dText.String(), "\n", " x\n", 1), d},
		{"json zero blocks", bJSON.String(), d},
		{"json two blocks", pre + blk + blk + post, d},
		{"json misplaced block", pre[:kindAt] + blk + pre[kindAt:] + post, d},
		{"json wrong generation", dJSON.String(), presenter.DaemonBaseline{Source: "watch start", Generation: 2, CurrentGeneration: 2}},
		{"json wrong current generation", dJSON.String(), presenter.DaemonBaseline{Source: "watch start", Generation: 1, CurrentGeneration: 3}},
		{"json wrong source", dJSON.String(), presenter.DaemonBaseline{Source: "rebase", Generation: 1, CurrentGeneration: 2}},
		{"json extra member", strings.Replace(dJSON.String(), `    "source"`, `    "x": 1,`+"\n"+`    "source"`, 1), d},
		{"json unknown member", strings.Replace(dJSON.String(), `    "source"`, `    "sourc"`, 1), d},
	}
	for _, c := range bad {
		if got, err := normaliseDaemonBlast(c.out, c.want); err == nil {
			t.Errorf("%s: accepted, normalised to:\n%s", c.name, got)
		}
	}
	for _, c := range []struct{ name, out string }{
		{"blast --base json with a block", dJSON.String()},
		{"blast --base text without a label", dText.String()[strings.Index(dText.String(), "\n")+1:]},
	} {
		if _, err := normaliseBlastBase(c.out); err == nil {
			t.Errorf("%s: accepted", c.name)
		}
	}
}

// xorshift is the property's deterministic PRNG.
type xorshift uint64

func (x *xorshift) next() uint64 {
	v := uint64(*x)
	v ^= v << 13
	v ^= v >> 7
	v ^= v << 17
	*x = xorshift(v)
	return v
}

func (x *xorshift) intn(n int) int { return int(x.next() % uint64(n)) }

// The generated tree: modules under modules/mK, units under live/<name>
// that instantiate one module each and depend on lower units.
type (
	gVar struct{ name, typ string }
	gOut struct {
		name      string
		sensitive bool
	}
	gMod struct {
		vars []gVar
		outs []gOut
	}
	// gDep is a dependency block; out is the output its inputs read, or
	// "" for a block that reads nothing (an ordering edge).
	gDep  struct{ label, target, out string }
	gUnit struct {
		name   string
		mod    int
		deps   []gDep
		pad    int  // leading comment lines: moves every position
		broken bool // an unterminated block: GRT100
	}
	gTree struct {
		mods   []gMod
		units  []gUnit
		serial int
	}
)

var genTypes = []string{"", "string", "number", "list(string)", "map(string)"}

func (g *gTree) clone() *gTree {
	c := &gTree{serial: g.serial}
	for _, m := range g.mods {
		c.mods = append(c.mods, gMod{vars: slices.Clone(m.vars), outs: slices.Clone(m.outs)})
	}
	for _, u := range g.units {
		u.deps = slices.Clone(u.deps)
		c.units = append(c.units, u)
	}
	return c
}

func (g *gTree) files() map[string]string {
	m := map[string]string{}
	for i, md := range g.mods {
		var b strings.Builder
		for _, v := range md.vars {
			if v.typ == "" {
				fmt.Fprintf(&b, "variable %q {}\n", v.name)
			} else {
				fmt.Fprintf(&b, "variable %q {\n  type = %s\n}\n", v.name, v.typ)
			}
		}
		for _, o := range md.outs {
			fmt.Fprintf(&b, "output %q {\n  value = \"x\"\n", o.name)
			if o.sensitive {
				b.WriteString("  sensitive = true\n")
			}
			b.WriteString("}\n")
		}
		m[fmt.Sprintf("modules/m%d/main.tf", i)] = b.String()
	}
	for _, u := range g.units {
		var b strings.Builder
		for range u.pad {
			b.WriteString("# moved\n")
		}
		fmt.Fprintf(&b, "terraform {\n  source = \"../../modules/m%d\"\n}\n", u.mod)
		for _, d := range u.deps {
			fmt.Fprintf(&b, "dependency %q {\n  config_path = \"../%s\"\n}\n", d.label, d.target)
		}
		b.WriteString("inputs = {\n")
		for _, d := range u.deps {
			if d.out != "" {
				fmt.Fprintf(&b, "  in_%s = dependency.%s.outputs.%s\n", d.label, d.label, d.out)
			}
		}
		b.WriteString("}\n")
		if u.broken {
			b.WriteString("locals {\n")
		}
		m["live/"+u.name+"/terragrunt.hcl"] = b.String()
	}
	return m
}

func (g *gTree) unitIndex(name string) int {
	return slices.IndexFunc(g.units, func(u gUnit) bool { return u.name == name })
}

// genTree is a baseline tree D: 4 to 12 units, 1 to 3 modules, typed and
// sensitive members, and dependency chains (biased to the previous unit).
func genTree(r *xorshift) *gTree {
	g := &gTree{}
	nm := 1 + r.intn(3)
	for range nm {
		var md gMod
		for j := range 1 + r.intn(3) {
			md.vars = append(md.vars, gVar{fmt.Sprintf("v%d", j), genTypes[r.intn(len(genTypes))]})
		}
		for j := range 1 + r.intn(3) {
			md.outs = append(md.outs, gOut{fmt.Sprintf("o%d", j), r.intn(4) == 0})
		}
		g.mods = append(g.mods, md)
	}
	for i := range 4 + r.intn(9) {
		u := gUnit{name: fmt.Sprintf("u%02d", i), mod: r.intn(nm)}
		for k := 0; i > 0 && k < 1+r.intn(2); k++ {
			j := i - 1
			if r.intn(3) == 0 {
				j = r.intn(i)
			}
			dep := gDep{label: fmt.Sprintf("d%02d", j), target: g.units[j].name}
			if slices.ContainsFunc(u.deps, func(d gDep) bool { return d.label == dep.label }) {
				continue
			}
			if r.intn(2) == 0 {
				outs := g.mods[g.units[j].mod].outs
				dep.out = outs[r.intn(len(outs))].name
			}
			u.deps = append(u.deps, dep)
		}
		g.units = append(g.units, u)
	}
	return g
}

// referencedOutput picks a (module, output index) that some dependency
// reads, or -1 when none is read.
func (g *gTree) referencedOutput(r *xorshift) (int, int) {
	type ref struct{ mod, out int }
	var refs []ref
	for _, u := range g.units {
		for _, d := range u.deps {
			if d.out == "" {
				continue
			}
			if t := g.unitIndex(d.target); t >= 0 {
				m := g.units[t].mod
				if o := slices.IndexFunc(g.mods[m].outs, func(o gOut) bool { return o.name == d.out }); o >= 0 {
					refs = append(refs, ref{m, o})
				}
			}
		}
	}
	if len(refs) == 0 {
		return -1, -1
	}
	x := refs[r.intn(len(refs))]
	return x.mod, x.out
}

// pickOutput is a referenced output most of the time, else any output.
func (g *gTree) pickOutput(r *xorshift) (int, int) {
	if m, o := g.referencedOutput(r); m >= 0 && r.intn(4) != 0 {
		return m, o
	}
	m := r.intn(len(g.mods))
	if len(g.mods[m].outs) == 0 {
		return -1, -1
	}
	return m, r.intn(len(g.mods[m].outs))
}

// genMutations turn D into C; each reports whether it changed anything.
var genMutations = []struct {
	name  string
	apply func(g *gTree, r *xorshift) bool
}{
	{"remove output", func(g *gTree, r *xorshift) bool {
		m, o := g.pickOutput(r)
		if m < 0 {
			return false
		}
		g.mods[m].outs = slices.Delete(g.mods[m].outs, o, o+1)
		return true
	}},
	{"rename output", func(g *gTree, r *xorshift) bool {
		m, o := g.pickOutput(r)
		if m < 0 {
			return false
		}
		g.mods[m].outs[o].name += "r"
		return true
	}},
	{"add variable", func(g *gTree, r *xorshift) bool {
		g.serial++
		m := r.intn(len(g.mods))
		g.mods[m].vars = append(g.mods[m].vars, gVar{fmt.Sprintf("nv%d", g.serial), genTypes[r.intn(len(genTypes))]})
		return true
	}},
	{"change type", func(g *gTree, r *xorshift) bool {
		m := r.intn(len(g.mods))
		if len(g.mods[m].vars) == 0 {
			return false
		}
		v := &g.mods[m].vars[r.intn(len(g.mods[m].vars))]
		v.typ = genTypes[(slices.Index(genTypes, v.typ)+1+r.intn(len(genTypes)-1))%len(genTypes)]
		return true
	}},
	{"toggle sensitive", func(g *gTree, r *xorshift) bool {
		m, o := g.pickOutput(r)
		if m < 0 {
			return false
		}
		g.mods[m].outs[o].sensitive = !g.mods[m].outs[o].sensitive
		return true
	}},
	{"add GRT001", func(g *gTree, r *xorshift) bool {
		u := &g.units[r.intn(len(g.units))]
		if len(u.deps) == 0 {
			return false
		}
		u.deps[r.intn(len(u.deps))].out = "nope"
		return true
	}},
	{"add GRT002", func(g *gTree, r *xorshift) bool {
		g.serial++
		u := &g.units[r.intn(len(g.units))]
		name := fmt.Sprintf("gone%d", g.serial)
		u.deps = append(u.deps, gDep{label: name, target: name})
		return true
	}},
	{"add GRT100", func(g *gTree, r *xorshift) bool {
		g.units[r.intn(len(g.units))].broken = true
		return true
	}},
	{"move lines", func(g *gTree, r *xorshift) bool {
		g.units[r.intn(len(g.units))].pad += 1 + r.intn(2)
		return true
	}},
	{"delete unit", func(g *gTree, r *xorshift) bool {
		if len(g.units) <= 2 {
			return false
		}
		i := r.intn(len(g.units))
		g.units = slices.Delete(g.units, i, i+1)
		return true
	}},
	{"extend chain", func(g *gTree, r *xorshift) bool {
		g.serial++
		j := len(g.units) - 1
		if r.intn(2) == 0 {
			j = r.intn(len(g.units))
		}
		dep := gDep{label: "dx", target: g.units[j].name}
		if outs := g.mods[g.units[j].mod].outs; len(outs) > 0 && r.intn(2) == 0 {
			dep.out = outs[r.intn(len(outs))].name
		}
		g.units = append(g.units, gUnit{name: fmt.Sprintf("x%02d", g.serial), mod: r.intn(len(g.mods)), deps: []gDep{dep}})
		return true
	}},
}

// applyTree rewrites dir from the files of from to those of to: each
// changed or new file through a temp file and a rename, each removed file
// deleted along with its emptied directories. It returns the dirty paths
// (repo-relative, slash-separated), as a watcher reports them.
func applyTree(t *testing.T, dir string, from, to map[string]string) []string {
	t.Helper()
	var dirty []string
	for rel, data := range to {
		if old, ok := from[rel]; ok && old == data {
			continue
		}
		p := filepath.Join(dir, filepath.FromSlash(rel))
		mustDo(t, os.MkdirAll(filepath.Dir(p), 0o755))
		tmp := filepath.Join(filepath.Dir(p), ".gruntled-test-tmp")
		mustDo(t, os.WriteFile(tmp, []byte(data), 0o644))
		retryFS(t, func() error { return os.Rename(tmp, p) })
		dirty = append(dirty, rel)
	}
	for rel := range from {
		if _, ok := to[rel]; ok {
			continue
		}
		retryFS(t, func() error { return os.Remove(filepath.Join(dir, filepath.FromSlash(rel))) })
		dirty = append(dirty, rel)
		for d := filepath.Dir(rel); d != "."; d = filepath.Dir(d) {
			if os.Remove(filepath.Join(dir, d)) != nil {
				break // not empty
			}
			dirty = append(dirty, filepath.ToSlash(d))
		}
	}
	slices.Sort(dirty)
	return slices.Compact(dirty)
}

// cachedReports reproduces the daemon cache path: dir holds from, one
// invalidating loader indexes it, dir is rewritten to to, and the same
// loader reindexes with the dirty paths.
func cachedReports(t *testing.T, dir string, from, to map[string]string) (checking.Report, checking.Report) {
	t.Helper()
	writeFiles(t, dir, from)
	root, err := os.OpenRoot(dir)
	mustDo(t, err)
	defer root.Close()
	fsys := root.FS()
	idx := watching.NewIndexer(terragrunt.NewLoader(fsys), tfsurface.NewReader(fsys))
	r1, err := idx.Index(context.Background(), nil, false)
	mustDo(t, err)
	r2, err := idx.Index(context.Background(), applyTree(t, dir, from, to), false)
	mustDo(t, err)
	return r1, r2
}

// publishedView is the blast view a fresh publisher serves after
// publishing d, then c.
func publishedView(t *testing.T, d, c checking.Report) *ipc.BlastView {
	t.Helper()
	p := newTestPublisher(t)
	p.publish(ready(d))
	p.publish(ready(c))
	s := p.served.Load()
	if s == nil || s.Blast == nil || s.Blast.Error != "" {
		t.Fatalf("no blast view: %+v", s)
	}
	return s.Blast
}

// blastCounts is the part of a blast JSON document the counters read.
type blastCounts struct {
	Broken []struct {
		Findings []struct {
			Code string `json:"code"`
		} `json:"findings"`
	} `json:"broken"`
	Impacted []struct {
		Distance int `json:"distance"`
	} `json:"impacted"`
	Changes []struct {
		TypeChanges []json.RawMessage `json:"type_changes"`
	} `json:"changes"`
}

func (bc blastCounts) hasGRT004() bool {
	for _, b := range bc.Broken {
		for _, f := range b.Findings {
			if f.Code == "GRT004" {
				return true
			}
		}
	}
	return false
}

// firstDiff locates the first differing line of a and b.
func firstDiff(a, b string) string {
	la, lb := strings.Split(a, "\n"), strings.Split(b, "\n")
	for i := range max(len(la), len(lb)) {
		x, y := "<eof>", "<eof>"
		if i < len(la) {
			x = la[i]
		}
		if i < len(lb) {
			y = lb[i]
		}
		if x != y {
			return fmt.Sprintf("line %d:\n  daemon: %q\n  blast:  %q", i+1, x, y)
		}
	}
	return "equal"
}

// TestDaemonBlastEqualsBlastBase (DAEMON-07): for seeded random baselines
// D and mutated trees C, the view a publisher serves after Check(D) then
// Check(C) equals `blast --base D C` (text and JSON) after normalisation,
// with the same exit code. Variant a uses fresh loaders; variant b runs C
// through the same invalidating loader as D with the dirty paths (the
// daemon cache path). Counters prove the cases are not vacuous.
func TestDaemonBlastEqualsBlastBase(t *testing.T) {
	cases := 1000
	if testing.Short() {
		cases = 200
	}
	want := presenter.DaemonBaseline{Source: sourceWatchStart, Generation: 1, CurrentGeneration: 2}
	root := t.TempDir()
	r := xorshift(0x9e3779b97f4a7c15)
	var nBroken, nGRT004, nDeep, nTypes int
	for i := range cases {
		d := genTree(&r)
		c := d.clone()
		var applied []string
		for range 1 + r.intn(3) {
			m := genMutations[r.intn(len(genMutations))]
			if m.apply(c, &r) {
				applied = append(applied, m.name)
			}
		}
		dFiles, cFiles := d.files(), c.files()
		caseDir := filepath.Join(root, strconv.Itoa(i))
		dDir, cDir, wDir := filepath.Join(caseDir, "d"), filepath.Join(caseDir, "c"), filepath.Join(caseDir, "w")
		writeFiles(t, dDir, dFiles)
		writeFiles(t, cDir, cFiles)
		where := func() string {
			return fmt.Sprintf("case %d, mutations %q\nD: %q\nC: %q", i, applied, dFiles, cFiles)
		}

		type ref struct {
			out  string
			code int
		}
		var base [2]ref // text, json
		for k, args := range [][]string{
			{"blast", "--base", dDir, cDir},
			{"blast", "--format", "json", "--base", dDir, cDir},
		} {
			out, stderr, code := runCLI(t, args...)
			if stderr != "" || (code != exitOK && code != exitFindings) {
				t.Fatalf("%v: exit %d, stderr %q\n%s", args, code, stderr, where())
			}
			norm, err := normaliseBlastBase(out)
			if err != nil {
				t.Fatalf("%v: %v\n%s", args, err, where())
			}
			base[k] = ref{norm, code}
		}
		if base[0].code != base[1].code {
			t.Fatalf("blast text exit %d, json exit %d\n%s", base[0].code, base[1].code, where())
		}

		ra1, ra2 := freshReport(t, dDir), freshReport(t, cDir)
		rb1, rb2 := cachedReports(t, wDir, dFiles, cFiles)
		for _, v := range []struct {
			name string
			view *ipc.BlastView
		}{
			{"fresh loaders", publishedView(t, ra1, ra2)},
			{"shared invalidating loader", publishedView(t, rb1, rb2)},
		} {
			code := exitOK
			if v.view.HasErrors {
				code = exitFindings
			}
			if code != base[0].code {
				t.Fatalf("%s: daemon exit %d, blast exit %d\n%s", v.name, code, base[0].code, where())
			}
			for k, doc := range []string{string(v.view.Text), string(v.view.JSON)} {
				got, err := normaliseDaemonBlast(doc, want)
				if err != nil {
					t.Fatalf("%s: %v\n%s", v.name, err, where())
				}
				if got != base[k].out {
					hint := ""
					if v.name == "shared invalidating loader" {
						hint = " (variant b only: a Loader cache bug, 16-RESEARCH risk 1)"
					}
					t.Fatalf("%s%s: view differs from blast --base, %s\n%s", v.name, hint, firstDiff(got, base[k].out), where())
				}
			}
		}

		var bc blastCounts
		mustDo(t, json.Unmarshal([]byte(base[1].out), &bc))
		if len(bc.Broken) > 0 {
			nBroken++
		}
		if bc.hasGRT004() {
			nGRT004++
		}
		for _, im := range bc.Impacted {
			if im.Distance >= 2 {
				nDeep++
				break
			}
		}
		for _, ch := range bc.Changes {
			if len(ch.TypeChanges) > 0 {
				nTypes++
				break
			}
		}
		mustDo(t, os.RemoveAll(caseDir))
	}
	t.Logf("%d cases: broken %d, GRT004 %d, impacted distance >= 2 %d, type changes %d", cases, nBroken, nGRT004, nDeep, nTypes)
	if nBroken == 0 || nGRT004 == 0 || nDeep == 0 || nTypes == 0 {
		t.Fatalf("vacuous property: broken %d, GRT004 %d, distance >= 2 %d, type changes %d", nBroken, nGRT004, nDeep, nTypes)
	}
}

// daemonLabelRE is the first line of a daemon blast view.
var daemonLabelRE = regexp.MustCompile(`^baseline: daemon generation ([0-9]+) \(([a-z ]+)\), current generation ([0-9]+)\n`)

// servedBaseline parses the label of a daemon blast view.
func servedBaseline(text string) (presenter.DaemonBaseline, bool) {
	m := daemonLabelRE.FindStringSubmatch(text)
	if m == nil {
		return presenter.DaemonBaseline{}, false
	}
	g0, err0 := strconv.ParseUint(m[1], 10, 64)
	g, err := strconv.ParseUint(m[3], 10, 64)
	if err0 != nil || err != nil {
		return presenter.DaemonBaseline{}, false
	}
	return presenter.DaemonBaseline{Source: m[2], Generation: g0, CurrentGeneration: g}, true
}

// waitBlastFinal waits until the daemon on dir serves a blast view past
// its first generation, its plain report equals check on dir, AND the
// blast view holds every line in final: the daemon then holds the final
// tree, never a half-written one. The report/check comparison alone is
// vacuous when the edit leaves check output unchanged (a type-only change),
// and applyTree's temp file can make a generation of its own before the
// rename lands; final must therefore name lines only the final tree's blast
// view can contain (TestReportBlastE2E passes each scenario's has lines).
func waitBlastFinal(t *testing.T, dir string, report func(args ...string) cliResult, final ...string) {
	t.Helper()
	var last string
	eventually(t, func() bool {
		bl := report("--blast", dir)
		d, ok := servedBaseline(bl.stdout)
		if !ok || d.CurrentGeneration < 2 {
			last = "report --blast: " + bl.String()
			return false
		}
		for _, w := range final {
			if !strings.Contains(bl.stdout, w) {
				last = fmt.Sprintf("report --blast lacks %q: %s", w, bl.String())
				return false
			}
		}
		got, want := report("--format", "text", dir), runCheckAt("text", dir)
		if got != want {
			last = "report " + got.String() + "\ncheck " + want.String()
			return false
		}
		return true
	}, "the final tree in report and report --blast", func() string { return last })
}

// assertBlastEquivalent compares report --blast (text and json) on dir
// with blast --base baseDir dir, normalised, with the same exit code.
func assertBlastEquivalent(t *testing.T, report func(args ...string) cliResult, baseDir, dir string) {
	t.Helper()
	text := report("--blast", dir)
	want, ok := servedBaseline(text.stdout)
	if !ok || want.Source != sourceWatchStart || want.Generation != 1 || want.CurrentGeneration < 2 {
		t.Fatalf("report --blast label: %s", text)
	}
	for _, f := range []string{"text", "json"} {
		got := report("--blast", "--format", f, dir)
		out, stderr, code := runCLI(t, "blast", "--format", f, "--base", baseDir, dir)
		if got.stderr != "" || stderr != "" || got.code != code {
			t.Fatalf("%s: report --blast %s\nblast --base: exit %d, stderr %q", f, got, code, stderr)
		}
		g, err := normaliseDaemonBlast(got.stdout, want)
		mustDo(t, err)
		b, err := normaliseBlastBase(out)
		mustDo(t, err)
		if g != b {
			t.Fatalf("%s: report --blast differs from blast --base, %s\nreport --blast:\n%s\nblast --base:\n%s", f, firstDiff(g, b), got.stdout, out)
		}
	}
}

// e2eScenarios are fixed (D, C) pairs for the real daemon.
var e2eScenarios = []struct {
	name string
	d, c map[string]string
	has  []string // lines report --blast must hold (non-vacuity)
}{
	{
		"GRT004 and a distance-4 chain",
		map[string]string{
			"modules/net/main.tf":       "output \"id\" { value = \"x\" }\noutput \"old\" { value = \"y\" }\n",
			"live/net/terragrunt.hcl":   "terraform {\n  source = \"../../modules/net\"\n}\n",
			"live/app/terragrunt.hcl":   "dependency \"net\" {\n  config_path = \"../net\"\n}\ninputs = { id = dependency.net.outputs.old }\n",
			"live/a/terragrunt.hcl":     "dependency \"net\" {\n  config_path = \"../net\"\n}\n",
			"live/b/terragrunt.hcl":     "dependency \"a\" {\n  config_path = \"../a\"\n}\n",
			"live/c/terragrunt.hcl":     "dependency \"b\" {\n  config_path = \"../b\"\n}\n",
			"live/other/terragrunt.hcl": "",
		},
		nil,
		[]string{
			"    live/app/terragrunt.hcl:4:17: GRT004 ",
			"  live/b (distance 3, from module modules/net, path live/b -> live/a -> live/net)\n",
			"  live/c (distance 4, ",
		},
	},
	{
		"type-only change",
		map[string]string{
			"modules/net/main.tf":     "variable \"cidr\" {\n  type = string\n}\noutput \"id\" {\n  value = \"x\"\n}\n",
			"live/net/terragrunt.hcl": "terraform {\n  source = \"../../modules/net\"\n}\n",
			"live/a/terragrunt.hcl":   "dependency \"net\" {\n  config_path = \"../net\"\n}\ninputs = { id = dependency.net.outputs.id }\n",
		},
		nil,
		[]string{"\nBroken (0):\n", "\nType changes (1):\n  modules/net variable cidr: type string -> bool\n"},
	},
	{
		"GRT100 and GRT002",
		map[string]string{
			"modules/net/main.tf":     "output \"id\" { value = \"x\" }\n",
			"live/net/terragrunt.hcl": "terraform {\n  source = \"../../modules/net\"\n}\n",
			"live/a/terragrunt.hcl":   "dependency \"net\" {\n  config_path = \"../net\"\n}\ninputs = { id = dependency.net.outputs.id }\n",
			"live/b/terragrunt.hcl":   "dependency \"a\" {\n  config_path = \"../a\"\n}\n",
		},
		nil,
		[]string{": GRT100 ", "    live/b/terragrunt.hcl:5:17: GRT002 "},
	},
}

func init() {
	edit := func(d map[string]string, set map[string]string) map[string]string {
		c := map[string]string{}
		for k, v := range d {
			c[k] = v
		}
		for k, v := range set {
			c[k] = v
		}
		return c
	}
	e2eScenarios[0].c = edit(e2eScenarios[0].d, map[string]string{"modules/net/main.tf": "output \"id\" { value = \"x\" }\n"})
	// The poll scanner cannot see an edit that keeps size and mtime
	// (watch/poll.go scanner), so the type change must change the size:
	// string -> bool, not string -> number.
	e2eScenarios[1].c = edit(e2eScenarios[1].d, map[string]string{"modules/net/main.tf": "variable \"cidr\" {\n  type = bool\n}\noutput \"id\" {\n  value = \"x\"\n}\n"})
	e2eScenarios[2].c = edit(e2eScenarios[2].d, map[string]string{
		"live/a/terragrunt.hcl": "dependency \"net\" {\n  config_path = \"../net\"\n}\ninputs = { id = dependency.net.outputs.id }\nlocals {\n",
		"live/b/terragrunt.hcl": "dependency \"a\" {\n  config_path = \"../a\"\n}\ndependency \"gone\" {\n  config_path = \"../gone\"\n}\n",
	})
}

// TestReportBlastE2E (DAEMON-07): through a real daemon on both
// transports, report --blast equals blast --base over a copy of the
// baseline, text and JSON, with the same exit code; after a rebase (socket
// only) the view is empty, exit 0, labelled (rebase).
func TestReportBlastE2E(t *testing.T) {
	for _, goos := range reportGOOS {
		for _, sc := range e2eScenarios {
			t.Run(goos+"/"+sc.name, func(t *testing.T) {
				dir, baseCopy := t.TempDir(), t.TempDir()
				writeFiles(t, dir, sc.d)
				writeFiles(t, baseCopy, sc.d)
				deps := testDeps(t)
				deps.goos = goos
				startWatch(t, dir, deps, "--poll")
				report := func(args ...string) cliResult { return runReportArgs(deps.env, goos, args...) }
				var first cliResult
				eventually(t, func() bool { first = report(dir); return first.code != exitFailure },
					"first report", func() string { return first.String() })

				applyTree(t, dir, sc.d, sc.c)
				waitBlastFinal(t, dir, report, sc.has...)
				assertBlastEquivalent(t, report, baseCopy, dir)
				view := report("--blast", dir)
				for _, h := range sc.has {
					if !strings.Contains(view.stdout, h) {
						t.Fatalf("report --blast lacks %q:\n%s", h, view.stdout)
					}
				}

				if goos == "windows" {
					return // rebase needs the socket
				}
				skipNoUnixSocket(t)
				g := servedGeneration(t, deps.env, dir)
				got := runRebaseAt(deps.env, goos, "--expect", strconv.FormatUint(g, 10), dir)
				if got.code != exitOK || got.stderr != "" {
					t.Fatalf("rebase: %s", got)
				}
				text := report("--blast", dir)
				label := "baseline: " + daemonLabel(sourceRebase, g, g) + "\n"
				if text.code != exitOK || text.stderr != "" || text.stdout != label+"Broken (0):\nImpacted (0):\n" {
					t.Fatalf("report --blast after rebase: %s", text)
				}
				js := report("--blast", "--format", "json", dir)
				var doc struct {
					Broken   []any `json:"broken"`
					Impacted []any `json:"impacted"`
					Changes  []any `json:"changes"`
					Daemon   struct {
						Source string `json:"source"`
					} `json:"daemon_baseline"`
				}
				mustDo(t, json.Unmarshal([]byte(js.stdout), &doc))
				if js.code != exitOK || doc.Broken == nil || doc.Impacted == nil || doc.Changes == nil ||
					len(doc.Broken)+len(doc.Impacted)+len(doc.Changes) != 0 || doc.Daemon.Source != sourceRebase {
					t.Fatalf("report --blast json after rebase: %s", js)
				}
			})
		}
	}
}

// TestReportBlastPitfall15 (T-13-02-7): a removed output still read is a
// GRT004 in report --blast; after rebase it leaves Broken, while plain
// report still shows the GRT001 at the same site.
func TestReportBlastPitfall15(t *testing.T) {
	skipNoUnixSocket(t)
	d := map[string]string{
		"vpc/terragrunt.hcl": "",
		"vpc/main.tf":        "output \"vpc_id\" { value = \"x\" }\noutput \"old\" { value = \"y\" }\n",
		"app/terragrunt.hcl": "dependency \"vpc\" {\n  config_path = \"../vpc\"\n}\ninputs = { id = dependency.vpc.outputs.old }\n",
		"app/main.tf":        "variable \"id\" {}\n",
	}
	c := map[string]string{}
	for k, v := range d {
		c[k] = v
	}
	c["vpc/main.tf"] = "output \"vpc_id\" { value = \"x\" }\n"
	dir := t.TempDir()
	writeFiles(t, dir, d)
	deps := testDeps(t)
	startWatch(t, dir, deps, "--poll")
	report := func(args ...string) cliResult { return runReportArgs(deps.env, "linux", args...) }
	var first cliResult
	eventually(t, func() bool { first = report(dir); return first.code == exitOK }, "first clean report", func() string { return first.String() })

	applyTree(t, dir, d, c)
	waitBlastFinal(t, dir, report)
	before := report("--blast", dir)
	const site = "app/terragrunt.hcl:4:17"
	if before.code != exitFindings || !strings.Contains(before.stdout, "    "+site+": GRT004 ") || strings.Contains(before.stdout, "GRT001") {
		t.Fatalf("report --blast before rebase: %s", before)
	}
	got := runRebaseAt(deps.env, "linux", dir)
	if got.code != exitOK || !strings.HasSuffix(got.stdout, "; 1 error findings in Broken accepted\n") {
		t.Fatalf("rebase: %s", got)
	}
	after := report("--blast", dir)
	if after.code != exitOK || !strings.Contains(after.stdout, "\nBroken (0):\n") || strings.Contains(after.stdout, "GRT004") {
		t.Fatalf("report --blast after rebase: %s", after)
	}
	if plain := report(dir); plain.code != exitFindings || !strings.Contains(plain.stdout, site+": GRT001 ") {
		t.Fatalf("plain report after rebase: %s", plain)
	}
}
