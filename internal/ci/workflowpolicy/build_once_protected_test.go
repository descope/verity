package workflowpolicy

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateProtectedBuildWorkflow_accepts_exact_fixture(t *testing.T) {
	// Given an exact protected wrapper around the unprivileged build producer.
	data := readBuildOnceFixture(t, ".github", "workflows", protectedBuildVerityWorkflowName)

	// When the protected build contract is evaluated.
	violations, err := validateProtectedBuildWorkflow(protectedBuildVerityWorkflowName, data)

	// Then exact-main authorization, attestation, and output proxying are accepted.
	require.NoError(t, err)
	assert.Empty(t, violations)
}

func TestValidateProtectedBuildWorkflow_rejects_security_mutation_fixtures(t *testing.T) {
	// Given the exact protected wrapper and named hostile mutations.
	base := readBuildOnceFixture(t, ".github", "workflows", protectedBuildVerityWorkflowName)
	mutations := readBuildOnceMutations(t, "protected-workflow-mutations.yaml")

	for _, mutation := range mutations {
		t.Run(mutation.Name, func(t *testing.T) {
			data := applyBuildOnceMutation(t, base, mutation)

			// When the mutated protected wrapper is evaluated.
			violations, err := validateProtectedBuildWorkflow(protectedBuildVerityWorkflowName, data)

			// Then the named fail-closed contract rejects it.
			require.NoError(t, err)
			require.NotEmpty(t, violations)
			assert.Contains(t, buildOnceViolationDetails(violations), mutation.Want)
		})
	}
}
