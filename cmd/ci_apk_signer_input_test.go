package cmd

import (
	"bytes"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/urfave/cli/v3"
)

func TestReadAPKSigningKey_reads_environment_and_clears_inheritance(t *testing.T) {
	// Given distinct generated sentinel bytes in the approved environment boundary and stdin.
	environmentKey := []byte("sentinel-key-from-environment")
	t.Setenv("APK_REPOSITORY_PRIVATE_KEY", string(environmentKey))
	command := &cli.Command{Reader: bytes.NewReader([]byte("stdin-must-not-win"))}

	// When the signing key boundary is read.
	key, err := readAPKSigningKey(command)

	// Then Go consumes the environment key and removes it before children run.
	require.NoError(t, err)
	assert.Equal(t, environmentKey, key)
	_, present := os.LookupEnv("APK_REPOSITORY_PRIVATE_KEY")
	assert.False(t, present)
}

func TestReadAPKSigningKey_reads_stdin_when_environment_is_absent(t *testing.T) {
	// Given generated sentinel bytes only on stdin.
	stdinKey := []byte("sentinel-key-from-stdin")
	command := &cli.Command{Reader: bytes.NewReader(stdinKey)}

	// When the signing key boundary is read.
	key, err := readAPKSigningKey(command)

	// Then non-workflow callers retain the bounded stdin contract.
	require.NoError(t, err)
	assert.Equal(t, stdinKey, key)
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

func TestReadAPKSigningKey_rejects_oversized_environment(t *testing.T) {
	// Given an environment key larger than the bounded signer input contract.
	t.Setenv("APK_REPOSITORY_PRIVATE_KEY", string(bytes.Repeat([]byte{'k'}, maxAPKSigningKeyBytes+1)))
	command := &cli.Command{}

	// When the key is read.
	key, err := readAPKSigningKey(command)

	// Then the input fails closed and the environment is cleared.
	require.Error(t, err)
	assert.Nil(t, key)
	_, present := os.LookupEnv("APK_REPOSITORY_PRIVATE_KEY")
	assert.False(t, present)
}
