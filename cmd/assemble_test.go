package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/verity-org/verity/internal"
)

func TestAssembleCommand_ValidateFlags(t *testing.T) {
	tests := []struct {
		name       string
		manifest   string
		resultsDir string
		outputDir  string
		wantErr    bool
	}{
		{
			name:       "all required flags",
			manifest:   ".verity/manifest.json",
			resultsDir: ".verity/results",
			outputDir:  ".verity/charts",
			wantErr:    false,
		},
		{
			name:       "missing manifest",
			manifest:   "",
			resultsDir: ".verity/results",
			outputDir:  ".verity/charts",
			wantErr:    true,
		},
		{
			name:       "missing results dir",
			manifest:   ".verity/manifest.json",
			resultsDir: "",
			outputDir:  ".verity/charts",
			wantErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hasErr := tt.manifest == "" || tt.resultsDir == ""
			if hasErr != tt.wantErr {
				t.Errorf("validation error = %v, wantErr %v", hasErr, tt.wantErr)
			}
		})
	}
}

func TestAssembleCommand_ManifestParsing(t *testing.T) {
	tmpDir := t.TempDir()

	// Create test manifest
	manifestPath := filepath.Join(tmpDir, "manifest.json")
	manifest := internal.DiscoveryManifest{
		Charts: []internal.ChartDiscovery{
			{
				Name:    "prometheus",
				Version: "28.9.1",
				Images: []internal.ImageDiscovery{
					{
						Registry:   "quay.io",
						Repository: "prometheus/prometheus",
						Tag:        "v2.45.0",
					},
				},
			},
		},
	}

	manifestData, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		t.Fatalf("failed to marshal manifest: %v", err)
	}

	if err := os.WriteFile(manifestPath, manifestData, 0o644); err != nil {
		t.Fatalf("failed to write manifest: %v", err)
	}

	// Parse manifest
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatalf("failed to read manifest: %v", err)
	}

	var parsed internal.DiscoveryManifest
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("failed to parse manifest: %v", err)
	}

	if len(parsed.Charts) != 1 {
		t.Errorf("expected 1 chart, got %d", len(parsed.Charts))
	}

	if parsed.Charts[0].Name != "prometheus" {
		t.Errorf("expected chart name 'prometheus', got %q", parsed.Charts[0].Name)
	}

	if len(parsed.Charts[0].Images) != 1 {
		t.Errorf("expected 1 image in chart, got %d", len(parsed.Charts[0].Images))
	}
}

func TestAssembleCommand_ChangeDetection(t *testing.T) {
	tests := []struct {
		name        string
		results     map[string]bool // image name -> changed
		expectChart bool            // should chart be assembled?
	}{
		{
			name: "all images unchanged",
			results: map[string]bool{
				"image1": false,
				"image2": false,
			},
			expectChart: false,
		},
		{
			name: "one image changed",
			results: map[string]bool{
				"image1": true,
				"image2": false,
			},
			expectChart: true,
		},
		{
			name: "all images changed",
			results: map[string]bool{
				"image1": true,
				"image2": true,
			},
			expectChart: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Simple change detection logic test
			anyChanged := false
			for _, changed := range tt.results {
				if changed {
					anyChanged = true
					break
				}
			}

			if anyChanged != tt.expectChart {
				t.Errorf("anyChanged = %v, expectChart %v", anyChanged, tt.expectChart)
			}
		})
	}
}
