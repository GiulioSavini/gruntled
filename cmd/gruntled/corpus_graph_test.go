package main

// MORE-06: GRT002/GRT003 on the pinned three-repo corpus.
//
// graphOracle is a textual oracle that shares no code with gruntled's
// parser, loader or analyzers. It scans every terragrunt.hcl (and the files
// it includes through a path the oracle can resolve) with a small byte
// scanner and regexps, resolves literal config_path and dependencies paths
// values with path.Join against the unit directory, classifies targets with
// os.Stat, and finds cycles by mutual reachability (BFS per node), not by
// Tarjan. Every disagreement with gruntled on the unmutated corpora is
// listed by hand with its reason (denisGraphOracleOnly).

import (
	"bytes"
	"cmp"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"
)

// graphFinding is one GRT002/GRT003 diagnostic, observed or expected.
type graphFinding struct {
	Code, File string
	Line, Col  int
	Unit, Msg  string
}

func (f graphFinding) pos() string {
	return f.File + ":" + strconv.Itoa(f.Line) + ":" + strconv.Itoa(f.Col)
}

func (f graphFinding) String() string {
	return fmt.Sprintf("%s %s (unit %s) %s", f.Code, f.pos(), f.Unit, f.Msg)
}

func graphSort(fs []graphFinding) {
	slices.SortFunc(fs, func(a, b graphFinding) int {
		return cmp.Or(
			cmp.Compare(a.File, b.File),
			cmp.Compare(a.Line, b.Line),
			cmp.Compare(a.Col, b.Col),
			cmp.Compare(a.Unit, b.Unit),
			cmp.Compare(a.Code, b.Code),
			cmp.Compare(a.Msg, b.Msg),
		)
	})
}

// graphDiff returns the multiset differences want-got (missing) and
// got-want (extra).
func graphDiff(want, got []graphFinding) (missing, extra []graphFinding) {
	count := map[graphFinding]int{}
	for _, g := range got {
		count[g]++
	}
	for _, w := range want {
		if count[w] > 0 {
			count[w]--
			continue
		}
		missing = append(missing, w)
	}
	for _, g := range got {
		if count[g] > 0 {
			count[g]--
			extra = append(extra, g)
		}
	}
	return missing, extra
}

// graphGruntled splits a gruntled report into its GRT002/GRT003 findings
// and the projection of every other diagnostic.
func graphGruntled(rep corpusReport) (graph []graphFinding, other []corpusDiag) {
	for _, d := range rep.Diagnostics {
		switch d.Code {
		case "GRT002", "GRT003":
			graph = append(graph, graphFinding{d.Code, d.File, d.Line, d.Column, d.Unit, d.Message})
		default:
			other = append(other, corpusDiag{d.Code, d.Severity, d.File, d.Line, d.Column, d.Unit})
		}
	}
	graphSort(graph)
	corpusSortDiags(other)
	return graph, other
}

// ---------------------------------------------------------------------------
// Textual oracle.

// oracleMask returns, for every byte of src, the top-level brace depth
// before that byte when the byte is code, or -1 when it is inside a string
// (including its ${...} templates), a comment or a heredoc.
func oracleMask(src string) []int {
	mask := make([]int, len(src))
	depth := 0
	type frame struct {
		str    bool
		braces int
	}
	var stack []frame
	setNeg := func(from, to int) {
		for k := from; k < to && k < len(src); k++ {
			mask[k] = -1
		}
	}
	for i := 0; i < len(src); {
		c := src[i]
		var next byte
		if i+1 < len(src) {
			next = src[i+1]
		}
		if len(stack) == 0 {
			mask[i] = depth
			switch {
			case c == '#' || (c == '/' && next == '/'):
				j := strings.IndexByte(src[i:], '\n')
				if j < 0 {
					j = len(src) - i
				}
				setNeg(i, i+j)
				i += j
				continue
			case c == '/' && next == '*':
				end := len(src)
				if j := strings.Index(src[i+2:], "*/"); j >= 0 {
					end = i + 2 + j + 2
				}
				setNeg(i, end)
				i = end
				continue
			case c == '<' && next == '<':
				if m := oracleHeredocRe.FindStringSubmatch(src[i:]); m != nil {
					end := len(src)
					off := i + len(m[0])
					for off < len(src) {
						nl := strings.IndexByte(src[off:], '\n')
						lineEnd := len(src)
						if nl >= 0 {
							lineEnd = off + nl
						}
						if strings.TrimSpace(src[off:lineEnd]) == m[1] {
							end = lineEnd
							break
						}
						off = lineEnd + 1
					}
					setNeg(i, end)
					i = end
					continue
				}
			case c == '"':
				mask[i] = -1
				stack = append(stack, frame{str: true})
			case c == '{':
				depth++
			case c == '}':
				depth--
			}
			i++
			continue
		}
		mask[i] = -1
		top := &stack[len(stack)-1]
		if top.str {
			switch {
			case c == '\\':
				setNeg(i, i+2)
				i += 2
				continue
			case c == '"':
				stack = stack[:len(stack)-1]
			case (c == '$' || c == '%') && next == '{':
				setNeg(i, i+2)
				stack = append(stack, frame{})
				i += 2
				continue
			}
			i++
			continue
		}
		switch c {
		case '"':
			stack = append(stack, frame{str: true})
		case '{':
			top.braces++
		case '}':
			if top.braces == 0 {
				stack = stack[:len(stack)-1]
			} else {
				top.braces--
			}
		}
		i++
	}
	return mask
}

var (
	oracleHeredocRe = regexp.MustCompile(`^<<-?([A-Za-z_][A-Za-z0-9_]*)[ \t]*\r?\n`)
	oracleBlockRe   = regexp.MustCompile(`(?m)^[ \t]*(?:dependency[ \t]+"([^"]*)"|(dependencies)|(include)(?:[ \t]+"[^"]*")?)[ \t]*\{`)
	oracleFindRe    = regexp.MustCompile(`^find_in_parent_folders\(\s*(?:"([^"$%\\]*)")?\s*\)`)
)

type oracleBlock struct {
	Kind, Label string // Kind: dependency, dependencies, include
	Open, Close int    // byte offsets of the braces
}

// oracleBlocks finds the top-level dependency, dependencies and include
// blocks of src.
func oracleBlocks(src string, mask []int) []oracleBlock {
	var out []oracleBlock
	for _, m := range oracleBlockRe.FindAllStringSubmatchIndex(src, -1) {
		start := m[0]
		for src[start] == ' ' || src[start] == '\t' {
			start++
		}
		if mask[start] != 0 {
			continue
		}
		b := oracleBlock{Open: m[1] - 1, Close: -1}
		switch {
		case m[2] >= 0:
			b.Kind, b.Label = "dependency", src[m[2]:m[3]]
		case m[4] >= 0:
			b.Kind = "dependencies"
		default:
			b.Kind = "include"
		}
		for j := b.Open + 1; j < len(src); j++ {
			if src[j] == '}' && mask[j] == 1 {
				b.Close = j
				break
			}
		}
		if b.Close > 0 {
			out = append(out, b)
		}
	}
	return out
}

// oracleAttr returns the offset of the value of the first attribute name
// directly inside b.
func oracleAttr(src string, mask []int, b oracleBlock, name string) (int, bool) {
	re := regexp.MustCompile(`\b` + regexp.QuoteMeta(name) + `[ \t]*=[ \t]*`)
	body := src[b.Open+1 : b.Close]
	for _, m := range re.FindAllStringIndex(body, -1) {
		p := b.Open + 1 + m[0]
		v := b.Open + 1 + m[1]
		if mask[p] != 1 || (v < len(src) && src[v] == '=') {
			continue
		}
		if p > 0 && !strings.ContainsRune(" \t\n{;", rune(src[p-1])) {
			continue
		}
		return v, true
	}
	return 0, false
}

// oracleString reads the string literal starting at p (src[p] == '"') and
// returns its raw content and the offset just after the closing quote.
func oracleString(src string, mask []int, p int) (raw string, end int, ok bool) {
	if p >= len(src) || src[p] != '"' {
		return "", 0, false
	}
	q := p + 1
	for q < len(src) && mask[q] == -1 {
		q++
	}
	if q-1 <= p || src[q-1] != '"' {
		return "", 0, false
	}
	raw = src[p+1 : q-1]
	if strings.ContainsAny(raw, "\n\\") {
		return "", 0, false
	}
	return raw, q, true
}

// oracleRestIs reports whether the text after end, up to the newline,
// starts (after blanks) with one of the allowed tokens or is empty.
func oracleRestIs(src string, end int, allowed ...string) bool {
	rest := src[end:]
	if nl := strings.IndexByte(rest, '\n'); nl >= 0 {
		rest = rest[:nl]
	}
	rest = strings.TrimLeft(rest, " \t\r")
	if rest == "" {
		return true
	}
	for _, a := range allowed {
		if strings.HasPrefix(rest, a) {
			return true
		}
	}
	return false
}

// oracleResolve turns a literal value into a repo-relative target, or
// false for anything the oracle does not model (interpolation other than a
// leading ${get_terragrunt_dir()}, absolute paths, "", outside the repo,
// a non-default file).
func oracleResolve(raw, unitDir string) (string, bool) {
	s := raw
	if rest, ok := strings.CutPrefix(s, "${get_terragrunt_dir()}"); ok {
		if rest == "" {
			return unitDir, true
		}
		s = "." + rest
	}
	if s == "" || strings.Contains(s, "${") || strings.Contains(s, "%{") || path.IsAbs(s) {
		return "", false
	}
	target := path.Join(unitDir, s)
	if target == ".." || strings.HasPrefix(target, "../") {
		return "", false
	}
	if strings.HasSuffix(target, ".hcl") || strings.HasSuffix(target, ".json") {
		return "", false
	}
	return target, true
}

func oracleLineCol(src string, off int) (int, int) {
	line := strings.Count(src[:off], "\n") + 1
	col := off - (strings.LastIndexByte(src[:off], '\n') + 1) + 1
	return line, col
}

type oracleTargetKind int

const (
	oracleTargetSkip oracleTargetKind = iota
	oracleTargetUnit
	oracleTargetMissing
	oracleTargetNoConfig
)

func oracleClassify(root, target string) oracleTargetKind {
	full := filepath.Join(root, filepath.FromSlash(target))
	fi, err := os.Stat(full)
	if errors.Is(err, fs.ErrNotExist) {
		return oracleTargetMissing
	}
	if err != nil || !fi.IsDir() {
		return oracleTargetSkip
	}
	for _, n := range []string{"terragrunt.hcl", "terragrunt.hcl.json"} {
		if _, err := os.Stat(filepath.Join(full, n)); err == nil {
			return oracleTargetUnit
		}
	}
	return oracleTargetNoConfig
}

// oracleEdge is one dependency declaration of a unit, in declaration order.
type oracleEdge struct {
	File      string // repo-relative file holding the value
	Line, Col int
	Kind      string // "block" or "paths"
	Label     string // block label, or the raw paths entry
	Target    string
}

// oracleSource is one file contributing declarations to a unit.
type oracleSource struct {
	rel, src string
	mask     []int
}

// oracleIncludes resolves the include blocks of a unit file to the files
// they name, when the oracle can: a literal (optionally
// ${get_terragrunt_dir()}-prefixed) path, or find_in_parent_folders("x"),
// or find_in_parent_folders() meaning "terragrunt.hcl". Includes with
// merge_strategy = "no_merge" contribute nothing. notes counts the rest.
func oracleIncludes(root, unitDir string, s oracleSource, notes map[string]int) []string {
	var out []string
	for _, b := range oracleBlocks(s.src, s.mask) {
		if b.Kind != "include" {
			continue
		}
		if v, ok := oracleAttr(s.src, s.mask, b, "merge_strategy"); ok && strings.HasPrefix(s.src[v:], `"no_merge"`) {
			continue
		}
		v, ok := oracleAttr(s.src, s.mask, b, "path")
		if !ok {
			notes["include without path"]++
			continue
		}
		var file string
		if raw, _, ok := oracleString(s.src, s.mask, v); ok {
			p := raw
			if rest, cut := strings.CutPrefix(p, "${get_terragrunt_dir()}"); cut {
				p = "." + rest
			}
			if strings.Contains(p, "${") || path.IsAbs(p) {
				notes["include path not modelled"]++
				continue
			}
			file = path.Join(unitDir, p)
		} else if m := oracleFindRe.FindStringSubmatch(s.src[v:]); m != nil {
			name := m[1]
			if name == "" {
				name = "terragrunt.hcl"
			}
			for dir := path.Dir(unitDir); ; dir = path.Dir(dir) {
				if unitDir == "." {
					break
				}
				cand := path.Join(dir, name)
				if fi, err := os.Stat(filepath.Join(root, filepath.FromSlash(cand))); err == nil && !fi.IsDir() {
					file = cand
					break
				}
				if dir == "." {
					break
				}
			}
			if file == "" {
				notes["find_in_parent_folders finds nothing"]++
				continue
			}
		} else {
			notes["include path not modelled"]++
			continue
		}
		if file == ".." || strings.HasPrefix(file, "../") {
			notes["include outside repository"]++
			continue
		}
		out = append(out, file)
	}
	return out
}

// oracleUnitEdges returns the declarations of the unit in unitDir, in the
// order: unit-file blocks, included-file blocks (labels the unit file
// already declares are overridden), unit-file paths, included paths.
// Disabled blocks and values the oracle cannot resolve are dropped.
func oracleUnitEdges(root, unitDir string, notes map[string]int) []oracleEdge {
	rel := path.Join(unitDir, "terragrunt.hcl")
	b, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel)))
	if err != nil {
		notes["unit file unreadable"]++
		return nil
	}
	unit := oracleSource{rel: rel, src: string(b)}
	unit.mask = oracleMask(unit.src)
	sources := []oracleSource{unit}
	for _, inc := range oracleIncludes(root, unitDir, unit, notes) {
		ib, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(inc)))
		if err != nil {
			notes["included file unreadable"]++
			continue
		}
		s := oracleSource{rel: inc, src: string(ib)}
		s.mask = oracleMask(s.src)
		sources = append(sources, s)
	}

	var blocks, paths []oracleEdge
	seenLabel := map[string]bool{}
	for si, s := range sources {
		for _, blk := range oracleBlocks(s.src, s.mask) {
			switch blk.Kind {
			case "dependency":
				if si > 0 && seenLabel[blk.Label] {
					continue
				}
				if si == 0 {
					seenLabel[blk.Label] = true
				}
				if v, ok := oracleAttr(s.src, s.mask, blk, "enabled"); ok {
					val := s.src[v:]
					if i := strings.IndexAny(val, "\n}#"); i >= 0 {
						val = val[:i]
					}
					if strings.TrimSpace(val) != "true" {
						notes["disabled block"]++
						continue
					}
				}
				v, ok := oracleAttr(s.src, s.mask, blk, "config_path")
				if !ok {
					notes["block without config_path"]++
					continue
				}
				raw, end, ok := oracleString(s.src, s.mask, v)
				if !ok || !oracleRestIs(s.src, end, "#", "//", "}") {
					notes["config_path not a literal"]++
					continue
				}
				target, ok := oracleResolve(raw, unitDir)
				if !ok {
					notes["config_path not resolvable"]++
					continue
				}
				line, col := oracleLineCol(s.src, v)
				blocks = append(blocks, oracleEdge{s.rel, line, col, "block", blk.Label, target})
			case "dependencies":
				v, ok := oracleAttr(s.src, s.mask, blk, "paths")
				if !ok || s.src[v] != '[' {
					notes["paths not a list"]++
					continue
				}
				depth := 0
				for k := v; k < blk.Close; k++ {
					if s.mask[k] < 0 {
						if s.src[k] == '"' && s.mask[k-1] >= 0 && depth == 1 {
							raw, end, ok := oracleString(s.src, s.mask, k)
							if !ok || !oracleRestIs(s.src, end, ",", "]", "#", "//") {
								notes["paths entry not a literal"]++
								continue
							}
							target, ok := oracleResolve(raw, unitDir)
							if !ok {
								notes["paths entry not resolvable"]++
								continue
							}
							line, col := oracleLineCol(s.src, k)
							paths = append(paths, oracleEdge{s.rel, line, col, "paths", raw, target})
						}
						continue
					}
					switch s.src[k] {
					case '[':
						depth++
					case ']':
						depth--
					}
					if depth == 0 {
						break
					}
				}
			}
		}
	}
	return append(blocks, paths...)
}

// oracleResult is what the textual oracle derives for one tree.
type oracleResult struct {
	Findings []graphFinding // GRT002 and GRT003, sorted
	Pairs    []string       // "unit target" per edge, self-loops included
	Notes    map[string]int // shapes the oracle left out, by kind
}

// graphOracle computes the GRT002/GRT003 set of the tree at root.
func graphOracle(t testing.TB, root string) oracleResult {
	t.Helper()
	res := oracleResult{Notes: map[string]int{}}
	var units []string
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			switch d.Name() {
			case ".git", ".terraform", ".terragrunt-cache":
				return filepath.SkipDir
			}
			return nil
		}
		switch d.Name() {
		case "terragrunt.hcl":
			rel, err := filepath.Rel(root, filepath.Dir(p))
			if err != nil {
				return err
			}
			units = append(units, filepath.ToSlash(rel))
		case "terragrunt.hcl.json":
			res.Notes["json unit not scanned"]++
		}
		return nil
	})
	if err != nil {
		t.Fatalf("oracle walk: %v", err)
	}
	slices.Sort(units)

	adj := map[string][]string{}
	firstEdge := map[[2]string]oracleEdge{}
	for _, u := range units {
		for _, e := range oracleUnitEdges(root, u, res.Notes) {
			switch oracleClassify(root, e.Target) {
			case oracleTargetMissing, oracleTargetNoConfig:
				problem := "directory does not exist"
				if oracleClassify(root, e.Target) == oracleTargetNoConfig {
					problem = "directory has no terragrunt.hcl"
				}
				msg := "dependency " + strconv.Quote(e.Label) + " config_path resolves to " + strconv.Quote(e.Target) + ": " + problem
				if e.Kind == "paths" {
					msg = "dependencies path " + strconv.Quote(e.Label) + " resolves to " + strconv.Quote(e.Target) + ": " + problem
				}
				res.Findings = append(res.Findings, graphFinding{"GRT002", e.File, e.Line, e.Col, u, msg})
			case oracleTargetUnit:
				k := [2]string{u, e.Target}
				if _, ok := firstEdge[k]; !ok {
					firstEdge[k] = e
					adj[u] = append(adj[u], e.Target)
					res.Pairs = append(res.Pairs, u+" "+e.Target)
				}
			default:
				res.Notes["target not a directory"]++
			}
		}
	}

	// Cycles by mutual reachability.
	reach := func(from string) map[string]bool {
		seen := map[string]bool{}
		queue := slices.Clone(adj[from])
		for len(queue) > 0 {
			n := queue[0]
			queue = queue[1:]
			if seen[n] {
				continue
			}
			seen[n] = true
			queue = append(queue, adj[n]...)
		}
		return seen
	}
	reachOf := map[string]map[string]bool{}
	nodes := make([]string, 0, len(adj))
	for n := range adj {
		nodes = append(nodes, n)
	}
	slices.Sort(nodes)
	for _, n := range nodes {
		reachOf[n] = reach(n)
	}
	done := map[string]bool{}
	for _, n := range nodes {
		if done[n] || !reachOf[n][n] {
			continue
		}
		var scc []string
		for _, m := range nodes {
			if m == n || (reachOf[n][m] && reachOf[m][n]) {
				scc = append(scc, m)
				done[m] = true
			}
		}
		slices.Sort(scc)
		in := map[string]bool{}
		for _, m := range scc {
			in[m] = true
		}
		ring := true
		succ := map[string]string{}
		for _, m := range scc {
			var s []string
			for _, x := range adj[m] {
				if in[x] {
					s = append(s, x)
				}
			}
			if len(s) != 1 {
				ring = false
			} else {
				succ[m] = s[0]
			}
		}
		if len(scc) > 1 && slices.ContainsFunc(scc, func(m string) bool { return slices.Contains(adj[m], m) }) {
			ring = false
		}
		var msg string
		if ring {
			parts := []string{strconv.Quote(scc[0])}
			for cur := succ[scc[0]]; ; cur = succ[cur] {
				parts = append(parts, strconv.Quote(cur))
				if cur == scc[0] {
					break
				}
			}
			msg = "dependency cycle: " + strings.Join(parts, " -> ")
		} else {
			q := make([]string, len(scc))
			for i, m := range scc {
				q[i] = strconv.Quote(m)
			}
			msg = "dependency cycle among: " + strings.Join(q, ", ")
		}
		// Position: the smallest member's first edge into the cycle.
		var first oracleEdge
		for _, e := range oracleUnitEdges(root, scc[0], map[string]int{}) {
			if in[e.Target] && oracleClassify(root, e.Target) == oracleTargetUnit {
				first = e
				break
			}
		}
		res.Findings = append(res.Findings, graphFinding{"GRT003", first.File, first.Line, first.Col, scc[0], msg})
	}
	graphSort(res.Findings)
	slices.Sort(res.Pairs)
	return res
}

// TestDenisExpectedGraphShape checks the hand-derived denis256 GRT002/GRT003
// table without the corpus.
func TestDenisExpectedGraphShape(t *testing.T) {
	if len(denisExpectedGraph) == 0 {
		t.Fatal("denisExpectedGraph is empty")
	}
	seen := map[string]bool{}
	files := map[string]bool{}
	for _, e := range denisExpectedGraph {
		k := e.Code + " " + e.pos()
		if seen[k] {
			t.Errorf("duplicate entry %s", k)
		}
		seen[k] = true
		files[e.File] = true
		if e.Code != "GRT002" && e.Code != "GRT003" {
			t.Errorf("%s: code %q, want GRT002 or GRT003", e.pos(), e.Code)
		}
		if e.Line <= 0 || e.Col <= 0 {
			t.Errorf("%s: line and column must be positive", e.pos())
		}
		if e.Msg == "" || e.Why == "" || e.Unit == "" {
			t.Errorf("%s: empty message, unit or reason", e.pos())
		}
	}
	for _, f := range []string{
		"hcl/terragrunt.hcl",
		"module-output-broken/m1/terragrunt.hcl",
		"tf-lint-regeneration/dev/template/terragrunt.hcl",
		"5728-broken-includes/test.hcl",
	} {
		if !files[f] {
			t.Errorf("denisExpectedGraph has no entry in %s", f)
		}
	}
	for _, e := range denisGraphOracleOnly {
		if e.Why == "" || seen[e.Code+" "+e.pos()] {
			t.Errorf("oracle-only %s: needs a reason and must not be expected from gruntled", e.pos())
		}
	}
}

// ---------------------------------------------------------------------------
// Env-gated corpus tests.

// graphTSort cross-checks the oracle's cycle verdict with coreutils tsort
// on the same pairs (self-loops dropped: tsort treats "a a" as a node).
func graphTSort(t testing.TB, pairs []string, wantLoop bool) {
	t.Helper()
	bin, err := exec.LookPath("tsort")
	if err != nil {
		t.Logf("tsort not on PATH; cross-check skipped")
		return
	}
	var in []string
	for _, p := range pairs {
		a, b, ok := strings.Cut(p, " ")
		if !ok || strings.Contains(b, " ") {
			t.Fatalf("tsort cannot take pair %q", p)
		}
		if a != b {
			in = append(in, p)
		}
	}
	cmd := exec.Command(bin)
	cmd.Stdin = strings.NewReader(strings.Join(in, "\n") + "\n")
	out, err := cmd.CombinedOutput()
	loop := err != nil && strings.Contains(string(out), "input contains a loop")
	if err != nil && !loop {
		t.Fatalf("tsort: %v\n%s", err, out)
	}
	if loop != wantLoop {
		t.Errorf("tsort loop=%v, want %v (%d pairs)", loop, wantLoop, len(in))
	}
}

// graphTGRun runs the pinned terragrunt oracle
//
//	terragrunt run --all --non-interactive --no-auto-init --tf-path <tofu> -- version
//
// in dir (always a scratch copy: it may write .terragrunt-cache) and
// returns its normalized output with root replaced by <ROOT>.
// --no-auto-init keeps it from asking for backend input; the graph checks
// (missing unit, cycle) happen during queue construction, before any unit
// runs. The exit code is not the signal: on the clean corpora units fail
// later because dependency outputs need state.
func graphTGRun(t testing.TB, bin, root, dir string) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, bin, "run", "--all", "--non-interactive", "--no-auto-init", "--no-color",
		"--tf-path", filepath.Join(filepath.Dir(bin), "tofu"), "--", "version")
	cmd.Env = tgEnv(t, bin)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	var ee *exec.ExitError
	if err != nil && !errors.As(err, &ee) {
		t.Fatalf("terragrunt: %v", err)
	}
	return strings.Join(tgNormalize(string(out), root), "\n")
}

// graphTGMarkers are the queue-construction errors of terragrunt v1.1.6.
var graphTGMarkers = []string{
	"does not contain a terragrunt.hcl file",
	"cycle detected during queue construction",
}

func graphTGClean(t testing.TB, bin, root, dir string) {
	t.Helper()
	out := graphTGRun(t, bin, root, dir)
	for _, m := range graphTGMarkers {
		if strings.Contains(out, m) {
			t.Errorf("terragrunt reports %q on the unmutated tree:\n%s", m, out)
		}
	}
	t.Logf("terragrunt: clean queue construction, %d units printed a tofu version", strings.Count(out, "OpenTofu v"))
}

// graphIsELF reports whether p starts with the ELF magic.
func graphIsELF(p string) bool {
	f, err := os.Open(p)
	if err != nil {
		return false
	}
	defer f.Close()
	var b [4]byte
	n, _ := f.Read(b[:])
	return n == 4 && string(b[:]) == "\x7fELF"
}

// graphRepo is one pinned corpus.
type graphRepo struct {
	Name    string
	Require func(testing.TB) string
	// Copy makes the scratch copy. denis256 checks in 29 ELF terragrunt
	// binaries (1.85 GB) that gruntled never reads and a tmpfs cannot hold;
	// its copy leaves them out, and every test proves fidelity by requiring
	// gruntled's JSON on the copy to equal, byte for byte, the JSON on the
	// checkout.
	Copy func(testing.TB, string) string
}

var graphRepos = []graphRepo{
	{"iso20022", corpusRequire, corpusCopy},
	{"secret", secretRequire, corpusCopy},
	{"denis256", denisRequire, func(t testing.TB, src string) string { return corpusCopyFiltered(t, src, graphIsELF) }},
}

// graphMutation is one injected graph error: applied alone to a fresh copy,
// it must add exactly Want to gruntled's output and to the oracle's.
type graphMutation struct {
	Repo      string
	M         corpusMutation // only Name, File, Old, New are used
	Want      graphFinding
	Synthetic bool   // adds a block or entry instead of editing a value
	TSortLoop bool   // a multi-unit cycle tsort must see
	TGDir     string // where the pinned terragrunt runs, relative to the copy
	TGWant    string // what it must print (<ROOT> = the copy)
}

const graphTGCycle = "cycle detected during queue construction"

func graphTGMissing(target string) string {
	return `Path: "<ROOT>/` + target + `/terragrunt.hcl"`
}

var graphMutations = []graphMutation{
	// iso20022: iac.src/ecr_health depends on iac.src/s3_runtime and is
	// depended on by iac.src/lambda_health.
	{
		Repo: "iso20022",
		M:    corpusMutation{Name: "config_path_missing_dir", File: "iac.src/ecr_health/terragrunt.hcl", Old: `config_path  = "../s3_runtime"`, New: `config_path  = "../s3_runtime_gone"`},
		Want: graphFinding{"GRT002", "iac.src/ecr_health/terragrunt.hcl", 2, 18, "iac.src/ecr_health",
			`dependency "s3" config_path resolves to "iac.src/s3_runtime_gone": directory does not exist`},
		TGDir: ".", TGWant: graphTGMissing("iac.src/s3_runtime_gone"),
	},
	{
		Repo: "iso20022",
		M:    corpusMutation{Name: "paths_missing_dir", File: "iac.src/ecr_health/terragrunt.hcl", Old: "inputs = {\n", New: "dependencies {\n  paths = [\"../gone_unit\"]\n}\n\ninputs = {\n"},
		Want: graphFinding{"GRT002", "iac.src/ecr_health/terragrunt.hcl", 13, 12, "iac.src/ecr_health",
			`dependencies path "../gone_unit" resolves to "iac.src/gone_unit": directory does not exist`},
		Synthetic: true, TGDir: ".", TGWant: graphTGMissing("iac.src/gone_unit"),
	},
	{
		Repo: "iso20022",
		M:    corpusMutation{Name: "back_edge_cycle", File: "iac.src/ecr_health/terragrunt.hcl", Old: "inputs = {\n", New: "dependency \"back\" {\n  config_path = \"../lambda_health\"\n}\n\ninputs = {\n"},
		Want: graphFinding{"GRT003", "iac.src/ecr_health/terragrunt.hcl", 13, 17, "iac.src/ecr_health",
			`dependency cycle: "iac.src/ecr_health" -> "iac.src/lambda_health" -> "iac.src/ecr_health"`},
		Synthetic: true, TSortLoop: true, TGDir: ".", TGWant: graphTGCycle,
	},
	{
		Repo: "iso20022",
		M:    corpusMutation{Name: "self_loop", File: "iac.src/ecr_health/terragrunt.hcl", Old: "inputs = {\n", New: "dependency \"self\" {\n  config_path = \"../ecr_health\"\n}\n\ninputs = {\n"},
		Want: graphFinding{"GRT003", "iac.src/ecr_health/terragrunt.hcl", 13, 17, "iac.src/ecr_health",
			`dependency cycle: "iac.src/ecr_health" -> "iac.src/ecr_health"`},
		Synthetic: true, TGDir: ".", TGWant: graphTGCycle,
	},
	{
		Repo: "iso20022",
		M:    corpusMutation{Name: "config_path_module_only_dir", File: "iac.src/ecr_health/terragrunt.hcl", Old: `config_path  = "../s3_runtime"`, New: `config_path  = "../s3_crr"`},
		Want: graphFinding{"GRT002", "iac.src/ecr_health/terragrunt.hcl", 2, 18, "iac.src/ecr_health",
			`dependency "s3" config_path resolves to "iac.src/s3_crr": directory has no terragrunt.hcl`},
		TGDir: ".", TGWant: graphTGMissing("iac.src/s3_crr"),
	},

	// secret: real edges ecr -> acm, lambda -> acm, lambda -> ecr.
	{
		Repo: "secret",
		M:    corpusMutation{Name: "config_path_missing_dir", File: "terragrunt/ecr/terragrunt.hcl", Old: `config_path = "../acm"`, New: `config_path = "../acm_gone"`},
		Want: graphFinding{"GRT002", "terragrunt/ecr/terragrunt.hcl", 12, 17, "terragrunt/ecr",
			`dependency "acm" config_path resolves to "terragrunt/acm_gone": directory does not exist`},
		TGDir: ".", TGWant: graphTGMissing("terragrunt/acm_gone"),
	},
	{
		Repo: "secret",
		M:    corpusMutation{Name: "paths_missing_dir", File: "terragrunt/ecr/terragrunt.hcl", Old: `paths = ["../acm"]`, New: `paths = ["../acm", "../gone"]`},
		Want: graphFinding{"GRT002", "terragrunt/ecr/terragrunt.hcl", 8, 22, "terragrunt/ecr",
			`dependencies path "../gone" resolves to "terragrunt/gone": directory does not exist`},
		Synthetic: true, TGDir: ".", TGWant: graphTGMissing("terragrunt/gone"),
	},
	{
		Repo: "secret",
		M:    corpusMutation{Name: "back_edge_cycle", File: "terragrunt/acm/terragrunt.hcl", Old: "terraform {\n  source = \"../../aws//acm\"\n}\n", New: "terraform {\n  source = \"../../aws//acm\"\n}\n\ndependency \"back\" {\n  config_path = \"../ecr\"\n}\n"},
		Want: graphFinding{"GRT003", "terragrunt/acm/terragrunt.hcl", 8, 17, "terragrunt/acm",
			`dependency cycle: "terragrunt/acm" -> "terragrunt/ecr" -> "terragrunt/acm"`},
		Synthetic: true, TSortLoop: true, TGDir: ".", TGWant: graphTGCycle,
	},
	{
		Repo: "secret",
		M:    corpusMutation{Name: "self_loop", File: "terragrunt/ecr/terragrunt.hcl", Old: "terraform {\n  source = \"../../aws//ecr\"\n}\n", New: "terraform {\n  source = \"../../aws//ecr\"\n}\n\ndependency \"self\" {\n  config_path = \"../ecr\"\n}\n"},
		Want: graphFinding{"GRT003", "terragrunt/ecr/terragrunt.hcl", 8, 17, "terragrunt/ecr",
			`dependency cycle: "terragrunt/ecr" -> "terragrunt/ecr"`},
		Synthetic: true, TGDir: ".", TGWant: graphTGCycle,
	},

	// denis256: issue-2565 is the chain C -> B -> A of resolved units with
	// no other diagnostic. Terragrunt cannot build the queue for the whole
	// repository (stack and function errors unrelated to the graph), so the
	// pinned binary runs on the issue-2565 subtree; the textual oracle runs
	// on the whole copy.
	{
		Repo: "denis256",
		M:    corpusMutation{Name: "config_path_missing_dir", File: "issue-2565/B/terragrunt.hcl", Old: `config_path = "../A"`, New: `config_path = "../A_gone"`},
		Want: graphFinding{"GRT002", "issue-2565/B/terragrunt.hcl", 5, 17, "issue-2565/B",
			`dependency "A" config_path resolves to "issue-2565/A_gone": directory does not exist`},
		TGDir: "issue-2565", TGWant: graphTGMissing("issue-2565/A_gone"),
	},
	{
		Repo: "denis256",
		M:    corpusMutation{Name: "paths_missing_dir", File: "issue-2565/B/terragrunt.hcl", Old: "dependency \"A\" {\n", New: "dependencies {\n  paths = [\"../gone\"]\n}\ndependency \"A\" {\n"},
		Want: graphFinding{"GRT002", "issue-2565/B/terragrunt.hcl", 5, 12, "issue-2565/B",
			`dependencies path "../gone" resolves to "issue-2565/gone": directory does not exist`},
		Synthetic: true, TGDir: "issue-2565", TGWant: graphTGMissing("issue-2565/gone"),
	},
	{
		Repo: "denis256",
		M:    corpusMutation{Name: "back_edge_cycle", File: "issue-2565/A/terragrunt.hcl", Old: "terraform {\n  source = \"./\"\n}", New: "terraform {\n  source = \"./\"\n}\ndependency \"C\" {\n  config_path = \"../C\"\n}\n"},
		Want: graphFinding{"GRT003", "issue-2565/A/terragrunt.hcl", 5, 17, "issue-2565/A",
			`dependency cycle: "issue-2565/A" -> "issue-2565/C" -> "issue-2565/B" -> "issue-2565/A"`},
		Synthetic: true, TSortLoop: true, TGDir: "issue-2565", TGWant: graphTGCycle,
	},
	{
		Repo: "denis256",
		M:    corpusMutation{Name: "self_loop", File: "issue-2565/B/terragrunt.hcl", Old: `config_path = "../A"`, New: `config_path = "../B"`},
		Want: graphFinding{"GRT003", "issue-2565/B/terragrunt.hcl", 5, 17, "issue-2565/B",
			`dependency cycle: "issue-2565/B" -> "issue-2565/B"`},
		TGDir: "issue-2565", TGWant: graphTGCycle,
	},
}

// graphOracleCheck requires the oracle's findings on root to equal want
// exactly, and tsort to agree there is no multi-unit cycle.
func graphOracleCheck(t testing.TB, root string, want []graphFinding) {
	t.Helper()
	res := graphOracle(t, root)
	want = slices.Clone(want)
	graphSort(want)
	missing, extra := graphDiff(want, res.Findings)
	for _, m := range missing {
		t.Errorf("oracle MISS %s", m)
	}
	for _, e := range extra {
		t.Errorf("oracle EXTRA %s", e)
	}
	keys := make([]string, 0, len(res.Notes))
	for k := range res.Notes {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	for _, k := range keys {
		t.Logf("oracle left out: %s x%d", k, res.Notes[k])
	}
	t.Logf("oracle: %d edges, %d findings", len(res.Pairs), len(res.Findings))
	graphTSort(t, res.Pairs, false)
}

// TestCorpusGraphClean: iso20022 and secret produce no GRT002/GRT003 and
// their pinned baseline is unchanged; denis256 produces exactly
// denisExpectedGraph. The textual oracle agrees on all three; the pinned
// terragrunt builds its queue on scratch copies of iso20022 and secret.
func TestCorpusGraphClean(t *testing.T) {
	for _, repo := range graphRepos {
		t.Run(repo.Name, func(t *testing.T) {
			root := repo.Require(t)
			d0 := corpusDigest(t, root)
			rep, raw, code := corpusRunJSON(t, root)
			graph, other := graphGruntled(rep)
			s := rep.Summary
			t.Logf("summary: units=%d resolved=%d config_unknown=%d errors=%d graph=%d other=%d",
				s.Units, s.Resolved, s.ConfigUnknown, s.Errors, len(graph), len(other))

			var want []graphFinding
			switch repo.Name {
			case "iso20022":
				if code != 0 || len(rep.Diagnostics) != 0 || s.Units != 65 || s.ConfigUnknown != 3 || s.Errors != 0 {
					t.Errorf("iso20022 baseline: exit %d, %d diagnostics, units %d, config_unknown %d, errors %d; want 0, 0, 65, 3, 0\n%s",
						code, len(rep.Diagnostics), s.Units, s.ConfigUnknown, s.Errors, raw)
				}
			case "secret":
				if code != 0 || len(rep.Diagnostics) != 0 || s.Units != 4 || s.ConfigUnknown != 1 || s.Errors != 0 {
					t.Errorf("secret baseline: exit %d, %d diagnostics, units %d, config_unknown %d, errors %d; want 0, 0, 4, 1, 0\n%s",
						code, len(rep.Diagnostics), s.Units, s.ConfigUnknown, s.Errors, raw)
				}
			case "denis256":
				for _, e := range denisExpectedGraph {
					want = append(want, e.finding())
				}
				graphSort(want)
				var grt001 int
				for _, d := range other {
					if d.Code == "GRT001" {
						grt001++
					}
				}
				if code != 1 || grt001 != len(denisExpected) {
					t.Errorf("denis256 baseline: exit %d, %d GRT001; want 1, %d", code, grt001, len(denisExpected))
				}
			}
			missing, extra := graphDiff(want, graph)
			for _, m := range missing {
				t.Errorf("gruntled MISS %s", m)
			}
			for _, e := range extra {
				t.Errorf("gruntled EXTRA %s", e)
			}

			oracleWant := slices.Clone(want)
			if repo.Name == "denis256" {
				for _, e := range denisGraphOracleOnly {
					oracleWant = append(oracleWant, e.finding())
				}
			}
			graphOracleCheck(t, root, oracleWant)

			if repo.Name != "denis256" {
				t.Run("terragrunt", func(t *testing.T) {
					bin := tgVerifyPinned(t)
					cp := repo.Copy(t, root)
					graphTGClean(t, bin, cp, cp)
				})
			}

			if d := corpusDigest(t, root); d != d0 {
				t.Errorf("corpus checkout changed: digest %s -> %s", d0, d)
			}
		})
	}
}

// TestCorpusGraphMutation: every mutation, alone on a fresh copy, adds
// exactly its diagnostic (gruntled and textual oracle), exits 1, leaves
// every other diagnostic unchanged, reverts to byte-identical JSON, and is
// confirmed by the pinned terragrunt.
func TestCorpusGraphMutation(t *testing.T) {
	for _, repo := range graphRepos {
		t.Run(repo.Name, func(t *testing.T) {
			root := repo.Require(t)
			d0 := corpusDigest(t, root)
			_, rootRaw, rootCode := corpusRunJSON(t, root)
			n := 0
			for _, gm := range graphMutations {
				if gm.Repo != repo.Name {
					continue
				}
				n++
				t.Run(gm.M.Name, func(t *testing.T) {
					graphRunMutation(t, repo, root, rootRaw, rootCode, gm)
				})
			}
			if want := map[string]int{"iso20022": 5, "secret": 4, "denis256": 4}[repo.Name]; n != want {
				t.Errorf("%d mutations for %s, want %d", n, repo.Name, want)
			}
			if d := corpusDigest(t, root); d != d0 {
				t.Errorf("corpus checkout changed: digest %s -> %s", d0, d)
			}
		})
	}
}

func graphRunMutation(t *testing.T, repo graphRepo, root string, rootRaw []byte, rootCode int, gm graphMutation) {
	cp := repo.Copy(t, root)
	pre, preRaw, preCode := corpusRunJSON(t, cp)
	if !bytes.Equal(preRaw, rootRaw) || preCode != rootCode {
		t.Fatalf("copy fidelity: gruntled JSON on the copy differs from the checkout (exit %d vs %d)", preCode, rootCode)
	}
	preGraph, preOther := graphGruntled(pre)
	oPre := graphOracle(t, cp)

	orig := corpusApply(t, cp, gm.M)
	rep, raw, code := corpusRunJSON(t, cp)
	if code != 1 {
		t.Errorf("mutated exit %d, want 1", code)
	}
	graph, other := graphGruntled(rep)
	want := append(slices.Clone(preGraph), gm.Want)
	graphSort(want)
	missing, extra := graphDiff(want, graph)
	for _, m := range missing {
		t.Errorf("gruntled MISS %s", m)
	}
	for _, e := range extra {
		t.Errorf("gruntled EXTRA %s", e)
	}
	if !slices.Equal(other, preOther) {
		t.Errorf("GRT001/GRT100 changed: %d before, %d after\n%s", len(preOther), len(other), raw)
	}
	for _, g := range graph {
		t.Logf("gruntled: %s", g)
	}

	oPost := graphOracle(t, cp)
	added, removed := graphDiff(oPost.Findings, oPre.Findings)
	if len(added) != 1 || added[0] != gm.Want || len(removed) != 0 {
		t.Errorf("oracle delta: added %v, removed %v; want exactly %s", added, removed, gm.Want)
	}
	graphTSort(t, oPost.Pairs, gm.TSortLoop)

	p := filepath.Join(cp, filepath.FromSlash(gm.M.File))
	info, err := os.Stat(p)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, orig, info.Mode().Perm()); err != nil {
		t.Fatal(err)
	}
	_, postRaw, postCode := corpusRunJSON(t, cp)
	if !bytes.Equal(postRaw, preRaw) || postCode != preCode {
		t.Errorf("revert: JSON identical=%v, exit %d; want true, %d", bytes.Equal(postRaw, preRaw), postCode, preCode)
	}

	t.Run("terragrunt", func(t *testing.T) {
		bin := tgVerifyPinned(t)
		dir := filepath.Join(cp, filepath.FromSlash(gm.TGDir))
		graphTGClean(t, bin, cp, dir)
		corpusApply(t, cp, gm.M)
		out := graphTGRun(t, bin, cp, dir)
		if !strings.Contains(out, gm.TGWant) {
			t.Errorf("terragrunt does not print %q:\n%s", gm.TGWant, out)
		}
		for _, l := range strings.Split(out, "\n") {
			if strings.Contains(l, gm.TGWant) || strings.Contains(l, "does not contain a terragrunt.hcl") {
				t.Logf("terragrunt: %s", l)
			}
		}
	})
}
