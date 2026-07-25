package artifactprovenance

import (
	"context"
	"fmt"
	"net/http"
)

type VerifyOptions struct {
	Identity       Identity
	ArtifactDigest string
	ManifestPath   string
	Token          string
	APIBaseURL     string
	HTTPClient     *http.Client
}

func VerifyDownloaded(ctx context.Context, options *VerifyOptions) error {
	digest, err := parseArtifactDigest(options.ArtifactDigest)
	if err != nil {
		return err
	}
	manifestIdentity, err := readManifest(options.ManifestPath)
	if err != nil {
		return err
	}
	if !manifestIdentity.matches(&options.Identity) {
		return fmt.Errorf("%w: downloaded manifest identity differs from expected identity", ErrProvenanceMismatch)
	}
	client, err := newGitHubClient(options.APIBaseURL, options.Token, options.HTTPClient)
	if err != nil {
		return err
	}
	run, err := client.run(ctx, &options.Identity)
	if err != nil {
		return err
	}
	if err := verifyRun(run, &options.Identity); err != nil {
		return err
	}
	artifacts, err := client.artifacts(ctx, &options.Identity)
	if err != nil {
		return err
	}
	return verifyArtifactList(artifacts, &options.Identity, digest)
}

func verifyRun(run workflowRun, expected *Identity) error {
	switch {
	case run.ID != expected.runID:
		return fmt.Errorf("%w: run ID %d", ErrProvenanceMismatch, run.ID)
	case run.RunAttempt != expected.runAttempt:
		return fmt.Errorf("%w: run attempt %d", ErrProvenanceMismatch, run.RunAttempt)
	case run.HeadSHA != expected.sourceSHA:
		return fmt.Errorf("%w: run source SHA %q", ErrProvenanceMismatch, run.HeadSHA)
	case run.Repository.FullName != expected.repository:
		return fmt.Errorf("%w: run repository %q", ErrProvenanceMismatch, run.Repository.FullName)
	default:
		return nil
	}
}

func verifyArtifactList(list artifactList, expected *Identity, digest string) error {
	if list.TotalCount != 1 || len(list.Artifacts) != 1 {
		return fmt.Errorf("%w: expected one artifact, got %d", ErrProvenanceMismatch, len(list.Artifacts))
	}
	artifact := list.Artifacts[0]
	switch {
	case artifact.Expired:
		return fmt.Errorf("%w: artifact is expired", ErrProvenanceMismatch)
	case artifact.Name != expected.artifactName:
		return fmt.Errorf("%w: artifact name %q", ErrProvenanceMismatch, artifact.Name)
	case artifact.Digest != digest:
		return fmt.Errorf("%w: artifact digest %q", ErrProvenanceMismatch, artifact.Digest)
	case artifact.WorkflowRun.ID != expected.runID:
		return fmt.Errorf("%w: artifact run ID %d", ErrProvenanceMismatch, artifact.WorkflowRun.ID)
	case artifact.WorkflowRun.HeadSHA != expected.sourceSHA:
		return fmt.Errorf("%w: artifact source SHA %q", ErrProvenanceMismatch, artifact.WorkflowRun.HeadSHA)
	default:
		return nil
	}
}
