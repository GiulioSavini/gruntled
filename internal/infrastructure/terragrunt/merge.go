package terragrunt

import (
	"io/fs"
	"regexp"
	"sort"
	"strings"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/zclconf/go-cty/cty"

	"github.com/GiulioSavini/gruntled/internal/domain/repograph"
)

// resolvedInclude is one of a unit's include blocks, resolved to the file it
// points at: the file has been read, parsed, and proven not to itself
// contain an include block (research Pitfall 3). It carries the include's
// own merge_strategy facts, but merge EXCLUSION for no_merge happens later,
// in buildEffectiveFiles: path_relative_to_include and its siblings must
// still see every include, no_merge or not (research Pattern 4).
type resolvedInclude struct {
	// label is the include block's label, "" for a bare include.
	label string
	// dir is the repo-relative directory the included file lives in.
	dir string
	// pf is the included file's parsed facts.
	pf *parsedFile
	// noMerge is true when merge_strategy == "no_merge": this include
	// contributes nothing to the unit's effective source, dependencies,
	// generate blocks or references, though it still participates in
	// path_relative_to_include's directory bookkeeping.
	noMerge bool
	// deep is true when merge_strategy == "deep".
	deep bool
}

// validateIncludeDecls checks every structural rule an include block set
// must satisfy BEFORE any path is evaluated: at most one label per block, no
// duplicate labels, at most one bare (unlabeled) include, a path attribute
// present, and a literal merge_strategy in {"", "shallow", "deep",
// "no_merge"} when the attribute is present at all ("deep_map_only", which
// Terragrunt accepts for dependency merge but rejects for includes, is
// invalid here). Returns "" when every decl is valid, or the single
// ReasonInvalidInclude-class reason (always ReasonInvalidInclude itself) to
// use otherwise.
func validateIncludeDecls(decls []includeDecl) string {
	seenLabels := map[string]bool{}
	bareCount := 0
	for _, d := range decls {
		switch len(d.labels) {
		case 0:
			bareCount++
			if bareCount > 1 {
				return ReasonInvalidInclude
			}
		case 1:
			if seenLabels[d.labels[0]] {
				return ReasonInvalidInclude
			}
			seenLabels[d.labels[0]] = true
		default:
			return ReasonInvalidInclude
		}
		if d.path == nil {
			return ReasonInvalidInclude
		}
		if d.mergeStrategy != nil {
			s, ok := literalString(d.mergeStrategy)
			if !ok {
				return ReasonInvalidInclude
			}
			switch s {
			case "", "shallow", "deep", "no_merge":
			default:
				return ReasonInvalidInclude
			}
		}
	}
	return ""
}

// mergeStrategyOf reads decl's already-validated merge_strategy literal
// (validateIncludeDecls has already proven it is "", "shallow", "deep" or
// "no_merge", or absent) into its noMerge/deep booleans.
func mergeStrategyOf(decl includeDecl) (noMerge, deep bool) {
	if decl.mergeStrategy == nil {
		return false, false
	}
	s, _ := literalString(decl.mergeStrategy)
	switch s {
	case "no_merge":
		return true, false
	case "deep":
		return false, true
	default:
		return false, false
	}
}

// toIncludeRefs projects every resolved include (no_merge included) into the
// includeRef slice the six path functions need for path_relative_to_include
// and its siblings, in declaration order.
func toIncludeRefs(resolved []resolvedInclude) []includeRef {
	refs := make([]includeRef, len(resolved))
	for i, r := range resolved {
		refs[i] = includeRef{label: r.label, dir: r.dir}
	}
	return refs
}

// effectiveFile is one file contributing to a unit's effective
// configuration, in precedence order (highest first): the child's own
// terragrunt.hcl, then every non-no_merge include from last-declared to
// first-declared (research Pattern 6: child > last include > ... > first
// include).
type effectiveFile struct {
	pf   *parsedFile
	dir  string
	kind scopeKind
	// deep is only meaningful when kind == scopeIncluded: whether this
	// include's merge_strategy is "deep".
	deep bool
}

// buildEffectiveFiles assembles a unit's effective file list per research
// Pattern 6's precedence, dropping no_merge includes entirely: they
// contribute no source, no dependencies, no generate blocks and no
// references.
func buildEffectiveFiles(childPF *parsedFile, unitDir string, resolved []resolvedInclude) []effectiveFile {
	files := []effectiveFile{{pf: childPF, dir: unitDir, kind: scopeUnit}}
	for i := len(resolved) - 1; i >= 0; i-- {
		r := resolved[i]
		if r.noMerge {
			continue
		}
		files = append(files, effectiveFile{pf: r.pf, dir: r.dir, kind: scopeIncluded, deep: r.deep})
	}
	return files
}

// validateEffectiveFile checks one effective file's own terraform,
// dependency and generate blocks: more than one terraform block; a
// dependency block with a label count other than one, an expansion block,
// or a label duplicated within this single file; or a generate block with
// a label count other than one, or a label duplicated within this single
// file (G9), are each structurally invalid, independent of merging. A
// generate label repeated ACROSS files (child vs. an include) is a valid
// merge -- Terragrunt selects by precedence, highest wins -- and is left to
// mergeGenerateUnknownReason.
func validateEffectiveFile(ef effectiveFile) string {
	if len(ef.pf.terraforms) > 1 {
		return ReasonInvalidTerraformBlock
	}
	seenDeps := map[string]bool{}
	for _, d := range ef.pf.deps {
		if len(d.labels) != 1 || d.hasExpansion {
			return ReasonInvalidDependency
		}
		if seenDeps[d.labels[0]] {
			return ReasonInvalidDependency
		}
		seenDeps[d.labels[0]] = true
	}
	seenGenerate := map[string]bool{}
	for _, g := range ef.pf.generates {
		if len(g.labels) != 1 {
			return ReasonInvalidGenerate
		}
		if seenGenerate[g.labels[0]] {
			return ReasonInvalidGenerate
		}
		seenGenerate[g.labels[0]] = true
	}
	return ""
}

// fileScope builds the evalScope a path-bearing attribute written in ef
// should be evaluated with: scopeUnit (S1, with the child's own includes)
// when ef is the child itself, or scopeIncluded (S2, with included set to
// ef's own directory) when ef is a merged include. unitDir is always the
// CHILD unit's directory, per research Pitfall 4: every relative path and
// get_terragrunt_dir/find_in_parent_folders resolves against the child,
// regardless of which file the expression is written in.
func fileScope(fsys fs.FS, unitDir string, childRefs []includeRef, ef effectiveFile) evalScope {
	if ef.kind == scopeIncluded {
		return evalScope{fsys: fsys, unitDir: unitDir, kind: scopeIncluded, included: ef.dir}
	}
	return evalScope{fsys: fsys, unitDir: unitDir, kind: scopeUnit, includes: childRefs}
}

// depOccurrence is one file's depDecl for a given dependency label, paired
// with the effectiveFile it came from.
type depOccurrence struct {
	ef   effectiveFile
	decl depDecl
}

// collectDependencyLabels groups every effective file's dependency
// declarations by label, in files' precedence order (files is already
// child-then-includes-last-to-first), so occurrences[0] is always the
// highest-precedence declaration of that label.
func collectDependencyLabels(files []effectiveFile) map[string][]depOccurrence {
	byLabel := map[string][]depOccurrence{}
	for _, ef := range files {
		for _, d := range ef.pf.deps {
			label := d.labels[0]
			byLabel[label] = append(byLabel[label], depOccurrence{ef: ef, decl: d})
		}
	}
	return byLabel
}

// mergeReferences concatenates every effective file's references (no_merge
// includes are already excluded from files) and sorts the result by
// Position, then Dependency, then Output -- matching the tie-break the
// domain's own Unit constructors apply, so ports.UnitConfig.References is
// already in canonical order before assembly.
func mergeReferences(files []effectiveFile) []repograph.Reference {
	var refs []repograph.Reference
	for _, ef := range files {
		refs = append(refs, ef.pf.refs...)
	}
	sortRefs(refs)
	return refs
}

// sortRefs sorts refs in place by Position, then Dependency, then Output --
// the same tie-break the domain's own Unit constructors apply, so
// ports.UnitConfig.References is already canonical before assembly.
func sortRefs(refs []repograph.Reference) {
	sort.SliceStable(refs, func(i, j int) bool {
		a, b := refs[i], refs[j]
		if c := a.Pos().Compare(b.Pos()); c != 0 {
			return c < 0
		}
		if a.Dependency() != b.Dependency() {
			return a.Dependency() < b.Dependency()
		}
		return a.Output() < b.Output()
	})
}

// generateMayDeclareOutputs decides research Pattern 7's
// generate-may-declare-outputs rule for one generate block's contents
// expression: nil contents, contents that are not a plain string template
// (a function call such as file(...) or templatefile(...), or a bare
// reference), or a template containing a non-literal part (an
// interpolation) all count as "may declare outputs" -- gruntled never
// evaluates the content to find out. A template whose parts are ALL string
// literals (a heredoc with only literal text qualifies) is scanned for the
// literal `output` block/attribute shape; only a positive match there
// counts.
func generateMayDeclareOutputs(contents hcl.Expression) bool {
	if contents == nil {
		return true
	}
	tmpl, ok := contents.(*hclsyntax.TemplateExpr)
	if !ok {
		return true
	}
	var sb strings.Builder
	for _, part := range tmpl.Parts {
		lit, ok := part.(*hclsyntax.LiteralValueExpr)
		if !ok {
			return true
		}
		if !lit.Val.Type().Equals(cty.String) {
			return true
		}
		sb.WriteString(lit.Val.AsString())
	}
	return generateOutputPatternRE.MatchString(sb.String())
}

// generateOutputPatternRE matches a literal `output` block header
// ("output ..." at the start of a line, HCL syntax) or a JSON-style
// "output": key, inside a generate block's literal contents.
var generateOutputPatternRE = regexp.MustCompile(`(?m)^\s*output\b|"output"\s*:`)

// mergeGenerateUnknownReason walks files in precedence order and returns
// ReasonGenerateMayDeclareOutputs the first time an effective generate
// block (merged by label, first-in-precedence wins per label) fails
// generateMayDeclareOutputs, or "" if every effective generate block is
// provably free of an output declaration (including when there are none at
// all).
func mergeGenerateUnknownReason(files []effectiveFile) string {
	seen := map[string]bool{}
	for _, ef := range files {
		for _, g := range ef.pf.generates {
			if len(g.labels) != 1 {
				// Defensive: validateEffectiveFile (G9) already rejects any
				// generate block whose label count isn't exactly one before
				// this function ever runs, so this is unreachable in
				// practice. Kept so a future change to that ordering fails
				// closed here (the module becomes unknown) instead of
				// panicking on g.labels[0] below or skipping the block.
				return ReasonInvalidGenerate
			}
			label := g.labels[0]
			if seen[label] {
				continue
			}
			seen[label] = true
			if generateMayDeclareOutputs(g.contents) {
				return ReasonGenerateMayDeclareOutputs
			}
		}
	}
	return ""
}

// unitDirOverlaysModule reports whether unitDir directly holds a
// non-ignored .tf/.tf.json/.tofu/.tofu.json file: Terragrunt copies the
// unit directory's own files over the module's working copy
// (CopyFolderContents), so such a unit's effective module surface is not
// the module directory's surface alone (research Pitfall 7). A ReadDir
// failure means whether it overlays is itself unknown (G6): the caller
// must treat the module as unknown rather than assume it does not overlay,
// so reason is ReasonModuleFileUnreadable in that case, "" otherwise
// (whether or not it overlays -- the bool return still answers that).
func unitDirOverlaysModule(fsys fs.FS, unitDir string) (overlays bool, reason string) {
	entries, err := fs.ReadDir(fsys, unitDir)
	if err != nil {
		return false, ReasonModuleFileUnreadable
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if isIgnoredFileName(name) {
			continue
		}
		if _, ok := moduleExt(name); ok {
			return true, ""
		}
	}
	return false, ""
}

// isIgnoredFileName reports whether name is a Terraform-ignored file:
// prefix ".", suffix "~", or both prefix and suffix "#". Mirrors
// tfsurface's own filter, kept as a small, independent copy since this
// package must not import an infrastructure sibling for a four-line check.
func isIgnoredFileName(name string) bool {
	if strings.HasPrefix(name, ".") {
		return true
	}
	if strings.HasSuffix(name, "~") {
		return true
	}
	if strings.HasPrefix(name, "#") && strings.HasSuffix(name, "#") {
		return true
	}
	return false
}

// moduleExt returns name's module file extension category, checked
// most-specific first so ".tf.json"/".tofu.json" are never mistaken for
// plain ".tf"/".tofu".
func moduleExt(name string) (ext string, ok bool) {
	switch {
	case strings.HasSuffix(name, ".tf.json"):
		return ".tf.json", true
	case strings.HasSuffix(name, ".tofu.json"):
		return ".tofu.json", true
	case strings.HasSuffix(name, ".tf"):
		return ".tf", true
	case strings.HasSuffix(name, ".tofu"):
		return ".tofu", true
	}
	return "", false
}
