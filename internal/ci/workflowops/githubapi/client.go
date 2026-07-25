package githubapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const runsPerPage = 100

type Client struct {
	Runner Runner
}

type rawRunsPage struct {
	TotalCount   int      `json:"total_count"`
	WorkflowRuns []rawRun `json:"workflow_runs"`
}

type rawRun struct {
	ID             int64     `json:"id"`
	Attempt        int64     `json:"run_attempt"`
	Status         string    `json:"status"`
	Conclusion     *string   `json:"conclusion"`
	CreatedAt      time.Time `json:"created_at"`
	URL            string    `json:"html_url"`
	Event          string    `json:"event"`
	DisplayTitle   string    `json:"display_title"`
	HeadBranch     *string   `json:"head_branch"`
	HeadSHA        string    `json:"head_sha"`
	HeadRepository *struct {
		FullName string `json:"full_name"`
	} `json:"head_repository"`
}

func (client Client) ListWorkflowRuns(ctx context.Context, request ListRunsRequest) ([]WorkflowRun, error) {
	if client.Runner == nil {
		return nil, fmt.Errorf("%w: runner is required", ErrInvalidRequest)
	}
	if request.Repository.String() == "" || request.Workflow == "" || request.Branch == "" || request.Status == "" {
		return nil, fmt.Errorf("%w: repository, workflow, branch, and status are required", ErrInvalidRequest)
	}

	var runs []WorkflowRun
	for page := 1; ; page++ {
		query := url.Values{
			"branch":   []string{request.Branch},
			"status":   []string{request.Status},
			"per_page": []string{strconv.Itoa(runsPerPage)},
			"page":     []string{strconv.Itoa(page)},
		}
		response, err := client.Runner.Do(ctx, Request{
			Method: http.MethodGet,
			Path:   fmt.Sprintf("/repos/%s/actions/workflows/%s/runs", request.Repository.String(), url.PathEscape(request.Workflow)),
			Query:  query,
		})
		if err != nil {
			return nil, fmt.Errorf("list workflow runs: %w", err)
		}
		if response.StatusCode != http.StatusOK {
			return nil, &StatusError{StatusCode: response.StatusCode, Operation: "list workflow runs"}
		}

		var payload rawRunsPage
		if err := decodeSingleJSON(response.Body, &payload); err != nil {
			return nil, fmt.Errorf("%w: list workflow runs: %w", ErrInvalidResponse, err)
		}
		if payload.TotalCount < 0 {
			return nil, fmt.Errorf("%w: negative workflow run count", ErrInvalidResponse)
		}
		for index := range payload.WorkflowRuns {
			run, err := parseRun(&payload.WorkflowRuns[index], request.Repository)
			if err != nil {
				return nil, err
			}
			runs = append(runs, run)
		}
		if len(payload.WorkflowRuns) < runsPerPage || len(runs) >= payload.TotalCount {
			return runs, nil
		}
	}
}

type GetContentRequest struct {
	Repository Repository
	RemotePath string
	Branch     string
}

func (client Client) GetContentSHA(ctx context.Context, request GetContentRequest) (string, error) {
	if client.Runner == nil || request.Repository.String() == "" || request.RemotePath == "" || request.Branch == "" {
		return "", fmt.Errorf("%w: content metadata request is incomplete", ErrInvalidRequest)
	}
	response, err := client.Runner.Do(ctx, Request{
		Method: http.MethodGet,
		Path:   fmt.Sprintf("/repos/%s/contents/%s", request.Repository.String(), escapePath(request.RemotePath)),
		Query:  url.Values{"ref": []string{request.Branch}},
	})
	if err != nil {
		return "", fmt.Errorf("get report metadata: %w", err)
	}
	if response.StatusCode == http.StatusNotFound {
		return "", ErrNotFound
	}
	if response.StatusCode != http.StatusOK {
		return "", &StatusError{StatusCode: response.StatusCode, Operation: "get report metadata"}
	}
	var payload struct {
		SHA string `json:"sha"`
	}
	if err := decodeSingleJSON(response.Body, &payload); err != nil || payload.SHA == "" {
		return "", fmt.Errorf("%w: report metadata", ErrInvalidResponse)
	}
	return payload.SHA, nil
}

type PutContentRequest struct {
	Repository Repository
	RemotePath string
	Branch     string
	Message    string
	Content    string
	SHA        string
}

func (client Client) PutContent(ctx context.Context, request *PutContentRequest) error {
	if request == nil {
		return fmt.Errorf("%w: content mutation request is incomplete", ErrInvalidRequest)
	}
	if client.Runner == nil || request.Repository.String() == "" || request.RemotePath == "" || request.Branch == "" || request.Message == "" || request.Content == "" {
		return fmt.Errorf("%w: content mutation request is incomplete", ErrInvalidRequest)
	}
	payload := struct {
		Message string `json:"message"`
		Content string `json:"content"`
		Branch  string `json:"branch"`
		SHA     string `json:"sha,omitempty"`
	}{Message: request.Message, Content: request.Content, Branch: request.Branch, SHA: request.SHA}
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("encode report content: %w", err)
	}
	response, err := client.Runner.Do(ctx, Request{
		Method: http.MethodPut,
		Path:   fmt.Sprintf("/repos/%s/contents/%s", request.Repository.String(), escapePath(request.RemotePath)),
		Body:   body,
	})
	if err != nil {
		return fmt.Errorf("put report content: %w", err)
	}
	if response.StatusCode != http.StatusOK && response.StatusCode != http.StatusCreated {
		return &StatusError{StatusCode: response.StatusCode, Operation: "put report content"}
	}
	return nil
}

func parseRun(raw *rawRun, repository Repository) (WorkflowRun, error) {
	if raw.ID < 1 || raw.Attempt < 1 || raw.Status == "" || raw.CreatedAt.IsZero() || raw.URL == "" || raw.HeadBranch == nil || raw.HeadSHA == "" || raw.HeadRepository == nil {
		return WorkflowRun{}, fmt.Errorf("%w: workflow run required field", ErrInvalidResponse)
	}
	if raw.HeadRepository.FullName != repository.String() {
		return WorkflowRun{}, fmt.Errorf("%w: workflow run repository does not match", ErrInvalidResponse)
	}
	conclusion := ""
	if raw.Conclusion != nil {
		conclusion = *raw.Conclusion
	}
	return WorkflowRun{
		ID: raw.ID, Attempt: raw.Attempt, Status: raw.Status, Conclusion: conclusion,
		CreatedAt: raw.CreatedAt, URL: raw.URL, Event: raw.Event,
		DisplayTitle: raw.DisplayTitle, HeadBranch: *raw.HeadBranch,
		HeadSHA: raw.HeadSHA, HeadRepository: raw.HeadRepository.FullName,
	}, nil
}

func decodeSingleJSON[T any](data []byte, destination *T) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	if err := decoder.Decode(destination); err != nil {
		return err
	}
	var trailing json.RawMessage
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return ErrMultipleJSON
		}
		return err
	}
	return nil
}

func escapePath(value string) string {
	parts := strings.Split(value, "/")
	for index, part := range parts {
		parts[index] = url.PathEscape(part)
	}
	return strings.Join(parts, "/")
}
