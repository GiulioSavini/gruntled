package sourceresolve_test

import (
	"testing"

	"github.com/GiulioSavini/gruntled/internal/infrastructure/sourceresolve"
)

func TestClassify(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want sourceresolve.Source
	}{
		{"empty", "", sourceresolve.Source{Kind: sourceresolve.KindInvalid}},
		{"whitespace only", "   ", sourceresolve.Source{Kind: sourceresolve.KindInvalid}},

		{"forced git getter with query", "git::https://example.com/m.git//vpc?ref=v1", sourceresolve.Source{Kind: sourceresolve.KindRemote}},
		{"forced s3 getter", "s3::https://s3.amazonaws.com/b/m.zip", sourceresolve.Source{Kind: sourceresolve.KindRemote}},
		{"forced hg getter", "hg::http://x", sourceresolve.Source{Kind: sourceresolve.KindRemote}},
		{"forced gcs getter", "gcs::https://www.googleapis.com/storage/v1/b/m", sourceresolve.Source{Kind: sourceresolve.KindRemote}},

		{"git scheme", "git://h/r.git", sourceresolve.Source{Kind: sourceresolve.KindRemote}},
		{"ssh scheme", "ssh://git@h/r.git", sourceresolve.Source{Kind: sourceresolve.KindRemote}},
		{"https scheme", "https://h/m.zip", sourceresolve.Source{Kind: sourceresolve.KindRemote}},
		{"http scheme", "http://h/m.zip", sourceresolve.Source{Kind: sourceresolve.KindRemote}},
		{"tfr scheme", "tfr:///terraform-aws-modules/vpc/aws?version=5.0.0", sourceresolve.Source{Kind: sourceresolve.KindRemote}},
		{"oci scheme", "oci://registry/m", sourceresolve.Source{Kind: sourceresolve.KindRemote}},

		{"file scheme is invalid", "file:///opt/modules/vpc", sourceresolve.Source{Kind: sourceresolve.KindInvalid}},

		{"github.com shorthand", "github.com/org/repo//vpc?ref=v1", sourceresolve.Source{Kind: sourceresolve.KindRemote}},
		{"gitlab.com shorthand", "gitlab.com/org/repo", sourceresolve.Source{Kind: sourceresolve.KindRemote}},
		{"bitbucket.org shorthand", "bitbucket.org/org/repo", sourceresolve.Source{Kind: sourceresolve.KindRemote}},
		{"scp-like shorthand", "git@github.com:org/repo.git//vpc", sourceresolve.Source{Kind: sourceresolve.KindRemote}},
		{"amazonaws.com shorthand", "bucket.s3.amazonaws.com/m.zip", sourceresolve.Source{Kind: sourceresolve.KindRemote}},
		{"googleapis.com shorthand", "www.googleapis.com/storage/v1/b/m.zip", sourceresolve.Source{Kind: sourceresolve.KindRemote}},

		{"query on local path is invalid", "../modules/vpc?ref=v1", sourceresolve.Source{Kind: sourceresolve.KindInvalid}},
		{"backslash is invalid", `C:\modules\vpc`, sourceresolve.Source{Kind: sourceresolve.KindInvalid}},

		{"local with subdir", "../../aws//lambda", sourceresolve.Source{Kind: sourceresolve.KindLocal, Root: "../../aws", Subdir: "lambda"}},
		{"local without subdir", "units/chicken", sourceresolve.Source{Kind: sourceresolve.KindLocal, Root: "units/chicken", Subdir: ""}},
		{"registry-looking string is local", "terraform-aws-modules/vpc/aws", sourceresolve.Source{Kind: sourceresolve.KindLocal, Root: "terraform-aws-modules/vpc/aws", Subdir: ""}},
		{"dot", ".", sourceresolve.Source{Kind: sourceresolve.KindLocal, Root: ".", Subdir: ""}},
		{"dot slash", "./", sourceresolve.Source{Kind: sourceresolve.KindLocal, Root: "./", Subdir: ""}},
		{"dot double slash", ".//", sourceresolve.Source{Kind: sourceresolve.KindLocal, Root: ".", Subdir: ""}},
		{"virtual root prefix", "/__gruntled_repo_root__/modules//vpc", sourceresolve.Source{Kind: sourceresolve.KindLocal, Root: "/__gruntled_repo_root__/modules", Subdir: "vpc"}},
		{"empty root is invalid", "//vpc", sourceresolve.Source{Kind: sourceresolve.KindInvalid}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := sourceresolve.Classify(tt.raw)
			if got != tt.want {
				t.Fatalf("Classify(%q) = %+v, want %+v", tt.raw, got, tt.want)
			}
		})
	}
}
