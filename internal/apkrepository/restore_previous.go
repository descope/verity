package apkrepository

import (
	"archive/tar"
	"archive/zip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

type RestorePreviousOptions struct {
	OutputDir  string
	Repository string
	Stdout     io.Writer
	runner     commandRunner
}

type artifactList struct {
	Artifacts []pagesArtifact `json:"artifacts"`
}

type pagesArtifact struct {
	ID        int64            `json:"id"`
	Expired   bool             `json:"expired"`
	CreatedAt string           `json:"created_at"`
	Workflow  artifactWorkflow `json:"workflow_run"`
}

type artifactWorkflow struct {
	ID         int64  `json:"id"`
	HeadBranch string `json:"head_branch"`
}

type workflowConclusion struct {
	Conclusion string `json:"conclusion"`
}

func RestorePrevious(ctx context.Context, options *RestorePreviousOptions) error {
	if options == nil {
		return errOptionsRequired
	}
	if err := validateOutputDirectory(options.OutputDir); err != nil {
		return err
	}
	if strings.TrimSpace(options.Repository) == "" {
		return errRepositoryEnvironmentRequired
	}
	runner := options.runner
	if runner == nil {
		runner = execCommandRunner{}
	}
	artifacts, err := listPagesArtifacts(ctx, runner, options.Repository)
	if err != nil {
		return err
	}
	artifactID, err := latestSuccessfulArtifact(ctx, runner, options.Repository, artifacts)
	if err != nil {
		return err
	}
	if err := os.RemoveAll(options.OutputDir); err != nil {
		return fmt.Errorf("clear previous Pages output: %w", err)
	}
	if err := os.MkdirAll(options.OutputDir, 0o755); err != nil {
		return fmt.Errorf("create previous Pages output: %w", err)
	}
	stdout := writerOrDiscard(options.Stdout)
	if artifactID == 0 {
		_, err := fmt.Fprintln(stdout, "No retained successful main-branch Pages artifact found; APK repository will bootstrap from the candidate set")
		return err
	}
	if err := downloadAndExtractPages(ctx, runner, options.Repository, artifactID, options.OutputDir); err != nil {
		return err
	}
	_, err = fmt.Fprintf(stdout, "Restored previous Pages artifact %d\n", artifactID)
	return err
}

func listPagesArtifacts(ctx context.Context, runner commandRunner, repository string) ([]pagesArtifact, error) {
	result, err := runRequired(ctx, runner, command{
		name: "gh",
		args: []string{"api", "repos/" + repository + "/actions/artifacts", "--method", "GET", "-f", "name=github-pages", "-f", "per_page=100"},
	})
	if err != nil {
		return nil, fmt.Errorf("list Pages artifacts: %w", err)
	}
	var response artifactList
	if err := json.Unmarshal(result.stdout, &response); err != nil {
		return nil, fmt.Errorf("decode Pages artifacts: %w", err)
	}
	eligible := make([]pagesArtifact, 0, len(response.Artifacts))
	for _, artifact := range response.Artifacts {
		if !artifact.Expired && artifact.Workflow.HeadBranch == "main" {
			eligible = append(eligible, artifact)
		}
	}
	sort.Slice(eligible, func(i, j int) bool { return eligible[i].CreatedAt > eligible[j].CreatedAt })
	return eligible, nil
}

func latestSuccessfulArtifact(ctx context.Context, runner commandRunner, repository string, artifacts []pagesArtifact) (int64, error) {
	for _, artifact := range artifacts {
		result, err := runRequired(ctx, runner, command{
			name: "gh",
			args: []string{"api", "repos/" + repository + "/actions/runs/" + strconv.FormatInt(artifact.Workflow.ID, 10)},
		})
		if err != nil {
			return 0, fmt.Errorf("inspect Pages workflow run %d: %w", artifact.Workflow.ID, err)
		}
		var run workflowConclusion
		if err := json.Unmarshal(result.stdout, &run); err != nil {
			return 0, fmt.Errorf("decode Pages workflow run %d: %w", artifact.Workflow.ID, err)
		}
		if run.Conclusion == "success" {
			return artifact.ID, nil
		}
	}
	return 0, nil
}

func downloadAndExtractPages(ctx context.Context, runner commandRunner, repository string, artifactID int64, outputDir string) error {
	temporaryDir, err := os.MkdirTemp("", "verity-pages-artifact-")
	if err != nil {
		return fmt.Errorf("create Pages artifact directory: %w", err)
	}
	defer os.RemoveAll(temporaryDir)
	archivePath := filepath.Join(temporaryDir, "pages.zip")
	archive, err := os.Create(archivePath)
	if err != nil {
		return fmt.Errorf("create Pages archive: %w", err)
	}
	_, runErr := runRequired(ctx, runner, command{
		name:   "gh",
		args:   []string{"api", "repos/" + repository + "/actions/artifacts/" + strconv.FormatInt(artifactID, 10) + "/zip"},
		stdout: archive,
	})
	closeErr := archive.Close()
	if runErr != nil {
		return fmt.Errorf("download Pages artifact %d: %w", artifactID, runErr)
	}
	if closeErr != nil {
		return fmt.Errorf("close Pages archive: %w", closeErr)
	}
	zipReader, err := zip.OpenReader(archivePath)
	if err != nil {
		return fmt.Errorf("open Pages artifact zip: %w", err)
	}
	defer zipReader.Close()
	for _, entry := range zipReader.File {
		if entry.Name != "artifact.tar" {
			continue
		}
		reader, err := entry.Open()
		if err != nil {
			return fmt.Errorf("open artifact.tar: %w", err)
		}
		extractErr := extractAPKSubtree(reader, outputDir)
		closeErr := reader.Close()
		return errors.Join(extractErr, closeErr)
	}
	return errMissingArtifactTar
}

func extractAPKSubtree(reader io.Reader, outputDir string) error {
	tarReader := tar.NewReader(reader)
	for {
		header, err := tarReader.Next()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("read Pages artifact tar: %w", err)
		}
		rawName := filepath.FromSlash(strings.TrimPrefix(header.Name, "./"))
		if rawName != "apk" && !strings.HasPrefix(rawName, "apk"+string(filepath.Separator)) {
			continue
		}
		cleanName := filepath.Clean(rawName)
		if filepath.IsAbs(cleanName) || cleanName == ".." || strings.HasPrefix(cleanName, ".."+string(filepath.Separator)) {
			return fmt.Errorf("%w: %s", errUnsafeArtifactPath, header.Name)
		}
		destination := filepath.Join(outputDir, cleanName)
		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(destination, header.FileInfo().Mode().Perm()); err != nil {
				return fmt.Errorf("create Pages directory %q: %w", destination, err)
			}
		case tar.TypeReg:
			if err := writeTarFile(destination, header.FileInfo().Mode().Perm(), tarReader); err != nil {
				return err
			}
		default:
			return fmt.Errorf("%w: %s", errUnsupportedArtifactEntry, header.Name)
		}
	}
}

func writeTarFile(path string, mode os.FileMode, reader io.Reader) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create Pages file directory: %w", err)
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, mode)
	if err != nil {
		return fmt.Errorf("create Pages file %q: %w", path, err)
	}
	if _, err := io.Copy(file, reader); err != nil {
		_ = file.Close()
		return fmt.Errorf("write Pages file %q: %w", path, err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close Pages file %q: %w", path, err)
	}
	return nil
}
