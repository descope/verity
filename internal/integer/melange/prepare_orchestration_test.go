package melange

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_Prepare_is_noop_when_spec_is_not_needed(t *testing.T) {
	// Given
	options := &BuildOptions{Paths: testPaths(t.TempDir())}

	// When
	err := Prepare(context.Background(), options)

	// Then
	require.NoError(t, err)
	assert.IsType(t, ExecRunner{}, options.Runner)
	assert.NotNil(t, options.Stdout)
	assert.NotNil(t, options.Stderr)
}

func Test_Prepare_propagates_staging_failure_before_key_generation(t *testing.T) {
	// Given
	runner := &orchestrationRunner{}
	options := &BuildOptions{
		Paths:  testPaths(t.TempDir()),
		Spec:   Spec{Bespoke: []string{"missing.yaml"}},
		Runner: runner,
	}

	// When
	err := Prepare(context.Background(), options)

	// Then
	require.ErrorContains(t, err, "read lock file")
	assert.Empty(t, runner.commands)
}

func Test_signingKeyPairExists_reports_absent_incomplete_and_nonregular_pairs(t *testing.T) {
	tests := []struct {
		name      string
		setup     func(t *testing.T, paths Paths)
		want      bool
		wantError error
	}{
		{name: "absent"},
		{
			name: "incomplete",
			setup: func(t *testing.T, paths Paths) {
				writeTestFile(t, filepath.Join(paths.WorkDir, "melange.rsa"), "private")
			},
			wantError: errIncompleteSigningKeyPair,
		},
		{
			name: "nonregular",
			setup: func(t *testing.T, paths Paths) {
				require.NoError(t, os.MkdirAll(filepath.Join(paths.WorkDir, "melange.rsa"), 0o755))
			},
			wantError: errNotRegularFile,
		},
		{
			name: "complete",
			setup: func(t *testing.T, paths Paths) {
				writeTestFile(t, filepath.Join(paths.WorkDir, "melange.rsa"), "private")
				writeTestFile(t, filepath.Join(paths.WorkDir, "melange.rsa.pub"), "public")
			},
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Given
			paths := testPaths(t.TempDir())
			if tt.setup != nil {
				tt.setup(t, paths)
			}

			// When
			got, err := signingKeyPairExists(&paths)

			// Then
			if tt.wantError != nil {
				require.ErrorIs(t, err, tt.wantError)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}
