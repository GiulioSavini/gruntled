package hclconv_test

import (
	"errors"
	"fmt"
	"io/fs"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/GiulioSavini/gruntled/internal/infrastructure/hclconv"
	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
)

// --- ReadFileLimited -------------------------------------------------------

// statLiesFS wraps an fstest.MapFS whose Stat always reports size 0,
// forcing ReadFileLimited's post-read length check (rather than its
// Stat-based refusal) to catch an oversize file.
type statLiesFS struct {
	fstest.MapFS
}

func (f statLiesFS) Stat(name string) (fs.FileInfo, error) {
	info, err := f.MapFS.Stat(name)
	if err != nil {
		return nil, err
	}
	return zeroSizeInfo{info}, nil
}

type zeroSizeInfo struct{ fs.FileInfo }

func (zeroSizeInfo) Size() int64 { return 0 }

// countingReadFS wraps an fstest.MapFS and counts Open calls (and
// ReadFile calls, should anything use that path), so a test can assert
// ReadFileLimited opens a file exactly once, or not at all when it
// refuses the file at Stat.
type countingReadFS struct {
	fstest.MapFS
	reads int
}

func (f *countingReadFS) Open(name string) (fs.File, error) {
	f.reads++
	return f.MapFS.Open(name)
}

func (f *countingReadFS) ReadFile(name string) ([]byte, error) {
	f.reads++
	return f.MapFS.ReadFile(name)
}

func TestReadFileLimited(t *testing.T) {
	t.Run("exactly MaxFileBytes reads ok", func(t *testing.T) {
		fsys := fstest.MapFS{
			"f.tf": &fstest.MapFile{Data: []byte(strings.Repeat("a", hclconv.MaxFileBytes))},
		}
		src, err := hclconv.ReadFileLimited(fsys, "f.tf")
		if err != nil {
			t.Fatalf("ReadFileLimited: unexpected error: %v", err)
		}
		if len(src) != hclconv.MaxFileBytes {
			t.Fatalf("len(src) = %d, want %d", len(src), hclconv.MaxFileBytes)
		}
	})

	t.Run("MaxFileBytes+1 refused before reading", func(t *testing.T) {
		fsys := &countingReadFS{MapFS: fstest.MapFS{
			"f.tf": &fstest.MapFile{Data: []byte(strings.Repeat("a", hclconv.MaxFileBytes+1))},
		}}
		_, err := hclconv.ReadFileLimited(fsys, "f.tf")
		if !errors.Is(err, hclconv.ErrFileTooLarge) {
			t.Fatalf("err = %v, want ErrFileTooLarge", err)
		}
		if fsys.reads != 0 {
			t.Fatalf("ReadFile calls = %d, want 0 (refused at Stat)", fsys.reads)
		}
	})

	t.Run("missing file is not ErrFileTooLarge", func(t *testing.T) {
		fsys := fstest.MapFS{}
		_, err := hclconv.ReadFileLimited(fsys, "missing.tf")
		if err == nil {
			t.Fatalf("err = nil, want a not-exist error")
		}
		if errors.Is(err, hclconv.ErrFileTooLarge) {
			t.Fatalf("err = %v, want anything but ErrFileTooLarge", err)
		}
		if !errors.Is(err, fs.ErrNotExist) {
			t.Fatalf("err = %v, want errors.Is(fs.ErrNotExist)", err)
		}
	})

	t.Run("Stat under-reports size, caught after read", func(t *testing.T) {
		fsys := statLiesFS{MapFS: fstest.MapFS{
			"f.tf": &fstest.MapFile{Data: []byte(strings.Repeat("a", hclconv.MaxFileBytes+1))},
		}}
		_, err := hclconv.ReadFileLimited(fsys, "f.tf")
		if !errors.Is(err, hclconv.ErrFileTooLarge) {
			t.Fatalf("err = %v, want ErrFileTooLarge", err)
		}
	})

	t.Run("success path opens exactly once", func(t *testing.T) {
		fsys := &countingReadFS{MapFS: fstest.MapFS{
			"f.tf": &fstest.MapFile{Data: []byte(`a = 1`)},
		}}
		src, err := hclconv.ReadFileLimited(fsys, "f.tf")
		if err != nil {
			t.Fatalf("ReadFileLimited: unexpected error: %v", err)
		}
		if string(src) != "a = 1" {
			t.Fatalf("src = %q, want %q", src, "a = 1")
		}
		if fsys.reads != 1 {
			t.Fatalf("Open+ReadFile calls = %d, want 1", fsys.reads)
		}
	})

	// 02-REVIEW G18: opening a FIFO for reading blocks until a writer
	// appears, so a non-regular file is refused at Stat, before any Open.
	for _, mode := range []fs.FileMode{fs.ModeNamedPipe, fs.ModeDevice, fs.ModeSocket, fs.ModeCharDevice | fs.ModeDevice} {
		t.Run("non-regular "+mode.String()+" refused before opening", func(t *testing.T) {
			fsys := &countingReadFS{MapFS: fstest.MapFS{
				"f.tf": &fstest.MapFile{Data: []byte(`a = 1`), Mode: mode},
			}}
			_, err := hclconv.ReadFileLimited(fsys, "f.tf")
			if !errors.Is(err, hclconv.ErrNotRegularFile) {
				t.Fatalf("err = %v, want ErrNotRegularFile", err)
			}
			if fsys.reads != 0 {
				t.Fatalf("Open+ReadFile calls = %d, want 0 (refused at Stat)", fsys.reads)
			}
		})
	}

	t.Run("Stat says regular but the open handle does not", func(t *testing.T) {
		fsys := openModeFS{
			MapFS: fstest.MapFS{"f.tf": &fstest.MapFile{Data: []byte(`a = 1`)}},
			mode:  fs.ModeNamedPipe,
		}
		_, err := hclconv.ReadFileLimited(fsys, "f.tf")
		if !errors.Is(err, hclconv.ErrNotRegularFile) {
			t.Fatalf("err = %v, want ErrNotRegularFile", err)
		}
	})
}

// openModeFS reports f.tf as a regular file through Stat but hands out an
// open file whose own Stat reports mode: the file was swapped between the
// Stat pre-check and Open.
type openModeFS struct {
	fstest.MapFS
	mode fs.FileMode
}

func (f openModeFS) Open(name string) (fs.File, error) {
	file, err := f.MapFS.Open(name)
	if err != nil {
		return nil, err
	}
	return modeFile{File: file, mode: f.mode}, nil
}

type modeFile struct {
	fs.File
	mode fs.FileMode
}

func (m modeFile) Stat() (fs.FileInfo, error) {
	info, err := m.File.Stat()
	if err != nil {
		return nil, err
	}
	return modeInfo{FileInfo: info, mode: m.mode}, nil
}

type modeInfo struct {
	fs.FileInfo
	mode fs.FileMode
}

func (m modeInfo) Mode() fs.FileMode { return m.mode }

// --- CheckNativeDepth: rejects ---------------------------------------------

const nativeDeepN = 200_000

func wrap(open, inner, close string) []byte {
	return []byte("a = " + open + inner + close)
}

func repeatBoth(unit string, n int, inner string) []byte {
	return wrap(strings.Repeat(unit, n), inner, "")
}

func TestCheckNativeDepthRejects(t *testing.T) {
	deepBlocks := strings.Repeat("b {\n", nativeDeepN) + strings.Repeat("}\n", nativeDeepN)

	cases := []struct {
		name string
		src  []byte
	}{
		{
			name: "parens",
			src:  wrap(strings.Repeat("(", nativeDeepN), "1", strings.Repeat(")", nativeDeepN)),
		},
		{
			name: "bang",
			src:  repeatBoth("!", nativeDeepN, "true"),
		},
		{
			name: "minus tight",
			src:  repeatBoth("-", nativeDeepN, "1"),
		},
		{
			name: "minus spaced",
			src:  repeatBoth("- ", nativeDeepN, "1"),
		},
		{
			name: "brackets",
			src:  wrap(strings.Repeat("[", nativeDeepN), "1", strings.Repeat("]", nativeDeepN)),
		},
		{
			name: "object braces",
			src:  wrap(strings.Repeat("{x = ", nativeDeepN), "1", strings.Repeat("}", nativeDeepN)),
		},
		{
			name: "nested blocks",
			src:  []byte(deepBlocks),
		},
		{
			name: "template nest",
			src:  wrap(strings.Repeat(`"${`, nativeDeepN), "1", strings.Repeat(`}"`, nativeDeepN)),
		},
		{
			name: "heredoc-in-interp nest",
			src:  wrap(strings.Repeat("<<EOT\n${", nativeDeepN), "1", strings.Repeat("}\nEOT\n", nativeDeepN)),
		},
		{
			name: "for-expr nest",
			src:  wrap(strings.Repeat("[for v in x : ", nativeDeepN), "1", strings.Repeat("]", nativeDeepN)),
		},
		{
			// Accepted before 02-12: a 32k-deep else-chain is 32k levels
			// of parseTernaryConditional recursion (02-REVIEW G17).
			name: "256 KiB ternary else-chain",
			src:  wrap(strings.Repeat("x ? 1 : ", (256*1024)/len("x ? 1 : ")), "0", ""),
		},
		{
			// Accepted before 02-12: 64k links in one expression build a
			// 64k-deep left-leaning AST (02-REVIEW G17).
			name: "256 KiB plus chain",
			src:  wrap(strings.Repeat("x + ", (256*1024)/len("x + ")), "x", ""),
		},
		{
			name: "boundary MaxNestingDepth+1 parens",
			src: wrap(
				strings.Repeat("(", hclconv.MaxNestingDepth+1),
				"1",
				strings.Repeat(")", hclconv.MaxNestingDepth+1),
			),
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if err := hclconv.CheckNativeDepth(c.src); !errors.Is(err, hclconv.ErrNestingTooDeep) {
				t.Fatalf("CheckNativeDepth(%s) = %v, want ErrNestingTooDeep", c.name, err)
			}
		})
	}
}

// --- CheckNativeDepth: accepts ---------------------------------------------

const realisticTerragruntHCL = `
include "root" {
  path = find_in_parent_folders("root.hcl")
}

dependency "vpc" {
  config_path = "../vpc"
  mock_outputs = {
    vpc_id = "vpc-mock"
  }
}

inputs = {
  name = "example"
  tags = {
    env = "prod"
    nested = {
      deeper = {
        value = 1
      }
    }
  }
  notes = <<EOT
some documentation
about this unit
EOT
}
`

func TestCheckNativeDepthAccepts(t *testing.T) {
	t.Run("MaxNestingDepth parens exactly, inclusive", func(t *testing.T) {
		src := wrap(
			strings.Repeat("(", hclconv.MaxNestingDepth),
			"1",
			strings.Repeat(")", hclconv.MaxNestingDepth),
		)
		if err := hclconv.CheckNativeDepth(src); err != nil {
			t.Fatalf("CheckNativeDepth: %v, want nil", err)
		}
	})

	t.Run("realistic terragrunt.hcl", func(t *testing.T) {
		if err := hclconv.CheckNativeDepth([]byte(realisticTerragruntHCL)); err != nil {
			t.Fatalf("CheckNativeDepth: %v, want nil", err)
		}
	})

	t.Run("parens inside a string", func(t *testing.T) {
		src := []byte(`a = "` + strings.Repeat("(", 5000) + `"` + "\n")
		if err := hclconv.CheckNativeDepth(src); err != nil {
			t.Fatalf("CheckNativeDepth: %v, want nil", err)
		}
	})

	t.Run("parens inside a line comment", func(t *testing.T) {
		src := []byte("# " + strings.Repeat("(", 5000) + "\n")
		if err := hclconv.CheckNativeDepth(src); err != nil {
			t.Fatalf("CheckNativeDepth: %v, want nil", err)
		}
	})

	t.Run("brackets inside a block comment", func(t *testing.T) {
		src := []byte("/* " + strings.Repeat("[", 5000) + " */\n")
		if err := hclconv.CheckNativeDepth(src); err != nil {
			t.Fatalf("CheckNativeDepth: %v, want nil", err)
		}
	})

	t.Run("braces inside a heredoc body", func(t *testing.T) {
		src := []byte("a = <<EOT\n" + strings.Repeat("{", 5000) + "\nEOT\n")
		if err := hclconv.CheckNativeDepth(src); err != nil {
			t.Fatalf("CheckNativeDepth: %v, want nil", err)
		}
	})

	t.Run("mismatched closers never under-count", func(t *testing.T) {
		src := []byte("a = ((((]]]]\n")
		if err := hclconv.CheckNativeDepth(src); err != nil {
			t.Fatalf("CheckNativeDepth: %v, want nil", err)
		}
	})
}

// --- CheckNativeDepth: ternaries and chains (02-REVIEW G17) ----------------

// trueChain is the ternary chain nested in its true branch,
// `1?1?...1:1:1`, with n `?`.
func trueChain(n int) string {
	return strings.Repeat("1?", n) + "1" + strings.Repeat(":1", n)
}

// elseChain is the ternary chain nested in its false branch,
// `a?b:a?b:...1`, with n `?`. A push-on-`?`/pop-on-`:` count stays at 1
// on it, yet hclsyntax recurses once per `?` (parseTernaryConditional
// parses the false branch with ParseExpression).
func elseChain(n int) string {
	return strings.Repeat("a?b:", n) + "1"
}

func TestCheckNativeDepthTernary(t *testing.T) {
	const maxD = hclconv.MaxNestingDepth
	cases := []struct {
		name string
		expr string
		deep bool
	}{
		{"true-chain MaxNestingDepth-1", trueChain(maxD - 1), false},
		{"true-chain MaxNestingDepth", trueChain(maxD), false},
		{"true-chain MaxNestingDepth+1", trueChain(maxD + 1), true},
		{"else-chain MaxNestingDepth-1", elseChain(maxD - 1), false},
		{"else-chain MaxNestingDepth", elseChain(maxD), false},
		{"else-chain MaxNestingDepth+1", elseChain(maxD + 1), true},
		{
			"else-chain over lines inside parens",
			"(" + strings.Repeat("a ?\n b :\n ", maxD+1) + "1)",
			true,
		},
		{
			"else-chain over lines inside a for-object",
			"{ for k, v in m : k => " + strings.Repeat("v ?\n 1 :\n ", maxD+1) + "1 }",
			true,
		},
		{
			"else-chain in a function argument in a template",
			`"${f(` + elseChain(maxD+1) + `)}"`,
			true,
		},
		{
			"brackets and ternaries add up",
			strings.Repeat("(", maxD/2) + elseChain(maxD/2+1) + strings.Repeat(")", maxD/2),
			true,
		},
		{
			"brackets and ternaries add up to exactly MaxNestingDepth",
			strings.Repeat("(", maxD/2) + elseChain(maxD/2) + strings.Repeat(")", maxD/2),
			false,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := hclconv.CheckNativeDepth([]byte("x = " + c.expr + "\n"))
			switch {
			case c.deep && !errors.Is(err, hclconv.ErrNestingTooDeep):
				t.Fatalf("CheckNativeDepth = %v, want ErrNestingTooDeep", err)
			case !c.deep && err != nil:
				t.Fatalf("CheckNativeDepth = %v, want nil", err)
			}
		})
	}
}

// TestCheckNativeDepthTernaryRealistic proves a file holding thousands of
// ternaries, each released by a newline, a comma or its closing bracket,
// is not mistaken for a deep one.
func TestCheckNativeDepthTernaryRealistic(t *testing.T) {
	var b strings.Builder
	b.WriteString("locals {\n")
	for i := range 5000 {
		fmt.Fprintf(&b, "  a%d = var.flag ? \"x\" : \"y\" # note %d\n", i, i)
	}
	b.WriteString("}\n\nobj_lines = {\n")
	for i := range 2000 {
		fmt.Fprintf(&b, "  k%d = var.flag ? %d : 0\n", i, i)
	}
	b.WriteString("}\n\nobj_commas = { ")
	for i := range 2000 {
		fmt.Fprintf(&b, "k%d = var.flag ? %d : 0, ", i, i)
	}
	b.WriteString("}\n\ncall = f(")
	for i := range 500 {
		if i > 0 {
			b.WriteString(", ")
		}
		fmt.Fprintf(&b, "var.flag ? %d : 0", i)
	}
	b.WriteString(")\n\n")
	for i := range 1000 {
		fmt.Fprintf(&b, "for%d = [for x in xs : x ? 1 : 2]\n", i)
	}
	b.WriteString("nested = a ? (b ? (c ? (d ? (e ? (f ? (g ? (h ? (i ? (j ? 1 : 0) : 0) : 0) : 0) : 0) : 0) : 0) : 0) : 0) : 0\n")
	b.WriteString("doc = <<EOT\n" + strings.Repeat("?", 5000) + "\nEOT\n")

	if err := hclconv.CheckNativeDepth([]byte(b.String())); err != nil {
		t.Fatalf("CheckNativeDepth = %v, want nil", err)
	}
}

func TestCheckNativeDepthChain(t *testing.T) {
	const maxC = hclconv.MaxExpressionChain
	var attrs strings.Builder
	for i := range 20_000 {
		fmt.Fprintf(&attrs, "a%d = a + b + c\n", i)
	}
	cases := []struct {
		name string
		src  string
		deep bool
	}{
		{"plus chain MaxExpressionChain-1", "x = " + strings.Repeat("1+", maxC-1) + "1\n", false},
		{"plus chain MaxExpressionChain", "x = " + strings.Repeat("1+", maxC) + "1\n", false},
		{"plus chain MaxExpressionChain+1", "x = " + strings.Repeat("1+", maxC+1) + "1\n", true},
		{"mixed operators over the cap", "x = " + strings.Repeat("a == b && c || ", maxC/3+1) + "1\n", true},
		{"dot chain over the cap", "x = a" + strings.Repeat(".b", maxC+1) + "\n", true},
		{"index chain over the cap", "x = x" + strings.Repeat("[a]", maxC+1) + "\n", true},
		{"20,000 short chains on their own lines", attrs.String(), false},
		{"20,000 short chains separated by commas", "x = [" + strings.Repeat("a + b, ", 20_000) + "]\n", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := hclconv.CheckNativeDepth([]byte(c.src))
			switch {
			case c.deep && !errors.Is(err, hclconv.ErrNestingTooDeep):
				t.Fatalf("CheckNativeDepth = %v, want ErrNestingTooDeep", err)
			case !c.deep && err != nil:
				t.Fatalf("CheckNativeDepth = %v, want nil", err)
			}
		})
	}
}

// --- CheckJSONDepth ----------------------------------------------------

const jsonDeepN = 200_000

func TestCheckJSONDepth(t *testing.T) {
	t.Run("200k-deep object nest rejected", func(t *testing.T) {
		src := []byte(strings.Repeat(`{"a":`, jsonDeepN) + "1" + strings.Repeat("}", jsonDeepN))
		if err := hclconv.CheckJSONDepth(src); !errors.Is(err, hclconv.ErrNestingTooDeep) {
			t.Fatalf("CheckJSONDepth = %v, want ErrNestingTooDeep", err)
		}
	})

	t.Run("200k-deep array nest rejected", func(t *testing.T) {
		src := []byte(strings.Repeat("[", jsonDeepN) + "1" + strings.Repeat("]", jsonDeepN))
		if err := hclconv.CheckJSONDepth(src); !errors.Is(err, hclconv.ErrNestingTooDeep) {
			t.Fatalf("CheckJSONDepth = %v, want ErrNestingTooDeep", err)
		}
	})

	t.Run("boundary MaxNestingDepth exactly accepted", func(t *testing.T) {
		src := []byte(strings.Repeat("[", hclconv.MaxNestingDepth) + "1" + strings.Repeat("]", hclconv.MaxNestingDepth))
		if err := hclconv.CheckJSONDepth(src); err != nil {
			t.Fatalf("CheckJSONDepth = %v, want nil", err)
		}
	})

	t.Run("boundary MaxNestingDepth+1 rejected", func(t *testing.T) {
		src := []byte(strings.Repeat("[", hclconv.MaxNestingDepth+1) + "1" + strings.Repeat("]", hclconv.MaxNestingDepth+1))
		if err := hclconv.CheckJSONDepth(src); !errors.Is(err, hclconv.ErrNestingTooDeep) {
			t.Fatalf("CheckJSONDepth = %v, want ErrNestingTooDeep", err)
		}
	})

	t.Run("braces inside a JSON string ignored", func(t *testing.T) {
		src := []byte(`{"s": "` + strings.Repeat("{", 5000) + `"}`)
		if err := hclconv.CheckJSONDepth(src); err != nil {
			t.Fatalf("CheckJSONDepth = %v, want nil", err)
		}
	})

	t.Run("escaped quote keeps braces inside the string", func(t *testing.T) {
		// The string is: \" (an escaped quote, does not end the string),
		// then 5000 '{', then the real closing quote.
		src := []byte(`{"s": "\"` + strings.Repeat("{", 5000) + `"}`)
		if err := hclconv.CheckJSONDepth(src); err != nil {
			t.Fatalf("CheckJSONDepth = %v, want nil", err)
		}
	})

	t.Run("control byte ends string, following braces are counted", func(t *testing.T) {
		// A raw newline inside the string ends it (without being
		// consumed), exactly like hcl/json's scanner. The real open
		// braces that follow must be counted, proving they are not
		// mistaken for still being inside the (unterminated) string.
		src := []byte(`{"s": "abc` + "\n" + strings.Repeat("{", hclconv.MaxNestingDepth+2))
		if err := hclconv.CheckJSONDepth(src); !errors.Is(err, hclconv.ErrNestingTooDeep) {
			t.Fatalf("CheckJSONDepth = %v, want ErrNestingTooDeep", err)
		}
	})
}

// --- CheckNativeDepth: template control blocks (sec #261, #271) -------------

// nestTemplate returns `x = "<open>×n<inner><close>×n"` as a quoted string.
func nestTemplate(open, inner, close string, n int) []byte {
	return []byte(`x = "` + strings.Repeat(open, n) + inner + strings.Repeat(close, n) + `"` + "\n")
}

func TestCheckNativeDepthTemplateControl(t *testing.T) {
	const deep = hclconv.MaxNestingDepth + 1
	rejects := []struct {
		name string
		src  []byte
	}{
		{"nested if", nestTemplate("%{if a}", "x", "%{endif}", deep)},
		{"nested for", nestTemplate("%{for v in l}", "x", "%{endfor}", deep)},
		{"strip markers", nestTemplate("%{~ if a ~}", "x", "%{~ endif ~}", deep)},
		{"mixed if and for", nestTemplate("%{if a}%{for v in l}", "x", "%{endfor}%{endif}", deep/2+1)},
		{"heredoc", []byte("x = <<EOT\n" + strings.Repeat("%{if a}\n", deep) + "x\n" + strings.Repeat("%{endif}\n", deep) + "EOT\n")},
		// sec #271: the keyword after a comment or newline token.
		{"block comment before keyword", nestTemplate("%{/*c*/if a}", "x", "%{endif}", deep)},
		{"line comment before keyword", nestTemplate("%{#c\nif a}", "x", "%{endif}", deep)},
		{"newline before keyword", nestTemplate("%{\nif a}", "x", "%{endif}", deep)},
		{"strip marker and comment", nestTemplate("%{~/**/if a}", "x", "%{endif}", deep)},
		{"newline before for", nestTemplate("%{\nfor x in [1]}", "x", "%{endfor}", deep)},
	}
	for _, c := range rejects {
		t.Run("rejects "+c.name, func(t *testing.T) {
			if err := hclconv.CheckNativeDepth(c.src); !errors.Is(err, hclconv.ErrNestingTooDeep) {
				t.Fatalf("CheckNativeDepth = %v, want ErrNestingTooDeep", err)
			}
		})
	}

	realistic := []byte(`x = <<EOT
%{ for k, v in m ~}
%{ if v != "" ~}
${k} = ${v}
%{ else ~}
${k} is empty
%{ endif ~}
%{ endfor ~}
EOT
`)
	accepts := []struct {
		name string
		src  []byte
	}{
		{"990 nested ifs", nestTemplate("%{if a}", "x", "%{endif}", 990)},
		{"50,000 sibling blocks", []byte(`x = "` + strings.Repeat("%{if a}y%{else}z%{endif}", 50_000) + `"` + "\n")},
		{"realistic heredoc template", realistic},
		{"stray endif first", []byte(`x = "%{endif}` + strings.Repeat("%{if a}", 990) + "x" + strings.Repeat("%{endif}", 990) + `"` + "\n")},
		{"escaped directive", nestTemplate("%%{if a}", "x", "%%{endif}", deep)},
	}
	for _, c := range accepts {
		t.Run("accepts "+c.name, func(t *testing.T) {
			if err := hclconv.CheckNativeDepth(c.src); err != nil {
				t.Fatalf("CheckNativeDepth = %v, want nil", err)
			}
		})
	}

	t.Run("escape is literal", func(t *testing.T) {
		toks, _ := hclsyntax.LexConfig([]byte(`x = "%%{if a}y%%{endif}"`), "", hcl.InitialPos)
		for _, tok := range toks {
			if tok.Type == hclsyntax.TokenTemplateControl {
				t.Fatalf("%%%%{ lexed as a template control token: %q", tok.Bytes)
			}
		}
	})
}
