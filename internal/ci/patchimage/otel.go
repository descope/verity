package patchimage

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

const defaultOtelBaseURL = "https://github.com/equinix-labs/otel-cli/releases/download"

var (
	ErrUnsupportedRunner = errors.New("unsupported otel-cli runner")
	ErrOtelDigest        = errors.New("otel-cli archive digest mismatch")
	ErrOtelBinaryMissing = errors.New("otel-cli binary missing from archive")
	ErrOtelHTTPStatus    = errors.New("otel-cli download returned non-success status")
)

type HTTPDoer interface {
	Do(*http.Request) (*http.Response, error)
}

type OtelInstaller struct {
	Client  HTTPDoer
	BaseURL string
}

type OtelInstallInput struct {
	Version        string
	GOOS           string
	GOARCH         string
	ExpectedSHA256 string
	HomeDir        string
	GitHubPath     string
}

func (installer OtelInstaller) Install(ctx context.Context, input *OtelInstallInput) error {
	if input.GOOS != "linux" || (input.GOARCH != "amd64" && input.GOARCH != "arm64") {
		return fmt.Errorf("%w: %s_%s", ErrUnsupportedRunner, input.GOOS, input.GOARCH)
	}
	baseURL := installer.BaseURL
	if baseURL == "" {
		baseURL = defaultOtelBaseURL
	}
	archiveURL := fmt.Sprintf("%s/v%s/otel-cli_%s_%s_%s.tar.gz", strings.TrimRight(baseURL, "/"), input.Version, input.Version, input.GOOS, input.GOARCH)
	archive, err := installer.download(ctx, archiveURL)
	if err != nil {
		return err
	}
	if err := verifyOtelDigest(archive, input.ExpectedSHA256); err != nil {
		return err
	}
	binary, err := extractOtelBinary(archive)
	if err != nil {
		return err
	}
	return installOtelBinary(input, binary)
}

func (installer OtelInstaller) download(ctx context.Context, archiveURL string) ([]byte, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, archiveURL, http.NoBody)
	if err != nil {
		return nil, fmt.Errorf("create otel-cli request: %w", err)
	}
	client := installer.Client
	if client == nil {
		client = http.DefaultClient
	}
	response, err := client.Do(request)
	if err != nil {
		return nil, fmt.Errorf("download otel-cli: %w", err)
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		closeErr := response.Body.Close()
		return nil, errors.Join(fmt.Errorf("%w: HTTP %d", ErrOtelHTTPStatus, response.StatusCode), closeErr)
	}
	archive, err := io.ReadAll(io.LimitReader(response.Body, 64<<20))
	closeErr := response.Body.Close()
	if err != nil {
		return nil, errors.Join(fmt.Errorf("read otel-cli archive: %w", err), closeErr)
	}
	if closeErr != nil {
		return nil, fmt.Errorf("close otel-cli response body: %w", closeErr)
	}
	return archive, nil
}

func installOtelBinary(input *OtelInstallInput, binary []byte) error {
	binDir := filepath.Join(input.HomeDir, ".local", "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		return fmt.Errorf("create otel-cli bin directory: %w", err)
	}
	if err := os.WriteFile(filepath.Join(binDir, "otel-cli"), binary, 0o755); err != nil {
		return fmt.Errorf("write otel-cli binary: %w", err)
	}
	if err := appendLine(input.GitHubPath, binDir); err != nil {
		return err
	}
	return nil
}

func verifyOtelDigest(archive []byte, expected string) error {
	expectedBytes, err := hex.DecodeString(expected)
	if err != nil || len(expectedBytes) != sha256.Size {
		return fmt.Errorf("%w: invalid expected digest", ErrOtelDigest)
	}
	digest := sha256.Sum256(archive)
	if subtle.ConstantTimeCompare(digest[:], expectedBytes) != 1 {
		return ErrOtelDigest
	}
	return nil
}

func extractOtelBinary(archive []byte) (binary []byte, err error) {
	gzipReader, err := gzip.NewReader(bytes.NewReader(archive))
	if err != nil {
		return nil, fmt.Errorf("open otel-cli gzip archive: %w", err)
	}
	defer func() {
		if closeErr := gzipReader.Close(); closeErr != nil {
			err = errors.Join(err, fmt.Errorf("close otel-cli gzip archive: %w", closeErr))
		}
	}()
	tarReader := tar.NewReader(gzipReader)
	for {
		header, nextErr := tarReader.Next()
		if errors.Is(nextErr, io.EOF) {
			return nil, ErrOtelBinaryMissing
		}
		if nextErr != nil {
			return nil, fmt.Errorf("read otel-cli tar archive: %w", nextErr)
		}
		if filepath.Base(header.Name) != "otel-cli" || !header.FileInfo().Mode().IsRegular() {
			continue
		}
		binary, readErr := io.ReadAll(io.LimitReader(tarReader, 64<<20))
		if readErr != nil {
			return nil, fmt.Errorf("read otel-cli binary: %w", readErr)
		}
		return binary, nil
	}
}

func appendLine(path, value string) (err error) {
	file, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("open append target %q: %w", path, err)
	}
	defer func() {
		if closeErr := file.Close(); closeErr != nil && err == nil {
			err = fmt.Errorf("close append target %q: %w", path, closeErr)
		}
	}()
	if _, err := fmt.Fprintln(file, value); err != nil {
		return fmt.Errorf("append target %q: %w", path, err)
	}
	return nil
}
