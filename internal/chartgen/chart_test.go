package chartgen

import (
	"bytes"
	"fmt"
	"reflect"
	"testing"
	"text/template"

	"gopkg.in/yaml.v3"

	"github.com/verity-org/verity/internal/config"
)

const (
	testPromRepo       = "verity.supply/prom"
	testPromServerRepo = "verity.supply/prometheus"
	testPromServerTag  = "v3.2.1"
)

func TestBuildWrapperChart(t *testing.T) {
	original := config.ChartSpec{
		Name:       "prometheus",
		Version:    "28.9.1",
		Repository: "oci://ghcr.io/prometheus-community/charts",
	}

	chart, err := BuildWrapperChart(original, nil, nil)
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
		name        string
		chartValues map[string]any
		overrides   []ValueOverride
		assert      func(t *testing.T, values map[string]any)
	}{
		{
			name:        "single image override",
			chartValues: nil,
			overrides: []ValueOverride{{
				Path:       "image",
				Repository: testPromRepo,
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
				if image["repository"] != testPromRepo || image["tag"] != "v3" {
					t.Fatalf("prometheus.image = %#v, want repository/tag override", image)
				}
			},
		},
		{
			name:        "nested path",
			chartValues: nil,
			overrides: []ValueOverride{{
				Path:       "server.image",
				Repository: testPromServerRepo,
				Tag:        testPromServerTag,
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
				if image["repository"] != testPromServerRepo || image["tag"] != testPromServerTag {
					t.Fatalf("prometheus.server.image = %#v, want repository/tag override", image)
				}
			},
		},
		{
			name:        "multiple overrides",
			chartValues: nil,
			overrides: []ValueOverride{
				{Path: "image", Repository: testPromRepo, Tag: "v3"},
				{Path: "server.image", Repository: testPromServerRepo, Tag: testPromServerTag},
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
				if img["repository"] != testPromRepo || img["tag"] != "v3" {
					t.Fatalf("prometheus.image = %#v, want override", img)
				}
				if serverImg["repository"] != testPromServerRepo || serverImg["tag"] != testPromServerTag {
					t.Fatalf("prometheus.server.image = %#v, want override", serverImg)
				}
			},
		},
		{
			name: "chart values merged before image overrides",
			chartValues: map[string]any{
				"testFramework.enabled": false,
				"grafana.enabled":       false,
			},
			overrides: []ValueOverride{{
				Path:       "image",
				Repository: testPromRepo,
				Tag:        "v3",
			}},
			assert: func(t *testing.T, values map[string]any) {
				prom, ok := values["prometheus"].(map[string]any)
				if !ok {
					t.Fatal("prometheus root missing")
				}
				testFramework, ok := prom["testFramework"].(map[string]any)
				if !ok || !reflect.DeepEqual(testFramework["enabled"], false) {
					t.Fatalf("testFramework.enabled = %#v, want false", prom["testFramework"])
				}
				grafana, ok := prom["grafana"].(map[string]any)
				if !ok || !reflect.DeepEqual(grafana["enabled"], false) {
					t.Fatalf("grafana.enabled = %#v, want false", prom["grafana"])
				}
				image, ok := prom["image"].(map[string]any)
				if !ok {
					t.Fatal("image missing")
				}
				if image["repository"] != testPromRepo || image["tag"] != "v3" {
					t.Fatalf("prometheus.image = %#v, want override", image)
				}
			},
		},
	}

	original := config.ChartSpec{Name: "prometheus", Version: "28.9.1", Repository: "oci://repo/charts"}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			chart, err := BuildWrapperChart(original, tt.chartValues, tt.overrides)
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

// TestBuildWrapperChartValuesMergeMapValue covers the mergeMapValue
// path that buildValuesTree uses when applying map-shaped image
// overrides on top of chartValues entries living at the same dotted
// path. Split out from TestBuildWrapperChartValues to keep the
// per-function cyclomatic complexity within the maintidx threshold.
func TestBuildWrapperChartValuesMergeMapValue(t *testing.T) {
	tests := []struct {
		name        string
		chartValues map[string]any
		overrides   []ValueOverride
		assert      func(t *testing.T, values map[string]any)
	}{
		{
			// Regression for verity-org/verity#326 — gitea's
			// `image.rootless: false` (a chartValues sibling of
			// `image.repository` / `image.tag`) was being silently
			// dropped because the image-override pass replaced the
			// entire `image:` subtree with `{repository, tag}`,
			// losing the rootless sibling. mergeMapValue must
			// preserve unrelated chart-value keys that land at the
			// same dotted path as a map-shaped image override.
			name: "chart values siblings under image path preserved through image override",
			chartValues: map[string]any{
				"image.rootless": false,
			},
			overrides: []ValueOverride{{
				Path:       "image",
				Repository: testPromRepo,
				Tag:        "v3",
			}},
			assert: func(t *testing.T, values map[string]any) {
				prom, ok := values["prometheus"].(map[string]any)
				if !ok {
					t.Fatal("prometheus root missing")
				}
				image, ok := prom["image"].(map[string]any)
				if !ok {
					t.Fatalf("image missing: %#v", prom)
				}
				if !reflect.DeepEqual(image["rootless"], false) {
					t.Fatalf("image.rootless dropped by override merge: %#v", image)
				}
				if image["repository"] != testPromRepo || image["tag"] != "v3" {
					t.Fatalf("image override fields missing: %#v", image)
				}
			},
		},
		{
			// Nested-path variant: chartValues sibling deep under an
			// image override (`server.image.rootless: true`) must
			// survive the override pass at the same path.
			name: "chart values siblings under nested image path preserved",
			chartValues: map[string]any{
				"server.image.rootless":   true,
				"server.image.pullPolicy": "Always",
			},
			overrides: []ValueOverride{{
				Path:       "server.image",
				Repository: testPromServerRepo,
				Tag:        testPromServerTag,
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
					t.Fatalf("server.image missing: %#v", server)
				}
				if !reflect.DeepEqual(image["rootless"], true) {
					t.Fatalf("server.image.rootless dropped: %#v", image)
				}
				if image["pullPolicy"] != "Always" {
					t.Fatalf("server.image.pullPolicy dropped: %#v", image)
				}
				if image["repository"] != testPromServerRepo || image["tag"] != testPromServerTag {
					t.Fatalf("server.image override fields missing: %#v", image)
				}
			},
		},
		{
			// Override at a path with no prior chartValues sibling
			// must still produce the full {repository, tag} leaf —
			// preserves backwards compatibility with the single-image
			// override path.
			name: "override at unoccupied image path writes fresh leaf",
			chartValues: map[string]any{
				"unrelated.flag": true,
			},
			overrides: []ValueOverride{{
				Path:       "image",
				Repository: testPromRepo,
				Tag:        "v3",
			}},
			assert: func(t *testing.T, values map[string]any) {
				prom, ok := values["prometheus"].(map[string]any)
				if !ok {
					t.Fatal("prometheus root missing")
				}
				image, ok := prom["image"].(map[string]any)
				if !ok {
					t.Fatalf("image missing: %#v", prom)
				}
				if image["repository"] != testPromRepo || image["tag"] != "v3" {
					t.Fatalf("image override fields missing: %#v", image)
				}
				if _, present := image["rootless"]; present {
					t.Fatalf("image.rootless leaked from elsewhere: %#v", image)
				}
				unrelated, ok := prom["unrelated"].(map[string]any)
				if !ok {
					t.Fatalf("unrelated subtree missing: %#v", prom)
				}
				if !reflect.DeepEqual(unrelated["flag"], true) {
					t.Fatalf("unrelated.flag chart value lost: %#v", unrelated)
				}
			},
		},
	}

	original := config.ChartSpec{Name: "prometheus", Version: "28.9.1", Repository: "oci://repo/charts"}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			chart, err := BuildWrapperChart(original, tt.chartValues, tt.overrides)
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
		PatchedRepo: "verity.supply/kafka",
		PatchedTag:  "patched-tag",
	}}, vc)
	if err != nil {
		t.Fatalf("buildChartImageOverrides() error = %v", err)
	}

	chart, err := BuildWrapperChart(original, nil, overrides)
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
	if kafka["image"] != "verity.supply/kafka:patched-tag" {
		t.Fatalf("kafka.image = %#v, want patched image string", kafka["image"])
	}
}

func TestBuildWrapperChartValues_ChartImageOverrideCSV(t *testing.T) {
	original := config.ChartSpec{Name: "strimzi-kafka-operator", Version: "0.51.0", Repository: "oci://repo/charts"}
	vc := &config.VerityConfig{
		ChartImageOverrides: map[string][]config.ChartImageOverride{
			"strimzi-kafka-operator": {
				{
					Source: "STRIMZI_KAFKA_IMAGES",
					Type:   "csv",
					Path:   "kafka.versions[\"{version}\"]",
				},
			},
		},
	}

	overrides, err := buildChartImageOverrides(original.Name, []ImageMapping{
		{
			Source:       "STRIMZI_KAFKA_IMAGES",
			OriginalRepo: "quay.io/strimzi/kafka",
			OriginalTag:  "0.51.0-kafka-4.1.0",
			PatchedRepo:  "verity.supply/kafka",
			PatchedTag:   "patched-4.1.0",
		},
		{
			Source:       "STRIMZI_KAFKA_IMAGES",
			OriginalRepo: "quay.io/strimzi/kafka",
			OriginalTag:  "0.51.0-kafka-4.2.0",
			PatchedRepo:  "verity.supply/kafka",
			PatchedTag:   "patched-4.2.0",
		},
	}, vc)
	if err != nil {
		t.Fatalf("buildChartImageOverrides() error = %v", err)
	}

	chart, err := BuildWrapperChart(original, nil, overrides)
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
	versions, ok := kafka["versions"].(map[string]any)
	if !ok {
		t.Fatalf("versions node missing or invalid: %#v", kafka["versions"])
	}
	if versions["4.1.0"] != "verity.supply/kafka:patched-4.1.0" {
		t.Fatalf("versions[4.1.0] = %#v, want patched 4.1.0 image", versions["4.1.0"])
	}
	if versions["4.2.0"] != "verity.supply/kafka:patched-4.2.0" {
		t.Fatalf("versions[4.2.0] = %#v, want patched 4.2.0 image", versions["4.2.0"])
	}
}

func TestBuildWrapperChartEmptyOverrides(t *testing.T) {
	original := config.ChartSpec{Name: "prometheus", Version: "28.9.1", Repository: "oci://repo/charts"}

	chart, err := BuildWrapperChart(original, nil, []ValueOverride{})
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
	_, err := BuildWrapperChart(config.ChartSpec{Version: "1.0.0", Repository: "oci://repo/charts"}, nil, nil)
	if err == nil {
		t.Fatal("BuildWrapperChart() error = nil, want non-nil")
	}
}

func TestBuildValuesTree(t *testing.T) {
	tests := []struct {
		name      string
		chartName string
		overrides []ValueOverride
		want      map[string]any
	}{
		{
			// 3-field shape (postgres-operator, traefik, jenkins, etc.):
			// upstream chart templates `{{ image.registry }}/{{ image.repository }}`.
			// The wrapper composes the FQDN into the two sibling fields so
			// the rendered ref is `verity.supply/zalando/postgres-operator:v1.15.1`,
			// no leading slash. See #308 wave 2.
			name:      "composes registry sibling for grafana/traefik shape",
			chartName: "postgres-operator",
			overrides: []ValueOverride{{
				Path:        "image",
				Repository:  "zalando/postgres-operator",
				Tag:         "v1.15.1",
				SetRegistry: "verity.supply",
			}},
			want: map[string]any{
				"postgres-operator": map[string]any{
					"image": map[string]any{
						"registry":   "verity.supply",
						"repository": "zalando/postgres-operator",
						"tag":        "v1.15.1",
					},
				},
			},
		},
		{
			// kyverno 3.7.x — composes `defaultRegistry: verity.supply` +
			// `repository: kyverno` so kyverno's helper
			// `default (default .image.defaultRegistry .globalRegistry) .image.registry`
			// resolves to `verity.supply` (registry=nil, globalRegistry=nil → defaultRegistry fires).
			// Rendered ref: `verity.supply/kyverno:1.17`. See #308 wave 2.
			name:      "composes defaultRegistry sibling for kyverno shape",
			chartName: "kyverno",
			overrides: []ValueOverride{{
				Path:               "admissionController.container.image",
				Repository:         "kyverno",
				Tag:                "1.17",
				SetDefaultRegistry: "verity.supply",
			}},
			want: map[string]any{
				"kyverno": map[string]any{
					"admissionController": map[string]any{
						"container": map[string]any{
							"image": map[string]any{
								"defaultRegistry": "verity.supply",
								"repository":      "kyverno",
								"tag":             "1.17",
							},
						},
					},
				},
			},
		},
		{
			// Hybrid shape (`registry | default defaultRegistry`): both
			// siblings get the registry hostname so either short-circuit
			// path resolves to `verity.supply`.
			name:      "composes both registry and defaultRegistry siblings",
			chartName: "hybrid",
			overrides: []ValueOverride{{
				Path:               "image",
				Repository:         "foo/bar",
				Tag:                "v1",
				SetRegistry: "verity.supply",
				SetDefaultRegistry: "verity.supply",
			}},
			want: map[string]any{
				"hybrid": map[string]any{
					"image": map[string]any{
						"registry":        "verity.supply",
						"defaultRegistry": "verity.supply",
						"repository":      "foo/bar",
						"tag":             "v1",
					},
				},
			},
		},
		{
			name:      "single path",
			chartName: "prometheus",
			overrides: []ValueOverride{{
				Path:       "image",
				Repository: testPromRepo,
				Tag:        "v3",
			}},
			want: map[string]any{
				"prometheus": map[string]any{
					"image": map[string]any{
						"repository": testPromRepo,
						"tag":        "v3",
					},
				},
			},
		},
		{
			name:      "nested path",
			chartName: "prometheus",
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
			name:      "multiple merge",
			chartName: "prometheus",
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
			got := buildValuesTree(tt.chartName, nil, tt.overrides)
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("buildValuesTree() = %#v, want %#v", got, tt.want)
			}
		})
	}
}

// TestComposeRegistryRendersValidImageRefs is the #308 wave 2 regression
// test. It runs the full chart-gen pipeline (ResolveValuePathsWithSubcharts
// + buildValuesTree) for two fixtures that mirror the upstream chart
// template shapes that produced the leading-slash bug, then renders the
// merged values through a Helm-like text/template to assert the final
// image reference is well-formed (`verity.supply/<repo>:<tag>`, no
// leading slash, no empty registry).
//
// Shape 1 (grafana / traefik / argo-rollouts / falco / jenkins / postgres-operator
// / trivy-operator / alloy):
//
//	{{ .Values.image.registry }}/{{ .Values.image.repository }}:{{ .Values.image.tag }}
//
// Shape 2 (kyverno 3.7.x — `image.registry | default image.defaultRegistry`):
//
//	{{ .Values.image.registry | default .Values.image.defaultRegistry }}/...
//
// Both shapes must produce `verity.supply/grafana:12.3` after the
// fix — no `/ghcr.io/...` leading slash, no `docker.io/ghcr.io/...` double
// registry.
// renderCase is the table-row shape for TestComposeRegistryRendersValidImageRefs.
// Lifted to package scope so the per-case helpers (extracted to keep the
// main test function under the maintainability-index threshold) can
// reference it.
type renderCase struct {
	name              string
	valuesYML         string
	mappings          []ImageMapping
	templateExpr      string
	wantImage         string
	wantRegistrySet   string // expected wrapper leaf .registry value (informational)
	wantDefRegistry   string // expected wrapper leaf .defaultRegistry value (informational)
	wantRepoStripped  string // expected wrapper leaf .repository value
	assertRegistryKey string // "registry" or "defaultRegistry" — which sibling we expect populated
}

func TestComposeRegistryRendersValidImageRefs(t *testing.T) {
	cases := []renderCase{
		{
			// grafana / traefik / alloy / falco / postgres-operator / etc.
			// Upstream `image: { registry, repository, tag }` with template
			// shape `{{ image.registry }}/{{ image.repository }}:{{ image.tag }}`
			// (direct concatenation, no `default` short-circuit). Pre-fix
			// the wrapper wrote `registry: ""` → `/verity.supply/grafana:12.3`
			// (kubelet `InvalidImageName`).
			name: "grafana-shape direct concatenation produces valid ref",
			valuesYML: `image:
  registry: docker.io
  repository: grafana/grafana
  tag: ""
`,
			mappings: []ImageMapping{{
				OriginalRepo: "grafana/grafana",
				PatchedRepo:  "verity.supply/grafana",
				PatchedTag:   "12.3",
			}},
			templateExpr:      `{{ .image.registry }}/{{ .image.repository }}:{{ .image.tag }}`,
			wantImage:         "verity.supply/grafana:12.3",
			wantRegistrySet: "verity.supply",
			wantRepoStripped:  "grafana",
			assertRegistryKey: "registry",
		},
		{
			// kyverno 3.7.x — `image: { registry: ~, defaultRegistry, repository, tag }`
			// with helper `default (default .image.defaultRegistry .globalRegistry) .image.registry`.
			// Pre-fix the wrapper wrote `defaultRegistry: ""` and that broke
			// any chart whose template was a plain concat rather than the
			// `default`-short-circuit shape. After the fix the wrapper writes
			// `defaultRegistry: verity.supply` + `repository: kyverno`,
			// which composes correctly with kyverno's helper AND with any
			// plain-concat template.
			name: "kyverno-shape default short-circuit produces valid ref",
			valuesYML: `image:
  registry: ~
  defaultRegistry: "reg.kyverno.io"
  repository: "kyverno/kyverno"
  tag: ~
`,
			mappings: []ImageMapping{{
				OriginalRepo: "kyverno/kyverno",
				PatchedRepo:  "verity.supply/kyverno",
				PatchedTag:   "1.17",
			}},
			// kyverno's resolved registry expression: registry | default defaultRegistry.
			// In Helm: `default fallback primary` = primary if primary != empty else fallback.
			// Equivalent here: if .image.registry is empty, use .image.defaultRegistry.
			templateExpr:      `{{ orDefault .image.registry .image.defaultRegistry }}/{{ .image.repository }}:{{ .image.tag }}`,
			wantImage:         "verity.supply/kyverno:1.17",
			wantDefRegistry: "verity.supply",
			wantRepoStripped:  "kyverno",
			assertRegistryKey: "defaultRegistry",
		},
		{
			// Mixed shape: chart that templates `{{ registry | default defaultRegistry }}/...`
			// AND has both siblings declared upstream. Both wrapper leaves
			// must be set so the short-circuit resolves to `verity.supply` no
			// matter which path the helper takes.
			name: "registry|default-defaultRegistry shape with both siblings present",
			valuesYML: `image:
  registry: "docker.io"
  defaultRegistry: "fallback.example.com"
  repository: "foo/bar"
  tag: "v1"
`,
			mappings: []ImageMapping{{
				OriginalRepo: "foo/bar",
				PatchedRepo:  "verity.supply/foo/bar",
				PatchedTag:   "v1",
			}},
			templateExpr:      `{{ orDefault .image.registry .image.defaultRegistry }}/{{ .image.repository }}:{{ .image.tag }}`,
			wantImage:         "verity.supply/foo/bar:v1",
			wantRegistrySet: "verity.supply",
			wantDefRegistry: "verity.supply",
			wantRepoStripped:  "foo/bar",
			assertRegistryKey: "registry",
		},
		{
			// Upstream chart that defaults `registry: ""` (an explicit
			// empty-string declaration, which several Bitnami / community
			// charts ship as the "let the user override me" pattern).
			// Pre-#312-review-2 walkValues treated this as "no registry
			// sibling" and we silently produced `/ghcr.io/...:v1`. After
			// the fix the empty-string declaration counts as a real
			// declaration and the wrapper composes `registry: ghcr.io`.
			name: "registry empty-string upstream still composes",
			valuesYML: `image:
  registry: ""
  repository: "foo/bar"
  tag: "v1"
`,
			mappings: []ImageMapping{{
				OriginalRepo: "foo/bar",
				PatchedRepo:  "verity.supply/foo/bar",
				PatchedTag:   "v1",
			}},
			templateExpr:      `{{ .image.registry }}/{{ .image.repository }}:{{ .image.tag }}`,
			wantImage:         "verity.supply/foo/bar:v1",
			wantRegistrySet: "verity.supply",
			wantRepoStripped:  "foo/bar",
			assertRegistryKey: "registry",
		},
		{
			// Bitnami shape, EMPTY upstream global: chart helper resolves
			// `{{ default .Values.global.imageRegistry .Values.image.registry }}`
			// — global wins when set, otherwise per-image registry.
			// Postgres-operator and Bitnami's own postgresql sub-chart
			// ship `global.imageRegistry: ""` upstream, so the per-image
			// SetRegistry is sufficient on its own; the wave-3
			// neutralisation overrides the empty global with another
			// empty (no-op semantically, but the wrapper is consistent).
			name: "global.imageRegistry empty upstream — image.registry composes",
			valuesYML: `global:
  imageRegistry: ""
image:
  registry: docker.io
  repository: bitnamilegacy/postgresql
  tag: "16.1.0-debian-11-r15"
`,
			mappings: []ImageMapping{{
				OriginalRepo: "bitnamilegacy/postgresql",
				PatchedRepo:  "verity.supply/bitnamilegacy/postgresql",
				PatchedTag:   "16.1.0-debian-11-r15",
			}},
			templateExpr:      `{{ orDefault .global.imageRegistry .image.registry }}/{{ .image.repository }}:{{ .image.tag }}`,
			wantImage:         "verity.supply/bitnamilegacy/postgresql:16.1.0-debian-11-r15",
			wantRegistrySet: "verity.supply",
			wantRepoStripped:  "bitnamilegacy/postgresql",
			assertRegistryKey: "registry",
		},
		{
			// Bitnami shape, NON-EMPTY upstream global: this is the
			// regression case Copilot flagged in #312 round 3. Without
			// wave-3 neutralisation the wrapper rewrites `image.registry`
			// to `ghcr.io`, but Helm's `default global.imageRegistry`
			// still picks up the upstream `docker.io` (non-empty wins),
			// rendering `docker.io/verity-org/bitnamilegacy/postgresql`.
			// With neutralisation, the wrapper writes both
			// `image.registry: ghcr.io` AND `global.imageRegistry: ""`,
			// so the helper falls through to per-image and renders
			// `verity.supply/bitnamilegacy/postgresql:<tag>`.
			name: "global.imageRegistry NON-empty upstream — neutralised by wave-3",
			valuesYML: `global:
  imageRegistry: "docker.io"
image:
  registry: docker.io
  repository: bitnamilegacy/postgresql
  tag: "16.1.0-debian-11-r15"
`,
			mappings: []ImageMapping{{
				OriginalRepo: "bitnamilegacy/postgresql",
				PatchedRepo:  "verity.supply/bitnamilegacy/postgresql",
				PatchedTag:   "16.1.0-debian-11-r15",
			}},
			templateExpr:      `{{ orDefault .global.imageRegistry .image.registry }}/{{ .image.repository }}:{{ .image.tag }}`,
			wantImage:         "verity.supply/bitnamilegacy/postgresql:16.1.0-debian-11-r15",
			wantRegistrySet: "verity.supply",
			wantRepoStripped:  "bitnamilegacy/postgresql",
			assertRegistryKey: "registry",
		},
		{
			// tempo-distributed shape (#308 A.4): chart's helper is
			// `coalesce .global.image.registry .component.registry .tempo.registry`
			// — `coalesce` returns the first non-empty argument, so a
			// non-empty `global.image.registry: docker.io` upstream wins
			// over our per-image `tempo.image.registry: ghcr.io` rewrite.
			// Wave-3 fix: emit `global.image.registry: ""` so coalesce
			// falls through to the per-image registry.
			name: "global.image.registry takes precedence over image.registry",
			valuesYML: `global:
  image:
    registry: docker.io
image:
  registry: docker.io
  repository: grafana/tempo
  tag: "2.4"
`,
			mappings: []ImageMapping{{
				OriginalRepo: "grafana/tempo",
				PatchedRepo:  "verity.supply/tempo",
				PatchedTag:   "2.4",
			}},
			// Sprig `coalesce` mirrored: returns the first non-empty value.
			templateExpr:      `{{ coalesceFirst .global.image.registry .image.registry }}/{{ .image.repository }}:{{ .image.tag }}`,
			wantImage:         "verity.supply/tempo:2.4",
			wantRegistrySet: "verity.supply",
			wantRepoStripped:  "tempo",
			assertRegistryKey: "registry",
		},
	}

	original := config.ChartSpec{Name: "wrapper", Version: "0.0.0", Repository: "oci://repo/charts"}

	for i := range cases {
		tc := &cases[i]
		t.Run(tc.name, func(t *testing.T) {
			runComposeRegistryRenderCase(t, tc, original)
		})
	}
}

// runComposeRegistryRenderCase exercises a single
// TestComposeRegistryRendersValidImageRefs fixture end-to-end:
// resolve → build → assert wrapper leaf shape → merge → render →
// assert rendered ref. Extracted out of the main loop body purely to
// keep the test function under the maintainability-index threshold;
// every assertion below was previously inline in `t.Run`.
func runComposeRegistryRenderCase(t *testing.T, tc *renderCase, original config.ChartSpec) {
	t.Helper()

	// 1. Run the resolver to produce the override (mirrors what
	//    chart-gen does in production). May return more than one
	//    override when global-registry neutralisation (#308 wave 3)
	//    fires alongside the per-image rewrite.
	overrides, err := ResolveValuePaths([]byte(tc.valuesYML), tc.mappings, nil)
	if err != nil {
		t.Fatalf("ResolveValuePaths() error = %v", err)
	}
	if !hasImageOverride(overrides) {
		t.Fatalf("ResolveValuePaths() produced no per-image override: %#v", overrides)
	}

	// 2. Build the wrapper values tree.
	chart, err := BuildWrapperChart(original, nil, overrides)
	if err != nil {
		t.Fatalf("BuildWrapperChart() error = %v", err)
	}
	var wrapperValues map[string]any
	if err := yaml.Unmarshal(chart.ValuesYAML, &wrapperValues); err != nil {
		t.Fatalf("yaml.Unmarshal(ValuesYAML) error = %v", err)
	}
	leaf := navigateToImageLeaf(t, wrapperValues, original.Name)
	assertWrapperLeafShape(t, tc, leaf)

	// 3. Merge ALL wrapper overrides onto the upstream values (Helm's
	//    child-overrides-parent semantics) and render the chart's
	//    template expression. This is the end-to-end check that
	//    chart-gen's output is template-safe. The wave-3 global-registry
	//    neutralisation overrides MUST be applied here too — without
	//    them, a chart whose template defers to `global.image.registry`
	//    would still render the upstream global and our per-image
	//    SetRegistry would be silently ignored.
	merged := mergeUpstreamWithAllOverrides(t, tc.valuesYML, wrapperValues, original.Name)
	rendered := renderHelmExpr(t, tc.templateExpr, merged)
	if rendered != tc.wantImage {
		t.Fatalf("rendered image = %q, want %q\n  template: %s\n  merged values: %#v", rendered, tc.wantImage, tc.templateExpr, merged)
	}
	// 4. Sanity check: no leading slash (regression of #308 wave 2).
	if rendered[0] == '/' {
		t.Fatalf("rendered image has leading slash: %q (regression of #308)", rendered)
	}
}

// assertWrapperLeafShape covers the per-image-leaf assertions:
//   - repository was stripped of its registry prefix correctly,
//   - the wrapper does NOT emit an empty-string sibling (the wave-2 foot-gun),
//   - the expected sibling holds the registry hostname,
//   - the table-recorded `assertRegistryKey` is actually populated.
//
// The global-registry neutralisation entries write empty-string scalars
// at separate paths (`global.imageRegistry`, `global.image.registry`);
// they are exercised end-to-end by the rendering step that follows.
func assertWrapperLeafShape(t *testing.T, tc *renderCase, leaf map[string]any) {
	t.Helper()

	if got := leaf["repository"]; got != tc.wantRepoStripped {
		t.Fatalf("wrapper repository = %v, want %v (full leaf: %#v)", got, tc.wantRepoStripped, leaf)
	}
	if v, ok := leaf["registry"]; ok && v == "" {
		t.Fatalf("wrapper leaf must not emit empty-string registry: %#v", leaf)
	}
	if v, ok := leaf["defaultRegistry"]; ok && v == "" {
		t.Fatalf("wrapper leaf must not emit empty-string defaultRegistry: %#v", leaf)
	}
	if tc.wantRegistrySet != "" {
		if got := leaf["registry"]; got != tc.wantRegistrySet {
			t.Fatalf("wrapper registry = %v, want %v", got, tc.wantRegistrySet)
		}
	}
	if tc.wantDefRegistry != "" {
		if got := leaf["defaultRegistry"]; got != tc.wantDefRegistry {
			t.Fatalf("wrapper defaultRegistry = %v, want %v", got, tc.wantDefRegistry)
		}
	}
	if tc.assertRegistryKey != "" {
		v, ok := leaf[tc.assertRegistryKey]
		if !ok {
			t.Fatalf("wrapper leaf missing expected sibling %q: %#v", tc.assertRegistryKey, leaf)
		}
		s, isString := v.(string)
		if !isString || s == "" {
			t.Fatalf("wrapper leaf %q is empty or non-string: %#v", tc.assertRegistryKey, leaf)
		}
	}
}

// hasImageOverride reports whether the override list contains at least
// one per-image-leaf override (Repository != "" or Tag != ""), as opposed
// to only scalar neutralisation overrides. Used by
// TestComposeRegistryRendersValidImageRefs to assert that chart-gen
// produced an actual rewrite, regardless of how many neutralisation
// entries (#308 wave 3) it added alongside.
func hasImageOverride(overrides []ValueOverride) bool {
	for _, o := range overrides {
		if o.Repository != "" || o.Tag != "" {
			return true
		}
	}
	return false
}

// navigateToImageLeaf walks `wrapperValues[chartName].image` and returns the
// leaf map. The wrapper layout is `{ <chartName>: { image: { ... } } }` for
// a single top-level image override.
func navigateToImageLeaf(t *testing.T, wrapperValues map[string]any, chartName string) map[string]any {
	t.Helper()
	root, ok := wrapperValues[chartName].(map[string]any)
	if !ok {
		t.Fatalf("wrapper values %q root missing or not a map: %#v", chartName, wrapperValues)
	}
	leaf, ok := root["image"].(map[string]any)
	if !ok {
		// Some fixtures may use admissionController.container.image —
		// walk the only branch we care about. The current cases all use
		// `image` as the top-level path, so failing here is a bug.
		t.Fatalf("wrapper values root.image missing or not a map: %#v", root)
	}
	return leaf
}

// mergeUpstreamWithAllOverrides parses the upstream values YAML and
// recursively merges every node from the wrapper override tree onto it
// (Helm's child-overrides-parent semantics for maps; scalar nodes
// overwrite). The override tree is the full `wrapperValues` produced by
// `BuildWrapperChart`, scoped under `chartName`, so this captures both
// the per-image leaf rewrite AND any wave-3 global-registry
// neutralisation scalars (`global.imageRegistry: ""`, etc.).
func mergeUpstreamWithAllOverrides(t *testing.T, valuesYML string, wrapperValues map[string]any, chartName string) map[string]any {
	t.Helper()
	var upstream map[string]any
	if err := yaml.Unmarshal([]byte(valuesYML), &upstream); err != nil {
		t.Fatalf("yaml.Unmarshal(upstream) error = %v", err)
	}
	overrides, ok := wrapperValues[chartName].(map[string]any)
	if !ok {
		t.Fatalf("wrapper values %q root missing or not a map: %#v", chartName, wrapperValues)
	}
	deepMergeOverride(upstream, overrides)
	return upstream
}

// deepMergeOverride recursively merges `src` onto `dst`. Maps are merged
// (src keys override dst keys at each level); every other value type
// (string, bool, list, scalar empty-string) overwrites. Mirrors Helm's
// values-merge semantics closely enough for the regression-test
// templates the harness exercises.
func deepMergeOverride(dst, src map[string]any) {
	for k, v := range src {
		if vMap, ok := v.(map[string]any); ok {
			if dstMap, ok := dst[k].(map[string]any); ok {
				deepMergeOverride(dstMap, vMap)
				continue
			}
			cloned := make(map[string]any, len(vMap))
			deepMergeOverride(cloned, vMap)
			dst[k] = cloned
			continue
		}
		dst[k] = v
	}
}

// renderHelmExpr renders a text/template expression with a small subset of
// Helm-like helpers. Specifically we provide `orDefault` to mimic Helm's
// `default <fallback> <primary>` behaviour (primary if primary != empty,
// else fallback) — this is the only Helm-specific function the regression
// fixtures reference. We deliberately avoid pulling in Helm itself to keep
// the test hermetic.
func renderHelmExpr(t *testing.T, expr string, data map[string]any) string {
	t.Helper()
	funcs := template.FuncMap{
		"orDefault": func(primary, fallback any) any {
			if isHelmEmpty(primary) {
				return fallback
			}
			return primary
		},
		// coalesceFirst mirrors Sprig's `coalesce`: returns the first
		// non-empty argument. Tempo-distributed and several other Grafana
		// charts use this idiom in their image helpers; the regression
		// fixture for #308 wave 3 invokes it directly.
		"coalesceFirst": func(args ...any) any {
			for _, a := range args {
				if !isHelmEmpty(a) {
					return a
				}
			}
			return ""
		},
	}
	tmpl, err := template.New("render").Funcs(funcs).Parse(expr)
	if err != nil {
		t.Fatalf("template.Parse() error = %v", err)
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		t.Fatalf("template.Execute() error = %v", err)
	}
	return buf.String()
}

// TestIsHelmEmpty regresses the multi-type-switch Go gotcha where
// `case int, int32, int64, float32, float64:` keeps `x` typed as the
// union interface — comparing it against the untyped `0` literal returns
// FALSE for every numeric type other than `int`. The previous grouped
// form silently reported `int32(0)` / `int64(0)` / `float32(0)` /
// `float64(0)` as non-empty, breaking Helm's `default` semantics for any
// fixture that exercised numeric zeros (none today, but the helper is
// part of the regression-test harness for #308 wave 2 and must be
// trustworthy). Each numeric type now lives in its own case so `x`
// unwraps to the concrete type.
func TestIsHelmEmpty(t *testing.T) {
	cases := []struct {
		name string
		v    any
		want bool
	}{
		{"nil", nil, true},
		{"empty string", "", true},
		{"non-empty string", "x", false},
		{"empty list", []any{}, true},
		{"non-empty list", []any{1}, false},
		{"empty map", map[string]any{}, true},
		{"non-empty map", map[string]any{"k": 1}, false},
		{"false bool", false, true},
		{"true bool", true, false},

		// Numeric zero — these are the regression cases. The grouped
		// type-switch form returned `false` for every line below except
		// `int(0)`. Each must now report `true` (matching Helm's rule
		// that 0 is empty).
		{"int zero", int(0), true},
		{"int non-zero", int(1), false},
		{"int32 zero", int32(0), true},
		{"int32 non-zero", int32(1), false},
		{"int64 zero", int64(0), true},
		{"int64 non-zero", int64(1), false},
		{"float32 zero", float32(0), true},
		{"float32 non-zero", float32(0.5), false},
		{"float64 zero", float64(0), true},
		{"float64 non-zero", float64(0.5), false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := isHelmEmpty(tc.v); got != tc.want {
				t.Fatalf("isHelmEmpty(%T(%v)) = %v, want %v", tc.v, tc.v, got, tc.want)
			}
		})
	}
}

// isHelmEmpty mirrors Helm's `default` emptiness rule: nil, "", 0,
// empty list/map all count as empty. text/template's zero-value rendering
// of `nil` is `<no value>`, so we explicitly normalise.
//
// Numeric zero detection uses one type-switch case per concrete type
// rather than a grouped `case int, int32, int64, ...:`. The grouped form
// keeps `x` typed as the union interface, which compares against the
// untyped `0` literal as `interface{}(int32(0)) == 0` — and that
// comparison is FALSE for every numeric type other than `int`, because
// Go's type-switch only unwraps to a concrete type when the case lists
// exactly one type (Go spec, "Type switches"). The grouped form would
// silently report `int32(0)`/`int64(0)`/`float32(0)`/`float64(0)` as
// non-empty, which is precisely the wrong answer for Helm's `default`.
func isHelmEmpty(v any) bool {
	if v == nil {
		return true
	}
	switch x := v.(type) {
	case string:
		return x == ""
	case fmt.Stringer:
		return x.String() == ""
	case []any:
		return len(x) == 0
	case map[string]any:
		return len(x) == 0
	case bool:
		return !x
	case int:
		return x == 0
	case int32:
		return x == 0
	case int64:
		return x == 0
	case float32:
		return x == 0
	case float64:
		return x == 0
	}
	return false
}
