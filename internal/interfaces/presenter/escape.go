package presenter

import (
	"strings"
	"unicode"
	"unicode/utf16"
	"unicode/utf8"
)

const lowerHex = "0123456789abcdef"

// isFormatOrSeparator reports whether r is a format (Cf: bidi overrides
// and isolates, zero-width characters, BOM, soft hyphen, tags) or a line or
// paragraph separator (U+2028, U+2029) rune: runes that change how a line
// reads or breaks without being visible.
func isFormatOrSeparator(r rune) bool {
	return r == 0x2028 || r == 0x2029 || unicode.Is(unicode.Cf, r)
}

// termEscaped reports whether escapeTerm rewrites the valid rune r.
func termEscaped(r rune) bool {
	return r < 0x20 || (r >= 0x7f && r <= 0x9f) || isFormatOrSeparator(r)
}

// escapeTerm returns s with every byte sequence that a terminal could
// interpret replaced by a visible escape; s is returned unchanged (no
// allocation) when nothing needs escaping.
//
//	invalid UTF-8 byte           -> \xNN
//	U+0000-U+001F, U+007F        -> \xNN
//	U+0080-U+009F (C1)           -> \uNNNN
//	Cf, U+2028, U+2029           -> \uNNNN (\UNNNNNNNN above U+FFFF)
//
// Hex is lowercase. Everything else, including '\\', is copied verbatim,
// so a name made of printable characters prints exactly as it is.
func escapeTerm(s string) string {
	i := 0
	for i < len(s) {
		c := s[i]
		if c >= 0x20 && c < 0x7f {
			i++
			continue
		}
		r, size := utf8.DecodeRuneInString(s[i:])
		if (r == utf8.RuneError && size == 1) || termEscaped(r) {
			break
		}
		i += size
	}
	if i == len(s) {
		return s
	}

	var b strings.Builder
	b.Grow(len(s) + 16)
	b.WriteString(s[:i])
	for i < len(s) {
		c := s[i]
		if c >= 0x20 && c < 0x7f {
			b.WriteByte(c)
			i++
			continue
		}
		r, size := utf8.DecodeRuneInString(s[i:])
		switch {
		case r == utf8.RuneError && size == 1, r < 0x20, r == 0x7f:
			b.WriteString(`\x`)
			b.WriteByte(lowerHex[c>>4])
			b.WriteByte(lowerHex[c&0x0f])
		case termEscaped(r):
			writeHexEscape(&b, r)
		default:
			b.WriteString(s[i : i+size])
		}
		i += size
	}
	return b.String()
}

// writeHexEscape writes r as \uNNNN, or \UNNNNNNNN above U+FFFF.
func writeHexEscape(b *strings.Builder, r rune) {
	digits := 4
	if r > 0xffff {
		b.WriteString(`\U`)
		digits = 8
	} else {
		b.WriteString(`\u`)
	}
	for shift := (digits - 1) * 4; shift >= 0; shift -= 4 {
		b.WriteByte(lowerHex[(r>>uint(shift))&0x0f])
	}
}

// jsonEscaped reports whether escapeJSON rewrites the rune r: the runes a
// terminal may act on that encoding/json writes raw.
func jsonEscaped(r rune) bool {
	return (r >= 0x7f && r <= 0x9f) || isFormatOrSeparator(r)
}

// escapeJSON rewrites, in the output of encoding/json, every rune that a
// terminal could act on but encoding/json leaves raw: U+007F, U+0080-U+009F,
// Cf, U+2028 and U+2029 become \uXXXX (lowercase hex; runes above U+FFFF
// as a \uXXXX\uXXXX surrogate pair). Such runes can only occur inside JSON
// strings (every structural byte is ASCII) and never inside an escape
// sequence, so the result decodes to the same values. b itself is
// returned (no allocation) when nothing needs rewriting. A byte that is not
// valid UTF-8 (encoding/json never writes one) is copied unchanged.
func escapeJSON(b []byte) []byte {
	i := 0
	for i < len(b) {
		if b[i] < 0x7f {
			i++
			continue
		}
		r, size := utf8.DecodeRune(b[i:])
		if !(r == utf8.RuneError && size == 1) && jsonEscaped(r) {
			break
		}
		i += size
	}
	if i == len(b) {
		return b
	}

	out := make([]byte, 0, len(b)+32)
	out = append(out, b[:i]...)
	for i < len(b) {
		if b[i] < 0x7f {
			out = append(out, b[i])
			i++
			continue
		}
		r, size := utf8.DecodeRune(b[i:])
		switch {
		case r == utf8.RuneError && size == 1:
			out = append(out, b[i])
		case jsonEscaped(r):
			if r > 0xffff {
				hi, lo := utf16.EncodeRune(r)
				out = appendU4(out, hi)
				out = appendU4(out, lo)
			} else {
				out = appendU4(out, r)
			}
		default:
			out = append(out, b[i:i+size]...)
		}
		i += size
	}
	return out
}

// appendU4 appends the JSON escape \uXXXX for the UTF-16 code unit r.
func appendU4(dst []byte, r rune) []byte {
	return append(dst, '\\', 'u',
		lowerHex[(r>>12)&0x0f], lowerHex[(r>>8)&0x0f],
		lowerHex[(r>>4)&0x0f], lowerHex[r&0x0f])
}
