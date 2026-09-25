// Package sourceresolve classifies a Terragrunt terraform.source string as
// local, remote or invalid, with zero filesystem and network access. It
// mirrors Terragrunt's own detector chain (internal/getter/detect.go,
// internal/tf/source.go), whose last detector treats any unmatched string
// as a local path (research Pitfall 8): a bare "units/chicken" is local in
// Terragrunt, unlike plain Terraform.
//
// Misclassification in either direction fails safe: a wrongly-local remote
// string is caught by the module surface reader's existence check
// (module-dir-not-found), and there is no way to wrongly classify a real
// local path as remote through this chain. Zero I/O keeps this package pure
// and independent of the repository being analyzed.
package sourceresolve

import (
	"regexp"
	"strings"
)

// Kind is the outcome of classifying a source string.
type Kind int

const (
	// KindInvalid means the source is empty, malformed, or a construct
	// gruntled does not model (a real absolute path, or a query string on a
	// local path). Root and Subdir are unset.
	KindInvalid Kind = iota
	// KindLocal means the source is a local module reference. Root and
	// Subdir are set.
	KindLocal
	// KindRemote means the source is classified as a remote module
	// reference (forced getter, URL scheme, or a known host shorthand).
	// Root and Subdir are unset: gruntled never downloads remote sources.
	KindRemote
)

// Source is the outcome of Classify.
type Source struct {
	Kind Kind
	// Root is the source string up to (not including) the first "//", set
	// only for KindLocal.
	Root string
	// Subdir is the source string after the first "//", set only for
	// KindLocal. Empty when the source has no "//".
	Subdir string
}

var (
	forcedGetterRE = regexp.MustCompile(`^[A-Za-z0-9]+::`)
	urlSchemeRE    = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9+.\-]*://`)
	scpLikeRE      = regexp.MustCompile(`^[^/@:]+@[^/:]+:`)
)

// Classify classifies raw per the order documented in research Pattern 9:
//  1. trim -> empty = Invalid
//  2. contains a backslash = Invalid
//  3. forced getter prefix ("git::", "s3::", ...) = Remote
//  4. "file://" prefix = Invalid (a real absolute path)
//  5. a URL scheme ("git://", "https://", ...) = Remote
//  6. a host shorthand (github.com/, gitlab.com/, bitbucket.org/, an
//     SCP-like prefix, or containing amazonaws.com/ or googleapis.com/) =
//     Remote
//  7. contains "?" = Invalid (a query string on a local path)
//  8. otherwise Local, split at the first "//" into Root and Subdir; an
//     empty Root is Invalid.
func Classify(raw string) Source {
	s := strings.TrimSpace(raw)
	if s == "" {
		return Source{Kind: KindInvalid}
	}
	if strings.Contains(s, `\`) {
		return Source{Kind: KindInvalid}
	}
	if forcedGetterRE.MatchString(s) {
		return Source{Kind: KindRemote}
	}
	if strings.HasPrefix(s, "file://") {
		return Source{Kind: KindInvalid}
	}
	if urlSchemeRE.MatchString(s) {
		return Source{Kind: KindRemote}
	}
	if isHostShorthand(s) {
		return Source{Kind: KindRemote}
	}
	if strings.Contains(s, "?") {
		return Source{Kind: KindInvalid}
	}
	root, subdir, _ := strings.Cut(s, "//")
	if root == "" {
		return Source{Kind: KindInvalid}
	}
	return Source{Kind: KindLocal, Root: root, Subdir: subdir}
}

func isHostShorthand(s string) bool {
	switch {
	case strings.HasPrefix(s, "github.com/"),
		strings.HasPrefix(s, "gitlab.com/"),
		strings.HasPrefix(s, "bitbucket.org/"),
		strings.Contains(s, "amazonaws.com/"),
		strings.Contains(s, "googleapis.com/"):
		return true
	}
	return scpLikeRE.MatchString(s)
}
