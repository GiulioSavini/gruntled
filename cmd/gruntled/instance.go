package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync/atomic"
	"time"

	"github.com/GiulioSavini/gruntled/internal/infrastructure/ipc"
	"github.com/GiulioSavini/gruntled/internal/infrastructure/statusfile"
)

const (
	// pingTries and pingTimeout bound how long a second watch spends
	// learning who holds the lock: under 3 s in total.
	pingTries   = 3
	pingTimeout = 900 * time.Millisecond
	pingPause   = 100 * time.Millisecond
)

// instance is the single running daemon of one repository: it holds the
// lock and publishes snapshots through a socket or, where there is none,
// a report file. Its paths always derive from statusfile.Dir(root), never
// from --status-file, so a different --status-file cannot start a second
// daemon.
type instance struct {
	stderr io.Writer
	lock   *ipc.Lock
	server *ipc.Server
	info   ipc.Info
	snap   atomic.Pointer[ipc.Snapshot]

	// dump is the report file path in report-file mode, else empty.
	dump       string
	dumpW      *statusfile.Writer
	dumpFailed bool
}

// acquireInstance takes the repository lock and starts publishing. When
// another daemon holds the lock it prints where that daemon runs to stdout
// and returns done with exitOK. It runs before the watcher and the initial
// index start.
func acquireInstance(root, statusPath string, deps watchDeps, stdout, stderr io.Writer) (inst *instance, code int, done bool) {
	dir, err := statusfile.EnsureRepoDir(root, deps.env)
	if err != nil {
		fmt.Fprintf(stderr, "gruntled: runtime directory for %s: %v\n", root, err)
		return nil, exitFailure, true
	}
	useSocket := deps.goos != "windows"
	sock := filepath.Join(dir, ipc.SockName)
	if useSocket {
		if err := ipc.CheckSockPath(sock); err != nil {
			fmt.Fprintf(stderr, "gruntled: %v\n", err)
			return nil, exitFailure, true
		}
	}

	attempts := 1
	if !useSocket {
		// A concurrent report briefly holds the file exclusively there.
		attempts = 3
	}
	lock, err := ipc.LockRetry(filepath.Join(dir, ipc.LockName), attempts, deps.sleep)
	switch {
	case errors.Is(err, ipc.ErrHeld):
		if useSocket {
			alreadySocket(root, sock, deps, stdout)
		} else {
			alreadyDump(root, filepath.Join(dir, ipc.DumpName), stdout)
		}
		return nil, exitOK, true
	case err != nil:
		fmt.Fprintf(stderr, "gruntled: cannot lock %s: %v\n", dir, err)
		return nil, exitFailure, true
	}

	inst = &instance{
		stderr: stderr,
		lock:   lock,
		info:   ipc.Info{PID: os.Getpid(), Version: version, Root: root, StatusPath: statusPath},
	}
	if useSocket {
		ln, err := ipc.Listen(sock)
		switch {
		case err == nil:
			inst.server = ipc.Serve(ln, inst.info, inst.snap.Load)
			return inst, exitOK, false
		case !errors.Is(err, ipc.ErrUnsupported):
			_ = lock.Release()
			fmt.Fprintf(stderr, "gruntled: cannot listen: %v\n", err)
			return nil, exitFailure, true
		}
		// No sockets on this platform: fall back to the report file.
	}
	inst.dump = filepath.Join(dir, ipc.DumpName)
	inst.dumpW = statusfile.NewWriter(inst.dump)
	inst.writeDump(nil)
	return inst, exitOK, false
}

// alreadySocket prints the running daemon's location, asking it for its
// pid and status path; it never blocks for more than about 3 s.
func alreadySocket(root, sock string, deps watchDeps, stdout io.Writer) {
	for i := range pingTries {
		info, _, err := ipc.Query(sock, ipc.OpPing, pingTimeout)
		if err == nil {
			fmt.Fprintf(stdout, "gruntled: already watching %s (pid %d); socket: %s; status: %s\n", root, info.PID, sock, info.StatusPath)
			return
		}
		if i < pingTries-1 {
			deps.sleep(pingPause)
		}
	}
	// The holder is still starting or not answering: it is running anyway.
	fmt.Fprintf(stdout, "gruntled: already watching %s; socket: %s\n", root, sock)
}

// alreadyDump prints the running daemon's report file and, when the file
// can be read, its status path.
func alreadyDump(root, dump string, stdout io.Writer) {
	if info, _, err := ipc.ReadDump(dump); err == nil {
		fmt.Fprintf(stdout, "gruntled: already watching %s; report: %s; status: %s\n", root, dump, info.StatusPath)
		return
	}
	fmt.Fprintf(stdout, "gruntled: already watching %s; report: %s\n", root, dump)
}

// store publishes s to the socket handlers and, in report-file mode,
// rewrites the report file. Called from one goroutine at a time.
func (in *instance) store(s *ipc.Snapshot) {
	in.snap.Store(s)
	if in.dumpW != nil {
		in.writeDump(s)
	}
}

func (in *instance) writeDump(s *ipc.Snapshot) {
	if err := ipc.WriteDump(in.dumpW, in.info, s); err != nil && !in.dumpFailed {
		in.dumpFailed = true
		fmt.Fprintf(in.stderr, "gruntled: cannot write report file %s: %v (further failures are not reported)\n", in.dump, err)
	}
}

// close stops publishing, then releases the lock: the socket is unlinked
// (or the report file removed) while the lock is still held, so a new
// daemon never sees its own socket removed.
func (in *instance) close() {
	if in.server != nil {
		_ = in.server.Close()
	}
	if in.dump != "" {
		_ = os.Remove(in.dump)
	}
	_ = in.lock.Release()
}
