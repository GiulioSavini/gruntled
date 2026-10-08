package ipc

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

func lockPlatform(t *testing.T) {
	t.Helper()
	switch runtime.GOOS {
	case "linux", "darwin", "windows":
	default:
		t.Skip("lock is a no-op on " + runtime.GOOS)
	}
}

func TestTryLockExclusive(t *testing.T) {
	lockPlatform(t)
	p := filepath.Join(shortDir(t), LockName)
	a, err := TryLock(p)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := TryLock(p); !errors.Is(err, ErrHeld) {
		t.Fatalf("second TryLock: err = %v, want ErrHeld", err)
	}
	if runtime.GOOS != "windows" {
		fi, err := os.Stat(p)
		if err != nil {
			t.Fatal(err)
		}
		if fi.Mode().Perm() != 0o600 {
			t.Fatalf("lock mode = %v", fi.Mode().Perm())
		}
	}
	if err := a.Release(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(p); err != nil {
		t.Fatalf("Release removed the lock file: %v", err)
	}
	b, err := TryLock(p)
	if err != nil {
		t.Fatalf("TryLock after Release: %v", err)
	}
	if err := b.Release(); err != nil {
		t.Fatal(err)
	}
	if err := b.Release(); err != nil {
		t.Fatalf("second Release: %v", err)
	}
}

type sleepStub struct {
	calls []time.Duration
	at    int    // call number that runs hook
	hook  func() // e.g. release the holder
}

func (s *sleepStub) sleep(d time.Duration) {
	s.calls = append(s.calls, d)
	if s.hook != nil && len(s.calls) == s.at {
		s.hook()
	}
}

func TestLockRetryHeld(t *testing.T) {
	lockPlatform(t)
	p := filepath.Join(shortDir(t), LockName)
	holder, err := TryLock(p)
	if err != nil {
		t.Fatal(err)
	}
	defer holder.Release()

	s := &sleepStub{}
	if _, err := LockRetry(p, 4, s.sleep); !errors.Is(err, ErrHeld) {
		t.Fatalf("err = %v, want ErrHeld", err)
	}
	if len(s.calls) != 3 {
		t.Fatalf("sleep called %d times, want 3", len(s.calls))
	}
	for _, d := range s.calls {
		if d != 20*time.Millisecond {
			t.Fatalf("sleep(%v), want 20ms", d)
		}
	}

	s = &sleepStub{}
	if _, err := LockRetry(p, 0, s.sleep); !errors.Is(err, ErrHeld) || len(s.calls) != 0 {
		t.Fatalf("attempts 0: err = %v, sleeps = %d", err, len(s.calls))
	}
}

func TestLockRetryHolderReleases(t *testing.T) {
	lockPlatform(t)
	p := filepath.Join(shortDir(t), LockName)
	holder, err := TryLock(p)
	if err != nil {
		t.Fatal(err)
	}
	s := &sleepStub{at: 2, hook: func() { _ = holder.Release() }}
	l, err := LockRetry(p, 5, s.sleep)
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	defer l.Release()
	if len(s.calls) != 2 {
		t.Fatalf("sleep called %d times, want 2", len(s.calls))
	}
}

func TestProbe(t *testing.T) {
	lockPlatform(t)
	p := filepath.Join(shortDir(t), LockName)

	held, err := Probe(p)
	if err != nil || held {
		t.Fatalf("absent: held=%v err=%v", held, err)
	}
	if _, err := os.Lstat(p); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("Probe created the lock file: %v", err)
	}

	l, err := TryLock(p)
	if err != nil {
		t.Fatal(err)
	}
	held, err = Probe(p)
	if err != nil || !held {
		t.Fatalf("held: held=%v err=%v", held, err)
	}
	if err := l.Release(); err != nil {
		t.Fatal(err)
	}

	held, err = Probe(p)
	if err != nil || held {
		t.Fatalf("free: held=%v err=%v", held, err)
	}
	// Probe released what it took.
	l, err = TryLock(p)
	if err != nil {
		t.Fatalf("TryLock after Probe: %v", err)
	}
	_ = l.Release()
}
