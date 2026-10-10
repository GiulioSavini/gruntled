package repograph_test

import (
	"reflect"
	"strconv"
	"testing"

	"github.com/GiulioSavini/gruntled/internal/domain/repograph"
)

func TestNewSurfaceFacts(t *testing.T) {
	vars := []repograph.VariableDecl{
		{Name: "region", Required: repograph.TristateFalse, Type: "string"},
		{Name: "cidr", Required: repograph.TristateTrue, Type: "any"},
	}
	outs := []repograph.OutputDecl{
		{Name: "vpc_id", Type: "", Sensitive: repograph.TristateFalse},
		{Name: "arn", Type: "string", Sensitive: repograph.TristateTrue},
	}
	s, err := repograph.NewSurfaceFacts(vars, outs)
	if err != nil {
		t.Fatalf("NewSurfaceFacts: %v", err)
	}
	if got := s.Variables(); !reflect.DeepEqual(got, []string{"cidr", "region"}) {
		t.Errorf("Variables = %v", got)
	}
	if got := s.Outputs(); !reflect.DeepEqual(got, []string{"arn", "vpc_id"}) {
		t.Errorf("Outputs = %v", got)
	}
	if d, ok := s.Variable("cidr"); !ok || d != vars[1] {
		t.Errorf("Variable(cidr) = %+v, %v", d, ok)
	}
	if d, ok := s.Output("arn"); !ok || d != outs[1] {
		t.Errorf("Output(arn) = %+v, %v", d, ok)
	}
	if _, ok := s.Variable("nope"); ok {
		t.Error("Variable(nope) found")
	}
	if _, ok := s.Output("nope"); ok {
		t.Error("Output(nope) found")
	}

	// Same validation and messages as NewSurface.
	for _, c := range []struct {
		vars []repograph.VariableDecl
		outs []repograph.OutputDecl
		ns   [2][]string
	}{
		{vars: []repograph.VariableDecl{{Name: ""}}, ns: [2][]string{{""}, nil}},
		{vars: []repograph.VariableDecl{{Name: "a"}, {Name: "a"}}, ns: [2][]string{{"a", "a"}, nil}},
		{outs: []repograph.OutputDecl{{Name: ""}}, ns: [2][]string{nil, {""}}},
		{outs: []repograph.OutputDecl{{Name: "o"}, {Name: "o"}}, ns: [2][]string{nil, {"o", "o"}}},
	} {
		_, err := repograph.NewSurfaceFacts(c.vars, c.outs)
		_, want := repograph.NewSurface(c.ns[0], c.ns[1])
		if err == nil || want == nil || err.Error() != want.Error() {
			t.Errorf("NewSurfaceFacts error %v, NewSurface error %v", err, want)
		}
	}
}

func TestNewSurfaceFactsUnknownByDefault(t *testing.T) {
	s, err := repograph.NewSurface([]string{"a"}, []string{"o"})
	if err != nil {
		t.Fatal(err)
	}
	if d, ok := s.Variable("a"); !ok || d != (repograph.VariableDecl{Name: "a", Required: repograph.TristateUnknown, Type: ""}) {
		t.Errorf("Variable(a) = %+v, %v", d, ok)
	}
	if d, ok := s.Output("o"); !ok || d != (repograph.OutputDecl{Name: "o", Type: "", Sensitive: repograph.TristateUnknown}) {
		t.Errorf("Output(o) = %+v, %v", d, ok)
	}
	var zero repograph.Surface
	if _, ok := zero.Variable("a"); ok {
		t.Error("zero Surface has a variable")
	}
}

// xorshift drives the property loop: domain tests may not import math/rand.
type xorshift uint64

func (x *xorshift) intn(n int) int {
	*x ^= *x << 13
	*x ^= *x >> 7
	*x ^= *x << 17
	return int(uint64(*x) % uint64(n))
}

func TestSurfaceViewsUnchanged(t *testing.T) {
	rng := xorshift(15)
	names := []string{"a", "b", "c", "d", "vpc_id", "region", "x1", "x2"}
	for range 2000 {
		var vn, on []string
		var vd []repograph.VariableDecl
		var od []repograph.OutputDecl
		for _, n := range names {
			if rng.intn(2) == 0 {
				vn = append(vn, n)
				vd = append(vd, repograph.VariableDecl{Name: n, Required: repograph.Tristate(rng.intn(3)), Type: strconv.Itoa(rng.intn(3))})
			}
			if rng.intn(2) == 0 {
				on = append(on, n)
				od = append(od, repograph.OutputDecl{Name: n, Type: strconv.Itoa(rng.intn(3)), Sensitive: repograph.Tristate(rng.intn(3))})
			}
		}
		// Present the decls in reverse order: the surface sorts them.
		for i, j := 0, len(vd)-1; i < j; i, j = i+1, j-1 {
			vd[i], vd[j] = vd[j], vd[i]
		}
		a, err := repograph.NewSurface(vn, on)
		if err != nil {
			t.Fatal(err)
		}
		b, err := repograph.NewSurfaceFacts(vd, od)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(a.Variables(), b.Variables()) || !reflect.DeepEqual(a.Outputs(), b.Outputs()) {
			t.Fatalf("name views differ: %v/%v vs %v/%v", a.Variables(), a.Outputs(), b.Variables(), b.Outputs())
		}
		for _, n := range append(names, "zz") {
			if a.HasVariable(n) != b.HasVariable(n) || a.HasOutput(n) != b.HasOutput(n) {
				t.Fatalf("Has* differ for %q", n)
			}
		}
		for _, d := range vd {
			if got, ok := b.Variable(d.Name); !ok || got != d {
				t.Fatalf("Variable(%q) = %+v, want %+v", d.Name, got, d)
			}
		}
	}
}

func TestSurfaceFactsAreCopies(t *testing.T) {
	vars := []repograph.VariableDecl{{Name: "a", Required: repograph.TristateTrue, Type: "string"}}
	s, err := repograph.NewSurfaceFacts(vars, nil)
	if err != nil {
		t.Fatal(err)
	}
	vars[0].Type = "number"
	d, _ := s.Variable("a")
	if d.Type != "string" {
		t.Fatalf("the Surface aliases its input: %+v", d)
	}
	d.Type = "bool"
	if again, _ := s.Variable("a"); again.Type != "string" {
		t.Fatalf("Variable returned a reference: %+v", again)
	}
}
