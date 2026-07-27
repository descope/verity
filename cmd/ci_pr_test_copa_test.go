package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestResolvePRCopaMetadata_returns_typed_catalog_and_destination(t *testing.T) {
	// Given: a catalog entry with a Go source tag prefix and an unsafe artifact name.
	config := filepath.Join(t.TempDir(), "copa-config.yaml")
	require.NoError(t, os.WriteFile(config, []byte(`images:
  - name: org/example:image
    image: registry.example.com/org/example
    goVcsUrl: https://github.com/example/project
    goVcsTagPrefix: v
`), 0o600))

	// When: the PR patch metadata is resolved.
	metadata, err := resolvePRCopaMetadata(&prCopaMetadataInput{
		ConfigPath: config, ImageName: "org/example:image", ImageTag: "1.2.3",
		Platform: "linux/amd64", StagingRegistry: "localhost:5000/regression",
	})

	// Then: workflow outputs are bounded, catalog-derived, and deterministic.
	require.NoError(t, err)
	require.Equal(t, "registry.example.com/org/example:1.2.3", metadata.Source)
	require.Equal(t, "https://github.com/example/project@v1.2.3", metadata.GoVCSURL)
	require.Equal(t, "org-example-image", metadata.ArtifactName)
	require.Equal(t, "localhost:5000/regression:org-example-image-1.2.3-amd64", metadata.Destination)
}

func TestPinnedPRCopaImage_requires_sha256_digest(t *testing.T) {
	// Given: a valid destination tag and a registry digest.
	destination := "localhost:5000/regression:example-1-amd64"
	digest := "sha256:" + strings.Repeat("b", 64)

	// When: the patched runtime reference is composed.
	got, err := pinnedPRCopaImage(destination, digest)

	// Then: Trivy receives a digest-bound tag reference.
	require.NoError(t, err)
	require.Equal(t, destination+"@"+digest, got)

	_, err = pinnedPRCopaImage(destination, "latest")
	require.ErrorIs(t, err, errPRCommandFailed)
}
