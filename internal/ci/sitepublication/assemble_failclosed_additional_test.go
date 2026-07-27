package sitepublication

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/verity-org/verity/internal/ci/publication"
)

func TestAssembleSite_rejects_nil_canceled_and_manifest_identity_inputs(t *testing.T) {
	t.Run("nil request", func(t *testing.T) {
		// When
		result, err := AssembleSite(context.Background(), nil)

		// Then
		require.ErrorIs(t, err, ErrInvalidAssembly)
		assert.Equal(t, AssemblyResult{}, result)
	})

	t.Run("canceled context", func(t *testing.T) {
		// Given
		request := validAssemblyRequest(t)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		// When
		result, err := AssembleSite(ctx, request)

		// Then
		require.ErrorIs(t, err, context.Canceled)
		assert.Equal(t, AssemblyResult{}, result)
		assert.NoDirExists(t, request.OutputDir)
	})

	t.Run("invalid plan", func(t *testing.T) {
		// Given
		request := validAssemblyRequest(t)
		request.Plan.SchemaVersion = 0

		// When
		result, err := AssembleSite(context.Background(), request)

		// Then
		require.ErrorIs(t, err, ErrInvalidPlan)
		assert.Equal(t, AssemblyResult{}, result)
		assert.NoDirExists(t, request.OutputDir)
	})

	t.Run("manifest identity mismatch", func(t *testing.T) {
		// Given
		request := validAssemblyRequest(t)
		request.Manifest.SignerDigest = digestOf("e")

		// When
		result, err := AssembleSite(context.Background(), request)

		// Then
		require.ErrorIs(t, err, ErrInvalidAssembly)
		assert.Equal(t, AssemblyResult{}, result)
		assert.NoDirExists(t, request.OutputDir)
	})
}

func TestValidateAssemblyInputs_requires_safe_mode_specific_directories(t *testing.T) {
	tests := []struct {
		name    string
		request AssembleRequest
	}{
		{name: "empty output", request: AssembleRequest{Plan: PublicationPlan{Mode: publication.ModeBootstrap}, SignedAPKDir: "signed"}},
		{name: "dot output", request: AssembleRequest{Plan: PublicationPlan{Mode: publication.ModeBootstrap}, OutputDir: ".", SignedAPKDir: "signed"}},
		{name: "root output", request: AssembleRequest{Plan: PublicationPlan{Mode: publication.ModeBootstrap}, OutputDir: string(filepath.Separator), SignedAPKDir: "signed"}},
		{name: "bootstrap base", request: AssembleRequest{Plan: PublicationPlan{Mode: publication.ModeBootstrap}, BaseDir: "base", OutputDir: "output", SignedAPKDir: "signed"}},
		{name: "snapshot without base", request: AssembleRequest{Plan: PublicationPlan{Mode: publication.ModeSnapshot}, OutputDir: "output", SignedAPKDir: "signed"}},
		{name: "delta without base", request: AssembleRequest{Plan: PublicationPlan{Mode: publication.ModeDelta}, OutputDir: "output", SignedAPKDir: "signed"}},
		{name: "restore", request: AssembleRequest{Plan: PublicationPlan{Mode: publication.ModeRestore}, BaseDir: "base", OutputDir: "output", SignedAPKDir: "signed"}},
		{name: "missing signed repository", request: AssembleRequest{Plan: PublicationPlan{Mode: publication.ModeBootstrap}, OutputDir: "output"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// When
			err := validateAssemblyInputs(&test.request)

			// Then
			require.ErrorIs(t, err, ErrInvalidAssembly)
		})
	}
}

func TestPlanOverlays_rejects_unsafe_missing_and_reserved_sources(t *testing.T) {
	t.Run("empty name", func(t *testing.T) {
		// When
		mutations, err := planOverlays([]Overlay{{SourceDir: t.TempDir()}})

		// Then
		require.ErrorIs(t, err, ErrInvalidAssembly)
		assert.Nil(t, mutations)
	})

	t.Run("unsafe destination", func(t *testing.T) {
		// Given
		source := t.TempDir()
		writeSiteFile(t, source, "index.html", "sentinel")

		// When
		mutations, err := planOverlays([]Overlay{{Name: "sentinel", SourceDir: source, Destination: "../escape"}})

		// Then
		require.Error(t, err)
		assert.Nil(t, mutations)
	})

	t.Run("missing source", func(t *testing.T) {
		// When
		mutations, err := planOverlays([]Overlay{{Name: "sentinel", SourceDir: filepath.Join(t.TempDir(), "missing")}})

		// Then
		require.ErrorContains(t, err, `overlay "sentinel"`)
		assert.Nil(t, mutations)
	})

	for _, reserved := range []string{"apk/demo.apk", ".verity/metadata.json"} {
		t.Run("reserved "+reserved, func(t *testing.T) {
			// Given
			source := t.TempDir()
			writeSiteFile(t, source, reserved, "sentinel")

			// When
			mutations, err := planOverlays([]Overlay{{Name: "sentinel", SourceDir: source}})

			// Then
			require.ErrorIs(t, err, ErrInvalidAssembly)
			assert.Nil(t, mutations)
		})
	}
}

func TestAssembleSite_removes_stage_when_signed_repository_copy_fails(t *testing.T) {
	// Given
	request := validAssemblyRequest(t)
	request.SignedAPKDir = filepath.Join(filepath.Dir(request.SignedAPKDir), "missing-signed-repository")
	parent := filepath.Dir(request.OutputDir)

	// When
	result, err := AssembleSite(context.Background(), request)

	// Then
	require.ErrorContains(t, err, "copy signed APK repository")
	assert.Equal(t, AssemblyResult{}, result)
	assert.NoDirExists(t, request.OutputDir)
	stages, globErr := filepath.Glob(filepath.Join(parent, ".site-publication-stage-*"))
	require.NoError(t, globErr)
	assert.Empty(t, stages)
}

func TestAssembleSite_builds_bootstrap_without_base_site(t *testing.T) {
	// Given
	planRequest := validPlanRequest(t)
	planRequest.Manifest.Mode = publication.ModeBootstrap
	planRequest.Manifest.PreviousManifestDigest = ""
	planRequest.ExpectedMode = publication.ModeBootstrap
	planRequest.PreviousManifest = nil
	planRequest.AuthorizeBootstrap = true
	plan, err := CreatePlan(context.Background(), planRequest)
	require.NoError(t, err)
	root := t.TempDir()
	signed := filepath.Join(root, "signed")
	writeSiteFile(t, signed, "x86_64/demo.apk", "sentinel-x86")
	writeSiteFile(t, signed, "aarch64/demo.apk", "sentinel-arm")
	output := filepath.Join(root, "output")

	// When
	result, err := AssembleSite(context.Background(), &AssembleRequest{
		Plan: plan, Manifest: planRequest.Manifest, SignedAPKDir: signed, OutputDir: output,
	})

	// Then
	require.NoError(t, err)
	assert.Equal(t, plan.ManifestDigest, result.ManifestDigest)
	assert.FileExists(t, filepath.Join(output, PublicationManifestPath))
	assert.Equal(t, "sentinel-x86", readSiteFile(t, output, "apk/x86_64/demo.apk"))
}

func validAssemblyRequest(t *testing.T) *AssembleRequest {
	t.Helper()
	plan, manifest := validPlanAndManifest(t)
	root := t.TempDir()
	base := filepath.Join(root, "base")
	writeSiteFile(t, base, "index.html", "sentinel-base")
	sealSiteFixture(t, base, validPlanRequest(t).PreviousManifest)
	signed := filepath.Join(root, "signed")
	writeSiteFile(t, signed, "x86_64/demo.apk", "sentinel-x86")
	writeSiteFile(t, signed, "aarch64/demo.apk", "sentinel-arm")
	return &AssembleRequest{
		Plan: plan, Manifest: manifest, BaseDir: base, SignedAPKDir: signed,
		OutputDir: filepath.Join(root, "output"),
	}
}
