package scripts_test

import (
	"os"
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

	// Then: both matrices still execute on their native architecture runners.
	assert.Equal(t, 2, strings.Count(workflow, `family=${{ matrix.arch == 'amd64' && 'c8i+m8i' || 'c8g+m8g' }}/cpu=32/ram=64/image=ubuntu24-full-${{ matrix.arch == 'amd64' && 'x64' || 'arm64' }}/volume=200gb:gp3`))
	assert.NotContains(t, workflow, `runs-on: ${{ matrix.runner }}`)
	assert.Contains(t, workflow, "needs.detect-changed-images.outputs.smoke-has-changes == 'true'")

	for _, test := range []struct {
		jobName string
		kind    string
	}{
		{jobName: "integer-smoke-test", kind: "smoke"},
		{jobName: "integer-build-changed", kind: "build"},
	} {
		job := parsed.Jobs[test.jobName]
		var cache workflowStep
		var batch workflowStep
		var cacheKey workflowStep
		for _, step := range job.Steps {
			switch step.Name {
			case "Cache Trivy database":
				cache = step
			case "Get Trivy cache key":
				cacheKey = step
			case "Build and verify native-architecture Integer smoke batch", "Build and verify native-architecture Integer batch":
				batch = step
			}
		}

		// And: typed execution preserves strict native inputs for each path.
		assert.Contains(t, batch.Run, "./verity ci pr-test integer-batch", test.jobName)
		assert.Contains(t, batch.Run, "--kind "+test.kind, test.jobName)
		assert.Contains(t, batch.Run, `--arch "$INTEGER_ARCH"`, test.jobName)
		assert.Contains(t, batch.Run, `--package-arch "$INTEGER_PACKAGE_ARCH"`, test.jobName)
		assert.Contains(t, cacheKey.Run, "./verity ci pr-test trivy-cache-key", test.jobName)

		// And: each job uses the pinned repository cache pattern with a Trivy
		// version and UTC date key, without sharing databases across versions.
		require.Equal(t, "actions/cache@55cc8345863c7cc4c66a329aec7e433d2d1c52a9", cache.Uses, test.jobName)
		assert.Equal(t, "~/.cache/trivy", cache.With["path"], test.jobName)
		assert.Equal(t, `trivy-db-${{ steps.trivy-cache-key.outputs.version }}-${{ steps.trivy-cache-key.outputs.date }}`, cache.With["key"], test.jobName)
		assert.Equal(t, `trivy-db-${{ steps.trivy-cache-key.outputs.version }}-`, cache.With["restore-keys"], test.jobName)
	}
}

func TestPRWorkflowAggregateUsesTypedExactEvidenceGate(t *testing.T) {
	// Given: the required final PR result job.
	workflow := loadPRWorkflow(t)

	// When: its aggregate step is inspected.
	var aggregate workflowStep
	for _, step := range workflow.Jobs["pr-test-result"].Steps {
		if step.Name == "Aggregate PR test results" {
			aggregate = step
		}
	}

	// Then: exact matrices and marker directory flow into the typed evaluator.
	require.Contains(t, aggregate.Run, "./verity ci pr-test aggregate")
	require.Contains(t, aggregate.Run, `--expected-integer-matrix "$EXPECTED_INTEGER_MATRIX"`)
	require.Contains(t, aggregate.Run, `--expected-integer-smoke-matrix "$EXPECTED_INTEGER_SMOKE_MATRIX"`)
	require.Contains(t, aggregate.Run, "--security-dir integer-security-results")
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
