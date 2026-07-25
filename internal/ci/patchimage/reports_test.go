package patchimage

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/verity-org/verity/internal/ci/workflowops/retry"
)

func TestPreviousReportService_Download_decodesGitHubContent(t *testing.T) {
	// Given
	report := []byte(`{"Results":[]}`)
	response, err := json.Marshal(map[string]string{"content": base64.StdEncoding.EncodeToString(report)})
	require.NoError(t, err)
	path := filepath.Join(t.TempDir(), "prev-post.json")
	service := PreviousReportService{Runner: &fakeRunner{results: []runnerResult{{result: retry.Result{Stdout: response}}}}}

	// When
	result, err := service.Download(t.Context(), PreviousReportRequest{
		Repository: "verity-org/verity", ImageName: "nginx", SourceTag: "1.29.3", Destination: path,
	})

	// Then
	require.NoError(t, err)
	assert.True(t, result.Exists)
	written, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, report, written)
}

func TestPreviousReportService_Download_writesEmptyReportWhenGitHubFails(t *testing.T) {
	// Given
	path := filepath.Join(t.TempDir(), "prev-post.json")
	service := PreviousReportService{Runner: &fakeRunner{results: []runnerResult{{err: assert.AnError}}}}

	// When
	result, err := service.Download(t.Context(), PreviousReportRequest{
		Repository: "verity-org/verity", ImageName: "nginx", SourceTag: "1.29.3", Destination: path,
	})

	// Then
	require.NoError(t, err)
	assert.False(t, result.Exists)
	written, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.JSONEq(t, `{"Results":[]}`, string(written))
}

func TestPreflightService_Update_retriesSHAConflictWithFreshContent(t *testing.T) {
	// Given
	first := githubContentResponse(t, "old-sha", `{}`)
	second := githubContentResponse(t, "new-sha", `{"other":{"upstream_digest":"kept"}}`)
	runner := &fakeRunner{results: []runnerResult{
		{result: retry.Result{Stdout: first}},
		{err: assert.AnError},
		{result: retry.Result{Stdout: second}},
		{result: retry.Result{}},
	}}
	sleeper := &fakeSleeper{}
	service := PreflightService{Runner: runner, Sleeper: sleeper, Clock: fixedClock{value: time.Date(2026, time.July, 25, 12, 0, 0, 0, time.UTC)}}

	// When
	result, err := service.Update(t.Context(), &PreflightRequest{
		Repository: "verity-org/verity", ImageName: "nginx", SourceTag: "1.29.3",
		UpstreamDigest: "sha256:upstream", PatchedVulnerabilities: 0, MaxAttempts: 5, RetryDelay: 2 * time.Second,
	})

	// Then
	require.NoError(t, err)
	assert.True(t, result.Updated)
	assert.Equal(t, []time.Duration{2 * time.Second}, sleeper.delays)
	require.Len(t, runner.calls, 4)
	var payload struct {
		SHA     string `json:"sha"`
		Content string `json:"content"`
	}
	require.NoError(t, json.Unmarshal(runner.calls[3].Stdin, &payload))
	assert.Equal(t, "new-sha", payload.SHA)
	decoded, err := base64.StdEncoding.DecodeString(payload.Content)
	require.NoError(t, err)
	assert.Contains(t, string(decoded), `"other"`)
	assert.Contains(t, string(decoded), `"nginx/1.29.3"`)
}

func githubContentResponse(t *testing.T, sha, content string) []byte {
	t.Helper()
	response, err := json.Marshal(map[string]string{
		"sha": sha, "content": base64.StdEncoding.EncodeToString([]byte(content)),
	})
	require.NoError(t, err)
	return response
}

type fakeSleeper struct {
	delays []time.Duration
}

func (sleeper *fakeSleeper) Wait(_ context.Context, delay time.Duration) error {
	sleeper.delays = append(sleeper.delays, delay)
	return nil
}

type fixedClock struct {
	value time.Time
}

func (clock fixedClock) Now() time.Time {
	return clock.value
}
