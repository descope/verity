package workflowpolicy

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidatePublicationRollout_accepts_manual_only_bootstrap(t *testing.T) {
	// Given a non-runnable genesis lock and manual-only publisher.
	directory := writeSignerRolloutFixture(t, `{"digest":"","source_sha":"","bootstrap":true,"runnable":false}`)
	workflows := []workflowFile{{Name: "publish.yaml", Workflow: workflow{On: triggers{WorkflowDispatch: true}}}}

	// When the staged rollout contract is evaluated.
	violations := validatePublicationRollout(directory, workflows)

	// Then no automatic publication can start before a signer is pinned.
	assert.Empty(t, violations)
}

func TestValidatePublicationRollout_rejects_automatic_bootstrap(t *testing.T) {
	// Given a non-runnable genesis lock and an automatically triggered publisher.
	directory := writeSignerRolloutFixture(t, `{"digest":"","source_sha":"","bootstrap":true,"runnable":false}`)
	workflows := []workflowFile{{Name: "publish.yaml", Workflow: workflow{On: triggers{
		WorkflowDispatch: true, Schedule: true, Push: pushTrigger{Present: true},
	}}}}

	// When the staged rollout contract is evaluated.
	violations := validatePublicationRollout(directory, workflows)

	// Then the automatic trigger is rejected before merge.
	require.Len(t, violations, 1)
	assert.Contains(t, violations[0].Detail, "manual-only")
}

func TestValidatePublicationRollout_accepts_automatic_runnable_lock(t *testing.T) {
	// Given immutable signer coordinates and the steady-state trigger set.
	directory := writeSignerRolloutFixture(t, `{"digest":"sha256:abc","source_sha":"deadbeef","bootstrap":false,"runnable":true}`)
	workflows := []workflowFile{{Name: "publish.yaml", Workflow: workflow{On: triggers{
		WorkflowDispatch: true, Schedule: true, Push: pushTrigger{Present: true},
	}}}}

	// When the steady-state rollout contract is evaluated.
	violations := validatePublicationRollout(directory, workflows)

	// Then automatic publication is allowed only after pinning.
	assert.Empty(t, violations)
}

func writeSignerRolloutFixture(t *testing.T, lock string) string {
	t.Helper()
	root := t.TempDir()
	directory := filepath.Join(root, ".github", "workflows")
	require.NoError(t, os.MkdirAll(directory, 0o700))
	require.NoError(t, os.MkdirAll(filepath.Join(root, "ci"), 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(root, "ci", "apk-signer.lock.json"), []byte(lock), 0o600))
	return directory
}
