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
	GoVcsURL       string `json:"go-vcs-url,omitempty"`
}

// Discover enumerates all image+tag combos from the config.
// If targetRegistry is non-empty it overrides the config-level registry.
// overrides substitutes tag variants for chart-sourced images (from verity.yaml).
// excludeNames, when non-nil, causes chart-discovered images whose derived name
// matches a key in the set to be skipped (used to avoid conflicts with Integer images).
func Discover(cfg *config.CopaConfig, targetRegistry string, overrides map[string]config.Override, excludeNames map[string]struct{}) ([]DiscoveredImage, error) {
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
			if isExcluded(&img, excludeNames) {
				fmt.Fprintf(os.Stderr, "Skipping chart image %q: name %q excluded via --exclude-names\n", img.Source, img.Name)
				continue
			}
			key := img.Name + "|" + img.Source
			if _, exists := seen[key]; !exists {
				seen[key] = struct{}{}
				results = append(results, img)
			}
		}
	}

	return results, nil
}

// isExcluded checks whether a chart-discovered image should be skipped.
//
// Three name shapes are tried, in order:
//  1. exact match — img.Name as-is (e.g. "kyverno/background-controller")
//  2. basename — last path component (e.g. "background-controller")
//  3. slash-to-hyphen flatten — full path with "/" → "-"
//     (e.g. "kyverno-background-controller")
//
// The third shape exists because verity's Wolfi/Integer rebuild stubs are
// filenames like images/kyverno-background-controller.yaml, which the
// orchestrator passes to --exclude-names with hyphens. Without this
// normalization, chart-discovered names like "kyverno/background-controller"
// silently bypass the exclusion and re-enter the patch pipeline even when a
// Wolfi rebuild already exists for that image.
func isExcluded(img *DiscoveredImage, excludeNames map[string]struct{}) bool {
	if len(excludeNames) == 0 {
		return false
	}
	if _, ok := excludeNames[img.Name]; ok {
		return true
	}
	baseName := nameBasename(img.Name)
	if baseName != img.Name {
		if _, ok := excludeNames[baseName]; ok {
			return true
		}
	}
	if flat := strings.ReplaceAll(img.Name, "/", "-"); flat != img.Name {
		if _, ok := excludeNames[flat]; ok {
			return true
		}
	}
	return false
}

// nameBasename returns the last path component of an image ref with tag/digest stripped.
// e.g. "docker.io/library/rabbitmq:4.2.3" → "rabbitmq".
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
		img := DiscoveredImage{
			Name:           spec.Name,
			Source:         spec.Image + ":" + tag,
			TargetRegistry: imgRegistry,
			Platforms:      joinPlatforms(spec.Platforms),
		}
		if spec.GoVcsURL != "" {
			img.GoVcsURL = spec.GoVcsURL + "@" + spec.GoVcsTagPrefix + tag
		}
		result = append(result, img)
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
			Name:           repoPath(imageRef),
			Source:         imageRef,
			TargetRegistry: registry,
			Platforms:      DefaultPlatforms,
		})
	}
	return result, nil
}

// repoPath extracts the repository path from a full image reference by
// stripping the registry host, tag, and digest. The result preserves the
// upstream layout so patched images mirror their source structure under
// the target registry.
// e.g.:
//
//	"ghcr.io/kiwigrid/k8s-sidecar:1.28.0"                          → "kiwigrid/k8s-sidecar"
//	"mirror.gcr.io/valkey/valkey:9.0.3"                               → "valkey/valkey"
//	"quay.io/prometheus/prometheus:v3.2.1"                           → "prometheus/prometheus"
//	"ghcr.io/kubernetes/kube-state-metrics/kube-state-metrics:v2.10" → "kubernetes/kube-state-metrics/kube-state-metrics"
//	"nginx:1.25"                                                     → "nginx"
func repoPath(ref string) string {
	// Strip digest
	if idx := strings.Index(ref, "@"); idx != -1 {
		ref = ref[:idx]
	}
	// Strip tag
	lastSlash := strings.LastIndex(ref, "/")
	if lastColon := strings.LastIndex(ref, ":"); lastColon > lastSlash {
		ref = ref[:lastColon]
	}
	// Strip registry host: first component is a host if it contains "." or ":" or is "localhost"
	parts := strings.Split(ref, "/")
	if len(parts) >= 2 {
		first := parts[0]
		if strings.ContainsAny(first, ".:") || first == "localhost" {
			return strings.Join(parts[1:], "/")
		}
	}
	return ref
}

// joinPlatforms returns a comma-joined platform string, defaulting to DefaultPlatforms.
func joinPlatforms(platforms []string) string {
	if len(platforms) == 0 {
		return DefaultPlatforms
	}
	return strings.Join(platforms, ",")
}
