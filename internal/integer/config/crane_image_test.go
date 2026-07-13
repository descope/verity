package config

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestCraneBespokeRecipeMatchesDeclaredImageVersion(t *testing.T) {
	// Given: the crane image and its directly referenced bespoke recipe.
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("locating test file")
	}
	repoRoot := filepath.Clean(filepath.Join(filepath.Dir(file), "..", "..", ".."))
	def, err := LoadImage(filepath.Join(repoRoot, "images", "crane.yaml"))
	if err != nil {
		t.Fatalf("load crane image: %v", err)
	}
	recipeData, err := os.ReadFile(filepath.Join(repoRoot, "packages", "bespoke", "crane.yaml"))
	if err != nil {
		t.Fatalf("read crane recipe: %v", err)
	}
	var recipe struct {
		Package struct {
			Version string `yaml:"version"`
		} `yaml:"package"`
	}
	if err := yaml.Unmarshal(recipeData, &recipe); err != nil {
		t.Fatalf("parse crane recipe: %v", err)
	}

	// When: the default image version is compared with the package it builds.
	_, declared := def.Versions[recipe.Package.Version]

	// Then: the image tag cannot claim a different version from its binary.
	if !declared {
		t.Fatalf("crane recipe version %q is absent from declared image versions %v", recipe.Package.Version, def.Versions)
	}
}
