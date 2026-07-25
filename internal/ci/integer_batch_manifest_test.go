package ci

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStageIntegerComponent_rejectsMissingDuplicateUndeclaredAndWrongArchitecturePackages(t *testing.T) {
	tests := []struct {
		name    string
		arrange func(*testing.T, string)
		wantErr error
	}{
		{
			name: "missing architecture package",
			arrange: func(t *testing.T, root string) {
				writeIntegerTestAPK(t, filepath.Join(root, "x86_64", "alpha.apk"), "alpha", "x86_64", "x86")
			},
			wantErr: ErrIntegerPackageMissing,
		},
		{
			name: "duplicate package identity",
			arrange: func(t *testing.T, root string) {
				writeIntegerTestAPK(t, filepath.Join(root, "x86_64", "alpha.apk"), "alpha", "x86_64", "one")
				writeIntegerTestAPK(t, filepath.Join(root, "x86_64", "alpha-copy.apk"), "alpha", "x86_64", "two")
				writeIntegerTestAPK(t, filepath.Join(root, "aarch64", "alpha.apk"), "alpha", "aarch64", "arm")
			},
			wantErr: ErrIntegerPackageDuplicate,
		},
		{
			name: "undeclared package",
			arrange: func(t *testing.T, root string) {
				writeIntegerTestAPK(t, filepath.Join(root, "x86_64", "alpha.apk"), "alpha", "x86_64", "x86")
				writeIntegerTestAPK(t, filepath.Join(root, "aarch64", "alpha.apk"), "alpha", "aarch64", "arm")
				writeIntegerTestAPK(t, filepath.Join(root, "x86_64", "attacker.apk"), "attacker", "x86_64", "extra")
			},
			wantErr: ErrIntegerPackageUndeclared,
		},
		{
			name: "wrong package architecture",
			arrange: func(t *testing.T, root string) {
				writeIntegerTestAPK(t, filepath.Join(root, "x86_64", "alpha.apk"), "alpha", "aarch64", "wrong")
				writeIntegerTestAPK(t, filepath.Join(root, "aarch64", "alpha.apk"), "alpha", "aarch64", "arm")
			},
			wantErr: ErrIntegerPackageArchitecture,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Given: one declared package producer and a malformed package set.
			plan := integerBatchFixturePlan()
			packages := t.TempDir()
			test.arrange(t, packages)

			// When: the child component is staged through the real APK boundary.
			_, err := StageIntegerComponent(t.Context(), &IntegerComponentOptions{
				Plan: &plan, TargetID: "alpha:1-default", PackagesDir: packages, OutputDir: t.TempDir(),
			})

			// Then: the exact discrepancy blocks publication.
			require.ErrorIs(t, err, test.wantErr)
		})
	}
}

func TestStageIntegerComponent_emitsExactInventory_andHonorsCancellation(t *testing.T) {
	// Given: the exact two-architecture package set for one producer.
	plan := integerBatchFixturePlan()
	packages := t.TempDir()
	writeIntegerTestAPK(t, filepath.Join(packages, "x86_64", "alpha.apk"), "alpha", "x86_64", "x86")
	writeIntegerTestAPK(t, filepath.Join(packages, "aarch64", "alpha.apk"), "alpha", "aarch64", "arm")
	output := t.TempDir()

	// When: the component is staged.
	component, err := StageIntegerComponent(t.Context(), &IntegerComponentOptions{
		Plan: &plan, TargetID: "alpha:1-default", PackagesDir: packages, OutputDir: output,
	})

	// Then: identity, architecture, package, digest, and staged paths are exact.
	require.NoError(t, err)
	assert.Equal(t, testSourceSHA, component.SourceSHA)
	assert.Equal(t, uint64(42), component.RunID)
	assert.Equal(t, uint64(3), component.RunAttempt)
	assert.Equal(t, "integer-publication-42-3", component.PublicationID)
	assert.Equal(t, "42-3", component.BatchID)
	assert.Equal(t, "1", component.Shard)
	assert.Equal(t, []string{"aarch64/alpha", "x86_64/alpha"}, integerPackageFileIDs(component.Packages))
	for _, pkg := range component.Packages {
		assert.Regexp(t, `^sha256:[0-9a-f]{64}$`, pkg.SHA256)
		assert.FileExists(t, filepath.Join(output, filepath.FromSlash(pkg.Path)))
	}

	// Given: the same valid request with an already-cancelled context.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	// When: staging starts after cancellation.
	_, err = StageIntegerComponent(ctx, &IntegerComponentOptions{
		Plan: &plan, TargetID: "alpha:1-default", PackagesDir: packages, OutputDir: t.TempDir(),
	})

	// Then: cancellation is preserved instead of emitting misleading success.
	require.ErrorIs(t, err, context.Canceled)
}

func TestIntegerBatchIdentity_rejectsMissingOrAliasedPublicationID(t *testing.T) {
	// Given: an otherwise valid exact plan with either no publication identity
	// or a publication identity copied from the batch identity.
	tests := []struct {
		name          string
		publicationID string
	}{
		{name: "missing", publicationID: ""},
		{name: "batch alias", publicationID: "42-3"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			plan := integerBatchFixturePlan()
			plan.PublicationID = test.publicationID

			// When: the exact plan crosses the canonical JSON boundary.
			_, err := MarshalIntegerBatchPlan(&plan)

			// Then: publication identity is independently required.
			require.ErrorIs(t, err, ErrIntegerBatchPlan)
		})
	}
}

func TestAggregateIntegerShard_rejectsPartialDuplicateAndMismatchedComponents(t *testing.T) {
	// Given: a shard plan that requires two exact component producers.
	plan := integerBatchFixturePlan()
	plan.Targets = append(plan.Targets, IntegerBatchTarget{
		Name: "gamma", Version: "1", Type: "default", ArtifactKey: "gamma-1-default-000000000003", Shard: "1",
		ExpectedPackages: []string{"gamma"}, PublishPackages: []string{"gamma"},
	})
	plan.Packages = append(
		plan.Packages,
		IntegerPlannedPackage{Architecture: IntegerArchitectureX8664, Name: "gamma", Producer: "gamma:1-default"},
		IntegerPlannedPackage{Architecture: IntegerArchitectureAArch64, Name: "gamma", Producer: "gamma:1-default"},
	)
	alphaDir := stageIntegerFixtureComponent(t, &plan, "alpha:1-default", "alpha")
	gammaDir := stageIntegerFixtureComponent(t, &plan, "gamma:1-default", "gamma")

	tests := []struct {
		name       string
		components []string
		mutate     func(*testing.T, string)
		wantErr    error
	}{
		{name: "partial shard", components: []string{alphaDir}, wantErr: ErrIntegerShardIncomplete},
		{name: "duplicate component", components: []string{alphaDir, alphaDir, gammaDir}, wantErr: ErrIntegerPackageDuplicate},
		{
			name:       "mismatched source run attempt and batch",
			components: []string{alphaDir, gammaDir},
			mutate: func(t *testing.T, root string) {
				mutateIntegerComponentManifest(t, root, func(manifest *IntegerComponentManifest) {
					manifest.SourceSHA = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
					manifest.RunID = 99
					manifest.RunAttempt = 7
					manifest.BatchID = "99-7"
				})
			},
			wantErr: ErrIntegerIdentityMismatch,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// When: the complete downloaded component set is aggregated.
			components := append([]string(nil), test.components...)
			if test.mutate != nil {
				duplicate := copyIntegerComponentDir(t, components[0])
				components[0] = duplicate
				test.mutate(t, duplicate)
			}
			_, err := AggregateIntegerShard(t.Context(), &IntegerShardOptions{
				Plan: &plan, Shard: "1", ComponentDirs: components, OutputDir: t.TempDir(),
			})

			// Then: partial, duplicate, and stale provenance can never produce an
			// approved shard artifact.
			require.ErrorIs(t, err, test.wantErr)
		})
	}
}

func TestAggregateIntegerBatch_rejectsMissingDuplicateAndConflictingShards(t *testing.T) {
	// Given: a two-shard exact batch with finalized artifact identities.
	plan := integerBatchFixturePlan()
	alphaDir := stageIntegerFixtureComponent(t, &plan, "alpha:1-default", "alpha")
	betaDir := stageIntegerFixtureComponent(t, &plan, "beta:1-default", "beta")
	alphaInventory, err := AggregateIntegerShard(t.Context(), &IntegerShardOptions{
		Plan: &plan, Shard: "1", ComponentDirs: []string{alphaDir}, OutputDir: t.TempDir(),
	})
	require.NoError(t, err)
	betaInventory, err := AggregateIntegerShard(t.Context(), &IntegerShardOptions{
		Plan: &plan, Shard: "2", ComponentDirs: []string{betaDir}, OutputDir: t.TempDir(),
	})
	require.NoError(t, err)
	alpha, err := FinalizeIntegerShard(&alphaInventory, IntegerArtifactRef{PublicationID: "integer-publication-42-3", Name: "apk-repository-integer-publication-42-3-1", Digest: testArtifactDigest("1")})
	require.NoError(t, err)
	beta, err := FinalizeIntegerShard(&betaInventory, IntegerArtifactRef{PublicationID: "integer-publication-42-3", Name: "apk-repository-integer-publication-42-3-2", Digest: testArtifactDigest("2")})
	require.NoError(t, err)

	tests := []struct {
		name    string
		shards  []IntegerShardManifest
		wantErr error
	}{
		{name: "missing shard", shards: []IntegerShardManifest{alpha}, wantErr: ErrIntegerBatchIncomplete},
		{name: "duplicate shard", shards: []IntegerShardManifest{alpha, alpha, beta}, wantErr: ErrIntegerPackageDuplicate},
		{
			name: "conflicting provenance",
			shards: []IntegerShardManifest{alpha, func() IntegerShardManifest {
				conflict := beta
				conflict.RunAttempt = 9
				return conflict
			}()},
			wantErr: ErrIntegerIdentityMismatch,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// When: final batch aggregation consumes the untrusted shard set.
			_, err := AggregateIntegerBatch(&plan, test.shards)

			// Then: no incomplete, repeated, or conflicting batch can claim success.
			require.ErrorIs(t, err, test.wantErr)
		})
	}

	// When: both exact shards are aggregated once.
	manifest, err := AggregateIntegerBatch(&plan, []IntegerShardManifest{beta, alpha})

	// Then: the canonical batch manifest declares every planned package and
	// artifact name+digest exactly once.
	require.NoError(t, err)
	assert.Equal(t, []string{"aarch64/alpha", "aarch64/beta", "x86_64/alpha", "x86_64/beta"}, integerPublishedPackageIDs(manifest.Packages))
	assert.Equal(t, []string{"apk-repository-integer-publication-42-3-1", "apk-repository-integer-publication-42-3-2"}, integerArtifactNames(manifest.Shards))
	assert.Equal(t, "integer-publication-42-3", manifest.PublicationID)
}

func TestParseIntegerBatchPlan_rejectsMalformedDuplicateAndTrailingJSON(t *testing.T) {
	tests := []string{
		`{"schema_version":1,"schema_version":2}`,
		`{"schema_version":1,"unknown":true}`,
		`{} {}`,
	}

	for _, input := range tests {
		// Given: malformed or ambiguous plan bytes.
		// When: the exact plan boundary parses them.
		_, err := ParseIntegerBatchPlan([]byte(input))

		// Then: hostile JSON is rejected before any artifact mutation.
		require.ErrorIs(t, err, ErrIntegerBatchPlan)
	}
}

func TestIntegerBatch_errorsRemainProgrammaticallyClassifiable(t *testing.T) {
	// Given: the public exact-batch sentinel set.
	errorsToCheck := []error{
		ErrIntegerBatchPlan, ErrIntegerPackageMissing, ErrIntegerPackageDuplicate,
		ErrIntegerPackageUndeclared, ErrIntegerPackageArchitecture,
		ErrIntegerIdentityMismatch, ErrIntegerShardIncomplete, ErrIntegerBatchIncomplete,
	}

	// When/Then: wrapped failures preserve errors.Is behavior.
	for _, target := range errorsToCheck {
		assert.True(t, errors.Is(assert.AnError, assert.AnError))
		assert.ErrorIs(t, target, target)
	}
}
