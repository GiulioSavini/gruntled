package main

// VALID-06: `gruntled check` is faster than plain `terragrunt hcl validate`
// on the same repository. Env-gated on GRUNTLED_CORPUS and
// GRUNTLED_TERRAGRUNT_BIN, so CI skips it. os/exec is used only here, in a
// _test.go file.

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"
)

const (
	benchSamples = 21
	benchWarmups = 3
)

// benchProcField returns the value of the first line in a /proc file that
// starts with key, or "n/a".
func benchProcField(path, key string) string {
	b, err := os.ReadFile(path)
	if err != nil {
		return "n/a"
	}
	for _, line := range strings.Split(string(b), "\n") {
		if strings.HasPrefix(line, key) {
			if _, v, ok := strings.Cut(line, ":"); ok {
				return strings.Join(strings.Fields(v), " ")
			}
		}
	}
	return "n/a"
}

// benchProcFile returns the trimmed content of a /proc file, or "n/a".
func benchProcFile(path string) string {
	b, err := os.ReadFile(path)
	if err != nil {
		return "n/a"
	}
	return strings.TrimSpace(string(b))
}

// benchMachineInfo describes the machine as VALIDATION-BENCH lines. It reads
// /proc directly and runs nothing. loadavg is recorded so a busy machine is
// visible next to the numbers it produced.
func benchMachineInfo() string {
	cpu, mem, kernel, load := "n/a", "n/a", "n/a", "n/a"
	if runtime.GOOS == "linux" {
		cpu = benchProcField("/proc/cpuinfo", "model name")
		mem = benchProcField("/proc/meminfo", "MemTotal")
		kernel = benchProcFile("/proc/sys/kernel/osrelease")
		load = benchProcFile("/proc/loadavg")
	}
	var b strings.Builder
	fmt.Fprintf(&b, "VALIDATION-BENCH goos=%s goarch=%s\n", runtime.GOOS, runtime.GOARCH)
	fmt.Fprintf(&b, "VALIDATION-BENCH cpus=%d\n", runtime.NumCPU())
	fmt.Fprintf(&b, "VALIDATION-BENCH go=%s\n", runtime.Version())
	fmt.Fprintf(&b, "VALIDATION-BENCH cpu_model=%s\n", cpu)
	fmt.Fprintf(&b, "VALIDATION-BENCH mem_total=%s\n", mem)
	fmt.Fprintf(&b, "VALIDATION-BENCH kernel=%s\n", kernel)
	fmt.Fprintf(&b, "VALIDATION-BENCH loadavg=%s\n", load)
	return b.String()
}

// benchBuildGruntled builds the same binary a user runs, once per test.
func benchBuildGruntled(t testing.TB) string {
	t.Helper()
	out := filepath.Join(t.TempDir(), "gruntled")
	cmd := exec.Command("go", "build", "-trimpath", "-o", out, ".")
	cmd.Dir = "."
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("go build gruntled: %v\n%s", err, stderr.String())
	}
	return out
}

// benchRun runs one command with the shared env and cwd, discarding output
// unless the exit code is not 0. A failing run is fatal: an erroring run
// must never count as a fast one.
func benchRun(t testing.TB, name string, argv, env []string, dir string) time.Duration {
	t.Helper()
	cmd := exec.Command(argv[0], argv[1:]...)
	cmd.Env = env
	cmd.Dir = dir
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	start := time.Now()
	err := cmd.Run()
	d := time.Since(start)
	if err != nil {
		t.Fatalf("%s: %v (want exit 0)\n%s", name, err, out.String())
	}
	return d
}

type benchStats struct {
	min, p25, median, p75, max time.Duration
}

// benchQuantile is the nearest-rank quantile of a sorted slice.
func benchQuantile(sorted []time.Duration, q float64) time.Duration {
	i := int(q*float64(len(sorted)-1) + 0.5)
	return sorted[i]
}

func benchSummarize(sorted []time.Duration) benchStats {
	return benchStats{
		min:    sorted[0],
		p25:    benchQuantile(sorted, 0.25),
		median: benchQuantile(sorted, 0.5),
		p75:    benchQuantile(sorted, 0.75),
		max:    sorted[len(sorted)-1],
	}
}

func benchMS(d time.Duration) string {
	return fmt.Sprintf("%.3f", float64(d)/float64(time.Millisecond))
}

func benchList(ds []time.Duration) string {
	parts := make([]string, len(ds))
	for i, d := range ds {
		parts[i] = benchMS(d)
	}
	return strings.Join(parts, ",")
}

// TestBenchmarkVsTerragrunt times both tools as external processes on the
// same scratch copy, with the same env and cwd: 3 untimed warm-ups each,
// then 21 interleaved samples each, compared by median.
func TestBenchmarkVsTerragrunt(t *testing.T) {
	corpus := corpusRequire(t)
	tg := tgVerifyPinned(t)
	before := corpusDigest(t, corpus)
	dir := corpusCopy(t, corpus)
	gr := benchBuildGruntled(t)
	env := tgEnv(t, tg)

	grArgv := []string{gr, "check", dir}
	// Plain hcl validate only: no --inputs, --strict, --log-disable or any
	// other flag that changes the work terragrunt does.
	tgArgv := []string{tg, "hcl", "validate", "--working-dir", dir, "--no-color"}

	for range benchWarmups {
		benchRun(t, "gruntled warm-up", grArgv, env, dir)
		benchRun(t, "terragrunt warm-up", tgArgv, env, dir)
	}

	grSamples := make([]time.Duration, 0, benchSamples)
	tgSamples := make([]time.Duration, 0, benchSamples)
	for i := range benchSamples {
		if i%2 == 0 {
			grSamples = append(grSamples, benchRun(t, "gruntled", grArgv, env, dir))
			tgSamples = append(tgSamples, benchRun(t, "terragrunt", tgArgv, env, dir))
		} else {
			tgSamples = append(tgSamples, benchRun(t, "terragrunt", tgArgv, env, dir))
			grSamples = append(grSamples, benchRun(t, "gruntled", grArgv, env, dir))
		}
	}
	slices.Sort(grSamples)
	slices.Sort(tgSamples)
	gs, ts := benchSummarize(grSamples), benchSummarize(tgSamples)
	ratio := float64(ts.median) / float64(gs.median)

	var b strings.Builder
	b.WriteString("\n")
	b.WriteString(benchMachineInfo())
	fmt.Fprintf(&b, "VALIDATION-BENCH samples=%d warmups=%d\n", benchSamples, benchWarmups)
	for _, row := range []struct {
		name    string
		samples []time.Duration
		s       benchStats
	}{{"gruntled", grSamples, gs}, {"terragrunt", tgSamples, ts}} {
		fmt.Fprintf(&b, "VALIDATION-BENCH %s_samples_ms=%s\n", row.name, benchList(row.samples))
		fmt.Fprintf(&b, "VALIDATION-BENCH %s_min_ms=%s p25_ms=%s median_ms=%s p75_ms=%s max_ms=%s\n",
			row.name, benchMS(row.s.min), benchMS(row.s.p25), benchMS(row.s.median), benchMS(row.s.p75), benchMS(row.s.max))
	}
	fmt.Fprintf(&b, "VALIDATION-BENCH ratio_tg_over_gruntled_median=%.2f\n", ratio)
	t.Log(b.String())

	if gs.median >= ts.median {
		t.Errorf("VALID-06 FAILED: gruntled median %s ms >= terragrunt median %s ms\ngruntled:   %s\nterragrunt: %s",
			benchMS(gs.median), benchMS(ts.median), benchList(grSamples), benchList(tgSamples))
	}
	if after := corpusDigest(t, corpus); after != before {
		t.Errorf("corpus checkout changed during the benchmark: digest %s -> %s", before, after)
	}
}

// BenchmarkGruntledCheckCorpus is the in-process cost of `gruntled check`,
// without process start. It is not the VALID-06 comparison (see
// TestBenchmarkVsTerragrunt); it exists so `go test -bench` gives ns/op.
func BenchmarkGruntledCheckCorpus(b *testing.B) {
	corpus := corpusRequire(b)
	dir := corpusCopy(b, corpus)
	for b.Loop() {
		if code := run([]string{"check", dir}, io.Discard, io.Discard); code != 0 {
			b.Fatalf("gruntled check: exit %d, want 0", code)
		}
	}
}

// BenchmarkTerragruntHCLValidate is the process-level cost of plain
// `terragrunt hcl validate`, process start included. It is not comparable
// with BenchmarkGruntledCheckCorpus (process vs in-process) and is not the
// VALID-06 comparison.
func BenchmarkTerragruntHCLValidate(b *testing.B) {
	corpus := corpusRequire(b)
	tg := tgVerifyPinned(b)
	dir := corpusCopy(b, corpus)
	env := tgEnv(b, tg)
	argv := []string{tg, "hcl", "validate", "--working-dir", dir, "--no-color"}
	for b.Loop() {
		benchRun(b, "terragrunt", argv, env, dir)
	}
}
