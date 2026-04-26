package discovery

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"regexp"
	"sort"
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
	imageTagPattern        = regexp.MustCompile(`^[a-zA-Z0-9._-]+$`)
	imageDigestPattern     = regexp.MustCompile(`^sha256:[a-f0-9]{64}$`)
	imageNamePattern       = regexp.MustCompile(`^[A-Za-z0-9._:-]+(?:/[A-Za-z0-9._-]+)+$`)
)

// ExtractChartImages runs helm template for a chart and returns all unique image references found.
// Overrides are applied to substitute tag variants (e.g., distroless-libc → debian).
func ExtractChartImages(chart config.ChartSpec, overrides map[string]config.Override) ([]string, error) {
	if err := ValidateChartSpec(chart); err != nil {
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

// ValidateChartSpec checks that ChartSpec fields are safe to pass to helm.
// Guards against argument injection (e.g., names or versions starting with "--").
func ValidateChartSpec(chart config.ChartSpec) error {
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
				addImage(imgStr, seen, result)
			}
		}
		if env, ok := v["env"].([]any); ok {
			collectEnvImages(env, seen, result)
		}
		if args, ok := v["args"].([]any); ok {
			collectArgImages(args, seen, result)
		}
		if kind, _ := v["kind"].(string); kind == "ConfigMap" {
			if data, ok := v["data"].(map[string]any); ok {
				collectConfigMapImages(data, seen, result)
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

func collectEnvImages(env []any, seen map[string]struct{}, result *[]string) {
	for _, item := range env {
		entry, ok := item.(map[string]any)
		if !ok {
			continue
		}

		name, _ := entry["name"].(string)
		value, _ := entry["value"].(string)
		if !isImageEnvName(name) {
			continue
		}

		for _, image := range extractImageValues(value) {
			addImage(image, seen, result)
		}
	}
}

func collectConfigMapImages(data map[string]any, seen map[string]struct{}, result *[]string) {
	for key, raw := range data {
		value, ok := raw.(string)
		if !ok || !isImageEnvName(key) {
			continue
		}

		for _, image := range extractImageValues(value) {
			addImage(image, seen, result)
		}
	}
}

func collectArgImages(args []any, seen map[string]struct{}, result *[]string) {
	for _, item := range args {
		arg, ok := item.(string)
		if !ok {
			continue
		}

		for _, piece := range strings.Fields(arg) {
			candidate := piece
			if idx := strings.LastIndex(candidate, "="); idx >= 0 {
				candidate = candidate[idx+1:]
			}
			for _, image := range extractImageValues(candidate) {
				addImage(image, seen, result)
			}
		}
	}
}

func addImage(image string, seen map[string]struct{}, result *[]string) {
	if _, exists := seen[image]; exists {
		return
	}

	seen[image] = struct{}{}
	*result = append(*result, image)
}

func isImageEnvName(name string) bool {
	upper := strings.ToUpper(name)
	return strings.Contains(upper, "_IMAGE") || upper == "STRIMZI_DEFAULT_MAVEN_BUILDER"
}

func extractImageValues(value string) []string {
	var images []string
	for _, line := range strings.Split(value, "\n") {
		candidate := strings.TrimSpace(line)
		if candidate == "" {
			continue
		}
		if idx := strings.Index(candidate, "="); idx >= 0 {
			candidate = strings.TrimSpace(candidate[idx+1:])
		}
		if looksLikeImageRef(candidate) {
			images = append(images, candidate)
		}
	}
	return images
}

func looksLikeImageRef(value string) bool {
	if strings.TrimSpace(value) == "" || strings.ContainsAny(value, " \t\r\n") {
		return false
	}
	if strings.HasPrefix(value, "http://") || strings.HasPrefix(value, "https://") {
		return false
	}

	base := value
	if at := strings.Index(base, "@"); at >= 0 {
		digest := base[at+1:]
		if !imageDigestPattern.MatchString(digest) {
			return false
		}
		base = base[:at]
	}

	lastColon := strings.LastIndex(base, ":")
	if lastColon < 0 {
		return false
	}

	name := base[:lastColon]
	tag := base[lastColon+1:]
	if !imageTagPattern.MatchString(tag) {
		return false
	}
	if strings.HasPrefix(name, "/") || !strings.Contains(name, "/") {
		return false
	}
	if !imageNamePattern.MatchString(name) {
		return false
	}

	return true
}

// applyOverride substitutes a tag variant in an image reference using the overrides map.
// The map key is a partial image path; if the image contains it and its tag ends with
// "-<from>", that suffix is replaced with "-<to>". Only the tag portion is rewritten.
// Keys are evaluated in sorted order for deterministic behavior when multiple match.
func applyOverride(image string, overrides map[string]config.Override) string {
	keys := make([]string, 0, len(overrides))
	for k := range overrides {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	name, tag := splitRef(image)
	for _, key := range keys {
		override := overrides[key]
		if strings.Contains(image, key) {
			suffix := "-" + override.From
			if strings.HasSuffix(tag, suffix) {
				return name + ":" + tag[:len(tag)-len(suffix)] + "-" + override.To
			}
		}
	}
	return image
}

// splitRef splits an image reference into its name and tag components.
// e.g., "docker.io/foo/bar:1.0-alpine" → ("docker.io/foo/bar", "1.0-alpine").
// Returns (image, "") if no tag separator follows the last slash.
func splitRef(ref string) (name, tag string) {
	lastSlash := strings.LastIndex(ref, "/")
	if lastColon := strings.LastIndex(ref, ":"); lastColon > lastSlash {
		return ref[:lastColon], ref[lastColon+1:]
	}
	return ref, ""
}
