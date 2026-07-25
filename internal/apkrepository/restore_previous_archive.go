package apkrepository

import (
	"archive/zip"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"

	"github.com/verity-org/verity/internal/ci/publication"
	"github.com/verity-org/verity/internal/ci/sitepublication"
)

type buildSiteAttestationRequest struct {
	repository     string
	signerWorkflow string
	sourceRef      string
	sourceDigest   string
	archivePath    string
}

func downloadPriorArtifact(ctx context.Context, runner commandRunner, repository string, artifact *priorArtifact, destination string) error {
	file, err := os.Create(destination)
	if err != nil {
		return fmt.Errorf("create Build Site artifact zip: %w", err)
	}
	_, runErr := runRequired(ctx, runner, &command{
		name: "gh", args: []string{
			"api", "repos/" + repository + "/actions/artifacts/" + strconv.FormatInt(artifact.ID, 10) + "/zip",
		},
		stdout: file,
	})
	closeErr := file.Close()
	if err := errors.Join(runErr, closeErr); err != nil {
		return fmt.Errorf("download Build Site artifact: %w", err)
	}
	digest, err := fileSHA256(destination)
	if err != nil {
		return err
	}
	if digest != artifact.Digest {
		return fmt.Errorf("%w: API has %s, download has %s", errPagesArtifactDigest, artifact.Digest, digest)
	}
	return nil
}

func extractArtifactTar(zipPath, archivePath string) error {
	reader, err := zip.OpenReader(zipPath)
	if err != nil {
		return fmt.Errorf("open Build Site artifact zip: %w", err)
	}
	defer reader.Close()
	if len(reader.File) != 1 || reader.File[0].Name != "artifact.tar" || reader.File[0].FileInfo().IsDir() {
		return errLegacyPagesArtifact
	}
	input, err := reader.File[0].Open()
	if err != nil {
		return fmt.Errorf("open artifact.tar: %w", err)
	}
	output, err := os.OpenFile(archivePath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		_ = input.Close()
		return fmt.Errorf("create artifact.tar: %w", err)
	}
	_, copyErr := io.Copy(output, input)
	err = errors.Join(copyErr, input.Close(), output.Close())
	if err != nil {
		return fmt.Errorf("extract artifact.tar: %w", err)
	}
	return nil
}

func verifyBuildSiteAttestation(ctx context.Context, runner commandRunner, request *buildSiteAttestationRequest) error {
	_, err := runRequired(ctx, runner, &command{
		name: "gh", args: []string{
			"attestation", "verify", request.archivePath,
			"--repo", request.repository,
			"--signer-workflow", request.signerWorkflow,
			"--source-ref", request.sourceRef,
			"--source-digest", request.sourceDigest,
			"--deny-self-hosted-runners",
		},
	})
	if err != nil {
		return fmt.Errorf("verify Build Site artifact attestation: %w", err)
	}
	return nil
}

func validateCanonicalPreviousArchive(archivePath string, artifactDigest publication.Digest) (sitepublication.VerifiedSite, error) {
	outputDir, err := os.MkdirTemp("", "verity-previous-publication-verify-")
	if err != nil {
		return sitepublication.VerifiedSite{}, fmt.Errorf("create previous-publication verification directory: %w", err)
	}
	defer os.RemoveAll(outputDir)
	archive, err := os.Open(archivePath)
	if err != nil {
		return sitepublication.VerifiedSite{}, fmt.Errorf("open previous-publication archive: %w", err)
	}
	verified, extractErr := sitepublication.ExtractSiteArchive(archive, outputDir)
	closeErr := archive.Close()
	if err := errors.Join(extractErr, closeErr); err != nil {
		return sitepublication.VerifiedSite{}, err
	}
	return sitepublication.ValidateArchive(archivePath, artifactDigest, verified.ManifestDigest)
}

func restoreArchiveToOutput(options *RestorePreviousOptions, archivePath string) error {
	stage, err := prepareRestoreStage(options.OutputDir)
	if err != nil {
		return err
	}
	committed := false
	defer func() {
		if !committed {
			_ = os.RemoveAll(stage)
		}
	}()
	archive, err := os.Open(archivePath)
	if err != nil {
		return fmt.Errorf("open Build Site archive: %w", err)
	}
	verified, extractErr := sitepublication.ExtractSiteArchive(archive, stage)
	closeErr := archive.Close()
	if err := errors.Join(extractErr, closeErr); err != nil {
		return err
	}
	if verified.ManifestDigest != publication.Digest(options.ExpectedManifestDigest) {
		return fmt.Errorf("%w: expected %s, got %s", errPagesManifestDigest, options.ExpectedManifestDigest, verified.ManifestDigest)
	}
	if verified.Manifest.SourceSHA != publication.SourceSHA(options.ExpectedSourceSHA) ||
		uint64(verified.Manifest.RunID) != options.RunID || uint64(verified.Manifest.RunAttempt) != options.RunAttempt {
		return errWrongPagesRun
	}
	if err := replaceRestoredSite(stage, options.OutputDir); err != nil {
		return err
	}
	committed = true
	return nil
}

func prepareRestoreStage(outputDir string) (string, error) {
	if err := os.MkdirAll(filepath.Dir(outputDir), 0o755); err != nil {
		return "", fmt.Errorf("create restore output parent: %w", err)
	}
	stage, err := os.MkdirTemp(filepath.Dir(outputDir), ".build-site-restore-")
	if err != nil {
		return "", fmt.Errorf("create restore stage: %w", err)
	}
	if err := os.Remove(stage); err != nil {
		_ = os.RemoveAll(stage)
		return "", fmt.Errorf("prepare restore stage: %w", err)
	}
	return stage, nil
}

func replaceRestoredSite(stage, outputDir string) error {
	backup := outputDir + ".previous"
	if err := os.RemoveAll(backup); err != nil {
		return fmt.Errorf("remove stale restore backup: %w", err)
	}
	hadOutput := directoryExists(outputDir)
	if hadOutput {
		if err := os.Rename(outputDir, backup); err != nil {
			return fmt.Errorf("stage current site: %w", err)
		}
	}
	if err := os.Rename(stage, outputDir); err != nil {
		if hadOutput {
			if rollbackErr := os.Rename(backup, outputDir); rollbackErr != nil {
				return fmt.Errorf("publish restored site: %w (rollback failed: %w)", err, rollbackErr)
			}
		}
		return fmt.Errorf("publish restored site: %w", err)
	}
	if hadOutput {
		if err := os.RemoveAll(backup); err != nil {
			return fmt.Errorf("remove replaced site: %w", err)
		}
	}
	return nil
}
