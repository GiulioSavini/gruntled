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

// openRepoRoot opens dir and returns its absolute, symlink-resolved path:
// the key every per-repository runtime path (lock, socket, report file,
// status file) derives from. watch and report both resolve it here, so
// they always agree on the directory. The caller closes the handle.
func openRepoRoot(dir string, stderr io.Writer) (*os.Root, string, bool) {
	h, ok := openRepo(dir, stderr)
	if !ok {
		return nil, "", false
	}
	root, err := filepath.Abs(dir)
	if err == nil {
		root, err = filepath.EvalSymlinks(root)
	}
	if err != nil {
		_ = h.Close()
		fmt.Fprintf(stderr, "gruntled: cannot open repository: %v\n", err)
		return nil, "", false
	}
	return h, root, true
}

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
	// rebaseHook is the publisher's rebase, set before watch.Run starts.
	rebaseHook atomic.Pointer[func(*uint64) (ipc.RebaseResult, error)]

	// dump is the report file path in report-file mode, else empty.
	dump       string
	dumpW      *statusfile.Writer
	dumpFailed bool
}

// acquireInstance takes the repository lock and starts publishing. When
// another daemon holds the lock it prints where that daemon runs to stdout
// and returns done with exitOK. It runs before the watcher and the initial
// index start. A runtime directory inside the repository is refused with
// exitUsage before anything is created: the report file rewritten after
// every index would itself be a watched change.
func acquireInstance(root, statusPath string, deps watchDeps, stdout, stderr io.Writer) (inst *instance, code int, done bool) {
	rt, err := statusfile.Dir(root, deps.env)
	if err != nil {
		fmt.Fprintf(stderr, "gruntled: runtime directory for %s: %v\n", root, err)
		return nil, exitFailure, true
	}
	inside, err := statusfile.Inside(root, rt)
	if err != nil {
		fmt.Fprintf(stderr, "gruntled: runtime directory for %s: %v\n", root, err)
		return nil, exitFailure, true
	}
	if inside {
		fmt.Fprintf(stderr, "gruntled: runtime directory %s is inside the repository %s; set XDG_RUNTIME_DIR (linux) or the user cache directory outside it\n", rt, root)
		return nil, exitUsage, true
	}
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
		info:   daemonInfo(root, statusPath),
	}
	if useSocket {
		ln, err := ipc.Listen(sock)
		switch {
		case err == nil:
			inst.server = ipc.ServeHandler(ln, inst.info, inst)
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

// Snapshot is the served snapshot (ipc.Handler).
func (in *instance) Snapshot() *ipc.Snapshot { return in.snap.Load() }

// Rebase delegates to the publisher's rebase (ipc.Handler). It refuses
// with unsupported in report-file mode, where there is no socket client,
// and with indexing before the publisher is wired.
func (in *instance) Rebase(expect *uint64) (ipc.RebaseResult, error) {
	if in.dumpW != nil {
		return ipc.RebaseResult{}, ipc.Refuse(ipc.CodeUnsupported, "rebase needs the daemon socket; nothing changed")
	}
	h := in.rebaseHook.Load()
	if h == nil {
		return ipc.RebaseResult{}, ipc.Refuse(ipc.CodeIndexing, "the daemon is still indexing; nothing changed")
	}
	return (*h)(expect)
}

// setRebase wires the publisher's rebase into the socket handler.
func (in *instance) setRebase(f func(*uint64) (ipc.RebaseResult, error)) {
	in.rebaseHook.Store(&f)
}

// store seals s (its blast response is encoded once, and an over-cap view
// becomes too large here), publishes it to the socket handlers and, in
// report-file mode, rewrites the report file. Called from one goroutine at
// a time, before s is shared.
func (in *instance) store(s *ipc.Snapshot) {
	ipc.Seal(in.info, s)
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

// daemonInfo is what the daemon advertises to report and rebase clients;
// its version comes from versionInfo like --version and SARIF.
func daemonInfo(root, statusPath string) ipc.Info {
	v, _ := versionInfo()
	return ipc.Info{PID: os.Getpid(), Version: v, Root: root, StatusPath: statusPath}
}
