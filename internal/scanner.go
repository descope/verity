package internal

import (
	"fmt"
	"os"
	"strings"

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
	return dedup(findImages(values, "", "")), nil
}

// ScanForImages loads a chart directory and finds all container image references.
func ScanForImages(chartPath string) ([]Image, error) {
	ch, err := loader.LoadDir(chartPath)
	if err != nil {
		return nil, fmt.Errorf("loading chart %s: %w", chartPath, err)
	}
	return dedup(scanChart(ch, "")), nil
}

func scanChart(ch *chart.Chart, prefix string) []Image {
	var images []Image

	if ch.Values != nil {
		images = append(images, findImages(ch.Values, prefix, ch.Metadata.AppVersion)...)
	}

	for _, dep := range ch.Dependencies() {
		images = append(images, scanChart(dep, joinPath(prefix, dep.Name()))...)
	}

	return images
}

func findImages(values map[string]interface{}, prefix, appVersion string) []Image {
	var images []Image
	walk(values, prefix, "", func(path string, img Image) {
		img.Path = path
		// If tag is empty and appVersion is available, use "v" + appVersion
		if img.Tag == "" && appVersion != "" {
			// Check if appVersion already has "v" prefix
			if strings.HasPrefix(appVersion, "v") {
				img.Tag = appVersion
			} else {
				img.Tag = "v" + appVersion
			}
		}
		images = append(images, img)
	})
	return images
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
