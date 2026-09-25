package synthrepo_test

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/GiulioSavini/gruntled/internal/domain/diagnostic"
	"github.com/GiulioSavini/gruntled/internal/testsupport/synthrepo"
)

func mustGenerate(t *testing.T, spec synthrepo.Spec, dir string) synthrepo.Manifest {
	t.Helper()
	manifest, err := synthrepo.Generate(spec, dir)
	if err != nil {
		t.Fatalf("Generate(%+v, %q): unexpected error: %v", spec, dir, err)
	}
	return manifest
}

// listFiles returns every regular file under root, as slash-separated
// paths relative to root, sorted.
func listFiles(t *testing.T, root string) []string {
	t.Helper()
	var paths []string
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(root, p)
		if err != nil {
			return err
		}
		paths = append(paths, filepath.ToSlash(rel))
		return nil
	})
	if err != nil {
		t.Fatalf("walking %q: %v", root, err)
	}
	sort.Strings(paths)
	return paths
}

// onDiskDigest walks root and computes the same digest formula as
// synthrepo.Tree.Digest: for each regular file, in sorted relative-path
// order, path + "\x00" + content + "\x00".
func onDiskDigest(t *testing.T, root string) string {
	t.Helper()
	type fileEntry struct {
		path    string
		content []byte
	}
	var entries []fileEntry
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(root, p)
		if err != nil {
			return err
		}
		content, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		entries = append(entries, fileEntry{path: filepath.ToSlash(rel), content: content})
		return nil
	})
	if err != nil {
		t.Fatalf("walking %q: %v", root, err)
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].path < entries[j].path })

	h := sha256.New()
	for _, e := range entries {
		h.Write([]byte(e.path))
		h.Write([]byte{0})
		h.Write(e.content)
		h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil))
}

// bytesAt returns the bytes of content starting at the 1-based line and
// 1-based, byte-counted column. It is an independent implementation from
// synthrepo's internal lineCol, deliberately duplicated so this test
// package (which cannot see synthrepo's unexported helpers) proves the
// position semantics from scratch rather than trusting the production
// code that computed them.
func bytesAt(content []byte, line, col int) []byte {
	start := 0
	current := 1
	for current < line {
		idx := bytes.IndexByte(content[start:], '\n')
		if idx == -1 {
			return nil
		}
		start += idx + 1
		current++
	}
	offset := start + col - 1
	if offset < 0 || offset > len(content) {
		return nil
	}
	return content[offset:]
}

func TestGenerate_Deterministic(t *testing.T) {
	spec := synthrepo.Spec{Units: 10, IncludeDepth: 3, DependencyFanout: 2, Seed: 55}

	dir1 := t.TempDir()
	dir2 := t.TempDir()

	manifest1 := mustGenerate(t, spec, dir1)
	manifest2 := mustGenerate(t, spec, dir2)

	if !reflect.DeepEqual(manifest1, manifest2) {
		t.Fatalf("Generate() gave different Manifest across two destinations")
	}

	files1 := listFiles(t, dir1)
	files2 := listFiles(t, dir2)
	if !reflect.DeepEqual(files1, files2) {
		t.Fatalf("Generate() gave different file lists:\n%v\n%v", files1, files2)
	}

	for _, rel := range files1 {
		c1, err := os.ReadFile(filepath.Join(dir1, filepath.FromSlash(rel)))
		if err != nil {
			t.Fatalf("reading %q from dir1: %v", rel, err)
		}
		c2, err := os.ReadFile(filepath.Join(dir2, filepath.FromSlash(rel)))
		if err != nil {
			t.Fatalf("reading %q from dir2: %v", rel, err)
		}
		if !bytes.Equal(c1, c2) {
			t.Fatalf("file %q differs between the two destinations", rel)
		}
	}

	tree, _, err := synthrepo.Render(spec)
	if err != nil {
		t.Fatalf("Render(): unexpected error: %v", err)
	}
	want := tree.Digest()
	if got := onDiskDigest(t, dir1); got != want {
		t.Errorf("on-disk digest for dir1 = %q, want Render Tree.Digest() %q", got, want)
	}
	if got := onDiskDigest(t, dir2); got != want {
		t.Errorf("on-disk digest for dir2 = %q, want Render Tree.Digest() %q", got, want)
	}
}

func TestGenerate_InjectsBadOutputRef(t *testing.T) {
	spec := synthrepo.Spec{Units: 20, IncludeDepth: 3, DependencyFanout: 2, Seed: 7, Errors: []synthrepo.ErrorKind{synthrepo.BadOutputRef}}
	dir := t.TempDir()

	manifest := mustGenerate(t, spec, dir)

	if len(manifest.Expected) != 1 {
		t.Fatalf("len(manifest.Expected) = %d, want 1", len(manifest.Expected))
	}
	exp := manifest.Expected[0]
	if exp.Code != diagnostic.CodeUnknownOutput {
		t.Errorf("Code = %v, want %v", exp.Code, diagnostic.CodeUnknownOutput)
	}

	tgPath := filepath.Join(dir, filepath.FromSlash(exp.Pos.File().String()))
	content, err := os.ReadFile(tgPath)
	if err != nil {
		t.Fatalf("reading %q: %v", tgPath, err)
	}

	got := bytesAt(content, exp.Pos.Line(), exp.Pos.Column())
	want := fmt.Sprintf("dependency.%s.outputs.%s", exp.Dependency, exp.Output)
	if !bytes.HasPrefix(got, []byte(want)) {
		gotPreview := got
		if len(gotPreview) > len(want)+10 {
			gotPreview = gotPreview[:len(want)+10]
		}
		t.Errorf("bytes at (line %d, col %d) start with %q, want prefix %q", exp.Pos.Line(), exp.Pos.Column(), gotPreview, want)
	}

	targetMainPath := filepath.Join(dir, filepath.FromSlash(exp.Target.String()), "main.tf")
	targetMain, err := os.ReadFile(targetMainPath)
	if err != nil {
		t.Fatalf("reading %q: %v", targetMainPath, err)
	}
	badOutputDecl := fmt.Sprintf("output %q {", exp.Output)
	if bytes.Contains(targetMain, []byte(badOutputDecl)) {
		t.Errorf("target main.tf %q declares output %q, expected it to be undeclared", targetMainPath, exp.Output)
	}
}

// oracleRef identifies a bad dependency.X.outputs.Y reference the way
// both the test-local scan and the Manifest express it, so the two can
// be compared for exact equality.
type oracleRef struct {
	unit       string
	dependency string
	output     string
	line       int
	col        int
}

var depBlockRE = regexp.MustCompile(`dependency "([^"]+)" \{
  config_path = "([^"]+)"`)

var depRefRE = regexp.MustCompile(`dependency\.(\w+)\.outputs\.(\w+)`)

// localLineCol is deliberately a from-scratch reimplementation of
// synthrepo's unexported lineCol (this external test package cannot see
// it anyway), so the oracle scan proves the Manifest's positions from
// first principles rather than trusting the code under test.
func localLineCol(content []byte, offset int) (line, col int) {
	line = 1
	lastNewline := -1
	for i := 0; i < offset && i < len(content); i++ {
		if content[i] == '\n' {
			line++
			lastNewline = i
		}
	}
	return line, offset - lastNewline
}

// scanBadRefs independently scans every unit's terragrunt.hcl for
// dependency.X.outputs.Y references whose target module's main.tf does
// not declare the referenced output. It uses only the on-disk tree
// (regexp + string search), never synthrepo's Manifest, so it can serve
// as an oracle proving the Manifest is exact.
func scanBadRefs(t *testing.T, dir string, units []string) []oracleRef {
	t.Helper()

	mainCache := map[string]string{}
	readMain := func(unit string) string {
		if c, ok := mainCache[unit]; ok {
			return c
		}
		content, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(unit), "main.tf"))
		if err != nil {
			t.Fatalf("reading main.tf for unit %q: %v", unit, err)
		}
		s := string(content)
		mainCache[unit] = s
		return s
	}

	var bad []oracleRef
	for _, unit := range units {
		tgPath := filepath.Join(dir, filepath.FromSlash(unit), "terragrunt.hcl")
		raw, err := os.ReadFile(tgPath)
		if err != nil {
			t.Fatalf("reading %q: %v", tgPath, err)
		}
		content := string(raw)

		labelToTarget := map[string]string{}
		for _, m := range depBlockRE.FindAllStringSubmatch(content, -1) {
			label, configPath := m[1], m[2]
			labelToTarget[label] = path.Clean(path.Join(unit, configPath))
		}

		for _, m := range depRefRE.FindAllSubmatchIndex(raw, -1) {
			label := string(raw[m[2]:m[3]])
			output := string(raw[m[4]:m[5]])
			target, ok := labelToTarget[label]
			if !ok {
				t.Fatalf("unit %q references dependency %q with no matching dependency block", unit, label)
			}
			decl := fmt.Sprintf("output %q {", output)
			if strings.Contains(readMain(target), decl) {
				continue
			}
			line, col := localLineCol(raw, m[0])
			bad = append(bad, oracleRef{unit: unit, dependency: label, output: output, line: line, col: col})
		}
	}
	return bad
}

func manifestToOracleRefs(manifest synthrepo.Manifest) []oracleRef {
	refs := make([]oracleRef, 0, len(manifest.Expected))
	for _, e := range manifest.Expected {
		refs = append(refs, oracleRef{
			unit:       e.Unit.String(),
			dependency: e.Dependency,
			output:     e.Output,
			line:       e.Pos.Line(),
			col:        e.Pos.Column(),
		})
	}
	return refs
}

func sortOracleRefs(refs []oracleRef) {
	sort.Slice(refs, func(i, j int) bool {
		a, b := refs[i], refs[j]
		if a.unit != b.unit {
			return a.unit < b.unit
		}
		if a.line != b.line {
			return a.line < b.line
		}
		if a.col != b.col {
			return a.col < b.col
		}
		if a.dependency != b.dependency {
			return a.dependency < b.dependency
		}
		return a.output < b.output
	})
}

func TestGenerate_ManifestIsExactOracle(t *testing.T) {
	spec := synthrepo.Spec{
		Units: 20, IncludeDepth: 3, DependencyFanout: 3, Seed: 11,
		Errors: []synthrepo.ErrorKind{synthrepo.BadOutputRef, synthrepo.BadOutputRef},
	}
	dir := t.TempDir()
	manifest := mustGenerate(t, spec, dir)

	if len(manifest.Expected) != 2 {
		t.Fatalf("len(manifest.Expected) = %d, want 2", len(manifest.Expected))
	}
	if manifest.Expected[0] == manifest.Expected[1] {
		t.Fatalf("the two injected diagnostics are identical, want 2 distinct entries: %+v", manifest.Expected[0])
	}

	units := make([]string, len(manifest.Units))
	for i, u := range manifest.Units {
		units[i] = u.String()
	}

	got := scanBadRefs(t, dir, units)
	want := manifestToOracleRefs(manifest)
	sortOracleRefs(got)
	sortOracleRefs(want)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("independent scan found bad references %+v, Manifest.Expected gives %+v", got, want)
	}

	// A clean spec (no injected errors) must yield zero bad references
	// under the same independent scan.
	cleanSpec := spec
	cleanSpec.Errors = nil
	cleanDir := t.TempDir()
	cleanManifest := mustGenerate(t, cleanSpec, cleanDir)

	cleanUnits := make([]string, len(cleanManifest.Units))
	for i, u := range cleanManifest.Units {
		cleanUnits[i] = u.String()
	}
	cleanBad := scanBadRefs(t, cleanDir, cleanUnits)
	if len(cleanBad) != 0 {
		t.Fatalf("independent scan found %d bad references in a clean tree, want 0: %+v", len(cleanBad), cleanBad)
	}
	if len(cleanManifest.Expected) != 0 {
		t.Fatalf("clean spec Manifest.Expected has %d entries, want 0", len(cleanManifest.Expected))
	}
}

func TestGenerate_DestDirValidation(t *testing.T) {
	spec := synthrepo.Spec{Units: 2, IncludeDepth: 2, DependencyFanout: 1, Seed: 1}

	t.Run("non-empty", func(t *testing.T) {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, "existing.txt"), []byte("x"), 0o644); err != nil {
			t.Fatalf("seeding non-empty dir: %v", err)
		}
		before := listFiles(t, dir)
		if _, err := synthrepo.Generate(spec, dir); err == nil {
			t.Fatalf("Generate() into a non-empty dir: expected error, got nil")
		}
		after := listFiles(t, dir)
		if !reflect.DeepEqual(before, after) {
			t.Fatalf("Generate() into a non-empty dir wrote files: before=%v after=%v", before, after)
		}
	})

	t.Run("missing", func(t *testing.T) {
		dir := filepath.Join(t.TempDir(), "does-not-exist")
		if _, err := synthrepo.Generate(spec, dir); err == nil {
			t.Fatalf("Generate() into a missing dir: expected error, got nil")
		}
		if _, err := os.Stat(dir); !os.IsNotExist(err) {
			t.Fatalf("Generate() into a missing dir: directory was created")
		}
	})

	t.Run("regular-file", func(t *testing.T) {
		dir := t.TempDir()
		file := filepath.Join(dir, "not-a-dir")
		if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
			t.Fatalf("seeding regular file: %v", err)
		}
		if _, err := synthrepo.Generate(spec, file); err == nil {
			t.Fatalf("Generate() into a regular file: expected error, got nil")
		}
		content, err := os.ReadFile(file)
		if err != nil {
			t.Fatalf("reading %q: %v", file, err)
		}
		if string(content) != "x" {
			t.Fatalf("Generate() into a regular file modified its contents: got %q", content)
		}
	})
}
