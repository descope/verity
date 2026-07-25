package cmd

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/urfave/cli/v3"
)

func TestCIWorkflowOpsCommand_registers_typed_script_replacements(t *testing.T) {
	// Given: the workflow operations command group.
	wanted := []string{
		"archive-metrics",
		"resolve-metrics-producer",
		"push-reports",
		"retry-command",
		"retry-go-build",
		"retry-docker-login",
		"validate-metrics-json",
		"wait-for-workflows",
		"aggregate-integer-results",
	}

	// When: public subcommands are enumerated.
	registered := make(map[string]bool)
	for _, command := range ciWorkflowOpsCommand.Commands {
		registered[command.Name] = true
	}

	// Then: every assigned shell operation has a typed Go command.
	for _, name := range wanted {
		assert.True(t, registered[name], "missing workflowops command %q", name)
	}
}

func TestCIWorkflowOpsCommand_resolves_exact_metrics_producer(t *testing.T) {
	// Given: GitHub returns the requested run attempt and one exact metrics artifact.
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		switch request.URL.Path {
		case "/repos/verity-org/verity/actions/runs/42/attempts/3":
			_, err := writer.Write([]byte(`{
  "id":42,"run_attempt":3,"status":"in_progress","conclusion":null,
  "created_at":"2026-07-17T06:00:00Z","html_url":"https://github.com/verity-org/verity/actions/runs/42",
  "event":"workflow_call","display_title":"Patch nginx","head_branch":"main",
  "head_sha":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
  "head_repository":{"full_name":"verity-org/verity"}
}`))
			assert.NoError(t, err)
		case "/repos/verity-org/verity/actions/runs/42/artifacts":
			_, err := writer.Write([]byte(`{
  "total_count":1,
  "artifacts":[{"id":8,"name":"metrics-nginx-1.2.3","expired":false,
    "created_at":"2026-07-17T06:02:00Z",
    "workflow_run":{"id":42,"head_sha":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}}]
}`))
			assert.NoError(t, err)
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()
	outputPath := filepath.Join(t.TempDir(), "github-output")
	root := &cli.Command{Commands: []*cli.Command{CICommand}}

	// When: the public command resolves immutable producer metadata.
	err := root.Run(t.Context(), []string{
		"verity", "ci", "workflowops", "resolve-metrics-producer",
		"--repository", "verity-org/verity", "--github-token", "token", "--github-api-url", server.URL,
		"--run-id", "42", "--run-attempt", "3",
		"--source-sha", "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		"--artifact-name", "metrics-nginx-1.2.3", "--github-output", outputPath,
	})

	// Then: downstream steps receive only the exact validated identity and timestamp.
	require.NoError(t, err)
	output, err := os.ReadFile(outputPath)
	require.NoError(t, err)
	assert.Equal(t, "run-id=42\nrun-attempt=3\nartifact-name=metrics-nginx-1.2.3\nrun-created-at=2026-07-17T06:00:00Z\n", string(output))
}

func TestMetricsFinalizeWorkflow_uses_exact_typed_producer_metadata(t *testing.T) {
	// Given
	workflow, err := os.ReadFile(filepath.Join("..", ".github", "workflows", "metrics-finalize.yaml"))
	require.NoError(t, err)
	text := string(workflow)

	// Then
	assert.Contains(t, text, "artifact-name:\n        description: \"Exact producer metrics artifact name\"")
	assert.Contains(t, text, "./verity ci workflowops resolve-metrics-producer")
	assert.Contains(t, text, "run-id: ${{ steps.metadata.outputs.run-id }}")
	assert.Contains(t, text, "name: ${{ steps.metadata.outputs.artifact-name }}")
	assert.NotContains(t, text, "pattern: metrics-*")
	assert.NotContains(t, text, "gh api")
	assert.NotContains(t, text, "Resolve workflow creation time")
}

func TestCIWorkflowOpsCommand_validates_real_metrics_fixture(t *testing.T) {
	// Given: a real metrics artifact and the public CI command tree.
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "metrics-example.json"), []byte(cliMetricsFixture), 0o644))
	root := &cli.Command{Commands: []*cli.Command{CICommand}}

	// When: validation runs through verity ci workflowops.
	err := root.Run(t.Context(), []string{"verity", "ci", "workflowops", "validate-metrics-json", "42", "3", dir})

	// Then: the fixture is accepted through the real CLI surface.
	require.NoError(t, err)
}

func TestCIWorkflowOpsCommand_rejects_duplicate_metrics_key(t *testing.T) {
	// Given: a metrics artifact with a conflicting duplicate run identity key.
	dir := t.TempDir()
	tampered := strings.Replace(cliMetricsFixture, `"id":42`, `"id":999,"id":42`, 1)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "metrics-example.json"), []byte(tampered), 0o644))
	root := &cli.Command{Commands: []*cli.Command{CICommand}}

	// When: validation runs through the public CLI surface.
	err := root.Run(t.Context(), []string{"verity", "ci", "workflowops", "validate-metrics-json", "42", "3", dir})

	// Then: hostile JSON is rejected.
	require.Error(t, err)
}

func TestCIWorkflowOpsCommand_bounds_wait_API_by_declared_timeout(t *testing.T) {
	// Given: a GitHub-compatible API whose first request hangs until cancellation.
	server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, request *http.Request) {
		<-request.Context().Done()
	}))
	defer server.Close()
	root := &cli.Command{Commands: []*cli.Command{CICommand}}
	started := time.Now()

	// When: the public wait command declares a one-second total timeout.
	err := root.Run(t.Context(), []string{
		"verity", "ci", "workflowops", "wait-for-workflows",
		"--repository", "verity-org/verity", "--github-token", "x", "--github-api-url", server.URL,
		"--branch", "main", "--lookback-hours", "1", "--timeout-seconds", "1", "--interval-seconds", "1",
		"--api-timeout", "5s", "--event", "workflow_dispatch", "--batch-id", "42-1", "--expected-runs", "1",
		"--source-sha", "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", "test.yaml",
	})

	// Then: the in-flight API call is cancelled near one second, not after the server delay.
	require.ErrorContains(t, err, "producer wait timed out")
	require.Less(t, time.Since(started), 1500*time.Millisecond)
}

const cliMetricsFixture = `{
  "schema_version":"v1",
  "run":{"id":42,"attempt":3,"started_at":"2026-07-14T00:00:00Z","ended_at":"2026-07-14T00:01:00Z","conclusion":"success"},
  "image":{"name":"example","source_tag":"1.2.3","target_ref":"ghcr.io/verity-org/example:1.2.3","manifest_digest":"sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"},
  "scan":{
    "before":{"vuln_count":0,"by_severity":{"CRITICAL":0,"HIGH":0,"MEDIUM":0,"LOW":0,"UNKNOWN":0}},
    "after":{"vuln_count":0,"by_severity":{"CRITICAL":0,"HIGH":0,"MEDIUM":0,"LOW":0,"UNKNOWN":0}}
  },
  "platforms":{"amd64":null,"arm64":null},
  "supply_chain":{"rekor_url":null,"attestation_id":null,"sbom_digest":null,"attestation_bundle_path":null}
}`
