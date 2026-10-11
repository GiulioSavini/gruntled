package main

import (
	"bytes"
	"strings"
	"testing"
	"time"
)

// TestCheckJSONTypeBomb: a module .tf.json whose variable type string nests
// 400,000 list( levels (2.4 MB, under MaxFileBytes) crashed typeexpr with a
// fatal stack overflow before the 15-02 guards (sec #245). check on a repo
// using that module must behave exactly as without the file's type: the
// same exit code and the same GRT001 for the misspelled output reference.
// The hostile file is built here, never committed.
func TestCheckJSONTypeBomb(t *testing.T) {
	bomb := `{"variable":{"deep":{"type":"` + strings.Repeat("list(", 400_000) + "string" + strings.Repeat(")", 400_000) + `"}}}`
	for _, format := range []string{"text", "json"} {
		t.Run(format, func(t *testing.T) {
			plain := repo(t, "vpc_idd")
			hostile := repo(t, "vpc_idd")
			writeFiles(t, hostile, map[string]string{"vpc/vars.tf.json": bomb})
			writeFiles(t, plain, map[string]string{"vpc/vars.tf.json": `{"variable":{"deep":{}}}`})

			var want, wantErr bytes.Buffer
			wantCode := run([]string{"check", "--format", format, plain}, &want, &wantErr)
			var got, gotErr bytes.Buffer
			start := time.Now()
			code := run([]string{"check", "--format", format, hostile}, &got, &gotErr)
			t.Logf("check on the hostile repo took %v", time.Since(start))
			if wantCode != 1 || code != wantCode {
				t.Fatalf("exit %d, want %d (plain repo exit %d)\nstderr:\n%s", code, wantCode, wantCode, gotErr.String())
			}
			if !strings.Contains(want.String(), "GRT001") {
				t.Fatalf("plain repo has no GRT001:\n%s", want.String())
			}
			norm := func(s, dir string) string { return strings.ReplaceAll(s, dir, "<repo>") }
			if norm(got.String(), hostile) != norm(want.String(), plain) || gotErr.String() != wantErr.String() {
				t.Fatalf("output differs from the plain repo\n--- hostile:\n%s\n--- plain:\n%s", got.String(), want.String())
			}
		})
	}
}
