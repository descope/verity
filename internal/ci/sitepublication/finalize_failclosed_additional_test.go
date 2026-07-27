package sitepublication

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/verity-org/verity/internal/ci/publication"
)

func TestFinalizePublication_rejects_nil_canceled_and_unbound_requests(t *testing.T) {
	t.Run("nil request", func(t *testing.T) {
		// When
		plan, err := FinalizePublication(context.Background(), nil)

		// Then
		require.ErrorIs(t, err, ErrInvalidFinalPlan)
		assert.Equal(t, FinalPlan{}, plan)
	})

	plan, _, site := assembledFixture(t)
	valid := FinalizeRequest{
		Plan: plan, ExpectedPlanDigest: plan.PlanDigest, SiteDir: site,
		ArchivePath:     filepath.Join(filepath.Dir(site), "artifact.tar"),
		CurrentManifest: validPlanRequest(t).PreviousManifest,
	}
	tests := []struct {
		name    string
		ctx     func() context.Context
		mutate  func(*FinalizeRequest)
		wantErr error
	}{
		{name: "canceled context", ctx: canceledSignerContext, mutate: func(*FinalizeRequest) {}, wantErr: context.Canceled},
		{name: "invalid plan", ctx: context.Background, mutate: func(request *FinalizeRequest) { request.Plan.SchemaVersion = 0 }, wantErr: ErrInvalidPlan},
		{name: "plan digest mismatch", ctx: context.Background, mutate: func(request *FinalizeRequest) { request.ExpectedPlanDigest = digestOf("f") }, wantErr: ErrInvalidFinalPlan},
		{name: "missing site", ctx: context.Background, mutate: func(request *FinalizeRequest) { request.SiteDir = filepath.Join(filepath.Dir(site), "missing-site") }, wantErr: ErrInvalidAssembly},
		{name: "invalid current manifest", ctx: context.Background, mutate: func(request *FinalizeRequest) { request.CurrentManifest = &publication.Manifest{} }, wantErr: publication.ErrInvalidManifest},
		{name: "archive parent missing", ctx: context.Background, mutate: func(request *FinalizeRequest) {
			request.ArchivePath = filepath.Join(filepath.Dir(site), "missing", "artifact.tar")
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := valid
			test.mutate(&request)

			// When
			finalPlan, err := FinalizePublication(test.ctx(), &request)

			// Then
			require.Error(t, err)
			if test.wantErr != nil {
				require.ErrorIs(t, err, test.wantErr)
			}
			assert.Equal(t, FinalPlan{}, finalPlan)
			assert.NoFileExists(t, request.ArchivePath)
		})
	}
}

func TestValidateFinalArtifact_rejects_invalid_plan_fields_before_archive_access(t *testing.T) {
	valid := FinalPlan{
		SchemaVersion: SchemaVersion, ArtifactName: PagesArtifactName, ArtifactPath: "sentinel-artifact.tar",
		ArtifactDigest: digestOf("1"), ManifestDigest: digestOf("2"), SiteDigest: digestOf("3"),
		RunID: 42, RunAttempt: 3, DeployEligible: true,
		Attestation: AttestationPlan{
			SubjectPath: "sentinel-artifact.tar", SubjectDigest: digestOf("1"),
			Workflow: BuildSiteWorkflow, SourceSHA: testSourceSHA,
		},
	}
	tests := []struct {
		name   string
		mutate func(*FinalPlan)
	}{
		{name: "schema", mutate: func(plan *FinalPlan) { plan.SchemaVersion++ }},
		{name: "eligibility", mutate: func(plan *FinalPlan) { plan.DeployEligible = false }},
		{name: "artifact name", mutate: func(plan *FinalPlan) { plan.ArtifactName = "sentinel" }},
		{name: "workflow", mutate: func(plan *FinalPlan) { plan.Attestation.Workflow = "sentinel" }},
		{name: "empty artifact path", mutate: func(plan *FinalPlan) { plan.ArtifactPath = "" }},
		{name: "subject path", mutate: func(plan *FinalPlan) { plan.Attestation.SubjectPath = "other.tar" }},
		{name: "artifact digest", mutate: func(plan *FinalPlan) { plan.ArtifactDigest = "bad" }},
		{name: "manifest digest", mutate: func(plan *FinalPlan) { plan.ManifestDigest = "bad" }},
		{name: "site digest", mutate: func(plan *FinalPlan) { plan.SiteDigest = "bad" }},
		{name: "subject digest", mutate: func(plan *FinalPlan) { plan.Attestation.SubjectDigest = digestOf("f") }},
		{name: "run ID", mutate: func(plan *FinalPlan) { plan.RunID = 0 }},
		{name: "run attempt", mutate: func(plan *FinalPlan) { plan.RunAttempt = 0 }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			plan := valid
			test.mutate(&plan)

			// When
			err := ValidateFinalArtifact(&plan, plan.ArtifactPath)

			// Then
			require.ErrorIs(t, err, ErrInvalidFinalPlan)
		})
	}

	t.Run("nil plan", func(t *testing.T) {
		// When
		err := ValidateFinalArtifact(nil, "sentinel-artifact.tar")

		// Then
		require.ErrorIs(t, err, ErrInvalidFinalPlan)
	})

	t.Run("different archive path", func(t *testing.T) {
		// When
		err := ValidateFinalArtifact(&valid, "other.tar")

		// Then
		require.ErrorIs(t, err, ErrInvalidFinalPlan)
	})
}

func TestValidateFinalArtifact_rejects_site_digest_mismatch_after_archive_verification(t *testing.T) {
	// Given
	plan, _, site := assembledFixture(t)
	archive := filepath.Join(filepath.Dir(site), "artifact.tar")
	finalPlan, err := FinalizePublication(context.Background(), &FinalizeRequest{
		Plan: plan, ExpectedPlanDigest: plan.PlanDigest, SiteDir: site,
		ArchivePath: archive, CurrentManifest: validPlanRequest(t).PreviousManifest,
	})
	require.NoError(t, err)
	finalPlan.SiteDigest = digestOf("f")

	// When
	err = ValidateFinalArtifact(&finalPlan, archive)

	// Then
	require.ErrorIs(t, err, ErrArtifactTampered)
}

func TestFinalPlanCanonical_roundtrips_valid_finalized_output(t *testing.T) {
	// Given
	plan, _, site := assembledFixture(t)
	archive := filepath.Join(filepath.Dir(site), "artifact.tar")
	finalPlan, err := FinalizePublication(context.Background(), &FinalizeRequest{
		Plan: plan, ExpectedPlanDigest: plan.PlanDigest, SiteDir: site,
		ArchivePath: archive, CurrentManifest: validPlanRequest(t).PreviousManifest,
	})
	require.NoError(t, err)

	// When
	data, err := MarshalFinalPlanCanonical(&finalPlan)
	require.NoError(t, err)
	parsed, err := ParseFinalPlanCanonical(data)

	// Then
	require.NoError(t, err)
	assert.Equal(t, finalPlan, parsed)
}
