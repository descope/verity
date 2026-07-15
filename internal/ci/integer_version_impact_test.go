package ci

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/verity-org/verity/internal/integer/apkindex"
)

func TestPlanIntegerPRVersionScopedChangesSelectExactPackageBuildAndSmokeConsumers(t *testing.T) {
	// Given: versions 14 and 15 gain scoped recipes while discovery also finds stale 12/13 and latest 18.
	opts := versionScopedPostgresPlanOptions(t)

	// When: the definition and both scoped recipes change together.
	plan, err := PlanIntegerPR(opts)

	// Then: package, image build, and smoke gates receive exactly the changed consumers.
	require.NoError(t, err)
	require.True(t, plan.HasChanges)
	want := []map[string]string{
		{"image": "scoped-postgres", "version": "14", "type": "default"},
		{"image": "scoped-postgres", "version": "15", "type": "default"},
	}
	assert.Equal(t, want, plan.Matrix.Include)
	assert.Equal(t, want, plan.SmokeMatrix.Include)
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
	return opts
}
