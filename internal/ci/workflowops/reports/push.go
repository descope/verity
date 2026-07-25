package reports

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"strings"
	"time"

	"github.com/verity-org/verity/internal/ci/workflowops/githubapi"
	"github.com/verity-org/verity/internal/ci/workflowops/retry"
)

var (
	ErrInvalidPush        = errors.New("invalid report push request")
	ErrInvalidReport      = errors.New("invalid report JSON")
	ErrJSONObjectRequired = errors.New("top-level JSON object is required")
	ErrMultipleJSON       = errors.New("multiple JSON documents")
)

type File struct {
	RemotePath string
	LocalPath  string
}

type PushOptions struct {
	Repository     githubapi.Repository
	Branch         string
	Files          []File
	Attempts       int
	BaseDelay      time.Duration
	Jitter         time.Duration
	AttemptTimeout time.Duration
}

type PushResult struct {
	Pushed int
}

type Clock interface {
	Now() time.Time
}

type GitHub interface {
	GetContentSHA(context.Context, githubapi.GetContentRequest) (string, error)
	PutContent(context.Context, *githubapi.PutContentRequest) error
}

type Pusher struct {
	GitHub GitHub
	Engine retry.Engine
	Clock  Clock
	Stdout io.Writer
}

type preparedFile struct {
	remotePath string
	localPath  string
	content    string
}

func (pusher *Pusher) Push(ctx context.Context, options *PushOptions) (PushResult, error) {
	if err := validatePushOptions(options); err != nil {
		return PushResult{}, err
	}
	if pusher.GitHub == nil || pusher.Clock == nil {
		return PushResult{}, fmt.Errorf("%w: GitHub client and clock are required", ErrInvalidPush)
	}
	files, err := prepareFiles(options.Files)
	if err != nil {
		return PushResult{}, err
	}

	policy := retry.Policy{MaxAttempts: options.Attempts, BaseDelay: options.BaseDelay, Jitter: options.Jitter, AttemptTimeout: options.AttemptTimeout}
	for _, file := range files {
		message := fmt.Sprintf("chore: update %s @ %s", file.remotePath, pusher.Clock.Now().UTC().Format(time.RFC3339))
		err := pusher.Engine.Do(ctx, policy, func(attemptCtx context.Context, _ int) error {
			sha, err := pusher.GitHub.GetContentSHA(attemptCtx, githubapi.GetContentRequest{
				Repository: options.Repository, RemotePath: file.remotePath, Branch: options.Branch,
			})
			if err != nil && !errors.Is(err, githubapi.ErrNotFound) {
				return classifyGitHubError(err)
			}
			request := githubapi.PutContentRequest{
				Repository: options.Repository,
				RemotePath: file.remotePath,
				Branch:     options.Branch,
				Message:    message,
				Content:    file.content,
				SHA:        sha,
			}
			if err := pusher.GitHub.PutContent(attemptCtx, &request); err != nil {
				return classifyGitHubError(err)
			}
			return nil
		})
		if err != nil {
			return PushResult{}, fmt.Errorf("push report %q: %w", file.remotePath, err)
		}
		if pusher.Stdout != nil {
			if _, err := fmt.Fprintf(pusher.Stdout, "✓ Pushed %s → %s/%s\n", file.localPath, options.Branch, file.remotePath); err != nil {
				return PushResult{}, fmt.Errorf("write report push notice: %w", err)
			}
		}
	}
	return PushResult{Pushed: len(files)}, nil
}

func validatePushOptions(options *PushOptions) error {
	if options == nil || options.Repository.String() == "" || options.Branch == "" || len(options.Files) == 0 {
		return fmt.Errorf("%w: repository, branch, and files are required", ErrInvalidPush)
	}
	if options.Attempts < 1 || options.BaseDelay < 0 || options.Jitter < 0 || options.AttemptTimeout <= 0 {
		return fmt.Errorf("%w: retry policy is invalid", ErrInvalidPush)
	}
	return nil
}

func prepareFiles(files []File) ([]preparedFile, error) {
	prepared := make([]preparedFile, 0, len(files))
	for _, file := range files {
		if err := validateRemotePath(file.RemotePath); err != nil {
			return nil, err
		}
		info, err := os.Stat(file.LocalPath)
		if err != nil {
			return nil, fmt.Errorf("stat report %q: %w", file.LocalPath, err)
		}
		if !info.Mode().IsRegular() {
			return nil, fmt.Errorf("%w: report %q is not a regular file", ErrInvalidReport, file.LocalPath)
		}
		content, err := os.ReadFile(file.LocalPath)
		if err != nil {
			return nil, fmt.Errorf("read report %q: %w", file.LocalPath, err)
		}
		if err := validateJSONDocument(content); err != nil {
			return nil, fmt.Errorf("%w: %s: %w", ErrInvalidReport, file.LocalPath, err)
		}
		prepared = append(prepared, preparedFile{
			remotePath: file.RemotePath,
			localPath:  file.LocalPath,
			content:    base64.StdEncoding.EncodeToString(content),
		})
	}
	return prepared, nil
}

func validateRemotePath(value string) error {
	if value == "" || strings.Contains(value, "\\") || strings.HasPrefix(value, "/") || path.Clean(value) != value || path.Ext(value) != ".json" {
		return fmt.Errorf("%w: remote path %q is unsafe", ErrInvalidPush, value)
	}
	return nil
}

func validateJSONDocument(content []byte) error {
	trimmed := bytes.TrimSpace(content)
	if len(trimmed) == 0 || trimmed[0] != '{' {
		return ErrJSONObjectRequired
	}
	decoder := json.NewDecoder(bytes.NewReader(trimmed))
	var document map[string]json.RawMessage
	if err := decoder.Decode(&document); err != nil {
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

func classifyGitHubError(err error) error {
	if errors.Is(err, githubapi.ErrInvalidRequest) || errors.Is(err, githubapi.ErrInvalidResponse) {
		return retry.Permanent(err)
	}
	var statusErr *githubapi.StatusError
	if errors.As(err, &statusErr) && !statusErr.Retryable() {
		return retry.Permanent(err)
	}
	return err
}
