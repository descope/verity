package githubapi

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"time"
)

const artifactsPerPage = 100

var (
	sourceSHAPattern    = regexp.MustCompile(`^(?:[0-9a-f]{40}|[0-9a-f]{64})$`)
	artifactNamePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,254}$`)
)

type GetRunAttemptRequest struct {
	Repository Repository
	RunID      int64
	RunAttempt int64
	SourceSHA  string
}

func (client Client) GetWorkflowRunAttempt(ctx context.Context, request GetRunAttemptRequest) (WorkflowRun, error) {
	if client.Runner == nil || request.Repository.String() == "" || request.RunID < 1 || request.RunAttempt < 1 || !sourceSHAPattern.MatchString(request.SourceSHA) {
		return WorkflowRun{}, fmt.Errorf("%w: exact workflow run request is incomplete", ErrInvalidRequest)
	}
	response, err := client.Runner.Do(ctx, Request{
		Method: http.MethodGet,
		Path: fmt.Sprintf(
			"/repos/%s/actions/runs/%d/attempts/%d",
			request.Repository.String(), request.RunID, request.RunAttempt,
		),
	})
	if err != nil {
		return WorkflowRun{}, fmt.Errorf("get workflow run attempt: %w", err)
	}
	if response.StatusCode == http.StatusNotFound {
		return WorkflowRun{}, ErrNotFound
	}
	if response.StatusCode != http.StatusOK {
		return WorkflowRun{}, &StatusError{StatusCode: response.StatusCode, Operation: "get workflow run attempt"}
	}
	var payload rawRun
	if err := decodeSingleJSON(response.Body, &payload); err != nil {
		return WorkflowRun{}, fmt.Errorf("%w: workflow run attempt: %w", ErrInvalidResponse, err)
	}
	run, err := parseRun(&payload, request.Repository)
	if err != nil {
		return WorkflowRun{}, err
	}
	if run.ID != request.RunID || run.Attempt != request.RunAttempt || run.HeadSHA != request.SourceSHA {
		return WorkflowRun{}, fmt.Errorf("%w: workflow run identity does not match request", ErrInvalidResponse)
	}
	return run, nil
}

type WorkflowArtifact struct {
	ID        int64
	Name      string
	CreatedAt time.Time
	Expired   bool
	RunID     int64
	HeadSHA   string
}

type GetRunArtifactRequest struct {
	Repository   Repository
	RunID        int64
	ArtifactName string
	SourceSHA    string
}

type rawArtifactsPage struct {
	TotalCount int           `json:"total_count"`
	Artifacts  []rawArtifact `json:"artifacts"`
}

type rawArtifact struct {
	ID          int64     `json:"id"`
	Name        string    `json:"name"`
	Expired     bool      `json:"expired"`
	CreatedAt   time.Time `json:"created_at"`
	WorkflowRun *struct {
		ID      int64  `json:"id"`
		HeadSHA string `json:"head_sha"`
	} `json:"workflow_run"`
}

func (client Client) GetWorkflowRunArtifact(ctx context.Context, request GetRunArtifactRequest) (WorkflowArtifact, error) {
	if err := validateRunArtifactRequest(client, request); err != nil {
		return WorkflowArtifact{}, err
	}

	matches := make([]WorkflowArtifact, 0, 1)
	seen := 0
	for page := 1; ; page++ {
		payload, err := client.listWorkflowRunArtifactsPage(ctx, request, page)
		if err != nil {
			return WorkflowArtifact{}, err
		}
		seen += len(payload.Artifacts)
		for index := range payload.Artifacts {
			if payload.Artifacts[index].Name != request.ArtifactName {
				continue
			}
			artifact, err := parseWorkflowArtifact(&payload.Artifacts[index], request)
			if err != nil {
				return WorkflowArtifact{}, err
			}
			matches = append(matches, artifact)
		}
		if len(payload.Artifacts) < artifactsPerPage || seen >= payload.TotalCount {
			break
		}
	}

	if len(matches) == 0 {
		return WorkflowArtifact{}, fmt.Errorf("%w: workflow artifact %q", ErrNotFound, request.ArtifactName)
	}
	if len(matches) != 1 {
		return WorkflowArtifact{}, fmt.Errorf("%w: workflow artifact %q is ambiguous", ErrInvalidResponse, request.ArtifactName)
	}
	if matches[0].Expired {
		return WorkflowArtifact{}, fmt.Errorf("%w: workflow artifact %q is expired", ErrNotFound, request.ArtifactName)
	}
	return matches[0], nil
}

func validateRunArtifactRequest(client Client, request GetRunArtifactRequest) error {
	if client.Runner == nil || request.Repository.String() == "" || request.RunID < 1 ||
		!artifactNamePattern.MatchString(request.ArtifactName) || !sourceSHAPattern.MatchString(request.SourceSHA) {
		return fmt.Errorf("%w: exact workflow artifact request is incomplete", ErrInvalidRequest)
	}
	return nil
}

func (client Client) listWorkflowRunArtifactsPage(ctx context.Context, request GetRunArtifactRequest, page int) (rawArtifactsPage, error) {
	response, err := client.Runner.Do(ctx, Request{
		Method: http.MethodGet,
		Path:   fmt.Sprintf("/repos/%s/actions/runs/%d/artifacts", request.Repository.String(), request.RunID),
		Query: url.Values{
			"per_page": []string{strconv.Itoa(artifactsPerPage)},
			"page":     []string{strconv.Itoa(page)},
		},
	})
	if err != nil {
		return rawArtifactsPage{}, fmt.Errorf("list workflow run artifacts: %w", err)
	}
	if response.StatusCode != http.StatusOK {
		return rawArtifactsPage{}, &StatusError{StatusCode: response.StatusCode, Operation: "list workflow run artifacts"}
	}
	var payload rawArtifactsPage
	if err := decodeSingleJSON(response.Body, &payload); err != nil {
		return rawArtifactsPage{}, fmt.Errorf("%w: workflow run artifacts: %w", ErrInvalidResponse, err)
	}
	if payload.TotalCount < 0 {
		return rawArtifactsPage{}, fmt.Errorf("%w: negative workflow artifact count", ErrInvalidResponse)
	}
	return payload, nil
}

func parseWorkflowArtifact(raw *rawArtifact, request GetRunArtifactRequest) (WorkflowArtifact, error) {
	if raw.ID < 1 || raw.Name == "" || raw.CreatedAt.IsZero() || raw.WorkflowRun == nil || raw.WorkflowRun.ID < 1 || !sourceSHAPattern.MatchString(raw.WorkflowRun.HeadSHA) {
		return WorkflowArtifact{}, fmt.Errorf("%w: workflow artifact required field", ErrInvalidResponse)
	}
	if raw.Name != request.ArtifactName || raw.WorkflowRun.ID != request.RunID || raw.WorkflowRun.HeadSHA != request.SourceSHA {
		return WorkflowArtifact{}, fmt.Errorf("%w: workflow artifact identity does not match request", ErrInvalidResponse)
	}
	return WorkflowArtifact{
		ID: raw.ID, Name: raw.Name, CreatedAt: raw.CreatedAt, Expired: raw.Expired,
		RunID: raw.WorkflowRun.ID, HeadSHA: raw.WorkflowRun.HeadSHA,
	}, nil
}
