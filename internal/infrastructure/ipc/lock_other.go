//go:build !linux && !darwin && !windows

package ipc

// TryLock is a no-op on platforms without a supported lock primitive.
func TryLock(string) (*Lock, error) { return &Lock{}, nil }

// Probe always reports no holder on platforms without a lock primitive.
func Probe(string) (bool, error) { return false, nil }
