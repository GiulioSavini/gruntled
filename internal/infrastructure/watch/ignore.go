package watch

import (
	"io/fs"
	"strings"
)

// ignoredDirs are directory names whose whole subtree never reaches the dirty
// set. Compatible with terragrunt's skipDirNames except "vendor": a unit's
// source may point into vendor, so watching it is correct.
var ignoredDirs = map[string]bool{
	".git":              true,
	".terraform":        true,
	".terragrunt-cache": true,
}

// IgnoredDir reports whether a directory with this exact base name must not
// be walked or watched.
func IgnoredDir(name string) bool { return ignoredDirs[name] }

// IgnoredEntry reports whether the repo-relative, slash-separated path rel
// must never reach the dirty set. typ is the entry's type bits
// (fi.Mode().Type(), d.Type()): fs.ModeDir, fs.ModeSymlink, ..., or 0 for a
// regular file or an entry whose type is unknown because it is gone.
//
//   - any component equal to .git, .terraform or .terragrunt-cache: ignored,
//     whatever typ is;
//   - a directory (typ&fs.ModeDir != 0) is never pattern-ignored, so a
//     directory named 2024, x.tmp or bak~ is walked, watched and reindexed;
//   - a symlink (typ&fs.ModeSymlink != 0) is pattern-ignored only by the
//     Emacs lock rule: base name starting with ".#" (Emacs creates ".#name"
//     as a dangling symlink to "user@host.pid:boot"); every other pattern is
//     symlink-exempt, so a link named 2024 to a directory still triggers;
//   - anything else (regular file, other types, 0 = gone/unknown) is ignored
//     when its base name is an editor swap/backup/probe file.
func IgnoredEntry(rel string, typ fs.FileMode) bool {
	base := rel
	for {
		i := strings.IndexByte(rel, '/')
		comp := rel
		if i >= 0 {
			comp = rel[:i]
		}
		if ignoredDirs[comp] {
			return true
		}
		if i < 0 {
			base = comp
			break
		}
		rel = rel[i+1:]
	}
	switch {
	case typ&fs.ModeDir != 0:
		return false
	case typ&fs.ModeSymlink != 0:
		return strings.HasPrefix(base, ".#")
	}
	return ignoredBase(base)
}

// editorPattern reports whether the base name of rel matches an editor
// swap/backup/probe pattern, regardless of the entry type. The native
// adapter uses it to decide when an event path needs an Lstat to learn its
// type: normal names never pay that syscall.
func editorPattern(rel string) bool {
	if i := strings.LastIndexByte(rel, '/'); i >= 0 {
		rel = rel[i+1:]
	}
	return ignoredBase(rel)
}

// ignoredBase holds the editor FILE patterns. It is type-blind: callers go
// through IgnoredEntry, which decides which entry types it applies to.
func ignoredBase(b string) bool {
	switch {
	case b == "":
		return false
	case strings.HasSuffix(b, "~"):
		return true
	case strings.HasSuffix(b, ".swp"), strings.HasSuffix(b, ".swo"),
		strings.HasSuffix(b, ".swn"), strings.HasSuffix(b, ".swx"):
		return true
	case strings.HasPrefix(b, ".#"):
		return true
	case len(b) >= 2 && b[0] == '#' && b[len(b)-1] == '#':
		return true
	case b == "___jb_tmp___", b == "___jb_old___":
		return true
	case strings.HasSuffix(b, ".tmp"):
		return true
	case b == ".DS_Store":
		return true
	}
	return isVimProbe(b)
}

// isVimProbe matches vim's write-permission probe file ("4913" and its
// numbered successors): an all-digit name of length 4 or 5.
func isVimProbe(b string) bool {
	if len(b) < 4 || len(b) > 5 {
		return false
	}
	for i := 0; i < len(b); i++ {
		if b[i] < '0' || b[i] > '9' {
			return false
		}
	}
	return true
}
