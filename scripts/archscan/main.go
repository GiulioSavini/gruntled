// Command archscan prints every import in the repository's Go source whose
// path matches a regular expression. scripts/check-architecture.sh uses it
// as the source-level engine behind hcl-only-in-infrastructure,
// infrastructure-importers and testsupport-only-in-tests.
//
// It reads import declarations with go/parser (parser.ImportsOnly), not
// with a line-based scan. The awk scan it replaces could be defeated by a
// /* */ comment in front of a spec or by two specs on one line separated by
// ";" inside an import block (02-REVIEW G21). A real parser sees exactly
// the imports the compiler sees, and never matches an import path that only
// appears in a comment or in a string inside a function body.
//
// Build constraints are never evaluated: every *.go file is read, whatever
// its _GOOS/_GOARCH suffix or //go:build line. The scan exists to see the
// files that go list on the host platform cannot.
//
// Usage:
//
//	archscan -match RE [-exclude DIR/]... [-skip-tests]
//
// Each match prints "./path/to/file.go:LINE: import "x"" on stdout. The
// exit status is 0 when the scan ran (matches or not), 1 when at least one
// file's import section did not parse (reported on stderr), and 2 on a bad
// flag, a bad regular expression or an I/O error.
package main

import (
	"errors"
	"flag"
	"fmt"
	"go/parser"
	"go/token"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

// errParse reports that at least one file's import section did not parse.
var errParse = errors.New("archscan: at least one file did not parse")

type excludeList []string

func (e *excludeList) String() string { return strings.Join(*e, ",") }

func (e *excludeList) Set(v string) error {
	v = strings.TrimPrefix(filepath.ToSlash(v), "./")
	if v == "" {
		return errors.New("empty -exclude")
	}
	if !strings.HasSuffix(v, "/") {
		v += "/"
	}
	*e = append(*e, v)
	return nil
}

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, out, errOut io.Writer) int {
	fl := flag.NewFlagSet("archscan", flag.ContinueOnError)
	fl.SetOutput(errOut)
	match := fl.String("match", "", "regular expression matched against the unquoted import path (required)")
	skipTests := fl.Bool("skip-tests", false, "do not scan *_test.go files")
	var excludes excludeList
	fl.Var(&excludes, "exclude", "slash-separated directory prefix, relative to the root, to skip (repeatable)")
	if err := fl.Parse(args); err != nil {
		return 2
	}
	if *match == "" || fl.NArg() != 0 {
		fmt.Fprintln(errOut, "usage: archscan -match RE [-exclude DIR/]... [-skip-tests]")
		return 2
	}
	re, err := regexp.Compile(*match)
	if err != nil {
		fmt.Fprintf(errOut, "archscan: bad -match: %v\n", err)
		return 2
	}
	if err := scan(".", re, excludes, *skipTests, out, errOut); err != nil {
		if errors.Is(err, errParse) {
			return 1
		}
		fmt.Fprintf(errOut, "archscan: %v\n", err)
		return 2
	}
	return 0
}

// scan walks root and prints, to out, every import spec whose unquoted
// path matches re, as "./rel/path.go:LINE: import "x"".
//
// Directories are pruned the way the go tool ignores them: any directory
// below root whose name starts with "." or "_", or is testdata or vendor.
// A directory holding its own go.mod is NOT pruned; the single-module rule
// in check-architecture.sh rejects nested modules on its own.
//
// Files under an excludes prefix (slash-separated, relative to root, ending
// in "/") are skipped, and so are *_test.go files when skipTests is set.
//
// A file whose import section does not parse is reported on errOut and
// makes scan return errParse after the whole tree is walked: it is never
// skipped silently. Any other error (I/O) aborts the walk and is returned.
func scan(root string, re *regexp.Regexp, excludes []string, skipTests bool, out, errOut io.Writer) error {
	fset := token.NewFileSet()
	parseFailed := false
	walkErr := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if d.IsDir() {
			if rel == "." {
				return nil
			}
			name := d.Name()
			if strings.HasPrefix(name, ".") || strings.HasPrefix(name, "_") || name == "testdata" || name == "vendor" {
				return filepath.SkipDir
			}
			if excluded(rel+"/", excludes) {
				return filepath.SkipDir
			}
			return nil
		}
		if !d.Type().IsRegular() || !strings.HasSuffix(d.Name(), ".go") {
			return nil
		}
		if excluded(rel, excludes) {
			return nil
		}
		if skipTests && strings.HasSuffix(d.Name(), "_test.go") {
			return nil
		}
		src, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		display := "./" + rel
		f, perr := parser.ParseFile(fset, display, src, parser.ImportsOnly)
		if perr != nil {
			fmt.Fprintf(errOut, "%s: parse error: %v\n", display, perr)
			parseFailed = true
		}
		if f == nil {
			return nil
		}
		// Report what did parse too: a partial AST still names real imports.
		for _, spec := range f.Imports {
			p, err := strconv.Unquote(spec.Path.Value)
			if err != nil {
				if perr != nil {
					continue // already reported above
				}
				fmt.Fprintf(errOut, "%s: parse error: bad import path %s\n", fset.Position(spec.Path.Pos()), spec.Path.Value)
				parseFailed = true
				continue
			}
			if re.MatchString(p) {
				fmt.Fprintf(out, "%s:%d: import %q\n", display, fset.Position(spec.Path.Pos()).Line, p)
			}
		}
		return nil
	})
	if walkErr != nil {
		return walkErr
	}
	if parseFailed {
		return errParse
	}
	return nil
}

func excluded(rel string, excludes []string) bool {
	for _, e := range excludes {
		if strings.HasPrefix(rel, e) {
			return true
		}
	}
	return false
}
