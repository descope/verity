package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/verity-org/verity/internal/buildmetadata"
)

const (
	protectedRepository = "verity-org/verity"
	githubAPIVersion    = "2022-11-28"
)

type remoteOptions struct {
	APIBaseURL           string
	Token                string
	Repository           string
	RunID                int64
	RunAttempt           int64
	ArtifactName         string
	ArtifactDigest       string
	Identity             artifactIdentity
	ProtectedAttestation bool
	GitHubOutput         string
}

type remoteArtifactsResponse struct {
	TotalCount int              `json:"total_count"`
	Artifacts  []remoteArtifact `json:"artifacts"`
}

type remoteArtifact struct {
	ID          int64             `json:"id"`
	Name        string            `json:"name"`
	Expired     bool              `json:"expired"`
	Digest      string            `json:"digest"`
	WorkflowRun remoteWorkflowRun `json:"workflow_run"`
}

type remoteWorkflowRun struct {
	ID      int64  `json:"id"`
	HeadSHA string `json:"head_sha"`
}

func verifyRemoteArtifact(ctx context.Context, options *remoteOptions) error {
	name, err := buildArtifactName(options.Identity.BuildKey, options.RunID, options.RunAttempt)
	if err != nil || !options.Identity.valid() || options.ArtifactName != name {
		return artifactMismatch("immutable artifact name")
	}
	if err := validateRemoteOptions(options); err != nil {
		return err
	}
	if err := verifyCurrentRunAttempt(ctx, options); err != nil {
		return err
	}
	artifact, err := findCurrentRunArtifact(ctx, options)
	if err != nil {
		return err
	}
	if options.GitHubOutput != "" {
		if err := appendRemoteOutputs(options.GitHubOutput, artifact.ID, options.ProtectedAttestation); err != nil {
			return err
		}
	}
	return nil
}

func validateRemoteOptions(options *remoteOptions) error {
	if options.ProtectedAttestation && options.Repository != protectedRepository {
		return artifactMismatch("protected repository")
	}
	if options.APIBaseURL == "" || options.Token == "" || options.RunID <= 0 || options.RunAttempt <= 0 || !validArtifactDigest(options.ArtifactDigest) {
		return artifactMismatch("invalid verification input")
	}
	parts := strings.Split(options.Repository, "/")
	if len(parts) != 2 || !validRepositoryPart(parts[0]) || !validRepositoryPart(parts[1]) {
		return artifactMismatch("invalid repository")
	}
	return nil
}

func findCurrentRunArtifact(ctx context.Context, options *remoteOptions) (remoteArtifact, error) {
	next, expectedPath, err := artifactEndpoint(options)
	if err != nil {
		return remoteArtifact{}, err
	}
	client := &http.Client{Timeout: 30 * time.Second}
	seenPages := make(map[string]struct{})
	artifacts := newArtifactCollection()
	for next != nil {
		if len(seenPages) >= 1000 {
			return remoteArtifact{}, artifactMismatch("artifact API pagination limit")
		}
		pageURL := next.String()
		if _, exists := seenPages[pageURL]; exists {
			return remoteArtifact{}, artifactMismatch("artifact API pagination cycle")
		}
		seenPages[pageURL] = struct{}{}
		payload, link, err := fetchArtifactPage(ctx, client, pageURL, options.Token)
		if err != nil {
			return remoteArtifact{}, err
		}
		if err := artifacts.add(payload, options.ArtifactName); err != nil {
			return remoteArtifact{}, err
		}
		nextPage, err := nextArtifactPage(link, artifactPageRequest{
			Current: next, ExpectedPath: expectedPath, ArtifactName: options.ArtifactName,
		})
		if err != nil {
			return remoteArtifact{}, err
		}
		if nextPage.Host == "" {
			next = nil
		} else {
			next = &nextPage
		}
	}
	if !artifacts.complete() {
		return remoteArtifact{}, artifactMismatch("artifact API incomplete result set")
	}
	if len(artifacts.matches) != 1 || !matchesRemoteArtifact(artifacts.matches[0], options) {
		return remoteArtifact{}, artifactMismatch("artifact identity")
	}
	return artifacts.matches[0], nil
}

func artifactEndpoint(options *remoteOptions) (*url.URL, string, error) {
	base, err := url.Parse(options.APIBaseURL)
	if err != nil || (base.Scheme != "http" && base.Scheme != "https") || base.Host == "" || base.User != nil {
		return nil, "", artifactMismatch("artifact API base URL")
	}
	parts := strings.Split(options.Repository, "/")
	base.Path = path.Join("/", base.Path, "repos", parts[0], parts[1], "actions", "runs", strconv.FormatInt(options.RunID, 10), "artifacts")
	base.RawQuery = ""
	base.Fragment = ""
	query := base.Query()
	query.Set("name", options.ArtifactName)
	query.Set("per_page", "100")
	base.RawQuery = query.Encode()
	return base, base.Path, nil
}

func fetchArtifactPage(ctx context.Context, client *http.Client, endpoint, token string) (remoteArtifactsResponse, string, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, http.NoBody)
	if err != nil {
		return remoteArtifactsResponse{}, "", fmt.Errorf("create artifact verification request: %w", err)
	}
	request.Header.Set("Authorization", "Bearer "+token)
	request.Header.Set("Accept", "application/vnd.github+json")
	request.Header.Set("X-GitHub-Api-Version", githubAPIVersion)
	response, err := client.Do(request)
	if err != nil {
		return remoteArtifactsResponse{}, "", fmt.Errorf("request artifact verification: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return remoteArtifactsResponse{}, "", artifactMismatch("artifact API status")
	}
	var payload remoteArtifactsResponse
	decoder := json.NewDecoder(io.LimitReader(response.Body, 1<<20))
	if err := decoder.Decode(&payload); err != nil {
		return remoteArtifactsResponse{}, "", artifactMismatch("decode artifact API response")
	}
	var trailing json.RawMessage
	if err := decoder.Decode(&trailing); err != io.EOF {
		return remoteArtifactsResponse{}, "", artifactMismatch("trailing artifact API response")
	}
	return payload, response.Header.Get("Link"), nil
}

func matchesRemoteArtifact(artifact remoteArtifact, options *remoteOptions) bool {
	return artifact.Name == options.ArtifactName && !artifact.Expired && artifact.Digest == options.ArtifactDigest &&
		artifact.WorkflowRun.ID == options.RunID && artifact.WorkflowRun.HeadSHA == options.Identity.SourceSHA
}

func appendRemoteOutputs(outputPath string, artifactID int64, protected bool) (err error) {
	file, err := os.OpenFile(outputPath, os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0o600)
	if err != nil {
		return fmt.Errorf("open GitHub output: %w", err)
	}
	defer func() { err = errorsJoin(err, file.Close()) }()
	if _, err := fmt.Fprintf(file, "artifact-id=%d\nverify-attestation=%t\n", artifactID, protected); err != nil {
		return fmt.Errorf("write remote verification outputs: %w", err)
	}
	return nil
}

func validRepositoryPart(value string) bool {
	if value == "" {
		return false
	}
	for _, character := range value {
		if (character < 'a' || character > 'z') && (character < 'A' || character > 'Z') &&
			(character < '0' || character > '9') && character != '-' && character != '_' && character != '.' {
			return false
		}
	}
	return true
}

func validArtifactDigest(value string) bool {
	return strings.HasPrefix(value, "sha256:") && lowerHex(strings.TrimPrefix(value, "sha256:"), 64)
}

func artifactMismatch(reason string) error {
	return fmt.Errorf("%w: %s", buildmetadata.ErrArtifactMismatch, reason)
}
