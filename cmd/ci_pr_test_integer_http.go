package cmd

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const prMaxHTTPBodyBytes = 2 << 20

type prServiceContainerRequest struct {
	Name          string
	Image         string
	ContainerPort string
	DockerArgs    []string
	CommandArgs   []string
}

func startPRServiceContainer(
	ctx context.Context,
	deps *prIntegerDependencies,
	request *prServiceContainerRequest,
) (int, error) {
	args := make([]string, 0, 8+len(request.DockerArgs)+len(request.CommandArgs))
	args = append(
		args,
		"run", "--detach", "--rm", "--name", request.Name,
		"--publish", "127.0.0.1::"+request.ContainerPort,
	)
	args = append(args, request.DockerArgs...)
	args = append(args, request.Image)
	args = append(args, request.CommandArgs...)
	if _, err := runPRIntegerCommand(ctx, deps, &prCommandRequest{Name: "docker", Args: args}); err != nil {
		return 0, fmt.Errorf("start %s container: %w", request.Name, err)
	}
	result, err := runPRIntegerCommand(ctx, deps, &prCommandRequest{
		Name: "docker", Args: []string{"port", request.Name, request.ContainerPort + "/tcp"},
	})
	if err != nil {
		return 0, errors.Join(
			fmt.Errorf("resolve %s port: %w", request.Name, err),
			removePRIntegerContainer(ctx, deps, request.Name),
		)
	}
	lines := strings.Fields(string(result.Stdout))
	if len(lines) == 0 {
		return 0, errors.Join(
			fmt.Errorf("%w: docker did not publish %s", errPRCommandFailed, request.ContainerPort),
			removePRIntegerContainer(ctx, deps, request.Name),
		)
	}
	address := lines[len(lines)-1]
	colon := strings.LastIndex(address, ":")
	if colon < 0 || colon == len(address)-1 {
		return 0, errors.Join(
			fmt.Errorf("%w: malformed docker port %q", errPRCommandFailed, address),
			removePRIntegerContainer(ctx, deps, request.Name),
		)
	}
	port, err := strconv.Atoi(address[colon+1:])
	if err != nil || port < 1 || port > 65535 {
		return 0, errors.Join(
			fmt.Errorf("%w: malformed docker port %q", errPRCommandFailed, address),
			removePRIntegerContainer(ctx, deps, request.Name),
		)
	}
	return port, nil
}

func waitPRServiceReady(
	ctx context.Context,
	deps *prIntegerDependencies,
	container, url string,
	accept func([]byte) bool,
) error {
	readyContext, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()
	for range 90 {
		body, err := requestPRService(readyContext, deps, http.MethodGet, url, nil, "")
		if err == nil && accept(body) {
			return nil
		}
		running, inspectErr := runPRIntegerCommand(readyContext, deps, &prCommandRequest{
			Name: "docker", Args: []string{"inspect", "--format", "{{.State.Running}}", container},
		})
		if inspectErr != nil {
			return fmt.Errorf("%s stopped before readiness: %w", container, inspectErr)
		}
		if strings.TrimSpace(string(running.Stdout)) != "true" {
			return fmt.Errorf("%w: %s stopped before readiness", errPRCommandFailed, container)
		}
		if err := deps.Wait(readyContext, time.Second); err != nil {
			return fmt.Errorf("%s readiness: %w", container, err)
		}
	}
	return fmt.Errorf("%w: %s did not become ready", errPRCommandFailed, container)
}

func requestPRService(
	ctx context.Context,
	deps *prIntegerDependencies,
	method, url string,
	body []byte,
	contentType string,
) ([]byte, error) {
	request, err := http.NewRequestWithContext(ctx, method, url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create service request: %w", err)
	}
	if contentType != "" {
		request.Header.Set("Content-Type", contentType)
	}
	response, err := deps.HTTPClient.Do(request)
	if err != nil {
		return nil, fmt.Errorf("request %s: %w", url, err)
	}
	defer response.Body.Close()
	data, err := io.ReadAll(io.LimitReader(response.Body, prMaxHTTPBodyBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read %s response: %w", url, err)
	}
	if len(data) > prMaxHTTPBodyBytes {
		return nil, fmt.Errorf("%w: %s response is oversized", errPRCommandFailed, url)
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return nil, fmt.Errorf("%w: %s returned HTTP %d", errPRCommandFailed, url, response.StatusCode)
	}
	return data, nil
}

type prContainerSPDXRequest struct {
	Container     string
	ContainerPath string
	TempDir       string
	FileName      string
	Package       string
	Version       string
	License       string
}

func verifyPRContainerSPDX(
	ctx context.Context,
	deps *prIntegerDependencies,
	request *prContainerSPDXRequest,
) (err error) {
	path := filepath.Join(request.TempDir, request.FileName)
	defer func() {
		if removeErr := os.Remove(path); removeErr != nil && !os.IsNotExist(removeErr) {
			err = errors.Join(err, fmt.Errorf("remove SPDX document: %w", removeErr))
		}
	}()
	if _, err := runPRIntegerCommand(ctx, deps, &prCommandRequest{
		Name: "docker", Args: []string{"cp", request.Container + ":" + request.ContainerPath, path},
	}); err != nil {
		return fmt.Errorf("copy %s SPDX document: %w", request.Package, err)
	}
	return verifyPRSPDXPackage(path, request.Package, request.Version, request.License)
}
