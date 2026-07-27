package ci

import (
	"crypto/sha256"
	"fmt"
	"path/filepath"
	"slices"
	"sort"
	"strconv"
	"strings"

	intdiscovery "github.com/verity-org/verity/internal/integer/discovery"
)

const integerProductionShardSize = 250

func PlanIntegerProduction(options *IntegerProductionOptions) (IntegerBatchPlan, error) {
	if err := validateIntegerProductionOptions(options); err != nil {
		return IntegerBatchPlan{}, err
	}
	images, err := discoverIntegerPRImages(&IntegerPROptions{
		RepoRoot: options.RepoRoot, ConfigPath: options.ConfigPath, ImagesDir: options.ImagesDir,
		APKIndexURL: options.APKIndexURL, CacheDir: options.CacheDir, GenDir: options.GenDir,
	})
	if err != nil {
		return IntegerBatchPlan{}, fmt.Errorf("discover production Integer targets: %w", err)
	}
	selected, mode, err := selectIntegerProductionImages(options, images)
	if err != nil {
		return IntegerBatchPlan{}, err
	}
	targets, packages, err := planIntegerPackages(options, selected)
	if err != nil {
		return IntegerBatchPlan{}, err
	}
	return IntegerBatchPlan{
		SchemaVersion: IntegerBatchSchemaVersion,
		SourceSHA:     options.SourceSHA,
		RunID:         options.RunID,
		RunAttempt:    options.RunAttempt,
		PublicationID: options.PublicationID,
		BatchID:       options.BatchID,
		Mode:          mode,
		Event:         options.Event,
		Targets:       targets,
		Packages:      packages,
	}, nil
}

func selectIntegerProductionImages(options *IntegerProductionOptions, images []intdiscovery.DiscoveredImage) ([]intdiscovery.DiscoveredImage, IntegerBatchMode, error) {
	switch options.Event {
	case IntegerBatchEventSchedule, IntegerBatchEventWorkflowCall, IntegerBatchEventWorkflowDispatch:
		return filterProductionOnly(images, options.Only), IntegerBatchModeSnapshot, nil
	case IntegerBatchEventPush:
		if integerProductionWideChange(options.ChangedFiles) {
			return filterProductionOnly(images, options.Only), IntegerBatchModeSnapshot, nil
		}
		changes, err := loadIntegerPRChanges(&IntegerPROptions{
			ChangedFiles: options.ChangedFiles, RepoRoot: options.RepoRoot,
			BaseLockPath: options.BaseLockPath, BaseImagesDir: options.BaseImagesDir,
			ImagesDir: options.ImagesDir,
		})
		if err != nil {
			return nil, "", fmt.Errorf("classify production Integer impact: %w", err)
		}
		selection, err := resolveIntegerPRSelection(&IntegerPROptions{ImagesDir: options.ImagesDir}, images, changes)
		if err != nil {
			return nil, "", fmt.Errorf("resolve production Integer impact: %w", err)
		}
		selected := make([]intdiscovery.DiscoveredImage, 0)
		for _, image := range images {
			_, named := selection.imageNames[image.Name]
			_, variant := selection.variants[variantForImage(&image)]
			if changes.allImages || named || variant {
				selected = append(selected, image)
			}
		}
		return filterProductionOnly(selected, options.Only), IntegerBatchModeDelta, nil
	default:
		return nil, "", fmt.Errorf("%w: unsupported event %q", ErrIntegerBatchPlan, options.Event)
	}
}

func integerProductionWideChange(files []string) bool {
	for _, file := range files {
		file = filepath.ToSlash(strings.TrimSpace(file))
		switch {
		case file == "integer.yaml", strings.HasPrefix(file, "images/_base/"),
			strings.HasSuffix(file, ".go"), file == "go.mod", file == "go.sum",
			file == "mise.toml", file == "mise.lock",
			file == ".github/workflows/integer-orchestrator.yaml",
			file == ".github/workflows/integer-build-shard.yaml",
			file == ".github/workflows/integer-build-image.yaml":
			return true
		}
	}
	return false
}

func filterProductionOnly(images []intdiscovery.DiscoveredImage, only []string) []intdiscovery.DiscoveredImage {
	if len(only) == 0 {
		return images
	}
	wanted := make(map[string]struct{}, len(only))
	for _, name := range only {
		wanted[strings.TrimSpace(name)] = struct{}{}
	}
	selected := make([]intdiscovery.DiscoveredImage, 0, len(images))
	for _, image := range images {
		if _, ok := wanted[image.Name]; ok {
			selected = append(selected, image)
		}
	}
	return selected
}

func planIntegerPackages(options *IntegerProductionOptions, images []intdiscovery.DiscoveredImage) ([]IntegerBatchTarget, []IntegerPlannedPackage, error) {
	sort.Slice(images, func(i, j int) bool { return productionImageID(&images[i]) < productionImageID(&images[j]) })
	targets := make([]IntegerBatchTarget, 0, len(images))
	owners := map[string]string{}
	packageDeclarations := map[string]integerRecipeDeclaration{}
	for index := range images {
		image := &images[index]
		packages, declarations, err := integerRecipePackages(options.RepoRoot, options.ImagesDir, image)
		if err != nil {
			return nil, nil, fmt.Errorf("resolve packages for %s: %w", productionImageID(image), err)
		}
		target := IntegerBatchTarget{
			Name: image.Name, Version: image.Version, Type: image.Type, ArtifactKey: integerArtifactKey(image),
			Tags: append([]string(nil), image.Tags...), Registry: image.Registry,
			Shard: strconv.Itoa(index/integerProductionShardSize + 1), ExpectedPackages: packages,
			PublishPackages: []string{},
		}
		for _, name := range packages {
			if err := coalesceIntegerRecipeDeclaration(packageDeclarations, name, declarations[name]); err != nil {
				return nil, nil, err
			}
			if _, exists := owners[name]; !exists {
				owners[name] = target.ID()
				target.PublishPackages = append(target.PublishPackages, name)
			}
		}
		targets = append(targets, target)
	}
	type plannedPackageIdentity struct {
		architecture IntegerArchitecture
		name         string
	}
	plannedByIdentity := make(map[plannedPackageIdentity]IntegerPlannedPackage, len(owners)*2)
	for _, architecture := range []IntegerArchitecture{IntegerArchitectureAArch64, IntegerArchitectureX8664} {
		for name, producer := range owners {
			identity := plannedPackageIdentity{architecture: architecture, name: name}
			plannedByIdentity[identity] = IntegerPlannedPackage{Architecture: architecture, Name: name, Producer: producer}
		}
	}
	identities := make([]plannedPackageIdentity, 0, len(plannedByIdentity))
	for identity := range plannedByIdentity {
		identities = append(identities, identity)
	}
	slices.SortFunc(identities, func(left, right plannedPackageIdentity) int {
		return strings.Compare(string(left.architecture)+"/"+left.name, string(right.architecture)+"/"+right.name)
	})
	planned := make([]IntegerPlannedPackage, 0, len(identities))
	for _, identity := range identities {
		planned = append(planned, plannedByIdentity[identity])
	}
	return targets, planned, nil
}

func productionImageID(image *intdiscovery.DiscoveredImage) string {
	return image.Name + ":" + image.Version + "-" + image.Type
}

func integerArtifactKey(image *intdiscovery.DiscoveredImage) string {
	raw := image.Name + "-" + image.Version + "-" + image.Type
	var prefix strings.Builder
	for _, character := range raw {
		switch {
		case character >= 'a' && character <= 'z', character >= 'A' && character <= 'Z',
			character >= '0' && character <= '9', character == '.', character == '_', character == '-':
			prefix.WriteRune(character)
		default:
			prefix.WriteByte('-')
		}
	}
	value := prefix.String()
	if len(value) > 120 {
		value = value[:120]
	}
	digest := sha256.Sum256([]byte(image.Name + "\x00" + image.Version + "\x00" + image.Type))
	return fmt.Sprintf("%s-%x", value, digest[:6])
}
