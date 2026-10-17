package main

import (
	"flag"
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
