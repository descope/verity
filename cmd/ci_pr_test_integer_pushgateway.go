package cmd

import (
	"context"
	"fmt"
	"net/http"
	"regexp"
	"strings"
)

func runPRPushgatewaySmoke(
	ctx context.Context,
	deps *prIntegerDependencies,
	request *prIntegerBatchRequest,
	image string,
	metadata prPackageMetadata,
) (err error) {
	container := fmt.Sprintf("integer-pushgateway-%s-pr-%s", request.Architecture, request.Kind)
	port, err := startPRServiceContainer(ctx, deps, &prServiceContainerRequest{
		Name: container, Image: image, ContainerPort: "9091",
		CommandArgs: []string{"--web.listen-address=0.0.0.0:9091"},
	})
	if err != nil {
		return err
	}
	defer func() {
		err = joinPRCleanup(err, func() error { return removePRIntegerContainer(ctx, deps, container) })
	}()
	baseURL := fmt.Sprintf("http://127.0.0.1:%d", port)
	if err := waitPRServiceReady(ctx, deps, container, baseURL+"/-/ready", func(body []byte) bool {
		return strings.TrimSpace(string(body)) == "OK"
	}); err != nil {
		return err
	}
	metrics, err := requestPRService(ctx, deps, http.MethodGet, baseURL+"/metrics", nil, "")
	if err != nil {
		return err
	}
	buildInfo := regexp.MustCompile(`(?m)^pushgateway_build_info\{[^\n]*version="` + regexp.QuoteMeta(metadata.Version) + `"`)
	if !buildInfo.Match(metrics) {
		return fmt.Errorf("%w: Pushgateway build metric does not report %s", errPRCommandFailed, metadata.Version)
	}
	payload := []byte("# HELP integer_pushgateway_smoke Integer Pushgateway smoke metric.\n" +
		"# TYPE integer_pushgateway_smoke gauge\ninteger_pushgateway_smoke 7\n")
	if _, err := requestPRService(
		ctx,
		deps,
		http.MethodPost,
		baseURL+"/metrics/job/integer_pushgateway_smoke",
		payload,
		"text/plain; version=0.0.4",
	); err != nil {
		return err
	}
	metrics, err = requestPRService(ctx, deps, http.MethodGet, baseURL+"/metrics", nil, "")
	if err != nil {
		return err
	}
	if !strings.Contains(string(metrics), `integer_pushgateway_smoke{instance="",job="integer_pushgateway_smoke"} 7`) {
		return fmt.Errorf("%w: Pushgateway did not retain the smoke metric", errPRCommandFailed)
	}
	return verifyPRContainerSPDX(ctx, deps, &prContainerSPDXRequest{
		Container:     container,
		ContainerPath: "/var/lib/db/sbom/prometheus-pushgateway-" + metadata.FullVersion + ".spdx.json",
		TempDir:       request.RunnerTemp, FileName: "pushgateway-" + request.Architecture + ".spdx.json",
		Package: "prometheus-pushgateway", Version: metadata.FullVersion, License: "Apache-2.0",
	})
}
