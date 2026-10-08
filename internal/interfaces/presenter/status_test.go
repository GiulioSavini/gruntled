package presenter_test

import (
	"bytes"
	"errors"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/GiulioSavini/gruntled/internal/domain/diagnostic"
	"github.com/GiulioSavini/gruntled/internal/interfaces/presenter"
)

const stamp = "14:02:11"

func codeDiag(t *testing.T, code diagnostic.Code, sev diagnostic.Severity, line int) diagnostic.Diagnostic {
	t.Helper()
	d, err := diagnostic.New(code, sev, pos(t, "live/app/terragrunt.hcl", line, 1), "m")
	if err != nil {
		t.Fatalf("diagnostic.New: %v", err)
	}
	return d
}

// countingWriter records how many Write calls it received.
type countingWriter struct {
	bytes.Buffer
	calls int
}

func (c *countingWriter) Write(p []byte) (int, error) {
	c.calls++
	return c.Buffer.Write(p)
}

func TestStatusLine(t *testing.T) {
	e := diagnostic.SeverityError
	w := diagnostic.SeverityWarning
	cases := []struct {
		name  string
		diags func(t *testing.T) []diagnostic.Diagnostic
		want  string
	}{
		{"ok", func(*testing.T) []diagnostic.Diagnostic { return nil },
			"gruntled: ok @ 14:02:11\n"},
		{"one error", func(t *testing.T) []diagnostic.Diagnostic {
			return []diagnostic.Diagnostic{codeDiag(t, diagnostic.CodeUnknownOutput, e, 1)}
		}, "gruntled: 1 error (GRT001×1) @ 14:02:11\n"},
		{"two errors sorted codes", func(t *testing.T) []diagnostic.Diagnostic {
			return []diagnostic.Diagnostic{
				codeDiag(t, diagnostic.CodeDependencyCycle, e, 1),
				codeDiag(t, diagnostic.CodeUnknownOutput, e, 2),
			}
		}, "gruntled: 2 errors (GRT001×1 GRT003×1) @ 14:02:11\n"},
		{"three errors counted per code", func(t *testing.T) []diagnostic.Diagnostic {
			return []diagnostic.Diagnostic{
				codeDiag(t, diagnostic.CodeMissingDependencyTarget, e, 3),
				codeDiag(t, diagnostic.CodeUnknownOutput, e, 2),
				codeDiag(t, diagnostic.CodeUnknownOutput, e, 1),
			}
		}, "gruntled: 3 errors (GRT001×2 GRT002×1) @ 14:02:11\n"},
		{"warnings only", func(t *testing.T) []diagnostic.Diagnostic {
			return []diagnostic.Diagnostic{codeDiag(t, diagnostic.CodeUnknownOutput, w, 1)}
		}, "gruntled: ok @ 14:02:11\n"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var b countingWriter
			if err := presenter.StatusLine(&b, diagnostic.NewSet(tc.diags(t)...), stamp); err != nil {
				t.Fatalf("StatusLine: %v", err)
			}
			if got := b.String(); got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
			if b.calls != 1 {
				t.Fatalf("Write calls = %d, want 1", b.calls)
			}
		})
	}
}

func TestStatusLineFixedStates(t *testing.T) {
	cases := []struct {
		name  string
		write func(b *bytes.Buffer) error
		want  string
	}{
		{"indexing", func(b *bytes.Buffer) error { return presenter.StatusIndexing(b, stamp) }, "gruntled: indexing... @ 14:02:11\n"},
		{"stopped", func(b *bytes.Buffer) error { return presenter.StatusStopped(b, stamp) }, "gruntled: stopped @ 14:02:11\n"},
		{"failed", func(b *bytes.Buffer) error { return presenter.StatusFailed(b, "boom", stamp) }, "gruntled: failed (boom) @ 14:02:11\n"},
		{"failed empty", func(b *bytes.Buffer) error { return presenter.StatusFailed(b, "", stamp) }, "gruntled: failed (unknown error) @ 14:02:11\n"},
		{"failed blank", func(b *bytes.Buffer) error { return presenter.StatusFailed(b, " \n\t ", stamp) }, "gruntled: failed (unknown error) @ 14:02:11\n"},
		{"failed whitespace collapses", func(b *bytes.Buffer) error {
			return presenter.StatusFailed(b, "open a:\n\tno such   file\r\n", stamp)
		}, "gruntled: failed (open a: no such file) @ 14:02:11\n"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var b bytes.Buffer
			if err := tc.write(&b); err != nil {
				t.Fatalf("write: %v", err)
			}
			if got := b.String(); got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestStatusLineFailedTruncation(t *testing.T) {
	// 130 two-byte runes: a byte-based cut would split a rune.
	reason := strings.Repeat("é", 130)
	var b bytes.Buffer
	if err := presenter.StatusFailed(&b, reason, stamp); err != nil {
		t.Fatalf("StatusFailed: %v", err)
	}
	want := "gruntled: failed (" + strings.Repeat("é", 120) + ") @ 14:02:11\n"
	if got := b.String(); got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
	if !utf8.ValidString(b.String()) {
		t.Fatal("output is not valid UTF-8")
	}
}

func TestStatusLineEncoding(t *testing.T) {
	var b bytes.Buffer
	set := diagnostic.NewSet(codeDiag(t, diagnostic.CodeUnknownOutput, diagnostic.SeverityError, 1))
	if err := presenter.StatusLine(&b, set, stamp); err != nil {
		t.Fatalf("StatusLine: %v", err)
	}
	out := b.Bytes()
	if bytes.HasPrefix(out, []byte{0xEF, 0xBB, 0xBF}) {
		t.Fatal("output starts with a BOM")
	}
	if !bytes.Contains(out, []byte{0xC3, 0x97}) {
		t.Fatalf("output %q lacks U+00D7 encoded as UTF-8", out)
	}
	if bytes.Count(out, []byte("\n")) != 1 || !bytes.HasSuffix(out, []byte("\n")) {
		t.Fatalf("output %q is not exactly one line", out)
	}
}

func TestStatusLineWriterError(t *testing.T) {
	werr := failingWriter{err: errors.New("boom")}
	if err := presenter.StatusLine(werr, diagnostic.NewSet(), stamp); err == nil {
		t.Fatal("StatusLine: want writer error")
	}
	if err := presenter.StatusFailed(werr, "x", stamp); err == nil {
		t.Fatal("StatusFailed: want writer error")
	}
}
