package sitepublication

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/verity-org/verity/internal/ci/publication"
)

func TestValidateSignerPlan_rejects_identity_execution_and_cleanup_mutations(t *testing.T) {
	// Given
	publicationPlan, _ := validPlanAndManifest(t)
	request := signerRequest(t, &publicationPlan, t.TempDir())
	valid, err := BuildSignerPlan(request)
	require.NoError(t, err)
	tests := []struct {
		name    string
		mutate  func(*SignerPlan)
		wantErr error
	}{
		{name: "schema", mutate: func(plan *SignerPlan) { plan.SchemaVersion++ }, wantErr: ErrInvalidSignerPlan},
		{name: "digest", mutate: func(plan *SignerPlan) { plan.InputDigest = "bad" }, wantErr: ErrInvalidSignerPlan},
		{name: "signer source", mutate: func(plan *SignerPlan) { plan.SignerSourceSHA = "bad" }, wantErr: ErrInvalidSignerPlan},
		{name: "authorization plan", mutate: func(plan *SignerPlan) { plan.Authorization.PublicationPlanDigest = digestOf("f") }, wantErr: ErrInvalidSignerPlan},
		{name: "authorization mode", mutate: func(plan *SignerPlan) { plan.Authorization.Mode = publication.ModeSnapshot }, wantErr: ErrInvalidSignerPlan},
		{name: "image reference", mutate: func(plan *SignerPlan) { plan.ImageReference = "ghcr.io/sentinel/signer@" + string(digestOf("f")) }, wantErr: ErrInvalidSignerPlan},
		{name: "runtime", mutate: func(plan *SignerPlan) { plan.Execution.Runtime = "sentinel" }, wantErr: ErrInvalidSignerPlan},
		{name: "repository", mutate: func(plan *SignerPlan) { plan.Execution.Repository = "sentinel/repository" }, wantErr: ErrInvalidSignerPlan},
		{name: "path snapshot", mutate: func(plan *SignerPlan) { plan.Execution.PathSnapshot = "bad" }, wantErr: ErrInvalidSignerPlan},
		{name: "manifest path", mutate: func(plan *SignerPlan) { plan.Execution.ManifestPath = "../publication.json" }, wantErr: ErrInvalidSignerPlan},
		{
			name: "bootstrap delta paths",
			mutate: func(plan *SignerPlan) {
				plan.Execution.Mode = publication.ModeBootstrap
				plan.Authorization.Mode = publication.ModeBootstrap
			},
			wantErr: ErrInvalidSignerPlan,
		},
		{name: "delta base path", mutate: func(plan *SignerPlan) { plan.Execution.BaseAPKPath = "" }, wantErr: ErrInvalidSignerPlan},
		{name: "delta manifest path", mutate: func(plan *SignerPlan) { plan.Execution.DeltaManifestPath = "" }, wantErr: ErrInvalidSignerPlan},
		{
			name: "restore mode",
			mutate: func(plan *SignerPlan) {
				plan.Execution.Mode = publication.ModeRestore
				plan.Authorization.Mode = publication.ModeRestore
			},
			wantErr: ErrUnsupportedSignMode,
		},
		{
			name: "unknown mode",
			mutate: func(plan *SignerPlan) {
				plan.Execution.Mode = "sentinel"
				plan.Authorization.Mode = "sentinel"
			},
			wantErr: ErrInvalidSignerPlan,
		},
		{name: "overlapping data paths", mutate: func(plan *SignerPlan) { plan.Execution.OutputAPKPath = plan.Execution.PackagesPath }, wantErr: ErrInvalidSignerPlan},
		{
			name: "noncanonical cleanup directory",
			mutate: func(plan *SignerPlan) {
				plan.Cleanup.KeyDirectory += string(os.PathSeparator) + "."
			},
			wantErr: ErrInvalidSignerPlan,
		},
		{name: "cleanup key path", mutate: func(plan *SignerPlan) { plan.Cleanup.KeyPath += ".sentinel" }, wantErr: ErrInvalidSignerPlan},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			plan := cloneSignerPlan(t, &valid)
			test.mutate(&plan)

			// When
			validationErr := ValidateSignerPlan(&plan)

			// Then
			require.ErrorIs(t, validationErr, test.wantErr)
		})
	}
}

func TestValidateSignerPlan_rejects_nil_plan_and_preexisting_key_directory(t *testing.T) {
	t.Run("nil plan", func(t *testing.T) {
		// When
		err := ValidateSignerPlan(nil)

		// Then
		require.ErrorIs(t, err, ErrInvalidSignerPlan)
	})

	t.Run("preexisting key directory", func(t *testing.T) {
		// Given
		publicationPlan, _ := validPlanAndManifest(t)
		request := signerRequest(t, &publicationPlan, t.TempDir())
		plan, err := BuildSignerPlan(request)
		require.NoError(t, err)
		require.NoError(t, os.Mkdir(plan.Cleanup.KeyDirectory, 0o700))

		// When
		err = ValidateSignerPlan(&plan)

		// Then
		require.ErrorIs(t, err, ErrInvalidSignerPlan)
		assert.Contains(t, err.Error(), "already exists")
	})
}

func TestBuildSignerPlan_accepts_podman_plan_without_running_container(t *testing.T) {
	// Given
	publicationPlan, _ := validPlanAndManifest(t)
	request := signerRequest(t, &publicationPlan, t.TempDir())
	request.Runtime = string(ContainerRuntimePodman)

	// When
	plan, err := BuildSignerPlan(request)

	// Then
	require.NoError(t, err)
	assert.Equal(t, ContainerRuntimePodman, plan.Execution.Runtime)
	assert.Equal(t, trustedPodmanBinary, plan.Steps[0].Command.Name)
	assert.Equal(t, trustedPodmanBinary, plan.Steps[2].Command.Name)
}

func TestBuildSignerPlan_rejects_nil_and_unknown_runtime_requests(t *testing.T) {
	t.Run("nil request", func(t *testing.T) {
		// When
		plan, err := BuildSignerPlan(nil)

		// Then
		require.ErrorIs(t, err, ErrInvalidSignerPlan)
		assert.Equal(t, SignerPlan{}, plan)
	})

	t.Run("unknown runtime", func(t *testing.T) {
		// Given
		publicationPlan, _ := validPlanAndManifest(t)
		request := signerRequest(t, &publicationPlan, t.TempDir())
		request.Runtime = "sentinel-runtime"

		// When
		plan, err := BuildSignerPlan(request)

		// Then
		require.ErrorIs(t, err, ErrInvalidSignerPlan)
		assert.Equal(t, SignerPlan{}, plan)
	})
}

func cloneSignerPlan(t *testing.T, value *SignerPlan) SignerPlan {
	t.Helper()
	data, err := json.Marshal(value)
	require.NoError(t, err)
	var clone SignerPlan
	require.NoError(t, json.Unmarshal(data, &clone))
	return clone
}
