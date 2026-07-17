package scripts_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestMetricsFinalizeWorkflowHasNoLossyConcurrencyQueue(t *testing.T) {
	// Given: the reusable metrics writer workflow.
	data, err := os.ReadFile(filepath.Join("..", ".github", "workflows", "metrics-finalize.yaml"))
	require.NoError(t, err)
	type concurrencyConfig struct {
		Group            string `yaml:"group"`
		CancelInProgress bool   `yaml:"cancel-in-progress"`
	}
	var workflow struct {
		Concurrency *concurrencyConfig `yaml:"concurrency"`
		Jobs        map[string]struct {
			Concurrency *concurrencyConfig `yaml:"concurrency"`
		} `yaml:"jobs"`
	}
	require.NoError(t, yaml.Unmarshal(data, &workflow))

	// Then: GitHub cannot replace an older pending archive producer.
	require.Nil(t, workflow.Concurrency)
	for name, job := range workflow.Jobs {
		require.Nil(t, job.Concurrency, "job %s must not use a replacing concurrency queue", name)
	}
}
