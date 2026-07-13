package ci

import (
	"encoding/json"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"strings"

	"github.com/verity-org/verity/internal/integer/apkindex"
	intconfig "github.com/verity-org/verity/internal/integer/config"
	intdiscovery "github.com/verity-org/verity/internal/integer/discovery"
	"github.com/verity-org/verity/internal/integer/melange"
)

type integerImpactOptions struct {
	ChangedFiles []string
	RepoRoot     string
	BaseLockPath string
}

type integerVariant struct {
	image     string
	version   string
	imageType string
}

type integerBuildLock struct {
	Packages      map[string]integerBuildLockPackage `json:"packages"`
	PipelineFiles map[string]string                  `json:"pipeline_files"`
}

type integerBuildLockPackage struct {
	File   string            `json:"file"`
	SHA256 string            `json:"sha256"`
	Assets map[string]string `json:"assets"`
}

type integerInputImpact struct {
	upstream  map[string]struct{}
	bespoke   map[string]struct{}
	overrides map[string]struct{}
	pipelines map[string]struct{}
}

func changedIntegerInputImpact(opts integerImpactOptions) (integerInputImpact, error) {
	impact := newIntegerInputImpact()
	paths := melange.DefaultPaths(opts.RepoRoot)
	head, err := collectIntegerInputImpact(&impact, &paths, opts)
	if err != nil {
		return impact, err
	}
	if len(impact.pipelines) > 0 {
		if err := addPipelineRecipeImpact(&impact, &paths, head); err != nil {
			return impact, err
		}
	}
	return impact, nil
}

func addLockDiffImpact(impact *integerInputImpact, base, head integerBuildLock) {
	for key := range unionKeys(base.Packages, head.Packages) {
		if !sameBuildLockPackage(base.Packages[key], head.Packages[key]) {
			impact.upstream[key] = struct{}{}
		}
	}
	for file := range unionKeys(base.PipelineFiles, head.PipelineFiles) {
		if base.PipelineFiles[file] != head.PipelineFiles[file] && filepath.Ext(file) == ".yaml" {
			impact.pipelines[strings.TrimSuffix(filepath.ToSlash(file), ".yaml")] = struct{}{}
		}
	}
}

func sameBuildLockPackage(left, right integerBuildLockPackage) bool {
	return left.File == right.File && left.SHA256 == right.SHA256 && maps.Equal(left.Assets, right.Assets)
}

func addLockedPathImpact(keys map[string]struct{}, changed string, lock integerBuildLock) bool {
	matched := false
	for key, entry := range lock.Packages {
		stem := strings.TrimSuffix(filepath.ToSlash(entry.File), filepath.Ext(entry.File))
		if changed == filepath.ToSlash(entry.File) || strings.HasPrefix(changed, stem+"/") {
			keys[key] = struct{}{}
			matched = true
			continue
		}
		if _, ok := entry.Assets[changed]; ok {
			keys[key] = struct{}{}
			matched = true
		}
	}
	return matched
}

func integerImpactVariants(imagesDir string, imgs []intdiscovery.DiscoveredImage, impact integerInputImpact) (map[integerVariant]struct{}, error) {
	definitions := map[string]*intconfig.ImageDef{}
	variants := map[integerVariant]struct{}{}
	for _, img := range imgs {
		definitionFile := img.DefinitionFile
		if definitionFile == "" {
			definitionFile = filepath.Join(imagesDir, filepath.FromSlash(img.Name)+".yaml")
		}
		def, ok := definitions[definitionFile]
		if !ok {
			var err error
			def, err = intconfig.LoadImage(definitionFile)
			if err != nil {
				return nil, fmt.Errorf("load %s: %w", definitionFile, err)
			}
			definitions[definitionFile] = def
		}
		configSpec := def.Types[img.Type].Melange
		if configSpec == nil {
			continue
		}
		spec, err := melange.ResolveConfigSpec(configSpec, img.Version)
		if err != nil {
			return nil, fmt.Errorf("resolve %s:%s-%s: %w", img.Name, img.Version, img.Type, err)
		}
		if specConsumesImpact(spec, impact) {
			variants[variantForImage(&img)] = struct{}{}
		}
	}
	return variants, nil
}

func integerPinningToolingChanged(files []string) bool {
	for _, file := range files {
		file = filepath.ToSlash(strings.TrimSpace(file))
		switch {
		case strings.HasPrefix(file, "cmd/integer_build_melange"),
			strings.HasPrefix(file, "cmd/integer_melange"),
			strings.HasPrefix(file, "internal/ci/"),
			strings.HasPrefix(file, "internal/integer/apkindex/package_spec"),
			strings.HasPrefix(file, "internal/integer/melange/"),
			file == ".github/workflows/integer-build-image.yaml",
			file == ".github/workflows/pr-test.yaml":
			return true
		}
	}
	return false
}

func integerConstrainedMelangeVariants(imagesDir string, imgs []intdiscovery.DiscoveredImage) (map[integerVariant]struct{}, error) {
	definitions := map[string]*intconfig.ImageDef{}
	variants := map[integerVariant]struct{}{}
	for _, img := range imgs {
		definitionFile := img.DefinitionFile
		if definitionFile == "" {
			definitionFile = filepath.Join(imagesDir, filepath.FromSlash(img.Name)+".yaml")
		}
		def, ok := definitions[definitionFile]
		if !ok {
			var err error
			def, err = intconfig.LoadImage(definitionFile)
			if err != nil {
				return nil, fmt.Errorf("load %s: %w", definitionFile, err)
			}
			definitions[definitionFile] = def
		}
		template := def.Types[img.Type]
		if template.Melange == nil {
			continue
		}
		for _, packageSpec := range template.Packages {
			if apkindex.PackageName(packageSpec) != packageSpec {
				variants[variantForImage(&img)] = struct{}{}
				break
			}
		}
	}
	return variants, nil
}

func specConsumesImpact(spec melange.Spec, impact integerInputImpact) bool {
	if _, ok := impact.upstream[spec.Upstream]; ok {
		return true
	}
	if _, ok := impact.overrides[spec.EnvFile]; ok {
		return true
	}
	for _, file := range spec.Bespoke {
		if _, ok := impact.bespoke[file]; ok {
			return true
		}
	}
	return false
}

func loadIntegerBuildLock(path string) (integerBuildLock, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return integerBuildLock{}, fmt.Errorf("read bespoke lock: %w", err)
	}
	var lock integerBuildLock
	if err := json.Unmarshal(data, &lock); err != nil {
		return integerBuildLock{}, fmt.Errorf("parse bespoke lock: %w", err)
	}
	if lock.Packages == nil {
		lock.Packages = map[string]integerBuildLockPackage{}
	}
	if lock.PipelineFiles == nil {
		lock.PipelineFiles = map[string]string{}
	}
	return lock, nil
}

func newIntegerInputImpact() integerInputImpact {
	return integerInputImpact{
		upstream:  map[string]struct{}{},
		bespoke:   map[string]struct{}{},
		overrides: map[string]struct{}{},
		pipelines: map[string]struct{}{},
	}
}

func (i integerInputImpact) empty() bool {
	return len(i.upstream)+len(i.bespoke)+len(i.overrides) == 0
}

func variantForImage(img *intdiscovery.DiscoveredImage) integerVariant {
	return integerVariant{image: img.Name, version: img.Version, imageType: img.Type}
}

func unionKeys[V any](left, right map[string]V) map[string]struct{} {
	keys := make(map[string]struct{}, len(left)+len(right))
	for key := range left {
		keys[key] = struct{}{}
	}
	for key := range right {
		keys[key] = struct{}{}
	}
	return keys
}
