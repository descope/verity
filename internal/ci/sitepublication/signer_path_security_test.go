package sitepublication

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildSignerPlan_rejects_symlinked_and_hard_linked_host_paths(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*SignerRequest, *testing.T)
	}{
		{name: "workspace symlink", mutate: func(request *SignerRequest, t *testing.T) {
			alias := filepath.Join(filepath.Dir(request.WorkspaceDir), "workspace-alias")
			require.NoError(t, os.Symlink(request.WorkspaceDir, alias))
			request.WorkspaceDir = alias
		}},
		{name: "manifest symlink", mutate: func(request *SignerRequest, t *testing.T) {
			manifest := filepath.Join(request.WorkspaceDir, request.ManifestPath)
			target := manifest + ".real"
			require.NoError(t, os.Rename(manifest, target))
			require.NoError(t, os.Symlink(filepath.Base(target), manifest))
		}},
		{name: "output symlink", mutate: func(request *SignerRequest, t *testing.T) {
			output := filepath.Join(request.WorkspaceDir, request.OutputAPKPath)
			target := output + ".real"
			require.NoError(t, os.Rename(output, target))
			require.NoError(t, os.Symlink(filepath.Base(target), output))
		}},
		{name: "key directory symlink", mutate: func(request *SignerRequest, t *testing.T) {
			target := request.KeyDirectory + ".real"
			require.NoError(t, os.Mkdir(target, 0o700))
			require.NoError(t, os.Symlink(filepath.Base(target), request.KeyDirectory))
		}},
		{name: "manifest public-key hard link", mutate: func(request *SignerRequest, t *testing.T) {
			publicKey := filepath.Join(request.WorkspaceDir, request.PublicKeyPath)
			manifest := filepath.Join(request.WorkspaceDir, request.ManifestPath)
			require.NoError(t, os.Remove(publicKey))
			require.NoError(t, os.Link(manifest, publicKey))
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			publicationPlan, _ := validPlanAndManifest(t)
			request := signerRequest(t, &publicationPlan, t.TempDir())
			test.mutate(request, t)

			_, err := BuildSignerPlan(request)

			require.ErrorIs(t, err, ErrInvalidSignerPlan)
		})
	}
}

func TestExecuteSigner_rejects_host_path_swap_before_final_signer_step(t *testing.T) {
	tests := []struct {
		name string
		swap func(*SignerRequest, *testing.T)
	}{
		{name: "manifest", swap: func(request *SignerRequest, t *testing.T) {
			path := filepath.Join(request.WorkspaceDir, request.ManifestPath)
			target := path + ".attacker"
			require.NoError(t, os.Rename(path, target))
			require.NoError(t, os.Symlink(filepath.Base(target), path))
		}},
		{name: "output directory", swap: func(request *SignerRequest, t *testing.T) {
			path := filepath.Join(request.WorkspaceDir, request.OutputAPKPath)
			target := path + ".attacker"
			require.NoError(t, os.Rename(path, target))
			require.NoError(t, os.Mkdir(path, 0o755))
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			publicationPlan, _ := validPlanAndManifest(t)
			request := signerRequest(t, &publicationPlan, t.TempDir())
			plan, err := BuildSignerPlan(request)
			require.NoError(t, err)
			runner := &recordingExecutionRunner{run: func(index int, _ ExecutionCommand) (ExecutionResult, error) {
				if index == 1 {
					test.swap(request, t)
				}
				return ExecutionResult{}, nil
			}}

			_, err = ExecuteSigner(context.Background(), &plan, []byte("secret"), runner)

			require.ErrorIs(t, err, ErrInvalidSignerPlan)
			assert.NoDirExists(t, plan.Cleanup.KeyDirectory)
			assert.Len(t, runner.calls, 2)
		})
	}
}

func TestSignerMountArgumentParser_rejectsInvalidWritableField(t *testing.T) {
	_, err := parseDockerMountArgument("--mount=type=bind,src=/tmp/input,dst=/output,rw")
	require.Error(t, err)

	mount, err := parseDockerMountArgument("--mount=type=bind,src=/tmp/output,dst=/output")
	require.NoError(t, err)
	assert.False(t, mount.readonly)
}

func TestSignerDockerCommandConstruction_matchesRealParser(t *testing.T) {
	if _, err := os.Stat(trustedDockerBinary); err != nil {
		t.Skip("Docker is unavailable")
	}
	invalid := exec.CommandContext(context.Background(), trustedDockerBinary, "run", "--rm", "--mount=type=bind,src=/tmp,dst=/output,rw", "scratch", "true")
	invalidOutput, err := invalid.CombinedOutput()
	require.Error(t, err)
	assert.Contains(t, string(invalidOutput), "invalid field 'rw'")

	valid := exec.CommandContext(context.Background(), trustedDockerBinary, "run", "--rm", "--mount=type=bind,src=/tmp,dst=/output", "scratch", "true")
	validOutput, err := valid.CombinedOutput()
	require.Error(t, err)
	assert.NotContains(t, string(validOutput), "invalid field 'rw'")
}

func TestSignerMelangeCLI_exposesCanonicalCommands_whenAvailable(t *testing.T) {
	if _, err := os.Stat(trustedMelangeBinary); err != nil {
		t.Skip("Melange is unavailable on the host")
	}
	for _, args := range [][]string{{"--help"}, {"sign", "--help"}, {"index", "--help"}} {
		output, err := exec.CommandContext(context.Background(), trustedMelangeBinary, args...).CombinedOutput()
		require.NoErrorf(t, err, "%s: %s", strings.Join(args, " "), output)
	}
}
