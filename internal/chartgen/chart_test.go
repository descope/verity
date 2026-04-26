package chartgen

import (
	"reflect"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/verity-org/verity/internal/config"
)

func TestBuildWrapperChart(t *testing.T) {
	original := config.ChartSpec{
		Name:       "prometheus",
		Version:    "28.9.1",
		Repository: "oci://ghcr.io/prometheus-community/charts",
	}

	chart, err := BuildWrapperChart(original, nil)
	if err != nil {
		t.Fatalf("BuildWrapperChart() error = %v", err)
	}

	var chartDoc map[string]any
	if err := yaml.Unmarshal(chart.ChartYAML, &chartDoc); err != nil {
		t.Fatalf("yaml.Unmarshal(ChartYAML) error = %v", err)
	}

	if got := chartDoc["apiVersion"]; got != "v2" {
		t.Fatalf("apiVersion = %v, want v2", got)
	}
	if got := chartDoc["name"]; got != "prometheus" {
		t.Fatalf("name = %v, want prometheus", got)
	}
	if got := chartDoc["version"]; got != "28.9.1" {
		t.Fatalf("version = %v, want 28.9.1", got)
	}
	if got := chartDoc["type"]; got != "application" {
		t.Fatalf("type = %v, want application", got)
	}

	deps, ok := chartDoc["dependencies"].([]any)
	if !ok {
		t.Fatalf("dependencies type = %T, want []any", chartDoc["dependencies"])
	}
	if len(deps) != 1 {
		t.Fatalf("dependencies length = %d, want 1", len(deps))
	}
	dep, ok := deps[0].(map[string]any)
	if !ok {
		t.Fatalf("dependencies[0] type = %T, want map[string]any", deps[0])
	}
	if dep["name"] != original.Name || dep["version"] != original.Version || dep["repository"] != original.Repository {
		t.Fatalf("dependency = %#v, want name/version/repository from original", dep)
	}

	annotations, ok := chartDoc["annotations"].(map[string]any)
	if !ok {
		t.Fatalf("annotations type = %T, want map[string]any", chartDoc["annotations"])
	}
	if annotations["verity.supply/source-chart"] != original.Name {
		t.Fatalf("source-chart annotation = %v, want %s", annotations["verity.supply/source-chart"], original.Name)
	}
	if annotations["verity.supply/source-version"] != original.Version {
		t.Fatalf("source-version annotation = %v, want %s", annotations["verity.supply/source-version"], original.Version)
	}
	if annotations["verity.supply/source-repository"] != original.Repository {
		t.Fatalf("source-repository annotation = %v, want %s", annotations["verity.supply/source-repository"], original.Repository)
	}
}

func TestBuildWrapperChartValues(t *testing.T) {
	tests := []struct {
		name      string
		overrides []ValueOverride
		assert    func(t *testing.T, values map[string]any)
	}{
		{
			name: "single image override",
			overrides: []ValueOverride{{
				Path:       "image",
				Repository: "ghcr.io/verity-org/prom",
				Tag:        "v3",
			}},
			assert: func(t *testing.T, values map[string]any) {
				prom, ok := values["prometheus"].(map[string]any)
				if !ok {
					t.Fatalf("prometheus root missing or invalid: %#v", values["prometheus"])
				}
				image, ok := prom["image"].(map[string]any)
				if !ok {
					t.Fatalf("prometheus.image missing or invalid: %#v", prom["image"])
				}
				if image["repository"] != "ghcr.io/verity-org/prom" || image["tag"] != "v3" {
					t.Fatalf("prometheus.image = %#v, want repository/tag override", image)
				}
			},
		},
		{
			name: "nested path",
			overrides: []ValueOverride{{
				Path:       "server.image",
				Repository: "ghcr.io/verity-org/prometheus",
				Tag:        "v3.2.1",
			}},
			assert: func(t *testing.T, values map[string]any) {
				prom, ok := values["prometheus"].(map[string]any)
				if !ok {
					t.Fatal("prometheus root missing")
				}
				server, ok := prom["server"].(map[string]any)
				if !ok {
					t.Fatal("server missing")
				}
				image, ok := server["image"].(map[string]any)
				if !ok {
					t.Fatal("image missing")
				}
				if image["repository"] != "ghcr.io/verity-org/prometheus" || image["tag"] != "v3.2.1" {
					t.Fatalf("prometheus.server.image = %#v, want repository/tag override", image)
				}
			},
		},
		{
			name: "multiple overrides",
			overrides: []ValueOverride{
				{Path: "image", Repository: "ghcr.io/verity-org/prom", Tag: "v3"},
				{Path: "server.image", Repository: "ghcr.io/verity-org/prometheus", Tag: "v3.2.1"},
			},
			assert: func(t *testing.T, values map[string]any) {
				prom, ok := values["prometheus"].(map[string]any)
				if !ok {
					t.Fatal("prometheus root missing")
				}
				img, ok := prom["image"].(map[string]any)
				if !ok {
					t.Fatal("image missing")
				}
				server, ok := prom["server"].(map[string]any)
				if !ok {
					t.Fatal("server missing")
				}
				serverImg, ok := server["image"].(map[string]any)
				if !ok {
					t.Fatal("server.image missing")
				}
				if img["repository"] != "ghcr.io/verity-org/prom" || img["tag"] != "v3" {
					t.Fatalf("prometheus.image = %#v, want override", img)
				}
				if serverImg["repository"] != "ghcr.io/verity-org/prometheus" || serverImg["tag"] != "v3.2.1" {
					t.Fatalf("prometheus.server.image = %#v, want override", serverImg)
				}
			},
		},
	}

	original := config.ChartSpec{Name: "prometheus", Version: "28.9.1", Repository: "oci://repo/charts"}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			chart, err := BuildWrapperChart(original, tt.overrides)
			if err != nil {
				t.Fatalf("BuildWrapperChart() error = %v", err)
			}

			var values map[string]any
			if err := yaml.Unmarshal(chart.ValuesYAML, &values); err != nil {
				t.Fatalf("yaml.Unmarshal(ValuesYAML) error = %v", err)
			}

			tt.assert(t, values)
		})
	}
}

func TestBuildWrapperChartValues_ChartImageOverrideSingleSource(t *testing.T) {
	original := config.ChartSpec{Name: "strimzi-kafka-operator", Version: "0.51.0", Repository: "oci://repo/charts"}
	vc := &config.VerityConfig{
		ChartImageOverrides: map[string][]config.ChartImageOverride{
			"strimzi-kafka-operator": {
				{
					Source: "STRIMZI_DEFAULT_KAFKA_EXPORTER_IMAGE",
					Path:   "kafka.image",
				},
			},
		},
	}

	overrides, err := buildChartImageOverrides(original.Name, []ImageMapping{{
		Source:      "STRIMZI_DEFAULT_KAFKA_EXPORTER_IMAGE",
		PatchedRepo: "ghcr.io/verity-org/kafka",
		PatchedTag:  "patched-tag",
	}}, vc)
	if err != nil {
		t.Fatalf("buildChartImageOverrides() error = %v", err)
	}

	chart, err := BuildWrapperChart(original, overrides)
	if err != nil {
		t.Fatalf("BuildWrapperChart() error = %v", err)
	}

	var values map[string]any
	if err := yaml.Unmarshal(chart.ValuesYAML, &values); err != nil {
		t.Fatalf("yaml.Unmarshal(ValuesYAML) error = %v", err)
	}

	root, ok := values["strimzi-kafka-operator"].(map[string]any)
	if !ok {
		t.Fatalf("chart root missing or invalid: %#v", values["strimzi-kafka-operator"])
	}
	kafka, ok := root["kafka"].(map[string]any)
	if !ok {
		t.Fatalf("kafka node missing or invalid: %#v", root["kafka"])
	}
	if kafka["image"] != "ghcr.io/verity-org/kafka:patched-tag" {
		t.Fatalf("kafka.image = %#v, want patched image string", kafka["image"])
	}
}

func TestBuildWrapperChartEmptyOverrides(t *testing.T) {
	original := config.ChartSpec{Name: "prometheus", Version: "28.9.1", Repository: "oci://repo/charts"}

	chart, err := BuildWrapperChart(original, []ValueOverride{})
	if err != nil {
		t.Fatalf("BuildWrapperChart() error = %v", err)
	}

	var values map[string]any
	if err := yaml.Unmarshal(chart.ValuesYAML, &values); err != nil {
		t.Fatalf("yaml.Unmarshal(ValuesYAML) error = %v", err)
	}
	if len(values) != 0 {
		t.Fatalf("values map length = %d, want 0", len(values))
	}
}

func TestBuildWrapperChartEmptyName(t *testing.T) {
	_, err := BuildWrapperChart(config.ChartSpec{Version: "1.0.0", Repository: "oci://repo/charts"}, nil)
	if err == nil {
		t.Fatal("BuildWrapperChart() error = nil, want non-nil")
	}
}

func TestBuildValuesTree(t *testing.T) {
	tests := []struct {
		name      string
		overrides []ValueOverride
		want      map[string]any
	}{
		{
			name: "single path",
			overrides: []ValueOverride{{
				Path:       "image",
				Repository: "ghcr.io/verity-org/prom",
				Tag:        "v3",
			}},
			want: map[string]any{
				"prometheus": map[string]any{
					"image": map[string]any{
						"repository": "ghcr.io/verity-org/prom",
						"tag":        "v3",
					},
				},
			},
		},
		{
			name: "nested path",
			overrides: []ValueOverride{{
				Path:       "a.b.image",
				Repository: "repo/nested",
				Tag:        "tag-1",
			}},
			want: map[string]any{
				"prometheus": map[string]any{
					"a": map[string]any{
						"b": map[string]any{
							"image": map[string]any{
								"repository": "repo/nested",
								"tag":        "tag-1",
							},
						},
					},
				},
			},
		},
		{
			name: "multiple merge",
			overrides: []ValueOverride{
				{Path: "server.image", Repository: "repo/server", Tag: "sv"},
				{Path: "alertmanager.image", Repository: "repo/am", Tag: "am"},
			},
			want: map[string]any{
				"prometheus": map[string]any{
					"server": map[string]any{
						"image": map[string]any{"repository": "repo/server", "tag": "sv"},
					},
					"alertmanager": map[string]any{
						"image": map[string]any{"repository": "repo/am", "tag": "am"},
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildValuesTree("prometheus", tt.overrides)
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("buildValuesTree() = %#v, want %#v", got, tt.want)
			}
		})
	}
}
