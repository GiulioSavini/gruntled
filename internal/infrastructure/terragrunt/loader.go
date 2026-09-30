package terragrunt

import (
	"context"
	"io/fs"
	"path"
	"slices"
	"sort"
	"strings"

	"github.com/hashicorp/hcl/v2"

	"github.com/GiulioSavini/gruntled/internal/application/ports"
	"github.com/GiulioSavini/gruntled/internal/domain/repograph"
	"github.com/GiulioSavini/gruntled/internal/infrastructure/sourceresolve"
)

// Loader implements ports.UnitLoader over an fs.FS whose paths are
// repo-relative and slash-separated.
type Loader struct {
	fsys fs.FS
}

// NewLoader returns a Loader reading from fsys.
func NewLoader(fsys fs.FS) *Loader {
	return &Loader{fsys: fsys}
}

var _ ports.UnitLoader = (*Loader)(nil)

// LoadUnits implements ports.UnitLoader. It discovers every unit with
// discoverUnits, sorts them by RepoPath order (fs.WalkDir's lexical order is
// NOT RepoPath order: a depth-first walk visits "a/b" before its sibling
// "a-b", even though "a-b" < "a/b" as strings), and resolves each one
// through a single fileCache shared by the whole call, so every unit file
// and every include file is read and parsed at most once no matter how many
// units share it (research Pattern 2 / PARSE-03).
//
// Per-unit and per-file problems are never a Go error: they become
// UnknownReason fields on the returned UnitConfig and diagnostics in
// LoadResult.Diagnostics. error is reserved for context cancellation and a
// root-walk failure that makes the whole load meaningless.
func (l *Loader) LoadUnits(ctx context.Context) (ports.LoadResult, error) {
	if err := ctx.Err(); err != nil {
		return ports.LoadResult{}, err
	}

	entries, err := discoverUnits(l.fsys)
	if err != nil {
		return ports.LoadResult{}, err
	}
	sort.SliceStable(entries, func(i, j int) bool {
		return strings.Compare(entries[i].dir, entries[j].dir) < 0
	})

	cache := newFileCache(l.fsys)
	targets := newIncludeTargets()
	units := make([]ports.UnitConfig, 0, len(entries))
	for _, e := range entries {
		if err := ctx.Err(); err != nil {
			return ports.LoadResult{}, err
		}
		units = append(units, l.resolveUnit(cache, e, targets))
	}

	// 13. Include-target post-pass (research Pattern 4 / G3): any unit
	// whose own <dir>/terragrunt.hcl was located by some OTHER unit's
	// resolveIncludes becomes config-unknown ReasonIncludeTarget, UNLESS it
	// already has an earlier config-unknown reason of its own (the first
	// check that applies always wins, consistent with resolveUnit's fixed
	// order). A unit is a target under three rules (includeTargets): an
	// exact match of some include's canonical in-repo path (02-REVIEW G15;
	// unit directories are already canonical, since discoverUnits never
	// enters a symlinked directory), an ancestor of a unit whose includes
	// are unknowable or failed, or an include-free unit once some include's
	// file name is dynamic or terragrunt.hcl (G16). Its dependencies and references are dropped from this
	// standalone interpretation; they are still checked, correctly, once
	// per including unit, since mergeReferences attributes include-file
	// facts to the including unit, not to the include-target unit itself.
	for i, u := range units {
		if u.ConfigUnknownReason != "" {
			continue
		}
		if !targets.isTarget(u.Path.String()) {
			continue
		}
		units[i] = ports.UnitConfig{Path: u.Path, ConfigUnknownReason: ReasonIncludeTarget}
	}

	return ports.LoadResult{Units: units, Diagnostics: cache.syntaxDiagnostics()}, nil
}

// resolveUnit computes e's ports.UnitConfig against cache, in the fixed
// order documented inline below (plus the LoadUnits-level step 13
// include-target post-pass, which runs after every unit has gone through
// this function once): the first check that applies decides the unit's
// config-unknown or module-unknown state. A unit file or include file that
// exceeds an hclconv size or nesting limit is never parsed and makes the
// unit config-unknown ReasonConfigTooLarge / ReasonConfigTooDeep, never
// partially known (02-REVIEW G7). It never returns a Go error and
// never panics: a domain constructor failure that "should be impossible"
// given the checks already performed becomes config-unknown
// ReasonInvalidDependency instead of propagating, so a future change to a
// domain invariant fails closed here rather than crashing on user input.
// targets accumulates, across the whole LoadUnits call, every unit that
// may be some other unit's parent config (markIncludeDecls, plus
// markAncestors on every early return in steps 1 to 4), so step 13 can
// find every include-target unit afterward.
func (l *Loader) resolveUnit(cache *fileCache, e unitEntry, targets *includeTargets) ports.UnitConfig {
	unitDir := e.dir
	unitPath, err := repograph.NewRepoPath(unitDir)
	if err != nil {
		// discoverUnits only ever yields clean, repo-relative directories
		// (path.Dir of a walked file path). NewRepoPath can only fail here
		// if a future change to discoverUnits starts yielding an unclean or
		// escaping directory; the zero UnitConfig this returns is rejected
		// by indexing.Build's assemble stage (a resolved unit must have a
		// non-zero Path), which turns it into a whole-load error rather
		// than silently dropping the unit -- a loud failure, not a silent
		// one.
		return ports.UnitConfig{}
	}

	// 1. terragrunt.hcl.json beats terragrunt.hcl (Terragrunt's own
	// DefaultTerragruntConfigPaths order); terragrunt.autoinclude.hcl is a
	// Stacks feature this domain does not model.
	// Every early return in steps 1 and 2 marks the unit's ancestors as
	// include targets: its includes are unknowable, and
	// find_in_parent_folders reaches every ancestor (02-REVIEW G16).
	if e.jsonConfig {
		targets.markAncestors(unitDir)
		return ports.UnitConfig{Path: unitPath, ConfigUnknownReason: ReasonJSONConfigUnsupported}
	}
	if info, statErr := fs.Stat(l.fsys, path.Join(unitDir, "terragrunt.autoinclude.hcl")); statErr == nil && !info.IsDir() {
		targets.markAncestors(unitDir)
		return ports.UnitConfig{Path: unitPath, ConfigUnknownReason: ReasonAutoincludeUnsupported}
	}

	// 2. Read and parse the unit's own terragrunt.hcl. A read failure wins,
	// then a file refused for exceeding an hclconv size or nesting limit
	// (G7: never parsed, no GRT100), then a syntax error.
	unitFile, err := repograph.NewRepoPath(path.Join(unitDir, "terragrunt.hcl"))
	if err != nil {
		targets.markAncestors(unitDir)
		return ports.UnitConfig{Path: unitPath, ConfigUnknownReason: ReasonUnreadableConfig}
	}
	childPF := cache.get(unitFile)
	if childPF.readErr != nil {
		targets.markAncestors(unitDir)
		return ports.UnitConfig{Path: unitPath, ConfigUnknownReason: ReasonUnreadableConfig}
	}
	if childPF.limitReason != "" {
		targets.markAncestors(unitDir)
		return ports.UnitConfig{Path: unitPath, ConfigUnknownReason: childPF.limitReason}
	}
	if childPF.syntax != nil {
		targets.markAncestors(unitDir)
		return ports.UnitConfig{Path: unitPath, ConfigUnknownReason: ReasonSyntaxError}
	}
	if len(childPF.includes) == 0 {
		targets.noteIncludeFree(unitDir)
	}
	l.markIncludeDecls(targets, unitDir, childPF.includes)

	// 3. Structural include validation, before any path is evaluated. A
	// failure here or in step 4 also marks the unit's ancestors (the
	// fail-closed direction; markIncludeDecls already covered the precise
	// cases).
	if reason := validateIncludeDecls(childPF.includes); reason != "" {
		targets.markAncestors(unitDir)
		return ports.UnitConfig{Path: unitPath, ConfigUnknownReason: reason}
	}

	// 4. Resolve each include: evaluate its path, confirm it names an
	// existing regular file inside the repo, read and parse it once
	// (shared across every unit that includes it), and reject a second
	// level of include.
	resolved, reason := l.resolveIncludes(cache, unitDir, unitFile, childPF.includes)
	if reason != "" {
		targets.markAncestors(unitDir)
		return ports.UnitConfig{Path: unitPath, ConfigUnknownReason: reason}
	}
	childRefs := toIncludeRefs(resolved)

	// 5. Effective files in precedence order (child, then non-no_merge
	// includes last-to-first).
	files := buildEffectiveFiles(childPF, unitDir, resolved)

	// 6. Validate each effective file's own terraform/dependency shape.
	for _, ef := range files {
		if reason := validateEffectiveFile(ef); reason != "" {
			return ports.UnitConfig{Path: unitPath, ConfigUnknownReason: reason}
		}
	}

	// 7. Merge and resolve dependencies by label.
	byLabel := collectDependencyLabels(files)
	deps, invalid := l.resolveDependencies(unitDir, childRefs, byLabel)
	if invalid {
		return ports.UnitConfig{Path: unitPath, ConfigUnknownReason: ReasonInvalidDependency}
	}

	// 8. References, from every effective file, sorted by position.
	refs := mergeReferences(files)

	// Config is now fully known. 9. Resolve the module via terraform.source
	// (GRAPH-01/02/03).
	modulePath, moduleUnknownReason := l.resolveSource(unitDir, childRefs, files)

	// 10. A generate block that may declare outputs makes the module
	// unknown too, but only when the source itself already resolved.
	if moduleUnknownReason == "" {
		if reason := mergeGenerateUnknownReason(files); reason != "" {
			moduleUnknownReason = reason
		}
	}

	// 11. The unit directory overlaying the module's own files, only
	// meaningful when the module is somewhere other than the unit itself.
	// A ReadDir failure here (G6) makes the module unknown too, since
	// whether it overlays is itself unreadable.
	if moduleUnknownReason == "" && modulePath.Compare(unitPath) != 0 {
		if overlays, reason := unitDirOverlaysModule(l.fsys, unitDir); reason != "" {
			moduleUnknownReason = reason
		} else if overlays {
			moduleUnknownReason = ReasonUnitDirOverlaysModule
		}
	}

	// 12. Emit.
	if moduleUnknownReason != "" {
		return ports.UnitConfig{
			Path:                unitPath,
			ModuleUnknownReason: moduleUnknownReason,
			Dependencies:        deps,
			References:          refs,
		}
	}
	return ports.UnitConfig{
		Path:         unitPath,
		Module:       modulePath,
		Dependencies: deps,
		References:   refs,
	}
}

// markIncludeDecls records, for EVERY include decl of unitDir, which units
// it may name as a parent config. It runs before steps 3 and 4 and
// independently of their early returns, so a decl after a failing one
// still marks its target (02-REVIEW G16):
//   - a nil path marks unitDir's ancestors;
//   - a path that does not evaluate but provably names no in-repo unit
//     (includeReachesNoUnit) marks nothing;
//   - any other path that does not evaluate marks the ancestors, and also every
//     include-free unit unless its file name is fixed and is not
//     terragrunt.hcl (dynamicIncludeFileNames);
//   - a path that evaluates but names no in-repo regular file marks
//     nothing;
//   - a regular file is marked exactly by its lexical and canonical path;
//     if its canonical path cannot be proven, the ancestors and every
//     include-free unit are marked instead.
func (l *Loader) markIncludeDecls(targets *includeTargets, unitDir string, decls []includeDecl) {
	for _, d := range decls {
		if d.path == nil {
			targets.markAncestors(unitDir)
			continue
		}
		raw, ok := evalPath(d.path, evalScope{fsys: l.fsys, unitDir: unitDir, kind: scopeInclude})
		if !ok && includeReachesNoUnit(d.path) {
			continue
		}
		if !ok {
			targets.markAncestors(unitDir)
			names, fixed := dynamicIncludeFileNames(d.path)
			if !fixed || slices.Contains(names, "terragrunt.hcl") {
				targets.markAllIncludeFree()
			}
			continue
		}
		p, ok := resolvePath(unitDir, raw)
		if !ok {
			continue
		}
		info, statErr := fs.Stat(l.fsys, p)
		if statErr != nil || !info.Mode().IsRegular() {
			continue
		}
		canon, cerr := canonicalPath(l.fsys, p)
		if cerr != canonOK {
			targets.markAncestors(unitDir)
			targets.markAllIncludeFree()
			continue
		}
		targets.markExact(p, canon)
	}
}

// resolveIncludes evaluates and resolves unitDir's already
// structurally-valid include decls, in declaration order. unitFile is the
// unit's own terragrunt.hcl (an include resolving back to it is
// self-inclusion, invalid). Every include path that stats as an existing
// regular in-repo file is canonicalized (canonicalPath, 02-REVIEW G15).
// It marks no include target itself: markIncludeDecls already did, for
// every decl, before this runs. A path reaching its target through a symlink that escapes
// the repository is ReasonIncludeOutsideRepo; one whose symlinks cannot be
// resolved is ReasonUnreadableConfig. The JSON refusal, the
// duplicate-include check and the self-inclusion check compare canonical
// paths, so an alias cannot hide either; the file itself is still read
// through its lexical path, so reference positions keep the path the user
// wrote. It returns "" for reason on success.
func (l *Loader) resolveIncludes(cache *fileCache, unitDir string, unitFile repograph.RepoPath, decls []includeDecl) ([]resolvedInclude, string) {
	var resolved []resolvedInclude
	seenFiles := map[string]bool{}

	for _, d := range decls {
		scope := evalScope{fsys: l.fsys, unitDir: unitDir, kind: scopeInclude}
		raw, ok := evalPath(d.path, scope)
		if !ok {
			return nil, ReasonIncludeDynamicPath
		}
		p, ok := resolvePath(unitDir, raw)
		if !ok {
			return nil, ReasonIncludeOutsideRepo
		}
		info, statErr := fs.Stat(l.fsys, p)
		if statErr != nil || !info.Mode().IsRegular() {
			return nil, ReasonIncludeNotFound
		}
		canon, cerr := canonicalPath(l.fsys, p)
		switch cerr {
		case canonOutside:
			return nil, ReasonIncludeOutsideRepo
		case canonUnresolvable:
			return nil, ReasonUnreadableConfig
		}

		// G5: Terragrunt's own DefaultTerragruntConfigPaths (and
		// find_in_parent_folders' probe order, mirrored in
		// pathfuncs.go) prefers terragrunt.hcl.json over terragrunt.hcl.
		// This domain does not parse JSON Terragrunt configs.
		if strings.HasSuffix(p, ".json") || strings.HasSuffix(canon, ".json") {
			return nil, ReasonIncludeJSONUnsupported
		}

		if seenFiles[canon] || canon == unitFile.String() {
			return nil, ReasonInvalidInclude
		}
		seenFiles[canon] = true

		file, pathErr := repograph.NewRepoPath(p)
		if pathErr != nil {
			return nil, ReasonIncludeNotFound
		}
		pf := cache.get(file)
		if pf.readErr != nil {
			return nil, ReasonUnreadableConfig
		}
		if pf.limitReason != "" {
			return nil, pf.limitReason
		}
		if pf.syntax != nil {
			return nil, ReasonSyntaxError
		}
		if len(pf.includes) > 0 {
			return nil, ReasonNestedInclude
		}

		label := ""
		if len(d.labels) == 1 {
			label = d.labels[0]
		}
		noMerge, deep := mergeStrategyOf(d)

		resolved = append(resolved, resolvedInclude{
			label:   label,
			dir:     path.Dir(p),
			pf:      pf,
			noMerge: noMerge,
			deep:    deep,
		})
	}

	return resolved, ""
}

// resolveDependencies merges byLabel's occurrences per research Pattern 6
// (child > last include > ... > first include for a label appearing once or
// under shallow merge; deep merge keeps config_path precedence but marks
// every option fact unknown when the label appears in more than one file)
// and resolves each into a repograph.Dependency, sorted by label for
// determinism. invalid is true when a label's effective config_path is
// nil after merge, or a domain constructor rejects an otherwise-validated
// dependency: the caller turns the whole unit config-unknown in that case.
func (l *Loader) resolveDependencies(unitDir string, childRefs []includeRef, byLabel map[string][]depOccurrence) ([]repograph.Dependency, bool) {
	labels := make([]string, 0, len(byLabel))
	for label := range byLabel {
		labels = append(labels, label)
	}
	sort.Strings(labels)

	var deps []repograph.Dependency
	for _, label := range labels {
		occs := byLabel[label]
		deep := false
		for _, o := range occs {
			if o.ef.kind == scopeIncluded && o.ef.deep {
				deep = true
			}
		}

		var chosen depOccurrence
		var found bool
		var pos repograph.Position
		var opts repograph.DependencyOptions
		var pathPos repograph.Position

		if len(occs) == 1 || !deep {
			chosen = occs[0]
			found = chosen.decl.configPath != nil
			pos = chosen.decl.pos
			opts = chosen.decl.opts
			pathPos = chosen.decl.cpPos
		} else {
			pos = occs[0].decl.pos
			opts = repograph.DependencyOptions{}
			for _, o := range occs {
				if o.decl.configPath != nil {
					chosen = o
					found = true
					pathPos = o.decl.cpPos
					break
				}
			}
		}

		if !found {
			return nil, true
		}

		scope := fileScope(l.fsys, unitDir, childRefs, chosen.ef)
		dep, ok := l.resolveOneDependency(label, chosen.decl.configPath, scope, unitDir, pos, pathPos, opts)
		if !ok {
			return nil, true
		}
		deps = append(deps, dep)
	}
	return deps, false
}

// resolveOneDependency evaluates cpExpr in scope and resolves it against
// unitDir (always the CHILD unit dir, even when cpExpr is written in an
// include -- research Pitfall 4), then builds a resolved or unresolved
// repograph.Dependency. There are four outcomes:
//
//  1. cpExpr fails closed evaluation, or its evaluated path escapes the
//     repository: unresolved, ReasonConfigPathDynamic / ReasonConfigPathOutsideRepo.
//  2. The resolved path is a regular file named terragrunt.stack.hcl, or a
//     directory holding one (research Pattern 3: Terragrunt's
//     getTerragruntOutput tries tryGetStackOutput first, so the stack
//     always wins over a sibling terragrunt.hcl): unresolved,
//     ReasonConfigPathStack.
//  3. The resolved path is a regular file with any other name: unresolved,
//     ReasonConfigPathNondefaultFile (Terragrunt reads THAT file, which may
//     set a different source than the directory's own terragrunt.hcl).
//     Named "terragrunt.hcl" itself, it maps to its directory as before.
//  4. The resulting target directory is not a valid RepoPath (for example a
//     literal backslash surviving resolvePath as part of a path segment):
//     unresolved, ReasonConfigPathInvalid. Only this one dependency is
//     affected; the unit and its sibling dependencies stay resolved (G8).
//
// ok is false only on a domain constructor rejection ("should be
// impossible" after the checks above).
func (l *Loader) resolveOneDependency(label string, cpExpr hcl.Expression, scope evalScope, unitDir string, pos, pathPos repograph.Position, opts repograph.DependencyOptions) (repograph.Dependency, bool) {
	raw, ok := evalPath(cpExpr, scope)
	if !ok {
		d, err := repograph.NewUnresolvedDependency(label, ReasonConfigPathDynamic, pos, pathPos, opts)
		return d, err == nil
	}
	p, ok := resolvePath(unitDir, raw)
	if !ok {
		d, err := repograph.NewUnresolvedDependency(label, ReasonConfigPathOutsideRepo, pos, pathPos, opts)
		return d, err == nil
	}

	targetDir := p
	if info, statErr := fs.Stat(l.fsys, p); statErr == nil && info.Mode().IsRegular() {
		switch path.Base(p) {
		case "terragrunt.hcl":
			targetDir = path.Dir(p)
		case "terragrunt.stack.hcl":
			d, err := repograph.NewUnresolvedDependency(label, ReasonConfigPathStack, pos, pathPos, opts)
			return d, err == nil
		default:
			d, err := repograph.NewUnresolvedDependency(label, ReasonConfigPathNondefaultFile, pos, pathPos, opts)
			return d, err == nil
		}
	}

	targetPath, pathErr := repograph.NewRepoPath(targetDir)
	if pathErr != nil {
		d, err := repograph.NewUnresolvedDependency(label, ReasonConfigPathInvalid, pos, pathPos, opts)
		return d, err == nil
	}

	if info, statErr := fs.Stat(l.fsys, path.Join(targetDir, "terragrunt.stack.hcl")); statErr == nil && info.Mode().IsRegular() {
		d, err := repograph.NewUnresolvedDependency(label, ReasonConfigPathStack, pos, pathPos, opts)
		return d, err == nil
	}

	d, err := repograph.NewDependency(label, targetPath, pos, pathPos, repograph.TargetUnknown, opts)
	return d, err == nil
}

// resolveSource finds the highest-precedence effective file with a
// terraform.source attribute, evaluates and classifies it (research Pattern
// 9), and resolves a local source into a module RepoPath. When no effective
// file has a source at all, the module is the unit's own directory
// (GRAPH-02). moduleUnknownReason is "" exactly when modulePath is valid.
func (l *Loader) resolveSource(unitDir string, childRefs []includeRef, files []effectiveFile) (repograph.RepoPath, string) {
	for _, ef := range files {
		if len(ef.pf.terraforms) != 1 || ef.pf.terraforms[0].source == nil {
			continue
		}

		scope := fileScope(l.fsys, unitDir, childRefs, ef)
		raw, ok := evalPath(ef.pf.terraforms[0].source, scope)
		if !ok {
			return repograph.RepoPath{}, ReasonSourceDynamicPath
		}

		src := sourceresolve.Classify(raw)
		switch src.Kind {
		case sourceresolve.KindRemote:
			return repograph.RepoPath{}, ReasonRemoteSource
		case sourceresolve.KindInvalid:
			return repograph.RepoPath{}, ReasonInvalidSource
		default: // KindLocal
			rootDir, ok := resolvePath(unitDir, src.Root)
			if !ok {
				return repograph.RepoPath{}, ReasonSourceOutsideRepo
			}
			modRaw := rootDir
			if src.Subdir != "" {
				modRaw = path.Join(rootDir, src.Subdir)
			}
			if modRaw == ".." || strings.HasPrefix(modRaw, "../") {
				return repograph.RepoPath{}, ReasonSourceOutsideRepo
			}
			mp, pathErr := repograph.NewRepoPath(modRaw)
			if pathErr != nil {
				return repograph.RepoPath{}, ReasonSourceOutsideRepo
			}
			return mp, ""
		}
	}

	mp, pathErr := repograph.NewRepoPath(unitDir)
	if pathErr != nil {
		return repograph.RepoPath{}, ReasonInvalidSource
	}
	return mp, ""
}
