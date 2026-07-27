//go:build e2e

package repositoryops_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCLI_E2E_repositoryMutations(t *testing.T) {
	binary := buildVerityBinary(t)

	t.Run("records a bounded sync mutation transcript", func(t *testing.T) {
		// Given
		directory := t.TempDir()
		binDir := filepath.Join(directory, "bin")
		require.NoError(t, os.MkdirAll(binDir, 0o755))
		writeExecutable(t, filepath.Join(binDir, "git"), fakeGitScript)
		writeExecutable(t, filepath.Join(binDir, "gh"), fakeGitHubScript)
		gitTranscript := filepath.Join(directory, "git.transcript")
		githubTranscript := filepath.Join(directory, "gh.transcript")

		// When
		output, err := runCLI(t, cliInvocation{
			binary: binary,
			arguments: []string{
				"ci", "repository-ops", "sync-pr", "--repo-root", directory,
				"--base", "main", "--branch", "automation/integer-package-streams", "--max-changed-images", "1",
			},
			environment: []string{
				"PATH=" + binDir + string(os.PathListSeparator) + os.Getenv("PATH"),
				"FAKE_GIT_MODE=sync", "GIT_TRANSCRIPT=" + gitTranscript, "GH_TRANSCRIPT=" + githubTranscript,
				"GITHUB_TOKEN=least-privilege-token", "GH_TOKEN=must-not-be-used", "GITHUB_REPOSITORY=verity-org/verity",
			},
		})

		// Then
		require.NoError(t, err, output)
		gitCommands, readErr := os.ReadFile(gitTranscript)
		require.NoError(t, readErr)
		assert.Contains(t, string(gitCommands), "restore -- images/z.yaml")
		assert.Contains(t, string(gitCommands), "add -- images/a.yaml")
		assert.Contains(t, string(gitCommands), "push --force-with-lease=refs/heads/automation/integer-package-streams:")
		githubCommands, readErr := os.ReadFile(githubTranscript)
		require.NoError(t, readErr)
		assert.Contains(t, string(githubCommands), "token=least-privilege-token command=pr create")
	})

	t.Run("adds a validated image and constructs a PR command", func(t *testing.T) {
		// Given
		directory := t.TempDir()
		binDir := filepath.Join(directory, "bin")
		require.NoError(t, os.MkdirAll(binDir, 0o755))
		writeExecutable(t, filepath.Join(binDir, "git"), fakeGitScript)
		writeExecutable(t, filepath.Join(binDir, "gh"), fakeGitHubScript)
		configPath := filepath.Join(directory, "copa-config.yaml")
		require.NoError(t, os.WriteFile(configPath, []byte("images: []\n"), 0o600))
		newFakeGitMetadata(t, directory)
		gitTranscript := filepath.Join(directory, "git.transcript")
		githubTranscript := filepath.Join(directory, "gh.transcript")

		// When
		output, err := runCLI(t, cliInvocation{
			binary: binary,
			arguments: []string{
				"ci", "repository-ops", "add-standalone-image", "--name", "rclone", "--repository", "rclone/rclone",
				"--tag", "v1.70.3", "--registry", "docker.io", "--issue-number", "123",
				"--config", configPath, "--repo-root", directory, "--base", "main",
			},
			environment: []string{
				"PATH=" + binDir + string(os.PathListSeparator) + os.Getenv("PATH"),
				"FAKE_GIT_MODE=add", "GIT_TRANSCRIPT=" + gitTranscript, "GH_TRANSCRIPT=" + githubTranscript,
				"GITHUB_TOKEN=least-privilege-token", "GH_TOKEN=must-not-be-used", "GITHUB_REPOSITORY=verity-org/verity",
			},
		})

		// Then
		require.NoError(t, err, output)
		config, readErr := os.ReadFile(configPath)
		require.NoError(t, readErr)
		assert.Contains(t, string(config), "image: docker.io/rclone/rclone")
		gitCommands, readErr := os.ReadFile(gitTranscript)
		require.NoError(t, readErr)
		assert.Contains(t, string(gitCommands), "checkout -b add-image/rclone")
		githubCommands, readErr := os.ReadFile(githubTranscript)
		require.NoError(t, readErr)
		assert.Contains(t, string(githubCommands), "token=least-privilege-token command=pr create")
	})

	t.Run("rejects a pull request URL from another repository", func(t *testing.T) {
		// Given
		directory := t.TempDir()
		binDir := filepath.Join(directory, "bin")
		require.NoError(t, os.MkdirAll(binDir, 0o755))
		writeExecutable(t, filepath.Join(binDir, "git"), fakeGitScript)
		writeExecutable(t, filepath.Join(binDir, "gh"), fakeGitHubScript)

		// When
		output, err := runCLI(t, cliInvocation{
			binary: binary,
			arguments: []string{
				"ci", "repository-ops", "sync-pr", "--repo-root", directory,
				"--base", "main", "--branch", "automation/integer-package-streams", "--max-changed-images", "1",
			},
			environment: []string{
				"PATH=" + binDir + string(os.PathListSeparator) + os.Getenv("PATH"),
				"FAKE_GIT_MODE=sync", "GIT_TRANSCRIPT=" + filepath.Join(directory, "git.transcript"),
				"GH_TRANSCRIPT=" + filepath.Join(directory, "gh.transcript"),
				"FAKE_GH_URL=https://github.com/attacker/other/pull/42",
				"GITHUB_TOKEN=least-privilege-token", "GITHUB_REPOSITORY=verity-org/verity",
			},
		})

		// Then
		require.Error(t, err)
		assert.Contains(t, output, "malformed output")
	})

	t.Run("restores worktree bytes after a later git failure", func(t *testing.T) {
		// Given
		directory := t.TempDir()
		binDir := filepath.Join(directory, "bin")
		require.NoError(t, os.MkdirAll(binDir, 0o755))
		writeExecutable(t, filepath.Join(binDir, "git"), fakeGitScript)
		writeExecutable(t, filepath.Join(binDir, "gh"), fakeGitHubScript)
		configPath := filepath.Join(directory, "copa-config.yaml")
		dirtyPath := filepath.Join(directory, "dirty.txt")
		createdPath := filepath.Join(directory, "created.txt")
		configOriginal := []byte("# exact bytes\nimages: []\n")
		dirtyOriginal := []byte("dirty pre-state\n")
		require.NoError(t, os.WriteFile(configPath, configOriginal, 0o640))
		require.NoError(t, os.WriteFile(dirtyPath, dirtyOriginal, 0o600))
		newFakeGitMetadata(t, directory)

		// When
		output, err := runCLI(t, cliInvocation{
			binary: binary,
			arguments: []string{
				"ci", "repository-ops", "add-standalone-image", "--name", "rclone", "--repository", "rclone/rclone",
				"--tag", "v1.70.3", "--registry", "docker.io", "--issue-number", "123",
				"--config", configPath, "--repo-root", directory, "--base", "main",
			},
			environment: []string{
				"PATH=" + binDir + string(os.PathListSeparator) + os.Getenv("PATH"),
				"FAKE_GIT_MODE=add", "FAKE_GIT_FAIL_MODE=commit", "FAKE_GIT_MUTATE_PATH=" + dirtyPath,
				"FAKE_GIT_CREATED_PATH=" + createdPath, "GIT_TRANSCRIPT=" + filepath.Join(directory, "git.transcript"),
				"GH_TRANSCRIPT=" + filepath.Join(directory, "gh.transcript"),
				"GITHUB_TOKEN=least-privilege-token", "GITHUB_REPOSITORY=verity-org/verity",
			},
		})

		// Then
		require.Error(t, err, output)
		config, readErr := os.ReadFile(configPath)
		require.NoError(t, readErr)
		assert.Equal(t, configOriginal, config)
		dirty, readErr := os.ReadFile(dirtyPath)
		require.NoError(t, readErr)
		assert.Equal(t, dirtyOriginal, dirty)
		assert.NoFileExists(t, createdPath)
	})
}
