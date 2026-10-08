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
	"time"

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
  0  clean shutdown after SIGINT/SIGTERM, or --print-status-path / -h
  2  usage error: unknown flag, invalid duration, more than one path, --status-file inside the repository
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

	rootHandle, ok := openRepo(dir, stderr)
	if !ok {
		return exitFailure
	}
	defer rootHandle.Close()
	root, err := filepath.Abs(dir)
	if err == nil {
		root, err = filepath.EvalSymlinks(root)
	}
	if err != nil {
		fmt.Fprintf(stderr, "gruntled: cannot open repository: %v\n", err)
		return exitFailure
	}

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
		fmt.Fprintf(stderr, "gruntled: %v\n", err)
		return exitFailure
	}
	return exitOK
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

// watchPublisher turns run loop events into status file lines and stdout
// output. It is called only from the watch.Run goroutine.
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
	// last is the most recent snapshot; gen counts successful indexes.
	last *ipc.Snapshot
	gen  uint64
}

func (p *watchPublisher) publish(ev watch.Event) {
	stamp := p.now().Format("15:04:05")
	var line bytes.Buffer
	switch ev.Kind {
	case watch.EventIndexing:
		_ = presenter.StatusIndexing(&line, stamp)
	case watch.EventReady:
		_ = presenter.StatusLine(&line, ev.Report.Diagnostics, stamp)
		if !p.ready || !ev.Report.Diagnostics.Equal(p.printed) {
			var out bytes.Buffer
			if err := presenter.Text(&out, ev.Report.Diagnostics); err == nil {
				writeOut(p.stdout, p.stderr, &out)
			}
			p.printed = ev.Report.Diagnostics
		}
		p.ready = true
		p.gen++
		if s, err := buildSnapshot(ev.Report, p.gen); err == nil {
			p.setSnapshot(s)
		} else {
			fmt.Fprintf(p.stderr, "gruntled: rendering report: %v\n", err)
		}
	case watch.EventFailed:
		reason := "unknown error"
		if ev.Err != nil {
			reason = ev.Err.Error()
		}
		_ = presenter.StatusFailed(&line, reason, stamp)
		if p.ready {
			fmt.Fprintf(p.stderr, "gruntled: reindex failed: %s\n", presenter.SanitizeReason(reason))
		}
		// The last good bytes stay served, marked failed with the reason.
		if p.last != nil {
			s := *p.last
			s.State = ipc.StateFailed
			s.LastError = presenter.SanitizeReason(reason)
			p.setSnapshot(&s)
		}
	case watch.EventStopped:
		_ = presenter.StatusStopped(&line, stamp)
	default:
		return
	}
	if err := p.status.Write(line.String()); err != nil && !p.writeFailed {
		p.writeFailed = true
		fmt.Fprintf(p.stderr, "gruntled: cannot write status file %s: %v (further failures are not reported)\n", p.status.Path, err)
	}
}

// setSnapshot records s and hands it on. Snapshots are never mutated once
// handed on: handlers may be reading them.
func (p *watchPublisher) setSnapshot(s *ipc.Snapshot) {
	p.last = s
	if p.snapshots != nil {
		p.snapshots(s)
	}
}
