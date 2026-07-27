package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

func runPRWeaviateSmoke(
	ctx context.Context,
	deps *prIntegerDependencies,
	request *prIntegerBatchRequest,
	image string,
	metadata prPackageMetadata,
) (err error) {
	container := fmt.Sprintf("integer-weaviate-%s-pr-smoke", request.Architecture)
	port, err := startPRServiceContainer(ctx, deps, &prServiceContainerRequest{
		Name: container, Image: image, ContainerPort: "8080",
		DockerArgs: []string{
			"--env", "AUTHENTICATION_ANONYMOUS_ACCESS_ENABLED=true",
			"--env", "CLUSTER_HOSTNAME=" + container,
			"--env", "DEFAULT_VECTORIZER_MODULE=none",
			"--env", "DISABLE_TELEMETRY=true",
			"--env", "PERSISTENCE_DATA_PATH=/var/lib/weaviate",
		},
		CommandArgs: []string{"--host", "0.0.0.0", "--port", "8080", "--scheme", "http"},
	})
	if err != nil {
		return err
	}
	defer func() {
		err = joinPRCleanup(err, func() error { return removePRIntegerContainer(ctx, deps, container) })
	}()
	baseURL := fmt.Sprintf("http://127.0.0.1:%d", port)
	if err := waitPRServiceReady(ctx, deps, container, baseURL+"/v1/.well-known/ready", func([]byte) bool {
		return true
	}); err != nil {
		return err
	}
	data, err := requestPRService(ctx, deps, http.MethodGet, baseURL+"/v1/meta", nil, "")
	if err != nil {
		return err
	}
	var meta struct {
		Version string `json:"version"`
	}
	if err := json.Unmarshal(data, &meta); err != nil {
		return fmt.Errorf("parse Weaviate metadata: %w", err)
	}
	if meta.Version != metadata.Version {
		return fmt.Errorf("%w: Weaviate version is %q, want %q", errPRCommandFailed, meta.Version, metadata.Version)
	}
	return verifyPRContainerSPDX(ctx, deps, &prContainerSPDXRequest{
		Container:     container,
		ContainerPath: "/var/lib/db/sbom/weaviate-" + metadata.FullVersion + ".spdx.json",
		TempDir:       request.RunnerTemp, FileName: "weaviate-" + request.Architecture + ".spdx.json",
		Package: "weaviate", Version: metadata.FullVersion, License: "BSD-3-Clause",
	})
}
