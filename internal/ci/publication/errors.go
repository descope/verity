package publication

import "errors"

var (
	ErrInvalidManifest              = errors.New("invalid publication manifest")
	ErrIdentityMismatch             = errors.New("publication identity mismatch")
	ErrComponentMismatch            = errors.New("publication component mismatch")
	ErrSignerMismatch               = errors.New("publication signer mismatch")
	ErrCASMismatch                  = errors.New("publication compare-and-swap mismatch")
	ErrReplay                       = errors.New("publication replay")
	ErrStaleRunAttempt              = errors.New("stale publication run or attempt")
	ErrBootstrapUnauthorized        = errors.New("publication bootstrap is not authorized")
	ErrRestoreUnauthorized          = errors.New("publication restore is not authorized")
	ErrNotAncestor                  = errors.New("publication source is not an ancestor")
	ErrAncestryCommandFailed        = errors.New("publication ancestry command failed")
	ErrNonCanonicalManifest         = errors.New("publication manifest is not canonical")
	ErrSigningKeyEpochRollback      = errors.New("signing key epoch rollback")
	ErrSigningKeyRevocationRollback = errors.New("signing key revocation rollback")
	ErrSigningKeyStateChange        = errors.New("signing key state changed without epoch increase")
	ErrSigningKeyStateFile          = errors.New("invalid signing key state file")
)

var (
	errJSONKeyNotString        = errors.New("JSON object key is not a string")
	errDuplicateJSONKey        = errors.New("duplicate JSON key")
	errUnexpectedJSONDelimiter = errors.New("unexpected JSON delimiter")
	errTrailingJSONValue       = errors.New("trailing JSON value")
)
