package ci

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var errInjectedIntegerImageCommand = errors.New("injected command failure")

func TestPublishIntegerImage_runsStagingTrivyBeforeEveryFinalPromotion(t *testing.T) {
	// Given: a generated apko config, two final tags, and a recording command runner.
	runner := &recordingIntegerImageRunner{}
	options := integerImagePublishFixture(t, runner)

	// When: the exact image publication command runs.
	digest, err := PublishIntegerImage(t.Context(), &options)

	// Then: apko stages once, Trivy fails closed before either crane copy, and
	// the clean primary tag digest is returned.
	require.NoError(t, err)
	assert.Equal(t, "sha256:"+strings.Repeat("a", 64), digest)
	assert.Equal(t, []string{"apko:publish", "trivy:image", "crane:copy", "crane:copy", "crane:digest"}, runner.callIDs())
	trivy := runner.calls[1]
	assert.True(t, slices.Contains(trivy.Args, "--exit-code"))
	assert.True(t, slices.Contains(trivy.Args, "1"))
	assert.True(t, slices.Contains(trivy.Args, "UNKNOWN,LOW,MEDIUM,HIGH,CRITICAL"))
	assert.Contains(t, string(runner.apkoConfig), "org.opencontainers.image.revision: aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	assert.Contains(t, string(runner.apkoConfig), "org.verity.publication.id: integer-publication-42-3")
	assert.Contains(t, runner.calls[0].Args[len(runner.calls[0].Args)-1], "integer-publication-42-3")
}

func TestPublishIntegerImage_usesUniqueArchitectureKeyBasenames_whenMelangeEnabled(t *testing.T) {
	// Given: a multi-architecture image publication using staged Melange packages.
	runner := &recordingIntegerImageRunner{}
	options := integerImagePublishFixture(t, runner)
	options.Melange = true

	// When: the exact image publication command stages the image.
	_, err := PublishIntegerImage(t.Context(), &options)

	// Then: each architecture selects the unique basename embedded in its APKINDEX signature.
	require.NoError(t, err)
	require.NotEmpty(t, runner.calls)
	assert.Contains(t, runner.calls[0].Args, filepath.Join(options.Workspace, "packages", "repo", "melange-x86_64.rsa.pub"))
	assert.Contains(t, runner.calls[0].Args, filepath.Join(options.Workspace, "packages", "repo", "melange-aarch64.rsa.pub"))
}

func TestPublishIntegerImage_stopsBeforePromotion_whenStagingTrivyFails(t *testing.T) {
	// Given: a runner whose exact staged-image Trivy command fails.
	runner := &recordingIntegerImageRunner{failID: "trivy:image"}
	options := integerImagePublishFixture(t, runner)

	// When: publication reaches the unchanged staged-image gate.
	_, err := PublishIntegerImage(t.Context(), &options)

	// Then: no final tag is written after the zero-CVE failure.
	require.Error(t, err)
	assert.Equal(t, []string{"apko:publish", "trivy:image"}, runner.callIDs())
}

func TestPublishIntegerImage_rejectsMalformedTags_beforeExternalCommands(t *testing.T) {
	// Given: a publication request containing an empty final tag.
	runner := &recordingIntegerImageRunner{}
	options := integerImagePublishFixture(t, runner)
	options.Tags = "1,,latest"

	// When: the request is parsed at the Go boundary.
	_, err := PublishIntegerImage(t.Context(), &options)

	// Then: malformed input fails without staging or promotion side effects.
	require.ErrorIs(t, err, ErrIntegerBatchPlan)
	assert.Empty(t, runner.calls)
}

func integerImagePublishFixture(t *testing.T, runner *recordingIntegerImageRunner) IntegerImagePublishOptions {
	t.Helper()
	root := t.TempDir()
	configPath := filepath.Join(root, "image.apko.yaml")
	require.NoError(t, os.WriteFile(configPath, []byte("contents:\n  packages: [wolfi-base]\n"), 0o600))
	return IntegerImagePublishOptions{
		Image: "alpha", Version: "1", Type: "default", Registry: "ghcr.io/verity-org", Tags: "1,latest",
		ConfigPath: configPath, Workspace: root, SourceSHA: strings.Repeat("a", 40),
		RunID: 42, RunAttempt: 3, PublicationID: "integer-publication-42-3", CreatedAt: time.Date(2026, 7, 24, 12, 0, 0, 0, time.UTC), Runner: runner,
	}
}

type recordingIntegerImageRunner struct {
	calls      []IntegerImageCommand
	failID     string
	apkoConfig []byte
}

func (runner *recordingIntegerImageRunner) Run(ctx context.Context, command IntegerImageCommand) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	runner.calls = append(runner.calls, command)
	if command.Name == "apko" {
		data, err := os.ReadFile(command.Args[len(command.Args)-2])
		if err != nil {
			return nil, err
		}
		runner.apkoConfig = data
	}
	if runner.failID == command.ID() {
		return nil, errInjectedIntegerImageCommand
	}
	if command.ID() == "crane:digest" {
		return []byte("sha256:" + strings.Repeat("a", 64) + "\n"), nil
	}
	return nil, nil
}

func (runner *recordingIntegerImageRunner) callIDs() []string {
	ids := make([]string, 0, len(runner.calls))
	for _, call := range runner.calls {
		ids = append(ids, call.ID())
	}
	return ids
}
