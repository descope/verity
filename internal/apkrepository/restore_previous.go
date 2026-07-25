package apkrepository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strconv"

	"github.com/verity-org/verity/internal/ci/sitepublication"
)

var (
	errRestoreUnauthorized    = errors.New("build site restore is not authorized")
	errWrongPagesWorkflow     = errors.New("prior artifact is not from Build Site")
	errWrongPagesRun          = errors.New("prior Build Site run identity mismatch")
	errPagesArtifactNotFound  = errors.New("exact Build Site Pages artifact not found")
	errLegacyPagesArtifact    = errors.New("legacy Pages artifact has no canonical digest")
	errPagesArtifactDigest    = errors.New("build site artifact digest mismatch")
	errPagesManifestDigest    = errors.New("build site publication manifest digest mismatch")
	errAmbiguousPagesArtifact = errors.New("multiple exact Build Site Pages artifacts")
	restoreSHA                = regexp.MustCompile(`^[0-9a-f]{40}([0-9a-f]{24})?$`)
)

type RestorePreviousOptions struct {
	OutputDir              string
	Repository             string
	RunID                  uint64
	RunAttempt             uint64
	ExpectedSourceSHA      string
	ExpectedArtifactDigest string
	ExpectedManifestDigest string
	AuthorizeRestore       bool
	Stdout                 io.Writer
	runner                 commandRunner
}

type priorWorkflowRun struct {
	ID         uint64 `json:"id"`
	RunAttempt uint64 `json:"run_attempt"`
	Name       string `json:"name"`
	Path       string `json:"path"`
	HeadBranch string `json:"head_branch"`
	HeadSHA    string `json:"head_sha"`
	Conclusion string `json:"conclusion"`
}

type priorArtifactList struct {
	Artifacts []priorArtifact `json:"artifacts"`
}

type priorArtifact struct {
	ID       int64  `json:"id"`
	Name     string `json:"name"`
	Expired  bool   `json:"expired"`
	Digest   string `json:"digest"`
	Workflow struct {
		ID         uint64 `json:"id"`
		HeadBranch string `json:"head_branch"`
		HeadSHA    string `json:"head_sha"`
	} `json:"workflow_run"`
}

func RestorePrevious(ctx context.Context, options *RestorePreviousOptions) error {
	if err := validateRestoreOptions(options); err != nil {
		return err
	}
	runner := options.runner
	if runner == nil {
		runner = execCommandRunner{}
	}
	if err := validatePriorRun(ctx, runner, options); err != nil {
		return err
	}
	artifact, err := exactPagesArtifact(ctx, runner, options)
	if err != nil {
		return err
	}
	archiveDigest, err := restoreExactArtifact(ctx, runner, options, &artifact)
	if err != nil {
		return err
	}
	result := struct {
		Restored       bool   `json:"restored"`
		RunID          uint64 `json:"run_id"`
		ArtifactDigest string `json:"artifact_digest"`
	}{Restored: true, RunID: options.RunID, ArtifactDigest: archiveDigest}
	if err := json.NewEncoder(writerOrDiscard(options.Stdout)).Encode(result); err != nil {
		return fmt.Errorf("write restore result: %w", err)
	}
	return nil
}

func restoreExactArtifact(ctx context.Context, runner commandRunner, options *RestorePreviousOptions, artifact *priorArtifact) (string, error) {
	temporaryDir, err := os.MkdirTemp("", "verity-build-site-restore-")
	if err != nil {
		return "", fmt.Errorf("create restore directory: %w", err)
	}
	defer os.RemoveAll(temporaryDir)
	zipPath := filepath.Join(temporaryDir, "artifact.zip")
	if err := downloadPriorArtifact(ctx, runner, options.Repository, artifact, zipPath); err != nil {
		return "", err
	}
	archivePath := filepath.Join(temporaryDir, "artifact.tar")
	if err := extractArtifactTar(zipPath, archivePath); err != nil {
		return "", err
	}
	archiveDigest, err := fileSHA256(archivePath)
	if err != nil {
		return "", err
	}
	if archiveDigest != options.ExpectedArtifactDigest {
		return "", fmt.Errorf("%w: expected %s, got %s", errPagesArtifactDigest, options.ExpectedArtifactDigest, archiveDigest)
	}
	if err := verifyBuildSiteAttestation(ctx, runner, &buildSiteAttestationRequest{
		repository: options.Repository, signerWorkflow: sitepublication.BuildSiteWorkflowIdentity,
		sourceRef: "refs/heads/main", sourceDigest: options.ExpectedSourceSHA, archivePath: archivePath,
	}); err != nil {
		return "", err
	}
	if err := restoreArchiveToOutput(options, archivePath); err != nil {
		return "", err
	}
	return archiveDigest, nil
}

func validateRestoreOptions(options *RestorePreviousOptions) error {
	if options == nil {
		return errOptionsRequired
	}
	if !options.AuthorizeRestore {
		return errRestoreUnauthorized
	}
	if err := validateOutputDirectory(options.OutputDir); err != nil {
		return err
	}
	if options.Repository == "" {
		return errRepositoryEnvironmentRequired
	}
	if options.RunID == 0 || options.RunAttempt == 0 || !restoreSHA.MatchString(options.ExpectedSourceSHA) {
		return errWrongPagesRun
	}
	if !sha256Digest.MatchString(options.ExpectedArtifactDigest) || !sha256Digest.MatchString(options.ExpectedManifestDigest) {
		return fmt.Errorf("%w: expected digests", errLegacyPagesArtifact)
	}
	return nil
}

func validatePriorRun(ctx context.Context, runner commandRunner, options *RestorePreviousOptions) error {
	result, err := runRequired(ctx, runner, &command{
		name: "gh", args: []string{"api", "repos/" + options.Repository + "/actions/runs/" + strconv.FormatUint(options.RunID, 10)},
	})
	if err != nil {
		return fmt.Errorf("inspect prior Build Site run: %w", err)
	}
	var run priorWorkflowRun
	if err := json.Unmarshal(result.stdout, &run); err != nil {
		return fmt.Errorf("decode prior Build Site run: %w", err)
	}
	if run.Path != sitepublication.PublishWorkflow || run.Name != sitepublication.PublishWorkflowName {
		return errWrongPagesWorkflow
	}
	if run.ID != options.RunID || run.RunAttempt != options.RunAttempt || run.HeadBranch != "main" ||
		run.HeadSHA != options.ExpectedSourceSHA || run.Conclusion != "success" {
		return errWrongPagesRun
	}
	return nil
}

func exactPagesArtifact(ctx context.Context, runner commandRunner, options *RestorePreviousOptions) (priorArtifact, error) {
	result, err := runRequired(ctx, runner, &command{
		name: "gh", args: []string{"api", "repos/" + options.Repository + "/actions/runs/" + strconv.FormatUint(options.RunID, 10) + "/artifacts", "--method", "GET", "-f", "per_page=100"},
	})
	if err != nil {
		return priorArtifact{}, fmt.Errorf("list exact Build Site artifacts: %w", err)
	}
	var response priorArtifactList
	if err := json.Unmarshal(result.stdout, &response); err != nil {
		return priorArtifact{}, fmt.Errorf("decode Build Site artifacts: %w", err)
	}
	var selected *priorArtifact
	for index := range response.Artifacts {
		artifact := &response.Artifacts[index]
		if artifact.Name != sitepublication.PagesArtifactName || artifact.Expired || artifact.Workflow.ID != options.RunID {
			continue
		}
		if selected != nil {
			return priorArtifact{}, errAmbiguousPagesArtifact
		}
		selected = artifact
	}
	if selected == nil {
		return priorArtifact{}, errPagesArtifactNotFound
	}
	if !sha256Digest.MatchString(selected.Digest) {
		return priorArtifact{}, errLegacyPagesArtifact
	}
	if selected.Workflow.HeadBranch != "main" || selected.Workflow.HeadSHA != options.ExpectedSourceSHA {
		return priorArtifact{}, errWrongPagesRun
	}
	return *selected, nil
}
