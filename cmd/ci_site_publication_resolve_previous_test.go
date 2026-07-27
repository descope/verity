package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/verity-org/verity/internal/apkrepository"
)

func TestCISitePublicationResolvePrevious_exposes_exact_contract(t *testing.T) {
	// Given the site-publication command tree.
	var resolverFound bool
	flags := map[string]bool{}

	// When the resolver command is inspected.
	for _, command := range ciSitePublicationCommand.Commands {
		if command.Name != "resolve-previous" {
			continue
		}
		resolverFound = true
		for _, flag := range command.Flags {
			for _, name := range flag.Names() {
				flags[name] = true
			}
		}
	}

	// Then every required trust and output selector is explicit.
	assert.True(t, resolverFound)
	for _, name := range []string{"repository", "workflow", "branch", "artifact-name", "before-run-id", "github-output"} {
		assert.True(t, flags[name], "missing resolver flag %q", name)
	}
}

func TestAppendPreviousPublicationOutputs_emits_complete_record(t *testing.T) {
	// Given one fully verified previous publication.
	path := filepath.Join(t.TempDir(), "github-output")
	resolved := apkrepository.PreviousPublication{
		RunID: 60, RunAttempt: 2, SourceSHA: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		ArtifactDigest: "sha256:" + strings.Repeat("1", 64), ManifestDigest: "sha256:" + strings.Repeat("2", 64),
	}

	// When the resolver appends GitHub outputs.
	err := appendPreviousPublicationOutputs(path, &resolved)

	// Then all five values are emitted together after successful verification.
	require.NoError(t, err)
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(
		t,
		"run_id=60\nrun_attempt=2\nsource_sha=aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa\n"+
			"artifact_digest="+resolved.ArtifactDigest+"\nmanifest_digest="+resolved.ManifestDigest+"\n",
		string(data),
	)
}
