//go:build windows

package watch

// NewNative is not built on windows (keeps fsnotify and x/sys/windows out of
// the binary); the CLI falls back to NewPoll.
func NewNative(root string) (Watcher, error) { return nil, ErrNativeUnavailable }

// WarnPollAdvised is always false on windows: polling is already the only
// adapter.
func WarnPollAdvised(root string) bool { return false }
