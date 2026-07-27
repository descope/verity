package workflowpolicy

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDecodeWorkflow_rejects_invalid_reusable_job_secret_scalars(t *testing.T) {
	tests := []struct {
		name  string
		value string
	}{
		{name: "null", value: "null"},
		{name: "boolean", value: "true"},
		{name: "number", value: "42"},
		{name: "arbitrary string", value: "attacker"},
		{name: "empty string", value: `""`},
		{name: "spaced inherit", value: `" inherit "`},
		{name: "explicit tag", value: "!!str inherit"},
		{name: "custom tag", value: "!policy inherit"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Given a reusable job with a malformed scalar secrets form.
			input := fmt.Appendf(nil, "on: workflow_dispatch\njobs:\n  call:\n    uses: ./.github/workflows/called.yaml\n    secrets: %s\n", test.value)

			// When the typed workflow boundary parses the job.
			_, err := decodeWorkflow(input)

			// Then only the exact inherit literal is accepted as a scalar.
			assert.Error(t, err)
		})
	}
}

func TestDecodeWorkflow_rejects_invalid_or_mixed_reusable_job_secret_mappings(t *testing.T) {
	tests := []struct {
		name      string
		yamlValue string
	}{
		{name: "empty mapping", yamlValue: "{}"},
		{name: "empty value", yamlValue: `{INPUT: ""}`},
		{name: "null value", yamlValue: "{INPUT: null}"},
		{name: "boolean value", yamlValue: "{INPUT: true}"},
		{name: "number value", yamlValue: "{INPUT: 42}"},
		{name: "sequence value", yamlValue: "{INPUT: [secret]}"},
		{name: "mapping value", yamlValue: "{INPUT: {value: secret}}"},
		{name: "non-string key", yamlValue: "{42: secret}"},
		{name: "invalid secret name", yamlValue: "{bad.key: secret}"},
		{name: "duplicate key", yamlValue: "{INPUT: one, INPUT: two}"},
		{name: "case duplicate key", yamlValue: "{INPUT: one, input: two}"},
		{name: "tagged value", yamlValue: "{INPUT: !!str secret}"},
		{name: "mixed valid and null", yamlValue: "{INPUT: ${{ secrets.INPUT }}, OTHER: null}"},
		{name: "mixed valid and boolean", yamlValue: "{INPUT: literal, OTHER: false}"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Given a reusable job secrets map containing an invalid member.
			input := fmt.Appendf(nil, "on: workflow_dispatch\njobs:\n  call:\n    uses: ./.github/workflows/called.yaml\n    secrets: %s\n", test.yamlValue)

			// When the typed workflow boundary parses the map recursively.
			_, err := decodeWorkflow(input)

			// Then invalid keys or values reject the complete mapping.
			assert.Error(t, err)
		})
	}
}

func TestDecodeWorkflow_rejects_reusable_job_secrets_on_ordinary_run_jobs(t *testing.T) {
	tests := []struct {
		name      string
		yamlValue string
	}{
		{name: "inherit", yamlValue: "inherit"},
		{name: "mapping", yamlValue: "{INPUT: ${{ secrets.INPUT }}}"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Given a normal steps job using reusable-workflow-only secrets syntax.
			input := fmt.Appendf(nil, "on: workflow_dispatch\njobs:\n  run:\n    runs-on: ubuntu-24.04\n    secrets: %s\n    steps: [{run: echo invalid}]\n", test.yamlValue)

			// When the typed workflow boundary parses the job shape.
			_, err := decodeWorkflow(input)

			// Then ordinary run jobs cannot declare reusable-job secrets.
			assert.Error(t, err)
		})
	}
}

func TestDecodeWorkflow_rejects_secrets_on_hybrid_run_job_with_spoofed_uses(t *testing.T) {
	// Given a normal run job adds a reusable workflow reference to disguise its secrets form.
	input := []byte(`on: workflow_dispatch
jobs:
  run:
    uses: ./.github/workflows/called.yaml
    runs-on: ubuntu-24.04
    secrets: inherit
    steps:
      - run: echo invalid
`)

	// When the typed workflow boundary parses the hybrid job shape.
	_, err := decodeWorkflow(input)

	// Then reusable-job secrets remain forbidden on jobs with run-job fields.
	assert.Error(t, err)
}

func TestDecodeWorkflow_accepts_exact_reusable_job_secret_forms(t *testing.T) {
	tests := []struct {
		name      string
		yamlValue string
	}{
		{name: "inherit", yamlValue: "inherit"},
		{name: "expression and literal mapping", yamlValue: "\n      INPUT: ${{ secrets.INPUT }}\n      LABEL: literal-value"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Given a reusable workflow job using one exact GitHub secrets form.
			input := fmt.Appendf(nil, "on: workflow_dispatch\njobs:\n  call:\n    uses: ./.github/workflows/called.yaml\n    secrets: %s\n", test.yamlValue)

			// When the typed workflow boundary parses the job.
			_, err := decodeWorkflow(input)

			// Then inherit and nonempty string mappings remain valid.
			assert.NoError(t, err)
		})
	}
}

func TestValidateDirectory_rejects_unknown_local_reusable_workflow_secret(t *testing.T) {
	// Given a local reusable workflow caller supplies a secret absent from its workflow_call contract.
	root := copyWorkflowFixture(t, "valid")
	replaceWorkflowText(t, root, workflowMutation{
		workflow:    "integer-orchestrator-reusable.yaml",
		old:         "    with:\n      source_sha: ${{ needs.plan.outputs.source_sha }}",
		replacement: "    secrets:\n      attacker-token: ${{ secrets.GITHUB_TOKEN }}\n    with:\n      source_sha: ${{ needs.plan.outputs.source_sha }}",
	})

	// When the directory policy resolves the called workflow contract.
	_, err := ValidateDirectory(root)

	// Then undeclared secret names fail closed.
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrPolicyViolation)
}
