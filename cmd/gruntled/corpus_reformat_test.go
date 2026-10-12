package main

// REL-03 clause 2 (17-02 Task 3): reformat-only transforms give an empty
// blast. Each transform R1-R9 is applied corpus-wide over the included
// modules' regular .tf files (never terragrunt.hcl, never a symlink except
// R9, never inside a string, template, heredoc or comment), and must give
// exit 0 with Broken, Impacted and changes all empty (stricter than
// REL-03's empty Impacted), pass the non-vacuity guard (same modules,
// names, unit statuses, no new GRT100), and count at least one edit.

import (
	"bytes"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"testing"
)

// patch replaces src[At:At+Del] with Ins.
type patch struct {
	At, Del int
	Ins     string
}

func applyPatches(src string, ps []patch) string {
	sort.SliceStable(ps, func(i, j int) bool { return ps[i].At < ps[j].At })
	var b strings.Builder
	last := 0
	for _, p := range ps {
		if p.At < last {
			continue // overlapping: keep the first
		}
		b.WriteString(src[last:p.At])
		b.WriteString(p.Ins)
		last = p.At + p.Del
	}
	b.WriteString(src[last:])
	return b.String()
}

// moduleTFFiles lists the root-relative .tf files directly in module dir
// mod: regular files, or (links) the symlinked ones.
func moduleTFFiles(root, mod string, links bool) []string {
	ents, err := os.ReadDir(filepath.Join(root, filepath.FromSlash(mod)))
	if err != nil {
		return nil
	}
	var out []string
	for _, e := range ents {
		if !strings.HasSuffix(e.Name(), ".tf") {
			continue
		}
		isLink := e.Type()&os.ModeSymlink != 0
		if isLink == links && (isLink || e.Type().IsRegular()) {
			out = append(out, path.Join(mod, e.Name()))
		}
	}
	slices.Sort(out)
	return out
}

// fileTransform rewrites one file; n counts the edits it made.
type fileTransform func(src string) (out string, n int)

// applyFileTransform applies f to every regular .tf file of mods under
// root, in order, and returns one restore and the edit count.
func applyFileTransform(t testing.TB, root string, mods []string, f fileTransform) (func(), int) {
	t.Helper()
	var rs []func()
	total := 0
	for _, mod := range mods {
		for _, rel := range moduleTFFiles(root, mod, false) {
			b, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel)))
			if err != nil {
				t.Fatal(err)
			}
			out, n := f(string(b))
			if n == 0 || out == string(b) {
				continue
			}
			rs = append(rs, applyEdit(t, root, edit{File: rel, New: []byte(out)}))
			total += n
		}
	}
	return func() {
		for i := len(rs) - 1; i >= 0; i-- {
			rs[i]()
		}
	}, total
}

// R1 whitespace: tabs in code become two spaces, the blanks before each
// variable/output attribute's = become two spaces, and a blank line follows
// each top-level closing brace.
func r1Whitespace(src string) (string, int) {
	kinds := modelKinds(src)
	mask := oracleMask(src)
	var ps []patch
	for i := 0; i < len(src); i++ {
		if src[i] == '\t' && kinds[i] == 'c' {
			ps = append(ps, patch{i, 1, "  "})
		}
	}
	blocks, ok := findBlocks(src)
	if !ok {
		return src, 0
	}
	for _, b := range blocks {
		for _, a := range b.Attrs {
			ns := a.Start
			for ns < len(src) && (src[ns] == ' ' || src[ns] == '\t' || src[ns] == '{') {
				ns++
			}
			ne := ns + len(modelIdentRe.FindString(src[ns:]))
			eq := strings.IndexByte(src[ne:a.ValueStart], '=')
			if eq < 0 {
				continue
			}
			if src[ne:ne+eq] != "  " && !strings.Contains(src[ne:ne+eq], "\t") {
				ps = append(ps, patch{ne, eq, "  "})
			}
		}
	}
	for i := 0; i < len(src); i++ {
		if src[i] == '}' && mask[i] == 1 && kinds[i] == 'c' {
			ls := strings.LastIndexByte(src[:i], '\n') + 1
			le := strings.IndexByte(src[i:], '\n')
			if le >= 0 && strings.TrimSpace(src[ls:i+le]) == "}" {
				ps = append(ps, patch{i + le + 1, 0, "\n"})
			}
		}
	}
	return applyPatches(src, ps), len(ps)
}

// R2 comments: a line comment before each variable/output block, a block
// comment after its opening brace, and a line comment at the end of each
// attribute line (not after a heredoc, not on a line holding a comment).
func r2Comments(src string) (string, int) {
	blocks, ok := findBlocks(src)
	if !ok {
		return src, 0
	}
	kinds := modelKinds(src)
	var ps []patch
	for _, b := range blocks {
		ps = append(ps, patch{b.Start, 0, "# gruntled-r2\n"})
		if b.OneLine {
			continue // a one-line block body takes no comment
		}
		ps = append(ps, patch{b.Open + 1, 0, " /* r2 */"})
		for _, a := range b.Attrs {
			if a.End == 0 || src[a.End-1] != '\n' {
				continue
			}
			ls := strings.LastIndexByte(src[:a.End-1], '\n') + 1
			hasHere := bytes.IndexByte(kinds[a.Start:a.End], 'h') >= 0
			hasCom := bytes.IndexByte(kinds[ls:a.End-1], 'm') >= 0
			if hasHere || hasCom {
				continue
			}
			at := a.End - 1
			if at > 0 && src[at-1] == '\r' {
				at-- // CRLF: the comment goes before the carriage return
			}
			ps = append(ps, patch{at, 0, " // r2"})
		}
	}
	return applyPatches(src, ps), len(ps)
}

// R3 attribute reorder: reverse the depth-1 attributes of each multi-line
// variable/output block; nested blocks and comments between items keep
// their place.
func r3Reorder(src string) (string, int) {
	blocks, ok := findBlocks(src)
	if !ok {
		return src, 0
	}
	var ps []patch
	n := 0
	for _, b := range blocks {
		if b.OneLine || len(b.Attrs) < 2 {
			continue
		}
		okEnds := true
		for _, a := range b.Attrs {
			if a.End == 0 || src[a.End-1] != '\n' || a.Start <= b.Open {
				okEnds = false
			}
		}
		if !okEnds {
			continue
		}
		for i, a := range b.Attrs {
			r := b.Attrs[len(b.Attrs)-1-i]
			ps = append(ps, patch{a.Start, a.End - a.Start, src[r.Start:r.End]})
		}
		n++
	}
	return applyPatches(src, ps), n
}

// R3wrong is the deliberately wrong reorder of the red case: it reverses
// the lines of each block body, heredoc lines included, which breaks any
// block holding a heredoc.
func r3Wrong(src string) (string, int) {
	blocks, ok := findBlocks(src)
	if !ok {
		return src, 0
	}
	var ps []patch
	for _, b := range blocks {
		if !b.Heredoc {
			continue
		}
		bodyStart := strings.IndexByte(src[b.Open:], '\n') + b.Open + 1
		bodyEnd := strings.LastIndexByte(src[:b.Close], '\n') + 1
		lines := strings.SplitAfter(src[bodyStart:bodyEnd], "\n")
		lines = slices.DeleteFunc(lines, func(s string) bool { return s == "" })
		slices.Reverse(lines)
		ps = append(ps, patch{bodyStart, bodyEnd - bodyStart, strings.Join(lines, "")})
	}
	return applyPatches(src, ps), len(ps)
}

// R7 deprecated: add `deprecated = "gruntled-r7"` to every variable and
// output block that has none.
func r7Deprecated(src string) (string, int) {
	blocks, ok := findBlocks(src)
	if !ok {
		return src, 0
	}
	n := 0
	for i := len(blocks) - 1; i >= 0; i-- {
		b := blocks[i]
		if _, has := b.attr("deprecated"); has {
			continue
		}
		src = string(opInsertAttr("", src, b, `deprecated = "gruntled-r7"`).New)
		n++
	}
	return src, n
}

// r6Move moves every variable and output block of the modules' regular .tf
// files into <module>/gruntled_moved.tf.
func r6Move(t testing.TB, root string, mods []string) (func(), int) {
	t.Helper()
	var es []edit
	n := 0
	for _, mod := range mods {
		var moved strings.Builder
		for _, rel := range moduleTFFiles(root, mod, false) {
			b, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel)))
			if err != nil {
				t.Fatal(err)
			}
			src := string(b)
			blocks, ok := findBlocks(src)
			if !ok || len(blocks) == 0 {
				continue
			}
			keep := src
			for i := len(blocks) - 1; i >= 0; i-- {
				bl := blocks[i]
				text := src[bl.Start:bl.End]
				if !strings.HasSuffix(text, "\n") {
					text += "\n"
				}
				moved.WriteString(text + "\n")
				keep = keep[:bl.Start] + keep[bl.End:]
				n++
			}
			es = append(es, edit{File: rel, New: []byte(keep)})
		}
		if moved.Len() > 0 {
			es = append(es, opNewFile(path.Join(mod, "gruntled_moved.tf"), moved.String()))
		}
	}
	return applyEdits(t, root, es), n
}

// r5ToJSON converts the simple blocks of the modules' regular .tf files to
// <module>/gruntled_r5.tf.json.
func r5ToJSON(t testing.TB, root string, mods []string) (func(), int, int) {
	t.Helper()
	var rs []func()
	conv, skip := 0, 0
	for _, mod := range mods {
		for _, rel := range moduleTFFiles(root, mod, false) {
			b, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel)))
			if err != nil {
				t.Fatal(err)
			}
			src := string(b)
			blocks, ok := findBlocks(src)
			if !ok || len(blocks) == 0 {
				continue
			}
			es, c, s := opToJSON(root, rel, src, blocks, path.Join(mod, "gruntled_r5.tf.json"))
			conv += c
			skip += s
			for _, e := range es {
				rs = append(rs, applyEdit(t, root, e))
			}
		}
	}
	return func() {
		for i := len(rs) - 1; i >= 0; i-- {
			rs[i]()
		}
	}, conv, skip
}

// r9Materialise replaces every symlinked .tf file of mods with a regular
// file holding the same bytes.
func r9Materialise(t testing.TB, root string, mods []string) (func(), int) {
	t.Helper()
	var es []edit
	for _, mod := range mods {
		for _, rel := range moduleTFFiles(root, mod, true) {
			if _, err := confined(root, rel, true); err == nil {
				es = append(es, opMaterialise(rel))
			}
		}
	}
	return applyEdits(t, root, es), len(es)
}

// r4Inject writes <module>/gruntled_r4.tf declaring gruntled_m17_opt with
// the given optional() attribute type.
func r4Inject(t testing.TB, root string, mods []string, attr string) (func(), int) {
	t.Helper()
	var es []edit
	for _, mod := range mods {
		es = append(es, opNewFile(path.Join(mod, "gruntled_r4.tf"),
			"variable \"gruntled_m17_opt\" {\n  type    = object({a = "+attr+"})\n  default = {}\n}\n"))
	}
	return applyEdits(t, root, es), len(es)
}

// assertBlastEmpty2 runs blast --base base cur (JSON) and requires exit 0
// and empty broken, impacted and changes.
func assertBlastEmptyJSON(r reporter, label, base, cur string) {
	var out, errb bytes.Buffer
	code := run([]string{"blast", "--base", base, "--format", "json", cur}, &out, &errb)
	doc, err := decodeBlastErr(out.String())
	if err != nil || code != 0 || errb.Len() > 0 || len(doc.Broken)+len(doc.Impacted)+len(doc.Changes) != 0 || doc.Broken == nil || doc.Impacted == nil || doc.Changes == nil {
		r.Errorf("%s: blast not empty (exit %d, err %v, stderr %q):\n%s", label, code, err, errb.String(), out.String())
	}
}

// reformatRun runs R1-R9 on base/cur copies of one tree over mods and
// returns the edit counter per transform.
func reformatRun(t *testing.T, base, cur string, mods []string) map[string]int {
	t.Helper()
	counts := map[string]int{}
	gBase := &digestGuard{t: t, work: base, pristine: corpusDigest(t, base), n: 1}
	gCur := &digestGuard{t: t, work: cur, pristine: corpusDigest(t, cur), n: 1}
	check := func(label, b, c string) {
		t.Helper()
		assertBlastEmptyJSON(t, label, b, c)
		assertSurfaceUnchanged(t, b, c)
	}
	one := func(name string, f fileTransform) {
		restore, n := applyFileTransform(t, cur, mods, f)
		counts[name] = n
		check(name, base, cur)
		restore()
		gCur.Tick()
	}
	one("R1", r1Whitespace)
	one("R2", r2Comments)
	one("R3", r3Reorder)

	// R4: two-sided optional() default, added and changed.
	for _, v := range []struct{ name, base, cur string }{
		{"R4", "optional(string)", `optional(string, "x")`},
		{"R4-changed", `optional(string, "y")`, `optional(string, "x")`},
	} {
		pristineBase := corpusCopy(t, base)
		rb, n := r4Inject(t, base, mods, v.base)
		// The injection landed: base' vs the pristine tree adds the variable
		// to every module.
		var out, errb bytes.Buffer
		run([]string{"blast", "--base", pristineBase, "--format", "json", base}, &out, &errb)
		doc, err := decodeBlastErr(out.String())
		landed := err == nil && len(doc.Changes) == len(mods)
		for _, c := range doc.Changes {
			landed = landed && slices.Equal(c.AddedVariables, []string{"gruntled_m17_opt"})
		}
		if !landed {
			t.Errorf("%s: base' vs pristine does not add gruntled_m17_opt to each of %d modules:\n%s", v.name, len(mods), out.String())
		}
		rc, _ := r4Inject(t, cur, mods, v.cur)
		counts[v.name] = n
		check(v.name, base, cur)
		rc()
		rb()
		gBase.Tick()
		gCur.Tick()
	}

	// R5 both directions.
	rc, conv, skip := r5ToJSON(t, cur, mods)
	counts["R5"] = conv
	counts["R5-skipped"] = skip
	check("R5", base, cur)
	rc()
	rb, _, _ := r5ToJSON(t, base, mods)
	check("R5-reverse", base, cur)
	rb()
	gBase.Tick()
	gCur.Tick()

	rc, n := r6Move(t, cur, mods)
	counts["R6"] = n
	check("R6", base, cur)
	rc()
	gCur.Tick()

	one("R7", r7Deprecated)

	// R8: R1∘R2∘R3∘R6∘R7 (R7 first).
	var rs []func()
	total := 0
	r, n := applyFileTransform(t, cur, mods, r7Deprecated)
	rs, total = append(rs, r), total+n
	r, n = r6Move(t, cur, mods)
	rs, total = append(rs, r), total+n
	for i, f := range []fileTransform{r3Reorder, r2Comments, r1Whitespace} {
		r, n = applyFileTransform(t, cur, mods, f)
		rs, total = append(rs, r), total+n
		if i == 0 {
			counts["R8-R3"] = n // R3 on blocks R7 made multi-attribute
		}
	}
	counts["R8"] = total
	check("R8", base, cur)
	for i := len(rs) - 1; i >= 0; i-- {
		rs[i]()
	}
	gCur.Tick()

	rc, n = r9Materialise(t, cur, mods)
	counts["R9"] = n
	check("R9", base, cur)
	rc()
	gBase.Final()
	gCur.Final()
	return counts
}

var reformatNames = []string{"R1", "R2", "R3", "R4", "R4-changed", "R5", "R6", "R7", "R8", "R9"}

// TestCorpusReformat runs R1-R9 on the self-test tree (always) and on each
// pinned corpus over its included modules (env-gated).
func TestCorpusReformat(t *testing.T) {
	t.Run("selftest", func(t *testing.T) {
		pristine := buildSelfTestTree(t)
		model := buildCorpusModel(t, pristine)
		// Red case: a reorder that crosses a heredoc boundary breaks the parse
		// and must trip the non-vacuity guard.
		cur := corpusCopy(t, pristine)
		restore, n := applyFileTransform(t, cur, model.Eligible, r3Wrong)
		if n == 0 {
			t.Fatal("wrong R3 found no heredoc block")
		}
		rec := &recorder{}
		assertSurfaceUnchanged(rec, pristine, cur)
		if len(rec.msgs) == 0 {
			t.Errorf("the guard did not trip on a reorder across a heredoc")
		}
		restore()
		counts := reformatRun(t, corpusCopy(t, pristine), corpusCopy(t, pristine), model.Eligible)
		for _, r := range reformatNames {
			if counts[r] == 0 {
				t.Errorf("%s: no edit on the self-test tree", r)
			}
		}
		t.Logf("selftest counts %v", counts)
	})
	for _, repo := range graphRepos {
		t.Run(repo.Name, func(t *testing.T) {
			root := repo.Require(t)
			before := corpusDigest(t, root)
			model := buildCorpusModel(t, root)
			mods, _, _ := model.universe(corpusExclusions[repo.Name])
			counts := reformatRun(t, repo.Copy(t, root), repo.Copy(t, root), mods)
			for _, r := range reformatNames {
				t.Logf("CORPUS-REFORMAT %s %s edits=%d", repo.Name, r, counts[r])
				switch {
				case counts[r] > 0:
				case r == "R9" && repo.Name != "iso20022":
					// Only iso20022 has symlinked module files.
				case r == "R3" && counts["R8-R3"] > 0:
					// No block has two attributes before R7 adds one; R3
					// runs inside R8 on the blocks R7 made multi-attribute.
				default:
					t.Errorf("%s: no edit on %s", r, repo.Name)
				}
			}
			t.Logf("CORPUS-REFORMAT %s R8-R3 edits=%d", repo.Name, counts["R8-R3"])
			t.Logf("CORPUS-REFORMAT %s R5 skipped_blocks=%d modules=%d", repo.Name, counts["R5-skipped"], len(mods))
			if after := corpusDigest(t, root); after != before {
				t.Errorf("checkout changed")
			}
		})
	}
}

var _ = fmt.Sprintf
