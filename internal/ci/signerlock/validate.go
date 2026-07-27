package signerlock

import (
	"fmt"
	"strings"
)

// Validate enforces the signer lock's immutable image, provenance, and
// runnable-state requirements.
func Validate(lock Lock) error {
	if lock.Bootstrap {
		return &ValidationError{Field: "bootstrap", Value: "true", Cause: ErrBootstrap}
	}
	if !lock.Runnable {
		return &ValidationError{Field: "runnable", Value: "false", Cause: ErrNotRunnable}
	}
	if lock.Image != SignerImageRepository {
		return &ValidationError{Field: "image", Value: lock.Image, Cause: ErrInvalidImage}
	}
	if !isValidDigest(lock.Digest) {
		return &ValidationError{Field: "digest", Value: lock.Digest, Cause: ErrInvalidDigest}
	}
	if lock.Workflow != TrustedWorkflowIdentity {
		return &ValidationError{Field: "workflow", Value: lock.Workflow, Cause: ErrInvalidWorkflow}
	}
	if !isValidSourceSHA(lock.SourceSHA) {
		return &ValidationError{Field: "source_sha", Value: lock.SourceSHA, Cause: ErrInvalidSourceSHA}
	}
	return nil
}

// ValidateSource validates the lock and rejects a lock made from a stale
// signer image source revision.
func ValidateSource(lock Lock, expectedSourceSHA string) error {
	if err := Validate(lock); err != nil {
		return err
	}
	if !isValidSourceSHA(expectedSourceSHA) {
		return &ValidationError{Field: "expected_source_sha", Value: expectedSourceSHA, Cause: ErrInvalidSourceSHA}
	}
	if lock.SourceSHA != expectedSourceSHA {
		return &ValidationError{
			Field: "source_sha",
			Value: lock.SourceSHA,
			Cause: fmt.Errorf("%w: expected %s, got %s", ErrStaleSource, expectedSourceSHA, lock.SourceSHA),
		}
	}
	return nil
}

// Reference returns the immutable OCI reference used by a validated lock.
func (lock Lock) Reference() string { return lock.Image + "@" + lock.Digest }

func isValidDigest(value string) bool {
	if !strings.HasPrefix(value, digestPrefix) || len(value) != len(digestPrefix)+digestHexLength {
		return false
	}
	return lowerHex(value[len(digestPrefix):])
}

func isValidSourceSHA(value string) bool {
	return len(value) == sourceSHAHexLength && lowerHex(value)
}

func lowerHex(value string) bool {
	for _, char := range []byte(value) {
		if (char < '0' || char > '9') && (char < 'a' || char > 'f') {
			return false
		}
	}
	return true
}
