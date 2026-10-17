package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"slices"
	"strings"
)

// command is one entry of the registry: everything the CLI surface knows
// about a subcommand (name, one-line summary, usage text, flag definer,
// enumerated flag values, deliberately undocumented flags, entry point).
// The top-level usage, dispatch, --format validation, the drift test and the
// completions/man generators all read this table and nothing else.
type command struct {
	name    string
	summary string
	usage   string
	// flags registers the command's flags on fs.
	flags func(*flag.FlagSet)
	// values lists the accepted values of an enumerated flag, in the order
	// the usage line and the error message show them.
	values map[string][]string
	// hidden maps a defined flag that is absent from the usage text to the
	// reason; it must still be documented in docs/cli.md.
	hidden map[string]string
	run    func(ctx context.Context, args []string, stdout, stderr io.Writer) int
}

// Enumerated --format values. 19-03 extends formatsBlast.
var (
	formatsCheck = []string{"text", "json", "sarif"}
	formatsBlast = []string{"text", "json"}
)

// commands is the registry, in top-level usage order.
var commands = []command{
	{
		name:    "check",
		summary: "check a Terragrunt repository for broken dependency output references",
		usage:   checkUsage,
		flags:   func(fs *flag.FlagSet) { defineCheckFlags(fs) },
		values:  map[string][]string{"format": formatsCheck},
		run: func(_ context.Context, args []string, stdout, stderr io.Writer) int {
			return runCheck(args, stdout, stderr)
		},
	},
	{
		name:    "graph",
		summary: "print the repository graph as JSON (--json)",
		usage:   graphUsage,
		flags:   func(fs *flag.FlagSet) { defineGraphFlags(fs) },
		run: func(_ context.Context, args []string, stdout, stderr io.Writer) int {
			return runGraph(args, stdout, stderr)
		},
	},
	{
		name:    "blast",
		summary: "report which units a change breaks (Broken) or puts at risk (Impacted) against --base",
		usage:   blastUsage,
		flags:   func(fs *flag.FlagSet) { defineBlastFlags(fs) },
		values:  map[string][]string{"format": formatsBlast},
		run: func(_ context.Context, args []string, stdout, stderr io.Writer) int {
			return runBlast(args, stdout, stderr)
		},
	},
	{
		name:    "watch",
		summary: "reindex on every save and keep a one-line status file outside the repository",
		usage:   watchUsage,
		flags:   func(fs *flag.FlagSet) { defineWatchFlags(fs) },
		run:     runWatch,
	},
	{
		name:    "report",
		summary: "print the running watch daemon's result, like check, or its blast view (--blast)",
		usage:   reportUsage,
		flags:   func(fs *flag.FlagSet) { defineReportFlags(fs) },
		values:  map[string][]string{"format": formatsCheck},
		hidden:  map[string]string{"depth": "accepted only to print a pointer to blast --depth"},
		run: func(_ context.Context, args []string, stdout, stderr io.Writer) int {
			return runReport(args, stdout, stderr)
		},
	},
	{
		name:    "rebase",
		summary: "move the running watch daemon's blast baseline to its last result",
		usage:   rebaseUsage,
		flags:   func(fs *flag.FlagSet) { defineRebaseFlags(fs) },
		run: func(_ context.Context, args []string, stdout, stderr io.Writer) int {
			return runRebase(args, stdout, stderr)
		},
	},
}

// topUsage is the top-level usage, generated from the registry.
var topUsage = topUsageOf(commands)

// topUsageOf renders the top-level usage of cmds: one summary line per
// command, the global flags, and the pointer to the per-command help.
func topUsageOf(cmds []command) string {
	var b strings.Builder
	b.WriteString("usage: gruntled <command> [arguments]\n\nCommands:\n")
	for _, c := range cmds {
		fmt.Fprintf(&b, "  %-7s %s\n", c.name, c.summary)
	}
	b.WriteString("\nFlags:\n  --version   print the version and commit, then exit\n\nRun ")
	// `"gruntled X -h", ` per command, joined as today: three per line, the
	// last one introduced by "or".
	for i, c := range cmds {
		switch {
		case i == 0:
		case i == len(cmds)-1:
			b.WriteString(" or ")
		case i%3 == 0:
			b.WriteString(",\n")
		default:
			b.WriteString(", ")
		}
		fmt.Fprintf(&b, "%q", "gruntled "+c.name+" -h")
	}
	b.WriteString(" for\ndetails.\n")
	return b.String()
}

// lookupCommand returns the registry entry named name.
func lookupCommand(name string) (command, bool) {
	for _, c := range commands {
		if c.name == name {
			return c, true
		}
	}
	return command{}, false
}

// enumValid reports whether got is one of values.
func enumValid(values []string, got string) bool {
	return slices.Contains(values, got)
}

// enumError is the usage error for an enumerated flag, one line with the
// trailing newline: `gruntled: invalid --format "x" (want text, json or sarif)`.
func enumError(flag, got string, values []string) string {
	n := len(values)
	var want string
	switch {
	case n == 0:
		want = ""
	case n == 1:
		want = values[0]
	default:
		want = strings.Join(values[:n-1], ", ") + " or " + values[n-1]
	}
	return fmt.Sprintf("gruntled: invalid --%s %q (want %s)\n", flag, got, want)
}

// checkOpts holds the parsed check flags.
type checkOpts struct {
	format *string
}

func defineCheckFlags(fs *flag.FlagSet) *checkOpts {
	return &checkOpts{
		format: fs.String("format", "text", "output format: text, json or sarif"),
	}
}

// graphOpts holds the parsed graph flags.
type graphOpts struct {
	json *bool
}

func defineGraphFlags(fs *flag.FlagSet) *graphOpts {
	return &graphOpts{
		json: fs.Bool("json", false, "print the graph as JSON (required)"),
	}
}

// blastOpts holds the parsed blast flags.
type blastOpts struct {
	base, format *string
	depth        *depthFlag
}

func defineBlastFlags(fs *flag.FlagSet) *blastOpts {
	o := &blastOpts{
		base:   fs.String("base", "", "baseline tree to compare against"),
		format: fs.String("format", "text", "output format: text or json"),
		depth:  &depthFlag{},
	}
	fs.Var(o.depth, "depth", "list Impacted units up to distance N")
	return o
}

// reportOpts holds the parsed report flags. --depth is accepted only to
// print a pointer to blast --depth (hidden in the registry).
type reportOpts struct {
	format *string
	blast  *bool
	depth  *depthFlag
}

func defineReportFlags(fs *flag.FlagSet) *reportOpts {
	o := &reportOpts{
		format: fs.String("format", "text", "output format: text, json or sarif"),
		blast:  fs.Bool("blast", false, "print the daemon's blast view"),
		depth:  &depthFlag{},
	}
	fs.Var(o.depth, "depth", "not supported by report")
	return o
}

// rebaseOpts holds the parsed rebase flags.
type rebaseOpts struct {
	expect *depthFlag
}

func defineRebaseFlags(fs *flag.FlagSet) *rebaseOpts {
	o := &rebaseOpts{expect: &depthFlag{}}
	fs.Var(o.expect, "expect", "rebase only if the daemon is at generation N")
	return o
}
