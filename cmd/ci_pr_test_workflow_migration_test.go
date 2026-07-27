package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPRTestWorkflow_routes_owned_logic_through_typed_commands(t *testing.T) {
	// Given: the migrated pull-request workflow.
	data, err := os.ReadFile(filepath.Join("..", ".github", "workflows", "pr-test.yaml"))
	require.NoError(t, err)
	workflow := string(data)

	// When: the remaining executable surface is inspected.

	// Then: Integer execution/cache and final aggregation are typed commands.
	require.Equal(t, 2, strings.Count(workflow, "./verity ci pr-test trivy-cache-key"))
	require.Equal(t, 2, strings.Count(workflow, "./verity ci pr-test integer-batch"))
	require.Contains(t, workflow, "./verity ci pr-test aggregate")
	require.Contains(t, workflow, "--kind smoke")
	require.Contains(t, workflow, "--kind build")

	// And: Copa catalog, scan, patch, digest pinning, and verification are typed.
	require.Equal(t, 2, strings.Count(workflow, "./verity ci pr-test copa-metadata"))
	require.Equal(t, 2, strings.Count(workflow, "./verity ci repository-ops scan-before"))
	require.Equal(t, 2, strings.Count(workflow, "./verity ci repository-ops patch-image"))
	require.Equal(t, 2, strings.Count(workflow, "./verity ci pr-test copa-pin"))
	require.Equal(t, 2, strings.Count(workflow, "./verity ci repository-ops verify-patched"))
	require.NotContains(t, workflow, ".github/scripts/")

	// And: every runtime service image is digest-bound before use.
	require.Equal(t, 2, strings.Count(workflow, "moby/buildkit:v0.21.1@sha256:"))
	require.Equal(t, 2, strings.Count(workflow, "registry:2@sha256:"))
	require.NotContains(t, workflow, "image=moby/buildkit:v0.21.1 \\")
	require.NotContains(t, workflow, "docker image inspect \"$loaded_ref\"")
}
