package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestActivateArtifact_installs_only_after_reverification(t *testing.T) {
	// Given one extracted canonical artifact and an absent destination.
	extract := canonicalExtractFixture(t)
	_, err := extractArtifact(&extract)
	require.NoError(t, err)
	destination := filepath.Join(t.TempDir(), binaryName)

	// When activation reverifies and installs the binary.
	err = activateArtifact(activationOptions{
		ArtifactDirectory: extract.ArtifactDirectory,
		Destination:       destination,
		Identity:          extract.Identity,
	})

	// Then the installed bytes are executable.
	require.NoError(t, err)
	data, err := os.ReadFile(destination)
	require.NoError(t, err)
	assert.Equal(t, []byte("fake-verity-binary"), data)
	info, err := os.Lstat(destination)
	require.NoError(t, err)
	assert.NotZero(t, info.Mode().Perm()&0o111)
}

func TestActivateArtifact_leaves_tampered_payload_non_executable(t *testing.T) {
	// Given a verified artifact whose binary is modified before activation.
	extract := canonicalExtractFixture(t)
	_, err := extractArtifact(&extract)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(extract.ArtifactDirectory, binaryName), []byte("tampered"), 0o600))
	destination := filepath.Join(t.TempDir(), binaryName)

	// When activation reruns verification.
	err = activateArtifact(activationOptions{
		ArtifactDirectory: extract.ArtifactDirectory,
		Destination:       destination,
		Identity:          extract.Identity,
	})

	// Then activation fails without creating an executable.
	require.Error(t, err)
	assert.ErrorIs(t, err, errUntrustedArtifact)
	_, statErr := os.Lstat(destination)
	assert.ErrorIs(t, statErr, os.ErrNotExist)
}

func TestActivateArtifact_replaces_destination_symlink_without_following_it(t *testing.T) {
	// Given a valid artifact and an attacker-controlled destination symlink.
	extract := canonicalExtractFixture(t)
	_, err := extractArtifact(&extract)
	require.NoError(t, err)
	directory := t.TempDir()
	target := filepath.Join(directory, "outside")
	require.NoError(t, os.WriteFile(target, []byte("outside"), 0o600))
	destination := filepath.Join(directory, binaryName)
	require.NoError(t, os.Symlink(target, destination))

	// When the verified binary is atomically installed.
	err = activateArtifact(activationOptions{
		ArtifactDirectory: extract.ArtifactDirectory,
		Destination:       destination,
		Identity:          extract.Identity,
	})

	// Then the link itself is replaced and its target is untouched.
	require.NoError(t, err)
	info, err := os.Lstat(destination)
	require.NoError(t, err)
	assert.Zero(t, info.Mode()&os.ModeSymlink)
	outside, err := os.ReadFile(target)
	require.NoError(t, err)
	assert.Equal(t, []byte("outside"), outside)
}
