//go:build integration

package integration

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestCertManagerCSIDriverFixtureTracksChartVersion(t *testing.T) {
	// Given
	root, err := findRepoRoot()
	require.NoError(t, err)
	path := filepath.Join(root, "test", "chart-integration", "values", "cert-manager-csi-driver.yaml")
	body, err := os.ReadFile(path)
	require.NoError(t, err)
	var values map[string]struct {
		Image struct {
			Tag string `yaml:"tag"`
		} `yaml:"image"`
	}
	require.NoError(t, yaml.Unmarshal(body, &values))

	// When
	got := values["cert-manager-csi-driver"].Image.Tag

	// Then
	require.Equal(t, "0.15", got)
}
