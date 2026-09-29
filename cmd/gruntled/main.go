// Package main is the composition root of gruntled. It opens the repository
// with os.OpenRoot, wires the Terragrunt loader and Terraform surface reader
// (infrastructure) into the checking use case (application), and hands the
// report to the presenters (interfaces). It parses arguments and maps results
// to exit codes; it holds no analysis logic.
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

Run "gruntled check -h" for details.
`

const checkUsage = `usage: gruntled check [--format text|json] [path]

Check the Terragrunt repository at path (default ".") for dependency
output references that name an output the target module does not declare.
Flags may appear before or after path; "--" ends flag parsing.

Flags:
  --format text|json   output format (default "text")

Exit codes:
  0  analysis completed, no error diagnostics
  1  analysis completed, at least one error diagnostic (GRT001, GRT100)
  2  usage error: unknown command or flag, invalid --format, more than one path
  3  analysis could not run: path missing, not a directory or unreadable, or an internal failure
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
	default:
		fmt.Fprintf(stderr, "gruntled: unknown command %q\n", args[0])
		fmt.Fprint(stderr, topUsage)
		return exitUsage
	}
}

func runCheck(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("check", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.Usage = func() { fmt.Fprint(stderr, checkUsage) }
	format := fs.String("format", "text", "output format: text or json")

	// The stdlib flag package stops at the first positional argument, so
	// parse again after each one; an explicit "--" ends flag parsing.
	var paths []string
	for rest := args; ; {
		if err := fs.Parse(rest); err != nil {
			if errors.Is(err, flag.ErrHelp) {
				return exitOK
			}
			return exitUsage
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
		fmt.Fprint(stderr, checkUsage)
		return exitUsage
	}
	if *format != "text" && *format != "json" {
		fmt.Fprintf(stderr, "gruntled: invalid --format %q (want text or json)\n", *format)
		fmt.Fprint(stderr, checkUsage)
		return exitUsage
	}
	dir := "."
	if len(paths) == 1 {
		dir = paths[0]
	}

	root, err := os.OpenRoot(dir)
	if err != nil {
		fmt.Fprintf(stderr, "gruntled: cannot open repository: %v\n", err)
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
	if *format == "json" {
		err = presenter.JSON(&buf, rep.Graph, rep.Diagnostics)
	} else {
		err = presenter.Text(&buf, rep.Diagnostics)
	}
	if err != nil {
		fmt.Fprintf(stderr, "gruntled: %v\n", err)
		return exitFailure
	}
	if buf.Len() > 0 {
		if _, err := stdout.Write(buf.Bytes()); err != nil {
			fmt.Fprintf(stderr, "gruntled: writing output: %v\n", err)
			return exitFailure
		}
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
