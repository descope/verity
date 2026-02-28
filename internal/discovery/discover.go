package discovery

import (
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/verity-org/verity/internal/config"
)

// DefaultPlatforms is the default comma-separated list of platforms to patch.
const DefaultPlatforms = "linux/amd64,linux/arm64"

// DiscoveredImage represents one image+tag combination to be patched.
type DiscoveredImage struct {
	Name           string `json:"name"`
	Source         string `json:"source"`
	TargetRegistry string `json:"target-registry"`
	Platforms      string `json:"platforms"`
}

// Discover enumerates all image+tag combos from the config.
// If targetRegistry is non-empty it overrides the config-level registry.
// overrides substitutes tag variants for chart-sourced images (from verity.yaml).
func Discover(cfg *config.CopaConfig, targetRegistry string, overrides map[string]config.Override) ([]DiscoveredImage, error) {
	registry := targetRegistry
	if registry == "" {
		registry = cfg.Target.Registry
	}

	var results []DiscoveredImage
	seen := make(map[string]struct{})

	for i := range cfg.Images {
		imgs, err := discoverStandaloneImage(&cfg.Images[i], registry)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to discover tags for %q: %v\n", cfg.Images[i].Name, err)
			continue
		}
		for _, img := range imgs {
			key := img.Name + "|" + img.Source
			if _, exists := seen[key]; !exists {
				seen[key] = struct{}{}
				results = append(results, img)
			}
		}
	}

	for _, chartSpec := range cfg.Charts {
		imgs, err := discoverChartImages(chartSpec, overrides, registry)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to discover images from chart %q: %v\n", chartSpec.Name, err)
			continue
		}
		for _, img := range imgs {
			key := img.Name + "|" + img.Source
			if _, exists := seen[key]; !exists {
				seen[key] = struct{}{}
				results = append(results, img)
			}
		}
	}

	return results, nil
}

// LoadConfig reads and parses a copa-config.yaml file.
func LoadConfig(path string) (*config.CopaConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading config: %w", err)
	}
	var cfg config.CopaConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parsing config: %w", err)
	}
	return &cfg, nil
}

// LoadVerityConfig reads verity-specific configuration from verity.yaml.
// Returns an empty config (not an error) if the file does not exist.
func LoadVerityConfig(path string) (*config.VerityConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &config.VerityConfig{}, nil
		}
		return nil, fmt.Errorf("reading verity config: %w", err)
	}
	var vc config.VerityConfig
	if err := yaml.Unmarshal(data, &vc); err != nil {
		return nil, fmt.Errorf("parsing verity config: %w", err)
	}
	return &vc, nil
}

// LoadChartsFile reads chart dependencies from a Helm Chart.yaml file.
// Only the `dependencies:` field is read; all other Chart.yaml fields are ignored.
// Returns a nil slice (not an error) if the file does not exist, so callers
// can pass a default path unconditionally.
func LoadChartsFile(path string) ([]config.ChartSpec, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("reading charts file: %w", err)
	}
	var chartFile config.HelmChartFile
	if err := yaml.Unmarshal(data, &chartFile); err != nil {
		return nil, fmt.Errorf("parsing charts file: %w", err)
	}
	return chartFile.Dependencies, nil
}

func discoverStandaloneImage(spec *config.ImageSpec, registry string) ([]DiscoveredImage, error) {
	tags, err := FindTagsToPatch(spec)
	if err != nil {
		return nil, err
	}

	imgRegistry := registry
	if spec.Target.Registry != "" {
		imgRegistry = spec.Target.Registry
	}

	result := make([]DiscoveredImage, 0, len(tags))
	for _, tag := range tags {
		result = append(result, DiscoveredImage{
			Name:           spec.Name,
			Source:         spec.Image + ":" + tag,
			TargetRegistry: imgRegistry,
			Platforms:      joinPlatforms(spec.Platforms),
		})
	}
	return result, nil
}

func discoverChartImages(chart config.ChartSpec, overrides map[string]config.Override, registry string) ([]DiscoveredImage, error) {
	images, err := ExtractChartImages(chart, overrides)
	if err != nil {
		return nil, err
	}

	result := make([]DiscoveredImage, 0, len(images))
	for _, imageRef := range images {
		result = append(result, DiscoveredImage{
			Name:           nameFromRef(imageRef),
			Source:         imageRef,
			TargetRegistry: registry,
			Platforms:      DefaultPlatforms,
		})
	}
	return result, nil
}

// nameFromRef derives a safe, unique image name from a full image reference.
// For images with a registry and org (3+ path components), joins the org and
// name with "-" to prevent collisions between images with the same basename
// from different registries/orgs. When org and name are identical (e.g.,
// prometheus/prometheus), the duplicate is dropped. Single-component and
// two-component refs return the last component directly.
// e.g.:
//
//	"quay.io/prometheus/prometheus:v3.2.1" → "prometheus"
//	"quay.io/some-org/nginx:1.29"          → "some-org-nginx"
//	"ghcr.io/kiwigrid/k8s-sidecar:1.28.0" → "kiwigrid-k8s-sidecar"
//	"nginx:1.25"                           → "nginx"
func nameFromRef(ref string) string {
	// Strip digest
	if idx := strings.Index(ref, "@"); idx != -1 {
		ref = ref[:idx]
	}
	// Strip tag: last ":" must come after the last "/"
	lastSlash := strings.LastIndex(ref, "/")
	if lastColon := strings.LastIndex(ref, ":"); lastColon > lastSlash {
		ref = ref[:lastColon]
	}
	// Split into path components (hostname/org/name)
	parts := strings.Split(ref, "/")
	// 3+ parts: hostname/org/name — include org to prevent collisions
	if len(parts) >= 3 {
		org := parts[len(parts)-2]
		name := parts[len(parts)-1]
		if org == name {
			return name
		}
		return org + "-" + name
	}
	// 1-2 parts: no org or no hostname — use last component
	return parts[len(parts)-1]
}

// joinPlatforms returns a comma-joined platform string, defaulting to DefaultPlatforms.
func joinPlatforms(platforms []string) string {
	if len(platforms) == 0 {
		return DefaultPlatforms
	}
	return strings.Join(platforms, ",")
}
