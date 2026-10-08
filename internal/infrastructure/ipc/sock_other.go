//go:build !linux && !darwin

package ipc

import "time"

// Listener is not available on this platform.
type Listener struct{}

// Server is not available on this platform.
type Server struct{}

// Listen returns ErrUnsupported; use WriteDump/ReadDump instead.
func Listen(string) (*Listener, error) { return nil, ErrUnsupported }

// Serve returns an inert Server.
func Serve(*Listener, Info, func() *Snapshot) *Server { return &Server{} }

// Close is a no-op.
func (*Server) Close() error { return nil }

// Query returns ErrUnsupported.
func Query(string, string, time.Duration) (Info, *Snapshot, error) {
	return Info{}, nil, ErrUnsupported
}
