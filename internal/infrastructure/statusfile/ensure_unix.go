//go:build !windows

package statusfile

import (
	"fmt"
	"io/fs"
	"os"
	"syscall"
)

// geteuid is a variable so tests can simulate a foreign owner without root.
var geteuid = os.Geteuid

func checkOwner(dir string, fi fs.FileInfo) error {
	st, ok := fi.Sys().(*syscall.Stat_t)
	if !ok {
		return fmt.Errorf("%w: %s: owner unknown", ErrInsecureDir, dir)
	}
	if st.Uid != uint32(geteuid()) {
		return fmt.Errorf("%w: %s: owned by uid %d", ErrInsecureDir, dir, st.Uid)
	}
	return nil
}
