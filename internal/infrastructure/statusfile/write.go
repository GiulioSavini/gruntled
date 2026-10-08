package statusfile

import (
	"errors"
	"os"
	"path/filepath"
	"time"
)

// ErrInsecureDir is returned when a runtime directory is a symlink, not a
// directory, accessible by group or others, or owned by another user.
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
// It applies EnsureDir to the directory (0700, no symlink, not group or
// other accessible, owned by the caller on unix; the parent is not
// checked, so a custom path under /tmp works), writes a 0600 temp file in the same directory and renames it over Path.
func (w *Writer) Write(line string) error {
	dir := filepath.Dir(w.Path)
	if err := EnsureDir(dir); err != nil {
		return err
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
