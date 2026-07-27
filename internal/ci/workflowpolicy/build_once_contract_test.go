package workflowpolicy

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

type buildOnceMutation struct {
	Name        string `yaml:"name"`
	Old         string `yaml:"old"`
	Replacement string `yaml:"replacement"`
	Want        string `yaml:"want"`
}

func TestValidateBuildOnceWorkflow_accepts_exact_fixture(t *testing.T) {
	// Given an exact reusable build workflow fixture.
	data := readBuildOnceFixture(t, ".github", "workflows", "build-verity.yaml")

	// When the build-once contract is evaluated.
	violations, err := validateBuildOnceWorkflow("build-verity.yaml", data)

	// Then the reusable producer is accepted.
	require.NoError(t, err)
	assert.Empty(t, violations)
}

func TestValidateBuildOnceWorkflow_rejects_security_mutation_fixtures(t *testing.T) {
	// Given the exact workflow and named hostile mutations.
	base := readBuildOnceFixture(t, ".github", "workflows", "build-verity.yaml")
	mutations := readBuildOnceMutations(t, "workflow-mutations.yaml")

	for _, mutation := range mutations {
		t.Run(mutation.Name, func(t *testing.T) {
			data := applyBuildOnceMutation(t, base, mutation)

			// When the mutated producer is evaluated.
			violations, err := validateBuildOnceWorkflow("build-verity.yaml", data)

			// Then the named fail-closed contract rejects it.
			require.NoError(t, err)
			require.NotEmpty(t, violations)
			assert.Contains(t, buildOnceViolationDetails(violations), mutation.Want)
		})
	}
}

func TestValidateSetupVerityAction_accepts_exact_fixture(t *testing.T) {
	// Given an exact verified consumer action fixture.
	data := readBuildOnceFixture(t, ".github", "actions", "consume-verity", "action.yml")

	// When the consumer trust boundary is evaluated.
	violations, err := validateSetupVerityAction("action.yml", data)

	// Then current-run, archive, metadata, and attestation checks are accepted.
	require.NoError(t, err)
	assert.Empty(t, violations)
}

func TestValidateSetupVerityAction_rejects_security_mutation_fixtures(t *testing.T) {
	// Given the exact action and named hostile mutations.
	base := readBuildOnceFixture(t, ".github", "actions", "consume-verity", "action.yml")
	mutations := readBuildOnceMutations(t, "action-mutations.yaml")

	for _, mutation := range mutations {
		t.Run(mutation.Name, func(t *testing.T) {
			data := applyBuildOnceMutation(t, base, mutation)

			// When the mutated consumer action is evaluated.
			violations, err := validateSetupVerityAction("action.yml", data)

			// Then the named trust-boundary mutation is rejected.
			require.NoError(t, err)
			require.NotEmpty(t, violations)
			assert.Contains(t, buildOnceViolationDetails(violations), mutation.Want)
		})
	}
}

func TestRepositoryBuildOnceContract_accepts_exact_workflow_and_action(t *testing.T) {
	// Given the repository root containing the production workflow and action.
	repositoryRoot := filepath.Join("..", "..", "..")

	// When the complete build-once boundary is evaluated.
	violations, err := validateBuildOnceRepository(repositoryRoot)

	// Then the production files satisfy the same contract as the fixtures.
	require.NoError(t, err)
	assert.Empty(t, violations)
}

func TestRepositoryBuildOnceWorkflow_passes_shared_security_policies(t *testing.T) {
	// Given the production reusable workflow decoded through the strict schema.
	data, err := os.ReadFile(filepath.Join("..", "..", "..", ".github", "workflows", "build-verity.yaml"))
	require.NoError(t, err)
	parsed, err := decodeWorkflow(data)
	require.NoError(t, err)
	workflows := []workflowFile{{Name: "build-verity.yaml", Workflow: parsed}}

	// When shared least-privilege, pinning, secret, and Go-ownership policies run.
	violations := make([]Violation, 0, 16)
	violations = append(violations, validatePermissions(workflows)...)
	violations = append(violations, validatePinnedReferences(workflows)...)
	violations = append(violations, validateReusableJobSecrets(workflows)...)
	violations = append(violations, validateGoOwnedLogic(workflows)...)

	// Then the build-once boundary introduces no shared-policy bypass.
	assert.Empty(t, violations)
}

func TestValidateDirectory_registers_build_once_repository_contract(t *testing.T) {
	tests := []struct {
		name        string
		relative    string
		old         string
		replacement string
	}{
		{
			name:     "protected workflow source gate",
			relative: filepath.Join(".github", "workflows", protectedBuildVerityWorkflowName),
			old:      "github.ref_protected == true", replacement: "github.ref_protected == false",
		},
		{
			name:     "action attestation gate",
			relative: filepath.Join(".github", "actions", "setup-verity", "action.yml"),
			old:      "if: ${{ steps.verify-remote.outputs.verify-attestation == 'true' }}", replacement: "if: ${{ false }}",
		},
		{
			name:     "workflow build key output",
			relative: filepath.Join(".github", "workflows", "build-verity.yaml"),
			old:      "value: ${{ jobs.build.outputs.build-key }}", replacement: "value: ${{ jobs.build.outputs.artifact-name }}",
		},
		{
			name:     "workflow source SHA output",
			relative: filepath.Join(".github", "workflows", "build-verity.yaml"),
			old:      "value: ${{ jobs.build.outputs.source-sha }}", replacement: "value: ${{ inputs.source_sha }}",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Given a repository whose build-once workflow or consumer action is weakened.
			root := copyBuildOnceRepository(t)
			path := filepath.Join(root, test.relative)
			data, err := os.ReadFile(path)
			require.NoError(t, err)
			require.Contains(t, string(data), test.old)
			require.NoError(t, os.WriteFile(path, []byte(strings.Replace(string(data), test.old, test.replacement, 1)), 0o600))

			// When the same directory used by the public CLI is validated.
			_, err = ValidateDirectory(filepath.Join(root, ".github", "workflows"))

			// Then the shipped build-once policy rejects the repository mutation.
			require.Error(t, err)
			assert.ErrorIs(t, err, ErrPolicyViolation)
			assert.ErrorContains(t, err, string(RuleBuildOnce))
		})
	}
}

func copyBuildOnceRepository(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	workflowDirectory := filepath.Join(root, ".github", "workflows")
	require.NoError(t, os.MkdirAll(workflowDirectory, 0o700))
	entries, err := os.ReadDir(filepath.Join("testdata", "valid"))
	require.NoError(t, err)
	for _, entry := range entries {
		data, readErr := os.ReadFile(filepath.Join("testdata", "valid", entry.Name()))
		require.NoError(t, readErr)
		require.NoError(t, os.WriteFile(filepath.Join(workflowDirectory, entry.Name()), data, 0o600))
	}
	paths := map[string][]byte{
		filepath.Join(".github", "workflows", "build-verity.yaml"):              readBuildOnceFixture(t, ".github", "workflows", "build-verity.yaml"),
		filepath.Join(".github", "workflows", protectedBuildVerityWorkflowName): readBuildOnceFixture(t, ".github", "workflows", protectedBuildVerityWorkflowName),
		filepath.Join(".github", "actions", "setup-verity", "action.yml"):       readBuildOnceFixture(t, ".github", "actions", "consume-verity", "action.yml"),
	}
	for relative, data := range paths {
		path := filepath.Join(root, relative)
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o700))
		require.NoError(t, os.WriteFile(path, data, 0o600))
	}
	return root
}

func readBuildOnceFixture(t *testing.T, elements ...string) []byte {
	t.Helper()

	path := filepath.Join(append([]string{"testdata", "build-once"}, elements...)...)
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	return data
}

func readBuildOnceMutations(t *testing.T, name string) []buildOnceMutation {
	t.Helper()

	data := readBuildOnceFixture(t, name)
	var mutations []buildOnceMutation
	require.NoError(t, yaml.Unmarshal(data, &mutations))
	require.NotEmpty(t, mutations)
	return mutations
}

func applyBuildOnceMutation(t *testing.T, base []byte, mutation buildOnceMutation) []byte {
	t.Helper()

	require.NotEmpty(t, mutation.Old)
	require.Contains(t, string(base), mutation.Old, "stale mutation fixture %q", mutation.Name)
	return []byte(strings.Replace(string(base), mutation.Old, mutation.Replacement, 1))
}

func buildOnceViolationDetails(violations []Violation) string {
	details := make([]string, 0, len(violations))
	for _, violation := range violations {
		details = append(details, violation.Detail)
	}
	return strings.Join(details, "\n")
}
