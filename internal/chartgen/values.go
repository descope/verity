package chartgen

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/verity-org/verity/internal/config"
	"github.com/verity-org/verity/internal/discovery"
)

type ValueOverride struct {
	Path          string `json:"path"`
	Repository    string `json:"repository"`
	Tag           string `json:"tag"`
	Value         string `json:"value,omitempty"`
	ClearRegistry bool   `json:"clearRegistry,omitempty"`
}

type repoTagPair struct {
	Path        string
	Repo        string
	HasTag      bool
	Registry    string
	HasRegistry bool
}

func ResolveValuePaths(valuesYAML []byte, mappings []ImageMapping, overrides map[string]config.Override) ([]ValueOverride, error) {
	return ResolveValuePathsWithSubcharts(valuesYAML, nil, mappings, overrides)
}

func ResolveValuePathsWithSubcharts(valuesYAML []byte, subchartValues map[string][]byte, mappings []ImageMapping, overrides map[string]config.Override) ([]ValueOverride, error) {
	pairs, err := collectValuePairs(valuesYAML, "")
	if err != nil {
		return nil, err
	}

	subchartNames := make([]string, 0, len(subchartValues))
	for name := range subchartValues {
		subchartNames = append(subchartNames, name)
	}
	sort.Strings(subchartNames)
	for _, name := range subchartNames {
		subchartPairs, err := collectValuePairs(subchartValues[name], name)
		if err != nil {
			return nil, err
		}
		pairs = append(pairs, subchartPairs...)
	}
	sort.Slice(pairs, func(i, j int) bool {
		return pairs[i].Path < pairs[j].Path
	})

	return resolveValuePathPairs(pairs, mappings, overrides), nil
}

func resolveValuePathPairs(pairs []repoTagPair, mappings []ImageMapping, overrides map[string]config.Override) []ValueOverride {
	result := make([]ValueOverride, 0, len(mappings))
	matched := make([]bool, len(mappings))

	overrideKeys := make([]string, 0, len(overrides))
	for key := range overrides {
		overrideKeys = append(overrideKeys, key)
	}
	sort.Strings(overrideKeys)

	pathHasRegistry := make(map[string]bool, len(pairs))
	for _, p := range pairs {
		if p.HasRegistry {
			pathHasRegistry[p.Path] = true
		}
	}

	for i, m := range mappings {
		for _, key := range overrideKeys {
			override := overrides[key]
			if override.ValuePath == "" {
				continue
			}
			if matchesRepo(m.OriginalRepo, key) {
				result = append(result, ValueOverride{
					Path:          override.ValuePath,
					Repository:    m.PatchedRepo,
					Tag:           m.PatchedTag,
					ClearRegistry: pathHasRegistry[override.ValuePath],
				})
				matched[i] = true
				break
			}
		}
	}

	for _, pair := range pairs {
		if !pair.HasTag {
			continue
		}
		for i, m := range mappings {
			if matched[i] {
				continue
			}
			if matchesRepo(pair.Repo, m.OriginalRepo) {
				result = append(result, ValueOverride{
					Path:          pair.Path,
					Repository:    m.PatchedRepo,
					Tag:           m.PatchedTag,
					ClearRegistry: pair.HasRegistry,
				})
				matched[i] = true
				break
			}
		}
	}

	return result
}

func collectValuePairs(valuesYAML []byte, prefix string) ([]repoTagPair, error) {
	values := make(map[string]any)
	if len(valuesYAML) > 0 {
		if err := yaml.Unmarshal(valuesYAML, &values); err != nil {
			return nil, fmt.Errorf("unmarshal values YAML: %w", err)
		}
	}

	pairs := make([]repoTagPair, 0)
	walkValues(prefix, values, &pairs)
	return pairs, nil
}

func GetChartValues(chart config.ChartSpec) ([]byte, error) {
	if err := discovery.ValidateChartSpec(chart); err != nil {
		return nil, fmt.Errorf("validate chart spec: %w", err)
	}

	args := []string{"show", "values"}
	if strings.HasPrefix(chart.Repository, "oci://") {
		args = append(args, chart.Repository+"/"+chart.Name)
	} else {
		args = append(args, chart.Name, "--repo", chart.Repository)
	}
	args = append(args, "--version", chart.Version)

	out, err := runCommand(context.Background(), 5*time.Minute, "helm", args...)
	if err != nil {
		return nil, fmt.Errorf("get chart values for %s: %w", chart.Name, err)
	}

	return []byte(out), nil
}

// GetSubchartValues fetches the parent chart dependencies and returns each
// subchart values.yaml keyed by subchart name.
func GetSubchartValues(chart config.ChartSpec) (map[string][]byte, error) {
	if err := discovery.ValidateChartSpec(chart); err != nil {
		return nil, fmt.Errorf("validate chart spec: %w", err)
	}

	tmpDir, err := os.MkdirTemp("", "verity-chartgen-subcharts-")
	if err != nil {
		return nil, fmt.Errorf("create temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	if err := writeSubchartDependencyChart(tmpDir, chart); err != nil {
		return nil, err
	}

	if _, err := runCommand(context.Background(), 5*time.Minute, "helm", "dependency", "build", tmpDir); err != nil {
		return nil, fmt.Errorf("helm dependency build for %s: %w", chart.Name, err)
	}

	subchartValues := make(map[string][]byte)
	chartsDir := filepath.Join(tmpDir, "charts")
	parentExtractDir := filepath.Join(tmpDir, "parent-chart")
	if err := collectNestedSubchartValues(chartsDir, parentExtractDir, subchartValues); err != nil {
		return nil, err
	}

	return subchartValues, nil
}

func writeSubchartDependencyChart(tmpDir string, chart config.ChartSpec) error {
	chartYAML, err := yaml.Marshal(map[string]any{
		"apiVersion": "v2",
		"name":       "verity-subchart-values",
		"version":    "0.0.0",
		"dependencies": []map[string]string{{
			"name":       chart.Name,
			"version":    chart.Version,
			"repository": chart.Repository,
		}},
	})
	if err != nil {
		return fmt.Errorf("marshal temp Chart.yaml: %w", err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, "Chart.yaml"), chartYAML, 0o644); err != nil {
		return fmt.Errorf("write temp Chart.yaml: %w", err)
	}
	return nil
}

func collectNestedSubchartValues(chartsDir, parentExtractDir string, subchartValues map[string][]byte) error {
	archives, err := enumerateSubchartArchives(chartsDir)
	if err != nil {
		return err
	}
	for _, archive := range archives {
		chartName, err := extractTarball(archive, parentExtractDir)
		if err != nil {
			fmt.Fprintf(os.Stderr, "warning: skipping unreadable parent chart archive %s: %v\n", filepath.Base(archive), err)
			continue
		}
		if err := collectParentSubcharts(filepath.Join(parentExtractDir, chartName), subchartValues); err != nil {
			return err
		}
	}

	entries, err := os.ReadDir(chartsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("read charts directory: %w", err)
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		if err := collectParentSubcharts(filepath.Join(chartsDir, entry.Name()), subchartValues); err != nil {
			return err
		}
	}

	return nil
}

func collectParentSubcharts(parentChartDir string, subchartValues map[string][]byte) error {
	if parentChartDir == "" {
		return nil
	}
	return collectSubchartValues(filepath.Join(parentChartDir, "charts"), subchartValues)
}

func subchartKeyFromArchive(archive, version string) string {
	base := strings.TrimSuffix(filepath.Base(archive), ".tgz")
	if version != "" {
		if stripped, ok := strings.CutSuffix(base, "-"+version); ok {
			return stripped
		}
	}
	idx := strings.LastIndex(base, "-")
	if idx > 0 && idx < len(base)-1 {
		first := base[idx+1]
		if first >= '0' && first <= '9' {
			return base[:idx]
		}
	}
	return base
}

func enumerateSubchartArchives(chartsDir string) ([]string, error) {
	entries, err := os.ReadDir(chartsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read charts directory: %w", err)
	}

	archives := make([]string, 0)
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".tgz" {
			continue
		}
		archives = append(archives, filepath.Join(chartsDir, entry.Name()))
	}
	sort.Strings(archives)
	return archives, nil
}

func collectSubchartValues(chartsDir string, subchartValues map[string][]byte) error {
	archives, err := enumerateSubchartArchives(chartsDir)
	if err != nil {
		return err
	}
	for _, archive := range archives {
		valuesYAML, _, version, err := readChartArchive(archive)
		if err != nil {
			fmt.Fprintf(os.Stderr, "warning: skipping unreadable subchart archive %s: %v\n", filepath.Base(archive), err)
			continue
		}
		if len(valuesYAML) == 0 {
			continue
		}
		subchartValues[subchartKeyFromArchive(archive, version)] = valuesYAML
	}

	entries, err := os.ReadDir(chartsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("read charts directory: %w", err)
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		valuesPath := filepath.Join(chartsDir, entry.Name(), "values.yaml")
		valuesYAML, err := os.ReadFile(valuesPath)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			fmt.Fprintf(os.Stderr, "warning: skipping unreadable subchart values %s: %v\n", valuesPath, err)
			continue
		}
		subchartValues[entry.Name()] = valuesYAML
	}

	return nil
}

var (
	ErrUnsafeTarballEntry = errors.New("unsafe tarball entry path")
	ErrEmptyTarball       = errors.New("tarball contains no chart entries")
)

func safeExtractPath(destDir, name string) (string, error) {
	cleaned := filepath.Clean(filepath.FromSlash(name))
	if cleaned == "." || cleaned == ".." || filepath.IsAbs(cleaned) || strings.HasPrefix(cleaned, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("%w: %q", ErrUnsafeTarballEntry, name)
	}
	target := filepath.Join(destDir, cleaned)
	rel, err := filepath.Rel(destDir, target)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("%w: %q escapes %q", ErrUnsafeTarballEntry, name, destDir)
	}
	return target, nil
}

func extractTarball(tgzPath, destDir string) (string, error) {
	return visitTarballEntries(tgzPath, func(header *tar.Header, name string, tarReader io.Reader) error {
		switch header.Typeflag {
		case tar.TypeSymlink, tar.TypeLink:
			return nil
		}
		targetPath, err := safeExtractPath(destDir, name)
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(destDir, targetPath)
		if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return fmt.Errorf("%w: %q escapes %q", ErrUnsafeTarballEntry, name, destDir)
		}
		switch header.Typeflag {
		case tar.TypeDir:
			return extractTarballDirEntry(destDir, targetPath, name)
		case tar.TypeReg:
			return extractTarballRegularEntry(tgzPath, destDir, targetPath, name, header, tarReader)
		}
		return nil
	})
}

func extractTarballDirEntry(destDir, targetPath, name string) error {
	rel, err := filepath.Rel(destDir, targetPath)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return fmt.Errorf("%w: %q escapes %q", ErrUnsafeTarballEntry, name, destDir)
	}
	return os.MkdirAll(targetPath, 0o755)
}

func extractTarballRegularEntry(tgzPath, destDir, targetPath, name string, header *tar.Header, tarReader io.Reader) error {
	rel, err := filepath.Rel(destDir, targetPath)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return fmt.Errorf("%w: %q escapes %q", ErrUnsafeTarballEntry, name, destDir)
	}
	if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
		return fmt.Errorf("create parent directory for %s: %w", targetPath, err)
	}
	fileMode := os.FileMode(header.Mode) & 0o777
	if fileMode == 0 {
		fileMode = 0o644
	}
	rel, err = filepath.Rel(destDir, targetPath)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return fmt.Errorf("%w: %q escapes %q", ErrUnsafeTarballEntry, name, destDir)
	}
	out, err := os.OpenFile(targetPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, fileMode)
	if err != nil {
		return fmt.Errorf("open extracted file %s: %w", targetPath, err)
	}
	if _, err := io.Copy(out, tarReader); err != nil {
		out.Close()
		return fmt.Errorf("copy file %s from %s: %w", name, filepath.Base(tgzPath), err)
	}
	if err := out.Close(); err != nil {
		return fmt.Errorf("close extracted file %s: %w", targetPath, err)
	}
	return nil
}

// extractValuesFromTarball reads a Helm chart tarball and returns the chart
// name together with the raw values.yaml bytes when present.
func extractValuesFromTarball(tgzPath string) (valuesYAML []byte, chartName string, err error) {
	valuesYAML, chartName, _, err = readChartArchive(tgzPath)
	return valuesYAML, chartName, err
}

func readChartArchive(tgzPath string) (valuesYAML []byte, chartName, version string, err error) {
	var chartYAML []byte
	chartName, err = visitTarballEntries(tgzPath, func(_ *tar.Header, name string, tarReader io.Reader) error {
		parts := strings.Split(name, "/")
		if len(parts) != 2 {
			return nil
		}
		switch parts[1] {
		case "values.yaml":
			data, readErr := io.ReadAll(tarReader)
			if readErr != nil {
				return fmt.Errorf("read values.yaml from %s: %w", filepath.Base(tgzPath), readErr)
			}
			valuesYAML = data
		case "Chart.yaml":
			data, readErr := io.ReadAll(tarReader)
			if readErr != nil {
				return fmt.Errorf("read Chart.yaml from %s: %w", filepath.Base(tgzPath), readErr)
			}
			chartYAML = data
		}
		return nil
	})
	if err != nil {
		return nil, "", "", err
	}
	if len(chartYAML) > 0 {
		var doc struct {
			Version string `yaml:"version"`
		}
		if unmarshalErr := yaml.Unmarshal(chartYAML, &doc); unmarshalErr == nil {
			version = doc.Version
		}
	}
	return valuesYAML, chartName, version, nil
}

func visitTarballEntries(tgzPath string, visit func(header *tar.Header, name string, tarReader io.Reader) error) (string, error) {
	file, err := os.Open(tgzPath)
	if err != nil {
		return "", fmt.Errorf("open tarball %s: %w", filepath.Base(tgzPath), err)
	}
	defer file.Close()

	gzReader, err := gzip.NewReader(file)
	if err != nil {
		return "", fmt.Errorf("open gzip stream for %s: %w", filepath.Base(tgzPath), err)
	}
	defer gzReader.Close()

	tarReader := tar.NewReader(gzReader)
	chartName := ""
	for {
		header, err := tarReader.Next()
		if errors.Is(err, io.EOF) {
			if chartName == "" {
				return "", fmt.Errorf("%w: %s", ErrEmptyTarball, filepath.Base(tgzPath))
			}
			return chartName, nil
		}
		if err != nil {
			return "", fmt.Errorf("read tarball %s: %w", filepath.Base(tgzPath), err)
		}

		name := strings.TrimPrefix(header.Name, "./")
		if name == "" {
			continue
		}

		parts := strings.Split(name, "/")
		if len(parts) == 0 || parts[0] == "" {
			continue
		}
		if chartName == "" {
			chartName = parts[0]
		}

		if err := visit(header, name, tarReader); err != nil {
			return "", err
		}
	}
}

func walkValues(prefix string, node map[string]any, pairs *[]repoTagPair) {
	for key, val := range node {
		child, ok := val.(map[string]any)
		if !ok {
			continue
		}

		path := key
		if prefix != "" {
			path = prefix + "." + key
		}

		repo, hasRepo := child["repository"].(string)
		registry, registrySibling := child["registry"].(string)
		hasRegistry := registrySibling && registry != ""

		switch {
		case hasRepo && repo != "":
			_, hasTag := child["tag"]
			*pairs = append(*pairs, repoTagPair{
				Path:        path,
				Repo:        repo,
				HasTag:      hasTag,
				Registry:    registry,
				HasRegistry: hasRegistry,
			})
		case hasRegistry:
			*pairs = append(*pairs, repoTagPair{
				Path:        path,
				Registry:    registry,
				HasRegistry: true,
			})
		}

		walkValues(path, child, pairs)
	}
}

func matchesRepo(imageRepo, candidate string) bool {
	if imageRepo == candidate {
		return true
	}
	if strings.HasSuffix(imageRepo, "/"+candidate) {
		return true
	}
	if strings.HasSuffix(candidate, "/"+imageRepo) {
		return true
	}
	return false
}
