package main

// REL-03 clause 3 (17-03): every mutation of the set is caught, with exact
// equality, on every eligible site of the included modules of each pinned
// corpus. One mutation at a time on one working copy per corpus and class,
// through runMutation (proven able to fail by TestCorpusHarnessSelfTest);
// expected results come from expectBlast (model + descriptor), never from
// gruntled. Exactness has four named exceptions (sec #419): type_changes.old
// for non-primitive types is checked for inequality only; GRT004 messages by
// prefix (the dual oracle covers the rest); text output by tokens once per
// class (JSON is the authority); SENTINEL (m17SentinelCanon) pinned from
// HEAD after a hand check.
//
// M1 has a dual oracle: GRUNTLED_REF_BIN, a v0.3.0 binary built by
// scripts/corpus-gate.sh, must report GRT001 at exactly the sites where HEAD
// reports GRT004. The binary is accepted only with the pinned --version label
// AND a behavioural fingerprint no v0.4 build passes (sec #417).

import (
	"bytes"
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"
)

const refBinVersion = "gruntled v0.3.0 (e870e2984760f0a674bc87697e3f69329613758e)"

var refBinChecked sync.Map // path -> error

// refBin returns GRUNTLED_REF_BIN after checking it is v0.3.0: the label and
// the fingerprint (`rebase` is an unknown command; `blast --depth` is an
// undefined flag). Missing or wrong fails the test, never skips it.
func refBin(t testing.TB) string {
	t.Helper()
	bin := os.Getenv("GRUNTLED_REF_BIN")
	if bin == "" {
		t.Fatalf("GRUNTLED_REF_BIN is not set: M1's dual oracle needs a v0.3.0 binary; run scripts/corpus-gate.sh, which builds and exports it")
	}
	if v, ok := refBinChecked.Load(bin); ok {
		if v != nil {
			t.Fatal(v)
		}
		return bin
	}
	err := checkRefBin(bin)
	refBinChecked.Store(bin, err)
	if err != nil {
		t.Fatal(err)
	}
	return bin
}

func runBin(bin, dir string, args ...string) (stdout, stderr string, code int, err error) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Dir = dir
	var o, e bytes.Buffer
	cmd.Stdout, cmd.Stderr = &o, &e
	err = cmd.Run()
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		return o.String(), e.String(), ee.ExitCode(), nil
	}
	return o.String(), e.String(), 0, err
}

func checkRefBin(bin string) error {
	dir, err := os.MkdirTemp("", "refbin")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)
	out, _, code, err := runBin(bin, dir, "--version")
	if err != nil || code != 0 || strings.TrimSpace(out) != refBinVersion {
		return fmt.Errorf("GRUNTLED_REF_BIN %s: --version %q (exit %d, %v), want %q", bin, strings.TrimSpace(out), code, err, refBinVersion)
	}
	_, stderr, code, err := runBin(bin, dir, "rebase")
	first, _, _ := strings.Cut(stderr, "\n")
	if err != nil || code != 2 || first != `gruntled: unknown command "rebase"` {
		return fmt.Errorf("GRUNTLED_REF_BIN %s fails the fingerprint: rebase exit %d, stderr line 1 %q (a v0.3.0 build does not know rebase)", bin, code, first)
	}
	_, stderr, code, err = runBin(bin, dir, "blast", "--depth", "1", dir)
	if err != nil || code != 2 || !strings.Contains(stderr, "flag provided but not defined: -depth") {
		return fmt.Errorf("GRUNTLED_REF_BIN %s fails the fingerprint: blast --depth exit %d, stderr %q (a v0.3.0 build has no --depth)", bin, code, stderr)
	}
	return nil
}

// dualCompare compares v0.3.0's GRT001 set for the removed (module, output)
// with HEAD's GRT004 set: same (unit, file, line, col), messages equal after
// rewriting `was removed from` to `is not declared by`. An empty set fails:
// M1 only enumerates outputs with a firing site.
func dualCompare(m mutation, rep corpusReport, doc blastDoc) []string {
	key := func(unit, file string, line, col int, msg string) string {
		return fmt.Sprintf("%s %s:%d:%d %s", unit, file, line, col, msg)
	}
	var ref, head []string
	for _, d := range rep.Diagnostics {
		if d.Code == "GRT001" && strings.Contains(d.Message, fmt.Sprintf("output %q is not declared by module %q", m.Name, m.Module)) {
			ref = append(ref, key(d.Unit, d.File, d.Line, d.Column, d.Message))
		}
	}
	for _, b := range doc.Broken {
		for _, f := range b.Findings {
			if f.Code == "GRT004" {
				head = append(head, key(b.Unit, f.File, f.Line, f.Column, strings.Replace(f.Message, "was removed from", "is not declared by", 1)))
			}
		}
	}
	slices.Sort(ref)
	slices.Sort(head)
	if !slices.Equal(ref, head) || len(ref) == 0 {
		return []string{fmt.Sprintf("dual oracle: v0.3.0 GRT001 %v, HEAD GRT004 %v", ref, head)}
	}
	return nil
}

// dualOracle runs GRUNTLED_REF_BIN check on the mutated working copy and
// applies dualCompare.
func dualOracle(bin string) func(r reporter, m mutation, doc blastDoc) {
	return func(r reporter, m mutation, doc blastDoc) {
		if m.Class != "M1" {
			return
		}
		out, stderr, code, err := runBin(bin, currentHarness.cur, "check", "--format", "json", ".")
		if err != nil {
			r.Errorf("%s: v0.3.0 check: %v %s", m, err, stderr)
			return
		}
		var rep corpusReport
		if err := json.Unmarshal([]byte(out), &rep); err != nil {
			r.Errorf("%s: v0.3.0 check JSON (exit %d): %v", m, code, err)
			return
		}
		for _, d := range dualCompare(m, rep, doc) {
			r.Errorf("%s: %s", m, d)
		}
	}
}

// currentHarness is the harness the dual oracle reads the working copy from
// (one shard runs at a time, -p=1).
var currentHarness *harness

// ---------------------------------------------------------------------------
// Enumeration.

// mutSet is one class's enumeration on one corpus.
type mutSet struct {
	Muts     []mutation
	Touch    int            // excluded_by_touch
	Excluded map[string]int // other exclusions by reason
}

// simpleType reports whether s is in the oracle's simple type grammar:
// string | number | bool | any | list(T) | set(T) | map(T) | tuple([T,..])
// | object({k = T, ..}) | optional(T), with identifier or quoted keys.
func simpleType(s string) bool {
	p := &typeParser{s: s}
	p.space()
	if !p.typ() {
		return false
	}
	p.space()
	return p.i == len(p.s)
}

type typeParser struct {
	s string
	i int
}

func (p *typeParser) space() {
	for p.i < len(p.s) && strings.ContainsRune(" \t\r\n", rune(p.s[p.i])) {
		p.i++
	}
}

func (p *typeParser) lit(c byte) bool {
	p.space()
	if p.i < len(p.s) && p.s[p.i] == c {
		p.i++
		return true
	}
	return false
}

func (p *typeParser) ident() string {
	p.space()
	m := modelIdentRe.FindString(p.s[p.i:])
	p.i += len(m)
	return m
}

func (p *typeParser) typ() bool {
	switch p.ident() {
	case "string", "number", "bool", "any":
		return true
	case "list", "set", "map", "optional":
		return p.lit('(') && p.typ() && p.lit(')')
	case "tuple":
		if !p.lit('(') || !p.lit('[') {
			return false
		}
		for !p.lit(']') {
			if !p.typ() {
				return false
			}
			p.lit(',')
		}
		return p.lit(')')
	case "object":
		if !p.lit('(') || !p.lit('{') {
			return false
		}
		for !p.lit('}') {
			p.space()
			if p.i < len(p.s) && p.s[p.i] == '"' {
				end := strings.IndexByte(p.s[p.i+1:], '"')
				if end < 0 || strings.ContainsAny(p.s[p.i+1:p.i+1+end], `\$%`) {
					return false
				}
				p.i += end + 2
			} else if p.ident() == "" {
				return false
			}
			if !p.lit('=') || !p.typ() {
				return false
			}
			p.lit(',')
		}
		return p.lit(')')
	}
	return false
}

// declInfo indexes one module's declarations: per (kind, name) how many
// declarations exist across all its .tf (regular and linked) and .tf.json
// files, and the regular-file block when there is exactly one.
type declInfo struct {
	count  map[string]int
	linked map[string]bool
	block map[string]struct {
		File string
		B    tfBlock
		Src  string
	}
}

func moduleDecls(root, mod string) declInfo {
	d := declInfo{count: map[string]int{}, linked: map[string]bool{}, block: map[string]struct {
		File string
		B    tfBlock
		Src  string
	}{}}
	for _, links := range []bool{false, true} {
		for _, rel := range moduleTFFiles(root, mod, links) {
			b, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel)))
			if err != nil {
				continue
			}
			blocks, ok := findBlocks(string(b))
			if !ok {
				continue
			}
			for _, bl := range blocks {
				k := bl.Kind + " " + bl.Name
				d.count[k]++
				if links {
					d.linked[k] = true
				}
				if !links {
					d.block[k] = struct {
						File string
						B    tfBlock
						Src  string
					}{rel, bl, string(b)}
				}
			}
		}
	}
	ents, _ := os.ReadDir(filepath.Join(root, filepath.FromSlash(mod)))
	for _, e := range ents {
		if strings.HasSuffix(e.Name(), ".tf.json") {
			vars, outs := map[string]bool{}, map[string]bool{}
			if b, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(mod), e.Name())); err == nil && modelJSONNames(b, vars, outs) {
				for v := range vars {
					d.count["variable "+v]++
				}
				for o := range outs {
					d.count["output "+o]++
				}
			}
		}
	}
	return d
}

// sharedTargets maps the real path of every file a module links (a symlinked
// .tf) to the model modules linking it.
func sharedTargets(m *corpusModel) map[string][]string {
	out := map[string][]string{}
	for _, mod := range slices.Sorted(maps(m.Modules)) {
		if len(m.Modules[mod].Seeds) == 0 {
			continue
		}
		for _, rel := range moduleTFFiles(m.Root, mod, true) {
			real, err := filepath.EvalSymlinks(filepath.Join(m.Root, filepath.FromSlash(rel)))
			if err != nil {
				continue
			}
			out[real] = append(out[real], mod)
		}
	}
	return out
}

// enumerate lists class's mutations on the included modules of m, in
// (module, file, byte offset) order, with the exclusions counted.
func enumerate(m *corpusModel, ex []exclusion, included []string, class string) mutSet {
	set := mutSet{Excluded: map[string]int{}}
	inc := map[string]bool{}
	for _, p := range included {
		inc[p] = true
	}
	// (module, output) pairs with a site of shape other (catRefOther rows).
	otherPair := map[string]bool{}
	for _, e := range ex {
		if e.Category != catRefOther {
			continue
		}
		for _, s := range m.Units[e.Path].Sites {
			if fmt.Sprintf("%s:%d", s.File, s.Line) == e.Evidence && s.TargetUnit != "" {
				otherPair[m.Units[s.TargetUnit].Module+" "+s.Output] = true
			}
		}
	}
	firing := map[string]bool{}
	anySite := map[string]bool{}
	for _, s := range m.Sites {
		if s.TargetAny != "" && m.Units[s.TargetAny] != nil {
			anySite[m.Units[s.TargetAny].Module+" "+s.Output] = true
		}
		if !s.Silent && s.TargetUnit != "" {
			firing[m.Units[s.TargetUnit].Module+" "+s.Output] = true
		}
	}
	shared := sharedTargets(m)
	isShared := func(rel string) bool {
		real, err := filepath.EvalSymlinks(filepath.Join(m.Root, filepath.FromSlash(rel)))
		return err == nil && len(shared[real]) > 0
	}
	type cand struct {
		mod, file string
		off       int
		mu        mutation
	}
	var cands []cand
	for _, mod := range included {
		if class == "M3" {
			if _, err := os.Lstat(filepath.Join(m.Root, filepath.FromSlash(mod), m17File)); err == nil {
				set.Excluded["m17-file-exists"]++
				continue
			}
			cands = append(cands, cand{mod, "", 0, mutation{Class: "M3", Module: mod}})
			continue
		}
		if class == "M9" {
			continue
		}
		d := moduleDecls(m.Root, mod)
		kind := "output"
		if class == "M4" || class == "M5" {
			kind = "variable"
		}
		var names []string
		for k := range d.count {
			if strings.HasPrefix(k, kind+" ") {
				names = append(names, strings.TrimPrefix(k, kind+" "))
			}
		}
		slices.Sort(names)
		for _, name := range names {
			k := kind + " " + name
			pair := mod + " " + name
			switch class {
			case "M1":
				if !firing[pair] {
					continue
				}
			case "M2":
				if anySite[pair] {
					continue
				}
			}
			if class == "M1" || class == "M2" {
				if otherPair[pair] {
					set.Touch++
					continue
				}
			}
			blk, ok := d.block[k]
			switch {
			case d.count[k] > 1:
				set.Excluded["duplicate-declaration"]++
				continue
			case !ok && d.linked[k]:
				set.Excluded["declared-in-shared-link"]++ // M9 edits the link target
				continue
			case !ok:
				set.Excluded["declared-in-json"]++
				continue
			case isShared(blk.File):
				set.Touch++ // the file is also linked by another module
				continue
			}
			mu := mutation{Class: class, Module: mod, File: blk.File, Name: name}
			switch class {
			case "M4":
				if _, has := blk.B.attr("default"); !has {
					continue
				}
			case "M5":
				if a, has := blk.B.attr("type"); has {
					raw := strings.TrimSpace(blk.Src[a.ValueStart:a.End])
					if blk.B.OneLine && a.End == blk.B.Close {
						raw = strings.TrimSpace(blk.Src[a.ValueStart:blk.B.Close])
					}
					if !simpleType(raw) {
						set.Excluded["type-grammar"]++
						continue
					}
					mu.Detail = raw
				}
			case "M6":
				if _, has := blk.B.attr("type"); has {
					set.Excluded["type-declared"]++
					continue
				}
			case "M7":
				mu.Detail = "absent"
				if a, has := blk.B.attr("sensitive"); has {
					raw := strings.TrimSpace(blk.Src[a.ValueStart:a.End])
					if blk.B.OneLine && a.End == blk.B.Close {
						raw = strings.TrimSpace(blk.Src[a.ValueStart:blk.B.Close])
					}
					if raw != "true" && raw != "false" {
						set.Excluded["sensitive-nonliteral"]++
						continue
					}
					mu.Detail = raw
				}
			}
			cands = append(cands, cand{mod, blk.File, blk.B.Start, mu})
		}
	}
	if class == "M9" {
		for _, real := range slices.Sorted(maps(shared)) {
			rel, err := filepath.Rel(m.Root, real)
			if err != nil || strings.HasPrefix(rel, "..") {
				continue
			}
			rel = filepath.ToSlash(rel)
			// A linker instantiated only by excluded units is outside the
			// universe (its units carry no module, per their pinned rows): it
			// is dropped from the expected changes, never corrected. A linker
			// with some excluded seeds, or an eligible but not included one,
			// touches an excluded item.
			exUnit := map[string]bool{}
			for _, e := range ex {
				exUnit[e.Path] = true
			}
			var linkers []string
			allInc := true
			for _, l := range shared[real] {
				nEx := 0
				for _, s := range m.Modules[l].Seeds {
					if exUnit[s] {
						nEx++
					}
				}
				switch {
				case nEx == len(m.Modules[l].Seeds):
					set.Excluded["linker-of-excluded-units"]++
				case inc[l]:
					linkers = append(linkers, l)
				default:
					allInc = false
				}
			}
			b, err := os.ReadFile(real)
			if err != nil {
				continue
			}
			src := string(b)
			blocks, ok := findBlocks(src)
			if !ok {
				continue
			}
			for _, bl := range blocks {
				if bl.Kind != "variable" {
					continue
				}
				dup := false
				for _, l := range linkers {
					if moduleDecls(m.Root, l).count["variable "+bl.Name] > 1 {
						dup = true
					}
				}
				var subs []string
				if _, has := bl.attr("default"); has {
					subs = append(subs, "M4")
				}
				old := ""
				typeOK := true
				if a, has := bl.attr("type"); has {
					old = strings.TrimSpace(src[a.ValueStart:a.End])
					typeOK = simpleType(old)
				}
				if typeOK {
					subs = append(subs, "M5:"+old)
				} else {
					set.Excluded["type-grammar"]++
				}
				for _, sub := range subs {
					switch {
					case !allInc:
						set.Touch++
					case dup:
						set.Excluded["duplicate-declaration"]++
					default:
						cands = append(cands, cand{"", rel, bl.Start, mutation{Class: "M9", File: rel, Name: bl.Name, Detail: sub, Modules: slices.Clone(linkers)}})
					}
				}
			}
		}
	}
	slices.SortStableFunc(cands, func(a, b cand) int {
		return cmp.Or(cmp.Compare(a.mod, b.mod), cmp.Compare(a.file, b.file), cmp.Compare(a.off, b.off))
	})
	for _, c := range cands {
		set.Muts = append(set.Muts, c.mu)
	}
	return set
}

// maxDistance is the largest distance among the Impacted and Broken entries
// of an expected result.
func maxDistance(w blastWant) int {
	d := 0
	for _, im := range w.Doc.Impacted {
		d = max(d, im.Distance)
	}
	for _, b := range w.Doc.Broken {
		if b.Distance != nil {
			d = max(d, *b.Distance)
		}
	}
	return d
}

// mutPin is the pinned count of one (corpus, class).
type mutPin struct {
	Sites, Touch int
	Excluded     map[string]int
}

var mutationClasses = []string{"M1", "M2", "M3", "M4", "M5", "M6", "M7", "M9"}

// corpusMutationPins pins sites, excluded_by_touch and the other
// exclusions per corpus and class (sec #418); 17-04 pins the same values
// in docs/validation.md.
var corpusMutationPins = map[string]map[string]mutPin{
	"iso20022": {},
	"secret":   {},
	"denis256": {},
}

// corpusDepthPins pins, per corpus, the mutations whose expected result has
// an entry at distance >= 2, 3, 4 and 5 (M8).
var corpusDepthPins = map[string][4]int{
	"iso20022": {},
	"secret":   {},
	"denis256": {},
}

// TestCorpusMutation17 runs the mutation set, one class per subtest.
func TestCorpusMutation17(t *testing.T) {
	for _, repo := range graphRepos {
		t.Run(repo.Name, func(t *testing.T) {
			root := repo.Require(t)
			before := corpusDigest(t, root)
			model := buildCorpusModel(t, root)
			ex := corpusExclusions[repo.Name]
			included, _, _ := model.universe(ex)
			if len(included) != corpusIncluded[repo.Name] {
				t.Fatalf("included modules %d, pinned %d", len(included), corpusIncluded[repo.Name])
			}
			for _, class := range mutationClasses {
				t.Run(class, func(t *testing.T) {
					set := enumerate(model, ex, included, class)
					if class == "M9" && len(set.Muts)+set.Touch == 0 && repo.Name != "iso20022" {
						t.Logf("CORPUS-MUTATION %s M9 sites=0 caught=0 excluded_by_touch=0 (no file shared by symlink)", repo.Name)
						checkPin(t, repo.Name, class, set, 0)
						return
					}
					h := newHarness(t, model, repo.Copy(t, root), repo.Copy(t, root))
					h.pristine = root
					h.textDone = map[string]bool{}
					currentHarness = h
					if class == "M1" {
						h.onApplied = dualOracle(refBin(t))
					}
					for _, m := range set.Muts {
						runMutation(t, h, m, nil)
					}
					h.gBase.Final()
					h.gCur.Final()
					t.Logf("CORPUS-MUTATION %s %s sites=%d caught=%d excluded_by_touch=%d excluded=%v", repo.Name, class, h.sites, h.caught, set.Touch, set.Excluded)
					if h.caught != h.sites {
						t.Errorf("caught %d of %d sites", h.caught, h.sites)
					}
					checkPin(t, repo.Name, class, set, h.sites)
				})
			}
			t.Run("M8", func(t *testing.T) {
				var got [4]int
				for _, class := range mutationClasses {
					for _, m := range enumerate(model, ex, included, class).Muts {
						d := maxDistance(expectBlast(model, m))
						for k := 2; k <= 5; k++ {
							if d >= k {
								got[k-2]++
							}
						}
					}
				}
				t.Logf("CORPUS-MUTATION %s M8 depth>=2=%d depth>=3=%d depth>=4=%d depth>=5=%d", repo.Name, got[0], got[1], got[2], got[3])
				if got != corpusDepthPins[repo.Name] {
					t.Errorf("depth counters %v, pinned %v", got, corpusDepthPins[repo.Name])
				}
				if got[0] == 0 {
					t.Errorf("no mutation propagates to distance 2")
				}
				if repo.Name == "denis256" && (got[1] == 0 || got[2] == 0 || got[3] == 0) {
					t.Errorf("denis256 needs mutations at distance 3, 4 and 5: %v", got)
				}
			})
			if after := corpusDigest(t, root); after != before {
				t.Errorf("checkout changed")
			}
		})
	}
}

func checkPin(t *testing.T, corpus, class string, set mutSet, sites int) {
	t.Helper()
	want := corpusMutationPins[corpus][class]
	if want.Excluded == nil {
		want.Excluded = map[string]int{}
	}
	if sites != want.Sites || set.Touch != want.Touch || fmt.Sprint(set.Excluded) != fmt.Sprint(want.Excluded) {
		t.Errorf("%s %s: sites=%d excluded_by_touch=%d excluded=%v, pinned %+v", corpus, class, sites, set.Touch, set.Excluded, want)
	}
}

var _ = path.Join
