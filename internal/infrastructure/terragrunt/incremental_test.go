package terragrunt

import (
	"bytes"
	"context"
	"fmt"
	"io/fs"
	"maps"
	"path"
	"sort"
	"strings"
	"testing"
	"testing/fstest"

	"pgregory.net/rapid"

	"github.com/GiulioSavini/gruntled/internal/application/checking"
	"github.com/GiulioSavini/gruntled/internal/infrastructure/tfsurface"
	"github.com/GiulioSavini/gruntled/internal/infrastructure/watch"
	"github.com/GiulioSavini/gruntled/internal/interfaces/presenter"
)

// The incremental model: one fstest.MapFS mutated in place, one Loader
// kept for the whole sequence, and the dirty set the daemon will feed to
// Invalidate. After every reindex the incremental output must be byte for
// byte the output of a brand-new Loader over the same fsys.

// Small colliding alphabet: few dirs and file names, so ops keep hitting
// the same paths (overwrites, dangling references, includes that appear
// and disappear). "2024" and "x.tmp" are directory names that match
// editor FILE patterns (vim probe, *.tmp): the watcher must still deliver
// paths under them (12-01, v0.3 audit BLOCKER).
var (
	modelUnitDirs    = []string{"a", "b", "c", "a/sub", "2024", "x.tmp"}
	modelIncludes    = []string{"root.hcl", "a/root.hcl", "common.hcl", "b/common.hcl"}
	modelModules     = []string{"modules/m1/main.tf", "modules/m2/main.tf"}
	modelRenameDirs  = []string{"a", "b", "c", "a/sub", "modules/m1", "d", "2024", "x.tmp"}
	modelPatternDirs = map[string]bool{"2024": true, "x.tmp": true}
	modelAllFilePath = allModelFiles()
)

// ign is the watcher's ignore predicate as the model applies it to the
// dirty set. It is a variable only so TestIncrementalModelDetectsTypeBlindIgnore
// (and the mutation runs recorded in 12-05-SUMMARY) can swap in the
// pre-12-01 type-blind rule; production behaviour is watch.IgnoredEntry.
var ign = watch.IgnoredEntry

// delivered reports whether a watcher would hand the dirty entry (p, typ)
// to the daemon. It models both filters the adapters apply: the per-entry
// check in pending.add, and the subtree skip of poll's walk / native's
// addTree, which return SkipDir for an ignored directory and so hide
// every path below it.
func delivered(p string, typ fs.FileMode) bool {
	if ign(p, typ) {
		return false
	}
	for a := path.Dir(p); a != "."; a = path.Dir(a) {
		if ign(a, fs.ModeDir) {
			return false
		}
	}
	return true
}

// dirtyEntry is one path the watcher reports, with the entry type it
// carries (0 for a regular file or a gone entry).
type dirtyEntry struct {
	p   string
	typ fs.FileMode
}

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
	dirty []dirtyEntry

	loaded bool // inc has completed at least one successful load

	// non-vacuity counters, summed over every reindex
	reindexes         int
	noopReindexes     int
	resolvedIncludes  int
	grt100            int
	patternDirRenames int // dirsOnly renames from or to 2024 / x.tmp
	linkedIncludes    int // reindexes where a resolved unit's include is a symlink
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
	m.write("b/common.hcl", includeContents[1]) // a ready link target
	m.write("a/terragrunt.hcl", unitContent(2, "a", "b"))
	m.write("b/terragrunt.hcl", unitContent(1, "b", "a"))
}

func (m *incModel) touch(p string, typ fs.FileMode) {
	m.dirty = append(m.dirty, dirtyEntry{p, typ})
}

func (m *incModel) write(p, content string) {
	m.fsys[p] = &fstest.MapFile{Data: []byte(content)}
	m.touch(p, 0)
}

func (m *incModel) remove(p string) {
	delete(m.fsys, p)
	m.touch(p, 0)
}

// rename moves one file, dirtying both old and new paths.
func (m *incModel) rename(from, to string) {
	f := m.fsys[from]
	delete(m.fsys, from)
	m.fsys[to] = f
	m.touch(from, 0)
	m.touch(to, 0)
}

// link creates (or replaces) p as a symlink to target, stored relative to
// p's directory as a real link would be. Only p is dirtied: later edits of
// target dirty only target, which is all a watcher reports.
func (m *incModel) link(p, target string) {
	rel := relTo(path.Dir(p), target)
	m.fsys[p] = &fstest.MapFile{Mode: fs.ModeSymlink, Data: []byte(rel)}
	m.touch(p, fs.ModeSymlink)
}

// guarded applies op and undoes it when it leaves a symlink cycle behind:
// fstest.MapFS follows links without a loop limit and overflows the stack
// on a cycle, where a real filesystem would return ELOOP. Links are only
// ever leaf files in this model, so a cycle is a chain of link entries.
func (m *incModel) guarded(op func()) {
	snap := maps.Clone(m.fsys)
	n, pdr := len(m.dirty), m.patternDirRenames
	op()
	if !m.linkCycle() {
		return
	}
	clear(m.fsys) // the Loaders hold this map: restore it in place
	maps.Copy(m.fsys, snap)
	m.dirty = m.dirty[:n]
	m.patternDirRenames = pdr
}

func (m *incModel) linkCycle() bool {
	for k, f := range m.fsys {
		if f.Mode&fs.ModeSymlink == 0 {
			continue
		}
		seen := map[string]bool{k: true}
		for cur := k; ; {
			next := path.Join(path.Dir(cur), string(m.fsys[cur].Data))
			g, ok := m.fsys[next]
			if !ok || g.Mode&fs.ModeSymlink == 0 {
				break
			}
			if seen[next] {
				return true
			}
			seen[next] = true
			cur = next
		}
	}
	return false
}

// relTo returns the slash path leading from dir to target (both
// repo-relative and clean).
func relTo(dir, target string) string {
	if dir == "." {
		return target
	}
	d := strings.Split(dir, "/")
	t := strings.Split(target, "/")
	i := 0
	for i < len(d) && i < len(t)-1 && d[i] == t[i] {
		i++
	}
	return strings.Repeat("../", len(d)-i) + strings.Join(t[i:], "/")
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
			m.touch(k, 0)
			m.touch(nk, 0)
		}
	}
	if dirsOnly {
		m.touch(from, fs.ModeDir)
		m.touch(to, fs.ModeDir)
		if modelPatternDirs[from] || modelPatternDirs[to] {
			m.patternDirRenames++
		}
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

// reindex is the property: Invalidate the dirty paths a watcher would
// deliver, render incrementally, render a fresh Loader, compare bytes. It
// returns the fresh (full rescan) rendering.
func (m *incModel) reindex(t fatalf) string {
	noop := len(m.dirty) == 0 && m.loaded
	var batch []string
	for _, d := range m.dirty {
		if delivered(d.p, d.typ) {
			batch = append(batch, d.p)
		}
	}
	m.inc.Invalidate(batch...)
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
		return want
	}
	for _, d := range rep.Diagnostics.All() {
		if string(d.Key().Code) == "GRT100" {
			m.grt100++
		}
	}
	linked := false
	for _, u := range rep.Graph.Units() {
		dir := u.Path().String()
		f, exists := m.fsys[path.Join(dir, "terragrunt.hcl")]
		if exists && u.UnknownReason() == "" && bytes.Contains(f.Data, []byte("include")) {
			m.resolvedIncludes++
			if inc, ok := m.directInclude(dir, string(f.Data)); ok && m.fsys[inc].Mode&fs.ModeSymlink != 0 {
				linked = true
			}
		}
	}
	if linked {
		m.linkedIncludes++
	}
	return want
}

// directInclude returns the file a unit's own include block resolves to,
// for the include forms unitContent generates: find_in_parent_folders
// probes each ancestor from the parent of dir (following links, like
// fs.Stat in pathfuncs.go), a literal path is joined to dir.
func (m *incModel) directInclude(dir, src string) (string, bool) {
	for _, name := range []string{"root.hcl", "common.hcl"} {
		if !strings.Contains(src, `find_in_parent_folders("`+name+`")`) {
			continue
		}
		for d := dir; d != "."; {
			d = path.Dir(d)
			c := path.Join(d, name)
			if _, err := fs.Stat(m.fsys, c); err == nil {
				return c, true
			}
		}
		return "", false
	}
	if strings.Contains(src, `path = "../root.hcl"`) {
		return path.Join(dir, "../root.hcl"), true
	}
	return "", false
}

func TestIncrementalEqualsFull(t *testing.T) {
	var total incModel
	rapid.Check(t, func(rt *rapid.T) {
		m := newIncModel()
		m.seed()
		rt.Repeat(map[string]func(*rapid.T){
			"create": func(rt *rapid.T) {
				m.guarded(func() {
					p := rapid.SampledFrom(modelAllFilePath).Draw(rt, "path")
					m.write(p, drawContent(rt, p))
				})
			},
			"edit": func(rt *rapid.T) {
				m.guarded(func() {
					fs := m.files()
					if len(fs) == 0 {
						return
					}
					p := rapid.SampledFrom(fs).Draw(rt, "path")
					m.write(p, drawContent(rt, p))
				})
			},
			"delete": func(rt *rapid.T) {
				m.guarded(func() {
					fs := m.files()
					if len(fs) == 0 {
						return
					}
					m.remove(rapid.SampledFrom(fs).Draw(rt, "path"))
				})
			},
			"rename": func(rt *rapid.T) {
				m.guarded(func() {
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
				})
			},
			"rename-dir": func(rt *rapid.T) {
				m.guarded(func() {
					from := rapid.SampledFrom(modelRenameDirs).Draw(rt, "from")
					to := rapid.SampledFrom(modelRenameDirs).Draw(rt, "to")
					if from == to || strings.HasPrefix(to, from+"/") || strings.HasPrefix(from, to+"/") {
						return
					}
					m.renameDir(from, to, rapid.Bool().Draw(rt, "dirsOnly"))
				})
			},
			"link": func(rt *rapid.T) {
				m.guarded(func() {
					// Prefer an existing regular include as the target, so
					// links mostly resolve and units include through them.
					var targets []string
					for _, inc := range modelIncludes {
						if f, ok := m.fsys[inc]; ok && f.Mode&fs.ModeSymlink == 0 {
							targets = append(targets, inc)
						}
					}
					if len(targets) == 0 || rapid.IntRange(0, 3).Draw(rt, "dangling") == 0 {
						targets = modelIncludes
					}
					p := rapid.SampledFrom(modelIncludes).Draw(rt, "link")
					target := rapid.SampledFrom(targets).Draw(rt, "target")
					if p == target {
						return
					}
					m.link(p, target)
				})
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
		total.patternDirRenames += m.patternDirRenames
		total.linkedIncludes += m.linkedIncludes
	})
	t.Logf("reindexes=%d noop=%d resolvedIncludes=%d grt100=%d patternDirRenames=%d linkedIncludes=%d",
		total.reindexes, total.noopReindexes, total.resolvedIncludes, total.grt100,
		total.patternDirRenames, total.linkedIncludes)
	if !t.Failed() && (total.resolvedIncludes == 0 || total.grt100 == 0 || total.noopReindexes == 0 ||
		total.patternDirRenames == 0 || total.linkedIncludes == 0) {
		t.Fatalf("vacuous run: resolvedIncludes=%d grt100=%d noop=%d patternDirRenames=%d linkedIncludes=%d, all must be > 0",
			total.resolvedIncludes, total.grt100, total.noopReindexes, total.patternDirRenames, total.linkedIncludes)
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

	for _, dir := range []string{"2024", "x.tmp"} {
		first, second := patternDirTail(t, m, dir)
		if first == second {
			t.Fatalf("%s: editing %s/terragrunt.hcl left the output unchanged; the tail proves nothing", dir, dir)
		}
	}
	m.renameDir("2024", "e", true)
	m.reindex(t)
	if m.patternDirRenames == 0 {
		t.Fatalf("patternDirRenames = 0 after renaming 2024")
	}

	first, second := symlinkTail(t, m)
	if first == second {
		t.Fatalf("editing the symlink target b/common.hcl left the output unchanged; the tail proves nothing")
	}
	if m.linkedIncludes == 0 {
		t.Fatalf("linkedIncludes = 0: unit a did not resolve its include through the common.hcl symlink")
	}
}

// patternDirTail writes a finding-free unit under dir (a directory named
// like an editor file pattern), reindexes, rewrites it so the output
// changes (broken HCL, a GRT100), and reindexes again. It returns the two
// full-rescan renderings.
func patternDirTail(t fatalf, m *incModel, dir string) (string, string) {
	f := dir + "/terragrunt.hcl"
	m.write(f, unitContent(0, dir, "a"))
	first := m.reindex(t)
	m.write(f, unitContent(unitKinds-1, dir, "a"))
	second := m.reindex(t)
	return first, second
}

// symlinkTail makes common.hcl a link to b/common.hcl, has unit a include
// it through find_in_parent_folders, reindexes, then edits only the link
// target (as a watcher reports it) and reindexes again.
func symlinkTail(t fatalf, m *incModel) (string, string) {
	m.write("b/common.hcl", includeContents[0])
	m.link("common.hcl", "b/common.hcl")
	m.write("a/terragrunt.hcl", unitContent(4, "a", "b"))
	first := m.reindex(t)
	m.write("b/common.hcl", includeContents[3])
	second := m.reindex(t)
	return first, second
}

// recorder is a fatalf that records failures instead of stopping.
type recorder struct{ fails []string }

func (r *recorder) Fatalf(format string, args ...any) {
	r.fails = append(r.fails, fmt.Sprintf(format, args...))
}

// TestIncrementalModelDetectsTypeBlindIgnore proves the model can see the
// v0.3 audit BLOCKER: with the pre-12-01 type-blind ignore rule (editor
// file patterns applied to directories) the adapters skip the subtree of
// a directory named 2024 or x.tmp, the edit never reaches Invalidate, and
// the incremental == full check must fail on the reindex after the edit.
func TestIncrementalModelDetectsTypeBlindIgnore(t *testing.T) {
	saved := ign
	t.Cleanup(func() { ign = saved })
	ign = func(rel string, _ fs.FileMode) bool { return watch.IgnoredEntry(rel, 0) }

	for _, dir := range []string{"2024", "x.tmp"} {
		m := newIncModel()
		m.seed()
		r := &recorder{}
		m.reindex(r)
		f := dir + "/terragrunt.hcl"
		m.write(f, unitContent(0, dir, "a"))
		m.reindex(r)
		before := len(r.fails)
		m.write(f, unitContent(unitKinds-1, dir, "a"))
		m.reindex(r)
		if len(r.fails) == before {
			t.Fatalf("%s: type-blind ignore rule went undetected: incremental == full after editing %s", dir, f)
		}
	}
}
