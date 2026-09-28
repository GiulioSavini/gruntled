package hclconv

import (
	"errors"
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
)

// ErrFileTooLarge is returned by ReadFileLimited when a file exceeds
// MaxFileBytes.
var ErrFileTooLarge = errors.New("hclconv: file exceeds MaxFileBytes")

// ErrNestingTooDeep is returned by CheckNativeDepth and CheckJSONDepth when
// src's nesting exceeds MaxNestingDepth.
var ErrNestingTooDeep = errors.New("hclconv: nesting exceeds MaxNestingDepth")

// ReadFileLimited reads name from fsys, refusing it if it exceeds
// MaxFileBytes (02-REVIEW G7): hclsyntax.ParseConfig and hcl/json.Parse
// both recurse over their input, and Go cannot recover from the stack
// overflow a hostile or generated file can cause, so the only defence is
// refusing to parse it in the first place.
//
// fs.Stat is checked first, so an obviously oversize file is never read
// into memory; a Stat error is returned as-is. fs.ReadFile is then called
// exactly once — callers that need to prove a file is read at most once
// may rely on that — and its result is checked again in case Stat
// under-reported the size, which no fs.FS is required to avoid.
func ReadFileLimited(fsys fs.FS, name string) ([]byte, error) {
	info, err := fs.Stat(fsys, name)
	if err != nil {
		return nil, err
	}
	if info.Size() > MaxFileBytes {
		return nil, ErrFileTooLarge
	}

	src, err := fs.ReadFile(fsys, name)
	if err != nil {
		return nil, err
	}
	if len(src) > MaxFileBytes {
		return nil, ErrFileTooLarge
	}
	return src, nil
}

// CheckNativeDepth reports ErrNestingTooDeep when src's HCL native syntax
// (.hcl, .tf, .tofu) nests brackets, quotes, heredocs, template
// interpolations or unary (!, -) operator runs deeper than
// MaxNestingDepth (02-REVIEW G7).
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
// safe to hand to it.
func CheckNativeDepth(src []byte) error {
	toks, _ := hclsyntax.LexConfig(src, "", hcl.InitialPos)

	var stack []hclsyntax.TokenType
	run := 0
	for _, t := range toks {
		switch t.Type {
		case hclsyntax.TokenOBrace, hclsyntax.TokenOBrack, hclsyntax.TokenOParen,
			hclsyntax.TokenOQuote, hclsyntax.TokenOHeredoc,
			hclsyntax.TokenTemplateInterp, hclsyntax.TokenTemplateControl:
			stack = append(stack, t.Type)
			run = 0
		case hclsyntax.TokenCBrace:
			stack = popMatching(stack, hclsyntax.TokenOBrace)
			run = 0
		case hclsyntax.TokenCBrack:
			stack = popMatching(stack, hclsyntax.TokenOBrack)
			run = 0
		case hclsyntax.TokenCParen:
			stack = popMatching(stack, hclsyntax.TokenOParen)
			run = 0
		case hclsyntax.TokenCQuote:
			stack = popMatching(stack, hclsyntax.TokenOQuote)
			run = 0
		case hclsyntax.TokenCHeredoc:
			stack = popMatching(stack, hclsyntax.TokenOHeredoc)
			run = 0
		case hclsyntax.TokenTemplateSeqEnd:
			stack = popMatching(stack, hclsyntax.TokenTemplateInterp, hclsyntax.TokenTemplateControl)
			run = 0
		case hclsyntax.TokenBang, hclsyntax.TokenMinus:
			run++
		case hclsyntax.TokenNewline, hclsyntax.TokenComment:
			// Depth-neutral: a run of unary operators may wrap across a
			// line break, and a comment carries no nesting of its own.
		default:
			run = 0
		}
		if len(stack)+run > MaxNestingDepth {
			return ErrNestingTooDeep
		}
	}
	return nil
}

// popMatching pops stack's top element and returns the shortened slice
// when it is one of want, and returns stack unchanged otherwise. A
// mismatched or unmatched closing token is therefore never popped:
// over-counting depth only costs an unnecessary "too deep" verdict, but
// under-counting could let a hostile file reach the recursive parser, so
// ties always go to over-counting.
func popMatching(stack []hclsyntax.TokenType, want ...hclsyntax.TokenType) []hclsyntax.TokenType {
	if len(stack) == 0 {
		return stack
	}
	top := stack[len(stack)-1]
	for _, w := range want {
		if top == w {
			return stack[:len(stack)-1]
		}
	}
	return stack
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
