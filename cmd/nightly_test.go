package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/go-containerregistry/pkg/crane"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/urfave/cli/v3"

	"github.com/verity-org/verity/internal/discovery"
	intdiscovery "github.com/verity-org/verity/internal/integer/discovery"
)

var (
	errTestCloseFailed      = errors.New("close failed")
	errTestUnexpectedTarget = errors.New("should not be called")
	errTestBadTarget        = errors.New("bad target")
	errTestDigestNotFound   = errors.New("not found")
	errTestNetworkDown      = errors.New("network down")
	errTestWriteFailed      = errors.New("write failed")
)

func TestNightlySourceTag(t *testing.T) {
	tests := map[string]string{
		"registry.k8s.io/foo/bar:v1.2.3": "v1.2.3",
		"docker.io/library/nginx":        "latest",
		"localhost:5000/ns/img:tag":      "tag",
		"repo/img@sha256:abc":            "",
	}
	for ref, want := range tests {
		t.Run(ref, func(t *testing.T) {
			assert.Equal(t, want, sourceTag(ref))
		})
	}
}

func TestNightlyTargetRefs(t *testing.T) {
	copa, err := copaTargetRef(&discovery.DiscoveredImage{
		Name:           "prometheus/prometheus",
		Source:         "quay.io/prometheus/prometheus:v3.9.1",
		TargetRegistry: "ghcr.io/verity-org",
	})
	require.NoError(t, err)
	assert.Equal(t, "ghcr.io/verity-org/prometheus/prometheus:v3.9.1", copa)

	integer, err := integerTargetRef(&intdiscovery.DiscoveredImage{
		Name:     "node",
		Version:  "24",
		Type:     "default",
		Tags:     []string{"24", "latest"},
		Registry: "ghcr.io/verity-org",
	})
	require.NoError(t, err)
	assert.Equal(t, "ghcr.io/verity-org/node:24", integer)

	integerRefs, err := integerTargetRefs(&intdiscovery.DiscoveredImage{
		Name:     "node",
		Version:  "24",
		Type:     "dev",
		Tags:     []string{"24-dev", "24.18-dev", "latest-dev"},
		Registry: "ghcr.io/verity-org",
	})
	require.NoError(t, err)
	assert.Equal(t, []nightlyScanTarget{
		{ref: "ghcr.io/verity-org/node:24-dev", label: "ghcr.io/verity-org/node:24-dev"},
		{ref: "ghcr.io/verity-org/node:24.18-dev", label: "ghcr.io/verity-org/node:24.18-dev"},
		{ref: "ghcr.io/verity-org/node:latest-dev", label: "ghcr.io/verity-org/node:latest-dev"},
	}, integerRefs)
}

func TestNightlyTargetRefErrors(t *testing.T) {
	_, err := copaTargetRef(&discovery.DiscoveredImage{
		Name:           "digest",
		Source:         "repo/img@sha256:abc",
		TargetRegistry: "ghcr.io/verity-org",
	})
	require.ErrorIs(t, err, errSourceTagUnavailable)

	_, err = copaTargetRef(&discovery.DiscoveredImage{
		Name:   "missing-registry",
		Source: "repo/img:tag",
	})
	require.ErrorIs(t, err, errMissingTargetRegistry)

	_, err = integerTargetRefs(&intdiscovery.DiscoveredImage{
		Name:     "node",
		Version:  "24",
		Type:     "default",
		Registry: "ghcr.io/verity-org",
	})
	require.ErrorIs(t, err, errMissingIntegerTags)

	_, err = integerTargetRefs(&intdiscovery.DiscoveredImage{
		Name:    "node",
		Version: "24",
		Type:    "default",
		Tags:    []string{"24"},
	})
	require.ErrorIs(t, err, errMissingIntegerRegistry)
}

func TestNightlyDispatchInputs(t *testing.T) {
	dir := t.TempDir()

	copaPath := filepath.Join(dir, "copa.json")
	copaData, err := json.Marshal([]discovery.DiscoveredImage{{
		Name:           "library/nginx",
		Source:         "mirror.gcr.io/library/nginx:1.29.3",
		TargetRegistry: "ghcr.io/verity-org",
		Platforms:      "linux/amd64,linux/arm64",
		GoVcsURL:       "https://github.com/nginx/nginx@release-1.29.3",
	}})
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(copaPath, copaData, 0o644))

	copa, err := nightlyDispatchInputs(nightlyFamilyCopa, copaPath)
	require.NoError(t, err)
	require.Equal(t, []map[string]string{{
		"image-name":      "library/nginx",
		"source-ref":      "mirror.gcr.io/library/nginx:1.29.3",
		"target-registry": "ghcr.io/verity-org",
		"platforms":       "linux/amd64,linux/arm64",
		"go-vcs-url":      "https://github.com/nginx/nginx@release-1.29.3",
	}}, copa)

	integerPath := filepath.Join(dir, "integer.json")
	integerData, err := json.Marshal([]intdiscovery.DiscoveredImage{{
		Name:     "node",
		Version:  "24",
		Type:     "dev",
		Tags:     []string{"24-dev", "latest-dev"},
		Registry: "ghcr.io/verity-org",
	}})
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(integerPath, integerData, 0o644))

	integer, err := nightlyDispatchInputs(nightlyFamilyInteger, integerPath)
	require.NoError(t, err)
	require.Equal(t, []map[string]string{{
		"image":    "node",
		"version":  "24",
		"type":     "dev",
		"tags":     "24-dev,latest-dev",
		"registry": "ghcr.io/verity-org",
	}}, integer)
}

func TestNightlyPlanPropagatesDiscoveryErrors(t *testing.T) {
	root := &cli.Command{Commands: []*cli.Command{NightlyCommand}}
	err := root.Run(context.Background(), []string{
		"verity", "nightly", "plan",
		"--family", nightlyFamilyCopa,
		"--config", filepath.Join(t.TempDir(), "missing.yaml"),
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "loading copa config")
}

type closeBuffer struct {
	bytes.Buffer
	closeErr error
}

func (b *closeBuffer) Close() error {
	return b.closeErr
}

func TestAppendGitHubMatrixOutputToWritesAndCloses(t *testing.T) {
	w := &closeBuffer{}
	err := appendGitHubMatrixOutputTo(w, "out", 2, []byte(`[{"name":"x"}]`))
	require.NoError(t, err)
	assert.Equal(t, "count=2\nimages<<__VERITY_NIGHTLY_JSON__\n[{\"name\":\"x\"}]\n__VERITY_NIGHTLY_JSON__\n", w.String())
}

func TestAppendGitHubMatrixOutputToReportsCloseError(t *testing.T) {
	closeErr := errTestCloseFailed
	err := appendGitHubMatrixOutputTo(&closeBuffer{closeErr: closeErr}, "out", 1, []byte(`[]`))
	require.ErrorIs(t, err, closeErr)
	assert.Contains(t, err.Error(), "closing GitHub output out")
}

type failingWriteCloser struct{}

func (failingWriteCloser) Write([]byte) (int, error) {
	return 0, errTestWriteFailed
}

func (failingWriteCloser) Close() error {
	return errTestCloseFailed
}

func TestAppendGitHubMatrixOutputToJoinsWriteAndCloseErrors(t *testing.T) {
	err := appendGitHubMatrixOutputTo(failingWriteCloser{}, "out", 1, []byte(`[]`))
	require.ErrorIs(t, err, errTestWriteFailed)
	require.ErrorIs(t, err, errTestCloseFailed)
}

func TestAppendGitHubMatrixOutputWritesFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "github-output")
	require.NoError(t, appendGitHubMatrixOutput(path, 0, []byte(`[]`)))
	got, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Contains(t, string(got), "count=0\n")
	assert.Contains(t, string(got), "[]")
}

func TestAppendGitHubMatrixOutputReportsOpenError(t *testing.T) {
	err := appendGitHubMatrixOutput(filepath.Join(t.TempDir(), "missing", "out"), 1, []byte(`[]`))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "opening GitHub output")
}

func TestFilterDirtyForceSkipsTargetResolution(t *testing.T) {
	want := []string{"a", "b"}
	got, err := filterDirty(context.Background(), want, 1, func(string) ([]nightlyScanTarget, string, error) {
		return nil, "", errTestUnexpectedTarget
	}, true, func(s string) string { return s })
	require.NoError(t, err)
	assert.Equal(t, want, got)
}

func TestFilterDirtyKeepsTargetResolutionErrors(t *testing.T) {
	targetErr := errTestBadTarget
	got, err := filterDirty(context.Background(), []string{"a"}, 1, func(string) ([]nightlyScanTarget, string, error) {
		return nil, "", targetErr
	}, false, func(s string) string { return s })
	require.NoError(t, err)
	assert.Equal(t, []string{"a"}, got)
}

func TestScanPublishedTargetsDedupesCleanDigests(t *testing.T) {
	restore := stubScanFunctions(t)
	digestCalls := 0
	scanCalls := 0
	craneDigest = func(string, ...crane.Option) (string, error) {
		digestCalls++
		return "sha256:same", nil
	}
	trivyVulnCountFor = func(context.Context, string) (int, error) {
		scanCalls++
		return 0, nil
	}
	t.Cleanup(restore)

	decision := scanPublishedTargets(context.Background(), []nightlyScanTarget{
		{ref: "repo/img:1"},
		{ref: "repo/img:latest"},
	})
	require.False(t, decision.dirty)
	assert.Equal(t, 2, digestCalls)
	assert.Equal(t, 1, scanCalls)
}

func TestScanPublishedTargetsMarksVulnerableImageDirty(t *testing.T) {
	restore := stubScanFunctions(t)
	craneDigest = func(string, ...crane.Option) (string, error) { return "sha256:vuln", nil }
	trivyVulnCountFor = func(context.Context, string) (int, error) { return 3, nil }
	t.Cleanup(restore)

	decision := scanPublishedTargets(context.Background(), []nightlyScanTarget{{ref: "repo/img:1"}})
	require.True(t, decision.dirty)
	assert.Contains(t, decision.reason, "3 vulnerabilities")
}

func TestScanPublishedTargetsMarksDigestFailureDirty(t *testing.T) {
	restore := stubScanFunctions(t)
	craneDigest = func(string, ...crane.Option) (string, error) { return "", errTestDigestNotFound }
	t.Cleanup(restore)

	decision := scanPublishedTargets(context.Background(), []nightlyScanTarget{{ref: "repo/img:1"}})
	require.True(t, decision.dirty)
	assert.Contains(t, decision.reason, "digest lookup failed")
}

func TestScanPublishedTargetsMarksScanFailureDirty(t *testing.T) {
	restore := stubScanFunctions(t)
	craneDigest = func(string, ...crane.Option) (string, error) { return "sha256:ok", nil }
	trivyVulnCountFor = func(context.Context, string) (int, error) { return 0, errTestBadTarget }
	t.Cleanup(restore)

	decision := scanPublishedTargets(context.Background(), []nightlyScanTarget{{ref: "repo/img:1"}})
	require.True(t, decision.dirty)
	assert.Contains(t, decision.reason, "scan failed")
}

func TestScanPublishedTargetsMarksEmptyTargetListDirty(t *testing.T) {
	decision := scanPublishedTargets(context.Background(), nil)
	require.True(t, decision.dirty)
	assert.Contains(t, decision.reason, "no target tags")
}

func TestDispatchWorkflowUsesGitHubAPI(t *testing.T) {
	var gotPath, gotAuth string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		var payload struct {
			Ref    string            `json:"ref"`
			Inputs map[string]string `json:"inputs"`
		}
		require.NoError(t, json.NewDecoder(r.Body).Decode(&payload))
		assert.Equal(t, "main", payload.Ref)
		assert.Equal(t, map[string]string{"image": "node"}, payload.Inputs)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()
	restore := stubGitHubClient(t, server)
	t.Cleanup(restore)

	err := dispatchWorkflow(context.Background(), "token", "verity-org/verity", "integer-build-image.yaml", "main", map[string]string{"image": "node"}, 1)
	require.NoError(t, err)
	assert.Equal(t, "/repos/verity-org/verity/actions/workflows/integer-build-image.yaml/dispatches", gotPath)
	assert.Equal(t, "Bearer token", gotAuth)
}

func TestDispatchWorkflowReportsGitHubStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "secondary rate limit", http.StatusTooManyRequests)
	}))
	defer server.Close()
	restore := stubGitHubClient(t, server)
	t.Cleanup(restore)

	err := dispatchWorkflow(context.Background(), "token", "verity-org/verity", "workflow.yaml", "main", nil, 1)
	require.ErrorIs(t, err, errGitHubDispatchStatus)
	assert.Contains(t, err.Error(), "secondary rate limit")
}

func TestDispatchWorkflowRetriesAfterStatusFailure(t *testing.T) {
	attempts := 0
	var slept time.Duration
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		attempts++
		if attempts == 1 {
			http.Error(w, "try again", http.StatusBadGateway)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()
	restoreClient := stubGitHubClient(t, server)
	oldSleep := dispatchRetrySleep
	dispatchRetrySleep = func(d time.Duration) { slept = d }
	t.Cleanup(func() {
		restoreClient()
		dispatchRetrySleep = oldSleep
	})

	err := dispatchWorkflow(context.Background(), "token", "verity-org/verity", "workflow.yaml", "main", nil, 2)
	require.NoError(t, err)
	assert.Equal(t, 2, attempts)
	assert.Equal(t, 10*time.Second, slept)
}

func TestNightlyTrivyVulnCountParsesReport(t *testing.T) {
	dir := t.TempDir()
	trivy := filepath.Join(dir, "trivy")
	script := `#!/usr/bin/env bash
cat <<'JSON'
{"Results":[{"Vulnerabilities":[{"VulnerabilityID":"CVE-1"}]},{"Vulnerabilities":[{"VulnerabilityID":"CVE-2"},{"VulnerabilityID":"CVE-3"}]}]}
JSON
`
	require.NoError(t, os.WriteFile(trivy, []byte(script), 0o755))
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))

	count, err := nightlyTrivyVulnCount(context.Background(), "repo/img:tag")
	require.NoError(t, err)
	assert.Equal(t, 3, count)
}

func TestNightlyTrivyVulnCountReportsInvalidJSON(t *testing.T) {
	dir := t.TempDir()
	trivy := filepath.Join(dir, "trivy")
	require.NoError(t, os.WriteFile(trivy, []byte("#!/usr/bin/env bash\necho nope\n"), 0o755))
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))

	_, err := nightlyTrivyVulnCount(context.Background(), "repo/img:tag")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "parsing trivy report")
}

func TestNightlyTrivyVulnCountReportsMissingTrivy(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	_, err := nightlyTrivyVulnCount(context.Background(), "repo/img:tag")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "trivy not found")
}

func TestNightlyTrivyVulnCountReportsCommandFailure(t *testing.T) {
	dir := t.TempDir()
	trivy := filepath.Join(dir, "trivy")
	require.NoError(t, os.WriteFile(trivy, []byte("#!/usr/bin/env bash\necho scan failed >&2\nexit 7\n"), 0o755))
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))

	_, err := nightlyTrivyVulnCount(context.Background(), "repo/img:tag")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "scan failed")
}

func stubScanFunctions(t *testing.T) func() {
	t.Helper()
	oldDigest := craneDigest
	oldTrivy := trivyVulnCountFor
	return func() {
		craneDigest = oldDigest
		trivyVulnCountFor = oldTrivy
	}
}

func stubGitHubClient(t *testing.T, server *httptest.Server) func() {
	t.Helper()
	oldBaseURL := githubAPIBaseURL
	oldClient := githubHTTPClient
	githubAPIBaseURL = server.URL
	githubHTTPClient = server.Client()
	return func() {
		githubAPIBaseURL = oldBaseURL
		githubHTTPClient = oldClient
	}
}

type failingReader struct{}

func (failingReader) Read([]byte) (int, error) {
	return 0, io.ErrUnexpectedEOF
}

func (failingReader) Close() error {
	return nil
}

func TestDispatchWorkflowReportsResponseReadError(t *testing.T) {
	transport := roundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusInternalServerError,
			Status:     "500 Internal Server Error",
			Body:       failingReader{},
		}, nil
	})
	oldBaseURL := githubAPIBaseURL
	oldClient := githubHTTPClient
	githubAPIBaseURL = "https://example.test"
	githubHTTPClient = &http.Client{Transport: transport}
	t.Cleanup(func() {
		githubAPIBaseURL = oldBaseURL
		githubHTTPClient = oldClient
	})

	err := dispatchWorkflow(context.Background(), "token", "verity-org/verity", "workflow.yaml", "main", nil, 1)
	require.ErrorIs(t, err, io.ErrUnexpectedEOF)
}

func TestDispatchWorkflowReportsNilBodyStatus(t *testing.T) {
	transport := roundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusInternalServerError,
			Status:     "500 Internal Server Error",
		}, nil
	})
	oldBaseURL := githubAPIBaseURL
	oldClient := githubHTTPClient
	githubAPIBaseURL = "https://example.test"
	githubHTTPClient = &http.Client{Transport: transport}
	t.Cleanup(func() {
		githubAPIBaseURL = oldBaseURL
		githubHTTPClient = oldClient
	})

	err := dispatchWorkflow(context.Background(), "token", "verity-org/verity", "workflow.yaml", "main", nil, 1)
	require.ErrorIs(t, err, errGitHubDispatchStatus)
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return fn(r)
}

func TestDispatchWorkflowReportsTransportError(t *testing.T) {
	transportErr := errTestNetworkDown
	oldClient := githubHTTPClient
	githubHTTPClient = &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return nil, transportErr
	})}
	t.Cleanup(func() { githubHTTPClient = oldClient })

	err := dispatchWorkflow(context.Background(), "token", "verity-org/verity", "workflow.yaml", "main", nil, 1)
	require.ErrorIs(t, err, transportErr)
}

func TestNightlyDispatchInputsRejectsUnsupportedFamily(t *testing.T) {
	path := filepath.Join(t.TempDir(), "matrix.json")
	require.NoError(t, os.WriteFile(path, []byte(`[]`), 0o644))
	_, err := nightlyDispatchInputs("bad", path)
	require.ErrorIs(t, err, errUnsupportedNightlyFamily)
	assert.True(t, strings.Contains(err.Error(), "bad"))
}

func TestNightlyDispatchRejectsMissingToken(t *testing.T) {
	t.Setenv("GH_TOKEN", "")
	t.Setenv("GITHUB_TOKEN", "")
	root := &cli.Command{Commands: []*cli.Command{NightlyCommand}}
	err := root.Run(context.Background(), []string{
		"verity", "nightly", "dispatch",
		"--family", nightlyFamilyCopa,
		"--input", filepath.Join(t.TempDir(), "missing.json"),
		"--repo", "verity-org/verity",
		"--ref", "main",
	})
	require.ErrorIs(t, err, errMissingGitHubToken)
}

func TestIntegerImageNamesSkipsBaseDirectory(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "_base"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "nested"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "node.yaml"), []byte("name: node"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "nested", "tool.yaml"), []byte("name: tool"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "_base", "skip.yaml"), []byte("skip"), 0o644))

	names, err := integerImageNames(dir)
	require.NoError(t, err)
	assert.Equal(t, map[string]struct{}{
		"node":        {},
		"nested/tool": {},
	}, names)
}
