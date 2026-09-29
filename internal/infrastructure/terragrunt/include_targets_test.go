package terragrunt

import (
	"io/fs"
	"testing"
	"testing/fstest"
)

// TestCanonicalPath pins canonicalPath over fstest.MapFS symlink entries:
// in-repo links (directory, file, chain) resolve to the real file, a link
// escaping the repository is canonOutside, and anything that cannot be
// resolved with certainty (a loop, a backslash target, an fs.FS without
// fs.ReadLinkFS) is canonUnresolvable (02-REVIEW G15).
func TestCanonicalPath(t *testing.T) {
	parent := &fstest.MapFile{Data: []byte("")}
	cases := []struct {
		name     string
		fsys     fs.FS
		in       string
		wantPath string
		wantErr  canonErr
	}{
		{
			name:     "plain file",
			fsys:     fstest.MapFS{"live/parent/terragrunt.hcl": parent},
			in:       "live/parent/terragrunt.hcl",
			wantPath: "live/parent/terragrunt.hcl",
			wantErr:  canonOK,
		},
		{
			name: "directory link",
			fsys: fstest.MapFS{
				"live/parent/terragrunt.hcl": parent,
				"live/link":                  symlinkFile("parent"),
			},
			in:       "live/link/terragrunt.hcl",
			wantPath: "live/parent/terragrunt.hcl",
			wantErr:  canonOK,
		},
		{
			name: "file link",
			fsys: fstest.MapFS{
				"live/parent/terragrunt.hcl": parent,
				"live/alias.hcl":             symlinkFile("parent/terragrunt.hcl"),
			},
			in:       "live/alias.hcl",
			wantPath: "live/parent/terragrunt.hcl",
			wantErr:  canonOK,
		},
		{
			name: "chain of links",
			fsys: fstest.MapFS{
				"live/parent/terragrunt.hcl": parent,
				"a":                          symlinkFile("b"),
				"b":                          symlinkFile("live/parent"),
			},
			in:       "a/terragrunt.hcl",
			wantPath: "live/parent/terragrunt.hcl",
			wantErr:  canonOK,
		},
		{
			name: "link with dot-dot inside the repo",
			fsys: fstest.MapFS{
				"live/parent/terragrunt.hcl": parent,
				"other/deep/link":            symlinkFile("../../live/parent"),
			},
			in:       "other/deep/link/terragrunt.hcl",
			wantPath: "live/parent/terragrunt.hcl",
			wantErr:  canonOK,
		},
		{
			name: "link escaping the repo",
			fsys: fstest.MapFS{
				"live/link": symlinkFile("../../outside"),
			},
			in:      "live/link/terragrunt.hcl",
			wantErr: canonOutside,
		},
		{
			name: "absolute link target",
			fsys: fstest.MapFS{
				"live/link": symlinkFile("/etc"),
			},
			in:      "live/link/terragrunt.hcl",
			wantErr: canonOutside,
		},
		{
			name: "link loop",
			fsys: fstest.MapFS{
				"x": symlinkFile("y"),
				"y": symlinkFile("x"),
			},
			in:      "x/terragrunt.hcl",
			wantErr: canonUnresolvable,
		},
		{
			name: "backslash in link target",
			fsys: fstest.MapFS{
				"live/parent/terragrunt.hcl": parent,
				"live/link":                  symlinkFile(`..\live\parent`),
			},
			in:      "live/link/terragrunt.hcl",
			wantErr: canonUnresolvable,
		},
		{
			name: "missing path segment",
			fsys: fstest.MapFS{
				"live/parent/terragrunt.hcl": parent,
			},
			in:      "live/nope/terragrunt.hcl",
			wantErr: canonUnresolvable,
		},
		{
			name: "fs without ReadLinkFS",
			fsys: struct{ fs.FS }{fstest.MapFS{
				"live/parent/terragrunt.hcl": parent,
			}},
			in:      "live/parent/terragrunt.hcl",
			wantErr: canonUnresolvable,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := canonicalPath(tc.fsys, tc.in)
			if err != tc.wantErr {
				t.Fatalf("canonicalPath(%q) err = %v, want %v (path %q)", tc.in, err, tc.wantErr, got)
			}
			if tc.wantErr == canonOK && got != tc.wantPath {
				t.Fatalf("canonicalPath(%q) = %q, want %q", tc.in, got, tc.wantPath)
			}
		})
	}
}
