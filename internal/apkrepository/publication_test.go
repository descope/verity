package apkrepository

import (
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"encoding/pem"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const apkPublicKeyFingerprint = "416d7b8491fccfde1e5d247b4dfc0571ccd20e0610b192334d4ee1308d9adee7"

func TestPublishedKey_is_expected_RSA4096_trust_root(t *testing.T) {
	// Given the committed APK repository trust root.
	root := repositoryRoot(t)
	keyPEM, err := os.ReadFile(filepath.Join(root, "keys", "apk", "verity.rsa.pub"))
	require.NoError(t, err)
	block, rest := pem.Decode(keyPEM)
	require.NotNil(t, block)
	require.Empty(t, strings.TrimSpace(string(rest)))

	// When its cryptographic identity is parsed.
	publicKey, err := x509.ParsePKIXPublicKey(block.Bytes)
	require.NoError(t, err)
	rsaKey, ok := publicKey.(*rsa.PublicKey)
	require.True(t, ok)
	fingerprint := sha256.Sum256(block.Bytes)

	// Then the exact reviewed RSA-4096 key is pinned.
	assert.Equal(t, 4096, rsaKey.N.BitLen())
	assert.Equal(t, 65537, rsaKey.E)
	assert.Equal(t, apkPublicKeyFingerprint, hex.EncodeToString(fingerprint[:]))
	for _, relative := range []string{
		filepath.Join("site", "src", "data", "apk-repository.ts"),
		filepath.Join("docs", "guides", "install-apk-repository.md"),
		filepath.Join("docs", "architecture", "apk-repository.md"),
	} {
		content, readErr := os.ReadFile(filepath.Join(root, relative))
		require.NoError(t, readErr)
		assert.Contains(t, string(content), apkPublicKeyFingerprint, relative)
	}
}

func TestPublication_workflows_use_Go_repository_commands_and_approved_artifacts(t *testing.T) {
	// Given the producer and Pages publication workflows.
	root := repositoryRoot(t)
	producer, err := os.ReadFile(filepath.Join(root, ".github", "workflows", "integer-build-image-reusable.yaml"))
	require.NoError(t, err)
	publisher, err := os.ReadFile(filepath.Join(root, ".github", "workflows", "build-site.yaml"))
	require.NoError(t, err)
	producerText := string(producer)
	publisherText := string(publisher)

	// When the trust gate and publication commands are inspected.
	gatePosition := strings.Index(producerText, "name: Trivy publish gate (not clean, no go)")
	artifactPosition := strings.Index(producerText, "name: Upload exact Integer component")

	// Then packages originate after the strict gate and all Verity logic runs through Go.
	require.Greater(t, gatePosition, -1)
	require.Greater(t, artifactPosition, gatePosition)
	assert.Contains(t, producerText, "integer-component-${{ inputs.publication_id }}-${{ inputs.shard }}-${{ inputs.artifact_key }}")
	for _, command := range []string{
		"./verity ci apk-repository download-approved",
		"./verity ci site-publication resolve-previous",
		"./verity ci site-publication restore",
		"./verity ci publication compose",
		"./verity ci site-publication plan",
		"./verity ci site-publication signer-plan",
		"./verity ci site-publication signer-execute",
		"./verity ci site-publication assemble",
		"./verity ci site-publication finalize",
		"./verity ci apk-repository validate",
	} {
		assert.Contains(t, publisherText, command)
	}
	assert.NotContains(t, publisherText, ".github/scripts/assemble-apk-repository.sh")
	assert.NotContains(t, publisherText, ".github/scripts/validate-apk-repository.sh")
	assert.Contains(t, publisherText, "retention-days: \"30\"")
	assert.Contains(t, publisherText, "group: github-pages-publish")
}

func repositoryRoot(t *testing.T) string {
	t.Helper()
	_, currentFile, _, ok := runtime.Caller(0)
	require.True(t, ok)
	return filepath.Clean(filepath.Join(filepath.Dir(currentFile), "..", ".."))
}
