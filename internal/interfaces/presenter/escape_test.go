package presenter

import (
	"bytes"
	"encoding/json"
	"reflect"
	"testing"
	"unicode"
	"unicode/utf8"
)

// termEscapeCases is the escapeTerm table; it also seeds FuzzEscapeTerm.
var termEscapeCases = []struct {
	name, in, want string
}{
	{"empty", "", ""},
	{"plain path", "live/app/terragrunt.hcl", "live/app/terragrunt.hcl"},
	{"osc title", "a\x1b]0;PWNED\x07b", `a\x1b]0;PWNED\x07b`},
	{"tab lf cr", "\t\n\r", `\x09\x0a\x0d`},
	{"del", "\x7f", `\x7f`},
	{"c1 nel", "\u0085", `\u0085`},
	{"c1 csi", "\u009b2J", `\u009b2J`},
	{"rlo", "\u202e", `\u202e`},
	{"zwsp bom lri", "\u200b\ufeff\u2066", `\u200b\ufeff\u2066`},
	{"line para sep", "\u2028\u2029", `\u2028\u2029`},
	{"tag above bmp", "\U000e0001", `\U000e0001`},
	{"invalid utf8", "\xff\xfe", `\xff\xfe`},
	{"raw c1 byte", "a\x9bb", `a\x9bb`},
	{"non-ascii kept", "é日本🙂", "é日本🙂"},
	{"backslash kept", `a\b`, `a\b`},
	{"mixed", "x/\x1b[2Jé\u202ey", `x/\x1b[2Jé\u202ey`},
}

func TestEscapeTerm(t *testing.T) {
	for _, tc := range termEscapeCases {
		t.Run(tc.name, func(t *testing.T) {
			if got := escapeTerm(tc.in); got != tc.want {
				t.Fatalf("escapeTerm(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestEscapeTermNoAllocForNormalNames(t *testing.T) {
	for _, s := range []string{"live/app/terragrunt.hcl", "é日本🙂/terragrunt.hcl"} {
		n := testing.AllocsPerRun(100, func() { _ = escapeTerm(s) })
		if n != 0 {
			t.Errorf("escapeTerm(%q) allocates %v times, want 0", s, n)
		}
	}
}

// termNeedsEscape reports whether a decoded rune is one escapeTerm must
// rewrite (invalid UTF-8 is checked separately).
func termNeedsEscape(r rune) bool {
	return r < 0x20 || (r >= 0x7f && r <= 0x9f) || r == 0x2028 || r == 0x2029 || unicode.Is(unicode.Cf, r)
}

// hasTermUnsafe reports whether s holds a byte sequence escapeTerm must
// rewrite.
func hasTermUnsafe(s string) bool {
	for i := 0; i < len(s); {
		r, size := utf8.DecodeRuneInString(s[i:])
		if r == utf8.RuneError && size == 1 {
			return true
		}
		if termNeedsEscape(r) {
			return true
		}
		i += size
	}
	return false
}

func FuzzEscapeTerm(f *testing.F) {
	for _, tc := range termEscapeCases {
		f.Add(tc.in)
	}
	f.Fuzz(func(t *testing.T, s string) {
		got := escapeTerm(s)
		if !utf8.ValidString(got) {
			t.Fatalf("escapeTerm(%q) = %q: not valid UTF-8", s, got)
		}
		if hasTermUnsafe(got) {
			t.Fatalf("escapeTerm(%q) = %q: still holds a rune to escape", s, got)
		}
		if !hasTermUnsafe(s) && got != s {
			t.Fatalf("escapeTerm(%q) = %q: changed a string with nothing to escape", s, got)
		}
	})
}

// jsonEscapeCases is the escapeJSON table (inputs are encoding/json
// output); it also seeds FuzzEscapeJSON.
var jsonEscapeCases = []struct {
	name, in, want string
}{
	{"plain", `{"a":"x"}`, `{"a":"x"}`},
	{"del", "\"\x7f\"", `"\u007f"`},
	{"c1 csi", "\"\u009b2J\"", `"\u009b2J"`},
	{"rlo", "\"a\u202eb\"", `"a\u202eb"`},
	{"bom", "\"\ufeff\"", `"\ufeff"`},
	{"line para sep", "\"\u2028\u2029\"", `"\u2028\u2029"`},
	{"tag above bmp", "\"\U000e0001\"", `"\udb40\udc01"`},
	{"non-ascii kept", `"é日本🙂"`, `"é日本🙂"`},
	{"escaped esc kept", `"\u001b"`, `"\u001b"`},
	{"literal backslash-u kept", `"\\u202e"`, `"\\u202e"`},
}

func TestEscapeJSON(t *testing.T) {
	for _, tc := range jsonEscapeCases {
		t.Run(tc.name, func(t *testing.T) {
			if got := string(escapeJSON([]byte(tc.in))); got != tc.want {
				t.Fatalf("escapeJSON(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestEscapeJSONReturnsInputWhenNothingToEscape(t *testing.T) {
	for _, s := range []string{`{"a":"x"}`, `{"a":"é日本🙂"}`} {
		in := []byte(s)
		out := escapeJSON(in)
		if len(out) == 0 || &out[0] != &in[0] || len(out) != len(in) {
			t.Errorf("escapeJSON(%q) did not return its input slice", s)
		}
		n := testing.AllocsPerRun(100, func() { _ = escapeJSON(in) })
		if n != 0 {
			t.Errorf("escapeJSON(%q) allocates %v times, want 0", s, n)
		}
	}
}

// jsonNeedsEscape reports whether r is a rune escapeJSON must rewrite.
func jsonNeedsEscape(r rune) bool {
	return (r >= 0x7f && r <= 0x9f) || r == 0x2028 || r == 0x2029 || unicode.Is(unicode.Cf, r)
}

func hasJSONUnsafe(b []byte) bool {
	for i := 0; i < len(b); {
		r, size := utf8.DecodeRune(b[i:])
		if size == 1 && r == utf8.RuneError {
			i++
			continue
		}
		if jsonNeedsEscape(r) {
			return true
		}
		i += size
	}
	return false
}

// encodeLikePresenter encodes v with the encoder settings every presenter
// JSON writer uses.
func encodeLikePresenter(t *testing.T, v any) []byte {
	t.Helper()
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		t.Fatalf("Encode: %v", err)
	}
	return b.Bytes()
}

func TestEscapeJSONRoundTrip(t *testing.T) {
	type doc struct {
		Esc, Del, Nel, Csi, Rlo, Zwsp, Sep, Tag, Mixed, Plain string
		List                                                  []string
	}
	in := doc{
		Esc:   "a\x1bb",
		Del:   "a\x7fb",
		Nel:   "a\u0085b",
		Csi:   "c\u009b2Jd",
		Rlo:   "a\u202eb",
		Zwsp:  "a\u200bb",
		Sep:   "a\u2028b\u2029",
		Tag:   "a\U000e0001b",
		Mixed: "é🙂<&>\\u202e",
		Plain: "live/app/terragrunt.hcl",
		List:  []string{"\u009b", "x\u00ady"},
	}
	raw := encodeLikePresenter(t, in)
	esc := escapeJSON(raw)
	if !json.Valid(esc) {
		t.Fatalf("escaped JSON is not valid:\n%s", esc)
	}
	var fromRaw, fromEsc doc
	if err := json.Unmarshal(raw, &fromRaw); err != nil {
		t.Fatalf("Unmarshal raw: %v", err)
	}
	if err := json.Unmarshal(esc, &fromEsc); err != nil {
		t.Fatalf("Unmarshal escaped: %v", err)
	}
	if !reflect.DeepEqual(fromEsc, in) || !reflect.DeepEqual(fromEsc, fromRaw) {
		t.Fatalf("round trip differs:\n got %+v\nwant %+v", fromEsc, in)
	}
	if hasJSONUnsafe(esc) {
		t.Fatalf("escaped JSON still holds a raw rune to escape:\n%q", esc)
	}
	if bytes.IndexByte(esc, 0x7f) >= 0 {
		t.Fatalf("escaped JSON holds a raw DEL:\n%q", esc)
	}
}

func FuzzEscapeJSON(f *testing.F) {
	for _, tc := range jsonEscapeCases {
		f.Add(tc.in)
	}
	f.Add("a\x1b]0;PWNED\x07b\u009b\u202e\U000e0001\x7f\xff")
	f.Fuzz(func(t *testing.T, s string) {
		raw, err := json.Marshal(s)
		if err != nil {
			t.Fatalf("Marshal: %v", err)
		}
		esc := escapeJSON(raw)
		var a, b string
		if err := json.Unmarshal(raw, &a); err != nil {
			t.Fatalf("Unmarshal raw %q: %v", raw, err)
		}
		if err := json.Unmarshal(esc, &b); err != nil {
			t.Fatalf("Unmarshal escaped %q: %v", esc, err)
		}
		if a != b {
			t.Fatalf("escapeJSON changed the value: raw %q -> %q, escaped %q -> %q", raw, a, esc, b)
		}
		if hasJSONUnsafe(esc) || bytes.IndexByte(esc, 0x7f) >= 0 {
			t.Fatalf("escapeJSON(%q) = %q: still holds a raw rune to escape", raw, esc)
		}
	})
}
