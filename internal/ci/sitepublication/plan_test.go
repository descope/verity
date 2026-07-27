package sitepublication

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/verity-org/verity/internal/ci/publication"
)

func TestCreatePlan_accepts_exact_delta_and_is_canonical(t *testing.T) {
	// Given an exact producer set, current manifest, and pinned signer lock.
	request := validPlanRequest(t)

	// When the publication plan is created and round-tripped.
	plan, err := CreatePlan(context.Background(), request)
	require.NoError(t, err)
	encoded, err := MarshalPlanCanonical(&plan)
	require.NoError(t, err)
	parsed, err := ParsePlanCanonical(encoded)

	// Then only stable machine fields are emitted.
	require.NoError(t, err)
	assert.Equal(t, plan, parsed)
	assert.Equal(t, request.Manifest.PreviousManifestDigest, plan.PreviousManifestDigest)
	assert.Equal(t, request.SignerLock.Reference(), plan.SignerReference)
	assert.Equal(t, publication.SourceSHA(testSignerSourceSHA), plan.SignerSourceSHA)
	assert.NotEmpty(t, plan.PlanDigest)
}

func TestCreatePlan_rejects_mixed_incomplete_stale_CAS_replay_and_wrong_signer(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*testing.T, *PlanRequest)
		wantErr error
	}{
		{
			name: "mixed producer workflow",
			mutate: func(_ *testing.T, request *PlanRequest) {
				request.ExpectedComponents[0].Workflow = ".github/workflows/attacker.yaml"
			},
			wantErr: publication.ErrComponentMismatch,
		},
		{
			name: "incomplete producer set",
			mutate: func(_ *testing.T, request *PlanRequest) {
				request.ExpectedComponents = request.ExpectedComponents[:1]
			},
			wantErr: publication.ErrComponentMismatch,
		},
		{
			name: "stale run",
			mutate: func(t *testing.T, request *PlanRequest) {
				previous := testManifest(publication.ModeBootstrap, testPreviousSHA, 43, 1)
				digest, err := publication.DigestManifest(&previous)
				require.NoError(t, err)
				request.PreviousManifest = &previous
				request.Manifest.PreviousManifestDigest = digest
			},
			wantErr: publication.ErrStaleRunAttempt,
		},
		{
			name: "stale CAS base",
			mutate: func(_ *testing.T, request *PlanRequest) {
				request.Manifest.PreviousManifestDigest = digestOf("f")
			},
			wantErr: publication.ErrCASMismatch,
		},
		{
			name: "replayed run attempt",
			mutate: func(t *testing.T, request *PlanRequest) {
				previous := testManifest(publication.ModeBootstrap, testSourceSHA, 42, 3)
				digest, err := publication.DigestManifest(&previous)
				require.NoError(t, err)
				request.PreviousManifest = &previous
				request.Manifest.PreviousManifestDigest = digest
			},
			wantErr: publication.ErrReplay,
		},
		{
			name: "wrong signer digest",
			mutate: func(_ *testing.T, request *PlanRequest) {
				request.SignerLock.Digest = string(digestOf("e"))
			},
			wantErr: publication.ErrSignerMismatch,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Given one hostile or stale input mutation.
			request := validPlanRequest(t)
			test.mutate(t, request)

			// When a plan is requested.
			_, err := CreatePlan(context.Background(), request)

			// Then publication fails closed at the typed boundary.
			require.Error(t, err)
			assert.True(t, errors.Is(err, test.wantErr), "error %v does not match %v", err, test.wantErr)
		})
	}
}

func TestCreatePlan_requires_manual_bootstrap_and_restore_authorization(t *testing.T) {
	tests := []struct {
		name    string
		mode    publication.Mode
		wantErr error
	}{
		{name: "bootstrap", mode: publication.ModeBootstrap, wantErr: publication.ErrBootstrapUnauthorized},
		{name: "restore", mode: publication.ModeRestore, wantErr: publication.ErrRestoreUnauthorized},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Given a structurally valid manual mode without protected authorization.
			request := validPlanRequest(t)
			request.Manifest.Mode = test.mode
			request.ExpectedMode = test.mode
			if test.mode == publication.ModeBootstrap {
				request.Manifest.PreviousManifestDigest = ""
				request.PreviousManifest = nil
			}

			// When the plan is created.
			_, err := CreatePlan(context.Background(), request)

			// Then the explicit authorization gate blocks it.
			require.ErrorIs(t, err, test.wantErr)
		})
	}
}
