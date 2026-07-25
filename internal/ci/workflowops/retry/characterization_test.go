package retry

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRetryRegistryCommand_preserves_successful_stdout(t *testing.T) {
	// Given: the existing shell retry helper and a command with machine-readable output.
	script, err := filepath.Abs(filepath.Join("..", "..", "..", "..", ".github", "scripts", "retry-registry-command.sh"))
	require.NoError(t, err)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	command := exec.CommandContext(t.Context(), "bash", script, "stdout clean", "bash", "-c", "printf sha256:abc")
	command.Env = append(os.Environ(), "REGISTRY_COMMAND_ATTEMPTS=1", "REGISTRY_COMMAND_BASE_DELAY_SECONDS=1")
	command.Stdout = &stdout
	command.Stderr = &stderr

	// When: the wrapped command succeeds on its first attempt.
	err = command.Run()

	// Then: stdout is byte-for-byte unchanged and retry annotations stay on stderr.
	require.NoError(t, err)
	assert.Equal(t, "sha256:abc", stdout.String())
	assert.Contains(t, stderr.String(), "::group::stdout clean (attempt 1/1)")
}
