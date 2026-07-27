package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	neturl "net/url"
	"os"
	"strings"
	"time"

	"github.com/verity-org/verity/internal/discovery"
	intdiscovery "github.com/verity-org/verity/internal/integer/discovery"
)

func appendGitHubMatrixOutput(path string, count int, data []byte) error {
	file, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("opening GitHub output %s: %w", path, err)
	}
	return appendGitHubMatrixOutputTo(file, path, count, data)
}

func appendGitHubMatrixOutputTo(writer io.WriteCloser, path string, count int, data []byte) (retErr error) {
	defer func() {
		if closeErr := writer.Close(); closeErr != nil {
			retErr = errors.Join(retErr, fmt.Errorf("closing GitHub output %s: %w", path, closeErr))
		}
	}()
	if _, err := fmt.Fprintf(writer, "count=%d\nimages<<__VERITY_NIGHTLY_JSON__\n%s\n__VERITY_NIGHTLY_JSON__\n", count, data); err != nil {
		return fmt.Errorf("writing GitHub output %s: %w", path, err)
	}
	return nil
}

func nightlyDispatchInputs(family, inputPath, batchID string) ([]map[string]string, error) {
	data, err := os.ReadFile(inputPath)
	if err != nil {
		return nil, fmt.Errorf("reading dispatch matrix %s: %w", inputPath, err)
	}
	switch family {
	case nightlyFamilyCopa:
		var items []discovery.DiscoveredImage
		if err := json.Unmarshal(data, &items); err != nil {
			return nil, fmt.Errorf("parsing copa matrix: %w", err)
		}
		out := make([]map[string]string, 0, len(items))
		for _, item := range items {
			input := map[string]string{
				"image-name":      item.Name,
				"source-ref":      item.Source,
				"target-registry": item.TargetRegistry,
				"platforms":       item.Platforms,
			}
			if item.GoVcsURL != "" {
				input["go-vcs-url"] = item.GoVcsURL
			}
			if batchID != "" {
				input["batch-id"] = batchID
			}
			out = append(out, input)
		}
		return out, nil
	case nightlyFamilyInteger:
		var items []intdiscovery.DiscoveredImage
		if err := json.Unmarshal(data, &items); err != nil {
			return nil, fmt.Errorf("parsing integer matrix: %w", err)
		}
		out := make([]map[string]string, 0, len(items))
		for _, item := range items {
			out = append(out, map[string]string{
				"image":    item.Name,
				"version":  item.Version,
				"type":     item.Type,
				"tags":     strings.Join(item.Tags, ","),
				"registry": item.Registry,
			})
		}
		return out, nil
	default:
		return nil, fmt.Errorf("%w: %q", errUnsupportedNightlyFamily, family)
	}
}

func dispatchWorkflow(ctx context.Context, token, repo, workflow, ref string, inputs map[string]string, retries int) error {
	if retries < 1 {
		retries = 1
	}
	body, err := json.Marshal(map[string]any{
		"ref":    ref,
		"inputs": inputs,
	})
	if err != nil {
		return fmt.Errorf("marshalling dispatch body: %w", err)
	}
	endpoint := fmt.Sprintf("%s/repos/%s/actions/workflows/%s/dispatches", strings.TrimRight(githubAPIBaseURL, "/"), repo, neturl.PathEscape(workflow))
	var lastErr error
	for attempt := 1; attempt <= retries; attempt++ {
		request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
		if err != nil {
			return fmt.Errorf("creating dispatch request: %w", err)
		}
		request.Header.Set("Accept", "application/vnd.github+json")
		request.Header.Set("Authorization", "Bearer "+token)
		request.Header.Set("X-GitHub-Api-Version", "2022-11-28")
		request.Header.Set("Content-Type", "application/json")

		response, err := githubHTTPClient.Do(request)
		switch {
		case err != nil:
			lastErr = err
		case response.StatusCode == http.StatusNoContent:
			if response.Body != nil {
				if closeErr := response.Body.Close(); closeErr != nil {
					return fmt.Errorf("closing github dispatch response: %w", closeErr)
				}
			}
			return nil
		default:
			lastErr = githubDispatchResponseError(response)
		}
		if attempt < retries {
			dispatchRetrySleep(time.Duration(attempt*10) * time.Second)
		}
	}
	return fmt.Errorf("dispatching %s after %d attempt(s): %w", workflow, retries, lastErr)
}

func githubDispatchResponseError(response *http.Response) error {
	if response.Body == nil {
		return fmt.Errorf("%w: %s", errGitHubDispatchStatus, response.Status)
	}
	responseBody, readErr := io.ReadAll(io.LimitReader(response.Body, 4096))
	closeErr := response.Body.Close()
	switch {
	case readErr != nil:
		return fmt.Errorf("reading github dispatch error response: %w", readErr)
	case closeErr != nil:
		return fmt.Errorf("closing github dispatch error response: %w", closeErr)
	default:
		return fmt.Errorf("%w: %s: %s", errGitHubDispatchStatus, response.Status, strings.TrimSpace(string(responseBody)))
	}
}
