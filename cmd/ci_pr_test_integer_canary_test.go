package cmd

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

type canaryPRIntegerRunner struct {
	calls []prCommandRequest
}

func (r *canaryPRIntegerRunner) Run(_ context.Context, request *prCommandRequest) (prCommandResult, error) {
	r.calls = append(r.calls, *request)
	if containsArguments(request.Args, "integer", "melange", "pin-config") {
		path := argumentAfter(request.Args, "--config")
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return prCommandResult{}, err
		}
		if err := os.WriteFile(path, []byte("contents:\n  packages: [linkerd2-cli=25.0.0-r0@local]\n"), 0o600); err != nil {
			return prCommandResult{}, err
		}
	}
	if request.Name == "apko" {
		if err := os.WriteFile(request.Args[len(request.Args)-1], []byte("image"), 0o600); err != nil {
			return prCommandResult{}, err
		}
	}
	if request.Name == "trivy" {
		if err := os.WriteFile(argumentAfter(request.Args, "--output"), []byte(`{"Results":[]}`), 0o600); err != nil {
			return prCommandResult{}, err
		}
	}
	return prCommandResult{}, nil
}

func TestRunPRLinkerdCanary_uses_staged_local_pin_and_strict_scan(t *testing.T) {
	// Given: the exact Linkerd changed-image variant and fake build tools.
	runner := &canaryPRIntegerRunner{}
	request := prIntegerBatchRequest{
		Kind: prIntegerBatchBuild, Architecture: "arm64", PackageArchitecture: "aarch64",
		RepoRoot: t.TempDir(), VerityPath: "./verity",
	}

	// When: the production pinning canary executes.
	err := runPRLinkerdCanary(t.Context(), &prIntegerDependencies{
		Commands: runner,
	}, &request, prIntegerEntry{Image: "linkerd", Version: "25", Type: "default"})

	// Then: staged Melange, @local pinning, native apko, and all-severity Trivy are retained.
	require.NoError(t, err)
	var staged, pinConfig, architectureKeyring, strictScan bool
	for _, call := range runner.calls {
		staged = staged || containsArguments(call.Args, "--staged")
		pinConfig = pinConfig || containsArguments(call.Args, "integer", "melange", "pin-config")
		architectureKeyring = architectureKeyring || (call.Name == "apko" && containsArguments(
			call.Args,
			"--keyring-append",
			filepath.Join("packages", "repo", "melange-aarch64.rsa.pub"),
		))
		strictScan = strictScan || (call.Name == "trivy" && containsArguments(call.Args, "--severity", prIntegerSeverities))
	}
	require.True(t, staged)
	require.True(t, pinConfig)
	require.True(t, architectureKeyring)
	require.True(t, strictScan)
}
