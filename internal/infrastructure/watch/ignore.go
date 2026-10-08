package watch

import "strings"

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

// Ignored reports whether the repo-relative, slash-separated path rel must
// never reach the dirty set: any component is an ignored directory, or the
// base name is an editor swap/backup/probe file.
func Ignored(rel string) bool {
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
	return ignoredBase(base)
}

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
