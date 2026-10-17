package main

import (
	"flag"
	"os"
	"slices"
	"strings"
	"testing"
)

// wantTopUsage is the v0.4 top-level usage, verbatim: the registry must
// reproduce it byte for byte.
const wantTopUsage = `usage: gruntled <command> [arguments]

Commands:
  check   check a Terragrunt repository for broken dependency output references
  graph   print the repository graph as JSON (--json)
  blast   report which units a change breaks (Broken) or puts at risk (Impacted) against --base
  watch   reindex on every save and keep a one-line status file outside the repository
  report  print the running watch daemon's result, like check, or its blast view (--blast)
  rebase  move the running watch daemon's blast baseline to its last result

Flags:
  --version   print the version and commit, then exit

Run "gruntled check -h", "gruntled graph -h", "gruntled blast -h",
"gruntled watch -h", "gruntled report -h" or "gruntled rebase -h" for
details.
`

// TestTopUsageFromRegistry: topUsage is generated from commands and equals
// the v0.4 text; the table is in topUsage order and every entry's usage
// starts with its own name.
func TestTopUsageFromRegistry(t *testing.T) {
	if got := topUsageOf(commands); got != wantTopUsage {
		t.Errorf("topUsageOf(commands) differs from the v0.4 text:\n--- got\n%s\n--- want\n%s", got, wantTopUsage)
	}
	if topUsage != wantTopUsage {
		t.Errorf("topUsage differs from the v0.4 text")
	}
	wantOrder := []string{"check", "graph", "blast", "watch", "report", "rebase"}
	var names []string
	for _, c := range commands {
		names = append(names, c.name)
	}
	if !slices.Equal(names, wantOrder) {
		t.Fatalf("commands order %v, want %v", names, wantOrder)
	}
	for _, c := range commands {
		if !strings.HasPrefix(c.usage, "usage: gruntled "+c.name+" ") {
			t.Errorf("%s: usage does not start with its name:\n%s", c.name, c.usage)
		}
		got, ok := lookupCommand(c.name)
		if !ok || got.usage != c.usage {
			t.Errorf("lookupCommand(%q): ok=%v, usage differs", c.name, ok)
		}
		if c.run == nil || c.flags == nil || c.summary == "" {
			t.Errorf("%s: incomplete registry entry", c.name)
		}
	}
	if _, ok := lookupCommand("nosuch"); ok {
		t.Error(`lookupCommand("nosuch") returned ok`)
	}
}

// TestEnumError: the generated --format message is byte-identical to the
// v0.4 hand-written ones, and the registry carries the enumerated values.
func TestEnumError(t *testing.T) {
	if got, want := enumError("format", "yaml", []string{"text", "json", "sarif"}), "gruntled: invalid --format \"yaml\" (want text, json or sarif)\n"; got != want {
		t.Errorf("enumError three values:\n got %q\nwant %q", got, want)
	}
	if got, want := enumError("format", "yaml", []string{"text", "json"}), "gruntled: invalid --format \"yaml\" (want text or json)\n"; got != want {
		t.Errorf("enumError two values:\n got %q\nwant %q", got, want)
	}
	if got, want := enumError("format", "x", []string{"text"}), "gruntled: invalid --format \"x\" (want text)\n"; got != want {
		t.Errorf("enumError one value:\n got %q\nwant %q", got, want)
	}
	if !enumValid([]string{"text", "json"}, "json") {
		t.Error(`enumValid({text,json}, "json") = false`)
	}
	if enumValid([]string{"text", "json"}, "sarif") {
		t.Error(`enumValid({text,json}, "sarif") = true`)
	}
	want := map[string]map[string][]string{
		"check":  {"format": {"text", "json", "sarif"}},
		"report": {"format": {"text", "json", "sarif"}},
		"blast":  {"format": {"text", "json"}},
		"graph":  nil,
		"watch":  nil,
		"rebase": nil,
	}
	for _, c := range commands {
		w, ok := want[c.name]
		if !ok {
			t.Errorf("%s: unexpected command", c.name)
			continue
		}
		if len(w) != len(c.values) {
			t.Errorf("%s: values %v, want %v", c.name, c.values, w)
			continue
		}
		for k, v := range w {
			if !slices.Equal(c.values[k], v) {
				t.Errorf("%s: values[%q] = %v, want %v", c.name, k, c.values[k], v)
			}
		}
	}
	// Every values key is a defined flag of that command.
	for _, c := range commands {
		fs := flag.NewFlagSet(c.name, flag.ContinueOnError)
		c.flags(fs)
		for k := range c.values {
			if fs.Lookup(k) == nil {
				t.Errorf("%s: values key %q is not a flag", c.name, k)
			}
		}
	}
}

// TestHelpOnStdout: help, -h and --help print the usage on stdout, exit 0,
// nothing on stderr, at top level and for every command; help with an
// unknown or extra argument and every usage error stay on stderr, exit 2.
func TestHelpOnStdout(t *testing.T) {
	for _, args := range [][]string{{"help"}, {"-h"}, {"--help"}} {
		stdout, stderr, code := runCLI(t, args...)
		if code != exitOK || stdout != topUsage || stderr != "" {
			t.Errorf("%v: exit %d, stderr %q, stdout:\n%s", args, code, stderr, stdout)
		}
	}
	for _, c := range commands {
		for _, args := range [][]string{
			{"help", c.name},
			{c.name, "-h"},
			{c.name, "--help"},
			{c.name, "--help", "."},
		} {
			stdout, stderr, code := runCLI(t, args...)
			if code != exitOK || stdout != c.usage || stderr != "" {
				t.Errorf("%v: exit %d, stderr %q, stdout:\n%s", args, code, stderr, stdout)
			}
		}
	}
	stdout, stderr, code := runCLI(t, "help", "nosuch")
	if code != exitUsage || stdout != "" || !strings.Contains(stderr, `gruntled: unknown command "nosuch"`) || !strings.Contains(stderr, topUsage) {
		t.Errorf("help nosuch: exit %d, stdout %q, stderr:\n%s", code, stdout, stderr)
	}
	stdout, stderr, code = runCLI(t, "help", "check", "extra")
	if code != exitUsage || stdout != "" || !strings.Contains(stderr, "gruntled: help takes at most one command") || !strings.Contains(stderr, topUsage) {
		t.Errorf("help check extra: exit %d, stdout %q, stderr:\n%s", code, stdout, stderr)
	}
	stdout, stderr, code = runCLI(t)
	if code != exitUsage || stdout != "" || stderr != topUsage {
		t.Errorf("no args: exit %d, stdout %q, stderr:\n%s", code, stdout, stderr)
	}
	stdout, stderr, code = runCLI(t, "check", "--bogus")
	if code != exitUsage || stdout != "" || !strings.Contains(stderr, "flag provided but not defined: -bogus") || !strings.HasSuffix(stderr, checkUsage) {
		t.Errorf("check --bogus: exit %d, stdout %q, stderr:\n%s", code, stdout, stderr)
	}
	stdout, stderr, code = runCLI(t, "check", "--format", "yaml")
	if code != exitUsage || stdout != "" || stderr != enumError("format", "yaml", formatsCheck)+checkUsage {
		t.Errorf("check --format yaml: exit %d, stdout %q, stderr:\n%s", code, stdout, stderr)
	}
}

// checkCommandDocs ties every flag of c to its usage text and to its
// section of docs/cli.md. It returns one message per problem:
//   - a defined flag with no "  --name" usage line (unless hidden);
//     a single-letter flag must appear as ", -n" on the line of the flag
//     it aliases;
//   - a defined flag (hidden ones included) absent from the command's
//     section of doc: from "usage: gruntled <name> " to the next
//     "usage: gruntled " block;
//   - an enumerated flag whose usage line lacks the "|"-joined values;
//   - a "--word" token in the usage that names no defined flag (a flag of
//     another registered command, written "<other> --word", is allowed);
//   - a hidden key that is not a defined flag.
func checkCommandDocs(c command, doc string) []string {
	var out []string
	fs := flag.NewFlagSet(c.name, flag.ContinueOnError)
	c.flags(fs)
	defined := map[string]bool{}
	usageLines := strings.Split(c.usage, "\n")
	flagLine := func(name string) (string, bool) {
		for _, l := range usageLines {
			rest, ok := strings.CutPrefix(l, "  --"+name)
			if ok && (rest == "" || strings.ContainsAny(rest[:1], " =,")) {
				return l, true
			}
		}
		return "", false
	}
	section := docSection(doc, c.name)
	fs.VisitAll(func(f *flag.Flag) {
		defined[f.Name] = true
		if _, hidden := c.hidden[f.Name]; !hidden {
			if len(f.Name) == 1 {
				found := false
				for _, l := range usageLines {
					if strings.HasPrefix(l, "  --") && strings.Contains(l, ", -"+f.Name+" ") {
						found = true
					}
				}
				if !found {
					out = append(out, "flag -"+f.Name+" missing from usage")
				}
			} else if _, ok := flagLine(f.Name); !ok {
				out = append(out, "flag --"+f.Name+" missing from usage")
			}
		}
		dash := "--" + f.Name
		if len(f.Name) == 1 {
			dash = "-" + f.Name
		}
		if section == "" {
			out = append(out, "docs/cli.md has no usage block for "+c.name)
		} else if !strings.Contains(section, dash) {
			out = append(out, "docs/cli.md lacks "+dash+" for "+c.name)
		}
	})
	for name, values := range c.values {
		l, _ := flagLine(name)
		if !strings.Contains(l, "--"+name+" "+strings.Join(values, "|")) {
			out = append(out, "values for --"+name+" not in usage")
		}
	}
	for name := range c.hidden {
		if !defined[name] {
			out = append(out, "hidden flag --"+name+" is not defined")
		}
	}
	for _, tok := range flagTokens(c.usage) {
		if defined[tok.name] {
			continue
		}
		foreign := false
		for _, o := range commands {
			if o.name != c.name && strings.HasSuffix(tok.before, o.name+" ") {
				foreign = true
			}
		}
		if !foreign {
			out = append(out, "usage names undefined flag --"+tok.name)
		}
	}
	slices.Sort(out)
	return slices.Compact(out)
}

// docSection is the slice of doc from the "usage: gruntled <name> " block
// to the next "usage: gruntled " block, or "" when there is none.
func docSection(doc, name string) string {
	start := strings.Index(doc, "usage: gruntled "+name+" ")
	if start < 0 {
		return ""
	}
	rest := doc[start+1:]
	if end := strings.Index(rest, "\nusage: gruntled "); end >= 0 {
		rest = rest[:end]
	}
	return doc[start:start+1] + rest
}

type flagToken struct {
	name   string // without the leading --
	before string // the text of the line before the token
}

// flagTokens lists every "--word" token (a letter, then letters, digits
// and dashes) in text with the text preceding it on its line.
func flagTokens(text string) []flagToken {
	var out []flagToken
	for _, l := range strings.Split(text, "\n") {
		for i := 0; i+2 < len(l); i++ {
			if l[i] != '-' || l[i+1] != '-' || !isLower(l[i+2]) {
				continue
			}
			j := i + 2
			for j < len(l) && (isLower(l[j]) || l[j] == '-' || (l[j] >= '0' && l[j] <= '9')) {
				j++
			}
			out = append(out, flagToken{name: l[i+2 : j], before: l[:i]})
			i = j - 1
		}
	}
	return out
}

func isLower(b byte) bool { return b >= 'a' && b <= 'z' }

// TestRegistryFlagsDocumented: every flag of every command is in its usage
// text (or hidden with a reason) and in its docs/cli.md section, every
// enumerated value is on the usage line, and the usage names no undefined
// flag.
func TestRegistryFlagsDocumented(t *testing.T) {
	doc := readDocs(t)
	for _, c := range commands {
		for _, m := range checkCommandDocs(c, doc) {
			t.Errorf("%s: %s", c.name, m)
		}
	}
}

func readDocs(t *testing.T) string {
	t.Helper()
	b, err := os.ReadFile("../../docs/cli.md")
	if err != nil {
		t.Fatal(err)
	}
	return strings.ReplaceAll(string(b), "\r\n", "\n")
}

// TestRegistryFlagsDocumentedNegative: each failure mode of
// checkCommandDocs produces exactly one message naming the problem.
func TestRegistryFlagsDocumentedNegative(t *testing.T) {
	synth := func(usage string, values map[string][]string, hidden map[string]string) command {
		return command{
			name:  "synth",
			usage: usage,
			flags: func(fs *flag.FlagSet) {
				fs.String("format", "text", "")
				fs.Bool("zap", false, "")
			},
			values: values,
			hidden: hidden,
		}
	}
	const okUsage = "usage: gruntled synth [--format text|json] [--zap] [path]\n\nFlags:\n  --format text|json   output format\n  --zap                zap it\n"
	const okDoc = "usage: gruntled synth [--format text|json] [--zap] [path]\n  --format text|json\n  --zap\n\nusage: gruntled other --x\n"
	fmtValues := map[string][]string{"format": {"text", "json"}}
	check, _ := lookupCommand("check")
	for _, tc := range []struct {
		name string
		c    command
		doc  string
		want string
	}{
		{"baseline passes", synth(okUsage, fmtValues, nil), okDoc, ""},
		{"flag missing from usage", synth("usage: gruntled synth [--format text|json] [path]\n\nFlags:\n  --format text|json   output format\n", fmtValues, nil), okDoc, "flag --zap missing from usage"},
		{"usage names undefined flag", synth(okUsage+"  --ghost   boo\n", fmtValues, nil), okDoc, "usage names undefined flag --ghost"},
		{"values not in usage", synth(okUsage, map[string][]string{"format": {"text", "xml"}}, nil), okDoc, "values for --format not in usage"},
		{"hidden flag not defined", synth(okUsage, fmtValues, map[string]string{"nosuch": "x"}), okDoc, "hidden flag --nosuch is not defined"},
		{"hidden flag may be absent from usage but not from docs", synth("usage: gruntled synth [--format text|json] [path]\n\nFlags:\n  --format text|json   output format\n", fmtValues, map[string]string{"zap": "reason"}), "usage: gruntled synth [--format text|json] [path]\n  --format text|json\n", "docs/cli.md lacks --zap for synth"},
		{"real command, docs lack the flag", check, "usage: gruntled check [path]\n\nusage: gruntled graph --json [path]\n  --json\n", "docs/cli.md lacks --format for check"},
		{"real command, docs lack the section", check, "usage: gruntled graph --json [path]\n", "docs/cli.md has no usage block for check"},
		{"foreign flag after another command's name is allowed", synth(okUsage+"like blast --base does\n", fmtValues, nil), okDoc, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := checkCommandDocs(tc.c, tc.doc)
			if tc.want == "" {
				if len(got) != 0 {
					t.Fatalf("want no failure, got %q", got)
				}
				return
			}
			if len(got) != 1 || !strings.Contains(got[0], tc.want) {
				t.Fatalf("want exactly one failure containing %q, got %q", tc.want, got)
			}
		})
	}
}

// TestRegistrySelfConsistent: the values that validation rejects against
// are the ones the usage line shows, so enumError, usage and docs agree by
// construction.
func TestRegistrySelfConsistent(t *testing.T) {
	for _, c := range commands {
		for name, values := range c.values {
			joined := "--" + name + " " + strings.Join(values, "|")
			if !strings.Contains(c.usage, "\n  "+joined) {
				t.Errorf("%s: usage has no line %q", c.name, joined)
			}
			msg := enumError(name, "x", values)
			for _, v := range values {
				if !strings.Contains(msg, v) {
					t.Errorf("%s: %q lacks value %q", c.name, msg, v)
				}
			}
			if !strings.HasSuffix(msg, " or "+values[len(values)-1]+")\n") {
				t.Errorf("%s: %q does not end with the last value", c.name, msg)
			}
		}
	}
}
