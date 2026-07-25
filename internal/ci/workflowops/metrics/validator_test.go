package metrics

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateDirectory_accepts_shell_v1_success_record(t *testing.T) {
	// Given: the valid success record characterized by validate-metrics-json_test.sh.
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "metrics-example-1.2.3.json"), []byte(validMetricsJSON), 0o644))
	expected, err := NewExpectedRun(42, 3)
	require.NoError(t, err)

	// When: the typed validator checks the artifact directory.
	result, err := ValidateDirectory(t.Context(), expected, dir)

	// Then: the same record is accepted and counted once.
	require.NoError(t, err)
	assert.Equal(t, 1, result.Count)
}

func TestValidateDirectory_rejects_tampered_or_malformed_records(t *testing.T) {
	tests := []struct {
		name    string
		content string
	}{
		{name: "wrong run", content: strings.Replace(validMetricsJSON, `"id": 42`, `"id": 43`, 1)},
		{name: "severity total mismatch", content: strings.Replace(validMetricsJSON, `"vuln_count": 3`, `"vuln_count": 4`, 1)},
		{name: "missing platforms", content: strings.Replace(validMetricsJSON, `"platforms":`, `"removed_platforms":`, 1)},
		{name: "missing severity key", content: strings.Replace(validMetricsJSON, `"UNKNOWN": 0`, `"REMOVED": 0`, 1)},
		{name: "successful null target", content: strings.Replace(validMetricsJSON, `"target_ref": "ghcr.io/verity-org/example:1.2.3"`, `"target_ref": null`, 1)},
		{name: "fractional count", content: strings.Replace(validMetricsJSON, `"vuln_count": 3`, `"vuln_count": 3.5`, 1)},
		{name: "duplicate conflicting key", content: strings.Replace(validMetricsJSON, `"id": 42`, `"id": 999, "id": 42`, 1)},
		{name: "case variant key", content: strings.Replace(validMetricsJSON, `"id": 42`, `"ID": 42`, 1)},
		{name: "unknown key", content: strings.Replace(validMetricsJSON, `"id": 42`, `"id": 42, "trusted": true`, 1)},
		{name: "multiple documents", content: "{}\n" + validMetricsJSON},
		{name: "truncated JSON", content: validMetricsJSON[:len(validMetricsJSON)-3]},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Given: a metrics artifact altered at an external trust boundary.
			dir := t.TempDir()
			require.NoError(t, os.WriteFile(filepath.Join(dir, "metrics-example.json"), []byte(test.content), 0o644))
			expected, err := NewExpectedRun(42, 3)
			require.NoError(t, err)

			// When: validation parses the artifact.
			_, err = ValidateDirectory(t.Context(), expected, dir)

			// Then: invalid input fails closed.
			require.ErrorIs(t, err, ErrInvalidMetrics)
		})
	}
}

func TestValidateDirectory_accepts_sparse_failure_record(t *testing.T) {
	// Given: the shell-compatible failure form with nullable target and scans.
	content := strings.Replace(validMetricsJSON, `"conclusion": "success"`, `"conclusion": "failure"`, 1)
	content = strings.Replace(content, `"target_ref": "ghcr.io/verity-org/example:1.2.3"`, `"target_ref": null`, 1)
	content = strings.Replace(content, `"manifest_digest": "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"`, `"manifest_digest": null`, 1)
	start := strings.Index(content, `"scan": {`)
	end := strings.Index(content, `  "platforms": {`)
	require.GreaterOrEqual(t, start, 0)
	require.Greater(t, end, start)
	content = content[:start] + `"scan": {"before": null, "after": null},` + "\n  " + content[end:]
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "metrics-example.json"), []byte(content), 0o644))
	expected, err := NewExpectedRun(42, 3)
	require.NoError(t, err)

	// When: validation checks a failed workflow record.
	result, err := ValidateDirectory(t.Context(), expected, dir)

	// Then: nullable failure data remains valid parity behavior.
	require.NoError(t, err)
	assert.Equal(t, 1, result.Count)
}

func TestValidateDirectory_honors_cancellation_before_file_processing(t *testing.T) {
	// Given: a valid artifact and an already-cancelled operation.
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "metrics-example.json"), []byte(validMetricsJSON), 0o644))
	expected, err := NewExpectedRun(42, 3)
	require.NoError(t, err)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	// When: validation starts.
	_, err = ValidateDirectory(ctx, expected, dir)

	// Then: cancellation wins before artifact processing.
	require.ErrorIs(t, err, context.Canceled)
}

const validMetricsJSON = `{
  "schema_version": "v1",
  "run": {
    "id": 42,
    "attempt": 3,
    "started_at": "2026-07-14T00:00:00Z",
    "ended_at": "2026-07-14T00:01:00Z",
    "conclusion": "success"
  },
  "image": {
    "name": "example",
    "source_tag": "1.2.3",
    "target_ref": "ghcr.io/verity-org/example:1.2.3",
    "manifest_digest": "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
  },
  "scan": {
    "before": {
      "vuln_count": 3,
      "by_severity": {"CRITICAL": 1, "HIGH": 1, "MEDIUM": 1, "LOW": 0, "UNKNOWN": 0}
    },
    "after": {
      "vuln_count": 1,
      "by_severity": {"CRITICAL": 0, "HIGH": 1, "MEDIUM": 0, "LOW": 0, "UNKNOWN": 0}
    }
  },
  "platforms": {
    "amd64": {"arch": "amd64", "copa_duration_seconds": 12, "copa_exit_code": 0, "staging_digest": null},
    "arm64": null
  },
  "supply_chain": {
    "rekor_url": null,
    "attestation_id": null,
    "sbom_digest": null,
    "attestation_bundle_path": null
  }
}`
