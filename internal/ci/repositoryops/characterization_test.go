package repositoryops_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCharacterization_parseImageIssueForm_defaultsRegistryAndEmitsFields(t *testing.T) {
	// Given
	repoRoot := repositoryRoot(t)
	outputPath := filepath.Join(t.TempDir(), "github-output")
	body := strings.Join([]string{
		"### Image name", "rclone", "",
		"### Image repository", "rclone/rclone", "",
		"### Image tag", "v1.70.3", "",
		"### Image registry", "", "",
	}, "\n")
	command := exec.CommandContext(t.Context(), "bash", filepath.Join(repoRoot, ".github", "scripts", "parse-image-issue-form.sh"))
	command.Env = append(os.Environ(), "ISSUE_BODY="+body, "GITHUB_OUTPUT="+outputPath)

	// When
	output, err := command.CombinedOutput()

	// Then
	require.NoError(t, err, string(output))
	emitted, err := os.ReadFile(outputPath)
	require.NoError(t, err)
	assert.Equal(t, "name=rclone\nrepository=rclone/rclone\ntag=v1.70.3\nregistry=docker.io\n", string(emitted))
}

func TestCharacterization_nativePackageScripts_buildExactMelangeInvocation(t *testing.T) {
	tests := []struct {
		name        string
		script      string
		buildFile   string
		packageName string
	}{
		{name: "rclone", script: "test-rclone-package.sh", buildFile: "melange-work/specs/rclone.yaml/build.yaml", packageName: "rclone"},
		{name: "sealed secrets", script: "test-sealed-secrets-package.sh", buildFile: "melange-work/specs/sealed-secrets-0.yaml/build.yaml", packageName: "sealed-secrets-0"},
		{name: "step ca", script: "test-step-ca-package.sh", buildFile: "melange-work/specs/step-ca.yaml/build.yaml", packageName: "step-ca"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Given
			repoRoot := repositoryRoot(t)
			workspace := t.TempDir()
			binDir := filepath.Join(workspace, "bin")
			require.NoError(t, os.MkdirAll(binDir, 0o755))
			transcript := filepath.Join(workspace, "timeout.args")
			writeExecutable(t, filepath.Join(binDir, "timeout"), "#!/bin/sh\nprintf '%s\\n' \"$@\" > \"$TRANSCRIPT\"\n")
			command := exec.CommandContext(t.Context(), "bash", filepath.Join(repoRoot, ".github", "scripts", test.script), "x86_64")
			command.Env = append(
				os.Environ(),
				"GITHUB_WORKSPACE="+workspace,
				"PATH="+binDir+string(os.PathListSeparator)+os.Getenv("PATH"),
				"TRANSCRIPT="+transcript,
			)

			// When
			output, err := command.CombinedOutput()

			// Then
			require.NoError(t, err, string(output))
			data, err := os.ReadFile(transcript)
			require.NoError(t, err)
			got := strings.Split(strings.TrimSpace(string(data)), "\n")
			want := []string{
				"--signal=TERM", "--kill-after=1m", "30m", "melange", "test",
				"--arch", "x86_64",
				"--repository-append", filepath.Join(workspace, "packages", "repo"),
				"--repository-append", "https://packages.wolfi.dev/os",
				"--keyring-append", filepath.Join(workspace, "melange-work", "melange.rsa.pub"),
				"--keyring-append", "https://packages.wolfi.dev/os/wolfi-signing.rsa.pub",
				"--runner", "docker",
				"--pipeline-dirs", "melange-work/pipelines",
				test.buildFile, test.packageName,
			}
			assert.Equal(t, want, got)
		})
	}
}

func TestCharacterization_patchImage_handlesDigestPinnedSourceAndCopiesWhenClean(t *testing.T) {
	// Given
	repoRoot := repositoryRoot(t)
	workspace := t.TempDir()
	binDir := filepath.Join(workspace, "bin")
	require.NoError(t, os.MkdirAll(binDir, 0o755))
	patchArgs := filepath.Join(workspace, "patch.args")
	craneArgs := filepath.Join(workspace, "crane.args")
	craneState := filepath.Join(workspace, "crane.state")
	githubOutput := filepath.Join(workspace, "github-output")
	writeExecutable(t, filepath.Join(workspace, "verity"), "#!/bin/sh\nprintf '%s\\n' \"$@\" > \"$PATCH_TRANSCRIPT\"\necho 'no package updates found'\nexit 1\n")
	writeExecutable(t, filepath.Join(binDir, "crane"), `#!/bin/sh
printf '%s\n' "$*" >> "$CRANE_ARGS"
if [ "$1" = digest ]; then
  count=0
  [ -f "$CRANE_STATE" ] && count=$(cat "$CRANE_STATE")
  count=$((count + 1))
  printf '%s' "$count" > "$CRANE_STATE"
  [ "$count" -eq 1 ] && exit 1
  echo 'sha256:characterized'
fi
`)
	command := exec.CommandContext(t.Context(), "bash", filepath.Join(repoRoot, ".github", "scripts", "patch-image.sh"))
	command.Dir = workspace
	command.Env = append(
		os.Environ(),
		"PLATFORM=linux/amd64",
		"SOURCE=localhost:5000/foo:v1.2.3@sha256:deadbeef",
		"IMAGE_NAME=org/name",
		"STAGING_REGISTRY=registry.example/cache",
		"GITHUB_OUTPUT="+githubOutput,
		"PATH="+binDir+string(os.PathListSeparator)+os.Getenv("PATH"),
		"PATCH_TRANSCRIPT="+patchArgs,
		"CRANE_ARGS="+craneArgs,
		"CRANE_STATE="+craneState,
	)

	// When
	output, err := command.CombinedOutput()

	// Then
	require.NoError(t, err, string(output))
	patchData, err := os.ReadFile(patchArgs)
	require.NoError(t, err)
	assert.Contains(t, string(patchData), "registry.example/cache:org-name-v1.2.3-amd64")
	assert.Contains(t, string(patchData), "reports/localhost_5000_foo_v1.2.3@sha256_deadbeef.json")
	craneData, err := os.ReadFile(craneArgs)
	require.NoError(t, err)
	assert.Contains(t, string(craneData), "copy --platform linux/amd64 localhost:5000/foo:v1.2.3@sha256:deadbeef registry.example/cache:org-name-v1.2.3-amd64")
	emitted, err := os.ReadFile(githubOutput)
	require.NoError(t, err)
	assert.Contains(t, string(emitted), "exit-code=0\n")
	assert.Contains(t, string(emitted), "staging-digest=sha256:characterized\n")
}

func repositoryRoot(t *testing.T) string {
	t.Helper()
	workingDirectory, err := os.Getwd()
	require.NoError(t, err)
	return filepath.Join(workingDirectory, "..", "..", "..")
}

func writeExecutable(t *testing.T, path, content string) {
	t.Helper()
	require.NoError(t, os.WriteFile(path, []byte(content), 0o755))
}
