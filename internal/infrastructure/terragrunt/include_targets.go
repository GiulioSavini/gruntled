package terragrunt

import (
	"io/fs"
	"path"
	"strings"
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
// ReasonIncludeTarget).
type includeTargets struct {
	// exact holds every include file path (lexical and canonical) that
	// some unit's include resolved to an existing regular in-repo file.
	exact map[string]bool
}

// newIncludeTargets returns an empty accumulator.
func newIncludeTargets() *includeTargets {
	return &includeTargets{exact: map[string]bool{}}
}

// markExact records paths as include targets.
func (t *includeTargets) markExact(paths ...string) {
	for _, p := range paths {
		t.exact[p] = true
	}
}

// isTarget reports whether unitDir's own terragrunt.hcl is an include
// target.
func (t *includeTargets) isTarget(unitDir string) bool {
	return t.exact[path.Join(unitDir, "terragrunt.hcl")]
}
