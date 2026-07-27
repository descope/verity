package workflowpolicy

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateDirectory_rejects_all_PR_write_scopes(t *testing.T) {
	tests := []struct {
		scope string
		run   string
	}{
		{scope: "actions", run: "gh workflow run trusted.yaml"},
		{scope: "attestations", run: "gh attestation verify artifact"},
		{scope: "contents", run: "git push"},
		{scope: "id-token", run: "cosign sign example.invalid/image@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"},
		{scope: "issues", run: "gh issue create --title test --body test"},
		{scope: "packages", run: "crane copy source target"},
		{scope: "pages", run: "actions/deploy-pages@cd2ce8fcbc39b97be8ca5fce6e763baed58fa128"},
		{scope: "pull-requests", run: "gh pr create --title test --body test"},
		{scope: "security-events", run: "github/codeql-action/upload-sarif@1111111111111111111111111111111111111111"},
	}
	for _, test := range tests {
		t.Run(test.scope, func(t *testing.T) {
			// Given a pull-request job with an explicit write scope.
			root := copyWorkflowFixture(t, "valid")
			replaceWorkflowText(t, root, workflowMutation{
				workflow:    "pr-test.yaml",
				old:         "      contents: read\n      packages: read",
				replacement: "      " + test.scope + ": write",
			})
			replaceWorkflowText(t, root, workflowMutation{
				workflow:    "pr-test.yaml",
				old:         "./verity ci plan --kind integer-pr",
				replacement: test.run,
			})

			// When the workflow policy is evaluated.
			_, err := ValidateDirectory(root)

			// Then every non-authorized PR write fails closed.
			require.Error(t, err)
			assert.ErrorIs(t, err, ErrPolicyViolation)
			assert.ErrorContains(t, err, "pull-request")
		})
	}
}

func TestValidateDirectory_requires_immutable_cross_run_artifact_identity(t *testing.T) {
	tests := []struct {
		name  string
		value string
	}{
		{name: "static name", value: "chart-publication-fixed"},
		{name: "mutable ref expression", value: "chart-publication-${{ github.ref }}"},
		{name: "mutable environment identity", value: "chart-publication-${{ env.batch_id }}"},
		{name: "caller-controlled input identity", value: "chart-publication-${{ inputs.batch_id }}"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Given a reusable chart producer artifact name detached from immutable producer identity.
			root := copyCoherentWorkflowFixture(t)
			replaceWorkflowText(t, root, workflowMutation{
				workflow:    "chart-gen.yaml",
				old:         "      artifact_name: chart-publication-${{ inputs.publication_id }}",
				replacement: "      artifact_name: " + test.value,
			})

			// When the workflow policy is evaluated.
			_, err := ValidateDirectory(root)

			// Then ambiguous artifact selection is rejected.
			require.Error(t, err)
			assert.ErrorIs(t, err, ErrPolicyViolation)
			assert.ErrorContains(t, err, string(RuleProducerIdentity))
		})
	}
}

func TestValidateDirectory_requires_producer_owned_run_identity(t *testing.T) {
	// Given a reusable chart download whose run ID comes from mutable environment state.
	root := copyCoherentWorkflowFixture(t)
	replaceWorkflowText(t, root, workflowMutation{
		workflow:    "chart-gen.yaml",
		old:         "          run-id: ${{ inputs.run_id }}",
		replacement: "          run-id: ${{ env.run_id }}",
	})

	// When the workflow policy is evaluated.
	_, err := ValidateDirectory(root)

	// Then only the exact reusable input is accepted.
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrPolicyViolation)
	assert.ErrorContains(t, err, string(RuleProducerIdentity))
}

func TestValidateDirectory_rejects_reusable_workflow_suffix_spoofs(t *testing.T) {
	tests := []struct {
		name string
		uses string
	}{
		{
			name: "external pinned suffix",
			uses: "attacker/example/.github/workflows/integer-build-shard.yaml@1111111111111111111111111111111111111111",
		},
		{
			name: "nested local suffix",
			uses: "./nested/integer-build-shard.yaml",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Given a reusable workflow that only spoofs the trusted filename suffix.
			root := copyWorkflowFixture(t, "valid")
			replaceWorkflowText(t, root, workflowMutation{
				workflow:    "integer-orchestrator-reusable.yaml",
				old:         "./.github/workflows/integer-build-shard.yaml",
				replacement: test.uses,
			})

			// When the workflow policy is evaluated.
			_, err := ValidateDirectory(root)

			// Then only the exact trusted reusable workflow identity is accepted.
			require.Error(t, err)
			assert.ErrorIs(t, err, ErrPolicyViolation)
			assert.ErrorContains(t, err, string(RuleProducerIdentity))
		})
	}
}

func TestValidateDirectory_requires_workflow_call_identity_input_contract(t *testing.T) {
	tests := []struct {
		name        string
		old         string
		replacement string
	}{
		{
			name:        "batch input optional",
			old:         "      batch_id:\n        required: true\n        type: string",
			replacement: "      batch_id:\n        required: false\n        type: string",
		},
		{
			name:        "shard input wrong type",
			old:         "      shard:\n        required: true\n        type: string",
			replacement: "      shard:\n        required: true\n        type: boolean",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Given a weakened workflow_call identity input.
			root := copyWorkflowFixture(t, "valid")
			replaceWorkflowText(t, root, workflowMutation{
				workflow:    "integer-build-image-reusable.yaml",
				old:         test.old,
				replacement: test.replacement,
			})

			// When the workflow policy is evaluated.
			_, err := ValidateDirectory(root)

			// Then missing required string semantics fail closed.
			require.Error(t, err)
			assert.ErrorIs(t, err, ErrPolicyViolation)
			assert.ErrorContains(t, err, string(RuleProducerIdentity))
		})
	}
}

func TestValidateDirectory_rejects_noncanonical_policy_significant_names(t *testing.T) {
	tests := []struct {
		name     string
		mutation workflowMutation
		wantErr  error
	}{
		{
			name: "case-variant deploy action",
			mutation: workflowMutation{
				workflow:    "apk-repository.yaml",
				old:         "actions/checkout@9c091bb21b7c1c1d1991bb908d89e4e9dddfe3e0",
				replacement: "Actions/Deploy-Pages@cd2ce8fcbc39b97be8ca5fce6e763baed58fa128",
			},
			wantErr: ErrPolicyViolation,
		},
		{
			name: "nested case-variant PR trigger",
			mutation: workflowMutation{
				workflow:    "pr-test.yaml",
				old:         "  pull_request:",
				replacement: "  Pull_Request:\n    types: [opened]",
			},
			wantErr: ErrInvalidWorkflow,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Given a case-variant policy-significant YAML value.
			root := copyWorkflowFixture(t, "valid")
			replaceWorkflowText(t, root, test.mutation)

			// When the workflow policy is evaluated.
			_, err := ValidateDirectory(root)

			// Then typed parsing or policy evaluation rejects it.
			require.Error(t, err)
			assert.True(t, errors.Is(err, test.wantErr), "unexpected error: %v", err)
		})
	}
}

func TestValidateDirectory_requires_explicit_permissions_at_every_scope(t *testing.T) {
	tests := []struct {
		name     string
		mutation workflowMutation
	}{
		{
			name: "workflow permissions missing",
			mutation: workflowMutation{
				workflow: "apk-repository.yaml",
				old:      "permissions:\n  contents: read\n\n",
			},
		},
		{
			name: "job permissions missing",
			mutation: workflowMutation{
				workflow: "apk-repository.yaml",
				old:      "    permissions:\n      contents: read\n",
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Given repository defaults would determine effective permissions.
			root := copyWorkflowFixture(t, "valid")
			replaceWorkflowText(t, root, test.mutation)

			// When the workflow policy is evaluated.
			_, err := ValidateDirectory(root)

			// Then permission ambiguity is rejected.
			require.Error(t, err)
			assert.ErrorIs(t, err, ErrPolicyViolation)
			assert.ErrorContains(t, err, string(RuleLeastPrivilege))
		})
	}
}
