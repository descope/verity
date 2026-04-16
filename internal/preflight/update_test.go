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

func TestUpdateManifest_NewManifest(t *testing.T) {
	callCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		switch r.Method {
		case http.MethodGet:
			w.WriteHeader(http.StatusNotFound)
		case http.MethodPut:
			w.WriteHeader(http.StatusCreated)
		}
	}))
	defer srv.Close()

	origFetch := fetchManifestWithSHAURL
	origPush := pushManifestURL
	fetchManifestWithSHAURL = func(_, _ string) string { return srv.URL }
	pushManifestURL = func(_ string) string { return srv.URL }
	defer func() {
		fetchManifestWithSHAURL = origFetch
		pushManifestURL = origPush
	}()

	err := UpdateManifest("test/repo", "reports", "fake-token", UpdateEntry{
		ImageName:      "nginx",
		Tag:            "1.29.3",
		UpstreamDigest: "sha256:abc",
		PatchedVulns:   0,
	})
	require.NoError(t, err)
	assert.Equal(t, 2, callCount)
}

func TestUpdateManifest_ExistingManifest(t *testing.T) {
	existing := Manifest{
		"valkey/9.0.3": {UpstreamDigest: "sha256:old", PatchedVulns: 1},
	}
	data, err := json.Marshal(existing)
	require.NoError(t, err)
	encoded := base64.StdEncoding.EncodeToString(data)

	var putBody map[string]string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			resp := map[string]string{"content": encoded, "sha": "abc123"}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp) //nolint:errcheck // test helper
		case http.MethodPut:
			json.NewDecoder(r.Body).Decode(&putBody) //nolint:errcheck // test helper
			w.WriteHeader(http.StatusOK)
		}
	}))
	defer srv.Close()

	origFetch := fetchManifestWithSHAURL
	origPush := pushManifestURL
	fetchManifestWithSHAURL = func(_, _ string) string { return srv.URL }
	pushManifestURL = func(_ string) string { return srv.URL }
	defer func() {
		fetchManifestWithSHAURL = origFetch
		pushManifestURL = origPush
	}()

	err = UpdateManifest("test/repo", "reports", "fake-token", UpdateEntry{
		ImageName:      "nginx",
		Tag:            "1.29.3",
		UpstreamDigest: "sha256:new",
		PatchedVulns:   2,
	})
	require.NoError(t, err)

	assert.Equal(t, "abc123", putBody["sha"])

	decoded, err := base64.StdEncoding.DecodeString(putBody["content"])
	require.NoError(t, err)
	var merged Manifest
	require.NoError(t, json.Unmarshal(decoded, &merged))
	assert.Contains(t, merged, "valkey/9.0.3")
	assert.Contains(t, merged, "nginx/1.29.3")
	assert.Equal(t, "sha256:new", merged["nginx/1.29.3"].UpstreamDigest)
}

func TestUpdateManifest_PushError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			w.WriteHeader(http.StatusNotFound)
		case http.MethodPut:
			w.WriteHeader(http.StatusForbidden)
			w.Write([]byte(`{"message":"forbidden"}`)) //nolint:errcheck // test helper
		}
	}))
	defer srv.Close()

	origFetch := fetchManifestWithSHAURL
	origPush := pushManifestURL
	fetchManifestWithSHAURL = func(_, _ string) string { return srv.URL }
	pushManifestURL = func(_ string) string { return srv.URL }
	defer func() {
		fetchManifestWithSHAURL = origFetch
		pushManifestURL = origPush
	}()

	err := UpdateManifest("test/repo", "reports", "fake-token", UpdateEntry{
		ImageName: "nginx",
		Tag:       "1.29.3",
	})
	assert.Error(t, err)
	assert.ErrorIs(t, err, ErrUnexpectedStatus)
}

func TestUpdateManifest_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	origFetch := fetchManifestWithSHAURL
	fetchManifestWithSHAURL = func(_, _ string) string { return srv.URL }
	defer func() { fetchManifestWithSHAURL = origFetch }()

	err := UpdateManifest("test/repo", "reports", "fake-token", UpdateEntry{
		ImageName: "nginx",
		Tag:       "1.29.3",
	})
	assert.Error(t, err)
}
