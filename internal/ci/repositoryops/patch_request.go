package repositoryops

import (
	"errors"
	"fmt"
	"net/url"
	"path/filepath"
	"strings"
	"unicode"

	"github.com/google/go-containerregistry/pkg/name"

	veritypatch "github.com/verity-org/verity/internal/patch"
)

var (
	ErrInvalidPatchRequest = errors.New("invalid patch request")
	ErrNoPatchUpdates      = veritypatch.ErrNoUpdatesFound
)

type PatchRequestInput struct {
	Platform        string
	Source          string
	ImageName       string
	StagingRegistry string
	GoVCSURL        string
	Report          string
}

type PatchRequest struct {
	platform        string
	source          string
	imageName       string
	stagingRegistry string
	goVCSURL        string
	sourceTag       string
	destination     string
	report          string
}

func NewPatchRequest(input *PatchRequestInput) (PatchRequest, error) {
	if input == nil {
		return PatchRequest{}, fmt.Errorf("%w: input is required", ErrInvalidPatchRequest)
	}
	platform, err := parsePlatform(input.Platform)
	if err != nil {
		return PatchRequest{}, err
	}
	source := strings.TrimSpace(input.Source)
	if _, err := name.ParseReference(source, name.StrictValidation); err != nil {
		return PatchRequest{}, fmt.Errorf("%w: source image: %w", ErrInvalidPatchRequest, err)
	}
	sourceTag, err := taggedReferenceTag(source)
	if err != nil {
		return PatchRequest{}, err
	}
	imageName := strings.TrimSpace(input.ImageName)
	if imageName == "" || containsControl(imageName) {
		return PatchRequest{}, fmt.Errorf("%w: image name is empty or contains control characters", ErrInvalidPatchRequest)
	}
	staging := strings.TrimSpace(input.StagingRegistry)
	if _, err := name.NewRepository(staging, name.StrictValidation); err != nil {
		return PatchRequest{}, fmt.Errorf("%w: staging registry: %w", ErrInvalidPatchRequest, err)
	}
	if err := validateGoVCSURL(input.GoVCSURL); err != nil {
		return PatchRequest{}, err
	}

	safeImage := strings.NewReplacer("/", "-", ":", "-", " ", "-").Replace(imageName)
	arch := strings.TrimPrefix(platform, "linux/")
	destination := staging + ":" + safeImage + "-" + sourceTag + "-" + arch
	if _, err := name.NewTag(destination, name.StrictValidation); err != nil {
		return PatchRequest{}, fmt.Errorf("%w: destination image: %w", ErrInvalidPatchRequest, err)
	}
	report := strings.TrimSpace(input.Report)
	if report == "" {
		reportName := strings.NewReplacer("/", "_", ":", "_").Replace(source) + ".json"
		report = filepath.Join("reports", reportName)
	} else {
		report, err = validatedPath("Trivy report", report)
		if err != nil {
			return PatchRequest{}, err
		}
	}
	return PatchRequest{
		platform: platform, source: source, imageName: imageName, stagingRegistry: staging,
		goVCSURL: strings.TrimSpace(input.GoVCSURL), sourceTag: sourceTag,
		destination: destination, report: report,
	}, nil
}

func (r *PatchRequest) Source() string {
	if r == nil {
		return ""
	}
	return r.source
}

func parsePlatform(value string) (string, error) {
	switch strings.TrimSpace(value) {
	case "linux/amd64":
		return "linux/amd64", nil
	case "linux/arm64":
		return "linux/arm64", nil
	default:
		return "", fmt.Errorf("%w: unsupported platform %q", ErrInvalidPatchRequest, value)
	}
}

func taggedReferenceTag(reference string) (string, error) {
	withoutDigest, _, _ := strings.Cut(reference, "@")
	lastSlash := strings.LastIndex(withoutDigest, "/")
	lastColon := strings.LastIndex(withoutDigest, ":")
	if lastColon <= lastSlash || lastColon == len(withoutDigest)-1 {
		return "", fmt.Errorf("%w: source image must include a tag", ErrInvalidPatchRequest)
	}
	return withoutDigest[lastColon+1:], nil
}

func validateGoVCSURL(value string) error {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	parsed, err := url.ParseRequestURI(value)
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" || containsControl(value) {
		return fmt.Errorf("%w: invalid Go VCS URL %q", ErrInvalidPatchRequest, value)
	}
	return nil
}

func containsControl(value string) bool {
	return strings.IndexFunc(value, unicode.IsControl) >= 0
}
