package main

// DAEMON-10 (16-04): what the daemon's baseline costs. The publisher holds
// the baseline Report, the last Report and the served snapshot with its
// blast view; that must stay a small multiple of one Report, never a
// pinned parse cache.

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"

	"github.com/GiulioSavini/gruntled/internal/application/checking"
	"github.com/GiulioSavini/gruntled/internal/application/watching"
	"github.com/GiulioSavini/gruntled/internal/infrastructure/ipc"
	"github.com/GiulioSavini/gruntled/internal/infrastructure/terragrunt"
	"github.com/GiulioSavini/gruntled/internal/infrastructure/tfsurface"
	"github.com/GiulioSavini/gruntled/internal/testsupport/synthrepo"
)

// heapInuse is HeapInuse after two full collections.
func heapInuse() int64 {
	runtime.GC()
	runtime.GC()
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	return int64(m.HeapInuse)
}

// indexerOn is the daemon's indexer over dir: one invalidating loader and
// a surface reader on an os.Root, as runWatchWith builds them.
func indexerOn(t *testing.T, dir string) (*watching.Indexer, func()) {
	t.Helper()
	root, err := os.OpenRoot(dir)
	mustDo(t, err)
	fsys := root.FS()
	return watching.NewIndexer(terragrunt.NewLoader(fsys), tfsurface.NewReader(fsys)), func() { _ = root.Close() }
}

var firstOutput = regexp.MustCompile(`output "([^"]+)"`)

// TestDaemonMemorySynthrepo measures, on synthrepo trees of 65, 500 and
// 5,000 units, the retained heap of one Report and of a publisher holding
// the baseline, the last Report and the sealed snapshot after one edit
// (an output of unit 0 renamed, reindexed with that dirty path). It logs
// VALIDATION-BENCH lines for docs/validation.md and asserts the publisher
// retains at most 2.5 x one Report + the view bytes + 1 MiB. 5,000 units
// is skipped under -short.
func TestDaemonMemorySynthrepo(t *testing.T) {
	t.Log("\n" + benchMachineInfo())
	for _, units := range []int{65, 500, 5000} {
		t.Run(fmt.Sprintf("units=%d", units), func(t *testing.T) {
			if units >= 5000 && testing.Short() {
				t.Skip("5,000 units: CI, or one local run on its own")
			}
			dir := t.TempDir()
			if _, err := synthrepo.Generate(synthrepo.Spec{Units: units, IncludeDepth: 3, DependencyFanout: 3, Seed: 16}, dir); err != nil {
				t.Fatal(err)
			}
			// The edit: unit 0 is the one every other unit may depend on.
			var edit string
			mustDo(t, filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
				if err == nil && edit == "" && d.Name() == "main.tf" && strings.HasPrefix(filepath.Base(filepath.Dir(p)), "unit-") &&
					strings.Trim(strings.TrimPrefix(filepath.Base(filepath.Dir(p)), "unit-"), "0") == "" {
					edit = p
				}
				return err
			}))
			src, err := os.ReadFile(edit)
			mustDo(t, err)
			m := firstOutput.FindSubmatchIndex(src)
			if m == nil {
				t.Fatalf("%s declares no output", edit)
			}
			edited := string(src[:m[3]]) + "_renamed" + string(src[m[3]:])
			rel, err := filepath.Rel(dir, edit)
			mustDo(t, err)

			h0 := heapInuse()
			rep1 := func() checking.Report {
				idx, done := indexerOn(t, dir)
				defer done()
				r, err := idx.Index(context.Background(), nil, false)
				mustDo(t, err)
				return r
			}()
			h1 := heapInuse()

			p := newTestPublisher(t)
			info := ipc.Info{PID: os.Getpid()}
			p.snapshots = func(s *ipc.Snapshot) { ipc.Seal(info, s); p.served.Store(s) } // as instance.store
			p.publish(ready(rep1))
			rep2 := func() checking.Report {
				idx, done := indexerOn(t, dir)
				defer done()
				_, err := idx.Index(context.Background(), nil, false) // warm the cache
				mustDo(t, err)
				mustDo(t, os.WriteFile(edit, []byte(edited), 0o644))
				r, err := idx.Index(context.Background(), []string{filepath.ToSlash(rel)}, false)
				mustDo(t, err)
				return r
			}()
			p.publish(ready(rep2))
			h2 := heapInuse()

			s := p.served.Load()
			if s == nil || s.Blast == nil || s.Blast.Error != "" || s.Generation != 2 {
				t.Fatalf("served snapshot: %+v", s)
			}
			view := int64(len(s.Blast.Text) + len(s.Blast.JSON))
			report, pub := h1-h0, h2-h0
			t.Logf("VALIDATION-BENCH daemon_mem units=%d report_heap=%d publisher_heap=%d view_bytes=%d ratio=%.2f",
				units, report, pub, view, float64(pub)/float64(max(report, 1)))
			if !strings.Contains(string(s.Blast.Text), "_renamed") {
				t.Fatalf("the edit is not in the view:\n%.2000s", s.Blast.Text)
			}
			if bound := report*5/2 + view + 1<<20; pub > bound {
				t.Errorf("publisher retains %d bytes, over 2.5 x %d (one Report) + %d (view) + 1 MiB = %d", pub, report, view, bound)
			}
			runtime.KeepAlive(rep1)
			runtime.KeepAlive(rep2)
			runtime.KeepAlive(p)
		})
	}
}
