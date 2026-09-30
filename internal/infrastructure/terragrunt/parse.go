package terragrunt

import (
	"errors"
	"io/fs"
	"sort"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"

	"github.com/GiulioSavini/gruntled/internal/domain/diagnostic"
	"github.com/GiulioSavini/gruntled/internal/domain/repograph"
	"github.com/GiulioSavini/gruntled/internal/infrastructure/hclconv"
)

// includeDecl is one top-level `include` block, its facts unevaluated.
type includeDecl struct {
	// labels is the block's labels: zero for a bare `include { ... }`,
	// more than one is invalid (the loader decides what to do with it).
	labels []string
	// path is the `path` attribute's expression, nil when absent.
	path hcl.Expression
	// mergeStrategy is the `merge_strategy` attribute's expression, nil
	// when absent (Terragrunt's default is shallow).
	mergeStrategy hcl.Expression
	pos           repograph.Position
}

// terraformDecl is one top-level `terraform` block.
type terraformDecl struct {
	// source is the `source` attribute's expression, nil when the block
	// has no source attribute.
	source hcl.Expression
	pos    repograph.Position
}

// depDecl is one top-level `dependency` block.
type depDecl struct {
	// labels is the block's labels: exactly one is valid, the loader
	// decides what to do with zero or more than one.
	labels []string
	// configPath is the `config_path` attribute's expression, nil when
	// missing.
	configPath hcl.Expression
	// hasExpansion is true when the block has a nested `expansion` block
	// (Terragrunt v1.2.0), which makes the unit unknown at the loader.
	hasExpansion bool
	// opts is the block's DIAG-03 facts, captured by dependencyOptions.
	opts repograph.DependencyOptions
	// pos is the position of the `dependency` keyword itself
	// (block.TypeRange.Start).
	pos repograph.Position
	// cpPos is the position of the `config_path` attribute's value
	// expression, falling back to pos when the attribute is absent or its
	// position cannot be converted.
	cpPos repograph.Position
}

// generateDecl is one top-level `generate` block.
type generateDecl struct {
	labels []string
	// contents is the `contents` attribute's expression, nil when missing.
	contents hcl.Expression
	pos      repograph.Position
}

// parsedFile is every structural fact one HCL file (a unit's own
// terragrunt.hcl, or a file it includes) yields, extracted at most once
// and left unevaluated. readErr non-nil means the file could not even be
// read (or that an otherwise-impossible internal error happened while
// converting a position or a reference, which is handled the same way: the
// file becomes unusable, and nothing ever panics). syntax non-nil means
// the file was read but has an HCL syntax error, in which case no fact
// below is populated: hclsyntax's partial body on a syntax error could
// omit real blocks, so analysing it would under-count the file's facts
// (research Pattern 8), a false-positive risk this project does not take.
// limitReason non-empty means the file was read (or refused on its size
// alone) but deliberately not parsed, because it exceeds an hclconv limit
// (02-REVIEW G7); like readErr, it leaves every fact and syntax unset.
// Exactly one of readErr, limitReason and syntax is set when the file is
// unusable.
type parsedFile struct {
	path    repograph.RepoPath
	src     []byte
	readErr error
	syntax  *diagnostic.Diagnostic
	// limitReason is non-empty (ReasonConfigTooLarge or
	// ReasonConfigTooDeep) when the file was deliberately not parsed
	// because it exceeds an hclconv limit (02-REVIEW G7). Such a file has
	// no facts and no syntax diagnostic: it is not known to be invalid.
	limitReason string

	// Facts, populated only when readErr == nil, limitReason == "" and
	// syntax == nil. Each
	// slice is in source order: body.Blocks is already ordered, and
	// extractRefs sorts by byte offset.
	includes   []includeDecl
	terraforms []terraformDecl
	deps       []depDecl
	generates  []generateDecl
	refs       []repograph.Reference
}

// fileCache reads and parses each file at most once (research Pattern 2).
// It is single-threaded on purpose: parallelising it would need a per-key
// sync.Once, and syntaxDiagnostics' output would still need to be sorted
// afterward.
type fileCache struct {
	fsys  fs.FS
	files map[string]*parsedFile
}

// newFileCache returns a fileCache reading from fsys.
func newFileCache(fsys fs.FS) *fileCache {
	return &fileCache{fsys: fsys, files: map[string]*parsedFile{}}
}

// get returns p's parsedFile. The first call for a given p reads and
// parses the file; every later call, from however many distinct callers,
// returns the exact same *parsedFile without reading or parsing again. It
// never returns nil.
func (c *fileCache) get(p repograph.RepoPath) *parsedFile {
	key := p.String()
	if pf, ok := c.files[key]; ok {
		return pf
	}
	pf := c.parse(p)
	c.files[key] = pf
	return pf
}

// syntaxDiagnostics returns one GRT100 diagnostic per file get() has been
// called on and that had a syntax error, sorted by path. A file get() was
// never called on contributes nothing, and neither does a file that read
// and parsed without an HCL error.
func (c *fileCache) syntaxDiagnostics() []diagnostic.Diagnostic {
	var paths []string
	for k, pf := range c.files {
		if pf.syntax != nil {
			paths = append(paths, k)
		}
	}
	sort.Strings(paths)

	diags := make([]diagnostic.Diagnostic, 0, len(paths))
	for _, k := range paths {
		diags = append(diags, *c.files[k].syntax)
	}
	return diags
}

// parse reads and parses p exactly once. See parsedFile's doc comment for
// the readErr/limitReason/syntax/facts contract.
func (c *fileCache) parse(p repograph.RepoPath) *parsedFile {
	pf := &parsedFile{path: p}

	// G7: refuse a file over the size cap or the nesting limit before
	// hclsyntax ever sees it. ReadFileLimited opens the file at most once
	// (and never opens a non-regular one, 02-REVIEW G18), so parse-once
	// holds.
	src, err := hclconv.ReadFileLimited(c.fsys, p.String())
	if errors.Is(err, hclconv.ErrFileTooLarge) {
		pf.limitReason = ReasonConfigTooLarge
		return pf
	}
	if err != nil {
		pf.readErr = err
		return pf
	}
	if errors.Is(hclconv.CheckNativeDepth(src), hclconv.ErrNestingTooDeep) {
		pf.limitReason = ReasonConfigTooDeep
		return pf
	}
	pf.src = src

	f, diags := hclsyntax.ParseConfig(src, p.String(), hcl.InitialPos)
	if diags.HasErrors() {
		d, ok, err := hclconv.FirstSyntaxError(p, src, diags)
		if err != nil {
			pf.readErr = err
			return pf
		}
		if !ok {
			// diags.HasErrors() is true, so FirstSyntaxError always finds
			// an error-severity diagnostic; this branch exists so a
			// future change to that contract fails closed here instead of
			// silently discarding the file's error.
			pf.readErr = errors.New("terragrunt: internal error: syntax error diagnostics present but none found")
			return pf
		}
		pf.syntax = &d
		return pf
	}

	body, ok := f.Body.(*hclsyntax.Body)
	if !ok {
		// hclsyntax.ParseConfig always returns an *hclsyntax.Body when
		// diags has no errors; this exists so a future hcl change fails
		// closed here instead of panicking on the type assertion below.
		pf.readErr = errors.New("terragrunt: internal error: parsed body is not *hclsyntax.Body")
		return pf
	}

	for _, block := range body.Blocks {
		pos, err := hclconv.Position(p, src, block.TypeRange.Start)
		if err != nil {
			pf.readErr = err
			return pf
		}
		switch block.Type {
		case "include":
			pf.includes = append(pf.includes, includeDecl{
				labels:        block.Labels,
				path:          attrExpr(block.Body, "path"),
				mergeStrategy: attrExpr(block.Body, "merge_strategy"),
				pos:           pos,
			})
		case "terraform":
			pf.terraforms = append(pf.terraforms, terraformDecl{
				source: attrExpr(block.Body, "source"),
				pos:    pos,
			})
		case "dependency":
			cp := attrExpr(block.Body, "config_path")
			cpPos := pos
			if cp != nil {
				if vp, err := hclconv.Position(p, src, cp.Range().Start); err == nil {
					cpPos = vp
				}
			}
			pf.deps = append(pf.deps, depDecl{
				labels:       block.Labels,
				configPath:   cp,
				hasExpansion: hasBlock(block.Body, "expansion"),
				opts:         dependencyOptions(block.Body),
				pos:          pos,
				cpPos:        cpPos,
			})
		case "generate":
			pf.generates = append(pf.generates, generateDecl{
				labels:   block.Labels,
				contents: attrExpr(block.Body, "contents"),
				pos:      pos,
			})
		}
	}

	refs, err := extractRefs(p, src, body)
	if err != nil {
		pf.readErr = err
		return pf
	}
	pf.refs = refs

	return pf
}

// attrExpr returns name's expression from body, or nil when the attribute
// is absent. body.Attributes is a map, but a single named lookup by a
// fixed key is order-independent, unlike ranging over it.
func attrExpr(body *hclsyntax.Body, name string) hcl.Expression {
	attr, ok := body.Attributes[name]
	if !ok {
		return nil
	}
	return attr.Expr
}

// hasBlock reports whether body has at least one top-level block of the
// given type.
func hasBlock(body *hclsyntax.Body, blockType string) bool {
	for _, b := range body.Blocks {
		if b.Type == blockType {
			return true
		}
	}
	return false
}
