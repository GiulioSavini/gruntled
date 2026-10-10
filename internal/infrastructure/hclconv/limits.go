package hclconv

import (
	"errors"
	"io"
	"io/fs"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
)

const (
	// MaxFileBytes is the largest source file ReadFileLimited will read.
	// hclsyntax.LexConfig lexes the whole file eagerly regardless of what
	// it contains (about 1.4 GB RSS observed for a 4 MiB single-character-
	// token input in the 02-10 planning probe), so this bound also caps
	// memory, not only recursion.
	MaxFileBytes = 4 << 20 // 4 MiB

	// MaxNestingDepth is the largest bracket/quote/heredoc/template/unary
	// nesting depth CheckNativeDepth and CheckJSONDepth accept. The 02-10
	// planning probe found the smallest real fatal-stack-overflow crash
	// point at 60k levels (nested template interpolation); 1000 leaves at
	// least 12x headroom under it, and no hand-written HCL nests anywhere
	// near 1000.
	MaxNestingDepth = 1000

	// MaxExpressionChain is the largest number of binary-operator, `.` and
	// `[` links one expression may chain. hclsyntax parses such a chain in
	// a loop, so it never overflows the parser itself, but it builds a
	// left-leaning AST as deep as the chain, and every later recursive walk
	// (reference extraction, Variables) descends it: a 2M-long `+` chain
	// peaked at about 2.2 GB (02-REVIEW G17). A realistic expression chains
	// a few dozen links.
	MaxExpressionChain = 10_000
)

// ErrFileTooLarge is returned by ReadFileLimited when a file exceeds
// MaxFileBytes.
var ErrFileTooLarge = errors.New("hclconv: file exceeds MaxFileBytes")

// ErrNotRegularFile is returned (wrapped in an *fs.PathError) by
// ReadFileLimited when name is not a regular file: a FIFO, socket, device
// or directory.
var ErrNotRegularFile = errors.New("hclconv: not a regular file")

// ErrNestingTooDeep is returned by CheckNativeDepth and CheckJSONDepth when
// src's nesting exceeds MaxNestingDepth.
var ErrNestingTooDeep = errors.New("hclconv: nesting exceeds MaxNestingDepth")

// ReadFileLimited reads name from fsys, refusing it if it exceeds
// MaxFileBytes (02-REVIEW G7): hclsyntax.ParseConfig and hcl/json.Parse
// both recurse over their input, and Go cannot recover from the stack
// overflow a hostile or generated file can cause, so the only defence is
// refusing to parse it in the first place.
//
// It also refuses anything that is not a regular file, with
// ErrNotRegularFile, BEFORE opening it (02-REVIEW G18): opening a FIFO for
// reading blocks until a writer appears, so one FIFO named like a config
// or module file used to hang the whole process. fs.Stat is checked
// first, so a non-regular or obviously oversize file is never opened; a
// Stat error is returned as-is. The file is then opened exactly once —
// callers that need to prove a file is read at most once may rely on that
// — its open handle is checked to be regular again, and it is read
// through io.LimitReader(f, MaxFileBytes+1), so memory stays bounded even
// when Stat under-reported the size (no fs.FS is required to avoid that)
// or the file grew in between; more than MaxFileBytes read is
// ErrFileTooLarge.
//
// Residual: a regular file swapped for a FIFO between the Stat and the
// Open could still block in Open, because fs.FS has no non-blocking open.
// That needs someone racing gruntled on its own checkout.
func ReadFileLimited(fsys fs.FS, name string) ([]byte, error) {
	info, err := fs.Stat(fsys, name)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, &fs.PathError{Op: "read", Path: name, Err: ErrNotRegularFile}
	}
	if info.Size() > MaxFileBytes {
		return nil, ErrFileTooLarge
	}

	f, err := fsys.Open(name)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	if info, err := f.Stat(); err != nil {
		return nil, err
	} else if !info.Mode().IsRegular() {
		return nil, &fs.PathError{Op: "read", Path: name, Err: ErrNotRegularFile}
	}

	src, err := io.ReadAll(io.LimitReader(f, MaxFileBytes+1))
	if err != nil {
		return nil, err
	}
	if len(src) > MaxFileBytes {
		return nil, ErrFileTooLarge
	}
	return src, nil
}

// CheckNativeDepth reports ErrNestingTooDeep when src's HCL native syntax
// (.hcl, .tf, .tofu) would drive hclsyntax's recursive-descent parser, or
// the recursive walks over the AST it builds, too deep (02-REVIEW G7,
// G17). Four things add up to the depth it compares with
// MaxNestingDepth:
//
//   - bracket, quote, heredoc and template nesting: every open brace,
//     bracket, paren, quote, heredoc, `${` and `%{` is one level until its
//     matching closer;
//   - runs of unary operators (`!`, `-`), each one level;
//   - pending ternary `?` tokens. hclsyntax's parseTernaryConditional parses
//     the condition with parseBinaryOps and then BOTH the true and the
//     false branch with ParseExpression, which re-enters
//     parseTernaryConditional, so every `?` is one level of Go recursion
//     that is released only when the whole conditional expression ends.
//     That happens at a comma, at the closer of the bracket the `?` sits
//     in, or at a newline when newlines are significant there: in a body
//     (the file itself and block bodies) and in an object constructor that
//     is not a for-expression (parseObjectCons peeks for the `for` keyword
//     with newlines off). Parens, brackets, function arguments, template
//     sequences and for-expressions all ignore newlines. A line comment
//     ending in a newline counts as that newline, as the parser's peeker
//     turns it into one. Pending `?` are therefore counted per bracket
//     frame and reset only at those release points; a `:` never releases
//     one. Popping on `:` instead would be wrong: in the else-chain
//     `a?b:a?b:...1` every `:` is followed by another `?`, so a
//     push-on-`?`/pop-on-`:` count stays at 1 while the parser recurses
//     once per `?`, and a 4 MB else-chain killed the process with a fatal
//     stack overflow;
//   - open template control blocks: `%{if}` and `%{for}` stay open until
//     their `%{endif}` / `%{endfor}`, and evaluating the template recurses
//     once per open block (TemplateExpr -> ConditionalExpr / ForExpr). A
//     4 MB file of nested `%{if}` killed check, report and watch with a
//     fatal stack overflow (sec #261). The directive keyword is the first
//     token after the `%{` (strip marker included) that is not a comment or
//     a newline, so `%{/*c*/if`, `%{#c<NL>if` and `%{<NL>if` count too
//     (sec #271). `endif` and `endfor` close one block (never below zero),
//     `else` is neutral and anything else opens one, so a misread keyword
//     can only over-count. `%%{` is a literal and never lexes as a
//     directive.
//
// Separately, a frame whose chain count, the binary operators, `.` and `[`
// since its last release point, exceeds MaxExpressionChain is refused too
// (the long-chain half of G17). A `[` counts as one link of the enclosing
// frame, covering postfix index chains `x[a][a]...`, before it opens its
// own frame.
//
// Every tie goes to over-counting: over-counting only costs an
// unnecessary "too deep" verdict (an unknown unit or module, never a false
// GRT001), while under-counting could hand a hostile file to the
// recursive parser, which Go cannot recover from.
//
// It lexes src with hclsyntax.LexConfig — HCL's own tokenizer, exact by
// construction and never itself recursive — instead of a hand-written byte
// scanner. HCL's lexing rules are subtle (a quoted string does not end at
// a newline, heredoc markers are compared after trimming and only at the
// start of a line, and "$${"/"%%{" are escapes): a scanner that disagreed
// with the real tokenizer on any of these could under-count and let a
// hostile file back into the recursive parser, which is exactly the crash
// this function exists to prevent. Lex diagnostics are ignored: the parser
// reports syntax errors later, and nothing here changes whether src is
// safe to hand to it. Lexing materialises every token of src, so the
// memory peak of a MaxFileBytes file of one-byte tokens remains (see
// MaxFileBytes); the chain cap removes the deep AST and its walks, not
// that peak.
func CheckNativeDepth(src []byte) error {
	toks, _ := hclsyntax.LexConfig(src, "", hcl.InitialPos)

	// frames[0] is the file body, where newlines are significant.
	frames := []depthFrame{{open: hclsyntax.TokenNil, newlines: true}}
	run := 0     // current run of unary operators
	pending := 0 // pending `?` summed over every frame
	ctl := 0     // open %{if}/%{for} template control blocks
	for i, t := range toks {
		top := &frames[len(frames)-1]
		switch t.Type {
		case hclsyntax.TokenOBrace, hclsyntax.TokenOBrack, hclsyntax.TokenOParen,
			hclsyntax.TokenOQuote, hclsyntax.TokenOHeredoc,
			hclsyntax.TokenTemplateInterp, hclsyntax.TokenTemplateControl:
			if t.Type == hclsyntax.TokenTemplateControl {
				ctl = controlDepth(ctl, toks[i+1:])
			}
			if t.Type == hclsyntax.TokenOBrack {
				top.chain++ // x[a]: one postfix link of the enclosing frame
				if top.chain > MaxExpressionChain {
					return ErrNestingTooDeep
				}
			}
			newlines := t.Type == hclsyntax.TokenOBrace && !forFollows(toks[i+1:])
			frames = append(frames, depthFrame{open: t.Type, newlines: newlines})
			run = 0
		case hclsyntax.TokenCBrace:
			frames, pending = popFrame(frames, pending, hclsyntax.TokenOBrace)
			run = 0
		case hclsyntax.TokenCBrack:
			frames, pending = popFrame(frames, pending, hclsyntax.TokenOBrack)
			run = 0
		case hclsyntax.TokenCParen:
			frames, pending = popFrame(frames, pending, hclsyntax.TokenOParen)
			run = 0
		case hclsyntax.TokenCQuote:
			frames, pending = popFrame(frames, pending, hclsyntax.TokenOQuote)
			run = 0
		case hclsyntax.TokenCHeredoc:
			frames, pending = popFrame(frames, pending, hclsyntax.TokenOHeredoc)
			run = 0
		case hclsyntax.TokenTemplateSeqEnd:
			frames, pending = popFrame(frames, pending, hclsyntax.TokenTemplateInterp, hclsyntax.TokenTemplateControl)
			run = 0
		case hclsyntax.TokenQuestion:
			top.questions++
			pending++
			run = 0
		case hclsyntax.TokenMinus:
			// Binary or unary: counted both ways, over-counting either.
			top.chain++
			run++
		case hclsyntax.TokenBang:
			run++
		case hclsyntax.TokenPlus, hclsyntax.TokenStar, hclsyntax.TokenSlash,
			hclsyntax.TokenPercent, hclsyntax.TokenAnd, hclsyntax.TokenOr,
			hclsyntax.TokenEqualOp, hclsyntax.TokenNotEqual,
			hclsyntax.TokenLessThan, hclsyntax.TokenLessThanEq,
			hclsyntax.TokenGreaterThan, hclsyntax.TokenGreaterThanEq,
			hclsyntax.TokenDot:
			top.chain++
			run = 0
		case hclsyntax.TokenComma:
			pending = top.release(pending)
			run = 0
		case hclsyntax.TokenNewline, hclsyntax.TokenComment:
			// A run of unary operators may wrap across a line break, and a
			// comment carries no nesting of its own, so neither resets run.
			if top.newlines && (t.Type == hclsyntax.TokenNewline || endsInNewline(t.Bytes)) {
				pending = top.release(pending)
			}
		default:
			run = 0
		}
		if len(frames)-1+run+pending+ctl > MaxNestingDepth {
			return ErrNestingTooDeep
		}
		if frames[len(frames)-1].chain > MaxExpressionChain {
			return ErrNestingTooDeep
		}
	}
	return nil
}

// controlDepth returns the open template control block count after the
// directive whose tokens follow a TokenTemplateControl: endif/endfor close
// one block (floored at zero), else is neutral, and any other keyword (if,
// for, a misparse, nothing) opens one, so every doubt over-counts.
func controlDepth(ctl int, rest hclsyntax.Tokens) int {
	for _, t := range rest {
		if t.Type == hclsyntax.TokenComment || t.Type == hclsyntax.TokenNewline {
			continue
		}
		if t.Type == hclsyntax.TokenIdent {
			switch string(t.Bytes) {
			case "endif", "endfor":
				return max(ctl-1, 0)
			case "else":
				return ctl
			}
		}
		return ctl + 1
	}
	return ctl + 1
}

// depthFrame is one open bracket, quote, heredoc or template sequence in
// CheckNativeDepth's scan (the file body is the base frame).
type depthFrame struct {
	// open is the token type that opened the frame (TokenNil for the base).
	open hclsyntax.TokenType
	// newlines is true when a newline ends an expression in this frame.
	newlines bool
	// questions is the number of `?` pending in this frame.
	questions int
	// chain is the number of operator/`.`/`[` links since the frame's last
	// release point.
	chain int
}

// release resets f's counts at a release point (a comma, or a significant
// newline) and returns pending minus the `?` it held.
func (f *depthFrame) release(pending int) int {
	pending -= f.questions
	f.questions = 0
	f.chain = 0
	return pending
}

// popFrame pops the top frame when it was opened by one of want, taking
// its pending `?` and chain count with it, and returns frames unchanged
// otherwise. A mismatched or unmatched closing token therefore never pops
// (and the base frame never is): over-counting depth only costs an
// unnecessary "too deep" verdict, but under-counting could let a hostile
// file reach the recursive parser, so ties always go to over-counting.
func popFrame(frames []depthFrame, pending int, want ...hclsyntax.TokenType) ([]depthFrame, int) {
	if len(frames) == 1 {
		return frames, pending
	}
	top := frames[len(frames)-1]
	for _, w := range want {
		if top.open == w {
			return frames[:len(frames)-1], pending - top.questions
		}
	}
	return frames, pending
}

// forFollows reports whether the first token of rest, skipping newlines
// and comments, is the `for` keyword, which makes the brace before it a
// for-expression (mirroring parseObjectCons).
func forFollows(rest hclsyntax.Tokens) bool {
	for _, t := range rest {
		switch t.Type {
		case hclsyntax.TokenNewline, hclsyntax.TokenComment:
			continue
		}
		return t.Type == hclsyntax.TokenIdent && string(t.Bytes) == "for"
	}
	return false
}

// endsInNewline reports whether a comment token's bytes end in '\n', as a
// `#` or `//` line comment's do.
func endsInNewline(b []byte) bool {
	return len(b) > 0 && b[len(b)-1] == '\n'
}

// CheckJSONDepth reports ErrNestingTooDeep when src's JSON object/array
// nesting (.tf.json, .tofu.json) exceeds MaxNestingDepth (02-REVIEW G7).
//
// It scans bytes with the same string rule as hcl/json's own scanner
// (scanString in hcl/v2/json/scanner.go): outside a string, an unescaped
// `"` enters string mode; inside a string, a backslash toggles escaping,
// an unescaped `"` leaves string mode, and a byte below 0x20 also leaves
// string mode but is NOT consumed, so it is rescanned outside the string.
// Mirroring this exactly (rather than a naive quote-toggle) means brackets
// written inside a string are never mistaken for structural nesting, and
// brackets that follow a raw control byte inside an unterminated string
// ARE counted, exactly as hcl/json's own scanner would tokenize them.
func CheckJSONDepth(src []byte) error {
	var stack []byte
	inString := false
	escaping := false

	for i := 0; i < len(src); i++ {
		b := src[i]
		if inString {
			switch {
			case b == '\\':
				escaping = !escaping
			case b == '"':
				if !escaping {
					inString = false
				}
				escaping = false
			case b < 0x20:
				inString = false
				i-- // not consumed: rescanned outside the string below
			default:
				escaping = false
			}
			continue
		}

		switch b {
		case '"':
			inString = true
			escaping = false
		case '{', '[':
			stack = append(stack, b)
			if len(stack) > MaxNestingDepth {
				return ErrNestingTooDeep
			}
		case '}':
			if n := len(stack); n > 0 && stack[n-1] == '{' {
				stack = stack[:n-1]
			}
		case ']':
			if n := len(stack); n > 0 && stack[n-1] == '[' {
				stack = stack[:n-1]
			}
		}
	}
	return nil
}
