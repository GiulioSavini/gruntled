package analysis_test

import (
	"reflect"
	"testing"

	"github.com/GiulioSavini/gruntled/internal/domain/analysis"
	"github.com/GiulioSavini/gruntled/internal/domain/diagnostic"
	"github.com/GiulioSavini/gruntled/internal/domain/repograph"
)

// gotDiag is the comparable projection of a diagnostic used by the GRT002
// and GRT003 tests.
type gotDiag struct {
	Code diagnostic.Code
	Unit string
	Pos  string
	Msg  string
}

func project(t *testing.T, ds []diagnostic.Diagnostic) []gotDiag {
	t.Helper()
	var out []gotDiag
	for _, d := range ds {
		if d.Severity() != diagnostic.SeverityError {
			t.Errorf("severity = %v, want error", d.Severity())
		}
		u, ok := d.Unit()
		if !ok {
			t.Errorf("diagnostic %q has no unit", d.Message())
		}
		out = append(out, gotDiag{Code: d.Code(), Unit: u.String(), Pos: d.Pos().String(), Msg: d.Message()})
	}
	return out
}

// mustUnit builds a module-unknown unit (no module needed in the graph)
// carrying deps and path deps.
func mustUnit(t *testing.T, path string, deps []repograph.Dependency, pds []repograph.PathDependency) repograph.Unit {
	t.Helper()
	u, err := repograph.NewModuleUnknownUnit(repograph.MustRepoPath(path), "source-remote", deps, nil)
	if err != nil {
		t.Fatalf("NewModuleUnknownUnit: %v", err)
	}
	if pds != nil {
		u, err = u.WithPathDependencies(pds)
		if err != nil {
			t.Fatalf("WithPathDependencies: %v", err)
		}
	}
	return u
}

func mustDep(t *testing.T, name, target string, pos repograph.Position, state repograph.TargetState, opts repograph.DependencyOptions) repograph.Dependency {
	t.Helper()
	d, err := repograph.NewDependency(name, repograph.MustRepoPath(target), pos, pos, state, opts)
	if err != nil {
		t.Fatalf("NewDependency: %v", err)
	}
	return d
}

func mustPathDep(t *testing.T, target, literal string, pos repograph.Position, state repograph.TargetState) repograph.PathDependency {
	t.Helper()
	pd, err := repograph.NewPathDependency(repograph.MustRepoPath(target), literal, pos, state)
	if err != nil {
		t.Fatalf("NewPathDependency: %v", err)
	}
	return pd
}

func TestMissingTargets(t *testing.T) {
	file := repograph.MustRepoPath("live/app/terragrunt.hcl")
	pos := mustPos(t, file, 3, 17)
	pos2 := mustPos(t, file, 9, 12)
	def := withOpts(nil)

	const (
		blockMissing  = `dependency "vpc" config_path resolves to "live/vpc": directory does not exist`
		blockNoConfig = `dependency "vpc" config_path resolves to "live/vpc": directory has no terragrunt.hcl`
		pathMissing   = `dependencies path "../x" resolves to "live/x": directory does not exist`
		pathNoConfig  = `dependencies path "../x" resolves to "live/x": directory has no terragrunt.hcl`
	)

	unresolved := func(t *testing.T) repograph.Dependency {
		d, err := repograph.NewUnresolvedDependency("vpc", "config-path-not-literal", pos, pos, def)
		if err != nil {
			t.Fatalf("NewUnresolvedDependency: %v", err)
		}
		return d
	}
	unresolvedPath := func(t *testing.T) repograph.PathDependency {
		pd, err := repograph.NewUnresolvedPathDependency("config-path-not-literal", pos)
		if err != nil {
			t.Fatalf("NewUnresolvedPathDependency: %v", err)
		}
		return pd
	}

	tests := []struct {
		name  string
		units func(t *testing.T) []repograph.Unit
		want  []gotDiag
	}{
		{"unresolved block silent", func(t *testing.T) []repograph.Unit {
			return []repograph.Unit{mustUnit(t, "live/app", []repograph.Dependency{unresolved(t)}, nil)}
		}, nil},
		{"target unknown silent", func(t *testing.T) []repograph.Unit {
			return []repograph.Unit{mustUnit(t, "live/app", []repograph.Dependency{mustDep(t, "vpc", "live/vpc", pos, repograph.TargetUnknown, def)}, nil)}
		}, nil},
		{"target has config silent", func(t *testing.T) []repograph.Unit {
			return []repograph.Unit{mustUnit(t, "live/app", []repograph.Dependency{mustDep(t, "vpc", "live/vpc", pos, repograph.TargetHasConfig, def)}, nil)}
		}, nil},
		{"enabled false silent", func(t *testing.T) []repograph.Unit {
			o := withOpts(func(o *repograph.DependencyOptions) { o.Enabled = repograph.TristateFalse })
			return []repograph.Unit{mustUnit(t, "live/app", []repograph.Dependency{mustDep(t, "vpc", "live/vpc", pos, repograph.TargetDirMissing, o)}, nil)}
		}, nil},
		{"enabled non-literal (unknown) silent", func(t *testing.T) []repograph.Unit {
			o := withOpts(func(o *repograph.DependencyOptions) { o.Enabled = repograph.TristateUnknown })
			return []repograph.Unit{mustUnit(t, "live/app", []repograph.Dependency{mustDep(t, "vpc", "live/vpc", pos, repograph.TargetDirMissing, o)}, nil)}
		}, nil},
		{"deep-merged label (all options unknown) silent", func(t *testing.T) []repograph.Unit {
			o := repograph.DependencyOptions{
				Enabled:             repograph.TristateUnknown,
				SkipOutputs:         repograph.TristateUnknown,
				MockOutputs:         repograph.UnknownNames(),
				MockMergeWithState:  repograph.TristateUnknown,
				MockAllowedCommands: repograph.UnknownNames(),
			}
			return []repograph.Unit{mustUnit(t, "live/app", []repograph.Dependency{mustDep(t, "vpc", "live/vpc", pos, repograph.TargetNoConfig, o)}, nil)}
		}, nil},
		{"absent enabled (default true), dir missing", func(t *testing.T) []repograph.Unit {
			return []repograph.Unit{mustUnit(t, "live/app", []repograph.Dependency{mustDep(t, "vpc", "live/vpc", pos, repograph.TargetDirMissing, def)}, nil)}
		}, []gotDiag{{diagnostic.CodeMissingDependencyTarget, "live/app", pos.String(), blockMissing}}},
		{"no config", func(t *testing.T) []repograph.Unit {
			return []repograph.Unit{mustUnit(t, "live/app", []repograph.Dependency{mustDep(t, "vpc", "live/vpc", pos, repograph.TargetNoConfig, def)}, nil)}
		}, []gotDiag{{diagnostic.CodeMissingDependencyTarget, "live/app", pos.String(), blockNoConfig}}},
		{"skip_outputs and mock_outputs do not gate", func(t *testing.T) []repograph.Unit {
			o := withOpts(func(o *repograph.DependencyOptions) {
				o.SkipOutputs = repograph.TristateTrue
				o.MockOutputs = mustNames(t, "vpc_id")
			})
			return []repograph.Unit{mustUnit(t, "live/app", []repograph.Dependency{mustDep(t, "vpc", "live/vpc", pos, repograph.TargetDirMissing, o)}, nil)}
		}, []gotDiag{{diagnostic.CodeMissingDependencyTarget, "live/app", pos.String(), blockMissing}}},
		{"path dep missing", func(t *testing.T) []repograph.Unit {
			return []repograph.Unit{mustUnit(t, "live/app", nil, []repograph.PathDependency{mustPathDep(t, "live/x", "../x", pos, repograph.TargetDirMissing)})}
		}, []gotDiag{{diagnostic.CodeMissingDependencyTarget, "live/app", pos.String(), pathMissing}}},
		{"path dep no config", func(t *testing.T) []repograph.Unit {
			return []repograph.Unit{mustUnit(t, "live/app", nil, []repograph.PathDependency{mustPathDep(t, "live/x", "../x", pos, repograph.TargetNoConfig)})}
		}, []gotDiag{{diagnostic.CodeMissingDependencyTarget, "live/app", pos.String(), pathNoConfig}}},
		{"path dep has config / unknown silent", func(t *testing.T) []repograph.Unit {
			return []repograph.Unit{mustUnit(t, "live/app", nil, []repograph.PathDependency{
				mustPathDep(t, "live/x", "../x", pos, repograph.TargetHasConfig),
				mustPathDep(t, "live/y", "../y", pos2, repograph.TargetUnknown),
			})}
		}, nil},
		{"unresolved path dep silent", func(t *testing.T) []repograph.Unit {
			return []repograph.Unit{mustUnit(t, "live/app", nil, []repograph.PathDependency{unresolvedPath(t)})}
		}, nil},
		{"block and paths naming same missing target both fire, block first", func(t *testing.T) []repograph.Unit {
			return []repograph.Unit{mustUnit(t, "live/app",
				[]repograph.Dependency{mustDep(t, "vpc", "live/vpc", pos2, repograph.TargetDirMissing, def)},
				[]repograph.PathDependency{mustPathDep(t, "live/vpc", "../vpc", pos, repograph.TargetDirMissing)})}
		}, []gotDiag{
			{diagnostic.CodeMissingDependencyTarget, "live/app", pos2.String(), blockMissing},
			{diagnostic.CodeMissingDependencyTarget, "live/app", pos.String(), `dependencies path "../vpc" resolves to "live/vpc": directory does not exist`},
		}},
		{"shared include: one diagnostic per unit", func(t *testing.T) []repograph.Unit {
			shared := mustPos(t, repograph.MustRepoPath("_env/vpc.hcl"), 2, 17)
			return []repograph.Unit{
				mustUnit(t, "live/b", []repograph.Dependency{mustDep(t, "vpc", "live/vpc", shared, repograph.TargetDirMissing, def)}, nil),
				mustUnit(t, "live/a", []repograph.Dependency{mustDep(t, "vpc", "live/vpc", shared, repograph.TargetDirMissing, def)}, nil),
			}
		}, []gotDiag{
			{diagnostic.CodeMissingDependencyTarget, "live/a", "_env/vpc.hcl:2:17", blockMissing},
			{diagnostic.CodeMissingDependencyTarget, "live/b", "_env/vpc.hcl:2:17", blockMissing},
		}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			g := mustGraph(t, tc.units(t), nil)
			ds, err := analysis.MissingTargets(g)
			if err != nil {
				t.Fatalf("MissingTargets: %v", err)
			}
			if tc.want == nil && ds != nil {
				t.Errorf("want nil slice, got %#v", ds)
			}
			if got := project(t, ds); !reflect.DeepEqual(got, tc.want) {
				t.Errorf("got  %#v\nwant %#v", got, tc.want)
			}
		})
	}
}
