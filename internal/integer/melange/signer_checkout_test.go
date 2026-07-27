package melange

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAPKRepositorySignerCheckout_fetchesPinnedAncestor_withoutMutableBranch(t *testing.T) {
	// Given the repository signer recipe.
	paths := repositoryTestPaths(t)
	data, err := os.ReadFile(filepath.Join(paths.BespokeDir, "apk-repository-signer.yaml"))
	require.NoError(t, err)
	recipe := string(data)

	// When its source checkout contract is inspected.
	require.Regexp(t, `(?m)^\s+expected-commit: [0-9a-f]{40}$`, recipe)

	// Then Melange can reset to the immutable ancestor without resolving a mutable branch.
	require.Contains(t, recipe, "      depth: -1\n")
	require.NotRegexp(t, `(?m)^\s+branch:`, recipe)
}
