package buildmetadata

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
)

// ErrArtifactMismatch reports an untrusted remote artifact identity.
var ErrArtifactMismatch = errors.New("remote artifact mismatch")

const protectedRepository = "verity-org/verity"

// RemoteVerifyOptions identifies the GitHub Actions artifact to attest.
type RemoteVerifyOptions struct {
	APIBaseURL           string
	Token                string
	Repository           string
	RunID                int64
	ArtifactName         string
	ArtifactDigest       string
	SourceSHA            string
	ProtectedAttestation bool
}

type remoteArtifactsResponse struct {
	Artifacts []remoteArtifact `json:"artifacts"`
}

type remoteArtifact struct {
	Name        string            `json:"name"`
	Expired     bool              `json:"expired"`
	Digest      string            `json:"digest"`
	WorkflowRun remoteWorkflowRun `json:"workflow_run"`
}

type remoteWorkflowRun struct {
	ID      int64  `json:"id"`
	HeadSHA string `json:"head_sha"`
}

// VerifyCurrentRunArtifact verifies exact identity from GitHub's artifact API.
//
//nolint:gocritic // Keep the value options API aligned with the attestation seam.
func VerifyCurrentRunArtifact(ctx context.Context, options RemoteVerifyOptions) error {
	if err := validateRemoteOptions(&options); err != nil {
		return err
	}
	artifacts, err := fetchRemoteArtifacts(ctx, &options)
	if err != nil {
		return err
	}
	for _, artifact := range artifacts {
		if matchesRemoteArtifact(artifact, &options) {
			return nil
		}
	}
	return artifactMismatch("artifact identity")
}

func validateRemoteOptions(options *RemoteVerifyOptions) error {
	if options.ProtectedAttestation && options.Repository != protectedRepository {
		return artifactMismatch("protected repository")
	}
	if options.APIBaseURL == "" || options.Repository == "" || options.Token == "" || options.RunID <= 0 || options.ArtifactName == "" || !isLowerHex(options.SourceSHA, 40) || !validArtifactDigest(options.ArtifactDigest) {
		return artifactMismatch("invalid verification input")
	}
	return nil
}

func fetchRemoteArtifacts(ctx context.Context, options *RemoteVerifyOptions) ([]remoteArtifact, error) {
	endpoint := strings.TrimRight(options.APIBaseURL, "/") + "/repos/" + options.Repository + "/actions/runs/" + strconv.FormatInt(options.RunID, 10) + "/artifacts"
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, http.NoBody)
	if err != nil {
		return nil, fmt.Errorf("create artifact verification request: %w", err)
	}
	request.Header.Set("Authorization", "Bearer "+options.Token)
	request.Header.Set("Accept", "application/vnd.github+json")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return nil, fmt.Errorf("request artifact verification: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, artifactMismatch("artifact API status")
	}

	var payload remoteArtifactsResponse
	decoder := json.NewDecoder(io.LimitReader(response.Body, 1<<20))
	if err := decoder.Decode(&payload); err != nil {
		return nil, artifactMismatch("decode artifact API response")
	}
	return payload.Artifacts, nil
}

func matchesRemoteArtifact(artifact remoteArtifact, options *RemoteVerifyOptions) bool {
	return artifact.Name == options.ArtifactName && !artifact.Expired && artifact.Digest == options.ArtifactDigest && artifact.WorkflowRun.ID == options.RunID && artifact.WorkflowRun.HeadSHA == options.SourceSHA
}

func validArtifactDigest(value string) bool {
	return strings.HasPrefix(value, "sha256:") && isLowerHex(strings.TrimPrefix(value, "sha256:"), 64)
}

func artifactMismatch(reason string) error {
	return fmt.Errorf("%w: %s", ErrArtifactMismatch, reason)
}
