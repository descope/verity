package signerlock

import (
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"

	intconfig "github.com/verity-org/verity/internal/integer/config"
)

func TestBaseline_existingIntegerImageKeepsNonDevRuntimeContract(t *testing.T) {
	// Given an existing Integer image definition that was unchanged by this task.
	_, testFile, _, ok := runtime.Caller(0)
	require.True(t, ok)
	repoRoot := filepath.Clean(filepath.Join(filepath.Dir(testFile), "..", "..", ".."))

	// When the existing definition is loaded and validated.
	def, err := intconfig.LoadImage(filepath.Join(repoRoot, "images", "apko.yaml"))

	// Then its non-dev base and runtime package contract remain intact.
	require.NoError(t, err)
	require.NoError(t, intconfig.Validate(def))
	require.Contains(t, def.Types, "default")
	require.Equal(t, "wolfi-base", def.Types["default"].Base)
	require.Equal(t, []string{"apko"}, def.Types["default"].Packages)
}
