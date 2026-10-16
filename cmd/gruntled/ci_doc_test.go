package main

// TestCIDoc keeps docs/ci.md, .github/workflows/ci.yml and
// .pre-commit-hooks.yaml in step: the action pins and command lines the doc
// recommends are the ones the recipe-check / sarif-upload jobs run, and the
// hook file has the locked fields. Plain string checks; go.mod has no YAML
// dependency and this test does not add one.

import (
	"os"
	"strings"
	"testing"
)

// ciPins are the action pins the recipes use: full SHA plus version comment.
var ciPins = []struct {
	action, sha, version, job string
}{
	{"actions/checkout", "3d3c42e5aac5ba805825da76410c181273ba90b1", "v7.0.1", "recipe-check"},
	{"actions/setup-go", "b7ad1dad31e06c5925ef5d2fc7ad053ef454303e", "v7.0.0", "recipe-check"},
	{"github/codeql-action/upload-sarif", "2892aa5e19bbd11bc0cff5427e3b750a04d9e3c2", "v4.38.2", "sarif-upload"},
}

func readRepoFile(t *testing.T, rel string) string {
	t.Helper()
	b, err := os.ReadFile("../../" + rel)
	if err != nil {
		t.Fatalf("read %s: %v", rel, err)
	}
	return string(b)
}

// workflowJob returns the text of one top-level job in a workflow file: from
// its "  <name>:" line up to the next job header at the same indent.
func workflowJob(wf, name string) string {
	lines := strings.Split(wf, "\n")
	start := -1
	for i, l := range lines {
		if l == "  "+name+":" {
			start = i
			break
		}
	}
	if start < 0 {
		return ""
	}
	end := len(lines)
	for i := start + 1; i < len(lines); i++ {
		l := lines[i]
		if strings.HasPrefix(l, "  ") && !strings.HasPrefix(l, "   ") && strings.HasSuffix(l, ":") && !strings.HasPrefix(l, "  #") {
			end = i
			break
		}
	}
	return strings.Join(lines[start:end], "\n")
}

func TestCIDoc(t *testing.T) {
	doc := readRepoFile(t, "docs/ci.md")
	wf := readRepoFile(t, ".github/workflows/ci.yml")
	hooks := readRepoFile(t, ".pre-commit-hooks.yaml")

	// The corpus suite (17-04) runs only in its own workflow, on demand and
	// weekly, with read-only permissions, no secrets, no persisted token and
	// no shared Go cache (sec #420); ci.yml never runs it.
	t.Run("corpus_workflow", func(t *testing.T) {
		cw := readRepoFile(t, ".github/workflows/corpus.yml")
		for _, s := range []string{
			"workflow_dispatch:",
			"schedule:",
			"- cron: ",
			"permissions:\n  contents: read",
			"timeout-minutes: 150",
			"persist-credentials: false",
			"fetch-depth: 0",
			"cache: false",
			"go-version-file: go.mod",
			`bash scripts/fetch-corpora.sh "$RUNNER_TEMP/corpus"`,
			`>> "$GITHUB_ENV"`,
			"CORPUS_GATE_NONET: \"1\"",
			"bash scripts/corpus-gate.sh",
		} {
			if !strings.Contains(cw, s) {
				t.Errorf("corpus.yml: missing %q", s)
			}
		}
		// ubuntu-latest's AppArmor blocks unprivileged user namespaces, so
		// the gate's `unshare -rn` needs this sysctl first (17-05 F2); the
		// gate still runs with CORPUS_GATE_NONET=1.
		const userns = "sudo sysctl -w kernel.apparmor_restrict_unprivileged_userns=0"
		job := workflowJob(cw, "corpus")
		step := workflowStep(job, "allow unprivileged user namespaces")
		if !strings.Contains(step, userns) {
			t.Errorf("corpus.yml: step %q must run %q", "allow unprivileged user namespaces", userns)
		}
		if i, j := strings.Index(job, userns), strings.Index(job, "bash scripts/corpus-gate.sh"); i < 0 || j < 0 || i > j {
			t.Errorf("corpus.yml: the userns sysctl must come before the gate step")
		}
		// The gate runs with GOPROXY=off and setup-go has cache: false, so the
		// module cache must be filled (networked) and verified against go.sum
		// before the gate (17-05 sec F1). go.mod/go.sum are identical at
		// v0.3.0 and HEAD, so one download covers both builds.
		dl := workflowStep(job, "download modules")
		di, vi := strings.Index(dl, "go mod download"), strings.Index(dl, "go mod verify")
		if di < 0 || vi < 0 || di > vi {
			t.Errorf("corpus.yml: step %q must run go mod download, then go mod verify:\n%s", "download modules", dl)
		}
		if i, j := strings.Index(job, "- name: download modules"), strings.Index(job, "bash scripts/corpus-gate.sh"); i < 0 || j < 0 || i > j {
			t.Errorf("corpus.yml: the module download must come before the gate step")
		}
		for _, p := range ciPins[:2] {
			if uses := "uses: " + p.action + "@" + p.sha + " # " + p.version; !strings.Contains(cw, uses) {
				t.Errorf("corpus.yml: missing %q (the same action version as ci.yml)", uses)
			}
		}
		for _, bad := range []string{"secrets.", "write", "upload-artifact", "pull_request", "push:"} {
			if strings.Contains(cw, bad) {
				t.Errorf("corpus.yml: must not contain %q", bad)
			}
		}
		if strings.Count(cw, "uses: ") != 2 {
			t.Errorf("corpus.yml: %d actions, want exactly checkout and setup-go", strings.Count(cw, "uses: "))
		}
		for _, bad := range []string{"GRUNTLED_CORPUS", "corpus-gate", "fetch-corpora"} {
			if strings.Contains(wf, bad) {
				t.Errorf("ci.yml must not run the corpus suite: contains %q", bad)
			}
		}
	})

	t.Run("pins", func(t *testing.T) {
		for _, p := range ciPins {
			uses := "uses: " + p.action + "@" + p.sha + " # " + p.version
			if !strings.Contains(doc, uses) {
				t.Errorf("docs/ci.md: missing %q", uses)
			}
			job := workflowJob(wf, p.job)
			if job == "" {
				t.Errorf("ci.yml: job %q not found", p.job)
				continue
			}
			if !strings.Contains(job, uses) {
				t.Errorf("ci.yml job %s: missing %q", p.job, uses)
			}
		}
	})

	t.Run("doc commands", func(t *testing.T) {
		for _, s := range []string{
			"gruntled check .",
			"gruntled check --format sarif .",
			"go install github.com/GiulioSavini/gruntled/cmd/gruntled@",
			"sha256sum -c --ignore-missing checksums.txt",
			"if: ${{ !cancelled() }}",
			"security-events: write",
			"image: golang:1.27",
			"$(go env GOPATH)/bin/gruntled check .",
		} {
			if !strings.Contains(doc, s) {
				t.Errorf("docs/ci.md: missing %q", s)
			}
		}
	})

	t.Run("recipe-check job", func(t *testing.T) {
		job := workflowJob(wf, "recipe-check")
		if job == "" {
			t.Fatal("ci.yml: job recipe-check not found")
		}
		for _, s := range []string{
			"go install ./cmd/gruntled",
			"gruntled check cmd/gruntled/testdata/clean-fixture",
			"gruntled check --format sarif cmd/gruntled/testdata/clean-fixture",
			"gruntled check cmd/gruntled/testdata/sarif-fixture",
			"pre-commit try-repo",
			"gruntled-check --all-files",
		} {
			if !strings.Contains(job, s) {
				t.Errorf("ci.yml job recipe-check: missing %q", s)
			}
		}
	})

	t.Run("fuzz step", func(t *testing.T) {
		step := workflowStep(workflowJob(wf, "check"), "fuzz FuzzTypeSig")
		if step == "" {
			t.Fatal("ci.yml job check: step \"fuzz FuzzTypeSig\" not found")
		}
		for _, s := range []string{
			"timeout-minutes: 5",
			"set -o pipefail",
			"-run '^$' -fuzz '^FuzzTypeSig$'",
			"-fuzztime 30s",
			"-fuzzminimizetime 200x",
			`tee "$log"`,
			`log="$RUNNER_TEMP/fuzz.log"`,
			`tail -n 5 "$log"`,
			`tail -n 200 "$log"`,
			"for f in internal/infrastructure/tfsurface/testdata/fuzz/FuzzTypeSig/*; do",
			`cat "$f"`,
		} {
			if !strings.Contains(step, s) {
				t.Errorf("ci.yml fuzz step: missing %q", s)
			}
		}
		for _, s := range []string{"secrets.", "permissions:"} {
			if strings.Contains(step, s) {
				t.Errorf("ci.yml fuzz step: unexpected %q", s)
			}
		}
	})

	t.Run("hook file", func(t *testing.T) {
		lines := map[string]bool{}
		for _, l := range strings.Split(hooks, "\n") {
			lines[strings.TrimSpace(l)] = true
			if strings.HasPrefix(strings.TrimSpace(l), "files:") {
				t.Errorf(".pre-commit-hooks.yaml: unexpected files: filter %q", l)
			}
		}
		for _, s := range []string{
			"- id: gruntled-check",
			"language: golang",
			"entry: gruntled check",
			"pass_filenames: false",
			"always_run: true",
		} {
			if !lines[s] {
				t.Errorf(".pre-commit-hooks.yaml: missing line %q", s)
			}
		}
	})

	t.Run("doc hook snippet", func(t *testing.T) {
		for _, s := range []string{
			"repo: https://github.com/GiulioSavini/gruntled",
			"id: gruntled-check",
		} {
			if !strings.Contains(doc, s) {
				t.Errorf("docs/ci.md: missing %q", s)
			}
		}
	})

	t.Run("release guard", func(t *testing.T) {
		rel := readRepoFile(t, ".github/workflows/release.yml")
		guard := workflowStep(workflowJob(rel, "release"), "guard")
		if guard == "" {
			t.Fatal("release.yml: job release has no step guard")
		}
		regex := `^v[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.-]+)?$`
		for _, s := range []string{
			regex,
			`want=$(tr -d '[:space:]' < VERSION)`,
			`if [ "$want" != "$GITHUB_REF_NAME" ]; then`,
			`VERSION is '$want' but the tag is '$GITHUB_REF_NAME'`,
			"merge-base --is-ancestor",
			// sec #489: bash [[ =~ ]] bracket ranges are ASCII only under the C locale.
			"LC_ALL: C",
		} {
			if !strings.Contains(guard, s) {
				t.Errorf("release.yml guard step: missing %q", s)
			}
		}
		// Cheapest first, nothing from the network before the local checks:
		// tag regex, then VERSION == tag, then the fetch and ancestor check.
		ir, iv, if_ := strings.Index(guard, regex), strings.Index(guard, "< VERSION"), strings.Index(guard, "git fetch origin master")
		if !(ir >= 0 && ir < iv && iv < if_) {
			t.Errorf("release.yml guard step order: regex at %d, VERSION check at %d, fetch at %d", ir, iv, if_)
		}
		if strings.Contains(guard, "${{") {
			t.Errorf("release.yml guard step must read the tag only from $GITHUB_REF_NAME")
		}
	})

	t.Run("no expression injection in run blocks", func(t *testing.T) {
		// sec #487: one scanner over both workflows. Any ${{ }} must be
		// outside run: blocks, and the expected non-run occurrences are
		// listed so the scanner is proven to see them.
		for _, c := range []struct {
			file    string
			outside []string
		}{
			{".github/workflows/release.yml", []string{"group: release-${{ github.ref }}", "GH_TOKEN: ${{ github.token }}"}},
			{".github/workflows/ci.yml", []string{"group: ci-${{ github.ref }}", "cancel-in-progress: ${{ github.event_name == 'pull_request' }}", "runs-on: ${{ matrix.os }}", "sarif_file: ${{ runner.temp }}/upload.sarif"}},
		} {
			text := readRepoFile(t, c.file)
			inRun, outRun := expressionsByContext(text)
			for _, l := range inRun {
				t.Errorf("%s: ${{ inside a run block: %q", c.file, l)
			}
			if len(outRun) != len(c.outside) {
				t.Errorf("%s: want %d non-run ${{ lines, got %d: %q", c.file, len(c.outside), len(outRun), outRun)
			}
			for i, want := range c.outside {
				if i < len(outRun) && strings.TrimSpace(outRun[i]) != want {
					t.Errorf("%s: non-run ${{ line %d = %q, want %q", c.file, i, strings.TrimSpace(outRun[i]), want)
				}
			}
		}
		// Non-vacuity: the scanner catches block and inline run lines.
		synthetic := "jobs:\n  j:\n    steps:\n      - name: a\n        run: |\n          echo ok\n          echo ${{ github.ref_name }}\n      - name: b\n        run: echo ${{ github.event.head_commit.message }}\n      - name: c\n        env:\n          X: ${{ github.ref }}\n        run: echo fine\n"
		inRun, outRun := expressionsByContext(synthetic)
		if len(inRun) != 2 || len(outRun) != 1 {
			t.Errorf("scanner on synthetic workflow: inRun=%q outRun=%q, want 2 and 1", inRun, outRun)
		}
	})

	t.Run("release step order", func(t *testing.T) {
		rel := readRepoFile(t, ".github/workflows/release.yml")
		got := stepNames(workflowJob(rel, "release"))
		want := []string{"guard", "proofs", "build", "smoke", "publish"}
		// Ordered subsequence: later plans may insert steps (18-03: notes
		// after guard) but never reorder or drop these.
		j := 0
		for _, g := range got {
			if j < len(want) && g == want[j] {
				j++
			}
		}
		if j != len(want) {
			t.Errorf("release.yml job release steps %q do not contain %q in order", got, want)
		}
		if len(got) == 0 || got[0] != "guard" {
			t.Errorf("release.yml: guard must be the first named step, steps are %q", got)
		}
	})
}

// expressionsByContext splits the lines of a workflow file that contain
// "${{" into those inside a run: block (a "run: |"/"run: >" block body, or an
// inline "run: ..." line) and those outside. Plain line scanning: a block body
// is every following line indented deeper than the run: key (blank lines
// included), and ends at the first line that is not.
func expressionsByContext(wf string) (inRun, outRun []string) {
	indentOf := func(l string) int { return len(l) - len(strings.TrimLeft(l, " ")) }
	lines := strings.Split(wf, "\n")
	for i := 0; i < len(lines); i++ {
		l := lines[i]
		tr := strings.TrimSpace(l)
		if !strings.HasPrefix(tr, "run:") && !strings.HasPrefix(tr, "- run:") {
			if strings.Contains(l, "${{") {
				outRun = append(outRun, l)
			}
			continue
		}
		if strings.Contains(l, "${{") {
			inRun = append(inRun, l)
		}
		val := strings.TrimSpace(strings.TrimPrefix(strings.TrimPrefix(tr, "- "), "run:"))
		if val != "|" && val != ">" && val != "|-" && val != ">-" {
			continue // inline run, already checked
		}
		base := indentOf(l)
		if strings.HasPrefix(tr, "- run:") {
			base += 2
		}
		for i+1 < len(lines) && (strings.TrimSpace(lines[i+1]) == "" || indentOf(lines[i+1]) > base) {
			i++
			if strings.Contains(lines[i], "${{") {
				inRun = append(inRun, lines[i])
			}
		}
	}
	return inRun, outRun
}

// stepNames lists the "- name:" values of a job's steps in order.
func stepNames(job string) []string {
	var names []string
	for _, l := range strings.Split(job, "\n") {
		if tr := strings.TrimLeft(l, " "); strings.HasPrefix(tr, "- name: ") {
			names = append(names, strings.TrimPrefix(tr, "- name: "))
		}
	}
	return names
}

// workflowStep returns the text of one step of a job: from its
// "- name: <name>" line up to the next "- name:" line at the same indent.
func workflowStep(job, name string) string {
	lines := strings.Split(job, "\n")
	start, indent := -1, ""
	for i, l := range lines {
		if t := strings.TrimLeft(l, " "); t == "- name: "+name {
			start, indent = i, l[:len(l)-len(t)]
			break
		}
	}
	if start < 0 {
		return ""
	}
	end := len(lines)
	for i := start + 1; i < len(lines); i++ {
		if strings.HasPrefix(lines[i], indent+"- ") {
			end = i
			break
		}
	}
	return strings.Join(lines[start:end], "\n")
}
