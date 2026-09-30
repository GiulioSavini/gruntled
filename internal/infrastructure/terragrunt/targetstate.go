package terragrunt

import (
	"errors"
	"io/fs"
	"path"

	"github.com/GiulioSavini/gruntled/internal/domain/repograph"
)

// unitConfigNames are the file names that make a directory a unit, compared
// case-sensitively like Terragrunt on a case-sensitive filesystem.
var unitConfigNames = [...]string{"terragrunt.hcl", "terragrunt.hcl.json"}

// classifyTarget reports what is on disk at a resolved dependency target
// dir (repo-relative, slash-separated, "." for the repo root). Only a
// not-exist error yields TargetDirMissing; any other read error (permission,
// not a directory, a symlink escaping the root) yields TargetUnknown so the
// caller never reports a missing target it could not actually observe.
//
// Names are matched exactly from the directory listing, never by probing,
// so a case-insensitive filesystem cannot make "Terragrunt.hcl" count. A
// matching entry is followed with fs.Stat: a dangling symlink counts as
// absent, a directory named like the file does not count, and any other
// stat error is TargetUnknown.
func (l *Loader) classifyTarget(dir string) repograph.TargetState {
	entries, err := fs.ReadDir(l.fsys, dir)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return repograph.TargetDirMissing
		}
		return repograph.TargetUnknown
	}
	for _, e := range entries {
		name := e.Name()
		if name != unitConfigNames[0] && name != unitConfigNames[1] {
			continue
		}
		info, statErr := fs.Stat(l.fsys, path.Join(dir, name))
		switch {
		case statErr == nil:
			if !info.IsDir() {
				return repograph.TargetHasConfig
			}
		case errors.Is(statErr, fs.ErrNotExist):
			// Dangling symlink: absent, keep looking.
		default:
			return repograph.TargetUnknown
		}
	}
	return repograph.TargetNoConfig
}
