package chartgen

import (
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/verity-org/verity/internal/config"
	"github.com/verity-org/verity/internal/discovery"
)

type Config struct {
	ChartsFile     string
	VerityConfig   string
	TargetRegistry string
	ChartRegistry  string
	ExcludeNames   map[string]struct{}
	DryRun         bool
}

type ChartResult struct {
	Name           string          `json:"name"`
	Version        string          `json:"version"`
	WrapperName    string          `json:"wrapperName"`
	WrapperVersion string          `json:"wrapperVersion"`
	Repository     string          `json:"repository"`
	Registry       string          `json:"registry"`
	ImageMappings  []ImageMapping  `json:"imageMappings"`
	ValueOverrides []ValueOverride `json:"valueOverrides"`
}

type DryRunResult struct {
	GeneratedAt   string        `json:"generatedAt"`
	ChartRegistry string        `json:"chartRegistry"`
	Charts        []ChartResult `json:"charts"`
}

func Run(cfg *Config) (*DryRunResult, error) {
	charts, err := discovery.LoadChartsFile(cfg.ChartsFile)
	if err != nil {
		return nil, fmt.Errorf("load charts file: %w", err)
	}

	vc, err := discovery.LoadVerityConfig(cfg.VerityConfig)
	if err != nil {
		return nil, fmt.Errorf("load verity config: %w", err)
	}

	result := &DryRunResult{
		GeneratedAt:   time.Now().UTC().Format(time.RFC3339),
		ChartRegistry: cfg.ChartRegistry,
		Charts:        make([]ChartResult, 0, len(charts)),
	}

	for _, chart := range charts {
		chartResult, include, err := processChart(cfg, chart, vc)
		if err != nil {
			return nil, err
		}
		if include {
			result.Charts = append(result.Charts, chartResult)
		}
	}

	return result, nil
}

func processChart(cfg *Config, chart config.ChartSpec, vc *config.VerityConfig) (ChartResult, bool, error) {
	fmt.Fprintf(os.Stderr, "info: processing chart %s@%s\n", chart.Name, chart.Version)

	imageRefs, err := discovery.ExtractChartImages(chart, vc.Overrides)
	if err != nil {
		return ChartResult{}, false, fmt.Errorf("extract images for chart %s: %w", chart.Name, err)
	}

	remainingRefs, replacementMappings := applyReplacements(imageRefs, vc, cfg.ExcludeNames)

	mappings, err := BuildImageMappings(remainingRefs, cfg.TargetRegistry, cfg.ExcludeNames)
	if err != nil {
		return ChartResult{}, false, fmt.Errorf("build image mappings for chart %s: %w", chart.Name, err)
	}
	allMappings := make([]ImageMapping, 0, len(replacementMappings)+len(mappings))
	allMappings = append(allMappings, replacementMappings...)
	allMappings = append(allMappings, mappings...)

	if len(allMappings) == 0 {
		fmt.Fprintf(os.Stderr, "warning: no patched image mappings for chart %s@%s; skipping\n", chart.Name, chart.Version)
		return ChartResult{}, false, nil
	}

	valuesYAML, err := GetChartValues(chart)
	if err != nil {
		return ChartResult{}, false, fmt.Errorf("get chart values for %s: %w", chart.Name, err)
	}

	valueOverrides, err := ResolveValuePaths(valuesYAML, allMappings, vc.Overrides)
	if err != nil {
		return ChartResult{}, false, fmt.Errorf("resolve value paths for %s: %w", chart.Name, err)
	}

	wrapper, err := BuildWrapperChart(chart, valueOverrides)
	if err != nil {
		return ChartResult{}, false, fmt.Errorf("build wrapper chart for %s: %w", chart.Name, err)
	}

	chartResult := ChartResult{
		Name:           chart.Name,
		Version:        chart.Version,
		WrapperName:    wrapper.Name,
		WrapperVersion: wrapper.Version,
		Repository:     chart.Repository,
		Registry:       cfg.ChartRegistry,
		ImageMappings:  allMappings,
		ValueOverrides: valueOverrides,
	}

	if cfg.DryRun {
		fmt.Fprintf(os.Stderr, "info: dry-run enabled; skipping package/push for %s\n", wrapper.Name)
		return chartResult, true, nil
	}

	tgzPath, err := PackageChart(wrapper)
	if err != nil {
		return ChartResult{}, false, fmt.Errorf("package wrapper chart %s: %w", wrapper.Name, err)
	}

	if err := PushChart(tgzPath, cfg.ChartRegistry); err != nil {
		_ = os.Remove(tgzPath)
		return ChartResult{}, false, fmt.Errorf("push wrapper chart %s: %w", wrapper.Name, err)
	}

	if err := os.Remove(tgzPath); err != nil {
		fmt.Fprintf(os.Stderr, "warning: failed to remove packaged chart %s: %v\n", tgzPath, err)
	}

	return chartResult, true, nil
}

func applyReplacements(imageRefs []string, vc *config.VerityConfig, excludeNames map[string]struct{}) ([]string, []ImageMapping) {
	if vc == nil || len(vc.Replacements) == 0 {
		return imageRefs, nil
	}

	// Sort patterns longest-first so a more-specific pattern wins over a
	// shorter pattern whose text is contained within it. The matcher below
	// uses strings.Contains, so e.g. when both "kyverno/kyverno" and
	// "kyverno/kyverno-cli" are configured, the cli pattern must be tried
	// first or the bare "kyverno/kyverno" pattern would greedily claim the
	// cli image (and the kyvernopre image too — which IS the desired
	// behavior for kyvernopre, since it shares the kyverno binary).
	// Ties broken alphabetically for deterministic order.
	patterns := make([]string, 0, len(vc.Replacements))
	for p := range vc.Replacements {
		patterns = append(patterns, p)
	}
	sort.Slice(patterns, func(i, j int) bool {
		if len(patterns[i]) != len(patterns[j]) {
			return len(patterns[i]) > len(patterns[j])
		}
		return patterns[i] < patterns[j]
	})

	remaining := make([]string, 0, len(imageRefs))
	var replacements []ImageMapping

	for _, imageRef := range imageRefs {
		name := repoPath(imageRef)
		sourceRepo, sourceTag := splitRef(imageRef)

		if isExcluded(name, imageRef, excludeNames) {
			fmt.Fprintf(os.Stderr, "warning: skipping excluded image %q (%s)\n", name, imageRef)
			continue
		}

		matched := false
		for _, pattern := range patterns {
			if name != pattern && !strings.Contains(name, pattern) {
				continue
			}
			repl := vc.Replacements[pattern]
			patchedRepo := repl.Registry + "/" + repl.Image
			patchedTag := sourceTag
			if repl.Tag != "" {
				patchedTag = repl.Tag
			}
			replacements = append(replacements, ImageMapping{
				OriginalRepo: sourceRepo,
				OriginalTag:  sourceTag,
				PatchedRepo:  patchedRepo,
				PatchedTag:   patchedTag,
			})
			fmt.Fprintf(os.Stderr, "info: replacing %q with Integer image %s:%s\n", imageRef, patchedRepo, patchedTag)
			matched = true
			break
		}
		if !matched {
			remaining = append(remaining, imageRef)
		}
	}

	return remaining, replacements
}
