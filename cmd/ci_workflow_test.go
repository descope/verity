package cmd

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/urfave/cli/v3"

	"github.com/verity-org/verity/internal/ci/workflowpolicy"
)

type cliWorkflowMutation struct {
	file        string
	old         string
	replacement string
}

func TestCIWorkflowCommand_validates_compliant_workflows(t *testing.T) {
	// Given the public CI command tree and a compliant workflow fixture.
	var stdout bytes.Buffer
	command := newWorkflowTestRoot(&stdout)
	fixture := filepath.Join("..", "internal", "ci", "workflowpolicy", "testdata", "valid")

	// When the workflow validator command runs.
	err := command.Run(context.Background(), []string{"verity", "ci", "workflow", "validate", fixture})

	// Then it reports a real successful validation.
	require.NoError(t, err)
	assert.Equal(t, "workflow policy validated: 9 workflows\n", stdout.String())
}

func TestCIWorkflowCommand_returns_error_without_success_output_when_input_is_malformed(t *testing.T) {
	// Given malformed workflow input and captured command output.
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "broken.yaml"), []byte("jobs: ["), 0o600))
	var stdout bytes.Buffer
	command := newWorkflowTestRoot(&stdout)

	// When validation runs.
	err := command.Run(context.Background(), []string{"verity", "ci", "workflow", "validate", root})

	// Then the typed parse error is returned and no misleading success is printed.
	require.Error(t, err)
	assert.ErrorIs(t, err, workflowpolicy.ErrInvalidWorkflow)
	assert.NotContains(t, stdout.String(), "validated")
}

func TestCIWorkflowCommand_never_prints_success_for_hostile_workflows(t *testing.T) {
	tests := []struct {
		name      string
		mutations []cliWorkflowMutation
	}{
		{
			name: "used PR contents write",
			mutations: []cliWorkflowMutation{
				{file: "pr-test.yaml", old: "      contents: read\n      packages: read", replacement: "      contents: write"},
				{file: "pr-test.yaml", old: "./verity ci plan --kind integer-pr", replacement: "git push"},
			},
		},
		{
			name: "static cross-run artifact",
			mutations: []cliWorkflowMutation{
				{file: "build-site.yaml", old: "          name: ${{ needs.integer.outputs.manifest_artifact_name }}", replacement: "          name: apk-repository-fixed\n          run-id: \"12345\""},
			},
		},
		{
			name: "unrelated artifact producer",
			mutations: []cliWorkflowMutation{
				{file: "integer-orchestrator-reusable.yaml", old: "      manifest_artifact_digest: ${{ steps.upload-manifest.outputs.artifact-digest }}", replacement: "      manifest_artifact_digest: ${{ needs.plan.outputs.manifest_artifact_digest }}"},
			},
		},
		{
			name: "null concurrency scalar",
			mutations: []cliWorkflowMutation{
				{file: "apk-repository.yaml", old: "permissions:\n", replacement: "concurrency: null\n\npermissions:\n"},
			},
		},
		{
			name: "external reusable suffix spoof",
			mutations: []cliWorkflowMutation{
				{file: "integer-orchestrator-reusable.yaml", old: "    uses: ./.github/workflows/integer-build-shard.yaml\n", replacement: "    uses: attacker/example/.github/workflows/integer-build-shard.yaml@1111111111111111111111111111111111111111\n"},
			},
		},
		{
			name: "optional workflow identity input",
			mutations: []cliWorkflowMutation{
				{file: "integer-build-image-reusable.yaml", old: "      batch_id:\n        required: true\n        type: string", replacement: "      batch_id:\n        required: false\n        type: string"},
			},
		},
		{
			name: "case-variant deploy action",
			mutations: []cliWorkflowMutation{
				{file: "apk-repository.yaml", old: "actions/checkout@9c091bb21b7c1c1d1991bb908d89e4e9dddfe3e0", replacement: "Actions/Deploy-Pages@cd2ce8fcbc39b97be8ca5fce6e763baed58fa128"},
			},
		},
		{
			name: "nested case-variant PR trigger",
			mutations: []cliWorkflowMutation{
				{file: "pr-test.yaml", old: "  pull_request:", replacement: "  Pull_Request:\n    types: [opened]"},
			},
		},
		{
			name: "missing job permissions",
			mutations: []cliWorkflowMutation{
				{file: "apk-repository.yaml", old: "    permissions:\n      contents: read\n", replacement: ""},
			},
		},
		{
			name: "nested duplicate permission",
			mutations: []cliWorkflowMutation{
				{file: "apk-repository.yaml", old: "    permissions:\n      contents: read\n", replacement: "    permissions:\n      contents: read\n      contents: read\n"},
			},
		},
		{
			name: "unknown job field",
			mutations: []cliWorkflowMutation{
				{file: "apk-repository.yaml", old: "    runs-on: ubuntu-24.04\n", replacement: "    runs-on: ubuntu-24.04\n    unknown-policy-field: true\n"},
			},
		},
		{
			name: "YAML anchor",
			mutations: []cliWorkflowMutation{
				{file: "apk-repository.yaml", old: "name: APK Repository Validation", replacement: "name: &workflow_name APK Repository Validation"},
			},
		},
		{
			name: "null reusable secrets",
			mutations: []cliWorkflowMutation{
				{file: "integer-orchestrator-reusable.yaml", old: "    uses: ./.github/workflows/integer-build-shard.yaml\n", replacement: "    uses: ./.github/workflows/integer-build-shard.yaml\n    secrets: null\n"},
			},
		},
		{
			name: "boolean reusable secrets",
			mutations: []cliWorkflowMutation{
				{file: "integer-orchestrator-reusable.yaml", old: "    uses: ./.github/workflows/integer-build-shard.yaml\n", replacement: "    uses: ./.github/workflows/integer-build-shard.yaml\n    secrets: true\n"},
			},
		},
		{
			name: "numeric reusable secrets",
			mutations: []cliWorkflowMutation{
				{file: "integer-orchestrator-reusable.yaml", old: "    uses: ./.github/workflows/integer-build-shard.yaml\n", replacement: "    uses: ./.github/workflows/integer-build-shard.yaml\n    secrets: 42\n"},
			},
		},
		{
			name: "arbitrary scalar reusable secrets",
			mutations: []cliWorkflowMutation{
				{file: "integer-orchestrator-reusable.yaml", old: "    uses: ./.github/workflows/integer-build-shard.yaml\n", replacement: "    uses: ./.github/workflows/integer-build-shard.yaml\n    secrets: attacker\n"},
			},
		},
		{
			name: "mixed reusable secrets mapping",
			mutations: []cliWorkflowMutation{
				{file: "integer-orchestrator-reusable.yaml", old: "    uses: ./.github/workflows/integer-build-shard.yaml\n", replacement: "    uses: ./.github/workflows/integer-build-shard.yaml\n    secrets:\n      artifact-token: ${{ secrets.GITHUB_TOKEN }}\n      other-token: null\n"},
			},
		},
		{
			name: "ordinary job inherit secrets",
			mutations: []cliWorkflowMutation{
				{file: "apk-repository.yaml", old: "    steps:\n", replacement: "    secrets: inherit\n    steps:\n"},
			},
		},
		{
			name: "ordinary job mapped secrets",
			mutations: []cliWorkflowMutation{
				{file: "apk-repository.yaml", old: "    steps:\n", replacement: "    secrets:\n      TOKEN: ${{ secrets.GITHUB_TOKEN }}\n    steps:\n"},
			},
		},
		{
			name: "undeclared local reusable secret",
			mutations: []cliWorkflowMutation{
				{file: "integer-orchestrator-reusable.yaml", old: "    with:\n      source_sha: ${{ needs.plan.outputs.source_sha }}", replacement: "    secrets:\n      attacker-token: ${{ secrets.GITHUB_TOKEN }}\n    with:\n      source_sha: ${{ needs.plan.outputs.source_sha }}"},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Given a fresh compliant fixture with one hostile mutation class.
			root := filepath.Join(t.TempDir(), "workflows")
			source := filepath.Join("..", "internal", "ci", "workflowpolicy", "testdata", "valid")
			require.NoError(t, os.CopyFS(root, os.DirFS(source)))
			for _, mutation := range test.mutations {
				applyCLIWorkflowMutation(t, root, mutation)
			}
			var stdout bytes.Buffer
			command := newWorkflowTestRoot(&stdout)

			// When the CLI validates the hostile fixture.
			err := command.Run(context.Background(), []string{"verity", "ci", "workflow", "validate", root})

			// Then it fails and never emits the success banner.
			require.Error(t, err)
			assert.NotContains(t, stdout.String(), "workflow policy validated")
		})
	}
}

func TestCIWorkflowCommand_is_registered_without_editing_ci_command(t *testing.T) {
	// Given the CI command assembled by package initialization.
	registered := false

	// When its direct children are inspected.
	for _, command := range CICommand.Commands {
		if command.Name == "workflow" {
			registered = true
		}
	}

	// Then the workflow policy surface is available to the built binary.
	assert.True(t, registered)
}

func newWorkflowTestRoot(stdout *bytes.Buffer) *cli.Command {
	return &cli.Command{
		Writer: stdout,
		Commands: []*cli.Command{
			{Name: "ci", Commands: []*cli.Command{newCIWorkflowCommand()}},
		},
	}
}

func applyCLIWorkflowMutation(t *testing.T, root string, mutation cliWorkflowMutation) {
	t.Helper()

	path := filepath.Join(root, mutation.file)
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Contains(t, string(data), mutation.old, "stale mutation for %s", mutation.file)
	updated := strings.Replace(string(data), mutation.old, mutation.replacement, 1)
	require.NoError(t, os.WriteFile(path, []byte(updated), 0o600))
}
