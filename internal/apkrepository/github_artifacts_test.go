package apkrepository

import (
	"bytes"
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDownloadApproved_downloads_exact_batch_and_verifies_every_attestation(t *testing.T) {
	// Given an approved Integer batch and a fake gh process boundary.
	output := filepath.Join(t.TempDir(), "artifacts")
	runner := &fakeCommandRunner{}
	runner.run = func(request command) (commandResult, error) {
		joined := request.name + " " + strings.Join(request.args, " ")
		if strings.HasPrefix(joined, "gh run download 1234") {
			writeTestFile(t, filepath.Join(output, "artifact", "x86_64", "demo.apk"), "package")
			return commandResult{}, nil
		}
		if strings.HasPrefix(joined, "gh attestation verify ") {
			return commandResult{}, nil
		}
		return commandResult{}, assert.AnError
	}
	var stdout bytes.Buffer

	// When approved packages are downloaded.
	err := DownloadApproved(context.Background(), &DownloadApprovedOptions{
		BatchID:    "1234-2",
		OutputDir:  output,
		Repository: "verity-org/verity",
		Stdout:     &stdout,
		runner:     runner,
	})

	// Then the run ID, batch pattern, builder identity, branch, and runner policy are exact.
	require.NoError(t, err)
	require.Len(t, runner.calls, 2)
	assert.Equal(t, []string{"run", "download", "1234", "--repo", "verity-org/verity", "--pattern", "apk-repository-1234-2-*", "--dir", output}, runner.calls[0].args)
	assert.Contains(t, runner.calls[1].args, "github.com/verity-org/verity/.github/workflows/integer-build-image.yaml")
	assert.Contains(t, runner.calls[1].args, "refs/heads/main")
	assert.Contains(t, runner.calls[1].args, "--deny-self-hosted-runners")
	assert.Contains(t, stdout.String(), "Downloaded and verified 1 approved APK packages")
}

func TestDownloadApproved_fails_when_batch_has_no_APKs(t *testing.T) {
	// Given a successful artifact download without approved APK files.
	output := filepath.Join(t.TempDir(), "artifacts")
	runner := &fakeCommandRunner{run: func(command) (commandResult, error) { return commandResult{}, nil }}

	// When the batch is consumed.
	err := DownloadApproved(context.Background(), &DownloadApprovedOptions{
		BatchID: "1234-1", OutputDir: output, Repository: "verity-org/verity", runner: runner,
	})

	// Then publication fails closed before any attestation command.
	require.Error(t, err)
	assert.ErrorContains(t, err, "did not publish approved APK artifacts")
	require.Len(t, runner.calls, 1)
}
