package chartgen

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/verity-org/verity/internal/imageref"
)

// ImageMapping represents the mapping from an original image to its patched replacement.
type ImageMapping struct {
	OriginalRepo string `json:"originalRepo"`
	OriginalTag  string `json:"originalTag"`
	Source       string `json:"source,omitempty"`
	PatchedRepo  string `json:"patchedRepo"`
	PatchedTag   string `json:"patchedTag"`
}

// BuildImageMappings queries the target registry for patched versions of each image
// and returns mappings for images that have been successfully patched.
func BuildImageMappings(imageRefs []string, targetRegistry string, excludeNames map[string]struct{}) ([]ImageMapping, error) {
	ctx := context.Background()
	mappings := make([]ImageMapping, 0, len(imageRefs))

	for _, imageRef := range imageRefs {
		sourceRepo, sourceTag := splitRef(imageRef)
		name := repoPath(imageRef)

		if isExcluded(name, imageRef, excludeNames) {
			fmt.Fprintf(os.Stderr, "warning: skipping excluded image %q (%s)\n", name, imageRef)
			continue
		}

		patchedRepo := targetRegistry + "/" + name
		lsOutput, err := runCommand(ctx, 30*time.Second, "crane", "ls", patchedRepo)
		if err != nil {
			// crane ls fails if repo doesn't exist — treat as no patched images.
			fmt.Fprintf(os.Stderr, "warning: cannot list tags for %s: %v\n", patchedRepo, err)
			continue
		}

		tags := strings.Split(strings.TrimSpace(lsOutput), "\n")
		found := false
		for _, tag := range tags {
			if strings.TrimSpace(tag) == sourceTag {
				found = true
				break
			}
		}
		if !found {
			fmt.Fprintf(os.Stderr, "warning: tag %s not found in %s\n", sourceTag, patchedRepo)
			continue
		}

		mappings = append(mappings, ImageMapping{
			OriginalRepo: sourceRepo,
			OriginalTag:  sourceTag,
			PatchedRepo:  patchedRepo,
			PatchedTag:   sourceTag,
		})
	}

	return mappings, nil
}

// isExcluded checks whether a chart-discovered image should be skipped.
func isExcluded(name, imageRef string, excludeNames map[string]struct{}) bool {
	if len(excludeNames) == 0 {
		return false
	}
	if _, ok := excludeNames[name]; ok {
		return true
	}
	baseName := nameBasename(name)
	if baseName != name {
		if _, ok := excludeNames[baseName]; ok {
			return true
		}
	}
	return false
}

// repoPath returns the registry-stripped, tag/digest-stripped portion of
// an image ref. Thin wrapper over imageref.RepoPath so chartgen, doctor,
// and any future consumer share one implementation — divergence here would
// silently mask the very class of failures doctor exists to surface.
func repoPath(ref string) string {
	return imageref.RepoPath(ref)
}

func nameBasename(ref string) string {
	if idx := strings.Index(ref, "@"); idx != -1 {
		ref = ref[:idx]
	}
	lastSlash := strings.LastIndex(ref, "/")
	if lastColon := strings.LastIndex(ref, ":"); lastColon > lastSlash {
		ref = ref[:lastColon]
	}
	if lastSlash >= 0 {
		return ref[lastSlash+1:]
	}
	return ref
}

// splitRef splits an image reference into its name and tag components.
// Thin wrapper over imageref.SplitRef.
func splitRef(ref string) (name, tag string) {
	return imageref.SplitRef(ref)
}

// runCommand executes a CLI command with a timeout and returns stdout.
func runCommand(ctx context.Context, timeout time.Duration, name string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, name, args...)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("%s %s: %w\nstderr: %s", name, strings.Join(args, " "), err, stderr.String())
	}

	return stdout.String(), nil
}

// ErrNilChart is returned when a nil chart is passed to PackageChart.
var ErrNilChart = errors.New("package chart: chart is nil")

// ErrEmptyChartName is returned when an empty chart name is provided.
var ErrEmptyChartName = errors.New("build wrapper chart: original chart name is required")

// ErrNoArchivePath is returned when helm package output doesn't contain the archive path.
var ErrNoArchivePath = errors.New("helm package output did not contain chart archive path")
