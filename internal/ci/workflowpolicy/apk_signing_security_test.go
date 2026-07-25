package workflowpolicy

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRepositoryWorkflows_satisfy_APK_signing_security_contract(t *testing.T) {
	// Given the repository's complete live workflow set.
	workflowDirectory := filepath.Join("..", "..", "..", ".github", "workflows")

	// When every typed workflow policy is evaluated together.
	_, err := ValidateDirectory(workflowDirectory)

	// Then protected publication has no policy bypass or secret exposure.
	assert.NoError(t, err)
}

const (
	//nolint:gosec // G101 false positive: this fixture is a GitHub secret reference expression, not key material.
	productionSigningSecret = "${{ secrets.APK_REPOSITORY_PRIVATE_KEY }}"
	pinnedSignerImage       = "ghcr.io/verity-org/apk-repository-signer@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
)

func TestValidateAPKSigningBoundary_rejects_secret_exposure_and_mutable_tools(t *testing.T) {
	tests := []struct {
		name string
		run  string
		env  scalarMap
	}{
		{name: "ambient environment", run: secureSignerRun(), env: scalarMap{"APK_REPOSITORY_PRIVATE_KEY": productionSigningSecret}},
		{name: "docker environment", run: strings.Replace(secureSignerRun(), "docker run", "docker run --env APK_REPOSITORY_PRIVATE_KEY", 1)},
		{name: "docker argv", run: secureSignerRun() + " --private-key " + productionSigningSecret},
		{name: "xtrace", run: "set -x\n" + secureSignerRun()},
		{name: "runtime apk installation", run: "apk add melange\n" + secureSignerRun()},
		{name: "host resolved melange", run: "melange sign package.apk\n" + secureSignerRun()},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Given a production signing job that exposes the key or mutates its executable boundary.
			workflows := signerWorkflows(test.run, test.env)

			// When the signing boundary policy is evaluated.
			violations := validateAPKSigningBoundary(workflows)

			// Then the workflow fails closed.
			assert.Contains(t, violationRules(violations), RuleAPKSigningBoundary)
		})
	}
}

func TestValidateAPKSigningBoundary_requires_isolated_nonroot_container(t *testing.T) {
	tests := []struct {
		name string
		run  string
	}{
		{name: "network enabled", run: strings.Replace(secureSignerRun(), " --network=none", "", 1)},
		{name: "root user", run: strings.Replace(secureSignerRun(), "--user=65532:65532", "--user=root", 1)},
		{name: "capabilities retained", run: strings.Replace(secureSignerRun(), " --cap-drop=ALL", "", 1)},
		{name: "capability added", run: secureSignerRun() + " --cap-add=SYS_ADMIN"},
		{name: "container environment", run: secureSignerRun() + " --env-file signer.env"},
		{name: "writable whole workspace", run: secureSignerRun() + " --volume $GITHUB_WORKSPACE:/work"},
		{name: "whole workspace mount syntax", run: secureSignerRun() + " --mount type=bind,src=$GITHUB_WORKSPACE,dst=/work"},
		{name: "writable root filesystem", run: strings.Replace(secureSignerRun(), " --read-only", "", 1)},
		{name: "privilege escalation allowed", run: strings.Replace(secureSignerRun(), " --security-opt=no-new-privileges", "", 1)},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Given a signer container with one isolation guarantee removed.
			workflows := signerWorkflows(test.run, nil)

			// When the signing boundary policy is evaluated.
			violations := validateAPKSigningBoundary(workflows)

			// Then the signer is rejected.
			assert.Contains(t, violationRules(violations), RuleAPKSigningBoundary)
		})
	}
}

func TestValidateAPKSigningBoundary_accepts_stdin_only_isolated_signer(t *testing.T) {
	// Given an immutable signer whose key channel is stdin and whose container is isolated.
	workflows := signerWorkflows(secureSignerRun(), nil)

	// When the signing boundary policy is evaluated.
	violations := validateAPKSigningBoundary(workflows)

	// Then no signing-boundary violation is emitted.
	assert.NotContains(t, violationRules(violations), RuleAPKSigningBoundary)
}

func TestValidateDirectory_requires_canonical_APK_signing_key_state(t *testing.T) {
	// Given the production publication workflow omits its canonical key state.
	root := copyCoherentWorkflowFixture(t)
	replaceWorkflowText(t, root, workflowMutation{
		workflow: "build-site.yaml",
		old:      "          --signing-key-state ci/apk-signing-key-state.json\n",
	})

	// When workflow policy validates the publication boundary.
	_, err := ValidateDirectory(root)

	// Then signing without epoch and fingerprint state fails closed.
	require.Error(t, err)
	assert.ErrorContains(t, err, string(RuleAPKSigningBoundary))
}

func TestValidateAPKSigningBoundary_accepts_bounded_BuildSite_stdin_bridge(t *testing.T) {
	tests := []struct {
		name   string
		output string
	}{
		{name: "stdout redirect", output: "> signer-result.json"},
		{name: "output flag", output: "--output signer-result.json"},
		{name: "output equals flag", output: "--output=signer-result.json"},
		{name: "machine output flags", output: "--record-output signer-result.json --github-output \"$GITHUB_OUTPUT\""},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Given the exact Build Site env-to-stdin bridge and a bounded machine record destination.
			workflows := siteSignerWorkflows(boundedSignerRun(test.output), nil)

			// When the APK signing boundary is evaluated.
			violations := validateAPKSigningBoundary(workflows)

			// Then the bounded bridge is allowed without allowing direct key exposure.
			assert.NotContains(t, violationRules(violations), RuleAPKSigningBoundary)
		})
	}
}

func TestValidateAPKSigningBoundary_rejects_unbounded_BuildSite_stdin_bridges(t *testing.T) {
	tests := []struct {
		name string
		run  string
	}{
		{name: "key file", run: replaceSignerCommand(boundedSignerRun("> signer-result.json"), `printf '%s' "$signing_key" > signer-key`)},
		{name: "arbitrary pipe", run: replaceSignerCommand(boundedSignerRun("> signer-result.json"), `cat signer-key | ./verity ci site-publication signer-execute signer-plan.json > signer-result.json`)},
		{name: "wrong output file", run: strings.Replace(boundedSignerRun("> signer-result.json"), "> signer-result.json", "> /tmp/signer-result.json", 1)},
		{name: "missing environment cleanup", run: strings.Replace(boundedSignerRun("> signer-result.json"), "unset APK_REPOSITORY_PRIVATE_KEY\n", "", 1)},
		{name: "xtrace", run: "set -x\n" + boundedSignerRun("> signer-result.json")},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Given a signer-looking pipe that violates one bounded bridge invariant.
			workflows := siteSignerWorkflows(test.run, nil)

			// When both signing-boundary and Go-owned shell policies are evaluated.
			violations := append(validateAPKSigningBoundary(workflows), validateGoOwnedLogic(workflows)...)

			// Then the key path remains fail-closed.
			assert.Contains(t, violationRules(violations), RuleAPKSigningBoundary)
		})
	}
}

func TestValidateGoOwnedLogic_accepts_only_bounded_BuildSite_stdin_bridge(t *testing.T) {
	// Given the Build Site signer job with the exact approved env-to-stdin bridge.
	workflows := siteSignerWorkflows(boundedSignerRun("--record-output signer-result.json --github-output \"$GITHUB_OUTPUT\""), nil)

	// When Go-owned shell policy is evaluated.
	violations := validateGoOwnedLogic(workflows)

	// Then the bounded bridge is the only permitted pipeline.
	assert.NotContains(t, violationRules(violations), RuleGoOwnedLogic)
}

func TestValidateAPKSigningBoundary_rejects_job_environment_and_weak_job_container(t *testing.T) {
	// Given a signing job whose secret is ambient and whose job container is root/networked/capable.
	workflows := []workflowFile{{Name: "apk-repository.yaml", Workflow: workflow{Jobs: map[string]workflowJob{
		"sign": {
			Env: scalarMap{"APK_REPOSITORY_PRIVATE_KEY": productionSigningSecret},
			Container: containerSpec{
				Image:   pinnedSignerImage,
				Options: "--user root --cap-add SYS_ADMIN --network host",
				Volumes: stringList{"${{ github.workspace }}:/work"},
			},
			Steps: []workflowStep{{Name: "Sign APK repository", Run: "./verity ci apk-repository sign"}},
		},
	}}}}

	// When the signing boundary policy is evaluated.
	violations := validateAPKSigningBoundary(workflows)

	// Then ambient job secrets and weak job-container isolation fail closed.
	assert.Contains(t, violationRules(violations), RuleAPKSigningBoundary)
}

func TestValidateProtectedDispatchRefs_rejects_caller_controlled_checkout_ref(t *testing.T) {
	// Given a manually dispatched workflow that checks out an arbitrary caller input.
	workflows := []workflowFile{{Name: "apk-repository.yaml", Workflow: workflow{
		On: triggers{WorkflowDispatch: true},
		Jobs: map[string]workflowJob{"sign": {Steps: []workflowStep{{
			Uses: "actions/checkout@9c091bb21b7c1c1d1991bb908d89e4e9dddfe3e0",
			With: scalarMap{"ref": "${{ inputs.ref }}"},
		}}}},
	}}}

	// When protected dispatch refs are evaluated.
	violations := validateProtectedDispatchRefs(workflows)

	// Then caller-selected code is rejected before secret use.
	assert.Contains(t, violationRules(violations), RuleProtectedDispatch)
}

func TestValidateMelangeArtifactPaths_rejects_private_key_capable_uploads(t *testing.T) {
	tests := []struct {
		name string
		path string
	}{
		{name: "whole work directory", path: "melange-work/"},
		{name: "private rsa key", path: "melange-work/melange.rsa"},
		{name: "recursive key glob", path: "melange-work/**/*.key"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Given an artifact upload capable of containing an ephemeral Melange private key.
			workflows := []workflowFile{{Name: "integer-build-image-reusable.yaml", Workflow: workflow{Jobs: map[string]workflowJob{
				"melange": {Steps: []workflowStep{{
					Uses: "actions/upload-artifact@043fb46d1a93c77aae656e7c1c64a875d1fc6a0a",
					With: scalarMap{"path": test.path},
				}}},
			}}}}

			// When artifact paths are evaluated.
			violations := validateMelangeArtifactPaths(workflows)

			// Then private-key-capable uploads are forbidden.
			assert.Contains(t, violationRules(violations), RulePrivateKeyArtifact)
		})
	}
}

func TestValidateSignerProvenance_requires_immutable_digest_and_attestation(t *testing.T) {
	tests := []struct {
		name   string
		digest string
		verify string
	}{
		{name: "caller controlled digest", digest: "${{ inputs.signer_digest }}", verify: "true"},
		{name: "attestation disabled", digest: "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", verify: "false"},
		{name: "digest omitted", digest: "", verify: "true"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Given a signer activation with mutable or unattested identity.
			workflows := []workflowFile{{Name: "apk-repository.yaml", Workflow: workflow{Jobs: map[string]workflowJob{
				"sign": {Steps: []workflowStep{{
					Uses: "./.github/actions/setup-verity",
					With: scalarMap{"artifact-digest": test.digest, "verify-attestation": test.verify},
				}}},
			}}}}

			// When signer provenance is evaluated.
			violations := validateSignerProvenance(workflows)

			// Then mutable or unattested signer activation is rejected.
			assert.Contains(t, violationRules(violations), RuleSignerProvenance)
		})
	}
}

func TestValidatePinnedReferences_rejects_unpinned_signer_image(t *testing.T) {
	// Given a signer command that executes a mutable image tag.
	workflows := signerWorkflows(strings.Replace(secureSignerRun(), pinnedSignerImage, "ghcr.io/verity-org/apk-repository-signer:latest", 1), nil)

	// When executable image references are evaluated.
	violations := validatePinnedReferences(workflows)

	// Then the mutable signer image is rejected.
	assert.Contains(t, violationRules(violations), RulePinnedReference)
}

func signerWorkflows(run string, env scalarMap) []workflowFile {
	return []workflowFile{{Name: "apk-repository.yaml", Workflow: workflow{Jobs: map[string]workflowJob{
		"sign": {Steps: []workflowStep{{Name: "Sign APK repository", Run: run, Env: env}}},
	}}}}
}

func siteSignerWorkflows(run string, env scalarMap) []workflowFile {
	if env == nil {
		env = scalarMap{apkPrivateKeySecretName: productionSigningSecret}
	}
	return []workflowFile{{Name: buildSiteWorkflowName, Workflow: workflow{Jobs: map[string]workflowJob{
		buildSiteSignerJob: {Steps: []workflowStep{{Name: "Execute isolated signer with stdin key", Run: run, Env: env}}},
	}}}}
}

func boundedSignerRun(output string) string {
	return "signing_key=\"$APK_REPOSITORY_PRIVATE_KEY\"\n" +
		"unset APK_REPOSITORY_PRIVATE_KEY\n" +
		"printf '%s' \"$signing_key\" | ./verity ci site-publication signer-execute signer-plan.json " + output + "\n" +
		"unset signing_key"
}

func replaceSignerCommand(run, replacement string) string {
	return strings.Replace(run, `printf '%s' "$signing_key" | ./verity ci site-publication signer-execute signer-plan.json > signer-result.json`, replacement, 1)
}

func secureSignerRun() string {
	return "docker run --rm --interactive --network=none --read-only --user=65532:65532 --cap-drop=ALL --security-opt=no-new-privileges " + pinnedSignerImage
}
