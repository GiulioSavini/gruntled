package main

// REL-03 (17-02 Task 1): the independent corpus model.
//
// corpusModel is computed from source text only, with the textual scanner
// of corpus_graph_test.go (oracleMask, oracleBlocks, oracleAttr,
// oracleString, oracleIncludes, oracleResolve, oracleClassify) and the
// requirement text (BLAST-03/04, MORE-03, BLAST-06..10). It imports no
// internal/ package and no HCL library (check-architecture Step 6), so it
// shares no code with gruntled's parser, loader or impact engine.
//
// TestCorpusBlastModel compares it once per corpus with HEAD `graph --json`
// on the pristine tree. Every difference must be covered by a pinned
// per-item exclusion (corpusExclusions) whose category is a predicate the
// model evaluates over source text. Direction rule: where gruntled reports
// LESS than the model (a missing unit module, live edge, reference, known
// module or name) the difference fails unless a predicate item covers it;
// where gruntled reports MORE, it is an oracle limit and may be excluded
// with its predicate. An exclusion removes the item from the universe; it
// never copies gruntled's value into the model. The model handed to the
// mutation harness (buildCorpusModel) never reads graph --json.

import (
	"bytes"
	"cmp"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
)

// modelEdge is one dependency block of a unit after include merging.
type modelEdge struct {
	Label     string
	File      string // file holding config_path (or the block)
	Line, Col int    // position of the config_path value
	Enabled   string // absent, true, false, nonlit
	Skip      string // skip_outputs: absent, true, false, nonlit
	Target    string // resolved target dir; "" when config_path is not a resolvable literal
	TargetIs  oracleTargetKind
	Live      bool // BLAST-03: enabled absent/true, skip absent/false, literal target unit
}

// refSite is one dependency.<label>.outputs.<output> traversal in a unit's
// effective configuration (its own file and the includes it merges).
type refSite struct {
	Unit, File   string
	Line, Col    int
	Label        string
	Output       string
	Shape        string // plain, guarded, other
	TargetUnit   string // "" when the label's edge is not a literal unit target
	Module       string // the target unit's model module, "" when none
	Silent       bool   // DIAG-03: the edge is disabled, skip_outputs, non-literal, or not a unit
	SilentReason string
}

// modelUnit is one unit (a directory with a terragrunt.hcl).
type modelUnit struct {
	Path       string
	Files      []string // the unit file, then resolved includes
	ConfigOK   bool     // the model can read the effective configuration
	ConfigWhy  string   // why not
	ConfigAt   string   // file:line evidence
	Source     string   // literal terraform.source, "" when absent
	SourceAt   string   // file:line of the source value
	Module     string   // resolved module dir, "" when none
	ModuleWhy  string   // why there is no module (remote, nonliteral, outside, ...)
	GenerateAt string   // file:line of the first generate block, "" when none
	// GenSurfaceAt is the file:line of the first generate block that could
	// change the module surface blast compares: its contents may declare an
	// output (a literal body with an output block, or a non-literal body),
	// or its literal path names a file the module already has (overwrite).
	// A generate that only declares variables or providers writes the same
	// file on both sides of every module edit, so it changes no diff.
	GenSurfaceAt string
	Edges        []modelEdge
	Sites        []refSite
	// RawTargets are the literal, resolvable targets of the unit file's own
	// dependency blocks that are not literally disabled, computed even when
	// the configuration is not readable: the units an excluded unit may hold
	// a live edge to (its touch set).
	RawTargets []string
	Incs       []modelInclude // resolved includes (label, file), when readable
}

// modelModule is one module directory some unit points at.
type modelModule struct {
	Path       string
	Known      bool
	Why        string // why not known
	WhyAt      string
	Files      []string // module files read (.tf, .tf.json), sorted
	Override   string   // first override file, "" when none
	Vars, Outs []string
	Seeds      []string // units whose model module is this one, sorted
}

// exclusion is one pinned item removed from the universe.
type exclusion struct {
	Path      string // unit or module path the item is about
	Category  string
	Direction string // gruntled-less, gruntled-more, oracle-limit
	Evidence  string // file:line (or file) the predicate must reproduce
}

// corpusModel is the independent model of one pristine corpus.
type corpusModel struct {
	Root     string
	Units    map[string]*modelUnit
	UnitList []string
	Modules  map[string]*modelModule
	Seeds    map[string][]string // module -> instantiating units
	Rev      map[string][]string // unit -> units with a live edge to it, sorted
	Sites    []refSite
	Eligible []string // modules eligible for mutation before exclusions
	Excluded []exclusion
}

// Exclusion categories. Each is a predicate over source text
// (modelPredicates); the test re-evaluates it for every pinned row and
// requires the row's Evidence back.
const (
	catSource       = "source not modelled"                                 // gruntled-more: a source the model cannot evaluate
	catInclude      = "include path not modelled"                           // gruntled-more: an include the model cannot resolve
	catJSONUnit     = "terragrunt.hcl.json unit"                            // gruntled-more: a unit the model does not scan
	catGenerate     = "generate block may declare outputs"                  // gruntled-less: module-unknown
	catIncludeTgt   = "include target"                                      // gruntled-less: config-unknown
	catDupDep       = "duplicate dependency label in one file"              // gruntled-less: config-unknown
	catBareIncludes = "more than one unlabeled include"                     // gruntled-less: config-unknown
	catConfigSyntax = "unit config syntax error the oracle does not detect" // gruntled-less: config-unknown
	catTooDeep      = "nesting over the 1000-level cap"                     // gruntled-less: config-unknown
	catOverlay      = "module dir contains the unit dir"                    // gruntled-less: module-unknown
	catDeepMerge    = "deep-merged dependency (non-literal facts)"          // gruntled-less: enabled/skip_outputs unknown
	catConfigPath   = "config_path absolute or unresolvable"                // gruntled-less: dependency not modelled
	catModuleSyntax = "module syntax error the oracle does not detect"      // gruntled-less: module unknown
	catRefOther     = "reference shape other"                               // oracle-limit: outcome not predicted
	catDirSymlink   = "dir-symlink alias"                                   // oracle-limit: one edit changes two module paths
)

var (
	modelTerraformRe = regexp.MustCompile(`(?m)^[ \t]*terraform[ \t]*\{`)
	modelGenerateRe  = regexp.MustCompile(`(?m)^[ \t]*generate[ \t]+(?:"[^"\n]*"|[A-Za-z_][A-Za-z0-9_-]*)[ \t]*\{`)
	modelSurfaceRe   = regexp.MustCompile(`(?m)^[ \t]*(variable|output)[ \t]+(?:"([^"\n]*)"|([A-Za-z_][A-Za-z0-9_-]*))[ \t]*\{`)
	modelRefRe       = regexp.MustCompile(`dependency\.([A-Za-z_][A-Za-z0-9_-]*)\.outputs\.([A-Za-z_][A-Za-z0-9_-]*)`)
	modelRemoteRe    = regexp.MustCompile(`^(?:[a-zA-Z][a-zA-Z0-9+.-]*::|[a-zA-Z][a-zA-Z0-9+.-]*://|git@|github\.com/|bitbucket\.org/|gitlab\.com/)`)
)

var modelBareDepRe = regexp.MustCompile(`(?m)^[ \t]*dependency[ \t]+([A-Za-z_][A-Za-z0-9_-]*)[ \t]*\{`)

// modelDepBlocks returns the depth-0 dependency blocks of src, with a quoted
// label (oracleBlocks) or a bare identifier label, in source order.
func modelDepBlocks(src string, mask []int) []oracleBlock {
	var out []oracleBlock
	for _, b := range oracleBlocks(src, mask) {
		if b.Kind == "dependency" {
			out = append(out, b)
		}
	}
	for _, mm := range modelBareDepRe.FindAllStringSubmatchIndex(src, -1) {
		start := mm[0]
		for src[start] == ' ' || src[start] == '\t' {
			start++
		}
		if mask[start] != 0 {
			continue
		}
		if b, ok := modelBlockAt(src, mask, mm[1]-1); ok {
			b.Kind, b.Label = "dependency", src[mm[2]:mm[3]]
			out = append(out, b)
		}
	}
	slices.SortFunc(out, func(a, b oracleBlock) int { return cmp.Compare(a.Open, b.Open) })
	return out
}

// modelBlockAt returns the block whose `{` is at open, closed by the first
// '}' at depth 1 (the oracleBlocks rule).
func modelBlockAt(src string, mask []int, open int) (oracleBlock, bool) {
	for j := open + 1; j < len(src); j++ {
		if src[j] == '}' && mask[j] == 1 {
			return oracleBlock{Open: open, Close: j}, true
		}
	}
	return oracleBlock{}, false
}

// modelTopBlocks returns the depth-0 blocks matched by re.
func modelTopBlocks(src string, mask []int, re *regexp.Regexp) []oracleBlock {
	var out []oracleBlock
	for _, m := range re.FindAllStringIndex(src, -1) {
		start := m[0]
		for src[start] == ' ' || src[start] == '\t' {
			start++
		}
		if mask[start] != 0 {
			continue
		}
		if b, ok := modelBlockAt(src, mask, m[1]-1); ok {
			out = append(out, b)
		}
	}
	return out
}

// modelLit reads a depth-1 attribute of b as absent, true, false or nonlit,
// with its value offset.
func modelLit(src string, mask []int, b oracleBlock, name string) (string, int) {
	v, ok := oracleAttr(src, mask, b, name)
	if !ok {
		return "absent", -1
	}
	val := src[v:]
	if i := strings.IndexAny(val, "\n}"); i >= 0 {
		val = val[:i]
	}
	if i := strings.Index(val, "#"); i >= 0 {
		val = val[:i]
	}
	if i := strings.Index(val, "//"); i >= 0 {
		val = val[:i]
	}
	switch strings.TrimSpace(val) {
	case "true":
		return "true", v
	case "false":
		return "false", v
	}
	return "nonlit", v
}

// oracleBalanced reports whether src scans as balanced HCL: braces,
// brackets and parentheses over code bytes never go negative and end at
// zero, and the file does not end inside a string, template or heredoc.
func oracleBalanced(src string) bool {
	kinds, closed := modelKindsEOF(src)
	if !closed {
		return false
	}
	var b, k, p int
	for i := 0; i < len(src); i++ {
		if kinds[i] != 'c' {
			continue
		}
		switch src[i] {
		case '{':
			b++
		case '}':
			b--
		case '[':
			k++
		case ']':
			k--
		case '(':
			p++
		case ')':
			p--
		}
		if b < 0 || k < 0 || p < 0 {
			return false
		}
	}
	return b == 0 && k == 0 && p == 0
}

func modelKinds(src string) []byte {
	k, _ := modelKindsEOF(src)
	return k
}

// modelKinds classifies every byte of src: 'c' code, 's' string text,
// 't' template code (inside ${...} or %{...} of a string or heredoc),
// 'h' heredoc text, 'm' comment. Only 'c' and 't' bytes can hold a
// traversal. closed reports that no string, template or heredoc is open at
// the end of src.
func modelKindsEOF(src string) (k []byte, closed bool) {
	k = make([]byte, len(src))
	type frame struct {
		str    bool // in string text (else template code)
		here   string
		braces int
	}
	var stack []frame
	for i := 0; i < len(src); {
		c := src[i]
		var next byte
		if i+1 < len(src) {
			next = src[i+1]
		}
		if len(stack) == 0 || !stack[len(stack)-1].str {
			kind := byte('c')
			if len(stack) > 0 {
				kind = 't'
			}
			switch {
			case c == '#' || (c == '/' && next == '/'):
				j := strings.IndexByte(src[i:], '\n')
				if j < 0 {
					j = len(src) - i
				}
				for x := i; x < i+j; x++ {
					k[x] = 'm'
				}
				i += j
				continue
			case c == '/' && next == '*':
				end := len(src)
				if j := strings.Index(src[i+2:], "*/"); j >= 0 {
					end = i + 2 + j + 2
				}
				for x := i; x < end; x++ {
					k[x] = 'm'
				}
				i = end
				continue
			case c == '<' && next == '<' && len(stack) == 0:
				if m := oracleHeredocRe.FindStringSubmatch(src[i:]); m != nil {
					// The opener is code; its line end belongs to the heredoc.
					for x := i; x < i+len(m[0]); x++ {
						k[x] = kind
						if src[x] == '\n' || src[x] == '\r' {
							k[x] = 'h'
						}
					}
					stack = append(stack, frame{str: true, here: m[1]})
					i += len(m[0])
					continue
				}
			case c == '"':
				k[i] = 's'
				stack = append(stack, frame{str: true})
				i++
				continue
			case c == '{' && len(stack) > 0:
				stack[len(stack)-1].braces++
			case c == '}' && len(stack) > 0:
				top := &stack[len(stack)-1]
				if top.braces == 0 {
					k[i] = 't'
					stack = stack[:len(stack)-1]
					i++
					continue
				}
				top.braces--
			}
			k[i] = kind
			i++
			continue
		}
		top := stack[len(stack)-1]
		textKind := byte('s')
		if top.here != "" {
			textKind = 'h'
		}
		switch {
		case top.here != "" && (i == 0 || src[i-1] == '\n'):
			nl := strings.IndexByte(src[i:], '\n')
			lineEnd := len(src)
			if nl >= 0 {
				lineEnd = i + nl
			}
			if strings.TrimSpace(src[i:lineEnd]) == top.here {
				for x := i; x < lineEnd; x++ {
					k[x] = 'h'
				}
				stack = stack[:len(stack)-1]
				i = lineEnd
				continue
			}
			fallthrough
		default:
			switch {
			case top.here == "" && c == '\\':
				k[i] = textKind
				if i+1 < len(src) {
					k[i+1] = textKind
				}
				i += 2
				continue
			case top.here == "" && c == '"':
				k[i] = 's'
				stack = stack[:len(stack)-1]
				i++
				continue
			case (c == '$' || c == '%') && next == '{':
				k[i], k[i+1] = 't', 't'
				stack = append(stack, frame{})
				i += 2
				continue
			}
			k[i] = textKind
			i++
		}
	}
	return k, len(stack) == 0
}

// modelShape classifies the line holding a traversal: plain is corpusOracle's
// plainRe (`attr = dependency.l.outputs.y` alone on its line); guarded is a
// line with try(, can(, a conditional, && or ||, or a for expression; the
// rest is other.
func modelShape(line, label, output string) string {
	plain := regexp.MustCompile(`^\s*[A-Za-z0-9_]+\s*=\s*dependency\.` + regexp.QuoteMeta(label) + `\.outputs\.` + regexp.QuoteMeta(output) + `\s*$`)
	if plain.MatchString(line) {
		return "plain"
	}
	for _, g := range []string{"try(", "can(", " ? ", "&&", "||", "[for ", "{for ", "for "} {
		if strings.Contains(line, g) {
			return "guarded"
		}
	}
	return "other"
}

// modelSourceFile is one file of a unit's effective configuration.
type modelSourceFile struct {
	rel, src string
	mask     []int
}

func (m *corpusModel) readSource(rel string) (modelSourceFile, error) {
	b, err := os.ReadFile(filepath.Join(m.Root, filepath.FromSlash(rel)))
	if err != nil {
		return modelSourceFile{}, err
	}
	s := string(b)
	return modelSourceFile{rel: rel, src: s, mask: oracleMask(s)}, nil
}

func modelAt(f modelSourceFile, off int) string {
	line, _ := oracleLineCol(f.src, off)
	return fmt.Sprintf("%s:%d", f.rel, line)
}

// modelInclude is one include a unit merges: its label and file.
type modelInclude struct{ Label, File string }

// modelPathFuncs evaluates the Terragrunt path functions in s by their
// documented meaning, in the including unit's context (an included file is
// evaluated as part of the child, so the same values apply in either file):
// ${get_terragrunt_dir()} is the unit dir; ${get_parent_terragrunt_dir()}
// the dir of the (first) included file, or the unit dir with no include;
// ${get_parent_terragrunt_dir("l")} the dir of include "l";
// ${path_relative_to_include()} the path from the include dir to the unit
// dir and ${path_relative_from_include()} the reverse ("." with no
// include). rooted reports that s now starts at the repository root. Any
// other interpolation is not modelled (ok false).
func modelPathFuncs(s, unitDir string, incs []modelInclude) (out string, rooted, ok bool) {
	incDir := unitDir
	if len(incs) > 0 {
		incDir = path.Dir(incs[0].File)
	}
	rel := func(from, to string) string {
		r, err := filepath.Rel(filepath.FromSlash(from), filepath.FromSlash(to))
		if err != nil {
			return "\x00"
		}
		return filepath.ToSlash(r)
	}
	if strings.HasPrefix(s, "${get_terragrunt_dir()}") {
		s, rooted = unitDir+strings.TrimPrefix(s, "${get_terragrunt_dir()}"), true
	} else if strings.HasPrefix(s, "${get_parent_terragrunt_dir()}") {
		s, rooted = incDir+strings.TrimPrefix(s, "${get_parent_terragrunt_dir()}"), true
	} else if m := modelParentLabelRe.FindStringSubmatch(s); m != nil {
		found := false
		for _, inc := range incs {
			if inc.Label == m[1] {
				s, rooted, found = path.Dir(inc.File)+s[len(m[0]):], true, true
			}
		}
		if !found {
			return "", false, false
		}
	}
	s = strings.ReplaceAll(s, "${path_relative_to_include()}", rel(incDir, unitDir))
	s = strings.ReplaceAll(s, "${path_relative_from_include()}", rel(unitDir, incDir))
	if strings.Contains(s, "${") || strings.Contains(s, "%{") || strings.Contains(s, "\x00") {
		return "", false, false
	}
	return s, rooted, true
}

var modelIncludeLabelRe = regexp.MustCompile(`include[ \t]+"([^"]*)"`)

var modelParentLabelRe = regexp.MustCompile(`^\$\{get_parent_terragrunt_dir\("([^"]*)"\)\}`)

// modelJoin places a path-function-evaluated value: rooted values are
// repository paths, other relative values are joined to the unit dir.
func modelJoin(s, unitDir string, rooted bool) (string, bool) {
	var target string
	switch {
	case rooted:
		target = path.Clean(s)
	case path.IsAbs(s) || s == "":
		return "", false
	default:
		target = path.Join(unitDir, s)
	}
	if target == ".." || strings.HasPrefix(target, "../") {
		return "", false
	}
	return target, true
}

// resolveSource turns a literal terraform.source into a module dir. Local
// sources are relative paths (after modelPathFuncs), with go-getter's `//`
// subdirectory split dropped; anything with a scheme, a forced getter or a
// host is remote.
func resolveSource(raw, unitDir string, incs []modelInclude) (string, string) {
	if modelRemoteRe.MatchString(raw) || strings.Contains(raw, "?ref=") {
		return "", "remote"
	}
	s, rooted, ok := modelPathFuncs(raw, unitDir, incs)
	if !ok {
		return "", "nonliteral"
	}
	if !rooted && !strings.HasPrefix(s, "./") && !strings.HasPrefix(s, "../") && s != "." && s != ".." {
		return "", "not-a-relative-path"
	}
	target, ok := modelJoin(strings.Replace(s, "//", "/", 1), unitDir, rooted)
	if !ok {
		return "", "outside"
	}
	return target, ""
}

// findInParent evaluates find_in_parent_folders(name) for the unit in
// unitDir: the first parent dir (starting at the unit dir's parent) holding
// an entry called name.
func (m *corpusModel) findInParent(unitDir, name string) (string, bool) {
	if unitDir == "." {
		return "", false
	}
	for dir := path.Dir(unitDir); ; dir = path.Dir(dir) {
		cand := path.Clean(path.Join(dir, name))
		if _, err := os.Stat(filepath.Join(m.Root, filepath.FromSlash(cand))); err == nil {
			return cand, true
		}
		if dir == "." {
			return "", false
		}
	}
}

// buildCorpusModel computes the model of the pristine tree at root from
// source text only.
func buildCorpusModel(t testing.TB, root string) *corpusModel {
	t.Helper()
	m := &corpusModel{Root: root, Units: map[string]*modelUnit{}, Modules: map[string]*modelModule{},
		Seeds: map[string][]string{}, Rev: map[string][]string{}}
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			// Hidden directories hold units too (gruntled and Terragrunt
			// discover them); only VCS and tool caches are skipped.
			switch d.Name() {
			case ".git", ".terragrunt-cache", ".terraform":
				return filepath.SkipDir
			}
			return nil
		}
		if d.Name() == "terragrunt.hcl" && d.Type().IsRegular() {
			rel, err := filepath.Rel(root, filepath.Dir(p))
			if err != nil {
				return err
			}
			m.UnitList = append(m.UnitList, filepath.ToSlash(rel))
		}
		return nil
	})
	if err != nil {
		t.Fatalf("model walk: %v", err)
	}
	slices.Sort(m.UnitList)
	isUnit := map[string]bool{}
	for _, u := range m.UnitList {
		isUnit[u] = true
	}
	for _, u := range m.UnitList {
		m.Units[u] = m.buildUnit(u, isUnit)
	}
	// Modules and seeds.
	for _, u := range m.UnitList {
		mu := m.Units[u]
		if mu.Module == "" {
			continue
		}
		mod := m.Modules[mu.Module]
		if mod == nil {
			mod = m.buildModule(mu.Module)
			m.Modules[mu.Module] = mod
		}
		mod.Seeds = append(mod.Seeds, u)
	}
	for p, mod := range m.Modules {
		slices.Sort(mod.Seeds)
		m.Seeds[p] = mod.Seeds
	}
	// Live reverse edges and site silence.
	for _, u := range m.UnitList {
		mu := m.Units[u]
		for _, e := range mu.Edges {
			if e.Live {
				m.Rev[e.Target] = append(m.Rev[e.Target], u)
			}
		}
		for i := range mu.Sites {
			m.Sites = append(m.Sites, mu.Sites[i])
		}
	}
	for k, v := range m.Rev {
		slices.Sort(v)
		m.Rev[k] = slices.Compact(v)
	}
	for _, p := range slices.Sorted(maps(m.Modules)) {
		mod := m.Modules[p]
		if !mod.Known || mod.Override != "" || len(mod.Seeds) == 0 {
			continue
		}
		gen := false
		for _, s := range mod.Seeds {
			if m.Units[s].GenSurfaceAt != "" {
				gen = true
			}
		}
		if !gen {
			m.Eligible = append(m.Eligible, p)
		}
	}
	return m
}

func maps[V any](mm map[string]V) func(func(string) bool) {
	return func(yield func(string) bool) {
		for k := range mm {
			if !yield(k) {
				return
			}
		}
	}
}

// buildUnit reads one unit's effective configuration.
func (m *corpusModel) buildUnit(u string, isUnit map[string]bool) *modelUnit {
	mu := &modelUnit{Path: u, ConfigOK: true}
	rel := path.Join(u, "terragrunt.hcl")
	if u == "." {
		rel = "terragrunt.hcl"
	}
	unit, err := m.readSource(rel)
	if err != nil {
		mu.ConfigOK, mu.ConfigWhy, mu.ConfigAt = false, "unreadable", rel
		return mu
	}
	for _, b := range modelDepBlocks(unit.src, unit.mask) {
		// A block literally disabled in the unit file (enabled = false or
		// skip_outputs = true) can never propagate, whatever else is read.
		if en, _ := modelLit(unit.src, unit.mask, b, "enabled"); en == "false" {
			continue
		}
		if sk, _ := modelLit(unit.src, unit.mask, b, "skip_outputs"); sk == "true" {
			continue
		}
		if v, ok := oracleAttr(unit.src, unit.mask, b, "config_path"); ok {
			if raw, _, ok := oracleString(unit.src, unit.mask, v); ok {
				if s2, rooted, ok := modelPathFuncs(raw, u, nil); ok {
					if target, ok := modelJoin(s2, u, rooted); ok {
						if path.Base(target) == "terragrunt.hcl" {
							target = path.Dir(target)
						}
						mu.RawTargets = append(mu.RawTargets, target)
					}
				}
			} else if fm := oracleFindRe.FindStringSubmatch(unit.src[v:]); fm != nil && fm[1] != "" {
				if target, ok := m.findInParent(u, fm[1]); ok {
					if path.Base(target) == "terragrunt.hcl" {
						target = path.Dir(target)
					}
					mu.RawTargets = append(mu.RawTargets, target)
				}
			}
		}
	}
	if !oracleBalanced(unit.src) {
		mu.ConfigOK, mu.ConfigWhy, mu.ConfigAt = false, "unbalanced", rel
		return mu
	}
	notes := map[string]int{}
	srcs := []modelSourceFile{unit}
	mu.Files = []string{rel}
	for _, inc := range oracleIncludes(m.Root, u, oracleSource{rel: unit.rel, src: unit.src, mask: unit.mask}, notes) {
		f, err := m.readSource(inc)
		if err != nil {
			mu.ConfigOK, mu.ConfigWhy, mu.ConfigAt = false, "include unreadable", inc
			return mu
		}
		if !oracleBalanced(f.src) {
			mu.ConfigOK, mu.ConfigWhy, mu.ConfigAt = false, "include unbalanced", inc
			return mu
		}
		srcs = append(srcs, f)
		mu.Files = append(mu.Files, inc)
	}
	if len(notes) > 0 {
		keys := slices.Sorted(maps(notes))
		mu.ConfigOK, mu.ConfigWhy, mu.ConfigAt = false, "include not modelled: "+strings.Join(keys, ", "), rel
		for _, b := range oracleBlocks(unit.src, unit.mask) {
			if b.Kind == "include" {
				mu.ConfigAt = modelAt(unit, b.Open)
				break
			}
		}
		return mu
	}
	// Include labels, in the order oracleIncludes resolved the files (it
	// skips no_merge includes, and every other include resolved above).
	var incs []modelInclude
	for _, b := range oracleBlocks(unit.src, unit.mask) {
		if b.Kind != "include" {
			continue
		}
		if v, ok := oracleAttr(unit.src, unit.mask, b, "merge_strategy"); ok && strings.HasPrefix(unit.src[v:], `"no_merge"`) {
			continue
		}
		label := ""
		lineStart := strings.LastIndexByte(unit.src[:b.Open], '\n') + 1
		if lm := modelIncludeLabelRe.FindStringSubmatch(unit.src[lineStart:b.Open]); lm != nil {
			label = lm[1]
		}
		incs = append(incs, modelInclude{Label: label})
	}
	if len(incs) != len(srcs)-1 {
		mu.ConfigOK, mu.ConfigWhy, mu.ConfigAt = false, "include count mismatch", rel
		return mu
	}
	for i := range incs {
		incs[i].File = srcs[i+1].rel
	}
	mu.Incs = incs
	// terraform.source: the unit file wins, then includes in order.
	sourceSet := false
	for _, f := range srcs {
		for _, b := range modelTopBlocks(f.src, f.mask, modelTerraformRe) {
			v, ok := oracleAttr(f.src, f.mask, b, "source")
			if !ok || sourceSet {
				continue
			}
			sourceSet = true
			mu.SourceAt = modelAt(f, v)
			raw, end, ok := oracleString(f.src, f.mask, v)
			if !ok || !oracleRestIs(f.src, end, "#", "//", "}") {
				mu.ModuleWhy = "nonliteral"
				continue
			}
			mu.Source = raw
			// A relative source, also one set in an included file, is
			// resolved against the including unit's directory.
			mu.Module, mu.ModuleWhy = resolveSource(raw, u, incs)
		}
	}
	if !sourceSet {
		mu.Module = u
	}
	for _, f := range srcs {
		for _, gb := range modelTopBlocks(f.src, f.mask, modelGenerateRe) {
			at := modelAt(f, gb.Open)
			if mu.GenerateAt == "" {
				mu.GenerateAt = at
			}
			if mu.GenSurfaceAt == "" && m.generateTouchesSurface(f, gb, mu.Module) {
				mu.GenSurfaceAt = at
			}
		}
	}

	// Dependency blocks: unit-file labels override included ones.
	byLabel := map[string]modelEdge{}
	var order []string
	for si, f := range srcs {
		for _, b := range modelDepBlocks(f.src, f.mask) {
			if _, ok := byLabel[b.Label]; ok && si > 0 {
				continue
			}
			e := modelEdge{Label: b.Label, File: f.rel}
			e.Enabled, _ = modelLit(f.src, f.mask, b, "enabled")
			e.Skip, _ = modelLit(f.src, f.mask, b, "skip_outputs")
			e.Line, e.Col = oracleLineCol(f.src, b.Open)
			if v, ok := oracleAttr(f.src, f.mask, b, "config_path"); ok {
				e.Line, e.Col = oracleLineCol(f.src, v)
				if raw, end, ok := oracleString(f.src, f.mask, v); ok && oracleRestIs(f.src, end, "#", "//", "}") {
					if s2, rooted, ok := modelPathFuncs(raw, u, incs); ok {
						if target, ok := modelJoin(s2, u, rooted); ok {
							if path.Base(target) == "terragrunt.hcl" {
								target = path.Dir(target)
							}
							if !strings.HasSuffix(target, ".hcl") && !strings.HasSuffix(target, ".json") {
								e.Target = target
								e.TargetIs = oracleClassify(m.Root, target)
							}
						}
					}
				} else if fm := oracleFindRe.FindStringSubmatch(f.src[v:]); fm != nil && oracleRestIs(f.src, v+len(fm[0]), "#", "//", "}") {
					name := fm[1]
					if name == "" {
						name = "terragrunt.hcl"
					}
					if target, ok := m.findInParent(u, name); ok {
						// A config_path may name the dependency's config file
						// itself; Terragrunt then uses its directory.
						if path.Base(target) == "terragrunt.hcl" {
							target = path.Dir(target)
						}
						e.Target = target
						e.TargetIs = oracleClassify(m.Root, target)
					}
				}
			}
			e.Live = (e.Enabled == "absent" || e.Enabled == "true") && (e.Skip == "absent" || e.Skip == "false") &&
				e.Target != "" && e.TargetIs == oracleTargetUnit && isUnit[e.Target]
			if _, ok := byLabel[b.Label]; !ok {
				order = append(order, b.Label)
			}
			byLabel[b.Label] = e
		}
	}
	for _, l := range order {
		mu.Edges = append(mu.Edges, byLabel[l])
	}

	// References.
	for _, f := range srcs {
		kinds := modelKinds(f.src)
		lines := strings.Split(f.src, "\n")
		for _, loc := range modelRefRe.FindAllStringSubmatchIndex(f.src, -1) {
			p := loc[0]
			if kinds[p] != 'c' && kinds[p] != 't' {
				continue
			}
			if p > 0 {
				prev := f.src[p-1]
				if prev == '.' || prev == '_' || prev == '-' || (prev >= 'a' && prev <= 'z') || (prev >= 'A' && prev <= 'Z') || (prev >= '0' && prev <= '9') {
					continue
				}
			}
			label, output := f.src[loc[2]:loc[3]], f.src[loc[4]:loc[5]]
			line, col := oracleLineCol(f.src, p)
			s := refSite{Unit: u, File: f.rel, Line: line, Col: col, Label: label, Output: output,
				Shape: modelShape(lines[line-1], label, output)}
			e, ok := byLabel[label]
			switch {
			case !ok:
				s.Silent, s.SilentReason = true, "undeclared label"
			case e.Enabled != "absent" && e.Enabled != "true":
				s.Silent, s.SilentReason = true, "enabled "+e.Enabled
			case e.Skip != "absent" && e.Skip != "false":
				s.Silent, s.SilentReason = true, "skip_outputs "+e.Skip
			case e.Target == "" || e.TargetIs != oracleTargetUnit || !isUnit[e.Target]:
				s.Silent, s.SilentReason = true, "target not a literal unit"
			default:
				s.TargetUnit = e.Target
			}
			mu.Sites = append(mu.Sites, s)
		}
	}
	return mu
}

var modelOutputBlockRe = regexp.MustCompile(`(?m)^[ \t]*output[ \t]+(?:"|[A-Za-z_])`)

// generateTouchesSurface reports whether generate block b could change the
// surface of module dir mod (see modelUnit.GenSurfaceAt).
func (m *corpusModel) generateTouchesSurface(f modelSourceFile, b oracleBlock, mod string) bool {
	if v, ok := oracleAttr(f.src, f.mask, b, "path"); ok {
		raw, _, ok := oracleString(f.src, f.mask, v)
		if !ok || strings.Contains(raw, "${") {
			return true
		}
		if mod != "" {
			if _, err := os.Lstat(filepath.Join(m.Root, filepath.FromSlash(mod), filepath.FromSlash(raw))); err == nil {
				return true
			}
		}
	} else {
		return true
	}
	v, ok := oracleAttr(f.src, f.mask, b, "contents")
	if !ok {
		return true
	}
	if hm := oracleHeredocRe.FindStringSubmatch(f.src[v:]); hm != nil {
		body := f.src[v+len(hm[0]):]
		end := len(body)
		for off := 0; off < len(body); {
			nl := strings.IndexByte(body[off:], '\n')
			lineEnd := len(body)
			if nl >= 0 {
				lineEnd = off + nl
			}
			if strings.TrimSpace(body[off:lineEnd]) == hm[1] {
				end = off
				break
			}
			off = lineEnd + 1
		}
		body = body[:end]
		return modelOutputBlockRe.MatchString(body) || strings.Contains(body, "${") || strings.Contains(body, "%{")
	}
	if raw, end, ok := oracleString(f.src, f.mask, v); ok && oracleRestIs(f.src, end, "#", "//", "}") {
		return strings.Contains(raw, "output") || strings.Contains(raw, "${")
	}
	return true
}

// buildModule reads the surface names of one module dir.
func (m *corpusModel) buildModule(p string) *modelModule {
	mod := &modelModule{Path: p, Known: true}
	dir := filepath.Join(m.Root, filepath.FromSlash(p))
	ents, err := os.ReadDir(dir)
	if err != nil {
		mod.Known, mod.Why, mod.WhyAt = false, "module dir missing", p
		return mod
	}
	vars, outs := map[string]bool{}, map[string]bool{}
	for _, e := range ents {
		n := e.Name()
		isTF := strings.HasSuffix(n, ".tf") || strings.HasSuffix(n, ".tofu")
		isJSON := strings.HasSuffix(n, ".tf.json") || strings.HasSuffix(n, ".tofu.json")
		if !isTF && !isJSON {
			continue
		}
		rel := path.Join(p, n)
		fi, err := os.Stat(filepath.Join(dir, n))
		if err != nil || !fi.Mode().IsRegular() {
			continue
		}
		base := strings.TrimSuffix(strings.TrimSuffix(strings.TrimSuffix(n, ".json"), ".tf"), ".tofu")
		if base == "override" || strings.HasSuffix(base, "_override") {
			if mod.Override == "" {
				mod.Override = rel
			}
		}
		mod.Files = append(mod.Files, rel)
		b, err := os.ReadFile(filepath.Join(dir, n))
		if err != nil {
			mod.Known, mod.Why, mod.WhyAt = false, "unreadable", rel
			continue
		}
		if isJSON {
			if !modelJSONNames(b, vars, outs) {
				mod.Known, mod.Why, mod.WhyAt = false, "unparseable json", rel
			}
			continue
		}
		src := string(b)
		if !oracleBalanced(src) {
			mod.Known, mod.Why, mod.WhyAt = false, "unbalanced", rel
			continue
		}
		mask := oracleMask(src)
		for _, sm := range modelSurfaceRe.FindAllStringSubmatchIndex(src, -1) {
			start := sm[0]
			for src[start] == ' ' || src[start] == '\t' {
				start++
			}
			if mask[start] != 0 {
				continue
			}
			name := ""
			if sm[4] >= 0 {
				name = src[sm[4]:sm[5]]
			} else {
				name = src[sm[6]:sm[7]]
			}
			if src[sm[2]:sm[3]] == "variable" {
				vars[name] = true
			} else {
				outs[name] = true
			}
		}
	}
	slices.Sort(mod.Files)
	if len(mod.Files) == 0 && mod.Known {
		mod.Known, mod.Why, mod.WhyAt = false, "no terraform files", p
	}
	mod.Vars = slices.Sorted(maps(vars))
	mod.Outs = slices.Sorted(maps(outs))
	return mod
}

// modelJSONNames adds the variable and output names of a Terraform JSON file
// ({"variable": {"n": {...}}} or a list of such objects).
func modelJSONNames(b []byte, vars, outs map[string]bool) bool {
	var doc map[string]json.RawMessage
	if err := json.Unmarshal(b, &doc); err != nil {
		return false
	}
	add := func(raw json.RawMessage, into map[string]bool) bool {
		if raw == nil {
			return true
		}
		var obj map[string]json.RawMessage
		if json.Unmarshal(raw, &obj) == nil {
			for k := range obj {
				into[k] = true
			}
			return true
		}
		var list []map[string]json.RawMessage
		if json.Unmarshal(raw, &list) == nil {
			for _, o := range list {
				for k := range o {
					into[k] = true
				}
			}
			return true
		}
		return false
	}
	return add(doc["variable"], vars) && add(doc["output"], outs)
}

// modelReach is one unit's place in the blast cone.
type modelReach struct {
	Distance    int
	Source, Via string
}

// bfs walks live reverse edges from seeds (BLAST-03/04): seeds have
// distance 1 and are their own source; a unit first reached at level k+1
// gets distance k+1; its via is the smallest predecessor at level k (the
// shortest path read from the unit back is smallest at its first hop, and
// RepoPath.Compare is a string compare), its source that predecessor's
// source. skip lists units traversed but not listed (Broken), still kept
// in the returned map with their distance so callers can drop them.
func (m *corpusModel) bfs(seeds []string) map[string]modelReach {
	out := map[string]modelReach{}
	frontier := slices.Clone(seeds)
	slices.Sort(frontier)
	frontier = slices.Compact(frontier)
	for _, s := range frontier {
		out[s] = modelReach{Distance: 1, Source: s}
	}
	for d := 1; len(frontier) > 0; d++ {
		next := map[string]string{} // unit -> smallest via
		for _, u := range frontier {
			for _, dep := range m.Rev[u] {
				if _, seen := out[dep]; seen {
					continue
				}
				if v, ok := next[dep]; !ok || u < v {
					next[dep] = u
				}
			}
		}
		frontier = frontier[:0]
		for dep, via := range next {
			out[dep] = modelReach{Distance: d + 1, Source: out[via].Source, Via: via}
			frontier = append(frontier, dep)
		}
		slices.Sort(frontier)
	}
	return out
}

// ---------------------------------------------------------------------------
// Comparison with graph --json (TestCorpusBlastModel only).

// graphFull is the part of Graph JSON the comparison reads.
type graphFull struct {
	Units []struct {
		Path       string `json:"path"`
		Status     string `json:"status"`
		Module     string `json:"module"`
		Reason     string `json:"reason"`
		References []struct {
			Dependency string `json:"dependency"`
			Output     string `json:"output"`
			Position   struct {
				File   string `json:"file"`
				Line   int    `json:"line"`
				Column int    `json:"column"`
			} `json:"position"`
		} `json:"references"`
	} `json:"units"`
	Modules []struct {
		Path      string   `json:"path"`
		Known     bool     `json:"known"`
		Variables []string `json:"variables"`
		Outputs   []string `json:"outputs"`
		Reason    string   `json:"reason"`
	} `json:"modules"`
	Edges []struct {
		Kind     string `json:"kind"`
		From     string `json:"from"`
		To       string `json:"to"`
		Name     string `json:"name"`
		Position struct {
			File   string `json:"file"`
			Line   int    `json:"line"`
			Column int    `json:"column"`
		} `json:"position"`
		TargetState string `json:"target_state"`
		Enabled     string `json:"enabled"`
		SkipOutputs string `json:"skip_outputs"`
	} `json:"edges"`
}

// modelDiff is one difference between the model and graph --json.
type modelDiff struct {
	Subject   string // unit or module path
	Kind      string // unit, module-of, edge, ref, module-known, module-names
	Direction string // gruntled-less or gruntled-more
	Detail    string
}

func (d modelDiff) String() string {
	return fmt.Sprintf("%s %s [%s] %s", d.Direction, d.Subject, d.Kind, d.Detail)
}

func modelTri(s string) string {
	switch s {
	case "absent":
		return ""
	case "nonlit":
		return "unknown"
	}
	return s
}

// compareWithGraph lists every difference between the model and HEAD's
// graph --json on root.
func compareWithGraph(t *testing.T, m *corpusModel) []modelDiff {
	t.Helper()
	var out, errb bytes.Buffer
	if code := run([]string{"graph", "--json", m.Root}, &out, &errb); code != 0 && code != 1 {
		t.Fatalf("graph --json exit %d: %s", code, errb.String())
	}
	var g graphFull
	if err := json.Unmarshal(out.Bytes(), &g); err != nil {
		t.Fatalf("graph --json: %v", err)
	}
	var diffs []modelDiff
	add := func(subject, kind, dir, format string, a ...any) {
		diffs = append(diffs, modelDiff{subject, kind, dir, fmt.Sprintf(format, a...)})
	}

	gUnits := map[string]int{}
	for i, u := range g.Units {
		gUnits[u.Path] = i
	}
	for _, u := range m.UnitList {
		if _, ok := gUnits[u]; !ok {
			add(u, "unit", "gruntled-less", "model unit missing from graph")
		}
	}
	for _, u := range g.Units {
		if _, ok := m.Units[u.Path]; !ok {
			add(u.Path, "unit", "gruntled-more", "graph unit %s (%s) not in model", u.Status, u.Reason)
		}
	}

	// Unit -> module, edges and references.
	type edgeKey struct {
		From, Label, File string
		Line, Col         int
		To, State         string
		Enabled, Skip     string
	}
	gEdges := map[string][]edgeKey{}
	for _, e := range g.Edges {
		if e.Kind != "block" {
			continue
		}
		gEdges[e.From] = append(gEdges[e.From], edgeKey{e.From, e.Name, e.Position.File, e.Position.Line, e.Position.Column, e.To, e.TargetState, e.Enabled, e.SkipOutputs})
	}
	type refKey struct {
		Unit, Label, Output, File string
		Line, Col                 int
	}
	for _, u := range m.UnitList {
		mu := m.Units[u]
		gi, ok := gUnits[u]
		if !ok {
			continue
		}
		gu := g.Units[gi]
		gMod := ""
		if gu.Status == "resolved" {
			gMod = gu.Module
		}
		modelMod := ""
		if mu.ConfigOK {
			modelMod = mu.Module
		}
		switch {
		case modelMod == gMod:
		case gMod == "":
			add(u, "module-of", "gruntled-less", "model module %q, graph %s (%s)", modelMod, gu.Status, gu.Reason)
		case modelMod == "":
			add(u, "module-of", "gruntled-more", "graph module %q, model none (config %v %s; module %s)", gMod, mu.ConfigOK, mu.ConfigWhy, mu.ModuleWhy)
		default:
			add(u, "module-of", "gruntled-less", "model module %q, graph module %q", modelMod, gMod)
		}
		// Edges (block kind, resolved targets only).
		var mEdges []edgeKey
		if mu.ConfigOK {
			for _, e := range mu.Edges {
				if e.Target == "" {
					continue
				}
				state := map[oracleTargetKind]string{oracleTargetUnit: "has-config", oracleTargetMissing: "dir-missing", oracleTargetNoConfig: "no-config", oracleTargetSkip: "unknown"}[e.TargetIs]
				en, sk := modelTri(e.Enabled), modelTri(e.Skip)
				if en == "" {
					en = "true"
				}
				if sk == "" {
					sk = "false"
				}
				mEdges = append(mEdges, edgeKey{u, e.Label, e.File, e.Line, e.Col, e.Target, state, en, sk})
			}
		}
		ge := gEdges[u]
		for _, e := range mEdges {
			if !slices.Contains(ge, e) {
				add(u, "edge", "gruntled-less", "model edge %+v", e)
			}
		}
		for _, e := range ge {
			if !slices.Contains(mEdges, e) {
				add(u, "edge", "gruntled-more", "graph edge %+v", e)
			}
		}
		// References.
		var mRefs, gRefs []refKey
		if mu.ConfigOK {
			for _, s := range mu.Sites {
				mRefs = append(mRefs, refKey{u, s.Label, s.Output, s.File, s.Line, s.Col})
			}
		}
		for _, r := range gu.References {
			gRefs = append(gRefs, refKey{u, r.Dependency, r.Output, r.Position.File, r.Position.Line, r.Position.Column})
		}
		for _, r := range mRefs {
			if !slices.Contains(gRefs, r) {
				add(u, "ref", "gruntled-less", "model ref %+v", r)
			}
		}
		for _, r := range gRefs {
			if !slices.Contains(mRefs, r) {
				add(u, "ref", "gruntled-more", "graph ref %+v", r)
			}
		}
	}

	// Modules: known flag and names, for every module either side knows of.
	gMods := map[string]int{}
	for i, mod := range g.Modules {
		gMods[mod.Path] = i
	}
	for _, p := range slices.Sorted(maps(m.Modules)) {
		mod := m.Modules[p]
		gi, ok := gMods[p]
		if !ok {
			if mod.Known && len(mod.Seeds) > 0 && anyConfigOK(m, mod.Seeds) {
				add(p, "module-known", "gruntled-less", "model known module not in graph modules[]")
			}
			continue
		}
		gm := g.Modules[gi]
		switch {
		case mod.Known && !gm.Known:
			add(p, "module-known", "gruntled-less", "model known (%d files), graph unknown (%s)", len(mod.Files), gm.Reason)
		case !mod.Known && gm.Known:
			add(p, "module-known", "gruntled-more", "graph known, model unknown (%s at %s)", mod.Why, mod.WhyAt)
		case mod.Known && gm.Known:
			gv, go_ := slices.Clone(gm.Variables), slices.Clone(gm.Outputs)
			slices.Sort(gv)
			slices.Sort(go_)
			if !slices.Equal(gv, mod.Vars) {
				add(p, "module-names", "gruntled-less", "variables model %v graph %v", mod.Vars, gv)
			}
			if !slices.Equal(go_, mod.Outs) {
				add(p, "module-names", "gruntled-less", "outputs model %v graph %v", mod.Outs, go_)
			}
		}
	}
	for _, gm := range g.Modules {
		if _, ok := m.Modules[gm.Path]; !ok && gm.Known {
			add(gm.Path, "module-known", "gruntled-more", "graph known module not in model")
		}
	}
	slices.SortFunc(diffs, func(a, b modelDiff) int {
		return cmp.Or(cmp.Compare(a.Subject, b.Subject), cmp.Compare(a.Kind, b.Kind), cmp.Compare(a.Detail, b.Detail))
	})
	return diffs
}

func anyConfigOK(m *corpusModel, units []string) bool {
	for _, u := range units {
		if m.Units[u].ConfigOK {
			return true
		}
	}
	return false
}

// ---------------------------------------------------------------------------
// Predicates: each category is checked against source text, row by row.

// modelPredicate reports whether row's category applies to row.Path with
// row.Evidence as its trigger; want describes the evidence it found.
type modelPredicate func(m *corpusModel, row exclusion) (ok bool, want string)

func unitFile(u string) string {
	if u == "." {
		return "terragrunt.hcl"
	}
	return path.Join(u, "terragrunt.hcl")
}

// single adapts a predicate that computes one evidence string.
func single(f func(m *corpusModel, p string) string) modelPredicate {
	return func(m *corpusModel, row exclusion) (bool, string) {
		got := f(m, row.Path)
		return got != "" && got == row.Evidence, got
	}
}

var modelHeaderRe = regexp.MustCompile(`^[ \t]*[A-Za-z_][A-Za-z0-9_-]*(?:[ \t]+(?:"[^"\n]*"|[A-Za-z_][A-Za-z0-9_-]*))*[ \t]*\{`)
var modelLabelledHeaderRe = regexp.MustCompile(`^[ \t]*[A-Za-z_][A-Za-z0-9_-]*(?:[ \t]+"[^"\n]*")+[ \t]*\{`)
var modelAttrLineRe = regexp.MustCompile(`^[ \t]*[A-Za-z_][A-Za-z0-9_-]*[ \t]*=`)
var modelNoLabelSurfaceRe = regexp.MustCompile(`^[ \t]*(?:variable|output)[ \t]*\{`)

// modelLineStarts returns the byte offset of each line start.
func modelLineStarts(src string) []int {
	starts := []int{0}
	for i := 0; i < len(src); i++ {
		if src[i] == '\n' {
			starts = append(starts, i+1)
		}
	}
	return starts
}

// modelLint finds the first syntax error brace balance cannot see, as
// file:line: template markers ({{) in code; a labelled block header inside
// an object constructor; at nesting depth 0, a line that is not a block
// header, an attribute or a closing brace; a variable or output block with
// no label; or a block opened with a body on the same line that does not
// close on that line.
func modelLint(rel, src string) string {
	kinds, _ := modelKindsEOF(src)
	starts := modelLineStarts(src)
	// Per-line nesting depth over code bytes ({[( ), and whether the
	// innermost open brace is an object constructor (opened after = , ( [ { :).
	type open struct{ object bool }
	var stack []open
	lineDepth := make([]int, len(starts))
	lineInObject := make([]bool, len(starts))
	li := 0
	prevCode := byte(0)
	for i := 0; i < len(src); i++ {
		for li+1 < len(starts) && starts[li+1] <= i {
			li++
			lineDepth[li] = len(stack)
			lineInObject[li] = len(stack) > 0 && stack[len(stack)-1].object
		}
		if kinds[i] != 'c' {
			continue
		}
		c := src[i]
		switch c {
		case '{', '[', '(':
			obj := c != '{' || strings.IndexByte("=,([{:", prevCode) >= 0
			stack = append(stack, open{object: obj})
		case '}', ']', ')':
			if len(stack) > 0 {
				stack = stack[:len(stack)-1]
			}
		}
		if c != ' ' && c != '\t' && c != '\n' && c != '\r' {
			prevCode = c
		}
	}
	for n, st := range starts {
		end := len(src)
		if n+1 < len(starts) {
			end = starts[n+1] - 1
		}
		line := src[st:end]
		trim := strings.TrimSpace(line)
		if trim == "" {
			continue
		}
		first := st + strings.Index(line, trim)
		at := fmt.Sprintf("%s:%d", rel, n+1)
		if kinds[first] != 'c' {
			continue
		}
		if strings.Contains(trim, "{{") {
			return at
		}
		if lineInObject[n] && modelLabelledHeaderRe.MatchString(line) {
			return at
		}
		if lineDepth[n] > 0 && !lineInObject[n] && modelAttrLineRe.MatchString(line) && n+1 < len(starts) && lineDepth[n+1] < lineDepth[n] {
			// An argument and the closing brace of its block on one line.
			return at
		}
		if lineDepth[n] == 0 {
			if modelNoLabelSurfaceRe.MatchString(line) {
				return at
			}
			if !modelHeaderRe.MatchString(line) && !modelAttrLineRe.MatchString(line) && !strings.HasPrefix(trim, "}") {
				return at
			}
			if modelHeaderRe.MatchString(line) {
				rest := strings.TrimSpace(line[len(modelHeaderRe.FindString(line)):])
				if rest != "" && !strings.HasPrefix(rest, "#") && !strings.HasPrefix(rest, "//") && !strings.HasSuffix(rest, "}") {
					return at
				}
			}
		}
	}
	return ""
}

// modelMaxDepth is the deepest bracket nesting over code bytes of src.
func modelMaxDepth(src string) int {
	kinds, _ := modelKindsEOF(src)
	d, best := 0, 0
	for i := 0; i < len(src); i++ {
		if kinds[i] != 'c' && kinds[i] != 't' {
			continue
		}
		switch src[i] {
		case '{', '[', '(':
			d++
			best = max(best, d)
		case '}', ']', ')':
			d--
		}
	}
	return best
}

var modelPredicates = map[string]modelPredicate{
	catSource: single(func(m *corpusModel, p string) string {
		u := m.Units[p]
		if u == nil || !u.ConfigOK || u.Module != "" || u.ModuleWhy == "remote" || u.ModuleWhy == "outside" {
			return ""
		}
		return u.SourceAt
	}),
	catInclude: single(func(m *corpusModel, p string) string {
		u := m.Units[p]
		if u == nil || u.ConfigOK || !strings.HasPrefix(u.ConfigWhy, "include not modelled") {
			return ""
		}
		return u.ConfigAt
	}),
	catJSONUnit: single(func(m *corpusModel, p string) string {
		j := path.Join(p, "terragrunt.hcl.json")
		if _, err := os.Stat(filepath.Join(m.Root, filepath.FromSlash(j))); err != nil {
			return ""
		}
		if _, ok := m.Units[p]; ok {
			return ""
		}
		return j
	}),
	catGenerate: single(func(m *corpusModel, p string) string {
		if u := m.Units[p]; u != nil {
			return u.GenSurfaceAt
		}
		return ""
	}),
	catIncludeTgt: single(func(m *corpusModel, p string) string {
		if m.Units[p] == nil {
			return ""
		}
		for _, v := range m.UnitList {
			if v != p && len(m.Units[v].Files) > 1 && slices.Contains(m.Units[v].Files[1:], unitFile(p)) {
				return unitFile(v)
			}
		}
		// The documented ancestor rule: a strict ancestor of a unit whose
		// includes cannot be resolved (an include path the model does not
		// evaluate, or a terragrunt.hcl.json unit) may be its parent config.
		under := func(v string) bool { return p == "." || strings.HasPrefix(v, p+"/") }
		for _, v := range m.UnitList {
			if under(v) && strings.HasPrefix(m.Units[v].ConfigWhy, "include not modelled") {
				return m.Units[v].ConfigAt
			}
		}
		var jsonUnit string
		_ = filepath.WalkDir(filepath.Join(m.Root, filepath.FromSlash(p)), func(q string, d fs.DirEntry, err error) error {
			if err != nil || jsonUnit != "" {
				return filepath.SkipDir
			}
			if d.IsDir() && (d.Name() == ".git" || d.Name() == ".terragrunt-cache" || d.Name() == ".terraform") {
				return filepath.SkipDir
			}
			if d.Name() == "terragrunt.hcl.json" {
				rel, _ := filepath.Rel(m.Root, q)
				if v := path.Dir(filepath.ToSlash(rel)); under(v) {
					jsonUnit = filepath.ToSlash(rel)
				}
			}
			return nil
		})
		return jsonUnit
	}),
	catDupDep: single(func(m *corpusModel, p string) string {
		f, err := m.readSource(unitFile(p))
		if err != nil {
			return ""
		}
		seen := map[string]bool{}
		for _, b := range modelDepBlocks(f.src, f.mask) {
			if seen[b.Label] {
				return modelAt(f, b.Open)
			}
			seen[b.Label] = true
		}
		return ""
	}),
	catBareIncludes: single(func(m *corpusModel, p string) string {
		f, err := m.readSource(unitFile(p))
		if err != nil {
			return ""
		}
		n := 0
		for _, b := range oracleBlocks(f.src, f.mask) {
			lineStart := strings.LastIndexByte(f.src[:b.Open], '\n') + 1
			if b.Kind == "include" && !modelIncludeLabelRe.MatchString(f.src[lineStart:b.Open]) {
				if n++; n == 2 {
					return modelAt(f, b.Open)
				}
			}
		}
		return ""
	}),
	catConfigSyntax: single(func(m *corpusModel, p string) string {
		f, err := m.readSource(unitFile(p))
		if err != nil {
			return ""
		}
		return modelLint(f.rel, f.src)
	}),
	catTooDeep: single(func(m *corpusModel, p string) string {
		f, err := m.readSource(unitFile(p))
		if err != nil || modelMaxDepth(f.src) <= 1000 {
			return ""
		}
		return f.rel
	}),
	catOverlay: single(func(m *corpusModel, p string) string {
		u := m.Units[p]
		if u == nil || u.Module == "" || u.Module == p || !strings.HasPrefix(p+"/", u.Module+"/") && u.Module != "." {
			return ""
		}
		return u.SourceAt
	}),
	catDeepMerge: single(func(m *corpusModel, p string) string {
		f, err := m.readSource(unitFile(p))
		if err != nil {
			return ""
		}
		own := map[string]bool{}
		for _, b := range modelDepBlocks(f.src, f.mask) {
			own[b.Label] = true
		}
		u := m.Units[p]
		for _, b := range oracleBlocks(f.src, f.mask) {
			if b.Kind != "include" {
				continue
			}
			v, ok := oracleAttr(f.src, f.mask, b, "merge_strategy")
			if !ok || !strings.HasPrefix(f.src[v:], `"deep"`) {
				continue
			}
			for _, inc := range u.Files[1:] {
				g, err := m.readSource(inc)
				if err != nil {
					continue
				}
				for _, db := range modelDepBlocks(g.src, g.mask) {
					if own[db.Label] {
						return modelAt(f, b.Open)
					}
				}
			}
		}
		return ""
	}),
	catConfigPath: single(func(m *corpusModel, p string) string {
		f, err := m.readSource(unitFile(p))
		if err != nil {
			return ""
		}
		for _, b := range modelDepBlocks(f.src, f.mask) {
			v, ok := oracleAttr(f.src, f.mask, b, "config_path")
			if !ok {
				continue
			}
			raw, _, ok := oracleString(f.src, f.mask, v)
			if !ok {
				continue
			}
			var incs []modelInclude
			if u := m.Units[p]; u != nil {
				incs = u.Incs
			}
			s2, rooted, ok := modelPathFuncs(raw, p, incs)
			if !ok {
				return modelAt(f, v)
			}
			if _, ok := modelJoin(s2, p, rooted); !ok {
				return modelAt(f, v)
			}
		}
		return ""
	}),
	catModuleSyntax: single(func(m *corpusModel, p string) string {
		mod := m.Modules[p]
		if mod == nil {
			return ""
		}
		for _, rel := range mod.Files {
			if strings.HasSuffix(rel, ".json") {
				continue
			}
			b, err := os.ReadFile(filepath.Join(m.Root, filepath.FromSlash(rel)))
			if err != nil {
				continue
			}
			if at := modelLint(rel, string(b)); at != "" {
				return at
			}
		}
		return ""
	}),
	catRefOther: func(m *corpusModel, row exclusion) (bool, string) {
		u := m.Units[row.Path]
		if u == nil {
			return false, ""
		}
		var found []string
		for _, s := range u.Sites {
			if s.Shape == "other" && !s.Silent {
				at := fmt.Sprintf("%s:%d", s.File, s.Line)
				found = append(found, at)
				if at == row.Evidence {
					return true, at
				}
			}
		}
		return false, strings.Join(found, " ")
	},
	catDirSymlink: single(func(m *corpusModel, p string) string {
		if m.Modules[p] == nil {
			return ""
		}
		return m.aliasOf(p)
	}),
}

// aliasOf returns, for module p, a symlinked component of its path or
// another model module with the same real directory; "" when neither.
func (m *corpusModel) aliasOf(p string) string {
	cur := m.Root
	for _, seg := range strings.Split(p, "/") {
		if seg == "." {
			continue
		}
		cur = filepath.Join(cur, seg)
		if fi, err := os.Lstat(cur); err == nil && fi.Mode()&fs.ModeSymlink != 0 {
			rel, _ := filepath.Rel(m.Root, cur)
			return filepath.ToSlash(rel)
		}
	}
	real, err := filepath.EvalSymlinks(filepath.Join(m.Root, filepath.FromSlash(p)))
	if err != nil {
		return ""
	}
	for _, q := range slices.Sorted(maps(m.Modules)) {
		if q == p {
			continue
		}
		if r2, err := filepath.EvalSymlinks(filepath.Join(m.Root, filepath.FromSlash(q))); err == nil && r2 == real {
			return q
		}
	}
	return ""
}

// universe applies the exclusions: excluded units (with their touch sets:
// the unit and the targets of its unit-file dependencies) and excluded
// modules, then returns the included modules: eligible, not excluded, with
// no excluded seed and no excluded touch unit in their BFS cone.
func (m *corpusModel) universe(ex []exclusion) (included []string, touch map[string]bool, exMods map[string]bool) {
	touch, exMods = map[string]bool{}, map[string]bool{}
	for _, e := range ex {
		if e.Category == catRefOther {
			continue // per-site: applied per (module, output) by the mutation set
		}
		if u := m.Units[e.Path]; u != nil {
			touch[e.Path] = true
			for _, t := range u.RawTargets {
				touch[t] = true
			}
			for _, ed := range u.Edges {
				if ed.Live {
					touch[ed.Target] = true
				}
			}
		} else {
			touch[e.Path] = true // a unit the model does not scan (JSON unit)
		}
		if m.Modules[e.Path] != nil && m.Units[e.Path] == nil || e.Category == catDirSymlink || e.Category == catModuleSyntax {
			exMods[e.Path] = true
		}
	}
	for _, p := range m.Eligible {
		if exMods[p] {
			continue
		}
		ok := true
		for u := range m.bfs(m.Seeds[p]) {
			if touch[u] {
				ok = false
				break
			}
		}
		if ok {
			included = append(included, p)
		}
	}
	return included, touch, exMods
}

// TestCorpusBlastModel compares the independent model with graph --json on
// each pristine corpus and enforces the pinned exclusion table.
func TestCorpusBlastModel(t *testing.T) {
	for _, r := range graphRepos {
		t.Run(r.Name, func(t *testing.T) {
			root := r.Require(t)
			before := corpusDigest(t, root)
			m := buildCorpusModel(t, root)
			diffs := compareWithGraph(t, m)
			ex := corpusExclusions[r.Name]

			// Every row's predicate holds with its pinned evidence.
			counts := map[string]int{}
			for _, e := range ex {
				pred, ok := modelPredicates[e.Category]
				if !ok {
					t.Errorf("row %s: unknown category %q", e.Path, e.Category)
					continue
				}
				if hit, want := pred(m, e); !hit {
					t.Errorf("row %s (%s): predicate does not hold with evidence %q (found %q)", e.Path, e.Category, e.Evidence, want)
				}
				if e.Direction != "gruntled-less" && e.Direction != "gruntled-more" && e.Direction != "oracle-limit" {
					t.Errorf("row %s: direction %q", e.Path, e.Direction)
				}
				counts[e.Category]++
			}

			// Every difference is covered by a row with the same subject and
			// direction, or derived: a module-level difference whose every
			// model seed is an excluded unit (or which is an excluded unit's
			// own path). An uncovered gruntled-less difference is a finding.
			exUnit := map[string]bool{}
			for _, e := range ex {
				exUnit[e.Path] = true
			}
			used := map[int]bool{}
			derived := 0
			for _, d := range diffs {
				covered := false
				for i, e := range ex {
					if e.Path == d.Subject && e.Direction == d.Direction {
						covered, used[i] = true, true
					}
				}
				if !covered && (d.Kind == "module-known" || d.Kind == "module-names") {
					all := exUnit[d.Subject]
					if mod := m.Modules[d.Subject]; mod != nil && len(mod.Seeds) > 0 {
						all = true
						for _, s := range mod.Seeds {
							all = all && exUnit[s]
						}
					}
					if all {
						covered = true
						derived++
					}
				}
				if !covered {
					t.Errorf("uncovered difference: %s | candidates: %s", d, predicateCandidates(m, d.Subject))
				}
			}
			for i, e := range ex {
				if e.Direction != "oracle-limit" && !used[i] {
					t.Errorf("stale row %s (%s, %s): covers no difference", e.Path, e.Category, e.Direction)
				}
			}

			// Oracle-limit rows: every firing other-shaped site and every
			// dir-symlink alias module has its row.
			for _, s := range m.Sites {
				if s.Shape != "other" || s.Silent {
					continue
				}
				at := fmt.Sprintf("%s:%d", s.File, s.Line)
				if !slices.ContainsFunc(ex, func(e exclusion) bool { return e.Category == catRefOther && e.Path == s.Unit && e.Evidence == at }) {
					t.Errorf("site %s (unit %s, dependency.%s.outputs.%s) has shape other and no %q row", at, s.Unit, s.Label, s.Output, catRefOther)
				}
			}
			for _, p := range slices.Sorted(maps(m.Modules)) {
				if a := m.aliasOf(p); a != "" && len(m.Modules[p].Seeds) > 0 {
					if !slices.ContainsFunc(ex, func(e exclusion) bool { return e.Category == catDirSymlink && e.Path == p }) {
						t.Errorf("module %s is aliased (%s) and has no %q row", p, a, catDirSymlink)
					}
				}
			}

			included, touch, _ := m.universe(ex)
			pin := corpusIncluded[r.Name]
			st := m.stats(included)
			t.Logf("CORPUS-MODEL-FACTS %s %+v", r.Name, st)
			if want := corpusModelFacts[r.Name]; st != want {
				t.Errorf("model facts %+v, pinned %+v", st, want)
			}
			if r.Name == "denis256" && (st.Reach3 == 0 || st.Reach4 == 0 || st.Reach5 == 0) {
				t.Errorf("denis256: included modules reaching distance 3/4/5: %d/%d/%d, each must be > 0", st.Reach3, st.Reach4, st.Reach5)
			}
			var b strings.Builder
			fmt.Fprintf(&b, "CORPUS-MODEL %s units=%d modules=%d eligible=%d included_modules=%d graph_known=%d share=%.3f touch=%d derived=%d",
				r.Name, len(m.UnitList), len(m.Modules), len(m.Eligible), len(included), graphKnownModules(t, root), float64(len(included))/float64(max(1, graphKnownModules(t, root))), len(touch), derived)
			for _, c := range slices.Sorted(maps(counts)) {
				fmt.Fprintf(&b, " excluded_%s=%d", strings.ReplaceAll(c, " ", "_"), counts[c])
			}
			t.Log(b.String())
			if len(included) != pin {
				t.Errorf("included modules %d, pinned %d", len(included), pin)
			}
			if len(included) == 0 {
				t.Errorf("no included module: the corpus proves nothing")
			}
			if after := corpusDigest(t, root); after != before {
				t.Errorf("checkout changed: digest %s -> %s", before, after)
			}
		})
	}
}

// predicateCandidates lists the categories whose predicate holds for path,
// with the evidence each found (the review input for a new row).
func predicateCandidates(m *corpusModel, p string) string {
	var out []string
	for _, c := range slices.Sorted(maps(modelPredicates)) {
		if c == catRefOther {
			continue
		}
		if _, want := modelPredicates[c](m, exclusion{Path: p}); want != "" {
			out = append(out, fmt.Sprintf("{%q %q}", c, want))
		}
	}
	if len(out) == 0 {
		return "none"
	}
	return strings.Join(out, " ")
}

// graphKnownModules counts the known modules of graph --json on root (the
// denominator of the included share; logging only).
func graphKnownModules(t *testing.T, root string) int {
	t.Helper()
	var out, errb bytes.Buffer
	run([]string{"graph", "--json", root}, &out, &errb)
	var g graphFull
	if err := json.Unmarshal(out.Bytes(), &g); err != nil {
		t.Fatalf("graph --json: %v", err)
	}
	n := 0
	for _, mod := range g.Modules {
		if mod.Known {
			n++
		}
	}
	return n
}

// modelFacts are the pinned per-corpus model counts.
type modelFacts struct {
	Seeds      int // units whose model module is an included module
	LiveEdges  int // live dependency edges (BLAST-03), all units
	PlainSites int // firing (non-silent) plain reference sites into included modules
	Pairs      int // distinct (module, output) pairs of those sites
	Guarded    int // firing guarded sites (silent by the lazy-guard rule)
	Reach2     int // included modules whose cone reaches distance >= 2
	Reach3     int
	Reach4     int
	Reach5     int
}

func (m *corpusModel) stats(included []string) modelFacts {
	var f modelFacts
	inc := map[string]bool{}
	for _, p := range included {
		inc[p] = true
		f.Seeds += len(m.Seeds[p])
		maxd := 0
		for _, r := range m.bfs(m.Seeds[p]) {
			maxd = max(maxd, r.Distance)
		}
		for k, n := range map[int]*int{2: &f.Reach2, 3: &f.Reach3, 4: &f.Reach4, 5: &f.Reach5} {
			if maxd >= k {
				*n++
			}
		}
	}
	for _, u := range m.UnitList {
		for _, e := range m.Units[u].Edges {
			if e.Live {
				f.LiveEdges++
			}
		}
	}
	pairs := map[string]bool{}
	for _, s := range m.Sites {
		if s.Silent || !m.Units[s.Unit].ConfigOK {
			continue
		}
		mod := m.Units[s.TargetUnit].Module
		if !inc[mod] {
			continue
		}
		switch s.Shape {
		case "plain":
			f.PlainSites++
			pairs[mod+" "+s.Output] = true
		case "guarded":
			f.Guarded++
		}
	}
	f.Pairs = len(pairs)
	return f
}

// corpusModelFacts pins the model counts per corpus.
var corpusModelFacts = map[string]modelFacts{
	"iso20022": {Seeds: 62, LiveEdges: 11, PlainSites: 22, Pairs: 4, Reach2: 2},
	"secret":   {Seeds: 3, LiveEdges: 3, PlainSites: 4, Pairs: 3, Reach2: 2},
	"denis256": {Seeds: 799, LiveEdges: 588, PlainSites: 460, Pairs: 93, Reach2: 151, Reach3: 26, Reach4: 12, Reach5: 2},
}

// corpusIncluded pins the included-module count per corpus.
var corpusIncluded = map[string]int{
	"iso20022": 62,  // of 62 known (share 1.000)
	"secret":   3,   // of 3 known (share 1.000)
	"denis256": 795, // of 818 known (share 0.972)
}

// corpusExclusions is the pinned per-item table: every model/graph
// difference and every item the model cannot predict, with the predicate
// category that justifies it and the file:line evidence the predicate must
// reproduce. Reviewed by sec before 17-03 (bus id in 17-02-SUMMARY).
var corpusExclusions = map[string][]exclusion{
	"iso20022": {
		{"iac.cicd/codebuild_project", catDupDep, "gruntled-less", "iac.cicd/codebuild_project/terragrunt.hcl:6"},
		{"iac.src/scheduler_recover", catDupDep, "gruntled-less", "iac.src/scheduler_recover/terragrunt.hcl:6"},
		{"iac.src/scheduler_timeout", catDupDep, "gruntled-less", "iac.src/scheduler_timeout/terragrunt.hcl:6"},
	},
	"secret": {
		{"terragrunt", catIncludeTgt, "gruntled-less", "terragrunt/acm/terragrunt.hcl"},
	},
	"denis256": {
		{"autoinclude-bugs/case1-object-key-leak/units/app", catModuleSyntax, "gruntled-less", "autoinclude-bugs/case1-object-key-leak/units/app/main.tf:1"},
		{"broken-dependencies/dependency", catModuleSyntax, "gruntled-less", "broken-dependencies/dependency/main.tf:8"},
		{"catalog-failure", catModuleSyntax, "gruntled-less", "catalog-failure/variables.tf:1"},
		{"centralized-cache", catIncludeTgt, "gruntled-less", "centralized-cache/app/terragrunt.hcl"},
		{"complex-mock/app", catRefOther, "oracle-limit", "complex-mock/app/terragrunt.hcl:24"},
		{"complex-mock/app", catRefOther, "oracle-limit", "complex-mock/app/terragrunt.hcl:25"},
		{"complex-mock/app", catRefOther, "oracle-limit", "complex-mock/app/terragrunt.hcl:26"},
		{"complex-mock/app", catRefOther, "oracle-limit", "complex-mock/app/terragrunt.hcl:27"},
		{"deep-merge-fix/module", catDeepMerge, "gruntled-less", "deep-merge-fix/module/terragrunt.hcl:12"},
		{"deep-merge-fix/module", catDeepMerge, "gruntled-more", "deep-merge-fix/module/terragrunt.hcl:12"},
		{"discussion-621", catIncludeTgt, "gruntled-less", "discussion-621/app1/terragrunt.hcl"},
		{"discussion-621/app1", catGenerate, "gruntled-less", "discussion-621/terragrunt.hcl:5"},
		{"discussion-621/app2", catGenerate, "gruntled-less", "discussion-621/terragrunt.hcl:5"},
		{"discussion-625", catIncludeTgt, "gruntled-less", "discussion-625/app/terragrunt.hcl"},
		{"discussion-690/tg-modules", catIncludeTgt, "gruntled-less", "discussion-690/tg-modules/moda/terragrunt.hcl"},
		{"discussion-692", catIncludeTgt, "gruntled-less", "discussion-692/kubernetes/terragrunt.hcl"},
		{"discussion-692/kubernetes", catGenerate, "gruntled-less", "discussion-692/terragrunt.hcl:6"},
		{"discussion-692/storage", catGenerate, "gruntled-less", "discussion-692/terragrunt.hcl:6"},
		{"discussion-695/app", catRefOther, "oracle-limit", "discussion-695/app/terragrunt.hcl:12"},
		{"discussion-710", catIncludeTgt, "gruntled-less", "discussion-710/app/terragrunt.hcl"},
		{"discussion-734", catIncludeTgt, "gruntled-less", "discussion-734/dev/terragrunt.hcl"},
		{"discussion-734/dev", catGenerate, "gruntled-less", "discussion-734/terragrunt.hcl:7"},
		{"discussion-734/stage", catGenerate, "gruntled-less", "discussion-734/terragrunt.hcl:7"},
		{"discussion-738/app", catRefOther, "oracle-limit", "discussion-738/app/inputs.hcl:2"},
		{"discussion-793", catIncludeTgt, "gruntled-less", "discussion-793/app1/terragrunt.hcl"},
		{"discussion-812/app", catRefOther, "oracle-limit", "discussion-812/app/terragrunt.hcl:16"},
		{"discussion-815", catIncludeTgt, "gruntled-less", "discussion-815/env1/terragrunt.hcl"},
		{"encryption", catConfigSyntax, "gruntled-less", "encryption/terragrunt.hcl:14"},
		{"engine/dependency", catIncludeTgt, "gruntled-less", "engine/dependency/app/terragrunt.hcl"},
		{"engine/opentofu-run-all", catIncludeTgt, "gruntled-less", "engine/opentofu-run-all/app1/terragrunt.hcl"},
		{"engine/terraform-run-all", catIncludeTgt, "gruntled-less", "engine/terraform-run-all/app1/terragrunt.hcl"},
		{"iam_role", catIncludeTgt, "gruntled-less", "iam_role/app/terragrunt.hcl"},
		{"init-error/init-s3-error", catIncludeTgt, "gruntled-less", "init-error/init-s3-error/d1/terragrunt.hcl"},
		{"init-error/init-s3-error-deps", catIncludeTgt, "gruntled-less", "init-error/init-s3-error-deps/d1/terragrunt.hcl"},
		{"issue-1855/terragrunt", catGenerate, "gruntled-less", "issue-1855/terragrunt/terragrunt.hcl:5"},
		{"issue-1855/terragrunt", catRefOther, "oracle-limit", "issue-1855/terragrunt/terragrunt.hcl:9"},
		{"issue-1892", catIncludeTgt, "gruntled-less", "issue-1892/app/terragrunt.hcl"},
		{"issue-1892/app2", catBareIncludes, "gruntled-less", "issue-1892/app2/terragrunt.hcl:6"},
		{"issue-2071", catGenerate, "gruntled-less", "issue-2071/terragrunt.hcl:11"},
		{"issue-2132/generate-var-file", catGenerate, "gruntled-less", "issue-2132/generate-var-file/terragrunt.hcl:5"},
		{"issue-2163", catIncludeTgt, "gruntled-less", "issue-2163/app/terragrunt.hcl"},
		{"issue-2171", catIncludeTgt, "gruntled-less", "issue-2171/app/mod1/terragrunt.hcl"},
		{"issue-2173/.ci", catIncludeTgt, "gruntled-less", "issue-2173/.ci/iam/terragrunt.hcl"},
		{"issue-2277", catIncludeTgt, "gruntled-less", "issue-2277/development/ec2-instance/terragrunt.hcl"},
		{"issue-2298", catIncludeTgt, "gruntled-less", "issue-2298/app/terragrunt.hcl:3"},
		{"issue-2298/app", catInclude, "gruntled-more", "issue-2298/app/terragrunt.hcl:3"},
		{"issue-2377", catIncludeTgt, "gruntled-less", "issue-2377/app/terragrunt.hcl"},
		{"issue-2377/app", catRefOther, "oracle-limit", "issue-2377/app/terragrunt.hcl:24"},
		{"issue-2389", catIncludeTgt, "gruntled-less", "issue-2389/app1/terragrunt.hcl"},
		{"issue-2459", catIncludeTgt, "gruntled-less", "issue-2459/app/terragrunt.hcl"},
		{"issue-2470/project1", catGenerate, "gruntled-less", "issue-2470/generate.hcl:26"},
		{"issue-2470/project2", catGenerate, "gruntled-less", "issue-2470/generate.hcl:26"},
		{"issue-2528/app", catGenerate, "gruntled-less", "issue-2528/app/terragrunt.hcl:5"},
		{"issue-2528/app", catRefOther, "oracle-limit", "issue-2528/app/terragrunt.hcl:23"},
		{"issue-2528/app", catRefOther, "oracle-limit", "issue-2528/app/terragrunt.hcl:24"},
		{"issue-2543/project", catIncludeTgt, "gruntled-less", "issue-2543/project/app/terragrunt.hcl"},
		{"issue-2569-2", catIncludeTgt, "gruntled-less", "issue-2569-2/environments/dev/ap-southeast-2/s3/terragrunt.hcl"},
		{"issue-2627/tfe/nonprod", catIncludeTgt, "gruntled-less", "issue-2627/tfe/nonprod/encryption/terragrunt.hcl"},
		{"issue-2714/local-state", catIncludeTgt, "gruntled-less", "issue-2714/local-state/eks/terragrunt.hcl"},
		{"issue-2714/s3-state", catIncludeTgt, "gruntled-less", "issue-2714/s3-state/eks/terragrunt.hcl"},
		{"issue-2718", catIncludeTgt, "gruntled-less", "issue-2718/app/terragrunt.hcl"},
		{"issue-2718/app", catRefOther, "oracle-limit", "issue-2718/app/terragrunt.hcl:23"},
		{"issue-2734/app1", catConfigPath, "gruntled-less", "issue-2734/app1/terragrunt.hcl:2"},
		{"issue-2748", catIncludeTgt, "gruntled-less", "issue-2748/app/terragrunt.hcl"},
		{"issue-2955/app", catRefOther, "oracle-limit", "issue-2955/app/terragrunt.hcl:11"},
		{"issue-3310-v2", catIncludeTgt, "gruntled-less", "issue-3310-v2/infra/terragrunt.hcl"},
		{"issue-3441-v2", catIncludeTgt, "gruntled-less", "issue-3441-v2/module-a/terragrunt.hcl"},
		{"issue-3441-v3", catIncludeTgt, "gruntled-less", "issue-3441-v3/module-a/terragrunt.hcl"},
		{"issue-3446", catGenerate, "gruntled-less", "issue-3446/terragrunt.hcl:9"},
		{"issue-3446-v2/department-a/rds", catGenerate, "gruntled-less", "issue-3446-v2/provider.hcl:9"},
		{"issue-3446-v2/department-a/vpc", catGenerate, "gruntled-less", "issue-3446-v2/provider.hcl:9"},
		{"issue-3446-v2/department-b/rds", catGenerate, "gruntled-less", "issue-3446-v2/provider.hcl:9"},
		{"issue-3446-v2/department-b/vpc", catGenerate, "gruntled-less", "issue-3446-v2/provider.hcl:9"},
		{"issue-3482", catIncludeTgt, "gruntled-less", "issue-3482/prod/terragrunt.hcl"},
		{"issue-3505", catIncludeTgt, "gruntled-less", "issue-3505/app/terragrunt.hcl"},
		{"issue-3590", catIncludeTgt, "gruntled-less", "issue-3590/app/terragrunt.hcl"},
		{"issue-3590/app", catOverlay, "gruntled-less", "issue-3590/app/terragrunt.hcl:6"},
		{"issue-4209/systems/account-card/prod/app", catBareIncludes, "gruntled-less", "issue-4209/systems/account-card/prod/app/terragrunt.hcl:16"},
		{"json", catIncludeTgt, "gruntled-less", "json/child/terragrunt.hcl.json"},
		{"json/child", catJSONUnit, "gruntled-more", "json/child/terragrunt.hcl.json"},
		{"locals-evaluation", catIncludeTgt, "gruntled-less", "locals-evaluation/app/app1/terragrunt.hcl"},
		{"locals-tf", catGenerate, "gruntled-less", "locals-tf/terragrunt.hcl:12"},
		{"merge-import", catIncludeTgt, "gruntled-less", "merge-import/app/terragrunt.hcl"},
		{"mock-output/module1", catRefOther, "oracle-limit", "mock-output/module1/terragrunt.hcl:22"},
		{"module-dependencies", catIncludeTgt, "gruntled-less", "module-dependencies/stack-base/mod1/terragrunt.hcl"},
		{"multiple-dependencies/app", catRefOther, "oracle-limit", "multiple-dependencies/app/terragrunt.hcl:41"},
		{"panics/07-deep-nested-quadratic-hang", catTooDeep, "gruntled-less", "panics/07-deep-nested-quadratic-hang/terragrunt.hcl"},
		{"perf-tests-v2/test/code", catIncludeTgt, "gruntled-less", "perf-tests-v2/test/code/apps/app-1/terragrunt.hcl"},
		{"perf-tests/code-v2/code", catIncludeTgt, "gruntled-less", "perf-tests/code-v2/code/apps/app-1/terragrunt.hcl"},
		{"plan-multiple-modules", catIncludeTgt, "gruntled-less", "plan-multiple-modules/app1/terragrunt.hcl"},
		{"plan-not-working", catIncludeTgt, "gruntled-less", "plan-not-working/app/terragrunt.hcl"},
		{"plan-not-working/app", catGenerate, "gruntled-less", "plan-not-working/terragrunt.hcl:1"},
		{"plan-not-working/app", catRefOther, "oracle-limit", "plan-not-working/app/terragrunt.hcl:14"},
		{"plan-not-working/dependency", catGenerate, "gruntled-less", "plan-not-working/terragrunt.hcl:1"},
		{"pr-2852", catIncludeTgt, "gruntled-less", "pr-2852/app/terragrunt.hcl"},
		{"provider-cache/nested-modules", catIncludeTgt, "gruntled-less", "provider-cache/nested-modules/terragrunt/terragrunt.hcl"},
		{"remote-state", catIncludeTgt, "gruntled-less", "remote-state/mysql/terragrunt.hcl"},
		{"reuse-locals", catIncludeTgt, "gruntled-less", "reuse-locals/envs/dev/app/terragrunt.hcl"},
		{"s3-reconfigure", catIncludeTgt, "gruntled-less", "s3-reconfigure/app/terragrunt.hcl"},
		{"scaffold/base-template", catSource, "gruntled-more", "scaffold/base-template/terragrunt.hcl:3"},
		{"scaffold/local/external-template", catSource, "gruntled-more", "scaffold/local/external-template/terragrunt.hcl:3"},
		{"scaffold/test1/.boilerplate", catConfigSyntax, "gruntled-less", "scaffold/test1/.boilerplate/terragrunt.hcl:4"},
		{"stack-autoincludes/object-computed-key-leak/units/app", catModuleSyntax, "gruntled-less", "stack-autoincludes/object-computed-key-leak/units/app/main.tf:3"},
		{"tf-lint-regeneration", catIncludeTgt, "gruntled-less", "tf-lint-regeneration/dev/apps/app-1/terragrunt.hcl"},
		{"variable-resolving", catGenerate, "gruntled-less", "variable-resolving/terragrunt.hcl:5"},
		{"versions-race-conditions", catIncludeTgt, "gruntled-less", "versions-race-conditions/dep-template/terragrunt.hcl"},
	},
}
