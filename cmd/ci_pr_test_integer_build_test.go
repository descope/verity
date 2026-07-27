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
