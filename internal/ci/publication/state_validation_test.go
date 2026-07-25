package publication

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestValidate_rejects_stale_base_and_replay(t *testing.T) {
	// Given a valid delta successor.
	previous, candidate, options := testDeltaValidation(t)
	options.PreviousManifest = &previous
	options.Runner = &fakeRunner{result: CommandResult{ExitCode: 0}}

	// When the candidate names a stale previous digest.
	stale := candidate
	stale.PreviousManifestDigest = "sha256:ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff"
	err := Validate(context.Background(), &stale, &options)

	// Then CAS rejects the stale state.
	require.ErrorIs(t, err, ErrCASMismatch)

	// When a changed payload reuses the previous run identity.
	replay := candidate
	replay.RunID = previous.RunID
	replay.RunAttempt = previous.RunAttempt
	replay.BatchID = previous.BatchID
	replay.SourceSHA = previous.SourceSHA
	options.ExpectedIdentity = ProducerIdentity{
		SourceSHA: replay.SourceSHA, RunID: replay.RunID,
		RunAttempt: replay.RunAttempt, BatchID: replay.BatchID,
	}
	err = Validate(context.Background(), &replay, &options)

	// Then replay is rejected independently of payload bytes.
	require.ErrorIs(t, err, ErrReplay)
}

func TestValidate_enforces_monotonic_run_attempts(t *testing.T) {
	// Given current state at run 41 attempt 2 for one source commit.
	previous := testManifest(ModeBootstrap, "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	previous.RunID = 41
	previous.RunAttempt = 2
	previous.BatchID = "41-2"
	previousDigest, err := DigestManifest(&previous)
	require.NoError(t, err)

	tests := []struct {
		name    string
		runID   RunID
		attempt RunAttempt
		wantErr error
	}{
		{name: "older run", runID: 40, attempt: 9, wantErr: ErrStaleRunAttempt},
		{name: "older attempt in same run", runID: 41, attempt: 1, wantErr: ErrStaleRunAttempt},
		{name: "newer attempt in same run", runID: 41, attempt: 3},
		{name: "newer run restarts attempt", runID: 42, attempt: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			candidate := testManifest(ModeDelta, previous.SourceSHA)
			candidate.RunID = tt.runID
			candidate.RunAttempt = tt.attempt
			candidate.BatchID = BatchID(fmt.Sprintf("%d-%d", tt.runID, tt.attempt))
			candidate.PreviousManifestDigest = previousDigest
			options := exactOptions(&candidate)
			options.PreviousManifest = &previous
			options.Runner = &fakeRunner{result: CommandResult{ExitCode: 0}}

			// When the candidate is validated against current publication state.
			err := Validate(context.Background(), &candidate, &options)

			// Then stale attempts fail while valid forward progress remains accepted.
			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)
		})
	}
}

func TestValidate_requires_explicit_bootstrap_and_restore_authorization(t *testing.T) {
	// Given a first publication with no current state.
	bootstrap := testManifest(ModeBootstrap, "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb")
	options := exactOptions(&bootstrap)
	options.Runner = &fakeRunner{result: CommandResult{ExitCode: 0}}

	// When bootstrap authorization is absent.
	err := Validate(context.Background(), &bootstrap, &options)

	// Then first publication is denied.
	require.ErrorIs(t, err, ErrBootstrapUnauthorized)

	// When bootstrap is explicitly authorized.
	options.AuthorizeBootstrap = true
	err = Validate(context.Background(), &bootstrap, &options)

	// Then it is accepted.
	require.NoError(t, err)

	// Given an exact restore over an existing state.
	previous, restore, restoreOptions := testDeltaValidation(t)
	restore.Mode = ModeRestore
	restoreOptions.ExpectedMode = ModeRestore
	restoreOptions.PreviousManifest = &previous
	restoreOptions.Runner = &fakeRunner{result: CommandResult{ExitCode: 0}}

	// When restore authorization is absent, then it is denied.
	err = Validate(context.Background(), &restore, &restoreOptions)
	require.ErrorIs(t, err, ErrRestoreUnauthorized)

	// When restore is explicitly authorized, then it is accepted.
	restoreOptions.AuthorizeRestore = true
	err = Validate(context.Background(), &restore, &restoreOptions)
	require.NoError(t, err)
}

func TestValidate_uses_exit_status_for_ancestry_and_honors_repeated_cancellation(t *testing.T) {
	// Given a valid candidate and a runner printing a misleading success message with exit 1.
	previous, candidate, options := testDeltaValidation(t)
	misleading := &fakeRunner{result: CommandResult{Stdout: []byte("ancestor=true\n"), ExitCode: 1}}
	options.PreviousManifest = &previous
	options.Runner = misleading

	// When ancestry is checked.
	err := Validate(context.Background(), &candidate, &options)

	// Then exit status, not output text, rejects the candidate.
	require.ErrorIs(t, err, ErrNotAncestor)

	// Given an already-cancelled context.
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	runner := &fakeRunner{result: CommandResult{ExitCode: 0}}
	options.Runner = runner

	// When validation is interrupted repeatedly.
	for range 3 {
		err = Validate(cancelled, &candidate, &options)
		require.ErrorIs(t, err, context.Canceled)
	}

	// Then no command starts after cancellation.
	require.Empty(t, runner.calls)
}

func TestValidate_honors_cancellation_during_each_ancestry_command(t *testing.T) {
	// Given a valid candidate whose runner is interrupted after returning output.
	previous, candidate, options := testDeltaValidation(t)
	options.PreviousManifest = &previous

	// When each ancestry attempt cancels while the command is in flight.
	for range 3 {
		ctx, cancel := context.WithCancel(context.Background())
		runner := &cancelingRunner{cancel: cancel}
		options.Runner = runner
		err := Validate(ctx, &candidate, &options)

		// Then cancellation wins over the runner's misleading zero exit status.
		require.ErrorIs(t, err, context.Canceled)
		require.Equal(t, 1, runner.calls)
	}
}

type cancelingRunner struct {
	cancel context.CancelFunc
	calls  int
}

func (r *cancelingRunner) Run(_ context.Context, _ Command) (CommandResult, error) {
	r.calls++
	r.cancel()
	return CommandResult{Stdout: []byte("ancestor=true\n"), ExitCode: 0}, nil
}
