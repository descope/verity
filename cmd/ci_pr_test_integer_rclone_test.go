package cmd

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

type rclonePRIntegerRunner struct {
	calls []prCommandRequest
}

func (r *rclonePRIntegerRunner) Run(_ context.Context, request *prCommandRequest) (prCommandResult, error) {
	r.calls = append(r.calls, *request)
	if request.Name != "docker" || len(request.Args) == 0 {
		return prCommandResult{}, nil
	}
	if request.Args[0] == "run" && request.Args[len(request.Args)-1] == "--version" {
		return prCommandResult{Stdout: []byte("rclone v1.74.4\n")}, nil
	}
	if request.Args[0] == "run" && containsArguments(request.Args, "copy", "/work/source", "/work/destination") {
		volume := argumentAfter(request.Args, "--volume")
		root := strings.TrimSuffix(volume, ":/work")
		data, err := os.ReadFile(filepath.Join(root, "source", "payload.txt"))
		if err != nil {
			return prCommandResult{}, err
		}
		if err := os.WriteFile(filepath.Join(root, "destination", "payload.txt"), data, 0o644); err != nil {
			return prCommandResult{}, err
		}
	}
	if request.Args[0] == "cp" {
		data, err := json.Marshal(prSPDXDocument{
			SPDXVersion: "SPDX-2.3",
			Packages: []prSPDXPackage{{
				Name: "rclone", VersionInfo: "1.74.4-r0", LicenseDeclared: "MIT",
			}},
		})
		if err != nil {
			return prCommandResult{}, err
		}
		if err := os.WriteFile(request.Args[2], data, 0o600); err != nil {
			return prCommandResult{}, err
		}
	}
	return prCommandResult{}, nil
}

func TestRunPRRcloneSmoke_checks_copy_checksum_and_SPDX_with_pinned_image(t *testing.T) {
	// Given: fake Docker implementing rclone's observable CLI and filesystem behavior.
	runner := &rclonePRIntegerRunner{}
	image := "sha256:" + strings.Repeat("f", 64)

	// When: the typed rclone runtime proof executes.
	request := &prIntegerBatchRequest{Kind: prIntegerBatchSmoke, Architecture: "amd64", RunnerTemp: t.TempDir()}
	err := runPRRcloneSmoke(t.Context(), &prIntegerDependencies{
		Commands: runner,
	}, request, image, prPackageMetadata{Version: "1.74.4", FullVersion: "1.74.4-r0"})

	// Then: version, copy/checksum, and SPDX all pass through the immutable ID.
	require.NoError(t, err)
	for _, call := range runner.calls {
		if call.Name == "docker" && (call.Args[0] == "run" || call.Args[0] == "create") {
			require.Contains(t, call.Args, image)
		}
	}
}
