package scripts_test

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func githubScriptPath(t *testing.T, name string) string {
	t.Helper()

	_, currentFile, _, ok := runtime.Caller(0)
	require.True(t, ok)

	return filepath.Join(filepath.Dir(currentFile), "..", ".github", "scripts", name)
}

func runGithubScript(t *testing.T, repoRoot, name string, args ...string) (string, error) {
	t.Helper()

	cmd := exec.CommandContext(context.Background(), "bash", append([]string{githubScriptPath(t, name)}, args...)...)
	cmd.Dir = repoRoot
	cmd.Env = os.Environ()
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func writeTarGz(t *testing.T, path string, names ...string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	f, err := os.Create(path)
	require.NoError(t, err)
	defer f.Close()

	gz := gzip.NewWriter(f)
	defer gz.Close()
	tw := tar.NewWriter(gz)
	defer tw.Close()

	for _, name := range names {
		require.NoError(t, tw.WriteHeader(&tar.Header{
			Name:    name,
			Mode:    0o644,
			Size:    int64(len("test\n")),
			ModTime: time.Unix(0, 0),
		}))
		_, err := tw.Write([]byte("test\n"))
		require.NoError(t, err)
	}
}

func TestAssembleAPKRepositoryNoPackagesCreatesMarker(t *testing.T) {
	repoRoot := t.TempDir()
	outputDir := filepath.Join(repoRoot, "site", "dist", "apk")
	require.NoError(t, os.MkdirAll(filepath.Join(repoRoot, "apk-artifacts"), 0o755))

	output, err := runGithubScript(t, repoRoot, "assemble-apk-repository.sh", "--output", outputDir, "apk-artifacts")

	require.NoError(t, err)
	assert.Contains(t, output, "No APK files found")
	assert.FileExists(t, filepath.Join(outputDir, ".no-apks-found"))
}

func TestValidateAPKRepositoryRejectsRootPackages(t *testing.T) {
	repoRoot := t.TempDir()
	repoDir := filepath.Join(repoRoot, "repo")
	writeTempFile(t, filepath.Join(repoDir, "bad.apk"), "not a real apk")

	output, err := runGithubScript(t, repoRoot, "validate-apk-repository.sh", repoDir)

	require.Error(t, err)
	assert.Contains(t, output, "APK files must live under architecture directories")
}

func TestValidateAPKRepositoryRejectsNestedPackages(t *testing.T) {
	repoRoot := t.TempDir()
	repoDir := filepath.Join(repoRoot, "repo")
	writeTempFile(t, filepath.Join(repoDir, "x86_64", "nested", "bad.apk"), "not a real apk")

	output, err := runGithubScript(t, repoRoot, "validate-apk-repository.sh", repoDir)

	require.Error(t, err)
	assert.Contains(t, output, "directly under architecture directories")
}

func TestValidateAPKRepositoryRejectsUnsupportedArchDirectory(t *testing.T) {
	repoRoot := t.TempDir()
	repoDir := filepath.Join(repoRoot, "repo")
	archDir := filepath.Join(repoDir, "not-an-arch")
	writeTempFile(t, filepath.Join(archDir, "demo.apk"), "not a real apk")
	writeTarGz(t, filepath.Join(archDir, "APKINDEX.tar.gz"), "APKINDEX")

	output, err := runGithubScript(t, repoRoot, "validate-apk-repository.sh", repoDir)

	require.Error(t, err)
	assert.Contains(t, output, "unsupported architecture directory")
}

func TestAssembleAPKRepositoryRejectsDuplicateDestinations(t *testing.T) {
	repoRoot := t.TempDir()
	writeTempFile(t, filepath.Join(repoRoot, "source-a", "x86_64", "demo.apk"), "a")
	writeTempFile(t, filepath.Join(repoRoot, "source-b", "x86_64", "demo.apk"), "b")

	output, err := runGithubScript(t, repoRoot, "assemble-apk-repository.sh", "source-a", "source-b")

	require.Error(t, err)
	assert.Contains(t, output, "duplicate APK destination x86_64/demo.apk")
}

func TestAssembleAPKRepositoryRequiresRSAKeyName(t *testing.T) {
	repoRoot := t.TempDir()

	output, err := runGithubScript(t, repoRoot, "assemble-apk-repository.sh", "--key-name", "custom", "missing-source")

	require.Error(t, err)
	assert.Contains(t, output, "key name must end with .rsa")
}

func TestValidateAPKRepositoryRequiresSignedIndexAndPublicKey(t *testing.T) {
	repoRoot := t.TempDir()
	repoDir := filepath.Join(repoRoot, "repo")
	archDir := filepath.Join(repoDir, "x86_64")
	writeTempFile(t, filepath.Join(archDir, "demo.apk"), "not a real apk")
	writeTarGz(t, filepath.Join(archDir, "APKINDEX.tar.gz"), "APKINDEX")

	output, err := runGithubScript(t, repoRoot, "validate-apk-repository.sh", "--require-signature", repoDir)
	require.Error(t, err)
	assert.Contains(t, output, "no public key")

	writeTempFile(t, filepath.Join(repoDir, "verity-apk-repository.rsa.pub"), "public key")
	writeTarGz(t, filepath.Join(archDir, "APKINDEX.tar.gz"), "APKINDEX", ".SIGN.RSA.other.rsa.pub")

	output, err = runGithubScript(t, repoRoot, "validate-apk-repository.sh", "--require-signature", repoDir)
	require.Error(t, err)
	assert.Contains(t, output, "has no matching root public key")

	writeTarGz(t, filepath.Join(archDir, "APKINDEX.tar.gz"), "APKINDEX", ".SIGN.RSA.verity-apk-repository.rsa.pub")

	output, err = runGithubScript(t, repoRoot, "validate-apk-repository.sh", "--require-signature", repoDir)
	require.NoError(t, err)
	assert.Contains(t, output, "APK repository layout valid")
}
