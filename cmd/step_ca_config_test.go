package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/verity-org/verity/internal/integer/apkindex"
	intconfig "github.com/verity-org/verity/internal/integer/config"
)

func Test_StepCA_uses_fixed_bespoke_package_for_only_variant(t *testing.T) {
	// Given: the checked-in Step CA image definition.
	def, err := intconfig.LoadImage(filepath.Join("..", "images", "step-ca.yaml"))
	require.NoError(t, err)

	// When: the only supported image variant is resolved.
	tmpl := def.Types["default"]

	// Then: it selects only the local rebuild and preserves the runtime contract.
	require.Len(t, def.Types, 1)
	require.NotNil(t, tmpl.Melange)
	require.Equal(t, "step-ca.yaml", tmpl.Melange.Bespoke.First())
	require.Equal(t, []string{"step-ca"}, tmpl.Packages)
	require.Equal(t, "/usr/bin/step-ca", tmpl.Entrypoint)
}

func Test_StepCA_recipe_pins_fixed_source_runtime_and_provenance(t *testing.T) {
	// Given: the bespoke package recipe selected by the image.
	recipe, err := os.ReadFile(filepath.Join("..", "packages", "bespoke", "step-ca.yaml"))
	require.NoError(t, err)
	text := string(recipe)

	// When: its source, dependency fixes, smoke tests, and license metadata are inspected.

	// Then: revision r15 rebuilds immutable Apache-2.0 v0.30.2 with every recorded fix after public r10.
	require.Contains(t, text, "Adapted from wolfi-dev/os@b9dcc7dc733fd3100bfd7974ea0725188abe18b2")
	require.Contains(t, text, "name: step-ca")
	require.Contains(t, text, "version: \"0.30.2\"")
	require.Contains(t, text, "epoch: 15")
	require.Contains(t, text, "license: Apache-2.0")
	require.Contains(t, text, "repository: https://github.com/smallstep/certificates")
	require.Contains(t, text, "tag: v${{package.version}}")
	require.Contains(t, text, "expected-commit: 6e8ec61405239cf3f37b2bbf260a587b7d2e4e31")
	require.Contains(t, text, "go-1.26=1.26.5-r1")
	require.Contains(t, text, "go.opentelemetry.io/otel@v1.44.0")
	require.Contains(t, text, "go.opentelemetry.io/otel/metric@v1.44.0")
	require.Contains(t, text, "go.opentelemetry.io/otel/sdk@v1.44.0")
	require.Contains(t, text, "go.opentelemetry.io/otel/sdk/metric@v1.44.0")
	require.Contains(t, text, "go.opentelemetry.io/otel/trace@v1.44.0")
	require.Contains(t, text, "Verify the complete fixed module graph")
	require.Contains(t, text, "go list -m all")
	require.Contains(t, text, "Verify linked OpenTelemetry modules")
	require.Contains(t, text, "go version -m")
	require.Contains(t, text, "step-ca --version")
	require.Contains(t, text, "step-ca --help")
	require.Contains(t, text, "step ca init")
	require.Contains(t, text, "step ca health")
	require.Contains(t, text, "apk info --who-owns /usr/bin/step-ca")
	require.Contains(t, text, "apk info --license step-ca")
	require.Contains(t, text, "usr/share/licenses/${{package.name}}/LICENSE")
	require.Contains(t, text, "spdx.json")
	require.Contains(t, text, ".versionInfo == \"${{package.full-version}}\"")
	require.NotContains(t, text, ".versionInfo == \"0.30.2-r15\"")
	require.Contains(t, text, "licenseDeclared == \"Apache-2.0\"")
}

func Test_StepCA_resolves_only_fixed_local_package(t *testing.T) {
	// Given: the checked-in image template and package produced by the bespoke recipe.
	def, err := intconfig.LoadImage(filepath.Join("..", "images", "step-ca.yaml"))
	require.NoError(t, err)
	tmpl := def.Types["default"]

	// When: the local Melange artifact is pinned for the image build.
	err = pinLocalPackageVersions(&tmpl, "0", []apkindex.Package{
		{Name: "step-ca", Version: "0.30.2-r15"},
	})

	// Then: apko can only select the approved fixed locally built revision.
	require.NoError(t, err)
	require.Equal(t, []string{"step-ca=0.30.2-r15@local"}, tmpl.Packages)
}

func Test_StepCA_runs_package_smoke_in_native_dual_arch_workflows(t *testing.T) {
	// Given: the shared package-test script, reusable image workflow, and pull-request smoke workflow.
	testScript, err := os.ReadFile(filepath.Join("..", ".github", "scripts", "test-step-ca-package.sh"))
	require.NoError(t, err)
	prWorkflow, err := os.ReadFile(filepath.Join("..", ".github", "workflows", "pr-test.yaml"))
	require.NoError(t, err)

	// When: their Step CA package-test gates are inspected.
	scriptText := string(testScript)
	prText := string(prWorkflow)
	buildText := readGeneratedWorkflowFixture(t, "integer-build-image-reusable.yaml")

	// Then: both native architecture paths call one bounded checked-in Melange test entrypoint.
	require.Contains(t, scriptText, "case \"$arch\" in")
	require.Contains(t, scriptText, "timeout --signal=TERM --kill-after=1m 30m melange test")
	require.Contains(t, scriptText, "--pipeline-dirs melange-work/pipelines")
	require.Contains(t, scriptText, "melange-work/specs/step-ca.yaml/build.yaml")
	workingDirectoryIndex := strings.Index(scriptText, "cd \"$workspace\"")
	testCommandIndex := strings.Index(scriptText, "timeout --signal=TERM --kill-after=1m 30m melange test")
	require.NotEqual(t, -1, workingDirectoryIndex)
	require.Less(t, workingDirectoryIndex, testCommandIndex)
	requireIntegerImageReusablePackageGate(t, buildText)
	require.Contains(t, prText, "./verity ci pr-test integer-batch")
	require.NotContains(t, prText, "test-step-ca-package.sh")
}
