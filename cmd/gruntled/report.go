package main

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"path/filepath"
	"runtime"
	"time"

	"github.com/GiulioSavini/gruntled/internal/infrastructure/ipc"
	"github.com/GiulioSavini/gruntled/internal/infrastructure/statusfile"
	"github.com/GiulioSavini/gruntled/internal/interfaces/presenter"
)

const reportUsage = `usage: gruntled report [--format text|json|sarif] [path]

Print the diagnostics of the watch daemon running for the Terragrunt
repository at path (default "."), in the same format and bytes as check.
The daemon is only read; report never starts one. It prints the last
published result: after a failed reindex that is the previous result,
with a warning on stderr.
Flags may appear before or after path; "--" ends flag parsing.

Flags:
  --format text|json|sarif   output format (default "text")

Exit codes:
  0  report printed, no error diagnostic
  1  report printed, at least one error diagnostic (GRT001-GRT003, GRT100)
  2  usage error: unknown flag, invalid --format, more than one path
  3  no daemon is running for the repository, the daemon is still indexing, protocol mismatch, or the daemon could not be queried
`

// reportTimeout bounds one query to the daemon.
const reportTimeout = 2 * time.Second

// reportDeps are the seams of runReportWith; production uses
// defaultReportDeps.
type reportDeps struct {
	env  statusfile.Env
	goos string
}

func defaultReportDeps() reportDeps {
	return reportDeps{env: statusfile.OSEnv(), goos: runtime.GOOS}
}

func runReport(args []string, stdout, stderr io.Writer) int {
	return runReportWith(args, stdout, stderr, defaultReportDeps())
}

// runReportWith prints the daemon's last published result. It only reads:
// no directory, file, socket or process is ever created.
func runReportWith(args []string, stdout, stderr io.Writer, deps reportDeps) int {
	var format *string
	dir, code, done := parseArgs("report", reportUsage, args, stderr, func(fs *flag.FlagSet) {
		format = fs.String("format", "text", "output format: text, json or sarif")
	})
	if done {
		return code
	}
	switch *format {
	case "text", "json", "sarif":
	default:
		fmt.Fprintf(stderr, "gruntled: invalid --format %q (want text, json or sarif)\n", *format)
		fmt.Fprint(stderr, reportUsage)
		return exitUsage
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
	// Both levels are checked, as the daemon does, so a foreign directory
	// cannot hand report a forged result.
	for _, d := range []string{filepath.Dir(rt), rt} {
		if err := statusfile.CheckDir(d); err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				return noDaemon(root, stderr)
			}
			fmt.Fprintf(stderr, "gruntled: runtime directory: %v\n", err)
			return exitFailure
		}
	}

	snap, err := queryDaemon(rt, deps.goos)
	var verr *ipc.VersionError
	switch {
	case errors.Is(err, ipc.ErrNoDaemon):
		return noDaemon(root, stderr)
	case errors.As(err, &verr):
		fmt.Fprintf(stderr, "gruntled: daemon runs protocol v%d, this gruntled speaks v%d; restart the daemon\n", verr.Daemon, verr.Client)
		return exitFailure
	case err != nil:
		fmt.Fprintf(stderr, "gruntled: cannot query the watch daemon for %s: %v\n", root, err)
		return exitFailure
	case snap == nil:
		fmt.Fprintln(stderr, "gruntled: the daemon is still indexing; try again")
		return exitFailure
	}

	var out []byte
	switch *format {
	case "json":
		out = snap.JSON
	case "sarif":
		out = snap.SARIF
	default:
		out = snap.Text
	}
	if !writeOut(stdout, stderr, bytes.NewBuffer(out)) {
		return exitFailure
	}
	if *format == "text" && len(snap.Summary) > 0 {
		if _, err := stderr.Write(snap.Summary); err != nil {
			return exitFailure
		}
	}
	if snap.State == ipc.StateFailed {
		reason := presenter.SanitizeReason(snap.LastError)
		if reason == "" {
			reason = "unknown error"
		}
		fmt.Fprintf(stderr, "gruntled: last reindex failed: %s; showing the previous result\n", reason)
	}
	if snap.HasErrors {
		return exitFindings
	}
	return exitOK
}

func noDaemon(root string, stderr io.Writer) int {
	fmt.Fprintf(stderr, "gruntled: no watch daemon is running for %s\n", root)
	return exitFailure
}

// queryDaemon asks the daemon whose runtime directory is rt for its
// snapshot: over the socket, or, where the daemon has none, from the report
// file it rewrites, read only while its lock is held so a file left behind
// by a dead daemon is never shown. A nil snapshot means still indexing.
func queryDaemon(rt, goos string) (*ipc.Snapshot, error) {
	if goos != "windows" {
		_, s, err := ipc.Query(filepath.Join(rt, ipc.SockName), ipc.OpReport, reportTimeout)
		if !errors.Is(err, ipc.ErrUnsupported) {
			return s, err
		}
	}
	held, err := ipc.Probe(filepath.Join(rt, ipc.LockName))
	if err != nil {
		return nil, err
	}
	if !held {
		return nil, ipc.ErrNoDaemon
	}
	_, s, err := ipc.ReadDump(filepath.Join(rt, ipc.DumpName))
	if errors.Is(err, fs.ErrNotExist) {
		// The lock is taken just before the first report file is written.
		return nil, nil
	}
	return s, err
}
