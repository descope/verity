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
// Example: "ghcr.io/verity-org/library/nginx:1.25.3-patched" ->
//
//	registry="ghcr.io", repository="verity-org/library/nginx", tag="1.25.3-patched"
func ParseImageRef(ref string) (registry, repository, tag string) {
	// Split off tag
	parts := strings.Split(ref, ":")
	if len(parts) > 1 {
		tag = parts[len(parts)-1]
		ref = strings.Join(parts[:len(parts)-1], ":")
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
