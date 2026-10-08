//go:build windows

package ipc

import (
	"os"
	"syscall"
)

// errSharingViolation is ERROR_SHARING_VIOLATION: someone else has the
// file open with share mode 0.
const errSharingViolation = syscall.Errno(32)

func openExclusive(path string, disposition uint32) (*os.File, error) {
	p, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return nil, err
	}
	h, err := syscall.CreateFile(p, syscall.GENERIC_READ|syscall.GENERIC_WRITE, 0, nil, disposition, syscall.FILE_ATTRIBUTE_NORMAL, 0)
	if err != nil {
		return nil, err
	}
	return os.NewFile(uintptr(h), path), nil
}

// TryLock opens path exclusively (share mode 0), creating it if needed.
// The OS closes the handle when the process dies.
func TryLock(path string) (*Lock, error) {
	f, err := openExclusive(path, syscall.OPEN_ALWAYS)
	if err == errSharingViolation {
		return nil, ErrHeld
	}
	if err != nil {
		return nil, err
	}
	return &Lock{f: f}, nil
}

// Probe reports whether another holder owns the lock. An absent file means
// no holder and is not created; a free lock is opened and closed at once.
func Probe(path string) (bool, error) {
	f, err := openExclusive(path, syscall.OPEN_EXISTING)
	switch err {
	case nil:
		return false, f.Close()
	case errSharingViolation:
		return true, nil
	case syscall.ERROR_FILE_NOT_FOUND, syscall.ERROR_PATH_NOT_FOUND:
		return false, nil
	}
	return false, err
}
