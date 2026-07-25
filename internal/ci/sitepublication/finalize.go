package sitepublication

import (
	"context"
	"errors"
	"fmt"

	"github.com/verity-org/verity/internal/ci/publication"
)

func FinalizePublication(ctx context.Context, request *FinalizeRequest) (FinalPlan, error) {
	if request == nil {
		return FinalPlan{}, fmt.Errorf("%w: request is required", ErrInvalidFinalPlan)
	}
	if err := ctx.Err(); err != nil {
		return FinalPlan{}, err
	}
	if err := validatePlan(&request.Plan); err != nil {
		return FinalPlan{}, err
	}
	if request.ExpectedPlanDigest != request.Plan.PlanDigest {
		return FinalPlan{}, fmt.Errorf("%w: publication plan digest", ErrInvalidFinalPlan)
	}
	verified, err := VerifySite(request.SiteDir)
	if err != nil {
		return FinalPlan{}, err
	}
	if verified.ManifestDigest != request.Plan.ManifestDigest {
		return FinalPlan{}, fmt.Errorf("%w: publication manifest digest", ErrUndeclaredMutation)
	}
	currentDigest := publication.Digest("")
	if request.CurrentManifest != nil {
		currentDigest, err = publication.DigestManifest(request.CurrentManifest)
		if err != nil {
			return FinalPlan{}, fmt.Errorf("digest current publication manifest: %w", err)
		}
	}
	if err := publication.CompareAndSwap(currentDigest, request.Plan.PreviousManifestDigest, request.Plan.ManifestDigest); err != nil {
		return FinalPlan{}, err
	}
	artifactDigest, err := PackSite(request.SiteDir, request.ArchivePath)
	if err != nil {
		return FinalPlan{}, err
	}
	archived, err := ValidateArchive(request.ArchivePath, artifactDigest, request.Plan.ManifestDigest)
	if err != nil {
		return FinalPlan{}, errors.Join(err, removeSecureFilePath(request.ArchivePath))
	}
	if archived.SiteDigest != verified.SiteDigest {
		return FinalPlan{}, errors.Join(
			fmt.Errorf("%w: site changed before archive eligibility", ErrUndeclaredMutation),
			removeSecureFilePath(request.ArchivePath),
		)
	}
	return FinalPlan{
		SchemaVersion: SchemaVersion, ArtifactName: PagesArtifactName,
		ArtifactPath: request.ArchivePath, ArtifactDigest: artifactDigest,
		ManifestDigest: request.Plan.ManifestDigest, SiteDigest: archived.SiteDigest,
		RunID: request.Plan.RunID, RunAttempt: request.Plan.RunAttempt,
		Attestation: AttestationPlan{
			SubjectPath: request.ArchivePath, SubjectDigest: artifactDigest,
			Workflow: BuildSiteWorkflow, SourceSHA: request.Plan.SourceSHA,
		},
		DeployEligible: true,
	}, nil
}

func ValidateFinalArtifact(plan *FinalPlan, archivePath string) error {
	if err := validateFinalPlan(plan); err != nil {
		return err
	}
	if archivePath != plan.ArtifactPath {
		return fmt.Errorf("%w: artifact path", ErrInvalidFinalPlan)
	}
	verified, err := ValidateArchive(archivePath, plan.ArtifactDigest, plan.ManifestDigest)
	if err != nil {
		return err
	}
	if verified.SiteDigest != plan.SiteDigest {
		return fmt.Errorf("%w: site digest", ErrArtifactTampered)
	}
	return nil
}

func validateFinalPlan(plan *FinalPlan) error {
	if plan == nil || plan.SchemaVersion != SchemaVersion || !plan.DeployEligible {
		return fmt.Errorf("%w: schema or eligibility", ErrInvalidFinalPlan)
	}
	if plan.ArtifactName != PagesArtifactName || plan.Attestation.Workflow != BuildSiteWorkflow {
		return fmt.Errorf("%w: artifact identity", ErrInvalidFinalPlan)
	}
	if plan.ArtifactPath == "" || plan.Attestation.SubjectPath != plan.ArtifactPath {
		return fmt.Errorf("%w: artifact path", ErrInvalidFinalPlan)
	}
	for _, digest := range []publication.Digest{plan.ArtifactDigest, plan.ManifestDigest, plan.SiteDigest} {
		if !digestPattern.MatchString(string(digest)) {
			return fmt.Errorf("%w: digest", ErrInvalidFinalPlan)
		}
	}
	if plan.Attestation.SubjectDigest != plan.ArtifactDigest || plan.RunID == 0 || plan.RunAttempt == 0 {
		return fmt.Errorf("%w: attestation identity", ErrInvalidFinalPlan)
	}
	return nil
}
