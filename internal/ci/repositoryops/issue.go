package repositoryops

import (
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/google/go-containerregistry/pkg/name"
)

var (
	ErrInvalidImageName       = errors.New("invalid image name")
	ErrInvalidImageRepository = errors.New("invalid image repository")
	ErrInvalidImageTag        = errors.New("invalid image tag")
	ErrInvalidImageRegistry   = errors.New("invalid image registry")
	ErrMissingIssueField      = errors.New("missing image issue field")
	ErrInvalidImageIssue      = errors.New("invalid parsed image issue")
	imageNamePattern          = regexp.MustCompile(`^[a-z][a-z0-9._-]{0,127}$`)
	imageRepositoryPattern    = regexp.MustCompile(`^[a-z0-9][a-z0-9._/-]{0,255}$`)
)

var allowedRegistries = map[string]struct{}{
	"docker.io":         {},
	"gcr.io":            {},
	"mirror.gcr.io":     {},
	"ghcr.io":           {},
	"quay.io":           {},
	"mcr.microsoft.com": {},
	"registry.k8s.io":   {},
	"public.ecr.aws":    {},
}

type ImageIssue struct {
	name       string
	repository string
	tag        string
	registry   string
}

type ImageIssueInput struct {
	Name       string
	Repository string
	Tag        string
	Registry   string
}

func ParseImageIssue(body string) (ImageIssue, error) {
	if strings.TrimSpace(body) == "" {
		return ImageIssue{}, ErrMissingIssueField
	}
	fields := issueFields(body)
	return NewImageIssue(ImageIssueInput{
		Name: fields["Image name"], Repository: fields["Image repository"],
		Tag: fields["Image tag"], Registry: fields["Image registry"],
	})
}

func NewImageIssue(input ImageIssueInput) (ImageIssue, error) {
	imageName := strings.TrimSpace(input.Name)
	repository := strings.TrimSpace(input.Repository)
	tag := strings.TrimSpace(input.Tag)
	registry := strings.TrimSpace(input.Registry)
	if registry == "" {
		registry = "docker.io"
	}
	if imageName == "" || repository == "" || tag == "" {
		return ImageIssue{}, ErrMissingIssueField
	}
	if !imageNamePattern.MatchString(imageName) {
		return ImageIssue{}, fmt.Errorf("%w: %q", ErrInvalidImageName, imageName)
	}
	if !imageRepositoryPattern.MatchString(repository) {
		return ImageIssue{}, fmt.Errorf("%w: %q", ErrInvalidImageRepository, repository)
	}
	if !imageTagPattern.MatchString(tag) {
		return ImageIssue{}, fmt.Errorf("%w: %q", ErrInvalidImageTag, tag)
	}
	if _, ok := allowedRegistries[registry]; !ok {
		return ImageIssue{}, fmt.Errorf("%w: %q", ErrInvalidImageRegistry, registry)
	}
	if _, err := name.NewRepository(registry+"/"+repository, name.StrictValidation); err != nil {
		return ImageIssue{}, fmt.Errorf("%w: %w", ErrInvalidImageRepository, err)
	}
	return ImageIssue{name: imageName, repository: repository, tag: tag, registry: registry}, nil
}

func (i ImageIssue) Name() string {
	return i.name
}

func (i ImageIssue) Repository() string {
	return i.repository
}

func (i ImageIssue) Tag() string {
	return i.tag
}

func (i ImageIssue) Registry() string {
	return i.registry
}

func (i ImageIssue) ImageRepository() string {
	return i.registry + "/" + i.repository
}

func issueFields(body string) map[string]string {
	wanted := map[string]struct{}{
		"Image name": {}, "Image repository": {}, "Image tag": {}, "Image registry": {},
	}
	fields := make(map[string]string, len(wanted))
	current := ""
	for line := range strings.SplitSeq(strings.ReplaceAll(body, "\r\n", "\n"), "\n") {
		if heading, ok := strings.CutPrefix(line, "### "); ok {
			label := strings.TrimSpace(heading)
			if _, ok := wanted[label]; ok {
				current = label
			} else {
				current = ""
			}
			continue
		}
		if current == "" || fields[current] != "" {
			continue
		}
		value := strings.Join(strings.Fields(line), " ")
		if value != "" {
			fields[current] = value
		}
	}
	return fields
}
