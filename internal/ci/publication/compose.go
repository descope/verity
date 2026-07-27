package publication

import (
	"context"
	"fmt"
	"slices"

	ci "github.com/verity-org/verity/internal/ci"
)

func Compose(ctx context.Context, request *ComposeRequest) (ComposeResult, error) {
	if request == nil {
		return ComposeResult{}, fmt.Errorf("%w: request is required", ErrComposeInvalid)
	}
	parsed, err := parseProducerSet(request)
	if err != nil {
		return ComposeResult{}, err
	}
	previousDigest, err := resolvePreviousDigest(request)
	if err != nil {
		return ComposeResult{}, err
	}
	operations, err := composeAPKOperations(request, &parsed.integer)
	if err != nil {
		return ComposeResult{}, err
	}
	trusted := append([]string(nil), request.TrustedSigningKeyFingerprints...)
	revoked := append([]string(nil), request.RevokedSigningKeyFingerprints...)
	slices.Sort(trusted)
	slices.Sort(revoked)
	manifest := Manifest{
		SchemaVersion: SchemaVersion, SourceSHA: request.Identity.SourceSHA,
		RunID: request.Identity.RunID, RunAttempt: request.Identity.RunAttempt,
		BatchID: request.Identity.BatchID, Mode: request.Mode,
		PreviousManifestDigest: previousDigest, Components: parsed.components,
		SignerDigest: request.SignerDigest, APKOperations: operations,
		SigningKeyEpoch:               request.SigningKeyEpoch,
		ActiveSigningKeyFingerprint:   request.ActiveSigningKeyFingerprint,
		TrustedSigningKeyFingerprints: trusted, RevokedSigningKeyFingerprints: revoked,
	}
	if err := Validate(ctx, &manifest, &ValidationOptions{
		ExpectedIdentity: request.Identity, ExpectedMode: request.Mode,
		ExpectedComponents: parsed.components, ExpectedSignerDigest: request.SignerDigest,
		PublicationSHA: request.PublicationSHA, PreviousManifest: request.PreviousManifest,
		AuthorizeBootstrap: request.AuthorizeBootstrap, AuthorizeRestore: request.AuthorizeRestore,
		RepositoryDir: request.RepositoryDir, Runner: request.Runner,
	}); err != nil {
		return ComposeResult{}, err
	}
	publicationJSON, err := MarshalCanonical(&manifest)
	if err != nil {
		return ComposeResult{}, err
	}
	componentsJSON, err := MarshalComponentsCanonical(manifest.Components)
	if err != nil {
		return ComposeResult{}, err
	}
	return ComposeResult{Manifest: manifest, PublicationJSON: publicationJSON, ComponentsJSON: componentsJSON}, nil
}

func resolvePreviousDigest(request *ComposeRequest) (Digest, error) {
	if request.PreviousManifest == nil {
		if request.PreviousManifestDigest != "" {
			return "", fmt.Errorf("%w: digest provided without previous manifest", ErrComposeInvalid)
		}
		return "", nil
	}
	digest, err := DigestManifest(request.PreviousManifest)
	if err != nil {
		return "", fmt.Errorf("digest previous publication: %w", err)
	}
	if request.PreviousManifestDigest != "" && request.PreviousManifestDigest != digest {
		return "", fmt.Errorf("%w: previous manifest digest", ErrProducerConflict)
	}
	return digest, nil
}

func composeAPKOperations(request *ComposeRequest, integer *ci.IntegerBatchManifest) ([]APKOperation, error) {
	if request.APKOperations != nil && request.APKDelta != nil {
		return nil, fmt.Errorf("%w: apk operations and delta are mutually exclusive", ErrComposeInvalid)
	}
	if request.APKOperations != nil {
		return append([]APKOperation(nil), request.APKOperations...), nil
	}
	if request.APKDelta != nil {
		return operationsFromDelta(request.APKDelta, integer)
	}
	operations := make([]APKOperation, 0, len(integer.Packages))
	for _, pkg := range integer.Packages {
		operations = append(operations, APKOperation{
			Action: APKUpsert, Architecture: Architecture(pkg.Architecture), PackageName: pkg.Name,
			ArtifactName: pkg.Artifact.Name, ArtifactDigest: Digest(pkg.Artifact.Digest),
		})
	}
	return operations, nil
}
