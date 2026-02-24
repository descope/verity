package internal

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

// CopaOutputResult represents a single patch result from Copa's --output-json.
type CopaOutputResult struct {
	Name         string `json:"name"`
	Status       string `json:"status"` // "Patched", "Skipped", "Failed"
	SourceImage  string `json:"source_image"`
	PatchedImage string `json:"patched_image"`
	Details      string `json:"details"`
}

// CopaOutput represents Copa's --output-json structure.
type CopaOutput struct {
	Results []CopaOutputResult `json:"results"`
}

// ChartImageMap maps charts to their constituent images.
type ChartImageMap struct {
	Charts []ChartImageEntry `json:"charts" yaml:"charts"`
}

// ChartImageEntry represents a chart and its images.
type ChartImageEntry struct {
	Name       string   `json:"name" yaml:"name"`
	Version    string   `json:"version" yaml:"version"`
	Repository string   `json:"repository" yaml:"repository"`
	Images     []string `json:"images" yaml:"images"`
}

// ParseCopaOutput reads Copa's --output-json file.
func ParseCopaOutput(path string) (*CopaOutput, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading copa output: %w", err)
	}

	var output CopaOutput
	if err := json.Unmarshal(data, &output); err != nil {
		return nil, fmt.Errorf("parsing copa output: %w", err)
	}

	return &output, nil
}

// ParseChartImageMap reads the chart-image-map.yaml file.
func ParseChartImageMap(path string) (*ChartImageMap, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading chart-image-map: %w", err)
	}

	var mapping ChartImageMap
	if err := yaml.Unmarshal(data, &mapping); err != nil {
		return nil, fmt.Errorf("parsing chart-image-map: %w", err)
	}

	return &mapping, nil
}

// ParseImageRef parses a full image reference into registry, repository, and tag.
// Handles both tag-based and digest-based references:
// - "ghcr.io/verity-org/nginx:1.25.3" -> registry="ghcr.io", repository="verity-org/nginx", tag="1.25.3"
// - "nginx@sha256:abc123" -> registry="", repository="nginx", tag="sha256:abc123"
// - "nginx:1.25@sha256:abc" -> registry="", repository="nginx", tag="sha256:abc" (digest takes precedence)
//
// Note: For digest references, the entire digest (e.g., "sha256:abc123") is returned as the tag.
func ParseImageRef(ref string) (registry, repository, tag string) {
	// Check for digest first (@ separator) - digests take precedence over tags
	if idx := strings.Index(ref, "@"); idx != -1 {
		tag = ref[idx+1:] // Everything after @ is the digest (e.g., "sha256:abc123")
		ref = ref[:idx]   // Remove digest from ref
	} else {
		// No digest, check for tag (: separator)
		// Need to be careful: registry might have port (e.g., localhost:5000)
		// Strategy: find the last ":" after the last "/"
		lastSlash := strings.LastIndex(ref, "/")
		if lastColon := strings.LastIndex(ref, ":"); lastColon > lastSlash {
			tag = ref[lastColon+1:]
			ref = ref[:lastColon]
		}
	}

	// Split off registry (first component before /)
	slashParts := strings.SplitN(ref, "/", 2)
	if len(slashParts) > 1 {
		// Check if first part looks like a registry (contains . or :, or is localhost)
		if strings.Contains(slashParts[0], ".") ||
			strings.Contains(slashParts[0], ":") ||
			slashParts[0] == "localhost" {
			registry = slashParts[0]
			repository = slashParts[1]
		} else {
			// No registry, just repository (e.g., "library/nginx")
			repository = ref
		}
	} else {
		repository = ref
	}

	return registry, repository, tag
}

// NormalizeImageRef converts an image reference to a canonical form for comparison.
// Adds docker.io registry if missing, normalizes library/ prefix.
func NormalizeImageRef(ref string) string {
	registry, repository, tag := ParseImageRef(ref)

	// Default to docker.io if no registry
	if registry == "" {
		registry = "docker.io"
	}

	// Add library/ prefix for official Docker images
	if registry == "docker.io" && !strings.Contains(repository, "/") {
		repository = "library/" + repository
	}

	result := registry + "/" + repository
	if tag != "" {
		result += ":" + tag
	}

	return result
}
