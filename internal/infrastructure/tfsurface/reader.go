// Package tfsurface implements ports.SurfaceReader: it reads a module
// directory's declared variable and output names from its .tf, .tf.json,
// .tofu and .tofu.json files.
//
// The reader never under-counts silently: anything it cannot read
// completely makes the whole surface unknown, because an under-counted
// surface is a GRT001 false positive. A file with a syntax error, an
// unreadable file (an escaping or dangling symlink), an oversize file, an
// overly deeply nested file, or an invalid block (a label-less output, for
// example) each make the surface unknown rather than being skipped and
// letting the remaining files stand in for it. In order of precedence when
// several apply: module-file-unreadable > module-file-too-large >
// module-file-too-deep > syntax-error > invalid-module-block >
// no-terraform-files.
//
// A kept file is never handed to hclsyntax.ParseConfig or hcljson.Parse
// without first passing hclconv's size cap and nesting-depth pre-scan
// (02-REVIEW G7): both parsers recurse over their input, and Go cannot
// recover from the fatal stack overflow a hostile or generated file can
// cause, so refusing to parse is the only defence.
//
// In-repo file symlinks ARE followed (research Pitfall 1: the primary
// corpus symlinks global.tf into all 65 of its module directories), through
// whatever fs.FS the Reader is given; when that FS is backed by os.Root, an
// escaping or dangling link surfaces as a read error, which this package
// turns into module-file-unreadable.
package tfsurface

import (
	"context"
	"errors"
	"io/fs"
	"path"
	"strings"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	hcljson "github.com/hashicorp/hcl/v2/json"

	"github.com/GiulioSavini/gruntled/internal/application/ports"
	"github.com/GiulioSavini/gruntled/internal/domain/diagnostic"
	"github.com/GiulioSavini/gruntled/internal/domain/repograph"
	"github.com/GiulioSavini/gruntled/internal/infrastructure/hclconv"
)

// Unknown-reason constants (research Pattern 7 / Pattern 11), stable
// kebab-case so they are testable and greppable.
const (
	// ReasonModuleDirNotFound means the module directory does not exist
	// (or is not a directory) in the repository.
	ReasonModuleDirNotFound = "module-dir-not-found"
	// ReasonNoTerraformFiles means the directory exists but contains no
	// .tf, .tf.json, .tofu or .tofu.json file after ignored names and
	// OpenTofu precedence are applied.
	ReasonNoTerraformFiles = "no-terraform-files"
	// ReasonModuleFileUnreadable means a kept file could not be read: it
	// is a broken or escaping symlink, or fs.Stat/fs.ReadFile otherwise
	// failed.
	ReasonModuleFileUnreadable = "module-file-unreadable"
	// ReasonModuleFileTooLarge means a kept file exceeds
	// hclconv.MaxFileBytes. It is never parsed: hclsyntax.ParseConfig and
	// hcljson.Parse both recurse over their input, and their recursion
	// cannot be recovered from on a hostile or generated file (02-REVIEW
	// G7).
	ReasonModuleFileTooLarge = "module-file-too-large"
	// ReasonModuleFileTooDeep means a kept file's bracket, quote, heredoc,
	// template or unary-operator nesting exceeds hclconv.MaxNestingDepth.
	// It is never parsed, for the same reason as ReasonModuleFileTooLarge.
	ReasonModuleFileTooDeep = "module-file-too-deep"
	// ReasonSyntaxError means a kept file has invalid HCL or JSON syntax.
	// Its partial body is never analyzed.
	ReasonSyntaxError = "syntax-error"
	// ReasonInvalidModuleBlock means a kept file parsed, but a variable or
	// output block did not conform to the expected schema (for example a
	// label-less output block).
	ReasonInvalidModuleBlock = "invalid-module-block"
)

// moduleSchema selects only the variable and output blocks, each with
// exactly one label (the declared name).
var moduleSchema = &hcl.BodySchema{
	Blocks: []hcl.BlockHeaderSchema{
		{Type: "variable", LabelNames: []string{"name"}},
		{Type: "output", LabelNames: []string{"name"}},
	},
}

// Reader implements ports.SurfaceReader over an fs.FS whose paths are
// repo-relative and slash-separated.
type Reader struct {
	fsys fs.FS
}

// NewReader returns a Reader that reads module directories from fsys.
func NewReader(fsys fs.FS) *Reader {
	return &Reader{fsys: fsys}
}

var _ ports.SurfaceReader = (*Reader)(nil)

// ReadSurface implements ports.SurfaceReader.
func (r *Reader) ReadSurface(ctx context.Context, module repograph.RepoPath) (ports.SurfaceResult, error) {
	if err := ctx.Err(); err != nil {
		return ports.SurfaceResult{}, err
	}

	entries, err := fs.ReadDir(r.fsys, module.String())
	if err != nil {
		return ports.SurfaceResult{UnknownReason: ReasonModuleDirNotFound}, nil
	}

	kept := keepModuleFiles(entries)
	if len(kept) == 0 {
		return ports.SurfaceResult{UnknownReason: ReasonNoTerraformFiles}, nil
	}

	var (
		diags                                                  []diagnostic.Diagnostic
		vars, outs                                             = map[string]struct{}{}, map[string]struct{}{}
		unreadable, tooLarge, tooDeep, syntaxErr, invalidBlock bool
	)

	for _, name := range kept {
		rel := path.Join(module.String(), name)
		file, ferr := repograph.NewRepoPath(rel)
		if ferr != nil {
			unreadable = true
			continue
		}

		info, statErr := fs.Stat(r.fsys, rel)
		if statErr != nil {
			unreadable = true
			continue
		}
		if info.IsDir() {
			continue // a directory whose name happens to match a module extension
		}

		src, readErr := hclconv.ReadFileLimited(r.fsys, rel)
		if readErr != nil {
			if errors.Is(readErr, hclconv.ErrFileTooLarge) {
				tooLarge = true
			} else {
				unreadable = true
			}
			continue
		}

		var depthErr error
		if strings.HasSuffix(name, ".json") {
			depthErr = hclconv.CheckJSONDepth(src)
		} else {
			depthErr = hclconv.CheckNativeDepth(src)
		}
		if depthErr != nil {
			tooDeep = true
			continue // never hand a hostile nesting to a recursive parser
		}

		body, pdiags := parseModuleFile(name, rel, src)
		if pdiags.HasErrors() {
			syntaxErr = true
			if d, ok, derr := hclconv.FirstSyntaxError(file, src, pdiags); derr == nil && ok {
				diags = append(diags, d)
			}
			continue // never analyze the partial body
		}

		content, _, cdiags := body.PartialContent(moduleSchema)
		if cdiags.HasErrors() {
			invalidBlock = true
		}
		for _, b := range content.Blocks {
			if len(b.Labels) == 0 || b.Labels[0] == "" {
				continue
			}
			switch b.Type {
			case "variable":
				vars[b.Labels[0]] = struct{}{}
			case "output":
				outs[b.Labels[0]] = struct{}{}
			}
		}
	}

	switch {
	case unreadable:
		return ports.SurfaceResult{UnknownReason: ReasonModuleFileUnreadable}, nil
	case tooLarge:
		return ports.SurfaceResult{UnknownReason: ReasonModuleFileTooLarge}, nil
	case tooDeep:
		return ports.SurfaceResult{UnknownReason: ReasonModuleFileTooDeep}, nil
	case syntaxErr:
		return ports.SurfaceResult{UnknownReason: ReasonSyntaxError, Diagnostics: sortedDiags(diags)}, nil
	case invalidBlock:
		return ports.SurfaceResult{UnknownReason: ReasonInvalidModuleBlock}, nil
	}

	surface, err := repograph.NewSurface(setKeys(vars), setKeys(outs))
	if err != nil {
		return ports.SurfaceResult{}, err
	}
	return ports.SurfaceResult{Surface: surface}, nil
}

// parseModuleFile parses a kept module file's source with the parser that
// matches its extension: hclsyntax for .tf/.tofu, hcl/json for
// .tf.json/.tofu.json. Both parsers return a non-nil *hcl.File even for
// mid-edit or non-UTF-8 source (verified in research Pattern 8), so Body is
// always safe to read; the caller never calls PartialContent on it when
// diags.HasErrors(), per Pattern 8's "never analyze the partial body".
func parseModuleFile(name, rel string, src []byte) (hcl.Body, hcl.Diagnostics) {
	if strings.HasSuffix(name, ".json") {
		f, diags := hcljson.Parse(src, rel)
		return f.Body, diags
	}
	f, diags := hclsyntax.ParseConfig(src, rel, hcl.InitialPos)
	return f.Body, diags
}

// keepModuleFiles filters entries down to the module files this reader
// considers, in the order ReadDir returned them (lexical): names ending in
// .tf, .tf.json, .tofu or .tofu.json, minus Terraform's ignored names
// (prefix ".", suffix "~", or "#...#") and minus any .tf/.tf.json file that
// has a same-named .tofu/.tofu.json sibling (OpenTofu precedence).
func keepModuleFiles(entries []fs.DirEntry) []string {
	type candidate struct {
		name, ext, base string
	}
	var candidates []candidate
	tofuBases := map[string]bool{}
	tofuJSONBases := map[string]bool{}
	for _, e := range entries {
		name := e.Name()
		if isIgnoredFileName(name) {
			continue
		}
		ext, ok := moduleExt(name)
		if !ok {
			continue
		}
		base := strings.TrimSuffix(name, ext)
		candidates = append(candidates, candidate{name: name, ext: ext, base: base})
		switch ext {
		case ".tofu":
			tofuBases[base] = true
		case ".tofu.json":
			tofuJSONBases[base] = true
		}
	}
	kept := make([]string, 0, len(candidates))
	for _, c := range candidates {
		if c.ext == ".tf" && tofuBases[c.base] {
			continue
		}
		if c.ext == ".tf.json" && tofuJSONBases[c.base] {
			continue
		}
		kept = append(kept, c.name)
	}
	return kept
}

// isIgnoredFileName reports whether name is a Terraform-ignored file:
// prefix ".", suffix "~", or both prefix and suffix "#".
func isIgnoredFileName(name string) bool {
	if strings.HasPrefix(name, ".") {
		return true
	}
	if strings.HasSuffix(name, "~") {
		return true
	}
	if strings.HasPrefix(name, "#") && strings.HasSuffix(name, "#") {
		return true
	}
	return false
}

// moduleExt returns the module file extension category of name, checked
// most-specific first so ".tf.json"/".tofu.json" are never mistaken for
// plain ".tf"/".tofu".
func moduleExt(name string) (ext string, ok bool) {
	switch {
	case strings.HasSuffix(name, ".tf.json"):
		return ".tf.json", true
	case strings.HasSuffix(name, ".tofu.json"):
		return ".tofu.json", true
	case strings.HasSuffix(name, ".tf"):
		return ".tf", true
	case strings.HasSuffix(name, ".tofu"):
		return ".tofu", true
	}
	return "", false
}

// sortedDiags returns ds in the domain's canonical diagnostic order.
func sortedDiags(ds []diagnostic.Diagnostic) []diagnostic.Diagnostic {
	if len(ds) == 0 {
		return nil
	}
	return diagnostic.NewSet(ds...).All()
}

// setKeys returns m's keys as a slice in map order; repograph.NewSurface
// sorts and validates them, so no ordering is needed here.
func setKeys(m map[string]struct{}) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
