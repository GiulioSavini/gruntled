package terragrunt

import (
	"io/fs"
	"path"
	"strings"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
)

// canonErr is canonicalPath's outcome.
type canonErr int

const (
	// canonOK means the path resolved to a symlink-free in-repo path.
	canonOK canonErr = iota
	// canonOutside means a symlink on the path leads outside the
	// repository (an absolute target, or a relative one climbing above
	// the root).
	canonOutside
	// canonUnresolvable means the path's identity could not be proven: a
	// link loop (more than maxSymlinkFollows links), a backslash in a link
	// target, an Lstat/ReadLink error, or an fs.FS without fs.ReadLinkFS.
	canonUnresolvable
)

// maxSymlinkFollows caps the links canonicalPath follows for one path,
// like Linux's MAXSYMLINKS: past it the path is treated as a loop.
const maxSymlinkFollows = 40

// canonicalPath returns the symlink-free, repo-relative path of the file p
// (a clean repo-relative path, as resolvePath returns it) names in fsys.
//
// Include targets must be compared by this canonical identity, not by the
// lexical path the include wrote: an include of "../../link/terragrunt.hcl"
// where live/link is a symlink to "parent" reads live/parent/terragrunt.hcl,
// and a lexical comparison never marks live/parent as a parent config, so
// it is analysed standalone against the wrong directory (the
// 02-VERIFICATION gap, 02-REVIEW G15). Unit paths themselves never need
// this: discoverUnits never enters a symlinked directory, so every unit
// directory is already canonical.
//
// It walks p one segment at a time on top of a symlink-free prefix. A
// non-link segment extends the prefix; a link is replaced by its target
// joined with the link's parent directory, and the walk restarts on the
// result followed by the remaining segments. Every doubt fails closed: an
// absolute target or one climbing above the root is canonOutside; a
// backslash target (whose meaning is platform-dependent), any Lstat or
// ReadLink error, more than maxSymlinkFollows links, or an fs.FS that does
// not implement fs.ReadLinkFS (so links cannot be seen at all) is
// canonUnresolvable. The caller then makes the including unit
// config-unknown instead of resolving against a guessed path.
func canonicalPath(fsys fs.FS, p string) (string, canonErr) {
	rl, ok := fsys.(fs.ReadLinkFS)
	if !ok {
		return "", canonUnresolvable
	}

	remaining := splitPath(p)
	resolved := "."
	follows := 0
	for len(remaining) > 0 {
		seg := remaining[0]
		remaining = remaining[1:]
		next := path.Join(resolved, seg)

		info, err := rl.Lstat(next)
		if err != nil {
			return "", canonUnresolvable
		}
		if info.Mode()&fs.ModeSymlink == 0 {
			resolved = next
			continue
		}

		follows++
		if follows > maxSymlinkFollows {
			return "", canonUnresolvable
		}
		target, err := rl.ReadLink(next)
		if err != nil {
			return "", canonUnresolvable
		}
		if strings.Contains(target, `\`) {
			return "", canonUnresolvable
		}
		if path.IsAbs(target) {
			return "", canonOutside
		}
		joined := path.Join(resolved, target)
		if joined == ".." || strings.HasPrefix(joined, "../") {
			return "", canonOutside
		}
		remaining = append(splitPath(joined), remaining...)
		resolved = "."
	}
	return resolved, canonOK
}

// splitPath splits a clean repo-relative path into its segments ("."
// yields none).
func splitPath(p string) []string {
	if p == "." || p == "" {
		return nil
	}
	return strings.Split(p, "/")
}

// includeTargets accumulates, across one LoadUnits call, which unit
// directories are parent configs of some other unit (LoadUnits step 13,
// ReasonIncludeTarget). A unit is a target under any of three rules:
//
//  1. exact: some unit's include resolved, by canonical path, to its
//     terragrunt.hcl;
//  2. ancestor: it is a strict ancestor directory of a unit whose includes
//     are unknowable or failed, since find_in_parent_folders reaches every
//     ancestor;
//  3. include-free: some include's path is dynamic and its file name is
//     unknown or "terragrunt.hcl", so any unit with no include of its own
//     could be the parent.
//
// Every rule only ever fails toward unknown; see the catalogue for the
// residuals.
type includeTargets struct {
	// exact holds every include file path (lexical and canonical) that
	// some unit's include resolved to an existing regular in-repo file.
	exact map[string]bool
	// ancestors holds every strict ancestor directory of a unit whose
	// includes are unknowable or failed.
	ancestors map[string]bool
	// anyIncludeFree is set once some include's target file name is
	// dynamic or "terragrunt.hcl".
	anyIncludeFree bool
	// includeFree holds every unit whose own terragrunt.hcl parsed with
	// zero include blocks.
	includeFree map[string]bool
}

// newIncludeTargets returns an empty accumulator.
func newIncludeTargets() *includeTargets {
	return &includeTargets{
		exact:       map[string]bool{},
		ancestors:   map[string]bool{},
		includeFree: map[string]bool{},
	}
}

// markExact records paths as include targets.
func (t *includeTargets) markExact(paths ...string) {
	for _, p := range paths {
		t.exact[p] = true
	}
}

// markAncestors records every strict ancestor directory of unitDir, up to
// and including ".", as a target.
func (t *includeTargets) markAncestors(unitDir string) {
	for d := unitDir; d != "."; {
		d = path.Dir(d)
		t.ancestors[d] = true
	}
}

// markAllIncludeFree makes every include-free unit a target.
func (t *includeTargets) markAllIncludeFree() {
	t.anyIncludeFree = true
}

// noteIncludeFree records that unitDir's own terragrunt.hcl has no include
// block.
func (t *includeTargets) noteIncludeFree(unitDir string) {
	t.includeFree[unitDir] = true
}

// isTarget reports whether unitDir's own terragrunt.hcl is an include
// target under any of the three rules.
func (t *includeTargets) isTarget(unitDir string) bool {
	return t.exact[path.Join(unitDir, "terragrunt.hcl")] ||
		t.ancestors[unitDir] ||
		(t.anyIncludeFree && t.includeFree[unitDir])
}

// dynamicIncludeFileNames returns the file names an include path
// expression can end in without evaluating it, and ok=false when the name
// itself is not fixed.
//
// Only the file name matters. Units are exactly the files named
// terragrunt.hcl, and a dynamic prefix can point anywhere in the repo, so a
// fixed directory suffix proves nothing (a symlink can alias it). A fixed
// file name other than terragrunt.hcl, on the other hand, can reach a unit
// only through a symlinked file, which is the documented residual.
//
// A literal string, or a template made only of literal strings, gives the
// base name of its text. A template whose LAST part is a literal containing
// "/" gives the text after the last "/", which must be non-empty. A
// conditional gives the union of both branches, and is ok only if both are.
// Anything else is not ok.
func dynamicIncludeFileNames(expr hcl.Expression) ([]string, bool) {
	switch e := expr.(type) {
	case *hclsyntax.LiteralValueExpr:
		text, ok := literalString(e)
		if !ok {
			return nil, false
		}
		return nonEmptyBase(text)
	case *hclsyntax.TemplateExpr:
		if len(e.Parts) == 0 {
			return nil, false
		}
		if text, ok := literalString(e); ok {
			return nonEmptyBase(text)
		}
		last, ok := literalString(e.Parts[len(e.Parts)-1])
		if !ok {
			return nil, false
		}
		i := strings.LastIndex(last, "/")
		if i < 0 || i == len(last)-1 {
			return nil, false
		}
		return []string{last[i+1:]}, true
	case *hclsyntax.ConditionalExpr:
		a, okA := dynamicIncludeFileNames(e.TrueResult)
		b, okB := dynamicIncludeFileNames(e.FalseResult)
		if !okA || !okB {
			return nil, false
		}
		return append(a, b...), true
	}
	return nil, false
}

// includeReachesNoUnit reports whether an include path that failed to
// evaluate provably names no in-repo unit, in one of two ways:
//
//   - Terragrunt itself rejects it: it references a variable other than
//     values outside any try or can call. Terragrunt 1.1.6 decodes include
//     blocks before locals, so local, include, feature and dependency are
//     all "Unknown variable" there and the unit fails to parse.
//   - It is find_in_parent_folders with no argument or one literal file
//     name without "/". Such a call can only fail by probing every in-repo
//     ancestor without a match, and every ancestor above the root yields a
//     path outside the repository.
func includeReachesNoUnit(expr hcl.Expression) bool {
	if w, ok := expr.(*hclsyntax.TemplateWrapExpr); ok {
		expr = w.Wrapped
	}
	return rejectedByTerragrunt(expr) || findInParentFoldersMiss(expr)
}

// rejectedByTerragrunt reports whether expr references a variable other
// than values with no try or can call that could catch the error.
func rejectedByTerragrunt(expr hcl.Expression) bool {
	syn, ok := expr.(hclsyntax.Expression)
	if !ok {
		return false
	}
	catches := false
	hclsyntax.VisitAll(syn, func(n hclsyntax.Node) hcl.Diagnostics {
		if c, ok := n.(*hclsyntax.FunctionCallExpr); ok && (c.Name == "try" || c.Name == "can") {
			catches = true
		}
		return nil
	})
	if catches {
		return false
	}
	for _, v := range expr.Variables() {
		if v.RootName() != "values" {
			return true
		}
	}
	return false
}

// findInParentFoldersMiss reports whether expr is a find_in_parent_folders
// call with no argument, or with one literal file name without "/".
func findInParentFoldersMiss(expr hcl.Expression) bool {
	c, ok := expr.(*hclsyntax.FunctionCallExpr)
	if !ok || c.Name != "find_in_parent_folders" || c.ExpandFinal || len(c.Args) > 1 {
		return false
	}
	if len(c.Args) == 0 {
		return true
	}
	name, ok := literalString(c.Args[0])
	return ok && !strings.Contains(name, "/")
}

// nonEmptyBase returns the base name of p, or ok=false when p names no file.
func nonEmptyBase(p string) ([]string, bool) {
	if p == "" || strings.HasSuffix(p, "/") {
		return nil, false
	}
	return []string{path.Base(p)}, true
}
