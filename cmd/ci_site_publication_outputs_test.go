package cmd

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/urfave/cli/v3"

	"github.com/verity-org/verity/internal/ci/publication"
	"github.com/verity-org/verity/internal/ci/sitepublication"
)

func TestCISitePublicationRecordOutput_replacesStdoutWithAtomicFile(t *testing.T) {
	// Given a pre-existing machine record and a command with a record destination.
	root := t.TempDir()
	recordPath := filepath.Join(root, "records", "plan.json")
	require.NoError(t, os.MkdirAll(filepath.Dir(recordPath), 0o755))
	require.NoError(t, os.WriteFile(recordPath, []byte(`{"old":true}`), 0o600))
	var stdout bytes.Buffer
	command := &cli.Command{
		Flags:  []cli.Flag{&cli.StringFlag{Name: "record-output"}},
		Writer: &stdout,
		Action: func(_ context.Context, command *cli.Command) error {
			return writeMachineRecord(command, []byte(`{"new":true}`))
		},
	}

	// When the command writes its machine record.
	err := command.Run(context.Background(), []string{"test", "--record-output", recordPath})

	// Then stdout is empty, the destination is replaced, and no temp record remains.
	require.NoError(t, err)
	assert.Empty(t, stdout.String())
	record, err := os.ReadFile(recordPath)
	require.NoError(t, err)
	assert.JSONEq(t, `{"new":true}`, string(record))
	temporary, err := filepath.Glob(filepath.Join(filepath.Dir(recordPath), ".plan.json.tmp-*"))
	require.NoError(t, err)
	assert.Empty(t, temporary)
}

func TestCISitePublicationGitHubOutput_appendsValidatedTypedFields(t *testing.T) {
	// Given typed records produced by the site-publication domain operations.
	root := t.TempDir()
	outputPath := filepath.Join(root, "github-output")
	plan := sitepublication.PublicationPlan{
		PlanDigest:     "sha256:plan",
		ManifestDigest: "sha256:manifest",
		Mode:           publication.ModeDelta,
	}
	signer := sitepublication.SignerResult{OutputDigest: "sha256:output"}
	final := sitepublication.FinalPlan{
		ArtifactDigest: "sha256:artifact", ManifestDigest: "sha256:final-manifest", DeployEligible: true,
	}

	// When each command exposes its typed metadata to GitHub Actions.
	require.NoError(t, appendCISitePublicationPlanOutputs(outputPath, &plan))
	require.NoError(t, appendCISitePublicationSignerOutputs(outputPath, signer))
	require.NoError(t, appendCISitePublicationFinalizeOutputs(outputPath, &final))

	// Then every field is emitted from its typed record in GitHub's line format.
	data, err := os.ReadFile(outputPath)
	require.NoError(t, err)
	assert.Equal(t, "plan_digest=sha256:plan\nmanifest_digest=sha256:manifest\nmode=delta\noutput_digest=sha256:output\nartifact_digest=sha256:artifact\nmanifest_digest=sha256:final-manifest\ndeploy_eligible=true\n", string(data))
}

func TestCISitePublicationGitHubOutput_defaultsToEnvironment(t *testing.T) {
	// Given GitHub's output file is available only through its environment contract.
	outputPath := filepath.Join(t.TempDir(), "github-output")
	t.Setenv("GITHUB_OUTPUT", outputPath)

	// When the command does not provide an explicit output flag.
	got := sitePublicationGitHubOutputPath(&cli.Command{})

	// Then the runtime environment supplies the destination.
	assert.Equal(t, outputPath, got)
}

func TestCISitePublicationCommands_exposeMachineRecordAndGitHubOutputFlags(t *testing.T) {
	// Given the five machine-record-producing site-publication commands.
	recordCommands := []*cli.Command{
		ciSitePublicationPlanCommand,
		ciSitePublicationSignerPlanCommand,
		ciSitePublicationSignerExecuteCommand,
		ciSitePublicationAssembleCommand,
		ciSitePublicationFinalizeCommand,
	}
	githubCommands := []*cli.Command{
		ciSitePublicationPlanCommand,
		ciSitePublicationSignerExecuteCommand,
		ciSitePublicationFinalizeCommand,
	}

	// When their declared flags are inspected.
	for _, command := range recordCommands {
		// Then every machine record command accepts an atomic record destination.
		assert.True(t, sitePublicationCommandHasFlag(command, "record-output"), command.Name)
	}
	for _, command := range githubCommands {
		// Then every metadata-producing command accepts a GitHub output destination.
		assert.True(t, sitePublicationCommandHasFlag(command, "github-output"), command.Name)
	}
}

func sitePublicationCommandHasFlag(command *cli.Command, wanted string) bool {
	for _, flag := range command.Flags {
		if slices.Contains(flag.Names(), wanted) {
			return true
		}
	}
	return false
}
