package tfsurface

import (
	"context"
	"strconv"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/GiulioSavini/gruntled/internal/domain/repograph"
)

// forKinds are the four for-expression spellings hclsyntax builds a
// *hclsyntax.ForExpr for (sec #317).
var forKinds = []string{"list", "object", "quoted", "heredoc"}

// nestedFor returns levels nested for-expressions of the given kind, each
// iterating the literal list [0, 1, ..., n-1]: n^levels inner evaluations
// for anything that calls Value on it.
func nestedFor(kind string, levels, n int) string {
	elems := make([]string, n)
	for i := range elems {
		elems[i] = strconv.Itoa(i)
	}
	list := "[" + strings.Join(elems, ",") + "]"
	var open, close strings.Builder
	for l := range levels {
		v := "v" + strconv.Itoa(l)
		switch kind {
		case "list":
			open.WriteString("[for " + v + " in " + list + " : ")
			close.WriteString("]")
		case "object":
			open.WriteString("{for " + v + " in " + list + " : " + v + " => ")
			close.WriteString("}")
		case "quoted":
			open.WriteString("%{for " + v + " in " + list + "}")
			close.WriteString("%{endfor}")
		case "heredoc":
			open.WriteString("%{for " + v + " in " + list + "}\n")
			close.WriteString("%{endfor}\n")
		}
	}
	switch kind {
	case "quoted":
		return `"` + open.String() + "x" + close.String() + `"`
	case "heredoc":
		return "<<EOT\n" + open.String() + "x\n" + close.String() + "EOT\n"
	}
	return open.String() + "0" + close.String()
}

// forDefaultType is object({a=optional(T, <nested for>)}): a valid type
// constraint whose optional() default typeexpr evaluates with Value(nil).
func forDefaultType(kind string, levels, n int) string {
	elem := "any"
	if kind == "quoted" || kind == "heredoc" {
		elem = "string"
	}
	return "object({a=optional(" + elem + ", " + nestedFor(kind, levels, n) + ")})"
}

// forSizes runs check at 2 x 10 and, only when that passes, at 5 x 100:
// without the guard the small shape fails fast (typeexpr is called) and
// the big one, which would not finish, is skipped.
func forSizes(t *testing.T, check func(t *testing.T, levels, n int)) {
	t.Helper()
	if !t.Run("2x10", func(t *testing.T) { check(t, 2, 10) }) {
		t.Skip("the small shape reached typeexpr; 5x100 would not finish")
	}
	t.Run("5x100", func(t *testing.T) { check(t, 5, 100) })
}

// readForBomb reads module m holding name -> data within the time bound and
// returns the known surface.
func readForBomb(t *testing.T, name, data string) repograph.Surface {
	t.Helper()
	fsys := fstest.MapFS{"m/" + name: {Data: []byte(data)}}
	start := time.Now()
	res, err := NewReader(fsys).ReadSurface(context.Background(), repograph.MustRepoPath("m"))
	took := time.Since(start)
	if err != nil {
		t.Fatal(err)
	}
	if res.UnknownReason != "" {
		t.Fatalf("UnknownReason = %q (diags %v)", res.UnknownReason, res.Diagnostics)
	}
	if bound := time.Second * raceSlowdown; took > bound {
		t.Fatalf("ReadSurface took %v, want < %v", took, bound)
	}
	return res.Surface
}

// TestReadSurfaceForExprDefault (sec #317): nested for-expressions in an
// optional() default, in .tf and in .tf.json, make the type fact unknown
// at once; typeexpr is never called and the variable is still declared.
func TestReadSurfaceForExprDefault(t *testing.T) {
	for _, kind := range forKinds {
		for _, file := range []string{"main.tf", "main.tf.json"} {
			t.Run(kind+" "+file, func(t *testing.T) {
				forSizes(t, func(t *testing.T, levels, n int) {
					calls := countTypeexpr(t)
					typ := forDefaultType(kind, levels, n)
					data := "variable \"x\" {\n  type = " + typ + "\n}\n"
					if file == "main.tf.json" {
						data = `{"variable":{"x":{"type":` + jsonQuote(typ) + `}}}`
					}
					s := readForBomb(t, file, data)
					wantVar(t, s, repograph.VariableDecl{Name: "x", Required: tTrue, Type: ""})
					if *calls != 0 {
						t.Fatalf("typeexpr called %d times, want 0", *calls)
					}
				})
			})
		}
	}
}

// TestReadSurfaceForExprSensitive (sec #317 part 2): nested
// for-expressions in an output's sensitive are unknown without
// evaluation; a .tf.json string is returned verbatim, never evaluated.
func TestReadSurfaceForExprSensitive(t *testing.T) {
	empty := map[string]string{"list": "[]", "object": "{}", "quoted": `""`, "heredoc": `""`}
	for _, kind := range forKinds {
		for _, file := range []string{"main.tf", "main.tf.json"} {
			t.Run(kind+" "+file, func(t *testing.T) {
				forSizes(t, func(t *testing.T, levels, n int) {
					sens := "(" + nestedFor(kind, levels, n) + ") == " + empty[kind]
					data := "output \"o\" {\n  value = 1\n  sensitive = " + sens + "\n}\n"
					if file == "main.tf.json" {
						data = `{"output":{"o":{"value":1,"sensitive":` + jsonQuote("${"+sens+"}") + `}}}`
					}
					s := readForBomb(t, file, data)
					wantOut(t, s, repograph.OutputDecl{Name: "o", Type: "", Sensitive: tUnknown})
				})
			})
		}
	}
}
