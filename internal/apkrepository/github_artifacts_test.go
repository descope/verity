package apkrepository

import (
	"bytes"
	"context"
	"os"
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

func TestRestorePrevious_restores_latest_successful_main_pages_APK_subtree(t *testing.T) {
	// Given retained Pages artifacts where only the newest successful main run is eligible.
	output := filepath.Join(t.TempDir(), "previous")
	archive := pagesArtifactZip(t, map[string]string{
		"./apk/x86_64/demo.apk":        "package",
		"./apk/x86_64/APKINDEX.tar.gz": "index",
		"./index.html":                 "site",
	})
	runner := &fakeCommandRunner{}
	runner.run = func(request command) (commandResult, error) {
		joined := strings.Join(request.args, " ")
		switch {
		case strings.Contains(joined, "actions/artifacts/6/zip"):
			_, err := request.stdout.Write(archive)
			return commandResult{}, err
		case strings.Contains(joined, "actions/artifacts"):
			return commandResult{stdout: []byte(`{"artifacts":[{"id":7,"expired":false,"created_at":"2026-07-24T02:00:00Z","workflow_run":{"id":70,"head_branch":"main"}},{"id":6,"expired":false,"created_at":"2026-07-23T02:00:00Z","workflow_run":{"id":60,"head_branch":"main"}}]}`)}, nil
		case strings.Contains(joined, "actions/runs/70"):
			return commandResult{stdout: []byte(`{"conclusion":"failure"}`)}, nil
		case strings.Contains(joined, "actions/runs/60"):
			return commandResult{stdout: []byte(`{"conclusion":"success"}`)}, nil
		default:
			return commandResult{}, assert.AnError
		}
	}
	var stdout bytes.Buffer

	// When previous Pages state is restored.
	err := RestorePrevious(context.Background(), &RestorePreviousOptions{
		OutputDir: output, Repository: "verity-org/verity", Stdout: &stdout, runner: runner,
	})

	// Then only the APK subtree from the latest successful main artifact is extracted.
	require.NoError(t, err)
	assert.FileExists(t, filepath.Join(output, "apk", "x86_64", "demo.apk"))
	assert.NoFileExists(t, filepath.Join(output, "index.html"))
	assert.Contains(t, stdout.String(), "Restored previous Pages artifact 6")
}

func TestRestorePrevious_bootstraps_when_no_successful_artifact_exists(t *testing.T) {
	// Given no retained Pages artifacts.
	output := filepath.Join(t.TempDir(), "previous")
	writeTestFile(t, filepath.Join(output, "stale"), "stale")
	runner := &fakeCommandRunner{run: func(command) (commandResult, error) {
		return commandResult{stdout: []byte(`{"artifacts":[]}`)}, nil
	}}
	var stdout bytes.Buffer

	// When previous state is restored.
	err := RestorePrevious(context.Background(), &RestorePreviousOptions{
		OutputDir: output, Repository: "verity-org/verity", Stdout: &stdout, runner: runner,
	})

	// Then stale local state is cleared and publication bootstraps explicitly.
	require.NoError(t, err)
	_, statErr := os.Stat(filepath.Join(output, "stale"))
	assert.True(t, os.IsNotExist(statErr))
	assert.Contains(t, stdout.String(), "will bootstrap from the candidate set")
}

func TestRestorePrevious_rejects_archive_path_traversal(t *testing.T) {
	// Given a retained artifact with a malicious APK tar path.
	output := filepath.Join(t.TempDir(), "previous")
	archive := pagesArtifactZip(t, map[string]string{"./apk/../../escape": "bad"})
	runner := &fakeCommandRunner{run: func(request command) (commandResult, error) {
		joined := strings.Join(request.args, " ")
		switch {
		case strings.Contains(joined, "actions/artifacts/9/zip"):
			_, err := request.stdout.Write(archive)
			return commandResult{}, err
		case strings.Contains(joined, "actions/runs/90"):
			return commandResult{stdout: []byte(`{"conclusion":"success"}`)}, nil
		case strings.Contains(joined, "actions/artifacts"):
			return commandResult{stdout: []byte(`{"artifacts":[{"id":9,"expired":false,"created_at":"2026-07-24T02:00:00Z","workflow_run":{"id":90,"head_branch":"main"}}]}`)}, nil
		default:
			return commandResult{}, assert.AnError
		}
	}}

	// When the artifact is extracted.
	err := RestorePrevious(context.Background(), &RestorePreviousOptions{
		OutputDir: output, Repository: "verity-org/verity", runner: runner,
	})

	// Then extraction fails before writing outside the output root.
	require.Error(t, err)
	assert.ErrorContains(t, err, "unsafe Pages artifact path")
}

func TestRestorePrevious_rejects_double_dot_archive_entry(t *testing.T) {
	// Given an APK archive entry containing the traversal token CodeQL tracks.
	output := filepath.Join(t.TempDir(), "previous")
	archive := pagesArtifactZip(t, map[string]string{"./apk/demo..apk": "bad"})
	runner := &fakeCommandRunner{run: func(request command) (commandResult, error) {
		joined := strings.Join(request.args, " ")
		switch {
		case strings.Contains(joined, "actions/artifacts/9/zip"):
			_, err := request.stdout.Write(archive)
			return commandResult{}, err
		case strings.Contains(joined, "actions/runs/90"):
			return commandResult{stdout: []byte(`{"conclusion":"success"}`)}, nil
		case strings.Contains(joined, "actions/artifacts"):
			return commandResult{stdout: []byte(`{"artifacts":[{"id":9,"expired":false,"created_at":"2026-07-24T02:00:00Z","workflow_run":{"id":90,"head_branch":"main"}}]}`)}, nil
		default:
			return commandResult{}, assert.AnError
		}
	}}

	// When the artifact is extracted.
	err := RestorePrevious(context.Background(), &RestorePreviousOptions{
		OutputDir: output, Repository: "verity-org/verity", runner: runner,
	})

	// Then the entry is rejected before its tainted name reaches a filesystem operation.
	require.Error(t, err)
	assert.ErrorContains(t, err, "unsafe Pages artifact path")
}
