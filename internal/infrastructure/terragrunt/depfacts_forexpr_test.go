package terragrunt

import (
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/GiulioSavini/gruntled/internal/domain/repograph"
)

// nestedForBool returns `(<levels nested list fors over [0..n-1]>) == []`:
// a literal bool that costs n^levels inner evaluations to compute.
func nestedForBool(levels, n int) string {
	elems := make([]string, n)
	for i := range elems {
		elems[i] = strconv.Itoa(i)
	}
	list := "[" + strings.Join(elems, ",") + "]"
	var b strings.Builder
	for l := range levels {
		b.WriteString("[for v" + strconv.Itoa(l) + " in " + list + " : ")
	}
	b.WriteString("0" + strings.Repeat("]", levels))
	return "(" + b.String() + ") == []"
}

// TestDependencyOptionsForExpr (sec #318, pre-existing in v0.3.0): nested
// for-expressions in enabled or skip_outputs are never evaluated; the fact
// is unknown at once even at 5 levels x 100 elements. The 2 x 10 shape
// runs first, so without the guard it fails fast instead of running the
// big one.
func TestDependencyOptionsForExpr(t *testing.T) {
	for _, attr := range []string{"enabled", "skip_outputs"} {
		t.Run(attr, func(t *testing.T) {
			check := func(t *testing.T, levels, n int) {
				body, _ := parseBody(t, "d.hcl", attr+" = "+nestedForBool(levels, n)+"\n")
				start := time.Now()
				opts := dependencyOptions(body)
				took := time.Since(start)
				got := opts.Enabled
				if attr == "skip_outputs" {
					got = opts.SkipOutputs
				}
				if got != repograph.TristateUnknown {
					t.Fatalf("%s = %v, want unknown", attr, got)
				}
				if took > time.Second {
					t.Fatalf("dependencyOptions took %v, want < 1s", took)
				}
			}
			if !t.Run("2x10", func(t *testing.T) { check(t, 2, 10) }) {
				t.Skip("the small shape is evaluated; 5x100 would not finish")
			}
			t.Run("5x100", func(t *testing.T) { check(t, 5, 100) })
		})
	}
}
