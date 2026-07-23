package scripts_test

import (
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"encoding/pem"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const apkPublicKeyFingerprint = "90f7940b20391f49b417b9b3be49f01ee88b975313860b6e1a77bbf7b109c6d2"

func TestPublishedAPKKeyIsRSA4096WithExpectedFingerprint(t *testing.T) {
	// Given
	keyPath := filepath.Join(filepath.Dir(githubScriptPath(t, "assemble-apk-repository.sh")), "..", "..", "keys", "apk", "verity.rsa.pub")
	keyPEM, err := os.ReadFile(keyPath)
	require.NoError(t, err)
	block, rest := pem.Decode(keyPEM)
	require.NotNil(t, block)
	require.Empty(t, strings.TrimSpace(string(rest)))

	// When
	publicKey, err := x509.ParsePKIXPublicKey(block.Bytes)
	require.NoError(t, err)
	rsaKey, ok := publicKey.(*rsa.PublicKey)
	require.True(t, ok)
	fingerprint := sha256.Sum256(block.Bytes)

	// Then
	assert.Equal(t, 4096, rsaKey.N.BitLen())
	assert.Equal(t, 65537, rsaKey.E)
	assert.Equal(t, apkPublicKeyFingerprint, hex.EncodeToString(fingerprint[:]))
}

func TestValidateAPKRepositoryRejectsLegacyRSASignature(t *testing.T) {
	// Given
	repoRoot := t.TempDir()
	repoDir := filepath.Join(repoRoot, "repo")
	archDir := filepath.Join(repoDir, "x86_64")
	writeTempFile(t, filepath.Join(archDir, "demo.apk"), "not a real apk")
	writeTempFile(t, filepath.Join(repoDir, "verity.rsa.pub"), "public key")
	writeTarGz(t, filepath.Join(archDir, "APKINDEX.tar.gz"), "APKINDEX", ".SIGN.RSA.verity.rsa.pub")

	// When
	output, err := runGithubScript(t, repoRoot, "validate-apk-repository.sh", "--require-signature", repoDir)

	// Then
	require.Error(t, err)
	assert.Contains(t, output, "RSA256")
}

func TestValidateAPKRepositoryAcceptsMatchingRSA256SignatureName(t *testing.T) {
	// Given
	repoRoot := t.TempDir()
	repoDir := filepath.Join(repoRoot, "repo")
	archDir := filepath.Join(repoDir, "x86_64")
	writeTempFile(t, filepath.Join(archDir, "demo.apk"), "not a real apk")
	writeTempFile(t, filepath.Join(repoDir, "verity.rsa.pub"), "public key")
	writeTarGz(t, filepath.Join(archDir, "APKINDEX.tar.gz"), "APKINDEX", ".SIGN.RSA256.verity.rsa.pub")

	// When
	output, err := runGithubScript(t, repoRoot, "validate-apk-repository.sh", "--require-signature", repoDir)

	// Then
	require.NoError(t, err)
	assert.Contains(t, output, "APK repository layout valid")
}

func TestAPKPublicationUsesOnlyApprovedIntegerArtifacts(t *testing.T) {
	// Given
	workflowDir := filepath.Join(filepath.Dir(githubScriptPath(t, "assemble-apk-repository.sh")), "..", "workflows")
	producer, err := os.ReadFile(filepath.Join(workflowDir, "integer-build-image.yaml"))
	require.NoError(t, err)
	publisher, err := os.ReadFile(filepath.Join(workflowDir, "build-site.yaml"))
	require.NoError(t, err)
	downloader, err := os.ReadFile(githubScriptPath(t, "download-approved-apks.sh"))
	require.NoError(t, err)

	// When
	producerText := string(producer)
	publisherText := string(publisher)
	downloaderText := string(downloader)
	gatePosition := strings.Index(producerText, "name: Build, verify, and publish multi-arch image")
	artifactPosition := strings.Index(producerText, "name: Upload approved APK repository packages")

	// Then
	require.Greater(t, gatePosition, -1)
	require.Greater(t, artifactPosition, gatePosition)
	assert.Contains(t, producerText, "apk-repository-${{ inputs.batch_id }}-${{ needs.melange-prep.outputs.artifact_key }}")
	assert.Contains(t, publisherText, "download-approved-apks.sh")
	assert.Contains(t, downloaderText, `gh run download "$run_id"`)
	assert.Contains(t, downloaderText, `--pattern "apk-repository-${batch_id}-*"`)
	assert.Contains(t, downloaderText, "gh attestation verify")
	assert.Contains(t, publisherText, "download-previous-pages.sh")
	assert.Contains(t, publisherText, "select-apk-repository.sh")
	assert.Contains(t, publisherText, "retention-days: 30")
	assert.Contains(t, publisherText, "group: github-pages-publish")
}

func TestSelectAPKRepositoryPreservesPublishedBytesWhenPackagesUnchanged(t *testing.T) {
	// Given
	repoRoot := t.TempDir()
	candidate := filepath.Join(repoRoot, "candidate")
	previous := filepath.Join(repoRoot, "previous")
	output := filepath.Join(repoRoot, "site", "dist", "apk")
	writeTempFile(t, filepath.Join(candidate, "x86_64", "demo-1-r0.apk"), "same package")
	writeTempFile(t, filepath.Join(candidate, "x86_64", "APKINDEX.tar.gz"), "new index")
	writeTempFile(t, filepath.Join(candidate, "verity.rsa.pub"), "key")
	writeTempFile(t, filepath.Join(candidate, "repository-format"), "1\n")
	writeTempFile(t, filepath.Join(previous, "x86_64", "demo-1-r0.apk"), "same package")
	writeTempFile(t, filepath.Join(previous, "x86_64", "APKINDEX.tar.gz"), "published index")
	writeTempFile(t, filepath.Join(previous, "verity.rsa.pub"), "key")
	writeTempFile(t, filepath.Join(previous, "repository-format"), "1\n")
	writeTempFile(t, filepath.Join(output, "index.html"), "docs")

	// When
	commandOutput, err := runGithubScript(t, repoRoot, "select-apk-repository.sh", candidate, previous, output)

	// Then
	require.NoError(t, err)
	assert.Contains(t, commandOutput, "repository state unchanged")
	index, readErr := os.ReadFile(filepath.Join(output, "x86_64", "APKINDEX.tar.gz"))
	require.NoError(t, readErr)
	assert.Equal(t, "published index", string(index))
	assert.FileExists(t, filepath.Join(output, "index.html"))
}

func TestSelectAPKRepositoryPublishesCandidateWhenPackageChanges(t *testing.T) {
	// Given
	repoRoot := t.TempDir()
	candidate := filepath.Join(repoRoot, "candidate")
	previous := filepath.Join(repoRoot, "previous")
	output := filepath.Join(repoRoot, "output")
	writeTempFile(t, filepath.Join(candidate, "aarch64", "demo-2-r0.apk"), "changed package")
	writeTempFile(t, filepath.Join(candidate, "aarch64", "APKINDEX.tar.gz"), "new index")
	writeTempFile(t, filepath.Join(candidate, "verity.rsa.pub"), "key")
	writeTempFile(t, filepath.Join(previous, "aarch64", "demo-1-r0.apk"), "old package")
	writeTempFile(t, filepath.Join(previous, "aarch64", "APKINDEX.tar.gz"), "old index")
	writeTempFile(t, filepath.Join(previous, "verity.rsa.pub"), "key")

	// When
	commandOutput, err := runGithubScript(t, repoRoot, "select-apk-repository.sh", candidate, previous, output)

	// Then
	require.NoError(t, err)
	assert.Contains(t, commandOutput, "repository state changed")
	assert.FileExists(t, filepath.Join(output, "aarch64", "demo-2-r0.apk"))
	_, statErr := os.Stat(filepath.Join(output, "aarch64", "demo-1-r0.apk"))
	assert.True(t, os.IsNotExist(statErr))
}

func TestSelectAPKRepositoryPublishesCandidateWhenTrustRootChanges(t *testing.T) {
	// Given
	repoRoot := t.TempDir()
	candidate := filepath.Join(repoRoot, "candidate")
	previous := filepath.Join(repoRoot, "previous")
	output := filepath.Join(repoRoot, "output")
	for _, repository := range []string{candidate, previous} {
		writeTempFile(t, filepath.Join(repository, "x86_64", "demo-1-r0.apk"), "same package")
		writeTempFile(t, filepath.Join(repository, "x86_64", "APKINDEX.tar.gz"), "index")
	}
	writeTempFile(t, filepath.Join(candidate, "verity.rsa.pub"), "new key")
	writeTempFile(t, filepath.Join(previous, "verity.rsa.pub"), "old key")

	// When
	commandOutput, err := runGithubScript(t, repoRoot, "select-apk-repository.sh", candidate, previous, output)

	// Then
	require.NoError(t, err)
	assert.Contains(t, commandOutput, "repository state changed")
	key, readErr := os.ReadFile(filepath.Join(output, "verity.rsa.pub"))
	require.NoError(t, readErr)
	assert.Equal(t, "new key", string(key))
}
