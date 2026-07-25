package publication

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMarshalCanonical_rejects_upsert_digest_that_differs_from_component(t *testing.T) {
	// Given an upsert whose digest differs from its referenced component artifact.
	manifest := testManifest(ModeBootstrap, "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	manifest.APKOperations[0].ArtifactDigest = "sha256:3333333333333333333333333333333333333333333333333333333333333333"

	// When the manifest is validated for canonical encoding.
	_, err := MarshalCanonical(&manifest)

	// Then the unauthenticated operation digest is rejected.
	require.ErrorIs(t, err, ErrInvalidManifest)
}

func TestMarshalCanonical_rejects_upsert_linked_to_non_APK_component(t *testing.T) {
	// Given an upsert linked to a chart artifact with an otherwise matching digest.
	manifest := testManifest(ModeBootstrap, "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	manifest.Components[0].Name = "charts"
	manifest.Components[0].Kind = ComponentKindGeneric
	manifest.Components[0].Architecture = ""
	manifest.Components[0].ArtifactName = "charts-publication-42-3"
	manifest.Components[0].Workflow = ".github/workflows/chart-gen.yaml"
	manifest.APKOperations[0].ArtifactName = "charts-publication-42-3"

	// When the manifest is validated for canonical encoding.
	_, err := MarshalCanonical(&manifest)

	// Then non-APK producer artifacts cannot authorize APK operations.
	require.ErrorIs(t, err, ErrInvalidManifest)
}

func TestMarshalCanonical_accepts_exact_APK_linkage_across_components_and_architectures(t *testing.T) {
	// Given two architecture-specific APK components, an unrelated component, and a removal.
	manifest := multiComponentManifest()

	// When the manifest is validated for canonical encoding.
	_, err := MarshalCanonical(&manifest)

	// Then each matching upsert is accepted and the unchanged remove needs no component link.
	require.NoError(t, err)
}

func TestMarshalCanonical_rejects_missing_ambiguous_and_cross_component_linkage(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*Manifest)
	}{
		{
			name: "missing component artifact",
			mutate: func(manifest *Manifest) {
				manifest.APKOperations[0].ArtifactName = "missing-apk-artifact"
			},
		},
		{
			name: "ambiguous component artifact",
			mutate: func(manifest *Manifest) {
				duplicate := manifest.Components[0]
				duplicate.Name = "integer-x86-copy"
				manifest.Components = append(manifest.Components, duplicate)
			},
		},
		{
			name: "digest from different architecture component",
			mutate: func(manifest *Manifest) {
				manifest.APKOperations[1].ArtifactDigest = manifest.Components[0].ArtifactDigest
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Given an otherwise exact multi-component manifest.
			manifest := multiComponentManifest()
			tt.mutate(&manifest)

			// When the manifest is validated, then linkage ambiguity or mismatch fails closed.
			_, err := MarshalCanonical(&manifest)
			require.ErrorIs(t, err, ErrInvalidManifest)
		})
	}
}

func TestMarshalCanonical_rejects_cross_architecture_component_swap(t *testing.T) {
	// Given valid x86 and arm upserts whose complete component links are swapped.
	manifest := multiComponentManifest()
	x86Component := manifest.Components[0]
	armComponent := manifest.Components[1]
	manifest.APKOperations[0].ArtifactName = armComponent.ArtifactName
	manifest.APKOperations[0].ArtifactDigest = armComponent.ArtifactDigest
	manifest.APKOperations[1].ArtifactName = x86Component.ArtifactName
	manifest.APKOperations[1].ArtifactDigest = x86Component.ArtifactDigest

	// When the manifest is validated for canonical encoding.
	_, err := MarshalCanonical(&manifest)

	// Then exact name+digest links cannot cross architecture ownership.
	require.ErrorIs(t, err, ErrInvalidManifest)
}

func multiComponentManifest() Manifest {
	manifest := testManifest(ModeBootstrap, "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	manifest.Components = append(
		manifest.Components,
		Component{
			Name: "integer-arm64", Kind: ComponentKindAPK,
			Architecture:   ArchitectureAArch64,
			ArtifactName:   "integer-arm64-publication-42-3",
			ArtifactDigest: "sha256:2222222222222222222222222222222222222222222222222222222222222222",
			Workflow:       ".github/workflows/integer-orchestrator.yaml",
			Event:          EventWorkflowCall, Result: ResultSuccess,
		},
		Component{
			Name: "charts", Kind: ComponentKindGeneric,
			ArtifactName:   "charts-publication-42-3",
			ArtifactDigest: "sha256:4444444444444444444444444444444444444444444444444444444444444444",
			Workflow:       ".github/workflows/chart-gen.yaml",
			Event:          EventWorkflowCall, Result: ResultSuccess,
		},
	)
	manifest.APKOperations = append(
		manifest.APKOperations,
		APKOperation{
			Action: APKUpsert, Architecture: ArchitectureAArch64, PackageName: "demo-arm64",
			ArtifactName:   "integer-arm64-publication-42-3",
			ArtifactDigest: "sha256:2222222222222222222222222222222222222222222222222222222222222222",
		},
		APKOperation{Action: APKRemove, Architecture: ArchitectureX8664, PackageName: "retired"},
	)
	return manifest
}
