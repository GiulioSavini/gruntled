package presenter_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/GiulioSavini/gruntled/internal/domain/repograph"
	"github.com/GiulioSavini/gruntled/internal/interfaces/presenter"
)

const graphEmptyGolden = `{
  "version": 1,
  "kind": "graph",
  "units": [],
  "modules": [],
  "edges": [],
  "unresolved_dependencies": [],
  "summary": {
    "units": 0,
    "resolved": 0,
    "module_unknown": 0,
    "config_unknown": 0,
    "unknown_modules": 0
  }
}
`

// graphMixed holds a resolved unit with a block edge, a paths edge, an
// unresolved block dependency, an unresolved paths entry and a reference;
// a config-unknown unit; a module-unknown unit; the edge target unit; one
// known module and one unknown module.
func graphMixed(t *testing.T) *repograph.RepositoryGraph {
	t.Helper()
	const app = "live/app/terragrunt.hcl"
	surface, err := repograph.NewSurface([]string{"cidr"}, []string{"vpc_id"})
	if err != nil {
		t.Fatalf("NewSurface: %v", err)
	}
	vpcMod, err := repograph.NewModule(rp(t, "modules/vpc"), surface)
	if err != nil {
		t.Fatalf("NewModule: %v", err)
	}
	emptyMod, err := repograph.NewUnknownModule(rp(t, "modules/empty"), reasonNoTerraformFiles)
	if err != nil {
		t.Fatalf("NewUnknownModule: %v", err)
	}
	opts := repograph.DefaultDependencyOptions()
	depVPC, err := repograph.NewDependency("vpc", rp(t, "live/vpc"), pos(t, app, 1, 1), pos(t, app, 2, 17), repograph.TargetHasConfig, opts)
	if err != nil {
		t.Fatalf("NewDependency: %v", err)
	}
	depDyn, err := repograph.NewUnresolvedDependency("dyn", "config-path-dynamic", pos(t, app, 3, 1), pos(t, app, 4, 17), opts)
	if err != nil {
		t.Fatalf("NewUnresolvedDependency: %v", err)
	}
	ref, err := repograph.NewReference("vpc", "vpc_id", pos(t, app, 10, 9))
	if err != nil {
		t.Fatalf("NewReference: %v", err)
	}
	appUnit, err := repograph.NewResolvedUnit(rp(t, "live/app"), rp(t, "modules/vpc"), []repograph.Dependency{depVPC, depDyn}, []repograph.Reference{ref})
	if err != nil {
		t.Fatalf("NewResolvedUnit: %v", err)
	}
	pdNet, err := repograph.NewPathDependency(rp(t, "live/net"), "../net", pos(t, app, 6, 14), repograph.TargetNoConfig)
	if err != nil {
		t.Fatalf("NewPathDependency: %v", err)
	}
	pdDyn, err := repograph.NewUnresolvedPathDependency("paths-dynamic", pos(t, app, 6, 25))
	if err != nil {
		t.Fatalf("NewUnresolvedPathDependency: %v", err)
	}
	if appUnit, err = appUnit.WithPathDependencies([]repograph.PathDependency{pdDyn, pdNet}); err != nil {
		t.Fatalf("WithPathDependencies: %v", err)
	}
	vpcUnit, err := repograph.NewResolvedUnit(rp(t, "live/vpc"), rp(t, "modules/vpc"), nil, nil)
	if err != nil {
		t.Fatalf("NewResolvedUnit: %v", err)
	}
	broken, err := repograph.NewConfigUnknownUnit(rp(t, "live/broken"), reasonSyntaxError)
	if err != nil {
		t.Fatalf("NewConfigUnknownUnit: %v", err)
	}
	remote, err := repograph.NewModuleUnknownUnit(rp(t, "live/remote"), reasonRemoteSource, nil, nil)
	if err != nil {
		t.Fatalf("NewModuleUnknownUnit: %v", err)
	}
	g, err := repograph.NewRepositoryGraph(
		[]repograph.Unit{vpcUnit, remote, broken, appUnit},
		[]repograph.Module{vpcMod, emptyMod},
	)
	if err != nil {
		t.Fatalf("NewRepositoryGraph: %v", err)
	}
	return g
}

const graphMixedGolden = `{
  "version": 1,
  "kind": "graph",
  "units": [
    {
      "path": "live/app",
      "status": "resolved",
      "module": "modules/vpc",
      "references": [
        {
          "dependency": "vpc",
          "output": "vpc_id",
          "position": {
            "file": "live/app/terragrunt.hcl",
            "line": 10,
            "column": 9
          }
        }
      ]
    },
    {
      "path": "live/broken",
      "status": "config-unknown",
      "reason": "syntax-error",
      "references": []
    },
    {
      "path": "live/remote",
      "status": "module-unknown",
      "reason": "remote-source",
      "references": []
    },
    {
      "path": "live/vpc",
      "status": "resolved",
      "module": "modules/vpc",
      "references": []
    }
  ],
  "modules": [
    {
      "path": "modules/empty",
      "known": false,
      "variables": [],
      "outputs": [],
      "reason": "no-terraform-files"
    },
    {
      "path": "modules/vpc",
      "known": true,
      "variables": [
        "cidr"
      ],
      "outputs": [
        "vpc_id"
      ]
    }
  ],
  "edges": [
    {
      "kind": "block",
      "from": "live/app",
      "to": "live/vpc",
      "position": {
        "file": "live/app/terragrunt.hcl",
        "line": 2,
        "column": 17
      },
      "name": "vpc",
      "target_state": "has-config",
      "enabled": "true",
      "skip_outputs": "false"
    },
    {
      "kind": "paths",
      "from": "live/app",
      "to": "live/net",
      "position": {
        "file": "live/app/terragrunt.hcl",
        "line": 6,
        "column": 14
      },
      "target_state": "no-config",
      "enabled": "true",
      "skip_outputs": "false"
    }
  ],
  "unresolved_dependencies": [
    {
      "from": "live/app",
      "kind": "block",
      "name": "dyn",
      "position": {
        "file": "live/app/terragrunt.hcl",
        "line": 4,
        "column": 17
      },
      "reason": "config-path-dynamic"
    },
    {
      "from": "live/app",
      "kind": "paths",
      "position": {
        "file": "live/app/terragrunt.hcl",
        "line": 6,
        "column": 25
      },
      "reason": "paths-dynamic"
    }
  ],
  "summary": {
    "units": 4,
    "resolved": 2,
    "module_unknown": 1,
    "config_unknown": 1,
    "unknown_modules": 1
  }
}
`

func renderGraph(t *testing.T, g *repograph.RepositoryGraph) string {
	t.Helper()
	var b strings.Builder
	if err := presenter.Graph(&b, g); err != nil {
		t.Fatalf("Graph: %v", err)
	}
	return b.String()
}

func TestGraphEmpty(t *testing.T) {
	if got := renderGraph(t, emptyGraph(t)); got != graphEmptyGolden {
		t.Errorf("Graph(empty) =\n%s\nwant\n%s", got, graphEmptyGolden)
	}
}

func TestGraphMixedGolden(t *testing.T) {
	got := renderGraph(t, graphMixed(t))
	if got != graphMixedGolden {
		t.Errorf("Graph(mixed) =\n%s\nwant\n%s", got, graphMixedGolden)
	}
	if strings.Contains(got, "null") {
		t.Errorf("Graph(mixed) contains null:\n%s", got)
	}
}

func TestGraphNoNullWhenEmpty(t *testing.T) {
	if got := renderGraph(t, emptyGraph(t)); strings.Contains(got, "null") {
		t.Errorf("Graph(empty) contains null:\n%s", got)
	}
}

func TestGraphDoesNotEscapeHTML(t *testing.T) {
	g, err := repograph.NewRepositoryGraph(nil, []repograph.Module{mustUnknownModule(t, "modules/a&b", "x<y>")})
	if err != nil {
		t.Fatalf("NewRepositoryGraph: %v", err)
	}
	got := renderGraph(t, g)
	if !strings.Contains(got, `"modules/a&b"`) || !strings.Contains(got, `"x<y>"`) {
		t.Errorf("Graph escaped HTML characters:\n%s", got)
	}
}

func mustUnknownModule(t *testing.T, path, reason string) repograph.Module {
	t.Helper()
	m, err := repograph.NewUnknownModule(rp(t, path), reason)
	if err != nil {
		t.Fatalf("NewUnknownModule: %v", err)
	}
	return m
}

func TestGraphDeterministic(t *testing.T) {
	g := graphMixed(t)
	first := renderGraph(t, g)
	for i := 0; i < 5; i++ {
		if got := renderGraph(t, graphMixed(t)); got != first {
			t.Fatalf("run %d differs:\n%s\nfirst\n%s", i, got, first)
		}
	}
}

func TestGraphWriterErrorReturned(t *testing.T) {
	want := errors.New("boom")
	if err := presenter.Graph(failingWriter{err: want}, graphMixed(t)); !errors.Is(err, want) {
		t.Errorf("Graph(failing writer) = %v, want %v", err, want)
	}
}
