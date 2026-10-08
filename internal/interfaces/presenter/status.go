package presenter

import (
	"io"
	"sort"
	"strconv"
	"strings"
	"unicode"

	"github.com/GiulioSavini/gruntled/internal/domain/diagnostic"
)

// maxReasonRunes caps the failure reason so the status line stays short
// enough for a shell prompt.
const maxReasonRunes = 120

// StatusLine writes the one-line watch status for diags:
//
//	gruntled: ok @ <stamp>
//	gruntled: 2 errors (GRT001×1 GRT003×1) @ <stamp>
//
// Only SeverityError diagnostics are counted, grouped per code in
// ascending code order. stamp is pre-formatted by the caller. The whole
// line, newline included, goes out in a single write.
func StatusLine(w io.Writer, diags diagnostic.Set, stamp string) error {
	perCode := map[string]int{}
	total := 0
	for _, d := range diags.All() {
		if d.Severity() != diagnostic.SeverityError {
			continue
		}
		perCode[string(d.Code())]++
		total++
	}
	if total == 0 {
		return writeStatus(w, "ok", stamp)
	}
	codes := make([]string, 0, len(perCode))
	for c := range perCode {
		codes = append(codes, c)
	}
	sort.Strings(codes)
	parts := make([]string, len(codes))
	for i, c := range codes {
		parts[i] = c + "×" + strconv.Itoa(perCode[c])
	}
	noun := "errors"
	if total == 1 {
		noun = "error"
	}
	return writeStatus(w, strconv.Itoa(total)+" "+noun+" ("+strings.Join(parts, " ")+")", stamp)
}

// StatusIndexing writes the status line shown while the first index runs.
func StatusIndexing(w io.Writer, stamp string) error {
	return writeStatus(w, "indexing...", stamp)
}

// StatusStopped writes the status line left behind by a clean shutdown.
func StatusStopped(w io.Writer, stamp string) error {
	return writeStatus(w, "stopped", stamp)
}

// SanitizeReason turns every control rune (ESC, NUL, CR, LF, tab, DEL...)
// into a space, then collapses whitespace runs to single spaces and trims
// the ends. It does not cap the length.
func SanitizeReason(s string) string {
	s = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return ' '
		}
		return r
	}, s)
	return strings.Join(strings.Fields(s), " ")
}

// StatusFailed writes the status line for a run that could not produce a
// diagnostic set. reason goes through SanitizeReason, the result is cut to
// 120 runes, and an empty reason reads "unknown error".
func StatusFailed(w io.Writer, reason, stamp string) error {
	r := SanitizeReason(reason)
	if r == "" {
		r = "unknown error"
	}
	if runes := []rune(r); len(runes) > maxReasonRunes {
		r = string(runes[:maxReasonRunes])
	}
	return writeStatus(w, "failed ("+r+")", stamp)
}

func writeStatus(w io.Writer, body, stamp string) error {
	_, err := io.WriteString(w, "gruntled: "+body+" @ "+stamp+"\n")
	return err
}
