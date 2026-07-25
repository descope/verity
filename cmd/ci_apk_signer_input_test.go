package cmd

import (
	"bytes"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/urfave/cli/v3"
)

func TestReadAPKSigningKey_reads_stdin_and_rejects_ambient_inheritance(t *testing.T) {
	// Given distinct generated sentinel bytes in stdin and the legacy ambient variable.
	stdinKey := []byte("sentinel-key-from-stdin")
	t.Setenv("APK_REPOSITORY_PRIVATE_KEY", "ambient-key-must-not-be-used")
	command := &cli.Command{Reader: bytes.NewReader(stdinKey)}

	// When the signing key boundary is read.
	key, err := readAPKSigningKey(command)

	// Then only stdin is accepted and the ambient key is removed before children run.
	require.NoError(t, err)
	assert.Equal(t, stdinKey, key)
	_, present := os.LookupEnv("APK_REPOSITORY_PRIVATE_KEY")
	assert.False(t, present)
}

func TestReadAPKSigningKey_rejects_oversized_stdin(t *testing.T) {
	// Given a key stream larger than the bounded signer input contract.
	command := &cli.Command{Reader: bytes.NewReader(bytes.Repeat([]byte{'k'}, maxAPKSigningKeyBytes+1))}

	// When the key is read.
	key, err := readAPKSigningKey(command)

	// Then the input fails closed without returning secret bytes.
	require.Error(t, err)
	assert.Nil(t, key)
}
