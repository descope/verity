package patchimage

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/verity-org/verity/internal/ci/workflowops/retry"
)

var ErrWorkflowStartTimestamp = errors.New("workflow run_started_at is empty")

func ReadPlatformSet(directory string) PlatformSet {
	return PlatformSet{
		AMD64: readOptionalJSON(filepath.Join(directory, "platform-amd64.json")),
		ARM64: readOptionalJSON(filepath.Join(directory, "platform-arm64.json")),
	}
}

func readOptionalJSON(path string) json.RawMessage {
	content, err := os.ReadFile(path)
	if err != nil || !json.Valid(content) {
		return json.RawMessage("null")
	}
	var compact bytes.Buffer
	if err := json.Compact(&compact, content); err != nil {
		return json.RawMessage("null")
	}
	if compact.String() == "" {
		return json.RawMessage("null")
	}
	return json.RawMessage(compact.Bytes())
}

func WritePlatformMetrics(path string, metrics PlatformMetrics) error {
	return writeJSON(path, metrics)
}

func WriteMetricsDocument(path string, document *MetricsDocument) error {
	return writeJSON(path, document)
}

func writeJSON[T any](path string, value T) (err error) {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("open JSON output %q: %w", path, err)
	}
	defer func() {
		if closeErr := file.Close(); closeErr != nil && err == nil {
			err = fmt.Errorf("close JSON output %q: %w", path, closeErr)
		}
	}()
	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(value); err != nil {
		return fmt.Errorf("encode JSON output %q: %w", path, err)
	}
	return nil
}

func FileSHA256(path string) (string, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read digest input %q: %w", path, err)
	}
	digest := sha256.Sum256(content)
	return hex.EncodeToString(digest[:]), nil
}

type WorkflowStartService struct {
	Runner retry.Runner
}

func (service WorkflowStartService) Fetch(ctx context.Context, repository, runID string) (string, error) {
	endpoint := fmt.Sprintf("repos/%s/actions/runs/%s", repository, runID)
	result, err := service.runner().Run(ctx, &retry.Command{Name: "gh", Args: []string{"api", endpoint}})
	if err != nil {
		return "", fmt.Errorf("fetch workflow run: %w", err)
	}
	var response struct {
		StartedAt string `json:"run_started_at"`
	}
	if err := json.Unmarshal(result.Stdout, &response); err != nil {
		return "", fmt.Errorf("decode workflow run: %w", err)
	}
	if response.StartedAt == "" {
		return "", ErrWorkflowStartTimestamp
	}
	return response.StartedAt, nil
}

func (service WorkflowStartService) runner() retry.Runner {
	if service.Runner != nil {
		return service.Runner
	}
	return retry.ExecRunner{}
}
