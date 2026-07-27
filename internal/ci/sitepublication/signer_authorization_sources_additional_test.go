package sitepublication

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/verity-org/verity/internal/ci/publication"
)

func TestBuildSignerPlan_rejects_untrusted_authorization_sources(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*testing.T, *SignerRequest)
	}{
		{
			name: "missing publication manifest",
			mutate: func(t *testing.T, request *SignerRequest) {
				require.NoError(t, os.Remove(filepath.Join(request.WorkspaceDir, request.ManifestPath)))
			},
		},
		{
			name: "invalid publication manifest",
			mutate: func(t *testing.T, request *SignerRequest) {
				writeSignerFixtureBytes(t, request, request.ManifestPath, []byte(`{}`))
			},
		},
		{
			name: "manifest identity mismatch",
			mutate: func(t *testing.T, request *SignerRequest) {
				_, manifest := validPlanAndManifest(t)
				manifest.SignerDigest = digestOf("e")
				data, err := publication.MarshalCanonical(&manifest)
				require.NoError(t, err)
				writeSignerFixtureBytes(t, request, request.ManifestPath, data)
			},
		},
		{
			name: "empty packages directory",
			mutate: func(t *testing.T, request *SignerRequest) {
				path := filepath.Join(request.WorkspaceDir, request.PackagesPath, "demo.apk")
				require.NoError(t, os.Remove(path))
			},
		},
		{
			name: "invalid package semantics",
			mutate: func(t *testing.T, request *SignerRequest) {
				path := filepath.Join(request.WorkspaceDir, request.PackagesPath, "demo.apk")
				require.NoError(t, os.WriteFile(path, []byte("not-an-apk"), 0o644))
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Given
			publicationPlan, _ := validPlanAndManifest(t)
			request := signerRequest(t, &publicationPlan, t.TempDir())
			test.mutate(t, request)

			// When
			plan, err := BuildSignerPlan(request)

			// Then
			require.ErrorIs(t, err, ErrInvalidSignerPlan)
			assert.Equal(t, SignerPlan{}, plan)
		})
	}
}
