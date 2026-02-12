package internal

import (
	"os"
	"path/filepath"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestGenerateValuesOverride(t *testing.T) {
	results := []*PatchResult{
		{
			Original: Image{
				Registry:   "docker.io",
				Repository: "prom/alertmanager",
				Tag:        "v0.26.0",
				Path:       "alertmanager.image",
			},
			Patched: Image{
				Registry:   "ghcr.io/descope",
				Repository: "prom/alertmanager",
				Tag:        "v0.26.0-patched",
			},
			VulnCount: 3,
		},
		{
			Original: Image{
				Repository: "quay.io/prometheus/prometheus",
				Tag:        "v2.48.0",
				Path:       "server.image",
			},
			Patched: Image{
				Repository: "quay.io/prometheus/prometheus",
				Tag:        "v2.48.0-patched",
			},
			VulnCount: 5,
		},
		{
			// Skipped — should not appear in override.
			Original: Image{
				Repository: "quay.io/prometheus/node-exporter",
				Tag:        "v1.7.0",
				Path:       "nodeExporter.image",
			},
			Patched: Image{
				Repository: "quay.io/prometheus/node-exporter",
				Tag:        "v1.7.0",
			},
			Skipped: true,
		},
		{
			// Error — should not appear in override.
			Original: Image{
				Repository: "some/broken",
				Tag:        "latest",
				Path:       "broken.image",
			},
			Error: os.ErrNotExist,
		},
	}

	dir := t.TempDir()
	outFile := filepath.Join(dir, "patched-values.yaml")

	if err := GenerateValuesOverride(results, outFile); err != nil {
		t.Fatalf("GenerateValuesOverride() error: %v", err)
	}

	data, err := os.ReadFile(outFile)
	if err != nil {
		t.Fatalf("reading output: %v", err)
	}

	var got map[string]interface{}
	if err := yaml.Unmarshal(data, &got); err != nil {
		t.Fatalf("parsing output YAML: %v", err)
	}

	// Check alertmanager.image has patched values.
	am, ok := got["alertmanager"].(map[string]interface{})
	if !ok {
		t.Fatal("missing alertmanager key")
	}
	amImg, ok := am["image"].(map[string]interface{})
	if !ok {
		t.Fatal("missing alertmanager.image key")
	}
	if amImg["registry"] != "ghcr.io/descope" {
		t.Errorf("alertmanager.image.registry = %v, want ghcr.io/descope", amImg["registry"])
	}
	if amImg["repository"] != "prom/alertmanager" {
		t.Errorf("alertmanager.image.repository = %v, want prom/alertmanager", amImg["repository"])
	}
	if amImg["tag"] != "v0.26.0-patched" {
		t.Errorf("alertmanager.image.tag = %v, want v0.26.0-patched", amImg["tag"])
	}

	// Check server.image has patched values.
	srv, ok := got["server"].(map[string]interface{})
	if !ok {
		t.Fatal("missing server key")
	}
	srvImg, ok := srv["image"].(map[string]interface{})
	if !ok {
		t.Fatal("missing server.image key")
	}
	if srvImg["repository"] != "quay.io/prometheus/prometheus" {
		t.Errorf("server.image.repository = %v, want quay.io/prometheus/prometheus", srvImg["repository"])
	}
	if srvImg["tag"] != "v2.48.0-patched" {
		t.Errorf("server.image.tag = %v, want v2.48.0-patched", srvImg["tag"])
	}
	// No registry set for this image.
	if _, hasReg := srvImg["registry"]; hasReg {
		t.Error("server.image should not have registry key")
	}

	// Skipped and errored images should not appear.
	if _, ok := got["nodeExporter"]; ok {
		t.Error("nodeExporter should not appear in override (was skipped)")
	}
	if _, ok := got["broken"]; ok {
		t.Error("broken should not appear in override (had error)")
	}
}

func TestGenerateValuesOverride_AllSkipped(t *testing.T) {
	results := []*PatchResult{
		{
			Original: Image{Repository: "nginx", Tag: "1.25", Path: "image"},
			Patched:  Image{Repository: "nginx", Tag: "1.25"},
			Skipped:  true,
		},
	}

	dir := t.TempDir()
	outFile := filepath.Join(dir, "patched-values.yaml")

	if err := GenerateValuesOverride(results, outFile); err != nil {
		t.Fatalf("GenerateValuesOverride() error: %v", err)
	}

	// When all are skipped, no file should be written.
	if _, err := os.Stat(outFile); !os.IsNotExist(err) {
		t.Error("expected no file to be written when all results are skipped")
	}
}

func TestCountFixable(t *testing.T) {
	report := `{
		"Results": [
			{
				"Vulnerabilities": [
					{"FixedVersion": "1.2.3"},
					{"FixedVersion": ""},
					{"FixedVersion": "4.5.6"}
				]
			},
			{
				"Vulnerabilities": [
					{"FixedVersion": "7.8.9"}
				]
			}
		]
	}`

	dir := t.TempDir()
	path := filepath.Join(dir, "report.json")
	if err := os.WriteFile(path, []byte(report), 0o644); err != nil {
		t.Fatal(err)
	}

	count, err := countFixable(path)
	if err != nil {
		t.Fatalf("countFixable() error: %v", err)
	}
	if count != 3 {
		t.Errorf("countFixable() = %d, want 3", count)
	}
}
