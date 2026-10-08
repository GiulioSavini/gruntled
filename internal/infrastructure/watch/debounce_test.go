package watch

import (
	"reflect"
	"testing"
	"time"
)

var t0 = time.Date(2026, 10, 8, 12, 0, 0, 0, time.UTC)

func ms(n int) time.Duration { return time.Duration(n) * time.Millisecond }

func TestDebouncerDefaults(t *testing.T) {
	if DefaultQuiet != 150*time.Millisecond {
		t.Fatalf("DefaultQuiet = %v, want 150ms", DefaultQuiet)
	}
	if DefaultMaxWait != time.Second {
		t.Fatalf("DefaultMaxWait = %v, want 1s", DefaultMaxWait)
	}
}

func TestDebouncerTrailingQuiet(t *testing.T) {
	d := NewDebouncer(DefaultQuiet, DefaultMaxWait)
	if _, ok := d.Due(); ok {
		t.Fatal("empty debouncer reports a deadline")
	}
	if _, ok := d.Flush(t0); ok {
		t.Fatal("empty debouncer flushed")
	}
	d.Add(t0, Changes{Paths: []string{"b.hcl"}})
	if due, ok := d.Due(); !ok || !due.Equal(t0.Add(ms(150))) {
		t.Fatalf("Due = %v,%v want t0+150ms", due, ok)
	}
	d.Add(t0.Add(ms(100)), Changes{Paths: []string{"a.hcl", "b.hcl"}})
	due, ok := d.Due()
	if !ok || !due.Equal(t0.Add(ms(250))) {
		t.Fatalf("Due = %v,%v want t0+250ms", due, ok)
	}
	if _, ok := d.Flush(t0.Add(ms(249))); ok {
		t.Fatal("flushed before deadline")
	}
	got, ok := d.Flush(due)
	if !ok || !reflect.DeepEqual(got, Changes{Paths: []string{"a.hcl", "b.hcl"}}) {
		t.Fatalf("Flush = %#v,%v", got, ok)
	}
	if _, ok := d.Due(); ok {
		t.Fatal("deadline survived flush")
	}
	if _, ok := d.Flush(due.Add(time.Hour)); ok {
		t.Fatal("second flush returned a batch")
	}
}

func TestDebouncerMaxWaitCaps(t *testing.T) {
	d := NewDebouncer(DefaultQuiet, DefaultMaxWait)
	for i := 0; i <= 9; i++ {
		now := t0.Add(ms(100 * i))
		d.Add(now, Changes{Paths: []string{"f.hcl"}})
		if b, ok := d.Flush(now); ok {
			t.Fatalf("flushed at +%dms: %#v", 100*i, b)
		}
	}
	due, ok := d.Due()
	if !ok || !due.Equal(t0.Add(time.Second)) {
		t.Fatalf("Due = %v,%v want t0+1s", due, ok)
	}
	got, ok := d.Flush(t0.Add(time.Second))
	if !ok || !reflect.DeepEqual(got.Paths, []string{"f.hcl"}) {
		t.Fatalf("Flush = %#v,%v", got, ok)
	}
}

func TestDebouncerResyncSticky(t *testing.T) {
	d := NewDebouncer(DefaultQuiet, DefaultMaxWait)
	d.Add(t0, Changes{Resync: true})
	d.Add(t0.Add(ms(10)), Changes{Paths: []string{"z.hcl"}})
	got, ok := d.Flush(t0.Add(time.Second))
	if !ok || !got.Resync || !reflect.DeepEqual(got.Paths, []string{"z.hcl"}) {
		t.Fatalf("Flush = %#v,%v", got, ok)
	}
	d.Add(t0.Add(2*time.Second), Changes{Paths: []string{"y.hcl"}})
	got, _ = d.Flush(t0.Add(3 * time.Second))
	if got.Resync {
		t.Fatal("resync leaked into next batch")
	}
}

func TestDebouncerEmptyAddIsNoop(t *testing.T) {
	d := NewDebouncer(DefaultQuiet, DefaultMaxWait)
	d.Add(t0, Changes{})
	if _, ok := d.Due(); ok {
		t.Fatal("empty Changes armed the debouncer")
	}
}
