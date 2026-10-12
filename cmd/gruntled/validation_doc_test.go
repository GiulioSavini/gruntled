package main

// TestValidationDocPins keeps docs/validation.md in step with the pins and
// expectations the experiment tests use. It is always on.

import (
	"fmt"
	"os"
	"strings"
	"testing"
)

func TestValidationDocPins(t *testing.T) {
	b, err := os.ReadFile("../../docs/validation.md")
	if err != nil {
		t.Fatalf("read docs/validation.md: %v", err)
	}
	doc := string(b)

	want := []string{
		corpusPinnedCommit,
		denisPinnedCommit,
		tgPinnedSHA256,
		"terragrunt " + tgPinnedVersion,
		"GRUNTLED_CORPUS",
		"GRUNTLED_TERRAGRUNT_BIN",
		"GRUNTLED_CORPUS_DENIS256",
	}
	for _, e := range denisExpected {
		want = append(want, fmt.Sprintf("%s:%d:%d", e.File, e.Line, e.Col))
	}
	for _, e := range denisSilent {
		want = append(want, fmt.Sprintf("%s:%d:%d", e.File, e.Line, e.Col))
	}
	// The 11 primary-corpus oracle files of the two mutations.
	want = append(want,
		"iac.src/ecr_health/terragrunt.hcl",
		"iac.src/ecr_inbox/terragrunt.hcl",
		"iac.src/ecr_outbox/terragrunt.hcl",
		"iac.src/ecr_process/terragrunt.hcl",
		"iac.src/ecr_recover/terragrunt.hcl",
		"iac.src/ecr_release/terragrunt.hcl",
		"iac.src/ecr_timeout/terragrunt.hcl",
		"iac.src/ecr_uuid/terragrunt.hcl",
		"iac.mq/ecr_mq_generator/terragrunt.hcl",
		"iac.mq/ecr_mq_reader/terragrunt.hcl",
		"iac.mq/ecr_mq_writer/terragrunt.hcl",
	)
	// v0.2 GRT002/GRT003 (MORE-06).
	want = append(want,
		"## v0.2: GRT002 and GRT003 on the real corpus (MORE-06)",
		secretPinnedCommit,
		"GRUNTLED_CORPUS_SECRET",
		"GRT002",
		"GRT003",
		graphTGCycle,
	)
	for _, e := range denisExpectedGraph {
		want = append(want, fmt.Sprintf("%s:%d:%d", e.File, e.Line, e.Col), e.Msg)
	}
	for _, m := range graphMutations {
		want = append(want, m.Want.pos(), m.Want.Msg)
	}
	// v0.4 DAEMON-10: the daemon memory record at the three sizes
	// TestDaemonMemorySynthrepo measures.
	want = append(want,
		"## v0.4 daemon memory (DAEMON-10)",
		"VALIDATION-BENCH daemon_mem units=65 ",
		"VALIDATION-BENCH daemon_mem units=500 ",
		"VALIDATION-BENCH daemon_mem units=5000 ",
		"TestDaemonMemorySynthrepo",
	)
	want = append(want, rel03DocPins()...)
	for _, s := range want {
		if !strings.Contains(doc, s) {
			t.Errorf("docs/validation.md does not contain %q", s)
		}
	}
}

// rel03DocPins are the REL-03 strings docs/validation.md must hold, built
// from the package-level values the gated corpus tests assert, so a changed
// count, category or exception fails until the doc is updated.
func rel03DocPins() []string {
	pins := []string{
		"## v0.4: blast on the real corpora (REL-03)",
		corpusPinnedCommit, secretPinnedCommit, denisPinnedCommit,
		"scripts/fetch-corpora.sh", "scripts/corpus-gate.sh",
		"corpus-gate: network=isolated", "corpus-gate: 31/31",
		"256/256 identical",
		"TestCorpusDaemonSample",
		m17SentinelCanon,
	}
	pins = append(pins, rel03Exceptions...)
	corpora := []string{"iso20022", "secret", "denis256"}
	for _, c := range corpora {
		for _, class := range mutationClasses {
			p := corpusMutationPins[c][class]
			pins = append(pins, fmt.Sprintf("CORPUS-MUTATION %s %s sites=%d caught=%d excluded_by_touch=%d", c, class, p.Sites, p.Sites, p.Touch))
		}
		d := corpusDepthPins[c]
		pins = append(pins, fmt.Sprintf("CORPUS-MUTATION %s M8 depth>=2=%d depth>=3=%d depth>=4=%d depth>=5=%d", c, d[0], d[1], d[2], d[3]))
		pins = append(pins, fmt.Sprintf("| %s | %d | %d | %.3f |", c, corpusIncluded[c], corpusGraphKnown[c], float64(corpusIncluded[c])/float64(corpusGraphKnown[c])))
	}
	// Exclusion totals per category: | `category` | iso20022 | secret | denis256 | total |
	count := map[string]map[string]int{}
	for _, c := range corpora {
		for _, e := range corpusExclusions[c] {
			if count[e.Category] == nil {
				count[e.Category] = map[string]int{}
			}
			count[e.Category][c]++
		}
	}
	for cat, byC := range count {
		pins = append(pins, fmt.Sprintf("| `%s` | %d | %d | %d | %d |", cat, byC["iso20022"], byC["secret"], byC["denis256"], byC["iso20022"]+byC["secret"]+byC["denis256"]))
	}
	return pins
}
