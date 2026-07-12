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
