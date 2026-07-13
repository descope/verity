package ci

import (
	"fmt"
	"path/filepath"
	"strings"

	intdiscovery "github.com/verity-org/verity/internal/integer/discovery"
)

func changedIntegerDefinitions(files []string) (definitions map[string]struct{}, all bool) {
	definitions = map[string]struct{}{}
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
	return definitions, all
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
