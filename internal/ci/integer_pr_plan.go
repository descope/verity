package ci

import (
	"fmt"
	"os"

	"github.com/verity-org/verity/internal/integer/apkindex"
	intconfig "github.com/verity-org/verity/internal/integer/config"
	intdiscovery "github.com/verity-org/verity/internal/integer/discovery"
)

type integerPRChanges struct {
	definitions    map[string]struct{}
	impact         integerInputImpact
	allImages      bool
	pinningTooling bool
}

type integerPRSelection struct {
	imageNames map[string]struct{}
	variants   map[integerVariant]struct{}
}

var apkindexFetch = apkindex.Fetch

func PlanIntegerPR(opts *IntegerPROptions) (Plan, error) {
	plan := Plan{Kind: "integer-pr"}
	changes, err := loadIntegerPRChanges(opts)
	if err != nil {
		return plan, err
	}
	if changes.empty() {
		return emptyIntegerPRPlan(), nil
	}

	images, err := discoverIntegerPRImages(opts)
	if err != nil {
		return plan, err
	}
	selection, err := resolveIntegerPRSelection(opts, images, changes)
	if err != nil {
		return plan, err
	}
	builds, smokes := selectIntegerPRImages(images, selection.imageNames, selection.variants, changes.allImages)
	if len(smokes) == 0 {
		return emptyIntegerPRPlan(), nil
	}

	plan.HasChanges = true
	plan.Matrix = integerMatrix(builds)
	smokeMatrix := integerMatrix(smokes)
	plan.SmokeMatrix = &smokeMatrix
	return plan, nil
}

func loadIntegerPRChanges(opts *IntegerPROptions) (integerPRChanges, error) {
	definitions, allImages := changedIntegerDefinitions(opts.ChangedFiles)
	changes := integerPRChanges{
		definitions:    definitions,
		impact:         newIntegerInputImpact(),
		allImages:      allImages,
		pinningTooling: integerPinningToolingChanged(opts.ChangedFiles),
	}
	if allImages {
		return changes, nil
	}

	impact, err := changedIntegerInputImpact(integerImpactOptions{
		ChangedFiles: opts.ChangedFiles,
		RepoRoot:     defaultString(opts.RepoRoot, "."),
		BaseLockPath: opts.BaseLockPath,
	})
	if err != nil {
		return changes, fmt.Errorf("find changed bespoke inputs: %w", err)
	}
	changes.impact = impact
	return changes, nil
}

func (c integerPRChanges) empty() bool {
	return !c.allImages && len(c.definitions) == 0 && c.impact.empty() && !c.pinningTooling
}

func discoverIntegerPRImages(opts *IntegerPROptions) ([]intdiscovery.DiscoveredImage, error) {
	cfg, err := intconfig.LoadConfig(defaultString(opts.ConfigPath, "integer.yaml"))
	if err != nil {
		return nil, fmt.Errorf("load integer config: %w", err)
	}

	var packages []apkindex.Package
	if opts.APKIndexURL != "" {
		packages, err = apkindexFetch(opts.APKIndexURL, defaultString(opts.CacheDir, os.TempDir()), apkindex.DefaultCacheMaxAge)
		if err != nil {
			return nil, fmt.Errorf("fetch APKINDEX: %w", err)
		}
	}

	images, err := intdiscovery.DiscoverFromFiles(intdiscovery.Options{
		ImagesDir: defaultString(opts.ImagesDir, "images"),
		Registry:  cfg.Target.Registry,
		Packages:  packages,
		GenDir:    opts.GenDir,
	})
	if err != nil {
		return nil, fmt.Errorf("discover integer images: %w", err)
	}
	return images, nil
}

func resolveIntegerPRSelection(opts *IntegerPROptions, images []intdiscovery.DiscoveredImage, changes integerPRChanges) (integerPRSelection, error) {
	imagesDir := defaultString(opts.ImagesDir, "images")
	imageNames, err := changedIntegerImageNames(imagesDir, images, changes.definitions)
	if err != nil {
		return integerPRSelection{}, fmt.Errorf("resolve changed image definitions: %w", err)
	}
	selection := integerPRSelection{
		imageNames: imageNames,
		variants:   map[integerVariant]struct{}{},
	}
	if changes.allImages {
		return selection, nil
	}
	if !changes.impact.empty() {
		selection.variants, err = integerImpactVariants(imagesDir, images, changes.impact)
		if err != nil {
			return integerPRSelection{}, fmt.Errorf("resolve affected bespoke variants: %w", err)
		}
	}
	if changes.pinningTooling {
		canaries, err := integerConstrainedMelangeVariants(imagesDir, images)
		if err != nil {
			return integerPRSelection{}, fmt.Errorf("resolve package-pinning canaries: %w", err)
		}
		for variant := range canaries {
			selection.variants[variant] = struct{}{}
		}
	}
	return selection, nil
}

func emptyIntegerPRPlan() Plan {
	return Plan{Kind: "integer-pr", Matrix: Matrix{}, SmokeMatrix: &Matrix{}}
}
