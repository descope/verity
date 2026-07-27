package signerlock

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	validDigest    = "sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	validSourceSHA = "0123456789abcdef0123456789abcdef01234567"
)

func TestParse_acceptsDigestPinnedTrustedLock(t *testing.T) {
	// Given a complete lock for the trusted image builder.
	want := Lock{
		Image:     SignerImageRepository,
		Digest:    validDigest,
		Workflow:  TrustedWorkflowIdentity,
		SourceSHA: validSourceSHA,
		Runnable:  true,
	}
	data := marshalLock(t, want)

	// When the lock is parsed.
	got, err := Parse(data)

	// Then the typed contract is preserved and produces a digest reference.
	require.NoError(t, err)
	assert.Equal(t, want, got)
	assert.Equal(t, SignerImageRepository+"@"+validDigest, got.Reference())
}

func TestParse_rejectsMalformedJSON(t *testing.T) {
	// Given truncated JSON.
	data := []byte(`{"image":`)

	// When the lock is parsed.
	_, err := Parse(data)

	// Then parsing fails closed as malformed input.
	require.Error(t, err)
	require.ErrorIs(t, err, ErrMalformed)
}

func TestParse_rejectsFloatingTag(t *testing.T) {
	// Given a lock that uses a mutable image tag instead of the fixed repository name.
	lock := validLock()
	lock.Image += ":latest"

	// When the lock is parsed.
	_, err := Parse(marshalLock(t, lock))

	// Then the mutable reference is rejected.
	require.ErrorIs(t, err, ErrInvalidImage)
}

func TestParse_rejectsWrongRegistry(t *testing.T) {
	// Given a signer image from an untrusted registry.
	lock := validLock()
	lock.Image = "quay.io/verity-org/apk-repository-signer"

	// When the lock is parsed.
	_, err := Parse(marshalLock(t, lock))

	// Then the registry mismatch is rejected.
	require.ErrorIs(t, err, ErrInvalidImage)
	var validationErr *ValidationError
	require.ErrorAs(t, err, &validationErr)
	assert.Equal(t, "image", validationErr.Field)
}

func TestValidateSource_rejectsStaleSource(t *testing.T) {
	// Given a valid lock and a different current source revision.
	lock := validLock()
	expectedSourceSHA := "fedcba9876543210fedcba9876543210fedcba98"

	// When the lock is checked against the current source revision.
	err := ValidateSource(lock, expectedSourceSHA)

	// Then a stale lock cannot be treated as current.
	require.Error(t, err)
	require.ErrorIs(t, err, ErrStaleSource)
}

func TestParse_rejectsUntrustedWorkflow(t *testing.T) {
	// Given a lock that points at a different workflow identity.
	lock := validLock()
	lock.Workflow = "github.com/verity-org/verity/.github/workflows/untrusted.yaml"

	// When the lock is parsed.
	_, err := Parse(marshalLock(t, lock))

	// Then the trusted workflow contract rejects it.
	require.ErrorIs(t, err, ErrInvalidWorkflow)
}

func TestParse_rejectsMisleadingRunnableBootstrap(t *testing.T) {
	// Given a bootstrap sentinel that falsely claims it is runnable.
	lock := validLock()
	lock.Bootstrap = true

	// When the lock is parsed.
	_, err := Parse(marshalLock(t, lock))

	// Then the bootstrap state wins over the misleading runnable flag.
	require.Error(t, err)
	require.ErrorIs(t, err, ErrBootstrap)
}

func TestParse_rejectsPlaceholderDigest(t *testing.T) {
	// Given a runnable-looking lock whose digest is still a replacement marker.
	lock := validLock()
	lock.Digest = "sha256:REPLACE_WITH_REAL_DIGEST"

	// When the lock is parsed.
	_, err := Parse(marshalLock(t, lock))

	// Then a placeholder cannot pass as an image digest.
	require.Error(t, err)
	require.ErrorIs(t, err, ErrInvalidDigest)
}

func TestParse_rejectsUnknownFields(t *testing.T) {
	// Given a lock with an unrecognized field that could hide a policy bypass.
	data := marshalLock(t, validLock())
	data = append(data[:len(data)-1], []byte(`,"success":true}`)...)

	// When the lock is parsed.
	_, err := Parse(data)

	// Then strict decoding rejects the misleading field.
	require.Error(t, err)
	require.ErrorIs(t, err, ErrMalformed)
}

func TestLoad_bootstrapTemplateFailsClosed(t *testing.T) {
	// Given the checked-in bootstrap lock template.
	_, testFile, _, ok := runtime.Caller(0)
	require.True(t, ok)
	repoRoot := filepath.Clean(filepath.Join(filepath.Dir(testFile), "..", "..", ".."))
	path := filepath.Join(repoRoot, "ci", "apk-signer.lock.json")
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	var sentinel struct {
		Bootstrap bool `json:"bootstrap"`
		Runnable  bool `json:"runnable"`
	}
	require.NoError(t, json.Unmarshal(data, &sentinel))
	require.True(t, sentinel.Bootstrap)
	require.False(t, sentinel.Runnable)

	// When the template is loaded as a runnable lock.
	_, err = Load(path)

	// Then the explicit bootstrap sentinel prevents use until replaced.
	require.Error(t, err)
	require.ErrorIs(t, err, ErrBootstrap)
}

func validLock() Lock {
	return Lock{
		Image:     SignerImageRepository,
		Digest:    validDigest,
		Workflow:  TrustedWorkflowIdentity,
		SourceSHA: validSourceSHA,
		Runnable:  true,
	}
}

func marshalLock(t *testing.T, lock Lock) []byte {
	t.Helper()
	data, err := json.Marshal(lock)
	require.NoError(t, err)
	return data
}
