package cmd

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/urfave/cli/v3"

	"github.com/verity-org/verity/internal/ci/publication"
)

func TestCICommand_preserves_existing_command_surface(t *testing.T) {
	// Given the public CI command tree before publication commands are added.
	wanted := []string{"apk-repository", "plan", "vulncheck"}

	// When the existing subcommands are enumerated.
	registered := make(map[string]bool, len(CICommand.Commands))
	for _, command := range CICommand.Commands {
		registered[command.Name] = true
	}

	// Then every established CI command remains registered.
	for _, name := range wanted {
		assert.True(t, registered[name], "missing existing ci command %q", name)
	}
}

func TestCIPublicationCommand_validates_exact_manifest_in_dirty_worktree_and_rejects_stale_base(t *testing.T) {
	// Given a real three-commit history plus an unrelated dirty file.
	repository := t.TempDir()
	runGit(t, repository, "init", "-q")
	runGit(t, repository, "config", "user.email", "ci@example.invalid")
	runGit(t, repository, "config", "user.name", "CI Test")
	previousSHA := commitFile(t, repository, "one")
	candidateSHA := commitFile(t, repository, "two")
	publicationSHA := commitFile(t, repository, "three")
	require.NoError(t, os.WriteFile(filepath.Join(repository, "dirty"), []byte("uncommitted"), 0o600))

	previous := publicationFixture(publication.ModeBootstrap, previousSHA)
	previous.RunID = 41
	previous.RunAttempt = 2
	previous.BatchID = "41-2"
	previousDigest, err := publication.DigestManifest(&previous)
	require.NoError(t, err)
	candidate := publicationFixture(publication.ModeDelta, candidateSHA)
	candidate.PreviousManifestDigest = previousDigest

	root := t.TempDir()
	previousPath := writeManifest(t, root, "previous.json", &previous)
	candidatePath := writeManifest(t, root, "candidate.json", &candidate)
	componentsPath := filepath.Join(root, "components.json")
	components, err := publication.MarshalComponentsCanonical(candidate.Components)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(componentsPath, components, 0o600))

	args := []string{
		"verity", "ci", "publication", "validate",
		"--source-sha", string(candidate.SourceSHA),
		"--run-id", "42", "--run-attempt", "3", "--batch-id", "42-3",
		"--mode", "delta", "--components", componentsPath,
		"--signer-digest", string(candidate.SignerDigest),
		"--publication-sha", publicationSHA,
		"--previous-manifest", previousPath,
		"--repo-dir", repository,
		candidatePath,
	}

	// When the canonical candidate is validated through the public CLI.
	command := &cli.Command{Commands: []*cli.Command{CICommand}}
	err = command.Run(context.Background(), args)

	// Then the exact candidate is accepted despite unrelated worktree dirt.
	require.NoError(t, err)

	// When one stale-base byte sequence replaces the expected prior digest.
	stale := candidate
	stale.PreviousManifestDigest = "sha256:ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff"
	stalePath := writeManifest(t, root, "stale.json", &stale)
	args[len(args)-1] = stalePath
	command = &cli.Command{Commands: []*cli.Command{CICommand}}
	err = command.Run(context.Background(), args)

	// Then the CLI reports a CAS failure.
	require.ErrorIs(t, err, publication.ErrCASMismatch)
}

func runGit(t *testing.T, directory string, arguments ...string) string {
	t.Helper()
	command := exec.CommandContext(t.Context(), "git", arguments...)
	command.Dir = directory
	output, err := command.CombinedOutput()
	require.NoError(t, err, strings.TrimSpace(string(output)))
	return strings.TrimSpace(string(output))
}

func commitFile(t *testing.T, repository, name string) string {
	t.Helper()
	require.NoError(t, os.WriteFile(filepath.Join(repository, name), []byte(name), 0o600))
	runGit(t, repository, "add", name)
	runGit(t, repository, "commit", "-q", "-m", name)
	return runGit(t, repository, "rev-parse", "HEAD")
}

func writeManifest(t *testing.T, directory, name string, manifest *publication.Manifest) string {
	t.Helper()
	data, err := publication.MarshalCanonical(manifest)
	require.NoError(t, err)
	path := filepath.Join(directory, name)
	require.NoError(t, os.WriteFile(path, data, 0o600))
	return path
}

func publicationFixture(mode publication.Mode, sourceSHA string) publication.Manifest {
	return publication.Manifest{
		SchemaVersion: publication.SchemaVersion,
		SourceSHA:     publication.SourceSHA(sourceSHA),
		RunID:         42,
		RunAttempt:    3,
		BatchID:       "42-3",
		Mode:          mode,
		Components: []publication.Component{
			{
				Name: "integer", Kind: publication.ComponentKindAPK, ArtifactName: "integer-publication-42-3",
				Architecture:   publication.ArchitectureX8664,
				ArtifactDigest: "sha256:1111111111111111111111111111111111111111111111111111111111111111",
				Workflow:       ".github/workflows/integer-orchestrator.yaml",
				Event:          publication.EventWorkflowCall, Result: publication.ResultSuccess,
			},
		},
		SignerDigest: "sha256:2222222222222222222222222222222222222222222222222222222222222222",
		APKOperations: []publication.APKOperation{
			{
				Action: publication.APKUpsert, Architecture: publication.ArchitectureX8664,
				PackageName: "demo", ArtifactName: "integer-publication-42-3",
				ArtifactDigest: "sha256:1111111111111111111111111111111111111111111111111111111111111111",
			},
		},
	}
}
