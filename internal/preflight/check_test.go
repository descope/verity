package preflight

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/verity-org/verity/internal/discovery"
)

const testDigestSame = "sha256:same"

func TestExtractTag(t *testing.T) {
	tests := []struct {
		source string
		want   string
	}{
		{"mirror.gcr.io/library/nginx:1.29.3", "1.29.3"},
		{"mirror.gcr.io/library/nginx", "latest"},
		{"mirror.gcr.io/library/nginx@sha256:abc", ""},
	}
	for _, tc := range tests {
		assert.Equal(t, tc.want, extractTag(tc.source))
	}
}

func TestCheckCopaImage_FirstTime(t *testing.T) {
	img := discovery.DiscoveredImage{
		Name:   "valkey",
		Source: "mirror.gcr.io/valkey/valkey:9.0.3",
	}
	result := checkCopaImage(&img, Manifest{})

	assert.True(t, result.NeedsWork)
	assert.Contains(t, result.Reason, "first time")
}

func TestCheckCopaImage_UpstreamChanged(t *testing.T) {
	manifest := Manifest{
		"valkey/9.0.3": {UpstreamDigest: "sha256:old-valkey", PatchedVulns: 0},
	}
	img := discovery.DiscoveredImage{
		Name:   "valkey",
		Source: "mirror.gcr.io/valkey/valkey:9.0.3",
	}

	origFn := digestFn
	digestFn = func(_ string) (string, error) { return "sha256:new", nil }
	defer func() { digestFn = origFn }()

	result := checkCopaImage(&img, manifest)
	assert.True(t, result.NeedsWork)
	assert.Contains(t, result.Reason, "upstream digest changed")
}

func TestCheckCopaImage_UnchangedWithVulns(t *testing.T) {
	manifest := Manifest{
		"valkey/9.0.3": {UpstreamDigest: testDigestSame, PatchedVulns: 5},
	}
	img := discovery.DiscoveredImage{
		Name:   "valkey",
		Source: "mirror.gcr.io/valkey/valkey:9.0.3",
	}

	origFn := digestFn
	digestFn = func(_ string) (string, error) { return testDigestSame, nil }
	defer func() { digestFn = origFn }()

	result := checkCopaImage(&img, manifest)
	assert.True(t, result.NeedsWork)
	assert.Contains(t, result.Reason, "5 vulns remain in patched image")
}

func TestCheckCopaImage_SingleVulnUsesSingular(t *testing.T) {
	manifest := Manifest{
		"valkey/9.0.3": {UpstreamDigest: testDigestSame, PatchedVulns: 1},
	}
	img := discovery.DiscoveredImage{
		Name:   "valkey",
		Source: "mirror.gcr.io/valkey/valkey:9.0.3",
	}

	origFn := digestFn
	digestFn = func(_ string) (string, error) { return testDigestSame, nil }
	defer func() { digestFn = origFn }()

	result := checkCopaImage(&img, manifest)
	assert.True(t, result.NeedsWork)
	assert.Contains(t, result.Reason, "1 vuln remains in patched image",
		"single-vuln case must use singular noun and matching singular verb ('1 vuln remains'), not '1 vuln remain' or '1 vulns'")
	assert.NotContains(t, result.Reason, "1 vulns",
		"regression guard: prevent plural noun returning for count==1")
	assert.NotContains(t, result.Reason, "1 vuln remain ",
		"regression guard: prevent singular noun + plural verb ('1 vuln remain') subject-verb agreement bug")
}

func TestCheckCopaImage_UnchangedClean(t *testing.T) {
	manifest := Manifest{
		"nginx/1.29.3": {UpstreamDigest: testDigestSame, PatchedVulns: 0},
	}
	img := discovery.DiscoveredImage{
		Name:   "nginx",
		Source: "mirror.gcr.io/library/nginx:1.29.3",
	}

	origFn := digestFn
	digestFn = func(_ string) (string, error) { return testDigestSame, nil }
	defer func() { digestFn = origFn }()

	result := checkCopaImage(&img, manifest)
	assert.False(t, result.NeedsWork)
	assert.Contains(t, result.Reason, "unchanged")
}

func TestCheckCopaImage_DigestRef(t *testing.T) {
	img := discovery.DiscoveredImage{
		Name:   "valkey",
		Source: "mirror.gcr.io/valkey/valkey@sha256:abc123",
	}
	result := checkCopaImage(&img, Manifest{})

	assert.True(t, result.NeedsWork)
	assert.Contains(t, result.Reason, "digest-pinned")
}

func TestFilterCopaImages_MixedResults(t *testing.T) {
	images := []discovery.DiscoveredImage{
		{Name: "nginx", Source: "mirror.gcr.io/library/nginx:1.29.3"},
		{Name: "valkey", Source: "mirror.gcr.io/valkey/valkey:9.0.3"},
		{Name: "postgres", Source: "mirror.gcr.io/library/postgres:16.3"},
	}
	manifest := Manifest{
		"nginx/1.29.3":  {UpstreamDigest: "sha256:same-nginx", PatchedVulns: 0},
		"valkey/9.0.3":  {UpstreamDigest: "sha256:old-valkey", PatchedVulns: 0},
		"postgres/16.3": {UpstreamDigest: "sha256:same-pg", PatchedVulns: 2},
	}

	origFn := digestFn
	digestFn = func(ref string) (string, error) {
		switch ref {
		case "mirror.gcr.io/library/nginx:1.29.3":
			return "sha256:same-nginx", nil
		case "mirror.gcr.io/valkey/valkey:9.0.3":
			return "sha256:new-valkey", nil
		case "mirror.gcr.io/library/postgres:16.3":
			return "sha256:same-pg", nil
		default:
			return "", nil
		}
	}
	defer func() { digestFn = origFn }()

	needed, err := filterCopaImagesWithManifest(images, manifest)
	assert.NoError(t, err)

	names := make([]string, len(needed))
	for i, img := range needed {
		names[i] = img.Name
	}
	assert.ElementsMatch(t, []string{"valkey", "postgres"}, names)
}

func TestFilterCopaImages_WithManifestFetch(t *testing.T) {
	manifest := Manifest{
		"nginx/1.29.3": {UpstreamDigest: testDigestSame, PatchedVulns: 0},
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

	origDigest := digestFn
	digestFn = func(_ string) (string, error) { return testDigestSame, nil }
	defer func() { digestFn = origDigest }()

	images := []discovery.DiscoveredImage{
		{Name: "nginx", Source: "mirror.gcr.io/library/nginx:1.29.3"},
		{Name: "valkey", Source: "mirror.gcr.io/valkey/valkey:9.0.3"},
	}

	needed, err := FilterCopaImages(images, "test/repo", "reports", "")
	require.NoError(t, err)
	assert.Len(t, needed, 1)
	assert.Equal(t, "valkey", needed[0].Name)
}
