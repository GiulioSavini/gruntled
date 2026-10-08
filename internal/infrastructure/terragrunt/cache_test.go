package terragrunt

import (
	"context"
	"reflect"
	"testing"
	"testing/fstest"

	"github.com/GiulioSavini/gruntled/internal/application/ports"
	"github.com/GiulioSavini/gruntled/internal/domain/repograph"
)

// loadOK runs l.LoadUnits and fails the test on a Go-level error.
func loadOK(t *testing.T, l *Loader) ports.LoadResult {
	t.Helper()
	res, err := l.LoadUnits(context.Background())
	if err != nil {
		t.Fatalf("LoadUnits: %v", err)
	}
	return res
}

// countGRT100 returns how many GRT100 diagnostics res carries.
func countGRT100(res ports.LoadResult) int {
	n := 0
	for _, d := range res.Diagnostics {
		if string(d.Key().Code) == "GRT100" {
			n++
		}
	}
	return n
}

// unitReason returns the ConfigUnknownReason of the unit at dir.
func unitReason(t *testing.T, res ports.LoadResult, dir string) string {
	t.Helper()
	for _, u := range res.Units {
		if u.Path.String() == dir {
			return u.ConfigUnknownReason
		}
	}
	t.Fatalf("unit %q not found", dir)
	return ""
}

// assertSameAsFresh fails unless l's next LoadUnits equals a brand-new
// Loader's result over the same fsys: incremental == full.
func assertSameAsFresh(t *testing.T, got ports.LoadResult, fsys fstest.MapFS) {
	t.Helper()
	want := loadOK(t, NewLoader(fsys))
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("persistent-cache result differs from fresh Loader:\n got: %+v\nwant: %+v", got, want)
	}
}

func TestPersistentCacheStats(t *testing.T) {
	m := filesFS(map[string]string{
		"root.hcl":         `locals { x = 1 }`,
		"a/terragrunt.hcl": `include "root" { path = "../root.hcl" }`,
		"b/terragrunt.hcl": `terraform { source = "../mod" }`,
		"mod/main.tf":      `output "o" { value = 1 }`,
	})
	cfs := newCountingFS(m)
	l := NewLoader(cfs)

	first := loadOK(t, l)
	if _, misses := l.CacheStats(); misses == 0 {
		t.Fatalf("first load: misses = 0, want > 0")
	}
	files := []string{"root.hcl", "a/terragrunt.hcl", "b/terragrunt.hcl"}
	before := map[string]int{}
	for _, f := range files {
		before[f] = cfs.count(f)
	}

	second := loadOK(t, l)
	hits, misses := l.CacheStats()
	if misses != 0 {
		t.Fatalf("no-op reload: misses = %d, want 0", misses)
	}
	if hits == 0 {
		t.Fatalf("no-op reload: hits = 0, want > 0")
	}
	for _, f := range files {
		if got := cfs.count(f); got != before[f] {
			t.Errorf("no-op reload reopened %s: count %d -> %d", f, before[f], got)
		}
	}
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("no-op reload changed result")
	}

	m["b/terragrunt.hcl"] = &fstest.MapFile{Data: []byte(`terraform { source = "../mod2" }`)}
	l.Invalidate("b/terragrunt.hcl")
	third := loadOK(t, l)
	if _, misses := l.CacheStats(); misses != 1 {
		t.Fatalf("after one edit: misses = %d, want 1", misses)
	}
	assertSameAsFresh(t, third, m)
}

func TestPersistentCacheNoStaleSyntaxDiag(t *testing.T) {
	t.Run("deleted", func(t *testing.T) {
		m := filesFS(map[string]string{
			"ok/terragrunt.hcl":  `locals { x = 1 }`,
			"bad/terragrunt.hcl": `locals {`,
		})
		l := NewLoader(m)
		if n := countGRT100(loadOK(t, l)); n != 1 {
			t.Fatalf("broken unit: GRT100 = %d, want 1", n)
		}
		delete(m, "bad/terragrunt.hcl")
		l.Invalidate("bad/terragrunt.hcl")
		res := loadOK(t, l)
		if n := countGRT100(res); n != 0 {
			t.Fatalf("after delete: GRT100 = %d, want 0", n)
		}
		assertSameAsFresh(t, res, m)
	})

	t.Run("unreferenced", func(t *testing.T) {
		m := filesFS(map[string]string{
			"broken.hcl":       `locals {`,
			"u/terragrunt.hcl": `include "root" { path = "../broken.hcl" }`,
		})
		l := NewLoader(m)
		if n := countGRT100(loadOK(t, l)); n != 1 {
			t.Fatalf("broken include: GRT100 = %d, want 1", n)
		}
		// broken.hcl stays on disk and is NOT invalidated: only the unit
		// changes, so the diagnostic must follow what this load touched.
		m["u/terragrunt.hcl"] = &fstest.MapFile{Data: []byte(`locals { x = 1 }`)}
		l.Invalidate("u/terragrunt.hcl")
		res := loadOK(t, l)
		if n := countGRT100(res); n != 0 {
			t.Fatalf("after unreferencing: GRT100 = %d, want 0", n)
		}
		assertSameAsFresh(t, res, m)
	})
}

func TestPersistentCacheNegativeEntry(t *testing.T) {
	m := filesFS(map[string]string{
		"u/terragrunt.hcl": `include "root" { path = "../root.hcl" }`,
	})
	l := NewLoader(m)
	if r := unitReason(t, loadOK(t, l), "u"); r != ReasonIncludeNotFound {
		t.Fatalf("missing include: reason = %q, want %q", r, ReasonIncludeNotFound)
	}

	// Seed a negative (readErr) entry for root.hcl in the Loader's store,
	// as a read racing the file's creation would.
	root := repograph.MustRepoPath("root.hcl")
	if pf := newFileCacheOn(m, l.store).get(root); pf.readErr == nil {
		t.Fatalf("seeding: want readErr for missing root.hcl")
	}

	m["root.hcl"] = &fstest.MapFile{Data: []byte(`locals { x = 1 }`)}

	// Without Invalidate the cached readErr is still served: this is the
	// dirty-set contract (a create MUST be invalidated).
	if r := unitReason(t, loadOK(t, &Loader{fsys: m, store: l.store}), "u"); r != ReasonUnreadableConfig {
		t.Fatalf("un-invalidated create: reason = %q, want %q", r, ReasonUnreadableConfig)
	}

	l.Invalidate("root.hcl")
	res := loadOK(t, l)
	if r := unitReason(t, res, "u"); r != "" {
		t.Fatalf("after create+Invalidate: reason = %q, want known", r)
	}
	assertSameAsFresh(t, res, m)
}

func TestPersistentCacheInvalidateDirPrefix(t *testing.T) {
	m := filesFS(map[string]string{
		"a/terragrunt.hcl":     `locals { x = 1 }`,
		"a/sub/terragrunt.hcl": `locals { y = 1 }`,
		"ab/terragrunt.hcl":    `locals { z = 1 }`,
	})
	cfs := newCountingFS(m)
	l := NewLoader(cfs)
	loadOK(t, l)

	m["a/sub/terragrunt.hcl"] = &fstest.MapFile{Data: []byte(`locals {`)}
	abBefore := cfs.count("ab/terragrunt.hcl")
	l.Invalidate("a")
	res := loadOK(t, l)
	if _, misses := l.CacheStats(); misses < 1 {
		t.Fatalf("after dir Invalidate: misses = %d, want >= 1", misses)
	}
	if got := cfs.count("ab/terragrunt.hcl"); got != abBefore {
		t.Fatalf("Invalidate(\"a\") evicted ab/terragrunt.hcl: count %d -> %d", abBefore, got)
	}
	if n := countGRT100(res); n != 1 {
		t.Fatalf("edited child: GRT100 = %d, want 1", n)
	}
	assertSameAsFresh(t, res, m)

	// "." clears everything.
	l.Invalidate(".")
	loadOK(t, l)
	if hits, _ := l.CacheStats(); hits != 0 {
		t.Fatalf("after Invalidate(\".\"): hits = %d, want 0", hits)
	}
}
