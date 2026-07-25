package apkrepository

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/verity-org/verity/internal/ci/sitepublication"
)

func TestResolvePrevious_selects_latest_earlier_attested_publication(t *testing.T) {
	// Given exact trusted workflow runs and one digest-bearing Pages artifact.
	fixture := newResolveFixture(t)
	fixture.runs = []priorWorkflowRun{fixture.runs[1], fixture.runs[0]}
	runner := fixture.runner(t)

	// When the previous publication is resolved before the current run.
	resolved, err := ResolvePrevious(context.Background(), fixture.options(runner))

	// Then the latest earlier identity and canonical artifact digests are returned.
	require.NoError(t, err)
	assert.Equal(t, fixture.restore.runID, resolved.RunID)
	assert.Equal(t, fixture.restore.attempt, resolved.RunAttempt)
	assert.Equal(t, restoreSourceSHA, resolved.SourceSHA)
	assert.Equal(t, fixture.restore.artifactDigest, resolved.ArtifactDigest)
	assert.Equal(t, fixture.restore.manifestDigest, resolved.ManifestDigest)
	require.Len(t, runner.calls, 4)
	assert.Contains(t, runner.calls[0].args, "repos/verity-org/verity/actions/workflows/.github%2Fworkflows%2Fpublish.yaml/runs")
	assert.Contains(t, runner.calls[0].args, "--paginate")
	assert.Contains(t, runner.calls[0].args, "--slurp")
	assert.Contains(t, runner.calls[3].args, "--source-digest")
	assert.Contains(t, runner.calls[3].args, restoreSourceSHA)
}

func TestResolvePrevious_rejects_untrusted_selector_inputs_before_GitHub_calls(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*ResolvePreviousOptions)
	}{
		{name: "repository", mutate: func(options *ResolvePreviousOptions) { options.Repository = "attacker/repository" }},
		{name: "workflow", mutate: func(options *ResolvePreviousOptions) { options.Workflow = ".github/workflows/other.yaml" }},
		{name: "branch", mutate: func(options *ResolvePreviousOptions) { options.Branch = "release" }},
		{name: "artifact", mutate: func(options *ResolvePreviousOptions) { options.ArtifactName = "legacy-pages" }},
		{name: "before run", mutate: func(options *ResolvePreviousOptions) { options.BeforeRunID = 0 }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Given one caller-controlled selector outside the fixed production trust root.
			fixture := newResolveFixture(t)
			runner := fixture.runner(t)
			options := fixture.options(runner)
			test.mutate(options)

			// When resolution is requested.
			_, err := ResolvePrevious(context.Background(), options)

			// Then the request is rejected before any GitHub command executes.
			require.ErrorIs(t, err, errUntrustedPreviousResolver)
			assert.Empty(t, runner.calls)
		})
	}
}

func TestResolvePrevious_rejects_untrusted_latest_run_without_fallback(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*resolveFixture)
		wantErr error
	}{
		{name: "missing", mutate: func(f *resolveFixture) { f.runs = nil }, wantErr: errPreviousRunNotFound},
		{name: "ambiguous", mutate: func(f *resolveFixture) { f.runs = append(f.runs, f.runs[0]) }, wantErr: errAmbiguousPreviousRun},
		{name: "wrong workflow path", mutate: func(f *resolveFixture) { f.runs[0].Path = ".github/workflows/other.yaml" }, wantErr: errWrongPagesWorkflow},
		{name: "wrong workflow name", mutate: func(f *resolveFixture) { f.runs[0].Name = "Other" }, wantErr: errWrongPagesWorkflow},
		{name: "wrong branch", mutate: func(f *resolveFixture) { f.runs[0].HeadBranch = "release" }, wantErr: errWrongPagesRun},
		{name: "unsuccessful", mutate: func(f *resolveFixture) { f.runs[0].Conclusion = "failure" }, wantErr: errWrongPagesRun},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Given a latest earlier candidate with one trust-contract violation.
			fixture := newResolveFixture(t)
			test.mutate(fixture)

			// When resolution is attempted.
			_, err := ResolvePrevious(context.Background(), fixture.options(fixture.runner(t)))

			// Then resolution fails closed instead of falling back to the older valid run.
			require.ErrorIs(t, err, test.wantErr)
		})
	}
}

func TestResolvePrevious_requires_one_live_digest_bound_artifact(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*resolveFixture)
		wantErr error
	}{
		{name: "missing", mutate: func(f *resolveFixture) { f.artifacts = nil }, wantErr: errPagesArtifactNotFound},
		{name: "ambiguous", mutate: func(f *resolveFixture) { f.artifacts = append(f.artifacts, f.artifacts[0]) }, wantErr: errAmbiguousPagesArtifact},
		{name: "expired", mutate: func(f *resolveFixture) { f.artifacts[0].Expired = true }, wantErr: errExpiredPagesArtifact},
		{name: "legacy digest", mutate: func(f *resolveFixture) { f.artifacts[0].Digest = "" }, wantErr: errLegacyPagesArtifact},
		{name: "wrong run", mutate: func(f *resolveFixture) { f.artifacts[0].Workflow.ID = 59 }, wantErr: errWrongPagesRun},
		{name: "wrong source", mutate: func(f *resolveFixture) { f.artifacts[0].Workflow.HeadSHA = strings.Repeat("f", 40) }, wantErr: errWrongPagesRun},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Given the selected run and a malformed or ambiguous matching artifact set.
			fixture := newResolveFixture(t)
			test.mutate(fixture)

			// When resolution is attempted.
			_, err := ResolvePrevious(context.Background(), fixture.options(fixture.runner(t)))

			// Then the candidate is rejected before any bytes are trusted.
			require.ErrorIs(t, err, test.wantErr)
		})
	}
}

func TestResolvePrevious_rejects_unattested_malformed_and_noncanonical_archives(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*resolveFixture, *testing.T)
		wantErr error
	}{
		{name: "unattested", mutate: func(f *resolveFixture, _ *testing.T) { f.attestationErr = assert.AnError }, wantErr: assert.AnError},
		{name: "malformed", mutate: func(f *resolveFixture, t *testing.T) {
			f.restore.tarBytes = []byte("not a site archive")
			f.restore.rebuildZip(t)
			f.artifacts[0].Digest = f.restore.zipDigest
		}, wantErr: sitepublication.ErrInvalidArchive},
		{name: "noncanonical", mutate: func(f *resolveFixture, t *testing.T) {
			f.restore.tarBytes = append(f.restore.tarBytes, 0)
			f.restore.rebuildZip(t)
			f.artifacts[0].Digest = f.restore.zipDigest
		}, wantErr: sitepublication.ErrInvalidArchive},
		{name: "manifest identity mismatch", mutate: func(f *resolveFixture, _ *testing.T) {
			f.runs[0].HeadSHA = strings.Repeat("f", 40)
			f.artifacts[0].Workflow.HeadSHA = f.runs[0].HeadSHA
		}, wantErr: errWrongPagesRun},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Given a selected artifact that cannot satisfy the provenance/archive contract.
			fixture := newResolveFixture(t)
			test.mutate(fixture, t)

			// When resolution is attempted.
			_, err := ResolvePrevious(context.Background(), fixture.options(fixture.runner(t)))

			// Then no previous-publication identity is emitted.
			require.ErrorIs(t, err, test.wantErr)
		})
	}
}

type resolveFixture struct {
	restore        *restoreFixture
	runs           []priorWorkflowRun
	artifacts      []priorArtifact
	attestationErr error
}

func newResolveFixture(t *testing.T) *resolveFixture {
	t.Helper()
	restore := newRestoreFixture(t)
	run := priorWorkflowRun{
		ID: restore.runID, RunAttempt: restore.attempt, Name: sitepublication.PublishWorkflowName,
		Path: sitepublication.PublishWorkflow, HeadBranch: "main", HeadSHA: restoreSourceSHA, Conclusion: "success",
	}
	older := priorWorkflowRun{
		ID: 59, RunAttempt: 1, Name: sitepublication.PublishWorkflowName,
		Path: sitepublication.PublishWorkflow, HeadBranch: "main", HeadSHA: strings.Repeat("b", 40), Conclusion: "success",
	}
	artifact := priorArtifact{ID: 6, Name: sitepublication.PagesArtifactName, Digest: restore.zipDigest}
	artifact.Workflow.ID = restore.runID
	artifact.Workflow.HeadBranch = "main"
	artifact.Workflow.HeadSHA = restoreSourceSHA
	return &resolveFixture{restore: restore, runs: []priorWorkflowRun{run, older}, artifacts: []priorArtifact{artifact}}
}

func (fixture *resolveFixture) options(runner *fakeCommandRunner) *ResolvePreviousOptions {
	return &ResolvePreviousOptions{
		Repository: "verity-org/verity", Workflow: sitepublication.PublishWorkflow, Branch: "main",
		ArtifactName: sitepublication.PagesArtifactName, BeforeRunID: fixture.restore.runID + 1, runner: runner,
	}
}

func (fixture *resolveFixture) runner(t *testing.T) *fakeCommandRunner {
	t.Helper()
	runner := &fakeCommandRunner{}
	runner.run = func(request command) (commandResult, error) {
		joined := strings.Join(request.args, " ")
		switch {
		case strings.Contains(joined, "/actions/workflows/") && strings.Contains(joined, "/runs"):
			body, err := json.Marshal([]map[string]any{{"workflow_runs": fixture.runs}})
			require.NoError(t, err)
			return commandResult{stdout: body}, nil
		case strings.Contains(joined, fmt.Sprintf("actions/runs/%d/artifacts", fixture.restore.runID)):
			body, err := json.Marshal([]map[string]any{{"artifacts": fixture.artifacts}})
			require.NoError(t, err)
			return commandResult{stdout: body}, nil
		case strings.Contains(joined, "actions/artifacts/6/zip"):
			_, err := request.stdout.Write(fixture.restore.zipBytes)
			return commandResult{}, err
		case strings.HasPrefix(joined, "attestation verify "):
			return commandResult{}, fixture.attestationErr
		default:
			return commandResult{}, assert.AnError
		}
	}
	return runner
}
