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
	require.Contains(t, owners, "trivy-smoke-${{ matrix.image }}-${{ matrix.version }}-${{ matrix.type }}")
	require.Contains(t, owners, "trivy-build-${{ matrix.image }}-${{ matrix.version }}-${{ matrix.type }}")
}

func TestPRWorkflowDiffsBespokeLockForSelectiveImagePlanning(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", ".github", "workflows", "pr-test.yaml"))
	require.NoError(t, err)
	workflow := string(data)

	assert.Contains(t, workflow, `git show "${BASE_SHA}":packages/upstream.lock.json`)
	assert.Contains(t, workflow, `--base-upstream-lock "$RUNNER_TEMP/base-upstream.lock.json"`)
}

func TestPRWorkflowKeepsZeroVulnerabilityGateOnBuildAndSmoke(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", ".github", "workflows", "pr-test.yaml"))
	require.NoError(t, err)
	workflow := string(data)

	assert.Equal(t, 2, strings.Count(workflow, `--fail-on-severity "UNKNOWN,LOW,MEDIUM,HIGH,CRITICAL"`))
	assert.Equal(t, 2, strings.Count(workflow, `echo "Total vulnerabilities: ${TOTAL}"`))
}

func TestPRWorkflowExercisesProductionPinningOnLinkerdCanary(t *testing.T) {
	// Given: the affected-image PR workflow.
	data, err := os.ReadFile(filepath.Join("..", ".github", "workflows", "pr-test.yaml"))
	require.NoError(t, err)
	var workflow struct {
		Jobs map[string]struct {
			Steps []struct {
				Name string            `yaml:"name"`
				If   string            `yaml:"if"`
				Run  string            `yaml:"run"`
				Uses string            `yaml:"uses"`
				With map[string]string `yaml:"with"`
			} `yaml:"steps"`
		} `yaml:"jobs"`
	}
	require.NoError(t, yaml.Unmarshal(data, &workflow))

	// When: the changed-image build steps are inspected.
	var canary struct {
		Name string            `yaml:"name"`
		If   string            `yaml:"if"`
		Run  string            `yaml:"run"`
		Uses string            `yaml:"uses"`
		With map[string]string `yaml:"with"`
	}
	qemu := canary
	qemuIndex, canaryIndex := -1, -1
	for index, step := range workflow.Jobs["integer-build-changed"].Steps {
		switch step.Name {
		case "Set up QEMU for Linkerd pinning canary":
			qemu = step
			qemuIndex = index
		case "Exercise production package pinning":
			canary = step
			canaryIndex = index
		}
	}

	// Then: only Linkerd runs the real dual-architecture publish pinning path.
	const canaryCondition = "matrix.image == 'linkerd' && matrix.version == '25' && matrix.type == 'default'"
	require.Equal(t, "Set up QEMU for Linkerd pinning canary", qemu.Name)
	assert.Equal(t, canaryCondition, qemu.If)
	assert.Equal(t, "docker/setup-qemu-action@96fe6ef7f33517b61c61be40b68a1882f3264fb8", qemu.Uses)
	assert.Equal(t, "docker.io/tonistiigi/binfmt:latest@sha256:400a4873b838d1b89194d982c45e5fb3cda4593fbfd7e08a02e76b03b21166f0", qemu.With["image"])
	assert.Equal(t, "arm64", qemu.With["platforms"])
	assert.Less(t, qemuIndex, canaryIndex)
	require.Equal(t, "Exercise production package pinning", canary.Name)
	assert.Equal(t, canaryCondition, canary.If)
	assert.Contains(t, canary.Run, `--arch aarch64`)
	assert.Contains(t, canary.Run, `--staged`)
	assert.Contains(t, canary.Run, `./verity integer melange pin-config`)
	assert.Contains(t, canary.Run, `--repository packages/repo`)
	assert.Contains(t, canary.Run, `--arch x86_64`)
	assert.Contains(t, canary.Run, `apko build`)
	assert.Contains(t, canary.Run, `--repository-append "@local packages/repo"`)
	assert.Contains(t, canary.Run, `trivy image`)
}

func TestIntegerBuildWorkflowReadsMetadataThroughVerity(t *testing.T) {
	// Given: the production Integer build workflow.
	data, err := os.ReadFile(filepath.Join("..", ".github", "workflows", "integer-build-image.yaml"))
	require.NoError(t, err)
	workflow := string(data)

	// Then: metadata resolution uses declared-name lookup in the Go CLI.
	assert.Contains(t, workflow, `./verity integer metadata`)
	assert.NotContains(t, workflow, `image_yaml="images/${INPUT_IMAGE}.yaml"`)
}

func TestIntegerNightlyWorkflowAggregatesChildFailuresAndPublishesCurrentReports(t *testing.T) {
	// Given: the nightly parent and reusable Integer child workflows.
	parentData, err := os.ReadFile(filepath.Join("..", ".github", "workflows", "integer-orchestrator.yaml"))
	require.NoError(t, err)
	childData, err := os.ReadFile(filepath.Join("..", ".github", "workflows", "integer-build-image.yaml"))
	require.NoError(t, err)
	parent := string(parentData)
	child := string(childData)

	// When: nightly builds are dispatched through the parent matrix.
	// Then: each child is awaited, failures reach the parent conclusion, and
	// every terminal child state replaces any stale successful report.
	assert.Contains(t, parent, "uses: ./.github/workflows/integer-build-image.yaml")
	assert.Contains(t, parent, "fail-fast: false")
	assert.Contains(t, parent, "secrets: inherit")
	assert.Contains(t, parent, "batch_id: ${{ github.run_id }}")
	assert.Contains(t, parent, "CHILD_RESULT: ${{ needs.build.result }}")
	assert.Contains(t, parent, `[ "$CHILD_RESULT" = "skipped" ]`)
	assert.Contains(t, parent, "exit 1")

	assert.Contains(t, child, "workflow_call:")
	assert.Contains(t, child, "batch_id:")
	assert.Contains(t, child, "needs: [melange-prep, melange-build, build]")
	assert.Contains(t, child, "if: always()")
	assert.Contains(t, child, "MELANGE_PREP_RESULT")
	assert.Contains(t, child, "MELANGE_BUILD_RESULT")
	assert.Contains(t, child, "BUILD_RESULT")
	assert.Contains(t, child, "BATCH_ID")
	assert.Contains(t, child, "batch_id: $batch_id")
}
