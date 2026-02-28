package discovery

import (
	"errors"
	"fmt"
	"net"
	"regexp"
	"sort"
	"strings"

	"github.com/Masterminds/semver/v3"
	"github.com/google/go-containerregistry/pkg/crane"

	"github.com/verity-org/verity/internal/config"
)

// ErrUnknownStrategy is returned when an image spec has an unrecognized tag strategy.
var ErrUnknownStrategy = errors.New("unknown tag strategy")

// craneOptions returns crane options for the given image ref.
// Localhost registries (127.0.0.1, localhost) use plain HTTP.
func craneOptions(image string) []crane.Option {
	host := image
	if idx := strings.Index(host, "/"); idx != -1 {
		host = host[:idx]
	}
	// Strip port for host matching
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}
	if host == "localhost" || host == "127.0.0.1" || host == "::1" {
		return []crane.Option{crane.Insecure}
	}
	return nil
}

// FindTagsToPatch discovers the set of tags to patch for a given image spec.
func FindTagsToPatch(spec *config.ImageSpec) ([]string, error) {
	switch spec.Tags.Strategy {
	case "list":
		result := make([]string, len(spec.Tags.List))
		copy(result, spec.Tags.List)
		return result, nil
	case "pattern":
		return findTagsByPattern(spec)
	case "latest":
		return findTagsByLatest(spec)
	default:
		return nil, fmt.Errorf("%w: %s", ErrUnknownStrategy, spec.Tags.Strategy)
	}
}

func findTagsByLatest(spec *config.ImageSpec) ([]string, error) {
	allTags, err := crane.ListTags(spec.Image, craneOptions(spec.Image)...)
	if err != nil {
		return nil, err
	}

	versions := tagsToSortedVersions(ExcludeTags(allTags, spec.Tags.Exclude))
	if len(versions) == 0 {
		return []string{}, nil
	}
	return []string{versions[len(versions)-1].Original()}, nil
}

func findTagsByPattern(spec *config.ImageSpec) ([]string, error) {
	allTags, err := crane.ListTags(spec.Image, craneOptions(spec.Image)...)
	if err != nil {
		return nil, err
	}

	pattern, err := regexp.Compile(spec.Tags.Pattern)
	if err != nil {
		return nil, err
	}

	var matchingTags []string
	for _, tag := range allTags {
		if pattern.MatchString(tag) {
			matchingTags = append(matchingTags, tag)
		}
	}

	versions := tagsToSortedVersions(ExcludeTags(matchingTags, spec.Tags.Exclude))
	if len(versions) == 0 {
		return []string{}, nil
	}

	if spec.Tags.MaxTags > 0 && len(versions) > spec.Tags.MaxTags {
		versions = versions[len(versions)-spec.Tags.MaxTags:]
	}

	result := make([]string, len(versions))
	for i, v := range versions {
		result[i] = v.Original()
	}
	return result, nil
}

// tagsToSortedVersions parses tags as semver and returns them sorted ascending.
// Tags that cannot be parsed as semver are silently skipped.
func tagsToSortedVersions(tags []string) []*semver.Version {
	var versions []*semver.Version
	for _, t := range tags {
		if v, err := semver.NewVersion(t); err == nil {
			versions = append(versions, v)
		}
	}
	sort.Sort(semver.Collection(versions))
	return versions
}

// ExcludeTags returns a new slice with excluded entries removed.
func ExcludeTags(tags, exclusions []string) []string {
	if len(exclusions) == 0 {
		result := make([]string, len(tags))
		copy(result, tags)
		return result
	}
	exclusionSet := make(map[string]struct{})
	for _, ex := range exclusions {
		exclusionSet[ex] = struct{}{}
	}
	result := []string{}
	for _, tag := range tags {
		if _, found := exclusionSet[tag]; !found {
			result = append(result, tag)
		}
	}
	return result
}
