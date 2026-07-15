package ci

import (
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"reflect"
	"strings"

	intconfig "github.com/verity-org/verity/internal/integer/config"
	intdiscovery "github.com/verity-org/verity/internal/integer/discovery"
)

func changedIntegerDefinitions(files []string, imagesDir, baseImagesDir string) (definitions map[string]struct{}, variants integerVariantImpacts, all bool, err error) {
	definitions = map[string]struct{}{}
	variants = integerVariantImpacts{}
	for _, file := range files {
		file = filepath.ToSlash(strings.TrimSpace(file))
		switch {
		case file == "integer.yaml", strings.HasPrefix(file, "images/_base/"):
			all = true
		default:
			if _, ok := imageNameFromPath(file); ok {
				definitions[strings.TrimPrefix(file, "images/")] = struct{}{}
			}
		}
	}
	if all || strings.TrimSpace(baseImagesDir) == "" {
		return definitions, variants, all, nil
	}

	for relative := range definitions {
		headPath := filepath.Join(imagesDir, filepath.FromSlash(relative))
		basePath := filepath.Join(baseImagesDir, filepath.FromSlash(relative))
		if !fileExists(headPath) || !fileExists(basePath) {
			continue
		}
		head, loadErr := intconfig.LoadImage(headPath)
		if loadErr != nil {
			return nil, nil, false, fmt.Errorf("load head definition %s: %w", headPath, loadErr)
		}
		base, loadErr := intconfig.LoadImage(basePath)
		if loadErr != nil {
			return nil, nil, false, fmt.Errorf("load base definition %s: %w", basePath, loadErr)
		}
		if !reflect.DeepEqual(withoutScopedMelange(base), withoutScopedMelange(head)) {
			continue
		}

		delete(definitions, relative)
		maps.Copy(variants, changedScopedMelangeVariants(base, head))
	}
	return definitions, variants, false, nil
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func withoutScopedMelange(def *intconfig.ImageDef) intconfig.ImageDef {
	normalized := *def
	normalized.Versions = make(map[string]intconfig.VersionMeta, len(def.Versions))
	for version, meta := range def.Versions {
		meta.Melange = nil
		normalized.Versions[version] = meta
	}
	return normalized
}

func changedScopedMelangeVariants(base, head *intconfig.ImageDef) integerVariantImpacts {
	variants := integerVariantImpacts{}
	for version := range unionKeys(base.Versions, head.Versions) {
		baseMeta := base.Versions[version]
		headMeta := head.Versions[version]
		for imageType := range unionKeys(baseMeta.Melange, headMeta.Melange) {
			if !reflect.DeepEqual(baseMeta.Melange[imageType], headMeta.Melange[imageType]) {
				variants[integerVariant{image: head.Name, version: version, imageType: imageType}] = integerImpactVersion
			}
		}
	}
	return variants
}

func changedIntegerImageNames(imagesDir string, imgs []intdiscovery.DiscoveredImage, definitions map[string]struct{}) (map[string]struct{}, error) {
	names := map[string]struct{}{}
	for _, img := range imgs {
		definitionFile := img.DefinitionFile
		if definitionFile == "" {
			definitionFile = filepath.Join(imagesDir, filepath.FromSlash(img.Name)+".yaml")
		}
		relative, err := filepath.Rel(imagesDir, definitionFile)
		if err != nil {
			return nil, fmt.Errorf("make %q relative to %q: %w", definitionFile, imagesDir, err)
		}
		if _, ok := definitions[filepath.ToSlash(relative)]; ok {
			names[img.Name] = struct{}{}
		}
	}
	return names, nil
}
