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
	for _, s := range want {
		if !strings.Contains(doc, s) {
			t.Errorf("docs/validation.md does not contain %q", s)
		}
	}
}
