package ci

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/verity-org/verity/internal/integer/apkindex"
)

var errTemporaryAPKIndexOutage = errors.New("temporary apkindex outage")

func writeTestFile(t *testing.T, path, body string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(body), 0o644))
}

func setupIntegerPlanRepo(t *testing.T) (root string) {
	t.Helper()
	root = t.TempDir()
	writeTestFile(t, filepath.Join(root, "integer.yaml"), `
target:
  registry: ghcr.io/test-org
`)
	for _, base := range []string{"wolfi-base", "wolfi-dev", "wolfi-fips"} {
		writeTestFile(t, filepath.Join(root, "images", "_base", base+".yaml"), "# base\n")
	}
	writeTestFile(t, filepath.Join(root, "images", "node.yaml"), `
name: node
description: Node
upstream:
  package: nodejs-{{version}}
types:
  default:
    base: wolfi-base
    packages: ["nodejs-{{version}}"]
  dev:
    base: wolfi-dev
    packages: ["nodejs-{{version}}", "npm"]
versions:
  "20": {}
  "22": {}
`)
	writeTestFile(t, filepath.Join(root, "images", "curl.yaml"), `
name: curl
description: curl
upstream:
  package: curl
types:
  default:
    base: wolfi-base
    packages: ["curl"]
versions:
  latest:
    latest: true
`)
	writeTestFile(t, filepath.Join(root, "images", "caddy.yaml"), `
name: caddy
description: caddy
upstream:
  package: caddy
types:
  default:
    base: wolfi-base
    packages: ["caddy"]
  fips:
    base: wolfi-base
    fips-profile: go
    packages: ["caddy"]
    environment:
      GODEBUG: "fips140=on"
    melange:
      upstream: caddy
      env-file: fips.env
versions:
  "1": {}
  "2": {}
`)
	writeTestFile(t, filepath.Join(root, "images", "cilium.yaml"), `
name: cilium
description: cilium
upstream:
  package: cilium-{{version}}
types:
  default:
    base: wolfi-base
    packages: ["cilium-{{version}}"]
    melange:
      upstream: cilium-{{version}}
versions:
  "1.19": {}
`)
	writeTestFile(t, filepath.Join(root, "images", "platform", "envoy.yaml"), `
name: platform/envoy
description: envoy
upstream:
  package: envoy-{{version}}
types:
  default:
    base: wolfi-base
    packages: ["envoy-{{version}}"]
    melange:
      upstream: envoy-{{version}}
versions:
  "1.2": {}
`)
	return root
}

func TestPlanIntegerPRChangedImageBuildsLatestAndSmokesAllCombos(t *testing.T) {
	root := setupIntegerPlanRepo(t)

	plan, err := PlanIntegerPR(&IntegerPROptions{
		ChangedFiles: []string{"images/node.yaml"},
		ConfigPath:   filepath.Join(root, "integer.yaml"),
		ImagesDir:    filepath.Join(root, "images"),
		APKIndexURL:  "",
		GenDir:       filepath.Join(root, "gen"),
	})
	require.NoError(t, err)

	assert.True(t, plan.HasChanges)
	assert.ElementsMatch(t, []map[string]string{
		{"image": "node", "version": "22", "type": "default"},
		{"image": "node", "version": "22", "type": "dev"},
	}, plan.Matrix.Include)
	assert.Len(t, plan.SmokeMatrix.Include, 4)
}

func TestPlanIntegerPREmptyWhenNoImageFilesChanged(t *testing.T) {
	root := setupIntegerPlanRepo(t)

	plan, err := PlanIntegerPR(&IntegerPROptions{
		ChangedFiles: []string{"README.md"},
		ConfigPath:   filepath.Join(root, "integer.yaml"),
		ImagesDir:    filepath.Join(root, "images"),
		APKIndexURL:  "",
		GenDir:       filepath.Join(root, "gen"),
	})
	require.NoError(t, err)

	assert.False(t, plan.HasChanges)
	assert.Empty(t, plan.Matrix.Include)
	assert.Empty(t, plan.SmokeMatrix.Include)
}

func TestPlanIntegerPRMelangeChangesBuildAndSmokeEveryConsumer(t *testing.T) {
	root := setupIntegerPlanRepo(t)

	for _, changed := range []string{
		"packages/bespoke/locked/caddy.yaml",
		"packages/pipelines/test/daemon-check-output.yaml",
		"packages/upstream.lock.json",
		"packages/overrides/fips.env",
		"internal/integer/melange/build.go",
		"internal/integer/config/loader.go",
		"cmd/integer_melange.go",
		"cmd/integer_build.go",
		"cmd/integer.go",
		".github/workflows/integer-build-image.yaml",
		".github/workflows/pr-test.yaml",
	} {
		t.Run(changed, func(t *testing.T) {
			plan, err := PlanIntegerPR(&IntegerPROptions{
				ChangedFiles: []string{changed},
				ConfigPath:   filepath.Join(root, "integer.yaml"),
				ImagesDir:    filepath.Join(root, "images"),
				APKIndexURL:  "",
				GenDir:       filepath.Join(root, "gen"),
			})
			require.NoError(t, err)

			assert.True(t, plan.HasChanges)
			assert.ElementsMatch(t, []map[string]string{
				{"image": "caddy", "version": "2", "type": "default"},
				{"image": "caddy", "version": "2", "type": "fips"},
				{"image": "cilium", "version": "1.19", "type": "default"},
				{"image": "platform/envoy", "version": "1.2", "type": "default"},
			}, plan.Matrix.Include)
			assert.ElementsMatch(t, []map[string]string{
				{"image": "caddy", "version": "1", "type": "default"},
				{"image": "caddy", "version": "1", "type": "fips"},
				{"image": "caddy", "version": "2", "type": "default"},
				{"image": "caddy", "version": "2", "type": "fips"},
				{"image": "cilium", "version": "1.19", "type": "default"},
				{"image": "platform/envoy", "version": "1.2", "type": "default"},
			}, plan.SmokeMatrix.Include)
		})
	}
}

func TestPlanIntegerPRMelangeChangesIncludeEveryConsumerAlongsideChangedImages(t *testing.T) {
	root := setupIntegerPlanRepo(t)

	plan, err := PlanIntegerPR(&IntegerPROptions{
		ChangedFiles: []string{"images/node.yaml", "internal/integer/melange/build.go"},
		ConfigPath:   filepath.Join(root, "integer.yaml"),
		ImagesDir:    filepath.Join(root, "images"),
		APKIndexURL:  "",
		GenDir:       filepath.Join(root, "gen"),
	})
	require.NoError(t, err)

	assert.True(t, plan.HasChanges)
	assert.ElementsMatch(t, []map[string]string{
		{"image": "node", "version": "22", "type": "default"},
		{"image": "node", "version": "22", "type": "dev"},
		{"image": "caddy", "version": "2", "type": "default"},
		{"image": "caddy", "version": "2", "type": "fips"},
		{"image": "cilium", "version": "1.19", "type": "default"},
		{"image": "platform/envoy", "version": "1.2", "type": "default"},
	}, plan.Matrix.Include)
	assert.ElementsMatch(t, []map[string]string{
		{"image": "node", "version": "20", "type": "default"},
		{"image": "node", "version": "20", "type": "dev"},
		{"image": "node", "version": "22", "type": "default"},
		{"image": "node", "version": "22", "type": "dev"},
		{"image": "caddy", "version": "1", "type": "default"},
		{"image": "caddy", "version": "1", "type": "fips"},
		{"image": "caddy", "version": "2", "type": "default"},
		{"image": "caddy", "version": "2", "type": "fips"},
		{"image": "cilium", "version": "1.19", "type": "default"},
		{"image": "platform/envoy", "version": "1.2", "type": "default"},
	}, plan.SmokeMatrix.Include)
}

func TestPlanIntegerPRFailsClosedWhenAPKIndexFetchFails(t *testing.T) {
	root := setupIntegerPlanRepo(t)
	originalFetch := apkindexFetch
	apkindexFetch = func(string, string, time.Duration) ([]apkindex.Package, error) {
		return nil, errTemporaryAPKIndexOutage
	}
	t.Cleanup(func() { apkindexFetch = originalFetch })

	_, err := PlanIntegerPR(&IntegerPROptions{
		ChangedFiles: []string{"images/node.yaml"},
		ConfigPath:   filepath.Join(root, "integer.yaml"),
		ImagesDir:    filepath.Join(root, "images"),
		APKIndexURL:  "https://example.invalid/APKINDEX.tar.gz",
		GenDir:       filepath.Join(root, "gen"),
	})
	require.ErrorIs(t, err, errTemporaryAPKIndexOutage)
}

func TestPlanCopaPRDiffsSemanticImageEntries(t *testing.T) {
	root := t.TempDir()
	base := filepath.Join(root, "base-copa.yaml")
	head := filepath.Join(root, "copa.yaml")
	writeTestFile(t, base, `
target:
  registry: ghcr.io/test-org
images:
  - name: library/nginx
    image: mirror.gcr.io/library/nginx
    tags:
      strategy: list
      list: ["1.28.0"]
  - name: library/rabbitmq
    image: mirror.gcr.io/library/rabbitmq
    tags:
      strategy: list
      list: ["4.1.0"]
`)
	writeTestFile(t, head, `
target:
  registry: ghcr.io/test-org
images:
  - name: library/nginx
    image: mirror.gcr.io/library/nginx
    tags:
      strategy: list
      list: ["1.29.0"]
  - name: library/rabbitmq
    image: mirror.gcr.io/library/rabbitmq
    tags:
      strategy: list
      list: ["4.1.0"]
`)

	plan, err := PlanCopaPR(&CopaPROptions{
		ChangedFiles:   []string{"copa-config.yaml"},
		BaseConfigPath: base,
		HeadConfigPath: head,
		TargetRegistry: "ghcr.io/test-org",
	})
	require.NoError(t, err)

	assert.True(t, plan.HasChanges)
	assert.Equal(t, []map[string]string{{"name": "library/nginx", "tag": "1.29.0"}}, plan.Matrix.Include)
}

func TestPlanChartsMarksChangedDependencyStrict(t *testing.T) {
	root := t.TempDir()
	base := filepath.Join(root, "base-Chart.yaml")
	head := filepath.Join(root, "Chart.yaml")
	writeTestFile(t, base, `
dependencies:
  - name: grafana
    version: "10.0.0"
    repository: "https://grafana.github.io/helm-charts"
  - name: loki
    version: "7.0.0"
    repository: "https://grafana.github.io/helm-charts"
`)
	writeTestFile(t, head, `
dependencies:
  - name: grafana
    version: "10.1.0"
    repository: "https://grafana.github.io/helm-charts"
  - name: loki
    version: "7.0.0"
    repository: "https://grafana.github.io/helm-charts"
`)

	plan, err := PlanCharts(&ChartOptions{
		EventName:      "pull_request",
		ChangedFiles:   []string{"Chart.yaml"},
		ChartsFile:     head,
		BaseChartsFile: base,
	})
	require.NoError(t, err)

	assert.True(t, plan.Strict)
	assert.Equal(t, []map[string]string{{"chart": "grafana"}}, plan.Matrix.Include)
}

func TestPlanChartsMapsChangedImageThroughValuesFile(t *testing.T) {
	root := t.TempDir()
	charts := filepath.Join(root, "Chart.yaml")
	valuesDir := filepath.Join(root, "values")
	writeTestFile(t, charts, `
dependencies:
  - name: grafana
    version: "10.0.0"
    repository: "https://grafana.github.io/helm-charts"
  - name: loki
    version: "7.0.0"
    repository: "https://grafana.github.io/helm-charts"
`)
	writeTestFile(t, filepath.Join(valuesDir, "grafana.yaml"), "image: ghcr.io/verity-org/grafana:12\n")

	plan, err := PlanCharts(&ChartOptions{
		EventName:    "pull_request",
		ChangedFiles: []string{"images/grafana.yaml"},
		ChartsFile:   charts,
		ValuesDir:    valuesDir,
	})
	require.NoError(t, err)

	assert.False(t, plan.Strict)
	assert.Equal(t, []map[string]string{{"chart": "grafana"}}, plan.Matrix.Include)
}
