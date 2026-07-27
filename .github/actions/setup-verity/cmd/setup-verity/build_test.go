package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCanonicalBuildConfig_is_linux_amd64_cgo0_with_reproducible_flags(t *testing.T) {
	// Given the reusable Verity production target.

	// When its build-key configuration is constructed.
	config := canonicalBuildConfig()

	// Then every declared compiler input is exact and deterministic.
	assert.Equal(t, "linux", config.GOOS)
	assert.Equal(t, "amd64", config.GOARCH)
	assert.Equal(t, "0", config.CGOEnabled)
	assert.Equal(t, []string{"-buildvcs=true", "-trimpath", metadataLDFlagsContract}, config.Flags)
}

func TestBuildArtifactName_includes_exact_build_key(t *testing.T) {
	// Given a full lowercase build key and current workflow-run identity.

	// When the immutable artifact name is derived.
	name, err := buildArtifactName(testActionBuildKey, 42, 2)

	// Then the platform, complete key, run ID, and attempt are present.
	require.NoError(t, err)
	assert.Equal(t, "verity-linux-amd64-"+testActionBuildKey+"-42-2", name)
}

func TestBuildArtifactName_rejects_malformed_identity(t *testing.T) {
	tests := []struct {
		name       string
		buildKey   string
		runID      int64
		runAttempt int64
	}{
		{name: "truncated build key", buildKey: strings.Repeat("a", 63), runID: 42, runAttempt: 2},
		{name: "missing run ID", buildKey: testActionBuildKey, runAttempt: 2},
		{name: "missing run attempt", buildKey: testActionBuildKey, runID: 42},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Given malformed build or workflow-run identity.

			// When an artifact name is requested.
			_, err := buildArtifactName(test.buildKey, test.runID, test.runAttempt)

			// Then mutable or stale names are rejected.
			require.Error(t, err)
			assert.ErrorIs(t, err, errUntrustedArtifact)
		})
	}
}

func TestBuildEnvironment_overrides_all_output_affecting_Go_controls(t *testing.T) {
	// Given hostile process and persisted Go environment controls.
	environment := []string{
		"PATH=/trusted/bin", "SECRET_TOKEN=do-not-serialize", "GOOS=darwin", "GOARCH=arm64",
		"GOAMD64=v4", "CGO_ENABLED=1", "GOFLAGS=-race", "GOENV=/tmp/hostile-go-env",
		"GOEXPERIMENT=arenas", "GOFIPS140=v1.0.0", "GOTOOLCHAIN=go1.99.0+auto", "GOWORK=/tmp/hostile.work",
	}

	// When the production build environment is created.
	actual := buildEnvironment(environment)

	// Then unrelated values remain available but every output-affecting Go control is canonical.
	assert.Contains(t, actual, "PATH=/trusted/bin")
	assert.Contains(t, actual, "SECRET_TOKEN=do-not-serialize")
	for _, expected := range canonicalBuildEnvironment {
		assert.Contains(t, actual, expected)
	}
	joined := strings.Join(actual, "\n")
	for _, hostile := range []string{
		"GOOS=darwin", "GOARCH=arm64", "GOAMD64=v4", "CGO_ENABLED=1", "GOFLAGS=-race",
		"GOENV=/tmp/hostile-go-env", "GOEXPERIMENT=arenas", "GOFIPS140=v1.0.0",
		"GOTOOLCHAIN=go1.99.0+auto", "GOWORK=/tmp/hostile.work",
	} {
		assert.NotContains(t, joined, hostile)
	}
}

func TestAppendBuildOutputs_emits_reusable_identity_without_private_values(t *testing.T) {
	// Given exact public build identity and an unrelated private value in the process.
	t.Setenv("PRIVATE_BUILD_TOKEN", "must-not-leak")
	outputPath := filepath.Join(t.TempDir(), "github-output")
	result := buildResult{
		ArtifactName: "verity-linux-amd64-" + testActionBuildKey + "-42-2",
		BuildKey:     testActionBuildKey,
		SourceSHA:    testActionSourceSHA,
	}

	// When reusable workflow outputs are written.
	err := appendBuildOutputs(outputPath, result)

	// Then only artifact name, build key, and source SHA are exposed.
	require.NoError(t, err)
	data, err := os.ReadFile(outputPath)
	require.NoError(t, err)
	assert.Equal(
		t,
		"artifact-name="+result.ArtifactName+"\nbuild-key="+testActionBuildKey+"\nsource-sha="+testActionSourceSHA+"\n",
		string(data),
	)
	assert.NotContains(t, string(data), "must-not-leak")
}
