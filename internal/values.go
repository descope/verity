package internal

import (
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

// GenerateValuesOverride builds a Helm values override file that remaps
// original image references to their patched equivalents.
//
// Each PatchResult.Original.Path (e.g. "server.image") determines where in
// the nested YAML the image fields are set.
func GenerateValuesOverride(results []*PatchResult, path string) error {
	root := make(map[string]interface{})

	for _, r := range results {
		if r.Error != nil || r.Skipped {
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
