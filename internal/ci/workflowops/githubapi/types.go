package githubapi

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"time"
)

var (
	ErrInvalidRepository = errors.New("invalid GitHub repository")
	ErrInvalidRequest    = errors.New("invalid GitHub request")
	ErrInvalidResponse   = errors.New("invalid GitHub response")
	ErrNotFound          = errors.New("GitHub resource not found")
	ErrMultipleJSON      = errors.New("multiple JSON documents")
)

var repositoryPattern = regexp.MustCompile(`^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$`)

type Repository struct {
	value string
}

func NewRepository(value string) (Repository, error) {
	if !repositoryPattern.MatchString(value) || strings.Contains(value, "..") {
		return Repository{}, fmt.Errorf("%w: expected owner/name", ErrInvalidRepository)
	}
	return Repository{value: value}, nil
}

func (repository Repository) String() string {
	return repository.value
}

type Request struct {
	Method string
	Path   string
	Query  url.Values
	Body   []byte
}

type Response struct {
	StatusCode int
	Body       []byte
}

type Runner interface {
	Do(context.Context, Request) (Response, error)
}

type StatusError struct {
	StatusCode int
	Operation  string
}

func (err *StatusError) Error() string {
	return fmt.Sprintf("GitHub %s returned HTTP %d", err.Operation, err.StatusCode)
}

func (err *StatusError) Is(target error) bool {
	return target == ErrNotFound && err.StatusCode == 404
}

func (err *StatusError) Retryable() bool {
	return err.StatusCode == 408 || err.StatusCode == 409 || err.StatusCode == 429 || err.StatusCode >= 500
}

type WorkflowRun struct {
	ID             int64
	Attempt        int64
	Status         string
	Conclusion     string
	CreatedAt      time.Time
	URL            string
	Event          string
	DisplayTitle   string
	HeadBranch     string
	HeadSHA        string
	HeadRepository string
}

type ListRunsRequest struct {
	Repository Repository
	Workflow   string
	Branch     string
	Status     string
}
