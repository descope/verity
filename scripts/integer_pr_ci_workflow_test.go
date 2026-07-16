package scripts_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestPRWorkflowIntegerJobsRetainSecurityCoverageAndCacheTrivyDatabase(t *testing.T) {
	// Given: the Integer PR workflow.
	data, err := os.ReadFile(filepath.Join("..", ".github", "workflows", "pr-test.yaml"))
	require.NoError(t, err)
	workflow := string(data)
	parsed := loadPRWorkflow(t)

	// Then: both matrices still expand to native amd64 and arm64 legs.
	assert.Equal(t, 2, strings.Count(workflow, `["amd64", "arm64"][] as $arch`))
	assert.Equal(t, 2, strings.Count(workflow, `runs-on: ${{ matrix.runner }}`))
	assert.GreaterOrEqual(t, strings.Count(workflow, "ubuntu-24.04-arm"), 2)
	assert.Contains(t, workflow, "needs.detect-changed-images.outputs.smoke-has-changes == 'true'")

	for _, jobName := range []string{"integer-smoke-test", "integer-build-changed"} {
		job := parsed.Jobs[jobName]
		var runs strings.Builder
		var cache workflowStep
		for _, step := range job.Steps {
			runs.WriteString(step.Run)
			if step.Name == "Cache Trivy database" {
				cache = step
			}
		}

		// And: strict runtime and Trivy coverage remains in each build path.
		assert.Contains(t, runs.String(), `--fail-on-severity "UNKNOWN,LOW,MEDIUM,HIGH,CRITICAL"`, jobName)
		assert.Contains(t, runs.String(), `docker image inspect "$loaded_ref"`, jobName)
		assert.Contains(t, runs.String(), `--severity UNKNOWN,LOW,MEDIUM,HIGH,CRITICAL`, jobName)

		// And: each job uses the pinned repository cache pattern with a Trivy
		// version and UTC date key, without sharing databases across versions.
		require.Equal(t, "actions/cache@55cc8345863c7cc4c66a329aec7e433d2d1c52a9", cache.Uses, jobName)
		assert.Equal(t, "~/.cache/trivy", cache.With["path"], jobName)
		assert.Equal(t, `trivy-db-${{ steps.trivy-cache-key.outputs.version }}-${{ steps.trivy-cache-key.outputs.date }}`, cache.With["key"], jobName)
		assert.Equal(t, `trivy-db-${{ steps.trivy-cache-key.outputs.version }}-`, cache.With["restore-keys"], jobName)
	}
}

func TestPRWorkflowAggregateAcceptsSkippedEmptySmokeMatrixWithoutRelaxingBuildEvidence(t *testing.T) {
	// Given: a strict-build-only plan with no remaining smoke-only variants.
	aggregate := prAggregateScript(t)
	buildMatrix := `{"include":[{"image":"demo","version":"1","type":"default"}]}`
	emptySmokeMatrix := `{"include":[]}`
	env := append(
		os.Environ(),
		"CHANGES_RESULT=success", "INTEGER=true", "COPA=false",
		"DISCOVER_RESULT=success", "VALIDATE_RESULT=success",
		"DETECT_INTEGER_RESULT=success", "INTEGER_HAS_CHANGES=true",
		"INTEGER_SMOKE_RESULT=skipped", "INTEGER_BUILD_RESULT=success",
		"DETECT_COPA_RESULT=success", "COPA_CHANGED_RESULT=success", "COPA_REGRESSION_RESULT=success",
		"EXPECTED_INTEGER_MATRIX="+buildMatrix, "EXPECTED_INTEGER_SMOKE_MATRIX="+emptySmokeMatrix,
	)
	dir := t.TempDir()
	require.NoError(t, os.Mkdir(filepath.Join(dir, "integer-security-results"), 0o755))
	for _, arch := range []string{"amd64", "arm64"} {
		marker := filepath.Join(dir, "integer-security-results", "build-demo-1-default-"+arch+".passed")
		require.NoError(t, os.WriteFile(marker, nil, 0o600))
	}

	// When: final aggregation evaluates the independently accurate matrices.
	command := exec.CommandContext(t.Context(), "bash", "-c", aggregate)
	command.Dir = dir
	command.Env = env
	output, runErr := command.CombinedOutput()

	// Then: an empty smoke matrix requires a skipped smoke job while both
	// native strict-build markers remain mandatory.
	require.NoError(t, runErr, string(output))
}

func prAggregateScript(t *testing.T) string {
	t.Helper()
	workflow := loadPRWorkflow(t)
	for _, step := range workflow.Jobs["pr-test-result"].Steps {
		if step.Name == "Aggregate PR test results" {
			return step.Run
		}
	}
	require.FailNow(t, "aggregate step not found")
	return ""
}

type workflowStep struct {
	Name string            `yaml:"name"`
	Run  string            `yaml:"run"`
	Uses string            `yaml:"uses"`
	With map[string]string `yaml:"with"`
}

type prWorkflow struct {
	Jobs map[string]struct {
		Steps []workflowStep `yaml:"steps"`
	} `yaml:"jobs"`
}

func loadPRWorkflow(t *testing.T) prWorkflow {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", ".github", "workflows", "pr-test.yaml"))
	require.NoError(t, err)
	var workflow prWorkflow
	require.NoError(t, yaml.Unmarshal(data, &workflow))
	return workflow
}
