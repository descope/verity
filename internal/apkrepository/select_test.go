package apkrepository

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSelect_preserves_previous_bytes_when_package_state_is_unchanged(t *testing.T) {
	// Given equivalent package/key/format state but different generated indexes.
	root := t.TempDir()
	candidate := filepath.Join(root, "candidate")
	previous := filepath.Join(root, "previous")
	output := filepath.Join(root, "output")
	for _, repository := range []string{candidate, previous} {
		writeTestFile(t, filepath.Join(repository, "x86_64", "demo.apk"), "same package")
		writeTestFile(t, filepath.Join(repository, "verity.rsa.pub"), "same key")
		writeTestFile(t, filepath.Join(repository, "repository-format"), "1\n")
	}
	writeTestFile(t, filepath.Join(candidate, "x86_64", "APKINDEX.tar.gz"), "new index")
	writeTestFile(t, filepath.Join(previous, "x86_64", "APKINDEX.tar.gz"), "published index")
	writeTestFile(t, filepath.Join(output, "index.html"), "docs")
	var stdout bytes.Buffer

	// When the publication repository is selected.
	err := Select(context.Background(), &SelectOptions{
		CandidateDir: candidate,
		PreviousDir:  previous,
		OutputDir:    output,
		Stdout:       &stdout,
	})

	// Then the prior signed index is retained byte-for-byte and docs survive.
	require.NoError(t, err)
	assert.Contains(t, stdout.String(), "repository state unchanged")
	index, readErr := os.ReadFile(filepath.Join(output, "x86_64", "APKINDEX.tar.gz"))
	require.NoError(t, readErr)
	assert.Equal(t, "published index", string(index))
	assert.FileExists(t, filepath.Join(output, "index.html"))
}

func TestSelect_publishes_candidate_when_package_or_trust_state_changes(t *testing.T) {
	tests := []struct {
		name             string
		candidatePackage string
		previousPackage  string
		candidateKey     string
		previousKey      string
	}{
		{name: "package changes", candidatePackage: "new", previousPackage: "old", candidateKey: "key", previousKey: "key"},
		{name: "trust root changes", candidatePackage: "same", previousPackage: "same", candidateKey: "new key", previousKey: "old key"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Given candidate state that differs from the previous repository.
			root := t.TempDir()
			candidate := filepath.Join(root, "candidate")
			previous := filepath.Join(root, "previous")
			output := filepath.Join(root, "output")
			writeTestFile(t, filepath.Join(candidate, "aarch64", "demo.apk"), test.candidatePackage)
			writeTestFile(t, filepath.Join(candidate, "aarch64", "APKINDEX.tar.gz"), "candidate index")
			writeTestFile(t, filepath.Join(candidate, "verity.rsa.pub"), test.candidateKey)
			writeTestFile(t, filepath.Join(previous, "aarch64", "demo.apk"), test.previousPackage)
			writeTestFile(t, filepath.Join(previous, "aarch64", "APKINDEX.tar.gz"), "previous index")
			writeTestFile(t, filepath.Join(previous, "verity.rsa.pub"), test.previousKey)
			var stdout bytes.Buffer

			// When the publication repository is selected.
			err := Select(context.Background(), &SelectOptions{
				CandidateDir: candidate,
				PreviousDir:  previous,
				OutputDir:    output,
				Stdout:       &stdout,
			})

			// Then the complete candidate repository replaces managed state.
			require.NoError(t, err)
			assert.Contains(t, stdout.String(), "repository state changed")
			index, readErr := os.ReadFile(filepath.Join(output, "aarch64", "APKINDEX.tar.gz"))
			require.NoError(t, readErr)
			assert.Equal(t, "candidate index", string(index))
		})
	}
}

func TestSelect_returns_cancellation_before_mutating_output(t *testing.T) {
	// Given a cancelled publication request and pre-existing output bytes.
	root := t.TempDir()
	output := filepath.Join(root, "output")
	marker := filepath.Join(output, "index.html")
	writeTestFile(t, marker, "preserve me")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	// When repository selection starts.
	err := Select(ctx, &SelectOptions{
		CandidateDir: filepath.Join(root, "missing-candidate"),
		PreviousDir:  filepath.Join(root, "missing-previous"),
		OutputDir:    output,
	})

	// Then cancellation is reported before any output byte changes.
	require.ErrorIs(t, err, context.Canceled)
	contents, readErr := os.ReadFile(marker)
	require.NoError(t, readErr)
	assert.Equal(t, "preserve me", string(contents))
}
