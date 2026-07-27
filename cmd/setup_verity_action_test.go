package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSetupVerityAction_usesOneAttestationIdentitySelector(t *testing.T) {
	// Given the protected Verity activation action.
	data, err := os.ReadFile(filepath.Join("..", ".github", "actions", "setup-verity", "action.yml"))
	require.NoError(t, err)
	action := string(data)

	// When its GitHub attestation command is inspected.
	require.Contains(t, action, `--signer-workflow "github.com/verity-org/verity/.github/workflows/build-verity-protected.yaml"`)

	// Then it selects the exact workflow without combining mutually exclusive identity flags.
	require.Equal(t, 0, strings.Count(action, "--signer-repo"))
}
