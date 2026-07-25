//go:build e2e

package repositoryops_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCLI_E2E_readOnlyRepositoryOperations(t *testing.T) {
	binary := buildVerityBinary(t)

	t.Run("parses issue form into workflow output", func(t *testing.T) {
		// Given
		directory := t.TempDir()
		bodyPath := filepath.Join(directory, "issue.md")
		outputPath := filepath.Join(directory, "github-output")
		body := "### Image name\nrclone\n\n### Image repository\nrclone/rclone\n\n### Image tag\nv1.70.3\n\n### Image registry\n\n"
		require.NoError(t, os.WriteFile(bodyPath, []byte(body), 0o600))

		// When
		output, err := runCLI(t, cliInvocation{binary: binary, arguments: []string{
			"ci", "repository-ops", "parse-image-issue", "--body-file", bodyPath, "--github-output", outputPath,
		}})

		// Then
		require.NoError(t, err, output)
		workflowOutput, readErr := os.ReadFile(outputPath)
		require.NoError(t, readErr)
		assert.Equal(t, "name=rclone\nrepository=rclone/rclone\ntag=v1.70.3\nregistry=docker.io\n", string(workflowOutput))
	})

	t.Run("rejects prompt injection without invoking cosign canary", func(t *testing.T) {
		// Given
		directory := t.TempDir()
		binDir := filepath.Join(directory, "bin")
		require.NoError(t, os.MkdirAll(binDir, 0o755))
		marker := filepath.Join(directory, "cosign-invoked")
		writeExecutable(t, filepath.Join(binDir, "cosign"), "#!/bin/sh\nprintf invoked > \"$COSIGN_MARKER\"\n")
		bodyPath := filepath.Join(directory, "issue.md")
		body := "### Image name\nrclone\n\n### Image repository\nrclone/rclone$(cosign sign attacker)\n\n### Image tag\nv1.70.3\n"
		require.NoError(t, os.WriteFile(bodyPath, []byte(body), 0o600))

		// When
		output, err := runCLI(t, cliInvocation{
			binary:    binary,
			arguments: []string{"ci", "repository-ops", "parse-image-issue", "--body-file", bodyPath},
			environment: []string{
				"PATH=" + binDir + string(os.PathListSeparator) + os.Getenv("PATH"),
				"COSIGN_MARKER=" + marker,
			},
		})

		// Then
		require.Error(t, err)
		assert.Contains(t, output, "invalid image repository")
		assert.NoFileExists(t, marker)
	})

	t.Run("runs Trivy with literal arguments and records counts", func(t *testing.T) {
		// Given
		directory := t.TempDir()
		binDir := filepath.Join(directory, "bin")
		require.NoError(t, os.MkdirAll(binDir, 0o755))
		writeExecutable(t, filepath.Join(binDir, "trivy"), fakeTrivyScript)
		reportPath := filepath.Join(directory, "before.json")
		environmentPath := filepath.Join(directory, "github-env")
		transcriptPath := filepath.Join(directory, "trivy.args")
		report := `{"Results":[{"Type":"gobinary","Vulnerabilities":[]},{"Type":"alpine","Vulnerabilities":[]}]}`

		// When
		output, err := runCLI(t, cliInvocation{
			binary: binary,
			arguments: []string{
				"ci", "repository-ops", "scan-before", "--source", "docker.io/library/alpine:3.22",
				"--report", reportPath, "--github-env", environmentPath,
			},
			environment: []string{
				"PATH=" + binDir + string(os.PathListSeparator) + os.Getenv("PATH"),
				"TRIVY_REPORT=" + report,
				"TRIVY_TRANSCRIPT=" + transcriptPath,
			},
		})

		// Then
		require.NoError(t, err, output)
		workflowEnvironment, readErr := os.ReadFile(environmentPath)
		require.NoError(t, readErr)
		assert.Equal(t, "before_total=0\nbefore_go=0\nbefore_non_go=0\nskip_patch=true\n", string(workflowEnvironment))
		transcript, readErr := os.ReadFile(transcriptPath)
		require.NoError(t, readErr)
		assert.True(t, strings.HasSuffix(strings.TrimSpace(string(transcript)), "docker.io/library/alpine:3.22"))
	})

	t.Run("rejects duplicate Trivy Results that hide a vulnerability", func(t *testing.T) {
		// Given
		directory := t.TempDir()
		binDir := filepath.Join(directory, "bin")
		require.NoError(t, os.MkdirAll(binDir, 0o755))
		writeExecutable(t, filepath.Join(binDir, "trivy"), fakeTrivyScript)
		report := `{"Results":[{"Type":"alpine","Vulnerabilities":[{"VulnerabilityID":"CVE-1"}]}],"Results":[]}`

		// When
		output, err := runCLI(t, cliInvocation{
			binary: binary,
			arguments: []string{
				"ci", "repository-ops", "scan-before", "--source", "docker.io/library/alpine:3.22",
				"--report", filepath.Join(directory, "before.json"), "--github-env", filepath.Join(directory, "github-env"),
			},
			environment: []string{
				"PATH=" + binDir + string(os.PathListSeparator) + os.Getenv("PATH"),
				"TRIVY_REPORT=" + report,
				"TRIVY_TRANSCRIPT=" + filepath.Join(directory, "trivy.args"),
			},
		})

		// Then
		require.Error(t, err)
		assert.Contains(t, output, "duplicate object key")
	})

	t.Run("blocks a fixable non-Go vulnerability", func(t *testing.T) {
		// Given
		directory := t.TempDir()
		binDir := filepath.Join(directory, "bin")
		require.NoError(t, os.MkdirAll(binDir, 0o755))
		writeExecutable(t, filepath.Join(binDir, "trivy"), fakeTrivyScript)
		report := `{"Results":[{"Type":"alpine","Vulnerabilities":[{"VulnerabilityID":"CVE-1"}]}]}`

		// When
		output, err := runCLI(t, cliInvocation{
			binary: binary,
			arguments: []string{
				"ci", "repository-ops", "verify-patched", "--image", "ghcr.io/verity/alpine:3.22",
				"--image-label", "alpine 3.22", "--report", filepath.Join(directory, "after.json"),
				"--before-total", "1", "--before-go", "0", "--before-non-go", "1",
			},
			environment: []string{
				"PATH=" + binDir + string(os.PathListSeparator) + os.Getenv("PATH"),
				"TRIVY_REPORT=" + report,
				"TRIVY_TRANSCRIPT=" + filepath.Join(directory, "trivy.args"),
			},
		})

		// Then
		require.Error(t, err)
		assert.Contains(t, output, "fixable non-Go vulnerabilities remain")
	})

	t.Run("runs the native package recipe through melange", func(t *testing.T) {
		// Given
		directory := t.TempDir()
		binDir := filepath.Join(directory, "bin")
		require.NoError(t, os.MkdirAll(binDir, 0o755))
		transcriptPath := filepath.Join(directory, "melange.args")
		writeExecutable(t, filepath.Join(binDir, "melange"), "#!/bin/sh\nprintf '%s\\n' \"$@\" > \"$MELANGE_TRANSCRIPT\"\n")

		// When
		output, err := runCLI(t, cliInvocation{
			binary: binary,
			arguments: []string{
				"ci", "repository-ops", "test-package", "--kind", "rclone", "--arch", "x86_64", "--repo-root", directory,
			},
			environment: []string{
				"PATH=" + binDir + string(os.PathListSeparator) + os.Getenv("PATH"),
				"MELANGE_TRANSCRIPT=" + transcriptPath,
			},
		})

		// Then
		require.NoError(t, err, output)
		transcript, readErr := os.ReadFile(transcriptPath)
		require.NoError(t, readErr)
		assert.Contains(t, string(transcript), "melange-work/specs/rclone.yaml/build.yaml\nrclone\n")
	})
}
