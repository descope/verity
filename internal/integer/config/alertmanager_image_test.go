package config

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestAlertmanagerDefaultUsesApprovedBespokePackage(t *testing.T) {
	// Given: the Alertmanager image definition and its locked package recipe.
	_, file, _, ok := runtime.Caller(0)
	require.True(t, ok, "locating test file")
	repoRoot := filepath.Clean(filepath.Join(filepath.Dir(file), "..", "..", ".."))
	def, err := LoadImage(filepath.Join(repoRoot, "images", "alertmanager.yaml"))
	require.NoError(t, err)
	recipeData, err := os.ReadFile(filepath.Join(repoRoot, "packages", "bespoke", "locked", "prometheus-alertmanager.yaml"))
	require.NoError(t, err)

	var recipe struct {
		Package struct {
			Version   string `yaml:"version"`
			Epoch     int    `yaml:"epoch"`
			Copyright []struct {
				License string `yaml:"license"`
			} `yaml:"copyright"`
		} `yaml:"package"`
		Pipeline []struct {
			Uses string `yaml:"uses"`
			Name string `yaml:"name"`
			With struct {
				ExpectedCommit string `yaml:"expected-commit"`
				Deps           string `yaml:"deps"`
			} `yaml:"with"`
			Runs string `yaml:"runs"`
		} `yaml:"pipeline"`
	}
	require.NoError(t, yaml.Unmarshal(recipeData, &recipe))

	// When: the default image and recipe are checked against the approved matrix row.
	defaultType := def.Types["default"]
	require.NotNil(t, defaultType.Melange)

	// Then: the image builds the fixed, source-pinned Apache-2.0 package.
	require.Equal(t, "prometheus-alertmanager", defaultType.Melange.Upstream)
	require.Equal(t, "0.33.1", recipe.Package.Version)
	require.Equal(t, 4, recipe.Package.Epoch)
	require.Equal(t, "Apache-2.0", recipe.Package.Copyright[0].License)

	var checkoutCommit, bumpedDependencies, recoveredAssets string
	for _, step := range recipe.Pipeline {
		switch {
		case step.Uses == "git-checkout":
			checkoutCommit = step.With.ExpectedCommit
		case step.Uses == "go/bump":
			bumpedDependencies = step.With.Deps
		case step.Name == "Recover prebuilt UI dist from upstream binary":
			recoveredAssets = step.Runs
		}
	}
	require.Equal(t, "2c8da51e03f3dbbed24f9711ca2d76aab4eef9c5", checkoutCommit)
	require.Contains(t, bumpedDependencies, "go.opentelemetry.io/otel@v1.44.0")
	require.Contains(t, bumpedDependencies, "go.opentelemetry.io/otel/sdk@v1.44.0")
	require.Contains(t, bumpedDependencies, "golang.org/x/text@v0.39.0")
	require.True(t, strings.Contains(recoveredAssets, "93d802cba6a8d27239d747ce117df7648d326ab67394e32247540b030e9842ba") &&
		strings.Contains(recoveredAssets, "50b32a346c3c29da411643dfa84990440e8fd03800380d7e664f22a863b7a0cf"))
}
