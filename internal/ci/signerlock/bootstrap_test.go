package signerlock

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestBootstrapLock_failsClosedBeforeItCanRun(t *testing.T) {
	// Given the checked-in bootstrap sentinel lock.
	data := []byte(`{
  "image": "ghcr.io/verity-org/apk-repository-signer",
  "digest": "sha256:REPLACE_WITH_REAL_DIGEST",
  "workflow": "github.com/verity-org/verity/.github/workflows/integer-build-image.yaml",
  "source_sha": "REPLACE_WITH_REAL_SOURCE_SHA",
  "bootstrap": true,
  "runnable": false
}`)

	// When the lock is parsed.
	_, err := Parse(data)

	// Then the explicitly non-runnable bootstrap sentinel is rejected.
	require.ErrorIs(t, err, ErrBootstrap)
}
