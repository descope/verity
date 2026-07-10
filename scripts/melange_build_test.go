package scripts_test

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"syscall"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func melangeBuildScriptPath(t *testing.T) string {
	t.Helper()

	_, currentFile, _, ok := runtime.Caller(0)
	require.True(t, ok)

	return filepath.Join(filepath.Dir(currentFile), "..", ".github", "scripts", "melange-build.sh")
}

func writeTempFile(t *testing.T, path, content string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
}

func runMelangeBuild(t *testing.T, repoRoot string, env map[string]string) (string, error) {
	t.Helper()

	cmd := exec.CommandContext(context.Background(), "bash", melangeBuildScriptPath(t))
	cmd.Dir = repoRoot
	cmd.Env = os.Environ()
	for key, value := range env {
		cmd.Env = append(cmd.Env, key+"="+value)
	}

	out, err := cmd.CombinedOutput()
	return string(out), err
}

func installFakeMelange(t *testing.T, repoRoot string) (pathValue, logPath string) {
	t.Helper()

	binDir := filepath.Join(repoRoot, "bin")
	logPath = filepath.Join(repoRoot, "melange.log")
	writeTempFile(t, filepath.Join(binDir, "melange"), `#!/usr/bin/env bash
set -euo pipefail
echo "$*" >> "__MELANGE_LOG__"
case "$1" in
  keygen)
    touch "$2" "$2.pub"
    ;;
  build)
    mkdir -p packages/repo/x86_64
    ;;
  sign-index)
    ;;
esac
`)
	scriptPath := filepath.Join(binDir, "melange")
	script, readErr := os.ReadFile(scriptPath)
	require.NoError(t, readErr)
	require.NoError(t, os.WriteFile(scriptPath, []byte(strings.ReplaceAll(string(script), "__MELANGE_LOG__", logPath)), 0o755))
	require.NoError(t, os.Chmod(scriptPath, 0o755))
	pathValue = binDir + string(os.PathListSeparator) + os.Getenv("PATH")
	return pathValue, logPath
}

func TestMelangeBuildRejectsUnsafeBespokeValue(t *testing.T) {
	repoRoot := t.TempDir()

	output, err := runMelangeBuild(t, repoRoot, map[string]string{
		"BESPOKE_JSON": `["../evil.yaml"]`,
	})

	require.Error(t, err)
	assert.Contains(t, output, "BESPOKE contains unsafe characters")
}

func TestMelangeBuildFailsWhenBespokeFileMissing(t *testing.T) {
	repoRoot := t.TempDir()
	writeTempFile(t, filepath.Join(repoRoot, "packages", "upstream.lock.json"), `{"provenance":{"recipe_baseline_commit":"abc123"}}`)

	output, err := runMelangeBuild(t, repoRoot, map[string]string{
		"BESPOKE_JSON": `["custom.yaml"]`,
	})

	require.Error(t, err)
	assert.Contains(t, output, "Bespoke build file not found: packages/bespoke/custom.yaml")
}

func TestMelangeBuildBuildsBespokeWithoutBaselineCommit(t *testing.T) {
	repoRoot := t.TempDir()
	pathValue, logPath := installFakeMelange(t, repoRoot)
	writeTempFile(t, filepath.Join(repoRoot, "packages", "bespoke", "custom.yaml"), "package:\n  name: test\n")
	writeTempFile(t, filepath.Join(repoRoot, "packages", "upstream.lock.json"), `{"provenance":{}}`)

	output, err := runMelangeBuild(t, repoRoot, map[string]string{
		"BESPOKE_JSON": `["custom.yaml"]`,
		"PATH":         pathValue,
	})

	require.NoError(t, err, output)
	log, readErr := os.ReadFile(logPath)
	require.NoError(t, readErr)
	assert.Contains(t, string(log), "build melange-work/specs/custom.yaml/build.yaml")
}

func TestMelangeBuildStagesLockedBespokeRecipe(t *testing.T) {
	repoRoot := t.TempDir()
	pathValue, _ := installFakeMelange(t, repoRoot)
	recipe := "package:\n  name: cilium-1.19\n  version: \"1.19.5\"\n"
	writeTempFile(t, filepath.Join(repoRoot, "packages", "bespoke", "locked", "cilium-1.19.yaml"), recipe)
	writeTempFile(t, filepath.Join(repoRoot, "packages", "bespoke", "locked", "cilium-1.19", "loopback-location.patch"), "patch")
	writeTempFile(t, filepath.Join(repoRoot, "packages", "upstream.lock.json"), `{"packages":{"cilium-1.19":{"file":"cilium-1.19.yaml","sha256":"3db7a51598f6b5dd33a58d80ec9e2057ae84781648fa2b15f3d78f48e3b6a91c","assets":{"cilium-1.19/loopback-location.patch":"a4895eb44afc336fecbba6e520cd67e178dace0276655d102fceffa8e5f70570"}}},"pipeline_files":{}}`)

	output, err := runMelangeBuild(t, repoRoot, map[string]string{
		"UPSTREAM": "cilium-1.19",
		"PATH":     pathValue,
	})

	require.NoError(t, err, output)
	stagedRecipe, readErr := os.ReadFile(filepath.Join(repoRoot, "melange-work", "specs", "cilium-1.19", "build.yaml"))
	require.NoError(t, readErr)
	assert.Equal(t, recipe, string(stagedRecipe))
	stagedPatch, readErr := os.ReadFile(filepath.Join(repoRoot, "melange-work", "specs", "cilium-1.19", "loopback-location.patch"))
	require.NoError(t, readErr)
	assert.Equal(t, "patch", string(stagedPatch))
}

func TestMelangeBuildRejectsUnsafeUpstreamRecipePath(t *testing.T) {
	repoRoot := t.TempDir()
	writeTempFile(t, filepath.Join(repoRoot, "packages", "upstream.lock.json"), `{"packages":{"evil":{"file":"../evil.yaml","sha256":"abc"}}}`)

	output, err := runMelangeBuild(t, repoRoot, map[string]string{
		"UPSTREAM": "evil",
	})

	require.Error(t, err)
	assert.Contains(t, output, "recipe file must be a safe relative path without traversal")
}

func TestMelangeBuildRejectsTamperedLockedSidecar(t *testing.T) {
	repoRoot := t.TempDir()
	recipe := "package:\n  name: cilium-1.19\n  version: \"1.19.5\"\n"
	writeTempFile(t, filepath.Join(repoRoot, "packages", "bespoke", "locked", "cilium-1.19.yaml"), recipe)
	writeTempFile(t, filepath.Join(repoRoot, "packages", "bespoke", "locked", "cilium-1.19", "loopback-location.patch"), "tampered")
	writeTempFile(t, filepath.Join(repoRoot, "packages", "upstream.lock.json"), `{"packages":{"cilium-1.19":{"file":"cilium-1.19.yaml","sha256":"3db7a51598f6b5dd33a58d80ec9e2057ae84781648fa2b15f3d78f48e3b6a91c","assets":{"cilium-1.19/loopback-location.patch":"a4895eb44afc336fecbba6e520cd67e178dace0276655d102fceffa8e5f70570"}}},"pipeline_files":{}}`)

	output, err := runMelangeBuild(t, repoRoot, map[string]string{"UPSTREAM": "cilium-1.19"})

	require.Error(t, err)
	assert.Contains(t, output, "sha256 mismatch for recipe asset cilium-1.19/loopback-location.patch")
}

func TestMelangeBuildRejectsSymlinkedLockedSidecar(t *testing.T) {
	repoRoot := t.TempDir()
	recipe := "package:\n  name: cilium-1.19\n  version: \"1.19.5\"\n"
	writeTempFile(t, filepath.Join(repoRoot, "packages", "bespoke", "locked", "cilium-1.19.yaml"), recipe)
	require.NoError(t, os.MkdirAll(filepath.Join(repoRoot, "packages", "bespoke", "locked", "cilium-1.19"), 0o755))
	require.NoError(t, os.Symlink("/etc/hostname", filepath.Join(repoRoot, "packages", "bespoke", "locked", "cilium-1.19", "leak")))
	writeTempFile(t, filepath.Join(repoRoot, "packages", "upstream.lock.json"), `{"packages":{"cilium-1.19":{"file":"cilium-1.19.yaml","sha256":"3db7a51598f6b5dd33a58d80ec9e2057ae84781648fa2b15f3d78f48e3b6a91c","assets":{}}},"pipeline_files":{}}`)

	output, err := runMelangeBuild(t, repoRoot, map[string]string{"UPSTREAM": "cilium-1.19"})

	require.Error(t, err)
	assert.Contains(t, output, "Recipe cilium-1.19 sidecar contains non-regular file")
}

func TestMelangeBuildRejectsSymlinkedLockedSidecarRoot(t *testing.T) {
	repoRoot := t.TempDir()
	recipe := "package:\n  name: cilium-1.19\n  version: \"1.19.5\"\n"
	writeTempFile(t, filepath.Join(repoRoot, "packages", "bespoke", "locked", "cilium-1.19.yaml"), recipe)
	require.NoError(t, os.MkdirAll(filepath.Join(repoRoot, "external"), 0o755))
	writeTempFile(t, filepath.Join(repoRoot, "external", "leak"), "untracked")
	require.NoError(t, os.Symlink(filepath.Join(repoRoot, "external"), filepath.Join(repoRoot, "packages", "bespoke", "locked", "cilium-1.19")))
	writeTempFile(t, filepath.Join(repoRoot, "packages", "upstream.lock.json"), `{"packages":{"cilium-1.19":{"file":"cilium-1.19.yaml","sha256":"3db7a51598f6b5dd33a58d80ec9e2057ae84781648fa2b15f3d78f48e3b6a91c","assets":{}}},"pipeline_files":{}}`)

	output, err := runMelangeBuild(t, repoRoot, map[string]string{"UPSTREAM": "cilium-1.19"})

	require.Error(t, err)
	assert.Contains(t, output, "Recipe cilium-1.19 sidecar root must be a real directory")
}

func TestMelangeBuildRejectsSpecialFileSidecarRoot(t *testing.T) {
	repoRoot := t.TempDir()
	recipe := "package:\n  name: cilium-1.19\n  version: \"1.19.5\"\n"
	writeTempFile(t, filepath.Join(repoRoot, "packages", "bespoke", "locked", "cilium-1.19.yaml"), recipe)
	require.NoError(t, syscall.Mkfifo(filepath.Join(repoRoot, "packages", "bespoke", "locked", "cilium-1.19"), 0o600))
	writeTempFile(t, filepath.Join(repoRoot, "packages", "upstream.lock.json"), `{"packages":{"cilium-1.19":{"file":"cilium-1.19.yaml","sha256":"3db7a51598f6b5dd33a58d80ec9e2057ae84781648fa2b15f3d78f48e3b6a91c","assets":{}}},"pipeline_files":{}}`)

	output, err := runMelangeBuild(t, repoRoot, map[string]string{"UPSTREAM": "cilium-1.19"})

	require.Error(t, err)
	assert.Contains(t, output, "Recipe cilium-1.19 sidecar root must be a real directory")
}

func TestMelangeBuildRejectsSymlinkedLockedRoot(t *testing.T) {
	repoRoot := t.TempDir()
	recipe := "package:\n  name: cilium-1.19\n  version: \"1.19.5\"\n"
	externalRoot := filepath.Join(repoRoot, "external", "locked")
	writeTempFile(t, filepath.Join(externalRoot, "cilium-1.19.yaml"), recipe)
	require.NoError(t, os.MkdirAll(filepath.Join(repoRoot, "packages", "bespoke"), 0o755))
	require.NoError(t, os.Symlink(externalRoot, filepath.Join(repoRoot, "packages", "bespoke", "locked")))
	writeTempFile(t, filepath.Join(repoRoot, "packages", "upstream.lock.json"), `{"packages":{"cilium-1.19":{"file":"cilium-1.19.yaml","sha256":"3db7a51598f6b5dd33a58d80ec9e2057ae84781648fa2b15f3d78f48e3b6a91c","assets":{}}},"pipeline_files":{}}`)

	output, err := runMelangeBuild(t, repoRoot, map[string]string{"UPSTREAM": "cilium-1.19"})

	require.Error(t, err)
	assert.Contains(t, output, "recipe cilium-1.19 must resolve within repository path")
}

func TestLockedRecipeInventoryComplete(t *testing.T) {
	repoRoot := filepath.Clean(filepath.Join(filepath.Dir(melangeBuildScriptPath(t)), "..", ".."))
	lockData, err := os.ReadFile(filepath.Join(repoRoot, "packages", "upstream.lock.json"))
	require.NoError(t, err)

	var lock struct {
		Packages map[string]struct {
			File string `json:"file"`
		} `json:"packages"`
	}
	require.NoError(t, json.Unmarshal(lockData, &lock))
	for name, pkg := range lock.Packages {
		t.Run(name, func(t *testing.T) {
			_, statErr := os.Stat(filepath.Join(repoRoot, "packages", "bespoke", "locked", pkg.File))
			require.NoError(t, statErr)
		})
	}
}

func TestEveryMelangeUpstreamConsumerHasLockedRecipe(t *testing.T) {
	repoRoot := filepath.Clean(filepath.Join(filepath.Dir(melangeBuildScriptPath(t)), "..", ".."))
	lockData, err := os.ReadFile(filepath.Join(repoRoot, "packages", "upstream.lock.json"))
	require.NoError(t, err)

	var lock struct {
		Packages map[string]json.RawMessage `json:"packages"`
	}
	require.NoError(t, json.Unmarshal(lockData, &lock))

	images, err := filepath.Glob(filepath.Join(repoRoot, "images", "*.yaml"))
	require.NoError(t, err)
	for _, imagePath := range images {
		imageData, readErr := os.ReadFile(imagePath)
		require.NoError(t, readErr)
		var image struct {
			Types map[string]struct {
				Melange struct {
					Upstream string `yaml:"upstream"`
				} `yaml:"melange"`
			} `yaml:"types"`
			Versions map[string]struct {
				SkipTypes []string `yaml:"skip-types"`
			} `yaml:"versions"`
		}
		require.NoError(t, yaml.Unmarshal(imageData, &image))

		for typeName, imageType := range image.Types {
			upstream := imageType.Melange.Upstream
			if upstream == "" {
				continue
			}
			if !strings.Contains(upstream, "{{version}}") {
				require.Contains(t, lock.Packages, upstream, "%s type %s", filepath.Base(imagePath), typeName)
				continue
			}
			for version, config := range image.Versions {
				if containsString(config.SkipTypes, typeName) {
					continue
				}
				resolved := strings.ReplaceAll(upstream, "{{version}}", version)
				require.Contains(t, lock.Packages, resolved, "%s version %s type %s", filepath.Base(imagePath), version, typeName)
			}
		}
	}
}

func containsString(values []string, want string) bool {
	return slices.Contains(values, want)
}

func TestMelangeBuildRejectsUntrackedPipeline(t *testing.T) {
	repoRoot := t.TempDir()
	pathValue, _ := installFakeMelange(t, repoRoot)
	writeTempFile(t, filepath.Join(repoRoot, "packages", "bespoke", "custom.yaml"), "package:\n  name: test\n")
	writeTempFile(t, filepath.Join(repoRoot, "packages", "pipelines", "test", "custom.yaml"), "pipeline: []\n")
	writeTempFile(t, filepath.Join(repoRoot, "packages", "upstream.lock.json"), `{"pipeline_files":{}}`)

	output, err := runMelangeBuild(t, repoRoot, map[string]string{
		"BESPOKE_JSON": `["custom.yaml"]`,
		"PATH":         pathValue,
	})

	require.Error(t, err)
	assert.Contains(t, output, "Shared pipeline file set does not match lock manifest")
}
