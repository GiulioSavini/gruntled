package main

import (
	"bytes"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"path/filepath"
	"runtime"
	"sync"
	"time"

	"github.com/GiulioSavini/gruntled/internal/application/checking"
	"github.com/GiulioSavini/gruntled/internal/application/watching"
	"github.com/GiulioSavini/gruntled/internal/domain/diagnostic"
	"github.com/GiulioSavini/gruntled/internal/infrastructure/ipc"
	"github.com/GiulioSavini/gruntled/internal/infrastructure/statusfile"
	"github.com/GiulioSavini/gruntled/internal/infrastructure/terragrunt"
	"github.com/GiulioSavini/gruntled/internal/infrastructure/tfsurface"
	"github.com/GiulioSavini/gruntled/internal/infrastructure/watch"
	"github.com/GiulioSavini/gruntled/internal/interfaces/presenter"
)

const watchUsage = `usage: gruntled watch [--poll] [--poll-interval d] [--debounce d] [--status-file p] [--print-status-path] [path]

Watch the Terragrunt repository at path (default "."): index it fully, then
reindex only the changed files after each save, until interrupted
(SIGINT/SIGTERM). After every index a one-line status is written
atomically to a status file outside the repository; the diagnostics that
changed are printed to stdout in the same text format as check.
Flags may appear before or after path; "--" ends flag parsing.

Flags:
  --poll                 use stat polling instead of the native watcher (always on windows)
  --poll-interval d      polling period (default 500ms)
  --debounce d           quiet period after the last change before reindexing (default 150ms)
  --status-file p        write the status line to p instead of the default path; p must be outside the repository
  --print-status-path    print the status file path for path, then exit

Exit codes:
  0  clean shutdown after SIGINT/SIGTERM, already watching (another daemon runs for path), or --print-status-path / -h
  2  usage error: unknown flag, invalid duration, more than one path, --status-file or runtime directory inside the repository
  3  watch could not start: path missing, not a directory or unreadable, no watcher, or the initial index failed
`

// watchOpts holds the parsed watch flags.
type watchOpts struct {
	poll         *bool
	pollInterval *time.Duration
	debounce     *time.Duration
	statusFile   *string
	printPath    *bool
}

// defineWatchFlags registers the watch flags on fs. The --debounce default
// is watch.DefaultQuiet, never a separate literal.
func defineWatchFlags(fs *flag.FlagSet) *watchOpts {
	return &watchOpts{
		poll:         fs.Bool("poll", false, "use stat polling instead of the native watcher"),
		pollInterval: fs.Duration("poll-interval", watch.DefaultPollInterval, "polling period"),
		debounce:     fs.Duration("debounce", watch.DefaultQuiet, "quiet period before reindexing"),
		statusFile:   fs.String("status-file", "", "status file path (must be outside the repository)"),
		printPath:    fs.Bool("print-status-path", false, "print the status file path, then exit"),
	}
}

// watchDeps are the seams of runWatchWith; production uses
// defaultWatchDeps.
type watchDeps struct {
	now       func() time.Time
	newNative func(root string) (watch.Watcher, error)
	newPoll   func(root string, interval time.Duration) (watch.Watcher, error)
	env       statusfile.Env
	goos      string
	// maxWait caps a debounced burst; zero means watch.DefaultMaxWait.
	maxWait time.Duration
	// loaderHook, when set, sees the Loader before the daemon starts.
	loaderHook func(*terragrunt.Loader)
	// sleep pauses between lock and ping retries.
	sleep func(time.Duration)
}

func defaultWatchDeps() watchDeps {
	return watchDeps{
		now:       time.Now,
		newNative: watch.NewNative,
		newPoll:   watch.NewPoll,
		env:       statusfile.OSEnv(),
		goos:      runtime.GOOS,
		sleep:     time.Sleep,
	}
}

func runWatch(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	return runWatchWith(ctx, args, stdout, stderr, defaultWatchDeps())
}

func runWatchWith(ctx context.Context, args []string, stdout, stderr io.Writer, deps watchDeps) int {
	var o *watchOpts
	dir, code, done := parseArgs("watch", watchUsage, args, stderr, func(fs *flag.FlagSet) {
		o = defineWatchFlags(fs)
	})
	if done {
		return code
	}
	for _, f := range []struct {
		name string
		d    time.Duration
	}{{"--poll-interval", *o.pollInterval}, {"--debounce", *o.debounce}} {
		if f.d <= 0 {
			fmt.Fprintf(stderr, "gruntled: %s must be positive, got %v\n", f.name, f.d)
			fmt.Fprint(stderr, watchUsage)
			return exitUsage
		}
	}

	rootHandle, root, ok := openRepoRoot(dir, stderr)
	if !ok {
		return exitFailure
	}
	defer rootHandle.Close()

	var err error
	statusPath := *o.statusFile
	if statusPath != "" {
		if statusPath, err = filepath.Abs(statusPath); err != nil {
			fmt.Fprintf(stderr, "gruntled: --status-file: %v\n", err)
			return exitUsage
		}
	} else if statusPath, err = statusfile.Path(root, deps.env); err != nil {
		fmt.Fprintf(stderr, "gruntled: status file path: %v\n", err)
		return exitFailure
	}
	inside, err := statusfile.Inside(root, statusPath)
	if err != nil {
		fmt.Fprintf(stderr, "gruntled: status file path: %v\n", err)
		return exitFailure
	}
	if inside {
		fmt.Fprintf(stderr, "gruntled: status file %s is inside the repository; use --status-file outside %s\n", statusPath, root)
		return exitUsage
	}
	if *o.printPath {
		fmt.Fprintln(stdout, statusPath)
		return exitOK
	}

	// Single instance first: a second watch is detected even while the
	// first is still running its initial index.
	inst, code, done := acquireInstance(root, statusPath, deps, stdout, stderr)
	if done {
		return code
	}
	defer inst.close()

	// The watcher exists before the initial index starts, so no change made
	// during that index is lost.
	w, backend, ok := openWatcher(root, *o.poll, *o.pollInterval, deps, stderr)
	if !ok {
		return exitFailure
	}
	fmt.Fprintf(stderr, "gruntled: watching %s (%s); status: %s\n", root, backend, statusPath)

	fsys := rootHandle.FS()
	loader := terragrunt.NewLoader(fsys)
	if deps.loaderHook != nil {
		deps.loaderHook(loader)
	}

	pub := &watchPublisher{
		stdout:    stdout,
		stderr:    stderr,
		now:       deps.now,
		status:    statusfile.NewWriter(statusPath),
		snapshots: inst.store,
	}
	err = watch.Run(ctx, watch.Config{
		Watcher: w,
		Indexer: watching.NewIndexer(loader, tfsurface.NewReader(fsys)),
		Publish: pub.publish,
		Quiet:   *o.debounce,
		MaxWait: deps.maxWait,
	})
	if err != nil {
		printRunError(stderr, err)
		return exitFailure
	}
	return exitOK
}

// printRunError prints the error watch.Run returned (today only an
// initial-index root walk error) as one stderr line, with terminal
// controls and format runes neutralised (sec #42, defence in depth).
func printRunError(stderr io.Writer, err error) {
	fmt.Fprintf(stderr, "gruntled: %s\n", presenter.SanitizeReason(err.Error()))
}

// openWatcher picks the backend: polling when asked or on windows, else
// the native watcher, falling back to polling when the OS cannot provide
// one.
func openWatcher(root string, forcePoll bool, interval time.Duration, deps watchDeps, stderr io.Writer) (watch.Watcher, string, bool) {
	if !forcePoll && deps.goos != "windows" {
		w, err := deps.newNative(root)
		switch {
		case err == nil:
			if watch.WarnPollAdvised(root) {
				fmt.Fprintln(stderr, "gruntled: hint: native events miss Windows-side edits under /mnt/; use --poll if saves are not picked up")
			}
			return w, "fsnotify", true
		case errors.Is(err, watch.ErrWatchLimit) || errors.Is(err, watch.ErrNativeUnavailable):
			fmt.Fprintf(stderr, "gruntled: native watcher unavailable (%v); falling back to polling\n", err)
		default:
			fmt.Fprintf(stderr, "gruntled: cannot start watcher: %v\n", err)
			return nil, "", false
		}
	}
	w, err := deps.newPoll(root, interval)
	if err != nil {
		fmt.Fprintf(stderr, "gruntled: cannot start watcher: %v\n", err)
		return nil, "", false
	}
	return w, "poll", true
}

// Baseline sources, as the blast view labels them.
const (
	sourceWatchStart = "watch start"
	sourceRebase     = "rebase"
)

// watchPublisher turns run loop events into status file lines, stdout
// output and served snapshots, and owns the daemon's blast baseline.
// publish is called only from the watch.Run goroutine; rebase is called
// from socket handler goroutines. mu serialises them: it guards base,
// baseGen, baseSource, lastRep, gen and last, and every setSnapshot. The
// baseline is a checking.Report only, never a loader, FS or cache.
type watchPublisher struct {
	stdout, stderr io.Writer
	now            func() time.Time
	status         *statusfile.Writer
	// writeFailed is set once a status write failed; later failures are
	// not logged again.
	writeFailed bool
	// ready is set once an index has succeeded; before that a failure is
	// reported by runWatchWith from Run's error.
	ready   bool
	printed diagnostic.Set
	// snapshots, when set, receives every published snapshot.
	snapshots func(*ipc.Snapshot)

	mu sync.Mutex
	// last is the served snapshot; gen counts committed indexes; lastRep
	// is the Report of the last committed EventReady.
	last    *ipc.Snapshot
	gen     uint64
	lastRep checking.Report
	// base is the baseline Report: the first committed EventReady, or
	// lastRep at the last rebase. hasBase is false until then.
	base       checking.Report
	baseGen    uint64
	baseSource string
	hasBase    bool
}

func (p *watchPublisher) publish(ev watch.Event) {
	stamp := p.now().Format("15:04:05")
	var line, out bytes.Buffer
	var renderErr error
	switch ev.Kind {
	case watch.EventIndexing:
		_ = presenter.StatusIndexing(&line, stamp)
	case watch.EventReady:
		_ = presenter.StatusLine(&line, ev.Report.Diagnostics, stamp)
		if !p.ready || !ev.Report.Diagnostics.Equal(p.printed) {
			if err := presenter.Text(&out, ev.Report.Diagnostics); err != nil {
				out.Reset()
			}
			p.printed = ev.Report.Diagnostics
		}
		p.ready = true
		renderErr = p.commitReady(ev.Report)
	case watch.EventFailed:
		reason := "unknown error"
		if ev.Err != nil {
			reason = ev.Err.Error()
		}
		_ = presenter.StatusFailed(&line, reason, stamp)
		if p.ready {
			fmt.Fprintf(p.stderr, "gruntled: reindex failed: %s\n", presenter.SanitizeReason(reason))
		}
		// The last good bytes and blast view stay served, marked failed.
		p.mu.Lock()
		if p.last != nil {
			s := *p.last
			s.State = ipc.StateFailed
			s.LastError = presenter.SanitizeReason(reason)
			p.setSnapshot(&s)
		}
		p.mu.Unlock()
	case watch.EventStopped:
		_ = presenter.StatusStopped(&line, stamp)
	default:
		return
	}
	// stdout and the status file are written outside p.mu.
	writeOut(p.stdout, p.stderr, &out)
	if renderErr != nil {
		fmt.Fprintf(p.stderr, "gruntled: rendering report: %v\n", renderErr)
	}
	if err := p.status.Write(line.String()); err != nil && !p.writeFailed {
		p.writeFailed = true
		fmt.Fprintf(p.stderr, "gruntled: cannot write status file %s: %v (further failures are not reported)\n", p.status.Path, err)
	}
}

// commitReady builds the snapshot of rep, with its blast view, as the next
// generation and commits it. lastRep, gen, the first baseline and the
// served snapshot change together, and only when the snapshot was built.
func (p *watchPublisher) commitReady(rep checking.Report) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	next := p.gen + 1
	s, err := buildSnap(rep, next)
	if err != nil {
		return err
	}
	base, baseGen, src := p.base, p.baseGen, p.baseSource
	if !p.hasBase {
		base, baseGen, src = rep, next, sourceWatchStart
	}
	s.Blast = blastViewOrError(base, rep, presenter.DaemonBaseline{Source: src, Generation: baseGen, CurrentGeneration: next})
	p.lastRep, p.gen = rep, next
	p.base, p.baseGen, p.baseSource, p.hasBase = base, baseGen, src, true
	p.setSnapshot(s)
	return nil
}

// blastViewOrError is buildBlastView, or on failure a view carrying the
// sanitised error and the baseline metadata: never nil.
func blastViewOrError(base, cur checking.Report, d presenter.DaemonBaseline) *ipc.BlastView {
	v, err := buildBlastView(base, cur, d)
	if err != nil {
		return &ipc.BlastView{
			BaselineGeneration: d.Generation,
			BaselineSource:     d.Source,
			Error:              presenter.SanitizeReason("blast view: " + err.Error()),
		}
	}
	return v
}

// rebase makes the Report of the served snapshot the baseline. It refuses,
// changing nothing, while indexing, after a failed reindex, when the served
// view could not be computed (its accepted count would be unknown), when
// expect is set and differs from the served generation, and when the new
// view cannot be built. It writes nothing: no status line, no stdout. The
// generation does not move. It returns the number of error findings the
// served view held, which the new baseline accepts.
func (p *watchPublisher) rebase(expect *uint64) (ipc.RebaseResult, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	last := p.last
	switch {
	case last == nil:
		return ipc.RebaseResult{}, ipc.Refuse(ipc.CodeIndexing, "the daemon is still indexing; nothing changed")
	case last.State == ipc.StateFailed:
		return ipc.RebaseResult{}, ipc.Refuse(ipc.CodeFailed, "the last reindex failed; nothing changed")
	case last.Blast == nil || last.Blast.Error != "":
		return ipc.RebaseResult{}, ipc.Refuse(ipc.CodeUnavailable, "the blast view could not be computed; nothing changed")
	case expect != nil && *expect != last.Generation:
		return ipc.RebaseResult{}, ipc.Refuse(ipc.CodeExpectMismatch, fmt.Sprintf("the daemon is at generation %d, not %d", last.Generation, *expect))
	}
	v, err := buildBlastView(p.lastRep, p.lastRep, presenter.DaemonBaseline{Source: sourceRebase, Generation: last.Generation, CurrentGeneration: last.Generation})
	if err != nil {
		return ipc.RebaseResult{}, ipc.Refuse(ipc.CodeInternal, "the blast view could not be rebuilt; nothing changed")
	}
	accepted := last.Blast.BrokenErrors
	c := *last
	c.Blast = v
	p.base, p.baseGen, p.baseSource, p.hasBase = p.lastRep, last.Generation, sourceRebase, true
	p.setSnapshot(&c)
	return ipc.RebaseResult{Generation: last.Generation, BrokenErrors: accepted}, nil
}

// setSnapshot records s and hands it on. It runs inside p.mu; in
// report-file mode that includes the dump write (up to the response cap),
// which is harmless because the instance refuses rebase in that mode.
// Snapshots are never mutated once handed on: handlers may be reading them.
func (p *watchPublisher) setSnapshot(s *ipc.Snapshot) {
	p.last = s
	if p.snapshots != nil {
		p.snapshots(s)
	}
}
