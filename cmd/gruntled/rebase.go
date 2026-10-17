package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/GiulioSavini/gruntled/internal/infrastructure/ipc"
	"github.com/GiulioSavini/gruntled/internal/infrastructure/statusfile"
	"github.com/GiulioSavini/gruntled/internal/interfaces/presenter"
)

const rebaseUsage = `usage: gruntled rebase [--expect N] [path]

Move the baseline of the watch daemon running for the Terragrunt
repository at path (default ".") to its last published result, so that
report --blast compares against it from now on. Only the daemon's
in-memory baseline changes: nothing is written and nothing is reindexed.
It prints the number of error findings in Broken that the new baseline
accepts. rebase needs the daemon socket: it is not available on windows.
Flags may appear before or after path; "--" ends flag parsing.

Flags:
  --expect N   rebase only if the daemon is at generation N (an integer >= 1)

Exit codes:
  0  baseline moved
  2  usage error: unknown flag, invalid --expect, more than one path
  3  no daemon is running for the repository, the daemon refused (still indexing, failed reindex, generation mismatch, blast view unavailable), protocol mismatch, no usable answer, or not available on this platform
`

// rebaseTimeout bounds one rebase exchange; a variable for tests.
var rebaseTimeout = 10 * time.Second

const rebaseUnsupported = "gruntled: rebase needs the daemon socket, which this platform does not have; restart gruntled watch to reset the baseline"

func runRebase(args []string, stdout, stderr io.Writer) int {
	return runRebaseWith(args, stdout, stderr, defaultReportDeps())
}

// runRebaseWith asks the daemon to move its baseline. Like report, it only
// reads the runtime directory: no directory, file, socket or process is
// ever created.
func runRebaseWith(args []string, stdout, stderr io.Writer, deps reportDeps) int {
	var o *rebaseOpts
	dir, code, done := parseArgs("rebase", rebaseUsage, args, stderr, func(fs *flag.FlagSet) {
		o = defineRebaseFlags(fs)
	})
	if done {
		return code
	}
	expect := o.expect
	var want *uint64
	if expect.set {
		n, ok := parseGeneration(expect.value)
		if !ok {
			fmt.Fprintf(stderr, "gruntled: invalid --expect %q (want a generation >= 1)\n", expect.value)
			fmt.Fprint(stderr, rebaseUsage)
			return exitUsage
		}
		want = &n
	}
	if deps.goos == "windows" {
		fmt.Fprintln(stderr, rebaseUnsupported)
		return exitFailure
	}

	rootHandle, root, ok := openRepoRoot(dir, stderr)
	if !ok {
		return exitFailure
	}
	_ = rootHandle.Close()
	rt, err := statusfile.Dir(root, deps.env)
	if err != nil {
		fmt.Fprintf(stderr, "gruntled: runtime directory for %s: %v\n", root, err)
		return exitFailure
	}
	for _, d := range []string{filepath.Dir(rt), rt} {
		if err := statusfile.CheckDir(d); err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				return noDaemon(root, stderr)
			}
			fmt.Fprintf(stderr, "gruntled: runtime directory: %v\n", err)
			return exitFailure
		}
	}

	_, res, err := ipc.Rebase(filepath.Join(rt, ipc.SockName), want, rebaseTimeout)
	if err != nil {
		return rebaseFailed(root, err, stderr)
	}
	fmt.Fprintf(stdout, "gruntled: baseline moved to generation %d; %d error findings in Broken accepted\n", res.Generation, res.BrokenErrors)
	return exitOK
}

// rebaseFailed prints the fixed message for a rebase error; every path is
// exit 3. Daemon-provided text is sanitised.
func rebaseFailed(root string, err error, stderr io.Writer) int {
	var verr *ipc.VersionError
	var rerr *ipc.RefusedError
	switch {
	case errors.Is(err, ipc.ErrNoDaemon):
		return noDaemon(root, stderr)
	case errors.Is(err, ipc.ErrUnsupported):
		fmt.Fprintln(stderr, rebaseUnsupported)
	case errors.As(err, &verr):
		fmt.Fprintf(stderr, "gruntled: daemon runs protocol v%d, this gruntled speaks v%d; restart the daemon\n", verr.Daemon, verr.Client)
	case errors.As(err, &rerr):
		fmt.Fprintln(stderr, refusalMessage(rerr))
	case errors.Is(err, ipc.ErrOutcomeUnknown):
		why := ""
		if errors.Is(err, os.ErrDeadlineExceeded) {
			why = " (no answer within " + rebaseTimeout.String() + ")"
		}
		fmt.Fprintf(stderr, "gruntled: no usable answer from the daemon%s; the baseline may or may not have moved; run gruntled report --blast to check\n", why)
	default:
		// Only failures before the request was sent reach here.
		fmt.Fprintf(stderr, "gruntled: cannot reach the watch daemon for %s: %v\n", root, err)
	}
	return exitFailure
}

func refusalMessage(e *ipc.RefusedError) string {
	msg := presenter.SanitizeReason(e.Msg)
	switch e.Code {
	case ipc.CodeIndexing:
		return "gruntled: the daemon is still indexing; try again"
	case ipc.CodeFailed:
		if msg == "" {
			msg = "unknown error"
		}
		return "gruntled: the last reindex failed: " + msg + "; fix the tree and save, then rebase"
	case ipc.CodeExpectMismatch:
		return "gruntled: " + msg + "; nothing changed"
	case ipc.CodeUnavailable:
		return "gruntled: the daemon could not compute the blast view; nothing changed"
	case ipc.CodeInternal:
		return "gruntled: the daemon could not rebase; nothing changed"
	case ipc.CodeUnsupported:
		return "gruntled: the daemon does not support rebase; restart gruntled watch to reset the baseline"
	}
	return "gruntled: the daemon refused the rebase: " + msg + "; nothing changed"
}

// parseGeneration accepts only ^[1-9][0-9]*$ fitting a uint64.
func parseGeneration(s string) (uint64, bool) {
	if s == "" || s[0] < '1' || s[0] > '9' {
		return 0, false
	}
	for _, c := range []byte(s) {
		if c < '0' || c > '9' {
			return 0, false
		}
	}
	n, err := strconv.ParseUint(s, 10, 64)
	return n, err == nil
}
