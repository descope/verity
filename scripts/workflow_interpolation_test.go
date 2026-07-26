package scripts_test

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestVerityWorkflowCommandsUseEnvironmentInputs(t *testing.T) {
	directExpression := regexp.MustCompile(`(?s)\$\{\{[^}]*\b(?:inputs|matrix)\b[^}]*\}\}`)
	for _, expression := range []string{
		"${{inputs.image}}",
		"${{  matrix.arch }}",
		"${{ matrix['arch'] }}",
		"${{ fromJson(matrix.value) }}",
	} {
		require.True(t, directExpression.MatchString(expression), expression)
	}
	for _, filename := range []string{"integer-build-image.yaml", "pr-test.yaml"} {
		t.Run(filename, func(t *testing.T) {
			data, err := os.ReadFile(filepath.Join("..", ".github", "workflows", filename))
			require.NoError(t, err)
			var workflow struct {
				Jobs map[string]struct {
					Steps []struct {
						Name string `yaml:"name"`
						Run  string `yaml:"run"`
					} `yaml:"steps"`
				} `yaml:"jobs"`
			}
			require.NoError(t, yaml.Unmarshal(data, &workflow))

			for jobName, job := range workflow.Jobs {
				for _, step := range job.Steps {
					assert.NotRegexp(t, directExpression, step.Run, "%s/%s", jobName, step.Name)
				}
			}
		})
	}
}

func TestPRWorkflowUsesDistinctIntegerReportArtifactNames(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", ".github", "workflows", "pr-test.yaml"))
	require.NoError(t, err)
	var workflow struct {
		Jobs map[string]struct {
			Steps []struct {
				Name string            `yaml:"name"`
				Uses string            `yaml:"uses"`
				With map[string]string `yaml:"with"`
			} `yaml:"steps"`
		} `yaml:"jobs"`
	}
	require.NoError(t, yaml.Unmarshal(data, &workflow))

	owners := map[string]string{}
	for jobName, job := range workflow.Jobs {
		for _, step := range job.Steps {
			name := step.With["name"]
			if !strings.Contains(step.Uses, "actions/upload-artifact") || !strings.HasPrefix(name, "trivy-") {
				continue
			}
			if owner, exists := owners[name]; exists {
				t.Errorf("artifact name %q is shared by %s and %s/%s", name, owner, jobName, step.Name)
			}
			owners[name] = jobName + "/" + step.Name
		}
	}
	require.Contains(t, owners, "trivy-smoke-batch-${{ matrix.batch_id }}-${{ matrix.arch }}")
	require.Contains(t, owners, "trivy-build-batch-${{ matrix.batch_id }}-${{ matrix.arch }}")
}

func TestPRWorkflowDiffsBespokeLockForSelectiveImagePlanning(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", ".github", "workflows", "pr-test.yaml"))
	require.NoError(t, err)
	workflow := string(data)

	assert.Contains(t, workflow, "./verity ci pr-test plan-integer")
	assert.Contains(t, workflow, `--base-sha "$BASE_SHA"`)
	assert.Contains(t, workflow, `--temp-dir "$RUNNER_TEMP"`)
}

func TestPRWorkflowKeepsZeroVulnerabilityGateOnBuildAndSmoke(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", ".github", "workflows", "pr-test.yaml"))
	require.NoError(t, err)
	workflow := string(data)

	assert.Equal(t, 2, strings.Count(workflow, "./verity ci pr-test integer-batch"))
	assert.NotContains(t, workflow, `--exit-code 0`)
	assert.Contains(t, workflow, "--kind smoke")
	assert.Contains(t, workflow, "--kind build")
}

func TestPRWorkflowExercisesProductionPinningOnLinkerdCanary(t *testing.T) {
	// Given: the affected-image PR workflow.
	data, err := os.ReadFile(filepath.Join("..", ".github", "workflows", "pr-test.yaml"))
	require.NoError(t, err)
	workflow := string(data)

	// Then: each architecture uses its native runner and the typed build batch
	// owns the Linkerd production package-pinning canary.
	assert.NotContains(t, workflow, "docker/setup-qemu-action")
	assert.Contains(t, workflow, "./verity ci pr-test integer-batch")
	assert.Contains(t, workflow, "--kind build")
	assert.NotContains(t, workflow, `if [ "$image" = linkerd ]`)
}

func TestIntegerBuildWorkflowReadsMetadataThroughVerity(t *testing.T) {
	// Given: the thin production Integer build wrapper and reusable implementation.
	data, err := os.ReadFile(filepath.Join("..", ".github", "workflows", "integer-build-image.yaml"))
	require.NoError(t, err)
	wrapper := string(data)
	reusableData, err := os.ReadFile(filepath.Join("..", ".github", "workflows", "integer-build-image-reusable.yaml"))
	require.NoError(t, err)
	reusable := string(reusableData)

	// Then: the wrapper delegates to the reusable implementation, where metadata
	// resolution uses declared-name lookup in the Go CLI.
	assert.Contains(t, wrapper, "uses: ./.github/workflows/build-verity-protected.yaml")
	assert.Contains(t, wrapper, "uses: ./.github/workflows/integer-build-image-reusable.yaml")
	assert.Contains(t, wrapper, "verity_artifact_name: ${{ needs.build-verity.outputs.artifact-name }}")
	assert.Contains(t, reusable, `./verity integer metadata`)
	assert.NotContains(t, reusable, `image_yaml="images/${INPUT_IMAGE}.yaml"`)
	assert.NotContains(t, wrapper, `./verity integer metadata`)
}

func TestPRWorkflowRequiresDualArchitectureIntegerSecurityCompletion(t *testing.T) {
	// Given: the pull-request workflow for affected Integer images.
	data, err := os.ReadFile(filepath.Join("..", ".github", "workflows", "pr-test.yaml"))
	require.NoError(t, err)
	workflow := string(data)

	// Then: the typed planner feeds native runner matrices without QEMU.
	assert.Contains(t, workflow, "matrix: ${{ steps.detect.outputs.matrix }}")
	assert.Contains(t, workflow, "smoke-matrix: ${{ steps.detect.outputs.smoke-matrix }}")
	assert.Contains(t, workflow, "expected-matrix: ${{ steps.detect.outputs.expected-matrix }}")
	assert.Contains(t, workflow, "expected-smoke-matrix: ${{ steps.detect.outputs.expected-smoke-matrix }}")
	assert.NotContains(t, workflow, `for package_arch in x86_64 aarch64; do`)
	assert.Equal(t, 2, strings.Count(workflow, `runs-on: ${{ matrix.runner }}`))
	assert.Equal(t, 2, strings.Count(workflow, "./verity ci pr-test integer-batch"))
	assert.NotContains(t, workflow, "name: Set up QEMU for dual-architecture verification")

	// And: reports and completion evidence cannot collide across architectures.
	assert.Contains(t, workflow, `trivy-smoke-batch-${{ matrix.batch_id }}-${{ matrix.arch }}`)
	assert.Contains(t, workflow, `trivy-build-batch-${{ matrix.batch_id }}-${{ matrix.arch }}`)
	assert.Contains(t, workflow, `integer-security-smoke-batch-${{ matrix.batch_id }}-${{ matrix.arch }}`)
	assert.Contains(t, workflow, `integer-security-build-batch-${{ matrix.batch_id }}-${{ matrix.arch }}`)

	// And: exact matrices flow into the typed required result gate.
	assert.Contains(t, workflow, `EXPECTED_INTEGER_MATRIX: ${{ needs.detect-changed-images.outputs.expected-matrix }}`)
	assert.Contains(t, workflow, `EXPECTED_INTEGER_SMOKE_MATRIX: ${{ needs.detect-changed-images.outputs.expected-smoke-matrix }}`)
	assert.Contains(t, workflow, "./verity ci pr-test aggregate")
	assert.Contains(t, workflow, `--expected-integer-smoke-matrix "$EXPECTED_INTEGER_SMOKE_MATRIX"`)
	assert.Contains(t, workflow, `--expected-integer-matrix "$EXPECTED_INTEGER_MATRIX"`)
}

func TestPRWorkflowAggregateRejectsIncompleteIntegerArchitectureCoverage(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", ".github", "workflows", "pr-test.yaml"))
	require.NoError(t, err)
	workflow := string(data)

	// Then: the workflow delegates failure-closed matrix and marker checks to Go.
	assert.Contains(t, workflow, "./verity ci pr-test aggregate")
	assert.Contains(t, workflow, "--security-dir integer-security-results")
	assert.NotContains(t, workflow, "require_integer_markers()")
}
