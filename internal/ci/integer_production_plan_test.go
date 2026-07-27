package ci

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testSourceSHA = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

func TestPlanIntegerProduction_selectsAffectedTargets_forEveryProductionPathClass(t *testing.T) {
	tests := []struct {
		name        string
		changed     string
		wantMode    IntegerBatchMode
		wantTargets []string
	}{
		{name: "bespoke recipe", changed: "packages/bespoke/linkerd2-cli-25.yaml", wantMode: IntegerBatchModeDelta, wantTargets: []string{"linkerd:25-default"}},
		{name: "locked recipe", changed: "packages/bespoke/locked/cilium-1.19.yaml", wantMode: IntegerBatchModeDelta, wantTargets: []string{"cilium:1.19-default"}},
		{name: "locked asset", changed: "packages/bespoke/locked/caddy/Caddyfile", wantMode: IntegerBatchModeDelta, wantTargets: []string{"caddy:1-fips", "caddy:2-fips"}},
		{name: "shared pipeline", changed: "packages/pipelines/go/bump.yaml", wantMode: IntegerBatchModeDelta, wantTargets: []string{"caddy:1-fips", "caddy:2-fips"}},
		{name: "package override", changed: "packages/overrides/fips.env", wantMode: IntegerBatchModeDelta, wantTargets: []string{"caddy:1-fips", "caddy:2-fips"}},
		{name: "image definition", changed: "images/node.yaml", wantMode: IntegerBatchModeDelta, wantTargets: []string{"node:20-default", "node:20-dev", "node:22-default", "node:22-dev"}},
		{name: "Integer config", changed: "integer.yaml", wantMode: IntegerBatchModeSnapshot, wantTargets: allFixtureTargets()},
		{name: "Integer implementation", changed: "internal/integer/melange/build.go", wantMode: IntegerBatchModeSnapshot, wantTargets: allFixtureTargets()},
		{name: "Integer CI policy", changed: "internal/ci/integer_impact.go", wantMode: IntegerBatchModeSnapshot, wantTargets: allFixtureTargets()},
		{name: "Integer CLI", changed: "cmd/integer_build.go", wantMode: IntegerBatchModeSnapshot, wantTargets: allFixtureTargets()},
		{name: "Go entrypoint", changed: "main.go", wantMode: IntegerBatchModeSnapshot, wantTargets: allFixtureTargets()},
		{name: "Go module", changed: "go.mod", wantMode: IntegerBatchModeSnapshot, wantTargets: allFixtureTargets()},
		{name: "tool lock", changed: "mise.lock", wantMode: IntegerBatchModeSnapshot, wantTargets: allFixtureTargets()},
		{name: "orchestrator workflow", changed: ".github/workflows/integer-orchestrator.yaml", wantMode: IntegerBatchModeSnapshot, wantTargets: allFixtureTargets()},
		{name: "shard workflow", changed: ".github/workflows/integer-build-shard.yaml", wantMode: IntegerBatchModeSnapshot, wantTargets: allFixtureTargets()},
		{name: "image workflow", changed: ".github/workflows/integer-build-image.yaml", wantMode: IntegerBatchModeSnapshot, wantTargets: allFixtureTargets()},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Given: a complete Integer fixture and one production input change.
			root := setupIntegerProductionRepo(t)
			options := integerProductionOptions(root, IntegerBatchEventPush, test.changed)

			// When: the production batch is planned through Go impact analysis.
			plan, err := PlanIntegerProduction(&options)

			// Then: only the intended targets are selected, while broad code/workflow
			// changes fail safe to a complete snapshot.
			require.NoError(t, err)
			assert.Equal(t, test.wantMode, plan.Mode)
			assert.Equal(t, test.wantTargets, integerTargetIDs(plan.Targets))
		})
	}
}

func TestPlanIntegerProduction_computesLockDelta_fromBaseAndHead(t *testing.T) {
	// Given: a lock-only change to one consumed recipe.
	root := setupIntegerProductionRepo(t)
	baseLock := filepath.Join(root, "base-upstream.lock.json")
	headLock := filepath.Join(root, "packages", "upstream.lock.json")
	data, err := os.ReadFile(headLock)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(baseLock, data, 0o600))
	updated := strings.Replace(string(data), `"sha256": "cilium-recipe"`, `"sha256": "cilium-recipe-updated"`, 1)
	require.NoError(t, os.WriteFile(headLock, []byte(updated), 0o600))
	options := integerProductionOptions(root, IntegerBatchEventPush, "packages/upstream.lock.json")
	options.BaseLockPath = baseLock

	// When: the lock delta is planned.
	plan, err := PlanIntegerProduction(&options)

	// Then: only the changed locked recipe consumer and its two architecture
	// package declarations are present.
	require.NoError(t, err)
	assert.Equal(t, []string{"cilium:1.19-default"}, integerTargetIDs(plan.Targets))
	assert.Equal(t, []string{"aarch64/cilium-1.19", "x86_64/cilium-1.19"}, integerPackageIDs(plan.Packages))
}

func TestPlanIntegerProduction_scheduleEnumeratesCompleteUniquePackageSnapshot(t *testing.T) {
	// Given: every fixture image, including shared recipes and subpackages.
	root := setupIntegerProductionRepo(t)
	options := integerProductionOptions(root, IntegerBatchEventSchedule)

	// When: the scheduled production snapshot is planned.
	plan, err := PlanIntegerProduction(&options)

	// Then: every build target and every unique package for both architectures
	// is declared exactly once with one canonical producer.
	require.NoError(t, err)
	assert.Equal(t, IntegerBatchModeSnapshot, plan.Mode)
	assert.Equal(t, allFixtureTargets(), integerTargetIDs(plan.Targets))
	assert.Equal(t, []string{
		"aarch64/caddy", "aarch64/caddy-tools", "aarch64/cilium-1.19", "aarch64/envoy-1.2", "aarch64/linkerd2-cli-25",
		"x86_64/caddy", "x86_64/caddy-tools", "x86_64/cilium-1.19", "x86_64/envoy-1.2", "x86_64/linkerd2-cli-25",
	}, integerPackageIDs(plan.Packages))
	artifactKeys := map[string]struct{}{}
	for index := range plan.Targets {
		target := &plan.Targets[index]
		assert.Regexp(t, `^[A-Za-z0-9._-]+-[0-9a-f]{12}$`, target.ArtifactKey)
		_, duplicate := artifactKeys[target.ArtifactKey]
		assert.False(t, duplicate, target.ArtifactKey)
		artifactKeys[target.ArtifactKey] = struct{}{}
	}
	for _, pkg := range plan.Packages {
		assert.NotEmpty(t, pkg.Producer)
	}
}

func TestPlanIntegerProduction_realCraneCoalescesEquivalentPackageDeclarations(t *testing.T) {
	// Given: the repository's real crane definition, whose default and FIPS
	// recipes intentionally declare the same main APK identity.
	repoRoot := filepath.Clean(filepath.Join("..", ".."))
	options := IntegerProductionOptions{
		Event: IntegerBatchEventSchedule, SourceSHA: testSourceSHA, RunID: 42, RunAttempt: 3,
		PublicationID: "integer-publication-42-3", BatchID: "42-3", Only: []string{"crane"},
		RepoRoot: repoRoot, ConfigPath: filepath.Join(repoRoot, "integer.yaml"),
		ImagesDir: filepath.Join(repoRoot, "images"), APKIndexURL: "", CacheDir: t.TempDir(), GenDir: t.TempDir(),
	}

	// When: the real scheduled production plan is built.
	plan, err := PlanIntegerProduction(&options)

	// Then: the common crane declaration has one deterministic owner while
	// the locked recipe's distinct subpackages remain in the exact snapshot.
	require.NoError(t, err)
	assert.Equal(t, []string{"crane:0.21.7-default", "crane:0.21.7-fips"}, integerTargetIDs(plan.Targets))
	assert.Equal(t, []string{
		"aarch64/crane", "aarch64/crane-cov", "aarch64/krane",
		"x86_64/crane", "x86_64/crane-cov", "x86_64/krane",
	}, integerPackageIDs(plan.Packages))
	assert.Equal(t, []string{"crane"}, plan.Targets[0].PublishPackages)
	assert.Equal(t, []string{"crane-cov", "krane"}, plan.Targets[1].PublishPackages)
}

func TestPlanIntegerProduction_rejectsConflictingDuplicatePackageDeclarations(t *testing.T) {
	// Given: two recipes declare the same package name with different package
	// metadata, so they cannot represent one canonical APK identity.
	root := setupIntegerProductionRepo(t)
	writeTestFile(t, filepath.Join(root, "images", "conflicting-caddy.yaml"), `
name: conflicting-caddy
description: conflicting caddy declaration
upstream:
  package: caddy
types:
  default:
    base: wolfi-base
    packages: ["caddy"]
    melange:
      bespoke: conflicting-caddy.yaml
versions:
  "1": {}
`)
	writeTestFile(t, filepath.Join(root, "packages", "bespoke", "conflicting-caddy.yaml"), `
package:
  name: caddy
  version: "conflicting"
pipeline: []
`)
	options := integerProductionOptions(root, IntegerBatchEventSchedule)

	// When: the scheduled snapshot is planned.
	_, err := PlanIntegerProduction(&options)

	// Then: conflicting declarations still fail closed.
	require.ErrorIs(t, err, ErrIntegerPackageDuplicate)
	assert.ErrorContains(t, err, "packages/bespoke/conflicting-caddy.yaml")
	assert.ErrorContains(t, err, "packages/bespoke/locked/caddy.yaml")
}

func TestPlanIntegerProduction_recipeChangeEmitsDeclaredDeltaOnly(t *testing.T) {
	// Given: one bespoke recipe change.
	root := setupIntegerProductionRepo(t)
	options := integerProductionOptions(root, IntegerBatchEventPush, "packages/bespoke/linkerd2-cli-25.yaml")

	// When: the recipe delta is planned.
	plan, err := PlanIntegerProduction(&options)

	// Then: the manifest declares only that recipe's two architecture upserts.
	require.NoError(t, err)
	assert.Equal(t, IntegerBatchModeDelta, plan.Mode)
	assert.Equal(t, []string{"linkerd:25-default"}, integerTargetIDs(plan.Targets))
	assert.Equal(t, []string{"aarch64/linkerd2-cli-25", "x86_64/linkerd2-cli-25"}, integerPackageIDs(plan.Packages))
}

func setupIntegerProductionRepo(t *testing.T) string {
	t.Helper()
	root := setupIntegerPlanRepo(t)
	writeTestFile(t, filepath.Join(root, "packages", "bespoke", "locked", "caddy.yaml"), `
package:
  name: caddy
subpackages:
  - name: ${{package.name}}-tools
pipeline:
  - uses: build/wrapper
`)
	writeTestFile(t, filepath.Join(root, "packages", "bespoke", "locked", "cilium-1.19.yaml"), `
package:
  name: cilium-1.19
pipeline:
  - uses: test/ver-check
`)
	writeTestFile(t, filepath.Join(root, "packages", "bespoke", "locked", "envoy-1.2.yaml"), `
package:
  name: envoy-1.2
pipeline: []
`)
	writeTestFile(t, filepath.Join(root, "packages", "bespoke", "linkerd2-cli-25.yaml"), `
package:
  name: linkerd2-cli-25
pipeline: []
`)
	return root
}

func integerProductionOptions(root string, event IntegerBatchEvent, changed ...string) IntegerProductionOptions {
	return IntegerProductionOptions{
		Event:         event,
		SourceSHA:     testSourceSHA,
		RunID:         42,
		RunAttempt:    3,
		PublicationID: "integer-publication-42-3",
		BatchID:       "42-3",
		ChangedFiles:  changed,
		RepoRoot:      root,
		ConfigPath:    filepath.Join(root, "integer.yaml"),
		ImagesDir:     filepath.Join(root, "images"),
		APKIndexURL:   "",
		GenDir:        filepath.Join(root, "gen"),
	}
}

func integerTargetIDs(targets []IntegerBatchTarget) []string {
	ids := make([]string, 0, len(targets))
	for index := range targets {
		ids = append(ids, targets[index].ID())
	}
	slices.Sort(ids)
	return ids
}

func integerPackageIDs(packages []IntegerPlannedPackage) []string {
	ids := make([]string, 0, len(packages))
	for _, pkg := range packages {
		ids = append(ids, string(pkg.Architecture)+"/"+pkg.Name)
	}
	slices.Sort(ids)
	return ids
}

func allFixtureTargets() []string {
	return []string{
		"caddy:1-default", "caddy:1-fips", "caddy:2-default", "caddy:2-fips",
		"cilium:1.19-default", "curl:latest-default", "linkerd:25-default",
		"node:20-default", "node:20-dev", "node:22-default", "node:22-dev",
		"platform/envoy:1.2-default",
	}
}
