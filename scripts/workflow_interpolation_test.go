package scripts_test

import (
	"os"
	"os/exec"
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

	assert.Contains(t, workflow, `git show "${BASE_SHA}":packages/upstream.lock.json`)
	assert.Contains(t, workflow, `--base-upstream-lock "$RUNNER_TEMP/base-upstream.lock.json"`)
}

func TestPRWorkflowKeepsZeroVulnerabilityGateOnBuildAndSmoke(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", ".github", "workflows", "pr-test.yaml"))
	require.NoError(t, err)
	workflow := string(data)

	assert.Equal(t, 2, strings.Count(workflow, `--fail-on-severity "UNKNOWN,LOW,MEDIUM,HIGH,CRITICAL"`))
	assert.NotContains(t, workflow, `--exit-code 0`)
	assert.GreaterOrEqual(t, strings.Count(workflow, `--exit-code 1`), 3)
	assert.Equal(t, 2, strings.Count(workflow, `echo "${image}:${version}-${type} ${arch}: Total vulnerabilities: ${total}"`))
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
	var batch struct {
		Name string            `yaml:"name"`
		If   string            `yaml:"if"`
		Run  string            `yaml:"run"`
		Uses string            `yaml:"uses"`
		With map[string]string `yaml:"with"`
	}
	batchIndex := -1
	for index, step := range workflow.Jobs["integer-build-changed"].Steps {
		if step.Name == "Build and verify native-architecture Integer batch" {
			batch = step
			batchIndex = index
		}
	}

	// Then: each architecture uses its native runner and only Linkerd runs
	// the production package-pinning canary inside the batch.
	assert.NotContains(t, string(data), "docker/setup-qemu-action")
	assert.Contains(t, string(data), "ubuntu-24.04-arm")
	assert.NotEqual(t, -1, batchIndex)
	require.Equal(t, "Build and verify native-architecture Integer batch", batch.Name)
	assert.NotContains(t, batch.Run, `for package_arch in x86_64 aarch64; do`)
	assert.Contains(t, batch.Run, `timeout --signal=TERM --kill-after=1m 30m`)
	assert.Contains(t, batch.Run, `--arch "$INTEGER_PACKAGE_ARCH"`)
	assert.Contains(t, batch.Run, `[ "$image" = linkerd ] && [ "$version" = 25 ] && [ "$type" = default ]`)
	assert.Contains(t, batch.Run, `--staged`)
	assert.Contains(t, batch.Run, `./verity integer melange pin-config`)
	assert.Contains(t, batch.Run, `--repository packages/repo`)
	assert.Contains(t, batch.Run, `--arch "$INTEGER_PACKAGE_ARCH"`)
	assert.Contains(t, batch.Run, `apko build`)
	assert.Contains(t, batch.Run, `--repository-append "@local packages/repo"`)
	assert.Contains(t, batch.Run, `trivy image`)
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

func TestPRWorkflowRequiresDualArchitectureIntegerSecurityCompletion(t *testing.T) {
	// Given: the pull-request workflow for affected Integer images.
	data, err := os.ReadFile(filepath.Join("..", ".github", "workflows", "pr-test.yaml"))
	require.NoError(t, err)
	workflow := string(data)

	// Then: every selected image entry executes both architectures without
	// multiplying the matrix beyond GitHub's 256-job limit.
	assert.Equal(t, 2, strings.Count(workflow, `["amd64", "arm64"][] as $arch`))
	assert.Equal(t, 2, strings.Count(workflow, `group_by((.key / 16 | floor))`))
	assert.Contains(t, workflow, `expected-matrix=${expected_matrix}`)
	assert.Contains(t, workflow, `expected-smoke-matrix=${expected_smoke_matrix}`)
	assert.NotContains(t, workflow, `for package_arch in x86_64 aarch64; do`)
	assert.Equal(t, 2, strings.Count(workflow, `runs-on: ${{ matrix.runner }}`))
	assert.GreaterOrEqual(t, strings.Count(workflow, `ubuntu-24.04-arm`), 2)
	assert.Equal(t, 1, strings.Count(workflow, `for arch in amd64 arm64; do`))
	assert.Equal(t, 2, strings.Count(workflow, `docker load --input "$tar_path"`))
	assert.Equal(t, 2, strings.Count(workflow, `docker image inspect "$loaded_ref"`))
	assert.Equal(t, 2, strings.Count(workflow, `docker load did not report an image reference`))
	assert.Equal(t, 2, strings.Count(workflow, `runtime architecture mismatch`))
	assert.NotContains(t, workflow, `name: Set up QEMU for dual-architecture verification`)

	// And: reports and completion evidence cannot collide across architectures.
	assert.Contains(t, workflow, `trivy-smoke-batch-${{ matrix.batch_id }}-${{ matrix.arch }}`)
	assert.Contains(t, workflow, `trivy-build-batch-${{ matrix.batch_id }}-${{ matrix.arch }}`)
	assert.Contains(t, workflow, `integer-security-smoke-batch-${{ matrix.batch_id }}-${{ matrix.arch }}`)
	assert.Contains(t, workflow, `integer-security-build-batch-${{ matrix.batch_id }}-${{ matrix.arch }}`)

	// And: the required aggregate verifies every expected marker, so an absent,
	// skipped, cancelled, or failed arm64 leg cannot be reported as success.
	assert.Contains(t, workflow, `EXPECTED_INTEGER_MATRIX: ${{ needs.detect-changed-images.outputs.expected-matrix }}`)
	assert.Contains(t, workflow, `EXPECTED_INTEGER_SMOKE_MATRIX: ${{ needs.detect-changed-images.outputs.expected-smoke-matrix }}`)
	assert.Contains(t, workflow, `for arch in amd64 arm64; do`)
	assert.Contains(t, workflow, `::error::missing successful Integer ${kind} security leg: ${image}:${version}-${type} (${arch})`)
	assert.Contains(t, workflow, `require_integer_markers "$EXPECTED_INTEGER_SMOKE_MATRIX" smoke`)
	assert.Contains(t, workflow, `require_integer_markers "$EXPECTED_INTEGER_MATRIX" build`)
}

func TestPRWorkflowAggregateRejectsIncompleteIntegerArchitectureCoverage(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", ".github", "workflows", "pr-test.yaml"))
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

	var aggregate string
	for _, step := range workflow.Jobs["pr-test-result"].Steps {
		if step.Name == "Aggregate PR test results" {
			aggregate = step.Run
		}
	}
	require.NotEmpty(t, aggregate)

	matrix := `{"include":[{"image":"demo","version":"1","type":"default"}]}`
	baseEnv := append(os.Environ(),
		"CHANGES_RESULT=success", "INTEGER=true", "COPA=false",
		"DISCOVER_RESULT=success", "VALIDATE_RESULT=success",
		"DETECT_INTEGER_RESULT=success", "INTEGER_HAS_CHANGES=true",
		"INTEGER_SMOKE_RESULT=success", "INTEGER_BUILD_RESULT=success",
		"DETECT_COPA_RESULT=success", "COPA_CHANGED_RESULT=success", "COPA_REGRESSION_RESULT=success",
		"EXPECTED_INTEGER_MATRIX="+matrix, "EXPECTED_INTEGER_SMOKE_MATRIX="+matrix,
	)

	runAggregate := func(t *testing.T, env []string, missingMarker string) (string, error) {
		t.Helper()
		dir := t.TempDir()
		require.NoError(t, os.Mkdir(filepath.Join(dir, "integer-security-results"), 0o755))
		for _, kind := range []string{"smoke", "build"} {
			for _, arch := range []string{"amd64", "arm64"} {
				name := kind + "-demo-1-default-" + arch + ".passed"
				if name == missingMarker {
					continue
				}
				require.NoError(t, os.WriteFile(filepath.Join(dir, "integer-security-results", name), nil, 0o600))
			}
		}
		command := exec.CommandContext(t.Context(), "bash", "-c", aggregate)
		command.Dir = dir
		command.Env = env
		output, runErr := command.CombinedOutput()
		return string(output), runErr
	}

	t.Run("complete dual architecture evidence passes", func(t *testing.T) {
		output, runErr := runAggregate(t, baseEnv, "")
		require.NoError(t, runErr, output)
	})

	t.Run("missing arm64 evidence fails", func(t *testing.T) {
		output, runErr := runAggregate(t, baseEnv, "build-demo-1-default-arm64.passed")
		require.Error(t, runErr)
		assert.Contains(t, output, "missing successful Integer build security leg: demo:1-default (arm64)")
	})

	for _, result := range []string{"skipped", "cancelled", "failure"} {
		t.Run(result+" matrix job fails", func(t *testing.T) {
			env := append([]string{}, baseEnv...)
			env = append(env, "INTEGER_BUILD_RESULT="+result)
			output, runErr := runAggregate(t, env, "")
			require.Error(t, runErr)
			assert.Contains(t, output, "integer-build-changed did not succeed: "+result)
		})
	}

	for name, invalidMatrix := range map[string]string{
		"malformed":       `{`,
		"null":            `null`,
		"missing include": `{}`,
	} {
		t.Run(name+" expected matrix fails closed", func(t *testing.T) {
			env := append([]string{}, baseEnv...)
			env = append(env, "EXPECTED_INTEGER_MATRIX="+invalidMatrix)
			output, runErr := runAggregate(t, env, "")
			require.Error(t, runErr)
			assert.Contains(t, output, "invalid expected Integer build matrix")
		})
	}
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
	assert.Contains(t, parent, "batch_id: ${{ github.run_id }}-${{ github.run_attempt }}")
	assert.Contains(t, parent, `github.event_name }}" = "schedule"`)
	assert.Contains(t, parent, "PLAN_ARGS+=(--force)")
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
