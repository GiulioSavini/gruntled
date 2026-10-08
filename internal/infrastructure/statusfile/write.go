package statusfile

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"time"
)

// ErrInsecureDir is returned when the status directory already exists and
// group or other users can access it.
var ErrInsecureDir = errors.New("statusfile: directory is accessible by other users")

// renameAttempts and renameBackoff bound the rename retry. On windows a
// reader holding the status file open makes the replace fail with a
// sharing violation; a few short retries ride it out.
const renameAttempts = 5

var renameBackoff = [renameAttempts - 1]time.Duration{
	10 * time.Millisecond,
	20 * time.Millisecond,
	40 * time.Millisecond,
	80 * time.Millisecond,
}

// Writer replaces the status file atomically: a reader sees either the
// previous line or the new one, never a partial write. It never logs; the
// caller decides what a failed write means.
type Writer struct {
	Path   string
	Rename func(oldpath, newpath string) error // default os.Rename
	Sleep  func(time.Duration)                 // default time.Sleep
}

// NewWriter returns a Writer for path with the real rename and sleep.
func NewWriter(path string) *Writer {
	return &Writer{Path: path, Rename: os.Rename, Sleep: time.Sleep}
}

// Write writes line verbatim (the caller includes the trailing newline).
// It creates the directory with mode 0700, refuses a pre-existing
// directory accessible by group or others (not checked on windows),
// writes a 0600 temp file in the same directory and renames it over Path.
func (w *Writer) Write(line string) error {
	dir := filepath.Dir(w.Path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	if runtime.GOOS != "windows" {
		fi, err := os.Stat(dir)
		if err != nil {
			return err
		}
		if fi.Mode().Perm()&0o077 != 0 {
			return ErrInsecureDir
		}
	}

	tmp, err := os.CreateTemp(dir, ".status-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	if _, err := tmp.WriteString(line); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
		return err
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpName)
		return err
	}

	rename := w.Rename
	if rename == nil {
		rename = os.Rename
	}
	sleep := w.Sleep
	if sleep == nil {
		sleep = time.Sleep
	}
	for i := range renameAttempts {
		if err = rename(tmpName, w.Path); err == nil {
			return nil
		}
		if i < len(renameBackoff) {
			sleep(renameBackoff[i])
		}
	}
	_ = os.Remove(tmpName)
	return err
}
