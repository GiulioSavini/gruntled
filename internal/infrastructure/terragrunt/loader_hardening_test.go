package terragrunt

import (
	"context"
	"reflect"
	"sort"
	"sync"
	"testing"
	"testing/fstest"
)

// hardeningFS has units whose paths exercise prefix semantics: "a" is a
// parent of "a/b" and a string prefix (but not a path prefix) of "ab".
func hardeningFS() fstest.MapFS {
	unit := func(dir string) *fstest.MapFile {
		return &fstest.MapFile{Data: []byte(`terraform { source = "` + up(dir) + `modules/m" }
dependency "x" { config_path = "` + up(dir) + `c" }
inputs = { v = dependency.x.outputs.o }`)}
	}
	return fstest.MapFS{
		"a/terragrunt.hcl":    unit("a"),
		"a/b/terragrunt.hcl":  unit("a/b"),
		"ab/terragrunt.hcl":   unit("ab"),
		"ab/x/terragrunt.hcl": unit("ab/x"),
		"c/terragrunt.hcl":    {Data: []byte(`terraform { source = "../modules/m" }`)},
		"modules/m/main.tf":   {Data: []byte(`output "o" { value = 1 }`)},
	}
}

// warmLoader returns a Loader over fsys after one full load, and the miss
// count of that first load (the number of store entries a cold load fills).
func warmLoader(t *testing.T, fsys fstest.MapFS) (*Loader, int) {
	t.Helper()
	l := NewLoader(fsys)
	if _, err := l.LoadUnits(context.Background()); err != nil {
		t.Fatalf("LoadUnits: %v", err)
	}
	_, misses := l.CacheStats()
	if misses == 0 {
		t.Fatalf("cold load had no misses; fixture is vacuous")
	}
	return l, misses
}

func storeKeys(l *Loader) []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	keys := make([]string, 0, len(l.store.files))
	for k := range l.store.files {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func TestInvalidateBatchEquivalent(t *testing.T) {
	fsys := hardeningFS()
	batch, _ := warmLoader(t, fsys)
	single, _ := warmLoader(t, fsys)

	batch.Invalidate("a", "ab/x/terragrunt.hcl")
	single.Invalidate("a")
	single.Invalidate("ab/x/terragrunt.hcl")

	got, want := storeKeys(batch), storeKeys(single)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("batch evicted differently:\n got  %q\n want %q", got, want)
	}
	for _, k := range got {
		if k == "a/terragrunt.hcl" || k == "a/b/terragrunt.hcl" || k == "ab/x/terragrunt.hcl" {
			t.Errorf("%q not evicted", k)
		}
	}
	if !contains(got, "ab/terragrunt.hcl") {
		t.Errorf(`"a" evicted "ab/terragrunt.hcl" (prefix must be path-wise); keys %q`, got)
	}
	if !contains(got, "c/terragrunt.hcl") {
		t.Errorf("unrelated c/terragrunt.hcl evicted; keys %q", got)
	}
}

func contains(s []string, v string) bool {
	for _, x := range s {
		if x == v {
			return true
		}
	}
	return false
}

// assertClearedAll checks that the next load re-reads every file.
func assertClearedAll(t *testing.T, l *Loader, coldMisses int, input []string) {
	t.Helper()
	if keys := storeKeys(l); len(keys) != 0 {
		t.Fatalf("Invalidate(%q) left %q in the store", input, keys)
	}
	if _, err := l.LoadUnits(context.Background()); err != nil {
		t.Fatalf("LoadUnits: %v", err)
	}
	hits, misses := l.CacheStats()
	if hits != 0 || misses != coldMisses {
		t.Fatalf("Invalidate(%q): hits=%d misses=%d, want hits=0 misses=%d", input, hits, misses, coldMisses)
	}
}

func TestInvalidateEscapingClearsAll(t *testing.T) {
	fsys := hardeningFS()
	for _, raw := range []string{"", ".", "/abs/x", "..", "../x", "a/../../x"} {
		for _, input := range [][]string{{raw}, {"c/terragrunt.hcl", raw, "a"}} {
			l, cold := warmLoader(t, fsys)
			l.Invalidate(input...)
			assertClearedAll(t, l, cold, input)
		}
	}
}

func TestInvalidateBackslashClearsAll(t *testing.T) {
	fsys := hardeningFS()
	for _, raw := range []string{`a\b`, `a\b\terragrunt.hcl`} {
		for _, input := range [][]string{{raw}, {"a/b/terragrunt.hcl", raw}, {raw, "c"}} {
			l, cold := warmLoader(t, fsys)
			l.Invalidate(input...)
			assertClearedAll(t, l, cold, input)
		}
	}
}

func TestInvalidateCleanedInputs(t *testing.T) {
	fsys := hardeningFS()
	want, _ := warmLoader(t, fsys)
	want.Invalidate("a/b")
	for _, raw := range []string{"a//b/", "./a/b"} {
		l, _ := warmLoader(t, fsys)
		l.Invalidate(raw)
		if got := storeKeys(l); !reflect.DeepEqual(got, storeKeys(want)) {
			t.Errorf("Invalidate(%q) = %q, want %q", raw, got, storeKeys(want))
		}
	}
	if contains(storeKeys(want), "a/b/terragrunt.hcl") || !contains(storeKeys(want), "a/terragrunt.hcl") {
		t.Fatalf("Invalidate(\"a/b\") evicted the wrong keys: %q", storeKeys(want))
	}
}

// TestConcurrentInvalidateLoad is meant for CI's -race run: without the
// Loader mutex, concurrent Invalidate and LoadUnits write the store map
// concurrently. The fixture is never mutated, so the final load must equal
// a fresh Loader's.
func TestConcurrentInvalidateLoad(t *testing.T) {
	fsys := hardeningFS()
	l := NewLoader(fsys)
	ctx := context.Background()

	const workers, iters = 8, 20
	var wg sync.WaitGroup
	errs := make(chan error, workers*iters)
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			for i := 0; i < iters; i++ {
				switch (w + i) % 4 {
				case 0:
					l.Invalidate("a")
				case 1:
					l.Invalidate(".")
				case 2:
					l.CacheStats()
				default:
					if _, err := l.LoadUnits(ctx); err != nil {
						errs <- err
					}
				}
			}
		}(w)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatalf("LoadUnits: %v", err)
	}

	got, err := l.LoadUnits(ctx)
	if err != nil {
		t.Fatalf("final LoadUnits: %v", err)
	}
	want, err := NewLoader(fsys).LoadUnits(ctx)
	if err != nil {
		t.Fatalf("fresh LoadUnits: %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("concurrent Loader diverged from a fresh one:\n got  %+v\n want %+v", got, want)
	}
}
