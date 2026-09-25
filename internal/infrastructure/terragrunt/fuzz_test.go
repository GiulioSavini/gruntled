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

// FuzzLoadUnits fuzzes the loader's per-unit terragrunt.hcl body against a
// fixed surrounding repository: a shared root.hcl, the fuzzed unit
// live/app (with a module that declares output "o"), and a plain
// live/vpc unit (with a module that declares output "id") a dependency
// block could point at. No input, however malformed, may panic, return a
// Go error (per-file and per-unit problems are never errors, only unknown
// reasons and diagnostics), name a file outside the fixture in a
// diagnostic, or produce a reference whose position falls outside its own
// file's actual bytes.
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
	for _, s := range seeds {
		f.Add(s)
	}

	f.Fuzz(func(t *testing.T, body string) {
		repo := fstest.MapFS{
			"root.hcl":                {Data: []byte("inputs = {}\n")},
			"live/app/terragrunt.hcl": {Data: []byte(body)},
			"live/app/main.tf": {Data: []byte(`output "o" {
  value = 1
}
`)},
			"live/vpc/terragrunt.hcl": {Data: []byte("")},
			"live/vpc/main.tf": {Data: []byte(`output "id" {
  value = 1
}
`)},
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
		lines := strings.Split(body, "\n")
		for _, ref := range appUnit.References() {
			pos := ref.Pos()
			if pos.File().String() != "live/app/terragrunt.hcl" {
				continue
			}
			if pos.Line() < 1 || pos.Line() > len(lines) {
				t.Fatalf("reference position out of range: %s (body has %d lines)", pos, len(lines))
			}
			lineLen := len(lines[pos.Line()-1])
			if pos.Column() < 1 || pos.Column() > lineLen+1 {
				t.Fatalf("reference column out of range: %s (line length %d)", pos, lineLen)
			}
		}
	})
}
