package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/verity-org/verity/internal/ci"
)

const prIntegerBatchSize = 16

type prIntegerEntry struct {
	Image   string `json:"image"`
	Version string `json:"version"`
	Type    string `json:"type"`
}

type prIntegerExpectedMatrix struct {
	Include []prIntegerEntry `json:"include"`
}

type prIntegerBatch struct {
	BatchID             int              `json:"batch_id"`
	Entries             []prIntegerEntry `json:"entries"`
	Architecture        string           `json:"arch"`
	PackageArchitecture string           `json:"package_arch"`
	Runner              string           `json:"runner"`
}

type prIntegerBatchMatrix struct {
	Include []prIntegerBatch `json:"include"`
}

func newPRIntegerBatchMatrix(matrix ci.Matrix) (prIntegerBatchMatrix, error) {
	entries := make([]prIntegerEntry, 0, len(matrix.Include))
	for index, raw := range matrix.Include {
		entry := prIntegerEntry{
			Image:   strings.TrimSpace(raw["image"]),
			Version: strings.TrimSpace(raw["version"]),
			Type:    strings.TrimSpace(raw["type"]),
		}
		if entry.Image == "" || entry.Version == "" || entry.Type == "" {
			return prIntegerBatchMatrix{}, fmt.Errorf("%w: Integer matrix entry %d is incomplete", errPRCommandFailed, index)
		}
		entries = append(entries, entry)
	}

	result := prIntegerBatchMatrix{Include: []prIntegerBatch{}}
	for start, batchID := 0, 0; start < len(entries); start, batchID = start+prIntegerBatchSize, batchID+1 {
		end := min(start+prIntegerBatchSize, len(entries))
		batchEntries := append([]prIntegerEntry(nil), entries[start:end]...)
		result.Include = append(
			result.Include,
			prIntegerBatch{
				BatchID: batchID, Entries: batchEntries, Architecture: "amd64",
				PackageArchitecture: "x86_64", Runner: "ubuntu-latest",
			},
			prIntegerBatch{
				BatchID: batchID, Entries: batchEntries, Architecture: "arm64",
				PackageArchitecture: "aarch64", Runner: "ubuntu-24.04-arm",
			},
		)
	}
	return result, nil
}

func prExpectedIntegerMatrix(matrix ci.Matrix) (prIntegerExpectedMatrix, error) {
	batched, err := newPRIntegerBatchMatrix(matrix)
	if err != nil {
		return prIntegerExpectedMatrix{}, err
	}
	entries := make([]prIntegerEntry, 0, len(matrix.Include))
	for index := 0; index < len(batched.Include); index += 2 {
		entries = append(entries, batched.Include[index].Entries...)
	}
	if len(matrix.Include) == 0 {
		entries = []prIntegerEntry{}
	}
	return prIntegerExpectedMatrix{Include: entries}, nil
}

func marshalPRJSON(value any) (string, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return "", fmt.Errorf("marshal PR test JSON: %w", err)
	}
	return string(data), nil
}

type prGitShowRequest struct {
	RepoRoot string
	Revision string
	Path     string
}

func gitShowPRFile(ctx context.Context, request prGitShowRequest) (data []byte, found bool, err error) {
	result, err := runPRCommand(ctx, &prCommandRequest{
		Name: "git",
		Args: []string{"show", request.Revision + ":" + filepath.ToSlash(request.Path)},
		Dir:  request.RepoRoot,
	})
	if err != nil {
		return nil, false, fmt.Errorf("read %s from %s: %w", request.Path, request.Revision, err)
	}
	if result.ExitCode != 0 {
		return nil, false, nil
	}
	return result.Stdout, true, nil
}

func writePRFile(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create parent for %q: %w", path, err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return fmt.Errorf("write %q: %w", path, err)
	}
	return nil
}
