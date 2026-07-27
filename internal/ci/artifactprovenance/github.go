package artifactprovenance

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const defaultAPIBaseURL = "https://api.github.com"

var errGitHubAPI = errors.New("GitHub artifact API request failed")

type githubClient struct {
	baseURL string
	token   string
	client  *http.Client
}

type workflowRun struct {
	ID         uint64 `json:"id"`
	RunAttempt uint64 `json:"run_attempt"`
	HeadSHA    string `json:"head_sha"`
	Repository struct {
		FullName string `json:"full_name"`
	} `json:"repository"`
}

type artifactList struct {
	TotalCount int `json:"total_count"`
	Artifacts  []struct {
		ID          uint64 `json:"id"`
		Name        string `json:"name"`
		Digest      string `json:"digest"`
		Expired     bool   `json:"expired"`
		WorkflowRun struct {
			ID      uint64 `json:"id"`
			HeadSHA string `json:"head_sha"`
		} `json:"workflow_run"`
	} `json:"artifacts"`
}

func newGitHubClient(baseURL, token string, client *http.Client) (*githubClient, error) {
	if strings.TrimSpace(token) == "" {
		return nil, fmt.Errorf("%w: GitHub token is required", ErrInvalidProvenance)
	}
	if baseURL == "" {
		baseURL = defaultAPIBaseURL
	}
	parsed, err := url.Parse(baseURL)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return nil, fmt.Errorf("%w: API base URL %q", ErrInvalidProvenance, baseURL)
	}
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	return &githubClient{baseURL: strings.TrimSuffix(baseURL, "/"), token: token, client: client}, nil
}

func (client *githubClient) run(ctx context.Context, identity *Identity) (workflowRun, error) {
	var result workflowRun
	path := fmt.Sprintf("/repos/%s/actions/runs/%d", identity.repository, identity.runID)
	if err := client.getJSON(ctx, path, &result); err != nil {
		return workflowRun{}, fmt.Errorf("get workflow run: %w", err)
	}
	return result, nil
}

func (client *githubClient) artifacts(ctx context.Context, identity *Identity) (artifactList, error) {
	var result artifactList
	path := fmt.Sprintf("/repos/%s/actions/runs/%d/artifacts?name=%s", identity.repository, identity.runID, url.QueryEscape(identity.artifactName))
	if err := client.getJSON(ctx, path, &result); err != nil {
		return artifactList{}, fmt.Errorf("get workflow artifacts: %w", err)
	}
	return result, nil
}

func (client *githubClient) getJSON(ctx context.Context, path string, target any) error {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, client.baseURL+path, http.NoBody)
	if err != nil {
		return fmt.Errorf("create GitHub request: %w", err)
	}
	request.Header.Set("Authorization", "Bearer "+client.token)
	request.Header.Set("Accept", "application/vnd.github+json")
	request.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	response, err := client.client.Do(request)
	if err != nil {
		return fmt.Errorf("execute GitHub request: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		body, readErr := io.ReadAll(io.LimitReader(response.Body, 4096))
		if readErr != nil {
			return fmt.Errorf("%w: status %d: read body: %w", errGitHubAPI, response.StatusCode, readErr)
		}
		return fmt.Errorf("%w: status %d: %s", errGitHubAPI, response.StatusCode, strings.TrimSpace(string(body)))
	}
	decoder := json.NewDecoder(io.LimitReader(response.Body, 1<<20))
	if err := decoder.Decode(target); err != nil {
		return fmt.Errorf("decode GitHub response: %w", err)
	}
	return nil
}
