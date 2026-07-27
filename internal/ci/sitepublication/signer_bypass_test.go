package sitepublication

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildSignerPlan_uses_immutable_signer_and_data_only_mounts(t *testing.T) {
	// Given a valid pinned signer request with an attacker-controlled workspace.
	publicationPlan, _ := validPlanAndManifest(t)
	request := signerRequest(t, &publicationPlan, t.TempDir())
	plan, err := BuildSignerPlan(request)
	require.NoError(t, err)
	require.Len(t, plan.Steps, 3)

	// When the protected sign command is inspected.
	arguments := plan.Steps[2].Command.Args
	joined := strings.Join(arguments, " ")

	// Then only the image-internal signer can execute and every data bind is constrained.
	assert.Contains(t, arguments, "--entrypoint=/usr/bin/apk-repository-signer")
	assert.Contains(t, arguments, "--workdir=/")
	assert.Contains(t, arguments, "--tmpfs="+signerTmpfsArgument)
	assert.NotContains(t, joined, "/work/verity")
	assert.NotContains(t, joined, ",dst=/work")
	assert.Contains(t, arguments, "--mount=type=bind,src="+filepath.Join(request.WorkspaceDir, "publication.json")+",dst=/inputs/publication.json,readonly")
	assert.Contains(t, arguments, "--mount=type=bind,src="+filepath.Join(request.WorkspaceDir, "packages")+",dst=/inputs/packages,readonly")
	assert.Contains(t, arguments, "--mount=type=bind,src="+filepath.Join(request.WorkspaceDir, "signed-apk")+",dst=/output")
	assert.NotContains(t, joined, request.KeyDirectory)
	assert.Equal(t, 1, countWritableBindMounts(arguments))
	assert.NotContains(t, arguments, "--signing-key")
	assert.Contains(t, joined, "ci site-publication sign")
}

func TestBuildSignerPlan_rejects_overlapping_signer_data_paths(t *testing.T) {
	// Given an output path nested below the read-only package input.
	publicationPlan, _ := validPlanAndManifest(t)
	request := signerRequest(t, &publicationPlan, t.TempDir())
	request.OutputAPKPath = "packages/signed-apk"

	// When the signer plan is built.
	_, err := BuildSignerPlan(request)

	// Then the aliased input/output mount is rejected before key handling.
	require.ErrorIs(t, err, ErrInvalidSignerPlan)
}

func TestBuildSignerPlan_uses_fixed_runtime_executables(t *testing.T) {
	// Given a valid pinned signer request.
	publicationPlan, _ := validPlanAndManifest(t)
	plan, err := BuildSignerPlan(signerRequest(t, &publicationPlan, t.TempDir()))
	require.NoError(t, err)

	// When its executable identities are inspected.
	pull := plan.Steps[0].Command.Name
	attest := plan.Steps[1].Command.Name
	sign := plan.Steps[2].Command.Name

	// Then no PATH alias or wrapper can replace Docker or GitHub CLI.
	assert.Equal(t, "/usr/bin/docker", pull)
	assert.Equal(t, "/usr/bin/gh", attest)
	assert.Equal(t, "/usr/bin/docker", sign)
}

func TestValidateSignerPlan_rejects_alias_wrapper_and_last_flag_wins_bypasses(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*SignerPlan)
	}{
		{name: "typed runtime wrapper", mutate: func(plan *SignerPlan) {
			plan.Execution.Runtime = ContainerRuntime("/tmp/docker-wrapper")
		}},
		{name: "repository option injection", mutate: func(plan *SignerPlan) {
			plan.Execution.Repository = "--repo/attacker"
		}},
		{name: "runtime wrapper", mutate: func(plan *SignerPlan) {
			plan.Steps[0].Command.Name = "/tmp/docker-wrapper"
			plan.Steps[2].Command.Name = "/tmp/docker-wrapper"
		}},
		{name: "network alias", mutate: addSignerRunArgument("--net=host")},
		{name: "network override", mutate: addSignerRunArgument("--network=host")},
		{name: "writable root override", mutate: addSignerRunArgument("--read-only=false")},
		{name: "root user alias", mutate: addSignerRunArgument("-u=0:0")},
		{name: "root user override", mutate: addSignerRunArgument("--user=0:0")},
		{name: "privileged override", mutate: addSignerRunArgument("--privileged=true")},
		{name: "capability addition", mutate: addSignerRunArgument("--cap-add=SYS_ADMIN")},
		{name: "security option override", mutate: addSignerRunArgument("--security-opt=seccomp=unconfined")},
		{name: "device addition", mutate: addSignerRunArgument("--device=/dev/kvm")},
		{name: "entrypoint override", mutate: addSignerRunArgument("--entrypoint=/bin/sh")},
		{name: "relative entrypoint", mutate: addSignerRunArgument("--entrypoint=melange")},
		{name: "workdir override", mutate: addSignerRunArgument("--workdir=/tmp")},
		{name: "key environment", mutate: addSignerRunArgument("--env=APK_REPOSITORY_PRIVATE_KEY")},
		{name: "key environment file", mutate: addSignerRunArgument("--env-file=/tmp/key.env")},
		{name: "docker host override", mutate: addSignerRunArgument("--host=tcp://attacker")},
		{name: "docker context override", mutate: addSignerRunArgument("--context=hostile")},
		{name: "docker config override", mutate: addSignerRunArgument("--config=/tmp/hostile")},
		{name: "key mount shadow", mutate: addSignerRunArgument("--mount=type=bind,src=/tmp,dst=/run/secrets,rw")},
		{name: "workspace shadow mount", mutate: addSignerRunArgument("--volume=/tmp:/work:rw")},
		{name: "mutable image with pinned decoy", mutate: func(plan *SignerPlan) {
			arguments := plan.Steps[2].Command.Args
			index := signerImageIndex(t, arguments, plan.ImageReference)
			arguments[index] = "ghcr.io/verity-org/apk-repository-signer:latest"
			arguments = append(arguments, plan.ImageReference)
			plan.Steps[2].Command.Args = arguments
		}},
		{name: "post-image command override", mutate: func(plan *SignerPlan) {
			arguments := plan.Steps[2].Command.Args
			index := signerImageIndex(t, arguments, plan.ImageReference)
			arguments[index+1] = "/bin/sh"
		}},
		{name: "extra post-image argument", mutate: func(plan *SignerPlan) {
			plan.Steps[2].Command.Args = append(plan.Steps[2].Command.Args, "--extra")
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Given one serialized-plan isolation bypass.
			publicationPlan, _ := validPlanAndManifest(t)
			plan, err := BuildSignerPlan(signerRequest(t, &publicationPlan, t.TempDir()))
			require.NoError(t, err)
			test.mutate(&plan)

			// When the hostile plan is checked before stdin key handling.
			err = ValidateSignerPlan(&plan)

			// Then aliases, wrappers, overrides, and post-image commands fail closed.
			require.ErrorIs(t, err, ErrInvalidSignerPlan)
		})
	}
}

func TestSignerSafeEnvironment_removes_runtime_and_key_overrides(t *testing.T) {
	// Given inherited credentials plus Docker/Podman routing overrides.
	environment := []string{
		"PATH=/tmp/wrappers:/usr/bin", "GH_TOKEN=token", "APK_REPOSITORY_PRIVATE_KEY=secret",
		"DOCKER_HOST=tcp://attacker", "DOCKER_CONTEXT=hostile", "DOCKER_CONFIG=/tmp/config",
		"DOCKER_TLS_VERIFY=1", "DOCKER_CERT_PATH=/tmp/certs", "HOME=/tmp/attacker",
		"CONTAINER_HOST=tcp://attacker", "CONTAINER_CONNECTION=hostile", "XDG_RUNTIME_DIR=/tmp/hostile",
	}

	// When the signer subprocess environment is constructed.
	filtered := signerSafeEnvironment(environment)

	// Then only explicitly required, non-routing values remain.
	assert.Equal(t, []string{"PATH=/usr/bin:/bin", "GH_TOKEN=token"}, filtered)
}

func addSignerRunArgument(argument string) func(*SignerPlan) {
	return func(plan *SignerPlan) {
		arguments := plan.Steps[2].Command.Args
		index := signerImageIndex(nil, arguments, plan.ImageReference)
		arguments = append(arguments, "")
		copy(arguments[index+1:], arguments[index:])
		arguments[index] = argument
		plan.Steps[2].Command.Args = arguments
	}
}

func signerImageIndex(t *testing.T, arguments []string, image string) int {
	if t != nil {
		t.Helper()
	}
	for index, argument := range arguments {
		if argument == image {
			return index
		}
	}
	if t != nil {
		require.FailNow(t, "signer image not found")
	}
	return len(arguments)
}

func TestValidateSignerPlan_rejects_cleanup_path_alias(t *testing.T) {
	// Given a key cleanup path that aliases the workspace lexically.
	publicationPlan, _ := validPlanAndManifest(t)
	plan, err := BuildSignerPlan(signerRequest(t, &publicationPlan, t.TempDir()))
	require.NoError(t, err)
	plan.Cleanup.KeyDirectory = filepath.Dir(plan.Cleanup.KeyDirectory) + string(filepath.Separator) + "workspace/../key"
	plan.Cleanup.KeyPath = filepath.Join(plan.Cleanup.KeyDirectory, "verity.rsa")

	// When the cleanup contract is validated.
	err = ValidateSignerPlan(&plan)

	// Then non-canonical directory aliases are rejected.
	require.ErrorIs(t, err, ErrInvalidSignerPlan)
}

func TestValidateSignerPlan_rejects_retargeted_workspace_even_with_regenerated_steps(t *testing.T) {
	// Given a hostile workspace retargeting with serialized steps regenerated consistently.
	publicationPlan, _ := validPlanAndManifest(t)
	plan, err := BuildSignerPlan(signerRequest(t, &publicationPlan, t.TempDir()))
	require.NoError(t, err)
	plan.Execution.WorkspaceDir = filepath.Join(t.TempDir(), "attacker-workspace")
	require.NoError(t, os.MkdirAll(filepath.Join(plan.Execution.WorkspaceDir, plan.Execution.PackagesPath), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(plan.Execution.WorkspaceDir, plan.Execution.PackagesPath, "demo.apk"), []byte("attacker"), 0o644))
	plan.Steps, err = buildSignerSteps(&plan)
	require.NoError(t, err)

	// When the retargeted plan is validated.
	err = ValidateSignerPlan(&plan)

	// Then an independently retargeted host workspace is rejected.
	require.ErrorIs(t, err, ErrInvalidSignerPlan)
}

func TestValidateSignerPlan_rejects_consistent_untrusted_repository_retargeting(t *testing.T) {
	// Given a hostile plan whose trusted execution fields and serialized steps agree.
	publicationPlan, _ := validPlanAndManifest(t)
	plan, err := BuildSignerPlan(signerRequest(t, &publicationPlan, t.TempDir()))
	require.NoError(t, err)
	plan.Execution.Repository = "attacker/example"
	plan.Steps, err = buildSignerSteps(&plan)
	require.NoError(t, err)

	// When the canonical plan is validated.
	err = ValidateSignerPlan(&plan)

	// Then a consistent retargeting still fails closed.
	require.ErrorIs(t, err, ErrInvalidSignerPlan)
}

func countWritableBindMounts(arguments []string) int {
	count := 0
	for _, argument := range arguments {
		if strings.HasPrefix(argument, "--mount=") && !strings.HasSuffix(argument, ",readonly") {
			count++
		}
	}
	return count
}
