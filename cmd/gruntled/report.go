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
	"strconv"
	"time"

	"github.com/GiulioSavini/gruntled/internal/infrastructure/ipc"
	"github.com/GiulioSavini/gruntled/internal/infrastructure/statusfile"
	"github.com/GiulioSavini/gruntled/internal/interfaces/presenter"
)

const reportUsage = `usage: gruntled report [--blast] [--format text|json|sarif] [path]

Print the diagnostics of the watch daemon running for the Terragrunt
repository at path (default "."), in the same format and bytes as check.
The daemon is only read; report never starts one. It prints the last
published result: after a failed reindex that is the previous result,
with a warning on stderr.
With --blast it prints the daemon's blast view instead: the blast of the
current tree against the daemon's in-memory baseline, in the same format
as blast --base (text or json).
Flags may appear before or after path; "--" ends flag parsing.

Flags:
  --blast                    print the daemon's blast view (text or json)
  --format text|json|sarif   output format (default "text")

Exit codes:
  0  report printed, no error diagnostic
  1  report printed, at least one error diagnostic (GRT001-GRT003, GRT100; in Broken with --blast)
  2  usage error: unknown flag, invalid --format, sarif with --blast, --depth, more than one path
  3  no daemon is running for the repository, the daemon is still indexing, protocol mismatch, blast view over the 64 MiB cap or unavailable, or the daemon could not be queried
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
	var blast *bool
	depth := &depthFlag{}
	dir, code, done := parseArgs("report", reportUsage, args, stderr, func(fs *flag.FlagSet) {
		format = fs.String("format", "text", "output format: text, json or sarif")
		blast = fs.Bool("blast", false, "print the daemon's blast view")
		fs.Var(depth, "depth", "not supported by report")
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
	if *blast && *format == "sarif" {
		fmt.Fprintln(stderr, `gruntled: invalid --format "sarif" with --blast (want text or json)`)
		fmt.Fprint(stderr, reportUsage)
		return exitUsage
	}
	if depth.set {
		fmt.Fprintln(stderr, "gruntled: --depth is not supported by report; use gruntled blast --base <dir> --depth N")
		fmt.Fprint(stderr, reportUsage)
		return exitUsage
	}
	op := ipc.OpReport
	if *blast {
		op = ipc.OpBlast
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

	snap, err := queryDaemon(rt, deps.goos, op)
	var verr *ipc.VersionError
	var rerr *ipc.RefusedError
	switch {
	case errors.Is(err, ipc.ErrNoDaemon):
		return noDaemon(root, stderr)
	case errors.As(err, &verr):
		fmt.Fprintf(stderr, "gruntled: daemon runs protocol v%d, this gruntled speaks v%d; restart the daemon\n", verr.Daemon, verr.Client)
		return exitFailure
	case *blast && errors.As(err, &rerr) && rerr.Code == ipc.CodeTooLarge:
		return blastTooLarge(stderr, refusedSize(rerr.Msg))
	case err != nil:
		fmt.Fprintf(stderr, "gruntled: cannot query the watch daemon for %s: %v\n", root, err)
		return exitFailure
	case snap == nil:
		fmt.Fprintln(stderr, "gruntled: the daemon is still indexing; try again")
		return exitFailure
	}

	if *blast {
		return printBlast(snap, *format, stdout, stderr)
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
	warnFailed(snap, stderr)
	if snap.HasErrors {
		return exitFindings
	}
	return exitOK
}

// warnFailed prints the previous-result warning after a failed reindex.
func warnFailed(snap *ipc.Snapshot, stderr io.Writer) {
	if snap.State != ipc.StateFailed {
		return
	}
	reason := presenter.SanitizeReason(snap.LastError)
	if reason == "" {
		reason = "unknown error"
	}
	fmt.Fprintf(stderr, "gruntled: last reindex failed: %s; showing the previous result\n", reason)
}

// printBlast prints the daemon's blast view in format (text or json). A
// view over the cap or one the daemon could not compute exits 3; the exit
// code is 1 iff Broken holds an error.
func printBlast(snap *ipc.Snapshot, format string, stdout, stderr io.Writer) int {
	v := snap.Blast
	switch {
	case v.TooLarge > 0:
		return blastTooLarge(stderr, v.TooLarge)
	case v.Error != "":
		fmt.Fprintf(stderr, "gruntled: the daemon could not compute the blast view: %s\n", presenter.SanitizeReason(v.Error))
		return exitFailure
	}
	out := v.Text
	if format == "json" {
		out = v.JSON
	}
	if !writeOut(stdout, stderr, bytes.NewBuffer(out)) {
		return exitFailure
	}
	warnFailed(snap, stderr)
	if v.HasErrors {
		return exitFindings
	}
	return exitOK
}

func blastTooLarge(stderr io.Writer, n int64) int {
	size := "more than 64 MiB"
	if n > 0 {
		size = strconv.FormatInt(n, 10) + " bytes"
	}
	fmt.Fprintf(stderr, "gruntled: the blast view is %s, over the 64 MiB response cap; run gruntled blast --base <dir>\n", size)
	return exitFailure
}

// refusedSize reads n from a too_large refusal "blast view is <n> bytes, ...";
// 0 when the message has another shape.
func refusedSize(msg string) int64 {
	var n int64
	if _, err := fmt.Sscanf(msg, "blast view is %d bytes", &n); err != nil || n < 0 {
		return 0
	}
	return n
}

func noDaemon(root string, stderr io.Writer) int {
	fmt.Fprintf(stderr, "gruntled: no watch daemon is running for %s\n", root)
	return exitFailure
}

// queryDaemon asks the daemon whose runtime directory is rt for its
// snapshot (op report or blast): over the socket, or, where the daemon has
// none, from the report file it rewrites, read only while its lock is held
// so a file left behind by a dead daemon is never shown. A nil snapshot
// means still indexing. For blast the view is validated on both transports
// (ipc.ValidBlastView): a missing or empty view is an error.
func queryDaemon(rt, goos, op string) (*ipc.Snapshot, error) {
	if goos != "windows" {
		_, s, err := ipc.Query(filepath.Join(rt, ipc.SockName), op, reportTimeout)
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
	if err == nil && s != nil && op == ipc.OpBlast {
		if verr := ipc.ValidBlastView(s.Blast); verr != nil {
			return nil, fmt.Errorf("ipc: malformed report file: %w", verr)
		}
	}
	return s, err
}
