package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func readGeneratedWorkflowFixture(t *testing.T, name string) string {
	t.Helper()

	data, err := os.ReadFile(filepath.Join("..", "internal", "ci", "workflowpolicy", "testdata", "valid", name))
	require.NoError(t, err)
	return string(data)
}

func requireIntegerImageReusablePackageGate(t *testing.T, workflow string) {
	t.Helper()

	require.Contains(t, workflow, "name: Integer Build Image Reusable")
	require.Contains(t, workflow, "  workflow_call:")
	require.Contains(t, workflow, "Test staged packages natively (${{ matrix.arch }})")
	require.Contains(t, workflow, "./verity ci integer-image test-packages")
	require.Contains(t, workflow, "verify-attestation: \"true\"")
}

func requireIntegerImageReusableImageGate(t *testing.T, workflow string) {
	t.Helper()

	require.Contains(t, workflow, "name: Integer Build Image Reusable")
	require.Contains(t, workflow, "  workflow_call:")
	require.Contains(t, workflow, "Trivy publish gate (not clean, no go)")
	require.Contains(t, workflow, "--fail-on-severity UNKNOWN,LOW,MEDIUM,HIGH,CRITICAL")
	require.Contains(t, workflow, "Stage, scan, and publish exact multi-arch image")
	require.Contains(t, workflow, "Sign image with cosign (keyless)")
	require.Contains(t, workflow, "Attest SBOM")
	require.Contains(t, workflow, "predicate-type: https://spdx.dev/Document")
}
