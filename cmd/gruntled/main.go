// Package main is the composition root of gruntled. It opens the repository
// with os.OpenRoot, wires the Terragrunt loader and Terraform surface reader
// (infrastructure) into the use cases (application), and hands the results
// to the presenters (interfaces): check runs the analyzers, graph only
// builds the repository graph. It parses arguments and maps results to exit
// codes; it holds no analysis logic.
package main

import (
	"bytes"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/GiulioSavini/gruntled/internal/application/checking"
	"github.com/GiulioSavini/gruntled/internal/application/indexing"
	"github.com/GiulioSavini/gruntled/internal/infrastructure/terragrunt"
	"github.com/GiulioSavini/gruntled/internal/infrastructure/tfsurface"
	"github.com/GiulioSavini/gruntled/internal/interfaces/presenter"
)

const (
	exitOK       = 0
	exitFindings = 1
	exitUsage    = 2
	exitFailure  = 3
)

const topUsage = `usage: gruntled <command> [arguments]

Commands:
  check   check a Terragrunt repository for broken dependency output references
  graph   print the repository graph as JSON (--json)

Run "gruntled check -h" or "gruntled graph -h" for details.
`

const checkUsage = `usage: gruntled check [--format text|json|sarif] [path]

Check the Terragrunt repository at path (default ".") for dependency
output references that name an output the target module does not declare.
Flags may appear before or after path; "--" ends flag parsing.

Flags:
  --format text|json|sarif   output format (default "text")

Exit codes:
  0  analysis completed, no error diagnostics
  1  analysis completed, at least one error diagnostic (GRT001-GRT003, GRT100)
  2  usage error: unknown command or flag, invalid --format, more than one path
  3  analysis could not run: path missing, not a directory or unreadable, or an internal failure
`

const graphUsage = `usage: gruntled graph --json [path]

Print the dependency graph of the Terragrunt repository at path (default ".")
as a JSON document. No analyzers run; unknown units and modules are reported
in the document, not as failures.
Flags may appear before or after path; "--" ends flag parsing.

Flags:
  --json   print the graph as JSON (--json is required; text output is reserved)

Exit codes:
  0  graph printed, even with unknown units
  2  usage error: unknown flag, missing --json, more than one path
  3  analysis could not run: path missing, not a directory or unreadable, an internal failure, or stdout write failed
`

func main() { os.Exit(run(os.Args[1:], os.Stdout, os.Stderr)) }

func run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprint(stderr, topUsage)
		return exitUsage
	}
	switch args[0] {
	case "-h", "-help", "--help", "help":
		fmt.Fprint(stderr, topUsage)
		return exitOK
	case "check":
		return runCheck(args[1:], stdout, stderr)
	case "graph":
		return runGraph(args[1:], stdout, stderr)
	default:
		fmt.Fprintf(stderr, "gruntled: unknown command %q\n", args[0])
		fmt.Fprint(stderr, topUsage)
		return exitUsage
	}
}

// parseArgs parses flags defined by define and at most one positional path.
// The stdlib flag package stops at the first positional argument, so it
// parses again after each one; an explicit "--" ends flag parsing. When done
// is true the caller returns code immediately.
func parseArgs(name, usage string, args []string, stderr io.Writer, define func(*flag.FlagSet)) (dir string, code int, done bool) {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.Usage = func() { fmt.Fprint(stderr, usage) }
	define(fs)

	var paths []string
	for rest := args; ; {
		if err := fs.Parse(rest); err != nil {
			if errors.Is(err, flag.ErrHelp) {
				return "", exitOK, true
			}
			return "", exitUsage, true
		}
		if fs.NArg() == 0 {
			break
		}
		if consumed := len(rest) - fs.NArg(); consumed > 0 && rest[consumed-1] == "--" {
			paths = append(paths, fs.Args()...)
			break
		}
		paths = append(paths, fs.Arg(0))
		rest = fs.Args()[1:]
	}
	if len(paths) > 1 {
		fmt.Fprintln(stderr, "gruntled: more than one path given")
		fmt.Fprint(stderr, usage)
		return "", exitUsage, true
	}
	dir = "."
	if len(paths) == 1 {
		dir = paths[0]
	}
	return dir, exitOK, false
}

// openRepo opens dir as an os.Root; the caller closes it.
func openRepo(dir string, stderr io.Writer) (*os.Root, bool) {
	root, err := os.OpenRoot(dir)
	if err != nil {
		fmt.Fprintf(stderr, "gruntled: cannot open repository: %v\n", err)
		return nil, false
	}
	return root, true
}

// writeOut writes buf to stdout in one call; an empty buffer writes nothing.
func writeOut(stdout, stderr io.Writer, buf *bytes.Buffer) bool {
	if buf.Len() == 0 {
		return true
	}
	if _, err := stdout.Write(buf.Bytes()); err != nil {
		fmt.Fprintf(stderr, "gruntled: writing output: %v\n", err)
		return false
	}
	return true
}

func runCheck(args []string, stdout, stderr io.Writer) int {
	var format *string
	dir, code, done := parseArgs("check", checkUsage, args, stderr, func(fs *flag.FlagSet) {
		format = fs.String("format", "text", "output format: text, json or sarif")
	})
	if done {
		return code
	}
	switch *format {
	case "text", "json", "sarif":
	default:
		fmt.Fprintf(stderr, "gruntled: invalid --format %q (want text, json or sarif)\n", *format)
		fmt.Fprint(stderr, checkUsage)
		return exitUsage
	}

	root, ok := openRepo(dir, stderr)
	if !ok {
		return exitFailure
	}
	defer root.Close()
	fsys := root.FS()

	rep, err := checking.Check(context.Background(), terragrunt.NewLoader(fsys), tfsurface.NewReader(fsys))
	if err != nil {
		fmt.Fprintf(stderr, "gruntled: %v\n", err)
		return exitFailure
	}

	var buf bytes.Buffer
	switch *format {
	case "json":
		err = presenter.JSON(&buf, rep.Graph, rep.Diagnostics)
	case "sarif":
		err = presenter.SARIF(&buf, rep.Graph, rep.Diagnostics, presenter.ToolInfo{Version: "dev"})
	default:
		err = presenter.Text(&buf, rep.Diagnostics)
	}
	if err != nil {
		fmt.Fprintf(stderr, "gruntled: %v\n", err)
		return exitFailure
	}
	if !writeOut(stdout, stderr, &buf) {
		return exitFailure
	}
	if *format == "text" {
		if err := presenter.Summary(stderr, rep.Graph, rep.Diagnostics); err != nil {
			return exitFailure
		}
	}
	if rep.Diagnostics.HasErrors() {
		return exitFindings
	}
	return exitOK
}

func runGraph(args []string, stdout, stderr io.Writer) int {
	var asJSON *bool
	dir, code, done := parseArgs("graph", graphUsage, args, stderr, func(fs *flag.FlagSet) {
		asJSON = fs.Bool("json", false, "print the graph as JSON (required)")
	})
	if done {
		return code
	}
	if !*asJSON {
		fmt.Fprintln(stderr, "gruntled: graph: --json is required; text output is reserved")
		fmt.Fprint(stderr, graphUsage)
		return exitUsage
	}

	root, ok := openRepo(dir, stderr)
	if !ok {
		return exitFailure
	}
	defer root.Close()
	fsys := root.FS()

	res, err := indexing.Build(context.Background(), terragrunt.NewLoader(fsys), tfsurface.NewReader(fsys))
	if err != nil {
		fmt.Fprintf(stderr, "gruntled: %v\n", err)
		return exitFailure
	}

	var buf bytes.Buffer
	if err := presenter.Graph(&buf, res.Graph); err != nil {
		fmt.Fprintf(stderr, "gruntled: %v\n", err)
		return exitFailure
	}
	if !writeOut(stdout, stderr, &buf) {
		return exitFailure
	}
	return exitOK
}
