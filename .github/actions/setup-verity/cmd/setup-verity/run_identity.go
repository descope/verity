package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"strings"
	"time"
)

const (
	buildVerityWorkflowPath          = ".github/workflows/build-verity.yaml"
	protectedBuildVerityWorkflowPath = ".github/workflows/build-verity-protected.yaml"
)

type remoteRunAttempt struct {
	ID                  int64                      `json:"id"`
	RunAttempt          int64                      `json:"run_attempt"`
	HeadSHA             string                     `json:"head_sha"`
	Repository          remoteRepository           `json:"repository"`
	ReferencedWorkflows []remoteReferencedWorkflow `json:"referenced_workflows"`
}

type remoteRepository struct {
	ID       int64  `json:"id"`
	FullName string `json:"full_name"`
}

type remoteReferencedWorkflow struct {
	Path string `json:"path"`
	SHA  string `json:"sha"`
}

func verifyCurrentRunAttempt(ctx context.Context, options *remoteOptions) error {
	endpoint, err := runAttemptEndpoint(options)
	if err != nil {
		return err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), http.NoBody)
	if err != nil {
		return fmt.Errorf("create workflow-run verification request: %w", err)
	}
	request.Header.Set("Authorization", "Bearer "+options.Token)
	request.Header.Set("Accept", "application/vnd.github+json")
	request.Header.Set("X-GitHub-Api-Version", githubAPIVersion)
	response, err := (&http.Client{Timeout: 30 * time.Second}).Do(request)
	if err != nil {
		return fmt.Errorf("request workflow-run verification: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return artifactMismatch("workflow-run API status")
	}
	var run remoteRunAttempt
	decoder := json.NewDecoder(io.LimitReader(response.Body, 1<<20))
	if err := decoder.Decode(&run); err != nil {
		return artifactMismatch("decode workflow-run API response")
	}
	var trailing json.RawMessage
	if err := decoder.Decode(&trailing); err != io.EOF {
		return artifactMismatch("trailing workflow-run API response")
	}
	if !matchesCurrentRunAttempt(&run, options) {
		return artifactMismatch("workflow-run identity")
	}
	return nil
}

func runAttemptEndpoint(options *remoteOptions) (*url.URL, error) {
	base, err := url.Parse(options.APIBaseURL)
	if err != nil || (base.Scheme != "http" && base.Scheme != "https") || base.Host == "" || base.User != nil {
		return nil, artifactMismatch("workflow-run API base URL")
	}
	parts := strings.Split(options.Repository, "/")
	if len(parts) != 2 {
		return nil, artifactMismatch("workflow-run repository")
	}
	base.Path = path.Join("/", base.Path, "repos", parts[0], parts[1], "actions", "runs",
		strconv.FormatInt(options.RunID, 10), "attempts", strconv.FormatInt(options.RunAttempt, 10))
	base.RawQuery = ""
	base.Fragment = ""
	return base, nil
}

func matchesCurrentRunAttempt(run *remoteRunAttempt, options *remoteOptions) bool {
	if run.ID != options.RunID || run.RunAttempt != options.RunAttempt || run.HeadSHA != options.Identity.SourceSHA ||
		run.Repository.ID <= 0 || run.Repository.FullName != options.Repository {
		return false
	}
	workflowPath := buildVerityWorkflowPath
	if options.ProtectedAttestation {
		workflowPath = protectedBuildVerityWorkflowPath
	}
	expectedPath := options.Repository + "/" + workflowPath
	matches := 0
	for _, workflow := range run.ReferencedWorkflows {
		separator := strings.LastIndex(workflow.Path, "@")
		if separator > 0 && workflow.Path[:separator] == expectedPath && workflow.SHA == options.Identity.SourceSHA {
			matches++
		}
	}
	return matches == 1
}
