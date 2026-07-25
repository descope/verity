package patchimage

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/verity-org/verity/internal/ci/workflowops/retry"
)

type PreviousReportRequest struct {
	Repository  string
	ImageName   string
	SourceTag   string
	Destination string
}

type PreviousReportResult struct {
	Exists bool
	Bytes  int
}

type PreviousReportService struct {
	Runner retry.Runner
}

func (service PreviousReportService) Download(ctx context.Context, request PreviousReportRequest) (PreviousReportResult, error) {
	endpoint := fmt.Sprintf("repos/%s/contents/reports/%s/%s/post.json?ref=reports", request.Repository, request.ImageName, request.SourceTag)
	result, runErr := service.runner().Run(ctx, &retry.Command{Name: "gh", Args: []string{"api", endpoint}})
	content, decodeErr := decodeGitHubContent(result.Stdout)
	exists := runErr == nil && decodeErr == nil
	if !exists {
		content = []byte(`{"Results":[]}`)
	}
	if err := os.WriteFile(request.Destination, content, 0o600); err != nil {
		return PreviousReportResult{}, fmt.Errorf("write previous report %q: %w", request.Destination, err)
	}
	return PreviousReportResult{Exists: exists, Bytes: len(content)}, nil
}

func (service PreviousReportService) runner() retry.Runner {
	if service.Runner != nil {
		return service.Runner
	}
	return retry.ExecRunner{}
}

func decodeGitHubContent(response []byte) ([]byte, error) {
	var payload struct {
		Content string `json:"content"`
	}
	if err := json.Unmarshal(response, &payload); err != nil {
		return nil, fmt.Errorf("decode GitHub content response: %w", err)
	}
	content, err := base64.StdEncoding.DecodeString(payload.Content)
	if err != nil {
		return nil, fmt.Errorf("decode GitHub content: %w", err)
	}
	return content, nil
}

type Clock interface {
	Now() time.Time
}

type PreflightRequest struct {
	Repository             string
	ImageName              string
	SourceTag              string
	UpstreamDigest         string
	PatchedVulnerabilities int
	MaxAttempts            int
	RetryDelay             time.Duration
}

type PreflightResult struct {
	Updated bool
}

type PreflightService struct {
	Runner  retry.Runner
	Sleeper retry.Sleeper
	Clock   Clock
}

func (service PreflightService) Update(ctx context.Context, request *PreflightRequest) (PreflightResult, error) {
	attempts := request.MaxAttempts
	if attempts < 1 {
		attempts = 5
	}
	for attempt := 1; attempt <= attempts; attempt++ {
		current, err := service.fetch(ctx, request.Repository)
		if err == nil {
			payload, payloadErr := service.updatedPayload(request, current)
			if payloadErr != nil {
				return PreflightResult{}, payloadErr
			}
			endpoint := fmt.Sprintf("repos/%s/contents/reports/preflight-manifest.json", request.Repository)
			_, putErr := service.runner().Run(ctx, &retry.Command{
				Name: "gh", Args: []string{"api", "--method", "PUT", endpoint, "--input", "-"}, Stdin: payload,
			})
			if putErr == nil {
				return PreflightResult{Updated: true}, nil
			}
		}
		if attempt < attempts {
			if err := service.sleeper().Wait(ctx, request.RetryDelay); err != nil {
				return PreflightResult{}, fmt.Errorf("wait before preflight retry: %w", err)
			}
		}
	}
	return PreflightResult{}, nil
}

type preflightCurrent struct {
	SHA     string
	Content []byte
}

func (service PreflightService) fetch(ctx context.Context, repository string) (preflightCurrent, error) {
	endpoint := fmt.Sprintf("repos/%s/contents/reports/preflight-manifest.json?ref=reports", repository)
	result, err := service.runner().Run(ctx, &retry.Command{Name: "gh", Args: []string{"api", endpoint}})
	if err != nil {
		combined := string(result.Stdout) + string(result.Stderr)
		if strings.Contains(combined, `"Not Found"`) {
			return preflightCurrent{Content: []byte(`{}`)}, nil
		}
		return preflightCurrent{}, fmt.Errorf("fetch preflight manifest: %w", err)
	}
	var response struct {
		SHA     string `json:"sha"`
		Content string `json:"content"`
	}
	if err := json.Unmarshal(result.Stdout, &response); err != nil {
		return preflightCurrent{}, fmt.Errorf("decode preflight response: %w", err)
	}
	content, err := base64.StdEncoding.DecodeString(response.Content)
	if err != nil {
		return preflightCurrent{}, fmt.Errorf("decode preflight content: %w", err)
	}
	return preflightCurrent{SHA: response.SHA, Content: content}, nil
}

func (service PreflightService) updatedPayload(request *PreflightRequest, current preflightCurrent) ([]byte, error) {
	manifest := make(map[string]json.RawMessage)
	if err := json.Unmarshal(current.Content, &manifest); err != nil {
		return nil, fmt.Errorf("decode preflight manifest: %w", err)
	}
	entry, err := json.Marshal(struct {
		UpstreamDigest         string `json:"upstream_digest"`
		PatchedVulnerabilities int    `json:"patched_vulns"`
		LastPatched            string `json:"last_patched"`
	}{
		UpstreamDigest: request.UpstreamDigest, PatchedVulnerabilities: request.PatchedVulnerabilities,
		LastPatched: service.clock().Now().UTC().Format(time.RFC3339),
	})
	if err != nil {
		return nil, fmt.Errorf("encode preflight entry: %w", err)
	}
	key := request.ImageName + "/" + request.SourceTag
	manifest[key] = entry
	content, err := json.Marshal(manifest)
	if err != nil {
		return nil, fmt.Errorf("encode preflight manifest: %w", err)
	}
	payload := struct {
		Message string `json:"message"`
		Content string `json:"content"`
		Branch  string `json:"branch"`
		SHA     string `json:"sha,omitempty"`
	}{
		Message: "chore: update preflight manifest for " + key,
		Content: base64.StdEncoding.EncodeToString(content), Branch: "reports", SHA: current.SHA,
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("encode preflight request: %w", err)
	}
	return encoded, nil
}

func (service PreflightService) runner() retry.Runner {
	if service.Runner != nil {
		return service.Runner
	}
	return retry.ExecRunner{}
}

func (service PreflightService) sleeper() retry.Sleeper {
	if service.Sleeper != nil {
		return service.Sleeper
	}
	return retry.TimerSleeper{}
}

func (service PreflightService) clock() Clock {
	if service.Clock != nil {
		return service.Clock
	}
	return systemClock{}
}

type systemClock struct{}

func (systemClock) Now() time.Time {
	return time.Now()
}
