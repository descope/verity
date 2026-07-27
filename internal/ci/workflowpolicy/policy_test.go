package workflowpolicy

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIntegerBuildImageWorkflow_preserves_localZeroCVEGate_beforeGoOwnedPublication(t *testing.T) {
	// Given the existing Integer image workflow.
	workflowPath := filepath.Join("..", "..", "..", ".github", "workflows", "integer-build-image-reusable.yaml")
	data, err := os.ReadFile(workflowPath)
	require.NoError(t, err)
	workflow := string(data)

	// When the fail-closed local gate and typed publication boundary are located.
	gate := strings.Index(workflow, "--fail-on-severity UNKNOWN,LOW,MEDIUM,HIGH,CRITICAL")
	promotion := strings.Index(workflow, "./verity ci integer-image publish")

	// Then the local vulnerability gate remains present and precedes the Go
	// command whose unit tests lock staged Trivy before final crane promotion.
	require.NotEqual(t, -1, gate, "missing fail-closed Trivy gate")
	require.NotEqual(t, -1, promotion, "missing Go-owned image publication")
	assert.Less(t, gate, promotion)
}

func TestValidateDirectory_rejects_second_pages_deployer(t *testing.T) {
	// Given a valid workflow set mutated with a second Pages deployment owner.
	root := copyWorkflowFixture(t, "valid")
	apkWorkflow := filepath.Join(root, "apk-repository.yaml")
	data, err := os.ReadFile(apkWorkflow)
	require.NoError(t, err)
	data = append(data, []byte(`
  deploy:
    runs-on: ubuntu-24.04
    permissions:
      pages: write
      id-token: write
    steps:
      - uses: actions/deploy-pages@cd2ce8fcbc39b97be8ca5fce6e763baed58fa128
`)...)
	require.NoError(t, os.WriteFile(apkWorkflow, data, 0o600))

	// When the workflow policy is evaluated.
	_, err = ValidateDirectory(root)

	// Then the extra deployer fails the typed policy check.
	require.Error(t, err)
	require.ErrorIs(t, err, ErrPolicyViolation)
	assert.ErrorContains(t, err, "pages-owner")
	assert.ErrorContains(t, err, "apk-repository.yaml")
}

func copyWorkflowFixture(t *testing.T, name string) string {
	t.Helper()

	source := filepath.Join("testdata", name)
	destination := t.TempDir()
	entries, err := os.ReadDir(source)
	require.NoError(t, err)
	for _, entry := range entries {
		require.False(t, entry.IsDir(), "fixture directories must be flat")
		data, readErr := os.ReadFile(filepath.Join(source, entry.Name()))
		require.NoError(t, readErr)
		require.NoError(t, os.WriteFile(filepath.Join(destination, entry.Name()), data, 0o600))
	}
	return destination
}
