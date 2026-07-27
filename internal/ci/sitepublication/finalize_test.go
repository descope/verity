package sitepublication

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/verity-org/verity/internal/ci/publication"
)

func TestFinalizePublication_emits_attestation_plan_and_deploy_eligibility(t *testing.T) {
	// Given an assembled site bound to the current CAS state.
	plan, _, site := assembledFixture(t)
	root := filepath.Dir(site)
	archive := filepath.Join(root, "artifact.tar")

	// When the final artifact is packed and validated.
	finalPlan, err := FinalizePublication(context.Background(), &FinalizeRequest{
		Plan: plan, ExpectedPlanDigest: plan.PlanDigest, SiteDir: site,
		ArchivePath: archive, CurrentManifest: validPlanRequest(t).PreviousManifest,
	})
	require.NoError(t, err)
	err = ValidateFinalArtifact(&finalPlan, archive)

	// Then the artifact is deployable only with an exact digest-bound attestation.
	require.NoError(t, err)
	assert.True(t, finalPlan.DeployEligible)
	assert.Equal(t, PagesArtifactName, finalPlan.ArtifactName)
	assert.Equal(t, finalPlan.ArtifactDigest, finalPlan.Attestation.SubjectDigest)
	assert.Equal(t, BuildSiteWorkflow, finalPlan.Attestation.Workflow)
	assert.Equal(t, plan.ManifestDigest, finalPlan.ManifestDigest)
}

func TestFinalizePublication_rejects_undeclared_site_and_manifest_mutations(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*testing.T, string)
	}{
		{
			name: "site file",
			mutate: func(t *testing.T, site string) {
				writeSiteFile(t, site, "catalog/keep.json", "tampered")
			},
		},
		{
			name: "publication manifest",
			mutate: func(t *testing.T, site string) {
				writeSiteFile(t, site, PublicationManifestPath, "{}")
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Given a completed assembly modified after its file contract was written.
			plan, _, site := assembledFixture(t)
			test.mutate(t, site)

			// When finalization revalidates the exact bytes.
			_, err := FinalizePublication(context.Background(), &FinalizeRequest{
				Plan: plan, ExpectedPlanDigest: plan.PlanDigest, SiteDir: site,
				ArchivePath:     filepath.Join(filepath.Dir(site), "artifact.tar"),
				CurrentManifest: validPlanRequest(t).PreviousManifest,
			})

			// Then post-assembly changes are rejected.
			require.ErrorIs(t, err, ErrUndeclaredMutation)
		})
	}
}

func TestFinalizePublication_rechecks_CAS_for_concurrent_publications(t *testing.T) {
	// Given a valid plan whose base was superseded by a different publication.
	plan, _, site := assembledFixture(t)
	winner := testManifest(publication.ModeDelta, testSourceSHA, 43, 1)
	winner.PreviousManifestDigest = plan.PreviousManifestDigest

	// When the stale candidate reaches the final deploy gate.
	_, err := FinalizePublication(context.Background(), &FinalizeRequest{
		Plan: plan, ExpectedPlanDigest: plan.PlanDigest, SiteDir: site,
		ArchivePath: filepath.Join(filepath.Dir(site), "artifact.tar"), CurrentManifest: &winner,
	})

	// Then compare-and-swap ordering blocks the stale upload.
	require.ErrorIs(t, err, publication.ErrCASMismatch)
}

func TestValidateFinalArtifact_rejects_tampered_archive(t *testing.T) {
	// Given a finalized deterministic artifact.
	plan, _, site := assembledFixture(t)
	archive := filepath.Join(filepath.Dir(site), "artifact.tar")
	finalPlan, err := FinalizePublication(context.Background(), &FinalizeRequest{
		Plan: plan, ExpectedPlanDigest: plan.PlanDigest, SiteDir: site,
		ArchivePath: archive, CurrentManifest: validPlanRequest(t).PreviousManifest,
	})
	require.NoError(t, err)
	file, err := os.OpenFile(archive, os.O_APPEND|os.O_WRONLY, 0)
	require.NoError(t, err)
	_, err = file.WriteString("tamper")
	require.NoError(t, err)
	require.NoError(t, file.Close())

	// When the artifact is checked before upload/deploy.
	err = ValidateFinalArtifact(&finalPlan, archive)

	// Then digest tampering is rejected.
	require.ErrorIs(t, err, ErrArtifactTampered)
}

func assembledFixture(t *testing.T) (PublicationPlan, publication.Manifest, string) {
	t.Helper()
	plan, manifest := validPlanAndManifest(t)
	root := t.TempDir()
	base := filepath.Join(root, "base")
	writeSiteFile(t, base, "index.html", "site")
	writeSiteFile(t, base, "catalog/keep.json", "catalog")
	sealSiteFixture(t, base, validPlanRequest(t).PreviousManifest)
	signedAPK := filepath.Join(root, "apk")
	writeSiteFile(t, signedAPK, "x86_64/demo.apk", "x86")
	writeSiteFile(t, signedAPK, "aarch64/demo.apk", "arm")
	site := filepath.Join(root, "site")
	_, err := AssembleSite(context.Background(), &AssembleRequest{
		Plan: plan, Manifest: manifest, BaseDir: base, SignedAPKDir: signedAPK, OutputDir: site,
	})
	require.NoError(t, err)
	return plan, manifest, site
}
