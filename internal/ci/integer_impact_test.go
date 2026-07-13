package ci

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPlanIntegerPRUnrelatedInternalChangesDoNotFanOut(t *testing.T) {
	root := setupIntegerPlanRepo(t)

	for _, changed := range []string{
		"internal/ci/plan.go",
		"internal/ci/plan_test.go",
		"internal/integer/config/loader.go",
		"cmd/integer.go",
	} {
		t.Run(changed, func(t *testing.T) {
			plan, err := PlanIntegerPR(integerPlanOptions(root, changed))
			require.NoError(t, err)
			assert.False(t, plan.HasChanges)
			assert.Empty(t, plan.Matrix.Include)
			assert.Empty(t, plan.SmokeMatrix.Include)
		})
	}
}

func TestPlanIntegerPRPinningToolingSelectsConstrainedMelangeCanary(t *testing.T) {
	root := setupIntegerPlanRepo(t)

	for _, changed := range []string{
		"cmd/integer_build.go",
		"cmd/integer_build_melange.go",
		"cmd/integer_melange.go",
		"internal/integer/apkindex/fetch.go",
		"internal/integer/apkindex/package_spec.go",
		"internal/integer/apkindex/parser.go",
		"internal/integer/apkindex/versions.go",
		"internal/integer/melange/pin_config.go",
		".github/workflows/integer-build-image.yaml",
		".github/workflows/pr-test.yaml",
	} {
		t.Run(changed, func(t *testing.T) {
			plan, err := PlanIntegerPR(integerPlanOptions(root, changed))
			require.NoError(t, err)
			require.True(t, plan.HasChanges)
			expected := []map[string]string{{"image": "linkerd", "version": "25", "type": "default"}}
			assert.Equal(t, expected, plan.Matrix.Include)
			assert.Equal(t, expected, plan.SmokeMatrix.Include)
		})
	}
}

func TestPlanIntegerPRMelangeChangesBuildAndSmokeEveryConsumer(t *testing.T) {
	root := setupIntegerPlanRepo(t)

	tests := map[string]struct {
		changed      string
		image        string
		imageType    string
		buildVersion string
		builds       int
		smokes       int
	}{
		"locked recipe":  {"packages/bespoke/locked/caddy.yaml", "caddy", "fips", "2", 1, 2},
		"locked sidecar": {"packages/bespoke/locked/caddy/Caddyfile", "caddy", "fips", "2", 1, 2},
		"override":       {"packages/overrides/fips.env", "caddy", "fips", "2", 1, 2},
		"pipeline":       {"packages/pipelines/go/bump.yaml", "caddy", "fips", "2", 1, 2},
		"other pipeline": {"packages/pipelines/test/ver-check.yaml", "cilium", "default", "1.19", 1, 1},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			plan, err := PlanIntegerPR(integerPlanOptions(root, test.changed))
			require.NoError(t, err)
			require.True(t, plan.HasChanges)
			assert.Len(t, plan.Matrix.Include, test.builds)
			assert.Len(t, plan.SmokeMatrix.Include, test.smokes)
			for _, entry := range plan.Matrix.Include {
				assert.Equal(t, test.image, entry["image"])
				assert.Equal(t, test.imageType, entry["type"])
				assert.Equal(t, test.buildVersion, entry["version"])
			}
			for _, entry := range plan.SmokeMatrix.Include {
				assert.Equal(t, test.image, entry["image"])
				assert.Equal(t, test.imageType, entry["type"])
			}
		})
	}
}

func TestPlanIntegerPRRecipeImpactUsesDiscoveredDefinitionPath(t *testing.T) {
	// Given: an image definition whose nested file path does not match its name.
	root := setupIntegerPlanRepo(t)
	writeTestFile(t, filepath.Join(root, "images", "platform", "custom-file.yaml"), `
name: renamed-image
description: renamed image
upstream:
  package: renamed-image
types:
  default:
    base: wolfi-base
    packages: ["renamed-image"]
    melange:
      bespoke: renamed-image.yaml
versions:
  latest: {}
`)
	writeTestFile(t, filepath.Join(root, "packages", "bespoke", "renamed-image.yaml"), "pipeline: []\n")

	// When: the bespoke recipe changes.
	plan, err := PlanIntegerPR(integerPlanOptions(root, "packages/bespoke/renamed-image.yaml"))

	// Then: planning uses the path discovered from the filesystem rather than reconstructing it from the image name.
	require.NoError(t, err)
	require.True(t, plan.HasChanges)
	assert.Equal(t, []map[string]string{{"image": "renamed-image", "version": "latest", "type": "default"}}, plan.Matrix.Include)
	assert.Equal(t, plan.Matrix.Include, plan.SmokeMatrix.Include)
}

func TestPlanIntegerPRChangedDefinitionUsesDiscoveredName(t *testing.T) {
	// Given: a nested definition whose file path and declared name differ.
	root := setupIntegerPlanRepo(t)
	writeTestFile(t, filepath.Join(root, "images", "platform", "custom-file.yaml"), `
name: renamed-image
description: renamed image
upstream:
  package: renamed-image
types:
  default:
    base: wolfi-base
    packages: ["renamed-image"]
versions:
  latest: {}
`)

	// When: the definition file changes directly.
	plan, err := PlanIntegerPR(integerPlanOptions(root, "images/platform/custom-file.yaml"))

	// Then: the planner selects the declared image name.
	require.NoError(t, err)
	require.True(t, plan.HasChanges)
	assert.Equal(t, []map[string]string{{"image": "renamed-image", "version": "latest", "type": "default"}}, plan.Matrix.Include)
	assert.Equal(t, plan.Matrix.Include, plan.SmokeMatrix.Include)
}

func TestPlanIntegerPRUnusedPipelineDoesNotSelectImages(t *testing.T) {
	root := setupIntegerPlanRepo(t)
	plan, err := PlanIntegerPR(integerPlanOptions(root, "packages/pipelines/test/unused.yaml"))
	require.NoError(t, err)
	assert.False(t, plan.HasChanges)
}

func TestPlanIntegerPRLockDiffSelectsOnlyChangedPackageConsumer(t *testing.T) {
	root := setupIntegerPlanRepo(t)
	baseLock := filepath.Join(root, "base-upstream.lock.json")
	writeTestFile(t, baseLock, `
{
  "packages": {
    "caddy": {"file": "caddy.yaml", "sha256": "caddy-recipe", "assets": {"caddy/Caddyfile": "caddyfile"}},
    "cilium-1.19": {"file": "cilium-1.19.yaml", "sha256": "old-cilium-recipe", "assets": {}},
    "envoy-1.2": {"file": "envoy-1.2.yaml", "sha256": "envoy-recipe"}
  },
  "pipeline_files": {
    "build/wrapper.yaml": "wrapper",
    "go/bump.yaml": "go-bump",
    "test/ver-check.yaml": "ver-check",
    "test/unused.yaml": "unused"
  }
}
`)
	opts := integerPlanOptions(root, "packages/upstream.lock.json")
	opts.BaseLockPath = baseLock

	plan, err := PlanIntegerPR(opts)
	require.NoError(t, err)
	require.True(t, plan.HasChanges)
	assert.Equal(t, []map[string]string{{"image": "cilium", "version": "1.19", "type": "default"}}, plan.Matrix.Include)
	assert.Equal(t, plan.Matrix.Include, plan.SmokeMatrix.Include)
}

func TestPlanIntegerPRLockChangeRequiresBaseForImpactDiff(t *testing.T) {
	root := setupIntegerPlanRepo(t)
	_, err := PlanIntegerPR(integerPlanOptions(root, "packages/upstream.lock.json"))
	require.ErrorIs(t, err, errBaseIntegerLockRequired)
}

func TestPlanIntegerPRChangedImageAndPinningToolingIncludesCanary(t *testing.T) {
	root := setupIntegerPlanRepo(t)
	plan, err := PlanIntegerPR(integerPlanOptions(root, "images/node.yaml", "internal/integer/melange/build.go"))
	require.NoError(t, err)
	require.True(t, plan.HasChanges)
	assert.ElementsMatch(t, []map[string]string{
		{"image": "linkerd", "version": "25", "type": "default"},
		{"image": "node", "version": "22", "type": "default"},
		{"image": "node", "version": "22", "type": "dev"},
	}, plan.Matrix.Include)
	assert.Len(t, plan.SmokeMatrix.Include, 5)
}

func TestPlanIntegerPRDirectBespokeRecipeSelectsConsumer(t *testing.T) {
	root := setupIntegerPlanRepo(t)
	writeTestFile(t, filepath.Join(root, "images", "custom.yaml"), `
name: custom
description: custom
upstream:
  package: custom
types:
  default:
    base: wolfi-base
    packages: ["custom"]
    melange:
      bespoke: custom.yaml
versions:
  latest: {}
`)
	writeTestFile(t, filepath.Join(root, "packages", "bespoke", "custom.yaml"), "pipeline: []\n")

	plan, err := PlanIntegerPR(integerPlanOptions(root, "packages/bespoke/custom.yaml"))
	require.NoError(t, err)
	require.True(t, plan.HasChanges)
	assert.Equal(t, []map[string]string{{"image": "custom", "version": "latest", "type": "default"}}, plan.Matrix.Include)
	assert.Equal(t, plan.Matrix.Include, plan.SmokeMatrix.Include)
}

func integerPlanOptions(root string, changed ...string) *IntegerPROptions {
	return &IntegerPROptions{
		ChangedFiles: changed,
		RepoRoot:     root,
		ConfigPath:   filepath.Join(root, "integer.yaml"),
		ImagesDir:    filepath.Join(root, "images"),
		APKIndexURL:  "",
		GenDir:       filepath.Join(root, "gen"),
	}
}
