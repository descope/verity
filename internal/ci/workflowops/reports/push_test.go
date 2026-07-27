package reports

import (
	"bytes"
	"context"
	"encoding/base64"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/verity-org/verity/internal/ci/workflowops/githubapi"
	"github.com/verity-org/verity/internal/ci/workflowops/retry"
)

func TestPusher_refetches_SHA_after_concurrent_update(t *testing.T) {
	// Given: a valid report whose first PUT loses a concurrent update race.
	dir := t.TempDir()
	localPath := filepath.Join(dir, "report.json")
	report := []byte(`{"status":"success"}`)
	require.NoError(t, os.WriteFile(localPath, report, 0o644))
	repository, err := githubapi.NewRepository("verity-org/verity")
	require.NoError(t, err)
	github := &fakeGitHub{
		shas:    []string{"old-sha", "new-sha"},
		putErrs: []error{&githubapi.StatusError{StatusCode: 409, Operation: "put report"}, nil},
	}
	var output bytes.Buffer
	pusher := Pusher{
		GitHub: github,
		Engine: retry.Engine{Sleeper: instantSleeper{}, Random: zeroRandom{}},
		Clock:  fixedClock{now: time.Date(2026, 7, 24, 9, 0, 0, 0, time.UTC)},
		Stdout: &output,
	}

	// When: the report is pushed with two attempts available.
	result, err := pusher.Push(t.Context(), &PushOptions{
		Repository:     repository,
		Branch:         "reports",
		Files:          []File{{RemotePath: "reports/example/latest.json", LocalPath: localPath}},
		Attempts:       2,
		AttemptTimeout: time.Second,
	})

	// Then: the second attempt uses fresh metadata and publishes the exact JSON bytes.
	require.NoError(t, err)
	assert.Equal(t, 1, result.Pushed)
	assert.Equal(t, []string{"old-sha", "new-sha"}, github.putSHAs)
	assert.Equal(t, base64.StdEncoding.EncodeToString(report), github.contents[1])
	assert.Contains(t, output.String(), "reports/reports/example/latest.json")
}

func TestPusher_rejects_tampered_set_before_any_remote_mutation(t *testing.T) {
	// Given: one valid report followed by a multi-document tampered report.
	dir := t.TempDir()
	valid := filepath.Join(dir, "valid.json")
	tampered := filepath.Join(dir, "tampered.json")
	require.NoError(t, os.WriteFile(valid, []byte(`{"ok":true}`), 0o644))
	require.NoError(t, os.WriteFile(tampered, []byte("{}\n{}\n"), 0o644))
	repository, err := githubapi.NewRepository("verity-org/verity")
	require.NoError(t, err)
	github := &fakeGitHub{}
	pusher := Pusher{GitHub: github, Clock: fixedClock{now: time.Date(2026, 7, 24, 9, 0, 0, 0, time.UTC)}}

	// When: the full report set is preflighted.
	_, err = pusher.Push(t.Context(), &PushOptions{
		Repository: repository,
		Branch:     "reports",
		Files: []File{
			{RemotePath: "reports/valid.json", LocalPath: valid},
			{RemotePath: "reports/tampered.json", LocalPath: tampered},
		},
		Attempts:       1,
		AttemptTimeout: time.Second,
	})

	// Then: validation fails closed before the valid prefix can be pushed.
	require.ErrorIs(t, err, ErrInvalidReport)
	assert.Zero(t, github.getCalls)
	assert.Empty(t, github.putSHAs)
}

func TestPusher_creates_report_when_remote_file_is_missing(t *testing.T) {
	// Given: a valid report with no existing Contents API resource.
	dir := t.TempDir()
	localPath := filepath.Join(dir, "report.json")
	require.NoError(t, os.WriteFile(localPath, []byte(`{"status":"success"}`), 0o644))
	repository, err := githubapi.NewRepository("verity-org/verity")
	require.NoError(t, err)
	github := &fakeGitHub{}
	pusher := Pusher{GitHub: github, Clock: fixedClock{now: time.Date(2026, 7, 24, 9, 0, 0, 0, time.UTC)}}

	// When: the report is pushed once.
	result, err := pusher.Push(t.Context(), &PushOptions{
		Repository:     repository,
		Branch:         "reports",
		Files:          []File{{RemotePath: "reports/example/latest.json", LocalPath: localPath}},
		Attempts:       1,
		AttemptTimeout: time.Second,
	})

	// Then: creation omits SHA and succeeds.
	require.NoError(t, err)
	assert.Equal(t, 1, result.Pushed)
	assert.Equal(t, []string{""}, github.putSHAs)
}

func TestPusher_bounds_GitHub_attempt_by_timeout(t *testing.T) {
	// Given: a valid report and a GitHub call that blocks until its attempt context ends.
	dir := t.TempDir()
	localPath := filepath.Join(dir, "report.json")
	require.NoError(t, os.WriteFile(localPath, []byte(`{"status":"success"}`), 0o644))
	repository, err := githubapi.NewRepository("verity-org/verity")
	require.NoError(t, err)
	pusher := Pusher{
		GitHub: blockingReportGitHub{},
		Engine: retry.Engine{Sleeper: instantSleeper{}, Random: zeroRandom{}},
		Clock:  fixedClock{now: time.Date(2026, 7, 24, 9, 0, 0, 0, time.UTC)},
	}
	started := time.Now()

	// When: the outbound metadata request exceeds its per-attempt timeout.
	_, err = pusher.Push(t.Context(), &PushOptions{
		Repository: repository, Branch: "reports",
		Files:    []File{{RemotePath: "reports/example/latest.json", LocalPath: localPath}},
		Attempts: 2, AttemptTimeout: 20 * time.Millisecond,
	})

	// Then: cancellation returns promptly without a retry delay or second attempt.
	require.ErrorIs(t, err, context.DeadlineExceeded)
	assert.Less(t, time.Since(started), 150*time.Millisecond)
}

type fakeGitHub struct {
	shas     []string
	putErrs  []error
	putSHAs  []string
	contents []string
	getCalls int
}

func (github *fakeGitHub) GetContentSHA(context.Context, githubapi.GetContentRequest) (string, error) {
	index := github.getCalls
	github.getCalls++
	if index >= len(github.shas) {
		return "", githubapi.ErrNotFound
	}
	return github.shas[index], nil
}

func (github *fakeGitHub) PutContent(_ context.Context, request *githubapi.PutContentRequest) error {
	github.putSHAs = append(github.putSHAs, request.SHA)
	github.contents = append(github.contents, request.Content)
	index := len(github.putSHAs) - 1
	if index < len(github.putErrs) {
		return github.putErrs[index]
	}
	return nil
}

type instantSleeper struct{}

func (instantSleeper) Wait(context.Context, time.Duration) error {
	return nil
}

type zeroRandom struct{}

func (zeroRandom) Intn(int) (int, error) {
	return 0, nil
}

type fixedClock struct {
	now time.Time
}

type blockingReportGitHub struct{}

func (blockingReportGitHub) GetContentSHA(ctx context.Context, _ githubapi.GetContentRequest) (string, error) {
	<-ctx.Done()
	return "", ctx.Err()
}

func (blockingReportGitHub) PutContent(context.Context, *githubapi.PutContentRequest) error {
	return nil
}

func (clock fixedClock) Now() time.Time {
	return clock.now
}
