package terragrunt

// Config-unknown reasons (ports.UnitConfig.ConfigUnknownReason). A
// config-unknown unit carries no dependencies and no references: its own
// terragrunt.hcl, or a merged include, is broken or uses a construct this
// domain does not model. Checked in loader.go's resolveUnit, in the fixed
// order documented there; the first one that applies wins.
const (
	// ReasonSyntaxError means the unit's own terragrunt.hcl, or a merged
	// include file, has an HCL syntax error. GRT100 is emitted once for
	// that file (see fileCache.syntaxDiagnostics).
	ReasonSyntaxError = "syntax-error"
	// ReasonUnreadableConfig means the unit's own terragrunt.hcl, or an
	// include file it resolved to, could not be read (fs.ReadFile, or the
	// fs.Stat that hclconv.ReadFileLimited does first, failed), even though
	// it was found by discovery or by the include's own fs.Stat, or an
	// include path whose symlinks could not be resolved (a link loop, a
	// backslash link target, an Lstat/ReadLink error, or an fs.FS that
	// cannot report links), so which file it names is unknown.
	ReasonUnreadableConfig = "unreadable-config"
	// ReasonJSONConfigUnsupported means the unit directory holds
	// terragrunt.hcl.json. Terragrunt itself prefers the JSON variant over
	// terragrunt.hcl when both exist (DefaultTerragruntConfigPaths lists
	// JSON first); this domain does not parse JSON Terragrunt configs.
	ReasonJSONConfigUnsupported = "json-config-unsupported"
	// ReasonAutoincludeUnsupported means the unit directory holds
	// terragrunt.autoinclude.hcl, which Terragrunt auto-merges as a Stacks
	// feature this domain does not model.
	ReasonAutoincludeUnsupported = "autoinclude-unsupported"
	// ReasonInvalidInclude means an include block (or the unit's set of
	// include blocks) is structurally invalid: more than one label, a
	// duplicate label, more than one bare include, a missing path
	// attribute, a merge_strategy that is not a literal in {"", "shallow",
	// "deep", "no_merge"}, the same file included twice, or a unit
	// including its own terragrunt.hcl.
	ReasonInvalidInclude = "invalid-include"
	// ReasonIncludeDynamicPath means an include's path attribute failed
	// closed evaluation (a variable, or a function outside the six PARSE-02
	// path functions, such as get_env or local.x).
	ReasonIncludeDynamicPath = "include-dynamic-path"
	// ReasonIncludeOutsideRepo means an include's evaluated path escapes
	// the repository (a real absolute path, or a relative path resolving
	// above the repo root), or reaches its target through a symlink that
	// escapes the repository.
	ReasonIncludeOutsideRepo = "include-outside-repo"
	// ReasonIncludeNotFound means an include's resolved path is not an
	// existing regular file in the repository.
	ReasonIncludeNotFound = "include-not-found"
	// ReasonNestedInclude means an included file itself contains an
	// include block, which Terragrunt itself rejects
	// (TooManyLevelsOfInheritanceError): only a single level of include is
	// supported.
	ReasonNestedInclude = "nested-include"
	// ReasonInvalidTerraformBlock means one effective file (the unit's own
	// body, or a merged include) has more than one top-level terraform
	// block.
	ReasonInvalidTerraformBlock = "invalid-terraform-block"
	// ReasonInvalidDependency means a dependency block is structurally
	// invalid (label count other than one, a duplicate label within a
	// single file, an expansion block), or a dependency's config_path is
	// missing from every effective file that declares that label, or a
	// domain constructor rejected an otherwise-validated dependency (an
	// internal invariant violation this loader never lets escalate to a
	// crash or a returned error).
	ReasonInvalidDependency = "invalid-dependency"
	// ReasonIncludeTarget means this unit's own terragrunt.hcl is a file
	// some OTHER unit resolved as an include (a parent config, per research
	// 03-RESEARCH.md Pattern 4). A parent config's path-bearing attributes
	// and references resolve differently per including unit; analysing it
	// standalone checks it against the wrong directory (the exact false
	// GRT001 reproduced at live/terragrunt.hcl:4:21 in the corpus). Its
	// references are still checked, correctly, once per including unit:
	// this only removes the parent's own, doubly-wrong self-interpretation.
	// Targets are matched by canonical in-repo path (02-REVIEW G15): an
	// include that reaches the parent through a symlinked directory, a
	// symlinked file or a chain of links still marks the real parent.
	// A unit is a target under any of three rules (02-REVIEW G15/G16):
	// an exact canonical match of some include's file; a strict ancestor
	// of a unit whose includes are unknowable or failed; or, once some
	// include's file name is dynamic or terragrunt.hcl, every include-free
	// unit. Each rule only fails toward unknown; the catalogue
	// (02-TERRAGRUNT-EDGECASES.md) lists the residuals.
	// Catalogue STACK-09 ("included and independently runnable") becomes a
	// documented false negative. An earlier config-unknown reason on the
	// same unit is kept (the first check that applies always wins).
	ReasonIncludeTarget = "include-target"
	// ReasonIncludeJSONUnsupported means an include's path resolves to a
	// file ending in ".json" (an explicit "root.hcl.json", or
	// find_in_parent_folders() probing terragrunt.hcl.json ahead of
	// terragrunt.hcl, Terragrunt's own DefaultTerragruntConfigPaths order).
	// This domain does not parse JSON Terragrunt configs.
	ReasonIncludeJSONUnsupported = "include-json-unsupported"
	// ReasonInvalidGenerate means a generate block is structurally invalid:
	// zero or two-or-more labels, or a label duplicated within one
	// effective file. Matches ReasonInvalidDependency's shape (Terragrunt
	// itself rejects an unlabeled or multi-labeled generate block, and a
	// duplicate label within one file). Merging distinct files that both
	// declare the SAME label is valid (Terragrunt merges by label, highest
	// precedence wins); only a duplicate WITHIN one file is invalid.
	ReasonInvalidGenerate = "invalid-generate"
	// ReasonConfigTooLarge means the unit's own terragrunt.hcl, or an
	// include file it resolved to, exceeds hclconv.MaxFileBytes. The file
	// is never parsed: hclsyntax recurses over its input, and the fatal
	// stack overflow a hostile file can cause cannot be recovered from
	// (02-REVIEW G7). No GRT100 is emitted, because the file is not known
	// to be invalid.
	ReasonConfigTooLarge = "config-too-large"
	// ReasonConfigTooDeep means the unit's own terragrunt.hcl, or an
	// include file it resolved to, nests deeper than
	// hclconv.MaxNestingDepth. The file is never parsed, for the same
	// reason as ReasonConfigTooLarge, and no GRT100 is emitted.
	ReasonConfigTooDeep = "config-too-deep"
)

// Module-unknown reasons (ports.UnitConfig.ModuleUnknownReason). A
// module-unknown unit's own configuration is known and its dependencies and
// references are kept: GRT001 (Phase 3) checks the TARGET unit's module, not
// the referencing unit's, so a unit whose OWN module is unknown must not
// poison references made INTO it by other units.
const (
	// ReasonSourceDynamicPath means the effective terraform.source
	// attribute failed closed evaluation.
	ReasonSourceDynamicPath = "source-dynamic-path"
	// ReasonSourceOutsideRepo means the source classified as local, but its
	// root (or the final module path once joined with a "//" subdir)
	// resolves outside the repository -- including a real absolute path
	// such as "/opt/shared-modules/vpc" (SRC-04).
	ReasonSourceOutsideRepo = "source-outside-repo"
	// ReasonRemoteSource means the source classified as remote (GRAPH-03):
	// a forced getter, a URL scheme, or a host shorthand. gruntled never
	// downloads a remote source.
	ReasonRemoteSource = "remote-source"
	// ReasonInvalidSource means the source is empty, malformed, or a
	// construct this domain does not model (a query string on a local
	// path, a "file://" URL, a backslash).
	ReasonInvalidSource = "invalid-source"
	// ReasonGenerateMayDeclareOutputs means an effective generate block's
	// contents are not provably free of an `output` declaration: contents
	// missing, not a plain string template, or a literal template whose
	// text contains "output" or a \u escape anywhere (02-REVIEW G19; the
	// check over-approximates on purpose and only ever costs coverage).
	ReasonGenerateMayDeclareOutputs = "generate-may-declare-outputs"
	// ReasonUnitDirOverlaysModule means the unit's source points somewhere
	// other than its own directory, and the unit directory itself holds a
	// .tf/.tf.json/.tofu/.tofu.json file: Terragrunt copies the unit
	// directory's files over the module's working copy, so the effective
	// module surface is not the module directory's surface alone.
	ReasonUnitDirOverlaysModule = "unit-dir-overlays-module"
	// ReasonModuleFileUnreadable means the unit directory could not be
	// listed (fs.ReadDir failed) while checking whether it overlays the
	// module's own files: whether it does is unknown, so the module itself
	// must be treated as unknown rather than silently assumed not to
	// overlay. The string intentionally matches tfsurface's own
	// module-file-unreadable reason (02-REVIEW G6): both packages face the
	// identical "can't list this directory" fact about a module.
	ReasonModuleFileUnreadable = "module-file-unreadable"
)

// Unresolved-dependency reasons (repograph.NewUnresolvedDependency's reason
// argument). The dependency itself is kept on the unit, never dropped: a
// dropped dependency would turn a dependency.X.outputs.Y reference into a
// reference to an undeclared dependency, a false-positive risk.
const (
	// ReasonConfigPathDynamic means the dependency's config_path attribute
	// failed closed evaluation.
	ReasonConfigPathDynamic = "config-path-dynamic"
	// ReasonConfigPathOutsideRepo means the dependency's evaluated
	// config_path escapes the repository.
	ReasonConfigPathOutsideRepo = "config-path-outside-repo"
	// ReasonConfigPathStack means the dependency's resolved target directory
	// holds terragrunt.stack.hcl (or config_path names that file directly).
	// Terragrunt's getTerragruntOutput calls tryGetStackOutput first
	// (research 03-RESEARCH.md Pattern 3): when a stack file is present, the
	// dependency's outputs come from the stack's nested unit outputs, not
	// from the unit module this domain resolves, so the target is never a
	// module. This wins even when the directory also holds a terragrunt.hcl.
	ReasonConfigPathStack = "config-path-stack"
	// ReasonConfigPathNondefaultFile means config_path names an existing
	// regular file other than terragrunt.hcl (or terragrunt.stack.hcl,
	// which gets ReasonConfigPathStack instead). Terragrunt reads THAT file
	// as the target unit's config, which may set a different source than
	// the directory's own terragrunt.hcl, so mapping it to the directory
	// would be a guess.
	ReasonConfigPathNondefaultFile = "config-path-nondefault-file"
	// ReasonConfigPathInvalid means the dependency's resolved target is not
	// a valid RepoPath (for example a backslash survives resolvePath as
	// part of a path segment). Only this one dependency becomes unresolved;
	// its sibling dependencies and the unit itself stay resolved (G8).
	ReasonConfigPathInvalid = "config-path-invalid"
	// ReasonConfigPathEmpty means a dependency block's config_path
	// evaluated to the empty string. It is kept unresolved (never a
	// self-edge) and stays silent: Terragrunt v1.1.6 reports "config_path
	// could not be resolved", not a cycle. A `dependencies { paths }`
	// element "" is different: it resolves to the unit itself (a
	// self-edge, GRT003), matching Terragrunt's "cycle detected".
	ReasonConfigPathEmpty = "config-path-empty"
	// ReasonDependenciesPathsDynamic means a `dependencies` block's paths
	// attribute is not a literal list (local.x, concat(...), a for
	// expression). The whole block is one unresolved entry at the paths
	// value: no edges, no GRT002, but not absent either.
	ReasonDependenciesPathsDynamic = "dependencies-paths-dynamic"
)
