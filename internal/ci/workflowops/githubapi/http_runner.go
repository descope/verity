package githubapi

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const maxResponseBytes = 16 << 20

type HTTPRunner struct {
	baseURL *url.URL
	token   string
	client  *http.Client
}

func NewHTTPRunner(baseURL, token string) (*HTTPRunner, error) {
	parsed, err := url.Parse(baseURL)
	if err != nil {
		return nil, fmt.Errorf("%w: parse API base URL", ErrInvalidRequest)
	}
	if (parsed.Scheme != "https" && parsed.Scheme != "http") || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return nil, fmt.Errorf("%w: API base URL is unsupported", ErrInvalidRequest)
	}

	transport := &http.Transport{
		Proxy:                 http.ProxyFromEnvironment,
		DialContext:           (&net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}).DialContext,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          100,
		MaxIdleConnsPerHost:   10,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: 30 * time.Second,
		ExpectContinueTimeout: time.Second,
	}
	return &HTTPRunner{
		baseURL: parsed,
		token:   token,
		client:  &http.Client{Transport: transport, Timeout: 60 * time.Second},
	}, nil
}

func (runner *HTTPRunner) Do(ctx context.Context, request Request) (response Response, retErr error) {
	if runner == nil || runner.baseURL == nil || runner.client == nil {
		return Response{}, fmt.Errorf("%w: HTTP runner is not initialized", ErrInvalidRequest)
	}
	if request.Method == "" || !strings.HasPrefix(request.Path, "/") {
		return Response{}, fmt.Errorf("%w: method and absolute API path are required", ErrInvalidRequest)
	}

	endpoint := *runner.baseURL
	endpoint.Path = strings.TrimRight(endpoint.Path, "/") + request.Path
	endpoint.RawQuery = request.Query.Encode()
	httpRequest, err := http.NewRequestWithContext(ctx, request.Method, endpoint.String(), bytes.NewReader(request.Body))
	if err != nil {
		return Response{}, fmt.Errorf("create GitHub request: %w", err)
	}
	httpRequest.Header.Set("Accept", "application/vnd.github+json")
	httpRequest.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	httpRequest.Header.Set("User-Agent", "verity-workflowops")
	if len(request.Body) > 0 {
		httpRequest.Header.Set("Content-Type", "application/json")
	}
	if runner.token != "" {
		httpRequest.Header.Set("Authorization", "Bearer "+runner.token)
	}

	httpResponse, err := runner.client.Do(httpRequest)
	if err != nil {
		return Response{}, fmt.Errorf("send GitHub request: %w", err)
	}
	defer func() {
		if closeErr := httpResponse.Body.Close(); closeErr != nil {
			retErr = errors.Join(retErr, fmt.Errorf("close GitHub response: %w", closeErr))
		}
	}()
	body, err := io.ReadAll(io.LimitReader(httpResponse.Body, maxResponseBytes+1))
	if err != nil {
		return Response{}, fmt.Errorf("read GitHub response: %w", err)
	}
	if len(body) > maxResponseBytes {
		return Response{}, fmt.Errorf("%w: response exceeds %d bytes", ErrInvalidResponse, maxResponseBytes)
	}
	return Response{StatusCode: httpResponse.StatusCode, Body: body}, nil
}
