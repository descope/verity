package runson

import (
	"bytes"
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSystemVerifier_configuresProductionBoundaries(t *testing.T) {
	verifier := systemVerifier()

	assert.Equal(t, "http://169.254.169.254", verifier.metadataEndpoint)
	assert.NotNil(t, verifier.client)
	assert.NotNil(t, verifier.getenv)
	assert.NotNil(t, verifier.architecture)
	assert.NotNil(t, verifier.cpuCount)
	assert.NotNil(t, verifier.memoryBytes)
	assert.NotNil(t, verifier.diskBytes)
	assert.NotNil(t, verifier.execute)
}

func TestSystemCapacityObservers_returnPositiveValues(t *testing.T) {
	memory, err := readSystemMemory()
	require.NoError(t, err)
	assert.Positive(t, memory)

	disk, err := readRootDisk()
	require.NoError(t, err)
	assert.Positive(t, disk)
}

func TestExecuteCommand_capturesOutputAndFailure(t *testing.T) {
	output, err := executeCommand(context.Background(), "sh", "-c", "printf sentinel")
	require.NoError(t, err)
	assert.Equal(t, "sentinel", string(output))

	_, err = executeCommand(context.Background(), "sh", "-c", "printf failure >&2; exit 7")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failure")
}

func TestLimitedBuffer_capsOutput(t *testing.T) {
	var buffer limitedBuffer
	input := bytes.Repeat([]byte("x"), commandOutputLimit+1)

	written, err := buffer.Write(input)

	require.NoError(t, err)
	assert.Equal(t, len(input), written)
	assert.True(t, buffer.truncated)
	assert.Len(t, buffer.Bytes(), commandOutputLimit)
	assert.Len(t, buffer.String(), commandOutputLimit)
}
