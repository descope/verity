package publication

import (
	"cmp"
	"context"
	"fmt"
	"slices"
	"sort"
)

func Validate(ctx context.Context, manifest *Manifest, options *ValidationOptions) error {
	if manifest == nil {
		return invalidManifest("manifest is required")
	}
	if options == nil {
		return invalidManifest("validation options are required")
	}
	if err := validateManifestShape(manifest); err != nil {
		return err
	}
	if !shaPattern.MatchString(string(options.PublicationSHA)) {
		return fmt.Errorf("%w: publication SHA %q", ErrIdentityMismatch, options.PublicationSHA)
	}
	actualIdentity := ProducerIdentity{
		SourceSHA: manifest.SourceSHA, RunID: manifest.RunID,
		RunAttempt: manifest.RunAttempt, BatchID: manifest.BatchID,
	}
	if actualIdentity != options.ExpectedIdentity {
		return fmt.Errorf("%w: got %+v", ErrIdentityMismatch, actualIdentity)
	}
	if manifest.Mode != options.ExpectedMode {
		return fmt.Errorf("%w: expected mode %q, got %q", ErrIdentityMismatch, options.ExpectedMode, manifest.Mode)
	}
	if !componentsEqual(manifest.Components, options.ExpectedComponents) {
		return ErrComponentMismatch
	}
	if manifest.SignerDigest != options.ExpectedSignerDigest {
		return fmt.Errorf("%w: got %s", ErrSignerMismatch, manifest.SignerDigest)
	}
	if err := validateModeAuthorization(manifest, options); err != nil {
		return err
	}
	if err := validatePreviousManifest(manifest, options.PreviousManifest); err != nil {
		return err
	}
	runner := options.Runner
	if runner == nil {
		runner = ExecRunner{Dir: options.RepositoryDir}
	}
	if options.PreviousManifest != nil {
		if err := validateAncestry(ctx, runner, ancestryRequest{
			ancestor:   options.PreviousManifest.SourceSHA,
			descendant: manifest.SourceSHA,
		}); err != nil {
			return fmt.Errorf("previous publication ancestry: %w", err)
		}
	}
	if err := validateAncestry(ctx, runner, ancestryRequest{
		ancestor:   manifest.SourceSHA,
		descendant: options.PublicationSHA,
	}); err != nil {
		return fmt.Errorf("producer ancestry: %w", err)
	}
	return nil
}

func componentsEqual(actual, expected []Component) bool {
	left := append([]Component(nil), actual...)
	right := append([]Component(nil), expected...)
	sort.Slice(left, func(i, j int) bool { return compareComponent(&left[i], &left[j]) < 0 })
	sort.Slice(right, func(i, j int) bool { return compareComponent(&right[i], &right[j]) < 0 })
	return slices.Equal(left, right)
}

func compareComponent(left, right *Component) int {
	if left.Name != right.Name {
		return cmp.Compare(left.Name, right.Name)
	}
	return cmp.Compare(left.ArtifactName, right.ArtifactName)
}

func validateModeAuthorization(manifest *Manifest, options *ValidationOptions) error {
	switch manifest.Mode {
	case ModeBootstrap:
		if !options.AuthorizeBootstrap {
			return ErrBootstrapUnauthorized
		}
	case ModeRestore:
		if !options.AuthorizeRestore {
			return ErrRestoreUnauthorized
		}
	case ModeSnapshot, ModeDelta:
		return nil
	default:
		return fmt.Errorf("%w: unsupported mode %q", ErrInvalidManifest, manifest.Mode)
	}
	return nil
}

func validatePreviousManifest(manifest, previous *Manifest) error {
	nextDigest, err := DigestManifest(manifest)
	if err != nil {
		return err
	}
	if previous == nil {
		if manifest.Mode != ModeBootstrap {
			return fmt.Errorf("%w: previous manifest is missing", ErrCASMismatch)
		}
		return CompareAndSwap("", manifest.PreviousManifestDigest, nextDigest)
	}
	if manifest.Mode == ModeBootstrap {
		return fmt.Errorf("%w: state already exists", ErrBootstrapUnauthorized)
	}
	previousDigest, err := DigestManifest(previous)
	if err != nil {
		return fmt.Errorf("digest previous publication manifest: %w", err)
	}
	if err := CompareAndSwap(previousDigest, manifest.PreviousManifestDigest, nextDigest); err != nil {
		return err
	}
	if err := validateSigningKeyStateProgression(manifest, previous); err != nil {
		return err
	}
	return validateRunProgression(manifest, previous)
}

func validateSigningKeyStateProgression(candidate, previous *Manifest) error {
	candidatePresent := signingKeyStatePresent(candidate)
	previousPresent := signingKeyStatePresent(previous)
	if !candidatePresent && !previousPresent {
		return nil
	}
	if !candidatePresent {
		if previous.SigningKeyEpoch == 0 {
			return fmt.Errorf("%w: candidate key state is missing at key epoch 0", ErrSigningKeyStateChange)
		}
		return fmt.Errorf("%w: candidate key epoch 0 is below previous key epoch %d", ErrSigningKeyEpochRollback, previous.SigningKeyEpoch)
	}
	if !previousPresent {
		return nil
	}
	if candidate.SigningKeyEpoch < previous.SigningKeyEpoch {
		return fmt.Errorf("%w: candidate key epoch %d is below previous key epoch %d", ErrSigningKeyEpochRollback, candidate.SigningKeyEpoch, previous.SigningKeyEpoch)
	}
	if candidate.SigningKeyEpoch == previous.SigningKeyEpoch {
		if !signingKeyStateEqual(candidate, previous) {
			return fmt.Errorf("%w: key epoch %d changed signing key state", ErrSigningKeyStateChange, candidate.SigningKeyEpoch)
		}
		return nil
	}
	for _, fingerprint := range previous.RevokedSigningKeyFingerprints {
		if candidate.ActiveSigningKeyFingerprint == fingerprint {
			return fmt.Errorf("%w: revoked fingerprint %q became active", ErrSigningKeyRevocationRollback, fingerprint)
		}
		if _, trusted := slices.BinarySearch(candidate.TrustedSigningKeyFingerprints, fingerprint); trusted {
			return fmt.Errorf("%w: revoked fingerprint %q became trusted", ErrSigningKeyRevocationRollback, fingerprint)
		}
		if _, revoked := slices.BinarySearch(candidate.RevokedSigningKeyFingerprints, fingerprint); !revoked {
			return fmt.Errorf("%w: candidate revoked fingerprints omit %q", ErrSigningKeyRevocationRollback, fingerprint)
		}
	}
	return nil
}

func signingKeyStateEqual(left, right *Manifest) bool {
	return left.SigningKeyEpoch == right.SigningKeyEpoch &&
		left.ActiveSigningKeyFingerprint == right.ActiveSigningKeyFingerprint &&
		slices.Equal(left.TrustedSigningKeyFingerprints, right.TrustedSigningKeyFingerprints) &&
		slices.Equal(left.RevokedSigningKeyFingerprints, right.RevokedSigningKeyFingerprints)
}

func validateRunProgression(manifest, previous *Manifest) error {
	if manifest.RunID < previous.RunID {
		return fmt.Errorf("%w: run %d precedes %d", ErrStaleRunAttempt, manifest.RunID, previous.RunID)
	}
	if manifest.RunID > previous.RunID {
		return nil
	}
	if manifest.SourceSHA != previous.SourceSHA {
		return fmt.Errorf("%w: run %d changed source SHA", ErrIdentityMismatch, manifest.RunID)
	}
	if manifest.RunAttempt < previous.RunAttempt {
		return fmt.Errorf("%w: attempt %d precedes %d for run %d", ErrStaleRunAttempt, manifest.RunAttempt, previous.RunAttempt, manifest.RunID)
	}
	if manifest.RunAttempt == previous.RunAttempt {
		return ErrReplay
	}
	return nil
}

type ancestryRequest struct {
	ancestor   SourceSHA
	descendant SourceSHA
}

func validateAncestry(ctx context.Context, runner Runner, request ancestryRequest) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	result, err := runner.Run(ctx, Command{
		Name: "git",
		Args: []string{"merge-base", "--is-ancestor", string(request.ancestor), string(request.descendant)},
	})
	if err != nil {
		return fmt.Errorf("run git merge-base: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	switch result.ExitCode {
	case 0:
		return nil
	case 1:
		return fmt.Errorf("%w: %s is not an ancestor of %s", ErrNotAncestor, request.ancestor, request.descendant)
	default:
		return fmt.Errorf("%w: git exited %d: %s", ErrAncestryCommandFailed, result.ExitCode, result.Stderr)
	}
}
