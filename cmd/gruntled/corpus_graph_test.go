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
	"cmp"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"
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
