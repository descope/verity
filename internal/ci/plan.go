package ci

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/verity-org/verity/internal/config"
	copadiscovery "github.com/verity-org/verity/internal/discovery"
	"github.com/verity-org/verity/internal/integer/apkindex"
	intconfig "github.com/verity-org/verity/internal/integer/config"
	intdiscovery "github.com/verity-org/verity/internal/integer/discovery"
)

type Matrix struct {
	Include []map[string]string `json:"include"`
}

type Plan struct {
	Kind        string `json:"kind"`
	HasChanges  bool   `json:"hasChanges"`
	Strict      bool   `json:"strict,omitempty"`
	Matrix      Matrix `json:"matrix"`
	SmokeMatrix Matrix `json:"smokeMatrix,omitempty"`
}

type IntegerPROptions struct {
	ChangedFiles []string
	ConfigPath   string
	ImagesDir    string
	APKIndexURL  string
	CacheDir     string
	GenDir       string
}

type CopaPROptions struct {
	ChangedFiles     []string
	BaseConfigPath   string
	HeadConfigPath   string
	TargetRegistry   string
	ChartsFile       string
	VerityConfig     string
	IntegerImagesDir string
}

type ChartOptions struct {
	EventName      string
	InputChart     string
	ChangedFiles   []string
	ChartsFile     string
	BaseChartsFile string
	VerityConfig   string
	ValuesDir      string
}

func PlanIntegerPR(opts IntegerPROptions) (Plan, error) {
	plan := Plan{Kind: "integer-pr"}
	imageNames, allImages := changedIntegerImages(opts.ChangedFiles)
	if !allImages && len(imageNames) == 0 {
		plan.Matrix = Matrix{}
		plan.SmokeMatrix = Matrix{}
		return plan, nil
	}

	cfg, err := intconfig.LoadConfig(defaultString(opts.ConfigPath, "integer.yaml"))
	if err != nil {
		return plan, fmt.Errorf("load integer config: %w", err)
	}

	var pkgs []apkindex.Package
	if opts.APKIndexURL != "" {
		pkgs, err = apkindex.Fetch(opts.APKIndexURL, defaultString(opts.CacheDir, os.TempDir()), apkindex.DefaultCacheMaxAge)
		if err != nil {
			return plan, fmt.Errorf("fetch apkindex: %w", err)
		}
	}

	imgs, err := intdiscovery.DiscoverFromFiles(intdiscovery.Options{
		ImagesDir: defaultString(opts.ImagesDir, "images"),
		Registry:  cfg.Target.Registry,
		Packages:  pkgs,
		GenDir:    opts.GenDir,
	})
	if err != nil {
		return plan, fmt.Errorf("discover integer images: %w", err)
	}
	if !allImages {
		imgs = filterIntegerByName(imgs, imageNames)
	}
	if len(imgs) == 0 {
		plan.Matrix = Matrix{}
		plan.SmokeMatrix = Matrix{}
		return plan, nil
	}

	plan.HasChanges = true
	plan.SmokeMatrix = integerMatrix(imgs)
	plan.Matrix = latestIntegerMatrix(imgs)
	return plan, nil
}

func PlanCopaPR(opts CopaPROptions) (Plan, error) {
	plan := Plan{Kind: "copa-pr"}
	if !containsPath(opts.ChangedFiles, "copa-config.yaml") {
		plan.Matrix = Matrix{}
		return plan, nil
	}

	names, err := changedCopaNames(opts.BaseConfigPath, defaultString(opts.HeadConfigPath, "copa-config.yaml"))
	if err != nil {
		return plan, err
	}
	if len(names) == 0 {
		plan.Matrix = Matrix{}
		return plan, nil
	}

	cfg, err := copadiscovery.LoadConfig(defaultString(opts.HeadConfigPath, "copa-config.yaml"))
	if err != nil {
		return plan, fmt.Errorf("load copa config: %w", err)
	}
	cfg.Charts = nil
	images, err := copadiscovery.DiscoverWithChartValues(cfg, opts.TargetRegistry, nil, nil, nil, nil)
	if err != nil {
		return plan, fmt.Errorf("discover copa images: %w", err)
	}
	images = filterCopaByName(images, names)
	if len(images) == 0 {
		plan.Matrix = Matrix{}
		return plan, nil
	}
	plan.HasChanges = true
	plan.Matrix = firstCopaTagMatrix(images)
	return plan, nil
}

func PlanCharts(opts ChartOptions) (Plan, error) {
	plan := Plan{Kind: "chart"}
	charts, err := copadiscovery.LoadChartsFile(defaultString(opts.ChartsFile, "Chart.yaml"))
	if err != nil {
		return plan, fmt.Errorf("load charts: %w", err)
	}
	chartNames := chartNameSet(charts)

	var selected []string
	if strings.TrimSpace(opts.InputChart) != "" {
		selected = []string{strings.TrimSpace(opts.InputChart)}
	} else if opts.EventName != "pull_request" {
		selected = sortedChartNames(chartNames)
	} else {
		selected, plan.Strict, err = affectedCharts(opts, charts, chartNames)
		if err != nil {
			return plan, err
		}
	}

	selected = filterKnownCharts(selected, chartNames)
	plan.HasChanges = len(selected) > 0
	plan.Matrix = chartMatrix(selected)
	return plan, nil
}

func changedIntegerImages(files []string) (names map[string]struct{}, all bool) {
	names = map[string]struct{}{}
	for _, f := range files {
		f = filepath.ToSlash(strings.TrimSpace(f))
		switch {
		case f == "integer.yaml", strings.HasPrefix(f, "images/_base/"):
			all = true
		case strings.HasPrefix(f, "images/") && strings.HasSuffix(f, ".yaml") && !strings.Contains(f, "/_base/"):
			name := strings.TrimSuffix(strings.TrimPrefix(f, "images/"), ".yaml")
			if name != "" {
				names[name] = struct{}{}
			}
		}
	}
	return names, all
}

func changedCopaNames(basePath, headPath string) (map[string]struct{}, error) {
	head, err := loadCopaImages(headPath)
	if err != nil {
		return nil, fmt.Errorf("load head copa config: %w", err)
	}
	base := map[string]config.ImageSpec{}
	if basePath != "" {
		base, err = loadCopaImages(basePath)
		if err != nil {
			return nil, fmt.Errorf("load base copa config: %w", err)
		}
	}

	names := map[string]struct{}{}
	for name, img := range head {
		if old, ok := base[name]; !ok || !reflect.DeepEqual(old, img) {
			names[name] = struct{}{}
		}
	}
	return names, nil
}

func loadCopaImages(path string) (map[string]config.ImageSpec, error) {
	cfg, err := copadiscovery.LoadConfig(path)
	if err != nil {
		return nil, err
	}
	out := make(map[string]config.ImageSpec, len(cfg.Images))
	for _, img := range cfg.Images {
		out[img.Name] = img
	}
	return out, nil
}

func affectedCharts(opts ChartOptions, charts []config.ChartSpec, chartNames map[string]struct{}) ([]string, bool, error) {
	if matchesAny(opts.ChangedFiles, broadChartPatterns()) || changedUnder(opts.ChangedFiles, "images/_base/") {
		return sortedChartNames(chartNames), false, nil
	}

	selected := map[string]struct{}{}
	strict := false
	if containsPath(opts.ChangedFiles, "Chart.yaml") {
		strict = true
		changed, err := changedChartDependencies(opts.BaseChartsFile, defaultString(opts.ChartsFile, "Chart.yaml"))
		if err != nil {
			return nil, false, err
		}
		for _, name := range changed {
			if _, ok := chartNames[name]; ok {
				selected[name] = struct{}{}
			}
		}
	}

	changedImages := changedImageNames(opts.ChangedFiles)
	if len(changedImages) > 0 {
		vc, err := copadiscovery.LoadVerityConfig(defaultString(opts.VerityConfig, "verity.yaml"))
		if err != nil {
			return nil, false, fmt.Errorf("load verity config: %w", err)
		}
		chartValueKeys := mapKeys(vc.ChartValues)
		for _, image := range changedImages {
			addFuzzyChartMatches(selected, normalize(image), charts, chartValueKeys, chartNames)
			addValueFileMatches(selected, defaultString(opts.ValuesDir, filepath.Join("test", "chart-integration", "values")), image, chartNames)
			addReplacementMatches(selected, image, vc.Replacements, charts, chartValueKeys, chartNames)
		}
	}

	return sortedChartNames(selected), strict, nil
}

func changedChartDependencies(basePath, headPath string) ([]string, error) {
	head, err := loadChartDepMap(headPath)
	if err != nil {
		return nil, fmt.Errorf("load head Chart.yaml: %w", err)
	}
	base := map[string]config.ChartSpec{}
	if basePath != "" {
		base, err = loadChartDepMap(basePath)
		if err != nil {
			return nil, fmt.Errorf("load base Chart.yaml: %w", err)
		}
	}
	var changed []string
	for name, dep := range head {
		if old, ok := base[name]; !ok || old.Version != dep.Version || old.Repository != dep.Repository {
			changed = append(changed, name)
		}
	}
	sort.Strings(changed)
	return changed, nil
}

func loadChartDepMap(path string) (map[string]config.ChartSpec, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var file config.HelmChartFile
	if err := yaml.Unmarshal(data, &file); err != nil {
		return nil, err
	}
	out := make(map[string]config.ChartSpec, len(file.Dependencies))
	for _, dep := range file.Dependencies {
		out[dep.Name] = dep
	}
	return out, nil
}

func addFuzzyChartMatches(selected map[string]struct{}, imageNorm string, charts []config.ChartSpec, chartValueKeys []string, chartNames map[string]struct{}) {
	for _, chart := range charts {
		chartNorm := normalize(chart.Name)
		if imageNorm == chartNorm || strings.HasPrefix(imageNorm, chartNorm) || strings.HasPrefix(chartNorm, imageNorm) {
			selected[chart.Name] = struct{}{}
		}
	}
	for _, key := range chartValueKeys {
		keyNorm := normalize(key)
		if imageNorm == keyNorm || strings.HasPrefix(imageNorm, keyNorm) || strings.HasPrefix(keyNorm, imageNorm) {
			if _, ok := chartNames[key]; ok {
				selected[key] = struct{}{}
			}
		}
	}
}

func addValueFileMatches(selected map[string]struct{}, valuesDir, image string, chartNames map[string]struct{}) {
	entries, err := os.ReadDir(valuesDir)
	if err != nil {
		return
	}
	re := regexp.MustCompile(`verity-org/(.*/)?` + regexp.QuoteMeta(image) + `([:@"'\s]|$)|\b` + regexp.QuoteMeta(image) + `\b`)
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".yaml" {
			continue
		}
		chart := strings.TrimSuffix(entry.Name(), ".yaml")
		if _, ok := chartNames[chart]; !ok {
			continue
		}
		data, err := os.ReadFile(filepath.Join(valuesDir, entry.Name()))
		if err == nil && re.Match(data) {
			selected[chart] = struct{}{}
		}
	}
}

func addReplacementMatches(selected map[string]struct{}, image string, replacements map[string]config.Replacement, charts []config.ChartSpec, chartValueKeys []string, chartNames map[string]struct{}) {
	imageNorm := normalize(image)
	for _, repl := range replacements {
		replImage := repl.Image
		if idx := strings.LastIndex(replImage, "/"); idx >= 0 {
			replImage = replImage[idx+1:]
		}
		replNorm := normalize(replImage)
		if replNorm == imageNorm || strings.HasPrefix(replNorm, imageNorm) || strings.HasPrefix(imageNorm, replNorm) {
			addFuzzyChartMatches(selected, replNorm, charts, chartValueKeys, chartNames)
		}
	}
}

func integerMatrix(imgs []intdiscovery.DiscoveredImage) Matrix {
	include := make([]map[string]string, 0, len(imgs))
	for _, img := range imgs {
		include = append(include, map[string]string{
			"image":   img.Name,
			"version": img.Version,
			"type":    img.Type,
		})
	}
	return Matrix{Include: include}
}

func latestIntegerMatrix(imgs []intdiscovery.DiscoveredImage) Matrix {
	latest := map[string]intdiscovery.DiscoveredImage{}
	for _, img := range imgs {
		key := img.Name + "\x00" + img.Type
		prev, ok := latest[key]
		if !ok || apkindex.VersionLess(prev.Version, img.Version) {
			latest[key] = img
		}
	}
	out := make([]intdiscovery.DiscoveredImage, 0, len(latest))
	for _, img := range latest {
		out = append(out, img)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Name != out[j].Name {
			return out[i].Name < out[j].Name
		}
		if out[i].Type != out[j].Type {
			return out[i].Type < out[j].Type
		}
		return apkindex.VersionLess(out[i].Version, out[j].Version)
	})
	return integerMatrix(out)
}

func firstCopaTagMatrix(imgs []copadiscovery.DiscoveredImage) Matrix {
	sort.Slice(imgs, func(i, j int) bool {
		if imgs[i].Name != imgs[j].Name {
			return imgs[i].Name < imgs[j].Name
		}
		return imgs[i].Source < imgs[j].Source
	})
	seen := map[string]struct{}{}
	include := []map[string]string{}
	for _, img := range imgs {
		if _, ok := seen[img.Name]; ok {
			continue
		}
		seen[img.Name] = struct{}{}
		include = append(include, map[string]string{
			"name": img.Name,
			"tag":  sourceTag(img.Source),
		})
	}
	return Matrix{Include: include}
}

func chartMatrix(charts []string) Matrix {
	include := make([]map[string]string, 0, len(charts))
	for _, chart := range charts {
		include = append(include, map[string]string{"chart": chart})
	}
	return Matrix{Include: include}
}

func filterIntegerByName(imgs []intdiscovery.DiscoveredImage, names map[string]struct{}) []intdiscovery.DiscoveredImage {
	out := imgs[:0]
	for _, img := range imgs {
		if _, ok := names[img.Name]; ok {
			out = append(out, img)
		}
	}
	return out
}

func filterCopaByName(imgs []copadiscovery.DiscoveredImage, names map[string]struct{}) []copadiscovery.DiscoveredImage {
	out := imgs[:0]
	for _, img := range imgs {
		if _, ok := names[img.Name]; ok {
			out = append(out, img)
		}
	}
	return out
}

func changedImageNames(files []string) []string {
	set := map[string]struct{}{}
	for _, f := range files {
		f = filepath.ToSlash(strings.TrimSpace(f))
		if strings.HasPrefix(f, "images/") && strings.HasSuffix(f, ".yaml") && !strings.Contains(f, "/_base/") {
			name := strings.TrimSuffix(strings.TrimPrefix(f, "images/"), ".yaml")
			if name != "" {
				set[name] = struct{}{}
			}
		}
	}
	return sortedChartNames(set)
}

func chartNameSet(charts []config.ChartSpec) map[string]struct{} {
	set := make(map[string]struct{}, len(charts))
	for _, chart := range charts {
		set[chart.Name] = struct{}{}
	}
	return set
}

func filterKnownCharts(charts []string, known map[string]struct{}) []string {
	out := charts[:0]
	for _, chart := range charts {
		if _, ok := known[chart]; ok {
			out = append(out, chart)
		}
	}
	sort.Strings(out)
	return out
}

func sortedChartNames(set map[string]struct{}) []string {
	out := make([]string, 0, len(set))
	for name := range set {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

func mapKeys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func broadChartPatterns() []*regexp.Regexp {
	return []*regexp.Regexp{
		regexp.MustCompile(`^test/chart-integration/`),
		regexp.MustCompile(`^\.github/workflows/chart-integration\.yaml$`),
		regexp.MustCompile(`^mise\.toml$`),
		regexp.MustCompile(`^Makefile$`),
		regexp.MustCompile(`^verity\.yaml$`),
		regexp.MustCompile(`^internal/chartgen/`),
		regexp.MustCompile(`^internal/discovery/`),
	}
}

func matchesAny(files []string, patterns []*regexp.Regexp) bool {
	for _, f := range files {
		f = filepath.ToSlash(strings.TrimSpace(f))
		for _, p := range patterns {
			if p.MatchString(f) {
				return true
			}
		}
	}
	return false
}

func changedUnder(files []string, prefix string) bool {
	for _, f := range files {
		if strings.HasPrefix(filepath.ToSlash(strings.TrimSpace(f)), prefix) {
			return true
		}
	}
	return false
}

func containsPath(files []string, path string) bool {
	for _, f := range files {
		if filepath.ToSlash(strings.TrimSpace(f)) == path {
			return true
		}
	}
	return false
}

func normalize(s string) string {
	s = strings.ToLower(s)
	var b strings.Builder
	for _, r := range s {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func sourceTag(ref string) string {
	ref = strings.SplitN(ref, "@", 2)[0]
	if idx := strings.LastIndex(ref, ":"); idx >= 0 && idx > strings.LastIndex(ref, "/") {
		return ref[idx+1:]
	}
	return "latest"
}

func defaultString(v, fallback string) string {
	if strings.TrimSpace(v) == "" {
		return fallback
	}
	return v
}

func Marshal(plan Plan) ([]byte, error) {
	if plan.Matrix.Include == nil {
		plan.Matrix.Include = []map[string]string{}
	}
	if plan.SmokeMatrix.Include == nil && plan.Kind == "integer-pr" {
		plan.SmokeMatrix.Include = []map[string]string{}
	}
	return json.MarshalIndent(plan, "", "  ")
}
