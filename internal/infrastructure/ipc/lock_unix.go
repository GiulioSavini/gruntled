//go:build linux || darwin

package ipc

import (
	"errors"
	"io/fs"
	"os"
	"syscall"
)

// TryLock takes a non-blocking exclusive flock on path, creating it 0600.
// It never follows a symlink at path.
func TryLock(path string) (*Lock, error) {
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE|syscall.O_NOFOLLOW, 0o600)
	if err != nil {
		return nil, err
	}
	if err := flock(f); err != nil {
		_ = f.Close()
		return nil, err
	}
	return &Lock{f: f}, nil
}

// Probe reports whether another holder owns the lock. An absent file means
// no holder and is not created; a free lock is taken and released at once.
func Probe(path string) (bool, error) {
	f, err := os.OpenFile(path, os.O_RDWR|syscall.O_NOFOLLOW, 0)
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	defer f.Close()
	err = flock(f)
	if errors.Is(err, ErrHeld) {
		return true, nil
	}
	return false, err
}

// flock locks f exclusively without blocking. It goes through SyscallConn
// so the descriptor keeps its mode.
func flock(f *os.File) error {
	rc, err := f.SyscallConn()
	if err != nil {
		return err
	}
	var ferr error
	if err := rc.Control(func(fd uintptr) {
		for {
			ferr = syscall.Flock(int(fd), syscall.LOCK_EX|syscall.LOCK_NB)
			if ferr != syscall.EINTR {
				return
			}
		}
	}); err != nil {
		return err
	}
	if ferr == syscall.EWOULDBLOCK || ferr == syscall.EAGAIN {
		return ErrHeld
	}
	return ferr
}
