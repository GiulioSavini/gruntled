package presenter

import (
	"bytes"
	"io"
	"strconv"

	"github.com/GiulioSavini/gruntled/internal/domain/diagnostic"
)

// Text writes one line per diagnostic, in diags' canonical order:
//
//	path:line:col: CODE message
//
// with " (unit U)" appended when the diagnostic is attributed to a unit,
// so the per-unit findings of a shared include file stay distinguishable.
// Severity is not printed: every v0.1 code is an error, and the JSON
// format carries it. An empty Set writes nothing.
//
// The path, message and unit go through escapeTerm, so a crafted name or
// message cannot write control sequences to the terminal and every
// diagnostic stays on one line; names made of printable characters print
// unchanged.
func Text(w io.Writer, diags diagnostic.Set) error {
	var b bytes.Buffer
	for _, d := range diags.All() {
		p := d.Pos()
		b.WriteString(escapeTerm(p.File().String()))
		b.WriteByte(':')
		b.WriteString(strconv.Itoa(p.Line()))
		b.WriteByte(':')
		b.WriteString(strconv.Itoa(p.Column()))
		b.WriteString(": ")
		b.WriteString(string(d.Code()))
		b.WriteByte(' ')
		b.WriteString(escapeTerm(d.Message()))
		if u, ok := d.Unit(); ok {
			b.WriteString(" (unit ")
			b.WriteString(escapeTerm(u.String()))
			b.WriteByte(')')
		}
		b.WriteByte('\n')
	}
	if b.Len() == 0 {
		return nil
	}
	_, err := w.Write(b.Bytes())
	return err
}
