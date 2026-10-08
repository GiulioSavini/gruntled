package terragrunt

import (
	"bytes"
	"context"
	"path"
	"sort"
	"strings"
	"testing"
	"testing/fstest"

	"pgregory.net/rapid"

	"github.com/GiulioSavini/gruntled/internal/application/checking"
	"github.com/GiulioSavini/gruntled/internal/infrastructure/tfsurface"
	"github.com/GiulioSavini/gruntled/internal/interfaces/presenter"
)

// The incremental model: one fstest.MapFS mutated in place, one Loader
// kept for the whole sequence, and the dirty set the daemon will feed to
// Invalidate. After every reindex the incremental output must be byte for
// byte the output of a brand-new Loader over the same fsys.

// Small colliding alphabet: few dirs and file names, so ops keep hitting
// the same paths (overwrites, dangling references, includes that appear
// and disappear).
var (
	modelUnitDirs    = []string{"a", "b", "c", "a/sub"}
	modelIncludes    = []string{"root.hcl", "a/root.hcl", "common.hcl", "b/common.hcl"}
	modelModules     = []string{"modules/m1/main.tf", "modules/m2/main.tf"}
	modelRenameDirs  = []string{"a", "b", "c", "a/sub", "modules/m1", "d"}
	modelAllFilePath = allModelFiles()
)

func allModelFiles() []string {
	var out []string
	for _, d := range modelUnitDirs {
		out = append(out, d+"/terragrunt.hcl")
	}
	out = append(out, modelIncludes...)
	return append(out, modelModules...)
}

// up returns the "../" prefix leading from dir back to the repo root.
func up(dir string) string {
	return strings.Repeat("../", strings.Count(dir, "/")+1)
}

// unitContent renders a terragrunt.hcl for dir. sibling is the target of
// the dependency variant.
func unitContent(kind int, dir, sibling string) string {
	u := up(dir)
	switch kind {
	case 0:
		return `terraform { source = "` + u + `modules/m1" }`
	case 1:
		return `terraform { source = "` + u + `modules/m2" }
dependency "x" { config_path = "` + u + sibling + `" }
inputs = { v = dependency.x.outputs.o }`
	case 2:
		return `include "root" { path = find_in_parent_folders("root.hcl") }
terraform { source = "` + u + `modules/m1" }`
	case 3:
		return `include "root" { path = "../root.hcl" }`
	case 4:
		return `include "c" { path = find_in_parent_folders("common.hcl") }
terraform { source = "` + u + `modules/m2" }`
	case 5:
		return ``
	default:
		return `terraform {`
	}
}

const unitKinds = 7

var includeContents = []string{
	"locals { x = 1 }\ninputs = { r = 1 }",
	"inputs = { r = 2 }",
	"",
	"locals {",
	`include "n" { path = "common.hcl" }`,
}

var moduleContents = []string{
	"variable \"v\" {}\noutput \"o\" { value = 1 }",
	"output \"p\" { value = 2 }",
	"output \"o\" {",
}

// fatalf is the slice of testing.T / rapid.T the model needs.
type fatalf interface {
	Fatalf(format string, args ...any)
}

type incModel struct {
	fsys  fstest.MapFS
	inc   *Loader
	dirty []string

	loaded bool // inc has completed at least one successful load

	// non-vacuity counters, summed over every reindex
	reindexes        int
	noopReindexes    int
	resolvedIncludes int
	grt100           int
}

func newIncModel() *incModel {
	fsys := fstest.MapFS{}
	return &incModel{fsys: fsys, inc: NewLoader(fsys)}
}

// seed writes a small valid baseline (modules, root.hcl, a unit that
// includes it) so random sequences start from resolvable includes instead
// of having to build them up from nothing.
func (m *incModel) seed() {
	m.write("modules/m1/main.tf", moduleContents[0])
	m.write("modules/m2/main.tf", moduleContents[0])
	m.write("root.hcl", includeContents[0])
	m.write("a/terragrunt.hcl", unitContent(2, "a", "b"))
	m.write("b/terragrunt.hcl", unitContent(1, "b", "a"))
}

func (m *incModel) write(p, content string) {
	m.fsys[p] = &fstest.MapFile{Data: []byte(content)}
	m.dirty = append(m.dirty, p)
}

func (m *incModel) remove(p string) {
	delete(m.fsys, p)
	m.dirty = append(m.dirty, p)
}

// rename moves one file, dirtying both old and new paths.
func (m *incModel) rename(from, to string) {
	f := m.fsys[from]
	delete(m.fsys, from)
	m.fsys[to] = f
	m.dirty = append(m.dirty, from, to)
}

// renameDir moves every file under from/ to to/. dirsOnly dirties just
// the two directory paths (prefix eviction); otherwise every file path.
func (m *incModel) renameDir(from, to string, dirsOnly bool) {
	var moved []string
	for k := range m.fsys {
		if strings.HasPrefix(k, from+"/") {
			moved = append(moved, k)
		}
	}
	sort.Strings(moved)
	for _, k := range moved {
		nk := to + strings.TrimPrefix(k, from)
		m.fsys[nk] = m.fsys[k]
		delete(m.fsys, k)
		if !dirsOnly {
			m.dirty = append(m.dirty, k, nk)
		}
	}
	if dirsOnly {
		m.dirty = append(m.dirty, from, to)
	}
}

func (m *incModel) files() []string {
	out := make([]string, 0, len(m.fsys))
	for k := range m.fsys {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// render runs the full check over l and serialises it exactly as the CLI
// would: JSON report, separator, graph. A Go-level error is rendered too,
// so incremental and full must agree on failures as well.
func render(l *Loader, fsys fstest.MapFS) (string, checking.Report, bool) {
	rep, err := checking.Check(context.Background(), l, tfsurface.NewReader(fsys))
	if err != nil {
		return "error: " + err.Error(), rep, false
	}
	var b bytes.Buffer
	if err := presenter.JSON(&b, rep.Graph, rep.Diagnostics); err != nil {
		return "json error: " + err.Error(), rep, false
	}
	b.WriteString("\n---\n")
	if err := presenter.Graph(&b, rep.Graph); err != nil {
		return "graph error: " + err.Error(), rep, false
	}
	return b.String(), rep, true
}

// reindex is the property: Invalidate the dirty set, render incrementally,
// render a fresh Loader, compare bytes.
func (m *incModel) reindex(t fatalf) {
	noop := len(m.dirty) == 0 && m.loaded
	m.inc.Invalidate(m.dirty...)
	m.dirty = m.dirty[:0]

	got, _, ok := render(m.inc, m.fsys)
	want, rep, _ := render(NewLoader(m.fsys), m.fsys)
	if got != want {
		t.Fatalf("incremental != full rescan\nfiles: %v\n--- incremental ---\n%s\n--- full ---\n%s", m.files(), got, want)
	}
	m.reindexes++
	if noop && ok {
		m.noopReindexes++
		if _, misses := m.inc.CacheStats(); misses != 0 {
			t.Fatalf("reindex with nothing dirty: misses = %d, want 0", misses)
		}
	}
	m.loaded = ok

	if rep.Graph == nil {
		return
	}
	for _, d := range rep.Diagnostics.All() {
		if string(d.Key().Code) == "GRT100" {
			m.grt100++
		}
	}
	for _, u := range rep.Graph.Units() {
		f, exists := m.fsys[path.Join(u.Path().String(), "terragrunt.hcl")]
		if exists && u.UnknownReason() == "" && bytes.Contains(f.Data, []byte("include")) {
			m.resolvedIncludes++
		}
	}
}

func TestIncrementalEqualsFull(t *testing.T) {
	var total incModel
	rapid.Check(t, func(rt *rapid.T) {
		m := newIncModel()
		m.seed()
		rt.Repeat(map[string]func(*rapid.T){
			"create": func(rt *rapid.T) {
				p := rapid.SampledFrom(modelAllFilePath).Draw(rt, "path")
				m.write(p, drawContent(rt, p))
			},
			"edit": func(rt *rapid.T) {
				fs := m.files()
				if len(fs) == 0 {
					return
				}
				p := rapid.SampledFrom(fs).Draw(rt, "path")
				m.write(p, drawContent(rt, p))
			},
			"delete": func(rt *rapid.T) {
				fs := m.files()
				if len(fs) == 0 {
					return
				}
				m.remove(rapid.SampledFrom(fs).Draw(rt, "path"))
			},
			"rename": func(rt *rapid.T) {
				fs := m.files()
				if len(fs) == 0 {
					return
				}
				from := rapid.SampledFrom(fs).Draw(rt, "from")
				to := rapid.SampledFrom(modelAllFilePath).Draw(rt, "to")
				if from == to {
					return
				}
				m.rename(from, to)
			},
			"rename-dir": func(rt *rapid.T) {
				from := rapid.SampledFrom(modelRenameDirs).Draw(rt, "from")
				to := rapid.SampledFrom(modelRenameDirs).Draw(rt, "to")
				if from == to || strings.HasPrefix(to, from+"/") || strings.HasPrefix(from, to+"/") {
					return
				}
				m.renameDir(from, to, rapid.Bool().Draw(rt, "dirsOnly"))
			},
			"reindex": func(rt *rapid.T) {
				m.reindex(rt)
			},
		})
		// Final reindex so trailing mutations are always checked.
		m.reindex(rt)
		total.reindexes += m.reindexes
		total.noopReindexes += m.noopReindexes
		total.resolvedIncludes += m.resolvedIncludes
		total.grt100 += m.grt100
	})
	t.Logf("reindexes=%d noop=%d resolvedIncludes=%d grt100=%d",
		total.reindexes, total.noopReindexes, total.resolvedIncludes, total.grt100)
	if !t.Failed() && (total.resolvedIncludes == 0 || total.grt100 == 0 || total.noopReindexes == 0) {
		t.Fatalf("vacuous run: resolvedIncludes=%d grt100=%d noop=%d, all must be > 0",
			total.resolvedIncludes, total.grt100, total.noopReindexes)
	}
}

// drawContent draws a template matching p's kind.
func drawContent(rt *rapid.T, p string) string {
	switch {
	case strings.HasSuffix(p, "/terragrunt.hcl"):
		dir := strings.TrimSuffix(p, "/terragrunt.hcl")
		kind := rapid.IntRange(0, unitKinds-1).Draw(rt, "unitKind")
		sib := rapid.SampledFrom(modelUnitDirs).Draw(rt, "sibling")
		return unitContent(kind, dir, sib)
	case strings.HasSuffix(p, ".tf"):
		return rapid.SampledFrom(moduleContents).Draw(rt, "module")
	default:
		return rapid.SampledFrom(includeContents).Draw(rt, "include")
	}
}

// TestIncrementalModelNonVacuous drives the same model helpers through a
// fixed sequence and proves the fixture reaches what the property is
// about: an include resolved through MapFS (fs.ReadLinkFS), a GRT100, a
// no-op reindex with zero misses, and directory-prefix eviction.
func TestIncrementalModelNonVacuous(t *testing.T) {
	m := newIncModel()
	m.seed()
	m.reindex(t)
	if m.resolvedIncludes == 0 {
		t.Fatalf("include in a/terragrunt.hcl did not resolve over MapFS")
	}

	m.reindex(t)
	if m.noopReindexes != 1 {
		t.Fatalf("noopReindexes = %d, want 1", m.noopReindexes)
	}

	m.write("c/terragrunt.hcl", unitContent(unitKinds-1, "c", "a"))
	m.reindex(t)
	if m.grt100 == 0 {
		t.Fatalf("broken c/terragrunt.hcl produced no GRT100")
	}

	m.write("a/sub/terragrunt.hcl", unitContent(3, "a/sub", "a"))
	m.write("a/root.hcl", includeContents[1])
	m.reindex(t)
	m.renameDir("a", "d", true)
	m.reindex(t)
	m.rename("root.hcl", "common.hcl")
	m.reindex(t)
	m.remove("c/terragrunt.hcl")
	m.reindex(t)
}
