package terragrunt

import (
	"errors"
	"io/fs"
	"path"
	"strings"

	"github.com/zclconf/go-cty/cty"
	"github.com/zclconf/go-cty/cty/function"
)

// virtualRoot is the sentinel absolute path the six functions return for a
// repo-relative directory, so a value stays checkout-independent: no real
// absolute path can ever leak out of an evaluated expression.
const virtualRoot = "/__gruntled_repo_root__"

// virtual returns the virtual-absolute form of a repo-relative directory p.
// virtual(".") is exactly virtualRoot, with no trailing slash.
func virtual(p string) string {
	if p == "." {
		return virtualRoot
	}
	return virtualRoot + "/" + p
}

// scopeKind is which of the three evaluation scopes (research Pattern 4) an
// expression is evaluated in.
type scopeKind int

const (
	// scopeInclude is S0: the child's own include { path = ... } block.
	// TrackInclude is nil here, matching Terragrunt's WithTrackInclude(nil)
	// before DecodeBaseBlocks.
	scopeInclude scopeKind = iota + 1
	// scopeUnit is S1: the child unit's own body.
	scopeUnit
	// scopeIncluded is S2: an expression written inside an included file.
	scopeIncluded
)

// includeRef is one include block of the unit being evaluated, in
// declaration order, including no_merge includes (path_relative_* and
// get_parent_terragrunt_dir see every include, merge exclusion is a
// separate later concern).
type includeRef struct {
	label string
	dir   string // repo-relative dir of the included file
}

// evalScope is everything the six path functions need to answer: which
// repo they are reading (for find_in_parent_folders), which unit's
// directory they resolve against, which of the three scopes they are
// evaluated in, the unit's own includes (for S1), and the dir of the
// included file the expression lives in (for S2).
type evalScope struct {
	fsys     fs.FS
	unitDir  string // repo-relative dir of the CHILD unit ("." for the repo root)
	kind     scopeKind
	includes []includeRef // every include block of the unit, declaration order
	included string       // S2: repo-relative dir of the included file the expression is written in
}

// errTooManyArgs is find_in_parent_folders called with more than the two
// arguments (name, fallback) it supports.
var errTooManyArgs = errors.New("terragrunt: find_in_parent_folders: too many arguments")

// errInvalidArg is returned both when an argument to one of the six
// functions is present but not a known, non-null string (which can only
// happen for a nested call this closed EvalContext lets through, since
// every literal argument is already typed cty.String by its VarParam), and
// when path_relative_to_include, path_relative_from_include or
// get_parent_terragrunt_dir is called in S1 with 2+ includes and no
// argument selecting one by label, or an argument naming a label that does
// not exist.
var errInvalidArg = errors.New("terragrunt: invalid argument")

// errNotFoundInRepo is find_in_parent_folders exhausting every ancestor up
// to and including the repo root without a match and without a fallback.
var errNotFoundInRepo = errors.New("terragrunt: find_in_parent_folders: not found in repository")

// functions returns exactly the six Terragrunt path functions, closed over
// s. Every other construct (any other function name, or a variable) fails
// closed because the EvalContext built from this map has Variables: nil and
// no other function.
func (s evalScope) functions() map[string]function.Function {
	return map[string]function.Function{
		"get_terragrunt_dir": function.New(&function.Spec{
			Type: function.StaticReturnType(cty.String),
			Impl: func(args []cty.Value, retType cty.Type) (cty.Value, error) {
				return cty.StringVal(virtual(s.unitDir)), nil
			},
		}),
		"get_original_terragrunt_dir": function.New(&function.Spec{
			Type: function.StaticReturnType(cty.String),
			Impl: func(args []cty.Value, retType cty.Type) (cty.Value, error) {
				return cty.StringVal(virtual(s.unitDir)), nil
			},
		}),
		"find_in_parent_folders": function.New(&function.Spec{
			VarParam: &function.Parameter{Name: "args", Type: cty.String},
			Type:     function.StaticReturnType(cty.String),
			Impl: func(args []cty.Value, retType cty.Type) (cty.Value, error) {
				return s.findInParentFolders(args)
			},
		}),
		"path_relative_to_include": function.New(&function.Spec{
			VarParam: &function.Parameter{Name: "name", Type: cty.String},
			Type:     function.StaticReturnType(cty.String),
			Impl: func(args []cty.Value, retType cty.Type) (cty.Value, error) {
				return s.pathRelativeToInclude(args)
			},
		}),
		"path_relative_from_include": function.New(&function.Spec{
			VarParam: &function.Parameter{Name: "name", Type: cty.String},
			Type:     function.StaticReturnType(cty.String),
			Impl: func(args []cty.Value, retType cty.Type) (cty.Value, error) {
				return s.pathRelativeFromInclude(args)
			},
		}),
		"get_parent_terragrunt_dir": function.New(&function.Spec{
			VarParam: &function.Parameter{Name: "name", Type: cty.String},
			Type:     function.StaticReturnType(cty.String),
			Impl: func(args []cty.Value, retType cty.Type) (cty.Value, error) {
				return s.getParentTerragruntDir(args)
			},
		}),
	}
}

// argStrings converts every arg to a plain string, failing if any arg is
// not wholly known or is null. In the closed EvalContext this package
// builds (Variables nil, six functions only), an unknown or null arg can
// only come from a nested call to one of the six functions that itself
// failed, which already surfaces as a diagnostic; this guard exists so
// that case fails closed here too rather than panicking on AsString().
func argStrings(args []cty.Value) ([]string, bool) {
	out := make([]string, len(args))
	for i, a := range args {
		if !a.IsWhollyKnown() || a.IsNull() {
			return nil, false
		}
		out[i] = a.AsString()
	}
	return out, true
}

// findInParentFolders mirrors terragrunt pkg/config/config_helpers.go
// findInParentFoldersImpl: probe each ancestor of s.unitDir, starting at
// its parent, up to and including the repo root.
func (s evalScope) findInParentFolders(args []cty.Value) (cty.Value, error) {
	if len(args) > 2 {
		return cty.NilVal, errTooManyArgs
	}
	names, ok := argStrings(args)
	if !ok {
		return cty.NilVal, errInvalidArg
	}
	name := ""
	fallback, hasFallback := "", len(names) == 2
	if len(names) >= 1 {
		name = names[0]
	}
	if hasFallback {
		fallback = names[1]
	}
	dir := s.unitDir
	for dir != "." {
		dir = path.Dir(dir)
		var candidates []string
		if name == "" || name == "terragrunt.hcl" {
			candidates = []string{path.Join(dir, "terragrunt.hcl.json"), path.Join(dir, "terragrunt.hcl")}
		} else {
			candidates = []string{path.Join(dir, name)}
		}
		for _, c := range candidates {
			if _, err := fs.Stat(s.fsys, c); err == nil {
				return cty.StringVal(virtual(c)), nil
			}
		}
	}
	if hasFallback {
		return cty.StringVal(fallback), nil
	}
	return cty.NilVal, errNotFoundInRepo
}

// pathRelativeToInclude implements path_relative_to_include per the
// research Pattern 4 table.
func (s evalScope) pathRelativeToInclude(args []cty.Value) (cty.Value, error) {
	switch s.kind {
	case scopeInclude:
		return cty.StringVal("."), nil
	case scopeIncluded:
		return cty.StringVal(relDir(s.included, s.unitDir)), nil
	default: // scopeUnit
		rel, ok := toIncludeRel(s, args)
		if !ok {
			return cty.NilVal, errInvalidArg
		}
		return cty.StringVal(rel), nil
	}
}

// pathRelativeFromInclude implements path_relative_from_include per the
// research Pattern 4 table.
func (s evalScope) pathRelativeFromInclude(args []cty.Value) (cty.Value, error) {
	switch s.kind {
	case scopeInclude:
		return cty.StringVal("."), nil
	case scopeIncluded:
		return cty.StringVal(relDir(s.unitDir, s.included)), nil
	default: // scopeUnit
		rel, ok := fromIncludeRel(s, args)
		if !ok {
			return cty.NilVal, errInvalidArg
		}
		return cty.StringVal(rel), nil
	}
}

// getParentTerragruntDir implements get_parent_terragrunt_dir per the
// research Pattern 4 table.
func (s evalScope) getParentTerragruntDir(args []cty.Value) (cty.Value, error) {
	switch s.kind {
	case scopeInclude:
		return cty.StringVal(virtual(s.unitDir)), nil
	case scopeIncluded:
		return cty.StringVal(virtual(s.included)), nil
	default: // scopeUnit
		from, ok := fromIncludeRel(s, args)
		if !ok {
			return cty.NilVal, errInvalidArg
		}
		return cty.StringVal(virtual(path.Join(s.unitDir, from))), nil
	}
}

// toIncludeRel selects the S1 (scopeUnit) include for path_relative_to_include:
// "." with zero includes, the single include's dir with exactly one
// (any argument ignored), or the include named by the sole string argument
// when there are two or more. It fails when there are 2+ includes and the
// argument is missing or does not name any include's label.
func toIncludeRel(s evalScope, args []cty.Value) (string, bool) {
	names, ok := argStrings(args)
	if !ok {
		return "", false
	}
	switch len(s.includes) {
	case 0:
		return ".", true
	case 1:
		return relDir(s.includes[0].dir, s.unitDir), true
	default:
		if len(names) != 1 {
			return "", false
		}
		for _, inc := range s.includes {
			if inc.label == names[0] {
				return relDir(inc.dir, s.unitDir), true
			}
		}
		return "", false
	}
}

// fromIncludeRel is toIncludeRel's mirror for path_relative_from_include
// and get_parent_terragrunt_dir.
func fromIncludeRel(s evalScope, args []cty.Value) (string, bool) {
	names, ok := argStrings(args)
	if !ok {
		return "", false
	}
	switch len(s.includes) {
	case 0:
		return ".", true
	case 1:
		return relDir(s.unitDir, s.includes[0].dir), true
	default:
		if len(names) != 1 {
			return "", false
		}
		for _, inc := range s.includes {
			if inc.label == names[0] {
				return relDir(s.unitDir, inc.dir), true
			}
		}
		return "", false
	}
}

// relDir mirrors filepath.Rel for cleaned, slash-separated, repo-relative
// dirs ("." meaning the repo root).
func relDir(from, to string) string {
	split := func(p string) []string {
		if p == "." {
			return nil
		}
		return strings.Split(p, "/")
	}
	f, t := split(from), split(to)
	i := 0
	for i < len(f) && i < len(t) && f[i] == t[i] {
		i++
	}
	parts := make([]string, 0, len(f)-i+len(t)-i)
	for range f[i:] {
		parts = append(parts, "..")
	}
	parts = append(parts, t[i:]...)
	if len(parts) == 0 {
		return "."
	}
	return strings.Join(parts, "/")
}
