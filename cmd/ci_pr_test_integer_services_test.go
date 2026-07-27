package cmd

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type servicePRIntegerRunner struct {
	address string
	calls   []prCommandRequest
}

func (r *servicePRIntegerRunner) Run(_ context.Context, request *prCommandRequest) (prCommandResult, error) {
	r.calls = append(r.calls, *request)
	if request.Name != "docker" || len(request.Args) == 0 {
		return prCommandResult{}, nil
	}
	switch request.Args[0] {
	case "port":
		return prCommandResult{Stdout: []byte(r.address + "\n")}, nil
	case "inspect":
		return prCommandResult{Stdout: []byte("true\n")}, nil
	case "cp":
		source := request.Args[1]
		destination := request.Args[2]
		pkg := "prometheus-pushgateway"
		version := "1.11.3-r2"
		license := "Apache-2.0"
		if strings.Contains(source, "weaviate-") {
			pkg = "weaviate"
			version = "1.32.10-r0"
			license = "BSD-3-Clause"
		}
		data, err := json.Marshal(prSPDXDocument{
			SPDXVersion: "SPDX-2.3",
			Packages:    []prSPDXPackage{{Name: pkg, VersionInfo: version, LicenseDeclared: license}},
		})
		if err != nil {
			return prCommandResult{}, err
		}
		if err := os.WriteFile(destination, data, 0o600); err != nil {
			return prCommandResult{}, err
		}
	}
	return prCommandResult{}, nil
}

func TestRunPRPushgatewaySmoke_checks_health_metrics_and_SPDX_with_pinned_image(t *testing.T) {
	// Given: a Pushgateway-compatible local HTTP endpoint and fake Docker.
	posted := false
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/-/ready":
			writePRTestResponse(t, writer, "OK\n")
		case "/metrics/job/integer_pushgateway_smoke":
			posted = true
			writer.WriteHeader(http.StatusAccepted)
		case "/metrics":
			writePRTestResponse(t, writer, "pushgateway_build_info{version=\"1.11.3\"} 1\n")
			if posted {
				writePRTestResponse(t, writer, "integer_pushgateway_smoke{instance=\"\",job=\"integer_pushgateway_smoke\"} 7\n")
			}
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()
	runner := &servicePRIntegerRunner{address: strings.TrimPrefix(server.URL, "http://")}
	image := "sha256:" + strings.Repeat("d", 64)

	// When: the typed runtime proof executes.
	request := &prIntegerBatchRequest{Kind: prIntegerBatchSmoke, Architecture: "amd64", RunnerTemp: t.TempDir()}
	err := runPRPushgatewaySmoke(t.Context(), &prIntegerDependencies{
		Commands: runner, HTTPClient: server.Client(), Wait: noWaitPRInteger,
	}, request, image, prPackageMetadata{Version: "1.11.3", FullVersion: "1.11.3-r2"})

	// Then: health, write/read metrics, and SPDX all pass through the immutable ID.
	require.NoError(t, err)
	require.True(t, posted)
	requireDockerRunUsesImage(t, runner.calls, image)
}

func TestRunPRWeaviateSmoke_checks_ready_version_and_SPDX_with_pinned_image(t *testing.T) {
	// Given: a Weaviate-compatible local HTTP endpoint and fake Docker.
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/v1/.well-known/ready":
			writer.WriteHeader(http.StatusNoContent)
		case "/v1/meta":
			writePRTestResponse(t, writer, `{"version":"1.32.10"}`)
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()
	runner := &servicePRIntegerRunner{address: strings.TrimPrefix(server.URL, "http://")}
	image := "sha256:" + strings.Repeat("e", 64)

	// When: the typed runtime proof executes.
	request := &prIntegerBatchRequest{Kind: prIntegerBatchSmoke, Architecture: "arm64", RunnerTemp: t.TempDir()}
	err := runPRWeaviateSmoke(t.Context(), &prIntegerDependencies{
		Commands: runner, HTTPClient: server.Client(), Wait: noWaitPRInteger,
	}, request, image, prPackageMetadata{Version: "1.32.10", FullVersion: "1.32.10-r0"})

	// Then: readiness, exact version, and SPDX all pass through the immutable ID.
	require.NoError(t, err)
	requireDockerRunUsesImage(t, runner.calls, image)
}

func noWaitPRInteger(context.Context, time.Duration) error {
	return nil
}

func requireDockerRunUsesImage(t *testing.T, calls []prCommandRequest, image string) {
	t.Helper()
	for index := range calls {
		call := &calls[index]
		if call.Name == "docker" && len(call.Args) > 0 && call.Args[0] == "run" {
			require.Contains(t, call.Args, image)
			return
		}
	}
	require.Fail(t, "docker run was not invoked")
}

func writePRTestResponse(t *testing.T, writer http.ResponseWriter, body string) {
	t.Helper()
	_, err := writer.Write([]byte(body))
	require.NoError(t, err)
}
