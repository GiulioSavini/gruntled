package main

// REL-03 (17-02 Task 2): the textual module editor used by the reformat
// transforms and the mutation set.
//
// findBlocks locates `variable` and `output` blocks at depth 0 and their
// depth-1 attributes with byte extents, from modelKinds (code, string,
// template, heredoc and comment bytes) plus a ( [ { counter over code bytes.
// It imports no HCL library: check-architecture Step 6 forbids it here, and
// that keeps the editor independent of gruntled's parser.
//
// Every edit goes through applyEdit, which confines writes to the working
// copy (sec #415): the target must be a regular file (or a new file) whose
// path has no symlinked component and whose real path lies under the real
// root. It records the file's sha256 and mode and returns a restore that
// writes the original bytes back and re-checks the sha256.

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// tfAttr is one depth-1 attribute of a block: [Start, End) covers its whole
// lines (End is just past the newline that ends the value), or, inside a
// single-line block, the bytes up to the closing brace.
type tfAttr struct {
	Name       string
	Start, End int
	ValueStart int
}

// tfBlock is one depth-0 variable or output block. Start is the start of
// its header line; Close is the index of its closing brace; End is just
// past the newline after Close (or len(src)).
type tfBlock struct {
	Kind, Name  string
	Start, Open int
	Close, End  int
	Attrs       []tfAttr
	Nested      int  // nested blocks (validation, precondition, ...)
	OneLine     bool // header and closing brace on one line
	Heredoc     bool // a heredoc appears in the body
}

func (b tfBlock) attr(name string) (tfAttr, bool) {
	for _, a := range b.Attrs {
		if a.Name == name {
			return a, true
		}
	}
	return tfAttr{}, false
}

// findBlocks returns the depth-0 variable and output blocks of src in
// source order. ok is false when src does not scan as balanced HCL.
func findBlocks(src string) ([]tfBlock, bool) {
	if !oracleBalanced(src) {
		return nil, false
	}
	mask := oracleMask(src)
	kinds := modelKinds(src)
	var out []tfBlock
	for _, sm := range modelSurfaceRe.FindAllStringSubmatchIndex(src, -1) {
		start := sm[0]
		for src[start] == ' ' || src[start] == '\t' {
			start++
		}
		if mask[start] != 0 {
			continue
		}
		b := tfBlock{Kind: src[sm[2]:sm[3]], Start: strings.LastIndexByte(src[:start], '\n') + 1, Open: sm[1] - 1, Close: -1}
		if sm[4] >= 0 {
			b.Name = src[sm[4]:sm[5]]
		} else {
			b.Name = src[sm[6]:sm[7]]
		}
		for j := b.Open + 1; j < len(src); j++ {
			if src[j] == '}' && mask[j] == 1 {
				b.Close = j
				break
			}
		}
		if b.Close < 0 {
			return nil, false
		}
		b.End = len(src)
		if nl := strings.IndexByte(src[b.Close:], '\n'); nl >= 0 {
			b.End = b.Close + nl + 1
		}
		b.OneLine = !strings.Contains(src[b.Open:b.Close], "\n")
		b.Attrs, b.Nested, b.Heredoc = scanAttrs(src, kinds, b)
		out = append(out, b)
	}
	return out, true
}

// scanAttrs splits the body of b into depth-1 attributes and nested blocks.
func scanAttrs(src string, kinds []byte, b tfBlock) (attrs []tfAttr, nested int, heredoc bool) {
	i := b.Open + 1
	for i < b.Close {
		// Skip blanks, newlines and comments between items.
		if c := src[i]; c == ' ' || c == '\t' || c == '\r' || c == '\n' || kinds[i] == 'm' {
			i++
			continue
		}
		lineStart := strings.LastIndexByte(src[:i], '\n') + 1
		if lineStart <= b.Open {
			lineStart = i // single-line block: the item starts after the brace
		}
		m := modelIdentRe.FindStringIndex(src[i:b.Close])
		if m == nil || m[0] != 0 {
			// Not an identifier: skip to the next line (defensive).
			nl := strings.IndexByte(src[i:b.Close], '\n')
			if nl < 0 {
				break
			}
			i += nl + 1
			continue
		}
		name := src[i : i+m[1]]
		j := i + m[1]
		for j < b.Close && (src[j] == ' ' || src[j] == '\t') {
			j++
		}
		isAttr := j < b.Close && src[j] == '=' && (j+1 >= len(src) || src[j+1] != '=')
		// The item ends at the first newline at bracket depth 0 over code
		// bytes (strings, templates and heredocs are not code), or at the
		// block's closing brace.
		depth := 0
		k := i
		end := b.Close
		for ; k < b.Close; k++ {
			if kinds[k] == 'h' {
				heredoc = true
			}
			if kinds[k] != 'c' {
				continue
			}
			switch src[k] {
			case '{', '[', '(':
				depth++
			case '}', ']', ')':
				depth--
			case '\n':
				if depth == 0 {
					end = k + 1
				}
			}
			if end != b.Close {
				break
			}
		}
		if isAttr {
			vs := j + 1
			for vs < end && (src[vs] == ' ' || src[vs] == '\t') {
				vs++
			}
			attrs = append(attrs, tfAttr{Name: name, Start: lineStart, End: end, ValueStart: vs})
		} else {
			nested++
		}
		i = end
	}
	return attrs, nested, heredoc
}

var modelIdentRe = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_-]*`)

// edit is one file change in the working copy: New replaces the file (or
// creates it with Create); Remove deletes it; Materialise replaces a
// symlink with a regular file holding its target's bytes.
type edit struct {
	File        string // root-relative, slash-separated
	New         []byte
	Create      bool
	Remove      bool
	Materialise bool
}

// confined checks that rel names a path under root with no symlinked
// component except, when allowLink, the last one, and returns its full path.
func confined(root, rel string, allowLink bool) (string, error) {
	if rel == "" || filepath.IsAbs(rel) || strings.HasPrefix(filepath.Clean(filepath.FromSlash(rel)), "..") {
		return "", fmt.Errorf("path %q is not root-relative", rel)
	}
	realRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return "", err
	}
	parts := strings.Split(filepath.ToSlash(filepath.Clean(filepath.FromSlash(rel))), "/")
	cur := root
	for i, p := range parts {
		cur = filepath.Join(cur, p)
		fi, err := os.Lstat(cur)
		if errors.Is(err, fs.ErrNotExist) {
			if i == len(parts)-1 {
				break // a file the edit creates
			}
			return "", fmt.Errorf("%s: parent %s missing", rel, cur)
		}
		if err != nil {
			return "", err
		}
		if fi.Mode()&fs.ModeSymlink != 0 && !(allowLink && i == len(parts)-1) {
			return "", fmt.Errorf("%s: component %s is a symlink", rel, strings.Join(parts[:i+1], "/"))
		}
	}
	full := filepath.Join(root, filepath.FromSlash(rel))
	dir := filepath.Dir(full)
	realDir, err := filepath.EvalSymlinks(dir)
	if err != nil {
		return "", err
	}
	if realDir != realRoot && !strings.HasPrefix(realDir, realRoot+string(filepath.Separator)) {
		return "", fmt.Errorf("%s resolves outside the root", rel)
	}
	if allowLink {
		if real, err := filepath.EvalSymlinks(full); err != nil || (real != realRoot && !strings.HasPrefix(real, realRoot+string(filepath.Separator))) {
			return "", fmt.Errorf("%s: link target outside the root or unresolvable", rel)
		}
	}
	return full, nil
}

// tryApplyEdit applies e under root, or returns why it refuses. The restore
// puts the original bytes, mode or link back and re-checks the sha256.
func tryApplyEdit(root string, e edit) (restore func() error, err error) {
	full, err := confined(root, e.File, e.Materialise)
	if err != nil {
		return nil, err
	}
	fi, statErr := os.Lstat(full)
	switch {
	case e.Create:
		if statErr == nil {
			return nil, fmt.Errorf("%s exists; Create refuses to overwrite", e.File)
		}
		if err := os.WriteFile(full, e.New, 0o644); err != nil {
			return nil, err
		}
		return func() error { return os.Remove(full) }, nil
	case statErr != nil:
		return nil, statErr
	case e.Materialise:
		if fi.Mode()&fs.ModeSymlink == 0 {
			return nil, fmt.Errorf("%s is not a symlink", e.File)
		}
		target, err := os.Readlink(full)
		if err != nil {
			return nil, err
		}
		data, err := os.ReadFile(full)
		if err != nil {
			return nil, err
		}
		if err := os.Remove(full); err != nil {
			return nil, err
		}
		if err := os.WriteFile(full, data, 0o644); err != nil {
			return nil, err
		}
		return func() error {
			if err := os.Remove(full); err != nil {
				return err
			}
			if err := os.Symlink(target, full); err != nil {
				return err
			}
			if got, _ := os.Readlink(full); got != target {
				return fmt.Errorf("%s: link restored to %q, want %q", e.File, got, target)
			}
			return nil
		}, nil
	case !fi.Mode().IsRegular():
		return nil, fmt.Errorf("%s is not a regular file", e.File)
	}
	orig, err := os.ReadFile(full)
	if err != nil {
		return nil, err
	}
	sum := sha256.Sum256(orig)
	mode := fi.Mode().Perm()
	if e.Remove {
		err = os.Remove(full)
	} else {
		err = os.WriteFile(full, e.New, mode)
	}
	if err != nil {
		return nil, err
	}
	return func() error {
		if err := os.WriteFile(full, orig, mode); err != nil {
			return err
		}
		if err := os.Chmod(full, mode); err != nil {
			return err
		}
		back, err := os.ReadFile(full)
		if err != nil {
			return err
		}
		if sha256.Sum256(back) != sum {
			return fmt.Errorf("%s: restored bytes differ", e.File)
		}
		return nil
	}, nil
}

// applyEdit applies e under root and returns its restore; a refusal or an
// I/O error fails the test.
func applyEdit(t testing.TB, root string, e edit) (restore func()) {
	t.Helper()
	r, err := tryApplyEdit(root, e)
	if err != nil {
		t.Fatalf("applyEdit %s: %v", e.File, err)
	}
	return func() {
		t.Helper()
		if err := r(); err != nil {
			t.Fatalf("restore %s: %v", e.File, err)
		}
	}
}

// applyEdits applies es in order and returns one restore that undoes them
// in reverse order.
func applyEdits(t testing.TB, root string, es []edit) (restore func()) {
	t.Helper()
	var rs []func()
	for _, e := range es {
		rs = append(rs, applyEdit(t, root, e))
	}
	return func() {
		for i := len(rs) - 1; i >= 0; i-- {
			rs[i]()
		}
	}
}

// digestGuard checks every n calls (and on Final) that the working copy is
// byte-identical to the pristine tree it was copied from.
type digestGuard struct {
	t        testing.TB
	work     string
	pristine string // corpusDigest of the pristine tree
	n, count int
}

func (g *digestGuard) Tick() {
	g.count++
	if g.count%g.n == 0 {
		g.Final()
	}
}

func (g *digestGuard) Final() {
	g.t.Helper()
	if got := corpusDigest(g.t, g.work); got != g.pristine {
		g.t.Fatalf("working copy drifted after %d edits: digest %s, pristine %s", g.count, got, g.pristine)
	}
}

// ---------------------------------------------------------------------------
// Editor operations. Each returns the edit(s) for one change; none touches a
// string, template, heredoc or comment interior.

// opDeleteBlock removes b's lines.
func opDeleteBlock(file, src string, b tfBlock) edit {
	return edit{File: file, New: []byte(src[:b.Start] + src[b.End:])}
}

// opDeleteAttr removes a's lines from a multi-line block.
func opDeleteAttr(file, src string, a tfAttr) edit {
	return edit{File: file, New: []byte(src[:a.Start] + src[a.End:])}
}

// opSetAttr replaces the value of attribute a with value, or adds
// `name = value` as the block's last attribute when absent.
func opSetAttr(file, src string, b tfBlock, name, value string) edit {
	if a, ok := b.attr(name); ok {
		end := a.End
		if end > 0 && end <= len(src) && src[end-1] == '\n' {
			end--
		}
		return edit{File: file, New: []byte(src[:a.ValueStart] + value + src[end:])}
	}
	return opInsertAttr(file, src, b, name+" = "+value)
}

// opInsertAttr inserts one attribute line before b's closing brace.
func opInsertAttr(file, src string, b tfBlock, line string) edit {
	if b.OneLine {
		body := strings.TrimSpace(src[b.Open+1 : b.Close])
		inner := "  " + line + "\n"
		if body != "" {
			inner = "  " + body + "\n" + inner
		}
		return edit{File: file, New: []byte(src[:b.Open+1] + "\n" + inner + src[b.Close:])}
	}
	closeLine := strings.LastIndexByte(src[:b.Close], '\n') + 1
	return edit{File: file, New: []byte(src[:closeLine] + "  " + line + "\n" + src[closeLine:])}
}

// opNewFile creates a file holding text.
func opNewFile(file, text string) edit {
	return edit{File: file, New: []byte(text), Create: true}
}

// opMoveBlock moves b from file to dest (appended, or created).
func opMoveBlock(root, file, src string, b tfBlock, dest string) []edit {
	text := src[b.Start:b.End]
	if !strings.HasSuffix(text, "\n") {
		text += "\n"
	}
	out := []edit{opDeleteBlock(file, src, b)}
	if old, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(dest))); err == nil {
		out = append(out, edit{File: dest, New: append(bytes.Clone(old), []byte("\n"+text)...)})
	} else {
		out = append(out, opNewFile(dest, text))
	}
	return out
}

// opMaterialise replaces the symlink at file with a regular file.
func opMaterialise(file string) edit {
	return edit{File: file, Materialise: true}
}

// opToJSON converts the blocks of src that are simple (single-line
// attribute values, no heredoc, no %{ directive, no nested block, only
// type/default/description/sensitive/nullable/ephemeral/deprecated/value
// attributes) into Terraform JSON in dest, and removes them from src. type
// becomes a string holding the type expression; default and value become
// "${<expr>}" (default presence is the fact blast reads); description a
// string; sensitive, nullable and ephemeral a JSON bool. It returns the
// edits and the number of blocks converted and skipped.
func opToJSON(root, file, src string, blocks []tfBlock, dest string) (es []edit, converted, skipped int) {
	doc := map[string]map[string]map[string]any{}
	if old, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(dest))); err == nil {
		if json.Unmarshal(old, &doc) != nil {
			return nil, 0, len(blocks)
		}
	}
	keep := src
	for i := len(blocks) - 1; i >= 0; i-- {
		b := blocks[i]
		obj, ok := blockJSON(src, b)
		if !ok {
			skipped++
			continue
		}
		if doc[b.Kind] == nil {
			doc[b.Kind] = map[string]map[string]any{}
		}
		if _, dup := doc[b.Kind][b.Name]; dup {
			skipped++
			continue
		}
		doc[b.Kind][b.Name] = obj
		keep = keep[:b.Start] + keep[b.End:]
		converted++
	}
	if converted == 0 {
		return nil, 0, skipped
	}
	out, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return nil, 0, len(blocks)
	}
	out = append(out, '\n')
	_, statErr := os.Lstat(filepath.Join(root, filepath.FromSlash(dest)))
	return []edit{{File: file, New: []byte(keep)}, {File: dest, New: out, Create: statErr != nil}}, converted, skipped
}

func blockJSON(src string, b tfBlock) (map[string]any, bool) {
	if b.Nested > 0 || b.Heredoc {
		return nil, false
	}
	obj := map[string]any{}
	for _, a := range b.Attrs {
		raw := strings.TrimSpace(src[a.ValueStart:a.End])
		if b.OneLine && a.End == b.Close {
			raw = strings.TrimSpace(src[a.ValueStart:b.Close])
		}
		if raw == "" || strings.Contains(raw, "\n") || strings.Contains(raw, "%{") || strings.Contains(raw, "#") || strings.Contains(raw, "//") || strings.Contains(raw, "/*") {
			return nil, false
		}
		switch a.Name {
		case "type":
			obj["type"] = raw
		case "default", "value":
			obj[a.Name] = "${" + raw + "}"
		case "description":
			s, ok := hclSimpleString(raw)
			if !ok {
				return nil, false
			}
			obj["description"] = s
		case "sensitive", "nullable", "ephemeral":
			switch raw {
			case "true":
				obj[a.Name] = true
			case "false":
				obj[a.Name] = false
			default:
				return nil, false
			}
		case "deprecated":
			s, ok := hclSimpleString(raw)
			if !ok {
				return nil, false
			}
			obj["deprecated"] = s
		default:
			return nil, false
		}
	}
	return obj, true
}

// hclSimpleString returns the content of a quoted literal with no escape,
// template or directive.
func hclSimpleString(raw string) (string, bool) {
	if len(raw) < 2 || raw[0] != '"' || raw[len(raw)-1] != '"' {
		return "", false
	}
	in := raw[1 : len(raw)-1]
	if strings.ContainsAny(in, "\"\\") || strings.Contains(in, "${") || strings.Contains(in, "%{") {
		return "", false
	}
	return in, true
}
