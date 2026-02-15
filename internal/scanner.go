package internal

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
	"helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/chart/loader"
)

// Image represents a container image found in chart values.
type Image struct {
	Registry   string `yaml:"registry,omitempty"`
	Repository string `yaml:"repository"`
	Tag        string `yaml:"tag,omitempty"`
	Path       string `yaml:"path"`
}

// Reference returns the full image reference string.
func (img Image) Reference() string {
	ref := img.Repository
	if img.Registry != "" {
		ref = img.Registry + "/" + ref
	}
	if img.Tag != "" {
		ref = ref + ":" + img.Tag
	}
	return ref
}

// ParseImagesFile reads a YAML file of helm-values style image definitions
// and returns all container image references found.
func ParseImagesFile(path string) ([]Image, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", path, err)
	}
	var values map[string]interface{}
	if err := yaml.Unmarshal(data, &values); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", path, err)
	}
	if len(values) == 0 {
		return nil, nil
	}
	return dedup(findImages(values, "", "", nil)), nil
}

// ScanForImages loads a chart directory and finds all container image references.
func ScanForImages(chartPath string) ([]Image, error) {
	ch, err := loader.LoadDir(chartPath)
	if err != nil {
		return nil, fmt.Errorf("loading chart %s: %w", chartPath, err)
	}
	cache := map[string]string{} // shared across all subcharts
	return dedup(scanChart(ch, "", cache)), nil
}

func scanChart(ch *chart.Chart, prefix string, cache map[string]string) []Image {
	var images []Image

	if ch.Values != nil {
		images = append(images, findImages(ch.Values, prefix, ch.Metadata.AppVersion, cache)...)
	}

	for _, dep := range ch.Dependencies() {
		images = append(images, scanChart(dep, joinPath(prefix, dep.Name()), cache)...)
	}

	return images
}

// tagChecker is the function used to probe whether an image tag exists.
// Replaceable in tests for deterministic behavior.
var tagChecker func(ctx context.Context, ref string) bool = imageExists

func findImages(values map[string]interface{}, prefix, appVersion string, cache map[string]string) []Image {
	if cache == nil {
		cache = map[string]string{}
	}
	var images []Image
	walk(values, prefix, "", func(path string, img Image) {
		img.Path = path
		// If tag is empty and appVersion is available, resolve the correct tag.
		// Chart templates vary: some use appVersion as-is, others prepend "v".
		// We check which variant actually exists in the registry.
		if img.Tag == "" && appVersion != "" {
			key := img.Registry + "/" + img.Repository + "@" + appVersion
			if cached, ok := cache[key]; ok {
				img.Tag = cached
			} else {
				img.Tag = resolveTag(img, appVersion)
				cache[key] = img.Tag
			}
		}
		images = append(images, img)
	})
	return images
}

// resolveTag determines the correct image tag when falling back to appVersion.
// It checks the registry to see if appVersion or "v"+appVersion exists as a tag,
// since chart templates vary in whether they prepend "v" to Chart.AppVersion.
func resolveTag(img Image, appVersion string) string {
	if strings.HasPrefix(appVersion, "v") {
		return appVersion
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Try appVersion as-is first
	candidate := img
	candidate.Tag = appVersion
	if tagChecker(ctx, candidate.Reference()) {
		return appVersion
	}

	// Try with "v" prefix
	candidate.Tag = "v" + appVersion
	if tagChecker(ctx, candidate.Reference()) {
		return "v" + appVersion
	}

	// Default to as-is if neither resolves (will fail at pull time with a clear error)
	return appVersion
}

func walk(node interface{}, path, parentKey string, fn func(string, Image)) {
	switch v := node.(type) {
	case map[string]interface{}:
		// Check if this map itself is an image definition.
		if img, ok := extractImage(v, parentKey); ok {
			fn(path, img)
			return
		}

		// Check for "image" as a direct string value (e.g. image: "nginx:1.25").
		if s, ok := stringVal(v, "image"); ok && looksLikeRef(s) {
			fn(joinPath(path, "image"), parseRef(s))
		}

		// Recurse into children.
		for k, val := range v {
			walk(val, joinPath(path, k), k, fn)
		}

	case []interface{}:
		for i, item := range v {
			walk(item, fmt.Sprintf("%s[%d]", path, i), "", fn)
		}
	}
}

// extractImage checks whether a map looks like {repository: ..., tag: ...}.
func extractImage(m map[string]interface{}, parentKey string) (Image, bool) {
	repo, ok := stringVal(m, "repository")
	if !ok || !looksLikeImage(repo) {
		return Image{}, false
	}

	_, hasTag := stringVal(m, "tag")
	_, hasDigest := stringVal(m, "digest")
	isImageKey := strings.EqualFold(parentKey, "image")

	if !isImageKey && !hasTag && !hasDigest {
		return Image{}, false
	}

	img := Image{Repository: repo}
	if reg, ok := stringVal(m, "registry"); ok {
		img.Registry = reg
	}
	if tag, ok := stringVal(m, "tag"); ok {
		img.Tag = tag
	}
	return img, true
}

func stringVal(m map[string]interface{}, key string) (string, bool) {
	v, ok := m[key]
	if !ok {
		return "", false
	}
	switch s := v.(type) {
	case string:
		return s, s != ""
	case int, int64, float64:
		return fmt.Sprintf("%v", s), true
	default:
		return "", false
	}
}

func looksLikeImage(repo string) bool {
	if repo == "" || repo == "true" || repo == "false" {
		return false
	}
	if strings.HasPrefix(repo, "http://") || strings.HasPrefix(repo, "https://") {
		return false
	}
	return !strings.Contains(repo, " ")
}

func looksLikeRef(s string) bool {
	return strings.Contains(s, "/") &&
		!strings.Contains(s, " ") &&
		!strings.HasPrefix(s, "http://") &&
		!strings.HasPrefix(s, "https://")
}

func parseRef(ref string) Image {
	img := Image{}
	if idx := strings.LastIndex(ref, ":"); idx > 0 && !strings.Contains(ref[idx:], "/") {
		img.Tag = ref[idx+1:]
		ref = ref[:idx]
	}
	parts := strings.SplitN(ref, "/", 2)
	if len(parts) == 2 && (strings.Contains(parts[0], ".") || strings.Contains(parts[0], ":")) {
		img.Registry = parts[0]
		img.Repository = parts[1]
	} else {
		img.Repository = ref
	}
	return img
}

func joinPath(base, key string) string {
	if base == "" {
		return key
	}
	return base + "." + key
}

func dedup(images []Image) []Image {
	seen := make(map[string]bool)
	var result []Image
	for _, img := range images {
		key := img.Reference()
		if !seen[key] {
			seen[key] = true
			result = append(result, img)
		}
	}
	return result
}

// ImageOverride specifies a tag replacement for images matching a repository.
// When an image's repository matches, the From substring in the tag is replaced with To.
type ImageOverride struct {
	Repository string // image repository to match (e.g. "timberio/vector")
	From       string // substring to replace in the tag
	To         string // replacement string
}

// ParseOverrides reads the "overrides" section from a YAML values file.
// The expected format:
//
//	overrides:
//	  timberio/vector:
//	    from: "distroless-libc"
//	    to: "debian"
func ParseOverrides(path string) ([]ImageOverride, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", path, err)
	}
	var raw map[string]interface{}
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", path, err)
	}

	ovr, ok := raw["overrides"]
	if !ok {
		return nil, nil
	}
	ovrMap, ok := ovr.(map[string]interface{})
	if !ok {
		return nil, nil
	}

	var overrides []ImageOverride
	for repo, v := range ovrMap {
		m, ok := v.(map[string]interface{})
		if !ok {
			continue
		}
		from, _ := m["from"].(string)
		to, _ := m["to"].(string)
		if from == "" {
			continue
		}
		overrides = append(overrides, ImageOverride{
			Repository: repo,
			From:       from,
			To:         to,
		})
	}
	return overrides, nil
}

// chartImageMarker is the comment line that marks the beginning of the
// auto-generated chart-images section in values.yaml.
const chartImageMarker = "# Images discovered from charts (auto-generated by verity discover, do not edit)\n"

// MergeChartImages writes discovered chart images into the chart-images section
// of a values YAML file. The chart-images section is fully replaced on each run.
// Manually maintained images and overrides outside this section are preserved.
func MergeChartImages(valuesPath string, images []Image) error {
	existing, err := os.ReadFile(valuesPath)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("reading %s: %w", valuesPath, err)
	}

	// Strip existing chart-images section (from marker to EOF).
	content := string(existing)
	if idx := strings.Index(content, chartImageMarker); idx >= 0 {
		content = strings.TrimRight(content[:idx], "\n") + "\n"
	}

	if len(images) == 0 {
		return os.WriteFile(valuesPath, []byte(content), 0o644)
	}

	// Sort images for deterministic output.
	sorted := make([]Image, len(images))
	copy(sorted, images)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].Reference() < sorted[j].Reference()
	})

	// Build the chart-images section.
	var sb strings.Builder
	sb.WriteString("\n")
	sb.WriteString(chartImageMarker)
	sb.WriteString("chart-images:\n")

	seen := make(map[string]bool)
	for _, img := range sorted {
		key := imageEntryKey(img)
		if seen[key] {
			// Append registry to disambiguate duplicate repository paths.
			disambiguated := key + "-" + strings.ReplaceAll(img.Registry, ".", "-")
			if seen[disambiguated] {
				continue
			}
			key = disambiguated
		}
		seen[key] = true

		sb.WriteString(fmt.Sprintf("  %s:\n", key))
		sb.WriteString("    image:\n")
		if img.Registry != "" {
			sb.WriteString(fmt.Sprintf("      registry: %s\n", img.Registry))
		}
		sb.WriteString(fmt.Sprintf("      repository: %s\n", img.Repository))
		if img.Tag != "" {
			sb.WriteString(fmt.Sprintf("      tag: \"%s\"\n", img.Tag))
		}
	}

	content += sb.String()
	return os.WriteFile(valuesPath, []byte(content), 0o644)
}

// imageEntryKey generates a YAML key for a chart-discovered image
// from its repository path (e.g. "prometheus/prometheus" → "prometheus-prometheus").
func imageEntryKey(img Image) string {
	return strings.ReplaceAll(img.Repository, "/", "-")
}

// ApplyOverrides applies tag replacements to images matching override rules.
// Returns the modified image list.
func ApplyOverrides(images []Image, overrides []ImageOverride) []Image {
	if len(overrides) == 0 {
		return images
	}

	result := make([]Image, len(images))
	for i, img := range images {
		result[i] = img
		for _, o := range overrides {
			// Match by repository, with or without registry prefix
			if img.Repository == o.Repository ||
				(img.Registry != "" && img.Registry+"/"+img.Repository == o.Repository) {
				if img.Tag != "" && strings.Contains(img.Tag, o.From) {
					result[i].Tag = strings.Replace(img.Tag, o.From, o.To, 1)
				}
			}
		}
	}
	return result
}
