package cmd

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestRunPRMelangePackageTest_usesArchitectureKeyring(t *testing.T) {
	// Given: an aarch64 package test and a recording command runner.
	runner := &canaryPRIntegerRunner{}
	request := prIntegerBatchRequest{
		PackageArchitecture: "aarch64",
		RepoRoot:            t.TempDir(),
	}

	// When: the package test command is assembled.
	err := runPRMelangePackageTest(
		t.Context(),
		&prIntegerDependencies{Commands: runner},
		&request,
		"tempo-2.10.yaml",
		"tempo-2.10",
		time.Minute,
	)

	// Then: Melange trusts the staged key for the requested architecture.
	require.NoError(t, err)
	require.Len(t, runner.calls, 1)
	require.True(t, containsArguments(
		runner.calls[0].Args,
		"--keyring-append",
		filepath.Join(request.RepoRoot, "packages", "repo", "melange-aarch64.rsa.pub"),
	))
}

func TestPRMelangeBuildTimeout_selectsSourceBuildBudget(t *testing.T) {
	tests := []struct {
		name  string
		entry prIntegerEntry
		want  time.Duration
	}{
		{
			name:  "ordinary Integer target",
			entry: prIntegerEntry{Image: "caddy", Version: "2", Type: "default"},
			want:  30 * time.Minute,
		},
		{
			name:  "dotnet 8 source build",
			entry: prIntegerEntry{Image: "dotnet", Version: "8", Type: "aspnet"},
			want:  90 * time.Minute,
		},
		{
			name:  "dotnet version without local source rebuild",
			entry: prIntegerEntry{Image: "dotnet", Version: "9", Type: "aspnet"},
			want:  30 * time.Minute,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			require.Equal(t, test.want, prMelangeBuildTimeout(test.entry))
		})
	}
}
