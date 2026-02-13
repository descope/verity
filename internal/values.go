package internal

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/google/go-containerregistry/pkg/crane"
	"gopkg.in/yaml.v3"
)

// WrapperChart represents a Helm chart that wraps another chart with patched images.
type WrapperChart struct {
	Name         string
	Version      string
	Description  string
	Dependencies []Dependency
}

// CreateWrapperChart creates a complete Helm chart directory that subcharts the original
// with patched image values. This allows users to install the wrapper chart and get
// patched images while still being able to customize all original chart values.
//
// If registry is provided, it queries for existing wrapper chart versions to auto-increment
// the patch level. Otherwise, defaults to patch level 0.
//
// Returns the wrapper chart version that was created.
func CreateWrapperChart(dep Dependency, results []*PatchResult, outputDir, registry string) (string, error) {
	chartName := dep.Name
	chartDir := filepath.Join(outputDir, chartName)

	if err := os.MkdirAll(chartDir, 0o755); err != nil {
		return "", fmt.Errorf("creating chart directory: %w", err)
	}

	// Determine patch level by querying registry for existing versions
	patchLevel := 0
	if registry != "" {
		patchLevel = getNextPatchLevel(registry, chartName, dep.Version)
	}

	version := fmt.Sprintf("%s-%d", dep.Version, patchLevel)

	// Create Chart.yaml
	// Version format: {upstream-version}-{patch-level}
	// Example: prometheus 25.8.0 → prometheus 25.8.0-0
	// Patch level auto-increments when republishing the same upstream version
	wrapper := WrapperChart{
		Name:         chartName,
		Version:      version,
		Description:  fmt.Sprintf("%s with Copa-patched container images", dep.Name),
		Dependencies: []Dependency{dep},
	}
	if err := writeChartYaml(filepath.Join(chartDir, "Chart.yaml"), wrapper); err != nil {
		return "", err
	}

	// Create values.yaml with patched images namespaced under the dependency name
	if err := GenerateNamespacedValuesOverride(dep.Name, results, filepath.Join(chartDir, "values.yaml")); err != nil {
		return "", err
	}

	// Create .helmignore
	helmignore := `# Patterns to ignore when building packages
.git/
.gitignore
*.swp
*.bak
*.tmp
*~
.DS_Store
`
	if err := os.WriteFile(filepath.Join(chartDir, ".helmignore"), []byte(helmignore), 0o644); err != nil {
		return "", fmt.Errorf("writing .helmignore: %w", err)
	}

	// Create reports/ directory and copy Trivy JSON reports
	reportsDir := filepath.Join(chartDir, "reports")
	if err := os.MkdirAll(reportsDir, 0o755); err != nil {
		return "", fmt.Errorf("creating reports directory: %w", err)
	}

	for _, r := range results {
		// Prefer the upstream (pre-patch) report for "before" data.
		src := r.UpstreamReportPath
		if src == "" {
			src = r.ReportPath
		}
		if src == "" {
			continue
		}
		// Always name by the original image ref so site data can match it.
		reportName := sanitize(r.Original.Reference()) + ".json"
		destPath := filepath.Join(reportsDir, reportName)
		if err := copyFile(src, destPath); err != nil {
			return "", fmt.Errorf("copying report %s: %w", reportName, err)
		}
	}

	// Save override metadata for site data generation.
	if err := SaveOverrides(results, chartDir); err != nil {
		return "", fmt.Errorf("saving overrides: %w", err)
	}

	return version, nil
}

func writeChartYaml(path string, chart WrapperChart) error {
	type chartYaml struct {
		APIVersion   string       `yaml:"apiVersion"`
		Name         string       `yaml:"name"`
		Description  string       `yaml:"description"`
		Type         string       `yaml:"type"`
		Version      string       `yaml:"version"`
		Dependencies []Dependency `yaml:"dependencies"`
	}

	c := chartYaml{
		APIVersion:   "v2",
		Name:         chart.Name,
		Description:  chart.Description,
		Type:         "application",
		Version:      chart.Version,
		Dependencies: chart.Dependencies,
	}

	data, err := yaml.Marshal(c)
	if err != nil {
		return fmt.Errorf("marshaling Chart.yaml: %w", err)
	}
	return os.WriteFile(path, data, 0o644)
}

// GenerateNamespacedValuesOverride generates a values.yaml file with patched images
// namespaced under the chart name. This allows the values to be used with a parent
// chart that subcharts the original.
func GenerateNamespacedValuesOverride(chartName string, results []*PatchResult, path string) error {
	inner := make(map[string]interface{})

	for _, r := range results {
		if r.Error != nil {
			continue
		}
		// Include skipped images that have a valid patched ref (e.g. already
		// patched in registry). This ensures values.yaml stays current even
		// when all images are up-to-date and no new patching occurs.
		if r.Skipped && r.Patched.Repository == "" {
			continue
		}
		setImageAtPath(inner, r.Original.Path, r.Patched)
	}

	if len(inner) == 0 {
		return nil
	}

	// Wrap the values under the chart name
	root := map[string]interface{}{
		chartName: inner,
	}

	data, err := yaml.Marshal(root)
	if err != nil {
		return fmt.Errorf("marshaling values override: %w", err)
	}

	// Build comment header noting any image overrides applied.
	var header string
	for _, r := range results {
		if r.OverriddenFrom != "" && r.Error == nil && !r.Skipped {
			header += fmt.Sprintf("# NOTE: %s was overridden from %q to %q for Copa compatibility\n",
				r.Original.Repository, r.OverriddenFrom, r.Original.Tag)
		}
	}

	var out []byte
	if header != "" {
		out = append([]byte(header), data...)
	} else {
		out = data
	}
	return os.WriteFile(path, out, 0o644)
}

// GenerateValuesOverride builds a Helm values override file that remaps
// original image references to their patched equivalents.
//
// Each PatchResult.Original.Path (e.g. "server.image") determines where in
// the nested YAML the image fields are set.
func GenerateValuesOverride(results []*PatchResult, path string) error {
	root := make(map[string]interface{})

	for _, r := range results {
		if r.Error != nil {
			continue
		}
		if r.Skipped && r.Patched.Repository == "" {
			continue
		}
		setImageAtPath(root, r.Original.Path, r.Patched)
	}

	if len(root) == 0 {
		return nil
	}

	data, err := yaml.Marshal(root)
	if err != nil {
		return fmt.Errorf("marshaling values override: %w", err)
	}
	return os.WriteFile(path, data, 0o644)
}

// setImageAtPath sets registry/repository/tag at a dot-separated path like
// "server.image" → {server: {image: {registry: ..., repository: ..., tag: ...}}}.
func setImageAtPath(root map[string]interface{}, dotPath string, img Image) {
	parts := strings.Split(dotPath, ".")
	current := root

	// Walk/create intermediate maps.
	for _, key := range parts {
		if existing, ok := current[key]; ok {
			if m, ok := existing.(map[string]interface{}); ok {
				current = m
			} else {
				m := make(map[string]interface{})
				current[key] = m
				current = m
			}
		} else {
			m := make(map[string]interface{})
			current[key] = m
			current = m
		}
	}

	// Set the image fields at the leaf.
	if img.Registry != "" {
		current["registry"] = img.Registry
	}
	current["repository"] = img.Repository
	if img.Tag != "" {
		current["tag"] = img.Tag
	}
}

// copyFile copies a file from src to dst.
func copyFile(src, dst string) error {
	sourceFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer sourceFile.Close()

	destFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer destFile.Close()

	if _, err := io.Copy(destFile, sourceFile); err != nil {
		return err
	}

	return destFile.Sync()
}

// getNextPatchLevel queries the OCI registry for existing wrapper chart versions
// and returns the next patch level for the given upstream version.
// Returns 0 if no existing versions found or on error.
func getNextPatchLevel(registry, chartName, upstreamVersion string) int {
	// OCI chart reference: {registry}/charts/{chartname}
	// Example: ghcr.io/<org>/charts/prometheus
	chartRef := fmt.Sprintf("%s/charts/%s", registry, chartName)

	// List all tags for this chart
	tags, err := crane.ListTags(chartRef)
	if err != nil {
		// If chart doesn't exist yet or error, start at patch level 0
		return 0
	}

	// Find all tags matching this upstream version pattern: {version}-{patch}
	// Example: 25.8.0-0, 25.8.0-1, 25.8.0-2
	prefix := upstreamVersion + "-"
	var patchLevels []int

	for _, tag := range tags {
		if strings.HasPrefix(tag, prefix) {
			// Extract patch level from tag
			patchStr := strings.TrimPrefix(tag, prefix)
			if patch, err := strconv.Atoi(patchStr); err == nil {
				patchLevels = append(patchLevels, patch)
			}
		}
	}

	if len(patchLevels) == 0 {
		// No existing versions for this upstream version, start at 0
		return 0
	}

	// Find highest patch level and increment
	sort.Ints(patchLevels)
	return patchLevels[len(patchLevels)-1] + 1
}
