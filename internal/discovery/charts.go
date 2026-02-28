package discovery

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/verity-org/verity/internal/config"
)

// Sentinel errors for chart spec validation.
var (
	ErrInvalidChartName    = errors.New("chart name must not start with '-'")
	ErrInvalidChartVersion = errors.New("chart version must not start with '-'")
	ErrInvalidChartRepo    = errors.New("chart repository must start with oci://, https://, or http://")
)

// ExtractChartImages runs helm template for a chart and returns all unique image references found.
// Overrides are applied to substitute tag variants (e.g., distroless-libc → debian).
func ExtractChartImages(chart config.ChartSpec, overrides map[string]config.Override) ([]string, error) {
	if err := validateChartSpec(chart); err != nil {
		return nil, err
	}

	args := helmTemplateArgs(chart)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, "helm", args...)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("helm template %s: %w\nstderr: %s", chart.Name, err, stderr.String())
	}

	images, err := extractImagesFromManifests(stdout.Bytes())
	if err != nil {
		return nil, fmt.Errorf("extracting images from %s manifests: %w", chart.Name, err)
	}

	result := make([]string, 0, len(images))
	for _, img := range images {
		result = append(result, applyOverride(img, overrides))
	}
	return result, nil
}

// helmTemplateArgs builds the helm template argument list for a chart spec.
func helmTemplateArgs(chart config.ChartSpec) []string {
	if strings.HasPrefix(chart.Repository, "oci://") {
		// OCI registry: helm template <name> <oci-repo>/<name> --version <ver>
		return []string{
			"template", chart.Name,
			chart.Repository + "/" + chart.Name,
			"--version", chart.Version,
		}
	}
	// HTTP repository: helm template <name> <name> --repo <url> --version <ver>
	return []string{
		"template", chart.Name, chart.Name,
		"--repo", chart.Repository,
		"--version", chart.Version,
	}
}

// extractImagesFromManifests parses multi-document Helm YAML output and collects unique image references.
func extractImagesFromManifests(data []byte) ([]string, error) {
	seen := make(map[string]struct{})
	var result []string

	decoder := yaml.NewDecoder(bytes.NewReader(data))
	for {
		var doc map[string]any
		if err := decoder.Decode(&doc); err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return nil, fmt.Errorf("decoding YAML document: %w", err)
		}
		if doc == nil {
			continue
		}
		walkNode(doc, seen, &result)
	}

	return result, nil
}

// validateChartSpec checks that ChartSpec fields are safe to pass to helm.
// Guards against argument injection (e.g., names or versions starting with "--").
func validateChartSpec(chart config.ChartSpec) error {
	if strings.HasPrefix(chart.Name, "-") {
		return fmt.Errorf("%w: %q", ErrInvalidChartName, chart.Name)
	}
	if strings.HasPrefix(chart.Version, "-") {
		return fmt.Errorf("%w: %q", ErrInvalidChartVersion, chart.Version)
	}
	if !strings.HasPrefix(chart.Repository, "oci://") &&
		!strings.HasPrefix(chart.Repository, "https://") &&
		!strings.HasPrefix(chart.Repository, "http://") {
		return fmt.Errorf("%w: %q", ErrInvalidChartRepo, chart.Repository)
	}
	return nil
}

// walkNode recursively searches decoded YAML nodes for "image" string fields.
func walkNode(node any, seen map[string]struct{}, result *[]string) {
	switch v := node.(type) {
	case map[string]any:
		if img, ok := v["image"]; ok {
			if imgStr, ok := img.(string); ok && imgStr != "" {
				if _, exists := seen[imgStr]; !exists {
					seen[imgStr] = struct{}{}
					*result = append(*result, imgStr)
				}
			}
		}
		for _, val := range v {
			walkNode(val, seen, result)
		}
	case []any:
		for _, item := range v {
			walkNode(item, seen, result)
		}
	}
}

// applyOverride substitutes a tag variant in an image reference using the overrides map.
// The map key is a partial image path; if the image contains it and its tag contains
// the "from" suffix, the suffix is replaced with "to".
func applyOverride(image string, overrides map[string]config.Override) string {
	for key, override := range overrides {
		if strings.Contains(image, key) {
			old := "-" + override.From
			replacement := "-" + override.To
			if strings.Contains(image, old) {
				return strings.Replace(image, old, replacement, 1)
			}
		}
	}
	return image
}
