package repositoryops

import (
	"errors"
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/google/go-containerregistry/pkg/name"
	"gopkg.in/yaml.v3"

	"github.com/verity-org/verity/internal/config"
)

var (
	ErrCatalogEntryNotFound  = errors.New("catalog image entry not found")
	ErrDuplicateCatalogEntry = errors.New("duplicate catalog image entry")
	ErrInvalidCatalogRequest = errors.New("invalid catalog request")
	imageTagPattern          = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._-]{0,127}$`)
)

type CatalogRequestInput struct {
	ConfigPath string
	ImageName  string
	ImageTag   string
}

type CatalogRequest struct {
	configPath string
	imageName  string
	imageTag   string
}

func NewCatalogRequest(input CatalogRequestInput) (CatalogRequest, error) {
	configPath, err := validatedPath("catalog config", input.ConfigPath)
	if err != nil {
		return CatalogRequest{}, err
	}
	imageName := strings.TrimSpace(input.ImageName)
	if imageName == "" || containsControl(imageName) {
		return CatalogRequest{}, fmt.Errorf("%w: image name is empty or contains control characters", ErrInvalidCatalogRequest)
	}
	imageTag := strings.TrimSpace(input.ImageTag)
	if !imageTagPattern.MatchString(imageTag) {
		return CatalogRequest{}, fmt.Errorf("%w: image tag %q", ErrInvalidCatalogRequest, imageTag)
	}
	return CatalogRequest{configPath: configPath, imageName: imageName, imageTag: imageTag}, nil
}

type CatalogEntry struct {
	Source   string
	GoVCSURL string
}

func ReadCatalogEntry(request CatalogRequest) (CatalogEntry, error) {
	data, err := os.ReadFile(request.configPath)
	if err != nil {
		return CatalogEntry{}, fmt.Errorf("read catalog %q: %w", request.configPath, err)
	}
	var catalog config.CopaConfig
	if err := yaml.Unmarshal(data, &catalog); err != nil {
		return CatalogEntry{}, fmt.Errorf("parse catalog %q: %w", request.configPath, err)
	}
	matches := make([]config.ImageSpec, 0, 1)
	for index := range catalog.Images {
		image := &catalog.Images[index]
		if image.Name == request.imageName {
			matches = append(matches, *image)
		}
	}
	if len(matches) == 0 {
		return CatalogEntry{}, fmt.Errorf("%w: %s", ErrCatalogEntryNotFound, request.imageName)
	}
	if len(matches) != 1 {
		return CatalogEntry{}, fmt.Errorf("%w: %s appears %d times", ErrDuplicateCatalogEntry, request.imageName, len(matches))
	}
	image := matches[0]
	source := strings.TrimSpace(image.Image) + ":" + request.imageTag
	if _, err := name.ParseReference(source, name.StrictValidation); err != nil {
		return CatalogEntry{}, fmt.Errorf("catalog source image %q: %w", source, err)
	}
	goVCSURL := ""
	if strings.TrimSpace(image.GoVcsURL) != "" {
		if err := validateGoVCSURL(image.GoVcsURL); err != nil {
			return CatalogEntry{}, fmt.Errorf("catalog Go VCS URL: %w", err)
		}
		if containsControl(image.GoVcsTagPrefix) {
			return CatalogEntry{}, fmt.Errorf("%w: Go VCS tag prefix contains control characters", ErrInvalidCatalogRequest)
		}
		goVCSURL = strings.TrimSpace(image.GoVcsURL) + "@" + image.GoVcsTagPrefix + request.imageTag
	}
	return CatalogEntry{Source: source, GoVCSURL: goVCSURL}, nil
}
