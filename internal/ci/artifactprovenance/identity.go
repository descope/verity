package artifactprovenance

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
)

var (
	ErrInvalidProvenance  = errors.New("invalid artifact provenance")
	ErrProvenanceMismatch = errors.New("artifact provenance mismatch")
)

var (
	repositoryPattern     = regexp.MustCompile(`^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$`)
	sourceSHAPattern      = regexp.MustCompile(`^[a-f0-9]{40}([a-f0-9]{24})?$`)
	publicationIDPattern  = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)
	artifactNamePattern   = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)
	artifactDigestPattern = regexp.MustCompile(
		`^sha256:[a-f0-9]{64}$`,
	)
)

type IdentityInput struct {
	Repository    string
	RunID         uint64
	RunAttempt    uint64
	SourceSHA     string
	PublicationID string
	ArtifactName  string
}

type Identity struct {
	repository    string
	runID         uint64
	runAttempt    uint64
	sourceSHA     string
	publicationID string
	artifactName  string
}

func ParseIdentity(input *IdentityInput) (Identity, error) {
	input.Repository = strings.TrimSpace(input.Repository)
	input.SourceSHA = strings.TrimSpace(input.SourceSHA)
	input.PublicationID = strings.TrimSpace(input.PublicationID)
	input.ArtifactName = strings.TrimSpace(input.ArtifactName)
	switch {
	case !repositoryPattern.MatchString(input.Repository):
		return Identity{}, fmt.Errorf("%w: repository %q", ErrInvalidProvenance, input.Repository)
	case input.RunID == 0:
		return Identity{}, fmt.Errorf("%w: run ID must be positive", ErrInvalidProvenance)
	case input.RunAttempt == 0:
		return Identity{}, fmt.Errorf("%w: run attempt must be positive", ErrInvalidProvenance)
	case !sourceSHAPattern.MatchString(input.SourceSHA):
		return Identity{}, fmt.Errorf("%w: source SHA %q", ErrInvalidProvenance, input.SourceSHA)
	case !publicationIDPattern.MatchString(input.PublicationID):
		return Identity{}, fmt.Errorf("%w: publication ID %q", ErrInvalidProvenance, input.PublicationID)
	case !artifactNamePattern.MatchString(input.ArtifactName):
		return Identity{}, fmt.Errorf("%w: artifact name %q", ErrInvalidProvenance, input.ArtifactName)
	default:
		return Identity{
			repository: input.Repository, runID: input.RunID, runAttempt: input.RunAttempt,
			sourceSHA: input.SourceSHA, publicationID: input.PublicationID, artifactName: input.ArtifactName,
		}, nil
	}
}

func parseArtifactDigest(value string) (string, error) {
	value = strings.TrimSpace(value)
	if !artifactDigestPattern.MatchString(value) {
		return "", fmt.Errorf("%w: artifact digest %q", ErrInvalidProvenance, value)
	}
	return value, nil
}

func (identity *Identity) matches(other *Identity) bool {
	return *identity == *other
}
