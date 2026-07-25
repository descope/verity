package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

type prWorkflowContract struct {
	On          map[string]any                   `yaml:"on"`
	Permissions map[string]string                `yaml:"permissions"`
	Jobs        map[string]prWorkflowContractJob `yaml:"jobs"`
}

type prWorkflowContractJob struct {
	Permissions map[string]string        `yaml:"permissions"`
	Environment any                      `yaml:"environment"`
	Secrets     any                      `yaml:"secrets"`
	With        map[string]any           `yaml:"with"`
	Steps       []prWorkflowContractStep `yaml:"steps"`
}

type prWorkflowContractStep struct {
	Run  string         `yaml:"run"`
	Uses string         `yaml:"uses"`
	With map[string]any `yaml:"with"`
}

func TestPRTestWorkflow_uses_typed_read_only_secret_free_commands(t *testing.T) {
	// Given: the real pull-request workflow.
	path := filepath.Join("..", ".github", "workflows", "pr-test.yaml")
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	var workflow prWorkflowContract
	require.NoError(t, yaml.Unmarshal(data, &workflow))

	// When: its permissions and executable trust surface are inspected.

	// Then: every job is explicit least privilege and no PR job can reach protected credentials.
	require.NotEmpty(t, workflow.Jobs)
	for name, job := range workflow.Jobs {
		require.NotEmpty(t, job.Permissions, "job %s must declare permissions", name)
		for permission, access := range job.Permissions {
			require.Equal(t, "read", access, "job %s permission %s", name, permission)
		}
		require.Nil(t, job.Environment, "job %s must not target a protected environment", name)
		require.Nil(t, job.Secrets, "job %s must not receive secrets", name)
	}
	require.NotContains(t, string(data), "pull_request_target")
	require.NotContains(t, string(data), "${{ secrets.")
	require.NotContains(t, string(data), "id-token:")
	require.NotContains(t, string(data), "attestations:")

	build := workflow.Jobs["build-verity"]
	require.Equal(t, "false", build.With["protected_attestation"])

	for name, job := range workflow.Jobs {
		for _, step := range job.Steps {
			if !strings.Contains(step.Run, "./verity ci pr-test") {
				continue
			}
			require.Contains(t, string(data), "uses: ./.github/actions/setup-verity", "job %s", name)
		}
	}
}
