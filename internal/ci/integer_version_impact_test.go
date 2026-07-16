package ci

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/verity-org/verity/internal/integer/apkindex"
	intdiscovery "github.com/verity-org/verity/internal/integer/discovery"
)

func TestSelectIntegerPRImagesSharedDefinitionChangeRemainsFamilyWideWithScopedImpact(t *testing.T) {
	// Given: a shared definition field and two version-scoped recipes change together.
	images := make([]intdiscovery.DiscoveredImage, 0, 7)
	for _, version := range []string{"12", "13", "14", "15", "16", "17", "18"} {
		images = append(images, intdiscovery.DiscoveredImage{Name: "postgres", Version: version, Type: "default"})
	}
	changedImages := map[string]struct{}{"postgres": {}}
	impacts := integerVariantImpacts{
		{image: "postgres", version: "14", imageType: "default"}: integerImpactVersion,
		{image: "postgres", version: "15", imageType: "default"}: integerImpactVersion,
	}

	// When: selecting required builds and smokes.
	builds, smokes := selectIntegerPRImages(images, changedImages, impacts, false)

	// Then: the shared definition change still fans out, regardless of the narrower recipe impact.
	require.Equal(t, []string{"18"}, imageVersions(builds))
	require.Equal(t, []string{"12", "13", "14", "15", "16", "17"}, imageVersions(smokes))
}

func TestPlanIntegerPRSharedDefinitionChangeRemainsFamilyWideWithScopedRecipes(t *testing.T) {
	// Given: the scoped 14/15 recipe change also edits a shared image description.
	opts := versionScopedPostgresPlanOptions(t)
	definitionPath := filepath.Join(opts.ImagesDir, "scoped-postgres.yaml")
	definition, err := os.ReadFile(definitionPath)
	require.NoError(t, err)
	writeTestFile(t, definitionPath, strings.Replace(string(definition), "description: scoped postgres", "description: changed shared postgres", 1))

	// When: planning through semantic base-versus-head definition impact.
	plan, err := PlanIntegerPR(opts)

	// Then: the shared definition edit keeps family-wide coverage, with the
	// latest strict build removed from the smoke-only matrix.
	require.NoError(t, err)
	require.Equal(t, []string{"18"}, matrixVersions(plan.Matrix))
	require.Equal(t, []string{"12", "13", "14", "15", "16", "17"}, matrixVersions(*plan.SmokeMatrix))
}

func TestPlanIntegerPRVersionScopedChangesSelectExactStrictBuildConsumers(t *testing.T) {
	// Given: versions 14 and 15 gain scoped recipes while discovery also finds stale 12/13 and latest 18.
	opts := versionScopedPostgresPlanOptions(t)

	// When: the definition and both scoped recipes change together.
	plan, err := PlanIntegerPR(opts)

	// Then: the strict matrix receives exactly the changed consumers and the
	// smoke-only matrix does not duplicate them.
	require.NoError(t, err)
	require.True(t, plan.HasChanges)
	want := []map[string]string{
		{"image": "scoped-postgres", "version": "14", "type": "default"},
		{"image": "scoped-postgres", "version": "15", "type": "default"},
	}
	assert.Equal(t, want, plan.Matrix.Include)
	assert.Empty(t, plan.SmokeMatrix.Include)
}

func TestPlanIntegerPRVersionScopedChangesDoNotSubstituteUnrelatedVersions(t *testing.T) {
	// Given: discovery returns unsupported historical streams and a newer unmodified stream.
	opts := versionScopedPostgresPlanOptions(t)

	// When: planning the scoped 14/15 change.
	plan, err := PlanIntegerPR(opts)

	// Then: 12/13 are not added to smoke and 18 does not replace the required builds.
	require.NoError(t, err)
	for _, version := range []string{"12", "13", "18"} {
		unexpected := map[string]string{"image": "scoped-postgres", "version": version, "type": "default"}
		assert.NotContains(t, plan.Matrix.Include, unexpected)
		assert.NotContains(t, plan.SmokeMatrix.Include, unexpected)
	}
}

func versionScopedPostgresPlanOptions(t *testing.T) *IntegerPROptions {
	t.Helper()
	root := setupIntegerPlanRepo(t)
	baseImagesDir := filepath.Join(root, "base-images")
	writeTestFile(t, filepath.Join(baseImagesDir, "scoped-postgres.yaml"), `
name: scoped-postgres
description: scoped postgres
upstream:
  package: scoped-postgres-{{version}}
types:
  default:
    base: wolfi-base
    packages: ["scoped-postgres-{{version}}"]
versions:
  "14": {}
  "15": {}
  "16": {}
  "17": {}
  "18": {}
`)
	writeTestFile(t, filepath.Join(root, "images", "scoped-postgres.yaml"), `
name: scoped-postgres
description: scoped postgres
upstream:
  package: scoped-postgres-{{version}}
types:
  default:
    base: wolfi-base
    packages: ["scoped-postgres-{{version}}"]
versions:
  "14":
    melange:
      default:
        bespoke: scoped-postgres-14.yaml
  "15":
    melange:
      default:
        bespoke: scoped-postgres-15.yaml
  "16": {}
  "17": {}
  "18": {}
`)
	for _, version := range []string{"14", "15"} {
		writeTestFile(t, filepath.Join(root, "packages", "bespoke", "scoped-postgres-"+version+".yaml"), "pipeline: []\n")
	}

	originalFetch := apkindexFetch
	apkindexFetch = func(string, string, time.Duration) ([]apkindex.Package, error) {
		return []apkindex.Package{
			{Name: "scoped-postgres-12"},
			{Name: "scoped-postgres-13"},
			{Name: "scoped-postgres-14"},
			{Name: "scoped-postgres-15"},
			{Name: "scoped-postgres-16"},
			{Name: "scoped-postgres-17"},
			{Name: "scoped-postgres-18"},
		}, nil
	}
	t.Cleanup(func() { apkindexFetch = originalFetch })

	opts := integerPlanOptions(
		root,
		"images/scoped-postgres.yaml",
		"packages/bespoke/scoped-postgres-14.yaml",
		"packages/bespoke/scoped-postgres-15.yaml",
	)
	opts.APKIndexURL = "test://apkindex"
	opts.BaseImagesDir = baseImagesDir
	return opts
}

func imageVersions(images []intdiscovery.DiscoveredImage) []string {
	versions := make([]string, 0, len(images))
	for _, image := range images {
		versions = append(versions, image.Version)
	}
	return versions
}

func matrixVersions(matrix Matrix) []string {
	versions := make([]string, 0, len(matrix.Include))
	for _, entry := range matrix.Include {
		versions = append(versions, entry["version"])
	}
	return versions
}
