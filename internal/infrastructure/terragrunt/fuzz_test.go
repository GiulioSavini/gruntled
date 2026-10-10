package terragrunt

import (
	"context"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/GiulioSavini/gruntled/internal/application/indexing"
	"github.com/GiulioSavini/gruntled/internal/domain/repograph"
	"github.com/GiulioSavini/gruntled/internal/infrastructure/tfsurface"
)

// FuzzLoadUnits fuzzes two inputs against a fixed surrounding repository:
// body, the fuzzed unit live/app's own terragrunt.hcl (live/app has a
// module declaring output "o"), and shared, written to BOTH root.hcl and
// live/vpc/main.tf. live/vpc is a plain unit a dependency block can point
// at. The shared input reaches code the single-input target never did
// (02-REVIEW G10): include parsing and merging when body includes
// root.hcl through find_in_parent_folders("root.hcl"), dependencies and
// references declared in an include, and the module surface reader for
// the dependency target's main.tf.
//
// No input pair, however malformed, may panic, return a Go error
// (per-file and per-unit problems are never errors, only unknown reasons
// and diagnostics), name a file outside the fixture in a diagnostic, or
// produce a reference whose position falls outside its own file's actual
// bytes, whether that file is live/app/terragrunt.hcl or root.hcl.
func FuzzLoadUnits(f *testing.F) {
	seeds := []string{
		// A valid unit: include + dependency + an inputs reference.
		`include "root" {
  path = find_in_parent_folders()
}

dependency "vpc" {
  config_path = "../vpc"
}

inputs = {
  id = dependency.vpc.outputs.id
}
`,
		// Mid-edit: unterminated block.
		`locals {`,
		// 1,100 nested template directives (sec #261): refused by the
		// native depth pre-scan, never evaluated.
		"dependency \"vpc\" {\n  config_path = \"" + strings.Repeat("%{if a}", 1100) + "../vpc" + strings.Repeat("%{endif}", 1100) + "\"\n}\n",
		// Trailing partial traversal.
		`inputs = {
  x = dependency.vpc.outputs.
}
`,
		// Non-UTF-8 bytes.
		"\xff\xfe",
		// Unterminated heredoc.
		`generate "x" {
  path      = "x.tf"
  if_exists = "overwrite"
  contents  = <<-EOF
    unterminated
`,
		// Duplicate bare includes.
		`include "a" {}
include "a" {}
`,
		// Expansion block (Terragrunt v1.2.0).
		`dependency "vpc" {
  config_path = "../vpc"
  expansion {
    for_each = ["a"]
  }
}
`,
		// Deep include with a dependency.
		`include "root" {
  path           = find_in_parent_folders()
  merge_strategy = "deep"
}
`,
		// A function outside the six PARSE-02 functions in a path attribute.
		`terraform {
  source = run_cmd("x")
}
`,
		// A 10 KB line of a 2-byte-per-rune character.
		"locals {\n  a = \"" + strings.Repeat("é", 10*1024) + "\"\n}\n",
		// Empty input.
		"",
		// A bare interpolation opener.
		"${",

		// Eight real-world fixtures mined from denis256/terragrunt-tests
		// (research Pattern 8's recommended fuzz seed source), fetched at
		// authoring time and inlined so the test has no network dependency.

		// broken-dependencies/app/terragrunt.hcl
		`dependency "dependency" {
  config_path = "../dependency"

  mock_outputs = {
    test = "value"
  }

}

dependency "dependency2" {
  config_path = "../dependency2"

  mock_outputs = {
    test = "value"
  }

}
`,
		// broken-dependencies/dependency/terragrunt.hcl (empty)
		"",
		// broken-dependencies/dependency2/terragrunt.hcl (empty)
		"",
		// include-error/terragrunt.hcl: a malformed "dynamic" expression
		// inside locals makes this genuinely invalid HCL.
		`locals {
  source_base_url = "test"
  common_vars = read_terragrunt_config(find_in_parent_folders("common.hcl"))
  name_prefix = local.common_vars.locals.name_prefix
  name = "${basename(get_original_terragrunt_dir())}"
  account_id = basename(local.common_vars.locals.map1["key1"])
  qwe = local.common_vars.locals.x * 10000
  ami_id_map = {
    "us-west-2" = "123"
  }

  region_vars = read_terragrunt_config("test.hcl")


  region = "us-east-1"

  foo = [
    merge(
      { region = local.region },
      { qwe = 1 },
    )
  ]

  x = 1
  y = 0
  z = local.x * local.y

  qwe1 = dynamic  qwe {
    for_each = local.foo
    content = {
      region = local.region
      qwe = 1
    }
  }

  }

}

inputs = {
  cluster_version = "1.23"
}
`,
		// include-error/common.hcl
		`locals {
  name_prefix = "test"
  map1 = {
    "key1" : "1234"
  }
  x = 1
  y = ""
  n = {
    x : 1
    y : "2"
  }
  xyz  = ["1", 2, 3]
  one  = 1
  zero = 0
  p = {
    type : "boolean"
    value = "2"
  }



}
`,
		// include-error/app/terragrunt.hcl
		`include "root" {
  path = find_in_parent_folders()
}

locals {
  admin_sso_role_name = "1233"
}
`,
		// encryption/terragrunt.hcl
		`remote_state {
  backend = "s3"
  generate = {
    path      = "backend.tf"
    if_exists = "overwrite"
  }
  config = {
    encrypt = true
    bucket = "test-s3-test-tg-123-2024"
    key = "terraform.tfstate"
    region = "us-west-2"

    encryption = {
      key_provider "pbkdf2" "my_passphrase" {
      ## Enter a passphrase here:
      passphrase = ""
      }
    }

  }
}
`,
	}
	// defaultShared is the shared input every single-input seed above ran
	// with before G10: valid as root.hcl, and valid in main.tf too (a bare
	// attribute, which the module surface reader ignores).
	const defaultShared = "inputs = {}\n"
	for _, s := range seeds {
		f.Add(s, defaultShared)
	}

	// Seed pairs whose body always reaches root.hcl (and so the shared
	// input) through an include, depends on live/vpc (whose main.tf is
	// the shared input too) and references it.
	const sharedBody = `include "root" {
  path = find_in_parent_folders("root.hcl")
}

dependency "vpc" {
  config_path = "../vpc"
}

inputs = {
  id = dependency.vpc.outputs.id
}
`
	sharedSeeds := []string{
		// A valid module; root.hcl with an output block is still valid HCL.
		"output \"id\" {\n  value = 1\n}\n",
		// An include-declared dependency and reference.
		"dependency \"db\" {\n  config_path = \"../db\"\n}\ninputs = { x = dependency.db.outputs.y }\n",
		// Mid-edit syntax error in a shared file: GRT100 for root.hcl and
		// for live/vpc/main.tf.
		"locals {",
		// Over the nesting limit, yet only about 4 KB.
		"x = " + strings.Repeat("(", 2000) + "1" + strings.Repeat(")", 2000) + "\n",
		// A nested include.
		"include \"x\" {\n  path = \"other.hcl\"\n}\n",
		// Non-UTF-8 bytes.
		"\xff\xfe",
		// A generate block whose contents declare an output.
		"generate \"g\" {\n  path = \"o.tf\"\n  contents = \"output \\\"z\\\" {}\"\n}\n",
	}
	for _, sh := range sharedSeeds {
		f.Add(sharedBody, sh)
	}

	f.Fuzz(func(t *testing.T, body, shared string) {
		repo := fstest.MapFS{
			"root.hcl":                {Data: []byte(shared)},
			"live/app/terragrunt.hcl": {Data: []byte(body)},
			"live/app/main.tf": {Data: []byte(`output "o" {
  value = 1
}
`)},
			"live/vpc/terragrunt.hcl": {Data: []byte("")},
			"live/vpc/main.tf":        {Data: []byte(shared)},
		}

		res, err := indexing.Build(context.Background(), NewLoader(repo), tfsurface.NewReader(repo))
		if err != nil {
			t.Fatalf("indexing.Build returned an error for a per-file problem: %v", err)
		}

		known := make(map[string]bool, len(repo))
		for name := range repo {
			known[name] = true
		}
		for _, d := range res.Diagnostics.All() {
			file := d.Pos().File().String()
			if !known[file] {
				t.Fatalf("diagnostic references a file not in the fixture: %q", file)
			}
		}

		appUnit, ok := res.Graph.Unit(repograph.MustRepoPath("live/app"))
		if !ok || appUnit.Status() == repograph.StatusConfigUnknown {
			return
		}
		files := map[string][]string{
			"live/app/terragrunt.hcl": strings.Split(body, "\n"),
			"root.hcl":                strings.Split(shared, "\n"),
		}
		for _, ref := range appUnit.References() {
			pos := ref.Pos()
			lines, ok := files[pos.File().String()]
			if !ok {
				continue
			}
			if pos.Line() < 1 || pos.Line() > len(lines) {
				t.Fatalf("reference position out of range: %s (file has %d lines)", pos, len(lines))
			}
			lineLen := len(lines[pos.Line()-1])
			if pos.Column() < 1 || pos.Column() > lineLen+1 {
				t.Fatalf("reference column out of range: %s (line length %d)", pos, lineLen)
			}
		}
	})
}
