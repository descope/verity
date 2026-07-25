package cmd

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	errTestCopaMirrorCopy   = errors.New("upstream unavailable")
	errTestCopaMirrorDigest = errors.New("target missing")
)

func TestCopaOrchestratorPlan_selects_expected_mode_for_each_event(t *testing.T) {
	tests := []struct {
		name      string
		request   copaOrchestratorPlanRequest
		wantOnly  string
		wantForce bool
		wantErr   error
	}{
		{
			name:    "scheduled runs scan every image",
			request: copaOrchestratorPlanRequest{event: copaEventSchedule},
		},
		{
			name:      "manual runs force the selected image",
			request:   copaOrchestratorPlanRequest{event: copaEventWorkflowDispatch, image: "library/nginx"},
			wantOnly:  "library/nginx",
			wantForce: true,
		},
		{
			name:     "manual preflight scans instead of forcing",
			request:  copaOrchestratorPlanRequest{event: copaEventWorkflowDispatch, preflight: true, image: "library/nginx"},
			wantOnly: "library/nginx",
		},
		{
			name:      "push filters force only changed images",
			request:   copaOrchestratorPlanRequest{event: copaEventPush, changeMode: copaChangeModeFilter, changeFilter: "a,b"},
			wantOnly:  "a,b",
			wantForce: true,
		},
		{
			name:    "unsafe manual image is rejected",
			request: copaOrchestratorPlanRequest{event: copaEventWorkflowDispatch, image: "$(bad)"},
			wantErr: errInvalidCopaImage,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			selection, err := planCopaOrchestrator(test.request)
			if test.wantErr != nil {
				require.ErrorIs(t, err, test.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, test.wantOnly, selection.only)
			assert.Equal(t, test.wantForce, selection.force)
		})
	}
}

func TestCopaOrchestratorMirror_uses_existing_target_when_copy_fails(t *testing.T) {
	restore := stubCopaMirror(t)
	copaMirrorCopy = func(context.Context, string, string) error { return errTestCopaMirrorCopy }
	copaMirrorDigest = func(context.Context, string) (string, error) { return "sha256:existing", nil }
	t.Cleanup(restore)

	err := mirrorCopaImage(context.Background(), "upstream/image:tag", "target/image:tag")

	require.NoError(t, err)
}

func TestCopaOrchestratorMirror_fails_when_copy_and_target_lookup_fail(t *testing.T) {
	restore := stubCopaMirror(t)
	copaMirrorCopy = func(context.Context, string, string) error { return errTestCopaMirrorCopy }
	copaMirrorDigest = func(context.Context, string) (string, error) { return "", errTestCopaMirrorDigest }
	t.Cleanup(restore)

	err := mirrorCopaImage(context.Background(), "upstream/image:tag", "target/image:tag")

	require.ErrorIs(t, err, errCopaMirrorUnavailable)
	require.ErrorIs(t, err, errTestCopaMirrorCopy)
	require.ErrorIs(t, err, errTestCopaMirrorDigest)
}

func TestCopaOrchestratorWorkflow_uses_typed_commands_and_protected_dispatch_checkout(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", ".github", "workflows", "orchestrator.yaml"))
	require.NoError(t, err)
	workflow := string(data)

	for _, command := range []string{
		"./verity nightly copa-orchestrator mirror",
		"./verity nightly copa-orchestrator detect",
		"./verity nightly copa-orchestrator plan",
	} {
		assert.Contains(t, workflow, command)
	}
	assert.GreaterOrEqual(t, strings.Count(workflow, "ref: ${{ github.sha }}"), 3)
	assert.NotContains(t, workflow, "PLAN_ARGS=(")
	assert.NotContains(t, workflow, "git diff --name-only")
	assert.NotContains(t, workflow, "crane copy")
	assert.NotContains(t, workflow, "jq -r")

	for _, output := range []string{
		"images: ${{ steps.discover.outputs.images }}",
		"count: ${{ steps.discover.outputs.count }}",
		"source_sha: ${{ needs.verity.outputs.source-sha }}",
		"artifact_digest: ${{ steps.upload-plan.outputs.artifact-digest }}",
	} {
		assert.Contains(t, workflow, output)
	}
}

func stubCopaMirror(t *testing.T) func() {
	t.Helper()
	oldCopy := copaMirrorCopy
	oldDigest := copaMirrorDigest
	return func() {
		copaMirrorCopy = oldCopy
		copaMirrorDigest = oldDigest
	}
}
