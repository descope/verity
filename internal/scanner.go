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
	var values map[string]any
	if err := yaml.Unmarshal(data, &values); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", path, err)
	}
	if len(values) == 0 {
		return nil, nil
	}
	images := dedup(findImages(values, "", "", nil))
	// Sort for deterministic output (Go map iteration is randomized)
	sort.Slice(images, func(i, j int) bool {
		return images[i].Reference() < images[j].Reference()
	})
	return images, nil
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

func findImages(values map[string]any, prefix, appVersion string, cache map[string]string) []Image {
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

// ResolveImageTag determines the correct image tag when falling back to appVersion.
// It checks the registry to see if appVersion or "v"+appVersion exists as a tag,
// since chart templates vary in whether they prepend "v" to Chart.AppVersion.
// ResolveImageTag attempts to find the correct tag for an image by trying
// multiple variations. It tries the tag as-is first, then with a "v" prefix
// if the tag doesn't already have one. Returns an Image with the resolved tag,
// or the original image if no variation exists in the registry.
func ResolveImageTag(ctx context.Context, img Image) Image {
	// If no tag specified, return as-is (will default to "latest" elsewhere)
	if img.Tag == "" {
		return img
	}

	// If tag already starts with "v", try as-is first, then without "v"
	if strings.HasPrefix(img.Tag, "v") {
		// Try with "v" prefix first
		if tagChecker(ctx, img.Reference()) {
			return img
		}
		// Try without "v" prefix
		candidate := img
		candidate.Tag = strings.TrimPrefix(img.Tag, "v")
		if tagChecker(ctx, candidate.Reference()) {
			return candidate
		}
		// Fall back to original
		return img
	}

	// Tag doesn't start with "v", try without prefix first
	if tagChecker(ctx, img.Reference()) {
		return img
	}

	// Try with "v" prefix
	candidate := img
	candidate.Tag = "v" + img.Tag
	if tagChecker(ctx, candidate.Reference()) {
		return candidate
	}

	// Fall back to original tag if neither exists
	return img
}

func resolveTag(img Image, appVersion string) string {
	if strings.HasPrefix(appVersion, "v") {
		return appVersion
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Use the new ResolveImageTag function
	candidate := img
	candidate.Tag = appVersion
	resolved := ResolveImageTag(ctx, candidate)
	return resolved.Tag
}

func walk(node any, path, parentKey string, fn func(string, Image)) {
	switch v := node.(type) {
	case map[string]any:
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

	case []any:
		for i, item := range v {
			walk(item, fmt.Sprintf("%s[%d]", path, i), "", fn)
		}
	}
}

// extractImage checks whether a map looks like {repository: ..., tag: ...}.
func extractImage(m map[string]any, parentKey string) (Image, bool) {
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

func stringVal(m map[string]any, key string) (string, bool) {
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
	seen := make(map[string]*Image)
	var result []Image

	for _, img := range images {
		// Normalize the key by handling tag variations (v1.2.3 vs 1.2.3)
		normalizedKey := normalizeReference(img)

		if existing, found := seen[normalizedKey]; found {
			// If we already have this image, prefer the one with more specific tag
			// (prefer "v1.2.3" over "1.2.3" as it's more explicit)
			if shouldPrefer(img, *existing) {
				// Update the seen map and replace in result
				seen[normalizedKey] = &img
				// Find and replace the existing entry
				for i, r := range result {
					if normalizeReference(r) == normalizedKey {
						result[i] = img
						break
					}
				}
			}
			// Skip this image as we already have a version of it
			continue
		}

		seen[normalizedKey] = &img
		result = append(result, img)
	}
	return result
}

// normalizeReference returns a normalized reference string for deduplication.
// Strips "v" prefix from tags to treat "v1.2.3" and "1.2.3" as the same image.
func normalizeReference(img Image) string {
	ref := img.Repository
	if img.Registry != "" {
		ref = img.Registry + "/" + ref
	}
	if img.Tag != "" {
		// Normalize tag by removing "v" prefix for comparison
		tag := strings.TrimPrefix(img.Tag, "v")
		ref = ref + ":" + tag
	}
	return ref
}

// shouldPrefer returns true if img1 should be preferred over img2.
// Prefers tags with "v" prefix as they're more explicit.
func shouldPrefer(img1, img2 Image) bool {
	// If one has "v" prefix and the other doesn't, prefer the one with "v"
	hasV1 := strings.HasPrefix(img1.Tag, "v")
	hasV2 := strings.HasPrefix(img2.Tag, "v")

	if hasV1 && !hasV2 {
		return true
	}
	if !hasV1 && hasV2 {
		return false
	}

	// Otherwise prefer the first one we saw
	return false
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
	var raw map[string]any
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", path, err)
	}

	ovr, ok := raw["overrides"]
	if !ok {
		return nil, nil
	}
	ovrMap, ok := ovr.(map[string]any)
	if !ok {
		return nil, nil
	}

	var overrides []ImageOverride
	for repo, v := range ovrMap {
		m, ok := v.(map[string]any)
		if !ok {
			continue
		}
		from, ok := m["from"].(string)
		if !ok || from == "" {
			continue
		}
		to, _ := m["to"].(string) //nolint:errcheck // optional field
		overrides = append(overrides, ImageOverride{
			Repository: repo,
			From:       from,
			To:         to,
		})
	}
	return overrides, nil
}

// MergeChartImages appends chart-discovered images to a values YAML file,
// skipping any that already exist (matched by image reference). This keeps
// values.yaml as a single flat list — no separate sections.
//
// NOTE: This is intentionally append-only. If a chart drops an image in a
// future version, the old entry stays in values.yaml. This is acceptable
// because the patch matrix deduplicates by reference, so stale entries only
// cause a harmless extra patch job. Removing entries would risk deleting
// images that were added manually or used by other charts.
func MergeChartImages(valuesPath string, images []Image) error { //nolint:gocognit,gocyclo,cyclop,funlen // complex workflow
	if len(images) == 0 {
		return nil
	}

	existing, err := os.ReadFile(valuesPath)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("reading %s: %w", valuesPath, err)
	}

	// Parse existing images to know which references are already present.
	existingRefs := make(map[string]bool)
	if len(existing) > 0 {
		var values map[string]any
		if err := yaml.Unmarshal(existing, &values); err != nil {
			return fmt.Errorf("parsing %s: %w", valuesPath, err)
		}
		for _, img := range findImages(values, "", "", nil) {
			existingRefs[img.Reference()] = true
		}
	}

	// Collect only genuinely new images.
	var newImages []Image
	for _, img := range images {
		if !existingRefs[img.Reference()] {
			newImages = append(newImages, img)
		}
	}
	if len(newImages) == 0 {
		return nil
	}

	// Sort for deterministic output.
	sort.Slice(newImages, func(i, j int) bool {
		return newImages[i].Reference() < newImages[j].Reference()
	})

	// Discover existing top-level YAML keys to avoid collisions.
	content := string(existing)
	if content != "" && !strings.HasSuffix(content, "\n") {
		content += "\n"
	}
	usedKeys := make(map[string]bool)
	var topLevel map[string]any
	if err := yaml.Unmarshal(existing, &topLevel); err == nil && topLevel != nil {
		for k := range topLevel {
			usedKeys[k] = true
		}
	}

	// Append new entries at the same level as existing ones.
	var sb strings.Builder
	for _, img := range newImages {
		baseKey := imageEntryKey(img)
		key := baseKey
		if usedKeys[key] {
			// Disambiguate by incorporating the registry.
			disambiguator := strings.ReplaceAll(img.Registry, ".", "-")
			if disambiguator == "" {
				disambiguator = "image"
			}
			candidate := fmt.Sprintf("%s-%s", baseKey, disambiguator)
			i := 1
			for usedKeys[candidate] {
				candidate = fmt.Sprintf("%s-%s-%d", baseKey, disambiguator, i)
				i++
			}
			key = candidate
		}
		usedKeys[key] = true

		sb.WriteString(key + ":\n")
		sb.WriteString("  image:\n")
		if img.Registry != "" {
			sb.WriteString(fmt.Sprintf("    registry: %s\n", img.Registry))
		}
		sb.WriteString(fmt.Sprintf("    repository: %s\n", img.Repository))
		if img.Tag != "" {
			sb.WriteString(fmt.Sprintf("    tag: %q\n", img.Tag))
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
