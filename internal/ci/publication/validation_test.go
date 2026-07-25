package publication

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestValidate_accepts_exact_delta_when_identity_components_CAS_and_ancestry_match(t *testing.T) {
	// Given a previous publication and an exact successor from approved producers.
	previous := testManifest(ModeBootstrap, SourceSHA("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"))
	previous.RunID = 41
	previous.RunAttempt = 2
	previous.BatchID = "41-2"
	previousDigest, err := DigestManifest(&previous)
	require.NoError(t, err)
	candidate := testManifest(ModeDelta, SourceSHA("bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"))
	candidate.PreviousManifestDigest = previousDigest
	runner := &fakeRunner{result: CommandResult{ExitCode: 0}}

	// When the candidate is validated against the publication identity and state.
	err = Validate(context.Background(), &candidate, &ValidationOptions{
		ExpectedIdentity: ProducerIdentity{
			SourceSHA:  candidate.SourceSHA,
			RunID:      candidate.RunID,
			RunAttempt: candidate.RunAttempt,
			BatchID:    candidate.BatchID,
		},
		ExpectedMode:         candidate.Mode,
		ExpectedComponents:   candidate.Components,
		ExpectedSignerDigest: candidate.SignerDigest,
		PublicationSHA:       SourceSHA("cccccccccccccccccccccccccccccccccccccccc"),
		PreviousManifest:     &previous,
		Runner:               runner,
	})

	// Then validation succeeds only after both previous-state and producer ancestry checks.
	require.NoError(t, err)
	require.Equal(t, []Command{
		{Name: "git", Args: []string{"merge-base", "--is-ancestor", string(previous.SourceSHA), string(candidate.SourceSHA)}},
		{Name: "git", Args: []string{"merge-base", "--is-ancestor", string(candidate.SourceSHA), "cccccccccccccccccccccccccccccccccccccccc"}},
	}, runner.calls)
}

func TestValidate_rejects_wrong_producer_identity_and_components(t *testing.T) {
	// Given an exact delta publication and its expected producer contract.
	previous, candidate, options := testDeltaValidation(t)
	tests := []struct {
		name    string
		mutate  func(*Manifest, *ValidationOptions)
		wantErr error
	}{
		{
			name: "stale source SHA",
			mutate: func(manifest *Manifest, _ *ValidationOptions) {
				manifest.SourceSHA = "dddddddddddddddddddddddddddddddddddddddd"
			},
			wantErr: ErrIdentityMismatch,
		},
		{
			name: "wrong run attempt",
			mutate: func(manifest *Manifest, _ *ValidationOptions) {
				manifest.RunAttempt = 4
				manifest.BatchID = "42-4"
			},
			wantErr: ErrIdentityMismatch,
		},
		{
			name: "wrong expected batch",
			mutate: func(_ *Manifest, options *ValidationOptions) {
				options.ExpectedIdentity.BatchID = "42-4"
			},
			wantErr: ErrIdentityMismatch,
		},
		{
			name: "wrong mode",
			mutate: func(manifest *Manifest, _ *ValidationOptions) {
				manifest.Mode = ModeSnapshot
			},
			wantErr: ErrIdentityMismatch,
		},
		{
			name: "wrong artifact name",
			mutate: func(manifest *Manifest, _ *ValidationOptions) {
				manifest.Components[0].ArtifactName = "integer-publication-other"
				manifest.APKOperations[0].ArtifactName = "integer-publication-other"
			},
			wantErr: ErrComponentMismatch,
		},
		{
			name: "wrong artifact digest",
			mutate: func(manifest *Manifest, _ *ValidationOptions) {
				manifest.Components[0].ArtifactDigest = "sha256:4444444444444444444444444444444444444444444444444444444444444444"
				manifest.APKOperations[0].ArtifactDigest = manifest.Components[0].ArtifactDigest
			},
			wantErr: ErrComponentMismatch,
		},
		{
			name: "wrong workflow",
			mutate: func(manifest *Manifest, _ *ValidationOptions) {
				manifest.Components[0].Workflow = ".github/workflows/other.yaml"
			},
			wantErr: ErrComponentMismatch,
		},
		{
			name: "wrong event",
			mutate: func(manifest *Manifest, _ *ValidationOptions) {
				manifest.Components[0].Event = EventPush
			},
			wantErr: ErrComponentMismatch,
		},
		{
			name: "wrong result",
			mutate: func(manifest *Manifest, _ *ValidationOptions) {
				manifest.Components[0].Result = "cancelled"
			},
			wantErr: ErrInvalidManifest,
		},
		{
			name: "wrong signer digest",
			mutate: func(manifest *Manifest, _ *ValidationOptions) {
				manifest.SignerDigest = "sha256:5555555555555555555555555555555555555555555555555555555555555555"
			},
			wantErr: ErrSignerMismatch,
		},
		{
			name: "missing component",
			mutate: func(_ *Manifest, options *ValidationOptions) {
				options.ExpectedComponents = append(options.ExpectedComponents, Component{
					Name: "charts", Kind: ComponentKindGeneric, ArtifactName: "charts-publication-42-3",
					ArtifactDigest: "sha256:6666666666666666666666666666666666666666666666666666666666666666",
					Workflow:       ".github/workflows/chart-gen.yaml",
					Event:          EventWorkflowCall, Result: ResultSuccess,
				})
			},
			wantErr: ErrComponentMismatch,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// When one exact producer field is changed.
			changed := candidate
			changed.Components = append([]Component(nil), candidate.Components...)
			changed.APKOperations = append([]APKOperation(nil), candidate.APKOperations...)
			testOptions := options
			testOptions.ExpectedComponents = append([]Component(nil), options.ExpectedComponents...)
			tt.mutate(&changed, &testOptions)
			testOptions.PreviousManifest = &previous
			testOptions.Runner = &fakeRunner{result: CommandResult{ExitCode: 0}}
			err := Validate(context.Background(), &changed, &testOptions)

			// Then publication fails closed on the typed contract error.
			require.ErrorIs(t, err, tt.wantErr)
		})
	}
}

func testDeltaValidation(t *testing.T) (previous, candidate Manifest, options ValidationOptions) {
	t.Helper()
	previous = testManifest(ModeBootstrap, "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	previous.RunID = 41
	previous.RunAttempt = 2
	previous.BatchID = "41-2"
	previousDigest, err := DigestManifest(&previous)
	require.NoError(t, err)
	candidate = testManifest(ModeDelta, "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb")
	candidate.PreviousManifestDigest = previousDigest
	options = exactOptions(&candidate)
	return previous, candidate, options
}

func exactOptions(manifest *Manifest) ValidationOptions {
	return ValidationOptions{
		ExpectedIdentity: ProducerIdentity{
			SourceSHA: manifest.SourceSHA, RunID: manifest.RunID,
			RunAttempt: manifest.RunAttempt, BatchID: manifest.BatchID,
		},
		ExpectedMode:         manifest.Mode,
		ExpectedComponents:   append([]Component(nil), manifest.Components...),
		ExpectedSignerDigest: manifest.SignerDigest,
		PublicationSHA:       "cccccccccccccccccccccccccccccccccccccccc",
	}
}

func testManifest(mode Mode, sourceSHA SourceSHA) Manifest {
	return Manifest{
		SchemaVersion: SchemaVersion,
		SourceSHA:     sourceSHA,
		RunID:         42,
		RunAttempt:    3,
		BatchID:       "42-3",
		Mode:          mode,
		Components: []Component{
			{
				Name:           "integer",
				Kind:           ComponentKindAPK,
				Architecture:   ArchitectureX8664,
				ArtifactName:   "integer-publication-42-3",
				ArtifactDigest: "sha256:1111111111111111111111111111111111111111111111111111111111111111",
				Workflow:       ".github/workflows/integer-orchestrator.yaml",
				Event:          EventWorkflowCall,
				Result:         ResultSuccess,
			},
		},
		SignerDigest: "sha256:2222222222222222222222222222222222222222222222222222222222222222",
		APKOperations: []APKOperation{
			{
				Action:         APKUpsert,
				Architecture:   ArchitectureX8664,
				PackageName:    "demo",
				ArtifactName:   "integer-publication-42-3",
				ArtifactDigest: "sha256:1111111111111111111111111111111111111111111111111111111111111111",
			},
		},
	}
}

type fakeRunner struct {
	result CommandResult
	err    error
	calls  []Command
}

func (f *fakeRunner) Run(_ context.Context, command Command) (CommandResult, error) {
	f.calls = append(f.calls, command)
	return f.result, f.err
}
