package sitepublication

import (
	"context"
	"fmt"

	"github.com/verity-org/verity/internal/ci/publication"
	"github.com/verity-org/verity/internal/ci/signerlock"
)

func CreatePlan(ctx context.Context, request *PlanRequest) (PublicationPlan, error) {
	if request == nil {
		return PublicationPlan{}, fmt.Errorf("%w: request is required", ErrInvalidPlan)
	}
	if err := signerlock.ValidateSource(request.SignerLock, request.ExpectedSignerSourceSHA); err != nil {
		return PublicationPlan{}, fmt.Errorf("%w: signer lock: %w", ErrInvalidPlan, err)
	}
	manifest := request.Manifest
	if err := publication.Validate(ctx, &manifest, &publication.ValidationOptions{
		ExpectedIdentity:     request.ExpectedIdentity,
		ExpectedMode:         request.ExpectedMode,
		ExpectedComponents:   append([]publication.Component(nil), request.ExpectedComponents...),
		ExpectedSignerDigest: publication.Digest(request.SignerLock.Digest),
		PublicationSHA:       request.PublicationSHA,
		PreviousManifest:     request.PreviousManifest,
		AuthorizeBootstrap:   request.AuthorizeBootstrap,
		AuthorizeRestore:     request.AuthorizeRestore,
		RepositoryDir:        request.RepositoryDir,
		Runner:               request.Runner,
	}); err != nil {
		return PublicationPlan{}, err
	}
	manifestDigest, err := publication.DigestManifest(&manifest)
	if err != nil {
		return PublicationPlan{}, fmt.Errorf("digest publication manifest: %w", err)
	}
	plan := PublicationPlan{
		SchemaVersion:          SchemaVersion,
		ManifestDigest:         manifestDigest,
		PreviousManifestDigest: manifest.PreviousManifestDigest,
		Mode:                   manifest.Mode,
		SourceSHA:              manifest.SourceSHA,
		RunID:                  manifest.RunID,
		RunAttempt:             manifest.RunAttempt,
		BatchID:                manifest.BatchID,
		SignerDigest:           manifest.SignerDigest,
		SignerSourceSHA:        publication.SourceSHA(request.SignerLock.SourceSHA),
		SignerReference:        request.SignerLock.Reference(),
	}
	plan.PlanDigest, err = digestPlan(&plan)
	if err != nil {
		return PublicationPlan{}, err
	}
	return plan, nil
}
