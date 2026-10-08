//go:build windows

package statusfile

import "io/fs"

// checkOwner is a no-op on windows: the per-user base directory's ACL is
// the boundary there.
func checkOwner(string, fs.FileInfo) error { return nil }
