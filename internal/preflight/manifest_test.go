package preflight

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestManifestKey(t *testing.T) {
	tests := []struct {
		name  string
		image string
		tag   string
		want  string
	}{
		{name: "simple", image: "nginx", tag: "1.29.3", want: "nginx/1.29.3"},
		{name: "scoped image", image: "library/nginx", tag: "latest", want: "library/nginx/latest"},
		{name: "integer style", image: "node", tag: "22-default", want: "node/22-default"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, ManifestKey(tc.image, tc.tag))
		})
	}
}

func TestFetchManifest_NotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	origFetch := fetchManifestURL
	fetchManifestURL = func(_, _ string) string { return srv.URL }
	defer func() { fetchManifestURL = origFetch }()

	m, err := FetchManifest("test/repo", "reports", "")
	require.NoError(t, err)
	assert.Empty(t, m)
}

func TestFetchManifest_ValidManifest(t *testing.T) {
	manifest := Manifest{
		"nginx/1.29.3": {UpstreamDigest: "sha256:aaa", PatchedVulns: 0, LastPatched: "2026-03-07T00:00:00Z"},
		"valkey/9.0.3": {UpstreamDigest: "sha256:bbb", PatchedVulns: 3, LastPatched: "2026-03-06T00:00:00Z"},
	}
	data, err := json.Marshal(manifest)
	require.NoError(t, err)
	encoded := base64.StdEncoding.EncodeToString(data)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		resp := map[string]string{"content": encoded, "encoding": "base64"}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp) //nolint:errcheck // test helper
	}))
	defer srv.Close()

	origFetch := fetchManifestURL
	fetchManifestURL = func(_, _ string) string { return srv.URL }
	defer func() { fetchManifestURL = origFetch }()

	m, err := FetchManifest("test/repo", "reports", "")
	require.NoError(t, err)
	assert.Len(t, m, 2)
	assert.Equal(t, "sha256:aaa", m["nginx/1.29.3"].UpstreamDigest)
	assert.Equal(t, 3, m["valkey/9.0.3"].PatchedVulns)
}

func TestFetchManifest_APIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	origFetch := fetchManifestURL
	fetchManifestURL = func(_, _ string) string { return srv.URL }
	defer func() { fetchManifestURL = origFetch }()

	_, err := FetchManifest("test/repo", "reports", "")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "500")
}
