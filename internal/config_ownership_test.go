package internal_test

import (
	"os"
	"path/filepath"
	"sort"
	"testing"

	"gopkg.in/yaml.v3"
)

type imageName struct {
	Name string `yaml:"name"`
}

type copaImageConfig struct {
	Images []imageName `yaml:"images"`
}

func TestIntegerImagesDoNotOverlapCopa(t *testing.T) {
	copaPath := filepath.Join("..", "copa-config.yaml")
	copaData, err := os.ReadFile(copaPath)
	if err != nil {
		t.Fatalf("read %s: %v", copaPath, err)
	}

	var copa copaImageConfig
	if err := yaml.Unmarshal(copaData, &copa); err != nil {
		t.Fatalf("parse %s: %v", copaPath, err)
	}

	copaNames := make(map[string]struct{}, len(copa.Images))
	for _, image := range copa.Images {
		copaNames[image.Name] = struct{}{}
	}

	integerPaths, err := filepath.Glob(filepath.Join("..", "images", "*.yaml"))
	if err != nil {
		t.Fatalf("glob Integer image definitions: %v", err)
	}

	var overlaps []string
	for _, imagePath := range integerPaths {
		imageData, err := os.ReadFile(imagePath)
		if err != nil {
			t.Fatalf("read %s: %v", imagePath, err)
		}

		var image imageName
		if err := yaml.Unmarshal(imageData, &image); err != nil {
			t.Fatalf("parse %s: %v", imagePath, err)
		}
		if _, exists := copaNames[image.Name]; exists {
			overlaps = append(overlaps, image.Name)
		}
	}

	sort.Strings(overlaps)
	if len(overlaps) > 0 {
		t.Fatalf("image names owned by both Copa and Integer: %v", overlaps)
	}
}
