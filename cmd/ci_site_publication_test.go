package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/urfave/cli/v3"

	"github.com/verity-org/verity/internal/ci/publication"
	"github.com/verity-org/verity/internal/ci/signerlock"
	"github.com/verity-org/verity/internal/ci/sitepublication"
)

func TestCISitePublicationCommand_is_registered_without_replacing_existing_commands(t *testing.T) {
	// Given the shared verity ci command tree.
	registered := make(map[string]bool, len(CICommand.Commands))

	// When subcommands are enumerated.
	for _, command := range CICommand.Commands {
		registered[command.Name] = true
	}

	// Then the new typed surface and established commands coexist.
	for _, name := range []string{"site-publication", "publication", "apk-repository", "plan", "vulncheck"} {
		assert.True(t, registered[name], "missing ci command %q", name)
	}
}

func TestCISitePublicationPlan_emits_one_canonical_machine_record_on_repeated_runs(t *testing.T) {
	// Given canonical current/candidate manifests, exact components, signer lock, and real ancestry.
	repository := t.TempDir()
	runGit(t, repository, "init", "-q")
	runGit(t, repository, "config", "user.email", "ci@example.invalid")
	runGit(t, repository, "config", "user.name", "CI Test")
	previousSHA := commitFile(t, repository, "previous")
	candidateSHA := commitFile(t, repository, "candidate")
	previous := publicationFixture(publication.ModeBootstrap, previousSHA)
	previous.RunID, previous.RunAttempt, previous.BatchID = 41, 2, "41-2"
	previousDigest, err := publication.DigestManifest(&previous)
	require.NoError(t, err)
	candidate := publicationFixture(publication.ModeDelta, candidateSHA)
	candidate.PreviousManifestDigest = previousDigest
	root := t.TempDir()
	previousPath := writeManifest(t, root, "previous.json", &previous)
	candidatePath := writeManifest(t, root, "candidate.json", &candidate)
	components, err := publication.MarshalComponentsCanonical(candidate.Components)
	require.NoError(t, err)
	componentsPath := filepath.Join(root, "components.json")
	require.NoError(t, os.WriteFile(componentsPath, components, 0o600))
	lockPath := filepath.Join(root, "signer.json")
	lockBytes, err := json.Marshal(signerlock.Lock{
		Image: signerlock.SignerImageRepository, Digest: string(candidate.SignerDigest),
		Workflow:  signerlock.TrustedWorkflowIdentity,
		SourceSHA: "cccccccccccccccccccccccccccccccccccccccc", Runnable: true,
	})
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(lockPath, lockBytes, 0o600))
	arguments := make([]string, 0, 31)
	arguments = append(
		arguments,
		"verity", "ci", "site-publication", "plan",
		"--source-sha", candidateSHA, "--run-id", "42", "--run-attempt", "3", "--batch-id", "42-3",
		"--mode", "delta", "--components", componentsPath, "--signer-lock", lockPath,
		"--signer-source-sha", "cccccccccccccccccccccccccccccccccccccccc",
		"--publication-sha", candidateSHA, "--previous-manifest", previousPath, "--repo-dir", repository,
		candidatePath,
	)

	// When Build Site requests the same typed plan through fresh roots in one process.
	for range 2 {
		var stdout bytes.Buffer
		command := &cli.Command{Commands: []*cli.Command{CICommand}, Writer: &stdout}
		err = command.Run(context.Background(), arguments)

		// Then every stdout is one canonical plan record with no prose or secret material.
		require.NoError(t, err)
		parsed, parseErr := sitepublication.ParsePlanCanonical(stdout.Bytes())
		require.NoError(t, parseErr)
		assert.Equal(t, candidate.RunID, parsed.RunID)
		assert.Equal(t, candidate.SignerDigest, parsed.SignerDigest)
		assert.NotContains(t, stdout.String(), "validated")
		assert.NotContains(t, stdout.String(), "PRIVATE KEY")
	}

	// When Build Site requests a file-backed plan and GitHub metadata in one run.
	recordPath := filepath.Join(root, "plan-record.json")
	githubOutput := filepath.Join(root, "github-output")
	t.Setenv("GITHUB_OUTPUT", githubOutput)
	var stdout bytes.Buffer
	command := &cli.Command{Commands: []*cli.Command{CICommand}, Writer: &stdout}
	err = command.Run(context.Background(), append(arguments, "--record-output", recordPath))

	// Then the record replaces stdout and the outputs come from the validated plan.
	require.NoError(t, err)
	assert.Empty(t, stdout.String())
	record, err := os.ReadFile(recordPath)
	require.NoError(t, err)
	parsed, err := sitepublication.ParsePlanCanonical(record)
	require.NoError(t, err)
	assert.NotEmpty(t, parsed.PlanDigest)
	outputs, err := os.ReadFile(githubOutput)
	require.NoError(t, err)
	assert.Equal(t, "plan_digest="+string(parsed.PlanDigest)+"\nmanifest_digest="+string(parsed.ManifestDigest)+"\nmode="+string(parsed.Mode)+"\n", string(outputs))
}
