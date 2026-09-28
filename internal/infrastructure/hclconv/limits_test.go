package hclconv_test

import (
	"errors"
	"io/fs"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/GiulioSavini/gruntled/internal/infrastructure/hclconv"
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

// countingReadFS wraps an fstest.MapFS and counts ReadFile calls, so a
// test can assert ReadFileLimited calls fs.ReadFile at most once.
type countingReadFS struct {
	fstest.MapFS
	reads int
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

	t.Run("success path reads exactly once", func(t *testing.T) {
		fsys := &countingReadFS{MapFS: fstest.MapFS{
			"f.tf": &fstest.MapFile{Data: []byte(`a = 1`)},
		}}
		if _, err := hclconv.ReadFileLimited(fsys, "f.tf"); err != nil {
			t.Fatalf("ReadFileLimited: unexpected error: %v", err)
		}
		if fsys.reads != 1 {
			t.Fatalf("ReadFile calls = %d, want 1", fsys.reads)
		}
	})
}

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

	t.Run("256 KiB ternary chain", func(t *testing.T) {
		const unit = "x ? 1 : "
		n := (256 * 1024) / len(unit)
		src := wrap(strings.Repeat(unit, n), "0", "")
		if err := hclconv.CheckNativeDepth(src); err != nil {
			t.Fatalf("CheckNativeDepth: %v, want nil", err)
		}
	})

	t.Run("256 KiB plus chain", func(t *testing.T) {
		const unit = "x + "
		n := (256 * 1024) / len(unit)
		src := wrap(strings.Repeat(unit, n), "x", "")
		if err := hclconv.CheckNativeDepth(src); err != nil {
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
