package patchimage

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/verity-org/verity/internal/ci/workflowops/retry"
)

func TestReadPlatformSet_treatsMissingAndMalformedFilesAsNull(t *testing.T) {
	// Given
	directory := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(directory, "platform-amd64.json"), []byte(`{"arch":"amd64"}`), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(directory, "platform-arm64.json"), []byte(`{broken`), 0o600))

	// When
	platforms := ReadPlatformSet(directory)

	// Then
	assert.JSONEq(t, `{"arch":"amd64"}`, string(platforms.AMD64))
	assert.Equal(t, json.RawMessage("null"), platforms.ARM64)
}

func TestWorkflowStartService_Fetch_parsesRunTimestamp(t *testing.T) {
	// Given
	runner := &fakeRunner{results: []runnerResult{{result: retry.Result{Stdout: []byte(`{"run_started_at":"2026-07-25T10:00:00Z"}`)}}}}

	// When
	startedAt, err := (WorkflowStartService{Runner: runner}).Fetch(t.Context(), "verity-org/verity", "42")

	// Then
	require.NoError(t, err)
	assert.Equal(t, "2026-07-25T10:00:00Z", startedAt)
}
